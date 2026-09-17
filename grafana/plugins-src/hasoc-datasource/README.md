# HA SOC data source

A Grafana data source plugin for [HA SOC](https://github.com/trooperthorn/ha_int_soc)
(`trooperthorn/ha_int_soc`), a Home Assistant custom integration that adds
a security operations center panel to a Home Assistant install. Built
into the `ha_app_grafana` image from this directory rather than
distributed separately (see [docs/decisions.md](../../../docs/decisions.md)
in the repository root).

## Why WebSocket, not REST

HA SOC has no HTTP view of its own: every `ha_soc/*` command it registers
(`websocket_api.py`) rides on Home Assistant's own core WebSocket API, the
same one `trooperthorn-homeassistant-datasource` uses. This plugin talks
to that Home Assistant instance directly, not to HA SOC as a separate
service, and authenticates with a long-lived access token for a Home
Assistant admin user: HA SOC's own `require_soc_access` gate rejects a
non-admin token outright, and by default (`access_level: owner_only`)
requires the account owner specifically.

## Series

- **Posture**: one row, HA SOC's whole-install security score (`ha_soc/risk/posture`)
  - score, letter grade, whether it's still provisional (not every
    scoring term has computed once yet), and the five weighted factors
    behind it (`p_user`, `p_vuln`, `p_misconfig`, `p_integration`,
    `p_detection`), matching `RiskEngine.async_compute_posture` in
    `trooperthorn/ha_int_soc`'s `risk.py`.
- **Risk**: one row per Home Assistant user (`ha_soc/risk/list`) — score,
  risk band, factor count, and the single highest-point factor name (the
  full factor list, with per-factor point values and details, is not
  flattened into the table; query HA SOC's own panel for that).
- **Audit**: one row per tamper-evident audit log event
  (`ha_soc/audit/query`), bounded to the dashboard's own time range —
  timestamp, category, user, domain/service, entity ids, source IP, and
  the hash chain's sequence number. This reads the log; it does not call
  `ha_soc/audit/verify_chain` to prove the chain is unbroken, since that
  is a one-shot integrity check, not something a time-series query
  answers.

## Connection

One WebSocket connection per query, the same pattern
`trooperthorn-homeassistant-datasource` uses and for the same reason: a
Grafana backend plugin answers one request per query and holds no state
between them, so there is no benefit to a persistent connection.

## Layout

- `pkg/`: the Go backend (`grafana-plugin-sdk-go` + `gorilla/websocket`).
  `pkg/plugin/client.go` is the only file that talks to Home Assistant;
  `pkg/plugin/types.go` mirrors the `ha_soc/*` result shapes;
  `pkg/plugin/datasource.go` turns those into Grafana data frames.
- `src/`: the TypeScript/React frontend (config editor: instance URL, TLS
  verification, access token; query editor: series, audit limit).

## Building

```bash
go build ./...              # backend
go test ./...                 # backend tests (mock WebSocket server, no live instance needed)
npm install && npm run build      # frontend, into dist/
```

The repository's `grafana/Dockerfile` builds both for each `BUILD_ARCH` and
bundles the result under `/opt/grafana-app/plugins-bundled/trooperthorn-hasoc-datasource`,
the same way it builds the other bundled plugins.
