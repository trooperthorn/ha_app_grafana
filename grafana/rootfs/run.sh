#!/usr/bin/env bash
# Grafana (hardened): the launcher.
#
# Runs as root for exactly three things: fixing the ownership of /data
# (the Supervisor mounts it root-owned), rendering the configuration into
# /run/grafana-app, and starting nginx, ttyd and Grafana as the grafana user
# (472). Then it waits and forwards SIGTERM. Reads the app options from
# /data/options.json with jq: this image is FROM grafana/grafana, not a
# Home Assistant base image, so bashio is not present (the same choice the
# Technitium DNS app made). See docs/security.md and docs/decisions.md.

set -o errexit -o pipefail -o nounset

OPTIONS_FILE="/data/options.json"
RUN_DIR="/run/grafana-app"
SECRETS_DIR="/data/secrets"
GRAFANA_UID=472
GRAFANA_GID=472
APP_VERSION="2026.09.16.1"   # keep in lockstep with config.yaml on every release

log_info()    { printf '[%s] INFO: %s\n' "$(date -u '+%Y-%m-%d %H:%M:%S')" "$1"; }
log_warning() { printf '[%s] WARNING: %s\n' "$(date -u '+%Y-%m-%d %H:%M:%S')" "$1" >&2; }
log_error()   { printf '[%s] ERROR: %s\n' "$(date -u '+%Y-%m-%d %H:%M:%S')" "$1" >&2; }

# A scalar option, or the default when absent or null.
config_value() {
    local key="$1" default="$2" value
    value="$(jq -r --arg k "$key" '.[$k] // empty' "$OPTIONS_FILE" 2>/dev/null || true)"
    if [ -z "$value" ] || [ "$value" = "null" ]; then printf '%s' "$default"; else printf '%s' "$value"; fi
}

# A list option, one element per line.
config_list() {
    jq -r --arg k "$1" '(.[$k] // []) | .[] | tostring' "$OPTIONS_FILE" 2>/dev/null || true
}

as_grafana() {
    setpriv --reuid="$GRAFANA_UID" --regid="$GRAFANA_GID" --clear-groups --inh-caps=-all "$@"
}

# Grafana's auth proxy is only as trustworthy as the username it is handed,
# and nginx's map keys are quoted strings. Refuse anything that could not
# be a Home Assistant username and could break out of a map line.
valid_username() {
    [[ "$1" =~ ^[A-Za-z0-9._@+-]{1,64}$ ]]
}

log_info "Starting Grafana (hardened) ${APP_VERSION}..."

if [ ! -f "$OPTIONS_FILE" ]; then
    log_error "${OPTIONS_FILE} is missing; this container is meant to be started by the Home Assistant Supervisor."
    exit 1
fi

# --- /data layout, owned by the grafana user ------------------------------
mkdir -p /data/grafana /data/plugins \
         /data/provisioning/datasources /data/provisioning/dashboards /data/provisioning/plugins \
         /data/provisioning/notifiers /data/provisioning/alerting /data/provisioning/access-control \
         /data/log/grafana /data/log/nginx /data/terminal/sessions "$SECRETS_DIR" \
         "$RUN_DIR" "$RUN_DIR/client_body" "$RUN_DIR/proxy" "$RUN_DIR/fastcgi" "$RUN_DIR/uwsgi" "$RUN_DIR/scgi"
chown -R "$GRAFANA_UID:$GRAFANA_GID" /data/grafana /data/plugins /data/provisioning /data/log /data/terminal "$SECRETS_DIR" "$RUN_DIR"
chmod 0700 "$SECRETS_DIR" /data/terminal /data/terminal/sessions

# --- Secrets: generated once, never logged, never derived from a token ----
if [ ! -s "$SECRETS_DIR/secret_key" ]; then
    ( umask 077; head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n' > "$SECRETS_DIR/secret_key" )
    log_info "Generated the cookie signing key into ${SECRETS_DIR}/secret_key."
fi
if [ ! -s "$SECRETS_DIR/admin_password" ]; then
    ( umask 077; head -c 32 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | head -c 32 > "$SECRETS_DIR/admin_password" )
    log_info "Generated the initial admin password into ${SECRETS_DIR}/admin_password. It is not logged; the terminal can read it, and it works only on the optional API port."
fi
chmod 0600 "$SECRETS_DIR/secret_key" "$SECRETS_DIR/admin_password"
chown "$GRAFANA_UID:$GRAFANA_GID" "$SECRETS_DIR/secret_key" "$SECRETS_DIR/admin_password"
SECRET_KEY="$(cat "$SECRETS_DIR/secret_key")"
ADMIN_PASSWORD="$(cat "$SECRETS_DIR/admin_password")"

# --- Ingress entry: the URL prefix Grafana has to put in its links -------
# /addons/self/info needs no hassio_api grant. Outside the Supervisor (the
# CI smoke test) there is no token, and / is the right answer.
INGRESS_ENTRY="/"
if [ -n "${SUPERVISOR_TOKEN:-}" ]; then
    INGRESS_ENTRY="$(curl -fsS -m 10 -H "Authorization: Bearer ${SUPERVISOR_TOKEN}" \
        http://supervisor/addons/self/info 2>/dev/null | jq -r '.data.ingress_entry // "/"' || echo "/")"
    [ -n "$INGRESS_ENTRY" ] || INGRESS_ENTRY="/"
fi
# The token is used for that one call and removed before anything is
# started, so no child process, shell or transcript carries it.
unset SUPERVISOR_TOKEN
log_info "Ingress entry: ${INGRESS_ENTRY}"

# --- Roles ----------------------------------------------------------------
DEFAULT_ROLE="$(config_value 'default_role' 'none')"
case "$DEFAULT_ROLE" in none|Viewer) ;; *) log_warning "default_role '${DEFAULT_ROLE}' is not none or Viewer; using none."; DEFAULT_ROLE="none" ;; esac

