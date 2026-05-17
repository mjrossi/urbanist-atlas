# Seed data

Hand-curated data for development and Phase 1 dogfooding. Two file
formats:

| File | Format | Cardinality | Source |
|---|---|---|---|
| `regions_<cc>.toml` | TOML | ~10–100 rows/country | Hand-curated editorial. |
| `postal_codes_<cc>.csv` | CSV (3 columns) | 10k–50k rows/country at scale | Reshaped from Census/StatsCan/Royal Mail/etc. by an out-of-band ETL step. |
| `orgs.toml` | TOML | ~20 rows initially | Hand-curated editorial; grows via the submission queue post-Phase-1. |

See the canonical design at
`docs/superpowers/specs/2026-05-16-region-graph-design.md` and the
user-facing reference at `docs/region-graph.md`.

## Load order

The schema enforces ordering via foreign keys:

```sh
just loaddata    # runs loadregions × 2, loadpostal × 2, seed
```

Or step-by-step:

```sh
just loadregions seed/regions_us.toml US
just loadpostal  seed/postal_codes_us.csv US
just loadregions seed/regions_ca.toml CA
just loadpostal  seed/postal_codes_ca.csv CA
just loadregions seed/regions_pt.toml PT
just loadpostal  seed/postal_codes_pt.csv PT
just seed
```

A different order will fail loudly (FK errors or "slug not found" hints).

## CSV schema (postal_codes_*.csv)

```csv
postal_code,country,leaf_region_slug
11217,US,brooklyn
```

- `postal_code`: per-country format (5-digit US ZIP, 3-char CA FSA, 5-digit DE/FR/MX, outward UK code, 4-digit AU, 7-digit PT). Whitespace trimmed; CA truncated to FSA; UK to outward; PT stripped of the hyphen (so `1100-001` and `1100001` resolve identically); everything uppercased.
- `country`: redundant with `--country` but kept so cross-country rows are caught at parse time.
- `leaf_region_slug`: must exist in `regions` already (run `loadregions` first).

## TOML schema (regions_*.toml, orgs.toml)

See the worked examples in `docs/region-graph.md` and the bundled
fixtures here. Modeling conventions:

- State edges live on the **leaf** (city/borough), not on the metro.
- Multi-state / federation regions parent the metro or the leaf, **not** the state.
- `scope_tier` is editorial. Berlin is `de:land` but `scope_tier='local'` because Berliners experience it as a city.
- `scope_tier='national'` exists for country-wide umbrella orgs (MUBi national for PT, future Living Streets for UK). National regions get no incoming parent edges from the leaf chain, and the default `/lookup` filters them out of the ancestor walk. See [`docs/region-graph.md`](../../docs/region-graph.md) for the per-country editorial policy — US/CA do NOT create `us:national`/`ca:national` regions in v1, preserving the local-first ethos.

## Validation fixtures

`regions_pt.toml` + `postal_codes_pt.csv` (plus the PT entries in
`orgs.toml`) are a deliberate validation fixture for the multi-parent
DAG model, not a complete Portuguese directory. See
[`docs/superpowers/specs/2026-05-17-region-graph-pt-validation-design.md`](../../docs/superpowers/specs/2026-05-17-region-graph-pt-validation-design.md)
for what each region in the PT set is meant to prove about the model.
The primary shipping decision remains US + CA.

## Real-world data sources

`postal_codes_*.csv` in this directory are **bundled fixtures** (curated
ZIP coverage of the worked-example cities). Full-country imports use:

| Country | Source | URL |
|---|---|---|
| US | Census ZCTA crosswalk | https://www.census.gov/geographies/reference-files.html |
| CA | StatsCan Postal Code Conversion File (PCCF) | https://www150.statcan.gc.ca/n1/en/catalogue/92-154-X |
| PT | CTT via OpenPLZ (ODbL-licensed; OSM-derived) | https://www.openplzapi.org/en/ |
| DE | Various open sources (e.g. OpenGeoDB, Geonames) | https://download.geonames.org/export/zip/ |
| UK | ONS Postcode Directory | https://geoportal.statistics.gov.uk/ |

Each requires an out-of-band ETL pass (script, notebook) to reshape
into the 3-column format above before `loadpostal` is run. Sha256
checksums of the upstream files are tracked in this README when added.
