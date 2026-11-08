package main

import (
	"fmt"
	"hash/fnv"
	"html/template"
	"io/fs"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/darrenpaine/bookmarkbox/internal/models"
	"github.com/darrenpaine/bookmarkbox/ui"
)

// templateData holds the dynamic data passed to the HTML templates.
type templateData struct {
	CurrentYear     int
	CurrentPath     string // marks the current nav link
	Bookmark        *models.Bookmark
	Bookmarks       []*models.Bookmark
	Tag             *models.Tag
	Tags            []*models.Tag
	MergeInto       string // name of the tag a rename would merge into
	Form            any
	Flash           string
	FlashIsError    bool   // styles the flash as an error
	TagError        string // a failed add or remove on the view page's tag editor
	FocusTagInput   bool   // after adding a tag in place, keep typing in the field
	IsAuthenticated bool   // Has an authenticated ID
	SignupOpen      bool   // Set for logged-out visitors, to show the Signup link
	User            *models.User
}

// humanDate() gives a nice time/date format - UTC, or blank for unset time.
func humanDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("02 Jan 2006 at 15:04")
}

// Initialize a template.FuncMap object and store it in a global variable. This is
// essentially a string-keyed map which acts as a lookup between the names of our
// custom template functions and the functions themselves.
var functions = template.FuncMap{
	"humanDate":   humanDate,
	"shortDate":   func(t time.Time) string { return shortDate(t, time.Now()) },
	"domain":      domain,
	"colour":      colour,
	"initial":     initial,
	"listItem":    func(b *models.Bookmark) bookmarkItem { return bookmarkItem{b, "/"} },
	"detailsItem": func(b *models.Bookmark) bookmarkItem { return bookmarkItem{b, fmt.Sprintf("/bookmarks/%d", b.ID)} },
	"static":      func(path string) string { return "/static/" + path + "?v=" + staticVersion },
}

// staticVersion is a hash of every file under ui/static. Page links to static
// files carry it, so browsers can cache them for a year and still fetch a new
// copy as soon as any file changes.
var staticVersion = hashStatic()

func hashStatic() string {
	h := fnv.New64a()
	err := fs.WalkDir(ui.Files, "static", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(ui.Files, path)
		if err != nil {
			return err
		}
		h.Write([]byte(path))
		h.Write(data)
		return nil
	})
	if err != nil {
		panic(err) // the files are embedded, so this can't fail at run time
	}
	return fmt.Sprintf("%016x", h.Sum64())
}

// bookmarkItem is a Bookmark as shown in a list row or on its own page. Next is
// where its action forms return to when JavaScript is off.
type bookmarkItem struct {
	*models.Bookmark
	Next string
}

// shortDate gives "6 Nov", adding the year when it isn't the current one.
func shortDate(t, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	t = t.UTC()
	if t.Year() == now.UTC().Year() {
		return t.Format("2 Jan")
	}
	return t.Format("2 Jan 2006")
}

// colour picks one of 8 colour classes from a name, so a Tag or domain always
// gets the same colour without storing one.
func colour(name string) string {
	h := fnv.New32a()
	h.Write([]byte(name))
	return fmt.Sprintf("c%d", h.Sum32()%8)
}

// initial is the upper-case first letter of a name, for the letter icons.
func initial(name string) string {
	for _, r := range name {
		return strings.ToUpper(string(r))
	}
	return "?"
}

// domain() returns the host part of a URL without "www.", e.g. "go.dev".
func domain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(u.Hostname(), "www.")
}

func newTemplateCache() (map[string]*template.Template, error) {
	cache := map[string]*template.Template{}
	pages, err := fs.Glob(ui.Files, "html/pages/*.tmpl")
	if err != nil {
		return nil, err
	}
	for _, page := range pages {
		name := filepath.Base(page)
		patterns := []string{
			"html/base.tmpl",
			"html/partials/*.tmpl",
			page,
		}
		ts, err := template.New(name).Funcs(functions).ParseFS(ui.Files, patterns...)
		if err != nil {
			return nil, err
		}
		cache[name] = ts
	}
	return cache, nil
}
