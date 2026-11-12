# Threat model

What BookmarkBox protects, who can reach it, the threats, and what defends against each. It covers the app and its container, self-hosted or as the public demo. Update it in the same pull request as any change that affects a threat.

Each threat has a status:
- **Mitigated:** the defence named is in place, and can be checked in the code or settings.
- **Accepted:** not defended, for the reason given.
- **Open:** to be done, with when.

Reviewed for step 7b, before the public demo, and updated for demo mode in step 7c ([roadmap](requirements.md#roadmap)).

## What is protected

- **Bookmarks**, with their notes and Tags. They are private to their Owner ([ADR 2](adr/0002-multi-user-with-private-bookmarks.md)).
- **Passwords**, stored only as bcrypt hashes.
- **Sessions**, which act as the User until they expire.
- **API tokens**, from step 8.
- **The server**: its CPU, memory and disk, and the host it runs on.

## Who can reach it

| Who | Can |
|---|---|
| Anonymous visitors | Load the public pages, and post to log in and sign up |
| Signed-up Users | Everything for their own Bookmarks, including uploading a bookmark file. On the demo, anyone can be one, and the Demo User's login is public |
| The operator | Run the container, read its logs, and copy the database file |
| Dependencies | Go modules, the base images, and the GitHub Actions in CI |

## Trust boundaries

```
browser ──HTTPS──► proxy (Caddy, nginx, Render) ──HTTP──► app ──► SQLite file on /data
```

- **Browser to proxy:** everything the browser sends is untrusted, including forwarded headers.
- **Proxy to app:** the app trusts one header from the proxy for the client's IP, and only when `CLIENT_IP_HEADER` names it.
- **App to SQLite:** the database is a file in the container's volume. Anyone who can read it has every Bookmark and password hash.

## Threats

### Spoofing: acting as someone else

| Threat | Defence | Status |
|---|---|---|
| Guessing passwords, or trying ones leaked from other sites | Login is limited to 10 attempts a minute per client, then blocked for a minute ([rate limiting](design.md#rate-limiting)). bcrypt at cost 12. Passwords need at least 8 characters | Mitigated |
| No second factor, and no check against known leaked passwords | A personal app: weighed in [scaling](scaling.md) for wider use | Accepted |
| Dodging the rate limit with a fake forwarded header | Forwarded headers are ignored unless `CLIENT_IP_HEADER` names one, and then only the last `X-Forwarded-For` hop counts. IPv6 clients count by their /64 | Mitigated |
| Stealing the session cookie | `HttpOnly`, `Secure` by default, `SameSite=Strict`, 12-hour lifetime. Scripts can't run inline (see XSS below) | Mitigated |
| `COOKIE_SECURE=false` on a home network sends the cookie over plain HTTP | Only for a network the self-hoster trusts; the docs say not to use it when reachable from the internet | Accepted |
| Session fixation | The session token is renewed at login and logout | Mitigated |
| A stolen session survives a password change | Sessions aren't tied to the User in the store, so a password change can't end the others | Open: with password reset ([scaling](scaling.md)) |
| A visitor changes the shared Demo User's password | Both password routes answer 403 for the Demo User, and the Account page offers no button | Mitigated |

### Tampering: changing what isn't yours

| Threat | Defence | Status |
|---|---|---|
| Another site posts a form as a logged-in User (CSRF) | Go's `http.CrossOriginProtection` refuses cross-origin posts, and the cookie is `SameSite=Strict` | Mitigated |
| SQL injection | Every value is a query parameter. Search words are quoted for FTS5 | Mitigated |
| Stored XSS through a title, notes, Tag or imported file | `html/template` escapes all output. Only `http` and `https` links are saved, from forms and import, so no `javascript:` links. The CSP is `default-src 'self'`, htmx's `eval` is off, and there are no inline scripts | Mitigated |
| Clickjacking | CSP `frame-ancestors 'none'` and `X-Frame-Options: deny` | Mitigated |
| Open redirect through a `next` or login target | Only paths on this site are followed (`safeNext`) | Mitigated |
| A crafted bookmark file | A streaming tokenizer, at most 20 MB, the same text and count limits as forms, and a preview before anything is saved. An unconfirmed import expires after an hour | Mitigated |
| A demo visitor changes or fills the shared Demo User's Bookmarks, or leaves something offensive for others to see | Allowed, so the demo shows everything. Every page says the data is wiped, and the next reset restores the seed | Accepted |
| An exported file that runs script when opened | Titles, URLs and Tags are HTML-escaped in the export | Mitigated |

### Repudiation: denying what was done

| Threat | Defence | Status |
|---|---|---|
| No record of who changed a Bookmark | The request log has each request's client IP, method and path. There is no per-User audit log, since Users only change their own data | Accepted |

### Information disclosure: seeing what isn't yours

| Threat | Defence | Status |
|---|---|---|
| Reading another User's Bookmark by changing an ID (IDOR) | Every Bookmark query filters on the Owner, and another User's Bookmark is a 404, the same as a missing one. Handler and model tests check it | Mitigated |
| Login timing shows which emails have an account | An unknown email is checked against a dummy hash, so it takes as long as a wrong password | Mitigated |
| Signup says "Email address is already in use" | Hiding it needs email confirmation, which doesn't exist yet. Signup is rate limited | Accepted, until email exists |
| Private pages kept in the browser or a shared cache | Logged-in pages send `Cache-Control: no-store`, and htmx keeps no page history | Mitigated |
| Opening a Bookmark tells its site which list or search it was opened from | `Referrer-Policy: origin-when-cross-origin` sends other sites only the origin | Mitigated |
| Stack traces shown to visitors | Only with the `-debug` flag, which has no environment variable | Mitigated |
| Searches appear in the request log, as part of the address | Logs stay with the operator. On the demo, Render keeps them | Accepted |
| A first visit over plain HTTP can be intercepted before the redirect to HTTPS | HSTS for a year on the demo, without `includeSubDomains`. Self-hosters set it at their proxy ([self-hosting](self-hosting.md#https-with-a-reverse-proxy)) | Mitigated |
| The database file and backups aren't encrypted | They stay on the operator's machine, and passwords are bcrypt hashes | Accepted |

### Denial of service: making it unusable

| Threat | Defence | Status |
|---|---|---|
| A flood of logins or signups, each costing a bcrypt hash | The rate limit, per route and client | Mitigated |
| Many clients filling the rate limiter's memory | At most 10,000 clients are tracked; past that, new ones wait | Mitigated |
| Large or slow requests | Forms are limited to 1 MB and imports to 20 MB. The server's read timeout is 5 seconds, write 10, idle 60 | Mitigated |
| Filling the disk | Limits on Users, Bookmarks per User and Tags per User, and on every text field. Bookmarks and Tags are unlimited by default when self-hosted, where every User is trusted | Mitigated |
| Expensive list queries | At most 10 Tag filters, searches of 200 characters, and a cap on search words | Mitigated |
| The demo deployed with a limit left unlimited, or with every visitor sharing one rate limit | With `DEMO_MODE` on, start-up fails if a limit is `0` or `CLIENT_IP_HEADER` is unset | Mitigated |
| A self-hoster turns on `DEMO_MODE` and loses their data | Off by default, and [self-hosting](self-hosting.md#settings) says to leave it unset | Mitigated |
| Bots taking all 25 demo signups | The Demo login still works, and the data resets every day. A full demo's signup page says so and shows the Demo login | Accepted |

### Elevation of privilege: getting more access

| Threat | Defence | Status |
|---|---|---|
| A bug in the app becomes control of the host | The container runs as a non-root user, with one volume | Mitigated |
| The same, with a writable container file system | A read-only root file system, with a tmpfs for `/tmp` | Open: with a later self-hosting update |
| A compromised or vulnerable dependency | Few dependencies, all checked by `go.sum`. `govulncheck` in CI, Dependabot weekly for Go modules, Actions and base images, and pinned versions | Mitigated |
| A tampered image | Images are built from the repository, not downloaded. Scanning, an SBOM and signing are for a published image ([scaling](scaling.md)) | Accepted while images aren't published |
| The metadata fetch reaching internal addresses (SSRF) | Private and loopback addresses refused after DNS lookup, including on redirects ([design](design.md#background-metadata-planned)) | Open: step 10 |

## Open items

| Item | When |
|---|---|
| End a User's other sessions when their password changes | With password reset |
| Hide whether an email has an account at signup | With email confirmation |
| Read-only container file system | A later self-hosting update |
| SSRF defences for the metadata fetch | Step 10 |

## Checks

- **CI** runs `go vet`, staticcheck, `govulncheck` and the tests with the race detector on every pull request, and the smoke test builds and exercises the real image.
- **Dependabot** opens weekly pull requests for Go modules, GitHub Actions and the Dockerfile's base images.
- **Reporting:** [SECURITY.md](../SECURITY.md) asks for reports through GitHub's private vulnerability reporting.
