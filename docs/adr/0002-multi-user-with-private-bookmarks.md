# Multi-user with private Bookmarks and no sharing

BookmarkBox has accounts, and every Bookmark and Tag belongs to one User, even though most self-hosted installs will have just one. The public demo needs separate visitors, and self-hosters may want to add family. Sharing and public Bookmarks are deliberately excluded, which keeps authorization to one rule: filter by Owner, and treat anything else as not found. Signup is bounded by `MAX_USERS`, which defaults to 1, so a self-hosted site on the internet is not open to strangers.

## Considered Options

- **Single user, with no signup:** simpler, but it can't support the demo's visitors or family use.
- **Sharing or public Bookmarks:** would need per-Bookmark permissions, and would replace the single Owner rule everywhere.
