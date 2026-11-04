
# Uses the default SQLite file ./bookmarkbox.db (created and migrated on start)
run:
	go run ./cmd/web -addr :4000

test:
	go test ./...
