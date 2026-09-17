# Unifi Network data source

A Grafana data source plugin for a Unifi Network controller's local
Integration API, built into the `ha_app_grafana` image from this directory
rather than distributed separately (see
[docs/decisions.md](../../../docs/decisions.md) in the repository root).

The client, its base path (`/proxy/network/integration/v1`), auth header
(`X-API-KEY`), pagination shape, and field candidate lists are all taken
from `trooperthorn/ha_int_soc`'s `unifi.py`/`unifi_core.py` and its
`docs/UNIFI-LOCAL-API-CONTRACT.md`, which verified them against Ubiquiti's
own Network 10.4.57 OpenAPI/Postman artifacts and a live controller. This
plugin is a read-only subset of that work: it does not cover ACL rules,
firewall policies, or Wi-Fi broadcast configuration, which are HA SOC's
security-audit concern rather than a monitoring dashboard's.

Each query returns one of:

- **Clients**: one row per connected client (name, MAC, IPv4, VLAN, SSID,
  wired/wireless, uptime, last seen, rx/tx bytes).
- **Devices**: one row per network infrastructure device (name, MAC, IPv4,
  model, state, firmware-updatable, last seen, rx/tx bytes).
- **WAN**: a single row derived from the gateway device's uplink/statistics
  object (port, up, rx/tx rate, WAN IP). This is a trimmed form of
  `unifi.py`'s `_derive_wan`: it does not walk the fuller
  `interfaces`/`ports` array shapes that remain on that project's own
  unverified backlog.

Authentication is a read-only local Integration API key, created in the
controller's UI (Settings > Control Plane > Integrations), the same key
type `ha_int_soc` uses. TLS verification defaults off, matching a typical
self-signed console (also `ha_int_soc`'s default), and can be turned on
in the config editor for a controller with a real certificate.

## Layout

- `pkg/`: the Go backend (`grafana-plugin-sdk-go`). `pkg/plugin/client.go`
  is the only file that talks to the controller; `pkg/plugin/normalize.go`
  resolves the candidate-key fields into typed rows; `pkg/plugin/datasource.go`
  turns those into Grafana data frames.
- `src/`: the TypeScript/React frontend (config editor: controller host,
  TLS verification, API key; query editor: series).

## Building

```bash
go build ./...              # backend
go test ./...                 # backend tests (mock HTTP server, no live controller needed)
npm install && npm run build      # frontend, into dist/
```

The repository's `grafana/Dockerfile` builds both for each `BUILD_ARCH` and
bundles the result under `/opt/grafana-app/plugins-bundled/trooperthorn-unifinetwork-datasource`,
the same way it builds the other bundled plugins.
