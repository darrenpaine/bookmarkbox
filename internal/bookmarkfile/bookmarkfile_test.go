package bookmarkfile

import (
	"bytes"
	"errors"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/darrenpaine/bookmarkbox/internal/assert"
)

// The fixtures are real exports from Brave and Firefox, trimmed, with every
// link, title and personal Folder name replaced. Their layout, attributes,
// dates and Root Folders are as the browsers wrote them.

func parseFile(t *testing.T, name string) []Link {
	t.Helper()
	f, err := os.Open("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	links, err := Parse(f, 0)
	if err != nil {
		t.Fatal(err)
	}
	return links
}

// find returns the first link with the URL.
func find(t *testing.T, links []Link, url string) Link {
	t.Helper()
	for _, l := range links {
		if l.URL == url {
			return l
		}
	}
	t.Fatalf("no link %s", url)
	return Link{}
}

func TestParseBrave(t *testing.T) {
	links := parseFile(t, "brave.html")
	assert.Equal(t, len(links), 35)

	tests := []struct {
		name    string
		url     string
		title   string
		folders []string
		added   int64
	}{
		// The bookmarks bar is marked as a Root Folder
		{"In the bookmarks bar", "https://www.example.org/page/1", "Page 1", nil, 1760687850},
		{"In a Folder", "https://example.net/page/2", "Page 2", []string{"Recipes"}, 1678699598},
		{"Nested Folders", "https://docs.example.com/page/8", "Page 8", []string{"Recipes", "Astronomy", "New folder"}, 1772014768},
		// Brave writes the contents of "Other bookmarks" loose at the top
		{"Folder from Other bookmarks", "https://www.example.org/page/26", "Page 26", []string{"Poetry"}, 1789579181},
		{"Loose at the top", "https://example.com/page/30", "Page 30", nil, 1760720705},
		{"Not a web address", "chrome://settings/24", "Page 24", nil, 1491017137},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := find(t, links, tt.url)
			assert.Equal(t, l.Title, tt.title)
			assert.Equal(t, slices.Equal(l.Folders, tt.folders), true)
			assert.Equal(t, l.Added.Unix(), tt.added)
			assert.Equal(t, l.HasTags, false)
			assert.Equal(t, l.Description, "")
		})
	}
}

func TestParseFirefox(t *testing.T) {
	links := parseFile(t, "firefox.html")
	assert.Equal(t, len(links), 37)

	tests := []struct {
		name    string
		url     string
		folders []string
	}{
		{"Firefox's own Folder", "https://www.example.org/page/1", []string{"Mozilla Firefox"}},
		// Not marked, but a Root Folder's name at the top level
		{"In a bookmarks bar from another browser", "https://docs.example.com/page/3", nil},
		{"In a Folder under it", "https://blog.example.org/page/4", []string{"Recipes"}},
		{"Loose in the menu", "https://example.com/page/30", nil},
		{"In the toolbar", "https://www.example.org/page/31", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := find(t, links, tt.url)
			assert.Equal(t, slices.Equal(l.Folders, tt.folders), true)
		})
	}
}

func TestParseDetails(t *testing.T) {
	file := `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<DL><p>
    <DT><H3>Work, Projects</H3>
    <DL><p>
        <DT><A HREF=" https://a.example/?x=1&amp;y=2 " ADD_DATE="1700000000000">Fish &amp; chips</A>
        <DD>Line one
line two &lt;b&gt;
        <DT><H3>Bookmarks bar</H3>
        <DL><p>
            <DT><A HREF="https://b.example" ADD_DATE="1700000000000000" TAGS="Go, web ,,">B</A>
        </DL><p>
        <DT><H3>Folder heading with no list</H3>
        <DT><A HREF="https://c.example" ADD_DATE="soon" TAGS="" FAVOURITE="true" READ_LATER="false" NOTES="a&#13;&#10;b">C</A>
    </DL><p>
    <DT><A HREF="https://d.example" ADD_DATE="-5">
    <DT><H3>Unclosed
    <DL><p>
        <DT><A HREF="https://e.example">E</A>
`
	links, err := Parse(strings.NewReader(file), 0)
	assert.NilError(t, err)
	want := []Link{
		{URL: "https://a.example/?x=1&y=2", Title: "Fish & chips", Description: "Line one\nline two <b>",
			Added: time.Unix(1700000000, 0).UTC(), Folders: []string{"Work, Projects"}},
		// Only Root Folders at the top level are known by name
		{URL: "https://b.example", Title: "B", Added: time.Unix(1700000000, 0).UTC(),
			Folders: []string{"Work, Projects", "Bookmarks bar"}, Tags: []string{"Go", "web"}, HasTags: true},
		{URL: "https://c.example", Title: "C", Folders: []string{"Work, Projects"}, HasTags: true,
			Favourite: true, Notes: "a\r\nb"},
		{URL: "https://d.example"},
		{URL: "https://e.example", Title: "E", Folders: []string{"Unclosed"}},
	}
	if !reflect.DeepEqual(links, want) {
		t.Errorf("got:\n%#v\nwant:\n%#v", links, want)
	}
}

func TestParseTooManyLinks(t *testing.T) {
	file := `<DL><p><DT><A HREF="https://a.example">A</A><DT><A HREF="https://b.example">B</A></DL>`
	_, err := Parse(strings.NewReader(file), 1)
	assert.Equal(t, errors.Is(err, ErrTooManyLinks), true)

	links, err := Parse(strings.NewReader(file), 2)
	assert.NilError(t, err)
	assert.Equal(t, len(links), 2)
}

// Everything Write writes, Parse reads back exactly.
func TestWriteThenParse(t *testing.T) {
	links := []Link{
		{
			URL:         `https://example.com/?a=1&b="2"&c='3'<>`,
			Title:       "  Spaced & <b>\"quoted\"</b> 'title'\t",
			Description: "Line one\r\nline two\n\n",
			Notes:       "Notes\r\n\twith tabs & \"quotes\"\n",
			Added:       time.Unix(1715000000, 0).UTC(),
			Tags:        []string{"go", "work things", "café"},
			HasTags:     true,
			Favourite:   true,
			ReadLater:   true,
		},
		{URL: "https://example.org", Title: "Café ☕", Description: " ", Added: time.Unix(1, 0).UTC(), HasTags: true},
		{URL: "https://example.net", Title: "No date", HasTags: true},
	}
	var buf bytes.Buffer
	assert.NilError(t, Write(&buf, links))
	got, err := Parse(&buf, 0)
	assert.NilError(t, err)
	if !reflect.DeepEqual(got, links) {
		t.Errorf("got:\n%#v\nwant:\n%#v", got, links)
	}
}

func TestWriteLayout(t *testing.T) {
	var buf bytes.Buffer
	links := []Link{{URL: "https://example.com", Title: "Example", Added: time.Unix(1715000000, 0)}}
	assert.NilError(t, Write(&buf, links))
	out := buf.String()
	assert.Equal(t, strings.HasPrefix(out, "<!DOCTYPE NETSCAPE-Bookmark-file-1>\n"), true)
	// An empty TAGS is written, so the link never comes back as unfiled
	assert.StringContains(t, out, `<DT><A HREF="https://example.com" ADD_DATE="1715000000" TAGS="">Example</A>`)
	assert.Equal(t, strings.Contains(out, "<DD>"), false)
}
