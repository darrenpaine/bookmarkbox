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
- `bookmarks` has `UNIQUE (user_id, normalised_url)`, and `tags` has `UNIQUE (user_id, name)`.
- Deletes cascade from `users` to `bookmarks` and `tags`, and from `bookmarks` and `tags` to `bookmark_tags`.
- Foreign keys need turning on for every connection. The default DSN does this with `_pragma=foreign_keys(1)`, along with WAL mode and a busy timeout.

### Authorization

Every Bookmark method in `internal/models` takes the Owner's user ID and filters on it. Another User's Bookmark therefore gives `ErrNoRecord` and a 404, the same as one that does not exist ([ADR 2](adr/0002-multi-user-with-private-bookmarks.md)).

### Tags

- Saved in one transaction with their Bookmark.
- Tag names are trimmed, lower-cased, de-duplicated and sorted.
- Updating a Bookmark changes its Tag links in one transaction. Links that stay are kept, so a Tag keeps its ID.
- Loaded as one comma-separated value, so a Tag name can never contain a comma.

### URL normalisation

`normalised_url` is the unique key, and `url` keeps the address as entered, for display. Every route into the system uses `models.NormaliseURL`.

| Changed | Kept as it is |
|---|---|
| Scheme and host lower-cased | `http` and `https` are different |
| Default ports removed | `www.` |
| One trailing slash removed | Query parameter order |
| `utm_*` parameters removed | Fragments, which can select pages in single-page apps |

The rule prefers an occasional near-duplicate to wrongly merging two pages.

### Removing unused Tags

The `tags_remove_unused` trigger deletes a Tag when its last row leaves `bookmark_tags`. Foreign key cascades fire it too, so it covers every way a Bookmark can lose a Tag, including routes added later. Chip colours will be derived from the Tag name, so nothing needs storing on a Tag.

### Renaming Tags

A rename updates the single row in `tags`, so every Bookmark follows without further changes. The new name is normalised like any other Tag name, and commas are rejected.

If the Owner already has a Tag with the new name, the page asks for confirmation, then merges in one transaction: Bookmarks move to the existing Tag, and the old Tag is removed. A Bookmark that already has both Tags keeps just one.

A rename never creates a new Tag, so the Tag limit does not apply.

### Limits

User, Bookmark and Tag limits are checked by counting at the end of the write transaction, and rolling back if over. SQLite allows one writer at a time and the transaction has already written, so concurrent requests cannot pass a limit. A failed save changes nothing and shows an error on the form.

- A limit of `0` means unlimited.
- The User limit counts every User, including the Demo User.
- The Tag limit is checked only when a save creates a new Tag, and after old links are removed. Swapping one Tag for another at the limit works, and someone over a lowered limit can still edit their Bookmarks.
- To close signup on a running site, set the User limit to the current number of Users.

When signup is full, logged-out pages hide the Signup link and `/user/signup` shows "Signup is closed". A POST that arrives anyway is refused with a 403.

### Text limits

Maximum lengths are shared constants in `internal/models`: title 200 characters, URL and description 2,000, notes 10,000, Tag name 50. Handlers validate against them, and the importer and API will use the same constants. There are no database `CHECK` rules.

Form posts are limited to 1 MB in `decodePostForm`. File uploads in step 6 will need their own larger limit.

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
| `GET, POST /bookmarks/{id}/edit` | Owner | Update a Bookmark |
| `POST /bookmarks/{id}/favourite` | Owner | Toggle Favourite |
| `POST /bookmarks/{id}/read-later` | Owner | Toggle Read later |
| `POST /bookmarks/{id}/tags` | Owner | Add one Tag |
| `POST /bookmarks/{id}/tags/remove` | Owner | Remove one Tag |
| `POST /bookmarks/{id}/delete` | Owner | Delete a Bookmark |
| `GET /tags` | Logged in | List the User's Tags |
| `POST /tags/{id}/rename` | Owner | Rename or merge a Tag |
| `GET /account/view` | Logged in | Account details |
| `GET, POST /account/password/update` | Logged in | Change password |
| `GET /ping` | Public | Health check, without a session |

