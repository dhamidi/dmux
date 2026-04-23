#!/usr/bin/env bash
# Smoke test for M1 acceptance criterion #5: `dmux kill-server`
# cleanly terminates the running server while an attach client is
# connected. Uses the auto-daemonize path (env DMUX=<path>) so the
# exercise matches how the user actually starts the server.
#
# Usage:
#   scripts/smoke-kill-server.sh
#
# Exits 0 on success, non-zero on any check failure.

set -u

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TESTDIR=$(mktemp -d "${TMPDIR:-/tmp}/dmux-kill-XXXXXX")
SOCK="$TESTDIR/sock"
DMUX="$TESTDIR/dmux"

cleanup() {
    kill -KILL "${ATTACH_PID:-0}" 2>/dev/null || true
    rm -rf "$TESTDIR"
}
trap cleanup EXIT

go build -o "$DMUX" "$ROOT/cmd/dmux" || { echo "build failed"; exit 1; }

# Attach client in the background. script(1) gives it a controlling
# tty so t.Raw() succeeds; its stdin is /dev/null so once the server
# disappears the blocked Read returns and the process exits.
DMUX="$SOCK" script -q /dev/null "$DMUX" </dev/null >/dev/null 2>&1 &
ATTACH_PID=$!

# Wait for server to come up.
for _ in $(seq 1 40); do
    [ -S "$SOCK" ] && break
    sleep 0.05
done
[ -S "$SOCK" ] || { echo "server never bound $SOCK"; exit 1; }
sleep 0.2

DMUX="$SOCK" "$DMUX" kill-server
RC=$?
if [ "$RC" -ne 0 ]; then
    echo "kill-server exit code: $RC"
    exit 1
fi

# Wait up to 2s for the attach client to notice and exit.
for _ in $(seq 1 40); do
    kill -0 "$ATTACH_PID" 2>/dev/null || break
    sleep 0.05
done
if kill -0 "$ATTACH_PID" 2>/dev/null; then
    echo "attach client did not exit within 2s"
    exit 1
fi

if [ -S "$SOCK" ]; then
    echo "socket file still present: $SOCK"
    exit 1
fi

echo "smoke-kill-server: OK"
exit 0
