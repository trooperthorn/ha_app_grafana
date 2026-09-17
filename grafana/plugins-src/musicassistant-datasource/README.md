# Music Assistant data source

A Grafana data source plugin for [Music
Assistant](https://github.com/music-assistant/server), built into the
`ha_app_grafana` image from this directory rather than distributed
separately (see [docs/decisions.md](../../../docs/decisions.md) in the
repository root).

It queries `POST /api` (Music Assistant's HTTP JSON-RPC endpoint) with the
`players/all` command, authenticated with a long-lived API token created
once from the Music Assistant web UI or its `auth/token/create` command.
Each query returns one of:

- **Players**: one row per registered player: id, name, availability,
  powered, playback state, volume level, elapsed time.
- **Now playing**: one row per player that currently has media loaded:
  title, artist, album, duration. Players with nothing loaded are omitted.

Music Assistant's primary API is a WebSocket (`/ws`) with the same
`command`/`args`/`result` envelope; this plugin uses the HTTP variant of
that same envelope instead, since a Grafana backend plugin issuing one
request per query does not need a persistent connection.

## Layout

- `pkg/`: the Go backend (`grafana-plugin-sdk-go`). `pkg/plugin/client.go`
  is the only file that talks to Music Assistant; `pkg/plugin/datasource.go`
  turns its response into Grafana data frames.
- `src/`: the TypeScript/React frontend (config editor: server URL and API
  token; query editor: series).

## Building

```bash
go build ./...              # backend
go test ./...                # backend tests (mock HTTP server, no live server needed)
npm install && npm run build     # frontend, into dist/
```

The repository's `grafana/Dockerfile` builds both for each `BUILD_ARCH` and
bundles the result under `/opt/grafana-app/plugins-bundled/trooperthorn-musicassistant-datasource`,
the same way it builds the SWIS and Technitium DNS plugins.
