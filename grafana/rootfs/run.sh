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
# Gated on the log_level option (set below, before it is first needed), not
# on Grafana's own [log] level: this is the launcher's own diagnostic
# output, for a problem in run.sh itself rather than in Grafana.
log_debug() {
    [ "${LOG_LEVEL:-info}" = "debug" ] || return 0
    printf '[%s] DEBUG: %s\n' "$(date -u '+%Y-%m-%d %H:%M:%S')" "$1" >&2
}

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

# sed's own delimiter ("|", used below) and its "&" backreference both need
# escaping in an operator-supplied value before it goes on the replacement
# side of a substitution.
sed_escape() { printf '%s' "$1" | sed -e 's/[&|\\]/\\&/g'; }

as_grafana() {
    setpriv --reuid="$GRAFANA_UID" --regid="$GRAFANA_GID" --clear-groups --inh-caps=-all "$@"
}

# At log_level: debug, dump the context around a failed ownership/permission
# change on a path: who this process actually is, the path's own and its
# parent's ownership/mode, what filesystem and mount options it sits on,
# and this process's effective capabilities. This is exactly what
# distinguishes the causes docs/operations.md lists (rootless Docker/Podman
# without an idmapped mount vs. a network/virtualized filesystem vs. a
# capability actually dropped) without guessing from the bare
# "Permission denied" alone.
debug_dump_path() {
    local path="$1"
    log_debug "-- ownership diagnostics for ${path} --"
    log_debug "id: $(id 2>&1)"
    log_debug "stat ${path}: $(stat -c 'owner=%u:%g mode=%a type=%F' "$path" 2>&1)"
    local parent; parent="$(dirname -- "$path")"
    log_debug "stat ${parent}: $(stat -c 'owner=%u:%g mode=%a type=%F' "$parent" 2>&1)"
    if command -v findmnt >/dev/null 2>&1; then
        log_debug "findmnt ${path}: $(findmnt -T "$path" -o TARGET,SOURCE,FSTYPE,OPTIONS 2>&1 | tr '\n' ' ')"
    else
        log_debug "mount entry: $(mount 2>&1 | grep -F " ${path} " || echo 'not found (path may be a subdirectory of a mounted volume)')"
    fi
    log_debug "capabilities: $(grep -E '^Cap(Eff|Prm|Bnd)' /proc/self/status 2>&1 | tr '\n' ' ')"
}

