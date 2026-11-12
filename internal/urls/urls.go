// Package urls holds URL helpers shared by the models, the migrations and the
// templates, so they all agree.
package urls

import (
	"net/url"
	"regexp"
	"strings"
)

// Domain returns the lower-case host of a URL without "www.", for example
// "go.dev". It gives "" for a URL that can't be parsed.
func Domain(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}

// webScheme matches a typed http or https scheme, however its slashes went
// wrong: "https://", "https:/", "https:", "https:///", "https:\\" or "https//".
var webScheme = regexp.MustCompile(`(?i)^(https?)(?::[/\\]*|//)`)

// otherScheme matches the start of an address with some other scheme, such as
// "mailto:" or "ftp://". A digit after the colon is a port, as in
// "localhost:8080", so that isn't a scheme.
var otherScheme = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:[^0-9]`)

// Complete turns a web address as people type it into a full one: "bbc.co.uk"
// becomes "https://bbc.co.uk", "//bbc.co.uk" gains "https:", and a scheme with
// the wrong slashes, such as "https:/bbc.co.uk", is mended. Anything else,
// including other schemes and text with no dot in its host, comes back only
// trimmed, for the validator to refuse.
func Complete(raw string) string {
	s := strings.TrimSpace(raw)
	if m := webScheme.FindStringSubmatch(s); m != nil {
		return strings.ToLower(m[1]) + "://" + s[len(m[0]):]
	}
	if rest, ok := strings.CutPrefix(s, "//"); ok {
		s = rest
	} else if s == "" || otherScheme.MatchString(s) {
		return s
	}
	host := s
	if i := strings.IndexAny(host, "/?#"); i >= 0 {
		host = host[:i]
	}
	if i := strings.LastIndex(host, "@"); i >= 0 {
		host = host[i+1:]
	}
	if !strings.Contains(host, ".") && !strings.HasPrefix(strings.ToLower(host), "localhost") && !strings.HasPrefix(host, "[") {
		return strings.TrimSpace(raw)
	}
	return "https://" + s
}
