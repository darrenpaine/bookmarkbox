package main

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/darrenpaine/bookmarkbox/internal/models"
)

// The List views.
const (
	viewCompact = "compact"
	viewCards   = "cards"
	viewTags    = "tags" // By Tag: Compact rows beside a Tag panel
)

var listViews = []string{viewCompact, viewCards, viewTags}

// Session keys for the remembered List view and sort, and the address of the
// last list looked at.
const (
	sessionListView = "listView"
	sessionListSort = "listSort"
	sessionListURL  = "listURL"
)

// The most Tags one list can filter by. More would only make a slow query.
const maxFilterTags = 10

// The longest search kept, in characters.
const maxSearchChars = 200

// listState is everything a list address can say: the List view, the sort,
// the Filters and a "Show more" marker. Its methods build the addresses for
// the links on a list page, which always name the view and sort.
type listState struct {
	View string
	models.ListQuery
}

// readListState reads a list address. A missing or unknown view or sort
// falls back to the one remembered for the session; one given is remembered.
// A search is the exception: without a sort it shows the best matches, and
// that sort is never remembered, as it means nothing without a search.
func (app *application) readListState(r *http.Request) listState {
	return app.parseListState(r, r.URL.Query(), true)
}

// parseListState reads a list address's query. With remember, a view or sort
// it gives is kept for the session; without, the session is only read.
func (app *application) parseListState(r *http.Request, q url.Values, remember bool) listState {
	s := listState{View: q.Get("view")}
	s.Sort = q.Get("sort")
	s.Search = strings.Join(strings.Fields(q.Get("q")), " ")
	if runes := []rune(s.Search); len(runes) > maxSearchChars {
		s.Search = strings.TrimSpace(string(runes[:maxSearchChars]))
	}
	if s.Search != "" && (s.Sort == "" || s.Sort == models.SortMatch) {
		s.Sort = models.SortMatch
	}
	ctx := r.Context()

	if slices.Contains(listViews, s.View) {
		if remember {
			app.sessionManager.Put(ctx, sessionListView, s.View)
		}
	} else if s.View = app.sessionManager.GetString(ctx, sessionListView); !slices.Contains(listViews, s.View) {
		s.View = viewCompact
	}
	if s.Sort == models.SortMatch && s.Search != "" {
		// Searching: the best matches, as above.
	} else if slices.Contains(models.Sorts, s.Sort) {
		if remember {
			app.sessionManager.Put(ctx, sessionListSort, s.Sort)
		}
	} else if s.Sort = app.sessionManager.GetString(ctx, sessionListSort); !slices.Contains(models.Sorts, s.Sort) {
		s.Sort = models.SortNewest
	}

	for _, raw := range q["tag"] {
		if name, problem := tagName(raw); problem == "" && !slices.Contains(s.Tags, name) && len(s.Tags) < maxFilterTags {
			s.Tags = append(s.Tags, name)
		}
	}
	s.Favourite = q.Get("fav") == "1"
	s.ReadLater = q.Get("later") == "1"
	if domain := strings.ToLower(strings.TrimSpace(q.Get("domain"))); len(domain) <= 253 {
		s.Domain = domain
	}
	s.After = q.Get("after")
	s.UpTo = q.Get("upto")
	return s
}

// URL is the address of this list, without any marker.
func (s listState) URL() string {
	v := url.Values{}
	v.Set("view", s.View)
	if s.Sort != "" { // empty: the remembered sort
		v.Set("sort", s.Sort)
	}
	if s.Search != "" {
		v.Set("q", s.Search)
	}
	for _, tag := range s.Tags {
		v.Add("tag", tag)
	}
	if s.Favourite {
		v.Set("fav", "1")
	}
	if s.ReadLater {
		v.Set("later", "1")
	}
	if s.Domain != "" {
		v.Set("domain", s.Domain)
	}
	// Encode sorts the keys; list them in a fixed, readable order instead.
	parts := []string{}
	for _, key := range []string{"view", "sort", "q", "tag", "fav", "later", "domain"} {
		for _, value := range v[key] {
			parts = append(parts, url.QueryEscape(key)+"="+url.QueryEscape(value))
		}
	}
	return "/bookmarks?" + strings.Join(parts, "&")
}

// clean drops the markers, so each link starts its list from the top.
func (s listState) clean() listState {
	s.After, s.UpTo = "", ""
	s.Tags = slices.Clone(s.Tags)
	return s
}

