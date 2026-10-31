#!/bin/sh
set -eu

VERSION="${1:-1.0.0}"
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT"
mkdir -p dist

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
  -ldflags "-s -w -X main.version=$VERSION" \
  -o dist/heimdrive-linux-amd64 .

CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath \
  -ldflags "-s -w -X main.version=$VERSION" \
  -o dist/heimdrive-linux-arm64 .

CGO_ENABLED=0 GOOS=linux GOARCH=386 GO386=softfloat go build -trimpath \
  -ldflags "-s -w -X main.version=$VERSION" \
  -o dist/heimdrive-linux-386 .

ls -lh dist/heimdrive-linux-amd64 dist/heimdrive-linux-arm64 dist/heimdrive-linux-386
