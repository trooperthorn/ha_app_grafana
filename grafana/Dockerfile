# Grafana (hardened) for Home Assistant.
#
# Built FROM Grafana's own OSS image, Ubuntu variant, pinned by tag and
# manifest digest (read from Docker Hub on 2026-09-16; grafana/grafana is
# the OSS edition, grafana/grafana-enterprise is the other one). Grafana's
# image already runs Grafana as the unprivileged user 472 and ships nothing
# but Grafana, curl, ca-certificates and tzdata. This file adds:
#
#   nginx-light  the Ingress reverse proxy and role gate (grafana/nginx.conf)
#   ttyd         the terminal server, off unless the option turns it on
#   jq           reads /data/options.json; bashio is not in this image
#   sqlite3      read-only inspection of grafana.db from the terminal
#   nano, less   the terminal's editor and pager; Ubuntu's minimal image has neither
#   procps       ps and pgrep, for the terminal and the smoke test
#   unzip        unpacking verified custom plugin zips
#   the SolarWinds SWIS data source plugin, built here from
#   grafana/plugins-src/swis-datasource (vendored into this repository from
#   trooperthorn/SolarWinds_OrionGuides at a pinned commit, the same as the
#   plugins below, rather than cloned at build time) so the image carries
#   its own provenance, no plugin is downloaded at run time, and this
#   build no longer breaks when that source repository's other branches
#   change.
#   the Technitium DNS data source plugin, built here from
#   grafana/plugins-src/technitium-datasource (this repository's own
#   source, since no separate upstream repository holds it), for the same
#   provenance reason.
#   the Music Assistant data source plugin, built here from
#   grafana/plugins-src/musicassistant-datasource, for the same reason.
#   the Unifi Network data source plugin, built here from
#   grafana/plugins-src/unifi-network-datasource, for the same reason.
#   the Unifi Protect data source plugin, built here from
#   grafana/plugins-src/unifi-protect-datasource, for the same reason.
#   the Home Assistant data source plugin, built here from
#   grafana/plugins-src/homeassistant-datasource, for the same reason.
#   the HA SOC data source plugin, built here from
#   grafana/plugins-src/hasoc-datasource, for the same reason.
#
# The Ubuntu variant rather than Alpine is deliberate: nginx-light and ttyd
# are Ubuntu packages (nginx-light is the same lean nginx the Technitium DNS
# app uses on Debian), and the SWIS plugin's Go backend is a static binary
# that runs on either. See docs/decisions.md.

# ---- Stage 1: the SWIS data source backend, from this repo's own source.
FROM golang:1.27.1-bookworm@sha256:648f440f42a0958804efb24df176f806f9d353b41f1c0627f666428e40310f6b AS swis-backend
# The Supervisor passes BUILD_ARCH (amd64 or aarch64); a plain docker build
# passes nothing and the machine's own architecture is used.
ARG BUILD_ARCH
COPY plugins-src/swis-datasource /src
WORKDIR /src
RUN set -eu; \
    case "${BUILD_ARCH:-$(uname -m)}" in \
        amd64|x86_64) goarch=amd64 ;; \
        aarch64|arm64) goarch=arm64 ;; \
        *) echo "unsupported architecture ${BUILD_ARCH:-$(uname -m)}" >&2; exit 1 ;; \
    esac; \
    CGO_ENABLED=0 GOOS=linux GOARCH="${goarch}" \
        go build -trimpath -ldflags="-s -w" -o "/out/gpx_swis_linux_${goarch}" ./pkg

# ---- Stage 2: the plugin frontend bundle, from the same source.
FROM node:26-bookworm-slim@sha256:cd9f682fa2885cd1056e830424764158570061c59736a1da836bc3d73df095ae AS swis-frontend
COPY plugins-src/swis-datasource /src
WORKDIR /src
RUN npm ci --no-audit --no-fund --ignore-scripts && npm run build

# ---- Stage 3: the Technitium DNS data source backend, from this repo's own source.
FROM golang:1.26.8-bookworm@sha256:9fdc884aacc3bec89b20ffc69f4bb369c78210e3e4f600387b5128b12c199f81 AS technitium-backend
ARG BUILD_ARCH
COPY plugins-src/technitium-datasource /src
WORKDIR /src
RUN set -eu; \
    case "${BUILD_ARCH:-$(uname -m)}" in \
        amd64|x86_64) goarch=amd64 ;; \
        aarch64|arm64) goarch=arm64 ;; \
        *) echo "unsupported architecture ${BUILD_ARCH:-$(uname -m)}" >&2; exit 1 ;; \
    esac; \
    CGO_ENABLED=0 GOOS=linux GOARCH="${goarch}" \
        go build -trimpath -ldflags="-s -w" -o "/out/gpx_technitium_dns_linux_${goarch}" ./pkg