### Look

- Own CSS in `ui/static/css/main.css`, with no framework. Colours are tokens on `:root`, redefined under `prefers-color-scheme: dark`, so dark mode follows the system setting with no toggle.
- **Type:** Instrument Sans for text, and IBM Plex Sans Condensed for headings and the lowercase `bookmarkbox` wordmark. Both are served from `ui/static/fonts` as Latin WOFF2 files, with their OFL licences, so no page loads anything from another site.
- **Accent:** one deep red, `#B3263A` (`#D9586C` in dark mode), for the Favourite star, main buttons, focus outlines and the current nav link.
- **Tag and icon colours:** a Tag's colour class (`c0` to `c7`) comes from an FNV hash of its name, so nothing is stored on a Tag and the CSP needs no inline styles. Letter icons use the same hash on the domain, and stand in for favicons until step 10.
- **App icon:** a lowercase b on a deep red square, drawn as SVG paths because favicons can't use web fonts, with 32px and 180px PNG fallbacks.
- **List rows:** a title opens the website itself. Details opens the Bookmark's own page, with its notes and Tag editor.
- **Layout:** a fixed bar holds the wordmark and the Add button. The main nav sits in the bar on wide screens and below it, scrolling away, on screens under 720px. Every control is at least 24px tall.

### In-place actions

htmx 2.0.11 is served from `ui/static/js`. htmx 2.0.x is used because 4.0 was released on 2026-11-01 as a rewrite and npm still tags it `next`. Revisit htmx 4 once it has matured and been widely tested.

- Every action is an ordinary form post, so everything works without JavaScript. With htmx, the handler sees `HX-Request` and returns just the changed fragment, from the partials in `ui/html/partials/bookmark.tmpl`.
- **Favourite and Read later** return the new toggle. Without JavaScript they redirect to a `next` path from the form, which must be a path on this site.
- **Delete** asks first with `hx-confirm`. From a list, `HX-Target` names the row and the empty response removes it. From a Bookmark's page, `HX-Redirect` goes home. Without JavaScript there is no confirmation.
- **Adding and removing a Tag** on a Bookmark's page returns the whole Tag editor. A problem, such as the Tag limit, is shown inside it with a 200, because htmx doesn't swap error responses. After adding, the field keeps focus.
- The `htmx-config` meta tag turns off htmx's inline indicator styles and `eval`, so the CSP stays `default-src 'self'`.
- Flash messages have two kinds, success and error.

### List views (Planned)

One list handler with query parameters and a few templates, for example `/bookmarks?view=cards&tag=go&fav=1&sort=title`. The last List view is stored in the session. The home list built in step 4 becomes the Compact List view.

- **Long lists:** a "Show more" button at the end adds the next 50 rows in place with htmx, and is an ordinary link to the next batch without JavaScript. The address updates as rows are added, so Back and refresh keep the place. Until then, the home page shows the newest 50.
- **Search in the fixed bar:** a search control joins the Add button. On a phone it is a button that opens the search box, so the bar stays one line.

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

## Configuration

Environment variables set the defaults, and flags override them for local use. An invalid value stops start-up with an error.

| Variable | Flag | Default | Purpose |
|---|---|---|---|
| `PORT` | `-addr` | `4000` | Listening port; `-addr` takes a full address |
| `DATABASE_PATH` | `-db` | `./bookmarkbox.db` | SQLite file (`/data/bookmarkbox.db` in the container) |
| `COOKIE_SECURE` | `-cookie-secure` | `true` | Secure session cookie; set `false` for plain HTTP on a home network |
| `MAX_USERS` | `-max-users` | `1` | Signup closes at this many Users; `0` is unlimited |
| `MAX_BOOKMARKS_PER_USER` | `-max-bookmarks` | `0` | `0` is unlimited |
| `MAX_TAGS_PER_USER` | `-max-tags` | `0` | `0` is unlimited |

