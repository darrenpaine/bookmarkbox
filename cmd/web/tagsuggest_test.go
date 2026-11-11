package main

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/darrenpaine/bookmarkbox/internal/assert"
)

// The add, edit and Bookmark pages list the User's Tags for autocomplete, most
// used first, and mark the fields that use them.
func TestTagSuggestions(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	defer ts.Close()
	ts.login(t)

	// The mock User has "go" on 3 Bookmarks and "docs" on 1.
	want := "<datalist id='tag-suggestions'>\n" +
		"    <option value='go' data-colour='" + colour("go") + "' data-count='3'></option>\n" +
		"    <option value='docs' data-colour='" + colour("docs") + "' data-count='1'></option>\n" +
		"    \n</datalist>"
	list := "<input id='tags' type='text' name='tags' value='"
	listAttrs := "autocomplete='off' data-tag-suggest='list'"
	one := "maxlength='50' data-tag-suggest='one'"

	pages := []struct {
		name, method, path, field string
		code                      int
	}{
		{"Add", http.MethodGet, "/bookmarks/new", listAttrs, http.StatusOK},
		{"Add with an error", http.MethodPost, "/bookmarks/new", listAttrs, http.StatusUnprocessableEntity},
		{"Edit", http.MethodGet, "/bookmarks/1/edit", listAttrs, http.StatusOK},
		{"Edit with an error", http.MethodPost, "/bookmarks/1/edit", listAttrs, http.StatusUnprocessableEntity},
		{"Bookmark", http.MethodGet, "/bookmarks/1", one, http.StatusOK},
	}
	for _, tt := range pages {
		t.Run(tt.name, func(t *testing.T) {
			var code int
			var body string
			if tt.method == http.MethodGet {
				code, _, body = ts.get(t, tt.path)
			} else {
				code, _, body = ts.postForm(t, tt.path, url.Values{"url": {""}, "title": {""}})
			}
			assert.Equal(t, code, tt.code)
			assert.StringContains(t, body, want)
			assert.StringContains(t, body, "<script src='/static/js/tags.js?v="+staticVersion+"' defer></script>")
			assert.StringContains(t, body, tt.field)
			if tt.field == listAttrs {
				assert.StringContains(t, body, list)
			}
		})
	}

	t.Run("Not on other pages", func(t *testing.T) {
		_, _, body := ts.get(t, "/bookmarks")
		if strings.Contains(body, "tag-suggestions") || strings.Contains(body, "tags.js") {
			t.Error("the list page has Tag suggestions")
		}
	})

	t.Run("The Tag editor swapped by htmx keeps the field, not a second list", func(t *testing.T) {
		_, _, body := ts.postHTMX(t, "/bookmarks/1/tags", url.Values{"name": {"web"}}, nil)
		assert.StringContains(t, body, one)
		if strings.Contains(body, "tag-suggestions") {
			t.Error("the Tag editor fragment repeats the datalist")
		}
	})
}
