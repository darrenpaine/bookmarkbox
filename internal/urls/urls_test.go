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
