# Wider use and scaling up

Notes for running BookmarkBox for many more Users than a household or the demo: signing in with Google or Microsoft, growing to 1,000 and then 1,000,000 Users, and the security work that comes with that. Nothing here is decided or planned yet. It records the options, and the few things worth doing earlier so that no option is closed off. Requirements are in [requirements.md](requirements.md) and the current design in [design.md](design.md).

## Prepare now

These cost little while the app is small, and are expensive to retrofit:

1. **Keep every query behind the models, scoped to one User.** This is already the rule. It keeps both a move to Postgres and a database per User possible. Avoid features that join across Users, apart from the Users table itself.
2. **Identify a User by their ID, never by their email.** OAuth accounts are identified by the provider's subject ID, and an email can change or be shared. Linking a sign-in to an existing account by email is only safe once emails are verified.
3. **Decide on encryption before relying on the no-cgo build.** Encrypting each SQLite file (SQLCipher or SQLite3MultipleCiphers) needs cgo, which `modernc.org/sqlite` was chosen to avoid. App-level field encryption or disk encryption don't need it. See [Confidentiality](#confidentiality).
4. **Add outbound email once, behind one interface.** Password reset, email verification and safe OAuth linking all need it. Self-hosters without email need another way, such as a command-line password reset.
5. **Keep shared state behind interfaces.** Sessions (the `scs` store), rate limits and the pending import are per-server today. More than one app server needs them in a shared store.
6. **Make background work a queue, not a goroutine.** Step 10's metadata fetch should record its jobs in the database, so that they survive a restart and can later move to separate workers.
7. **Share favicons between Users.** A favicon belongs to a site, not to a User, so step 10 should store each one once in a shared table keyed by host, rather than a copy on every Bookmark. Many Users saving the same site then need one fetch and one copy. Each User still sees only the favicons of their own Bookmarks. With a database per User, the shared table sits in the central database or in object storage behind a cache.
8. **Write the threat model before the public demo (step 7).** That is the first time strangers can reach the app. See [Hardening](#hardening-and-devsecops).

## Sign in with Google or Microsoft

- Use OpenID Connect (`golang.org/x/oauth2` with `coreos/go-oidc`), with the authorisation code flow, PKCE, `state` and `nonce`.
- Store identities in their own table, such as `user_identities (provider, subject, user_id)`, so that one User can have a password, Google and Microsoft. `users.hashed_password` becomes nullable.
- **Microsoft:** the multi-tenant endpoint returns tokens from any tenant. Check the issuer per tenant, and use `tid` with `oid` or `sub` as the identity, not the email.
- **Account linking:** only link a new provider to an existing account by email if the provider marks the email verified and the User proves they own the account, for example by logging in first. Otherwise an attacker with a matching unverified email could take the account over.
- The callback is a cross-site navigation, so the session cookie must stay `SameSite=Lax`, and the `state` must be checked.
- **Self-hosting:** each self-hoster registers their own client ID and secret with Google or Microsoft. It is off unless configured, and password login stays available.

## Growing to 1,000 Users

The current design copes. At about 1,000 Bookmarks each, that is a million rows and a database of a gigabyte or two, which SQLite handles easily on one small server. What changes is operations rather than code:

- Continuous backups with Litestream, and a tested restore.
- Rate limits on login and signup, which step 7 already needs.
- Password reset by email, and email verification.
- Monitoring and alerts: health, errors, disk space and backup age.
- Spam signups: a CAPTCHA or invitations, and the User limit.

## Growing to 1,000,000 Users

Throughput is not the main problem. If 5% of Users are active each day, making 20 requests each, that averages about 12 requests a second, which one server serves easily. The pressure comes from:

- **Data size:** about 500 Bookmarks each, with their search index, is roughly 500 GB in one file. Backups, restores and migrations of a file that size take hours.
- **One writer:** SQLite writes one transaction at a time. A large import holds the lock for a few seconds, so many imports at once queue up behind each other.
- **One machine:** an upgrade or a failure takes everyone offline.

The three options are:

| | Postgres | A SQLite file per User | Turso / libSQL |
|---|---|---|---|
| **Idea** | One shared database server, and several app servers | A central database for Users, identities and sessions, plus one file of Bookmarks and Tags per User | Managed SQLite, with a database per User |
| **For** | Well-trodden, with managed hosting, replicas and point-in-time recovery | Users are isolated by construction. Deleting a User deletes a file. Files are small, can be spread across servers by User ID, and can each have their own key. | Keeps the SQLite SQL. Per-User databases without running them yourself. |
| **Against** | Search moves to `tsvector`, and the triggers, `group_concat` and locking all need porting. One large shared dataset remains. | Migrations must run on every file, for example when it is opened. Needs a pool of open files, a way to route each User to their server, and backups of millions of files. Questions across Users need the central database. | A paid dependency, and the self-hosted version would differ from the hosted one. |

A database per User suits this app well, because no feature joins across Users. It is also the option that makes [per-User encryption](#confidentiality) and complete deletion simplest. An ADR should record the choice when the need is real. Until then, item 1 in [Prepare now](#prepare-now) keeps all three options open.

Program changes that every option needs at this size:

- Sessions, rate limits and pending imports in a shared store, such as the central database or Redis.
- Several app servers behind a load balancer, with no downtime for upgrades.
- Background jobs run by separate workers.
- Imports written in batches, so that they don't hold the write lock for long.
- An admin view, and audit logs of security events.

## Confidentiality

Bookmarks reveal a lot about a person, much as browsing history does. The levels of protection are:

1. **Disk encryption on the server.** This protects against stolen disks and is the baseline everywhere.
2. **Encrypted backups.** These protect copies held off the server.
3. **Server-held keys for each User (envelope encryption).** Each User's data is encrypted with their own key, and that key is encrypted with a master key held in a key management service. Destroying a User's key makes their data unreadable, even in old backups. Search and the metadata fetch still work, because the server can decrypt. With a SQLite file per User, the whole file can be encrypted (which needs cgo, as noted above).
4. **Keys held only by the User (end-to-end).** The server could never read Bookmarks. But server-side search, metadata fetching and the API would stop working, a forgotten password would lose everything without a recovery key, and OAuth Users have no password to derive a key from. This doesn't fit BookmarkBox's features.

Level 3 is the realistic aim at scale. Whoever runs the server can still read the data, and the privacy policy should say so plainly.

## Password reset and account security

- **Reset by email:** a random, single-use token, stored only as a hash, that expires in about an hour and is rate limited. The reply must be the same whether or not the email has an account. A reset ends the User's other sessions.
- **Self-hosters without email:** a command such as `bookmarkbox reset-password <email>`, run on the server.
- **Later:** two-step login (TOTP or passkeys), "log out everywhere", and checking new passwords against known breached ones.

## Hardening and DevSecOps

**Threat model.** Write one, for example with STRIDE, covering:
- what is worth protecting: Bookmarks, password hashes, sessions and API tokens
- who can reach the app: visitors, Users, the operator and dependencies
- the main threats:
  - account takeover by credential stuffing
  - one User reaching another's data (IDOR)
  - SSRF from the metadata fetch
  - stored XSS through imported titles: `html/template` escapes them, and import only accepts `http` and `https` links
  - large uploads, and denial of service
  - a compromised dependency or image

**In CI:**
- `govulncheck`, `staticcheck` and `gosec`
- Dependabot for Go modules, GitHub Actions and the base image
- image scanning, for example with Trivy, an SBOM, and signed images
- secrets never in the repository, checked with secret scanning

**At runtime:**
- the headers and CSP the app already sends, plus HSTS at the proxy
- a non-root container (already) with a read-only file system
- least privilege for the database files

**Process:**
- `security.txt` and a disclosure policy
- dependency updates on a schedule
- restore drills

## Other considerations

- **Privacy law (GDPR and similar):** a privacy policy, export (done), account deletion (planned), and limits on how long logs and backups are kept. A data processing agreement is needed with any hosting or email provider.
- **Email delivery:** a sending service, SPF, DKIM and DMARC, and handling bounces.
- **Abuse:** signup spam, and accounts used to store harmful links. Admin tools are needed to suspend accounts.
- **Cost and support:** hosting, email and backups cost money at this size, and a service people rely on needs someone to answer them.
