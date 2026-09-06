.PHONY: build dev test validate sqlc templ seed db-reset

build:
	go tool templ generate
	npm run css:build
	go build -o bin/app ./cmd/main.go

dev:
	@go tool templ generate
	@npm run css:build
	@trap 'kill 0' INT TERM EXIT; \
	go tool templ generate --watch & \
	npm run css:watch & \
	go tool air & \
	wait

test:
	go test ./...

# Validate everything: templates compile, formatting is clean,
# vet passes and the full test suite (unit + integration) is green.
validate:
	go tool templ generate
	test -z "$$(gofmt -l cmd db internal web)" || (echo "Unformatted Go files:" && gofmt -l cmd db internal web && exit 1)
	go vet ./...
	go test ./...

templ:
	go tool templ generate

sqlc:
	go tool sqlc generate -f db/sqlc.yaml

seed:
	go run ./cmd/seed

# Delete the local dev database (migrations re-run on next boot).
# Dev only: never run against production data.
db-reset:
	rm -f ./data/app.db*
