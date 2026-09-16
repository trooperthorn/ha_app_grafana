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
| `allow_embedding` | `false` | Off: Grafana renders only inside Ingress. On: the Home Assistant iframe card can frame it, and any other page can too |
| `terminal_enabled` | `false` | Offer the terminal to administrators at `/terminal/` under the panel |
| `terminal_session_recording` | `true` | Write every terminal session to a transcript with timing and record its hash |
| `terminal_idle_timeout_minutes` | `30` | Close a shell idle at its prompt for this long; `0` disables |
| `plugins` | `[]` | Catalogue plugins, `id` or `id@version`, installed into the data directory on start when missing |
| `custom_plugins` | `[]` | Plugins from a URL, each with the zip's SHA-256; see below |
| `log_level` | `info` | Grafana's own log level |
| `access_log_retention_days` | `90` | Daily rotations of the access log to keep |

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
signed needs to load. The SolarWinds SWIS data source is built into the
image and needs none of this.

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
