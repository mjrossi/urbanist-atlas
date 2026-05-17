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
just seed
```

A different order will fail loudly (FK errors or "slug not found" hints).

## CSV schema (postal_codes_*.csv)

```csv
postal_code,country,leaf_region_slug
11217,US,brooklyn
```

- `postal_code`: per-country format (5-digit US ZIP, 3-char CA FSA, 5-digit DE/FR/MX, outward UK code, 4-digit AU). Whitespace trimmed; CA truncated to FSA; UK to outward; everything uppercased.
- `country`: redundant with `--country` but kept so cross-country rows are caught at parse time.
- `leaf_region_slug`: must exist in `regions` already (run `loadregions` first).

## TOML schema (regions_*.toml, orgs.toml)

See the worked examples in `docs/region-graph.md` and the bundled
fixtures here. Modeling conventions:

- State edges live on the **leaf** (city/borough), not on the metro.
- Multi-state / federation regions parent the metro or the leaf, **not** the state.
- `scope_tier` is editorial. Berlin is `de:land` but `scope_tier='local'` because Berliners experience it as a city.

## Real-world data sources

`postal_codes_*.csv` in this directory are **bundled fixtures** (curated
ZIP coverage of the worked-example cities). Full-country imports use:

| Country | Source | URL |
|---|---|---|
| US | Census ZCTA crosswalk | https://www.census.gov/geographies/reference-files.html |
| CA | StatsCan Postal Code Conversion File (PCCF) | https://www150.statcan.gc.ca/n1/en/catalogue/92-154-X |
| DE | Various open sources (e.g. OpenGeoDB, Geonames) | https://download.geonames.org/export/zip/ |
| UK | ONS Postcode Directory | https://geoportal.statistics.gov.uk/ |

Each requires an out-of-band ETL pass (script, notebook) to reshape
into the 3-column format above before `loadpostal` is run. Sha256
checksums of the upstream files are tracked in this README when added.
