# Home Assistant data source

A Grafana data source plugin for a Home Assistant instance's own core
WebSocket API, built into the `ha_app_grafana` image from this directory
rather than distributed separately (see
[docs/decisions.md](../../../docs/decisions.md) in the repository root).

## Why WebSocket, not REST

Home Assistant's REST API already serves entity state (`GET /api/states`)
and raw state history (`GET /api/history/period`), so this plugin does not
duplicate those. It exists for the two things that REST does not cover at
all:

- **Long-term statistics** (`recorder/statistics_during_period`): the
  recorder's downsampled mean/min/max/sum series behind the History and
  Energy dashboards. WebSocket-only; there is no REST equivalent.
- **System health** (`system_health/info`): per-integration health facts
  (versions, connectivity, disk/database size, and so on). Also
  WebSocket-only, and its own protocol: HA confirms the subscription with a
  bare success result, then streams `initial`/`update`/`finish` events on
  the same connection rather than answering once. This plugin's client
  reads that whole sequence to completion before returning.

Both are documented at
[developers.home-assistant.io/docs/api/websocket](https://developers.home-assistant.io/docs/api/websocket)
and in Home Assistant core's own
`homeassistant/components/recorder/websocket_api.py` and
`homeassistant/components/system_health/__init__.py`.

## Connection

One WebSocket connection per query: a Grafana backend plugin answers one
HTTP request per query and holds no state between them, so there is no
benefit to keeping a persistent connection open, and doing so would need
its own reconnect/keepalive logic this plugin has no reason to carry. Each
connection completes the standard `auth_required`/`auth`/`auth_ok`
handshake with a long-lived access token (created under your Home
Assistant profile's Security tab), the same credential every HA
integration and the frontend itself uses.

## Layout

- `pkg/`: the Go backend (`grafana-plugin-sdk-go` + `gorilla/websocket`).
  `pkg/plugin/client.go` is the only file that talks to Home Assistant;
  `pkg/plugin/datasource.go` turns its responses into Grafana data frames.
- `src/`: the TypeScript/React frontend (config editor: instance URL, TLS
  verification, access token; query editor: series, statistic ids, period).

## Building

```bash
go build ./...              # backend
go test ./...                 # backend tests (mock WebSocket server, no live instance needed)
npm install && npm run build      # frontend, into dist/
```

The repository's `grafana/Dockerfile` builds both for each `BUILD_ARCH` and
bundles the result under `/opt/grafana-app/plugins-bundled/trooperthorn-homeassistant-datasource`,
the same way it builds the other bundled plugins.
