#!/usr/bin/env bash
# End-to-end smoke test against the real compiled binary: boot, auth, publish
# (canonical + ntfy-compatible), list, SSE live delivery, persistence reopen.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
TMP="$(mktemp -d)"
trap 'kill ${SERVER_PID:-} 2>/dev/null || true; rm -rf "$TMP"' EXIT

PORT=$((20000 + RANDOM % 20000))
ADDR="127.0.0.1:$PORT"
BASE="http://$ADDR"

echo "1/9 build"
CGO_ENABLED=0 go build -trimpath -o "$TMP/nudge" .

echo "2/9 boot server on $ADDR"
NUDGE_ADDR="$ADDR" NUDGE_DATA_DIR="$TMP/data" "$TMP/nudge" serve >"$TMP/server.log" 2>&1 &
SERVER_PID=$!

for i in $(seq 1 50); do
  curl -sf "$BASE/healthz" >/dev/null && break
  sleep 0.1
done
curl -sf "$BASE/healthz" >/dev/null || { echo "server failed to start"; cat "$TMP/server.log"; exit 1; }

ADMIN="$(cat "$TMP/data/admin.token")"
echo "   admin token: ${ADMIN:0:14}…"
auth=(-H "Authorization: Bearer $ADMIN")

echo "3/9 vapid public key"
curl -sf "$BASE/api/v1/vapid-public" | grep -q public_key

echo "4/9 create publish key"
KEY_JSON="$(curl -sf "${auth[@]}" -H 'Content-Type: application/json' \
  -d '{"name":"smoke","topic":"backups"}' "$BASE/api/v1/keys")"
KEY="$(printf '%s' "$KEY_JSON" | python3 -c 'import sys,json;print(json.load(sys.stdin)["token"])')"

echo "5/9 canonical publish"
curl -sf -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
  -d '{"title":"Backup OK","body":"nightly dump finished","level":"success"}' \
  "$BASE/api/v1/notify" | grep -q '"id"'

echo "6/9 ntfy-compatible publish"
curl -sf -X POST -H "Authorization: Bearer $KEY" -H 'X-Title: Disk' \
  --data 'disk usage 72%' "$BASE/nightly" >/dev/null

echo "7/9 list shows both events"
COUNT="$(curl -sf "${auth[@]}" "$BASE/api/v1/events?limit=10" \
  | python3 -c 'import sys,json;print(len(json.load(sys.stdin)["events"]))')"
[ "$COUNT" = "2" ] || { echo "expected 2 events, got $COUNT"; exit 1; }

echo "8/9 SSE live delivery"
( curl -sfN "${auth[@]}" "$BASE/api/v1/stream" >"$TMP/sse.out" & echo $! >"$TMP/sse.pid" )
sleep 0.5
curl -sf -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
  -d '{"title":"live-event","body":"via sse"}' "$BASE/api/v1/notify" >/dev/null
for i in $(seq 1 50); do grep -q live-event "$TMP/sse.out" && break; sleep 0.1; done
grep -q live-event "$TMP/sse.out" || { echo "SSE did not deliver"; cat "$TMP/sse.out"; exit 1; }
kill "$(cat "$TMP/sse.pid")" 2>/dev/null || true

echo "9/9 persistence across restart"
kill "$SERVER_PID"; wait "$SERVER_PID" 2>/dev/null || true
NUDGE_ADDR="$ADDR" NUDGE_DATA_DIR="$TMP/data" "$TMP/nudge" serve >>"$TMP/server.log" 2>&1 &
SERVER_PID=$!
for i in $(seq 1 50); do curl -sf "$BASE/healthz" >/dev/null && break; sleep 0.1; done
RECOUNT="$(curl -sf "${auth[@]}" "$BASE/api/v1/events?limit=10" \
  | python3 -c 'import sys,json;print(len(json.load(sys.stdin)["events"]))')"
[ "$RECOUNT" = "3" ] || { echo "expected 3 events after restart, got $RECOUNT"; exit 1; }

echo "ALL E2E CHECKS PASSED"
