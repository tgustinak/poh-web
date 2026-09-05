#!/bin/sh
# Regenerate sqlc code only when inputs are newer than outputs.
#
# sqlc rewrites its output files on every run, even when nothing changed.
# Running it unconditionally in air's pre_cmd would retrigger a rebuild
# on every cycle (infinite loop), so generation is skipped when up to date.
set -eu

cd "$(dirname "$0")/.."

OUT="internal/db/db.go"

if [ ! -f "$OUT" ] || find db/sqlc.yaml db/schema.sql db/queries -type f -newer "$OUT" -print -quit | grep -q .; then
  go tool sqlc generate -f db/sqlc.yaml
else
  echo "sqlc: up to date"
fi
