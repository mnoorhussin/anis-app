// Package crawl walks a customer's website and returns its readable pages.
//
// Politeness is not optional here. We fetch other people's servers on their
// customers' behalf, under a User-Agent that names us, so a crawler that
// ignores robots.txt or hammers a small shop's site gets AnisBot blocked
// network-wide — which breaks the feature for every customer, not just the one
// whose site we were rude to.
package crawl

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/mnoorhussin/anis-app/pocketbase/internal/extract"
	"github.com/mnoorhussin/anis-app/pocketbase/internal/fetch"
)

// Defaults.
const (
	DefaultMaxPages = 200
	// DefaultDelay between requests to the same host. Slow enough not to look
	// like an attack to a small shared-hosting site.
	DefaultDelay = 500 * time.Millisecond
	// MaxDelay caps what a site's own Crawl-delay can impose on us. A site
	// asking for 30s would otherwise stall a crawl for hours.
	MaxDelay = 5 * time.Second
)

// Doc is one successfully crawled page.
type Doc struct {
	URL   string
	Title string
	Text  string
}

// Result is the outcome of a crawl.
type Result struct {
	Docs []Doc
	// Skipped explains pages that were not indexed, so the dashboard can show
	// a customer why a page they expected is missing.
	Skipped []Skip
	// RobotsBlocked is true when robots.txt forbade the starting URL itself,
	// which is worth saying plainly rather than reporting "0 pages found".
	RobotsBlocked bool
}

// Skip is one page that was not indexed, and why.
type Skip struct {
	URL    string
	Reason string
}

// Options configure a crawl.
type Options struct {
	MaxPages int
	Delay    time.Duration
	Client   *fetch.Client
}

// Site crawls from a starting URL and returns the pages it could read.
//
// Breadth-first, same-host only, robots-respecting, with a hard page cap. The
// cap is a product decision as much as a technical one: it bounds embedding
// spend per source and keeps a crawl finishing in a time a customer will wait.
// The caller is responsible for validating the starting URL with
// fetch.CheckURL before queuing a crawl — that check exists to give a customer
// an immediate, comprehensible error at submit time. It is NOT repeated here,
// because it is not the security boundary: DNS can change between submit and
// crawl, so what actually protects us is the guard in the dialer, which
// re-checks every connection including each redirect hop.
func Site(ctx context.Context, start string, opts Options) (*Result, error) {
	root, err := url.Parse(start)
	if err != nil {
		return nil, fmt.Errorf("crawl: could not parse the starting URL: %w", err)
	}
	if root.Scheme != "http" && root.Scheme != "https" {
		return nil, fetch.ErrUnsupportedScheme
	}

	if opts.MaxPages <= 0 {
		opts.MaxPages = DefaultMaxPages
	}
	if opts.Client == nil {
		return nil, fmt.Errorf("crawl: no fetch client")
	}
	delay := opts.Delay
	if delay <= 0 {
		delay = DefaultDelay
	}

	robots := fetchRobots(ctx, opts.Client, root)
	if robots.CrawlDelay > 0 {
		d := time.Duration(robots.CrawlDelay * float64(time.Second))
		if d > delay {
			delay = min(d, MaxDelay)
		}
	}

	result := &Result{}
	if !robots.Allowed(root.Path) {
		result.RobotsBlocked = true
		return result, nil
	}

	seen := map[string]bool{normalise(root): true}
	queue := []string{root.String()}

	for len(queue) > 0 && len(result.Docs) < opts.MaxPages {
		if err := ctx.Err(); err != nil {
			return result, err
		}

		current := queue[0]
		queue = queue[1:]

		res, err := opts.Client.Get(ctx, current)
		if err != nil {
			result.Skipped = append(result.Skipped, Skip{current, shortError(err)})
			time.Sleep(delay)
			continue
		}
		if res.StatusCode >= 400 {
			result.Skipped = append(result.Skipped, Skip{current, fmt.Sprintf("HTTP %d", res.StatusCode)})
			time.Sleep(delay)
			continue
		}

		ct := strings.ToLower(res.ContentType)
		switch {
		case strings.Contains(ct, "html"), ct == "":
			u, parseErr := url.Parse(res.URL)
			if parseErr != nil {
				u = root
			}
			page, extractErr := extract.HTML(res.Body, u)
			if extractErr != nil {
				result.Skipped = append(result.Skipped, Skip{current, "could not parse the page"})
				break
			}
			// A page with almost no text is a redirect stub, a login wall or a
			// JavaScript shell. Indexing it adds noise and no answers.
			if len([]rune(page.Text)) < 120 {
				result.Skipped = append(result.Skipped, Skip{current, "not enough text to index"})
			} else {
				title := page.Title
				if title == "" {
					title = current
				}
				result.Docs = append(result.Docs, Doc{URL: res.URL, Title: title, Text: page.Text})
			}

			for _, link := range page.Links {
				if len(seen) >= opts.MaxPages*4 {
					break // bound the frontier, not just the fetches
				}
				lu, err := url.Parse(link)
				if err != nil || !extract.SameSite(root, lu) {
					continue
				}
				if !robots.Allowed(lu.Path) {
					continue
				}
				if looksLikeAsset(lu.Path) {
					continue
				}
				key := normalise(lu)
				if seen[key] {
					continue
				}
				seen[key] = true
				queue = append(queue, lu.String())
			}

		case strings.Contains(ct, "pdf"):
			text, pdfErr := extract.PDF(res.Body)
			if pdfErr != nil {
				// The reason is shown to the customer. For Arabic PDFs it
				// explains that the file stores text in display order and
				// cannot be read back — which is actionable, unlike "failed".
				result.Skipped = append(result.Skipped, Skip{current, shortError(pdfErr)})
				break
			}
			result.Docs = append(result.Docs, Doc{URL: res.URL, Title: titleFromURL(res.URL), Text: text})

		default:
			result.Skipped = append(result.Skipped, Skip{current, "not a page or a PDF"})
		}

		time.Sleep(delay)
	}

	return result, nil
}

