package models

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"slices"
	"strings"
)

// The sorts a list can use. Every sort ends with the Bookmark's ID, so rows
// with equal values always come in the same order.
const (
	SortNewest = "newest"
	SortOldest = "oldest"
	SortTitle  = "title"  // A-Z by title, ignoring case
	SortDomain = "domain" // A-Z by domain, then by title
)

// Sorts lists the sorts in the order a menu shows them.
var Sorts = []string{SortNewest, SortOldest, SortTitle, SortDomain}

// Rows per "Show more" batch, and the most a page shows when it is loaded
// down to a marker.
const (
	ListBatch = 50
	ListMax   = 1000
)

// ErrBadMarker means a list marker could not be read, or belongs to another sort.
var ErrBadMarker = errors.New("models: bad list marker")

// Filter narrows which Bookmarks a list shows. Every condition set must hold,
// so several Tags means Bookmarks with all of them.
type Filter struct {
	Tags      []string // normalised Tag names
	Favourite bool
	ReadLater bool
	Domain    string
}

// IsEmpty reports whether the filter shows every Bookmark.
func (f Filter) IsEmpty() bool {
	return len(f.Tags) == 0 && !f.Favourite && !f.ReadLater && f.Domain == ""
}

// ListQuery asks for one batch of a list. With neither marker set it starts
// at the top. After gives the batch following a row; UpTo gives every row from
// the top down to and including one, so a reloaded page keeps its place.
type ListQuery struct {
	Filter
	Sort  string
	After string
	UpTo  string
}

// ListPage is one batch of a list.
type ListPage struct {
	Bookmarks []*Bookmark
	Last      string // marker for the last row, or "" when there are no rows
	More      bool   // more rows follow the last one
	Total     int    // Bookmarks matching the filter, in all batches
}

// sortOrder describes one sort: the values that order it, before the ID, and
// whether it runs downwards. Every value runs the same way, so one row-value
// comparison finds the rows after a marker.
type sortOrder struct {
	keys []string
	desc bool
}

var sortOrders = map[string]sortOrder{
	SortNewest: {keys: []string{"b.created_at"}, desc: true},
	SortOldest: {keys: []string{"b.created_at"}},
	SortTitle:  {keys: []string{"lower(b.title)"}},
	SortDomain: {keys: []string{"b.domain", "lower(b.title)"}},
}

// marker is a row's place in a sort: its sort values and ID. It holds the
// values rather than just the ID, because the row may since have been deleted.
type marker struct {
	Sort string   `json:"s"`
	Keys []string `json:"k"`
	ID   int      `json:"i"`
}

func (mk marker) encode() string {
	data, _ := json.Marshal(mk) // can't fail for strings and an int
	return base64.RawURLEncoding.EncodeToString(data)
}

func decodeMarker(s, sort string) (marker, error) {
	var mk marker
	data, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return mk, ErrBadMarker
	}
	if err = json.Unmarshal(data, &mk); err != nil {
		return mk, ErrBadMarker
	}
	if mk.Sort != sort || len(mk.Keys) != len(sortOrders[sort].keys) {
		return mk, ErrBadMarker
	}
	return mk, nil
}

// where builds the conditions for a user's Bookmarks matching a filter.
func (f Filter) where(userID int) (string, []any) {
	conds := []string{"b.user_id = ?"}
	args := []any{userID}
	if f.Favourite {
		conds = append(conds, "b.favourite")
	}
	if f.ReadLater {
		conds = append(conds, "b.read_later")
	}
	if f.Domain != "" {
		conds = append(conds, "b.domain = ?")
		args = append(args, f.Domain)
	}
	for _, tag := range f.Tags {
		conds = append(conds, `EXISTS (SELECT 1 FROM bookmark_tags bt JOIN tags t ON t.id = bt.tag_id
		                               WHERE bt.bookmark_id = b.id AND t.name = ?)`)
		args = append(args, tag)
	}
	return strings.Join(conds, " AND "), args
}

