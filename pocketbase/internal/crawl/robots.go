package crawl

import (
	"strings"
)

// Robots is the subset of robots.txt that applies to us.
//
// Deliberately small. The full specification has wildcards, crawl-delay,
// sitemaps and host directives; this implements user-agent group selection and
// Allow/Disallow with the `*` and `$` wildcards, which is what actually gates
// a crawl. Anything unrecognised is ignored rather than guessed at.
type Robots struct {
	rules []rule
	// CrawlDelay in seconds, if the site asked for one.
	CrawlDelay float64
}

type rule struct {
	path  string
	allow bool
}

// ParseRobots reads a robots.txt for the given user-agent token.
//
// Group selection follows the convention: the most specific matching
// user-agent group wins, and `*` is the fallback. A site that names our bot
// explicitly is making a deliberate choice, so that group takes precedence
// over the wildcard even if it appears later in the file.
func ParseRobots(body string, userAgent string) *Robots {
	ua := strings.ToLower(userAgent)

	var wildcard, specific []rule
	var wildcardDelay, specificDelay float64

	// groupFor tracks which groups the current directive block applies to. A
	// block can name several user-agents before its first rule.
	inWildcard, inSpecific := false, false
	pendingAgents := true

	for _, raw := range strings.Split(body, "\n") {
		line := raw
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)

		switch key {
		case "user-agent":
			if !pendingAgents {
				// A user-agent line after rules starts a new group.
				inWildcard, inSpecific = false, false
				pendingAgents = true
			}
			v := strings.ToLower(value)
			switch {
			case v == "*":
				inWildcard = true
			case v != "" && strings.Contains(ua, v):
				// Our user-agent contains this group's token, so the group is
				// addressed to us. The test is deliberately one-directional:
				// checking whether the token contains OUR name instead would
				// make every crawler match the "AnisBot" group and inherit
				// rules meant only for us.
				inSpecific = true
			}

		case "disallow", "allow":
			pendingAgents = false
			allow := key == "allow"
			// An empty Disallow means "allow everything" and carries no path.
			if value == "" && !allow {
				continue
			}
			r := rule{path: value, allow: allow}
			if inSpecific {
				specific = append(specific, r)
			}
			if inWildcard {
				wildcard = append(wildcard, r)
			}

		case "crawl-delay":
			pendingAgents = false
			d := parseFloat(value)
			if inSpecific {
				specificDelay = d
			}
			if inWildcard {
				wildcardDelay = d
			}
		}
	}

	if len(specific) > 0 || specificDelay > 0 {
		return &Robots{rules: specific, CrawlDelay: specificDelay}
	}
	return &Robots{rules: wildcard, CrawlDelay: wildcardDelay}
}

// Allowed reports whether a path may be fetched.
//
// Longest matching rule wins, and Allow beats Disallow at equal length — the
// behaviour every major crawler implements, and the one site owners write
// their files expecting.
func (r *Robots) Allowed(path string) bool {
	if r == nil || len(r.rules) == 0 {
		return true
	}
	if path == "" {
		path = "/"
	}

	best := -1
	allowed := true
	for _, rule := range r.rules {
		if !matchPath(rule.path, path) {
			continue
		}
		n := len(rule.path)
		if n > best || (n == best && rule.allow) {
			best = n
			allowed = rule.allow
		}
	}
	return allowed
}

// matchPath implements robots.txt prefix matching with `*` and `$`.
func matchPath(pattern, path string) bool {
	mustEnd := strings.HasSuffix(pattern, "$")
	if mustEnd {
		pattern = strings.TrimSuffix(pattern, "$")
	}

	if !strings.Contains(pattern, "*") {
		if mustEnd {
			return path == pattern
		}
		return strings.HasPrefix(path, pattern)
	}

	parts := strings.Split(pattern, "*")
	pos := 0
	for i, part := range parts {
		if part == "" {
			continue
		}
		if i == 0 {
			if !strings.HasPrefix(path[pos:], part) {
				return false
			}
			pos += len(part)
			continue
		}
		idx := strings.Index(path[pos:], part)
		if idx < 0 {
			return false
		}
		pos += idx + len(part)
	}
	if mustEnd {
		return pos == len(path)
	}
	return true
}

func parseFloat(s string) float64 {
	var v float64
	var frac float64 = 0
	seenDot := false
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9':
			if seenDot {
				if frac == 0 {
					frac = 0.1
				}
				v += float64(c-'0') * frac
				frac /= 10
			} else {
				v = v*10 + float64(c-'0')
			}
		case c == '.' && !seenDot:
			seenDot = true
		default:
			return v
		}
	}
	return v
}
