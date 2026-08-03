package crawl

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mnoorhussin/anis-app/pocketbase/internal/fetch"
)

// testClient bypasses the SSRF dialer guard, which would otherwise refuse the
// loopback address httptest binds. The guard itself is tested in the fetch
// package; here we are testing crawl behaviour.
func testClient() *fetch.Client {
	c := fetch.New("AnisBot/test (+https://anis.chat/bot)", 5*time.Second)
	c.SetTransportForTest(http.DefaultTransport)
	return c
}

func page(title, body string, links ...string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "<html><head><title>%s</title></head><body><main><h1>%s</h1><p>%s</p>", title, title, body)
	for _, l := range links {
		fmt.Fprintf(&sb, `<a href="%s">link</a>`, l)
	}
	sb.WriteString("</main></body></html>")
	return sb.String()
}

// Long enough to clear the minimum-text threshold.
const filler = "الشحن مجاني للطلبات فوق مئتي ريال ونحن نوصل إلى جميع مدن المملكة خلال ثلاثة أيام عمل من تاريخ الطلب. "

func TestCrawlFollowsSameSiteLinks(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page("الرئيسية", strings.Repeat(filler, 2), "/shipping", "/returns")))
	})
	mux.HandleFunc("/shipping", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page("الشحن", strings.Repeat(filler, 2))))
	})
	mux.HandleFunc("/returns", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page("الإرجاع", strings.Repeat(filler, 2))))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	res, err := Site(context.Background(), srv.URL, Options{Client: testClient(), Delay: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Docs) != 3 {
		t.Fatalf("crawled %d pages, want 3: %+v", len(res.Docs), res.Skipped)
	}

	titles := map[string]bool{}
	for _, d := range res.Docs {
		titles[d.Title] = true
	}
	for _, want := range []string{"الرئيسية", "الشحن", "الإرجاع"} {
		if !titles[want] {
			t.Errorf("missing page %q", want)
		}
	}
}

func TestCrawlRespectsRobotsDisallow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("User-agent: *\nDisallow: /private\n"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "text/html")
		_, _ = w.Write([]byte(page("Home", strings.Repeat(filler, 2), "/private/secret", "/public")))
	})
	mux.HandleFunc("/public", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "text/html")
		_, _ = w.Write([]byte(page("Public", strings.Repeat(filler, 2))))
	})
	var privateHits int
	mux.HandleFunc("/private/secret", func(w http.ResponseWriter, _ *http.Request) {
		privateHits++
		w.Header().Set("content-type", "text/html")
		_, _ = w.Write([]byte(page("Secret", strings.Repeat(filler, 2))))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	res, err := Site(context.Background(), srv.URL, Options{Client: testClient(), Delay: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if privateHits != 0 {
		t.Errorf("fetched a disallowed path %d times — this gets AnisBot blocked network-wide", privateHits)
	}
	for _, d := range res.Docs {
		if strings.Contains(d.URL, "private") {
			t.Errorf("indexed a disallowed page: %s", d.URL)
		}
	}
}

func TestCrawlReportsWhenRobotsBlocksTheRoot(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("User-agent: *\nDisallow: /\n"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(page("Home", filler)))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	res, err := Site(context.Background(), srv.URL, Options{Client: testClient(), Delay: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	// "Blocked by robots.txt" is a different message from "we found nothing",
	// and the customer can act on the first one.
	if !res.RobotsBlocked {
		t.Error("expected RobotsBlocked to be reported")
	}
	if len(res.Docs) != 0 {
		t.Errorf("indexed %d pages despite a site-wide Disallow", len(res.Docs))
	}
}

func TestCrawlStaysOnTheSameHost(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("crawled a different host")
		w.Header().Set("content-type", "text/html")
		_, _ = w.Write([]byte(page("Other", filler)))
	}))
	defer other.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "text/html")
		_, _ = w.Write([]byte(page("Home", strings.Repeat(filler, 2), other.URL+"/x")))
	}))
	defer srv.Close()

	res, err := Site(context.Background(), srv.URL, Options{Client: testClient(), Delay: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range res.Docs {
		if strings.Contains(d.URL, other.URL) {
			t.Errorf("indexed an off-site page: %s", d.URL)
		}
	}
}

func TestCrawlObeysThePageCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/html")
		// Every page links to two more, so the frontier never empties.
		_, _ = w.Write([]byte(page("Page "+r.URL.Path, strings.Repeat(filler, 2),
			r.URL.Path+"a", r.URL.Path+"b")))
	}))
	defer srv.Close()

	res, err := Site(context.Background(), srv.URL, Options{
		Client: testClient(), Delay: time.Millisecond, MaxPages: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Docs) > 5 {
		t.Errorf("crawled %d pages, cap was 5", len(res.Docs))
	}
}