ROLES_MAP="$RUN_DIR/roles.map"
: > "$ROLES_MAP"
ADMIN_COUNT=0
while IFS= read -r user; do
    [ -n "$user" ] || continue
    if valid_username "$user"; then
        printf '"%s" "Admin";\n' "$user" >> "$ROLES_MAP"; ADMIN_COUNT=$((ADMIN_COUNT + 1))
    else
        log_warning "admin_users entry '${user}' is not a valid username and was ignored."
    fi
done < <(config_list 'admin_users')
while IFS= read -r user; do
    [ -n "$user" ] || continue
    if grep -q "^\"${user}\" " "$ROLES_MAP"; then
        log_warning "'${user}' is in both admin_users and editor_users; Admin wins."
    elif valid_username "$user"; then
        printf '"%s" "Editor";\n' "$user" >> "$ROLES_MAP"
    else
        log_warning "editor_users entry '${user}' is not a valid username and was ignored."
    fi
done < <(config_list 'editor_users')
chmod 0644 "$ROLES_MAP"
if [ "$ADMIN_COUNT" -eq 0 ]; then
    log_warning "admin_users is empty: nobody can administer this Grafana through the panel. Add your Home Assistant username to admin_users."
fi
log_info "Roles: ${ADMIN_COUNT} administrator(s), default role for everyone else: ${DEFAULT_ROLE}."

# --- Plugins: catalogue ids and hash-checked URLs into /data/plugins -----
UNSIGNED="trooperthorn-swis-datasource"
while IFS= read -r spec; do
    [ -n "$spec" ] || continue
    id="${spec%%@*}"
    if [ -d "/data/plugins/${id}" ]; then
        log_info "Plugin ${id} is already installed; leaving it as is."
        continue
    fi
    log_info "Installing catalogue plugin ${spec}..."
    if [ "$spec" = "$id" ]; then
        as_grafana grafana cli --pluginsDir /data/plugins plugins install "$id" \
            || log_error "Installing ${spec} failed; continuing without it."
    else
        as_grafana grafana cli --pluginsDir /data/plugins plugins install "$id" "${spec#*@}" \
            || log_error "Installing ${spec} failed; continuing without it."
    fi
done < <(config_list 'plugins')

CUSTOM_COUNT="$(jq -r '(.custom_plugins // []) | length' "$OPTIONS_FILE")"
for ((i = 0; i < CUSTOM_COUNT; i++)); do
    name="$(jq -r ".custom_plugins[$i].name" "$OPTIONS_FILE")"
    url="$(jq -r ".custom_plugins[$i].url" "$OPTIONS_FILE")"
    sha="$(jq -r ".custom_plugins[$i].sha256" "$OPTIONS_FILE")"
    unsigned="$(jq -r ".custom_plugins[$i].unsigned // false" "$OPTIONS_FILE")"
    if ! [[ "$name" =~ ^[a-z0-9-]+$ ]]; then
        log_error "custom_plugins[$i].name '${name}' is not a plugin id; skipped."; continue
    fi
    if [ "$unsigned" = "true" ]; then UNSIGNED="${UNSIGNED},${name}"; fi
    if [ -d "/data/plugins/${name}" ]; then
        log_info "Custom plugin ${name} is already installed; leaving it as is."
        continue
    fi
    tmp="$(mktemp -d "$RUN_DIR/plugin.XXXXXX")"
    log_info "Downloading custom plugin ${name} from ${url}..."
    if curl -fsSL -m 300 -o "$tmp/plugin.zip" "$url"; then
        got="$(sha256sum "$tmp/plugin.zip" | cut -d' ' -f1)"
        if [ "$got" = "$sha" ]; then
            mkdir -p "$tmp/unpack" && unzip -q "$tmp/plugin.zip" -d "$tmp/unpack"
            # A zip holds either the plugin files at its root or one folder.
            src="$tmp/unpack"
            if [ ! -f "$src/plugin.json" ]; then
                src="$(find "$tmp/unpack" -mindepth 1 -maxdepth 2 -name plugin.json -printf '%h\n' | head -n1 || true)"
            fi
            if [ -n "$src" ] && [ -f "$src/plugin.json" ]; then
                mv "$src" "/data/plugins/${name}"
                chown -R "$GRAFANA_UID:$GRAFANA_GID" "/data/plugins/${name}"
                log_info "Installed custom plugin ${name} (sha256 verified)."
            else
                log_error "Custom plugin ${name}: no plugin.json in the zip; not installed."
            fi
        else
            log_error "Custom plugin ${name}: sha256 mismatch (expected ${sha}, got ${got}); not installed."
        fi
    else
        log_error "Custom plugin ${name}: download failed; not installed."
    fi
    rm -rf "$tmp"
