# Usage rollups & the monthly digest

**Status:** Shipped (2026-09-06).

**Related:**
- [`2026-06-08-observability-design.md`](./2026-06-08-observability-design.md) —
  the prior slice. This one closes the gaps that one deferred or left
  unconsumable. Decisions D3 and D4 there are load-bearing here.
- [`../../../api/internal/coverage/recorder.go`](../../../api/internal/coverage/recorder.go) —
  the fire-and-forget recorder this design mirrors.
- [`../../../api/internal/httpapi/metrics.go`](../../../api/internal/httpapi/metrics.go) —
  the Prometheus registry. Unchanged by this slice.
- [`../../deploy.md`](../../deploy.md) §Monitoring & incident response —
  gains a §Usage digest subsection.

## Why this exists

Cloudflare emailed to say the Atlas passed 10,000 unique pageviews last
month. That number was a surprise, and nothing in the project could
contextualize it: not whether it was up or down, not which regions drew
the traffic, not whether those visitors found anything.

The 2026-06-08 slice built good instrumentation. The problem is not
missing signal — it is that the signal is unconsumable, and in one case
never emitted at all:

1. **Per-slug popularity has never been recorded in production.**
   The 2026-06-08 slice's D3 routed region/org popularity to DEBUG slog
   lines rather than Prometheus labels — correct, since `slug` is unbounded
   cardinality. But `--log-level` defaults to `info`
   (`api/cmd/server/serve.go:103`) and `fly.toml` never sets
   `URBANIST_LOG_LEVEL`. So `lookup ok`, `region view`, `org view`, and
   `region search` (`lookup.go:104`, `regions.go:71,94,100`,
   `orgs.go:27,42`) have never been written to a production log. The
   most editorially useful question the project can ask — *which metros
   and orgs do people actually look at* — has no data behind it.

2. **A hard 30-day retention cliff.** Fly's managed Prometheus retains
   ~30 days. Month-over-month comparison is structurally impossible, so
   "is it growing?" is unanswerable no matter how good the dashboard is.

3. **Three disconnected, pull-only surfaces.** Pageviews live in
   Cloudflare, API metrics in Fly Grafana, coverage gaps behind a
   bearer-token `curl`. Nothing aggregates them.

4. **Nothing is ever pushed.** The only automated notifications are
   failure alarms (`uptime.yml`, `backup-sqlite.yml`). Nothing reports
   how the project is *doing*, which is why an external email was the
   first news of a traffic milestone.

## Decisions

| # | Decision | Why |
|---|----------|-----|
| D1 | **Daily rollups in the existing SQLite DB**, not a new store or a hosted TSDB. | Aggregating by day collapses the cardinality problem that pushed popularity onto DEBUG logs, so the signal becomes both durable and cheap. Reuses goose migrations, sqlc, and the mounted volume — and inherits the nightly R2 backup, so history is backed up for free. |
| D2 | **The digest is the consumption layer; it is pushed, not pulled.** | A solo maintainer will not remember to open a dashboard. Extends the existing alarm-opens-a-GitHub-issue convention rather than adding a surface to visit. |
| D3 | **Lookups roll up by resolved region slug, never by raw postal code.** | Honors the 2026-06-08 D4 privacy bar ("non-empty traffic is aggregate-only"). A full ZIP at low daily volume is semi-identifying; a region slug is a public content identifier — and it is the unit actually curated, so it is more useful editorially. |
| D4 | **Counts buffer in memory and flush on an interval**, rather than writing per request. | SQLite then sees one small transaction per minute regardless of traffic spikes, so usage writes can never contend with the submission path over the shared connection. Safe because `fly.toml` already pins the app to one machine with `auto_stop_machines = false` — the same assumption the rate limiter and PR worker queue document. |
| D5 | **Do NOT set `URBANIST_LOG_LEVEL=debug` in production.** | The rollup replaces that signal properly. Flipping it would flood short-retention logs to answer a question the table now answers durably. The DEBUG lines stay as a debugging aid. |
| D6 | **Each digest source degrades independently.** | A Cloudflare token hiccup must not cost the content and coverage numbers. Only a total failure of all four sources fails the run. |
| D7 | **No new runtime dependency, no SaaS, no Worker.** | Matches the project's minimal-deps, no-SaaS posture. Cloudflare Analytics Engine (client funnels) stays deferred as a future slice. |

## Architecture

Four components, each mirroring an existing pattern.

### 1. `internal/usage` — the recorder

Mirrors `internal/coverage` closely. A `Recorder` holds an in-memory
`map[bucketKey]int` behind a mutex; a background goroutine flushes to
SQLite every `flushInterval` (60s) and once more on shutdown via
`Wait()`. A nil `*Recorder` is a valid no-op so handlers may call it
unconditionally — the convention `*Metrics` and `*coverage.Recorder`
already establish.

```go
// Increment buckets one event. Returns immediately; the write happens
// on the next flush.
func (r *Recorder) Increment(kind, key string)
```

