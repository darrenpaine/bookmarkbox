## <img src="ui/static/img/icon.svg" alt="" width="32" height="32"> bookmarkbox

A personal bookmark manager in Go, with tags and favourites.

Based on the 'SnippetBox' project from ["Let's Go" by Alex Edwards](https://lets-go.alexedwards.net/).

[![CI Status](https://github.com/darrenpaine/bookmarkbox/actions/workflows/ci.yml/badge.svg)](https://github.com/darrenpaine/bookmarkbox/actions/workflows/ci.yml)

### Updates
- Change to use the Chi router (clearer routes and middleware setup)
- Use native CORS protection included in Go 1.25+
- Fixes to display on mobile
- Snippets replaced with bookmarks: each user's bookmarks are private, with tags and a favourite star
- Switched MySQL to SQLite (`modernc.org/sqlite`, pure Go, no cgo); sessions stored in SQLite too
- Schema managed by embedded `goose` migrations, applied on start-up
- Edit saved bookmarks, and rename or merge tags
- Spot duplicates by normalised URL, and remove tags once nothing uses them
- Configurable limits on users, bookmarks and tags, plus text length limits
- Self-hosting with Docker Compose, with settings from environment variables
- Structured logging with `slog`, a clean shutdown, and CI on GitHub Actions
- New look: own CSS and fonts, dark mode, coloured tag ribbons, and a layout that works on phones
- Star, mark for reading later, tag and delete bookmarks in place with htmx, with no page reload
- List views (compact, cards and by tag) with filters for tags, domain, favourites and read later, four sorts, and "Show more" paging that never skips a row
- Search across titles, descriptions, URLs, notes and tags (SQLite FTS5), with results that update as you type
- Import the bookmark files from Chrome, Edge, Firefox and Brave, with a preview first and folders turned into tags; export them all in the same format
- Choose a light, dark or system theme on the Account page; on phones, the Account link is your initial in the top bar
- Tag autocomplete: the tag fields suggest your existing tags as you type, most used first
- Rate limits on login, signup and password change, with the client IP read from a trusted reverse proxy
- A [threat model](docs/threat-model.md), `govulncheck` in CI, weekly Dependabot updates, and a [security policy](SECURITY.md)
- Type an address without `https://`, such as `bbc.co.uk`, and it is completed for you

### Next steps
- Live demo on Render, with seeded data that resets regularly
- JSON API with personal API tokens
- Browser extension to save the current page, plus a right-click menu option
- Fetch page titles, descriptions and favicons in the background
- Later, for wider use: sign in with Google or Microsoft, password reset, and scaling to many users (see [scaling](docs/scaling.md))

See [requirements](docs/requirements.md), [design](docs/design.md) and the [decision records](docs/adr/).

### Self-hosting:
```bash
cp .env.example .env
docker compose up -d --build
```

See [self-hosting](docs/self-hosting.md) for HTTPS, home networks, backups and upgrades.

### Running for development:
No database set-up needed: the app creates `bookmarkbox.db` and applies the migrations on first run.

```bash
go run ./cmd/web -cookie-secure=false
# Specify a port or a different database file:
go run ./cmd/web -cookie-secure=false -addr :4000 -db other.db
# Allow more users, and limit each one (0 means unlimited):
go run ./cmd/web -cookie-secure=false -max-users 5 -max-bookmarks 1000 -max-tags 300
```

Environment variables set the same options, as listed in the [design](docs/design.md#configuration). Signup closes after the first user unless `-max-users` is raised. `make run` sets it to unlimited for development, and turns off the HTTPS-only session cookie.

In a browser go to http://localhost:4000

### Testing:
```bash
go test ./...
```
Model tests use a temporary SQLite file, so they need no set-up either.

`make smoke` tests the real image through Docker Compose, including backup and restore.
