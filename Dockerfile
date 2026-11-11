# syntax=docker/dockerfile:1

# build css
FROM node:22-bookworm-slim AS css
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci
COPY web ./web
RUN npm run css:build


# build go app
FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=css /src/web/static/css/output.css web/static/css/output.css
RUN go tool templ generate && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/main.go


# runtime
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /srv
COPY --from=build /out/app /srv/app
COPY --from=build /src/web/static /srv/web/static
ENV PORT=:8080 DATABASE_PATH=/data/app.db UPLOAD_DIR=/data/uploads LOG_LEVEL=info FORCE_SHUTDOWN_AFTER_IN_SECONDS=10 BEHIND_PROXY=true COOKIE_SECURE=always
VOLUME ["/data"]
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/srv/app"]
