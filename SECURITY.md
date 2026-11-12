# Security

## Reporting a vulnerability

Please report it privately through GitHub: on this repository's **Security** tab, choose **Report a vulnerability**. Only the maintainer sees the report. Please don't open a public issue.

Include what you found, how to reproduce it, and what someone could do with it. You should get a reply within a week. Once a fix is released, the report can be made public, with credit if you'd like it.

## In scope

- The BookmarkBox application and its container image, as built from this repository.
- The public demo, for problems in BookmarkBox itself. Please don't test by flooding it, using other visitors' accounts, or trying to break out of the container. Its data is deleted every day anyway.

Out of scope: Render's platform, problems that need the server's host or its reverse proxy to be compromised first, and reports from automated scanners without a way to reproduce them.

## Supported versions

Only the latest commit on `main` is supported. Self-hosters update by pulling and rebuilding, as described in [self-hosting](docs/self-hosting.md#upgrade).

The [threat model](docs/threat-model.md) lists what BookmarkBox defends against, and what it doesn't yet.
