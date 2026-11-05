package models

import (
	"testing"

	"github.com/darrenpaine/bookmarkbox/internal/assert"
)

func TestNormaliseURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"Unchanged", "https://go.dev/doc", "https://go.dev/doc"},
		{"Host and scheme case", "HTTPS://GO.dev/Doc", "https://go.dev/Doc"},
		{"Root trailing slash", "https://go.dev/", "https://go.dev"},
		{"Path trailing slash", "https://go.dev/doc/", "https://go.dev/doc"},
		{"Only one trailing slash", "https://go.dev/doc//", "https://go.dev/doc/"},
		{"Default https port", "https://go.dev:443/doc", "https://go.dev/doc"},
		{"Default http port", "http://go.dev:80/doc", "http://go.dev/doc"},
		{"Other port kept", "https://go.dev:8443/doc", "https://go.dev:8443/doc"},
		{"Port 80 on https kept", "https://go.dev:80/doc", "https://go.dev:80/doc"},
		{"utm parameters removed", "https://go.dev/?utm_source=x&a=1&UTM_Medium=y", "https://go.dev?a=1"},
		{"Only utm parameters", "https://go.dev/doc?utm_source=x", "https://go.dev/doc"},
		{"Query order kept", "https://go.dev/?b=2&a=1", "https://go.dev?b=2&a=1"},
		{"Scheme kept", "http://go.dev", "http://go.dev"},
		{"www kept", "https://www.go.dev", "https://www.go.dev"},
		{"Fragment kept", "https://example.com/#/page-a", "https://example.com#/page-a"},
		{"Encoded path kept", "https://example.com/a%2Fb/", "https://example.com/a%2Fb"},
		{"IPv6 host", "http://[::1]:80/doc", "http://[::1]/doc"},
		{"Surrounding spaces", "  https://go.dev  ", "https://go.dev"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormaliseURL(tt.url)
			assert.NilError(t, err)
			assert.Equal(t, got, tt.want)
		})
	}
}
