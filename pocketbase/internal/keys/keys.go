// Package keys generates the public identifiers that appear in customer-facing
// places — currently the widget key that goes into an embed snippet.
package keys

import (
	"crypto/rand"
	"encoding/base32"
	"strings"
)

// WidgetKeyPrefix marks a key's purpose wherever one turns up — in a support
// ticket, a log line, or a customer's page source.
const WidgetKeyPrefix = "wk_"

// widgetKeyBytes is the entropy behind a widget key.
//
// 16 bytes (128 bits) is far beyond what guessing could reach, which matters
// because the key names a workspace and the endpoint it unlocks is public.
// Note that guessing resistance is not what makes the widget safe — the key is
// visible in the page source of every site that embeds it. The Origin
// allow-list is what authorises. This just stops someone enumerating
// workspaces.
const widgetKeyBytes = 16

// Crockford-style alphabet without I, L, O and U: keys get read aloud in
// support conversations and pasted by hand, and those four are the ones people
// confuse with 1, 0 and each other.
var encoding = base32.NewEncoding("0123456789ABCDEFGHJKMNPQRSTVWXYZ").WithPadding(base32.NoPadding)

// NewWidgetKey returns a fresh widget key, e.g. "wk_4XR2K9QW7M3TB1NHZ5VC8DFG".
//
// Panics if the system CSPRNG fails. That is the right response: crypto/rand
// failing means the machine is in a state where continuing to mint identifiers
// is worse than stopping, and returning an error here would only invite a
// caller to ignore it and use a predictable key.
func NewWidgetKey() string {
	b := make([]byte, widgetKeyBytes)
	if _, err := rand.Read(b); err != nil {
		panic("keys: crypto/rand unavailable: " + err.Error())
	}
	return WidgetKeyPrefix + encoding.EncodeToString(b)
}

// IsWidgetKey reports whether s has the shape of a widget key.
//
// A cheap shape check only — it says nothing about whether the key exists.
// Use it to reject obvious junk before touching the database, so a flood of
// malformed requests from a misconfigured embed cannot turn into a flood of
// queries.
func IsWidgetKey(s string) bool {
	if !strings.HasPrefix(s, WidgetKeyPrefix) {
		return false
	}
	body := s[len(WidgetKeyPrefix):]
	if len(body) != encoding.EncodedLen(widgetKeyBytes) {
		return false
	}
	_, err := encoding.DecodeString(body)
	return err == nil
}
