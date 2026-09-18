# Security

Every control below is labelled **enforced** (the code or configuration
makes it so) or **cosmetic** (it informs, it does not prevent), and
**verified** (proved by CI or read from the source named) or **unverified**
(stated from documentation or reasoning, not observed). The one rule: no
claim here without a file and, where it exists, a test.

## Security rating

Base 5 of 6, per the Home Assistant app documentation's table.

| Setting | Effect | This app |
| --- | --- | --- |
| `ingress: true` | +2 | Set |
| custom `apparmor.txt` | +1 | Shipped (`grafana/apparmor.txt`) |
| `apparmor: false` | -1 | Not set |
| `privileged`, `kernel_modules` | -1 | Not set |
| `hassio_role: manager` | -1 | Not set (`default`) |
| `host_network: true` | -1 | Not set |
| `hassio_role: admin`, `host_pid` | -2 | Not set |
| `full_access`, `docker_api` | forced to 1 | Not set |

Net 8, clamped to the ceiling of 6. The app declares no API grants at all:
the one Supervisor call it makes (`/addons/self/info`, for the Ingress
entry) is available to every app without a role.

## Trust boundaries

```
browser  ──HA session──►  Home Assistant  ──►  Supervisor Ingress (172.30.32.2)
                                                      │ strips client X-Remote-*,
                                                      │ adds X-Remote-User-Id/-Name/-Display-Name
                                                      ▼
                                   nginx-light :1337  (grafana user)
                                     allow 172.30.32.2, 127.0.0.1; deny all
                                     role := map(admin_users, editor_users, default_role)
                                     X-WEBAUTH-USER/-NAME/-ROLE := identity, role
                                     /terminal/ only if role == Admin and option on
                                          │                        │
                                          ▼                        ▼
                             Grafana :3000 (127.0.0.1)    ttyd :7681 (127.0.0.1)
                             auth.proxy whitelist 127.0.0.1    script → bash -l
                                          │
                                          ▼  HTTPS 17774, pinned certificate
                                   SolarWinds SWIS (outside the host)
```

**Boundary 1, browser to Home Assistant.** Home Assistant authenticates.
Nothing here changes that. Enforced by Home Assistant; verified by reading
`supervisor/api/ingress.py`: client-supplied `X-Remote-User-*` headers are
dropped and the session's user is attached (2026-09-16).

**Boundary 2, Ingress to nginx.** `grafana/rootfs/etc/nginx/nginx.conf`
allows `172.30.32.2` and `127.0.0.1` and denies everything else; a request
with no `X-Remote-User-Name` gets 401. Enforced. Verified by
`scripts/smoke_test.sh` (401 without the header). Loopback is allowed so
the container health check works; the only processes on loopback are this
app's own.

**Boundary 3, nginx to Grafana.** Grafana's auth proxy accepts identity
headers only from `127.0.0.1` (`grafana.ini.template`, `[auth.proxy]
whitelist`), and nginx overwrites all three headers on every request, so a
browser-supplied `X-WEBAUTH-ROLE: Admin` cannot reach Grafana. Enforced.
Verified by the smoke test (an Editor sending that header stays Editor).
Grafana's own source confirms `Role` is one of the five proxy header fields
(`pkg/services/authn/clients/proxy.go`, read 2026-09-16). The role is
re-evaluated per request through the sync cache, whose key includes the
header values, so a change in the options applies on the next request
after restart.

**Boundary 4, nginx to the terminal.** `/terminal/` returns 403 unless the
role is Admin and `terminal_enabled` is on; nginx rewrites the query string
so the username the session wrapper records is Ingress's, not the
browser's. Enforced. The 403 gate is verified by the smoke test; the URL
argument reaching the wrapper is **unverified** (it needs a WebSocket
client the smoke test does not have), and the wrapper records `unknown`
if it does not arrive.

**Boundary 5, the container to the host.** No Home Assistant directory is
mapped (`map: []`), no host network, no Docker socket, no API role, no
privileged capability, custom AppArmor. Enforced by `config.yaml`; the
mapping and grants are verified by reading it, the AppArmor profile is
**unverified** on a live Supervisor (below).

**Boundary 6, the container to SolarWinds.** The SWIS plugin's own TLS
handling: pinned certificate, name check optional, verification never
silently off. Enforced and verified by the plugin's Go tests in
OrionGuides.

## Controls, one by one

