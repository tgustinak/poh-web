.PHONY: build dev test validate sqlc templ seed db-reset import-concerts deploy

DATABASE_PATH ?= ./data/app.db

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

# Import the concert history from the old WordPress site.
# Idempotent, safe to run repeatedly. Requires the sqlite3 CLI.
import-concerts:
	sqlite3 "$(DATABASE_PATH)" < db/imports/legacy_concerts.sql

# Delete the local dev database (migrations re-run on next boot).
# Dev only: never run against production data.
db-reset:
	rm -f ./data/app.db*

# Deploy to the VPS. Requires an SSH alias 'deploy-host' in your ssh config
# and the VPS deploy wrapper (/usr/local/bin/deploy-wrapper) set up.
# See the internal setup notes for full instructions.
deploy:
	./scripts/deploy.sh
