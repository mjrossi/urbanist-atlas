# ETL source data — vintage manifest

Pinned upstream sources for the `urbanist-atlas-server etl` pipeline.
Each source is identified by canonical URL, vintage label, and a
sha256 checksum the `download` step verifies. Re-downloading must
produce a file whose sha256 matches the value below, or `download`
fails loudly — the defense against silent upstream changes between
vintages.

The corresponding `etl/sources/` directory is gitignored (see
`.gitignore`); commit only the regenerated outputs under
`api/seed/`.

Design at
[`docs/superpowers/specs/2026-05-19-postal-coverage-design.md`](../docs/superpowers/specs/2026-05-19-postal-coverage-design.md).

## US

Concrete plan lands in slice #7.5.3. The expected sources are:

| File                              | Vintage     | URL                                                                                                                          | sha256          |
| --------------------------------- | ----------- | ---------------------------------------------------------------------------------------------------------------------------- | --------------- |
| `cbsa-est2023-delineation.csv`    | 2023 update | https://www.census.gov/programs-surveys/metro-micro/about/delineation-files.html                                             | _(TBD #7.5.3)_  |
| `zcta-place-relationship.txt`     | 2020 census | https://www.census.gov/geographies/reference-files/time-series/geo/relationship-files.html                                   | _(TBD #7.5.3)_  |
| `zcta-county-relationship.txt`    | 2020 census | https://www.census.gov/geographies/reference-files/time-series/geo/relationship-files.html                                   | _(TBD #7.5.3)_  |

Census files are public domain — no attribution required by Census,
but `LICENSE-DATA` will credit them anyway for transparency.

## CA

Concrete plan lands in slice #7.5.4. The expected sources are:

| File                              | Vintage     | URL                                                                                                                          | sha256          |
| --------------------------------- | ----------- | ---------------------------------------------------------------------------------------------------------------------------- | --------------- |
| `pccf.zip` (filtered at parse)    | 2025-Q1     | https://www150.statcan.gc.ca/n1/en/catalogue/92-154-X                                                                        | _(TBD #7.5.4)_  |
| `cma-reference.csv`               | 2021 census | https://www12.statcan.gc.ca/census-recensement/2021/geo/sip-pis/cma-rmr/index2021-eng.cfm                                    | _(TBD #7.5.4)_  |

Statistics Canada Open License requires attribution — `LICENSE-DATA`
will carry the required text once #7.5.4 ships.

## Vintage upgrade workflow

Bumping a source's vintage is a deliberate slice:

1. Update the vintage label + sha256 in this file.
2. Re-download via `urbanist-atlas-server etl download --country=...`.
3. Re-generate via `urbanist-atlas-server etl regenerate --country=...`.
4. Inspect the diff under `api/seed/` — large unexplained churn means
   the upstream changed semantics, not just vintage.
5. Commit `etl/SOURCES.md` + `api/seed/*` in one atomic commit.

Census ZCTA boundaries change every decennial census (next is 2030).
StatsCan PCCF updates quarterly; vintage upgrades on a slower cadence
are fine for our use case.
