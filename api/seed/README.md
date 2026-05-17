# Seed data

This directory holds two kinds of input the `urbanist-atlas-server`
binary consumes at startup:

1. **Postal-code crosswalks** (`test_postal_us.csv`, `test_postal_ca.csv`)
   loaded by `urbanist-atlas-server loadpostal --src <path> --country US|CA`
   into the `regions` and `postal_codes` tables.
2. **Curated organizations** (`orgs.yaml`) loaded by
   `urbanist-atlas-server seed` into `organizations` and
   `organization_regions`.

Both loaders are idempotent; re-running produces no diff.

## CSV format (loadpostal)

A single uniform schema across countries. Ten columns, header
required:

```
postal_code,country,city_name,city_slug,county_name,county_slug,metro_name,metro_slug,state_name,state_slug
```

- `postal_code` — 5-digit US ZIP or 3-character Canadian FSA. Whitespace
  and case are normalized inside the loader (so `m5v 3a8` becomes
  `M5V`); supply data however is convenient.
- `country` — `US` or `CA`. Must match the `--country` flag for the run.
- `city_*` / `county_*` / `metro_*` / `state_*` — name and URL-safe slug
  for each region tier. Slug is the `ON CONFLICT` target on the regions
  table, so it must be unique across the dataset and stable across
  reruns.
- Leave any non-applicable tier blank (Canadian rows typically have no
  county; some US rows have no metro). The `state_*` columns are
  required for every row — they hold the province for CA rows.

Real-world Census / StatsCan source files do **not** ship in this
shape. The maintainer's expectation is that an out-of-band ETL step (a
small script, an LLM-assisted transform, a notebook) reshapes the
canonical sources into this schema before `loadpostal` runs. That
script is intentionally not part of the binary — the binary should not
fetch data over the network at startup.

### Bundled fixtures

The two files committed here are small, hand-edited samples designed
to exercise the loader end-to-end without network access. They cover
the postal codes referenced by `orgs.yaml`:

| File                   | Rows | Regions covered                                              | sha256 |
|------------------------|------|--------------------------------------------------------------|--------|
| `test_postal_us.csv`   | 11   | NYC, Boston, SF, LA, Miami, Seattle                          | `e9aca095d4c8200d702601b29bdbcaa6940ee6e3b8f62ec2dea01b73ca301038` |
| `test_postal_ca.csv`   | 5    | Toronto (3 FSAs), Vancouver, Montréal                        | `65980fb986db47e2ef340dd05376b66ec48769240f8fbb3494d6058d395483e4` |
| `orgs.yaml`            | 16   | 13 US orgs + 3 Canadian orgs                                 | `4c04482b236e4af6f20b887d6b605eb07baf2773fe85ab3ced6e91a904d03494` |

The justfile recipe `just loadpostal api/seed/test_postal_us.csv US`
runs cleanly on a fresh checkout (with `pg-up` + `migrate-up`).

### Canonical upstream sources

When the project is ready to ingest real, comprehensive crosswalks, the
canonical free / public sources are:

- **US:** U.S. Census Bureau ZCTA → County / CBSA crosswalk, published
  annually. See
  <https://www.census.gov/geographies/reference-files/time-series/geo/relationship-files.html>
  (ZCTA-County and ZCTA-CBSA relationship files). License: public
  domain. Reshape into the schema above before `loadpostal`.
- **CA:** Statistics Canada Forward Sortation Area (FSA) → Census
  Metropolitan Area (CMA) lookup. See
  <https://www.statcan.gc.ca/en/lode/databases/fsa>. License:
  Statistics Canada Open Licence Agreement. Reshape into the schema
  above before `loadpostal`.

Document the sha256 of the source file you used in your local notes if
you build a private full-dataset ingest; the bundled fixtures here
remain the only files the binary needs to operate.

## YAML format (seed)

```yaml
orgs:
  - slug: transportation-alternatives        # unique, URL-safe
    name: Transportation Alternatives
    short_desc: One-sentence description.    # required
    website_url: https://transalt.org        # required
    contact_url: https://transalt.org/contact # optional
    tags: [advocacy, safe-streets]           # free-form labels
    regions:
      - country: US
        postal_codes: [11217, 10001]         # resolved to region IDs
```

The loader resolves each `postal_codes` entry through the
`postal_codes` table (which `loadpostal` must have populated). An
unknown postal code is a hard error — the loader refuses to write an
org no lookup could ever find. Every region the postal code falls
within (city, county, metro, state) becomes an `organization_regions`
row, so a single ZIP entry typically wires up four region links.

`status` is always set to `approved` by the seed loader; pending /
rejected / archived states are reserved for the submission queue
(slice #5).
