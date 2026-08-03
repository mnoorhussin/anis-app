package fetch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func TestIsPublicBlocksTheAddressesThatMatter(t *testing.T) {
	blocked := []struct{ addr, why string }{
		{"169.254.169.254", "cloud instance metadata — credentials"},
		{"127.0.0.1", "loopback — our own PocketBase admin"},
		{"127.0.0.53", "loopback, non-obvious address"},
		{"0.0.0.0", "unspecified"},
		{"10.0.0.5", "private"},
		{"172.16.0.1", "private"},
		{"172.31.255.255", "private, top of range"},
		{"192.168.1.1", "private"},
		{"100.64.0.1", "CGNAT"},
		{"192.0.0.1", "IETF protocol assignments"},
		{"198.18.0.1", "benchmarking"},
		{"224.0.0.1", "multicast"},
		{"255.255.255.255", "broadcast"},
		{"240.0.0.1", "reserved"},
		{"::1", "IPv6 loopback"},
		{"fc00::1", "IPv6 unique-local"},
		{"fe80::1", "IPv6 link-local"},
		{"::", "IPv6 unspecified"},
		// The one that catches people out: an IPv4-mapped IPv6 literal reaches
		// loopback unless the address is unmapped before the IPv4 rules run.
		{"::ffff:127.0.0.1", "IPv4-mapped loopback"},
		{"::ffff:169.254.169.254", "IPv4-mapped metadata"},
		{"::ffff:10.0.0.1", "IPv4-mapped private"},
	}

	for _, c := range blocked {
		addr, err := netip.ParseAddr(c.addr)
		if err != nil {
			t.Fatalf("bad fixture %q: %v", c.addr, err)
		}
		if IsPublic(addr) {
			t.Errorf("IsPublic(%s) = true, must be blocked (%s)", c.addr, c.why)
		}
	}
}

func TestIsPublicAllowsOrdinaryAddresses(t *testing.T) {
	for _, s := range []string{
		"8.8.8.8", "1.1.1.1", "93.184.216.34", "51.75.0.1", // OVH range
		"2606:4700:4700::1111",
	} {
		addr, err := netip.ParseAddr(s)
		if err != nil {
			t.Fatal(err)
		}
		if !IsPublic(addr) {
			t.Errorf("IsPublic(%s) = false, should be reachable", s)
		}
	}
}

func TestCheckURLRejectsNonHTTPSchemes(t *testing.T) {
	for _, raw := range []string{
		"file:///etc/passwd",
		"gopher://example.com/",
		"ftp://example.com/",
		"javascript:alert(1)",
		// PocketBase's own admin, the most tempting internal target.
		"unix:///var/run/docker.sock",
	} {
		if _, err := CheckURL(raw); err == nil {
			t.Errorf("CheckURL(%q) accepted a non-http scheme", raw)
		}
	}
}

func TestCheckURLRejectsPrivateLiterals(t *testing.T) {
	for _, raw := range []string{
		"http://127.0.0.1:8090/_/",
		"http://169.254.169.254/latest/meta-data/",
		"http://192.168.1.1/",
		"http://[::1]:8090/",
		"http://[::ffff:127.0.0.1]/",
	} {
		_, err := CheckURL(raw)
		if err == nil {
			t.Errorf("CheckURL(%q) accepted a private address", raw)
			continue
		}
		if !errors.Is(err, ErrBlockedAddress) {
			t.Errorf("CheckURL(%q) failed with %v, want ErrBlockedAddress", raw, err)
		}
	}
}

func TestCheckURLRejectsMalformed(t *testing.T) {
	for _, raw := range []string{"", "   ", "http://", "not a url at all"} {
		if _, err := CheckURL(raw); err == nil {
			t.Errorf("CheckURL(%q) accepted malformed input", raw)
		}
	}
}

// The security boundary is the dialer, not CheckURL. This proves a connection
// to a private address is refused even when the URL bypasses CheckURL
// entirely — which is what protects us from DNS that changes between the check
// and the connection.
func TestDialerRefusesPrivateAddressesAtConnectTime(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html><body>secret internal page</body></html>"))
	}))
	defer srv.Close()

	// httptest binds 127.0.0.1, so this URL is exactly the internal-service
	// case, and Get is called directly without CheckURL.
	c := New("AnisBot/test", 0)
	_, err := c.Get(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("fetched a loopback address — the dialer guard is not working")
	}
	if !strings.Contains(err.Error(), "not publicly routable") {
		t.Errorf("blocked for the wrong reason: %v", err)
	}
}

func TestGetDecodesWindows1256Arabic(t *testing.T) {
	// Plenty of older Arabic sites still serve windows-1256. Read as UTF-8
	// those bytes become mojibake that indexes happily and matches nothing.
	// "مرحبا" in windows-1256:
	body := []byte{0xE3, 0xD1, 0xCD, 0xC8, 0xC7}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "text/html; charset=windows-1256")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := New("AnisBot/test", 0)
	// Bypass the dialer guard for this test by pointing at the loopback server
	// through a transport without Control.
	c.SetTransportForTest(http.DefaultTransport)

	res, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if got := string(res.Body); got != "مرحبا" {
		t.Errorf("windows-1256 not decoded: got %q (% x)", got, res.Body)
	}
}

func TestGetRejectsOversizedResponses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		big := strings.Repeat("x", MaxBytes+1024)
		_, _ = w.Write([]byte(big))
	}))
	defer srv.Close()

	c := New("AnisBot/test", 0)
	c.SetTransportForTest(http.DefaultTransport)

	_, err := c.Get(context.Background(), srv.URL)
	if !errors.Is(err, ErrTooLarge) {
		t.Errorf("err = %v, want ErrTooLarge", err)
	}
}

func TestGetStopsAfterTooManyRedirects(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL+"/next", http.StatusFound)
	}))
	defer srv.Close()

	c := New("AnisBot/test", 0)
	c.SetTransportForTest(http.DefaultTransport)

	if _, err := c.Get(context.Background(), srv.URL); err == nil {
		t.Error("followed a redirect loop indefinitely")
	}
}

func TestUserAgentIsSent(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("user-agent")
	}))
	defer srv.Close()

	c := New("AnisBot/1.0 (+https://anis.chat/bot)", 0)
	c.SetTransportForTest(http.DefaultTransport)
	if _, err := c.Get(context.Background(), srv.URL); err != nil {
		t.Fatal(err)
	}
	// Site owners must be able to identify and rate-limit us.
	if !strings.Contains(seen, "AnisBot") {
		t.Errorf("user-agent = %q, should identify the crawler", seen)
	}
}
