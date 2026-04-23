#!/usr/bin/env bash
# Smoke test: two concurrent attach clients share one pane.
# Starts A under script(1), waits for the server to bind, starts B
# under script(1), sleeps long enough for both clients to render an
# initial frame, then issues kill-server. Both clients must exit
# within 2s and both script logs must contain a status bar line so
# we know the pane's dirty-signal subscription fed every attached
# client.
#
# Usage: scripts/smoke-multi-attach.sh
# Exits 0 on success, non-zero on any check failure.

set -u

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TESTDIR=$(mktemp -d "${TMPDIR:-/tmp}/dmux-multi-XXXXXX")
SOCK="$TESTDIR/sock"
DMUX="$TESTDIR/dmux"
LOGA="$TESTDIR/a.log"
LOGB="$TESTDIR/b.log"

cleanup() {
    kill -KILL "${APID:-0}" "${BPID:-0}" 2>/dev/null || true
    rm -rf "$TESTDIR"
}
trap cleanup EXIT

go build -o "$DMUX" "$ROOT/cmd/dmux" || { echo "build failed"; exit 1; }

DMUX="$SOCK" script -q "$LOGA" "$DMUX" </dev/null >/dev/null 2>&1 &
APID=$!

for _ in $(seq 1 40); do
    [ -S "$SOCK" ] && break
    sleep 0.05
done
[ -S "$SOCK" ] || { echo "server never bound $SOCK"; exit 1; }
sleep 0.3

DMUX="$SOCK" script -q "$LOGB" "$DMUX" </dev/null >/dev/null 2>&1 &
BPID=$!

# Wait long enough for both attach handlers to run their initial
# render and emit at least one Output frame (status row included).
sleep 1.0

DMUX="$SOCK" "$DMUX" kill-server >/dev/null
RC=$?
if [ "$RC" -ne 0 ]; then echo "kill-server exit code: $RC"; exit 1; fi

for _ in $(seq 1 40); do
    kill -0 "$APID" 2>/dev/null || kill -0 "$BPID" 2>/dev/null || break
    sleep 0.05
done
if kill -0 "$APID" 2>/dev/null; then echo "A did not exit within 2s"; exit 1; fi
if kill -0 "$BPID" 2>/dev/null; then echo "B did not exit within 2s"; exit 1; fi

grep -q '\[dmux\]' "$LOGA" || { echo "A never rendered status bar"; exit 1; }
grep -q '\[dmux\]' "$LOGB" || { echo "B never rendered status bar"; exit 1; }

echo "smoke-multi-attach: OK"
exit 0