Worst case on an ungraceful kill is losing <60s of counts. That is
acceptable for usage analytics and is explicitly *not* the tradeoff made
for submissions, which remain synchronous.

### 2. Migration `0003_usage_daily.sql` + sqlc queries

```sql
CREATE TABLE usage_daily (
  day   TEXT    NOT NULL,          -- 'YYYY-MM-DD', UTC
  kind  TEXT    NOT NULL,
  key   TEXT    NOT NULL,
  count INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (day, kind, key)
) WITHOUT ROWID;
```

| kind | key | Source |
|------|-----|--------|
| `region_view` | region slug | `GET /regions/{slug}` |
| `org_view` | org slug | `GET /orgs/{slug}` |
| `lookup` | resolved region slug (D3) | `GET /lookup` |
| `lookup_tier` | `local`\|`regional`\|`statewide`\|`empty` | `GET /lookup` |
| `lookup_result` | `hit`\|`miss`\|`military` | `GET /lookup` |
| `lookup_country` | `US`\|`CA`\|`other` | `GET /lookup` |

A `lookup` row is written **only for a resolved lookup** — an
unresolved one has no region slug to key on. Every lookup, resolved or
not, writes `lookup_result` and `lookup_country`, so the durable
hit/miss rate survives the Prometheus retention cliff. `lookup_tier`
is written only for hits and sums to `lookup_result` = `hit`, mirroring
the existing `incLookupTier` invariant in `metrics.go`.

Flush is an upsert (`ON CONFLICT (day, kind, key) DO UPDATE SET
count = count + excluded.count`). Rows are pruned opportunistically to
~400 days, so a year-over-year comparison is always in hand. Queries live
in `internal/store/sqlite/queries/usage_daily.sql` and generate through
the existing `go generate ./...` path.

### 3. `GET /api/v1/admin/usage`

Bearer-gated, registered beside `coverage-gaps` in `router.go:168`.
Query params `from`, `to`, `kind`, `limit`. Returns aggregated rows over
the range. This is an `openapi.yaml` change, so Go and TS types
regenerate via `just api-gen`; `just api-check` gates the drift.

### 4. `.github/workflows/usage-digest.yml`

Monthly cron plus `workflow_dispatch`, following the
`actions/github-script@v9` pattern in `uptime.yml`. Four sources:

| Section | Source | Answers |
|---------|--------|---------|
| Audience | Cloudflare GraphQL Analytics API | Pageviews/visitors, vs. previous month |
| Content | `/api/v1/admin/usage` | Top regions + orgs, biggest movers |
| Coverage | `/api/v1/admin/coverage-gaps` | What returns nothing, ranked |
| Health | Fly Prometheus HTTP API | Request volume, p95, 5xx rate, submission funnel |

Both the previous month and the one before it are fetched, so every
number ships with a delta rather than a bare count.

Unlike `uptime.yml`, the digest opens a **new** issue each month rather
than reusing one open issue: each month is a durable record, and the
issue list becomes the archive.

**Secrets.** Reuses `FLY_API_TOKEN_DEPLOY` and `CF_ACCOUNT_ID`. Adds two:
`URBANIST_ADMIN_TOKEN` (to call the admin endpoints) and a
Cloudflare Analytics-scoped API token.

## Error handling

Each source is fetched independently; a failure degrades that section to
a "⚠️ unavailable" line rather than failing the run (D6). The workflow
fails — and opens an alarm issue — only when every source fails, which
indicates something real rather than a token hiccup.

Recorder writes are best-effort and off the request path: a flush error
is logged and the buffer is dropped, never retried into unbounded growth
and never surfaced to a user request. This matches
`coverage.Recorder`'s posture.

## Testing

- `internal/usage`: buffer accumulation, flush-on-interval,
  flush-on-shutdown, nil-recorder no-op, and concurrent increments under
  `-race`. Mirrors `internal/coverage/recorder_test.go`, injecting a
  fake clock rather than sleeping.
- `internal/store/sqlite`: upsert-accumulates, range query, prune
  boundary. Alongside `coverage_test.go`.
- `internal/httpapi`: the admin endpoint end-to-end including the
  503-without-token path, following `coverage_test.go`.
- Gates: `just api-test` (race), `just api-lint`, `just api-check`
  (codegen drift). Workflow YAML validated by `workflow_dispatch` on a
  branch before the first cron fires.

## Non-goals

- No Cloudflare Analytics Engine Worker / client funnels (form
  abandonment, search→click-through). Still deferred; the pure
  static-assets deploy stays Worker-free.
- No Grafana Cloud remote-write. D1 makes it unnecessary for retention.
- No change to the Prometheus registry or to `metrics.go`.
- No flipping of the production log level (D5).
- No paging, on-call, or status page.

## Incidental

`CLAUDE.md`'s approved-dependency list omits `sqlc`, which the repo
already uses (`api/sqlc.yaml`, `internal/store/sqlite/gen/`). Worth a
one-line correction while in the area.
