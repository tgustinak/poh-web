# syntax=docker/dockerfile:1
FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN apt-get update && apt-get install -y --no-install-recommends nodejs npm \
 && npm ci \
 && go run github.com/a-h/templ/cmd/templ@v0.3.1020 generate \
 && npm run css:build \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/main.go

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /srv
COPY --from=build /out/app /srv/app
COPY --from=build /src/web/static /srv/web/static
ENV PORT=:8080 DATABASE_PATH=/data/app.db UPLOAD_DIR=/data/uploads LOG_LEVEL=info FORCE_SHUTDOWN_AFTER_IN_SECONDS=10 BEHIND_PROXY=true COOKIE_SECURE=always
VOLUME ["/data"]
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/srv/app"]
