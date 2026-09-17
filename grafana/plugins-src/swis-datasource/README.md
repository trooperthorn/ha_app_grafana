# SolarWinds SWIS data source for Grafana

A native Grafana data source plugin for SolarWinds Observability Self-Hosted (Orion). You
type SWQL in the panel editor, Grafana's time range and dashboard variables are applied
for you, the result comes back as a typed data frame, and verbs can be invoked from a
dashboard through an allowlist the administrator controls. It works the same on Grafana
OSS and Grafana Enterprise, because it uses nothing beyond the open plugin SDK.

```text
Grafana (any panel, alert rule, or variable)
   │  SWQL + time range           Grafana's plugin protocol (gRPC, local)
   ▼
gpx_swis  (this plugin's backend, a Go process Grafana starts)
   │  POST /Query with bound parameters      HTTPS, port 17774
   │  POST /Invoke/{Entity}/{Verb}           only for allowlisted verbs
   ▼
SWIS on the SolarWinds Platform server
```

The Orion credential lives in Grafana's encrypted secure settings and only the backend
process ever sees it. Nothing runs between Grafana and SWIS.

## What you get

| Feature | How |
| --- | --- |
| SWQL query editor with syntax highlighting | The panel editor is a code editor; Ctrl+S or leaving the editor runs the query |
| Time range applied to the query | `$__timeFilter(alias.Column)`, `$__timeFrom()` and `$__timeTo()` expand to **bound parameters**, so the range never becomes literal text |
| Dashboard variables | `$node`, `${node}`, `${ifaces:csv}` are expanded by Grafana before the statement is sent |
| Variables driven by SWQL | A variable's query is a SWQL statement; alias the label `__text` and the value `__value` |
| Table and time series formats | Table returns rows as-is. Time series needs a `DateTime` column; string columns become series labels, one series per distinct value |
| Typed columns | Numbers, booleans, timestamps and strings come back as their own field types, in SELECT-list order, with SWIS's bare timestamps read as UTC |
| Alerting | Grafana alert rules can be built on any query, because the plugin is a backend data source |
| Health check | "Save & test" runs one query against `Orion.Engines` and reports the engine name and platform version it found |
| Invoke | `POST /api/datasources/uid/<uid>/resources/invoke/<Entity>/<Verb>` with a positional JSON array, gated three ways (below) |
| Example queries | The editor offers ten starting points; every one is validated against the extracted 2026.2 schema on each build of this repository |
| Sample dashboard | [dashboards/solarwinds-overview.json](dashboards/solarwinds-overview.json): status, down nodes, alerts, alert rate, fullest volumes, and per-node CPU, latency and interface traffic |

## Install

The plugin is not in the Grafana catalogue and is not signed. Both Grafana OSS and
Enterprise load an unsigned plugin only when its id is listed in the
`allow_loading_unsigned_plugins` setting. That is the one configuration step that differs
from a catalogue plugin, and it is the same on both editions.

### 1. Build

Needs Go 1.26.5 or newer (`go.mod` states it) and Node 22. From this directory:

```bash
go test ./pkg/...
npm ci
npm run build
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o dist/gpx_swis_linux_amd64 ./pkg
```

or, from the repository root, `make grafana-plugin`. The result is `dist/`: `plugin.json`,
`module.js`, the logo and the backend binary. Build the binary for the platform Grafana
runs on; the name pattern is `gpx_swis_<os>_<arch>`, so a Windows host wants
`GOOS=windows GOARCH=amd64 go build -o dist/gpx_swis_windows_amd64.exe ./pkg`, and an ARM
Linux host wants `GOARCH=arm64`. Grafana picks the binary that matches its own platform.

### 2. Put it where Grafana looks

Copy `dist/` to `<plugins directory>/trooperthorn-swis-datasource/`. The plugins directory
is `/var/lib/grafana/plugins` on the packaged builds and in the official container.

### 3. Allow it to load

Configuration file (`grafana.ini` or `/etc/grafana/grafana.ini`):

```ini
[plugins]
allow_loading_unsigned_plugins = trooperthorn-swis-datasource
```

Container:

