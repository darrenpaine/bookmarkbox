package main

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/darrenpaine/bookmarkbox/internal/assert"
	"github.com/darrenpaine/bookmarkbox/internal/models/mocks"
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

// login logs the test client in as Alice (user 1 in the mocks).
func (ts *testServer) login(t *testing.T) {
	form := url.Values{}
	form.Add("email", "alice@example.com")
	form.Add("password", "pa$$word")
	code, _, _ := ts.postForm(t, "/user/login", form)
	assert.Equal(t, code, http.StatusSeeOther)
}

func TestBookmarkView(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	defer ts.Close()

	t.Run("Unauthenticated redirects to login", func(t *testing.T) {
		code, header, _ := ts.get(t, "/bookmarks/1")
		assert.Equal(t, code, http.StatusSeeOther)
		assert.StringContains(t, header.Get("Location"), "/user/login")
	})

	ts.login(t)
	tests := []struct {
		name     string
		urlPath  string
		wantCode int
		wantBody string
	}{
		{
			name:     "Own bookmark",
			urlPath:  "/bookmarks/1",
			wantCode: http.StatusOK,
			wantBody: "Effective Go",
		},
		{
			name:     "Non-existent or other user's ID",
			urlPath:  "/bookmarks/2",
			wantCode: http.StatusNotFound,
		},
		{
			name:     "Negative ID",
			urlPath:  "/bookmarks/-1",
			wantCode: http.StatusNotFound,
		},
		{
			name:     "Decimal ID",
			urlPath:  "/bookmarks/1.23",
			wantCode: http.StatusNotFound,
		},
		{
			name:     "String ID",
			urlPath:  "/bookmarks/foo",
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

func TestHome(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	defer ts.Close()

	code, _, body := ts.get(t, "/")
	assert.Equal(t, code, http.StatusOK)
	assert.StringContains(t, body, "Welcome to BookmarkBox")

	ts.login(t)
	code, _, body = ts.get(t, "/")
	assert.Equal(t, code, http.StatusOK)
	assert.StringContains(t, body, "<a href='/bookmarks/1'>Effective Go</a>")
	assert.StringContains(t, body, "go.dev")
}

// Test User Signup Form
func TestUserSignup(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	defer ts.Close()

	const (
		validName     = "Alice"
		validPassword = "pa$$word"
		validEmail    = "alice@example.com"
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
			name:         "Cross Origin Post Blocked",
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

// Test for Bookmark Create
func TestBookmarkCreate(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	defer ts.Close()

	const formTag = "<form action='/bookmarks/new' method='POST'>"

	t.Run("Unauthenticated has no access", func(t *testing.T) {
		code, header, _ := ts.get(t, "/bookmarks/new")
		assert.Equal(t, code, http.StatusSeeOther)
		assert.StringContains(t, header.Get("Location"), "/user/login")
	})

	ts.login(t)

	t.Run("Authenticated has access", func(t *testing.T) {
		code, _, body := ts.get(t, "/bookmarks/new")
		assert.Equal(t, code, http.StatusOK)
		assert.StringContains(t, body, formTag)
	})

	tests := []struct {
		name      string
		url       string
		title     string
		wantCode  int
		wantError string
	}{
		{
			name:     "Valid submission",
			url:      "https://pkg.go.dev",
			title:    "Go packages",
			wantCode: http.StatusSeeOther,
		},
		{
			name:      "Empty URL",
			url:       "",
			title:     "Go packages",
			wantCode:  http.StatusUnprocessableEntity,
			wantError: "This field cannot be blank",
		},
		{
			name:      "Not a web URL",
			url:       "ftp://example.com",
			title:     "FTP",
			wantCode:  http.StatusUnprocessableEntity,
			wantError: "This must be a full http:// or https:// address",
		},
		{
			name:      "Empty title",
			url:       "https://pkg.go.dev",
			title:     "  ",
			wantCode:  http.StatusUnprocessableEntity,
			wantError: "This field cannot be blank",
		},
		{
			name:      "Title too long",
			url:       "https://pkg.go.dev",
			title:     strings.Repeat("a", 201),
			wantCode:  http.StatusUnprocessableEntity,
			wantError: "This field cannot be more than 200 characters long",
		},
		{
			name:      "URL too long",
			url:       "https://pkg.go.dev/" + strings.Repeat("a", 2000),
			title:     "Long",
			wantCode:  http.StatusUnprocessableEntity,
			wantError: "This field cannot be more than 2000 characters long",
		},
		{
			name:      "Duplicate URL",
			url:       "https://go.dev/doc/effective_go",
			title:     "Effective Go",
			wantCode:  http.StatusUnprocessableEntity,
			wantError: "You have already saved this address",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := url.Values{}
			form.Add("url", tt.url)
			form.Add("title", tt.title)
			form.Add("tags", "go, docs")
			code, header, body := ts.postForm(t, "/bookmarks/new", form)
			assert.Equal(t, code, tt.wantCode)
			if tt.wantError != "" {
				assert.StringContains(t, body, tt.wantError)
				assert.StringContains(t, body, formTag)
			} else {
				assert.Equal(t, header.Get("Location"), "/bookmarks/2")
			}
		})
	}
}

func TestBookmarkEdit(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	defer ts.Close()

	t.Run("Unauthenticated redirects to login", func(t *testing.T) {
		code, header, _ := ts.get(t, "/bookmarks/1/edit")
		assert.Equal(t, code, http.StatusSeeOther)
		assert.StringContains(t, header.Get("Location"), "/user/login")
	})

	ts.login(t)

	t.Run("Form is filled in", func(t *testing.T) {
		code, _, body := ts.get(t, "/bookmarks/1/edit")
		assert.Equal(t, code, http.StatusOK)
		assert.StringContains(t, body, "<form action='/bookmarks/1/edit' method='POST'>")
		assert.StringContains(t, body, "value='Effective Go'")
		assert.StringContains(t, body, "value='docs, go'")
	})

	t.Run("Other user's bookmark", func(t *testing.T) {
		code, _, _ := ts.get(t, "/bookmarks/2/edit")
		assert.Equal(t, code, http.StatusNotFound)
	})

	tests := []struct {
		name      string
		path      string
		url       string
		title     string
		wantCode  int
		wantError string
	}{
		{"Valid", "/bookmarks/1/edit", "https://go.dev/doc", "Docs", http.StatusSeeOther, ""},
		{"Invalid stays on edit form", "/bookmarks/1/edit", "https://go.dev/doc", "", http.StatusUnprocessableEntity, "This field cannot be blank"},
		{"Duplicate URL", "/bookmarks/1/edit", mocks.DuplicateURL, "Docs", http.StatusUnprocessableEntity, "You have already saved this address"},
		{"Other user's bookmark", "/bookmarks/2/edit", "https://go.dev/doc", "Docs", http.StatusNotFound, ""},
		{"Invalid ID", "/bookmarks/foo/edit", "https://go.dev/doc", "Docs", http.StatusNotFound, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := url.Values{}
			form.Add("url", tt.url)
			form.Add("title", tt.title)
			code, header, body := ts.postForm(t, tt.path, form)
			assert.Equal(t, code, tt.wantCode)
			if tt.wantError != "" {
				assert.StringContains(t, body, tt.wantError)
				assert.StringContains(t, body, "<form action='/bookmarks/1/edit' method='POST'>")
			}
			if tt.wantCode == http.StatusSeeOther {
				assert.Equal(t, header.Get("Location"), "/bookmarks/1")
			}
		})
	}

	t.Run("Cross origin post blocked", func(t *testing.T) {
		code, _, _ := ts.postFormCrossOrigin(t, "/bookmarks/1/edit", url.Values{})
		assert.Equal(t, code, http.StatusForbidden)
	})
}

func TestTags(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	defer ts.Close()

	t.Run("Unauthenticated redirects to login", func(t *testing.T) {
		code, header, _ := ts.get(t, "/tags")
		assert.Equal(t, code, http.StatusSeeOther)
		assert.StringContains(t, header.Get("Location"), "/user/login")
	})

	ts.login(t)

	t.Run("List", func(t *testing.T) {
		code, _, body := ts.get(t, "/tags")
		assert.Equal(t, code, http.StatusOK)
		assert.StringContains(t, body, "<form action='/tags/2/rename' method='POST'>")
	})

	tests := []struct {
		name     string
		path     string
		newName  string
		confirm  string
		wantCode int
		wantBody string
	}{
		{"Rename", "/tags/1/rename", "documentation", "", http.StatusSeeOther, ""},
		{"Same name after normalising", "/tags/1/rename", " DOCS ", "", http.StatusSeeOther, ""},
		{"Blank", "/tags/1/rename", "  ", "", http.StatusUnprocessableEntity, "cannot be blank"},
		{"Comma", "/tags/1/rename", "a,b", "", http.StatusUnprocessableEntity, "cannot contain a comma"},
		{"Too long", "/tags/1/rename", strings.Repeat("a", 51), "", http.StatusUnprocessableEntity, "50 characters or fewer"},
		{"Existing name asks first", "/tags/1/rename", "go", "", http.StatusOK, "will merge into it, affecting\n    1 bookmark."},
		{"Plural count", "/tags/2/rename", "docs", "", http.StatusOK, "3 bookmarks."},
		{"Confirmed merge", "/tags/1/rename", "go", "true", http.StatusSeeOther, ""},
		{"Other user's tag", "/tags/9/rename", "x", "", http.StatusNotFound, ""},
		{"Invalid ID", "/tags/foo/rename", "x", "", http.StatusNotFound, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := url.Values{}
			form.Add("name", tt.newName)
			if tt.confirm != "" {
				form.Add("confirm", tt.confirm)
			}
			code, _, body := ts.postForm(t, tt.path, form)
			assert.Equal(t, code, tt.wantCode)
			if tt.wantBody != "" {
				assert.StringContains(t, body, tt.wantBody)
			}
		})
	}

	t.Run("Confirmation posts the merge", func(t *testing.T) {
		form := url.Values{}
		form.Add("name", "go")
		_, _, body := ts.postForm(t, "/tags/1/rename", form)
		assert.StringContains(t, body, "<input type='hidden' name='confirm' value='true'>")
		assert.StringContains(t, body, "<input type='hidden' name='name' value='go'>")
	})

	t.Run("Cross origin post blocked", func(t *testing.T) {
		code, _, _ := ts.postFormCrossOrigin(t, "/tags/1/rename", url.Values{})
		assert.Equal(t, code, http.StatusForbidden)
	})
}

func TestBookmarkLimits(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	defer ts.Close()
	ts.login(t)

	tests := []struct {
		name      string
		path      string
		title     string
		tags      string
		wantError string
	}{
		{"Bookmark limit", "/bookmarks/new", mocks.OverBookmarkLimitTitle, "", "You have reached your limit of bookmarks"},
		{"Tag limit on add", "/bookmarks/new", "Fine", mocks.OverTagLimitTag, "You have reached your limit of tags"},
		{"Tag limit on edit", "/bookmarks/1/edit", "Fine", mocks.OverTagLimitTag, "You have reached your limit of tags"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := url.Values{}
			form.Add("url", "https://pkg.go.dev")
			form.Add("title", tt.title)
			form.Add("tags", tt.tags)
			code, _, body := ts.postForm(t, tt.path, form)
			assert.Equal(t, code, http.StatusUnprocessableEntity)
			assert.StringContains(t, body, tt.wantError)
			assert.StringContains(t, body, "value='"+tt.title+"'")
		})
	}
}

func TestSignupClosed(t *testing.T) {
	signupForm := func() url.Values {
		form := url.Values{}
		form.Add("name", "Carol")
		form.Add("email", "carol@example.com")
		form.Add("password", "pa$$word")
		return form
	}

	t.Run("Open shows link and form", func(t *testing.T) {
		app := newTestApplication(t)
		ts := newTestServer(t, app.routes())
		defer ts.Close()
		_, _, home := ts.get(t, "/")
		assert.StringContains(t, home, "<a href='/user/signup'>Signup</a>")
		_, _, body := ts.get(t, "/user/signup")
		assert.StringContains(t, body, "<form action='/user/signup'")
	})

	t.Run("Closed hides link and form", func(t *testing.T) {
		app := newTestApplication(t)
		app.users = &mocks.UserModel{SignupClosed: true}
		ts := newTestServer(t, app.routes())
		defer ts.Close()

		_, _, home := ts.get(t, "/")
		if strings.Contains(home, "/user/signup") {
			t.Error("home page still links to signup")
		}
		assert.StringContains(t, home, "<a href='/user/login'>Log in</a>")

		code, _, body := ts.get(t, "/user/signup")
		assert.Equal(t, code, http.StatusOK)
		assert.StringContains(t, body, "Signup is closed")
		if strings.Contains(body, "<form action='/user/signup'") {
			t.Error("closed page shows the form")
		}

		code, _, body = ts.postForm(t, "/user/signup", signupForm())
		assert.Equal(t, code, http.StatusForbidden)
		assert.StringContains(t, body, "Signup is closed")

		// Logging in must still work.
		ts.login(t)
	})

	t.Run("Losing the race for the last place", func(t *testing.T) {
		app := newTestApplication(t)
		ts := newTestServer(t, app.routes())
		defer ts.Close()
		form := signupForm()
		form.Set("email", mocks.FullEmail)
		code, _, body := ts.postForm(t, "/user/signup", form)
		assert.Equal(t, code, http.StatusForbidden)
		assert.StringContains(t, body, "Signup is closed")
	})
}

func TestBookmarkCreateTextLimits(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	defer ts.Close()
	ts.login(t)

	tests := []struct {
		name      string
		field     string
		value     string
		wantCode  int
		wantError string
	}{
		{"Description at limit", "description", strings.Repeat("a", 2000), http.StatusSeeOther, ""},
		{"Description too long", "description", strings.Repeat("a", 2001), http.StatusUnprocessableEntity, "more than 2000 characters"},
		{"Notes at limit", "notes", strings.Repeat("a", 10000), http.StatusSeeOther, ""},
		{"Notes too long", "notes", strings.Repeat("a", 10001), http.StatusUnprocessableEntity, "more than 10000 characters"},
		{"Multi-byte notes count characters", "notes", strings.Repeat("é", 10000), http.StatusSeeOther, ""},
		{"Tag at limit", "tags", strings.Repeat("a", 50), http.StatusSeeOther, ""},
		{"Tag too long", "tags", "go, " + strings.Repeat("a", 51), http.StatusUnprocessableEntity, "50 characters or fewer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := url.Values{}
			form.Add("url", "https://pkg.go.dev")
			form.Add("title", "Go packages")
			form.Add(tt.field, tt.value)
			code, _, body := ts.postForm(t, "/bookmarks/new", form)
			assert.Equal(t, code, tt.wantCode)
			if tt.wantError != "" {
				assert.StringContains(t, body, tt.wantError)
			}
		})
	}
}

