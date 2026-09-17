# Changelog

## Unreleased

Added a Technitium DNS Server data source, built into the image the same
way SWIS is: query volume, response totals, and top clients/domains/blocked
domains from `/api/dashboard/stats/get`. New options `technitium_url` and
`technitium_api_token` provision it automatically when both are set.

## 2026.09.16.1

First release. Grafana 13.2.2 (OSS) behind Home Assistant Ingress only:
per-user roles from Home Assistant usernames, no login form, no default
password, random per-install secrets, no Home Assistant directories mapped
in, telemetry off, the SolarWinds SWIS data source built into the image,
hash-checked custom plugins, an optional recorded terminal for
administrators that reaches nothing beyond the container, an optional
token-only API port, a JSON access log naming the Home Assistant user, and
a custom AppArmor profile.
