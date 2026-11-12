package main

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/darrenpaine/bookmarkbox/internal/assert"
	"github.com/darrenpaine/bookmarkbox/internal/models/mocks"
)

const (
	demoBanner       = "This is a public demo."
	demoLoginButton  = "Log in as Demo"
	demoPasswordNote = "The demo account's password can't be changed"
)

// newDemoServer runs the app in demo mode. The mock's User 1, Alice, who
// ts.login logs in as, is the Demo User when demoUserID is 1.
func newDemoServer(t *testing.T, demoUserID int64, signupClosed bool) *testServer {
	t.Helper()
	app := newTestApplication(t)
	app.demoMode = true
	app.demoUserID.Store(demoUserID)
	app.users = &mocks.UserModel{SignupClosed: signupClosed}
	return newTestServer(t, app.routes())
}

// Outside demo mode, none of the demo's notices or headers appear.
func TestNotDemoMode(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	defer ts.Close()

	for _, path := range []string{"/", "/user/login", "/user/signup"} {
		code, header, body := ts.get(t, path)
		assert.Equal(t, code, http.StatusOK)
		assert.Equal(t, header.Get("Strict-Transport-Security"), "")
		for _, text := range []string{demoBanner, demoLoginButton, "real password"} {
			if strings.Contains(body, text) {
				t.Errorf("%s shows %q", path, text)
			}
		}
	}
}

func TestDemoLoggedOut(t *testing.T) {
	ts := newDemoServer(t, 1, false)
	defer ts.Close()

	t.Run("Home page", func(t *testing.T) {
		code, header, body := ts.get(t, "/")
		assert.Equal(t, code, http.StatusOK)
		assert.Equal(t, header.Get("Strict-Transport-Security"), "max-age=31536000")
		assert.StringContains(t, body, demoBanner)
		assert.StringContains(t, body, "Try the demo")
		assert.StringContains(t, body, "<dd>demo@example.com</dd>")
		assert.StringContains(t, body, "<dd>demo-password-1</dd>")
		assert.StringContains(t, body, "<input type='hidden' name='email' value='demo@example.com'>")
		assert.StringContains(t, body, "<input type='hidden' name='password' value='demo-password-1'>")
		assert.StringContains(t, body, demoLoginButton)
		assert.StringContains(t, body, "Run your own")
	})

	t.Run("Login page", func(t *testing.T) {
		code, _, body := ts.get(t, "/user/login")
		assert.Equal(t, code, http.StatusOK)
		assert.StringContains(t, body, "<dd>demo@example.com</dd>")
		assert.StringContains(t, body, "<dd>demo-password-1</dd>")
		assert.StringContains(t, body, demoLoginButton)
	})

	t.Run("Every response is HTTPS only", func(t *testing.T) {
		_, header, _ := ts.get(t, "/ping")
		assert.Equal(t, header.Get("Strict-Transport-Security"), "max-age=31536000")
	})

	t.Run("Signup", func(t *testing.T) {
		code, _, body := ts.get(t, "/user/signup")
		assert.Equal(t, code, http.StatusOK)
		assert.StringContains(t, body, "Don't use a real password.")
		assert.StringContains(t, body, "Any email-shaped text")
		assert.StringContains(t, body, "deleted every 24 hours")
	})
}

// When every account is taken, the signup page points to the Demo login.
func TestDemoFull(t *testing.T) {
	ts := newDemoServer(t, 1, true)
	defer ts.Close()

	code, _, body := ts.get(t, "/user/signup")
	assert.Equal(t, code, http.StatusOK)
	assert.StringContains(t, body, "The demo is full")
	assert.StringContains(t, body, demoLoginButton)

	code, _, body = ts.postForm(t, "/user/signup", url.Values{"name": {"Bob"}, "email": {"bob@example.com"}, "password": {"long-enough"}})
	assert.Equal(t, code, http.StatusForbidden)
	assert.StringContains(t, body, "The demo is full")
}

func TestDemoUser(t *testing.T) {
	ts := newDemoServer(t, 1, false)
	defer ts.Close()
	ts.login(t)

	t.Run("Banner when logged in", func(t *testing.T) {
		_, _, body := ts.get(t, "/bookmarks")
		assert.StringContains(t, body, demoBanner)
	})

	t.Run("Account page", func(t *testing.T) {
		code, _, body := ts.get(t, "/account/view")
		assert.Equal(t, code, http.StatusOK)
		assert.StringContains(t, body, demoPasswordNote)
		if strings.Contains(body, "href='/account/password/update'") {
			t.Error("the Demo User is offered a password change")
		}
	})

	t.Run("Password change is refused", func(t *testing.T) {
		code, _, body := ts.get(t, "/account/password/update")
		assert.Equal(t, code, http.StatusForbidden)
		assert.StringContains(t, body, demoPasswordNote)

		form := url.Values{"old_password": {"pa$$word"}, "new_password": {"new-password"}, "confirm_password": {"new-password"}}
		code, header, body := ts.postForm(t, "/account/password/update", form)
		assert.Equal(t, code, http.StatusForbidden)
		assert.Equal(t, header.Get("Location"), "")
		assert.StringContains(t, body, demoPasswordNote)
	})

	t.Run("Import warning", func(t *testing.T) {
		_, _, body := ts.get(t, "/account/import")
		assert.StringContains(t, body, "don't upload anything")
	})
}

// Visitors who sign up on the demo can change their own password.
func TestDemoSignedUpUser(t *testing.T) {
	ts := newDemoServer(t, 2, false)
	defer ts.Close()
	ts.login(t)

	code, _, body := ts.get(t, "/account/view")
	assert.Equal(t, code, http.StatusOK)
	assert.StringContains(t, body, "href='/account/password/update'")

	code, _, _ = ts.get(t, "/account/password/update")
	assert.Equal(t, code, http.StatusOK)

	form := url.Values{"old_password": {"pa$$word"}, "new_password": {"new-password"}, "confirm_password": {"new-password"}}
	code, header, _ := ts.postForm(t, "/account/password/update", form)
	assert.Equal(t, code, http.StatusSeeOther)
	assert.Equal(t, header.Get("Location"), "/account/view")
}
