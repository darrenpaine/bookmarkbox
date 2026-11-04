package models

import (
	"net/url"
	"strings"
)

// NormaliseURL returns the form of a URL used to spot Duplicates. It lower-cases
// the scheme and host, and removes default ports, one trailing slash and utm_*
// parameters. Everything else is kept: http and https differ, www. stays, and
// fragments stay because they can select pages in single-page apps.
func NormaliseURL(rawURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", err
	}
	// url.Parse has already lower-cased the scheme.
	u.Host = strings.ToLower(u.Host)
	switch u.Scheme {
	case "http":
		u.Host = strings.TrimSuffix(u.Host, ":80")
	case "https":
		u.Host = strings.TrimSuffix(u.Host, ":443")
	}
	u.Path = strings.TrimSuffix(u.Path, "/")
	u.RawPath = strings.TrimSuffix(u.RawPath, "/")
	u.RawQuery = dropUTMParameters(u.RawQuery)
	return u.String(), nil
}

// dropUTMParameters filters the raw query string rather than using url.Values,
// which would sort the parameters and change their encoding.
func dropUTMParameters(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}
	kept := []string{}
	for _, param := range strings.Split(rawQuery, "&") {
		name, _, _ := strings.Cut(param, "=")
		if !strings.HasPrefix(strings.ToLower(name), "utm_") {
			kept = append(kept, param)
		}
	}
	return strings.Join(kept, "&")
}
