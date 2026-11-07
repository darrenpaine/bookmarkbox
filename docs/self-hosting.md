# Self-hosting BookmarkBox

BookmarkBox runs as one container with its data on a named Docker volume. You need Docker with the Compose plugin; there's no database server to set up.

## Install

```bash
git clone https://github.com/darrenpaine/bookmarkbox.git
cd bookmarkbox
cp .env.example .env
docker compose up -d --build
```

Open <http://localhost:4000> and sign up. There's no default account, and signup closes after the first User unless you raise `MAX_USERS`.

Safari may refuse the secure session cookie on `http://localhost`. If logging in just returns you to the login page, use another browser, a proxy, or the home network settings below.

## Settings

Settings go in `.env`, and `.env.example` lists them all.

| Variable | Default | Purpose |
|---|---|---|
| `MAX_USERS` | `1` | Signup closes at this many Users; `0` is unlimited |
| `MAX_BOOKMARKS_PER_USER` | `0` | `0` is unlimited |
| `MAX_TAGS_PER_USER` | `0` | `0` is unlimited |
| `COOKIE_SECURE` | `true` | Send the session cookie over HTTPS only |
| `HOST_PORT` | `4000` | Published port on the host |
| `BIND_ADDRESS` | `127.0.0.1` | Host address the port is published on |

An invalid value stops the app at start-up, and `docker compose logs bookmarkbox` shows why. After changing `.env`, run `docker compose up -d`.

## Home network

For plain HTTP at `http://<server-address>:4000`, set both of these in `.env`:

```bash
BIND_ADDRESS=0.0.0.0
COOKIE_SECURE=false
```

If you leave `COOKIE_SECURE` on, the browser drops the session cookie over HTTP, so logging in returns to the login page with no error.

## HTTPS with a reverse proxy

BookmarkBox serves plain HTTP and leaves HTTPS to your proxy. With [Caddy](https://caddyserver.com/) on the same host, this `Caddyfile` is enough, and Caddy gets the certificate itself:

```
bookmarks.example.com {
    reverse_proxy 127.0.0.1:4000
}
```

Keep the default `BIND_ADDRESS` and `COOKIE_SECURE`. Any other proxy must pass on the original `Host` header, or form posts can be refused as cross-origin. With nginx, that means `proxy_set_header Host $host;`.

> **Before exposing it to the internet:** login and signup aren't rate limited yet. Use a strong password, and keep `MAX_USERS` at the number of people who need an account.

## Back up

Copying the database file from a running app isn't safe, so use SQLite's backup command, then copy the result to the host:

```bash
docker compose exec bookmarkbox sqlite3 /data/bookmarkbox.db ".backup /data/backup.db"
docker compose cp bookmarkbox:/data/backup.db ./bookmarkbox-$(date +%F).db
```

Keep backups private, because they contain all Bookmarks and password hashes. A backup left only on the volume would be lost with the volume.

BookmarkBox doesn't schedule backups. To run one nightly, create a `backups` folder, then add this to `crontab -e`, changing the path:

```
0 3 * * * cd /path/to/bookmarkbox && docker compose exec -T bookmarkbox sqlite3 /data/bookmarkbox.db ".backup /data/backup.db" && docker compose cp bookmarkbox:/data/backup.db ./backups/bookmarkbox-$(date +\%F).db
```

## Restore

This replaces all current data. Run it from the repository folder, changing the backup's file name in the second command:

```bash
docker compose stop
docker compose run --rm --no-deps -v "$PWD/bookmarkbox-2026-11-06.db:/restore.db:ro" --entrypoint sh bookmarkbox \
  -c 'rm -f /data/bookmarkbox.db-wal /data/bookmarkbox.db-shm && cp /restore.db /data/bookmarkbox.db'
docker compose up -d
```

The `rm` matters: a leftover write-ahead log from a crash must not be applied to the restored file.

## Upgrade

Back up first, then:

```bash
git pull
docker compose up -d --build
```

Database migrations run at start-up.

## Remove

`docker compose down` removes the container and keeps your data. `docker compose down -v` also deletes the volume, and all your Bookmarks with it.
