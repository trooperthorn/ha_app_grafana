#!/usr/bin/env bash
# Behavioural smoke test for the built image: starts it with a
# Supervisor-shaped /data and proves the gates the documentation claims.
# Used by .github/workflows/test.yml here and by the OrionGuides CI while
# the app lives in that repository.
#
#   scripts/smoke_test.sh <image-tag>
#
# Loopback is allowed by nginx for the container health check, so requests
# from inside the container stand in for the Ingress gateway, with the
# identity headers the Supervisor would attach.
set -euo pipefail

IMAGE="${1:?image tag}"
NAME="grafana-smoke-$$"
DATA="$(mktemp -d)"

cleanup() {
    if [ "${KEEP:-0}" != 1 ]; then
        docker rm -f "$NAME" >/dev/null 2>&1 || true
        # The launcher hands /data to uid 472 and keeps some of it root-owned.
        sudo rm -rf "$DATA" 2>/dev/null || rm -rf "$DATA" 2>/dev/null || true
    fi
}
trap cleanup EXIT

fail() { echo "::error::$1"; docker logs "$NAME" 2>&1 | tail -n 80 || true; exit 1; }
c() { docker exec "$NAME" curl -s -o /dev/null -w '%{http_code}' "$@"; }
j() { docker exec "$NAME" curl -s "$@"; }
expect() { local want="$1"; shift; local got; got="$(c "$@")"; [ "$got" = "$want" ] || fail "expected HTTP $want, got $got for: $*"; echo "  ok: $want $*"; }

cat > "$DATA/options.json" <<'JSON'
{"admin_users": ["sean"], "editor_users": ["ed"], "default_role": "none",
 "allow_embedding": false, "terminal_enabled": true, "terminal_session_recording": true,
 "terminal_idle_timeout_minutes": 30, "plugins": [], "custom_plugins": [],
 "log_level": "info", "access_log_retention_days": 90}
JSON

echo "== starting $IMAGE"
docker run -d --name "$NAME" -v "$DATA:/data" "$IMAGE" >/dev/null

echo "== waiting for healthy"
for i in $(seq 1 40); do
    status="$(docker inspect --format='{{.State.Health.Status}}' "$NAME" 2>/dev/null || echo unknown)"
    [ "$status" = "healthy" ] && break
    [ "$status" = "unhealthy" ] && fail "container reported unhealthy"
    sleep 5
done
[ "$status" = "healthy" ] || fail "container did not report healthy in time"

echo "== identity and role gates"
expect 401 http://127.0.0.1:1337/api/user
expect 403 -H 'X-Remote-User-Name: bob' http://127.0.0.1:1337/api/user
expect 200 -H 'X-Remote-User-Name: sean' -H 'X-Remote-User-Display-Name: Sean' http://127.0.0.1:1337/api/user
j -H 'X-Remote-User-Name: sean' http://127.0.0.1:1337/api/user | grep -q '"login":"sean"' || fail "auth proxy did not sign in sean"
j -H 'X-Remote-User-Name: sean' http://127.0.0.1:1337/api/user/orgs | grep -q '"role":"Admin"' || fail "sean is not Admin"
j -H 'X-Remote-User-Name: ed' http://127.0.0.1:1337/api/user/orgs | grep -q '"role":"Editor"' || fail "ed is not Editor"
j -H 'X-Remote-User-Name: ed' -H 'X-WEBAUTH-ROLE: Admin' http://127.0.0.1:1337/api/user/orgs | grep -q '"role":"Editor"' \
    || fail "a browser-supplied X-WEBAUTH-ROLE was honoured"
j -H 'X-Remote-User-Name: sean' http://127.0.0.1:1337/api/frontend/settings | grep -q '"disableLoginForm":true' || fail "login form is not disabled"
pw="$(docker exec "$NAME" cat /data/secrets/admin_password)"
expect 403 -u "admin:${pw}" -H 'X-Remote-User-Name: bob' http://127.0.0.1:1337/api/user
echo "  ok: basic auth with the generated password is not an entry on the Ingress port"

echo "== bundled SolarWinds plugin"
j -H 'X-Remote-User-Name: sean' http://127.0.0.1:1337/api/plugins/trooperthorn-swis-datasource/settings > "$DATA/plugin.json" || true
grep -q '"id":"trooperthorn-swis-datasource"' "$DATA/plugin.json" || fail "plugin settings not served: $(cat "$DATA/plugin.json")"
grep -q '"backend":true' "$DATA/plugin.json" || fail "plugin is not registered as a backend plugin"
if docker logs "$NAME" 2>&1 | grep -i "trooperthorn-swis-datasource" | grep -qi "problem with signature"; then
    fail "Grafana refused the bundled plugin's signature"
fi
echo "  ok: plugin registered"

echo "== terminal gate"
expect 403 -H 'X-Remote-User-Name: ed' http://127.0.0.1:1337/terminal/
expect 200 -H 'X-Remote-User-Name: sean' http://127.0.0.1:1337/terminal/

echo "== processes and token"
docker exec "$NAME" ps -eo user,comm > "$DATA/ps.txt"
for p in grafana nginx ttyd; do
    grep -Eq "^(472|grafana)\s+$p" "$DATA/ps.txt" || fail "$p is not running as the grafana user: $(cat "$DATA/ps.txt")"
done
if docker exec "$NAME" sh -c 'tr "\0" "\n" < /proc/$(pgrep -o -x grafana)/environ | grep -q SUPERVISOR_TOKEN'; then
    fail "SUPERVISOR_TOKEN is present in Grafana's environment"
fi
echo "  ok: unprivileged processes, no token"

echo "== tools"
for absent in wget nc netcat telnet ftp ssh scp sftp nmap ncat tcpdump rsync git python3 sudo tmux screen; do
    docker exec "$NAME" sh -c "! command -v $absent >/dev/null" || fail "$absent is present in the image"
done
for present in ttyd bash script nano less jq sqlite3 unzip nginx setpriv; do
    docker exec "$NAME" sh -c "command -v $present >/dev/null" || fail "$present is missing from the image"
done
docker exec "$NAME" test -x /usr/share/grafana/bin/grafana || fail "grafana binary missing"
echo "  ok"

echo "== access log identity"
docker exec "$NAME" tail -n 50 /data/log/nginx/access.log | grep -q '"user_name":"sean"' || fail "access log lacks the Home Assistant username"
echo "  ok"

echo "== smoke test passed"
