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
#   unzip        unpacking verified custom plugin zips
#   the SolarWinds SWIS data source plugin, built here from the pinned
#   OrionGuides commit so the image carries its own provenance and no
#   plugin is downloaded at run time.
#
# The Ubuntu variant rather than Alpine is deliberate: nginx-light and ttyd
# are Ubuntu packages (nginx-light is the same lean nginx the Technitium DNS
# app uses on Debian), and the SWIS plugin's Go backend is a static binary
# that runs on either. See docs/decisions.md.

# ---- Stage 1: the SWIS data source backend, from the pinned source commit.
FROM golang:1.24-bookworm@sha256:1a6d4452c65dea36aac2e2d606b01b4a029ec90cc1ae53890540ce6173ea77ac AS swis-backend
ARG SWIS_PLUGIN_REPO=https://github.com/trooperthorn/SolarWinds_OrionGuides.git
ARG SWIS_PLUGIN_COMMIT=07c0f67f927b9e29ef27a61f7226896be7c8dab6
# The Supervisor passes BUILD_ARCH (amd64 or aarch64); a plain docker build
# passes nothing and the machine's own architecture is used.
ARG BUILD_ARCH
WORKDIR /src
RUN git init -q . \
    && git remote add origin "${SWIS_PLUGIN_REPO}" \
    && git fetch -q --depth 1 origin "${SWIS_PLUGIN_COMMIT}" \
    && git checkout -q FETCH_HEAD
WORKDIR /src/apps/grafana-swis-datasource
RUN set -eu; \
    case "${BUILD_ARCH:-$(uname -m)}" in \
        amd64|x86_64) goarch=amd64 ;; \
        aarch64|arm64) goarch=arm64 ;; \
        *) echo "unsupported architecture ${BUILD_ARCH:-$(uname -m)}" >&2; exit 1 ;; \
    esac; \
    CGO_ENABLED=0 GOOS=linux GOARCH="${goarch}" \
        go build -trimpath -ldflags="-s -w" -o "/out/gpx_swis_linux_${goarch}" ./pkg

# ---- Stage 2: the plugin frontend bundle, from the same commit.
FROM node:22-bookworm-slim@sha256:83f487e0a63425e5b4d146fb5e5be574bcbe1b7b843d3ebafdd95eaf7767a7e5 AS swis-frontend
COPY --from=swis-backend /src/apps/grafana-swis-datasource /src
WORKDIR /src
RUN npm ci --no-audit --no-fund --ignore-scripts && npm run build

# ---- Stage 3: the app image.
FROM grafana/grafana:13.2.2-ubuntu@sha256:9a68a82ec38931e1baacee5d471cf74aa42565eb49554caa5290663529de6941

USER root

# hadolint ignore=DL3008
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        nginx-light \
        ttyd \
        jq \
        sqlite3 \
        unzip \
        logrotate \
        procps \
    && rm -rf /var/lib/apt/lists/* \
    # Nothing that opens a connection to another host: the terminal is local
    # to this container by construction, and CI asserts these stay absent.
    && rm -f /usr/bin/wget /usr/bin/nc /usr/bin/netcat /usr/bin/telnet /usr/bin/ftp \
    # nginx and ttyd run as the grafana user (472); give them their runtime trees.
    && mkdir -p /var/lib/nginx /var/log/nginx /run/grafana-app \
    && chown -R 472:472 /var/lib/nginx /var/log/nginx /run/grafana-app \
    && rm -f /etc/nginx/sites-enabled/default

# The SWIS plugin, bundled. plugins-bundled is a separate path from the
# user plugin directory under /data so an app update replaces this one and
# never touches what the operator installed.
COPY --from=swis-frontend /src/dist /usr/share/grafana/plugins-bundled/trooperthorn-swis-datasource
COPY --from=swis-backend /out/ /usr/share/grafana/plugins-bundled/trooperthorn-swis-datasource/
RUN chmod 0755 /usr/share/grafana/plugins-bundled/trooperthorn-swis-datasource/gpx_swis_* \
    && chown -R root:root /usr/share/grafana/plugins-bundled

COPY rootfs /
RUN chmod 0755 /run.sh /usr/local/bin/grafana_term_open \
    && chmod 0644 /etc/nginx/nginx.conf /etc/grafana/grafana.ini.template /etc/profile.d/grafana_terminal.sh

ARG BUILD_VERSION
ARG BUILD_ARCH
LABEL \
    io.hass.version="${BUILD_VERSION}" \
    io.hass.type="app" \
    io.hass.arch="${BUILD_ARCH}" \
    org.opencontainers.image.title="Grafana (hardened)" \
    org.opencontainers.image.source="https://github.com/trooperthorn/ha_app_grafana" \
    org.opencontainers.image.licenses="Apache-2.0"

# nginx answers /api/health itself from loopback (nginx.conf allows
# 127.0.0.1), so a 200 means nginx is up and Grafana behind it answered.
HEALTHCHECK --interval=30s --timeout=5s --start-period=90s --retries=3 \
    CMD curl -fsS -o /dev/null http://127.0.0.1:1337/api/health || exit 1

# root here is the launcher only: /run.sh fixes the ownership of /data,
# which the Supervisor mounts root-owned, then starts nginx, ttyd and
# Grafana as user 472 and does nothing else. See docs/security.md.
ENTRYPOINT []
CMD ["/run.sh"]
