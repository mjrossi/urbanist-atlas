# ETL architecture

How `urbanist-atlas-server etl` turns upstream geographic reference
files into the postal-code and MSA/CMA seed data the API ships.

Read this when adding a country, bumping a source vintage, or
changing the smallest-anchor rules. The point-in-time design
rationale lives in
[`docs/superpowers/specs/2026-05-19-postal-coverage-design.md`](./superpowers/specs/2026-05-19-postal-coverage-design.md);
this doc is the evergreen "how the system is laid out today" view.

## What gets generated, what gets hand-curated

The atlas's seed data splits cleanly:

| File | Hand-curated | Generated |
|---|---|---|
| `regions_<cc>_states.toml` / `_provinces.toml` | ✅ | |
| `regions_<cc>_multistate.toml` | ✅ | |
| `regions_<cc>.toml` (city / borough / county leaves) | ✅ | |
| `orgs.toml` | ✅ | |
| `regions_us_msas.toml` (393 MSAs) | | ✅ |
| `regions_ca_cmas.toml` (41 CMAs) | | ✅ |
| `postal_codes_us.csv` (~38k ZIPs) | | ✅ |
| `postal_codes_ca.csv` (~1.6k FSAs) | | ✅ |

Hand-curated files express editorial decisions (which cities,
boroughs, counties get their own region; which orgs belong in v1).
Generated files express mechanical crosswalks (ZCTA → place,
FSA → CMA, ZIP → smallest anchor) that would be tedious and
error-prone to maintain by hand.

The generator is deterministic: same upstream vintage in →
byte-identical seed output. Git diffs are signal-rich; a row
change means an upstream change or a real algorithm change, not
non-determinism.

## Top-level flow

```
upstream files       ETL pipeline                seed/                dev DB
─────────────        ────────────                ─────                ──────
etl/sources/us/  →  etl regenerate --country=US  →  postal_codes_us.csv  →  just loaddata
etl/sources/ca/  →  etl regenerate --country=CA  →  postal_codes_ca.csv  →
                                                   regions_us_msas.toml
                                                   regions_ca_cmas.toml
```

Three subcommands on the `urbanist-atlas-server` binary:

- **`etl download --country=<cc>`** — fetches the upstream files,
  verifies sha256, stages them under `etl/sources/<cc>/`. Run only
  when bumping vintages; the directory is gitignored so each
  contributor downloads their own copy.
- **`etl regenerate --country=<cc>`** — runs the per-country
  pipeline; writes the deterministic outputs under `api/seed/`.
  The committed seed files are the canonical "what shipped"
  artifact.
- **`loaddata`** (not under `etl`) — reloads the dev DB from the
  committed seed files. Idempotent.

Operator loop when bumping a vintage:

```sh
mise install
pip install -r etl/scripts/requirements.txt   # one-time, US xlsx step

urbanist-atlas-server etl download --country=US
urbanist-atlas-server etl regenerate --country=US

git diff api/seed/postal_codes_us.csv         # review the delta
just pg-reset && just pg-up && just loaddata  # reload dev DB
```

## Source pinning + determinism

Each country's plan lives in `api/internal/etl/<cc>/<cc>.go`. The
plan registers itself with `etl.Plans["<CC>"]` via `init()` and
declares its upstream sources:

```go
etl.Plans["US"] = etl.Country{
    Sources: []etl.SourceDescriptor{
        {Filename: "list1_2023.xlsx", URL: "...", SHA256: "...", Vintage: "..."},
        ...
    },
    Targets: []etl.OutputTarget{
        {Path: "regions_us_msas.toml", Format: "toml", MinRows: 380, MaxRows: 400},
        {Path: "postal_codes_us.csv",  Format: "csv",  MinRows: 30000, MaxRows: 45000},
    },
    Regenerate: Regenerate,
}
```

Two sources of truth for source pinning, kept in lockstep:

