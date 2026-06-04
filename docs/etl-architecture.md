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
| `regions_us_msas.toml` (393 MSAs + 94 cross-state portions) | | ✅ |
| `regions_ca_cmas.toml` (41 CMAs + 2 portions) | | ✅ |
| `postal_codes_us.csv` (~39.5k ZIPs) | | ✅ |
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
upstream files       ETL pipeline                 seed/ (committed)
─────────────        ────────────                 ────────────────
etl/sources/us/  →  etl regenerate --country=US  →  postal_codes_us.csv
etl/sources/ca/  →  etl regenerate --country=CA  →  postal_codes_ca.csv
                                                    regions_us_msas.toml
                                                    regions_ca_cmas.toml
```

The generated seed files are the end of the line: the API bakes
`api/seed/` into its image via `//go:embed` and loads it into the
in-memory store at boot, so there is no separate load step or dev
database to refresh — restart the server and the new bundle is live.

Two `etl` subcommands on the `urbanist-atlas-server` binary:

- **`etl download --country=<cc>`** — fetches the upstream files,
  verifies sha256, stages them under `etl/sources/<cc>/`. Run only
  when bumping vintages; the directory is gitignored so each
  contributor downloads their own copy.
- **`etl regenerate --country=<cc>`** — runs the per-country
  pipeline; writes the deterministic outputs under `api/seed/`.
  The committed seed files are the canonical "what shipped"
  artifact.

Operator loop when bumping a vintage:

