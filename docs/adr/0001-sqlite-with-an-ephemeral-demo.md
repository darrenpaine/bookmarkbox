# SQLite everywhere, with an ephemeral demo

BookmarkBox uses real SQLite through `modernc.org/sqlite` in development, tests, self-hosting and the demo. The public demo runs on Render's free tier with a local file that is lost on every spin-down, so it is reseeded at start-up. Self-hosting is the permanent home for real Bookmarks, and SQLite means one container and a volume, with no database server. The demo costs nothing and needs no second database service.

## Considered Options

- **Turso with `tursogo`:** tested 2026-11-04 on v0.8.3. It has no FTS5, and Turso's own experimental search failed in the published build. Foreign keys were off with no documented connection option to enable them, so deletes left orphaned rows silently, and `group_concat(... ORDER BY ...)` was rejected. It is a pre-1.0 rewrite rather than SQLite. Writes are local until pushed, which risks losing them on a spin-down.
- **Turso with `go-libsql`:** needs cgo, losing the pure-Go build.
- **`libsql-client-go`:** its repository is marked deprecated.
- **Postgres on Neon:** a second cold start, and local development would need Postgres or Docker.
- **Fly.io:** no free tier for new accounts.
- **Paid Render disk, or a VPS with Litestream:** both keep data, but the demo doesn't need that. If real Bookmarks are ever hosted, a VPS with Litestream is the simplest next step, with almost no code change.

## Consequences

- The app runs as a single instance, because one SQLite file cannot be shared across hosts.
- Nothing on the demo is permanent, and visitors must be told so.
- Search can rely on FTS5, which is confirmed working with this driver.
