package extract

import (
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
)

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestHTMLExtractsContentAndDropsBoilerplate(t *testing.T) {
	doc := `<!doctype html><html><head><title>سياسة الشحن</title>
	<style>body{color:red}</style><script>alert(1)</script></head>
	<body>
	  <header><a href="/">الرئيسية</a><a href="/about">من نحن</a></header>
	  <nav><a href="/contact">اتصل بنا</a></nav>
	  <main>
	    <h1>الشحن والتوصيل</h1>
	    <p>الشحن مجاني للطلبات فوق ٢٠٠ ريال.</p>
	    <p>مدة التوصيل ثلاثة أيام عمل.</p>
	  </main>
	  <footer>جميع الحقوق محفوظة</footer>
	</body></html>`

	page, err := HTML([]byte(doc), mustURL(t, "https://example.com/shipping"))
	if err != nil {
		t.Fatal(err)
	}

	if page.Title != "سياسة الشحن" {
		t.Errorf("title = %q", page.Title)
	}
	for _, want := range []string{"الشحن مجاني", "ثلاثة أيام عمل"} {
		if !strings.Contains(page.Text, want) {
			t.Errorf("text is missing %q\ngot: %s", want, page.Text)
		}
	}
	// Boilerplate appears on every page of a site. Left in, it dominates the
	// index and every query matches every page a little.
	for _, unwanted := range []string{"جميع الحقوق محفوظة", "alert(1)", "color:red"} {
		if strings.Contains(page.Text, unwanted) {
			t.Errorf("text still contains boilerplate/script %q", unwanted)
		}
	}
}

func TestHTMLResolvesLinksAndSkipsNonPages(t *testing.T) {
	doc := `<html><body>
	  <a href="/shipping">shipping</a>
	  <a href="returns">returns</a>
	  <a href="https://example.com/faq">faq</a>
	  <a href="https://other.example.org/x">other site</a>
	  <a href="mailto:hi@example.com">mail</a>
	  <a href="tel:+123">phone</a>
	  <a href="javascript:alert(1)">js</a>
	  <a href="#section">anchor</a>
	</body></html>`

	page, err := HTML([]byte(doc), mustURL(t, "https://example.com/docs/index.html"))
	if err != nil {
		t.Fatal(err)
	}

	got := strings.Join(page.Links, " ")
	for _, want := range []string{
		"https://example.com/shipping",
		"https://example.com/docs/returns",
		"https://example.com/faq",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing link %q; got %v", want, page.Links)
		}
	}
	for _, unwanted := range []string{"mailto:", "tel:", "javascript:"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("non-page link %q was kept", unwanted)
		}
	}
}

func TestSameSiteTreatsSubdomainsAsDifferent(t *testing.T) {
	root := mustURL(t, "https://example.com/")
	cases := []struct {
		u    string
		want bool
	}{
		{"https://example.com/shipping", true},
		{"http://example.com/shipping", false}, // different default port = different service
		{"https://EXAMPLE.com/x", true},        // host is case-insensitive
		{"https://blog.example.com/x", false},  // content the customer did not ask for
		{"https://example.com:443/x", true},    // default port normalised
		{"https://example.com:8443/x", false},  // a different service on the same host
		{"https://evil.org/x", false},
	}
	for _, c := range cases {
		if got := SameSite(root, mustURL(t, c.u)); got != c.want {
			t.Errorf("SameSite(%s) = %v, want %v", c.u, got, c.want)
		}
	}
}

func TestHTMLKeepsImageAltText(t *testing.T) {
	// On image-heavy Arabic storefronts the alt attribute is sometimes the
	// only text on the page.
	doc := `<html><body><img alt="عرض خاص على الشحن" src="/a.png"></body></html>`
	page, err := HTML([]byte(doc), mustURL(t, "https://example.com/"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page.Text, "عرض خاص") {
		t.Errorf("alt text dropped: %q", page.Text)
	}
}

/* -------------------------------------------------------------------------
 * PDF
 * ---------------------------------------------------------------------- */

// The Arabic PDF case is the reason the readability gate exists. Extraction
// returns every character with no decode errors, but in visual order — so it
// looks like a success and is unusable.
func TestPDFRejectsVisualOrderArabic(t *testing.T) {
	const fixture = `D:\Work\Projects\Books-project\Tigrinya_for_Arabic_speakers.pdf`
	body, err := os.ReadFile(fixture)
	if err != nil {
		t.Skipf("fixture not available: %v", err)
	}

	_, err = PDF(body)
	if err == nil {
		t.Fatal("accepted an Arabic PDF whose text is in display order — " +
			"it would index cleanly and match nothing a customer types")
	}
	if !errors.Is(err, ErrUnreadablePDF) {
		t.Errorf("err = %v, want ErrUnreadablePDF", err)
	}
	// The message reaches the customer, so it has to say something they can act on.
	if !strings.Contains(err.Error(), "display order") {
		t.Errorf("error does not explain the problem: %v", err)
	}
}

func TestPDFAcceptsLatinText(t *testing.T) {
	const fixture = `D:\Work\Projects\Books-project\Tigrinya_for_English_speakers.pdf`
	body, err := os.ReadFile(fixture)
	if err != nil {
		t.Skipf("fixture not available: %v", err)
	}

	text, err := PDF(body)
	if err != nil {
		t.Fatalf("rejected a readable Latin PDF: %v", err)
	}
	if len(text) < 500 {
		t.Errorf("extracted only %d bytes", len(text))
	}
	if !strings.Contains(text, "Tigrinya") {
		t.Errorf("expected content missing from extraction")
	}
}

func TestCheckReadable(t *testing.T) {
	cases := []struct {
		name    string
		text    string
		wantErr bool
	}{
		{"latin prose", strings.Repeat("Shipping is free over fifty dollars. ", 20), false},
		{"empty", "", true},
		{"no letters", "123 456 !!! ---", true},
		{
			"logical-order Arabic",
			strings.Repeat("الشحن مجاني في جميع أنحاء المملكة من الرياض إلى جدة مع خدمة سريعة. ", 20),
			false,
		},
		{
			"visual-order Arabic",
			// The same sentence with its words reversed, as extraction returns it.
			strings.Repeat("نحشلا يناجم يف عيمج ءاحنأ ةكلمملا نم ضايرلا ىلإ ةدج عم ةمدخ ةعيرس. ", 20),
			true,
		},
		{
			"presentation forms",
			strings.Repeat("ﻣﺮﺣﺒﺎ ﺑﻜﻢ ﻓﻲ ﻣﺘﺠﺮﻧﺎ ﺍﻟﺸﺤﻦ ﻣﺠﺎﻧﻲ ", 30),
			true,
		},
		{
			"mostly replacement characters",
			strings.Repeat("�", 100) + "abc",
			true,
		},
		// A short Arabic label has no function words and must not be rejected
		// for that alone.
		{"short Arabic label", "متجر النخبة", false},
	}

	for _, c := range cases {
		err := checkReadable(c.text)
		if (err != nil) != c.wantErr {
			t.Errorf("%s: err = %v, wantErr = %v", c.name, err, c.wantErr)
		}
	}
}