func TestCrawlSkipsThinPagesAndAssets(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "text/html")
		_, _ = w.Write([]byte(page("Home", strings.Repeat(filler, 2), "/thin", "/logo.png", "/app.js")))
	})
	mux.HandleFunc("/thin", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "text/html")
		_, _ = w.Write([]byte("<html><body><p>Hi</p></body></html>"))
	})
	var assetHits int
	for _, p := range []string{"/logo.png", "/app.js"} {
		mux.HandleFunc(p, func(w http.ResponseWriter, _ *http.Request) { assetHits++ })
	}
	srv := httptest.NewServer(mux)
	defer srv.Close()

	res, err := Site(context.Background(), srv.URL, Options{Client: testClient(), Delay: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if assetHits != 0 {
		t.Errorf("fetched %d asset URLs — wasted requests against the customer's server", assetHits)
	}
	for _, d := range res.Docs {
		if strings.Contains(d.Text, "Hi") && len(d.Text) < 100 {
			t.Error("indexed a near-empty page")
		}
	}
	// The customer should be told why a page they expected is missing.
	if len(res.Skipped) == 0 {
		t.Error("thin page was skipped without being reported")
	}
}

func TestCrawlDoesNotVisitTheSamePageTwice(t *testing.T) {
	hits := map[string]int{}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		hits[r.URL.Path]++
		w.Header().Set("content-type", "text/html")
		// Same page linked several ways: trailing slash, fragment, self.
		_, _ = w.Write([]byte(page("Home", strings.Repeat(filler, 2),
			"/shipping", "/shipping/", "/shipping#top", "/")))
	})
	mux.HandleFunc("/shipping", func(w http.ResponseWriter, r *http.Request) {
		hits[r.URL.Path]++
		w.Header().Set("content-type", "text/html")
		_, _ = w.Write([]byte(page("Shipping", strings.Repeat(filler, 2), "/")))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	if _, err := Site(context.Background(), srv.URL, Options{Client: testClient(), Delay: time.Millisecond}); err != nil {
		t.Fatal(err)
	}
	for path, n := range hits {
		if n > 1 {
			t.Errorf("fetched %s %d times", path, n)
		}
	}
}

/* ------------------------------- robots ------------------------------- */

func TestRobotsRules(t *testing.T) {
	body := `
User-agent: *
Disallow: /admin
Disallow: /cart
Allow: /admin/public

User-agent: AnisBot
Disallow: /no-anis
`
	// A group naming us explicitly wins over the wildcard.
	r := ParseRobots(body, "AnisBot/1.0")
	if r.Allowed("/no-anis/page") {
		t.Error("ignored a rule addressed to AnisBot specifically")
	}
	if !r.Allowed("/admin") {
		t.Error("applied wildcard rules even though a specific group exists")
	}

	// A crawler not named in the file gets the wildcard group.
	w := ParseRobots(body, "SomeOtherBot")
	if w.Allowed("/admin/secret") {
		t.Error("wildcard Disallow not applied")
	}
	if !w.Allowed("/admin/public") {
		t.Error("longer Allow should beat a shorter Disallow")
	}
	if !w.Allowed("/shipping") {
		t.Error("unrelated path was blocked")
	}
}

func TestRobotsWildcardsAndAnchors(t *testing.T) {
	r := ParseRobots("User-agent: *\nDisallow: /*.pdf$\nDisallow: /tmp/*/private\n", "AnisBot")
	cases := []struct {
		path string
		want bool
	}{
		{"/report.pdf", false},
		{"/a/b/report.pdf", false},
		{"/report.pdf.html", true}, // $ anchors the end
		{"/tmp/x/private", false},
		{"/tmp/x/public", true},
		{"/shipping", true},
	}
	for _, c := range cases {
		if got := r.Allowed(c.path); got != c.want {
			t.Errorf("Allowed(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestRobotsEmptyDisallowMeansAllowAll(t *testing.T) {
	r := ParseRobots("User-agent: *\nDisallow:\n", "AnisBot")
	if !r.Allowed("/anything") {
		t.Error("an empty Disallow means no restriction")
	}
}

func TestRobotsCrawlDelay(t *testing.T) {
	r := ParseRobots("User-agent: *\nCrawl-delay: 2.5\n", "AnisBot")
	if r.CrawlDelay < 2.4 || r.CrawlDelay > 2.6 {
		t.Errorf("CrawlDelay = %v, want 2.5", r.CrawlDelay)
	}
}

func TestRobotsMissingFileAllowsEverything(t *testing.T) {
	r := ParseRobots("", "AnisBot")
	if !r.Allowed("/anything") {
		t.Error("an absent robots.txt imposes no restrictions")
	}
}