func fetchRobots(ctx context.Context, client *fetch.Client, root *url.URL) *Robots {
	robotsURL := *root
	robotsURL.Path = "/robots.txt"
	robotsURL.RawQuery = ""
	robotsURL.Fragment = ""

	res, err := client.Get(ctx, robotsURL.String())
	if err != nil || res.StatusCode != 200 {
		// No robots.txt means no restrictions. This is the specified
		// behaviour, not a shortcut.
		return &Robots{}
	}
	return ParseRobots(string(res.Body), "AnisBot")
}

// normalise produces the key used for deduplication.
//
// Query strings are kept — many sites put real content behind ?page= — but the
// fragment is dropped and a trailing slash is ignored, so /shipping and
// /shipping/ are not crawled twice.
func normalise(u *url.URL) string {
	path := strings.TrimSuffix(u.Path, "/")
	if path == "" {
		path = "/"
	}
	key := strings.ToLower(u.Hostname()) + path
	if u.RawQuery != "" {
		key += "?" + u.RawQuery
	}
	return key
}

var assetExtensions = []string{
	".jpg", ".jpeg", ".png", ".gif", ".webp", ".svg", ".ico", ".avif",
	".css", ".js", ".mjs", ".map", ".woff", ".woff2", ".ttf", ".otf", ".eot",
	".zip", ".gz", ".tar", ".rar", ".7z", ".dmg", ".exe", ".apk",
	".mp4", ".webm", ".mp3", ".wav", ".avi", ".mov",
	".xml", ".rss", ".atom", ".json",
}

func looksLikeAsset(path string) bool {
	lower := strings.ToLower(path)
	for _, ext := range assetExtensions {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

func titleFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	base := u.Path
	if i := strings.LastIndexByte(base, '/'); i >= 0 {
		base = base[i+1:]
	}
	if base == "" {
		return u.Hostname()
	}
	return base
}

// shortError keeps a customer-facing reason readable, and avoids pasting a
// wall of Go error wrapping into the dashboard.
func shortError(err error) string {
	msg := err.Error()
	if i := strings.Index(msg, ": "); i >= 0 && len(msg) > 90 {
		msg = msg[i+2:]
	}
	if len(msg) > 180 {
		msg = msg[:180] + "…"
	}
	return msg
}
