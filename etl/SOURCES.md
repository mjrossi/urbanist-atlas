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

The US ETL pulls from **two sources** that play complementary roles in
the `postal_codes_us.csv` pipeline:

- **Primary — Census** (slice #7.5.3). Three Census Bureau reference
  files defining MSAs (`list1_2023`) and mapping ZCTA polygons to
  places + counties (`tab20_zcta520_*`). Resolves the ~33,700 ZIPs
  that exist as ZCTAs (i.e., have a residential / addressable-business
  geographic footprint).
- **Backfill — HUD** (slice #7.5.5). HUD's quarterly USPS ZIP-County
  crosswalk. Resolves the ~9k operational USPS ZIPs that Census ZCTA
  omits: P.O. Box-only ZIPs (no addressable buildings — e.g., 20811
  covering NIH / Walter Reed), single-building ZIPs (corporate /
  federal facilities USPS gave a dedicated ZIP), and APO/FPO military
  ZIPs. HUD is **additive only** — never overrides a ZCTA-source row;
  see [postal-coverage design §Two-source pipeline](../docs/superpowers/specs/2026-05-19-postal-coverage-design.md#two-source-pipeline-zcta--hud--slice-755).

Files live under `etl/sources/us/` (gitignored).

### Census source files (primary)

| File                                  | Vintage          | URL                                                                                                                            | sha256 |
| ------------------------------------- | ---------------- | ------------------------------------------------------------------------------------------------------------------------------ | ------ |
| `list1_2023.xlsx`                     | Census CBSA, July 2023      | https://www2.census.gov/programs-surveys/metro-micro/geographies/reference-files/2023/delineation-files/list1_2023.xlsx       | `952c4b1e78acbb54e6ec9412434b7602fedacbf021736351a63c181bdb753629` |
| `list1_2023.csv`                      | (derived from xlsx)         | (run `etl/scripts/xlsx_to_csv.py list1_2023.xlsx list1_2023.csv`)                                                              | `6ad49da23ac95fe35f6e038e6ebc54b59b071d1503c1bde249d0a585f199b14a` |
| `tab20_zcta520_place20_natl.txt`      | Census ZCTA-to-place, 2020  | https://www2.census.gov/geo/docs/maps-data/data/rel2020/zcta520/tab20_zcta520_place20_natl.txt                                | `698a5dad71ed419411677d0ffd8ecd9331067f59c472cdd239b92c12f698285d` |
| `tab20_zcta520_county20_natl.txt`     | Census ZCTA-to-county, 2020 | https://www2.census.gov/geo/docs/maps-data/data/rel2020/zcta520/tab20_zcta520_county20_natl.txt                               | `3ed41278d637dc249e0323306f68be8a6c234e3090f4de88ef328dee71aeaaaf` |

### HUD source file (backfill)

| File                                  | Vintage          | URL                                                                                                                            | sha256 |
| ------------------------------------- | ---------------- | ------------------------------------------------------------------------------------------------------------------------------ | ------ |
| `hud_zip_county_2026q1.csv`           | HUD USPS ZIP-to-County, 2026-Q1 | https://www.huduser.gov/portal/dataset/uspszip-api.html (download requires HUD account; pick the most-recent quarterly release) | `TBD — operator fills on first download (see HUD note below)` |

### HUD download — operator note

The HUD USPS ZIP Crosswalk requires a HUDUser account
(<https://www.huduser.gov/portal/dataset/uspszip-api.html>). The
direct download URL is account-scoped, so we can't pin a canonical
public URL the way Census files allow. On the first download:

1. Sign in to HUDUser, navigate to the *USPS ZIP Code Crosswalk
   Files*, pick the most recent **ZIP-to-County** release (quarterly
   cadence), and save the CSV to
   `etl/sources/us/hud_zip_county_YYYYqN.csv` (replace `YYYYqN` with
   the release label — e.g., `hud_zip_county_2026q1.csv`).
2. Compute the sha256 of the saved file
   (`shasum -a 256 etl/sources/us/hud_zip_county_*.csv`) and paste it
   into the table above plus the `SHA256` field in
   `api/internal/etl/us/us.go`. Commit both edits together.
3. Re-run `urbanist-atlas-server etl regenerate --country=US` to
   produce the updated `api/seed/postal_codes_us.csv` with HUD
   backfill rows (~5–10k net-new rows; ZCTA-source rows unchanged).

Subsequent vintage upgrades follow the same recipe — see *Vintage
upgrade workflow* below.

License: HUD USPS Crosswalk files are public-domain US-government
data (HUD is a federal agency; works of the US government are not
subject to copyright under 17 U.S.C. § 105). See `LICENSE-DATA` for
attribution recommendation.

### Manual refresh workflow

The Census Bureau publishes the CBSA delineation file in xlsx format
only — no CSV variant — so the workflow has a Python conversion step.

```sh
# 1a. mise install (one time) provisions Python 3.
# 1b. pip install -r etl/scripts/requirements.txt (one time)
#     pulls openpyxl. It's library-only so it doesn't fit mise's
#     pipx backend; the requirements file pins the version instead.

# 2. Fetch upstream files into etl/sources/us/.
mkdir -p etl/sources/us && cd etl/sources/us
curl -O https://www2.census.gov/programs-surveys/metro-micro/geographies/reference-files/2023/delineation-files/list1_2023.xlsx
curl -O https://www2.census.gov/geo/docs/maps-data/data/rel2020/zcta520/tab20_zcta520_place20_natl.txt
curl -O https://www2.census.gov/geo/docs/maps-data/data/rel2020/zcta520/tab20_zcta520_county20_natl.txt
# HUD ZIP-County CSV: download manually via HUDUser portal (account
# required), save as hud_zip_county_YYYYqN.csv (e.g., hud_zip_county_2026q1.csv).

# 3. Convert the xlsx to CSV.
python3 ../../scripts/xlsx_to_csv.py list1_2023.xlsx list1_2023.csv

# 4. Verify checksums against this file. Mismatch = upstream vintage
#    changed; bump the entries above and re-review the generated
#    seed diff.
shasum -a 256 list1_2023.xlsx list1_2023.csv tab20_*.txt hud_zip_county_*.csv

# 5. Run the ETL.
cd ../../../api
go run ./cmd/server etl regenerate --country=US --src=../etl/sources --out=seed
```

Census reference files are in the public domain (17 U.S.C. § 105).
See `LICENSE-DATA` for attribution recommendation.

## CA

Sources pinned by slice #7.5.4. Files live under `etl/sources/ca/`
(gitignored). The Postal Code Conversion File (PCCF, 92-154-X) is
licensed under restricted terms; we sidestep it by using the publicly
licensed FSA and CMA boundary files instead. FSA → CMA mapping is
done via a coarse prefix table in `api/internal/etl/ca/mappings.go`
rather than per-FSA spatial join (which would require the PCCF or a
shapefile-aware spatial library). A future slice can refine the
mapping if the PCCF terms change or a spatial-join workflow is added.

| File                              | Vintage          | URL                                                                                                                  | sha256 |
| --------------------------------- | ---------------- | -------------------------------------------------------------------------------------------------------------------- | ------ |
| `lfsa000b21a_e.zip`               | StatsCan FSA boundary, 2021 census | https://www12.statcan.gc.ca/census-recensement/2021/geo/sip-pis/boundary-limites/files-fichiers/lfsa000b21a_e.zip   | `9fd2b6adf66e5716d06f91ebdcdb5d8a4e8b9eeb520f8b4285030d34319959db` |
| `lcma000b21a_e.zip`               | StatsCan CMA boundary, 2021 census | https://www12.statcan.gc.ca/census-recensement/2021/geo/sip-pis/boundary-limites/files-fichiers/lcma000b21a_e.zip   | `a12dd39b3262edb48f9490b435d2f43b0327cc4af7d829f32aebae4d4b9f8fa0` |

The ETL parses only the DBF attribute table inside each zip (~150KB
extracted from a ~162MB zip for FSAs); the shapefile geometry is
ignored. Reading from inside the zip avoids polluting the repo with
extracted multi-MB artifacts.

### Manual refresh workflow

```sh
# 1. mise install (one time) provisions Go.

# 2. Fetch upstream files into etl/sources/ca/.
mkdir -p etl/sources/ca && cd etl/sources/ca
curl -O https://www12.statcan.gc.ca/census-recensement/2021/geo/sip-pis/boundary-limites/files-fichiers/lfsa000b21a_e.zip
curl -O https://www12.statcan.gc.ca/census-recensement/2021/geo/sip-pis/boundary-limites/files-fichiers/lcma000b21a_e.zip

# 3. Verify checksums.
shasum -a 256 *.zip

# 4. Run the ETL.
cd ../../../api
go run ./cmd/server etl regenerate --country=CA --src=../etl/sources --out=seed
```

StatsCan boundary files are released under the Statistics Canada
Open License (attribution required). See `LICENSE-DATA`.

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
