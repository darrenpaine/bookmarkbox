## BookmarkBox

A personal bookmark manager in Go, with tags and favourites.

Based on the 'SnippetBox' project from ["Let's Go" by Alex Edwards](https://lets-go.alexedwards.net/).

### Updates
- Change to use the Chi router (clearer routes and middleware setup)
- Use native CORS protection included in Go 1.25+
- Fixes to display on mobile
- Snippets replaced with bookmarks: each user's bookmarks are private, with tags and a favourite star
- Switched MySQL to SQLite (`modernc.org/sqlite`, pure Go, no cgo); sessions stored in SQLite too
- Schema managed by embedded `goose` migrations, applied on start-up

### Next steps
- Edit saved bookmarks, rename tags, spot duplicates by normalised URL, remove unused tags, add limits
- Self-hosting with Docker Compose, plus environment config, `slog` and CI
- New look: own CSS with dark mode, favicons, tag chips, and htmx for starring, tagging and deleting in place
- List views (compact, cards, by tag, favourites, read later) with sorting and filters
- Search across titles, URLs, notes and tags (SQLite FTS5)
- Import and export the bookmark files from Chrome, Edge, Firefox and Brave
- Live demo on Render, with seeded data that resets regularly
- JSON API with personal API tokens
- Browser extension to save the current page, plus a right-click menu option
- Fetch page titles, descriptions and favicons in the background

See [requirements](docs/requirements.md), [design](docs/design.md) and the [decision records](docs/adr/).

### Running:
No database set-up needed: the app creates `bookmarkbox.db` and applies the migrations on first run.

```bash
go run ./cmd/web
# Specify a port or a different database file:
go run ./cmd/web -addr :4000 -dsn "file:other.db?_pragma=foreign_keys(1)"
```

In a browser go to http://localhost:4000

### Testing:
```bash
go test ./...
```
Model tests use a temporary SQLite file, so they need no set-up either.
