# Provisioning

Files Grafana reads at start-up to configure itself. `docker compose up` in the parent
directory mounts this directory at `/etc/grafana/provisioning`.

- `datasources/datasources.yml` declares one "SolarWinds SWIS" data source with uid `swis`,
  reading the host, user, password and CA certificate from the `SWIS_HOST`, `SWIS_USER`,
  `SWIS_PASSWORD` and `SWIS_CACERT_PEM` environment variables. Nothing real is in the file.
- `dashboards/` is empty here. To provision the sample dashboard, add a provider that points
  at `../dashboards`:

```yaml
apiVersion: 1
providers:
  - name: SolarWinds
    folder: SolarWinds
    type: file
    options:
      path: /var/lib/grafana/dashboards/solarwinds
```

and mount the `dashboards/` directory at that path. The dashboard refers to the data source
by uid `swis`, which is what the provisioned data source carries.
