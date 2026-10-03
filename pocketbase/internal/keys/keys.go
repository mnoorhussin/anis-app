// Package keys generates the public identifiers that appear in customer-facing
// places — currently the widget key that goes into an embed snippet.
package keys

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
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

// InviteTokenPrefix marks an invitation link's token.
const InviteTokenPrefix = "inv_"

// inviteTokenBytes is the entropy behind an invitation token.
//
// Twice the widget key's, because the two are opposite kinds of secret. A
// widget key is public and only identifies; an invite token is a bearer
// credential — whoever holds it can join a workspace (as the invited email). It
// must stay unguessable for its whole lifetime, and only its hash is stored.
const inviteTokenBytes = 32

// NewInviteToken returns a fresh invitation token. Panics if the CSPRNG fails,
// for the same reason as NewWidgetKey.
func NewInviteToken() string {
	b := make([]byte, inviteTokenBytes)
	if _, err := rand.Read(b); err != nil {
		panic("keys: crypto/rand unavailable: " + err.Error())
	}
	return InviteTokenPrefix + encoding.EncodeToString(b)
}

// IsInviteToken reports whether s has the shape of an invitation token. A shape
// check only, so junk is rejected before it reaches the database.
func IsInviteToken(s string) bool {
	if !strings.HasPrefix(s, InviteTokenPrefix) {
		return false
	}
	body := s[len(InviteTokenPrefix):]
	if len(body) != encoding.EncodedLen(inviteTokenBytes) {
		return false
	}
	_, err := encoding.DecodeString(body)
	return err == nil
}

// HashInviteToken is what the database stores in place of the token.
//
// A plain SHA-256 rather than a password hash is correct here: the input is 256
// random bits, not something a person chose, so there is nothing for a slow
// hash to protect against. What hashing buys is that a leaked database, backup
// or log line does not contain a working link.
func HashInviteToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
