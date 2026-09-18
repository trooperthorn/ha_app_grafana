# ha_app_grafana

Home Assistant add-on: Grafana OSS hardened behind Ingress (per-user roles via
nginx auth-proxy, no login form, AppArmor profile, recorded terminal). Full
rationale for every departure from Grafana's defaults is in `docs/decisions.md`
(append-only log, read it before assuming something is arbitrary) and
`docs/security.md`. User-facing changes go in `grafana/CHANGELOG.md` under
`## Unreleased`, promoted to a version heading at release. `grafana/config.yaml`
`version` and `grafana/rootfs/run.sh`'s `APP_VERSION` must always match
(CI's "App config well-formed" check enforces this) — bump both together on
every release-worthy change, since the Supervisor only offers an update when
the version string changes.

## Bundled plugins

Seven Grafana data source plugins are built into the image, source under
`grafana/plugins-src/<name>/`, compiled in dedicated `Dockerfile` stages,
bundled under `/opt/grafana-app/plugins-bundled/`, auto-provisioned from
`config.yaml` options via `.yaml.template` files rendered by `run.sh`:
SolarWinds SWIS, Technitium DNS (+ a Query Logs series reading the "Query
Logs (Sqlite)" DNS app's own API), Music Assistant, Unifi Network, Unifi
Protect, Home Assistant, HA SOC. SWIS's source is vendored in this repo
(from `trooperthorn/SolarWinds_OrionGuides` at a pinned commit) rather than
git-fetched at build time, so that repo's other branches can't break this
build.

## Status as of 2026-09-17 (v2026.09.17.4)

v2026.09.17.4 fixed the first thing the live installation showed: an empty
Ingress panel with the browser's broken-page icon. Grafana's own
`allow_embedding = false` sends `X-Frame-Options: deny`, which blocks the
same-origin iframe every Ingress panel lives in. Grafana now runs with
embedding on (loopback only) and nginx sends `SAMEORIGIN` unless the
`allow_embedding` option is on; the smoke test asserts the header. See
docs/decisions.md. Not yet confirmed rendering on the live instance.

## Earlier status (v2026.09.17.3)

Two real production bugs were found and fixed this session, both only
reproducible under the Supervisor (CI's smoke test is a plain `docker run`
with no AppArmor attached, so neither would have been caught by CI):

1. **`/data`/`/run/grafana-app` chown crash-loop** — `apparmor.txt` granted
   only `r` on those two directory *entries* themselves, not `w`, while
   `run.sh` chowns them directly. Fixed: both are `rw` now.
2. **`Error: ✗ unable to open database file (14)`** — `grafana.ini.template`
   had `wal = true` (not Grafana's own default, undocumented why it was
   ever set). Grafana 13's unified storage subsystem opens a second,
   independent SQLite connection pool to the same `grafana.db`; WAL mode
   needs every connection to coordinate through a shared-memory index file,
   a documented cause of this exact error once a second pool is involved.
   Fixed: `wal = false`, matching Grafana's default. An existing
   installation's `grafana.db` self-converts on next connect, no manual
   migration needed.

**Neither fix has been verified against a live Supervisor.** This
development sandbox has no AppArmor kernel module at all (confirmed: no
`/sys/module/apparmor`, no `apparmor_parser`) and, separately, no route to
pull container images from any registry (Docker Hub's CDN and GHCR's blob
storage are both blocked by this session's network egress policy) — so
neither bug could be reproduced or the fix live-tested here. Both fixes are
reasoned from Grafana's own source (`pkg/storage/unified/sql/db/dbimpl/
dbimpl.go` for #2) plus, for #2, independently-reported external cases of
the same error text. **If either issue resurfaces after updating to
2026.09.17.3, that's the first thing to know** — the reasoning holds up on
paper but was never run for real.

If the WAL fix does *not* resolve the crash: check Settings > System > Logs
> Host in the Home Assistant UI for `DENIED`/`apparmor` lines — AppArmor
logs a denial under enforce mode the same as complain mode, so this works
without editing `apparmor.txt` or needing host SSH/`journalctl` first. See
`docs/operations.md`, "AppArmor denials".