func (s listState) WithView(view string) string {
	c := s.clean()
	c.View = view
	return c.URL()
}

func (s listState) WithSort(sort string) string {
	c := s.clean()
	c.Sort = sort
	return c.URL()
}

func (s listState) ToggleFavourite() string {
	c := s.clean()
	c.Favourite = !c.Favourite
	return c.URL()
}

func (s listState) ToggleReadLater() string {
	c := s.clean()
	c.ReadLater = !c.ReadLater
	return c.URL()
}

func (s listState) WithTag(tag string) string {
	c := s.clean()
	if !slices.Contains(c.Tags, tag) {
		c.Tags = append(c.Tags, tag)
	}
	return c.URL()
}

func (s listState) WithoutTag(tag string) string {
	c := s.clean()
	c.Tags = slices.DeleteFunc(c.Tags, func(t string) bool { return t == tag })
	return c.URL()
}

func (s listState) WithDomain(domain string) string {
	c := s.clean()
	c.Domain = domain
	return c.URL()
}

func (s listState) WithoutDomain() string {
	return s.WithDomain("")
}

// WithoutSearch drops the search. Its best-match sort goes too: with no sort
// in the address, the remembered one applies.
func (s listState) WithoutSearch() string {
	c := s.clean()
	c.Search = ""
	if c.Sort == models.SortMatch {
		c.Sort = ""
	}
	return c.URL()
}

// Cleared keeps the view and sort, and drops every Filter.
func (s listState) Cleared() string {
	c := s.clean()
	c.Filter = models.Filter{}
	if c.Sort == models.SortMatch {
		c.Sort = ""
	}
	return c.URL()
}

// AfterURL is the address of the batch after a marker, for "Show more".
func (s listState) AfterURL(marker string) string {
	return s.clean().URL() + "&after=" + url.QueryEscape(marker)
}

// UpToURL is the address of the list down to a marker, so a reload keeps
// every row "Show more" added.
func (s listState) UpToURL(marker string) string {
	return s.clean().URL() + "&upto=" + url.QueryEscape(marker)
}

// FilterCount is how many Filters are on.
func (s listState) FilterCount() int {
	n := len(s.Tags)
	for _, on := range []bool{s.Favourite, s.ReadLater, s.Domain != "", s.Search != ""} {
		if on {
			n++
		}
	}
	return n
}

// Describe names the Bookmarks a list shows, with how many there are, such
// as "12 bookmarks", "No favourites tagged go" or "3 bookmarks matching “sql”".
func (s listState) Describe(n int) string {
	noun := "bookmark"
	if s.Favourite {
		noun = "favourite"
	}
	if n != 1 {
		noun += "s"
	}
	parts := []string{noun}
	if s.Search != "" {
		parts = append(parts, "matching “"+s.Search+"”")
	}
	if s.ReadLater {
		parts = append(parts, "to read later")
	}
	if len(s.Tags) > 0 {
		parts = append(parts, "tagged "+joinAnd(s.Tags))
	}
	if s.Domain != "" {
		parts = append(parts, "from "+s.Domain)
	}
	count := fmt.Sprint(n)
	if n == 0 {
		count = "No"
	}
	return count + " " + strings.Join(parts, " ")
}

// joinAnd joins words as "a", "a and b", or "a, b and c".
func joinAnd(words []string) string {
	if len(words) < 2 {
		return strings.Join(words, "")
	}
	return strings.Join(words[:len(words)-1], ", ") + " and " + words[len(words)-1]
}

// listPage holds what a list page shows.
type listPage struct {
	State listState
	Here  string // this page's address, where row actions return without JavaScript
	*models.ListPage
	Tags  []*models.Tag // By Tag only: selected Tags first, then the rest A-Z
	Empty bool          // the User has no Bookmarks at all
}

// SortName is the label for a sort in the sort menu.
func (listPage) SortName(sort string) string {
	return map[string]string{
		models.SortNewest: "Newest",
		models.SortOldest: "Oldest",
		models.SortTitle:  "A–Z",
		models.SortDomain: "Domain",
		models.SortMatch:  "Best match",
	}[sort]
}

// ViewName is the label for a List view.
func (listPage) ViewName(view string) string {
	return map[string]string{viewCompact: "Compact", viewCards: "Cards", viewTags: "By Tag"}[view]
}