| Control | Where | Enforced or cosmetic | Verified |
| --- | --- | --- | --- |
| No Grafana login form, no basic auth | `grafana.ini.template` `[auth]`, `[auth.basic]` | Enforced | Smoke test: `disableLoginForm` true; basic auth with the real admin password gets 403 on the Ingress port |
| No default password | `run.sh` generates 32 random characters into `/data/secrets/admin_password` (0600) at first start and never logs it | Enforced | Smoke test reads the file to prove the password does not open the Ingress port |
| Cookie signing key not the Supervisor token | `run.sh` generates 32 random bytes into `/data/secrets/secret_key` | Enforced | By reading `run.sh`; the value is never printed |
| Supervisor token not in any child process | `run.sh` unsets it after the one Ingress-entry call, before starting anything | Enforced | Smoke test reads Grafana's `/proc/<pid>/environ` |
| Unprivileged processes | `setpriv --reuid=472 --regid=472 --clear-groups --inh-caps=-all` for nginx, ttyd and Grafana | Enforced | Smoke test checks `ps` for all three |
| Role per Home Assistant user | nginx `map` from `/run/grafana-app/roles.map`, written by `run.sh` from the options after validating each username against `^[A-Za-z0-9._@+-]{1,64}$` | Enforced | Smoke test: Admin, Editor, and 403 for an unlisted user |
| Default deny | `default_role: none` ships as the default and renders a static page | Enforced | Smoke test |
| Framing limited to the Home Assistant origin | `allow_embedding` option, default false, renders `add_header X-Frame-Options SAMEORIGIN always` into nginx; Grafana's own `allow_embedding` stays on because its header would be `deny`, which blocks the Ingress panel too | Enforced | Smoke test: the header is `SAMEORIGIN` on the Ingress port |
| Telemetry off | `[analytics]`, `[snapshots]`, `[news]` | Enforced | By reading the template |
| Plugin installs only through the options | `plugin_admin_enabled = false`; URL plugins hash-checked in `run.sh` before `unzip` | Enforced | By reading; the hash path is not exercised by the smoke test (no test plugin is hosted) |
| SWIS plugin provenance | Built in the Dockerfile from OrionGuides at commit `508ed54`, in pinned `golang` and `node` stages | Enforced by the build | Verified: the smoke test confirms the plugin is registered as a backend plugin without a signature complaint |
| Base image provenance | `grafana/grafana:13.2.2-ubuntu` by manifest digest | Enforced by the build | Digest read from Docker Hub 2026-09-16 |
| No network client tools | `Dockerfile` removes `wget`, `nc`, `netcat`, `telnet`, `ftp`; the base image has no `ssh`, `python3`, `git`, `rsync` | Enforced | Smoke test asserts each is absent |
| Terminal recording | `grafana_term_open` runs util-linux `script` with `--log-out` and `--log-timing`, hashes the transcript at exit into `index.jsonl` with the Home Assistant username | Enforced when the option is on | The wrapper is syntax-checked and shellchecked; a recorded session has not been produced in CI |
| Terminal idle timeout | bash `TMOUT` | Cosmetic (a running program ignores it) | n/a |
| Access log with identity | nginx `log_format` JSON, rotated by logrotate under `run.sh` | Cosmetic (it records; it prevents nothing) | Smoke test finds the username in the log |
| API port token-only | second nginx `server` on 3080 empties the auth-proxy headers; no login form, no basic auth, so only service-account tokens authenticate | Enforced | **Unverified** in CI (3080 is not exercised); by reading `nginx.conf` and the template |

## AppArmor profile

`grafana/apparmor.txt` replaces the generic default profile. Unlike the
Technitium DNS app's, its file rules are enumerated: the image is Grafana's
own and its layout is documented in Grafana's Dockerfile (homepath
`/usr/share/grafana`, binaries in `bin/`, the bundled plugin under
`plugins-bundled/`), and the writable trees are exactly `/data`,
`/run/grafana-app`, `/tmp`, `/var/lib/nginx` and `/var/log/nginx`. It grants
the launcher `chown`, `fowner`, `dac_override`, `setuid`, `setgid` and
`kill`, denies `sys_admin`, `sys_module`, `sys_ptrace`, `sys_rawio`,
`net_admin`, `net_raw`, `net_bind_service` (every port here is above 1024),
`dac_read_search`, `mknod`, `sys_chroot`, mounts and ptrace, and limits
network families to inet and inet6 stream and dgram.

It is **enforced and unverified**: only the Supervisor attaches it, and CI's
smoke test (a plain `docker run`) never does. The known failure mode is
fail-closed: a path Grafana or a plugin needs that is not listed produces
a permission error rather than a degraded run. A denial is logged to the
host's kernel audit log under enforce mode the same as it would be in
`complain` mode - only whether the operation is blocked differs, not
whether it is logged - so it is visible in Home Assistant's own UI at
Settings > System > Logs > Host without editing the profile or needing
host shell access first. The recovery procedure is in
[operations.md](operations.md), "AppArmor denials". See docs/decisions.md
for why complain mode is not this repository's default diagnostic step.

## What the terminal can and cannot do

Can: read and change anything under `/data` as the `grafana` user,
including `grafana.db` (dashboards, users, encrypted data source secrets),
the generated secrets, installed plugins and the transcripts; run `grafana
cli`; reach any network address the container can (SolarWinds, the Grafana
catalogue) with `curl`.

Cannot: read Home Assistant's configuration or `.storage`, reach the
Supervisor API (no token, no role), escalate (no `sudo`, no setuid tools
beyond the base image's), open an SSH, telnet or raw connection with a
shipped tool, or survive the container (nothing is installed persistently
except under `/data`).

`curl` and the ability to read `/data/secrets` are the accepted residuals:
the first is needed by the launcher and the health check, the second is
the operator's own state. Both are written down here rather than hidden.

## Container user

Grafana's image sets `USER 472`. This app's Dockerfile switches to root for
the launcher only, because the Supervisor mounts `/data` owned by root and
something has to hand it to user 472. After that, `run.sh` starts every
service through `setpriv` and holds no privilege it uses again except
`kill` for shutdown. The alternative, a Supervisor-side `data` mapping with
a non-root owner, does not exist in the app configuration schema as read on
2026-09-16.

## Out of scope

Grafana's own code, the Ubuntu base, and any plugin the operator installs.
CI scans the image (`security.yml`) and fails on fixable High or Critical
findings in OS packages and in anything this Dockerfile adds, including the
SWIS backend it compiles, which is placed under `/opt/grafana-app` so it is
scanned. Grafana's own binaries under `/usr/share/grafana` are excluded by
`.grype.yaml`, with the date and reason: they are upstream's build, only a
Grafana release changes them, and the first scan on 2026-09-16 found some
forty High advisories in the Go modules compiled into them (the Go standard
library at several versions, x/net, x/crypto, x/text, gRPC, OpenTelemetry,
a Tempo module). The exclusion is re-examined at every Grafana version
bump. Ubuntu's pending updates are applied at build time so the OS packages
are current at each build.
