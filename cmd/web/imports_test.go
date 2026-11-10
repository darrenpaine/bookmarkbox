package main

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/darrenpaine/bookmarkbox/internal/assert"
	"github.com/darrenpaine/bookmarkbox/internal/models"
	"github.com/darrenpaine/bookmarkbox/internal/models/mocks"
)

// upload posts a file to the import form, as a browser would. An empty
// filename sends the form with no file chosen.
func (ts *testServer) upload(t *testing.T, filename string, content []byte) (int, http.Header, string) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if filename != "" {
		part, err := mw.CreateFormFile("file", filename)
		if err != nil {
			t.Fatal(err)
		}
		part.Write(content)
	}
	mw.Close()
	rs, err := ts.Client().Post(ts.URL+"/account/import", mw.FormDataContentType(), &body)
	if err != nil {
		t.Fatal(err)
	}
	defer rs.Body.Close()
	b, err := io.ReadAll(rs.Body)
	if err != nil {
		t.Fatal(err)
	}
	return rs.StatusCode, rs.Header, string(b)
}

// bookmarkFile is a small export: one link Alice already has (in the mocks),
// one new link in no Folder, and one that isn't a web address.
func bookmarkFile(newTitle string) []byte {
	return []byte(`<!DOCTYPE NETSCAPE-Bookmark-file-1>
<DL><p>
    <DT><H3>Go</H3>
    <DL><p>
        <DT><A HREF="https://go.dev/doc/effective_go" ADD_DATE="1700000000">Effective Go</A>
    </DL><p>
    <DT><A HREF="https://new.example" ADD_DATE="1700000000">` + newTitle + `</A>
    <DT><A HREF="chrome://settings">Settings</A>
</DL><p>
`)
}

func TestImportNeedsLogin(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	defer ts.Close()

	for _, path := range []string{"/account/import", "/account/export"} {
		code, header, _ := ts.get(t, path)
		assert.Equal(t, code, http.StatusSeeOther)
		assert.StringContains(t, header.Get("Location"), "/user/login")
	}
}

func TestImport(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	defer ts.Close()
	ts.login(t)

	code, _, body := ts.get(t, "/account/import")
	assert.Equal(t, code, http.StatusOK)
	assert.StringContains(t, body, "Bookmark file")
	assert.StringContains(t, body, "Exporting from your browser")

	// Uploading shows the preview, and saves nothing yet
	code, header, _ := ts.upload(t, `C:\Users\alice\my bookmarks.html`, bookmarkFile("New page"))
	assert.Equal(t, code, http.StatusSeeOther)
	assert.Equal(t, header.Get("Location"), "/account/import")

	code, _, body = ts.get(t, "/account/import")
	assert.Equal(t, code, http.StatusOK)
	for _, want := range []string{
		"From <strong>my bookmarks.html</strong>",
		"1 new bookmark",
		"1 is already saved",
		"1 bookmark isn&#39;t in any folder, and will be tagged <strong>unfiled</strong>",
		"1 link is skipped",
		"Import 1 bookmark",
	} {
		assert.StringContains(t, body, want)
	}

	code, header, _ = ts.postForm(t, "/account/import/confirm", url.Values{})
	assert.Equal(t, code, http.StatusSeeOther)
	assert.Equal(t, header.Get("Location"), "/bookmarks")
	_, _, body = ts.get(t, "/bookmarks")
	assert.StringContains(t, body, "Imported 1 new bookmark.")

	// The upload is used up: confirming again imports nothing
	code, header, _ = ts.postForm(t, "/account/import/confirm", url.Values{})
	assert.Equal(t, code, http.StatusSeeOther)
	assert.Equal(t, header.Get("Location"), "/account/import")
	_, _, body = ts.get(t, "/account/import")
	assert.StringContains(t, body, "That upload has expired, or was already imported.")
	assert.StringContains(t, body, "Bookmark file")
}