// Sorts and Views list the choices, in menu order. A search can also be
// sorted by best match.
func (p listPage) Sorts() []string {
	if p.State.Search != "" {
		return append([]string{models.SortMatch}, models.Sorts...)
	}
	return models.Sorts
}
func (listPage) Views() []string { return listViews }

// Capped reports whether a search shows only its best matches, out of more.
func (p listPage) Capped() bool {
	return p.State.Sort == models.SortMatch && p.Total > len(p.Bookmarks)
}

// IsSelected reports whether the list is filtered by a Tag.
func (p listPage) IsSelected(tag string) bool {
	return slices.Contains(p.State.Tags, tag)
}

// The list of the user's Bookmarks, in the chosen List view, sort and Filters.
// htmx asks for "Show more" batches, and gets just the rows back.
func (app *application) bookmarkList(w http.ResponseWriter, r *http.Request) {
	userID := app.authenticatedUserID(r)
	state := app.readListState(r)
	page, err := app.bookmarks.List(userID, state.ListQuery)
	if errors.Is(err, models.ErrBadMarker) {
		// A marker from an old or edited address: start from the top.
		http.Redirect(w, r, state.clean().URL(), http.StatusSeeOther)
		return
	}
	if err != nil {
		app.serverError(w, err)
		return
	}
	lp := &listPage{State: state, Here: r.URL.RequestURI(), ListPage: page}
	app.rememberList(r, state)

	if isHTMX(r) && state.After != "" {
		// Reloading the new address shows every row so far.
		if page.Last != "" {
			w.Header().Set("HX-Push-Url", state.UpToURL(page.Last))
		}
		app.renderFragment(w, http.StatusOK, "listmore", lp)
		return
	}

	if page.Total == 0 && state.FilterCount() > 0 {
		// Is the list empty because of the Filters, or because there's nothing?
		all, err := app.bookmarks.Count(userID, models.Filter{})
		if err != nil {
			app.serverError(w, err)
			return
		}
		lp.Empty = all == 0
	} else {
		lp.Empty = page.Total == 0
	}
	if state.View == viewTags {
		tags, err := app.bookmarks.TagCounts(userID, state.Filter)
		if err != nil {
			app.serverError(w, err)
			return
		}
		// Selected Tags first, in the order they were chosen.
		slices.SortStableFunc(tags, func(a, b *models.Tag) int {
			ia, ib := slices.Index(state.Tags, a.Name), slices.Index(state.Tags, b.Name)
			switch {
			case ia >= 0 && ib >= 0:
				return ia - ib
			case ia >= 0:
				return -1
			case ib >= 0:
				return 1
			}
			return 0
		})
		lp.Tags = tags
	}

	data := app.newTemplateData(r)
	data.List = lp
	app.render(w, http.StatusOK, "list.tmpl", data)
}

// currentList is the list the page making an htmx request shows, from the
// address htmx sends with it, or false if that page isn't a list.
func (app *application) currentList(r *http.Request) (listState, bool) {
	u, err := url.Parse(r.Header.Get("HX-Current-URL"))
	if err != nil || u.Path != "/bookmarks" {
		return listState{}, false
	}
	return app.parseListState(r, u.Query(), false), true
}

// listTitle is the heading of a list page, counted again, for htmx to put in
// place of the old one after a row has gone. Writes nothing for other pages.
func (app *application) listTitle(w http.ResponseWriter, r *http.Request) error {
	state, ok := app.currentList(r)
	if !ok {
		return nil
	}
	n, err := app.bookmarks.Count(app.authenticatedUserID(r), state.Filter)
	if err != nil {
		return err
	}
	return app.templateCache["home.tmpl"].ExecuteTemplate(w, "listtitle", state.Describe(n))
}

// rememberList keeps the address of the list the User is looking at, without
// its "Show more" marker, so Back on a Bookmark's page returns to it.
func (app *application) rememberList(r *http.Request, state listState) {
	if state.After != "" {
		return // "Show more" adds rows to the list already remembered
	}
	url := state.clean().URL()
	if app.sessionManager.GetString(r.Context(), sessionListURL) != url {
		app.sessionManager.Put(r.Context(), sessionListURL, url)
	}
}

// backURL is where Back on a Bookmark's page goes: the list last looked at,
// or all Bookmarks.
func (app *application) backURL(r *http.Request) string {
	return safeNext(app.sessionManager.GetString(r.Context(), sessionListURL), "/bookmarks")
}
