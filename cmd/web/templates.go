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
	Tag             *models.Tag
	Tags            []*models.Tag
	MergeInto       string // name of the tag a rename would merge into
	Form            any
	Flash           string
	FlashIsError    bool   // styles the flash as an error
	TagError        string // a failed add or remove on the view page's tag editor
	FocusTagInput   bool   // after adding a tag in place, keep typing in the field
	List            *listPage
	IsAuthenticated bool // Has an authenticated ID
	SignupOpen      bool // Set for logged-out visitors, to show the Signup link
	User            *models.User
	Viewer          *models.Viewer // the logged-in User's name and theme, or nil
	BookmarkCount   int            // the User's Bookmarks, on the Account page
	Import          *importPreview // an uploaded file waiting to be imported
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
	"humanDate":    humanDate,
	"shortDate":    func(t time.Time) string { return shortDate(t, time.Now()) },
	"colour":       colour,
	"initial":      initial,
	"listItem":     func(b *models.Bookmark, p *listPage) bookmarkItem { return bookmarkItem{b, p.Here, &p.State} },
	"detailsItem":  func(b *models.Bookmark) bookmarkItem { return bookmarkItem{b, fmt.Sprintf("/bookmarks/%d", b.ID), nil} },
	"plural":       plural,
	"hasPrefix":    strings.HasPrefix,
	"colorScheme":  colorScheme,
	"themeChoices": func() []themeChoice { return themeChoices },
	"static":       func(path string) string { return "/static/" + path + "?v=" + staticVersion },
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
// where its action forms return to when JavaScript is off. List is the list
// it's shown in, which its Tag and domain links narrow; nil on its own page.
type bookmarkItem struct {
	*models.Bookmark
	Next string
	List *listState
}

// TagURL is the list of Bookmarks with a Tag: this list narrowed by it, or,
// away from a list, the remembered List view filtered by it alone.
func (b bookmarkItem) TagURL(tag string) string {
	if b.List == nil {
		return "/bookmarks?tag=" + url.QueryEscape(tag)
	}
	return b.List.WithTag(tag)
}

// DomainURL is the list of Bookmarks from this one's domain.
func (b bookmarkItem) DomainURL() string {
	if b.List == nil {
		return "/bookmarks?domain=" + url.QueryEscape(b.Domain)
	}
	return b.List.WithDomain(b.Domain)
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

// themeChoice is one option of the theme switch on the Account page.
type themeChoice struct{ Value, Name string }

var themeChoices = []themeChoice{
	{models.ThemeSystem, "System"},
	{models.ThemeLight, "Light"},
	{models.ThemeDark, "Dark"},
}

// colorScheme is the color-scheme a page declares for the viewer's theme:
// both for logged-out visitors and the System theme, so the device decides.
func colorScheme(v *models.Viewer) string {
	if v == nil || v.Theme == models.ThemeSystem {
		return "light dark"
	}
	return v.Theme
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
