# BookmarkBox design

How BookmarkBox works and is planned to work. Requirements are in [requirements.md](requirements.md), the reasons behind the major decisions are in [adr/](adr/), and options for running it at a much larger scale are in [scaling.md](scaling.md). Sections marked **Planned** are not built yet; the rest describes the code as it stands.

## Architecture

A single server-rendered Go web application.

| Part | Choice |
|---|---|
| Language | Go 1.27 |
| Router | Chi, with route groups for public and logged-in pages |
| Templates | `html/template`, embedded in the binary (`ui/`) |
| Database | SQLite through `modernc.org/sqlite` ([ADR 1](adr/0001-sqlite-with-an-ephemeral-demo.md)) |
| Migrations | goose, embedded (`migrations/`), applied at start-up |
| Sessions | `scs`, stored in SQLite through `sqlite3store` |
| Passwords | bcrypt, cost 12 |
| Cross-site protection | Go's `http.NewCrossOriginProtection` |
| Rate limiting | `golang.org/x/time/rate`, in memory |

Layout:

- `cmd/web`: handlers, routes, middleware, templates and `main`.
- `internal/models`: all SQL, behind interfaces. Handler tests use the mocks in `internal/models/mocks`.
- `internal/validator`: form validation.
- `internal/ratelimit`: attempts counted per key, for [rate limiting](#rate-limiting).
- `internal/urls`: URL helpers shared by the models, migrations and templates, such as `Domain`.
- `migrations`: numbered goose migrations, mostly SQL files. A migration that needs Go code is registered in `migrations.go`.
- `ui`: templates and static files.

Because it uses one SQLite file, the app runs as one instance.

## Data model

The schema starts in `migrations/00001_init.sql`. Tables: `users`, `sessions`, `bookmarks`, `tags` and `bookmark_tags`.

- `users.email` is unique and case-insensitive.
- `bookmarks` has `UNIQUE (user_id, normalised_url)`, and `tags` has `UNIQUE (user_id, name)`.
- Deletes cascade from `users` to `bookmarks` and `tags`, and from `bookmarks` and `tags` to `bookmark_tags`.
- `bookmarks.domain` is the URL's lower-case host without `www.`, from `urls.Domain`, set on every save. Migration 2 is a Go migration so that existing Bookmarks get theirs from the same function. It is indexed with `user_id` for the domain Filter and sort.
- `bookmarks_fts` is the search index, an FTS5 table from migration 3 with one row per Bookmark. See [Search](#search).
- `imports` holds each User's uploaded bookmark file while they preview it, from migration 4. See [Import and export](#import-and-export).
- `users.theme` is `system`, `light` or `dark`, from migration 5, with a `CHECK` constraint and `system` as the default. See [Theme](#theme).
- Foreign keys need turning on for every connection. The default DSN does this with `_pragma=foreign_keys(1)`, along with WAL mode and a busy timeout.
- Transactions begin with `BEGIN IMMEDIATE` (`_txlock=immediate`), taking the write lock at the start. In WAL mode, a transaction that has read can't wait to upgrade to a write if another connection wrote meanwhile, and fails at once with "database is locked". FTS5 reads its own settings the first time a connection uses the index, so without this, saving a Bookmark could fail under concurrent writes.

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

### Typed addresses

The add and edit forms complete an address before checking it, with `urls.Complete`, and the completed address is what is saved and shown again:
- An address with no scheme gets `https://`, as in `bbc.co.uk`, `www.bbc.co.uk/news` or `//bbc.co.uk`. Its host must contain a dot, or be `localhost` or an IPv6 address, so a word typed by mistake is refused rather than saved as `https://word`.
- A web scheme with the wrong slashes is mended: `https:/`, `https:`, `https:///`, `https:\\` and `https//` all become `https://`.
- Any other scheme, such as `mailto:` or `javascript:`, is left alone for the validator to refuse.

The field is `type='text'` with `inputmode='url'`, because a `type='url'` field makes the browser refuse `bbc.co.uk` before the server sees it. Imports aren't completed: a browser's export always has full addresses.

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
| `GET /` | Public | A welcome page, or a redirect to `/bookmarks` when logged in |
| `GET /about` | Public | About page |
| `GET, POST /user/signup` | Public | Sign up |
| `GET, POST /user/login` | Public | Log in |
| `POST /user/logout` | Logged in | Log out |
| `GET /bookmarks` | Logged in | The list, in a List view, with Filters and a sort |
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
| `GET /account/view` | Logged in | Account details, with links to import and export |
| `GET, POST /account/password/update` | Logged in | Change password |
| `POST /account/theme` | Logged in | Save the theme |
| `GET, POST /account/import` | Logged in | Upload a bookmark file, then see its preview |
| `POST /account/import/confirm` | Logged in | Import the uploaded file |
| `POST /account/import/cancel` | Logged in | Discard the uploaded file |
| `GET /account/export` | Logged in | Download every Bookmark as `bookmarkbox-YYYY-MM-DD.html` |
| `GET /ping` | Public | Health check, without a session |

### Look

- Own CSS in `ui/static/css/main.css`, with no framework. Colours are tokens on `:root`, redefined for dark mode. See [Theme](#theme).
- **Type:** Instrument Sans for text, and IBM Plex Sans Condensed for headings and the lowercase `bookmarkbox` wordmark. Both are served from `ui/static/fonts` as Latin WOFF2 files, with their OFL licences, so no page loads anything from another site.
- **Accent:** one deep red, `#B3263A` (`#D9586C` in dark mode), for the Favourite star, main buttons, focus outlines and the current nav link.
- **Tag and icon colours:** a Tag's colour class (`c0` to `c7`) comes from an FNV hash of its name, so nothing is stored on a Tag and the CSP needs no inline styles. Letter icons use the same hash on the domain, and stand in for favicons until step 10.
- **App icon:** a lowercase b on a deep red square, drawn as SVG paths because favicons can't use web fonts, with 32px and 180px PNG fallbacks.
- **List rows:** a title opens the website itself. Details opens the Bookmark's own page, with its notes and Tag editor.
- **Caching:** embedded files have no modification time, so `/static` sets `Cache-Control` itself. Page links carry `?v=` with a hash of every static file and are cached for a year. The CSS names fonts without a version, so they are cached for a year too, and a changed font needs a new file name. The two fonts every page uses are preloaded, so text isn't drawn in a fallback font first.
- **Layout:** a fixed bar holds the wordmark and the Add button. The main nav sits in the bar on wide screens and below it, scrolling away, on screens under 720px. Every control is at least 24px tall.
- **Account on small screens:** under 720px, Account is the User's initial in a ring at the end of the bar, filled on Account pages. It is round because it stands for a person, where sites get square letter tiles. The nav below the bar keeps Bookmarks and Tags, and Log out is on the Account page, which has it at every width.
- **Back:** a Bookmark's page has Back beside Edit and Delete. Each list page, whole or by htmx, saves its address in the session without its "Show more" marker, and Back goes there, or to `/bookmarks`. Deleting a Bookmark from its page goes to the same place. The address is checked with `safeNext`.

### Theme

- Each User chooses System, Light or Dark on the Account page, saved in `users.theme`. The `authenticate` middleware already looks the User up on every request, so the same query returns their name and theme for the page (`UserModel.Viewer`).
- The page sets `<html data-theme='light'>` or `'dark'`, and none for System. The `color-scheme` meta matches: `light`, `dark`, or `light dark` for System and logged-out pages, so form controls and scrollbars agree.
- The dark tokens are written twice in the CSS: under `:root[data-theme="dark"]`, and under `prefers-color-scheme: dark` for `:root:not([data-theme="light"])`. A test checks the two blocks match. Tag colours are tokens too (`--t0` to `--t7`), so they switch with the rest.
- No JavaScript chooses the theme, so a page never shows the wrong one first.
- The switch is a radio group styled like the List view switch. With htmx, choosing an option posts it at once and the response sends `HX-Refresh`, reloading the page in the new theme. Without JavaScript, a Save button inside `<noscript>` posts it. Only the three themes are accepted, and anything else gets 400.

### In-place actions

htmx 2.0.11 is served from `ui/static/js`. htmx 2.0.x is used because 4.0 was released on 2026-11-01 as a rewrite and npm still tags it `next`. Revisit htmx 4 once it has matured and been widely tested.

- Every action is an ordinary form post, so everything works without JavaScript. With htmx, the handler sees `HX-Request` and returns just the changed fragment, from the partials in `ui/html/partials/bookmark.tmpl`.
- **Favourite and Read later** return the new toggle. Without JavaScript they redirect to a `next` path from the form, which must be a path on this site.
- **Delete** asks first with `hx-confirm`. From a list, `HX-Target` names the row and the empty response removes it. From a Bookmark's page, `HX-Redirect` goes home. Without JavaScript there is no confirmation.
- When the row was on a list page, the response also carries the list's heading, counted again, as an out-of-band swap (`hx-swap-oob`) for `#list-title`: "100 bookmarks" becomes "99 bookmarks". The handler reads the list from the address htmx sends in `HX-Current-URL`, with the same parser as the list page but without changing the remembered view or sort, so the count keeps the list's Filters and search. Other counts, such as the Tag panel's, update on the next load.
- htmx keeps no copies of pages for Back (`historyCacheSize: 0`), so Bookmarks never sit in the browser's storage. Back loads the page again instead. A request with `HX-History-Restore-Request` gets a whole page.
- **Adding and removing a Tag** on a Bookmark's page returns the whole Tag editor. A problem, such as the Tag limit, is shown inside it with a 200, because htmx doesn't swap error responses. After adding, the field keeps focus.
- The `htmx-config` meta tag turns off htmx's inline indicator styles and `eval`, so the CSP stays `default-src 'self'`.
- Flash messages have two kinds, success and error.

### Tag autocomplete

- The add and edit forms and a Bookmark's page list the User's Tags in a hidden `<datalist id='tag-suggestions'>`, most used first, then A-Z, each with its colour class and Bookmark count. No route or request is needed.
- `ui/static/js/tags.js` is the app's only script besides htmx, and those pages alone load it. It is plain JavaScript, with no `eval` and no inline handlers, so the CSP is unchanged.
- A field opts in with `data-tag-suggest`: `list` for the comma-separated Tags field, `one` for the Tag editor's field. On first focus the script makes it an ARIA combobox with a listbox below it. It listens on the document, so the Tag editor keeps working after htmx swaps it.
- **Matching:** the word after the last comma, ignoring case, and only while typing at the end of the field. Tags starting with it come first, then Tags with a later word starting with it, then Tags containing it. Tags already in the field, or already on the Bookmark, are left out. At most 8 are shown, or 6 under 720px. An empty word shows nothing.
- **Choosing:** ↓ and ↑ move, Enter or Tab accepts, and Esc closes; Enter with nothing highlighted submits as before. A click or tap accepts. In a `list` field, accepting replaces the word and adds ", ". In a `one` field, it submits the form at once, so htmx adds the Tag in place and the field keeps focus.
- The highlight follows the mouse only when it moves, so a list that shifts under a still pointer doesn't take the highlight from the keyboard.
- Each suggestion is the Tag's own ribbon with its count, at least 36px tall. Without JavaScript the fields are plain text, as before.
- The handler tests check the list and the fields' markup. The script itself was checked in Firefox, with no browser tests in the repository yet.

### List views

`GET /bookmarks` serves every List view, for example `/bookmarks?view=cards&sort=title&q=sql&tag=go&tag=web&fav=1&later=1&domain=go.dev`.

- **View:** `compact`, `cards` or `tags` (By Tag). **Sort:** `newest`, `oldest`, `title` (A-Z, ignoring case) or `domain` (then by title).
- A missing or unknown view or sort uses the one remembered in the session, or Compact and newest. One given in the address is remembered. Filters are never remembered.
- Links on a list page always name the view and sort, so a copied address shows the same list later. `listState` in `cmd/web/list.go` builds them.
- **Filters** combine with AND. Several `tag` values mean Bookmarks with all of them, up to 10 Tags. Tag names are normalised as on the form, and the domain is lower-cased.
- The heading names what is shown, such as "12 favourites tagged go". When Filters match nothing, it says so with "Clear filters". "No bookmarks yet" is only for a User with no Bookmarks.
- **By Tag** adds a panel of the Tags on the Bookmarks that match the current Filters, with counts. Selected Tags come first, and the panel sticks beside the list. Under 720px it becomes a row of chips that scrolls sideways.
- Changing the view, a Filter or the sort loads the whole page. Only "Show more", live search and the row actions work in place.

### Show more

`BookmarkModel.List` returns 50 rows at a time, the total count, and a marker for the last row. It pages by the last row's place in the sort order, not by counting rows, so deleting a row, or changing one so it no longer matches the Filters, never makes the next batch skip one.

- **Marker:** the last row's sort values, as stored, and its ID, as base64url JSON. It holds the values rather than just the ID because that row may have been deleted. A marker that can't be read, or belongs to another sort, redirects to the top of the list.
- **Sorts:** every sort ends with the ID, and all its values run the same way, so one row-value comparison such as `(created_at, id) < (?, ?)` finds the next batch.
- `after=` gives the batch after a marker. With htmx, "Show more" swaps that batch in place of itself and sends `HX-Push-Url` with `upto=` and the new last row. Without JavaScript it is an ordinary link to the batch.
- `upto=` gives every row from the top down to and including a marker, up to 1,000, so reload and Back keep the place. Bookmarks added since appear in their place, never as duplicates.

### Search

A search is a Filter, `q=`, so it combines with the others and shows as a chip.

- **Index:** `bookmarks_fts`, an FTS5 table over title, description, URL, notes and Tags, whose rowid is the Bookmark's ID. It keeps its own copy of the text, because the Tags live in other tables. Triggers rebuild a Bookmark's row when it is added, edited or deleted, when it gains or loses a Tag, and when one of its Tags is renamed. A merge moves rows in `bookmark_tags`, so those triggers cover it. Migration 3 fills the index for existing Bookmarks, and the app never writes to it.
- **Words:** the `unicode61` tokenizer, ignoring case and accents. Every word must match, and also matches as the start of a longer word, so "migra" finds "migrations". Each word is quoted, so FTS5 syntax such as `OR`, `-` or `NEAR` is searched for as text. Words with no letter or digit are dropped, and only the first 20 words count. The search is kept to 200 characters.
- **Best match:** with no sort in the address, a search sorts by `match`, the BM25 rank, best first, with ties going to the newest. Title words count for most, then Tags, description, URL and notes. It shows only the best 100, with "Showing the best 100 of N" and a link to all of them, newest first. It has no "Show more" and no markers. The match sort is never remembered, and without a search it falls back to the remembered sort.
- **Other sorts** keep the search and show every match, 50 at a time, like any list.
- **The box:** in the fixed bar beside the Add button. Under 960px it is a button that opens the box in a panel below the bar. On the list page, typing updates the results after 500 ms: htmx fetches the whole page, swaps in its `<main>` and replaces the address. Hidden fields carry the view and Filters, but not the sort, so a new search starts by best match. On any other page the box is a plain form, so Enter opens `/bookmarks?q=`. Without JavaScript, Enter submits the form there too.
- No highlighting of the matched words.

## Import and export

All four browsers export the Netscape bookmark file (`bookmarks.html`), so one importer covers them. `internal/bookmarkfile` reads and writes the format and knows nothing else; `internal/models/imports.go` decides what the links become.

**Reading the file**

- `bookmarkfile.Parse` uses the `golang.org/x/net/html` tokenizer rather than the tree parser, which would rearrange the file's unclosed `<DT>` and `<p>` tags. It tracks the Folder path through nested `<DL>` lists: each `<H3>` is a Folder, each `<A>` a link, and a `<DD>` after a link is its description. Parsing is lenient, as browsers' is.
- **Root Folders** don't become Tags. Chrome-family exports mark only the bookmarks bar (`PERSONAL_TOOLBAR_FOLDER`) and write the contents of "Other bookmarks" loose at the top. Firefox marks its toolbar and "Other Bookmarks" (`UNFILED_BOOKMARKS_FOLDER`) and writes the menu's contents at the top. So a Folder is a Root Folder if it has either attribute, or if it is at the top level with a browser's English name for one, such as "Bookmarks bar" or "Other favorites".
- `ADD_DATE` is in seconds. A number too large for seconds is taken as milliseconds or microseconds and scaled down. `ICON` and `LAST_MODIFIED` are ignored.
- BookmarkBox's own attributes are read when present: `TAGS`, `NOTES`, `FAVOURITE` and `READ_LATER`.

**Preparing it** (`models.PrepareImport`)

- Each Folder in a link's path, and each name in its `TAGS`, becomes a Tag ([ADR 4](adr/0004-folders-become-flat-tags.md)). Commas become spaces and names are cut to 50 characters.
- A link with no Folder and no `TAGS` attribute is **unfiled**: if it is new, it gets the Tag `unfiled`. A Duplicate never gets it.
- Links that aren't `http` or `https`, or are over 2,000 characters, are skipped. An empty title becomes the URL. Text over its limit is cut, and a title is trimmed after cutting. A link with no date gets the time of the import.
- Links repeated in the file (by normalised URL) are merged into one, with every Tag and the earliest date.

**Preview and import**

- Uploading keeps the file, as uploaded, in `imports`: one per User, replaced by the next upload, and gone after an hour. The page then redirects to the preview, so refreshing it doesn't upload again.
- The preview and the import both read the file again and work out the same plan from the User's Bookmarks and Tags as they are: new, Duplicates, Duplicates gaining Tags, unfiled, and the counts afterwards. The preview writes nothing.
- An import over the Bookmark or Tag limit is shown as such on the preview, which has no Import button, and refused by the import. The rule matches a single save: an import that creates no Bookmark, or no Tag, passes that limit.
- The import is one transaction. It plans inside it, holding the write lock, so the limits can't be passed by a request arriving meanwhile. New Bookmarks are inserted with prepared statements, and Tag IDs are cached. 20,000 links take about two seconds.
- Uploads are limited to 20 MB and 20,000 links. Those requests, the import and the export get two minutes in place of the server's usual time limits, through `http.ResponseController`.

**Export**

- A flat list of links, oldest first, with `ADD_DATE`, `TAGS` (written even when empty, so a re-import never makes a Bookmark unfiled), `NOTES`, `FAVOURITE` and `READ_LATER`, and the description in a `<DD>`.
- Line breaks are written as character references, and so is whitespace at either end of a title or description, because the parser trims the whitespace that is layout. So the round trip is exact: the tests import, export and compare every field, with the date to the second. The favicon isn't exported; the metadata fetch will find it again.
- BookmarkBox's own attributes are for its round trip. Nothing relies on a browser keeping them: a Mozilla bug report describes Firefox ignoring `TAGS` on import, and Chromium's behaviour is unconfirmed.
- The API's bulk import (Planned) will apply the same preparation and limits.

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
- Favicons are stored once per host, in a table shared by all Users, not on each Bookmark. A site many Users save is fetched once, and refreshed now and then. Favicons are public and belong to the site, so sharing them reveals nothing about a User, provided each User is only shown the favicons of their own Bookmarks. See [scaling.md](scaling.md).
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
| `CLIENT_IP_HEADER` | `-client-ip-header` | unset | Header a trusted proxy puts the client's IP address in; see [Rate limiting](#rate-limiting) |
| `DEMO_MODE` | `-demo` | `false` | Run the [public demo](#public-demo), which deletes all data at start-up and every day |

- `-debug` shows stack traces in the browser. It is for local use and has no environment variable.
- The app takes a file path rather than a DSN, and always adds the foreign key, WAL, busy timeout and immediate transaction settings, so they can't be left out by accident.
- `make run` passes `-max-users 0` and `-cookie-secure=false`.

**Planned**, each arriving with the feature that uses it:

| Variable | Default | Purpose |
|---|---|---|
| `CORS_ORIGINS` | unset | Origins allowed to call `/api` |
| `FETCH_ALLOW_PRIVATE` | `false` | Let metadata fetching reach private addresses |

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

### Public demo

Demo mode is built; deploying it to Render is step 7d. Render's free web service runs the same image with `DEMO_MODE=true`, `MAX_USERS=25`, `MAX_BOOKMARKS_PER_USER=300` and `MAX_TAGS_PER_USER=300`. Render figures were checked on 2026-11-03 and 2026-11-04 and should be rechecked before deploying.

- Render's free filesystem is lost on every redeploy, restart or spin-down, and a service spins down after 15 minutes without traffic ([ADR 1](adr/0001-sqlite-with-an-ephemeral-demo.md)). Waking takes about a minute.
- **Start-up checks:** with `DEMO_MODE` on, start-up fails if any limit is `0` (unlimited) or `CLIENT_IP_HEADER` is unset. A forgotten setting would otherwise leave the public demo unlimited, or put every visitor behind one rate limit.
- **Reset:** at start-up, then every 24 hours from start-up, because steady traffic could stop the service spinning down. A failed timed reset is logged and the next one tried as usual; a failed start-up reset stops the app.
  - One transaction deletes every User and session, so cascades remove all Bookmarks, Tags and pending imports. It then recreates the Demo User and imports the seed through the real importer. Everyone is logged out.
  - The Demo User's ID is kept in memory after each reset, to recognise it.
- **Seed:** `internal/demo/seed.html` is an embedded browser bookmark file of 232 public links. Its Folders become about 30 Tags, and some links are starred, marked Read later, or have a description or notes. It stays below the limits of 300 Bookmarks and 300 Tags, leaving room for visitors.
  - At each reset the dates move forward, so the newest is the moment of the reset and the rest keep their spacing over the year before. The file keeps realistic fixed dates.
- **Demo User:** "Demo", `demo@example.com`, password `demo-password-1`. These are constants rather than secrets, because the home page shows them. `example.com` is reserved, so the address reaches nobody.
  - The logged-out home page has a "Try the demo" box with the login and a **Log in as Demo** button, and the login page shows the same above its form. The button is a form with hidden fields posting to `/user/login`, since the CSP allows no inline script, so it is rate limited like any login.
  - Its password can't be changed: the Account page explains why instead of offering the button, and both password routes answer 403. Token creation will be blocked the same way in step 8. Its theme, import, export, edits and deletes stay allowed, and the next reset undoes them.
- **Notices:** a banner on every page says the data is deleted every 24 hours and on restart, and links to the repository. The signup page says not to use a real password and that any email-shaped text works. The import page warns against uploading anything sensitive. When the 25 places are taken, the signup page says the demo is full and shows the Demo login.
- **HSTS:** every response sends `Strict-Transport-Security: max-age=31536000`, without `includeSubDomains`, which would cover all of `onrender.com`. Only demo mode sends it, since a self-hoster on plain HTTP would be locked out; they set it at their proxy.

### Rate limiting

Posts to log in, sign up and change a password are limited to 10 a minute from each client, as a burst or spread out. A client that goes over is refused for a whole minute. Otherwise one attempt would refill every 6 seconds, and someone retyping a password would never see the limit. bcrypt is deliberately slow, so a burst could otherwise stall a small instance, and guessing passwords becomes slow.

- Each route counts separately. Pages that show the forms are not limited.
- Over the limit, the form is shown again with "Too many attempts. Try again in a minute.", a 429 status and `Retry-After: 60`. What was typed is kept, apart from passwords, and the attempt is logged as a warning.
- The counts are in memory (`internal/ratelimit`, using `golang.org/x/time/rate`), so they are per server and start again on a restart. Clients that have refilled are forgotten, and at most 10,000 are tracked at once: past that, new clients are refused until some go idle. The handlers see only an `Allow(key)` interface, so a shared store can replace it ([scaling](scaling.md)).
- An IPv6 client is counted by its /64, because one home or server is usually given a whole /64.

**Client IP.** Without a proxy, the client is the connection's address. Behind one, every request comes from the proxy, so `CLIENT_IP_HEADER` names the header the proxy puts the client's address in. Forwarded headers are otherwise ignored, since any client can send them.

- `X-Forwarded-For` lists every hop, and only the last was added by the proxy in front of the app, so the last entry is used. The ones before it come from the client.
- Any other header, such as `True-Client-IP` on Render, must hold exactly one address.
- A missing or invalid value falls back to the connection's address, and the first one is logged as a warning, since it usually means the setting is wrong.
- The request log shows the same client IP.

### CI

GitHub Actions runs on pull requests and pushes to `main`, with two jobs:

- `go vet`, `staticcheck`, `govulncheck` and `go test -race`. govulncheck fails only on known vulnerabilities the code can reach.
- `scripts/smoke-test.sh`, which builds the real image and tests it through Docker Compose: health check, non-root user, saving data, restart, shutdown, backup and restore.

Images aren't published. Compose builds from the repository.

Dependabot (`.github/dependabot.yml`) opens weekly pull requests for Go modules, grouped into one, GitHub Actions, and the Dockerfile's base images.

### Security

[threat-model.md](threat-model.md) lists the threats and their defences, and [SECURITY.md](../SECURITY.md) says how to report a vulnerability, through GitHub's private vulnerability reporting.

- Passwords are bcrypt hashes at cost 12. bcrypt takes at most 72 bytes, so longer passwords are refused at signup and password change rather than cut short.
- An unknown email at login is checked against a dummy hash, so it takes as long as a wrong password.

### Logging

`log/slog` text logs on stdout, for all levels. `docker compose logs` is read by people, so there is no JSON option. Requests to `/ping` are skipped, because health checks would flood the log.

## Testing

- Handler tests in `cmd/web` use the model mocks and a test server.
- Model tests in `internal/models` run against a temporary SQLite file with real migrations and `testdata/setup.sql`. They need no set-up.
- `internal/bookmarkfile/testdata` holds real exports from Brave and Firefox, trimmed, with every link, title and personal Folder name replaced. Their layout, attributes, dates and Root Folders are as the browsers wrote them.
- Run everything with `make test`. `make smoke` tests the real image, and needs Docker.

## AI Tag suggestions (Possible later)

Kept behind an interface, so the feature can be switched off or mocked in tests.
