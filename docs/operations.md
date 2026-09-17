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

## "chown: changing ownership of '/data': Permission denied" at start

This can come from either of two independent causes; check the second one
first if the app runs under the Supervisor, since that is where it
actually turns up.

Fixed 2026-09-17: this app's own `apparmor.txt` granted only `r` on the
`/data` and `/run/grafana-app` directory *entries* themselves, separate
from the `rwk` granted on their contents. `run.sh` chowns both directories
directly, not only what is inside them, so the Supervisor's own AppArmor
enforcement denied every start regardless of the host - normal ownership,
a real bind mount, `CAP_CHOWN` present, and still `Permission denied`. A
plain `docker run` never attaches an AppArmor profile at all, which is why
this only ever showed up under the Supervisor. Update to a release with
the fix (both directories are `rw` in the profile) rather than working
around it; if `journalctl _TRANSPORT="audit" -g 'apparmor="DENIED"' -g
'profile="grafana"'` on the host still shows a chown denial on `/data` or
`/run/grafana-app` after updating, something is loading a stale profile -
see "AppArmor denials" below.

Outside the Supervisor (a plain `docker run` or `docker compose` with
`/data` bind-mounted from the host, no AppArmor profile in play), this can
still fail on a Docker/Podman mode or filesystem that refuses to let a
container change file ownership at all: rootless Docker/Podman without an
idmapped bind mount, or some network or virtualized filesystem shares.
Since 2026-09-17, `run.sh` no longer aborts the moment this happens (which
used to restart-loop with nothing but that one line repeated): it checks
whether the path is already writable by the grafana user (uid/gid 472) and
continues if so, and otherwise exits once with a diagnostic naming the
likely cause. If you hit the diagnostic rather than a clean start, fix the
host side rather than the container: `chown -R 472:472 <the host path
mapped to /data>`, switch to a plain Docker-managed named volume instead
of a bind mount, or enable idmapped mounts for the bind mount (`docker run
--mount type=bind,...,idmap=uids=...` or the Podman/Compose equivalent).

Set `log_level: debug` and restart to see exactly what the launcher itself
found: `id` (who the process actually is), `stat` on the path and its
parent (current owner/mode), the mount entry it sits on (source,
filesystem, mount options), and the process's effective capabilities. This
is the launcher's own diagnostic, separate from Grafana's `[log]` level,
and only this specific failure prints it; it does not turn on verbose
logging generally. Look for the `DEBUG:` lines around `-- ownership
diagnostics for /data --` in the app log. Normal-looking ownership and
capabilities there point at AppArmor (above), not the host.

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
`/tmp`, look for AppArmor denials without needing host SSH access at all:
open Settings > System > Logs in Home Assistant, switch to the "Host" tab,
and search for `DENIED` or `apparmor`. Each line names the operation and
path.

If you do have host shell access (an SSH add-on, or a non-HAOS install),
the equivalent is:

```bash
journalctl _TRANSPORT="audit" -g 'apparmor="DENIED"' -g 'profile="grafana"'
```

Add the narrowest rule that covers what a denial names to
`grafana/apparmor.txt`. If the app will not start at all (so there is no
running instance to generate denials from a normal request), add
`complain` to the profile flags
(`flags=(attach_disconnected,mediate_deleted,complain)`) first: this logs
what the profile would deny instead of enforcing it, letting the app
actually start so its real behavior generates the denials to fix, visible
in the same Host log tab. Reinstall, reproduce the failure, read the log,
fix the narrowest rule each denial names, then remove `complain` to
restore enforcement. Do not widen to `file,` to make a symptom go away.

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
