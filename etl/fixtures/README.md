# `etl/fixtures/` — committed regions sources for the offline seed gate

Unlike `etl/sources/` (gitignored, full upstream downloads), this
directory **is committed**. It holds the minimal subset of real upstream
data the `seed determinism` CI job (`just seed-check`) needs to
regenerate the two region files and assert they match what's committed
under `api/seed/`:

| File                          | What it is                                                                 | sha256 of contents |
| ----------------------------- | -------------------------------------------------------------------------- | ------------------ |
| `us/list1_2023.csv`           | Census CBSA delineation, July 2023 (xlsx→csv via `etl/scripts/xlsx_to_csv.py`) | `6ad49da23ac95fe35f6e038e6ebc54b59b071d1503c1bde249d0a585f199b14a` |
| `ca/lcma000b21a_e.zip`        | StatsCan CMA boundary file, 2021 — **DBF attribute table only**            | DBF: `6bf7efd60cb4637ce245456d4b1714f3225ffbc1f914f130bb73582191f1ca47` |

## Why these are vendored

The download-based gate was chronically flaky: GitHub Actions runners
cannot reach **Statistics Canada** (`www12.statcan.gc.ca`) — every
attempt is a TCP `i/o timeout`. Retries and caching can't fix an
unreachable host, and a cache can't seed itself from a download that
never succeeds in CI.

The **regions** pass needs very little: the US CBSA list (a 251 KB CSV)
and the CA CMA **attribute table** — `ParseCMAs` reads only the 29 KB
DBF inside the upstream 13 MB zip (`api/internal/etl/ca/cma.go`), and the
shapefile *geometry* is used only by the postal-pass spatial join. So we
commit the CSV and a minimal zip containing just the DBF (~4.6 KB), and
`seed-check` regenerates from these with **no network and no python**.

The 155 MB CA FSA source and the HUD-gated US postal CSV stay
un-vendored; their determinism is covered by the golden tests under
`just api-test` (synthetic fixtures in `api/internal/etl/*/testdata/`).

## Provenance

Both files derive byte-for-byte from the sha256-pinned upstream sources
in [`../SOURCES.md`](../SOURCES.md):

- `us/list1_2023.csv` — the exact CSV `xlsx_to_csv.py` emits from the
  pinned `list1_2023.xlsx`; its sha256 matches the value SOURCES.md pins.
- `ca/lcma000b21a_e.zip` — repackaged from the DBF inside the official
  `lcma000b21a_e.zip` (zip sha256
  `a12dd39b3262edb48f9490b435d2f43b0327cc4af7d829f32aebae4d4b9f8fa0`).
  The embedded DBF is identical to the upstream one.

## Refreshing (on a vintage bump)

These are inputs, not generated outputs, so refresh them whenever you
bump an upstream vintage (which also changes `api/seed/regions_*.toml`):

```sh
# 1. Stage the new full upstream sources under etl/sources/ as usual:
just etl-download US && just etl-download CA   # (+ the manual HUD/xlsx steps)
# 2. Rebuild these committed fixtures from them:
just seed-fixtures
# 3. Regenerate the seed and commit both the seed and the fixtures.
```

`just seed-fixtures` (in the repo `justfile`) is the canonical rebuild —
it is exactly how the files here were produced.
