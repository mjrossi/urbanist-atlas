# Postal-code coverage — design spec

**Status:** Approved (2026-05-19). Implementation plan at
`~/.claude/plans/review-the-roadmap-for-twinkling-volcano.md` (local to
the maintainer); execution across sub-slices #7.5.1 → #7.5.4.
**Slice:** Roadmap #7.5 — *Full-country postal data ingest*. Lands
before Phase 1 dogfood execution so the live demo returns credible
answers for any US/CA postal code.

---

## Context

`postal_codes` today contains 47 hand-picked fixture rows (29 US ZIPs,
10 CA FSAs, 8 PT codes from the slice #4.6 validation set). For Phase 1
dogfooding (the launch posture in `CLAUDE.md`), users typing any US ZIP
or Canadian postal code into the lookup form need a credible answer —
either real local orgs, or a clean "no local orgs in your area, here
are statewide/provincial ones" fallback. Today, ~99% of US/CA postal
codes return `postal-code-not-found`.

The roadmap's slice #7.5 calls for "Full-country postal data ingest" —
replacing the bundled fixtures with real Census ZCTA + StatsCan PCCF
data. Naive ingest hits a wall: the schema requires every postal code
to point at a region via `postal_codes.leaf_region_id`, but there are
only ~25 leaf regions today vs. ~33,000 US ZCTAs and ~1,600 CA FSAs.
99%+ of postal codes would have no leaf to point at.

This spec resolves that tension and breaks #7.5 into four executable
sub-slices.

## Goals

- **Make every US ZIP and CA FSA resolve** to a meaningful region —
  city leaf where we've curated one, MSA region where we haven't,
  state/province otherwise. No `postal-code-not-found` for valid
  US/CA codes.
- **Schema unchanged**: no migration to support the broader ingest.
  `lookup.sql` and the recursive ancestor walk work as-is.
- **Editorially scalable**: growing coverage from "Brooklyn" to "every
  ZIP in Brooklyn" should be a data edit, not a code change. The
  pipeline scales to 33k+ US rows and reproduces byte-identically.
- **Carry NYC's borough granularity** at v1, because boroughs are
  counties + have distinct civic identity + drive borough-specific
  advocacy ecosystems. Other US cities stay single-leaf for v1.
- **Preserve `local-first` editorial ethos** for US/CA — most postal
  codes should *not* return a long list of state/national results;
  results stay local where local exists, and degrade gracefully where
  it doesn't.

## Non-goals

- Chicago community areas and other sub-municipal neighborhoods. The
  ZCTA-to-county crosswalk doesn't distinguish them, and ZIP↔
  community-area mappings have fuzzy boundaries. Deferred until org
  density justifies the editorial work.
- LA County sub-cities (Santa Monica, Beverly Hills, Pasadena, …) as
  separate leaves. They map to the LA-metro region in v1; promoted to
  leaves only when org coverage warrants.
- Schema renames (`leaf_region_id` → `anchor_region_id`). The "leaf"
  name becomes a misnomer (the column now points at any region, not
  necessarily a leaf), but the rename costs a migration + SQL touch
  with no behavioral change. Deferred.
- Other countries (PT/UK/MX/DE/etc.). PT stays at its existing
  validation fixture. Adding a country is a separate slice that
  follows the same pattern documented here.
- Vintage upgrade flow. Census ZCTAs change every decennial census;
  PCCF updates quarterly. v1 pins to a specific vintage in
  `etl/SOURCES.md`; upgrades are a deliberate future slice.

---

## Design

### Smallest-anchor model

The core insight: `postal_codes.leaf_region_id` is a foreign key into
`regions`, but the schema has no constraint that the referenced row
must be a *leaf* in the DAG sense (no children). The recursive
ancestor walk in `lookup.sql` traverses upward from whatever region
the postal code points at — leaf city, intermediate metro, or
top-level state.

This unlocks a clean model: **point every postal code at the smallest
curated region for its area.**

Anchor priority per postal code, applied at seed-time:

```
if curated city leaf exists for ZCTA's place        → anchor = city leaf
elif ZCTA is in an NYC borough county               → anchor = borough leaf
elif ZCTA is in an MSA we curated                   → anchor = MSA region
else                                                → anchor = state/province
```

Concrete examples:

| Postal code  | Anchor         | Why                                                           |
| ------------ | -------------- | ------------------------------------------------------------- |
| 10001 (NYC)  | `manhattan`    | NYC borough county = New York County                          |
| 11217 (NYC)  | `brooklyn`     | NYC borough county = Kings County                             |
| 94110 (SF)   | `sf`           | We've curated SF as a city leaf                               |
| 33401 (WPB)  | `miami-msa`    | West Palm Beach has no city leaf; falls under Miami MSA       |
| 83702 (ID)   | `id` (state)   | Boise has no leaf, not in a curated MSA; state fallback       |
| H2X (Mtl)    | `montreal`     | Montréal is a curated CA city leaf                            |
| K1A (Ottawa) | `ottawa-cma`   | Ottawa CMA; assumes we curate Ottawa's CMA at #7.5.4          |

The granularity of postal-code resolution becomes a *data* choice
rather than a *code* choice — and grows organically per-ZIP as we
curate more city leaves.

### Region graph additions

Four kinds of new region rows, totaling ~490 additions:

| Tier                              | Count | Source                                                  |
| --------------------------------- | ----- | ------------------------------------------------------- |
| US states + DC + PR               | 52    | Hand-defined once, stable                               |
| CA provinces + territories        | 13    | Hand-defined once, stable                               |
| US MSAs (Census CBSAs)            | 384   | Generated from Census CBSA list, top-50 hand-cleaned    |
| CA CMAs (Statistics Canada)       | 35    | Generated from StatsCan CMA list, all hand-cleaned      |
| NYC borough leaves (already exist)| 5     | Existing in `regions_us.toml`; reparented under nyc     |

Existing curated city leaves (SF, Boston, Chicago, LA, Toronto,
Vancouver, etc.) stay where they are. New states/provinces sit at the
top of their country subgraphs with no parents. MSAs/CMAs sit between
states and leaves, parented under one or more states (multi-state
MSAs like NYC-Newark-Jersey City get multi-parent edges, which the
DAG already supports).

The 5 NYC borough leaves already exist in `regions_us.toml` — they
just get re-parented under `nyc` (which itself flips from local leaf
to regional intermediate in #7.5.2; see the [NYC borough special
case](#nyc-borough-special-case) below).

### Postal-code anchor assignment

Two Census crosswalks drive US assignment:

1. **ZCTA-to-place** — gives a place (city/town/CDP) per ZCTA. Used
   for assigning city leaves and MSA membership. Some ZCTAs (rural)
   have no place; they fall through.
2. **ZCTA-to-county** — gives a county per ZCTA. Used for:
   - NYC borough resolution (NYC boroughs = counties)
   - Rural fallback when no place is defined
   - MSA membership (via county → MSA, since Census defines MSAs as
     aggregations of counties)

StatsCan PCCF gives Canada in one file: postal code → CSD (city) → CMA
→ province. We filter the ~80MB PCCF to just the FSA + CMA + province
columns at ETL time (don't commit the full file).

The smallest-anchor algorithm runs per-postal-code with both
crosswalks loaded:

```
crosswalk_lookup(zcta):
    place    = zcta_to_place[zcta]    # may be None
    county   = zcta_to_county[zcta]
    msa      = county_to_msa[county]  # may be None (rural)
    state    = county_to_state[county]

    if place in curated_city_leaves:
        return place_to_leaf_slug[place]
    if county in nyc_borough_counties:
        return nyc_borough_to_leaf_slug[county]
    if msa in curated_msa_regions:    # all 384, in #7.5.3
        return msa_to_slug[msa]
    return state_to_slug[state]
```

For PT data already in the seed, this slice leaves it alone — PT's
~7 fixture postal codes stay pointed at the leaves they already
target. The smallest-anchor pattern extends naturally to PT (and any
future country) but isn't part of #7.5.

### NYC borough special case

NYC is the **only US city we split sub-municipally at v1**. Three
things make NYC uniquely structurally splittable:

1. **NYC's boroughs are counties.** Manhattan = New York County,
   Brooklyn = Kings County, Queens = Queens County, Bronx = Bronx
   County, Staten Island = Richmond County. The Census ZCTA-to-county
   crosswalk hands us borough resolution for free.
2. **They have distinct civic identity.** Each borough has a Borough
   President and council districts. Borough-specific advocacy groups
   exist (Brooklyn Spoke is borough-only; Bronx-only community groups
   exist). Most other US "city subdivisions" (LA neighborhoods,
   Chicago community areas, DC wards, Boston neighborhoods) lack
   distinct civic governance.
3. **Citywide and borough-specific orgs are both common.** This needs
   a regional NYC region that can hold citywide orgs (TransitCenter,
   Transportation Alternatives, Riders Alliance) while borough-only
   orgs attach to specific borough leaves.

No other US city has this combination. Philadelphia + Philadelphia
County are coterminous. Indianapolis–Marion County is consolidated.
DC is its own district with no counties. Honolulu spans an entire
island as one consolidated city-county. None of them split
naturally the way NYC does.

The DAG shape after the #7.5.2 split:

```
ny (state, regional)
 └─ nyc (regional intermediate; was local leaf)
     ├─ manhattan (leaf, local)
     ├─ brooklyn (leaf, local)
     ├─ queens (leaf, local)
     ├─ bronx (leaf, local)
     └─ staten-island (leaf, local)

nyc-metro (regional, parented under nyc-tristate)
 ├─ nyc (the regional intermediate)
 ├─ jersey-city (NJ leaf, …existing)
 ├─ newark (NJ leaf, …if added)
 └─ … other metro leaves
```

After the split:
- `nyc`'s `scope_tier` flips from `local` to `regional`. Its only
  parent is `nyc-metro`. The `nyc → ny` edge is dropped — the
  boroughs now carry the `ny` state edge per
  [region-graph rule §1](../region-graph.md) ("state edges live on
  the leaf, not on the metro").
- The 5 borough leaves already exist; their `parents` array gets `ny`
  added alongside `nyc`.
- Citywide NYC orgs (TransitCenter, Transportation Alternatives,
  Riders Alliance per `orgs.toml` lines 18/26/34) attach to `nyc`.
  They'll bucket as **Regional** results after the split (was Local
  pre-split). Editorial decision per-org in #7.5.2: re-attach to a
  specific borough to bucket as Local, or accept the new
  categorization.
- Borough-only orgs (Brooklyn Spoke) attach to the specific borough
  leaf; they continue to bucket as Local.

Existing `postal_codes_us.csv` already anchors NYC ZIPs at borough
leaves (none at `nyc` directly — verified by inspection); so the
#7.5.2 migration's data-fix step for postal codes is a no-op. The
migration touches only the `regions` and `region_parents` tables.

### Editorial review tier

The 384 Census MSAs have clinical names:

```
"New York-Newark-Jersey City, NY-NJ-PA Metropolitan Statistical Area"
"Los Angeles-Long Beach-Anaheim, CA Metropolitan Statistical Area"
"Crestview-Fort Walton Beach-Destin, FL Metropolitan Statistical Area"
"Yuba City, CA Metropolitan Statistical Area"
"Pocatello, ID Metropolitan Statistical Area"
```

If we let the ETL slugify them verbatim, the UI would render
"New York-Newark-Jersey City, NY-NJ-PA Metropolitan Statistical Area"
as the metro label on the homepage Browse panel. Bad UX.

**Two-tier review** at launch:

- **Tight pass on top-50 US MSAs by population** — friendly slug +
  display name pair (e.g. `nyc-metro` / "New York Metro", `la-metro`
  / "Los Angeles Metro", `boise-metro` / "Boise Metro"). These are
  the metros a user is most likely to look at; getting them right
  matters most for first impressions. Source: Census 2020 MSA
  population ranks.
- **Auto-generated names for the long-tail ~334 MSAs** — slug rule:
  first city + `-metro` (e.g. "Crestview-Fort Walton Beach-Destin"
  → `crestview-metro`). Display name rule: first city + " Metro"
  ("Crestview Metro"). Usually produces something tolerable for
  smaller metros where there's no editorial bandwidth.

The naming overrides live in `regions_us_msa_overrides.toml`,
keyed by Census CBSA code:

```toml
[[override]]
cbsa_code = "35620"
slug = "nyc-metro"
name = "New York Metro"

[[override]]
cbsa_code = "31080"
slug = "la-metro"
name = "Los Angeles Metro"
```

ETL applies overrides during regeneration. Fixing a long-tail name
later costs one TOML entry plus a regenerate, no code change.

CA's 35 CMAs are small enough that we hand-clean all of them in
#7.5.4 (StatsCan names are slightly less clinical than Census MSAs
but still benefit from a pass).

### Out-of-coverage UX

A US ZIP outside any curated MSA (which is rare in v1 since #7.5.3
seeds *all* 384 MSAs) falls through to the state-tier anchor. A
typical Boise lookup:

```
GET /api/v1/lookup?postal_code=83702&country=US
→ resolved_ancestry = [id (state)]
→ local = []
→ regional = [orgs attached to Idaho, if any — likely empty in v1]
```

The `id` state region exists (seeded in #7.5.1) but has no orgs
attached in v1. The result is a clean "no orgs found yet for Idaho"
page rather than a generic `postal-code-not-found` error. The
frontend can render this as a soft empty state with a
"submit-an-org" CTA (deferred to Phase 2 #5) and a
"browse other metros" link.

Rural ZIPs inside an MSA (e.g., a small town inside the Pittsburgh
MSA without its own city leaf) anchor at the MSA. The lookup returns
metro-tier orgs as Regional results — still useful, since metro
advocacy groups typically work across the metro's full footprint.

NYC postal codes resolve at borough granularity. SF and Boston at
city level. Most other big-city ZIPs at city level where we curate
the leaf; at MSA level where we don't.

#### Two-source pipeline (ZCTA + HUD) — slice #7.5.5

Census ZCTA omits three ZIP categories: P.O. Box-only ZIPs (no
addressable buildings — e.g., 20811 covering NIH/Walter Reed in
Bethesda), single-building ZIPs (high-volume mail receivers like
big-corp HQs that USPS assigns a dedicated ZIP), and APO/FPO military
ZIPs. A ZCTA-only pipeline returns `postal-code-not-found` for any
address in those buckets.

Slice #7.5.5 adds HUD's quarterly USPS ZIP-to-County crosswalk as a
second US ETL source. The pipeline is **additive**:

1. **ZCTA is primary.** The existing 6-tier crosswalk
   (`api/internal/etl/us/crosswalk.go:Crosswalk`) runs unchanged and
   continues to produce ~33,700 anchors with city-leaf precision where
   curated.
2. **HUD is additive backfill** for ZIPs not already in the ZCTA
   output. `CrosswalkHUDBackfill` groups HUD rows by ZIP, picks the
   row with `max(TOT_RATIO)` per ZIP, and walks the county FIPS
   through the existing fallback chain (NYC borough → countyToLeaf →
   countyToMSA → stateFIPSToSlug).
3. **HUD-only anchors lack the city-leaf tier.** HUD ZIP-County
   doesn't carry a place GEOID, so the tier-1 city-leaf anchor isn't
   available for HUD-source ZIPs. A ZIP that would ideally land at a
   curated city leaf will anchor at the county leaf or MSA instead —
   acceptable for the P.O. Box / single-building cohort.
4. **TOT_RATIO selection is correct for P.O. Box-only ZIPs.** A
   `max(RES_RATIO)` pick would be undefined for ZIPs where every row
   has `RES_RATIO ≈ 0`. `TOT_RATIO` weights residential + business +
   other together and always identifies a meaningful primary county.
5. **The writer dedups defensively, with ZCTA winning.** If a ZIP
   somehow appears in both sources (e.g., a future Census update
   re-includes a ZIP HUD had been backfilling), the ZCTA-source row
   wins the (country, postal_code) tie at `WritePostalCodesCSV` time.

**Canonical example: ZIP 20811 → washington-dc-metro.** Bethesda's
NIH/Walter Reed P.O. Box ZIP is omitted from Census ZCTA but present
in HUD; its primary HUD row maps to Montgomery County, MD (FIPS
24031) which is in CBSA 47900 (Washington-Arlington-Alexandria MSA →
slug `washington-dc-metro`). A `GET /api/v1/lookup?postal_code=20811`
that used to 404 now returns DC-metro orgs in the Regional bucket.

**APO/FPO note.** Military overseas ZIPs (090xx, 962xx, etc.) appear
in HUD but map to unusual county FIPS (`999xx`, etc.) that don't
exist in the US county set. The fallback chain silently drops these
— overseas military bases anchoring to a US state is editorially
wrong, and we have no advocacy orgs to surface for them in v1
anyway.

**Operator workflow.** HUD's USPS Crosswalk requires a HUDUser
account; the download URL is account-scoped, so we can't auto-fetch
it the way Census files allow. The operator runs the manual download
(documented in `etl/SOURCES.md` → *HUD download — operator note*),
saves the CSV under `etl/sources/us/hud_zip_county_<vintage>.csv`,
pins the sha256 in both `etl/SOURCES.md` and `api/internal/etl/us/us.go`,
then runs `etl regenerate --country=US`. Absence of the HUD CSV is
not an error — the orchestrator logs a hint and produces a ZCTA-only
`postal_codes_us.csv`.

---

## ETL pipeline

### Location and binary structure

New `etl` subcommand on the existing `urbanist-atlas-server` binary,
sitting alongside `loadregions` / `loadpostal` / `seed`:

```sh
./urbanist etl download --country=US           # fetches source files to etl/sources/
./urbanist etl regenerate --country=US         # generates seed TOML + CSV files
./urbanist etl regenerate --country=CA
```

The subcommand reads source files from `etl/sources/<country>/` (a
gitignored directory; downloaded on demand or pre-staged) and writes
to `api/seed/`. Two output paths per country:

- `regions_<cc>_msas.toml` (or `_cmas.toml` for CA) — regional MSA/CMA
  entries
- `postal_codes_<cc>.csv` — postal code → anchor mapping (regenerated
  in full each run)

State/province TOMLs (`regions_us_states.toml`,
`regions_ca_provinces.toml`) are **hand-defined and not touched by
ETL**. The override file (`regions_us_msa_overrides.toml`) is also
hand-maintained and read by ETL during regeneration (overrides
applied to MSA entries before emit).

New Go package `api/internal/etl/` houses the parser + writer logic.
Operator-side, not exposed on the public `pkg/atlas` surface. Mirrors
the existing `internal/loadregions` / `internal/loadpostal` /
`internal/seed` pattern.

### Source datasets

| Country | File                                  | Vintage     | License                                                   | Size  |
| ------- | ------------------------------------- | ----------- | --------------------------------------------------------- | ----- |
| US      | Census CBSA delineation file          | 2023 update | Public domain                                             | ~50KB |
| US      | Census ZCTA-to-place relationship     | 2020        | Public domain                                             | ~3MB  |
| US      | Census ZCTA-to-county relationship    | 2020        | Public domain                                             | ~2MB  |
| CA      | Statistics Canada PCCF                | 2025-Q1     | StatsCan Open License (attribution required)              | ~80MB |
| CA      | Statistics Canada CMA reference       | 2021 census | StatsCan Open License (attribution required)              | ~20KB |

Each source's URL, vintage, and sha256 checksum are tracked in
`etl/SOURCES.md`. Re-downloading must produce a file with a matching
checksum or ETL fails loudly (defends against silent upstream changes
between vintages).

### Reproducibility

ETL output must be **byte-identical across runs given the same
upstream inputs**:

- Rows sorted alphabetically by primary key (postal_code for CSVs;
  slug for TOMLs).
- LF line endings; trailing newline.
- No timestamps, "generated_at" headers, or run-specific metadata
  in the output.
- The ODbL `generated_at` header in API responses is set at request
  time, separately, by middleware — not stamped into seed data.

Reasons: generated TOML/CSV files are committed under `api/seed/`.
Non-deterministic output produces noisy git diffs every regeneration,
which destroys the signal of "what changed when I re-ran the ETL".
The pipeline is upsert-based (`just loaddata` is already idempotent
per the integration suite); byte-identical seed files preserve
that property at the file layer too.

ETL itself logs the upstream sha256 checksums on each run so anyone
can verify reproducibility.

### Source files: gitignored, on-demand

`etl/sources/` is gitignored. ETL contributors run `etl download`
once (or stage source files manually) before `etl regenerate`. The
download step fetches each upstream URL, validates checksum against
`etl/SOURCES.md`, and unpacks to the per-country directory.

Trade-off: contributors who want to regenerate must have network
access to the upstream sources, and any source URL rot becomes
brittle. The alternative (committing source files) bloats the repo
and creates large noisy diffs on vintage upgrades. The maintainer
may revisit this if upstream rot becomes a real problem.

---

## Migration and cutover

No schema migrations in #7.5 — the smallest-anchor model uses the
existing schema as-is. The only structural data migration is
**#7.5.2's `0004_split_nyc.sql`**, which:

1. Flips `nyc.scope_tier` from `local` to `regional`.
2. Drops the `nyc → ny` parent edge.
3. Adds `ny` to each borough leaf's parents (so the boroughs carry
   the state edge per region-graph rule §1).
4. Leaves `postal_codes` untouched (verified that no rows anchor at
   `nyc` directly).

Subsequent sub-slices (#7.5.3, #7.5.4) ship generated TOML/CSV files
+ a batched-insert loader path. No schema changes.

---

## Sub-slice decomposition

#7.5 lands across four sub-slices, ordered so each leaves the system
in a consistent state:

| Sub-slice  | Scope                                                                | Why this order                                                                                                       |
| ---------- | -------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------- |
| **#7.5.1** | Design spec + ETL scaffolding + states/provinces seed                | Foundation. No data-scale change; pure infrastructure. Lands the pipeline before generating large datasets.          |
| **#7.5.2** | NYC borough split + reparenting (+ migration if needed)              | Structural change to existing data. Must come before MSAs so the anchor catalog is complete when ZCTA crosswalks resolve NYC-area ZIPs. |
| **#7.5.3** | US MSAs + ~33k ZCTA postal codes + batched `loadpostal` loader       | Scale jump. Includes batched insert path to avoid per-row latency on Heroku and slow CI.                              |
| **#7.5.4** | CA CMAs + ~1.6k FSA postal codes                                     | Smaller than #7.5.3; reuses the batched loader. Closes the slice.                                                     |

### #7.5.1 — Foundation (this slice ships now)

**Deliverables:**

- This spec.
- Roadmap update: `docs/roadmap.md` row for #7.5 broken into the
  four sub-slices above.
- Region-graph reference update: `docs/region-graph.md` gains a
  "NYC modeling (special case)" section and a "State-tier
  conventions" section explaining the anchor-priority resolution.
- `etl` subcommand scaffolded on the `urbanist` binary
  (`api/cmd/server/etl.go`) with `download` and `regenerate`
  subcommands — both no-op stubs for now; logic lands in #7.5.3/4.
- `api/internal/etl/` package skeleton with shared types.
- `api/seed/regions_us_states.toml` — 52 hand-defined entries (50
  states + DC + PR), each `scope_tier=regional`, `kind=us:state`,
  `sort_priority=60`, no parents.
- `api/seed/regions_ca_provinces.toml` — 13 hand-defined entries (10
  provinces + 3 territories), each `scope_tier=regional`,
  `kind=ca:province` or `ca:territory`, `sort_priority=60`, no
  parents.
- Existing state entries (NY, NJ, CT, IL, IN, WI, CA, MA, FL, WA)
  moved from `regions_us.toml` into `regions_us_states.toml`;
  existing province entries (BC, ON, QC) moved from `regions_ca.toml`
  into `regions_ca_provinces.toml`. Cleaner separation by concern.
- `api/internal/loaddata/loaddata.go` extended to load the new
  state/province files first (states have no parents, can load in
  any order, but load them first for clarity).
- `etl/SOURCES.md` placeholder structure; per-country source list
  filled in by #7.5.3/4.
- `justfile` recipes `etl-download-us`, `etl-download-ca`,
  `etl-regenerate-us`, `etl-regenerate-ca`.
- `api/seed/README.md` updated to document the new file layout + ETL
  workflow.

**Verification:**

- `mise install && just pg-reset && just loaddata` succeeds on a
  fresh DB.
- `psql "$DATABASE_URL" -c "SELECT count(*) FROM regions WHERE kind = 'us:state'"`
  returns 52.
- `psql "$DATABASE_URL" -c "SELECT count(*) FROM regions WHERE kind IN ('ca:province', 'ca:territory')"`
  returns 13.
- Existing integration tests pass unchanged.
- `urbanist etl regenerate --country=US` runs without error (no-op
  stub) and exits cleanly.

### #7.5.2 — NYC borough split

**Deliverables:**

- Migration `api/migrations/0004_split_nyc.sql` per the
  [NYC borough special case](#nyc-borough-special-case) section.
- `regions_us.toml`: update `nyc` to `scope_tier=regional`, drop
  `ny` from parents (keep `nyc-metro`). Add `ny` to each borough
  leaf's parents.
- Update the hand-built NYC subgraph fixture in
  `api/internal/store/postgres/store_test.go:308-370` to match the
  new shape.
- **Editorial decision** during the slice: re-attach citywide NYC
  orgs (TransAlt, Riders Alliance, Brooklyn Spoke per `orgs.toml`)
  to specific boroughs to keep them bucketing Local, or accept they
  bucket Regional post-split.

**Verification:**

- Lookup of `11217 US` returns `brooklyn` as the leaf, with `nyc`,
  `nyc-metro`, `ny`, `nyc-tristate` as ancestors. `nyc.scope_tier`
  shows `regional`.
- The 3 NYC-citywide orgs land in Regional results (or Local if
  re-attached to a borough during the editorial pass).
- Borough-only orgs (Brooklyn Spoke) continue to bucket as Local
  for borough ZIPs.

### #7.5.3 — US MSAs + ZCTA postal codes

**Deliverables:**

- ETL parsers for: Census CBSA delineation file, ZCTA-to-place
  relationship file, ZCTA-to-county relationship file.
- `api/seed/regions_us_msas.toml` — 384 entries, deterministic
  ordering, auto-generated slug + name for the long tail.
- `api/seed/regions_us_msa_overrides.toml` — top-50 hand-cleaned
  entries.
- `api/seed/postal_codes_us.csv` — regenerated, ~33k rows, sorted
  by postal_code ascending, anchored per the smallest-anchor
  algorithm.
- **Batched insert path**: new sqlc query
  `UpsertPostalCodesBatch` taking three text/bigint arrays via
  Postgres `unnest`, called in chunks of ~500 by `loadpostal` for
  files above ~1k rows. Avoids per-row network latency on Heroku.
- **Integration test mini-fixture**:
  `api/internal/loaddata/testdata/seed-mini/` with a scaled-down
  region + postal_code + org fixture (a few dozen rows). Integration
  tests point at `seed-mini/`; the full `api/seed/` data only
  loads under `just loaddata` against real Heroku Postgres.
- `etl/SOURCES.md` populated with US source vintage + checksums.
- `LICENSE-DATA` updated with Census ZCTA + CBSA attribution.

**Verification:**

- `urbanist etl regenerate --country=US` produces the same
  `regions_us_msas.toml` + `postal_codes_us.csv` byte-for-byte
  across two consecutive runs.
- Lookup of varied US ZIPs returns the expected anchor (city leaf
  for SF/Boston/Chicago ZIPs; MSA for Crestview/Pocatello-like
  ZIPs; state for genuinely rural).
- Integration test runtime stays under ~30s (mini-fixture).
- `just loaddata` against a fresh dev Postgres completes in <60s
  (batched insert path).

### #7.5.4 — CA CMAs + FSA postal codes

**Deliverables (as shipped):**

- ETL parsers for the StatsCan FSA + CMA boundary file DBFs (NOT
  the PCCF — see "Sourcing pivot" below). The boundary zips are
  parsed in-place: only the ~150KB DBF attribute tables are read;
  the ~300MB shapefile geometry is ignored.
- Minimal stdlib-only DBF reader at `api/internal/etl/ca/dbf.go`
  (no new Go dependency).
- `api/seed/regions_ca_cmas.toml` — 41 entries (CMATYPE='B' filter;
  Census Agglomerations of type 'D' filtered out). 4 hand-cleaned
  overrides (toronto-cma, montreal-cma, metro-vancouver,
  ottawa-gatineau-cma); the remaining 37 use auto-generated
  `<slugified-name>-cma` slugs and the cleaned StatsCan name.
- `api/seed/postal_codes_ca.csv` — 1,643 FSAs anchored at city leaf
  (10), CMA via prefix table (522), or province (1,111).
- `etl/SOURCES.md` populated with CA source vintage + sha256s.
- `LICENSE-DATA` Statistics Canada Open Licence section with the
  required acknowledgement text.

**Sourcing pivot (recorded for the historical record):** the
original plan was to filter the PCCF (92-154-X) for FSA → CSD/CMA/
province columns. The PCCF Open Licence variant turned out to be
gated by a registration agreement we didn't want to bake into the
contributor workflow. We pivoted to the publicly-licensed boundary
files (lfsa000b21a_e + lcma000b21a_e) which give us FSA → province
directly but not FSA → CMA. A coarse hand-coded prefix table in
`api/internal/etl/ca/mappings.go` (M, L1-L6, L8-L9, H, V5-V7,
K1-K2, J8-J9, T2-T3, T5-T6) covers the major metros (Toronto,
Hamilton, Montréal, Vancouver, Ottawa-Gatineau, Calgary, Edmonton).
FSAs outside those prefixes fall through to province. A future
slice can replace the prefix table with per-FSA spatial join data
when access improves.

**Verification (as observed):**

- Lookup of varied CA FSAs returns the expected anchor (M5V →
  toronto, L4T → toronto-cma, L8P → hamilton-cma, K1A →
  ottawa-gatineau-cma, A0A → nl-province).
- `urbanist-atlas-server etl regenerate --country=CA` is reproducible.
- All 13 provinces + territories load via `regions_ca_provinces.toml`
  and surface in `/api/v1/metros` only if their kind is metro-equivalent
  (it isn't — `ca:province` is intentionally out of the metro list).

---

## Open questions

1. **Browse-list growth policy.** After #7.5.3 + #7.5.4, the
   `IsMetroKind` predicate returns true for 393 US MSAs + 41 CA
   CMAs + 1 metro-vancouver + 4 PT área-metropolitanas = ~440 metros
   on `/api/v1/metros` and the homepage Browse panel. The current
   UI was designed for ~10. **Decision deferred to a follow-up
   slice** (#7.5.5 or a separate Browse-policy slice). Three paths:
   (a) filter Browse to top-50-by-org-count (cheapest, hides
   metros with no orgs); (b) paginate; (c) tier the list with a
   "show all" toggle. The default API behavior (return all metros)
   stays correct; the UI policy is what changes.
2. **Source vintages (locked).** Census 2020 ZCTAs + Census 2023
   CBSA delineation + StatsCan 2021 census boundary files. All
   sha256-pinned in `etl/SOURCES.md`. Vintage upgrades are
   deliberate future slices.
3. **Override file naming (locked).** US overrides live in
   `api/seed/regions_us_msa_overrides.toml` (one file, applies to
   entries in `regions_us_msas.toml`). CA overrides live in
   `api/internal/etl/ca/mappings.go` (the small fixed CMA set
   didn't warrant a separate TOML file). The dichotomy is
   intentional: US has 393 MSAs with editorial open-endedness;
   CA has 41 CMAs and the override set is bounded.
4. **PCCF for finer CA granularity.** The current FSA-prefix
   mapping is approximate (e.g., L7 straddles Toronto and Hamilton
   CMAs but maps to province). A future slice could replace it
   with per-FSA spatial join data via either (a) shelling out to
   a Python+GeoPandas script during ETL or (b) negotiating PCCF
   Open Licence access. Tracked but not blocking Phase 1.
5. **Phase 2 / API-keys impact**: none. The smallest-anchor model
   doesn't change the wire contract; lookup responses look the
   same to clients. Anchors that resolve to MSA or state regions
   produce shorter `resolved_ancestry` arrays than city-anchored
   lookups, but that's allowed by the existing schema.

---

## References

- [Region graph design (slice #4.5)](./2026-05-16-region-graph-design.md) —
  the underlying DAG model + lookup algorithm
- [Region-graph PT validation (slice #4.6)](./2026-05-17-region-graph-pt-validation-design.md) —
  multi-parent DAG validation + national-tier introduction
- [Region-graph reference](../region-graph.md) — user-facing curator docs
- [Census CBSA delineation files](https://www.census.gov/geographies/reference-files.html)
- [Census ZCTA relationship files](https://www.census.gov/geographies/reference-files/time-series/geo/relationship-files.html)
- [Statistics Canada PCCF](https://www150.statcan.gc.ca/n1/en/catalogue/92-154-X)
- [Statistics Canada CMA reference](https://www12.statcan.gc.ca/census-recensement/2021/geo/sip-pis/cma-rmr/index2021-eng.cfm)
