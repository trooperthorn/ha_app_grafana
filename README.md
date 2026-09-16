# Grafana (hardened) for Home Assistant

A Home Assistant app repository packaging Grafana OSS behind Home Assistant
Ingress, with the security defaults the community Grafana app does not have,
the SolarWinds SWIS data source built into the image, a recorded terminal
that reaches nothing beyond the container, and a custom AppArmor profile.

It exists because the community app, read at its current source, gives every
Home Assistant user who can open the panel the Grafana Admin role, ships a
published default password, signs its cookies with the Supervisor token, maps
Home Assistant's configuration directory and TLS keys into the container for
every plugin to read, and fetches unsigned plugins from a URL on every boot
with no checksum. Each of those is reversed here, and
[docs/security.md](docs/security.md) says how and what remains unverified.

## Adding this repository

In Home Assistant, go to Settings, then Add-ons (App store), then the
three-dot menu, then Repositories, and add:

```
https://github.com/trooperthorn/ha_app_grafana
```

Then install "Grafana (hardened)" from the Local apps section, put your Home
Assistant username in `admin_users`, and start it.

## What is different

| The community app | This app |
| --- | --- |
| Every Ingress user is Admin | Role per Home Assistant username: `admin_users`, `editor_users`, everyone else `none` or `Viewer` |
| `admin` / `hassio`, basic auth on, login form on | No login form, no basic auth; the initial admin password is random, unlogged, and usable only on the optional API port |
| Cookie secret is the Supervisor token | 32 random bytes generated once into the app's data |
| `homeassistant_config`, `share`, `ssl` mapped in | Nothing mapped in; `/data` is the only writable tree |
| Plugins from a URL on every boot, no checksum | Plugins installed once into `/data/plugins`; URL plugins need a SHA-256 that is checked before unpacking; the SWIS data source is built into the image from a pinned commit |
| `allow_embedding` on for everyone | Off by default; an option turns it on for the iframe card |
| Telemetry, update checks, external snapshots on | Off |
| nginx and Grafana as root | Grafana, nginx and the terminal as the unprivileged `grafana` user; root only starts them |
| Default AppArmor profile | Custom profile: enumerated files, no escalation capabilities, no raw sockets |
| No terminal | Optional, administrators only, recorded, local to the container |
| No request log | JSON access log with the Home Assistant user id, name and role per request |

Both editions of Grafana upstream are unaffected by any of this: the image is
Grafana's own OSS image and every change is configuration in front of it.

## Documentation

- [grafana/DOCS.md](grafana/DOCS.md): options, roles, plugins, the terminal,
  the API port, logs and backups.
- [docs/security.md](docs/security.md): rating, trust boundaries, each
  control labelled enforced or cosmetic and verified or unverified.
- [docs/decisions.md](docs/decisions.md): why it is built this way.
- [docs/operations.md](docs/operations.md): first start, role changes,
  plugin installs, reading the logs and transcripts, AppArmor denials,
  backup and restore, the release process.
- [grafana/CHANGELOG.md](grafana/CHANGELOG.md).
- [SECURITY.md](SECURITY.md): vulnerability reporting.

## Status

`stage: experimental`. The image builds and passes its smoke test in CI (the
gates in [scripts/smoke_test.sh](scripts/smoke_test.sh) are exercised on
every push), but no real Home Assistant installation has run it yet, and the
AppArmor profile is enforced without having been observed on a live
Supervisor. See [docs/security.md](docs/security.md) for the list of what
that leaves open.
