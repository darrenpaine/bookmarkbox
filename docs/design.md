# BookmarkBox design

How BookmarkBox works and is planned to work. Requirements are in [requirements.md](requirements.md), and the reasons behind the major decisions are in [adr/](adr/). Sections marked **Planned** are not built yet; the rest describes the code as it stands.

## Architecture

A single server-rendered Go web application.

| Part | Choice |
|---|---|
| Language | Go 1.26 |
| Router | Chi, with route groups for public and logged-in pages |
| Templates | `html/template`, embedded in the binary (`ui/`) |
| Database | SQLite through `modernc.org/sqlite` ([ADR 1](adr/0001-sqlite-with-an-ephemeral-demo.md)) |
| Migrations | goose, embedded (`migrations/`), applied at start-up |
| Sessions | `scs`, stored in SQLite through `sqlite3store` |
| Passwords | bcrypt, cost 12 |
| Cross-site protection | Go's `http.NewCrossOriginProtection` |

Layout:

- `cmd/web`: handlers, routes, middleware, templates and `main`.
- `internal/models`: all SQL, behind interfaces. Handler tests use the mocks in `internal/models/mocks`.
- `internal/validator`: form validation.
- `migrations`: numbered SQL files.
- `ui`: templates and static files.

Because it uses one SQLite file, the app runs as one instance.

## Data model

The schema is in `migrations/00001_init.sql`. Tables: `users`, `sessions`, `bookmarks`, `tags` and `bookmark_tags`.

- `users.email` is unique and case-insensitive.
- `bookmarks` has `UNIQUE (user_id, url)`, and `tags` has `UNIQUE (user_id, name)`.
- Deletes cascade from `users` to `bookmarks` and `tags`, and from `bookmarks` and `tags` to `bookmark_tags`.
- Foreign keys need turning on for every connection. The default DSN does this with `_pragma=foreign_keys(1)`, along with WAL mode and a busy timeout.

### Authorization

Every Bookmark method in `internal/models` takes the Owner's user ID and filters on it. Another User's Bookmark therefore gives `ErrNoRecord` and a 404, the same as one that does not exist ([ADR 2](adr/0002-multi-user-with-private-bookmarks.md)).

### Tags

- Saved in one transaction with their Bookmark.
- Tag names are trimmed, lower-cased, de-duplicated and sorted.
- Updating a Bookmark (Planned) replaces its Tag links in one transaction.
- Loaded as one comma-separated value, so a Tag name can never contain a comma.

### URL normalisation (Planned)

A new `normalised_url` column replaces `url` in the unique constraint, and `url` keeps the address as entered, for display. All routes into the system share one normalising function.

| Changed | Kept as it is |
|---|---|
| Scheme and host lower-cased | `http` and `https` are different |
| Default ports removed | `www.` |
| One trailing slash removed | Query parameter order |
| `utm_*` parameters removed | Fragments, which can select pages in single-page apps |

The rule prefers an occasional near-duplicate to wrongly merging two pages.

### Removing unused Tags (Planned)

When a Bookmark is updated or deleted, any Tag left with no Bookmarks is removed in the same transaction. Chip colours are derived from the Tag name, so nothing needs storing on a Tag.

### Renaming Tags (Planned)

A rename updates the single row in `tags`, so every Bookmark follows without further changes. The new name is normalised like any other Tag name, and commas are rejected.

If the Owner already has a Tag with the new name, the page asks for confirmation, then merges in one transaction: Bookmarks move to the existing Tag, and the old Tag is removed. A Bookmark that already has both Tags keeps just one.

A rename never creates a new Tag, so the Tag limit does not apply.

### Limits (Planned)

User, Bookmark and Tag limits are checked inside the insert itself (`INSERT ... SELECT ... WHERE count < limit`), so concurrent requests cannot pass them. The Tag limit applies only when a save would create a new Tag, and reusing an existing Tag is always allowed. A limit of `0` means unlimited. `MAX_USERS` counts every User, including the Demo User.

To close signup on a running site, set `MAX_USERS` to the current number of Users.

## Web pages

| Route | Access | Purpose |
|---|---|---|
| `GET /` | Public | Latest Bookmarks when logged in, otherwise a welcome page |
| `GET /about` | Public | About page |
| `GET, POST /user/signup` | Public | Sign up |
| `GET, POST /user/login` | Public | Log in |
| `POST /user/logout` | Logged in | Log out |
| `GET, POST /bookmarks/new` | Logged in | Add a Bookmark |
| `GET /bookmarks/{id}` | Owner | View a Bookmark |
| `GET, POST /bookmarks/{id}/edit` | Owner | Update a Bookmark (Planned) |
| `POST /bookmarks/{id}/favourite` | Owner | Toggle Favourite |
| `POST /bookmarks/{id}/delete` | Owner | Delete a Bookmark |
| `GET /tags` | Logged in | List the User's Tags (Planned) |
| `POST /tags/{id}/rename` | Owner | Rename or merge a Tag (Planned) |
| `GET /account/view` | Logged in | Account details |
| `GET, POST /account/password/update` | Logged in | Change password |
| `GET /ping` | Public | Health check, without a session |

### Frontend (Planned)

- Replace the book's CSS, starting from a small base (Pico.css or Open Props), with own colour tokens.
- htmx for in-place actions, keeping server-rendered Go templates.

### List views (Planned)

One list handler with query parameters and a few templates, for example `/bookmarks?view=cards&tag=go&fav=1&sort=title`. The last List view is stored in the session.

Search uses an FTS5 index over title, URL, notes and Tags. FTS5 was confirmed working on `modernc.org/sqlite` on 2026-11-04.

