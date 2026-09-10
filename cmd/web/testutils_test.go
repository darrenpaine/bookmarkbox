package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/form/v4"
	"snippetbox.darrenpaine.com/internal/models/mocks"
)

// Create a newTestApplication helper which returns an instance of our
// application struct containing mocked dependencies.
func newTestApplication(t *testing.T) *application {
	templateCache, err := newTemplateCache()
	if err != nil {
		t.Fatal(err)
	}
	// And a form decoder.
	formDecoder := form.NewDecoder()
	// And a session manager instance. Note that we use the same settings as
	// production, except that we *don't* set a Store for the session manager.
	// If no store is set, the SCS package will default to using a transient
	// in-memory store, which is ideal for testing purposes.
	sessionManager := scs.New()
	sessionManager.Lifetime = 12 * time.Hour
	sessionManager.Cookie.Secure = true
	return &application{
		errorLog:       log.New(io.Discard, "", 0),
		infoLog:        log.New(io.Discard, "", 0),
		router:         chi.NewRouter(),
		snippets:       &mocks.SnippetModel{}, // Use the mock.
		users:          &mocks.UserModel{},    // Use the mock.
		templateCache:  templateCache,
		formDecoder:    formDecoder,
		sessionManager: sessionManager,
	}
}

// Define a custom testServer type which embeds a httptest.Server instance.
type testServer struct {
	*httptest.Server
}

// newTestServer() helper which initalizes and returns a custom testServer.
func newTestServer(t *testing.T, h http.Handler) *testServer {
	csrf := http.NewCrossOriginProtection()
	csrf.AddTrustedOrigin("https://trusted.example.com")
	ts := httptest.NewTLSServer(csrf.Handler(h))
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	// Collect cookies during testing.
	ts.Client().Jar = jar
	// Disable redirect-following for the test server client by setting a custom
	// CheckRedirect function. By always returning a http.ErrUseLastResponse
	// error it forces the client to immediately return the received response.
	ts.Client().CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &testServer{ts}
}

// get() makes a GET request to a given url path using the test server client.
// Returns the response status code, headers and body.
func (ts *testServer) get(t *testing.T, urlPath string) (int, http.Header, string) {
	rs, err := ts.Client().Get(ts.URL + urlPath)
	if err != nil {
		t.Fatal(err)
	}
	defer rs.Body.Close()
	body, err := io.ReadAll(rs.Body)
	if err != nil {
		t.Fatal(err)
	}
	bytes.TrimSpace(body)
	return rs.StatusCode, rs.Header, string(body)
}

// Helper to test POST requests
func (ts *testServer) postForm(t *testing.T, urlPath string, form url.Values) (int, http.Header, string) {
	rs, err := ts.Client().PostForm(ts.URL+urlPath, form)
	if err != nil {
		t.Fatal(err)
	}
	// Read the response body from the test server.
	defer rs.Body.Close()
	body, err := io.ReadAll(rs.Body)
	if err != nil {
		t.Fatal(err)
	}
	bytes.TrimSpace(body)
	// Return the response status, headers and body.
	return rs.StatusCode, rs.Header, string(body)
}
func (ts *testServer) postFormCrossOrigin(t *testing.T, urlPath string, form url.Values) (int, http.Header, string) {
	m := make(map[string]string, len(form))
	for k, v := range form {
		m[k] = v[0]
	}
	form_json, err_json := json.Marshal(m)
	if err_json != nil {
		log.Fatal(err_json)
	}
	req_body := bytes.NewReader(form_json)
	req, err := http.NewRequest("POST", ts.URL+urlPath, req_body)
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
	bytes.TrimSpace(resp_body)
	// Return the response status, headers and body.
	return resp.StatusCode, resp.Header, string(resp_body)
}
