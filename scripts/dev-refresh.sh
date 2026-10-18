#!/bin/sh
# Dev rebuild helper: rebuild Surge from this checkout and redeploy the binary
# so the `surge` command runs the code you just built.
#
# Usage:
#   scripts/dev-refresh.sh             # build + deploy + relabel
#   scripts/dev-refresh.sh --service   # also refresh and restart an installed system service
#
# Env overrides:
#   SURGE_INSTALL_DIR   deploy location for the binary (default: /usr/local/bin)

set -eu

INSTALL_DIR="${SURGE_INSTALL_DIR:-/usr/local/bin}"
REFRESH_SERVICE=0

log() { printf '%s\n' "$*" >&2; }
die() { log "error: $*"; exit 1; }

usage() {
  cat >&2 <<'EOF'
Usage: scripts/dev-refresh.sh [--service] [--help]

Rebuilds Surge in this checkout (go build -o surge .) and copies the fresh
binary to SURGE_INSTALL_DIR (default: /usr/local/bin) so `surge` runs the
latest build; it may prompt for your password when the target is a system path.
On SELinux systems the copy is relabeled bin_t, which the system service needs
to be allowed to execute it.

  --service   Also re-register and restart an already-installed system service
              so the running daemon picks up the new binary. Only affects the
              service if it is already installed; it never creates one.

Environment:
  SURGE_INSTALL_DIR   Deploy location (default: /usr/local/bin)
EOF
}

for arg in "$@"; do
  case "$arg" in
    --service) REFRESH_SERVICE=1 ;;
    -h|--help) usage; exit 0 ;;
    *) usage; die "unknown argument: $arg" ;;
  esac
done

command -v go >/dev/null 2>&1 || die "missing required command: go"

REPO_ROOT="$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$REPO_ROOT"

# Run a command as root, prompting via sudo when we are not root already.
run_root() {
  if [ "$(id -u)" = 0 ]; then
    "$@"
  elif command -v sudo >/dev/null 2>&1; then
    sudo "$@"
  else
    die "root privileges required: $*"
  fi
}

log "Building surge..."
go build -o surge .
VERSION="$(./surge --version 2>/dev/null || echo 'surge')"

# Deploy without sudo when the target directory is already writable (for
# example SURGE_INSTALL_DIR=~/.local/bin).
if [ ! -d "$INSTALL_DIR" ]; then
  run_root mkdir -p "$INSTALL_DIR"
fi

if [ -w "$INSTALL_DIR" ]; then
  install -m 0755 ./surge "$INSTALL_DIR/surge"
else
  run_root install -m 0755 ./surge "$INSTALL_DIR/surge"
fi
log "Deployed $VERSION to $INSTALL_DIR/surge"

# A binary in a system path needs the bin_t label for systemd to exec it.
if command -v restorecon >/dev/null 2>&1; then
  if [ -w "$INSTALL_DIR/surge" ]; then
    restorecon -v "$INSTALL_DIR/surge"
  else
    run_root restorecon -v "$INSTALL_DIR/surge"
  fi || log "warning: restorecon failed; the service may hit status=203/EXEC"
fi

if [ "$REFRESH_SERVICE" = 1 ]; then
  if ! command -v systemctl >/dev/null 2>&1; then
    log "note: systemctl not found; skipping service refresh"
  elif ! systemctl list-unit-files --type=service 2>/dev/null | grep -q '^surge\.service'; then
    log "note: surge.service is not installed; run 'sudo $INSTALL_DIR/surge service install' to create it"
  else
    # Rewrite ExecStart/ConditionFileIsExecutable to point at this binary, then
    # start it: the daemon keeps running the old binary until it is restarted.
    run_root "$INSTALL_DIR/surge" service install
    run_root "$INSTALL_DIR/surge" service start
    log "Service refreshed and restarted."
  fi
fi

case ":${PATH:-}:" in
  *":${INSTALL_DIR}:"*) ;;
  *) log "note: $INSTALL_DIR is not on your PATH; add it so 'surge' resolves there: export PATH=\"$INSTALL_DIR:\$PATH\"" ;;
esac
