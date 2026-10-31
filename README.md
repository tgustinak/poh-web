# P.O.H - web

P.O.H Webpage project.

**Versions**:

- `go:1.27.0`
- `npm:11.16.0`
- `node:v24.18.0`

## Development

Start a development server:

```bash
make dev
```

Run test suite:

```bash
make test
```

Validate everything (templates, gofmt, vet, unit + integration tests):

```bash
make validate
```

Run build:

```bash
make build
```

## Deployment

The site runs on an server as a single static
Go binary behind the proxy (TLS + reverse proxy). Full VPS setup instructions are
in [`the internal setup notes`](the internal setup notes).

### One-time local setup

Add an SSH alias to `your ssh config` on your machine:

```
Host deploy-host
    HostName <VPS_IP>
    Port 22
    User <USER>
    IdentityFile <KEY_FILE>
    IdentitiesOnly yes
```

### Deploying

Build the Go binary + CSS and deploy to the VPS:

```bash
make deploy
```

This runs [`scripts/deploy.sh`](scripts/deploy.sh), which:
1. Builds Tailwind CSS
2. Cross-compiles the Go binary for Linux (`CGO_ENABLED=0`)
3. Copies the binary and static assets to the VPS
4. Restarts the `pohkapela` systemd service

The VPS-side deploy wrapper (`/usr/local/bin/deploy-wrapper`) is run via
passwordless sudo limited to that single script — see `the internal setup notes`.

### Production environment

The app reads `/opt/app/.env` on the VPS. Key production settings:

```
PORT=:8080
DATABASE_PATH=/var/lib/app/app.db
UPLOAD_DIR=/var/lib/app/uploads
LOG_LEVEL=info
BEHIND_PROXY=true
COOKIE_SECURE=always
STATIC_DIR=/opt/app/web/static
```

### Import concert history

The concerts from the old WordPress site are kept in
`db/imports/legacy_concerts.sql`. The import is idempotent, so it can be
run repeatedly without creating duplicates. Apply it to a database that
has already been migrated (start the app once, or use an existing
production database):

```bash
make import-concerts
# or with an explicit database path:
make import-concerts DATABASE_PATH=/var/lib/app/app.db
# or directly:
sqlite3 ./data/app.db < db/imports/legacy_concerts.sql
```

## Ideas

Some ideas we can work on

### Playlist Maker

Would be nice to have some mini app that would have these features:

- you can add and edit songs to database (song name, duration)
- you can create a new playlist and add songs to it
- you can reorder the songs in the playlist
- you can add "extra" songs to the playlist
- it would calculate the duration of the playlist
- you can export the playlist to PDF for print
