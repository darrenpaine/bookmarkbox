// Package bookmarkfile reads and writes the Netscape bookmark file
// (bookmarks.html) that Chrome, Edge, Firefox and Brave all export and import.
// It knows only the file format: what the links become in BookmarkBox is
// decided by the models package.
package bookmarkfile

import (
	"errors"
	"fmt"
	"html"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode"

	nethtml "golang.org/x/net/html"
)

// Link is one link from a bookmark file.
type Link struct {
	URL         string
	Title       string
	Description string    // from the <DD> after the link
	Notes       string    // BookmarkBox's own NOTES attribute
	Added       time.Time // from ADD_DATE; zero when the file has no usable date
	Folders     []string  // the Folders it was in, outermost first, without Root Folders
	Tags        []string  // from a TAGS attribute, as written
	HasTags     bool      // the link had a TAGS attribute, even an empty one
	Favourite   bool      // BookmarkBox's own FAVOURITE attribute
	ReadLater   bool      // BookmarkBox's own READ_LATER attribute
}

// ErrTooManyLinks is returned by Parse when a file has more links than allowed.
var ErrTooManyLinks = errors.New("bookmarkfile: too many links")

// rootNames are the browsers' own top-level Folders, in English, as the
// exports name them. Root Folders never become Tags. Chrome-family exports
// mark only the bookmarks bar, and Firefox marks its toolbar and "Other
// Bookmarks", so the rest are recognised by name at the top level.
var rootNames = map[string]bool{
	"bookmarks bar":     true, // Chrome, Brave
	"bookmarks toolbar": true, // Firefox
	"bookmarks menu":    true, // Firefox
	"other bookmarks":   true, // Chrome, Brave, Firefox
	"mobile bookmarks":  true, // Chrome, Brave, Firefox
	"favorites bar":     true, // Edge
	"favourites bar":    true, // Edge, UK English
	"other favorites":   true, // Edge
	"other favourites":  true, // Edge, UK English
	"mobile favorites":  true, // Edge
	"mobile favourites": true, // Edge, UK English
}

// folder is one open <DL> list: the Folder it belongs to, if any.
type folder struct {
	name string
	root bool // a Root Folder, or the file's outermost list
}

// Parse reads every link in a bookmark file, with the Folders it was in.
// Parsing is lenient, as browsers' are: unknown tags and attributes are
// ignored, and lists left open at the end are closed. maxLinks, if above 0,
// is the most links allowed; a file with more returns ErrTooManyLinks.
func Parse(r io.Reader, maxLinks int) ([]Link, error) {
	z := nethtml.NewTokenizer(r)
	var (
		links   []Link
		stack   []folder // the open lists, outermost first
		pending *folder  // a Folder heading waiting for its list
		text    strings.Builder
		inText  string // the element whose text is being collected: "a", "h3" or "dd"
		h3Root  bool
		ddFor   = -1 // the link a <DD> describes
	)
	// finishText ends the text being collected, and stores it.
	finishText := func() {
		value := textValue(text.String())
		switch inText {
		case "a":
			links[len(links)-1].Title = value
		case "h3":
			name := value
			root := h3Root || (len(stack) == 1 && rootNames[strings.ToLower(name)])
			pending = &folder{name: name, root: root}
		case "dd":
			if ddFor >= 0 {
				links[ddFor].Description = value
			}
		}
		inText = ""
		text.Reset()
	}

	for {
		tt := z.Next()
		switch tt {
		case nethtml.ErrorToken:
			if inText != "" {
				finishText()
			}
			if err := z.Err(); !errors.Is(err, io.EOF) {
				return nil, err
			}
			return links, nil

		case nethtml.TextToken:
			if inText != "" {
				text.Write(z.Raw())
			}

		case nethtml.StartTagToken, nethtml.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			tag := string(name)
			// A <DD>'s text runs until the next item starts, and so does any
			// text left unclosed.
			switch tag {
			case "dt", "dd", "dl", "h3", "a", "hr":
				if inText != "" {
					finishText()
				}
			}
			switch tag {
			case "dl":
				if pending != nil {
					stack = append(stack, *pending)
					pending = nil
				} else {
					stack = append(stack, folder{root: true})
				}
			case "h3":
				h3Root = false
				for hasAttr {
					var key []byte
					key, _, hasAttr = z.TagAttr()
					k := string(key)
					if k == "personal_toolbar_folder" || k == "unfiled_bookmarks_folder" {
						h3Root = true
					}
				}
				inText = "h3"
			case "a":
				pending = nil // a Folder heading with no list of its own
				if maxLinks > 0 && len(links) == maxLinks {
					return nil, ErrTooManyLinks
				}
				link := Link{Folders: folderPath(stack)}
				for hasAttr {
					var key, val []byte
					key, val, hasAttr = z.TagAttr()
					readAttr(&link, string(key), string(val))
				}
				links = append(links, link)
				inText = "a"
			case "dd":
				ddFor = -1
				if len(links) > 0 && pending == nil {
					ddFor = len(links) - 1
				}
				inText = "dd"
			}

		case nethtml.EndTagToken:
			name, _ := z.TagName()
			tag := string(name)
			if inText != "" && (tag == inText || tag == "dl") {
				finishText()
			}
			if tag == "dl" {
				if len(stack) > 0 {
					stack = stack[:len(stack)-1]
				}
				pending = nil
			}
		}
	}
}

