package main

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/url"
	"testing"

	"snippetbox.darrenpaine.com/internal/assert"
)

func TestPing(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	defer ts.Close()

	t.Run("Same Origin", func(t *testing.T) {
		// Test GET /ping
		code, _, body := ts.get(t, "/ping")
		// Is it OK, "OK"?
		assert.Equal(t, code, http.StatusOK)
		assert.Equal(t, string(body), "OK")
	})

	// Test cross-origin GET /ping - should still work
	t.Run("Cross Origin", func(t *testing.T) {
		req, err := http.NewRequest("GET", ts.URL+"/ping", nil)
		if err != nil {
			log.Fatal(err)
		}
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		req.Header.Set("Origin", "https://evil.com")

		resp, err1 := ts.Client().Do(req)
		if err1 != nil {
			t.Fatal(err1)
		}
		// Read the response body from the test server.
		defer resp.Body.Close()
		resp_body, err2 := io.ReadAll(resp.Body)
		if err2 != nil {
			t.Fatal(err2)
		}
		assert.Equal(t, resp.StatusCode, http.StatusOK)
		assert.Equal(t, string(bytes.TrimSpace(resp_body)), "OK")
	})
}
func TestSnippetView(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	defer ts.Close()
	tests := []struct {
		name     string
		urlPath  string
		wantCode int
		wantBody string
	}{
		{
			name:     "Valid ID",
			urlPath:  "/snippet/view/1",
			wantCode: http.StatusOK,
			wantBody: "An old silent pond...",
		},
		{
			name:     "Non-existent ID",
			urlPath:  "/snippet/view/2",
			wantCode: http.StatusNotFound,
		},
		{
			name:     "Negative ID",
			urlPath:  "/snippet/view/-1",
			wantCode: http.StatusNotFound,
		},
		{
			name:     "Decimal ID",
			urlPath:  "/snippet/view/1.23",
			wantCode: http.StatusNotFound,
		},
		{
			name:     "String ID",
			urlPath:  "/snippet/view/foo",
			wantCode: http.StatusNotFound,
		},
		{
			name:     "Empty ID",
			urlPath:  "/snippet/view/",
			wantCode: http.StatusNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, _, body := ts.get(t, tt.urlPath)
			assert.Equal(t, code, tt.wantCode)
			if tt.wantBody != "" {
				assert.StringContains(t, body, tt.wantBody)
			}
		})
	}
}

// Test User Signup Form
func TestUserSignup(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	defer ts.Close()

	const (
		validName     = "Bob"
		validPassword = "validPa$$word"
		validEmail    = "bob@example.com"
		formTag       = "<form action='/user/signup' method='POST' novalidate>"
	)
	tests := []struct {
		name         string
		userName     string
		userEmail    string
		userPassword string
		wantCode     int
		wantFormTag  string
	}{
		{
			name:         "Valid submission",
			userName:     validName,
			userEmail:    validEmail,
			userPassword: validPassword,
			wantCode:     http.StatusSeeOther,
		},
		{
			name:         "Empty name",
			userName:     "",
			userEmail:    validEmail,
			userPassword: validPassword,
			wantCode:     http.StatusUnprocessableEntity,
			wantFormTag:  formTag,
		},
		{
			name:         "Empty email",
			userName:     validName,
			userEmail:    "",
			userPassword: validPassword,
			wantCode:     http.StatusUnprocessableEntity,
			wantFormTag:  formTag,
		},
		{
			name:         "Empty password",
			userName:     validName,
			userEmail:    validEmail,
			userPassword: "",
			wantCode:     http.StatusUnprocessableEntity,
			wantFormTag:  formTag,
		},
		{
			name:         "Invalid email",
			userName:     validName,
			userEmail:    "bob@example.",
			userPassword: validPassword,
			wantCode:     http.StatusUnprocessableEntity,
			wantFormTag:  formTag,
		},
		{
			name:         "Short password",
			userName:     validName,
			userEmail:    validEmail,
			userPassword: "pa$$",
			wantCode:     http.StatusUnprocessableEntity,
			wantFormTag:  formTag,
		},
		{
			name:         "Duplicate email",
			userName:     validName,
			userEmail:    "dupe@example.com",
			userPassword: validPassword,
			wantCode:     http.StatusUnprocessableEntity,
			wantFormTag:  formTag,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := url.Values{}
			form.Add("name", tt.userName)
			form.Add("email", tt.userEmail)
			form.Add("password", tt.userPassword)
			code, _, body := ts.postForm(t, "/user/signup", form)
			assert.Equal(t, code, tt.wantCode)
			if tt.wantFormTag != "" {
				assert.StringContains(t, body, tt.wantFormTag)
			}
		})
	}

	csrf_tests := []struct {
		name         string
		userName     string
		userEmail    string
		userPassword string
		wantCode     int
		wantError    string
	}{
		{
			name:         "Cross Origin Post",
			userName:     validName,
			userEmail:    validEmail,
			userPassword: validPassword,
			wantCode:     http.StatusForbidden,
			wantError:    "cross-origin request detected from Sec-Fetch-Site header\n",
		},
	}

	for _, tt := range csrf_tests {
		t.Run(tt.name, func(t *testing.T) {
			form := url.Values{}
			form.Add("name", tt.userName)
			form.Add("email", tt.userEmail)
			form.Add("password", tt.userPassword)
			code, _, body := ts.postFormCrossOrigin(t, "/user/signup", form)
			assert.Equal(t, code, tt.wantCode)
			if tt.wantError != "" {
				assert.StringContains(t, body, tt.wantError)
			}
		})
	}
}
