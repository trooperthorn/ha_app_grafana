# Changelog

## Unreleased

Fixed: a `/data` bind mount that refuses ownership changes (rootless
Docker/Podman without an idmapped mount, some network/virtualized
filesystem shares) used to make `run.sh` exit immediately on `chown` with
nothing but "Permission denied", which the Supervisor then restart-looped
forever. It now continues when the path is already writable by the
grafana user, and otherwise fails once with a diagnostic. See
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

## 2026.09.16.1

First release. Grafana 13.2.2 (OSS) behind Home Assistant Ingress only:
per-user roles from Home Assistant usernames, no login form, no default
password, random per-install secrets, no Home Assistant directories mapped
in, telemetry off, the SolarWinds SWIS data source built into the image,
hash-checked custom plugins, an optional recorded terminal for
administrators that reaches nothing beyond the container, an optional
token-only API port, a JSON access log naming the Home Assistant user, and
a custom AppArmor profile.
