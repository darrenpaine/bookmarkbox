package main

import (
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/darrenpaine/bookmarkbox/internal/assert"
)

// The theme switch on the Account page, and the theme on every page.
func TestTheme(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	defer ts.Close()

	t.Run("Logged out pages follow the device", func(t *testing.T) {
		_, _, body := ts.get(t, "/user/login")
		assert.StringContains(t, body, "<html lang='en'>")
		assert.StringContains(t, body, "<meta name='color-scheme' content='light dark'>")
	})

	t.Run("Logged out can't save a theme", func(t *testing.T) {
		code, header, _ := ts.postForm(t, "/account/theme", url.Values{"theme": {"dark"}})
		assert.Equal(t, code, http.StatusSeeOther)
		assert.Equal(t, header.Get("Location"), "/user/login")
	})

	ts.login(t)

	t.Run("System by default", func(t *testing.T) {
		_, _, body := ts.get(t, "/account/view")
		assert.StringContains(t, body, "<html lang='en'>")
		assert.StringContains(t, body, "<meta name='color-scheme' content='light dark'>")
		assert.StringContains(t, body, "<input type='radio' name='theme' value='system' checked>System")
		assert.StringContains(t, body, "<input type='radio' name='theme' value='dark'>Dark")
	})

	t.Run("Saving without htmx", func(t *testing.T) {
		code, header, _ := ts.postForm(t, "/account/theme", url.Values{"theme": {"dark"}})
		assert.Equal(t, code, http.StatusSeeOther)
		assert.Equal(t, header.Get("Location"), "/account/view")

		_, _, body := ts.get(t, "/account/view")
		assert.StringContains(t, body, "<html lang='en' data-theme='dark'>")
		assert.StringContains(t, body, "<meta name='color-scheme' content='dark'>")
		assert.StringContains(t, body, "<input type='radio' name='theme' value='dark' checked>Dark")
		assert.StringContains(t, body, "Theme saved.")

		// Every page, not just the Account page.
		_, _, body = ts.get(t, "/bookmarks")
		assert.StringContains(t, body, "<html lang='en' data-theme='dark'>")
	})

	t.Run("Saving with htmx reloads the page", func(t *testing.T) {
		code, header, body := ts.postHTMX(t, "/account/theme", url.Values{"theme": {"light"}}, nil)
		assert.Equal(t, code, http.StatusOK)
		assert.Equal(t, header.Get("HX-Refresh"), "true")
		assert.Equal(t, body, "")

		_, _, body = ts.get(t, "/account/view")
		assert.StringContains(t, body, "<html lang='en' data-theme='light'>")
		assert.StringContains(t, body, "<meta name='color-scheme' content='light'>")
	})

	t.Run("Back to System", func(t *testing.T) {
		ts.postForm(t, "/account/theme", url.Values{"theme": {"system"}})
		_, _, body := ts.get(t, "/account/view")
		assert.StringContains(t, body, "<html lang='en'>")
	})

	t.Run("Unknown themes are refused", func(t *testing.T) {
		for _, theme := range []string{"", "sepia", "DARK", "dark' onload='x"} {
			code, _, _ := ts.postForm(t, "/account/theme", url.Values{"theme": {theme}})
			assert.Equal(t, code, http.StatusBadRequest)
		}
	})

	t.Run("Cross origin", func(t *testing.T) {
		code, _, _ := ts.postFormCrossOrigin(t, "/account/theme", url.Values{"theme": {"dark"}})
		assert.Equal(t, code, http.StatusForbidden)
	})
}

// On small screens Account is the User's initial in the bar, and Log out is on
// the Account page.
func TestAccountMark(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	defer ts.Close()

	_, _, body := ts.get(t, "/")
	if strings.Contains(body, "account-mark") {
		t.Error("logged-out page shows the Account mark")
	}

	ts.login(t)
	_, _, body = ts.get(t, "/bookmarks")
	assert.StringContains(t, body, "<a class='account-mark' href='/account/view'>")
	assert.StringContains(t, body, "<span aria-hidden='true'>A</span>")
	assert.StringContains(t, body, "<span class='visually-hidden'>Account</span>")

	_, _, body = ts.get(t, "/account/view")
	assert.StringContains(t, body, "<a class='account-mark' href='/account/view' aria-current='page'>")
	assert.StringContains(t, body, "<form class='account-logout' action='/user/logout' method='POST'>")
}

// The dark colours are written twice: for a chosen Dark theme, and for the
// System theme on a dark device. The two copies must match.
func TestDarkThemeBlocksMatch(t *testing.T) {
	css, err := os.ReadFile("../../ui/static/css/main.css")
	if err != nil {
		t.Fatal(err)
	}
	chosen := regexp.MustCompile(`(?s):root\[data-theme="dark"\] \{\s*color-scheme: dark;(.*?)\}`).FindSubmatch(css)
	device := regexp.MustCompile(`(?s):root:not\(\[data-theme="light"\]\) \{(.*?)\}`).FindSubmatch(css)
	if chosen == nil || device == nil {
		t.Fatal("dark theme blocks not found in main.css")
	}
	tokens := func(block []byte) string { return strings.Join(strings.Fields(string(block)), " ") }
	assert.Equal(t, tokens(device[1]), tokens(chosen[1]))
}
