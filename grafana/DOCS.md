# Grafana (hardened)

Grafana OSS 13 for Home Assistant, built to be the opposite of the community
Grafana app on every point that matters to a host that holds monitoring
credentials: one door (Ingress), a role per Home Assistant user, no known
password, nothing of Home Assistant's mapped in, the SolarWinds SWIS data
source built into the image, and a terminal that reaches nothing beyond the
container. The reasoning is in the repository's `docs/security.md` and
`docs/decisions.md`.

## First start

1. Install the app and open its configuration. Put your Home Assistant
   username (the login name, not the display name) in `admin_users`. Until
   someone is listed there, nobody can administer Grafana and the panel
   shows a page saying so.
2. Start the app and open "Grafana" in the sidebar. You are signed in as
   your Home Assistant user with the Admin role. There is no Grafana login
   form anywhere.
3. Add the SolarWinds data source: Connections, Data sources, Add data
   source, "SolarWinds SWIS". It is already installed. Fill in the Orion
   server, a read-only Orion account, paste the SWIS certificate into the
   CA field and turn on "Ignore certificate name" (the stock certificate has
   no subject alternative names). Save & test reports the engine it found.
4. Import the sample dashboard from the plugin's `dashboards/` directory, or
   build your own with the example queries in the query editor.
5. To use the bundled Technitium DNS data source, set `technitium_url` (the
   DNS server's own web console origin, e.g. `http://192.168.1.10:5380`)
   and `technitium_api_token` (a non-expiring token from Administration,
   Sessions, Create API Token, in the Technitium console) and restart. The
   data source is provisioned automatically, named "Technitium DNS". If the
   server has the "Query Logs (Sqlite)" DNS app installed (or its MySQL,
   PostgreSQL, or SQL Server equivalents, which share the same API), the
   query editor's "Query logs" series reads its stored per-request log over
   the dashboard's own time range, instead of a separate time series
   database. Set `technitium_querylogs_app_name` only if that app was
   installed under a different name than the store default.
6. To use the bundled Music Assistant data source, set `musicassistant_url`
   (the server's own web console origin, e.g. `http://192.168.1.20:8095`)
   and `musicassistant_api_token` (a long-lived token from the Music
   Assistant web UI's user settings, or its `auth/token/create` API
   command) and restart. The data source is provisioned automatically,
   named "Music Assistant".
7. To use the bundled Unifi Network data source, set `unifi_network_host`
   (the controller/console address, e.g. `192.168.1.1`) and
   `unifi_network_api_key` (a local Integration API key from the
   controller's Settings > Control Plane > Integrations) and restart. Leave
   `unifi_network_verify_ssl` off (the default) for a typical self-signed
   console. The data source is provisioned automatically, named "Unifi
   Network".
8. To use the bundled Unifi Protect data source, set `unifi_protect_host`
   and `unifi_protect_api_key` (a local Integration API key from the
   console's Settings > Control Plane > Integrations; Protect uses its own
   key, separate from Network's) the same way, and restart. It exposes a
   camera inventory only, named "Unifi Protect": Protect's API has no
   historical events route to query, only a live WebSocket subscription a
   one-request-per-query backend plugin cannot honestly offer.
9. To use the bundled Home Assistant data source, set `homeassistant_url`
   (this instance's own origin, e.g. `http://homeassistant.local:8123`)
   and `homeassistant_access_token` (a long-lived access token from your
   Home Assistant profile's Security tab) and restart. It queries
   long-term statistics and system health over Home Assistant's own core
   WebSocket API, named "Home Assistant". Entity state and its raw
   history are already available from HA's REST API and are not
   duplicated here.
10. If you have [HA SOC](https://github.com/trooperthorn/ha_int_soc)
    installed on this Home Assistant instance, set `hasoc_url` (usually
    the same as `homeassistant_url`) and `hasoc_access_token` (a
    long-lived token for a Home Assistant **admin** user — HA SOC's own
    access gate requires it, and by default the account owner
    specifically) and restart. It exposes HA SOC's security posture
    score, per-user risk, and audit log, named "HA SOC".

## Who can do what

| Option | Grafana role | Can |
| --- | --- | --- |
| `admin_users` | Admin | Everything: data sources (including the SolarWinds credential), users, plugins' settings, the terminal |
| `editor_users` | Editor | Dashboards and alert rules, not data sources or users |
| `default_role: Viewer` | Viewer | See every dashboard, change nothing |
| `default_role: none` (default) | nothing | A page explaining how to be added |

Roles apply on the next request after saving the options and restarting
the app. The Home Assistant username is what Ingress attaches to every
request after authenticating the session; a browser cannot supply its own.

## Options

| Option | Default | Effect |
| --- | --- | --- |
| `admin_users` | `[]` | Home Assistant usernames given the Admin role |
| `editor_users` | `[]` | Home Assistant usernames given the Editor role |
| `default_role` | `none` | `none` or `Viewer` for everyone else |
| `allow_embedding` | `false` | Off: only pages on the Home Assistant origin (the Ingress panel, an iframe card pointing at the Ingress URL) can frame it. On: any page on any origin can frame it too |
| `terminal_enabled` | `false` | Offer the terminal to administrators at `/terminal/` under the panel |
| `terminal_session_recording` | `true` | Write every terminal session to a transcript with timing and record its hash |
| `terminal_idle_timeout_minutes` | `30` | Close a shell idle at its prompt for this long; `0` disables |
| `plugins` | `[]` | Catalogue plugins, `id` or `id@version`, installed into the data directory on start when missing |
| `custom_plugins` | `[]` | Plugins from a URL, each with the zip's SHA-256; see below |
| `log_level` | `info` | Grafana's own log level |
| `access_log_retention_days` | `90` | Daily rotations of the access log to keep |
| `technitium_url` | `""` | Technitium DNS Server web console origin; provisions the bundled data source when set together with `technitium_api_token` |
| `technitium_api_token` | `""` | A non-expiring API token from the Technitium console (Administration, Sessions, Create API Token) |
| `technitium_querylogs_app_name` | `""` | Only needed if the "Query Logs (Sqlite)" DNS app was installed under a different name than the store default |
| `musicassistant_url` | `""` | Music Assistant server web console origin; provisions the bundled data source when set together with `musicassistant_api_token` |
| `musicassistant_api_token` | `""` | A long-lived API token from the Music Assistant web UI (user settings) or its `auth/token/create` command |
| `unifi_network_host` | `""` | Unifi Network controller/console address; provisions the bundled data source when set together with `unifi_network_api_key` |
| `unifi_network_api_key` | `""` | A local Integration API key from the controller's Settings > Control Plane > Integrations |
| `unifi_network_verify_ssl` | `false` | Verify the controller's TLS certificate; off by default for a typical self-signed console |
| `unifi_protect_host` | `""` | Unifi Protect console address; provisions the bundled data source when set together with `unifi_protect_api_key` |
| `unifi_protect_api_key` | `""` | A local Integration API key from the console's Settings > Control Plane > Integrations (Protect's own key, separate from Network's) |
| `unifi_protect_verify_ssl` | `false` | Verify the console's TLS certificate; off by default for a typical self-signed console |
| `homeassistant_url` | `""` | This Home Assistant instance's own origin; provisions the bundled data source when set together with `homeassistant_access_token` |
| `homeassistant_access_token` | `""` | A long-lived access token from your Home Assistant profile's Security tab |
| `homeassistant_verify_ssl` | `false` | Verify the instance's TLS certificate; off by default |
| `hasoc_url` | `""` | The Home Assistant instance HA SOC is installed on (usually the same as `homeassistant_url`); provisions the bundled data source when set together with `hasoc_access_token` |
| `hasoc_access_token` | `""` | A long-lived access token for a Home Assistant **admin** user (HA SOC requires admin, and by default the account owner) |
| `hasoc_verify_ssl` | `false` | Verify the instance's TLS certificate; off by default |

## Plugins

Catalogue plugins install with `grafana cli` into `/data/plugins` and stay
there across app updates; pin the version (`grafana-clock-panel@2.1.8`)
so a restart cannot change what runs. Installing from the Grafana UI is
switched off: every plugin comes in through these options.

A custom plugin needs its zip's SHA-256:

```yaml
custom_plugins:
  - name: vendor-example-panel
    url: https://example.com/vendor-example-panel-1.0.0.zip
    sha256: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
    unsigned: true
```

The download is hashed before it is unpacked, and a mismatch is logged and
not installed. `unsigned: true` adds the id to Grafana's
`allow_loading_unsigned_plugins`, which is what a plugin Grafana has not
signed needs to load. The SolarWinds SWIS, Technitium DNS, Music
Assistant, Unifi Network, Unifi Protect, Home Assistant and HA SOC data
sources are built into the image and need none of this.

## The terminal

With `terminal_enabled: true`, an administrator opens
`<the panel's URL>/terminal/` in the browser (the panel's URL is the
`/api/hassio_ingress/...` path Home Assistant shows in the address bar). It
is a bash shell as the `grafana` user inside this container: `/data`
(Grafana's state, plugins, logs, the generated secrets), `grafana cli`,
`sqlite3` read-only on the Grafana database (`gdb`), `jq`, `nano`, `less`.
There is no SSH client, `wget`, `nc` or Python, and no Home Assistant
directory is mounted; the shell reaches nothing beyond this container. Each
session is recorded by util-linux `script` under `/data/terminal/sessions`
with a timing file, and its size and SHA-256 are appended to `index.jsonl`
with the Home Assistant username when the session ends. Non-administrators
and everyone with the option off get 403.

`curl` is present because the launcher and the health check use it. That is
the one accepted residual of "local only".

## The optional API port

Map container port 3080 to a host port under the app's Network settings to
let automation on the LAN call Grafana's HTTP API (a Home Assistant REST
sensor reading an alert state, a script rendering a panel). No Ingress
identity exists on that port: the login form and basic auth are off, so the
only thing that authenticates is a service-account token you create as an
administrator under Administration, Service accounts. It is plain HTTP on
your LAN; leave it unmapped unless you need it.

## Logs

The app log carries Grafana's and nginx's own output. The access log,
`/data/log/nginx/access.log`, holds one JSON line per request with the Home
Assistant user id, username, display name, the role applied, method, path
and status, rotated daily and kept for `access_log_retention_days`. Read it
from the terminal or from a backup.

## Backups

Home Assistant's backup of this app includes `/data`: the Grafana database
(dashboards, data sources with their encrypted secrets, users, alert rules),
the generated signing key and admin password, installed plugins, the access
log and the terminal transcripts. Restoring the backup restores all of it;
the signing key travelling with the database is what keeps existing data
source secrets readable.

## Known limits

- A transcript captures whatever is typed at a prompt, including a secret
  typed into a program that asked for one.
- The idle timeout is bash's `TMOUT`; a running program does not honour it.
- Grafana's image rendering (PNG export, images in alert notifications)
  needs the separate renderer service, which this app does not ship.
- Anonymous access and Home Assistant Cloud remote access do not combine, as
  with the community app; this app does not offer anonymous access at all.
