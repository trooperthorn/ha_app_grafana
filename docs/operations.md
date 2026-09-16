# Operations

## First start

1. Set `admin_users` to your Home Assistant username before the first start.
   The app starts either way, but until someone is listed the panel shows
   the not-authorized page to everyone and the log says
   `admin_users is empty`.
2. Start the app and watch the log for `Starting Grafana 13 on
   127.0.0.1:3000` and the Ingress entry line.
3. Open the panel. The first request creates your Grafana user with the
   Admin role.

## Changing roles

Edit `admin_users`, `editor_users` or `default_role` and restart the app.
`run.sh` regenerates the nginx role map on every start; Grafana applies the
new role on the next request because its auth-proxy cache is keyed on the
role header. A user removed from both lists with `default_role: none` gets
the not-authorized page on their next request; their Grafana user record
remains in the database and can be deleted under Administration, Users.

## Installing plugins

Catalogue: add `id@version` to `plugins` and restart. The install happens
once; the log says `Plugin <id> is already installed` on later starts. To
upgrade, change the version and delete `/data/plugins/<id>` from the
terminal (or a backup restore), then restart.

URL: compute the zip's SHA-256 (`sha256sum plugin.zip`), add the entry to
`custom_plugins` with `unsigned: true` if Grafana has not signed it, and
restart. A mismatch is logged as `sha256 mismatch` and nothing is unpacked.

The SolarWinds SWIS data source needs none of this; it is in the image.

## The terminal

Turn on `terminal_enabled`, restart, and open `<panel URL>/terminal/` as an
administrator. Transcripts are under `/data/terminal/sessions/`:
`<id>.out`, `<id>.timing`, and `index.jsonl` with one line per finished
session (`id`, `user`, `started`, `ended`, `bytes`, `sha256`). Replay one
with `scriptreplay --log-timing <id>.timing --log-out <id>.out` on any
machine with util-linux. Verify one has not changed with `sha256sum` against
its index line.

## Reading the access log

`/data/log/nginx/access.log`, one JSON object per line. Useful filters from
the terminal:

```bash
jq -r 'select(.status >= 400) | [.time, .user_name, .status, .uri] | @tsv' /data/log/nginx/access.log
jq -r 'select(.uri | startswith("/terminal")) | [.time, .user_name, .role, .status] | @tsv' /data/log/nginx/access.log
```

Rotation is daily, keeping `access_log_retention_days` files, run by a loop
in `run.sh` since the container has no cron.

## The API port

Map container port 3080 under the app's Network settings, restart, then as
an administrator create a service account and token under Administration,
Service accounts. From the LAN:

```bash
curl -H "Authorization: Bearer $TOKEN" http://homeassistant.local:3080/api/health
```

A request without a token gets 401; there is no login form or password to
try. Unmap the port to turn it off.

## AppArmor denials

If the app fails to start after installation, or Grafana logs
`permission denied` for a path under `/usr/share/grafana`, `/data` or
`/tmp`, check the host:

```bash
journalctl _TRANSPORT="audit" -g 'apparmor="DENIED"' -g 'profile="grafana"'
```

Each line names the operation and path. Add the narrowest rule that covers
it to `grafana/apparmor.txt`, or, to collect several at once, add
`complain` to the profile flags (`flags=(attach_disconnected,mediate_deleted,complain)`),
reinstall, use the app, read the log, then remove `complain`. Do not widen
to `file,` to make a symptom go away.

## Backup and restore

The Supervisor's app backup captures `/data`. Restoring it restores the
Grafana database, the signing key (which is what keeps the data source
secrets stored in the database decryptable), the admin password, installed
plugins, logs and transcripts. Restoring onto a different Home Assistant
keeps the role lists, since they are app options, and the Grafana users,
which are matched by username on the next request.

## Release process

The same automation as the Technitium DNS app: a merge to `main` runs Test,
Validate and Security, and `release.yml` tags and publishes the version in
`grafana/config.yaml`. `prepare-release.yml` opens the CalVer bump PR when
`main` carries unreleased changes under `grafana/`.
`scripts/set_version.py --next-from-tags` is the only writer of the version
and `scripts/build_release_artifacts.py --validate-only` the independent
reader. The version also appears in `run.sh` (`APP_VERSION`), and
`validate.yml` fails if the two differ. There is no downloadable artifact:
the Supervisor builds the image from the tagged tree.

## CI gate thresholds

ShellCheck at warning severity, hadolint at error, Grype on fixable High and
Critical findings. The smoke test in `scripts/smoke_test.sh` is the
behavioural gate and runs on every push.
