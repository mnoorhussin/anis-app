// Package fetch retrieves URLs supplied by customers, safely.
//
// "Supplied by customers" is the whole problem. A business pastes its website
// address and our server fetches it — which turns the crawler into a
// server-side request forge unless every address is checked. The realistic
// attacks are not exotic:
//
//   - http://169.254.169.254/latest/meta-data/ returns cloud instance
//     credentials on most providers.
//   - http://127.0.0.1:8090/_/ is our own PocketBase admin UI.
//   - http://192.168.x.x/ is whatever else shares the OVH network.
//
// A crawler that follows redirects makes this worse: a public URL can 302 to a
// private one, so the check has to happen on every hop, at connect time, not
// once on the URL the customer typed.
package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"

	"golang.org/x/net/html/charset"
)

// Limits on what a single fetch may cost us.
const (
	// DefaultTimeout bounds one page fetch end to end.
	DefaultTimeout = 20 * time.Second
	// MaxBytes caps a response body. A crawler that streams a 2 GB file into
	// memory is a denial of service against ourselves.
	MaxBytes = 8 << 20 // 8 MiB
	// MaxRedirects before giving up.
	MaxRedirects = 5
)

var (
	// ErrBlockedAddress means the URL resolves somewhere we refuse to reach.
	ErrBlockedAddress = errors.New("fetch: address is not publicly routable")
	// ErrTooLarge means the response exceeded MaxBytes.
	ErrTooLarge = errors.New("fetch: response too large")
	// ErrUnsupportedScheme means it was not http(s).
	ErrUnsupportedScheme = errors.New("fetch: only http and https are supported")
)

// Result is a fetched document.
type Result struct {
	URL         string
	ContentType string
	Body        []byte
	// StatusCode of the final response, after redirects.
	StatusCode int
}

// Client fetches URLs with the guards applied.
type Client struct {
	UserAgent string
	Timeout   time.Duration

	httpClient *http.Client
	// allowPrivate mirrors whether the transport guard was removed, so that
	// CheckURL agrees with what the dialer will actually permit. Without this
	// the two disagree and a development setup refuses its own fixture server.
	allowPrivate bool
}

// New builds a Client whose transport refuses to connect to a non-public
// address.
//
// The check lives in Dialer.Control, which the runtime calls with the ALREADY
// RESOLVED address immediately before connect. That placement is the point:
// validating the hostname earlier and connecting later leaves a window where
// DNS can return a public address to the check and a private one to the
// connection (DNS rebinding). Control has no such window — whatever it
// approves is what gets dialled.
func New(userAgent string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 10 * time.Second,
		Control: func(network, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("fetch: unparseable address %q", address)
			}
			addr, err := netip.ParseAddr(host)
			if err != nil {
				return fmt.Errorf("fetch: unparseable ip %q", host)
			}
			if !IsPublic(addr) {
				return fmt.Errorf("%w: %s", ErrBlockedAddress, addr)
			}
			return nil
		},
	}

	c := &Client{UserAgent: userAgent, Timeout: timeout}
	c.httpClient = &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext:           dialer.DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 15 * time.Second,
			// A customer's site is fetched once per crawl; pooling connections
			// across hosts just holds file descriptors open.
			MaxIdleConns:      10,
			IdleConnTimeout:   30 * time.Second,
			DisableKeepAlives: true,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= MaxRedirects {
				return fmt.Errorf("fetch: stopped after %d redirects", MaxRedirects)
			}
			// The scheme must be re-checked per hop: a public https URL is
			// perfectly entitled to redirect to file:// or gopher:// and the
			// stdlib would follow it.
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return ErrUnsupportedScheme
			}
			return nil
		},
	}
	return c
}

// NewAllowingPrivateAddresses builds a Client with the SSRF guard REMOVED.
//
// Development only, and gated at the call site behind an explicit environment
// variable. It exists so the crawler can be exercised end to end against a
// fixture server on 127.0.0.1 — the exact address the guard is there to refuse.
//
// In production this would let any customer aim the crawler at
// 169.254.169.254 and read the machine's cloud credentials back out of their
// own knowledge base. Nothing about that is subtle; do not wire it to anything
// but a local flag.
func NewAllowingPrivateAddresses(userAgent string, timeout time.Duration) *Client {
	c := New(userAgent, timeout)
	c.allowPrivate = true
	c.httpClient.Transport = &http.Transport{
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		DisableKeepAlives:     true,
	}
	return c
}

// SetTransportForTest replaces the HTTP transport, which removes the SSRF
// guard along with it.
//
// Test-only, and named so that a call site is obvious in review. Tests need it
// because httptest binds 127.0.0.1 — the exact address the guard exists to
// refuse — so a crawler test cannot reach its own fixture server otherwise.
// The guard itself is covered by TestDialerRefusesPrivateAddressesAtConnectTime,
// which deliberately does NOT use this.
//
// Production code must never call it. Doing so re-opens the path to cloud
// metadata and to our own admin UI.
func (c *Client) SetTransportForTest(rt http.RoundTripper) {
	c.httpClient.Transport = rt
}

