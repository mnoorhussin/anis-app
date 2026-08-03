// Package extract turns fetched bytes into the plain text that gets chunked.
package extract

import (
	"bytes"
	"net/url"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Page is the useful content of one HTML document.
type Page struct {
	Title string
	Text  string
	// Links are absolute, same-document-scheme URLs found in the page, for the
	// crawler to consider next.
	Links []string
}

// skipContent holds elements whose text is never page content.
//
// nav/header/footer/aside are excluded because a site's menu and footer appear
// on every page. Left in, they dominate the index: every chunk contains "Home
// About Contact Shipping Returns", so every query matches every page a little
// and nothing matches strongly. Removing boilerplate is most of what makes
// retrieval from a crawled site work at all.
var skipContent = map[atom.Atom]bool{
	atom.Script: true, atom.Style: true, atom.Noscript: true,
	atom.Nav: true, atom.Header: true, atom.Footer: true, atom.Aside: true,
	atom.Svg: true, atom.Iframe: true, atom.Template: true, atom.Form: true,
	atom.Select: true, atom.Button: true,
}

// blockLevel elements produce a paragraph break in the extracted text, which
// is what the chunker keys off.
var blockLevel = map[atom.Atom]bool{
	atom.P: true, atom.Div: true, atom.Section: true, atom.Article: true,
	atom.H1: true, atom.H2: true, atom.H3: true, atom.H4: true, atom.H5: true, atom.H6: true,
	atom.Li: true, atom.Tr: true, atom.Br: true, atom.Blockquote: true,
	atom.Dt: true, atom.Dd: true, atom.Pre: true, atom.Figcaption: true,
}

// HTML extracts readable text, the title, and same-site links.
//
// base is the URL the document was fetched from, used to resolve relative
// links and to decide which links are worth following.
func HTML(body []byte, base *url.URL) (*Page, error) {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	page := &Page{}
	var sb strings.Builder
	seen := map[string]bool{}

	var walk func(*html.Node, bool)
	walk = func(n *html.Node, inMain bool) {
		switch n.Type {
		case html.ElementNode:
			if skipContent[n.DataAtom] {
				return
			}
			if n.DataAtom == atom.Title && page.Title == "" {
				page.Title = strings.TrimSpace(textOf(n))
				return
			}
			if n.DataAtom == atom.A {
				if href := attr(n, "href"); href != "" {
					if abs := resolve(base, href); abs != "" && !seen[abs] {
						seen[abs] = true
						page.Links = append(page.Links, abs)
					}
				}
			}
			// An image's alt text is often the only description of a product
			// photo, and on Arabic sites it is frequently the only Arabic on
			// an otherwise image-based page.
			if n.DataAtom == atom.Img {
				if alt := strings.TrimSpace(attr(n, "alt")); alt != "" {
					sb.WriteString(alt)
					sb.WriteString("\n\n")
				}
			}
			if blockLevel[n.DataAtom] {
				sb.WriteString("\n\n")
			}

		case html.TextNode:
			if t := strings.TrimSpace(n.Data); t != "" {
				sb.WriteString(t)
				sb.WriteByte(' ')
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, inMain)
		}

		if n.Type == html.ElementNode && blockLevel[n.DataAtom] {
			sb.WriteString("\n\n")
		}
	}
	walk(doc, false)

	page.Text = tidy(sb.String())
	return page, nil
}

// tidy collapses the whitespace soup that walking a DOM produces into
// paragraphs the chunker can use.
func tidy(s string) string {
	lines := strings.Split(s, "\n")
	var out []string
	for _, l := range lines {
		if l = strings.Join(strings.Fields(l), " "); l != "" {
			out = append(out, l)
		}
	}

	// Drop consecutive duplicates. Menus and breadcrumbs that survived the
	// element filter usually arrive as repeated identical lines.
	var deduped []string
	for i, l := range out {
		if i > 0 && l == out[i-1] {
			continue
		}
		deduped = append(deduped, l)
	}

	return strings.TrimSpace(strings.Join(deduped, "\n\n"))
}

func textOf(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return sb.String()
}

func attr(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}

// resolve turns a href into an absolute http(s) URL, or "" if it is not one
// worth following.
func resolve(base *url.URL, href string) string {
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "#") {
		return ""
	}
	// mailto:, tel:, javascript:, data: — none are pages.
	if i := strings.Index(href, ":"); i > 0 {
		switch strings.ToLower(href[:i]) {
		case "http", "https":
		default:
			if !strings.HasPrefix(href, "//") {
				return ""
			}
		}
	}

	u, err := base.Parse(href)
	if err != nil {
		return ""
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	// A fragment is the same page.
	u.Fragment = ""
	return u.String()
}

// SameSite reports whether a URL belongs to the same site as the crawl root.
//
// Host AND port must match; scheme need not, so a site that mixes http and
// https links is not split in two. Default ports are normalised, so
// https://example.com and https://example.com:443 are the same site.
//
// Two things are deliberately treated as DIFFERENT sites:
//
//   - A subdomain. Crawling blog.example.com because example.com linked to it
//     pulls in content the customer did not ask us to index and may not even
//     control.
//   - A different port. That is a different service on the same machine —
//     often an admin panel or a staging app — and "the customer gave us their
//     website" is not consent to index it.
func SameSite(root, candidate *url.URL) bool {
	return strings.EqualFold(root.Hostname(), candidate.Hostname()) &&
		effectivePort(root) == effectivePort(candidate)
}

func effectivePort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}
