#!/usr/bin/env bash
# Exercises install.sh without root or systemd, against a locally built archive.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT

archive="$(ls "$ROOT"/dist/pypi-server_*_linux_amd64.tar.gz 2>/dev/null | head -n1)"
[ -n "$archive" ] || { echo "no archive in dist/ — run: TARGETS=linux/amd64 scripts/build-release.sh v0.0.0-test"; exit 1; }
version="$(basename "$archive" | sed 's/^pypi-server_\(.*\)_linux_amd64.tar.gz$/\1/')"

run() { NO_SYSTEMD=1 PREFIX="$tmp/bin" ETC_DIR="$tmp/etc" DATA_DIR="$tmp/data" LOCAL_ARCHIVE="$archive" VERSION="$version" "$@" bash "$ROOT/scripts/install.sh"; }

echo "--- fresh install"
run env | tee "$tmp/out1"
[ -x "$tmp/bin/pypi-server" ] || { echo "FAIL: binary not installed"; exit 1; }
[ "$("$tmp/bin/pypi-server" version)" = "$version" ] || { echo "FAIL: wrong version"; exit 1; }
[ -f "$tmp/etc/env" ] || { echo "FAIL: env template missing"; exit 1; }
[ "$(stat -c %a "$tmp/etc/env")" = "600" ] || { echo "FAIL: env should be 0600"; exit 1; }

echo "--- rerun is a no-op at the same version"
run env | tee "$tmp/out2"
grep -q "无需升级" "$tmp/out2" || { echo "FAIL: expected no-op"; exit 1; }

echo "--- FORCE reinstall keeps the existing env"
echo "PYPI_ADMIN_PATH=/custom" >> "$tmp/etc/env"
run env FORCE=1 | tee "$tmp/out3"
grep -q "已存在，保持不变" "$tmp/out3" || { echo "FAIL: env was not preserved"; exit 1; }
grep -q "PYPI_ADMIN_PATH=/custom" "$tmp/etc/env" || { echo "FAIL: env content lost"; exit 1; }
[ -f "$tmp/etc/env.example" ] || { echo "FAIL: env.example not refreshed"; exit 1; }

echo "--- corrupt archive is rejected"
echo garbage > "$tmp/bad.tar.gz"
if NO_SYSTEMD=1 PREFIX="$tmp/bin2" ETC_DIR="$tmp/etc2" LOCAL_ARCHIVE="$tmp/bad.tar.gz" bash "$ROOT/scripts/install.sh" 2>/dev/null; then
  echo "FAIL: corrupt archive accepted"; exit 1
fi
echo "install.sh OK"
