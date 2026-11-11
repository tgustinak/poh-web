#!/usr/bin/env bash
#
# Simple deploy script (run from your workstation).
#
# Builds the Go binary + CSS locally, uploads them and runs the remote
# deploy command. All environment-specific values come from a local,
# git-ignored `.deploy.env` file (or the environment). Required variables:
#   DEPLOY_HOST        SSH destination (e.g. an alias from your ssh config)
#   DEPLOY_REMOTE_CMD  command executed on the server to install + restart
#   DEPLOY_BIN_TMP     remote temp path for the uploaded binary
#   DEPLOY_STATIC_TMP  remote temp path for the uploaded static assets
# Optional:
#   DEPLOY_TEST_URL    URL printed at the end for a manual smoke test
#
set -euo pipefail

[[ -f .deploy.env ]] && source .deploy.env

: "${DEPLOY_HOST:?DEPLOY_HOST is not set (see .deploy.env)}"
: "${DEPLOY_REMOTE_CMD:?DEPLOY_REMOTE_CMD is not set}"
: "${DEPLOY_BIN_TMP:?DEPLOY_BIN_TMP is not set}"
: "${DEPLOY_STATIC_TMP:?DEPLOY_STATIC_TMP is not set}"
LOCAL_STATIC_DIR="web/static"

info() { echo "[deploy] $*"; }
err() {
  echo "[deploy] ERROR: $*" >&2
  exit 1
}

info "Building CSS..."
npm run css:build

info "Building Go binary for Linux..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -trimpath -ldflags="-s -w" \
  -o bin/app ./cmd/main.go

[[ -f bin/app ]] || err "Go build failed — bin/app not found"

info "Uploading binary..."
scp bin/app "${DEPLOY_HOST}:${DEPLOY_BIN_TMP}"

if [[ -d "${LOCAL_STATIC_DIR}" ]]; then
  info "Uploading static assets..."
  scp -r "${LOCAL_STATIC_DIR}/." "${DEPLOY_HOST}:${DEPLOY_STATIC_TMP}"
fi

info "Running remote deploy..."
ssh "${DEPLOY_HOST}" "${DEPLOY_REMOTE_CMD}"

info "Deploy complete."
[[ -n "${DEPLOY_TEST_URL:-}" ]] && info "Test: curl -I ${DEPLOY_TEST_URL}"
exit 0
