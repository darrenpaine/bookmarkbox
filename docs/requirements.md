# BookmarkBox requirements

What BookmarkBox must do and for whom. How it does it is in [design.md](design.md), the reasons behind the major decisions are in [adr/](adr/), and the terms used here are defined in [GLOSSARY.md](../GLOSSARY.md).

## Purpose

BookmarkBox is a personal bookmark manager in Go, with Tags and Favourites. It has two aims:

- Be useful day to day, self-hosted.
- Show my own work. It began as the Snippetbox example from *Let's Go* by Alex Edwards, which the README credits, followed by a list of what changed.

## Users and scope

- BookmarkBox is multi-user. Each User's Bookmarks are private to them, and another User's Bookmark must behave as if it does not exist.
- The two ways of running it are **self-hosted** and the **public demo**.

**Out of scope:** sharing Bookmarks, public Bookmarks, and teams or groups.

## Features

### Bookmarks and Tags

- A User can add, view, update and delete their Bookmarks, and mark them as Favourite or Read later.
- Updating a Bookmark can change any of its details, including its URL and Tags. Changing the URL to match another of the Owner's Bookmarks is rejected as a Duplicate.
- A Bookmark has a URL, title, description, notes, Tags and a favicon.
- An address can be typed without `https://`, as in `bbc.co.uk`, and a mistyped `https:/` is mended.
- A Bookmark can have many Tags. Tag names are trimmed and lower-cased.
- A Tag exists only while at least one Bookmark uses it. It is removed automatically when its last Bookmark is deleted or stops using it, so there is no separate delete action for Tags.
- The Tags field on the add and edit forms, and the Tag editor on a Bookmark's page, suggest the User's existing Tags as they type.
- A User can rename a Tag, which updates every Bookmark using it. Renaming to a Tag the User already has merges the two, after a confirmation.
- An Owner cannot have Duplicates. Two Users can save the same URL.
- The form, import and API all apply the same definition of a Duplicate.

### Look and feel

- Own visual identity, replacing the book's CSS: logo or wordmark, app favicon, own colours and a dense layout.
- A theme of light, dark or system, chosen on the Account page and kept with the User's account. System follows the device, and so do pages for logged-out visitors.
- Favicons beside links, coloured Tag chips and a star control.
- In-place actions without a full page reload: set Favourite or Read later, add or remove a Tag, delete after a confirmation, live search. Every action still works without JavaScript.
- A proper empty state, such as "No bookmarks yet", with a link to import from a browser.
- Works on a phone: the Add button stays on screen while scrolling, and every control is large enough to tap. The Account link is the User's initial in the top bar.
- A Bookmark's own page has a Back button beside Edit and Delete. It returns to the list last looked at, with its Filters and search, and deleting the Bookmark there returns to the same list.

### List views and search

A list shows the User's Bookmarks in one List view, narrowed by any combination of Filters, in one sort.

| List view | Shows |
|---|---|
| Compact | Favicon, title, domain and Tags; dense, for scanning |
| Cards | Adds the description, or the notes when there is none. A thumbnail comes with page metadata. |
| By Tag | Compact rows beside a Tag panel with counts |

| Filter | Shows |
|---|---|
| Favourite | Favourite Bookmarks only |
| Read later | Bookmarks marked Read later only |
| Tag | Bookmarks with every chosen Tag |
| Domain | Bookmarks from one domain |
| Search | Matches on title, description, URL, notes and Tags |

- Sort by newest, oldest, A-Z or domain. "Recent" is the newest-first sort.
- A search finds Bookmarks containing every word typed, including words that start with it, and shows the best matches first. Choosing another sort shows every match in that order. Results update as you type on the list page.
- Clicking a Tag or domain on a Bookmark filters the list by it.
- Long lists load 50 at a time with "Show more". Deleting or changing Bookmarks already shown never makes it skip one.
- The last List view and sort are remembered for the session. Filters are not.
- Later: Saved views.

### Import and export

- Import the bookmark file exported by Chrome, Edge, Firefox or Brave.
- Each Folder in a Bookmark's path becomes a flat Tag: `Programming/Go` gives `programming` and `go`. Root Folders such as "Bookmarks bar" do not become Tags.
- A new Bookmark directly in a Root Folder, with no Folder of its own, is tagged Unfiled. The preview says how many.
- A Bookmark already saved keeps everything, and only gains any new Tags.
- The original date added is kept.
- A preview comes before saving, such as "312 bookmarks, 18 duplicates, 24 folders -> tags. Import?"
- An import is all or nothing. If it would pass the User's Bookmark or Tag limit, the preview says so and nothing is saved.
- Export produces a file in the same format.
- A BookmarkBox export imported back into BookmarkBox loses nothing. Folder structure does not survive export, and browsers are only expected to keep the links (see [ADR 4](adr/0004-folders-become-flat-tags.md)).

### JSON API