# ---- Stage 4: the Technitium DNS plugin frontend bundle, from the same source.
FROM node:22-bookworm-slim@sha256:83f487e0a63425e5b4d146fb5e5be574bcbe1b7b843d3ebafdd95eaf7767a7e5 AS technitium-frontend
COPY --from=technitium-backend /src /src
WORKDIR /src
RUN npm ci --no-audit --no-fund --ignore-scripts && npm run build

# ---- Stage 5: the Music Assistant data source backend, from this repo's own source.
FROM golang:1.26.8-bookworm@sha256:9fdc884aacc3bec89b20ffc69f4bb369c78210e3e4f600387b5128b12c199f81 AS musicassistant-backend
ARG BUILD_ARCH
COPY plugins-src/musicassistant-datasource /src
WORKDIR /src
RUN set -eu; \
    case "${BUILD_ARCH:-$(uname -m)}" in \
        amd64|x86_64) goarch=amd64 ;; \
        aarch64|arm64) goarch=arm64 ;; \
        *) echo "unsupported architecture ${BUILD_ARCH:-$(uname -m)}" >&2; exit 1 ;; \
    esac; \
    CGO_ENABLED=0 GOOS=linux GOARCH="${goarch}" \
        go build -trimpath -ldflags="-s -w" -o "/out/gpx_music_assistant_linux_${goarch}" ./pkg

# ---- Stage 6: the Music Assistant plugin frontend bundle, from the same source.
FROM node:22-bookworm-slim@sha256:83f487e0a63425e5b4d146fb5e5be574bcbe1b7b843d3ebafdd95eaf7767a7e5 AS musicassistant-frontend
COPY --from=musicassistant-backend /src /src
WORKDIR /src
RUN npm ci --no-audit --no-fund --ignore-scripts && npm run build

# ---- Stage 7: the Unifi Network data source backend, from this repo's own source.
FROM golang:1.26.8-bookworm@sha256:9fdc884aacc3bec89b20ffc69f4bb369c78210e3e4f600387b5128b12c199f81 AS unifinetwork-backend
ARG BUILD_ARCH
COPY plugins-src/unifi-network-datasource /src
WORKDIR /src
RUN set -eu; \
    case "${BUILD_ARCH:-$(uname -m)}" in \
        amd64|x86_64) goarch=amd64 ;; \
        aarch64|arm64) goarch=arm64 ;; \
        *) echo "unsupported architecture ${BUILD_ARCH:-$(uname -m)}" >&2; exit 1 ;; \
    esac; \
    CGO_ENABLED=0 GOOS=linux GOARCH="${goarch}" \
        go build -trimpath -ldflags="-s -w" -o "/out/gpx_unifi_network_linux_${goarch}" ./pkg

# ---- Stage 8: the Unifi Network plugin frontend bundle, from the same source.
FROM node:22-bookworm-slim@sha256:83f487e0a63425e5b4d146fb5e5be574bcbe1b7b843d3ebafdd95eaf7767a7e5 AS unifinetwork-frontend
COPY --from=unifinetwork-backend /src /src
WORKDIR /src
RUN npm ci --no-audit --no-fund --ignore-scripts && npm run build

# ---- Stage 9: the Unifi Protect data source backend, from this repo's own source.
FROM golang:1.26.8-bookworm@sha256:9fdc884aacc3bec89b20ffc69f4bb369c78210e3e4f600387b5128b12c199f81 AS unifiprotect-backend
ARG BUILD_ARCH
COPY plugins-src/unifi-protect-datasource /src
WORKDIR /src
RUN set -eu; \
    case "${BUILD_ARCH:-$(uname -m)}" in \
        amd64|x86_64) goarch=amd64 ;; \
        aarch64|arm64) goarch=arm64 ;; \
        *) echo "unsupported architecture ${BUILD_ARCH:-$(uname -m)}" >&2; exit 1 ;; \
    esac; \
    CGO_ENABLED=0 GOOS=linux GOARCH="${goarch}" \
        go build -trimpath -ldflags="-s -w" -o "/out/gpx_unifi_protect_linux_${goarch}" ./pkg

# ---- Stage 10: the Unifi Protect plugin frontend bundle, from the same source.
FROM node:22-bookworm-slim@sha256:83f487e0a63425e5b4d146fb5e5be574bcbe1b7b843d3ebafdd95eaf7767a7e5 AS unifiprotect-frontend
COPY --from=unifiprotect-backend /src /src
WORKDIR /src
RUN npm ci --no-audit --no-fund --ignore-scripts && npm run build