- **`api/internal/etl/<cc>/<cc>.go`** — what the code actually
  verifies. The `SourceDescriptor.SHA256` field is checked by
  `etl download`; empty string skips the check (used for HUD,
  where the upstream is account-gated and the operator pins the
  hash on first download).
- **`etl/SOURCES.md`** — the human-readable manifest with URLs,
  vintage labels, license notes, and the manual refresh recipe.

If you bump a vintage, update both files in the same commit.

The `Targets` row bands are sanity rails: `etl regenerate` fails
loudly if the output row count falls outside the band, catching
parser bugs and upstream-format drift before they reach `loaddata`.

## US pipeline

Two complementary sources merged into a single output:

### Census (primary, ~33.7k ZCTAs)

- `list1_2023.xlsx` — CBSA delineation. Lists every Metropolitan
  Statistical Area + the counties they contain. Source for
  `regions_us_msas.toml`. The xlsx → csv conversion uses
  `etl/scripts/xlsx_to_csv.py` (Python + openpyxl); Census
  publishes no CSV variant.
- `tab20_zcta520_place20_natl.txt` — ZCTA-to-place crosswalk.
  Maps each 5-digit ZCTA to its primary Census-designated place.
- `tab20_zcta520_county20_natl.txt` — ZCTA-to-county crosswalk.
  Maps each ZCTA to its primary county.

The `Crosswalk` function walks each ZCTA through a smallest-anchor
rule:

```
ZIP 11217 → place "Brooklyn" → curated leaf "brooklyn"          (city/borough)
ZIP 60601 → no curated place → county "Cook"   → MSA "chicago-metro"
ZIP 99950 → no place, no county-MSA match     → state "ak"      (final fallback)
```

The recursive CTE in
[`api/internal/store/postgres/queries/lookup.sql`](../api/internal/store/postgres/queries/lookup.sql)
walks the region DAG upward from whatever the ZIP points at, so a
ZIP anchored at a borough surfaces NYC, the metro, the state, and
any multi-state federations without app-level fallback logic.

### HUD (additive backfill, ~5–10k operational ZIPs)

Census ZCTA covers polygons with residential or addressable-
business footprint. Operational ZIPs without one — P.O. Box-only
ZIPs (e.g. 20811 covering NIH / Walter Reed), single-building ZIPs
(corporate / federal facilities), APO/FPO military ZIPs — have no
ZCTA. HUD's quarterly USPS ZIP-County crosswalk covers them.

`CrosswalkHUDBackfill`
([`api/internal/etl/us/hud.go`](../api/internal/etl/us/hud.go))
emits anchor rows only for ZIPs absent from the ZCTA result set.
Per ZIP, it picks the row with the largest `TOT_RATIO` (correct
for P.O. Box-only ZIPs where `RES_RATIO=0`) and walks the same
county-FIPS → NYC-borough → county-leaf → MSA → state chain.

The writer merges ZCTA + HUD anchor slices with ZCTA winning any
tie. Result: every ZIP resolves to its smallest curated region,
across both data sources, with no app-level fallback logic.

### HUD download — operator-gated

HUD requires a HUDUser account, so the URL in `SourceDescriptor`
points at the portal landing page rather than the file. On first
download the operator:

1. Saves the file as
   `etl/sources/us/hud_zip_county_<vintage>.csv`.
2. Computes its sha256 and pins it in both `etl/SOURCES.md` and
   `api/internal/etl/us/us.go`.
3. Runs `etl regenerate --country=US` to materialize the ~5–10k
   net-new rows in `api/seed/postal_codes_us.csv`.

The orchestrator degrades gracefully when the HUD CSV is absent
(`findHUDFile` returns ""): the pipeline writes a ZCTA-only seed
and logs "no HUD ZIP-County CSV found in src dir — skipping
non-ZCTA backfill" so the contributor isn't blocked on the
account-gated download.

## CA pipeline

Two source files, neither of which require an account:

- `lfsa000b21a_e.zip` — StatsCan FSA boundary file (2021 census).
  The DBF attribute table inside maps each 3-character Forward
  Sortation Area to its province code.
