
# Uses the default SQLite file ./bookmarkbox.db (created and migrated on start)
# Signup is left unlimited for development; the app's default is one user.
run:
	go run ./cmd/web -addr :4000 -max-users 0

test:
	go test ./...
