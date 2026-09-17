# Decisions

Dated, so a later reader knows what was known when the choice was made.

## 2026-09-16: build a separate app rather than configure the community one

The community Grafana app (hassio-addons/addon-grafana, read at its head on
2026-09-16) has `custom_plugins` with an `unsigned` flag, so the SWIS data
source can be loaded there. It was still not the base to keep, for reasons
that are configuration in that app and cannot be overridden from its
options: `X-WEBAUTH-USER admin` for every Ingress request with
`auto_assign_org_role = Admin`; `admin_password = hassio` with basic auth
on; `secret_key` set to the Supervisor token; `homeassistant_config`,
`share` and `ssl` mapped; `allow_embedding = true`; plugins fetched from a
URL on every boot with no checksum and no persistence. See the table in
[../README.md](../README.md).

## 2026-09-16: Grafana's own image, Ubuntu variant, not a Home Assistant base image

Grafana publishes an image that already runs as user 472, ships only
Grafana, curl, ca-certificates and tzdata, and is pinned by digest. Building
on it keeps Grafana's own hardening and makes the app's additions the whole
diff. The Ubuntu variant was chosen over Alpine because `nginx-light` and
`ttyd` are Ubuntu packages, and `nginx-light` is the same lean nginx the
Technitium DNS app settled on (there, on Debian). The cost is no bashio, so
`run.sh` reads the options with `jq`, as the Technitium app does.

`grafana/grafana` is the OSS edition on Docker Hub; `grafana/grafana-oss`
is the older name and had not been updated since June 2026 when checked.
`grafana/grafana-enterprise` is the other edition and is not used.

## 2026-09-16: roles from Home Assistant usernames, default deny

Home Assistant has two tiers (administrator or not) and Ingress attaches
the username, the display name and the id, nothing about the tier. So the
Grafana role has to be a list in the options, and it is keyed by username
because that is what `X-Remote-User-Name` carries. A fresh install grants
nothing to anyone until the operator lists a username, and the page a
non-listed user sees says exactly that. `default_role: Viewer` is a
one-line change for a household that wants every Home Assistant user to
see the dashboards.

## 2026-09-16: no login form anywhere, one random admin password anyway

Grafana creates an admin account at first start whatever the configuration
says. Leaving it at the default would be the community app's mistake;
logging a generated one, as the Technitium app does, would put a credential
in the app log for an account that has no door to use it on. So the password
is generated, stored 0600 under `/data/secrets`, never logged, and readable
from the terminal by an administrator who needs it for the optional API port.

## 2026-09-16: plugins pinned, hash-checked, persisted, never from the UI

The community app's `custom_plugins` downloads on every start with no
checksum, into a directory that does not survive a restart. Here plugins
install into `/data/plugins` once, a URL plugin must carry its SHA-256, and
the Grafana catalogue UI is off so nothing arrives by a route the options do
not describe. The SWIS data source is compiled into the image from a pinned
OrionGuides commit rather than downloaded, so an image build is the whole
provenance.

## 2026-09-16: the terminal is the HA SOC Terminal's design, not app-ssh's

One `ttyd`, one PTY per connection, no tmux, `script` recording with a
hashed index, no client tools, the Supervisor token unset before the server
starts. What differs from HA SOC: the gate is nginx on the Ingress path with
the Admin role rather than the SOC panel's tier, because this app has no
panel of its own and Ingress is already authenticated; and the shell is the
`grafana` user, not root, because nothing here needs root.

## 2026-09-16: enumerated AppArmor file rules, enforced, unverified

The Technitium app left file mediation broad because the .NET runtime's
paths were not traced. Grafana's image is simpler and documented, so the
rules are written out. This is enforced from the first release at the
owner's direction, with the fail-closed risk and the recovery procedure
written into [security.md](security.md) and [operations.md](operations.md).

## 2026-09-16: an optional token-only API port instead of the community app's port 80

The community app maps port 80 with the login form on, which is where the
published password becomes reachable from the LAN. This app's optional port
carries no Ingress identity and no login form, so a service-account token is
the only credential that works on it, and it is unmapped until the operator
maps it. TLS on that port was not added: the app maps no `ssl` directory on
purpose, and a self-signed certificate on a LAN port protects less than the
token does. If encrypted LAN access is needed, put it behind Home
Assistant's own reverse proxy.