- Create a Bookmark, list Tags, check whether a URL is already saved, and bulk import.
- Personal API tokens are created on a settings page and can be used in place of a login.

### Browser extension

- Works in Chrome, Edge and Brave. Firefox needs small changes.
- The toolbar popup is pre-filled with the current tab's title and URL, with Tag autocomplete, Favourite and save.
- A right-click option, "Save link to BookmarkBox".
- Direct import from the browser's bookmarks, with no file export. HTML upload stays available for anyone without the extension.
- The User enters their server address, and the extension gets access only to that address.

### Page metadata

- Titles, descriptions and favicons are fetched in the background after saving, so saving never waits for them.

### Possible later

- Suggest Tags for a new Bookmark from its title and description, using the User's existing Tags. It must be possible to switch this off.
- Saved views.
- Delete account, under a "Dangerous actions" heading on the Account page.
- Bulk changes: select several Bookmarks, then delete them, add or remove a Tag, or edit them together. How this should work is still to be explored.
- Continuous backup with Litestream.

### Wider use (Possible later)

Options and early preparations for these are in [scaling.md](scaling.md).

- Sign up and log in with a Google or Microsoft account, alongside passwords. Self-hosters can turn it on with their own client ID.
- Reset a forgotten password by email, with a command-line reset for self-hosters without email.
- Scale to 1,000 and then 1,000,000 Users. This may mean Postgres, or a central database for Users with a SQLite file per User.
- Keep Bookmarks confidential at scale, for example by encrypting each User's data with its own key.
- A written threat model, with security checks in CI, before the public demo. Done in step 7b: [threat model](threat-model.md). Keep it up to date as features are added.
- Upgrades that don't surprise logged-in Users: notice of planned downtime as a banner and on login, a maintenance page during it, and upgrades without downtime at scale.

## Self-hosting

- Runs with Docker Compose as one service, with data on a named volume. No separate database server.
- Settings come from environment variables, and secrets never appear in the compose file.
- HTTPS comes from the self-hoster's own reverse proxy. The docs include a proxy example, and plain HTTP on a home network is supported.
- A fresh install allows one User. A self-hoster can raise the limit, for example for family.
- There is no default account.
- The docs give safe backup and restore commands.

## Public demo

- A live demo at no cost, on a free hosting tier. The README states that the first visit after idle takes about a minute.
- A shared Demo User with about 200 seeded Bookmarks of public links, not personal ones. Its login is shown on the home page.
- Visitors can sign up too, with a clear notice that all demo data gets wiped.
- Limits: 25 Users including the Demo User, 300 Bookmarks per User and 300 Tags per User.
- The Demo User cannot change its password or create API tokens. Users who sign up have no such restriction.
- All data resets at start-up and every 24 hours: Users, Bookmarks and sessions are removed, then the Demo User and seed data are recreated.

## Limits and safety

- A configurable maximum number of Users, of Bookmarks per User and of Tags per User. Limits hold even when two requests arrive together.
- When signup is full, the Signup link is hidden and the signup page says it is closed.
- Text fields are limited: URL and description to 2,000 characters, notes to 10,000, title to 200, Tag name to 50.
- Form posts are limited to 1 MB.
- Login, signup and password change are rate limited per client IP address, to 10 attempts a minute each. The client IP is read from a proxy's header only when the self-hoster names that header.
- Metadata fetching does not reach private or loopback addresses by default. Self-hosters can opt out; the demo cannot.
- Requests from other sites cannot act on a logged-in User's behalf.

## Roadmap

| # | Step | Status |
|---|---|---|
| 1 | **Foundations:** rename, SQLite, migrations, Bookmarks and Tags | Done |
| 2 | **Model fixes and editing:** update Bookmarks, rename Tags, Duplicates by normalised URL, remove unused Tags, text limits, User, Bookmark and Tag limits | Done |
| 3 | **Packaging:** environment config, `slog`, Dockerfile, compose, CI | Done |
| 4 | **Restyle:** own CSS, dark mode, htmx actions | Done |
| 5 | **List views and search,** with "Show more" for long lists | Done |
| 6 | **Import and export,** with round-trip tests | Done |
| 6a | **Theme switch:** dark, light or system on the Account page, and an Account icon in the nav on small screens | Done |
| 6b | **Tag autocomplete** on the add and edit forms | Done |
| 7 | **Public demo** on Render; needs step 6 for its seed data | Done |
| 7a | **Rate limiting** on login, signup and password change, with the client IP from a trusted proxy | Done |
| 7b | **Threat model,** with security checks in CI | Done |
| 7c | **Demo mode:** reset, seed data, the Demo User and notices | Done |
| 7d | **Deploy** to Render | Done |
| 8 | **JSON API and tokens** | Planned |
| 9 | **Browser extension:** popup, then right-click, then direct import | Planned |
| 10 | **Background metadata and favicons** | Planned |