func TestImportCancel(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	defer ts.Close()
	ts.login(t)

	ts.upload(t, "bookmarks.html", bookmarkFile("New page"))
	code, header, _ := ts.postForm(t, "/account/import/cancel", url.Values{})
	assert.Equal(t, code, http.StatusSeeOther)
	assert.Equal(t, header.Get("Location"), "/account/view")
	_, _, body := ts.get(t, "/account/import")
	assert.StringContains(t, body, "Bookmark file")
	assert.Equal(t, strings.Contains(body, "Import 1 bookmark"), false)
}

func TestImportOverLimit(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	defer ts.Close()
	ts.login(t)

	ts.upload(t, "bookmarks.html", bookmarkFile(mocks.OverBookmarkLimitTitle))
	_, _, body := ts.get(t, "/account/import")
	assert.StringContains(t, body, "past your limit of 1, so it can't be saved")
	assert.Equal(t, strings.Contains(body, "/account/import/confirm"), false)

	// Confirming anyway is refused, back at the preview
	code, header, _ := ts.postForm(t, "/account/import/confirm", url.Values{})
	assert.Equal(t, code, http.StatusSeeOther)
	assert.Equal(t, header.Get("Location"), "/account/import")
	_, _, body = ts.get(t, "/account/import")
	assert.StringContains(t, body, "Nothing was imported")
	assert.StringContains(t, body, "past your limit of 1")
}

func TestImportUploadErrors(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	defer ts.Close()
	ts.login(t)

	tests := []struct {
		name     string
		filename string
		content  []byte
		want     string
	}{
		{"No file chosen", "", nil, "Choose a bookmark file"},
		{"Not a bookmark file", "notes.txt", []byte("Just some notes"), "No bookmarks were found in this file"},
		{"No web addresses", "bookmarks.html", []byte(`<DL><DT><A HREF="chrome://settings">S</A></DL>`),
			"None of the links in this file are web addresses"},
		{"Just too large", "bookmarks.html", bytes.Repeat([]byte(" "), models.MaxImportBytes+1), "This file is too large"},
		{"Far too large", "bookmarks.html", bytes.Repeat([]byte(" "), models.MaxImportBytes+1<<20), "This file is too large"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, _, body := ts.upload(t, tt.filename, tt.content)
			assert.Equal(t, code, http.StatusUnprocessableEntity)
			assert.StringContains(t, body, tt.want)
		})
	}
	// Nothing was kept
	_, _, body := ts.get(t, "/account/import")
	assert.StringContains(t, body, "Bookmark file")
}

func TestImportCrossOrigin(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	defer ts.Close()
	ts.login(t)

	for _, path := range []string{"/account/import", "/account/import/confirm", "/account/import/cancel"} {
		code, _, _ := ts.postFormCrossOrigin(t, path, url.Values{})
		assert.Equal(t, code, http.StatusForbidden)
	}
}

func TestExport(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	defer ts.Close()
	ts.login(t)

	code, header, body := ts.get(t, "/account/export")
	assert.Equal(t, code, http.StatusOK)
	assert.StringContains(t, header.Get("Content-Disposition"), `attachment; filename="bookmarkbox-`)
	assert.StringContains(t, header.Get("Content-Type"), "text/html")
	assert.StringContains(t, body, "<!DOCTYPE NETSCAPE-Bookmark-file-1>")
	assert.StringContains(t, body, `<A HREF="https://go.dev/doc/effective_go" `)
	assert.StringContains(t, body, `TAGS="docs,go"`)
}

func TestAccountPage(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	defer ts.Close()
	ts.login(t)

	code, _, body := ts.get(t, "/account/view")
	assert.Equal(t, code, http.StatusOK)
	assert.StringContains(t, body, "<h2>Account details</h2>")
	assert.StringContains(t, body, "<h2>Your bookmarks</h2>")
	assert.StringContains(t, body, "href='/account/import'")
	assert.StringContains(t, body, "Export all (1 bookmark)")
}
