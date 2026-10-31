#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
BIN="${1:-$ROOT/dist/heimdrive-linux-amd64}"
PORT="${PORT:-18082}"
case "$BIN" in
    /*) ;;
    *) BIN="$ROOT/$BIN" ;;
esac

TMP=$(mktemp -d)
PID=""
cleanup() {
    if [ -n "$PID" ]; then
        kill "$PID" 2>/dev/null || true
        wait "$PID" 2>/dev/null || true
    fi
    rm -rf "$TMP"
}
trap cleanup EXIT INT TERM

mkdir -p "$TMP/data"
HEIMDRIVE_PASSWORD=measure "$BIN" -addr "127.0.0.1:$PORT" -data "$TMP/data" >"$TMP/server.log" 2>&1 &
PID=$!

for _ in $(seq 1 60); do
    if curl -fsS "http://127.0.0.1:$PORT/healthz" >/dev/null 2>&1; then
        break
    fi
    sleep 0.1
done
sleep 1
ps -o pid,rss,vsz,cmd -p "$PID"