# chown -R one or more paths to the grafana user, tolerating a host/mount
# that refuses ownership changes altogether (rootless Docker/Podman without
# an idmapped mount, some virtualized or network filesystems used for a
# bind-mounted /data). A refusal is fatal only if the first path given is
# not already usable by the grafana user once checked directly (the rest
# are assumed to share its mount); either way this fails once with a
# diagnostic rather than exiting via errexit on a bare "Permission denied"
# that the Supervisor then restart-loops forever.
chown_or_verify() {
    local err
    if err="$(chown -R "$GRAFANA_UID:$GRAFANA_GID" "$@" 2>&1)"; then
        return 0
    fi
    if as_grafana test -w "$1" -a -x "$1"; then
        log_warning "Could not chown ${1} and possibly others (${err##*: }); ${1} is already writable by the grafana user, continuing."
        debug_dump_path "$1"
        return 0
    fi
    log_error "Could not chown ${1} (${err##*: }), and it is not already writable by uid ${GRAFANA_UID}."
    log_error "This container cannot fix ownership on this host/mount by itself. This usually means /data is a bind mount from a Docker mode or filesystem that refuses ownership changes (rootless Docker/Podman without an idmapped mount, some network or virtualized filesystem shares). From the host, either: chown -R 472:472 <the host path mapped to /data>, or use a plain Docker-managed named volume instead of a bind mount, or enable idmapped mounts for the bind mount."
    log_error "Set log_level: debug and restart to see the full diagnostic (id, ownership/mode, mount, capabilities) this failure produced."
    debug_dump_path "$1"
    exit 1
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

# Read once, early: log_debug (and anything else in this script) needs it
# from the very first thing that can go wrong, not just the render() step
# further down that used to be the only reader.
LOG_LEVEL="$(config_value 'log_level' 'info')"
log_debug "log_level is debug; the launcher's own diagnostics (not just Grafana's) are on for this start."

# --- /data layout, owned by the grafana user ------------------------------
# The Supervisor mounts /data root-owned; whatever its mode, the grafana user
# has to traverse it, and nobody else in this container needs to.
chown_or_verify /data
chmod 0750 /data 2>/dev/null || true
mkdir -p /data/grafana /data/plugins \
         /data/provisioning/datasources /data/provisioning/dashboards /data/provisioning/plugins \
         /data/provisioning/notifiers /data/provisioning/alerting /data/provisioning/access-control \
         /data/log/grafana /data/log/nginx /data/terminal/sessions "$SECRETS_DIR" \
         "$RUN_DIR" "$RUN_DIR/client_body" "$RUN_DIR/proxy" "$RUN_DIR/fastcgi" "$RUN_DIR/uwsgi" "$RUN_DIR/scgi"
chown_or_verify /data/grafana /data/plugins /data/provisioning /data/log /data/terminal "$SECRETS_DIR" "$RUN_DIR"
chmod 0700 "$SECRETS_DIR" /data/terminal /data/terminal/sessions 2>/dev/null || true

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
# Non-fatal like chown_or_verify above, for the same reason: on a mount
# that refuses ownership changes, root itself already has these values in
# hand below, and only the terminal's "read the admin password" convenience
# needs the grafana user able to read the file back later.
chown "$GRAFANA_UID:$GRAFANA_GID" "$SECRETS_DIR/secret_key" "$SECRETS_DIR/admin_password" 2>/dev/null \
    || log_warning "Could not chown the generated secrets to the grafana user; the terminal may not be able to read ${SECRETS_DIR}/admin_password back."
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
UNSIGNED="trooperthorn-swis-datasource,trooperthorn-technitiumdns-datasource,trooperthorn-musicassistant-datasource,trooperthorn-unifinetwork-datasource,trooperthorn-unifiprotect-datasource,trooperthorn-homeassistant-datasource,trooperthorn-hasoc-datasource"
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
                chown -R "$GRAFANA_UID:$GRAFANA_GID" "/data/plugins/${name}" 2>/dev/null \
                    || log_warning "Could not chown custom plugin ${name} to the grafana user; it may fail to load."
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
# LOG_LEVEL was already read above, right after the options file was found,
# so log_debug works from the very first thing that can go wrong.
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

# --- Technitium DNS data source provisioning -------------------------------
# Both options must be set or the bundled data source is left unprovisioned;
# clearing either one removes the file so Grafana deprovisions it too.
TECHNITIUM_PROVISIONING="/data/provisioning/datasources/technitium.yaml"
TECHNITIUM_URL="$(config_value 'technitium_url' '')"
TECHNITIUM_API_TOKEN="$(config_value 'technitium_api_token' '')"
if [ -n "$TECHNITIUM_URL" ] && [ -n "$TECHNITIUM_API_TOKEN" ]; then
    ( umask 077
      sed -e "s|%%technitium_url%%|$(sed_escape "$TECHNITIUM_URL")|g" \
          -e "s|%%technitium_api_token%%|$(sed_escape "$TECHNITIUM_API_TOKEN")|g" \
          /etc/grafana/provisioning-datasources/technitium.yaml.template > "$TECHNITIUM_PROVISIONING" )
    chown "$GRAFANA_UID:$GRAFANA_GID" "$TECHNITIUM_PROVISIONING" 2>/dev/null \
        || log_warning "Could not chown the Technitium provisioning file to the grafana user."
    log_info "Provisioned the Technitium DNS data source (${TECHNITIUM_URL})."
else
    rm -f "$TECHNITIUM_PROVISIONING"
fi

# --- Music Assistant data source provisioning ------------------------------
# Both options must be set or the bundled data source is left unprovisioned;
# clearing either one removes the file so Grafana deprovisions it too.
MUSICASSISTANT_PROVISIONING="/data/provisioning/datasources/musicassistant.yaml"
MUSICASSISTANT_URL="$(config_value 'musicassistant_url' '')"
MUSICASSISTANT_API_TOKEN="$(config_value 'musicassistant_api_token' '')"
if [ -n "$MUSICASSISTANT_URL" ] && [ -n "$MUSICASSISTANT_API_TOKEN" ]; then
    ( umask 077
      sed -e "s|%%musicassistant_url%%|$(sed_escape "$MUSICASSISTANT_URL")|g" \
          -e "s|%%musicassistant_api_token%%|$(sed_escape "$MUSICASSISTANT_API_TOKEN")|g" \
          /etc/grafana/provisioning-datasources/musicassistant.yaml.template > "$MUSICASSISTANT_PROVISIONING" )
    chown "$GRAFANA_UID:$GRAFANA_GID" "$MUSICASSISTANT_PROVISIONING" 2>/dev/null \
        || log_warning "Could not chown the Music Assistant provisioning file to the grafana user."
    log_info "Provisioned the Music Assistant data source (${MUSICASSISTANT_URL})."
else
    rm -f "$MUSICASSISTANT_PROVISIONING"
fi

# --- Unifi Network data source provisioning --------------------------------
# Both host and api key must be set or the bundled data source is left
# unprovisioned; clearing either one removes the file so Grafana
# deprovisions it too.
UNIFI_NETWORK_PROVISIONING="/data/provisioning/datasources/unifinetwork.yaml"
UNIFI_NETWORK_HOST="$(config_value 'unifi_network_host' '')"
UNIFI_NETWORK_API_KEY="$(config_value 'unifi_network_api_key' '')"
UNIFI_NETWORK_VERIFY_SSL="$(config_value 'unifi_network_verify_ssl' 'false')"
if [ -n "$UNIFI_NETWORK_HOST" ] && [ -n "$UNIFI_NETWORK_API_KEY" ]; then
    ( umask 077
      sed -e "s|%%unifi_network_host%%|$(sed_escape "$UNIFI_NETWORK_HOST")|g" \
          -e "s|%%unifi_network_api_key%%|$(sed_escape "$UNIFI_NETWORK_API_KEY")|g" \
          -e "s|%%unifi_network_verify_ssl%%|$(sed_escape "$UNIFI_NETWORK_VERIFY_SSL")|g" \
          /etc/grafana/provisioning-datasources/unifinetwork.yaml.template > "$UNIFI_NETWORK_PROVISIONING" )
    chown "$GRAFANA_UID:$GRAFANA_GID" "$UNIFI_NETWORK_PROVISIONING" 2>/dev/null \
        || log_warning "Could not chown the Unifi Network provisioning file to the grafana user."
    log_info "Provisioned the Unifi Network data source (${UNIFI_NETWORK_HOST})."
else
    rm -f "$UNIFI_NETWORK_PROVISIONING"
fi

# --- Unifi Protect data source provisioning --------------------------------
# Both host and api key must be set or the bundled data source is left
# unprovisioned; clearing either one removes the file so Grafana
# deprovisions it too.
UNIFI_PROTECT_PROVISIONING="/data/provisioning/datasources/unifiprotect.yaml"
UNIFI_PROTECT_HOST="$(config_value 'unifi_protect_host' '')"
UNIFI_PROTECT_API_KEY="$(config_value 'unifi_protect_api_key' '')"
UNIFI_PROTECT_VERIFY_SSL="$(config_value 'unifi_protect_verify_ssl' 'false')"
if [ -n "$UNIFI_PROTECT_HOST" ] && [ -n "$UNIFI_PROTECT_API_KEY" ]; then
    ( umask 077
      sed -e "s|%%unifi_protect_host%%|$(sed_escape "$UNIFI_PROTECT_HOST")|g" \
          -e "s|%%unifi_protect_api_key%%|$(sed_escape "$UNIFI_PROTECT_API_KEY")|g" \
          -e "s|%%unifi_protect_verify_ssl%%|$(sed_escape "$UNIFI_PROTECT_VERIFY_SSL")|g" \
          /etc/grafana/provisioning-datasources/unifiprotect.yaml.template > "$UNIFI_PROTECT_PROVISIONING" )
    chown "$GRAFANA_UID:$GRAFANA_GID" "$UNIFI_PROTECT_PROVISIONING" 2>/dev/null \
        || log_warning "Could not chown the Unifi Protect provisioning file to the grafana user."
    log_info "Provisioned the Unifi Protect data source (${UNIFI_PROTECT_HOST})."
else
    rm -f "$UNIFI_PROTECT_PROVISIONING"
fi

# --- Home Assistant data source provisioning -------------------------------
# Both options must be set or the bundled data source is left unprovisioned;
# clearing either one removes the file so Grafana deprovisions it too.
HOMEASSISTANT_PROVISIONING="/data/provisioning/datasources/homeassistant.yaml"
HOMEASSISTANT_URL="$(config_value 'homeassistant_url' '')"
HOMEASSISTANT_ACCESS_TOKEN="$(config_value 'homeassistant_access_token' '')"
HOMEASSISTANT_VERIFY_SSL="$(config_value 'homeassistant_verify_ssl' 'false')"
if [ -n "$HOMEASSISTANT_URL" ] && [ -n "$HOMEASSISTANT_ACCESS_TOKEN" ]; then
    ( umask 077
      sed -e "s|%%homeassistant_url%%|$(sed_escape "$HOMEASSISTANT_URL")|g" \
          -e "s|%%homeassistant_access_token%%|$(sed_escape "$HOMEASSISTANT_ACCESS_TOKEN")|g" \
          -e "s|%%homeassistant_verify_ssl%%|$(sed_escape "$HOMEASSISTANT_VERIFY_SSL")|g" \
          /etc/grafana/provisioning-datasources/homeassistant.yaml.template > "$HOMEASSISTANT_PROVISIONING" )
    chown "$GRAFANA_UID:$GRAFANA_GID" "$HOMEASSISTANT_PROVISIONING" 2>/dev/null \
        || log_warning "Could not chown the Home Assistant provisioning file to the grafana user."
    log_info "Provisioned the Home Assistant data source (${HOMEASSISTANT_URL})."
else
    rm -f "$HOMEASSISTANT_PROVISIONING"
fi

# --- HA SOC data source provisioning ----------------------------------------
# Both options must be set or the bundled data source is left unprovisioned;
# clearing either one removes the file so Grafana deprovisions it too.
HASOC_PROVISIONING="/data/provisioning/datasources/hasoc.yaml"
HASOC_URL="$(config_value 'hasoc_url' '')"
HASOC_ACCESS_TOKEN="$(config_value 'hasoc_access_token' '')"
HASOC_VERIFY_SSL="$(config_value 'hasoc_verify_ssl' 'false')"
if [ -n "$HASOC_URL" ] && [ -n "$HASOC_ACCESS_TOKEN" ]; then
    ( umask 077
      sed -e "s|%%hasoc_url%%|$(sed_escape "$HASOC_URL")|g" \
          -e "s|%%hasoc_access_token%%|$(sed_escape "$HASOC_ACCESS_TOKEN")|g" \
          -e "s|%%hasoc_verify_ssl%%|$(sed_escape "$HASOC_VERIFY_SSL")|g" \
          /etc/grafana/provisioning-datasources/hasoc.yaml.template > "$HASOC_PROVISIONING" )
    chown "$GRAFANA_UID:$GRAFANA_GID" "$HASOC_PROVISIONING" 2>/dev/null \
        || log_warning "Could not chown the HA SOC provisioning file to the grafana user."
    log_info "Provisioned the HA SOC data source (${HASOC_URL})."
else
    rm -f "$HASOC_PROVISIONING"
fi

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
# Grafana's image sets GF_PATHS_* in its environment, and an environment
# variable outranks the configuration file, so the paths are set here as
# well or the database would land in the image's /var/lib/grafana and be
# lost on update.
export GF_PATHS_HOME=/usr/share/grafana
export GF_PATHS_DATA=/data/grafana
export GF_PATHS_LOGS=/data/log/grafana
export GF_PATHS_PLUGINS=/data/plugins
export GF_PATHS_PROVISIONING=/data/provisioning
export GF_PATHS_CONFIG="$RUN_DIR/grafana.ini"
export HOME=/data/grafana

log_info "Starting nginx-light on 1337 (Ingress) and 3080 (API, only if mapped)."
as_grafana nginx -c "$RUN_DIR/nginx.conf" -g 'daemon off;' &
NGINX_PID=$!

TTYD_PID=""
if [ "$TERMINAL_ENABLED" = "true" ]; then
    GRAFANA_TERM_RECORD="$(config_value 'terminal_session_recording' 'true')"
    GRAFANA_TERM_IDLE_MINUTES="$(config_value 'terminal_idle_timeout_minutes' '30')"
    GRAFANA_TERM_VERSION="$APP_VERSION"
    export GRAFANA_TERM_RECORD GRAFANA_TERM_IDLE_MINUTES GRAFANA_TERM_VERSION
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
    if [ -n "$TTYD_PID" ]; then kill -TERM "$TTYD_PID" 2>/dev/null || true; fi
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
