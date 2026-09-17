# Changelog

## Unreleased

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
