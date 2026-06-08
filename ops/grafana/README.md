# Grafana — Urbanist Atlas observability

Operator-side dashboards for the API's Prometheus metrics. Committed as
code so the dashboard is reproducible and reviewable; this tree parallels
[`etl/`](../../etl) (the other operator-side surface).

## Where the metrics come from

The API exposes a **private** Prometheus endpoint (`/metrics` on
`--metrics-port`, default `9091`). It binds all interfaces on Fly so the
managed-Prometheus scraper can reach it, yet stays private because the
port is **not** declared in `[http_service]`/`[[services]]`, so Fly's edge
never routes it to the public internet. Fly's **managed Prometheus**
scrapes it automatically (`[metrics]` in [`fly.toml`](../../fly.toml));
retention is ~30 days. Metric names and labels are defined in
[`api/internal/httpapi/metrics.go`](../../api/internal/httpapi/metrics.go).

## Dashboard: `dashboards/atlas-overview.json`

Panels (all PromQL over the `atlas_*` series):

| Panel | Question it answers |
|-------|---------------------|
| Request rate by route | Traffic shape; which endpoints are hot |
| Latency p50/p95/p99 | Is the API fast? Which routes are slow? |
| 5xx error ratio | Are we serving errors? (stat, color-thresholded) |
| Lookups hit/miss | How many ZIP/postal lookups resolve a region |
| Lookup result tiers (incl. **empty**) | **Coverage gap signal** — resolved region, zero orgs |
| Region search empty vs nonempty | Do submission-form searches find anything? |
| Region/org detail 404 ratio | Broken links / stale slugs in the wild |
| Submission funnel by status | created / rejected / rate-limited / error + admin actions |
| Readiness store-ping failures | SQLite volume reachability (should be flat zero) |

Per-region / per-org *popularity* is intentionally **not** a Prometheus
panel — slug is unbounded-cardinality, so popularity rides the DEBUG
`region view` / `org view` slog lines instead (query them in Fly's log
search). See the design spec for the rationale:
[`docs/superpowers/specs/2026-06-08-observability-design.md`](../../docs/superpowers/specs/2026-06-08-observability-design.md).

## Importing into Fly's managed Grafana (default)

Fly provides a hosted Grafana wired to the managed Prometheus — no extra
account or secret.

1. Open Grafana from the Fly dashboard (Monitoring → Grafana) or
   `https://fly-metrics.net`.
2. **Dashboards → New → Import → Upload JSON file** → pick
   `dashboards/atlas-overview.json`.
3. When prompted for the **Prometheus** data source, choose the Fly
   managed Prometheus (the `datasource` template variable then drives
   every panel).

On a dashboard change, re-export from Grafana (**Share → Export → Save to
file**, *Export for sharing externally* on) and overwrite the committed
JSON so the repo stays the source of truth.

## Optional upgrade: Grafana Cloud free tier

Only if >30-day retention or richer alert routing is wanted later. Add a
Prometheus **remote-write** target on Fly pointing at the Grafana Cloud
endpoint (store the API key as a Fly secret — see
[`docs/deploy.md`](../../docs/deploy.md) §Application secrets) and import
the same JSON. Deferred for v1.

## Alerting

Metric-threshold alerts (5xx spike, `atlas_store_ping_failures_total`,
submission errors) can be added later in Grafana's built-in alerting and
are **optional** for launch. The launch-critical alerts run in GitHub
Actions (no extra infra):

- **Backup failure** — [`backup-sqlite.yml`](../../.github/workflows/backup-sqlite.yml) opens an issue on failure.
- **External uptime** — [`uptime.yml`](../../.github/workflows/uptime.yml) probes `/healthz` on a cron and opens an issue if it's down.
