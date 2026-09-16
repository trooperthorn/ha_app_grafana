# Security Policy

## Reporting a vulnerability

Do not open a public issue containing exploit details, credentials, private
addresses, or logs. Use GitHub's private vulnerability-reporting feature for
this repository (Security, then Report a vulnerability). Include the app
version from `grafana/config.yaml`, the Home Assistant version, and the
steps that reproduce the problem.

## What is in scope

The app's own configuration and scripts: `grafana/config.yaml`,
`grafana/Dockerfile`, `grafana/apparmor.txt`, everything under
`grafana/rootfs/`, and the CI workflows. A weakness in Grafana itself, in
its base image, or in a plugin the operator installed belongs upstream;
this repository tracks Grafana's releases and rebuilds against them.

## Supported versions

The latest release only. The version is calendar-based (`YYYY.MM.DD.N`) and
the tag is the version with a `v` prefix.
