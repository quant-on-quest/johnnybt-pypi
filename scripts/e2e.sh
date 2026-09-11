#!/usr/bin/env bash
# End-to-end check with the real clients: uv publish → grant → uv pip install.
# Usage: scripts/e2e.sh   (needs uv, curl, python3)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORK="$(mktemp -d)"
PORT="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1])')"
URL="http://127.0.0.1:$PORT"
cleanup() {
  status=$?
  kill "${SERVER_PID:-0}" 2>/dev/null || true
  if [ "$status" -ne 0 ] && [ -f "$WORK/server.log" ]; then
    printf '\n\033[1;31m== server.log (tail)\033[0m\n'; tail -n 30 "$WORK/server.log"
  fi
  rm -rf "$WORK"
}
trap cleanup EXIT

step() { printf '\n\033[1;34m== %s\033[0m\n' "$*"; }
api()  { curl -sS -b "$WORK/cookies" -c "$WORK/cookies" -H 'Content-Type: application/json' -H 'Sec-Fetch-Site: same-origin' "$@"; }
json() { python3 -c "import sys, json; d=json.load(sys.stdin); print($1)"; }

step "build server"
(cd "$ROOT" && go build -o "$WORK/pypi-server" ./cmd/pypi-server)

step "start server on $URL"
PYPI_ADDR="127.0.0.1:$PORT" PYPI_DATA_DIR="$WORK/data" "$WORK/pypi-server" > "$WORK/server.log" 2>&1 &
SERVER_PID=$!
for _ in $(seq 1 50); do curl -s "$URL/api/v1/auth/me" >/dev/null 2>&1 && break; sleep 0.1; done
PASSWORD="$(sed -n 's/^password: //p' "$WORK/data/initial_admin_password")"
echo "admin password from first boot: $PASSWORD"

step "admin login + write token"
api -X POST "$URL/api/v1/auth/login" -d "{\"username\":\"admin\",\"password\":\"$PASSWORD\"}" >/dev/null
ADMIN_ID="$(api "$URL/api/v1/auth/me" | json 'd["id"]')"
WRITE_TOKEN="$(api -X POST "$URL/api/v1/users/$ADMIN_ID/tokens" -d '{"name":"ci","scope":"write"}' | json 'd["token"]')"
echo "write token: ${WRITE_TOKEN:0:12}…"

step "build demo package with uv"
mkdir -p "$WORK/demo/src/demo_pkg"
cat > "$WORK/demo/pyproject.toml" <<'PY'
[project]
name = "johnnybt-demo"
version = "0.1.0"
description = "Demo package for the private index"
requires-python = ">=3.10"
[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
[tool.hatch.build.targets.wheel]
packages = ["src/demo_pkg"]
PY
echo '__version__ = "0.1.0"' > "$WORK/demo/src/demo_pkg/__init__.py"
(cd "$WORK/demo" && uv build -q)
ls "$WORK/demo/dist"

step "uv publish → /legacy/"
uv publish --publish-url "$URL/legacy/" --username __token__ --password "$WRITE_TOKEN" "$WORK/demo/dist/"*
echo "re-publish must be rejected as duplicate:"
if uv publish --publish-url "$URL/legacy/" --username __token__ --password "$WRITE_TOKEN" "$WORK/demo/dist/"* 2>"$WORK/dup.err"; then
  echo "ERROR: duplicate upload accepted"; exit 1
fi
grep -q "already exists" "$WORK/dup.err" && echo "  ok (File already exists)"

step "customer alice: create, grant johnnybt-demo, read token"
ALICE_ID="$(api -X POST "$URL/api/v1/users" -d '{"username":"alice","note":"e2e"}' | json 'd["id"]')"
api -X PUT "$URL/api/v1/users/$ALICE_ID/entitlements/johnnybt-demo" -d '{"expires_at":"2099-12-31"}' -o /dev/null -w 'grant → HTTP %{http_code}\n'
ALICE_TOKEN="$(api -X POST "$URL/api/v1/users/$ALICE_ID/tokens" -d '{"name":"laptop"}' | json 'd["token"]')"

step "customer bob: no entitlement"
BOB_ID="$(api -X POST "$URL/api/v1/users" -d '{"username":"bob"}' | json 'd["id"]')"
BOB_TOKEN="$(api -X POST "$URL/api/v1/users/$BOB_ID/tokens" -d '{"name":"laptop"}' | json 'd["token"]')"

step "uv pip install as alice"
uv venv -q "$WORK/venv-alice"
VIRTUAL_ENV="$WORK/venv-alice" uv pip install -q --index-url "http://__token__:$ALICE_TOKEN@127.0.0.1:$PORT/simple/" johnnybt-demo
"$WORK/venv-alice/bin/python" -c 'import demo_pkg; print("alice installed johnnybt-demo", demo_pkg.__version__)'

step "uv pip install as bob must fail"
uv venv -q "$WORK/venv-bob"
if VIRTUAL_ENV="$WORK/venv-bob" uv pip install -q --index-url "http://__token__:$BOB_TOKEN@127.0.0.1:$PORT/simple/" johnnybt-demo 2>"$WORK/bob.err"; then
  echo "ERROR: bob installed a package he is not entitled to"; exit 1
fi
echo "  ok: $(grep -m1 -oE '(403|Forbidden|not entitled|No solution)[^\n]*' "$WORK/bob.err" | head -1)"

step "download log"
api "$URL/api/v1/downloads" | python3 -c '
import sys, json
rows = json.load(sys.stdin)
assert [r["username"] for r in rows] == ["alice"], rows
for r in rows: print("  %s ← %s (%s)" % (r["username"], r["filename"], r["user_agent"][:8]))
'

step "token self-check as alice"
curl -sS -X POST "$URL/api/v1/me" -H 'Content-Type: application/json' -d "{\"token\":\"$ALICE_TOKEN\"}" | python3 -c '
import sys, json
d = json.load(sys.stdin)
assert d["username"] == "alice" and [p["normalized_name"] for p in d["packages"]] == ["johnnybt-demo"], d
print("  alice →", ", ".join("%s (expires %s)" % (p["package_name"], str(p["expires_at"])[:10]) for p in d["packages"]))
'

printf '\n\033[1;32mE2E OK\033[0m\n'
