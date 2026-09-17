# Unifi Protect data source

A Grafana data source plugin for a Unifi Protect console's local
Integration API, built into the `ha_app_grafana` image from this directory
rather than distributed separately (see
[docs/decisions.md](../../../docs/decisions.md) in the repository root).

The client, its base path (`/proxy/protect/integration/v1`), auth header
(`X-API-KEY`), and field candidate lists are taken from
`trooperthorn/ha_int_soc`'s `unifi.py` and its
`docs/UNIFI-LOCAL-API-CONTRACT.md`, which verified them against Ubiquiti's
own Protect 7.2.105 OpenAPI/Postman artifacts and a live controller — the
same source `trooperthorn-unifinetwork-datasource` was built from.

## Cameras only, deliberately

This plugin has exactly one series: **Cameras**, one row per camera (id,
name, IP, MAC, recording state, last ring, channel count, online state,
and a console deep link). It does not offer an events/detections series,
because there isn't a REST one to offer: HA SOC's own contract
verification searched all of Protect 7.2.105's documented paths and found
no historical `/events`, `/detections`, or `/alarms` route. Live events
exist only as the persistent WebSocket subscription `GET
/subscribe/events`, which HA SOC consumes by reading Home Assistant's own
loaded `unifiprotect` integration's in-memory buffer instead of calling
Protect's API directly for history. A Grafana backend plugin answers one
HTTP request per query and holds no state between them, so it has no
honest way to offer that series either; claiming one anyway would mean
either fabricating data or silently maintaining a background WebSocket
connection an operator never asked for. See `docs/decisions.md` in the
repository root for the full reasoning.

`isRecording` is read as a plain boolean where the firmware exposes it,
and falls back to `recordingSettings.mode` (`always`/`detections`/`never`)
otherwise, the same dual handling `unifi.py`'s `_normalize_camera` uses,
proved by this plugin's own tests against both shapes.

## Layout

- `pkg/`: the Go backend (`grafana-plugin-sdk-go`). `pkg/plugin/client.go`
  is the only file that talks to the console; `pkg/plugin/normalize.go`
  resolves the candidate-key fields into a typed row; `pkg/plugin/datasource.go`
  turns those into a Grafana data frame.
- `src/`: the TypeScript/React frontend (config editor: console host, TLS
  verification, API key; the query editor has nothing to configure, since
  there is only one series).

## Building

```bash
go build ./...              # backend
go test ./...                 # backend tests (mock HTTP server, no live console needed)
npm install && npm run build      # frontend, into dist/
```

The repository's `grafana/Dockerfile` builds both for each `BUILD_ARCH` and
bundles the result under `/opt/grafana-app/plugins-bundled/trooperthorn-unifiprotect-datasource`,
the same way it builds the other bundled plugins.
