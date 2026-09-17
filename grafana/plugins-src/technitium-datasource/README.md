# Technitium DNS data source

A Grafana data source plugin for [Technitium DNS
Server](https://technitium.com/dns/), built into the `ha_app_grafana` image
from this directory rather than distributed separately (see
[docs/decisions.md](../../../docs/decisions.md) in the repository root).

It queries `GET /api/dashboard/stats/get` on the server's own web console
origin, authenticated with a non-expiring API token (Technitium console:
Administration, Sessions, Create API Token). Each query returns one of:

- **Query volume**: a time series of the response totals Technitium's
  dashboard chart carries (total queries, no error, server failure, ...).
- **Top clients** / **Top domains** / **Top blocked domains**: a table of
  name and hit count.

The time window is one of Technitium's own duration buckets (last hour,
day, week, month, year), set per query, not the dashboard's own time
picker; the API does not offer an arbitrary range for these lists.

## Layout

- `pkg/`: the Go backend (`grafana-plugin-sdk-go`). `pkg/plugin/client.go`
  is the only file that talks to Technitium; `pkg/plugin/datasource.go`
  turns its response into Grafana data frames.
- `src/`: the TypeScript/React frontend (config editor: server URL and API
  token; query editor: series and time window).

## Building

```bash
go build ./...            # backend
go test ./...              # backend tests (mock HTTP server, no live DNS server needed)
npm install && npm run build   # frontend, into dist/
```

The repository's `grafana/Dockerfile` builds both for each `BUILD_ARCH` and
bundles the result under `/opt/grafana-app/plugins-bundled/trooperthorn-technitiumdns-datasource`,
the same way it builds the SWIS plugin.