- `lcma000b21a_e.zip` — StatsCan CMA boundary file (2021 census).
  Lists every Census Metropolitan Area (population ≥100k, type B)
  with its UID, name, and constituent provinces.

The pipeline reads the DBF attribute tables (a minimal stdlib-only
DBF reader lives at `api/internal/etl/ca/dbf.go`) and emits
`regions_ca_cmas.toml` (41 CMAs) + `postal_codes_ca.csv`
(1,643 FSAs).

**FSA → CMA mapping caveat.** StatsCan's per-postal-code mapping
file (PCCF) is licence-restricted. Slice #7.5.4 sidesteps it with
a coarse FSA-prefix → CMA hand-mapping table in
`api/internal/etl/ca/mappings.go`:

```
M, L1/3/4/5/6           → Toronto
H                       → Montréal
V5–7                    → Vancouver
K1–2, J8–9              → Ottawa-Gatineau
T2–3                    → Calgary
T5–6                    → Edmonton
L8–9                    → Hamilton
```

Non-CMA FSAs fall back to their province. The result: every
Canadian FSA resolves to a curated region without licence
contamination. A future slice can replace the prefix table with a
per-FSA mapping when PCCF or open spatial-join data becomes
available.

## Adding a country

The smallest-anchor model generalizes; the per-country work is
mostly source selection + parser. Checklist:

1. **Pick sources.** A postal-code → smallest-region crosswalk
   from a publicly-licensed source. National statistical agencies
   are the usual first stop (Census US, StatsCan CA, ONS UK,
   Destatis DE, INEGI MX). Cross-check the licence against ODbL
   compatibility — see `LICENSE-DATA`.
2. **Pin sources.** Add the entries to `etl/SOURCES.md` (URL,
   vintage, sha256, licence note) and to a new
   `api/internal/etl/<cc>/<cc>.go` with `etl.Plans["<CC>"] = ...`.
3. **Write the hand-curated tiers.**
   - `api/seed/regions_<cc>_states.toml` (or province equivalent) —
     the top tier of the per-country hierarchy.
   - `api/seed/regions_<cc>_multistate.toml` if multi-state
     advocacy regions or transit federations apply.
   - `api/seed/regions_<cc>.toml` for editorial city / borough /
     county leaves you want by name.
4. **Implement the parser.** New `api/internal/etl/<cc>/` package.
   Convention: one file per source, plus an `output.go` that
   writes the deterministic TOML/CSV, plus a `<cc>.go` with the
   `init()` registration and the `Regenerate` orchestrator. Use
   stdlib parsers when possible (the CA pipeline ships a stdlib-
   only DBF reader; the US pipeline uses `encoding/csv`).
5. **Implement the crosswalk.** Walk each postal code through:
   curated city leaf (if any) → curated intermediate (borough,
   district) → metro/CMA equivalent → state/province. The
   recursive CTE in the Postgres store walks upward from
   whatever the row points at, so app-level fallback logic is
   not needed.
6. **Wire `loaddata`.** Add the new `loadregions` + `loadpostal`
   calls to whichever script `just loaddata` invokes (currently
   `api/cmd/server/loaddata.go`).
7. **Editorial policy.** Append a row to
   [`docs/region-graph.md`](./region-graph.md) §5 documenting the
   per-country `scope_tier` rules (does this country have a
   `national` tier? US/CA deliberately don't; PT/UK/NL do).
8. **Tests.** Add a regression for at least one anchor per tier
   (city-leaf, intermediate, metro, state) in the per-country ETL
   test file; add an integration test that loads the generated
   seed against testcontainers Postgres and runs a sample
   `/lookup`.

The CA pipeline (`api/internal/etl/ca/`) is a small worked example
to copy from. The US pipeline is a more complex one — read it
when your country has two complementary sources to merge.

See also: [`docs/region-graph.md`](./region-graph.md) for the
region-DAG modeling rules every country has to obey, and
[`api/seed/README.md`](../api/seed/README.md) for the seed file
formats and load order.
