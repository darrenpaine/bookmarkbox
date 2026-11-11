package main

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/darrenpaine/bookmarkbox/internal/assert"
)

// postFormHeaders posts a form with extra request headers.
func (ts *testServer) postFormHeaders(t *testing.T, urlPath string, form url.Values, headers map[string]string) (int, http.Header, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, ts.URL+urlPath, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	rs, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer rs.Body.Close()
	body, err := io.ReadAll(rs.Body)
	if err != nil {
		t.Fatal(err)
	}
	return rs.StatusCode, rs.Header, string(body)
}

var wrongLogin = url.Values{"email": {"alice@example.com"}, "password": {"wrong-password"}}

// Login, signup and password change allow ten attempts a minute from one
// client, then answer 429 with the form shown again.
func TestRateLimit(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	defer ts.Close()

	t.Run("Login", func(t *testing.T) {
		for i := range attemptsBurst {
			code, _, _ := ts.postForm(t, "/user/login", wrongLogin)
			if code != http.StatusUnprocessableEntity {
				t.Fatalf("attempt %d: got %d", i+1, code)
			}
		}
		code, header, body := ts.postForm(t, "/user/login", wrongLogin)
		assert.Equal(t, code, http.StatusTooManyRequests)
		assert.Equal(t, header.Get("Retry-After"), "60")
		assert.StringContains(t, body, "<p class='form-error'>Too many attempts. Try again in a minute.</p>")
		assert.StringContains(t, body, "value='alice@example.com'")
		if strings.Contains(body, "wrong-password") {
			t.Error("the password was shown again")
		}

		// Even the right password waits, and so does every later attempt
		// for a minute, not just until one more attempt has refilled.
		code, _, _ = ts.postForm(t, "/user/login", url.Values{"email": {"alice@example.com"}, "password": {"pa$$word"}})
		assert.Equal(t, code, http.StatusTooManyRequests)
	})

	t.Run("Signup counts separately", func(t *testing.T) {
		form := url.Values{"name": {"Bob"}, "email": {"bob@example.com"}, "password": {"short"}}
		for i := range attemptsBurst {
			code, _, _ := ts.postForm(t, "/user/signup", form)
			if code != http.StatusUnprocessableEntity {
				t.Fatalf("attempt %d: got %d", i+1, code)
			}
		}
		code, _, body := ts.postForm(t, "/user/signup", form)
		assert.Equal(t, code, http.StatusTooManyRequests)
		assert.StringContains(t, body, "Too many attempts. Try again in a minute.")
		assert.StringContains(t, body, "value='bob@example.com'")
	})

	t.Run("Password change", func(t *testing.T) {
		app := newTestApplication(t)
		ts := newTestServer(t, app.routes())
		defer ts.Close()
		ts.login(t)

		form := url.Values{"old_password": {"wrong-password"}, "new_password": {"new-password"}, "confirm_password": {"new-password"}}
		for i := range attemptsBurst {
			code, _, _ := ts.postForm(t, "/account/password/update", form)
			if code != http.StatusUnprocessableEntity {
				t.Fatalf("attempt %d: got %d", i+1, code)
			}
		}
		code, _, body := ts.postForm(t, "/account/password/update", form)
		assert.Equal(t, code, http.StatusTooManyRequests)
		assert.StringContains(t, body, "Too many attempts. Try again in a minute.")
	})

	t.Run("Pages are not limited", func(t *testing.T) {
		code, _, _ := ts.get(t, "/user/login")
		assert.Equal(t, code, http.StatusOK)
	})
}

