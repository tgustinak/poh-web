.PHONY: build dev test

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
