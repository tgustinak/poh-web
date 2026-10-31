#!/usr/bin/env bash
#
# pohkapela.sk — simple deploy script (run from your Mac)
#
# Builds the Go binary + CSS locally, copies to the VPS, restarts the service.
#
# Prerequisites:
#   - SSH alias 'deploy-host' in your ssh config pointing to your VPS
#   - Go toolchain and Node.js/npm installed locally
#   - On VPS: /usr/local/bin/deploy-wrapper wrapper + sudoers rule
#
# Usage:
#   chmod +x scripts/deploy.sh
#   ./scripts/deploy.sh
#
set -euo pipefail

###############################################################################
# Configuration
###############################################################################
VPS_HOST="deploy-host" # must match a Host entry in your ssh config
REMOTE_BIN_TMP="/tmp/pohkapela-app"
REMOTE_STATIC_TMP="~/pohkapela-static"
REMOTE_BIN="/opt/app/app"
REMOTE_STATIC_DIR="/opt/app/web/static"
LOCAL_STATIC_DIR="web/static"

###############################################################################
# Helpers
###############################################################################
info() { echo "[deploy] $*"; }
err() {
  echo "[deploy] ERROR: $*" >&2
  exit 1
}

###############################################################################
# 1. Build
###############################################################################
info "Building CSS..."
npm run css:build

info "Building Go binary for Linux..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -trimpath -ldflags="-s -w" \
  -o bin/app ./cmd/main.go

if [[ ! -f bin/app ]]; then
  err "Go build failed — bin/app not found"
fi

info "Build complete: $(file bin/app | sed 's/,.*//')"

###############################################################################
# 2. Copy files to VPS temp locations
###############################################################################
info "Copying binary to $VPS_HOST..."
scp bin/app "${VPS_HOST}:${REMOTE_BIN_TMP}"

if [[ -d "${LOCAL_STATIC_DIR}" ]]; then
  info "Copying static assets to $VPS_HOST..."
  scp -r "${LOCAL_STATIC_DIR}/." "${VPS_HOST}:${REMOTE_STATIC_TMP}"
fi

###############################################################################
# 3. Run deploy wrapper on VPS (handles install + statics + restart)
###############################################################################
info "Running deploy on VPS..."
ssh "${VPS_HOST}" "sudo /usr/local/bin/deploy-wrapper"

info "Deploy complete. App restarted."
info "Test: curl -I https://staging.example.com/"