# ---- Stage 11: the Home Assistant data source backend, from this repo's own source.
FROM golang:1.26.8-bookworm@sha256:9fdc884aacc3bec89b20ffc69f4bb369c78210e3e4f600387b5128b12c199f81 AS homeassistant-backend
ARG BUILD_ARCH
COPY plugins-src/homeassistant-datasource /src
WORKDIR /src
RUN set -eu; \
    case "${BUILD_ARCH:-$(uname -m)}" in \
        amd64|x86_64) goarch=amd64 ;; \
        aarch64|arm64) goarch=arm64 ;; \
        *) echo "unsupported architecture ${BUILD_ARCH:-$(uname -m)}" >&2; exit 1 ;; \
    esac; \
    CGO_ENABLED=0 GOOS=linux GOARCH="${goarch}" \
        go build -trimpath -ldflags="-s -w" -o "/out/gpx_home_assistant_linux_${goarch}" ./pkg

# ---- Stage 12: the Home Assistant plugin frontend bundle, from the same source.
FROM node:22-bookworm-slim@sha256:83f487e0a63425e5b4d146fb5e5be574bcbe1b7b843d3ebafdd95eaf7767a7e5 AS homeassistant-frontend
COPY --from=homeassistant-backend /src /src
WORKDIR /src
RUN npm ci --no-audit --no-fund --ignore-scripts && npm run build

# ---- Stage 13: the HA SOC data source backend, from this repo's own source.
FROM golang:1.26.8-bookworm@sha256:9fdc884aacc3bec89b20ffc69f4bb369c78210e3e4f600387b5128b12c199f81 AS hasoc-backend
ARG BUILD_ARCH
COPY plugins-src/hasoc-datasource /src
WORKDIR /src
RUN set -eu; \
    case "${BUILD_ARCH:-$(uname -m)}" in \
        amd64|x86_64) goarch=amd64 ;; \
        aarch64|arm64) goarch=arm64 ;; \
        *) echo "unsupported architecture ${BUILD_ARCH:-$(uname -m)}" >&2; exit 1 ;; \
    esac; \
    CGO_ENABLED=0 GOOS=linux GOARCH="${goarch}" \
        go build -trimpath -ldflags="-s -w" -o "/out/gpx_ha_soc_linux_${goarch}" ./pkg

# ---- Stage 14: the HA SOC plugin frontend bundle, from the same source.
FROM node:22-bookworm-slim@sha256:83f487e0a63425e5b4d146fb5e5be574bcbe1b7b843d3ebafdd95eaf7767a7e5 AS hasoc-frontend
COPY --from=hasoc-backend /src /src
WORKDIR /src
RUN npm ci --no-audit --no-fund --ignore-scripts && npm run build

# ---- Stage 15: the app image.
FROM grafana/grafana:13.2.2-ubuntu@sha256:9a68a82ec38931e1baacee5d471cf74aa42565eb49554caa5290663529de6941

USER root

