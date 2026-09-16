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
# Scratch for the test itself; /data is handed to the grafana user by the launcher.
WORK="$(mktemp -d)"

cleanup() {
    if [ "${KEEP:-0}" != 1 ]; then
        docker rm -f "$NAME" >/dev/null 2>&1 || true
        # The launcher hands /data to uid 472 and keeps some of it root-owned.
        sudo rm -rf "$DATA" 2>/dev/null || rm -rf "$DATA" 2>/dev/null || true
        rm -rf "$WORK"
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
j -H 'X-Remote-User-Name: sean' http://127.0.0.1:1337/api/plugins/trooperthorn-swis-datasource/settings > "$WORK/plugin.json" || true
grep -q '"id":"trooperthorn-swis-datasource"' "$WORK/plugin.json" || fail "plugin settings not served: $(cat "$WORK/plugin.json")"
grep -q '"type":"datasource"' "$WORK/plugin.json" || fail "plugin is not a data source: $(cat "$WORK/plugin.json")"
# The backend is proven by running it: a data source pointing at a host that
# does not exist makes the Go backend's health check answer with its own
# "could not reach SWIS" message, which no frontend-only plugin could produce.
j -X POST -H 'Content-Type: application/json' -H 'X-Remote-User-Name: sean' \
    -d '{"name":"swis-smoke","type":"trooperthorn-swis-datasource","access":"proxy","uid":"swis-smoke","jsonData":{"host":"orion.invalid","username":"smoke"},"secureJsonData":{"password":"smoke"}}' \
    http://127.0.0.1:1337/api/datasources > "$WORK/ds.json" || true
grep -q '"uid":"swis-smoke"' "$WORK/ds.json" || fail "could not create a SWIS data source: $(cat "$WORK/ds.json")"
j -H 'X-Remote-User-Name: sean' http://127.0.0.1:1337/api/datasources/uid/swis-smoke/health > "$WORK/health.json" || true
grep -q "could not reach SWIS" "$WORK/health.json" || fail "the plugin backend did not answer the health check: $(cat "$WORK/health.json")"
if docker logs "$NAME" 2>&1 | grep -i "trooperthorn-swis-datasource" | grep -qi "problem with signature"; then
    fail "Grafana refused the bundled plugin's signature"
fi
echo "  ok: plugin registered"

echo "== terminal gate"
expect 403 -H 'X-Remote-User-Name: ed' http://127.0.0.1:1337/terminal/
expect 200 -H 'X-Remote-User-Name: sean' http://127.0.0.1:1337/terminal/

echo "== processes and token"
docker exec "$NAME" ps -eo user,comm > "$WORK/ps.txt"
for p in grafana nginx ttyd; do
    grep -Eq "^(472|grafana)\s+$p" "$WORK/ps.txt" || fail "$p is not running as the grafana user: $(cat "$WORK/ps.txt")"
done
# Grafana's environ is readable only with CAP_SYS_PTRACE (the process is
# non-dumpable), which the container does not carry; docker exec --privileged
# grants it to this one command without changing the image.
environ="$(docker exec --privileged "$NAME" sh -c 'tr "\0" "\n" < /proc/$(pgrep -o -x grafana)/environ')" \
    || fail "could not read Grafana's environment"
printf '%s\n' "$environ" | grep -q '^GF_PATHS_DATA=/data/grafana$' || fail "Grafana's environment lacks the /data paths"
if printf '%s\n' "$environ" | grep -q SUPERVISOR_TOKEN; then
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

echo "== state lives under /data, nothing downloaded"
docker exec "$NAME" test -f /data/grafana/grafana.db || fail "grafana.db is not under /data/grafana"
if docker logs "$NAME" 2>&1 | grep -q "plugin.backgroundinstaller"; then
    fail "Grafana downloaded plugins at start-up (preinstall is meant to be off)"
fi
extra="$(docker exec "$NAME" ls -A /data/plugins)"
[ -z "$extra" ] || fail "unexpected plugins under /data/plugins: $extra"
echo "  ok"

echo "== access log identity"
docker exec "$NAME" tail -n 50 /data/log/nginx/access.log | grep -q '"user_name":"sean"' || fail "access log lacks the Home Assistant username"
echo "  ok"

echo "== smoke test passed"