// List returns one batch of the user's Bookmarks that match the query.
// Returns ErrBadMarker if a marker can't be used, and the caller can then
// start again from the top.
func (m *BookmarkModel) List(userID int, q ListQuery) (*ListPage, error) {
	if _, ok := sortOrders[q.Sort]; !ok {
		q.Sort = SortNewest
	}
	order := sortOrders[q.Sort]
	filterWhere, filterArgs := q.Filter.where(userID)

	page := &ListPage{Bookmarks: []*Bookmark{}}
	err := m.DB.QueryRow(`SELECT count(*) FROM bookmarks b WHERE `+filterWhere, filterArgs...).Scan(&page.Total)
	if err != nil {
		return nil, err
	}

	where, args := filterWhere, slices.Clone(filterArgs)
	limit := ListBatch
	if q.After != "" {
		cond, condArgs, err := order.compare(q.After, q.Sort, false)
		if err != nil {
			return nil, err
		}
		where += " AND " + cond
		args = append(args, condArgs...)
	}
	if q.UpTo != "" {
		cond, condArgs, err := order.compare(q.UpTo, q.Sort, true)
		if err != nil {
			return nil, err
		}
		where += " AND " + cond
		args = append(args, condArgs...)
		limit = ListMax
	}

	// The sort values are also selected as text, exactly as stored, for the marker.
	keyCols := make([]string, len(order.keys))
	for i, k := range order.keys {
		keyCols[i] = "CAST(" + k + " AS TEXT)"
	}
	stmt := `SELECT ` + bookmarkColumns + `, ` + strings.Join(keyCols, ", ") +
		` FROM bookmarks b WHERE ` + where + ` ORDER BY ` + order.orderBy() + ` LIMIT ?`
	rows, err := m.DB.Query(stmt, append(args, limit+1)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var last marker
	for rows.Next() {
		if len(page.Bookmarks) == limit {
			page.More = true
			break
		}
		keys := make([]string, len(order.keys))
		b, err := scanBookmark(rows, stringPointers(keys)...)
		if err != nil {
			return nil, err
		}
		page.Bookmarks = append(page.Bookmarks, b)
		last = marker{Sort: q.Sort, Keys: keys, ID: b.ID}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(page.Bookmarks) == 0 {
		return page, nil
	}
	page.Last = last.encode()

	// Loading down to a marker stops there, so look past it for more rows.
	if q.UpTo != "" && !page.More {
		cond, condArgs, err := order.compare(page.Last, q.Sort, false)
		if err != nil {
			return nil, err
		}
		stmt = `SELECT EXISTS (SELECT 1 FROM bookmarks b WHERE ` + filterWhere + ` AND ` + cond + `)`
		if err = m.DB.QueryRow(stmt, append(slices.Clone(filterArgs), condArgs...)...).Scan(&page.More); err != nil {
			return nil, err
		}
	}
	return page, nil
}

// compare gives the condition for rows after a marker in this sort or, with
// upTo, for rows from the top down to and including it.
func (o sortOrder) compare(s, sort string, upTo bool) (string, []any, error) {
	mk, err := decodeMarker(s, sort)
	if err != nil {
		return "", nil, err
	}
	op := map[[2]bool]string{
		{false, false}: ">", {false, true}: "<=", // up the alphabet or forward in time
		{true, false}: "<", {true, true}: ">=", // newest first
	}[[2]bool{o.desc, upTo}]
	cols := append(slices.Clone(o.keys), "b.id")
	placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(cols)), ", ")
	args := []any{}
	for _, k := range mk.Keys {
		args = append(args, k)
	}
	args = append(args, mk.ID)
	return "(" + strings.Join(cols, ", ") + ") " + op + " (" + placeholders + ")", args, nil
}

func (o sortOrder) orderBy() string {
	direction := " ASC"
	if o.desc {
		direction = " DESC"
	}
	return strings.Join(append(slices.Clone(o.keys), "b.id"), direction+", ") + direction
}

func stringPointers(s []string) []any {
	ptrs := make([]any, len(s))
	for i := range s {
		ptrs[i] = &s[i]
	}
	return ptrs
}

// TagCounts returns the Tags on the user's Bookmarks that match the filter,
// A-Z, each with how many of those Bookmarks have it.
func (m *BookmarkModel) TagCounts(userID int, f Filter) ([]*Tag, error) {
	where, args := f.where(userID)
	stmt := `SELECT t.id, t.name, count(*) FROM bookmarks b
	         JOIN bookmark_tags bt ON bt.bookmark_id = b.id
	         JOIN tags t ON t.id = bt.tag_id
	         WHERE ` + where + `
	         GROUP BY t.id ORDER BY t.name`
	rows, err := m.DB.Query(stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tags := []*Tag{}
	for rows.Next() {
		t := &Tag{}
		if err := rows.Scan(&t.ID, &t.Name, &t.BookmarkCount); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return tags, nil
}
