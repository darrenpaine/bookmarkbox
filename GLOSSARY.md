# BookmarkBox

A personal bookmark manager: each person keeps their own private, tagged collection of links.

## Language

### People

**User**:
A person with their own account and private collection of Bookmarks.

**Owner**:
The User a Bookmark or Tag belongs to. Nobody else can see or change it.

**Demo User**:
The shared, pre-filled User that exists only on the public demo, with its login shown to every visitor.
_Avoid_: Default user, guest

### Bookmarks

**Bookmark**:
A saved link, owned by one User.
_Avoid_: Link, snippet, item

**Duplicate**:
A second Bookmark from the same Owner whose URL matches an existing one once both are normalised. An Owner can't have Duplicates, but two Users can save the same URL.

**Tag**:
A User's own label for grouping Bookmarks. A Bookmark can have many Tags, and Tags are flat, with no parents or children. A Tag exists only while at least one Bookmark uses it.
_Avoid_: Label, category, folder

**Favourite**:
The state of a Bookmark its Owner has marked as special. The star is only the control that sets it.
_Avoid_: Star, starred, bookmarked

**Read later**:
A flag on a Bookmark its Owner means to come back to. It is independent of Favourite.

### Browsing

**List view**:
A way of displaying a set of Bookmarks: Compact, Cards or By Tag. It changes how Bookmarks look, never which ones are shown.
_Avoid_: View (on its own), layout

**Filter**:
A condition that narrows which Bookmarks a list shows, such as Favourite, Read later, a Tag, a domain or a search. Filters combine, and any List view can show any combination of them.
_Avoid_: View, smart folder

**Saved view**:
A named combination of filters that a User can return to.
_Avoid_: Smart folder, preset

### Browsers

**Folder**:
A container in a browser's own bookmarks. Folders exist only in browsers and never inside BookmarkBox; on import they become Tags.