// readAttr sets the field of a link that an attribute holds. Keys arrive in
// lower case, and values with entities already decoded.
func readAttr(link *Link, key, val string) {
	switch key {
	case "href":
		link.URL = strings.TrimSpace(val)
	case "add_date":
		link.Added = parseDate(val)
	case "tags":
		link.HasTags = true
		for t := range strings.SplitSeq(val, ",") {
			if t = strings.TrimSpace(t); t != "" {
				link.Tags = append(link.Tags, t)
			}
		}
	case "notes":
		link.Notes = val
	case "favourite":
		link.Favourite = isTrue(val)
	case "read_later":
		link.ReadLater = isTrue(val)
	}
}

// isTrue reads a yes/no attribute: present means yes, unless it says otherwise.
func isTrue(val string) bool {
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "false", "0", "no":
		return false
	}
	return true
}

// folderPath lists the Folder names of the open lists, without Root Folders.
func folderPath(stack []folder) []string {
	var path []string
	for _, f := range stack {
		if !f.root && f.name != "" {
			path = append(path, f.name)
		}
	}
	return path
}

// parseDate reads an ADD_DATE, which is in seconds since 1970. A few tools
// write milliseconds or microseconds instead, so a number too large to be
// seconds is scaled down. Anything unusable gives the zero time.
func parseDate(val string) time.Time {
	n, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64)
	if err != nil || n <= 0 {
		return time.Time{}
	}
	for n > 100_000_000_000 { // later than the year 5000 in seconds
		n /= 1000
	}
	return time.Unix(n, 0).UTC()
}

// textValue turns the raw text of an element into its value. Whitespace
// written literally at either end is the file's layout, not part of the
// value, so it is trimmed before entities are decoded. Write encodes any
// whitespace that belongs to the value, so it survives this.
func textValue(raw string) string {
	return html.UnescapeString(strings.TrimSpace(raw))
}

// Write writes links as a bookmark file, in the layout browsers export: a
// flat list with no Folders. Every link gets a TAGS attribute, even an empty
// one, so that importing the file again never treats it as unfiled.
func Write(w io.Writer, links []Link) error {
	ew := &errWriter{w: w}
	ew.write(`<!DOCTYPE NETSCAPE-Bookmark-file-1>
<!-- This is an automatically generated file.
     It will be read and overwritten.
     DO NOT EDIT! -->
<META HTTP-EQUIV="Content-Type" CONTENT="text/html; charset=UTF-8">
<TITLE>Bookmarks</TITLE>
<H1>Bookmarks</H1>
<DL><p>
`)
	for _, l := range links {
		ew.write(`    <DT><A HREF="` + escapeAttr(l.URL) + `"`)
		if !l.Added.IsZero() {
			ew.write(` ADD_DATE="` + strconv.FormatInt(l.Added.Unix(), 10) + `"`)
		}
		ew.write(` TAGS="` + escapeAttr(strings.Join(l.Tags, ",")) + `"`)
		if l.Favourite {
			ew.write(` FAVOURITE="true"`)
		}
		if l.ReadLater {
			ew.write(` READ_LATER="true"`)
		}
		if l.Notes != "" {
			ew.write(` NOTES="` + escapeAttr(l.Notes) + `"`)
		}
		ew.write(`>` + escapeText(l.Title) + "</A>\n")
		if l.Description != "" {
			ew.write("    <DD>" + escapeText(l.Description) + "\n")
		}
	}
	ew.write("</DL><p>\n")
	return ew.err
}

// errWriter keeps the first write error, so Write can check once at the end.
type errWriter struct {
	w   io.Writer
	err error
}

func (ew *errWriter) write(s string) {
	if ew.err == nil {
		_, ew.err = io.WriteString(ew.w, s)
	}
}

// escapeAttr escapes a value for a double-quoted attribute. Line breaks are
// written as character references, so each link stays on one line.
func escapeAttr(s string) string {
	s = html.EscapeString(s)
	s = strings.ReplaceAll(s, "\r", "&#13;")
	return strings.ReplaceAll(s, "\n", "&#10;")
}

// escapeText escapes element text. Besides line breaks, whitespace at either
// end is written as character references, because Parse trims the literal
// whitespace there.
func escapeText(s string) string {
	inner := strings.TrimFunc(s, unicode.IsSpace)
	if inner == "" {
		return refs(s)
	}
	start := strings.Index(s, inner)
	return refs(s[:start]) + escapeAttr(inner) + refs(s[start+len(inner):])
}

// refs writes every character as a numeric character reference.
func refs(s string) string {
	var b strings.Builder
	for _, r := range s {
		fmt.Fprintf(&b, "&#%d;", r)
	}
	return b.String()
}