```sh
mise install
pip install -r etl/scripts/requirements.txt   # one-time, US xlsx step

urbanist-atlas-server etl download --country=US
urbanist-atlas-server etl regenerate --country=US

git diff api/seed/postal_codes_us.csv         # review the delta
just api-run                                  # restart; no DB to reload
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
parser bugs and upstream-format drift before the bad output is
committed to the seed bundle.

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

The in-memory ancestor walk (`MemStore.AncestorRegions`, in
[`api/pkg/atlas/memstore.go`](../api/pkg/atlas/memstore.go)) walks
the region DAG upward from whatever the ZIP points at, so a ZIP
anchored at a borough surfaces NYC, the metro, the state, and any
multi-state federations without app-level fallback logic.

### Cross-state metros (stateless umbrella + portions)

A CBSA that spans multiple states can't parent under all of them — the
ancestor walk would carry one state's ZIP up into its neighbours
([`region-graph.md`](./region-graph.md) §1). So when a CBSA's
constituent counties span ≥2 states, the generator emits the metro as a
**stateless umbrella** (`parents = []`) plus one **`us:metro-portion`**
leaf per spanned state (`parents = [state, umbrella]`), and routes each
county to its own state's portion. The umbrella also carries a
directional **`rollup_states`** list (the spanned states) so the metro's
own orgs still surface on each state's `/region/{state}` page in the
browse direction, without the ancestor walk ever crossing the line. The
CA pipeline applies the identical shape to the lone cross-province CMA
(Ottawa-Gatineau → `ca:cma-portion`).

The curated flagships (`nyc-metro`, `chicago-metro`, `greater-boston`)
generate portions exactly like every other multi-state metro — there is
no flagship special-case (GitHub #79). Their override only supplies the
curated slug/name and, for the two advocacy-coalition flagships, an
umbrella parent (`nyc-tristate` / `chicagoland`) in place of the empty
one. The hand-curated borough/county leaves still win as the smaller
anchor (`county_resolver` tiers 1–2), so only ZIPs lacking a curated
leaf ride the portions and reach their own state — closing the former
bare-umbrella residual noted in `region-graph.md` §3.

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

The pipeline reads two things from each StatsCan zip: the **DBF
attribute table** (a minimal stdlib-only reader at
`api/internal/etl/ca/dbf.go` — CMA/FSA codes, names, province IDs)
and, for the FSA → CMA assignment, the **`.shp` polygon geometry**
(via `spatial.go`). It emits `regions_ca_cmas.toml` (41 CMAs + 2
Ottawa-Gatineau portions) + `postal_codes_ca.csv` (1,643 FSAs).

The CMA region rows come straight from the CMA DBF (`assignCMAs`):
one row per type-`B` CMAUID, slug/name/kind from
`regions_ca_cma_overrides.toml`, parents from the constituent
provinces. The smallest-anchor priority for each FSA is then
**curated city leaf → max-overlap CMA → province** (`crosswalk.go`).

### FSA → CMA: max-overlap spatial join

StatsCan's per-postal-code mapping file (PCCF) is licence-restricted,
so we never use it. Instead each FSA is anchored to the CMA it
**overlaps most by area**, computed directly from the boundary
polygons: the FSA `.shp` is clipped against each CMA `.shp`
(`polyclip` intersection) and the FSA goes to the CMA with the
largest intersection (`SpatialJoinFSAToCMA` in
[`spatial.go`](../api/internal/etl/ca/spatial.go), GitHub #81).

This replaced an earlier coarse FSA-prefix → CMA hand-table that
covered only the seven biggest metros and couldn't separate adjacent
CMAs sharing a prefix (Victoria and Nanaimo are both `V9`). All ~41
CMAs now resolve; ~768 FSAs re-anchor province → CMA versus the old
table, while `regions_ca_cmas.toml` is unchanged — only the postal
routing improved.

Three numerical details the join has to get right (each pinned by a
unit test in `spatial_test.go`, and detailed in the design spec's
[Open Question §4](./superpowers/specs/2026-05-19-postal-coverage-design.md)):

- **Even-odd hole handling.** `polyclip`'s boolean output gives
  result holes no guaranteed winding, so the area of an intersection
  is measured by contour *nesting* (a contour inside an odd number of
  others is a hole, subtracted), not by signed-area sign.
- **Large-coordinate precision.** EPSG:3347 coordinates sit ~9e6 m
  from the false origin, where the sweep-line / shoelace products
  (~1e13–1e14) lose precision and a genuine overlap can compute as
  ~0. Both polygons are **translated to a local origin before
  intersecting** — mandatory; preserve this on any vintage bump or
  reuse of `spatial.go`. Area is translation-invariant, so only the
  numerics change.
- **Noise floor.** Separately-digitized FSA and CMA boundaries leave
  sub-square-metre slivers along shared edges. An FSA anchors to a
  CMA only when its max overlap clears `minOverlapFraction` (0.1 %)
  of the FSA's own area; below that it falls through to province.
  0.1 % is calibrated to the empty gap between the ≤1e-4 % noise
  cluster and the ≥0.1 % genuine-overlap cluster in the 2021 vintage.

Ties break to the smallest CMA UID deterministically (CMAs are
UID-sorted and only a strictly larger overlap displaces the
incumbent). The cross-province Ottawa-Gatineau CMA dissolves its
per-province `.shp` records by UID, and its FSAs route to the
`ca:cma-portion` for the FSA's own **PRUID** — the province split is
attribute-driven, not geometry-driven. FSAs with no curated leaf and
no real CMA overlap fall back to their province, so every Canadian
FSA still resolves to a curated region with no licence contamination.

The join is **operator-side only** — it runs inside `etl regenerate`,
never in the server — and reads the 296 MB FSA `.shp` once per postal
regenerate (~25 s); a bounding-box pre-filter keeps the polygon-clip
count small. Pure-Go dependencies, no cgo:
[`github.com/jonas-p/go-shp`](https://github.com/jonas-p/go-shp)
(read `.shp` geometry from inside the zip) +
[`github.com/ctessum/polyclip-go`](https://github.com/ctessum/polyclip-go)
(polygon-intersection area).

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
   in-memory `MemStore.AncestorRegions` walk surfaces the ancestors
   upward from whatever the row points at, so app-level fallback
   logic is not needed.
6. **Register the outputs.** Add the new country's generated files to
   the `Targets` list in `api/internal/etl/<cc>/<cc>.go` so
   `etl regenerate` writes them under `api/seed/`, and add the country
   to the `countries` list in `api/internal/seedfiles/build.go` so the
   boot-time loader picks up its region + postal files. There is no
   load step to wire — the bundle is read into memory at boot.
7. **Editorial policy.** Append a row to
   [`docs/region-graph.md`](./region-graph.md) §5 documenting the
   per-country `scope_tier` rules (does this country have a
   `national` tier? US/CA deliberately don't; PT/UK/NL do).
8. **Tests.** Add a regression for at least one anchor per tier
   (city-leaf, intermediate, metro, state) in the per-country ETL
   test file; add a `seedfiles` loader test that builds a MemStore
   from the generated seed (via `BuildMemStore`) and runs a sample
   `atlas.Lookup` to confirm the anchors resolve end-to-end.

The CA pipeline (`api/internal/etl/ca/`) is a small worked example
to copy from. The US pipeline is a more complex one — read it
when your country has two complementary sources to merge.

See also: [`docs/region-graph.md`](./region-graph.md) for the
region-DAG modeling rules every country has to obey, and
[`api/seed/README.md`](../api/seed/README.md) for the seed file
formats and load order.