## Import and export (Planned)

All four browsers export the Netscape bookmark file (`bookmarks.html`), so one importer covers them.

**Import**

- Parse with `golang.org/x/net/html`, tracking the Folder path: `<H3>` Folders inside nested `<DL>` lists, and `<A HREF ADD_DATE ICON>` links.
- Each Folder name becomes a Tag ([ADR 4](adr/0004-folders-become-flat-tags.md)). Browser root Folders are skipped, and commas in names are replaced.
- `ADD_DATE` becomes `created_at`.
- Duplicates are found by normalised URL.
- A `TAGS` attribute is read if present, which also lets exports come back in.
- The preview runs before any write. Imports over the Bookmark or Tag limit are rejected there, and the API bulk import rejects them in the same way.

**Export**

- A flat list of links with `TAGS="go,work"`, `ADD_DATE`, and extra attributes for Favourite and Read later.
- The round-trip test runs import, export and compare.
- BookmarkBox's own attributes are for its round trip. Nothing relies on a browser keeping them: a Mozilla bug report describes Firefox ignoring `TAGS` on import, and Chromium's behaviour is unconfirmed.

## JSON API (Planned)

Routes sit under `/api/v1`: create a Bookmark, list Tags, "is this URL already saved?", and bulk import.

- **Auth:** personal tokens, stored as a hash and sent as `Authorization: Bearer ...`, in a separate Chi middleware group ([ADR 3](adr/0003-api-bearer-tokens.md)).
- **Cross-origin protection:** bypassed for `/api/` with `AddInsecureBypassPattern`, because no cookies are involved.
- **CORS:** only origins in `CORS_ORIGINS`, such as `chrome-extension://<id>`, may call `/api`. HTML routes stay closed.

## Browser extension (Planned)

- Manifest V3.
- A server URL setting, with `optional_host_permissions` requested at runtime, because each self-hoster has their own address.
- The `bookmarks` permission enables direct import, which sends the browser's bookmark tree to the bulk import API.
- The server's allowed origins are configurable, because an extension's ID differs between a store build and a locally loaded one.

## Background metadata (Planned)

- Fetch title, description and favicon in a goroutine after saving, with timeouts and `context` cancellation.
- Reject private and loopback addresses **after** DNS lookup, including on redirects, unless `FETCH_ALLOW_PRIVATE=true`. Demo mode forces this off.

## Configuration (Planned)

Existing flags stay for local use, with environment variables setting their defaults. Today the app has only `-addr`, `-dsn`, `-debug` and `-https`, and `-https` also sets the cookie's Secure flag.

| Variable | Default | Purpose |
|---|---|---|
| `PORT` | `4000` | Listening port |
| `DATABASE_PATH` | `./bookmarkbox.db` | SQLite file (`/data/bookmarkbox.db` in the container) |
| `COOKIE_SECURE` | `true` | Secure session cookie; set `false` for plain HTTP on a home network |
| `MAX_USERS` | `1` | Signup closes at this many Users; `0` is unlimited |
| `MAX_BOOKMARKS_PER_USER` | `0` | `0` is unlimited |
| `MAX_TAGS_PER_USER` | `0` | `0` is unlimited |
| `TRUSTED_PROXY` | unset | When set, the real client IP is taken from the forwarded header |
| `CORS_ORIGINS` | unset | Origins allowed to call `/api` |
| `FETCH_ALLOW_PRIVATE` | `false` | Let metadata fetching reach private addresses |
| `DEMO_MODE` | `false` | Turn on the demo behaviour below |

## Deployment (Planned)

### Container and self-hosting

- Multi-stage Dockerfile producing a small image. `modernc.org/sqlite` needs no cgo, so the binary can be static.
- `docker-compose.yml` has one `bookmarkbox` service, a named volume at `/data`, and `restart: unless-stopped`.
- The app serves plain HTTP, and TLS is handled by the self-hoster's proxy.
- Backups use `VACUUM INTO` or `sqlite3 .backup`. Copying a live WAL database file is not safe.

### Public demo

Render's free web service runs the same image with `DEMO_MODE=true`, `MAX_USERS=25`, `MAX_BOOKMARKS_PER_USER=300` and `MAX_TAGS_PER_USER=300`. Render figures were checked on 2026-11-03 and 2026-11-04 and should be rechecked before deploying.

- Render's free filesystem is lost on every redeploy, restart or spin-down, and a service spins down after 15 minutes without traffic ([ADR 1](adr/0001-sqlite-with-an-ephemeral-demo.md)). Waking takes about a minute.
- A timer also resets the data every 24 hours, because steady traffic could stop the service spinning down.
- A reset wipes all Users, Bookmarks and sessions, then recreates the Demo User and loads an embedded public bookmark file through the real importer.
- The seed of about 200 Bookmarks stays below the limits of 300 Bookmarks and 300 Tags, leaving room for visitors.
- Handlers block password changes and token creation for the Demo User.

### Rate limiting

An in-memory, per-IP limit of about 10 attempts a minute on login and signup. bcrypt is deliberately slow, so a burst could otherwise stall a small instance. The forwarded client IP is only trusted when `TRUSTED_PROXY` is set.

### CI

GitHub Actions runs `go test`, `go vet` and `staticcheck`.

### Logging

Currently `log.Logger`, moving to `log/slog` for structured logs.

## Testing

- Handler tests in `cmd/web` use the model mocks and a test server.
- Model tests in `internal/models` run against a temporary SQLite file with real migrations and `testdata/setup.sql`. They need no set-up.
- Run everything with `make test`.

## AI Tag suggestions (Possible later)

Kept behind an interface, so the feature can be switched off or mocked in tests.
