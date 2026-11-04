# Browser Folders become flat Tags

On import, each Folder in a Bookmark's path becomes its own flat Tag, so `Programming/Go` gives `programming` and `go`. Tags are flat throughout BookmarkBox: hierarchy would need path-aware filtering, display and export, while most people look for "go" rather than a position in a tree. The cost is accepted: `Work/Go` and `Programming/Go` share the Tag `go`, and the original Folder structure cannot be rebuilt.

## Considered Options

- **One hierarchical Tag, `programming/go`:** keeps the structure, but makes `/` meaningful in every Tag feature.

## Consequences

- Export writes a flat list with a `TAGS` attribute rather than Folders, because a Bookmark with several Tags has no single place in a tree.
- A BookmarkBox round trip is lossless, but nothing relies on browsers keeping Tags.
- Browser root Folders such as "Bookmarks bar" are skipped, and commas in Folder names are replaced, because Tags are stored comma-separated when read back.
