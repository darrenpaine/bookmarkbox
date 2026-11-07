#!/usr/bin/env bash
# Tests the real image with Docker Compose: start-up, health check, non-root
# user, saving data, clean shutdown, and the documented backup and restore.
# Uses its own project and volume, so an existing install isn't touched.
set -euo pipefail
cd "$(dirname "$0")/.."

# Nothing here reads input. Without this, "docker compose exec -T" run from a
# terminal can wait on stdin and never return.
exec </dev/null

export COMPOSE_PROJECT_NAME=bookmarkbox-smoke
export HOST_PORT=14000
url="http://127.0.0.1:$HOST_PORT"

workdir=$(mktemp -d -p .)
chmod 755 "$workdir" # the restore reads from here as the container's user

cleanup() {
    status=$?
    if [ "$status" -ne 0 ]; then
        docker compose logs --no-color || true
    fi
    docker compose down -v --remove-orphans || true
    rm -rf "$workdir"
    exit "$status"
}
trap cleanup EXIT

fail() {
    echo "FAIL: $1" >&2
    exit 1
}
sql() {
    docker compose exec -T bookmarkbox sqlite3 /data/bookmarkbox.db "$1"
}
expect() { # description, actual, expected
    if [ "$2" != "$3" ]; then
        fail "$1: got '$2', want '$3'"
    fi
    echo "ok: $1"
}

docker compose up -d --build --wait

expect "health check" "$(curl -sS "$url/ping")" "OK"
expect "runs as a non-root user" "$(docker compose exec -T bookmarkbox id -u)" "10001"

status=$(curl -sS -o /dev/null -w '%{http_code}' \
    -d 'name=Smoke&email=smoke@example.com&password=smoke-test-password' "$url/user/signup")
expect "signup" "$status" "303"
expect "user saved" "$(sql 'SELECT count(*) FROM users')" "1"

docker compose restart bookmarkbox
docker compose up -d --wait
expect "data survives a restart" "$(sql 'SELECT count(*) FROM users')" "1"

# The documented backup commands.
docker compose exec -T bookmarkbox sqlite3 /data/bookmarkbox.db ".backup /data/backup.db"
docker compose cp bookmarkbox:/data/backup.db "$workdir/backup.db"
expect "backup is intact" \
    "$(docker compose exec -T bookmarkbox sqlite3 /data/backup.db 'PRAGMA integrity_check')" "ok"

sql 'DELETE FROM users'
expect "user removed before restoring" "$(sql 'SELECT count(*) FROM users')" "0"

# Docker kills the container after 10 seconds, so a slower stop means SIGTERM
# wasn't handled.
started=$SECONDS
docker compose stop bookmarkbox
if [ $((SECONDS - started)) -ge 9 ]; then
    fail "stop took $((SECONDS - started)) seconds"
fi
echo "ok: stopped cleanly"

in_container() {
    docker compose run -T --rm --no-deps --entrypoint sh "$@"
}
files=$(in_container bookmarkbox -c 'ls /data')
case "$files" in
*-wal*) fail "write-ahead log left after stopping: $files" ;;
esac
echo "ok: write-ahead log checkpointed"

# The documented restore commands.
in_container -v "$PWD/$workdir/backup.db:/restore.db:ro" bookmarkbox -c \
    'rm -f /data/bookmarkbox.db-wal /data/bookmarkbox.db-shm && cp /restore.db /data/bookmarkbox.db'
docker compose up -d --wait

expect "restored user" "$(sql 'SELECT email FROM users')" "smoke@example.com"
sql "UPDATE users SET name = 'Restored'"
expect "restored database is writable" "$(sql 'SELECT name FROM users')" "Restored"

echo "All smoke tests passed"