// Without CLIENT_IP_HEADER every forwarded header is ignored, since anyone
// can send one. With it, only the named header counts, and only its last hop.
func TestRateLimitClientIP(t *testing.T) {
	exhaust := func(t *testing.T, ts *testServer, headers map[string]string) {
		t.Helper()
		for range attemptsBurst {
			ts.postFormHeaders(t, "/user/login", wrongLogin, headers)
		}
	}
	login := func(t *testing.T, ts *testServer, headers map[string]string) int {
		t.Helper()
		code, _, _ := ts.postFormHeaders(t, "/user/login", wrongLogin, headers)
		return code
	}

	t.Run("No header set", func(t *testing.T) {
		app := newTestApplication(t)
		ts := newTestServer(t, app.routes())
		defer ts.Close()
		exhaust(t, ts, map[string]string{"X-Forwarded-For": "203.0.113.1"})
		assert.Equal(t, login(t, ts, map[string]string{"X-Forwarded-For": "203.0.113.2"}), http.StatusTooManyRequests)
	})

	t.Run("X-Forwarded-For", func(t *testing.T) {
		app := newTestApplication(t)
		app.clientIPHeader = "X-Forwarded-For"
		ts := newTestServer(t, app.routes())
		defer ts.Close()
		exhaust(t, ts, map[string]string{"X-Forwarded-For": "198.51.100.7, 203.0.113.1"})
		// The client can change the hops before ours, but not ours.
		assert.Equal(t, login(t, ts, map[string]string{"X-Forwarded-For": "198.51.100.8, 203.0.113.1"}), http.StatusTooManyRequests)
		// Another client has its own allowance.
		assert.Equal(t, login(t, ts, map[string]string{"X-Forwarded-For": "203.0.113.2"}), http.StatusUnprocessableEntity)
	})

	t.Run("A single-address header", func(t *testing.T) {
		app := newTestApplication(t)
		app.clientIPHeader = "True-Client-Ip"
		ts := newTestServer(t, app.routes())
		defer ts.Close()
		exhaust(t, ts, map[string]string{"True-Client-IP": "2001:db8:1:2::10"})
		// The same /64 counts as one client.
		assert.Equal(t, login(t, ts, map[string]string{"True-Client-IP": "2001:db8:1:2::99"}), http.StatusTooManyRequests)
		assert.Equal(t, login(t, ts, map[string]string{"True-Client-IP": "2001:db8:1:3::10"}), http.StatusUnprocessableEntity)
		// X-Forwarded-For isn't the header named, so it's ignored.
		assert.Equal(t, login(t, ts, map[string]string{"True-Client-IP": "2001:db8:1:2::10", "X-Forwarded-For": "203.0.113.9"}), http.StatusTooManyRequests)
	})
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name    string
		header  string
		headers map[string]string
		want    string
	}{
		{"No proxy", "", map[string]string{"X-Forwarded-For": "203.0.113.1"}, "192.0.2.1"},
		{"Last hop", "X-Forwarded-For", map[string]string{"X-Forwarded-For": "1.2.3.4, 203.0.113.1"}, "203.0.113.1"},
		{"Only hop", "X-Forwarded-For", map[string]string{"X-Forwarded-For": "203.0.113.1"}, "203.0.113.1"},
		{"IPv6", "X-Forwarded-For", map[string]string{"X-Forwarded-For": "2001:db8::1"}, "2001:db8::1"},
		{"IPv4 written as IPv6", "True-Client-Ip", map[string]string{"True-Client-IP": "::ffff:203.0.113.1"}, "203.0.113.1"},
		{"Named header", "True-Client-Ip", map[string]string{"True-Client-IP": " 203.0.113.1 "}, "203.0.113.1"},
		{"Missing", "True-Client-Ip", nil, "192.0.2.1"},
		{"Not an address", "True-Client-Ip", map[string]string{"True-Client-IP": "unknown"}, "192.0.2.1"},
		{"A list where one is expected", "True-Client-Ip", map[string]string{"True-Client-IP": "1.2.3.4, 203.0.113.1"}, "192.0.2.1"},
		{"With a port", "X-Forwarded-For", map[string]string{"X-Forwarded-For": "203.0.113.1:4000"}, "192.0.2.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newTestApplication(t)
			app.clientIPHeader = tt.header
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			for name, value := range tt.headers {
				r.Header.Set(name, value)
			}
			assert.Equal(t, app.clientIP(r), tt.want)
		})
	}
}

// A missing header is logged once, not on every request.
func TestClientIPWarnsOnce(t *testing.T) {
	var log bytes.Buffer
	app := newTestApplication(t)
	app.logger = slog.New(slog.NewTextHandler(&log, nil))
	app.clientIPHeader = "True-Client-Ip"
	for range 3 {
		app.clientIP(httptest.NewRequest(http.MethodGet, "/", nil))
	}
	assert.Equal(t, strings.Count(log.String(), "client IP header missing"), 1)
}