## 2026-09-16: no `ha` CLI in the terminal

The HA SOC Terminal ships the Supervisor CLI with the `manager` role. This
terminal exists to look at Grafana, and a Supervisor role is a rating cost
and a capability the app has no use for. "Local only to the container"
means exactly that here.

## 2026-09-16: developed inside OrionGuides, to be moved to its own repository

The app is built and tested in `apps/ha_app_grafana/` of the OrionGuides
repository so that a change to the SWIS plugin and a change to the app that
bundles it land together and the same CI proves both. The directory is a
complete app repository (`repository.yaml` at its root, the release scripts
and workflows from the Technitium DNS app) and can be moved to
`trooperthorn/ha_app_grafana` verbatim; the Dockerfile's pinned OrionGuides
commit is what keeps the plugin source stable across that move.

Moved on 2026-09-16, the same day, once the OrionGuides CI had run the smoke
test green: the eleven commits under `apps/ha_app_grafana/` became this
repository's history, on top of the initial commit that carries the MIT
license. The staged copy was removed from OrionGuides.

## 2026-09-17: Technitium DNS plugin sourced in this repository, not a pinned external commit

The SWIS plugin is built from a pinned commit of `SolarWinds_OrionGuides`
because that plugin's source already lives there, developed alongside the
schema documentation it depends on. No equivalent repository exists for a
Technitium DNS data source, so its source lives directly in this
repository at `grafana/plugins-src/technitium-datasource` and the
Dockerfile builds it with `COPY` instead of a `git fetch` to a commit. An
image build is still the whole provenance; the difference is that this
repository's own history is the pin, rather than another repository's
commit hash. Scaffolded with `@grafana/create-plugin` (Go backend using
`grafana-plugin-sdk-go`, calling only `GET /api/dashboard/stats/get` with a
non-expiring API token) and trimmed of the generator's own CI, Docker dev
environment and Playwright e2e scaffolding, none of which this repository
needs a second copy of.

## 2026-09-17: Music Assistant data source queries its HTTP endpoint, not its WebSocket

Music Assistant's primary API is a WebSocket (`/ws`) using a
`{message_id, command, args}` request and `{message_id, result}` /
`{message_id, error_code, details}` response envelope (see
`music_assistant_models.api` in the `music-assistant/models` repository).
The same webserver also exposes `POST /api` with the identical envelope
over plain HTTP, documented in
`music_assistant/controllers/webserver/README.md` in the
`music-assistant/server` repository. A Grafana backend plugin issues one
request per query and does not benefit from a persistent connection, so
this plugin uses the HTTP form and authenticates with a long-lived token
(`auth/token/create`), never the WebSocket's session-based `auth` command.
It calls only `players/all`; Music Assistant has no documented
library-count/stats command as of this writing, so this plugin does not
claim one.

## 2026-09-17: Unifi Network plugin's contract taken from `trooperthorn/ha_int_soc`

The owner asked that this plugin reference the Unifi Network and Unifi
Protect work already done in his `ha_int_soc` (HA SOC) repository rather
than re-deriving the API from scratch. HA SOC's `docs/UNIFI-LOCAL-API-CONTRACT.md`
records a verification pass against Ubiquiti's own versioned Network
10.4.57 and Protect 7.2.105 OpenAPI/Postman artifacts (checksummed) and a
live controller, corrected a wrong assumption (ACL rules and Firewall
Policies are separate resources; a live controller returned an empty ACL
list while the real rules lived under Firewall Policies), and recorded
which field mappings are verified versus still on its own backlog.

This plugin reuses that contract directly: base path
`/proxy/network/integration/v1`, `X-API-KEY` header, no redirects followed,
an 8 MB response cap, `/sites` then `/sites/{id}/clients` and
`/sites/{id}/devices`, the `{"data": [...]}` offset/limit pagination
envelope (falling back to a bare list), and the same candidate-key lists
`unifi.py`'s `_normalize_client`/`_normalize_device` use for fields that
vary across firmwares (name, VLAN, SSID, uptime, bandwidth). It reuses
`_derive_wan`'s gateway-selection heuristic and its top-level
uplink/wan/wan1/wan2/internet node search, but not the fuller
`interfaces`/port-array shapes HA SOC's own contract doc still lists as
unverified. It does not port ACL rules, Firewall Policies, or Wi-Fi
broadcast configuration: those are HA SOC's security-audit surface, not a
monitoring dashboard's, and this plugin does not claim to audit anything.
Read-only by construction: it uses only `GET` routes and never HA SOC's
write-back path (`PUT` to disable a policy or rule), which this plugin has
no reason to carry.

