# Hosting cost spike — verify Fly Managed Postgres pricing & pick a Phase 1 target

**Status:** Spike — research deliverable for new roadmap slice **#19.5**.
A decision recorded here unblocks slice #20.
**Supersedes:** none.
**Related:**
- [`docs/roadmap.md`](../../roadmap.md) (slice rows #19, **#19.5**, #20)
- [`docs/superpowers/specs/2026-05-18-qa-deploy-design.md`](./2026-05-18-qa-deploy-design.md)
  (Architecture table row "DB" + Slice #20 section both depend on this
  decision)
- [`docs/deploy.md`](../../deploy.md) — created by PR #11; its
  §2/§3/§Secrets-rotation/§Troubleshooting sections are the surfaces
  that change when this decision changes
- [`CLAUDE.md`](../../../CLAUDE.md) §Hosting — names "Fly Managed
  Postgres" and gets rewritten to match the chosen target
- Open PR [#11](https://github.com/mjrossi/urbanist-atlas/pull/11) (held
  pending this decision) and PR
  [#12](https://github.com/mjrossi/urbanist-atlas/pull/12) (unaffected)

## Why this exists

Slice #20 of the launch chunk commits Urbanist Atlas to **Fly Managed
Postgres** (MPG) as the production database. While reviewing the open
launch-chunk PRs (#11, #12), MPG's lowest-tier price turned out to be
roughly **$38/mo before storage** — an order of magnitude above what a
low-traffic dogfood directory needs to spend, and well above peer
managed-Postgres offerings ($5–$25/mo). For Phase 1 dogfooding — locked
down behind a shared-secret gate, with the maintainer and a handful of
invited testers as the entire user base — the $38 floor is not
defensible.

This doc verifies the pricing finds against vendor sources, surveys
alternatives across the "cheapest possible, manual backups OK"
tolerance the maintainer is willing to accept, and recommends a target
that lets the Phase 1 dogfood ship for **≤ $5/mo all-in**, with a clear
upgrade path if Phase 2 traffic ever justifies it.

The operator profile assumed here: a 13+ year engineer comfortable
self-managing services. That tilts the recommendation away from
managed-DBaaS premiums and toward "ship a Postgres container next to
the API, own the `pg_dump`."

## Verified pricing snapshot (May 2026)

### Fly Managed Postgres (the status quo)

| Plan | Monthly | CPU / RAM | Notes |
|---|---|---|---|
| Basic | **$38.00** | shared-2x / 1 GB | minimum entry |
| Starter | $72.00 | | |
| Launch | $282.00 | | |
| Scale | $962.00 | | |
| Performance | $1,922.00 | | |

Storage: **$0.28 per provisioned GB-month** on top of the plan price.
All plans include HA, backups, and connection pooling.

Confirmed against `https://fly.io/mpg`,
`https://fly.io/docs/about/pricing/`, and several 2026 third-party
breakdowns
([Kuberns](https://kuberns.com/blogs/flyio-pricing/),
[Costbench](https://costbench.com/software/developer-tools/flyio/),
[Toolradar](https://toolradar.com/tools/flyio/pricing)).
WebFetch direct against `fly.io/*` returned 403 in this environment, so
the figures here lean on aggregator quotes of the official docs rather
than the docs page directly. Anyone implementing the decision should
re-verify in the live Fly dashboard before provisioning.

### Fly Machines + Volumes (the building blocks for the recommended path)

- **shared-cpu-1x**, 256 MB RAM, always-on: **~$1.94/mo**
- Add 1 GB RAM: roughly +$1.24/mo (so a shared-cpu-1x / 1 GB machine is
  ~$3/mo)
- **Persistent volumes**: $0.15/GB-month per older quotes;
  newer aggregators report $0.08/GB-month with the first 10 GB free.
  At our 1 GB scale both round to "essentially free." **Verify against
  the live calculator at `https://fly.io/calculator` before
  committing** — pricing has shifted recently.
- Daily volume snapshots: included, 5-day retention, billed separately
  starting Feb 2026 (per the docs page).

A sibling Fly app running `postgres:17-alpine` with a 1 GB volume
therefore lands somewhere between **$2 and $3/mo all-in** for the DB
itself.

### Alternatives surveyed

| Option | Monthly floor | Storage | Backups | Pause / scale-to-zero | Notes |
|---|---|---|---|---|---|
| **Fly Managed Postgres Basic** | $38 + $0.28/GB | none included | yes | no | The status quo; not justifiable at Phase 1 scale |
| **Fly machine + `postgres:17-alpine` + 1 GB volume** | **~$2–3** | 1 GB volume | self-managed (`pg_dump`) | no | Same image as our testcontainers; we own ops |
| **Hetzner CX22 (whole stack)** | **€3.79 (~$4.59)** | 40 GB included | self-managed | no | 2 vCPU / 4 GB RAM; runs API + Postgres + Caddy via docker compose |
| **Neon Free** | $0 | 0.5 GB | branch-based | mandatory scale-to-zero, cold starts | 100 CU-hours/mo cap; fine for dogfood traffic |
| **Neon Launch** | ~$19 | 10 GB | yes | optional | The first Neon tier with disable-able autosuspend |
| **Supabase Free** | $0 | 500 MB | none | **project pauses after 7 days idle** | Free, but no backups + pause behavior is risky |
| **Supabase Pro** | $25 | 8 GB | yes | no | Cheapest backed-up Supabase tier |
| **Render Postgres Starter** | $7 | 1 GB / 256 MB RAM | yes (paid plans) | no | Backups included; very tight RAM |
| **Render Postgres Basic** | $20 | larger | yes | no | First "real production" Render tier |
| **Render Postgres Free** | $0 | 1 GB | none | **database deleted 30 days after creation** | Hard expiry makes this a non-starter for shipped infra |
| **Heroku Postgres Essential-0** | $5 | 1 GB | daily | no | 20-connection cap |
| **Heroku Postgres Essential-1** | $9 | larger | daily | no | Looser connection cap |
| **Railway** | ~$5 + usage | usage-based | yes | no | $5 hobby plan includes some credit |
| **Crunchy Bridge** | $35+ | varies | yes | no | Skipped — pricing similar to Fly MPG |

Source notes: all prices pulled from May-2026 WebSearch aggregator
results; vendor pricing pages should be reconfirmed at provisioning
time.

## Finalists

### Finalist 1 (recommended): Fly app + sibling Fly app running `postgres:17-alpine`

A second Fly app named `urbanist-atlas-db` runs the official
`postgres:17-alpine` image with a 1 GB volume mounted at
`/var/lib/postgresql/data`. The API app connects over Fly's internal
6PN at `urbanist-atlas-db.internal:5432` — no public exposure, no TLS
between API and DB (private network), no `.flycast` needed for
app-to-app traffic in the same org.

**Cost:** ~$2–3/mo for the DB (one shared-cpu-1x machine + 1 GB
volume + snapshots), in addition to whatever the API app already
costs. Total expected monthly bill: **under $5** at idle, slightly
more under load.

**Why this wins:**

- Uses the same Postgres image as our existing testcontainers
  integration suite (`postgres:17-alpine`), eliminating a class of
  "works in tests, breaks in prod" risk.
- Keeps the entire stack inside a single `flyctl` workflow — no second
  vendor dashboard, no second auth, no second bill.
- The "deprecated" Fly Postgres tooling
  ([Fly Postgres (Unmanaged)](https://fly.io/docs/postgres/)) is
  *irrelevant* here. We're not using `fly pg`; we're shipping a plain
  Postgres Docker image to a generic Fly app with a `[mounts]` block.
  Fly continues to support arbitrary container deploys to volumes.
- PR #11's `loaddata` code, integration tests, and `justfile` recipe
  survive unchanged. Only `docs/deploy.md` §2/§3/§Troubleshooting and
  the rotation procedure get rewritten.
- Backups are a `pg_dump | gzip` from a scheduled GitHub Actions job
  (the same workflow file CI already lives in) shipping to Cloudflare
  R2, plus an ad-hoc `just db-backup` for the maintainer.

**Trade-offs accepted:**

- No automatic point-in-time recovery — we get nightly logical dumps
  and Fly's daily volume snapshots, nothing finer-grained. Acceptable
  for a low-write dogfood window.
- Single-node, no failover. If the DB machine restarts we have a
  minute or two of downtime. Acceptable.
- Manual major-version upgrades (pin to 17 for now; revisit before
  18 EOLs 17).

### Finalist 2 (runner-up — pick this if you want to drop Fly entirely): Hetzner CX22 whole stack

A single Hetzner CX22 (2 vCPU / 4 GB RAM / 40 GB NVMe, ~$4.59/mo)
running the API container, `postgres:17-alpine`, and Caddy for
automatic TLS via docker compose. Deploys via a `deploy.sh` that does
`git pull && docker compose up -d --build` invoked from a GitHub
Actions job over SSH.

**Cost:** ~$4.59/mo total — DB *and* API together — for the lowest
sustainable production-shaped bill on this list. Plenty of headroom
for Phase 2 traffic before the bill changes.

**Why this is the runner-up rather than the primary:**

- Higher one-time setup: provision the box, harden SSH, install
  `fail2ban`, configure automatic security updates, write the docker
  compose, wire Caddy to the DNS records, set up the deploy script,
  set up off-box backups.
- Loses Fly's edge-routing and zero-downtime deploy primitives.
- Replaces the existing `fly.toml` and `fly-*` justfile recipes
  entirely — bigger PR-#11 churn (close + reopen rather than rebase).

**When to pick this instead:** if the maintainer wants the absolute
floor on monthly spend, doesn't value Fly's anycast / health-check
machinery for a single-region dogfood, and would rather front-load a
day of VPS setup than pay Fly's machine markup forever.

### Finalist 3 (zero-ops fallback): Fly app + Neon Free

Keep the Fly app from slice #19. Provision a Neon project on the free
tier (0.5 GB storage, 100 CU-hours/mo, mandatory scale-to-zero,
cold-start latency ~1–2 s on first request after idle). Set
`URBANIST_DB_URL` to the Neon pooler connection string.

**Cost:** $0/mo until we exceed Neon's free caps; ~$19/mo (Launch
tier) to lift the cold-start and add proper backups.

**Why this is the fallback rather than the primary:**

- Doesn't reward the maintainer's self-managed comfort — it's a
  managed DBaaS we'd be paying for in cold-start latency rather than
  dollars.
- 100 CU-hours/mo is enough for dogfood but constrains us if we run
  background ETL during the postal-data ingest (slice #7.5) or if
  invited testers do bulk lookups.
- The free tier has no SLA; we'd be lifting to ~$19/mo Launch before
  long.

**When to pick this instead:** if "I'd rather not babysit a Postgres
container" wins over the $2–3/mo savings.

## Recommendation

**Adopt Finalist 1** (Fly sibling app running `postgres:17-alpine`).
It's the lowest-bill option that doesn't require leaving the Fly
workflow, it's identical to our test wire, and PR #11's code half ships
unchanged. Finalist 2 (Hetzner) is a legitimate next-best if dropping
Fly is on the table. Finalist 3 (Neon) is the bail-out path if either
self-hosted finalist ever feels like drudgery.

## Decision

**Adopt Heroku Basic dyno + Heroku Postgres Essential-0 in the `us`
(Virginia, Common Runtime) region.** This was not one of the four
finalists in the original spike; brainstorming with the maintainer
surfaced operator familiarity (the maintainer's prior employer is
Heroku) as a decision axis the spike under-weighted, and re-verified
May-2026 pricing across Heroku, Render, Cloudflare Workers
Containers, Hetzner, and Fly that confirmed Heroku at ~$12/mo total
is a defensible Phase 1 spend in exchange for: included near-PITR
backups (Aurora-backed Essential-0 with continuous WAL off-premise),
zero learning tax, and a clean Phase 2 transition.

The ~$7/mo delta over the spike's recommended Finalist 1 (Fly
sibling Postgres at ~$5/mo) is a deliberate trade — see the cost
table and trade-off discussion in
[`2026-05-18-heroku-deploy-design.md`](./2026-05-18-heroku-deploy-design.md)
§Why this exists.

The full implementation design + cascading file rewrites are
specified in
[`2026-05-18-heroku-deploy-design.md`](./2026-05-18-heroku-deploy-design.md);
the implementation plan is at
[`../plans/2026-05-18-heroku-deploy-implementation.md`](../plans/2026-05-18-heroku-deploy-implementation.md).

PR #11 (slice-20-fly-deploy-loaddata) is closed; the work continues
on `slice-20-heroku-deploy`. PR #12 (slice #21, Pages/DNS) rebases
on the new `docs/deploy.md` after the Heroku PR lands. The
reversibility plan (migrate back to Fly if dogfood reveals a fit
problem) is in the Heroku design doc's §Reversibility section.

## Impact map (read once a decision lands)

| Outcome | PR #11 disposition | PR #12 | Files to rewrite |
|---|---|---|---|
| Stay on MPG | merge unchanged | merge | append "alternatives considered" appendix here |
| Finalist 1 (Fly sibling PG) | rebase same branch; rewrite `docs/deploy.md` §2/§3/§rotation/§troubleshooting; add `infra/postgres/{Dockerfile,fly.toml}` + `just db-backup` | merge | `docs/deploy.md`, `CLAUDE.md` §Hosting, `README.md` §Deploy, `docs/roadmap.md` slice #20, this spec's Architecture row + Slice #20 section, `api/README.md` |
| Finalist 2 (Hetzner) | close; cherry-pick `loaddata`/test/justfile commits onto a fresh branch; add `docker-compose.yml` + `deploy/`; retire `fly.toml` and `fly-*` justfile recipes | merge (Pages section just points at new API host) | `docs/deploy.md` rewritten end-to-end, plus same docs as Finalist 1 |
| Finalist 3 (Neon) | rebase same branch; rewrite `docs/deploy.md` §2 around the Neon dashboard + URL paste; drop the `flyctl mpg` re-mirror step | merge | same as Finalist 1, minus the infra/ files |

Files that survive **any** outcome unchanged: `api/cmd/server/loaddata.go`,
`api/internal/loaddata/loaddata.go`,
`api/internal/store/postgres/loaddata_test.go`,
`web/public/_redirects`, `mise.development.toml`,
`.github/workflows/ci.yml`, and every dev `pg-*` justfile recipe.

## Verification (post-decision)

1. `just api-check` clean on the rebased PR #11 branch.
2. `just api-test-integration` clean (testcontainers still uses
   `postgres:17-alpine`, wire unchanged).
3. Local smoke against the chosen DB:
   - Provision the DB target.
   - Export `URBANIST_DB_URL` to its connection string.
   - `cd api && go run ./cmd/server migrate up && go run ./cmd/server loaddata`.
   - `go run ./cmd/server serve` → `curl
     "http://localhost:8080/api/v1/lookup?postal_code=10001&country=US"`
     returns populated JSON.
4. After deploy: `curl -H "X-Atlas-Client: $SECRET"
   https://qa-api.urbanistatlas.com/healthz` returns 200; a `/lookup`
   call returns seeded data.
5. `grep -ri "Fly Managed Postgres" .` returns either nothing or only
   this doc's history section (or the "alternatives considered"
   appendix on the status-quo path).
