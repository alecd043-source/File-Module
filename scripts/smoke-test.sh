#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
BIN="${1:-$ROOT/dist/heimdrive-linux-amd64}"
PORT="${PORT:-18080}"
case "$BIN" in
    /*) ;;
    *) BIN="$ROOT/$BIN" ;;
esac

if [ ! -x "$BIN" ]; then
    echo "Binary nicht ausführbar: $BIN" >&2
    exit 1
fi
if ! command -v curl >/dev/null 2>&1; then
    echo "curl wird benötigt." >&2
    exit 1
fi

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
HEIMDRIVE_PASSWORD=smoke-test-pass "$BIN" \
    -addr "127.0.0.1:$PORT" \
    -data "$TMP/data" \
    -name SmokeDrive >"$TMP/server.log" 2>&1 &
PID=$!

ready=0
for _ in $(seq 1 60); do
    if curl -fsS "http://127.0.0.1:$PORT/healthz" >/dev/null 2>&1; then
        ready=1
        break
    fi
    sleep 0.1
done
if [ "$ready" -ne 1 ]; then
    echo "Server wurde nicht bereit." >&2
    cat "$TMP/server.log" >&2
    exit 1
fi

COOKIES="$TMP/cookies.txt"
BASE="http://127.0.0.1:$PORT"

status=$(curl -sS -c "$COOKIES" -o /dev/null -w '%{http_code}' \
    --data 'username=admin&password=smoke-test-pass' "$BASE/login")
[ "$status" = "303" ] || { echo "Login fehlgeschlagen: $status" >&2; exit 1; }

index=$(curl -sS -b "$COOKIES" "$BASE/")
csrf=$(printf '%s' "$index" | sed -n 's/.*name="csrf-token" content="\([^"]*\)".*/\1/p')
[ -n "$csrf" ] || { echo "CSRF-Token fehlt." >&2; exit 1; }

printf 'Hallo HeimDrive\n' >"$TMP/smoke.txt"
upload=$(curl -sS -b "$COOKIES" -H "X-CSRF-Token: $csrf" \
    -F "file=@$TMP/smoke.txt;filename=smoke.txt" "$BASE/api/upload?path=")
printf '%s' "$upload" | grep -q '"name":"smoke.txt"' || { echo "Upload fehlgeschlagen: $upload" >&2; exit 1; }

duplicate=$(curl -sS -b "$COOKIES" -H "X-CSRF-Token: $csrf" \
    -F "file=@$TMP/smoke.txt;filename=smoke.txt" "$BASE/api/upload?path=")
printf '%s' "$duplicate" | grep -q '"name":"smoke (1).txt"' || { echo "Namenskonflikt fehlgeschlagen: $duplicate" >&2; exit 1; }

curl -fsS -b "$COOKIES" "$BASE/api/download?path=smoke.txt" -o "$TMP/download.txt"
cmp -s "$TMP/smoke.txt" "$TMP/download.txt" || { echo "Download stimmt nicht überein." >&2; exit 1; }

mkdir_json=$(curl -sS -b "$COOKIES" -H "X-CSRF-Token: $csrf" \
    -H 'Content-Type: application/json' \
    --data '{"path":"","name":"Ordner"}' "$BASE/api/mkdir")
printf '%s' "$mkdir_json" | grep -q '"name":"Ordner"' || { echo "mkdir fehlgeschlagen: $mkdir_json" >&2; exit 1; }

nested=$(curl -sS -b "$COOKIES" -H "X-CSRF-Token: $csrf" \
    -F "file=@$TMP/smoke.txt;filename=innen.txt" "$BASE/api/upload?path=Ordner")
printf '%s' "$nested" | grep -q '"name":"innen.txt"' || { echo "Upload in Ordner fehlgeschlagen: $nested" >&2; exit 1; }

curl -fsS -b "$COOKIES" "$BASE/api/download?path=Ordner" -o "$TMP/ordner.zip"
[ "$(dd if="$TMP/ordner.zip" bs=2 count=1 2>/dev/null)" = "PK" ] || { echo "ZIP-Download fehlgeschlagen." >&2; exit 1; }

status=$(curl -sS -b "$COOKIES" -o /dev/null -w '%{http_code}' -X DELETE \
    -H "X-CSRF-Token: $csrf" "$BASE/api/delete?path=Ordner")
[ "$status" = "204" ] || { echo "Löschen fehlgeschlagen: $status" >&2; exit 1; }

echo "Smoke-Test OK"