## 2026-09-17: Unifi Protect plugin has a cameras series and no events series

Built the same way as Unifi Network: base path
`/proxy/protect/integration/v1`, `X-API-KEY` header, `GET /cameras`
(Protect's own contract makes this an unpaginated array, unlike Network's
offset/limit collections, so no pagination helper was carried over), and
the same `_normalize_camera` candidate-key fields (`isRecording` as a
plain boolean on some firmwares, `recordingSettings.mode` on others; both
covered by this plugin's own tests).

Deliberately no events/detections series. HA SOC's contract verification
(`docs/UNIFI-LOCAL-API-CONTRACT.md`) searched every documented path in
Protect 7.2.105's OpenAPI spec and found no historical `/events`,
`/detections`, or `/alarms` route; live events exist only as the
persistent WebSocket subscription `GET /subscribe/events`, which HA SOC
itself works around by reading Home Assistant's loaded `unifiprotect`
integration's in-memory buffer rather than calling Protect's API for
history. A Grafana backend plugin answers one HTTP request per query and
holds no state between them: it has no route to poll for history and no
honest way to hold a persistent subscription open between queries either,
so offering an events series would mean fabricating data. This plugin
says so in its README rather than shipping a series that can't be real.

## 2026-09-17: /data ownership failure degrades instead of crash-looping

Reported: `chown "$GRAFANA_UID:$GRAFANA_GID" /data` failing with "Permission
denied" under `set -o errexit`, so `run.sh` exited immediately with nothing
but that one line, and the Supervisor (or `docker run --restart`)
restarted it in a tight loop that reproduced the same bare error forever.
The Supervisor's own `/data` mount is fresh and root-owned, so this
chown never used to fail there; it fails outside the Supervisor when
`/data` is a bind mount from a Docker/Podman mode or filesystem that
refuses ownership changes altogether (rootless Docker/Podman without an
idmapped bind mount, some network or virtualized filesystem shares).

`run.sh`'s `chown_or_verify` now treats that refusal as recoverable: if the
path is already writable and executable by uid 472 once checked directly,
it logs a warning and continues without changing ownership; only if the
path is genuinely unusable by that uid does it exit, once, with a
diagnostic naming the likely cause and the host-side fixes (chown the host
path, use a named volume instead of a bind mount, or an idmapped mount).
See docs/operations.md.

The owner was running with `log_level: debug` when this happened and asked
that the launcher use that setting for its own diagnostics, not only
Grafana's. `run.sh` now reads `log_level` immediately after finding
`options.json`, before anything else can fail, and a new `log_debug`/
`debug_dump_path` pair (gated on `log_level: debug`, independent of
Grafana's own `[log]` level) prints `id`, the failing path's and its
parent's owner/mode, its mount entry (source, filesystem, options), and
this process's effective capabilities whenever `chown_or_verify` cannot
chown a path, whether or not the fallback recovers it. This is what turns
"a mount refused chown" into which of rootless Docker/Podman, a
capability actually dropped, or a network/virtualized filesystem it was,
without asking the reporter to attach a debugger.

## 2026-09-16: Grafana's own binaries excluded from the scan by path

The first image scan failed on High advisories compiled into Grafana's
binary and its bundled plugins: three at first sight, some forty once the
whole table was read (the Go standard library at several versions, x/net,
x/crypto, x/text, gRPC, OpenTelemetry, a Tempo module). Nothing in this
repository can change those short of a Grafana release, and refusing to
build until Grafana ships one would leave the Ubuntu packages and the SWIS
backend, which this repository does control, unscanned. The HA SOC style
of one ignore entry per advisory was tried first and does not scale to
forty entries that all say the same thing, so `.grype.yaml` excludes the
one path, `/usr/share/grafana`, with the reason. To keep the SWIS backend
inside the gate it moved from Grafana's `plugins-bundled` to
`/opt/grafana-app/plugins-bundled`, which `grafana.ini` names as the
bundled plugin path. `apt-get upgrade` was added to the build at the same
time so glibc's pending fixes land.

## 2026-09-17: Home Assistant and HA SOC plugins reuse the same WebSocket client shape

The owner asked to also cover Home Assistant itself and HA SOC
(`trooperthorn/ha_int_soc`). Neither has a REST surface that answers this
app's questions: Home Assistant's long-term statistics
(`recorder/statistics_during_period`) and system health
(`system_health/info`) are WebSocket-only commands with no REST
equivalent (its REST API covers current entity state and raw history,
which this repository does not duplicate), and HA SOC has no HTTP view of
its own at all - every `ha_soc/*` command it registers rides on that same
Home Assistant core WebSocket API. So both new plugins are WebSocket
clients (`gorilla/websocket`, the first dependency in this repository not
already pulled in by `grafana-plugin-sdk-go`), completing Home
Assistant's own documented `auth_required`/`auth`/`auth_ok` handshake
with a long-lived access token per query, the same one the frontend and
every other HA integration uses.

`system_health/info` does not answer with the plain
`{"success":true,"result":...}` envelope every other command here uses:
it confirms the subscription with a bare success result, then streams
`initial`/`update`/`finish` events on the same connection, per Home
Assistant core's `homeassistant/components/system_health/__init__.py`.
The Home Assistant plugin's client reads that sequence to completion
before returning, since a Grafana backend plugin answers one request per
query and has nothing to gain from leaving the subscription open.

HA SOC's own `require_soc_access` decorator (`websocket_api.py`) rejects
any non-admin token, and by default (`access_level: owner_only`) accepts
only the account owner's; the HA SOC plugin's docs say this plainly
rather than let a confusing "unauthorized" surface with no explanation.
Its `posture`, `risk`, and `audit` series and their field names are taken
directly from `risk.py`'s `RiskEngine._compute_user_risk` /
`async_compute_posture` and `audit.py`'s `AuditLog.async_log` record
shape - the same "read the real source, do not guess the schema" standard
every other bundled plugin here was held to.

## 2026-09-17: Technitium query logs read over its own HTTP API, not a mounted SQLite file

A user who already runs Technitium's "Query Logs (Sqlite)" DNS app (or its
MySQL, PostgreSQL, or SQL Server equivalents) asked whether this
repository's Technitium plugin could read that stored history directly,
instead of standing up InfluxDB for DNS query history. Reading the
`.sqlite` file itself was ruled out: this app has nothing of the host
filesystem mapped in, on purpose, and adding a network file mount just for
one plugin would be a worse hole than the problem it solves.

Technitium's own web API already exposes it, confirmed against
`DnsServerCore/WebServiceLogsApi.cs` and `Apps/QueryLogsSqliteApp/App.cs`
in `TechnitiumSoftware/DnsServer`: any installed DNS app implementing
`IDnsQueryLogs` (the Sqlite app included) is queryable at
`GET /api/logs/query?name=<app>&classPath=<class>&...`, paginated, with
`start`/`end`/`clientIpAddress`/`qname` filters, over the same token this
plugin already authenticates dashboard stats with. So the Technitium
plugin gained a "Query logs" series that pages through that endpoint
across the dashboard's own time range (capped at 10,000 rows per query, to
keep a wide range against a busy resolver from pulling the whole log) -
the same "same host, same token, no new infrastructure" standard the
Unifi and Home Assistant plugins were held to. The app's class path
(`QueryLogsSqlite.App`) is fixed by its source and not configurable; its
display name is, hence `technitium_querylogs_app_name` for a server that
renamed it away from the store default.

