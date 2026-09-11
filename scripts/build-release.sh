#!/usr/bin/env bash
# Cross-compile release archives into dist/. Used by `make release` and the
# GitHub release workflow. Requires the web UI to be built first (make web).
#
#   scripts/build-release.sh v0.1.0
set -euo pipefail

VERSION="${1:?usage: build-release.sh <version>}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$ROOT/dist"
TARGETS="${TARGETS:-linux/amd64 linux/arm64 darwin/arm64}"

if [ ! -f "$ROOT/internal/web/dist/index.html" ]; then
  echo "error: web UI not built (run: make web)" >&2
  exit 1
fi

rm -rf "$OUT" && mkdir -p "$OUT"
for target in $TARGETS; do
  os="${target%/*}"; arch="${target#*/}"
  name="pypi-server_${VERSION}_${os}_${arch}"
  stage="$(mktemp -d)"
  echo "building $name"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -C "$ROOT" -trimpath \
    -ldflags="-s -w -X main.version=${VERSION}" -o "$stage/pypi-server" ./cmd/pypi-server
  cp "$ROOT/deploy/pypi-server.service" "$ROOT/deploy/env.example" "$ROOT/LICENSE" "$ROOT/README.md" "$stage/"
  tar -C "$stage" -czf "$OUT/$name.tar.gz" .
  rm -rf "$stage"
done
(cd "$OUT" && sha256sum ./*.tar.gz | sed 's|\./||' > checksums.txt)
echo "---"; cat "$OUT/checksums.txt"
