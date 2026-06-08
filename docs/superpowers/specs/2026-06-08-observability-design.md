# Observability & usage analytics

**Status:** Shipped (2026-06-08). Phases 1–3 (server-side usage metrics +
success logging, sampled coverage-gap capture, and the consumption layer
— dashboard + GitHub-Actions alerts + runbook) and Phase 4 (web error
visibility + request-ID correlation) are live. Client funnel analytics
are deferred — see §Deferred. The operator runbook is
[`docs/deploy.md`](../../deploy.md) §Monitoring & incident response.

**Related:**
- [`../../../CLAUDE.md`](../../../CLAUDE.md) §Tech conventions §Go —
  prometheus/client_golang is the only observability dep; slog for logs
- [`../../../api/internal/httpapi/metrics.go`](../../../api/internal/httpapi/metrics.go) — the Prometheus registry + series
- [`../../../api/internal/coverage/recorder.go`](../../../api/internal/coverage/recorder.go) — the sampled-empties recorder
- [`../../../ops/grafana/README.md`](../../../ops/grafana/README.md) — dashboard + import
- [`../../deploy.md`](../../deploy.md) — runbook (this slice adds §Monitoring & incident response)

## Why this exists

The app had solid *plumbing* (a private Prometheus endpoint, structured
slog, request IDs) but almost no *usage signal*: we could not answer
"how is the Atlas being used?" — which ZIPs/postal codes people search,
which searches come back empty, whether the submission form converts —
and there was no way to *see* the metrics we did collect (no dashboard,
no alerts), nor any client-side crash visibility.

This work closes those gaps **entirely within the already-approved
stack** (Prometheus + slog + Cloudflare; GitHub Actions for alerting),
with no error-tracking or product-analytics SaaS. As a solo project the
debugging model is reactive + manual: a user reports a problem with a
request ID, the maintainer greps the logs.

## Decisions

| # | Decision | Why |
|---|----------|-----|
| D1 | **Usage analytics first**, ops monitoring second. | The headline ask is "how is it used?", not SRE maturity. |
| D2 | **No error-tracking / product-analytics SaaS.** Extend Prometheus + slog + Cloudflare only. | Solo dev, privacy-first project, minimal-deps ethos. Logs + request-ID correlation are the primary debugging surface. |
| D3 | **Per-region/per-org popularity rides slog, NOT a Prometheus label.** | `slug` is unbounded (caller-supplied path params; ~628 regions + 236 orgs + attack churn). Keeps the registry's bounded-cardinality invariant (cf. `metricCountry`). Aggregate `{found}` counts go on Prometheus; per-slug popularity is a log query. |
| D4 | **Privacy bar = "aggregate + sampled empties".** Non-empty traffic is aggregate-only (Prometheus); raw user input is persisted **only** for EMPTY results, sampled + row-capped. | Coverage-gap data ("which input returns nothing?") is high editorial value; capturing it only for empties, sampled, and off by default keeps the cookieless/no-PII posture. |
| D5 | **Coverage writes are fire-and-forget, off the request path**, drained on shutdown. | A coverage write must never block or fail a user request; `Wait()` on shutdown avoids losing sampled rows. |
| D6 | **Consumption = Fly's bundled Grafana + GitHub-Actions alerts.** Grafana Cloud is a deferred optional upgrade. | No new account/secret/SaaS. Backup-failure and uptime are the two things that fail silently → GH issues. |

## What shipped

**Phase 1 — server-side usage metrics + logging** (no deps, no wire
change; `metrics.go` + handlers). New `atlas_*` series:
`lookup_results_total{country,tier}` (tier ∈ local|regional|statewide|
**empty**), `region_views_total{found}`, `org_views_total{found}`,
`region_search_total{result}` + `region_search_results` /
`region_search_query_length` histograms (length only, never the text),
`submission_validation_failures_total{field}` (clamped),
`admin_actions_total{action,outcome}`, `store_ping_failures_total`. Read
handlers emit DEBUG success logs (`lookup ok`, `region view`, `org view`,
`region search`); admin approve/reject emit INFO audit logs. No raw
postal/query is logged on success.

**Phase 2 — coverage-gap capture** (D4/D5). `coverage_gaps` SQLite table
(migration 0002) in the existing DB; `internal/coverage.Recorder` owns
the sampling RNG + the detached-context async write + opportunistic
prune. Lookup (resolved region, zero orgs) and search (non-blank query,
no matches) call `RecordEmpty`. Flags `URBANIST_COVERAGE_SAMPLE_RATE`
(default 0 = off) / `URBANIST_COVERAGE_MAX_ROWS` (default 5000). New
bearer-gated `GET /api/v1/admin/coverage-gaps` (wire-contract change;
`CoverageGap` schema; Go + TS types regenerated).

**Phase 3 — consumption.** `ops/grafana/dashboards/atlas-overview.json`
(import into Fly Grafana); `uptime.yml` (external `/healthz` probe →
issue) and a failure-notify step in `backup-sqlite.yml`; `docs/deploy.md`
§Monitoring & incident response.

**Phase 4 — web error visibility** (no SaaS, no new runtime deps). A
graceful error boundary (`web/src/routes/ErrorBoundary.tsx`) that
branches 404 vs unexpected error and surfaces the request id on the
error page so a user can quote it; `web/src/lib/clientErrors.ts` —
`requestIdOf` + dev-only `reportClientError` + `installGlobalErrorLogging`
(window `error`/`unhandledrejection`); react-query `QueryCache` /
`MutationCache` `onError` in `main.tsx` logging the `rid` centrally in
dev. Web Vitals piggyback on Cloudflare RUM (no code). This is the
SaaS-free realization of the "error tracking" the maintainer opted into:
the request id is the bridge from a user report to the server logs.

## Deferred

**Client usage funnels** (form abandonment, search→result click-through,
bounce) — the signals the server genuinely cannot see. The original plan
sketched these on "Cloudflare Web Analytics custom events," but CWA (the
cookieless RUM beacon this app uses) has **no JS custom-event API** —
custom events are a Zaraz / Analytics Engine capability, and both pull in
infrastructure (a tag-manager layer, or a Worker) that the no-SaaS /
pure-static-assets posture rejects for v1. CWA's automatic pageviews
(per-route popularity) plus the Phase 1 server metrics already cover the
bulk of "how is it used," so funnels wait for a deliberate sink choice:
either a minimal Cloudflare **Analytics Engine** Worker, or a first-party
`POST /api/v1/events`-style endpoint landing in Prometheus/SQLite. The
latter is also the path if client-side render crashes ever need to reach
the server logs.

## Verification

- Go: `just api-test` (race), `just api-lint`, `just api-check` (incl.
  codegen drift). Metrics asserted via `prometheus/.../testutil` in
  `metrics_test.go`; recorder sampling via a seeded RNG in
  `internal/coverage/recorder_test.go`; the admin endpoint + empty-capture
  path end-to-end in `httpapi/coverage_test.go`.
- Local: `just api-run`, drive traffic, `curl 127.0.0.1:9091/metrics |
  grep atlas_`; with `URBANIST_COVERAGE_SAMPLE_RATE=1`, hit an empty
  search and read it back from `/api/v1/admin/coverage-gaps`.
- Dashboard: `jq . ops/grafana/dashboards/atlas-overview.json`; import
  into Fly Grafana and confirm panels populate.

## Non-goals (v1)

- No OpenTelemetry / distributed tracing (request ID is the correlation
  primitive).
- No paging/on-call, no status page.
- No Grafana-provisioned metric alerts at launch (optional later).
- No exhaustive query logging — coverage capture is sampled by design.