## 2026-09-17: the /data ownership failure was this app's own AppArmor profile, not the host

A user hit the exact crash-loop the previous fix added diagnostics for,
with `log_level: debug` on. Those diagnostics did their job: normal
ownership (root:root, mode 755), a real ext4 bind mount, and `CAP_CHOWN`
present in capabilities - nothing consistent with the "rootless
Docker/Podman without an idmapped mount" explanation the error message
offered. That ruled the host out and pointed at this container's own
AppArmor profile instead.

`grafana/apparmor.txt` granted `/data/ r,` and `/run/grafana-app/ r,` -
read-only on those two directory *entries* - while `/data/** rwk,` and
`/run/grafana-app/** rwk,` covered only their contents. AppArmor mediates
`chown` as a write on the path being chowned, and `run.sh` chowns both
directories directly (`chown_or_verify /data`, and `/run/grafana-app` via
`$RUN_DIR` in the same call), not only what is inside them. Every start
was denied by this app's own profile, on every host, regardless of the
actual bind mount - the earlier fix's fallback-and-diagnose behavior kept
it from crash-looping forever, but it could never actually recover here.
Both directory entries are now `rw`. The profile note at its head warns
this was "NOT verified against a live Supervisor" for exactly this
reason: the CI smoke test's plain `docker run` never attaches it.

