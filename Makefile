
# Uses the default SQLite file ./bookmarkbox.db (created and migrated on start)
# Signup is left unlimited for development; the app's default is one user.
# Local development uses plain HTTP, so the session cookie can't be HTTPS-only.
run:
	go run ./cmd/web -addr :4000 -max-users 0 -cookie-secure=false

test:
	go test ./...

# Tests the real image; needs Docker.
smoke:
	./scripts/smoke-test.sh
