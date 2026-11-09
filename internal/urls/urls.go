// Package urls holds URL helpers shared by the models, the migrations and the
// templates, so they all agree.
package urls

import (
	"net/url"
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
