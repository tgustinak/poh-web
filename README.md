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

Deployment is done with `make deploy`, which runs `scripts/deploy.sh`. The
target host and paths are read from a local, git-ignored `.deploy.env` file
(see the variables listed at the top of the script). Production configuration
is supplied through environment variables (see `.env.example`).

### Import concert history

The concerts from the old WordPress site are kept in
`db/imports/legacy_concerts.sql`. The import is idempotent, so it can be
run repeatedly without creating duplicates. Apply it to a database that
has already been migrated (start the app once, or use an existing
production database):

```bash
make import-concerts
# or with an explicit database path:
make import-concerts DATABASE_PATH=/path/to/app.db
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
