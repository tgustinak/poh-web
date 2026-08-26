.PHONY: build dev test

build:
	go build -o bin/app ./cmd/main.go

dev:
	go run ./cmd/main.go

test:
	go test ./...
