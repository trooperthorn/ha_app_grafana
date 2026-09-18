# Changelog

## 2026.09.17.5

Docs only: the data source setup steps gave LAN and mDNS addresses for
Home Assistant, Technitium DNS and Music Assistant, none of which are
reachable from inside this app (no host networking). They now give the
Supervisor-network hostname for Home Assistant core and a Technitium app
on the same host, and the Supervisor bridge gateway for the
host-networked Music Assistant app.

## 2026.09.17.4

Fixed: the Ingress panel showed only a blank page with the browser's
broken-page icon. Grafana's `allow_embedding = false` (this app's default)
makes Grafana send `X-Frame-Options: deny`, and `deny` refuses every
frame, including the same-origin iframe Home Assistant uses for every
Ingress panel. Grafana's log showed each page load served with 200 and
then nothing, because the browser never ran the page. Grafana now runs
with `allow_embedding = true` (it is reachable only through nginx on
loopback) and nginx sends `X-Frame-Options: SAMEORIGIN` on the Ingress
port unless the `allow_embedding` option is on, in which case no header
is sent. The option keeps its meaning; the smoke test now checks the
header. See docs/decisions.md.

## 2026.09.17.3

Fixed: Grafana itself was exiting with `Error: ✗ unable to open database
file (14)` on every start under the Supervisor, right after the main
database connected successfully. `grafana.ini.template` set `wal = true`,
overriding Grafana's own upstream default (`false`) with no documented
reason in this repository. Grafana 13's unified storage subsystem opens a
second, independent SQLite connection pool to the same `grafana.db`
rather than reusing the first one, and WAL mode requires every connection
to coordinate through a shared-memory index file - a documented source of
exactly this error and error code once a second pool is involved. `wal`
is now `false`, matching Grafana's own default; SQLite converts an
existing WAL-mode database file back to a rollback journal automatically
on the next successful connection, so no manual migration is needed. See
docs/decisions.md.

## 2026.09.17.2

Fixed: the bundled AppArmor profile granted only read on the `/data` and
`/run/grafana-app` directory entries themselves (`/data/ r,`), separate
from the `rwk` granted on their contents (`/data/** rwk,`). `run.sh` chowns
those two directories directly, not only what is inside them, so every
start was denied by the app's own profile and misreported as a host/mount
ownership problem - the previous fix's diagnostics were themselves correct
(normal ownership, `CAP_CHOWN` present) but nothing pointed at AppArmor
specifically. Both directory entries are now `rw`.

Added a "Query logs" series to the Technitium DNS data source: reads the
per-request log stored by the "Query Logs (Sqlite)" DNS app (or its MySQL,
PostgreSQL, or SQL Server equivalents, which share the same API) over
Technitium's `/api/logs/query` endpoint, using the dashboard's own time
range and optional domain/client filters, so an existing installation does
not need a separate time series database for DNS query history. New option
`technitium_querylogs_app_name` is only needed if that app was installed
under a non-default name.

## 2026.09.17.1

Fixed: a `/data` bind mount that refuses ownership changes (rootless
Docker/Podman without an idmapped mount, some network/virtualized
filesystem shares) used to make `run.sh` exit immediately on `chown` with
nothing but "Permission denied", which the Supervisor then restart-looped
forever. It now continues when the path is already writable by the
grafana user, and otherwise fails once with a diagnostic. With
`log_level: debug`, that same failure also prints the launcher's own
diagnostics (id, path/parent ownership and mode, mount entry, effective
capabilities) rather than only Grafana's own debug logging. See
docs/operations.md.

Added a Technitium DNS Server data source, built into the image the same
way SWIS is: query volume, response totals, and top clients/domains/blocked
domains from `/api/dashboard/stats/get`. New options `technitium_url` and
`technitium_api_token` provision it automatically when both are set.

Added a Music Assistant data source, built in the same way: player state
(power, playback, volume) and now-playing info from its `POST /api`
JSON-RPC endpoint. New options `musicassistant_url` and
`musicassistant_api_token` provision it automatically when both are set.

Added a Unifi Network data source, built in the same way: connected
clients, network devices, and derived WAN status from a controller's local
Integration API, using the same base path, auth header, pagination and
field candidates verified in `trooperthorn/ha_int_soc`. New options
`unifi_network_host`, `unifi_network_api_key`, and
`unifi_network_verify_ssl` provision it automatically when host and key
are both set.

Added a Unifi Protect data source, built the same way: a camera inventory
(recording state, online state, channel count, console deep link) from a
console's local Integration API. Cameras only, deliberately: Protect's API
has no historical events/detections REST route, only a live WebSocket
subscription a one-request-per-query backend plugin cannot honestly
offer. New options `unifi_protect_host`, `unifi_protect_api_key`, and
`unifi_protect_verify_ssl` provision it automatically when host and key
are both set.

Added a Home Assistant data source: long-term statistics
(`recorder/statistics_during_period`) and system health
(`system_health/info`) over this instance's own core WebSocket API, since
neither has a REST equivalent. New options `homeassistant_url`,
`homeassistant_access_token`, and `homeassistant_verify_ssl` provision it
automatically when both url and token are set.

Added an HA SOC data source for
[trooperthorn/ha_int_soc](https://github.com/trooperthorn/ha_int_soc)
installs: the whole-install security posture score, per-user risk, and
the tamper-evident audit log, read over the same Home Assistant WebSocket
API (HA SOC has no API of its own). New options `hasoc_url`,
`hasoc_access_token`, and `hasoc_verify_ssl` provision it automatically
when both url and token are set; the token must belong to a Home
Assistant admin user.

## 2026.09.16.1

First release. Grafana 13.2.2 (OSS) behind Home Assistant Ingress only:
per-user roles from Home Assistant usernames, no login form, no default
password, random per-install secrets, no Home Assistant directories mapped
in, telemetry off, the SolarWinds SWIS data source built into the image,
hash-checked custom plugins, an optional recorded terminal for
administrators that reaches nothing beyond the container, an optional
token-only API port, a JSON access log naming the Home Assistant user, and
a custom AppArmor profile.
