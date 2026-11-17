#!/usr/bin/env bash
#
# Remote deploy step, installed on the VPS as /usr/local/bin/pohkapela-deploy:
#
#   scp scripts/remote-deploy.sh <host>:/tmp/remote-deploy.sh
#   ssh <host> 'sudo install -o root -g root -m 0755 /tmp/remote-deploy.sh /usr/local/bin/pohkapela-deploy'
#
# scripts/deploy.sh uploads:
#   /tmp/pohkapela-app                  the new binary
#   <ssh-user-home>/pohkapela-static/   the new static assets (CSS etc.)
#
# Run through `sudo`, $HOME is root's home, so a naive `$HOME/pohkapela-static`
# check never matches the uploaded directory and the static sync is skipped
# silently — the site then serves CSS built before the current templates.
# Resolve the uploader's home through SUDO_USER (or use the explicit path
# passed by scripts/deploy.sh) and refuse to deploy when assets are missing.
#
# This file is public: it must never contain host names, passwords or keys.
# It is also a root entry point, so every staged path is validated before use.
set -euo pipefail

BIN="/tmp/pohkapela-app"
TARGET="/opt/pohkapela/app"
STATIC_DIR="/opt/pohkapela/web/static"

DEPLOY_USER="${SUDO_USER:-$(id -un)}"

fail() {
	echo "pohkapela-deploy: $*" >&2
	exit 1
}

if [ "$(id -u)" -ne 0 ]; then
	fail "must run as root (use sudo)"
fi

if [ -n "${1:-}" ]; then
	STATIC_SRC="$1"
else
	DEPLOY_HOME="$(getent passwd "$DEPLOY_USER" | cut -d: -f6 || true)"
	if [ -z "$DEPLOY_HOME" ]; then
		fail "cannot resolve home for $DEPLOY_USER; pass the static dir as the first argument"
	fi
	STATIC_SRC="${DEPLOY_HOME}/pohkapela-static"
fi

case "$STATIC_SRC" in
"" | "/" | "/." )
	fail "refusing to use an unsafe static path: '$STATIC_SRC'"
	;;
esac

# --- staged binary -----------------------------------------------------------
# scp creates this file as the deploying user. Reject symlinks and files the
# uploader does not own, so a pre-planted /tmp file can never be installed root.

[ -f "$BIN" ] && [ ! -L "$BIN" ] || fail "missing or invalid binary at $BIN — upload it before deploying"

[ "$(stat -c %U -- "$BIN")" = "$DEPLOY_USER" ] ||
	fail "$BIN must be owned by $DEPLOY_USER"

[ -z "$(find "$BIN" -maxdepth 0 -perm /022 -print -quit)" ] ||
	fail "$BIN must not be writable by group or others"

# --- staged static assets ----------------------------------------------------

[ -d "$STATIC_SRC" ] && [ ! -L "$STATIC_SRC" ] ||
	fail "missing static assets at $STATIC_SRC — refusing to deploy so the app and CSS stay in sync"

[ "$(stat -c %U -- "$STATIC_SRC")" = "$DEPLOY_USER" ] ||
	fail "$STATIC_SRC must be owned by $DEPLOY_USER"

[ -f "$STATIC_SRC/css/output.css" ] ||
	fail "static assets at $STATIC_SRC are missing css/output.css"

# Symlinks would be copied into the served tree; reject them instead of
# letting a link point outside the static directory.
if [ -n "$(find "$STATIC_SRC" -type l -print -quit)" ]; then
	fail "static assets at $STATIC_SRC contain symlinks"
fi

# --- install -----------------------------------------------------------------

install -o app -g app -m 0750 "$BIN" "$TARGET"
rm -f "$BIN"

cp -RP "$STATIC_SRC/." "$STATIC_DIR/"
chown -R app:app "$STATIC_DIR"
rm -rf "$STATIC_SRC"

systemctl restart pohkapela
