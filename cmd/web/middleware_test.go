package main

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/darrenpaine/bookmarkbox/internal/assert"
)

func TestSecureHeaders(t *testing.T) {
	// Initialize a new httptest.ResponseRecorder and dummy http.Request.
	rr := httptest.NewRecorder()
	r, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Create a mock HTTP handler.
	content := "OK"
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(content))
	})
	// Pass the mock HTTP handler to our secureHeaders middleware.
	secureHeaders(next).ServeHTTP(rr, r)
	rs := rr.Result()

	// Has it set the items in the response header?
	headers := map[string]string{
		"Content-Security-Policy": "default-src 'self'; style-src 'self' fonts.googleapis.com; font-src fonts.gstatic.com",
		"Referrer-Policy":         "origin-when-cross-origin",
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "deny",
		"X-XSS-Protection":        "0",
	}
	for key, value := range headers {
		assert.Equal(t, rs.Header.Get(key), value)
	}

	// And called the next handler which returns OK status and "OK"?
	assert.Equal(t, rs.StatusCode, http.StatusOK)
	defer rs.Body.Close()
	body, err := io.ReadAll(rs.Body)
	if err != nil {
		t.Fatal(err)
	}
	bytes.TrimSpace(body)
	assert.Equal(t, string(body), content)
}

// Check CSRF protection on Get methods
func TestCsrfGet(t *testing.T) {
	protection := http.NewCrossOriginProtection()
	handler := protection.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Even a cross-site GET passes through
	req := httptest.NewRequest("GET", "/form", nil)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.Header.Set("Origin", "https://evil.com")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET should always be allowed, got %d", rec.Code)
	}
}

// Health checks stay out of the log, and other requests are logged.
func TestLogRequestSkipsPing(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{path: "/ping", want: ""},
		{path: "/about", want: "uri=/about"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			var out bytes.Buffer
			app := &application{logger: slog.New(slog.NewTextHandler(&out, nil))}
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
			app.logRequest(next).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, tt.path, nil))
			if tt.want == "" {
				assert.Equal(t, out.String(), "")
				return
			}
			assert.StringContains(t, out.String(), tt.want)
		})
	}
}