## 2026-09-17: SWIS plugin source vendored into this repository, pin retired

The earlier entry above ("Technitium DNS plugin sourced in this
repository, not a pinned external commit") explained why the SWIS plugin
alone was built by `git fetch`ing a pinned commit of
`trooperthorn/SolarWinds_OrionGuides`, rather than from source living in
this repository like every other bundled plugin: its source already lived
there, developed alongside the SWIS schema documentation it depends on.
That pin became a liability once other work continued on
`SolarWinds_OrionGuides`'s own branches - merging them there risked
breaking this image's build the moment anyone moved the pinned commit
forward, entirely outside this repository's own review.

So the plugin's source at that same pinned commit
(`508ed54fbfc397ecc2634a64ebc11c9c1f8537aa`) is now vendored into
`grafana/plugins-src/swis-datasource`, and the Dockerfile builds it with
`COPY` exactly like the six other bundled plugins, instead of `git init`
+ `git fetch --depth 1` to that commit. This repository's own history is
now the only pin SWIS has; `SolarWinds_OrionGuides` can merge, rebase, or
delete its `apps/grafana-swis-datasource` branch entirely without this
build noticing. A future SWIS change still has to be brought over by
copying it into `plugins-src/swis-datasource` and committing it here,
the same as any other bundled plugin's changes are - there is no longer
an automatic link to `SolarWinds_OrionGuides` at all.

## 2026-09-17: diagnosing an AppArmor denial does not need complain mode or host SSH

After the `/data`/`/run/grafana-app` chown fix (above), a user's next start
hit a new failure: Grafana itself exits with `Error: ✗ unable to open
database file (14)` (SQLite's `SQLITE_CANTOPEN`), consistently right after
the main database connects and runs its migrations successfully, right
after the `oss-ac-basic-role-seeder` background job finishes. Reading
Grafana 13.2.2's own source (`pkg/storage/unified/sql/db/dbimpl/dbimpl.go`)
confirms its newer unified-storage subsystem opens a second, independent
SQLite connection to the same `grafana.db` rather than reusing the first
one; that second open is what fails, timed suspiciously close to the
AppArmor profile this repository had just changed, but this could not be
confirmed from Grafana's own log alone (no stack trace, no subsystem name
on the fatal line), and this development environment has no AppArmor
kernel module and no Docker daemon at all, so the profile cannot be
loaded or tested here - `apparmor_parser`/`aa-status` do not exist and
`/sys/module/apparmor` is absent.

The profile's own header pointed at `journalctl _TRANSPORT="audit" -g
'apparmor="DENIED"'` on the host to confirm a denial, which a Home
Assistant OS install does not expose without an SSH add-on - asking for
it repeatedly wasted the user's time for no actionable result. The first
fix considered was switching the profile to `complain` mode so denials
would surface in Home Assistant's own Settings > System > Logs > Host tab
instead. That was the wrong fix: AppArmor writes a denial to the kernel
audit log under **enforce** mode too, not only in `complain` - `complain`
only changes whether the operation is *blocked*, not whether it is
*logged*. So the same Host log tab already shows a denial on the
currently enforced profile, with no profile change, no restart, and no
temporary security reduction needed at all. `docs/operations.md`'s
"AppArmor denials" section and `docs/security.md`'s profile section now
lead with that UI path (checked under enforce mode) and keep `journalctl`
only as the equivalent for an install with host shell access.
