package main

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/darrenpaine/bookmarkbox/internal/assert"
	"github.com/darrenpaine/bookmarkbox/internal/models"
	"github.com/darrenpaine/bookmarkbox/internal/models/mocks"
)

func TestListStateURLs(t *testing.T) {
	s := listState{View: viewCards, ListQuery: models.ListQuery{
		Sort:   models.SortTitle,
		Filter: models.Filter{Tags: []string{"go", "c++"}},
		After:  "marker",
	}}
	base := "/bookmarks?view=cards&sort=title&tag=go&tag=c%2B%2B"
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"URL", s.URL(), base},
		{"View", s.WithView(viewTags), "/bookmarks?view=tags&sort=title&tag=go&tag=c%2B%2B"},
		{"Sort", s.WithSort(models.SortOldest), "/bookmarks?view=cards&sort=oldest&tag=go&tag=c%2B%2B"},
		{"Favourite", s.ToggleFavourite(), base + "&fav=1"},
		{"Read later", s.ToggleReadLater(), base + "&later=1"},
		{"Add Tag", s.WithTag("web"), base + "&tag=web"},
		{"Add Tag already there", s.WithTag("go"), base},
		{"Remove Tag", s.WithoutTag("go"), "/bookmarks?view=cards&sort=title&tag=c%2B%2B"},
		{"Domain", s.WithDomain("go.dev"), base + "&domain=go.dev"},
		{"Cleared", s.Cleared(), "/bookmarks?view=cards&sort=title"},
		{"Show more", s.AfterURL("a b"), base + "&after=a+b"},
		{"Reload", s.UpToURL("m"), base + "&upto=m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.got, tt.want)
		})
	}
	// Building links never changes the list they came from.
	assert.Equal(t, len(s.Tags), 2)
	assert.Equal(t, s.After, "marker")

	// A search by best match: dropping the search, or every Filter, drops
	// that sort too, so the remembered one applies.
	s = listState{View: viewCompact, ListQuery: models.ListQuery{
		Sort:   models.SortMatch,
		Filter: models.Filter{Search: "go & sql", Favourite: true},
	}}
	assert.Equal(t, s.URL(), "/bookmarks?view=compact&sort=match&q=go+%26+sql&fav=1")
	assert.Equal(t, s.WithSort(models.SortTitle), "/bookmarks?view=compact&sort=title&q=go+%26+sql&fav=1")
	assert.Equal(t, s.WithoutSearch(), "/bookmarks?view=compact&fav=1")
	assert.Equal(t, s.Cleared(), "/bookmarks?view=compact")
	s.Sort = models.SortOldest
	assert.Equal(t, s.WithoutSearch(), "/bookmarks?view=compact&sort=oldest&fav=1")
}

func TestListStateDescribe(t *testing.T) {
	tests := []struct {
		filter models.Filter
		n      int
		want   string
	}{
		{models.Filter{}, 12, "12 bookmarks"},
		{models.Filter{}, 1, "1 bookmark"},
		{models.Filter{Favourite: true}, 0, "No favourites"},
		{models.Filter{Favourite: true, Tags: []string{"go"}}, 0, "No favourites tagged go"},
		{models.Filter{ReadLater: true, Tags: []string{"a", "b", "c"}, Domain: "go.dev"}, 3,
			"3 bookmarks to read later tagged a, b and c from go.dev"},
		{models.Filter{Search: "sql"}, 0, "No bookmarks matching “sql”"},
		{models.Filter{Search: "sql", Favourite: true, Tags: []string{"db"}}, 2,
			"2 favourites matching “sql” tagged db"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			s := listState{ListQuery: models.ListQuery{Filter: tt.filter}}
			assert.Equal(t, s.Describe(tt.n), tt.want)
		})
	}
}