# The upgrade applies Ubuntu's pending security updates to the packages
# Grafana's image shipped with (glibc had four open advisories on
# 2026-09-16); the scan in security.yml gates the result.
# hadolint ignore=DL3008
RUN apt-get update \
    && apt-get upgrade -y --no-install-recommends \
    && apt-get install -y --no-install-recommends \
        nginx-light \
        ttyd \
        jq \
        sqlite3 \
        unzip \
        logrotate \
        procps \
        nano \
        less \
    && rm -rf /var/lib/apt/lists/* \
    # Nothing that opens a connection to another host: the terminal is local
    # to this container by construction, and CI asserts these stay absent.
    && rm -f /usr/bin/wget /usr/bin/nc /usr/bin/netcat /usr/bin/telnet /usr/bin/ftp \
    # nginx and ttyd run as the grafana user (472); give them their runtime trees.
    && mkdir -p /var/lib/nginx /var/log/nginx /run/grafana-app \
    && chown -R 472:472 /var/lib/nginx /var/log/nginx /run/grafana-app \
    && rm -f /etc/nginx/sites-enabled/default

# The SWIS plugin, bundled under this app's own tree rather than Grafana's,
# for two reasons: an app update replaces it without touching what the
# operator installed under /data/plugins, and the vulnerability scan can
# exclude Grafana's shipped binaries (/usr/share/grafana, upstream's build)
# while still scanning this one, which is built here. grafana.ini points
# bundled_plugins at this directory.
COPY --from=swis-frontend /src/dist /opt/grafana-app/plugins-bundled/trooperthorn-swis-datasource
COPY --from=swis-backend /out/ /opt/grafana-app/plugins-bundled/trooperthorn-swis-datasource/
COPY --from=technitium-frontend /src/dist /opt/grafana-app/plugins-bundled/trooperthorn-technitiumdns-datasource
COPY --from=technitium-backend /out/ /opt/grafana-app/plugins-bundled/trooperthorn-technitiumdns-datasource/
COPY --from=musicassistant-frontend /src/dist /opt/grafana-app/plugins-bundled/trooperthorn-musicassistant-datasource
COPY --from=musicassistant-backend /out/ /opt/grafana-app/plugins-bundled/trooperthorn-musicassistant-datasource/
COPY --from=unifinetwork-frontend /src/dist /opt/grafana-app/plugins-bundled/trooperthorn-unifinetwork-datasource
COPY --from=unifinetwork-backend /out/ /opt/grafana-app/plugins-bundled/trooperthorn-unifinetwork-datasource/
COPY --from=unifiprotect-frontend /src/dist /opt/grafana-app/plugins-bundled/trooperthorn-unifiprotect-datasource
COPY --from=unifiprotect-backend /out/ /opt/grafana-app/plugins-bundled/trooperthorn-unifiprotect-datasource/
COPY --from=homeassistant-frontend /src/dist /opt/grafana-app/plugins-bundled/trooperthorn-homeassistant-datasource
COPY --from=homeassistant-backend /out/ /opt/grafana-app/plugins-bundled/trooperthorn-homeassistant-datasource/
COPY --from=hasoc-frontend /src/dist /opt/grafana-app/plugins-bundled/trooperthorn-hasoc-datasource
COPY --from=hasoc-backend /out/ /opt/grafana-app/plugins-bundled/trooperthorn-hasoc-datasource/
RUN chmod 0755 /opt/grafana-app/plugins-bundled/trooperthorn-swis-datasource/gpx_swis_* \
        /opt/grafana-app/plugins-bundled/trooperthorn-technitiumdns-datasource/gpx_technitium_dns_* \
        /opt/grafana-app/plugins-bundled/trooperthorn-musicassistant-datasource/gpx_music_assistant_* \
        /opt/grafana-app/plugins-bundled/trooperthorn-unifinetwork-datasource/gpx_unifi_network_* \
        /opt/grafana-app/plugins-bundled/trooperthorn-unifiprotect-datasource/gpx_unifi_protect_* \
        /opt/grafana-app/plugins-bundled/trooperthorn-homeassistant-datasource/gpx_home_assistant_* \
        /opt/grafana-app/plugins-bundled/trooperthorn-hasoc-datasource/gpx_ha_soc_* \
    && chown -R root:root /opt/grafana-app

COPY rootfs /
RUN chmod 0755 /run.sh /usr/local/bin/grafana_term_open \
    && chmod 0644 /etc/nginx/nginx.conf /etc/grafana/grafana.ini.template /etc/profile.d/grafana_terminal.sh \
        /etc/grafana/provisioning-datasources/technitium.yaml.template \
        /etc/grafana/provisioning-datasources/musicassistant.yaml.template \
        /etc/grafana/provisioning-datasources/unifiprotect.yaml.template \
        /etc/grafana/provisioning-datasources/unifinetwork.yaml.template \
        /etc/grafana/provisioning-datasources/homeassistant.yaml.template \
        /etc/grafana/provisioning-datasources/hasoc.yaml.template

ARG BUILD_VERSION
ARG BUILD_ARCH
LABEL \
    io.hass.version="${BUILD_VERSION}" \
    io.hass.type="app" \
    io.hass.arch="${BUILD_ARCH}" \
    org.opencontainers.image.title="Grafana (hardened)" \
    org.opencontainers.image.source="https://github.com/trooperthorn/ha_app_grafana" \
    org.opencontainers.image.licenses="MIT"

# nginx answers /api/health itself from loopback (nginx.conf allows
# 127.0.0.1), so a 200 means nginx is up and Grafana behind it answered.
HEALTHCHECK --interval=30s --timeout=5s --start-period=90s --retries=3 \
    CMD curl -fsS -o /dev/null http://127.0.0.1:1337/api/health || exit 1

# root here is the launcher only: /run.sh fixes the ownership of /data,
# which the Supervisor mounts root-owned, then starts nginx, ttyd and
# Grafana as user 472 and does nothing else. See docs/security.md.
ENTRYPOINT []
CMD ["/run.sh"]