func TestFormSizeLimit(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	defer ts.Close()
	ts.login(t)

	form := url.Values{}
	form.Add("notes", strings.Repeat("a", maxFormBytes))
	code, _, _ := ts.postForm(t, "/bookmarks/new", form)
	assert.Equal(t, code, http.StatusBadRequest)
}

// Favourite and delete only work on the user's own bookmarks.
func TestBookmarkActions(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	defer ts.Close()
	ts.login(t)

	for _, action := range []string{"favourite", "delete"} {
		t.Run(action+" own", func(t *testing.T) {
			code, _, _ := ts.postForm(t, "/bookmarks/1/"+action, url.Values{})
			assert.Equal(t, code, http.StatusSeeOther)
		})
		t.Run(action+" other", func(t *testing.T) {
			code, _, _ := ts.postForm(t, "/bookmarks/2/"+action, url.Values{})
			assert.Equal(t, code, http.StatusNotFound)
		})
	}
}

// Test for Change User Password
func TestUserChangePassword(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	defer ts.Close()

	const (
		validName     = "Alice"
		validPassword = "pa$$word"
		validEmail    = "alice@example.com"
		validNewPass  = "pa$$word2"
		formTag       = "<form action='/account/password/update' method='POST'>"
	)

	t.Run("Unauthenticated has no access", func(t *testing.T) {
		code, header, _ := ts.get(t, "/account/password/update")
		// Is it OK, "OK"?
		assert.Equal(t, code, http.StatusSeeOther)
		assert.StringContains(t, header.Get("Location"), "/user/login")
	})

	t.Run("Authenticated has access", func(t *testing.T) {

		update_form := url.Values{}
		update_form.Add("email", validEmail)
		update_form.Add("password", validPassword)
		ts.postForm(t, "/user/login", update_form)

		code, _, body := ts.get(t, "/account/password/update")
		// Is it showing the form?
		assert.Equal(t, code, http.StatusOK)
		assert.StringContains(t, body, "<form action='/account/password/update' method='POST'>")
	})

	csrf_tests := []struct {
		name         string
		userName     string
		userEmail    string
		userPassword string
		wantCode     int
		wantError    string
	}{
		{
			name:         "Cross Origin Post Blocked",
			userPassword: validPassword,
			wantCode:     http.StatusForbidden,
			wantError:    "cross-origin request detected from Sec-Fetch-Site header\n",
		},
	}

	for _, tt := range csrf_tests {
		t.Run(tt.name, func(t *testing.T) {
			login_form := url.Values{}
			login_form.Add("email", validEmail)
			login_form.Add("password", validPassword)
			ts.postForm(t, "/user/login", login_form)

			form := url.Values{}
			form.Add("old_password", validPassword)
			form.Add("new_password", validPassword)
			form.Add("confirm_password", validPassword)
			code, _, body := ts.postFormCrossOrigin(t, "/account/password/update", form)
			assert.Equal(t, code, tt.wantCode)
			if tt.wantError != "" {
				assert.StringContains(t, body, tt.wantError)
			}
		})
	}

	tests := []struct {
		name            string
		userName        string
		userEmail       string
		userPassword    string
		userNewPass     string
		userConfirmPass string
		wantCode        int
		wantFormTag     string
	}{
		{
			name:            "Valid submission",
			userName:        validName,
			userEmail:       validEmail,
			userPassword:    validPassword,
			userNewPass:     validNewPass,
			userConfirmPass: validNewPass,
			wantCode:        http.StatusSeeOther,
		},
		{
			name:            "Wrong password",
			userName:        validName,
			userEmail:       validEmail,
			userPassword:    "pa$$",
			userNewPass:     validNewPass,
			userConfirmPass: validNewPass,
			wantCode:        http.StatusUnprocessableEntity,
		},
		{
			name:            "Empty old password",
			userEmail:       validEmail,
			userPassword:    "",
			userNewPass:     validNewPass,
			userConfirmPass: validNewPass,
			wantCode:        http.StatusUnprocessableEntity,
			wantFormTag:     formTag,
		},
		{
			name:            "Empty new password",
			userEmail:       validEmail,
			userPassword:    validPassword,
			userNewPass:     "",
			userConfirmPass: validNewPass,
			wantCode:        http.StatusUnprocessableEntity,
			wantFormTag:     formTag,
		},
		{
			name:            "Empty confirm password",
			userEmail:       validEmail,
			userPassword:    validPassword,
			userNewPass:     validNewPass,
			userConfirmPass: "",
			wantCode:        http.StatusUnprocessableEntity,
			wantFormTag:     formTag,
		},
		{
			name:            "New passwords do not match",
			userEmail:       validEmail,
			userPassword:    validPassword,
			userNewPass:     validNewPass,
			userConfirmPass: validPassword,
			wantCode:        http.StatusUnprocessableEntity,
			wantFormTag:     formTag,
		},
		{
			name:            "New password too short",
			userEmail:       validEmail,
			userPassword:    validPassword,
			userNewPass:     "pa$$",
			userConfirmPass: "pa$$",
			wantCode:        http.StatusUnprocessableEntity,
			wantFormTag:     formTag,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			login_form := url.Values{}
			login_form.Add("email", validEmail)
			login_form.Add("password", validPassword)
			ts.postForm(t, "/user/login", login_form)

			form := url.Values{}
			form.Add("old_password", tt.userPassword)
			form.Add("new_password", tt.userNewPass)
			form.Add("confirm_password", tt.userConfirmPass)
			code, _, body := ts.postForm(t, "/account/password/update", form)
			assert.Equal(t, code, tt.wantCode)
			if tt.wantFormTag != "" {
				assert.StringContains(t, body, tt.wantFormTag)
			}
		})
	}
}