// getHTMX makes a GET request as htmx would.
func (ts *testServer) getHTMX(t *testing.T, urlPath string) (int, http.Header, string) {
	req, err := http.NewRequest(http.MethodGet, ts.URL+urlPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("HX-Request", "true")
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

func TestBookmarkList(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	defer ts.Close()

	t.Run("Needs a login", func(t *testing.T) {
		code, header, _ := ts.get(t, "/bookmarks")
		assert.Equal(t, code, http.StatusSeeOther)
		assert.Equal(t, header.Get("Location"), "/user/login")
	})

	ts.login(t)

	t.Run("Home goes to the list", func(t *testing.T) {
		code, header, _ := ts.get(t, "/")
		assert.Equal(t, code, http.StatusSeeOther)
		assert.Equal(t, header.Get("Location"), "/bookmarks")
	})

	t.Run("Compact rows", func(t *testing.T) {
		code, _, body := ts.get(t, "/bookmarks")
		assert.Equal(t, code, http.StatusOK)
		assert.StringContains(t, body, "<h1>1 bookmark</h1>")
		// The title opens the website, and Details opens the Bookmark's own page.
		assert.StringContains(t, body, "<a class='title' href='https://go.dev/doc/effective_go' rel='noopener noreferrer'>Effective Go</a>")
		assert.StringContains(t, body, "<a href='/bookmarks/1'>Details</a>")
		// The domain and Tags narrow the list.
		assert.StringContains(t, body, "href='/bookmarks?view=compact&amp;sort=newest&amp;domain=go.dev'")
		assert.StringContains(t, body, "href='/bookmarks?view=compact&amp;sort=newest&amp;tag=go'")
		// Row actions come back to this list without JavaScript.
		assert.StringContains(t, body, "<input type='hidden' name='next' value='/bookmarks'>")
	})

	t.Run("Filtered to nothing", func(t *testing.T) {
		code, _, body := ts.get(t, "/bookmarks?fav=1&tag=go")
		assert.Equal(t, code, http.StatusOK)
		assert.StringContains(t, body, "<h1>No favourites tagged go</h1>")
		assert.StringContains(t, body, "href='/bookmarks?view=compact&amp;sort=newest'>Clear filters</a>")
		if strings.Contains(body, "No bookmarks yet") {
			t.Error("shows the empty state for a User with Bookmarks")
		}
	})

	t.Run("Filters that match", func(t *testing.T) {
		_, _, body := ts.get(t, "/bookmarks?tag=GO&tag=docs&domain=Go.Dev")
		assert.StringContains(t, body, "<h1>1 bookmark tagged go and docs from go.dev</h1>")
		assert.StringContains(t, body, "aria-label='Remove filter go'")
		assert.StringContains(t, body, "aria-label='Remove filter go.dev'")
	})

	t.Run("Cards", func(t *testing.T) {
		_, _, body := ts.get(t, "/bookmarks?view=cards")
		assert.StringContains(t, body, "<li class='card' id='bookmark-1'>")
		assert.StringContains(t, body, "<p class='card-text'>Tips for writing clear, idiomatic Go code.</p>")
	})

	t.Run("By Tag", func(t *testing.T) {
		_, _, body := ts.get(t, "/bookmarks?view=tags&tag=go")
		assert.StringContains(t, body, "<section class='tag-panel' aria-label='Tags'>")
		assert.StringContains(t, body, "<li class='selected'>")
		assert.StringContains(t, body, "href='/bookmarks?view=tags&amp;sort=newest&amp;tag=go&amp;tag=docs'>docs</a>")
	})

	t.Run("Show more with htmx", func(t *testing.T) {
		code, header, body := ts.getHTMX(t, "/bookmarks?view=compact&sort=newest&after=start")
		assert.Equal(t, code, http.StatusOK)
		assert.Equal(t, header.Get("HX-Push-Url"), "/bookmarks?view=compact&sort=newest&upto="+mocks.MockMarker)
		assert.StringContains(t, body, "<li class='row' id='bookmark-1'>")
		if strings.Contains(body, "<html") {
			t.Error("sent a whole page to htmx")
		}
	})

	t.Run("Show more without htmx is a whole page", func(t *testing.T) {
		code, _, body := ts.get(t, "/bookmarks?after=start")
		assert.Equal(t, code, http.StatusOK)
		assert.StringContains(t, body, "<html")
	})

	t.Run("Search", func(t *testing.T) {
		code, _, body := ts.get(t, "/bookmarks?q=++effective++go")
		assert.Equal(t, code, http.StatusOK)
		assert.StringContains(t, body, "<h1>1 bookmark matching “effective go”</h1>")
		assert.StringContains(t, body, "<summary>Sort: Best match</summary>")
		assert.StringContains(t, body, "aria-label='Remove search effective go'")
		assert.StringContains(t, body, "href='/bookmarks?view=compact&amp;sort=title&amp;q=effective&#43;go'>A–Z</a>")
		// The box shows the search, and updates the results as you type.
		assert.StringContains(t, body, "value='effective go'")
		assert.StringContains(t, body, "hx-get='/bookmarks' hx-trigger='input changed delay:500ms, search'")
	})

	t.Run("Search keeps the Filters", func(t *testing.T) {
		_, _, body := ts.get(t, "/bookmarks?q=effective&tag=go&domain=go.dev")
		assert.StringContains(t, body, "<h1>1 bookmark matching “effective” tagged go from go.dev</h1>")
		assert.StringContains(t, body, "<input type='hidden' name='tag' value='go'>")
		assert.StringContains(t, body, "<input type='hidden' name='domain' value='go.dev'>")
	})

	t.Run("Search with no matches", func(t *testing.T) {
		_, _, body := ts.get(t, "/bookmarks?q=rust")
		assert.StringContains(t, body, "<h1>No bookmarks matching “rust”</h1>")
		assert.StringContains(t, body, "Clear filters</a>")
	})

	t.Run("An empty search is the plain list", func(t *testing.T) {
		_, _, body := ts.get(t, "/bookmarks?q=+++")
		assert.StringContains(t, body, "<h1>1 bookmark</h1>")
		assert.StringContains(t, body, "<summary>Sort: Newest</summary>")
	})

	t.Run("Live search gets a whole page", func(t *testing.T) {
		// htmx picks the <main> out of it.
		code, _, body := ts.getHTMX(t, "/bookmarks?view=compact&q=effective")
		assert.Equal(t, code, http.StatusOK)
		assert.StringContains(t, body, "<main>")
	})

	t.Run("Search elsewhere opens the list", func(t *testing.T) {
		_, _, body := ts.get(t, "/tags")
		assert.StringContains(t, body, "<form class='search' action='/bookmarks' method='GET' role='search'>")
		if strings.Contains(body, "hx-get='/bookmarks'") {
			t.Error("searching from another page updates that page")
		}
	})

	t.Run("Bad marker starts from the top", func(t *testing.T) {
		code, header, _ := ts.get(t, "/bookmarks?view=cards&tag=go&upto="+mocks.BadMarker)
		assert.Equal(t, code, http.StatusSeeOther)
		assert.Equal(t, header.Get("Location"), "/bookmarks?view=cards&sort=newest&tag=go")
	})
}

func TestBookmarkListRemembers(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	defer ts.Close()
	ts.login(t)

	// Nothing chosen yet: Compact, newest first.
	_, _, body := ts.get(t, "/bookmarks")
	assert.StringContains(t, body, "<a href='/bookmarks?view=compact&amp;sort=newest' aria-current='true'>Compact</a>")

	// Choosing a view and sort remembers them, but not the Filters.
	ts.get(t, "/bookmarks?view=cards&sort=title&fav=1")
	_, _, body = ts.get(t, "/bookmarks")
	assert.StringContains(t, body, "<a href='/bookmarks?view=cards&amp;sort=title' aria-current='true'>Cards</a>")
	assert.StringContains(t, body, "<summary>Sort: A–Z</summary>")
	assert.StringContains(t, body, "<h1>1 bookmark</h1>")

	// A search shows the best matches, without forgetting the remembered sort.
	_, _, body = ts.get(t, "/bookmarks?q=go")
	assert.StringContains(t, body, "<summary>Sort: Best match</summary>")
	_, _, body = ts.get(t, "/bookmarks?sort=match")
	assert.StringContains(t, body, "<summary>Sort: A–Z</summary>")

	// Unknown values are ignored.
	_, _, body = ts.get(t, "/bookmarks?view=nope&sort=nope")
	assert.StringContains(t, body, "<summary>Sort: A–Z</summary>")

	// Tag links from a Bookmark's own page use the remembered view.
	_, _, body = ts.get(t, "/bookmarks/1")
	assert.StringContains(t, body, "<a href='/bookmarks?tag=go' title='Bookmarks tagged go'>go</a>")
}