```bash
docker run -d -p 3000:3000 \
  -v "$PWD/dist:/var/lib/grafana/plugins/trooperthorn-swis-datasource" \
  -e GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS=trooperthorn-swis-datasource \
  grafana/grafana
```

`grafana/grafana` is the OSS image. `grafana/grafana-enterprise` takes the same flag and
the same volume. Restart Grafana after either change; it lists the plugin under
Administration, Plugins and data, as "SolarWinds SWIS".

If you would rather not carry the unsigned-plugin setting, Grafana's
[private plugin signing](https://grafana.com/developers/plugin-tools/publish-a-plugin/sign-a-plugin)
signs a plugin for the root URLs you name. It needs a Grafana Cloud account for the
signing key, but the signed plugin then loads on any OSS or Enterprise instance whose
root URL matches. `npm run sign` in this directory runs it.

### Home Assistant

[trooperthorn/ha_app_grafana](https://github.com/trooperthorn/ha_app_grafana) ships this
plugin inside a hardened Grafana for Home Assistant, built from a pinned commit of this
repository, with no unsigned-plugin setting to manage.

## Configure the data source

Connections, Data sources, Add data source, "SolarWinds SWIS".

| Field | Notes |
| --- | --- |
| Orion server | Hostname or IP, nothing else |
| REST port | 17774 from platform release 2023.1 onward. 17778 was the REST port through 2022.4.1 and is deprecated. 17777 is SOAP and will not answer |
| Username, Password | An Orion account. Give Grafana its own read-only account with the narrowest limitation that still shows what the dashboards need; account limitations apply to everything it reads |
| CA certificate | SWIS ships with a self-signed certificate. Paste that certificate (or the CA that issued it) here and verification stays on. `openssl s_client -connect orion.example.com:17774 -showcerts </dev/null` prints it |
| Ignore certificate name | Needed with the stock certificate, see below. The chain is still verified against the pasted certificate; only the name check is dropped |
| Skip TLS verification | Lab only. It lets anything on the network path read the credentials Grafana sends |
| Max rows per query | Rows beyond this are dropped and the panel shows a warning. Default 10000. It is a safety net, not a substitute for `TOP n` |
| Timeout | Seconds per request. Default 60 |
| Allowed verbs | One `Entity.Verb` per line. Empty means Invoke is off, which is the default |

### The self-signed certificate

Pasting the stock SWIS certificate into the CA field is not enough on its own, and the
reason is worth knowing because it looks like the pin is being ignored. The certificate
SWIS generates is issued to a fixed name and carries no subject alternative names, so a
verifier that trusts it still fails on the second check every TLS client makes, which is
that the name on the certificate matches the host it was asked to connect to. Go, which
this backend is written in, refuses a certificate with no subject alternative names
outright, whatever the common name says.

So there are three settings, and they are not the same:

| Setting | Chain checked | Name checked | Refuses a different certificate on the same host |
| --- | --- | --- | --- |
| CA certificate only | Yes | Yes | Yes. Fails against the stock certificate on the name |
| CA certificate + Ignore certificate name | Yes | No | **Yes** |
| Skip TLS verification | No | No | No |

The middle row is certificate pinning, and it is what the stock certificate needs. It is
not "verification off": the server has to present the exact certificate you pasted (or
one issued by it), so an interception between Grafana and the Orion server, which is the
host holding credentials for the rest of the estate, still fails loudly. Turn on the
third row only in a lab. The same ordering, and the OpenSSL commands to export and
fingerprint the certificate before trusting it, are in
[connecting.md](../../docs/swis/connecting.md#tls-and-the-self-signed-certificate).

If the health check fails on the name with the pin in place, the message says which
switch to turn on. If it fails with "not the pinned one", the server is presenting a
different certificate from the one you pasted: either it was replaced, or something is in
the path.

"Save & test" runs `SELECT TOP 1 e.EngineID, e.ServerName, e.EngineVersion FROM Orion.Engines e`
and shows what it found. If it authenticates but sees no engine, the message says so and
points at the account limitation, because that is the usual cause.

### Provisioning

For a Grafana that is configured from files, the same settings go in a provisioning file.
Values are read from Grafana's environment so nothing real is committed:

```yaml
apiVersion: 1
datasources:
  - name: SolarWinds SWIS
    uid: swis
    type: trooperthorn-swis-datasource
    access: proxy
    jsonData:
      host: ${SWIS_HOST}
      port: 17774
      username: ${SWIS_USER}
      tlsSkipVerify: false
      tlsIgnoreHostname: true
      maxRows: 10000
      timeoutSeconds: 60
      invokeAllow:
        - Orion.Nodes.PollNow
    secureJsonData:
      password: ${SWIS_PASSWORD}
      caCert: ${SWIS_CACERT_PEM}
```

The sample dashboard expects the data source uid `swis`; import it through Dashboards,
New, Import, or provision the `dashboards/` directory the way
[provisioning/README.md](provisioning/README.md) shows.

## Writing queries

Everything in [the SWQL guides](../../docs/swql/README.md) applies. Three things are specific
to Grafana.

**Bound the query.** Statistics and history entities (`Orion.CPULoad`,
`Orion.ResponseTime`, `Orion.NPM.InterfaceTraffic`, `Orion.AlertHistory`, `Orion.Events`)
are the largest tables on the server, and a dashboard refreshes every panel on a timer.
Keep `TOP n` and `$__timeFilter(...)` on every query against them.

```sql
SELECT TOP 10000
    c.DateTime,
    c.AvgLoad,
    c.AvgPercentMemoryUsed
FROM Orion.CPULoad c
WHERE c.NodeID = ${node}
  AND $__timeFilter(c.DateTime)
ORDER BY c.DateTime
```

The backend sends SWIS this, with `@__timeFrom` and `@__timeTo` bound as UTC ISO 8601
strings, and `${node}` already replaced by Grafana:

```sql
SELECT TOP 10000
    c.DateTime,
    c.AvgLoad,
    c.AvgPercentMemoryUsed
FROM Orion.CPULoad c
WHERE c.NodeID = 42
  AND c.DateTime >= @__timeFrom AND c.DateTime <= @__timeTo
ORDER BY c.DateTime
```

Statistics and history columns hold UTC, so a UTC bound is the comparison that means what
it says; see [date-and-time.md](../../docs/swql/date-and-time.md). `Orion.Events.EventTime`
is the documented exception and is local time, so a `$__timeFilter` on it is offset by the
SQL Server's zone.

**One series per label.** For the time series format, every string column in the SELECT
list becomes a label, and each distinct combination becomes its own series. This query
draws one line per interface on the node:

```sql
SELECT TOP 10000
    t.DateTime,
    t.Interface.FullName AS Interface,
    t.InAveragebps,
    t.OutAveragebps
FROM Orion.NPM.InterfaceTraffic t
WHERE t.NodeID = ${node}
  AND $__timeFilter(t.DateTime)
ORDER BY t.DateTime
```

**Variables.** A dashboard variable of type Query, using this data source, runs a SWQL
statement. Alias the label `__text` and the value `__value`:

```sql
SELECT TOP 5000
    n.Caption AS __text,
    n.NodeID AS __value
FROM Orion.Nodes n
ORDER BY n.Caption
```

A multi-value variable in an `IN` list is written `IN (${node:csv})`.

## Invoking verbs from a dashboard

Reading is safe. Invoking changes the monitored estate, so the plugin gates it three
times, and all three have to pass:

1. **The allowlist.** Only an `Entity.Verb` written into the data source settings can be
   called. The default is an empty list, which refuses everything, including for admins.
2. **The Grafana role.** The caller must be an Editor or Admin in the Grafana
   organisation. Viewers get 403. The role comes from Grafana's own session, not from the
   request.
3. **The Orion account.** SWIS still enforces the right the verb requires (`manageNodes`
   for `PollNow`, `allowUnmanage` for `Unmanage`, `clearEvents` for `Acknowledge`), so the
   Grafana service account needs that right too. If you want a read-only Grafana, do not
   grant it, and the allowlist becomes a second lock rather than the only one.

Every invocation is written to Grafana's server log with the Grafana user, the verb and
the arguments, so a change made from a dashboard is as traceable as one made from
PowerShell.

The call is an HTTP POST to the data source's resource endpoint with a **positional** JSON
array. Names appear in documentation but never on the wire; the order is the whole
contract, so look it up first:

```bash
python3 tools/schema_query.py verb Orion.Nodes PollNow
```

```bash
curl -sS -X POST "https://grafana.example.com/api/datasources/uid/swis/resources/invoke/Orion.Nodes/PollNow" \
  -H "Authorization: Bearer $GRAFANA_SERVICE_ACCOUNT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '["N:42"]'
```

```json
{"invoked": "Orion.Nodes.PollNow", "result": null}
```

`GET .../resources/verbs` returns the allowlist, so a panel can find out what it may offer.

From a panel, the plugin's `DataSource.invoke(entity, verb, args)` method does the same
call through Grafana's own HTTP client. Grafana ships no button panel; the community
"Business Forms" and "Button" panels can POST to a data source resource and are the usual
way to put an acknowledge or poll-now button on a dashboard. Their configuration is
theirs, not this plugin's, and was not exercised here.

An acknowledge button is the common case. The active-alerts example selects
`AlertObjectID` for exactly this reason: it is what `Orion.AlertActive.Acknowledge` takes,
as an array, followed by the note:

```json
["Orion.AlertActive", "Acknowledge", [[12345], "Acknowledged from Grafana"]]
```

## What was verified, and what was not

Verified here, on every build of this repository:

- Every SWQL statement in [src/examples.ts](src/examples.ts) and in the sample dashboard
  resolves against the extracted 2026.2 schema (`make validate` reads both, rewriting
  the Grafana macros the way the backend does first).
- The backend's Go tests run it against a stub SWIS over TLS with a self-signed
  certificate: macro expansion and refusal of malformed macros, UTC binding of the time
  range, column typing and order, timestamp parsing, row truncation, the long-to-wide
  pivot for time series, SWIS error messages reaching the panel, the health check's three
  outcomes, TLS verification staying on by default, certificate pinning against a
  certificate shaped like the stock SWIS one (a fixed name, no subject alternative
  names) with the name check off and a different certificate still refused, and the
  invoke gates.
- The frontend typechecks, lints and bundles with Grafana's own build configuration.

Not verified here, because this repository's build environment has no Grafana:

- The plugin loading and rendering in a live Grafana. The code uses the plugin SDK the
  way Grafana's `create-plugin` scaffold does, and the SDK versions pinned in `go.mod` and
  `package.json` are the ones that scaffold produced, but nobody has clicked "Save & test"
  on this build yet. `docker compose up` in this directory starts a development Grafana
  OSS with the plugin mounted and provisioned from the `SWIS_*` environment variables, and
  is the fastest way to do that.
- The minimum Grafana version. `plugin.json` declares 11.0.0 because every frontend
  component and SDK call used here existed by then, but that is a reading of the
  changelogs, not a test.
- Anything about a specific SWIS installation: which entities are present, what the
  service account can see, and whether `Orion.Events.EventTime` is local on your server.

## Development

Release notes are in [CHANGELOG.md](CHANGELOG.md).

```bash
npm run dev          # rebuild the frontend on change
go test ./pkg/...    # backend tests
docker compose up    # Grafana OSS on :3000 with the plugin mounted (needs SWIS_* set)
```

`.config/` is Grafana's generated build configuration and is updated by
`npx @grafana/create-plugin@latest update`, not by hand.

## Layout

```text
pkg/main.go              registers the plugin with Grafana
pkg/plugin/settings.go   data source settings and defaults
pkg/plugin/swis.go       the REST client: Query and Invoke, TLS, error envelope
pkg/plugin/macros.go     $__timeFilter, $__timeFrom, $__timeTo
pkg/plugin/frames.go     SWIS JSON rows to typed Grafana frames
pkg/plugin/datasource.go QueryData, CheckHealth, CallResource
pkg/plugin/plugin_test.go
src/datasource.ts        the frontend class: variables, invoke() helper
src/components/          the configuration and query editors
src/examples.ts          the validated example queries
dashboards/              the sample dashboard
provisioning/            development provisioning for docker compose
```