done

# --- Render the configuration --------------------------------------------
ALLOW_EMBEDDING="$(config_value 'allow_embedding' 'false')"
LOG_LEVEL="$(config_value 'log_level' 'info')"
TERMINAL_ENABLED="$(config_value 'terminal_enabled' 'false')"
TERMINAL_FLAG=0; [ "$TERMINAL_ENABLED" = "true" ] && TERMINAL_FLAG=1

# sed with a delimiter the values cannot contain; the secrets are hex and
# base64 alphanumerics, the entry is a URL path.
render() {
    sed -e "s|%%ingress_entry%%|${INGRESS_ENTRY}|g" \
        -e "s|%%admin_password%%|${ADMIN_PASSWORD}|g" \
        -e "s|%%secret_key%%|${SECRET_KEY}|g" \
        -e "s|%%allow_embedding%%|${ALLOW_EMBEDDING}|g" \
        -e "s|%%unsigned_plugins%%|${UNSIGNED}|g" \
        -e "s|%%log_level%%|${LOG_LEVEL}|g" \
        -e "s|%%default_role%%|${DEFAULT_ROLE}|g" \
        -e "s|%%terminal_enabled%%|${TERMINAL_FLAG}|g" "$1" > "$2"
}
( umask 077; render /etc/grafana/grafana.ini.template "$RUN_DIR/grafana.ini" )
chown "$GRAFANA_UID:$GRAFANA_GID" "$RUN_DIR/grafana.ini"
render /etc/nginx/nginx.conf "$RUN_DIR/nginx.conf"
chmod 0644 "$RUN_DIR/nginx.conf"

# --- Access log rotation ---------------------------------------------------
RETENTION="$(config_value 'access_log_retention_days' '90')"
cat > "$RUN_DIR/logrotate.conf" <<LR
/data/log/nginx/access.log {
    daily
    rotate ${RETENTION}
    compress
    missingok
    notifempty
    dateext
    su grafana grafana
    postrotate
        [ -f ${RUN_DIR}/nginx.pid ] && kill -USR1 "\$(cat ${RUN_DIR}/nginx.pid)" 2>/dev/null || true
    endscript
}
LR
( while true; do sleep 86400; logrotate --state "/data/log/nginx/logrotate.state" "$RUN_DIR/logrotate.conf" || true; done ) &

# --- Start the services as the grafana user --------------------------------
export GF_PATHS_HOME=/usr/share/grafana
export HOME=/data/grafana

log_info "Starting nginx-light on 1337 (Ingress) and 3080 (API, only if mapped)."
as_grafana nginx -c "$RUN_DIR/nginx.conf" -g 'daemon off;' &
NGINX_PID=$!

TTYD_PID=""
if [ "$TERMINAL_ENABLED" = "true" ]; then
    export GRAFANA_TERM_RECORD="$(config_value 'terminal_session_recording' 'true')"
    export GRAFANA_TERM_IDLE_MINUTES="$(config_value 'terminal_idle_timeout_minutes' '30')"
    export GRAFANA_TERM_VERSION="$APP_VERSION"
    log_info "Starting the terminal server on 127.0.0.1:7681 (recording=${GRAFANA_TERM_RECORD}, idle=${GRAFANA_TERM_IDLE_MINUTES}m)."
    # -a lets nginx pass the Home Assistant username as the wrapper's
    # argument; the browser cannot set it because nginx rewrites the URL.
    as_grafana ttyd --interface 127.0.0.1 --port 7681 --writable --url-arg \
        /usr/local/bin/grafana_term_open &
    TTYD_PID=$!
else
    log_info "Terminal is off (terminal_enabled: false)."
fi

log_info "Starting Grafana 13 on 127.0.0.1:3000 (Ingress entry ${INGRESS_ENTRY})."
as_grafana grafana server --homepath=/usr/share/grafana --config="$RUN_DIR/grafana.ini" --packaging=docker &
GRAFANA_PID=$!

shutdown() {
    log_info "Stopping..."
    kill -TERM "$GRAFANA_PID" 2>/dev/null || true
    [ -n "$TTYD_PID" ] && kill -TERM "$TTYD_PID" 2>/dev/null || true
    kill -QUIT "$NGINX_PID" 2>/dev/null || true
    wait "$GRAFANA_PID" 2>/dev/null || true
    exit 0
}
trap shutdown TERM INT

# If Grafana or nginx dies the app should restart, which the Supervisor does
# when this script exits non-zero.
while true; do
    if ! kill -0 "$GRAFANA_PID" 2>/dev/null; then log_error "Grafana exited."; exit 1; fi
    if ! kill -0 "$NGINX_PID" 2>/dev/null; then log_error "nginx exited."; exit 1; fi
    sleep 5 & wait $!
done
