package urls

import (
	"testing"

	"github.com/darrenpaine/bookmarkbox/internal/assert"
)

func TestDomain(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://go.dev/doc/", "go.dev"},
		{"https://www.example.com/a", "example.com"},
		{"https://WWW.Example.COM:8443/a", "example.com"},
		{"http://user:pw@sub.example.com/", "sub.example.com"},
		{" https://go.dev ", "go.dev"},
		{"https://www2.example.com", "www2.example.com"},
		{"not a url", ""},
		{"%zz", ""},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			assert.Equal(t, Domain(tt.url), tt.want)
		})
	}
}

func TestComplete(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		// Already complete
		{"https://bbc.co.uk", "https://bbc.co.uk"},
		{"http://example.com/a?b=c#d", "http://example.com/a?b=c#d"},
		{"  https://go.dev  ", "https://go.dev"},
		{"HTTPS://Example.com/Path", "https://Example.com/Path"},
		// No scheme
		{"bbc.co.uk", "https://bbc.co.uk"},
		{"www.bbc.co.uk/news?x=1", "https://www.bbc.co.uk/news?x=1"},
		{"//bbc.co.uk/news", "https://bbc.co.uk/news"},
		{"localhost:8080/admin", "https://localhost:8080/admin"},
		{"192.168.1.10:4000", "https://192.168.1.10:4000"},
		{"[::1]:4000/", "https://[::1]:4000/"},
		{"user@example.com:8443/", "https://user@example.com:8443/"},
		// Mended slashes
		{"https:/bbc.co.uk", "https://bbc.co.uk"},
		{"https:bbc.co.uk", "https://bbc.co.uk"},
		{"https:///bbc.co.uk", "https://bbc.co.uk"},
		{`https:\\bbc.co.uk`, "https://bbc.co.uk"},
		{"https//bbc.co.uk", "https://bbc.co.uk"},
		{"http:/example.com", "http://example.com"},
		// Left for the validator to refuse
		{"", ""},
		{"bbc", "bbc"},
		{"hello world", "hello world"},
		{"mailto:someone@example.com", "mailto:someone@example.com"},
		{"ftp://example.com", "ftp://example.com"},
		{"javascript:alert(1)", "javascript:alert(1)"},
		{"file:///etc/passwd", "file:///etc/passwd"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, Complete(tt.in), tt.want)
		})
	}
}