- `-debug` shows stack traces in the browser. It is for local use and has no environment variable.
- The app takes a file path rather than a DSN, and always adds the foreign key, WAL and busy timeout settings, so they can't be left out by accident.
- `make run` passes `-max-users 0` and `-cookie-secure=false`.

**Planned**, each arriving with the feature that uses it:

| Variable | Default | Purpose |
|---|---|---|
| `TRUSTED_PROXY` | unset | When set, the real client IP is taken from the forwarded header |
| `CORS_ORIGINS` | unset | Origins allowed to call `/api` |
| `FETCH_ALLOW_PRIVATE` | `false` | Let metadata fetching reach private addresses |
| `DEMO_MODE` | `false` | Turn on the demo behaviour below |

## Deployment

### Container and self-hosting

[self-hosting.md](self-hosting.md) has the instructions.

- A multi-stage Dockerfile builds a static binary, because `modernc.org/sqlite` needs no cgo. The final image is pinned Alpine with the `sqlite` package, which provides the backup command, and runs as a non-root user.
- `docker-compose.yml` has one `bookmarkbox` service, a named volume at `/data`, and `restart: unless-stopped`. It publishes on `127.0.0.1` unless `BIND_ADDRESS` says otherwise.
- The app serves plain HTTP, and TLS is handled by the self-hoster's proxy.
- The image's health check calls `/ping`. `wget` uses GET because the route answers nothing else.
- Backups use `sqlite3 .backup`. Copying a live WAL database file is not safe.

### Shutdown

On SIGTERM or an interrupt the app stops accepting connections, lets requests in progress finish for up to 8 seconds, then closes the database, which checkpoints the WAL file. Docker kills a container 10 seconds after asking it to stop, so the limit sits below that.

### Public demo (Planned)

Render's free web service runs the same image with `DEMO_MODE=true`, `MAX_USERS=25`, `MAX_BOOKMARKS_PER_USER=300` and `MAX_TAGS_PER_USER=300`. Render figures were checked on 2026-11-03 and 2026-11-04 and should be rechecked before deploying.

- Render's free filesystem is lost on every redeploy, restart or spin-down, and a service spins down after 15 minutes without traffic ([ADR 1](adr/0001-sqlite-with-an-ephemeral-demo.md)). Waking takes about a minute.
- A timer also resets the data every 24 hours, because steady traffic could stop the service spinning down.
- A reset wipes all Users, Bookmarks and sessions, then recreates the Demo User and loads an embedded public bookmark file through the real importer.
- The seed of about 200 Bookmarks stays below the limits of 300 Bookmarks and 300 Tags, leaving room for visitors.
- Handlers block password changes and token creation for the Demo User.

### Rate limiting (Planned)

An in-memory, per-IP limit of about 10 attempts a minute on login and signup. bcrypt is deliberately slow, so a burst could otherwise stall a small instance. The forwarded client IP is only trusted when `TRUSTED_PROXY` is set.

### CI

GitHub Actions runs on pull requests and pushes to `main`, with two jobs:

- `go vet`, `staticcheck` and `go test -race`.
- `scripts/smoke-test.sh`, which builds the real image and tests it through Docker Compose: health check, non-root user, saving data, restart, shutdown, backup and restore.

Images aren't published. Compose builds from the repository.

### Logging

`log/slog` text logs on stdout, for all levels. `docker compose logs` is read by people, so there is no JSON option. Requests to `/ping` are skipped, because health checks would flood the log.

## Testing

- Handler tests in `cmd/web` use the model mocks and a test server.
- Model tests in `internal/models` run against a temporary SQLite file with real migrations and `testdata/setup.sql`. They need no set-up.
- Run everything with `make test`. `make smoke` tests the real image, and needs Docker.

## AI Tag suggestions (Possible later)

Kept behind an interface, so the feature can be switched off or mocked in tests.