// IsPublic reports whether an address is one we are willing to connect to.
//
// Default-deny: anything not recognised as ordinary public unicast is
// refused. Being wrong in that direction fails a crawl; being wrong the other
// way hands out cloud credentials.
func IsPublic(addr netip.Addr) bool {
	// An IPv4-mapped IPv6 address (::ffff:127.0.0.1) must be unwrapped first.
	// Without this, every IPv4 rule below silently fails to match and loopback
	// becomes reachable through an IPv6 literal.
	if addr.Is4In6() {
		addr = addr.Unmap()
	}

	switch {
	case !addr.IsValid(),
		addr.IsUnspecified(),      // 0.0.0.0, ::
		addr.IsLoopback(),         // 127.0.0.0/8, ::1
		addr.IsPrivate(),          // 10/8, 172.16/12, 192.168/16, fc00::/7
		addr.IsLinkLocalUnicast(), // 169.254/16 — cloud metadata lives here
		addr.IsLinkLocalMulticast(),
		addr.IsInterfaceLocalMulticast(),
		addr.IsMulticast():
		return false
	}

	if addr.Is4() {
		b := addr.As4()
		switch {
		case b[0] == 100 && b[1] >= 64 && b[1] <= 127: // 100.64/10 CGNAT
			return false
		case b[0] == 192 && b[1] == 0 && b[2] == 0: // 192.0.0/24 IETF protocol
			return false
		case b[0] == 198 && (b[1] == 18 || b[1] == 19): // 198.18/15 benchmarking
			return false
		case b[0] >= 240: // 240/4 reserved, includes 255.255.255.255
			return false
		}
		return true
	}

	// IPv6 beyond the checks above: allow only global unicast, which excludes
	// the unique-local and documentation ranges.
	return addr.IsGlobalUnicast()
}

// CheckURL validates a URL before it is stored or queued.
//
// Called at submit time so a customer gets an immediate, comprehensible error
// rather than a source that sits in "queued" and then fails. It is NOT the
// security boundary — Dialer.Control is, because DNS can change between this
// call and the fetch.
func CheckURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("fetch: could not parse the URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, ErrUnsupportedScheme
	}
	if u.Host == "" {
		return nil, errors.New("fetch: the URL has no host")
	}

	host := u.Hostname()

	// A bare IP literal is checked directly; a hostname is resolved, and EVERY
	// answer must be acceptable. One private address among several is enough
	// to refuse — an attacker controls their own DNS and can order the records
	// however they like.
	if addr, err := netip.ParseAddr(host); err == nil {
		if !IsPublic(addr) {
			return nil, fmt.Errorf("%w: %s", ErrBlockedAddress, addr)
		}
		return u, nil
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, fmt.Errorf("fetch: could not resolve %q: %w", host, err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("fetch: %q resolves to nothing", host)
	}
	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip)
		if !ok || !IsPublic(addr) {
			return nil, fmt.Errorf("%w: %s resolves to %s", ErrBlockedAddress, host, ip)
		}
	}
	return u, nil
}

// CheckURL validates a URL under this client's policy.
//
// Callers should prefer this to the package-level function: it stays
// consistent with what the client's dialer will actually allow, so a
// development client that permits private addresses does not reject its own
// fixture server before it ever dials.
func (c *Client) CheckURL(raw string) (*url.URL, error) {
	if c.allowPrivate {
		u, err := url.Parse(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("fetch: could not parse the URL: %w", err)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return nil, ErrUnsupportedScheme
		}
		if u.Host == "" {
			return nil, errors.New("fetch: the URL has no host")
		}
		return u, nil
	}
	return CheckURL(raw)
}

// Get fetches a URL.
func (c *Client) Get(ctx context.Context, raw string) (*Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("user-agent", c.UserAgent)
	// Ask for what we can actually read. Sites that content-negotiate will
	// give us HTML rather than a JSON API response.
	req.Header.Set("accept", "text/html,application/xhtml+xml,application/pdf,text/plain;q=0.9,*/*;q=0.5")
	req.Header.Set("accept-language", "ar,en;q=0.9")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	// Read one byte past the cap so the limit can be detected rather than
	// silently truncating a document and indexing half a policy.
	body, err := io.ReadAll(io.LimitReader(res.Body, MaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("fetch: reading body: %w", err)
	}
	if len(body) > MaxBytes {
		return nil, ErrTooLarge
	}

	contentType := res.Header.Get("content-type")

	// Decode to UTF-8 using the charset the document declares.
	//
	// This matters more for Arabic than for most: plenty of older Arabic sites
	// are still served as windows-1256 or ISO-8859-6. Treating those bytes as
	// UTF-8 produces mojibake that embeds and indexes perfectly happily and
	// matches nothing a customer will ever type.
	if isTextual(contentType) {
		decoded, decodeErr := decodeToUTF8(body, contentType)
		if decodeErr == nil {
			body = decoded
		}
		// A charset we cannot decode is not fatal; the original bytes may
		// still be usable UTF-8.
	}

	return &Result{
		URL:         res.Request.URL.String(), // final URL, after redirects
		ContentType: contentType,
		Body:        body,
		StatusCode:  res.StatusCode,
	}, nil
}

func isTextual(contentType string) bool {
	ct := strings.ToLower(contentType)
	return strings.HasPrefix(ct, "text/") ||
		strings.Contains(ct, "xhtml") ||
		strings.Contains(ct, "+xml")
}

func decodeToUTF8(body []byte, contentType string) ([]byte, error) {
	reader, err := charset.NewReader(strings.NewReader(string(body)), contentType)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(io.LimitReader(reader, MaxBytes))
}
