# Seed Determinism Gate (#67) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make "committed `api/seed/` == `etl regenerate` output" a *checked* invariant instead of an honor-system convention, so the kind of drift that shipped in PR #66 (committed seed lagging the ETL code) fails CI instead of slipping through.

**Architecture:** Two complementary gates.
1. **Real-data drift gate** — a `just seed-check` recipe (mirrors the existing `api-gen-check` / `web-gen-check` pattern) that fetches the *public* upstream sources, regenerates the **HUD-free** targets in place, and `git diff --exit-code`s them. Covers `regions_us_msas.toml` (US, Census-only) and `regions_ca_cmas.toml` (CA, StatsCan CMA only). Wired into a new `data` job in `ci.yml`.
2. **Generator-logic determinism gate** — Go golden-file tests that feed *tiny synthetic* sources (including a synthetic HUD and synthetic CA FSA) through the full `Regenerate` pipeline and assert byte-for-byte output. Covers all generator logic — crucially the **US/CA postal** paths that the real-data gate can't (HUD is account-gated; CA FSA is 155 MB). Runs under the existing `api-test` step.

Enabling both requires two small ETL-command capabilities: a `--target` selector (so the US region file can be regenerated without clobbering the HUD-dependent postal CSV) and an `Optional`/target-aware `download` (so `etl download US --target regions` fetches only the public Census files and skips the account-gated HUD landing page).

**Tech Stack:** Go 1.26 (urfave/cli v3, `log/slog`, `google/go-cmp`), `just` 1.51, GitHub Actions (`jdx/mise-action@v4`), mise (Go/Node/python pinned), Python 3.14 + openpyxl (xlsx→csv step).

---

## Prerequisites (do not start until both are true)

1. **PR #66 is merged to `main`.** It backfills `regions_us_msas.toml` + `postal_codes_us.csv` so the US half of the gate starts green. (Decision: "After #66 merges".)
2. You are on a **fresh branch off updated `main`**, e.g. `git checkout main && git pull && git checkout -b ci/seed-determinism-gate`. Do NOT build this on the #66 branch (it would entangle with #66's unmerged overrides). Use the `superpowers:using-git-worktrees` skill if you want isolation.

**Local requirement for running the gate by hand:** `etl/sources/us/` and `etl/sources/ca/` must hold the sha256-pinned vintages from `etl/SOURCES.md`, *including* `hud_zip_county_2025q4.csv` (HUD) for any *full* (`--target all`) regenerate. The gate itself only needs the public sources.

---

## Decisions locked for this plan

- **CA postal: fixture-only.** The real-data gate covers US-region + CA-region only. US-postal and CA-postal *logic* are covered by the Task 7/8 golden tests via synthetic sources. CI never downloads the 155 MB CA FSA file.
- **Sources fetched in CI (not vendored).** The 155 MB FSA rules out vendoring; the existing `etl download` already does sha256-validated fetch. CI caches `etl/sources/` keyed on `etl/SOURCES.md` so it downloads once.

---

## File Structure

| File | Responsibility | Change |
|---|---|---|
| `api/internal/etl/etl.go` | `Target` type + helpers; `SourceDescriptor.Optional`/`.Targets`; `Country.Regenerate` signature | Modify |
| `api/internal/etl/etl_test.go` | unit tests for `Target` helpers + `SourceDescriptor.FeedsTarget` | Create |
| `api/internal/etl/us/us.go` | thread `target` into `Regenerate`; guard the HUD/postal block | Modify |
| `api/internal/etl/ca/ca.go` | thread `target`; guard the FSA parse + postal write | Modify |
| `api/internal/etl/us/us.go` (Sources) | mark HUD `Optional`, `Targets: postal` | Modify |
| `api/internal/etl/ca/ca.go` (Sources) | mark FSA `Targets: postal` | Modify |
| `api/cmd/server/etl.go` | `--target` flag; validate + pass to download loop & `Regenerate`; download skips `Optional`/non-target sources | Modify |
| `api/internal/etl/us/regenerate_golden_test.go` | end-to-end golden determinism test (synthetic sources incl. HUD) | Create |
| `api/internal/etl/us/testdata/golden/` | synthetic source inputs + golden outputs | Create |
| `api/internal/etl/ca/regenerate_golden_test.go` | end-to-end golden determinism test (synthetic zip+DBF) | Create |
| `api/internal/etl/ca/testdata/golden/` | synthetic source inputs + golden outputs | Create |
| `justfile` | `seed-check` recipe; wire into `ci` | Modify |
| `.github/workflows/ci.yml` | new `data` job running `seed-check` with source cache | Modify |
| `api/seed/regions_ca_cmas.toml` | CA region header backfill (start gate green) | Modify (regen) |
| `etl/SOURCES.md` | document the gate + the `Optional`/target conventions | Modify |
| `docs/deploy.md` | regenerate → verify → commit → deploy runbook subsection | Modify |

---

## Task 1: Backfill the stale CA region header (start the gate green)

The CA region file carries the same package-rename header drift PR #66 fixed for US (`internal/loadregions/write.go` → `internal/seedfiles/regions.go`). The gate would start RED on `main` until this is regenerated and committed.

**Files:**
- Modify (regen): `api/seed/regions_ca_cmas.toml`

- [ ] **Step 1: Confirm CA sources are staged**

Run: `ls -1 etl/sources/ca/`
Expected: includes `lcma000b21a_e.zip` and `lfsa000b21a_e.zip`. If absent: `just etl-download CA` then `ls`.

- [ ] **Step 2: Regenerate CA in place**

Run: `just etl-regenerate CA`
Expected: log ends `etl regenerate: complete country=CA`.

- [ ] **Step 3: Verify the diff is the header-only rename (the safe kind)**

Run: `git diff --stat api/seed/ && git diff api/seed/regions_ca_cmas.toml`
Expected: ONLY `api/seed/regions_ca_cmas.toml` changed, exactly one line:
```
-# parent resolution lives in internal/loadregions/write.go.
+# parent resolution lives in internal/seedfiles/regions.go.
```
`postal_codes_ca.csv` must be unchanged. If `postal_codes_ca.csv` also changed or any data rows moved, STOP — your CA source vintages don't match `etl/SOURCES.md`; re-stage and investigate.

- [ ] **Step 4: Commit**

```bash
git add api/seed/regions_ca_cmas.toml
git commit -m "seed: regenerate CA region file to clear stale header drift

The generated header comment still referenced the pre-rename
internal/loadregions/write.go path; a CA regenerate refreshes it to
internal/seedfiles/regions.go. Data rows unchanged. Makes the seed-check
gate (#67) start green for CA."
```

---

## Task 2: Add the `Target` type and source metadata to the etl package

**Files:**
- Modify: `api/internal/etl/etl.go`
- Create: `api/internal/etl/etl_test.go`

- [ ] **Step 1: Write the failing test**

Create `api/internal/etl/etl_test.go`:
```go
package etl

import "testing"

func TestTargetMembership(t *testing.T) {
	cases := []struct {
		target           Target
		regions, postal  bool
	}{
		{TargetAll, true, true},
		{TargetRegions, true, false},
		{TargetPostal, false, true},
	}
	for _, c := range cases {
		if got := c.target.Regions(); got != c.regions {
			t.Errorf("%q.Regions() = %v, want %v", c.target, got, c.regions)
		}
		if got := c.target.Postal(); got != c.postal {
			t.Errorf("%q.Postal() = %v, want %v", c.target, got, c.postal)
		}
	}
}

func TestParseTarget(t *testing.T) {
	for _, in := range []string{"all", "regions", "postal"} {
		if _, err := ParseTarget(in); err != nil {
			t.Errorf("ParseTarget(%q) unexpected err: %v", in, err)
		}
	}
	if _, err := ParseTarget("bogus"); err == nil {
		t.Error("ParseTarget(\"bogus\") = nil err, want error")
	}
}

func TestSourceFeedsTarget(t *testing.T) {
	all := SourceDescriptor{Filename: "a"}                       // empty Targets ⇒ feeds everything
	postalOnly := SourceDescriptor{Filename: "h", Targets: []Target{TargetPostal}}
	if !all.FeedsTarget(TargetRegions) || !all.FeedsTarget(TargetPostal) {
		t.Error("empty Targets should feed every target")
	}
	if postalOnly.FeedsTarget(TargetRegions) {
		t.Error("postal-only source should not feed regions")
	}
	if !postalOnly.FeedsTarget(TargetPostal) || !postalOnly.FeedsTarget(TargetAll) {
		t.Error("postal-only source should feed postal and all")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd api && go test ./internal/etl/ -run 'TestTarget|TestParseTarget|TestSourceFeeds' 2>&1 | head`
Expected: FAIL — `undefined: Target`, `undefined: ParseTarget`, `FeedsTarget`.

- [ ] **Step 3: Add the type, helpers, and source fields**

In `api/internal/etl/etl.go`, add after the imports / near the top of the type declarations:
```go
// Target selects which seed outputs a regenerate (or download) operates
// on. The default is TargetAll. TargetRegions exists so CI can refresh
// the HUD-free region files without touching the HUD-dependent postal
// CSV (US) or needing the 155 MB CA FSA source.
type Target string

const (
	TargetAll     Target = "all"
	TargetRegions Target = "regions"
	TargetPostal  Target = "postal"
)

// Regions reports whether this target includes the region (TOML) outputs.
func (t Target) Regions() bool { return t == TargetAll || t == TargetRegions }

// Postal reports whether this target includes the postal-code (CSV) outputs.
func (t Target) Postal() bool { return t == TargetAll || t == TargetPostal }

// ParseTarget validates a CLI --target value.
func ParseTarget(s string) (Target, error) {
	switch Target(s) {
	case TargetAll, TargetRegions, TargetPostal:
		return Target(s), nil
	default:
		return "", fmt.Errorf("invalid target %q (want one of: all, regions, postal)", s)
	}
}
```

In the `SourceDescriptor` struct, add two fields (after `Vintage`):
```go
	// Optional marks a source the download step must NOT fail on — e.g.
	// HUD, whose canonical URL is an account-gated landing page, not a
	// direct file. Download skips Optional sources with a notice; the
	// operator fetches them by hand. Regenerate already tolerates their
	// absence.
	Optional bool
	// Targets lists which output Targets this source feeds. Empty means
	// it feeds every target. Download uses this to fetch only what a
	// given --target needs (e.g. --target=regions skips the CA FSA).
	Targets []Target
```

Add a method below the struct:
```go
// FeedsTarget reports whether this source is needed to produce target.
func (s SourceDescriptor) FeedsTarget(target Target) bool {
	if len(s.Targets) == 0 || target == TargetAll {
		return true
	}
	for _, t := range s.Targets {
		if t == target {
			return true
		}
	}
	return false
}
```

Ensure `"fmt"` is imported in `etl.go` (add to the import block if missing).

- [ ] **Step 4: Run to verify it passes**

Run: `cd api && go test ./internal/etl/ -run 'TestTarget|TestParseTarget|TestSourceFeeds' -v 2>&1 | tail -20`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add api/internal/etl/etl.go api/internal/etl/etl_test.go
git commit -m "etl: add Target selector + Optional/Targets source metadata"
```

---

## Task 3: Thread `target` through `Country.Regenerate` and both generators

This is one cohesive compiling change: the field signature and both implementations move together.

**Files:**
- Modify: `api/internal/etl/etl.go` (the `Regenerate` field), `api/internal/etl/us/us.go`, `api/internal/etl/ca/ca.go`

- [ ] **Step 1: Write the failing test (region-only skips the postal output)**

Create `api/internal/etl/us/regenerate_target_test.go`:
```go
package us

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/mjrossi/urbanist-atlas/api/internal/etl"
)

// minimal public-source fixtures (no HUD) sufficient to write the region TOML.
const tinyCBSA = `CBSA Code,Metropolitan/Micropolitan Statistical Area,CBSA Title,State Name,FIPS State Code,FIPS County Code,Central/Outlying County
35620,Metropolitan Statistical Area,"New York-Newark-Jersey City, NY-NJ",New York,36,061,Central
`
const tinyZCTAPlace = `ZCTA5	GEOID	NAME	...
10001	3651000	New York city
`
const tinyZCTACounty = `ZCTA5	GEOID	NAME	...
10001	36061	New York County
`

func TestRegenerate_TargetRegionsSkipsPostal(t *testing.T) {
	src := t.TempDir()
	out := t.TempDir()
	mustWrite(t, filepath.Join(src, "list1_2023.csv"), tinyCBSA)
	mustWrite(t, filepath.Join(src, "tab20_zcta520_place20_natl.txt"), tinyZCTAPlace)
	mustWrite(t, filepath.Join(src, "tab20_zcta520_county20_natl.txt"), tinyZCTACounty)
	mustWrite(t, filepath.Join(out, "regions_us_msa_overrides.toml"), "")

	err := Regenerate(context.Background(), src, out, etl.TargetRegions, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if err != nil {
		t.Fatalf("Regenerate(regions): %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "regions_us_msas.toml")); err != nil {
		t.Errorf("region TOML not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "postal_codes_us.csv")); !os.IsNotExist(err) {
		t.Errorf("postal CSV should NOT be written for TargetRegions (stat err=%v)", err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
```
> NOTE: the exact tab/column layout of the ZCTA fixtures must match what `loadZCTAPlace`/`loadZCTACounty` parse. Before finalizing, open `api/internal/etl/us/zcta.go` and copy the real header + a representative row; replace the `...` placeholders above with the real columns. The test only asserts file presence/absence, so row *content* need not be meaningful — only parseable.

- [ ] **Step 2: Run to verify it fails to compile**

Run: `cd api && go test ./internal/etl/us/ -run TestRegenerate_TargetRegions 2>&1 | head`
Expected: FAIL — `Regenerate` takes 4 args, not 5 (`too many arguments`).

- [ ] **Step 3: Change the `Regenerate` field signature in `etl.go`**

In `api/internal/etl/etl.go`, the `Country.Regenerate` field:
```go
	Regenerate func(ctx context.Context, srcDir, outDir string, target Target, logger *slog.Logger) error
```

- [ ] **Step 4: Thread + guard in `api/internal/etl/us/us.go`**

Change the signature (line ~121):
```go
func Regenerate(ctx context.Context, srcDir, outDir string, target etl.Target, logger *slog.Logger) error {
```
Add the import `"github.com/mjrossi/urbanist-atlas/api/internal/etl"` if not present.

The CBSA + ZCTA loads and the region write (current lines 122–161) stay unconditional (regions need them). Wrap the **entire HUD/reconcile/backfill/postal-write block** (current lines 163–220, from `anchors, reasons := Crosswalk(...)` through the `logger.Info("etl us: wrote postal codes", ...)`) in:
```go
	if target.Postal() {
		anchors, reasons := Crosswalk(zctaPlace, zctaCounty, countyToMSA, cbsaToSlug)
		// ... existing HUD load, ReconcileCTLegacyCounties, CrosswalkHUDBackfill,
		// writeCSV, and the "wrote postal codes" log, unchanged ...
	}
```
Guard the region write too, for symmetry (so `--target postal` doesn't rewrite the TOML). Wrap current lines 157–161 (`msaTOMLPath := ...` through its log) in `if target.Regions() { ... }`.

- [ ] **Step 5: Thread + guard in `api/internal/etl/ca/ca.go`**

Change the signature (line ~70):
```go
func Regenerate(ctx context.Context, srcDir, outDir string, target etl.Target, logger *slog.Logger) error {
```
Add the `etl` import if not present.

CRITICAL: CA currently `ParseFSAs` the 155 MB zip up front (line ~80) before any write. Move that parse INTO the postal branch so `--target regions` never needs the FSA file. Restructure:
```go
	cmaZipPath := filepath.Join(srcDir, "lcma000b21a_e.zip")
	cmas, err := ParseCMAs(cmaZipPath)
	if err != nil {
		return err
	}
	logger.Info("etl ca: parsed CMA boundary", "cmas", len(cmas), "path", cmaZipPath)

	assignments := assignCMAs(cmas)
	knownCMASlugs := make(map[string]bool, len(assignments))
	for _, a := range assignments {
		knownCMASlugs[a.Slug] = true
	}

	if target.Regions() {
		tomlPath := filepath.Join(outDir, "regions_ca_cmas.toml")
		if err := writeCMAsToFile(tomlPath, assignments); err != nil {
			return err
		}
		logger.Info("etl ca: wrote CMAs", "path", tomlPath, "count", len(assignments))
	}

	if target.Postal() {
		fsaZipPath := filepath.Join(srcDir, "lfsa000b21a_e.zip")
		fsas, err := ParseFSAs(fsaZipPath)
		if err != nil {
			return err
		}
		logger.Info("etl ca: parsed FSA boundary", "fsas", len(fsas), "path", fsaZipPath)

		anchors, reasons := Crosswalk(fsas, knownCMASlugs)
		csvPath := filepath.Join(outDir, "postal_codes_ca.csv")
		if err := writeCSVToFile(csvPath, anchors); err != nil {
			return err
		}
		logger.Info("etl ca: wrote postal codes", "path", csvPath, "count", len(anchors), "by_reason", fmt.Sprintf("%+v", reasons))
	}
	return nil
```

- [ ] **Step 6: Run the target test + full etl build**

Run: `cd api && go build ./... && go test ./internal/etl/... -run 'TestRegenerate_TargetRegions' -v 2>&1 | tail -15`
Expected: build OK; PASS.

- [ ] **Step 7: Commit**

```bash
git add api/internal/etl/etl.go api/internal/etl/us/us.go api/internal/etl/ca/ca.go api/internal/etl/us/regenerate_target_test.go
git commit -m "etl: thread Target through Regenerate; guard postal/region writes

US: postal/HUD block runs only for TargetPostal. CA: the 155 MB FSA
parse moves into the postal branch so TargetRegions needs only the CMA
source."
```

---

## Task 4: Add `--target` to the CLI and make `download` target/Optional-aware

**Files:**
- Modify: `api/cmd/server/etl.go`
- Modify: `api/internal/etl/us/us.go` (HUD source descriptor), `api/internal/etl/ca/ca.go` (FSA source descriptor)

- [ ] **Step 1: Tag the gated/targeted sources**

In `api/internal/etl/us/us.go`, the HUD `SourceDescriptor` — add fields:
```go
		{
			Filename: "hud_zip_county_2025q4.csv",
			URL:      "https://www.huduser.gov/portal/dataset/uspszip-api.html",
			SHA256:   "2795b91c26703d1150f2545683da0b6638d006f213e48cc70318e384b3f00f8b",
			Vintage:  "HUD USPS ZIP-to-County crosswalk, 2025-Q4 (operator-downloaded; HUD account required)",
			Optional: true,
			Targets:  []etl.Target{etl.TargetPostal},
		},
```
In `api/internal/etl/ca/ca.go`, the FSA `SourceDescriptor` (`lfsa000b21a_e.zip`) — add:
```go
			Targets:  []etl.Target{etl.TargetPostal},
```
(The CMA source and all US Census sources keep empty `Targets` — they feed both regions and postal.)

- [ ] **Step 2: Add the `--target` flag to `commonFlags`**

In `api/cmd/server/etl.go`, append to `commonFlags` (after the `out` flag):
```go
		&cli.StringFlag{
			Name:  "target",
			Usage: "which outputs to operate on: all, regions, or postal",
			Value: "all",
		},
```

- [ ] **Step 3: Pass target into regenerate**

In `runEtlRegenerate`, after resolving `outDir`:
```go
	target, err := etl.ParseTarget(c.String("target"))
	if err != nil {
		return fmt.Errorf("etl regenerate: %w", err)
	}
```
Change the call site:
```go
	if err := plan.Regenerate(ctx, srcDir, outDir, target, logger); err != nil {
```
Add `"target", target` to the "etl regenerate: start" log fields.

- [ ] **Step 4: Make the download loop skip Optional / non-target sources**

In `runEtlDownload`, parse the target after resolving `srcDir`:
```go
	target, err := etl.ParseTarget(c.String("target"))
	if err != nil {
		return fmt.Errorf("etl download: %w", err)
	}
```
Replace the download loop body:
```go
	for _, src := range plan.Sources {
		if src.Optional {
			logger.Info("etl download: skipping account-gated source (fetch manually)",
				"filename", src.Filename, "url", src.URL)
			continue
		}
		if !src.FeedsTarget(target) {
			logger.Info("etl download: skipping source not needed for target",
				"filename", src.Filename, "target", target)
			continue
		}
		dst := filepath.Join(srcDir, src.Filename)
		if err := downloadSource(ctx, client, src, dst, logger); err != nil {
			return fmt.Errorf("etl download %s: %w", src.Filename, err)
		}
	}
```

- [ ] **Step 5: Build + manual smoke (regions-only US, no HUD/no clobber)**

```bash
cd api && go build ./...
# region-only regen must NOT touch postal_codes_us.csv:
go run ./cmd/server etl regenerate --country US --target regions --src ../etl/sources --out seed
cd .. && git status --short api/seed/
```
Expected: build OK; `git status` shows `regions_us_msas.toml` clean (already current from #66) and `postal_codes_us.csv` UNCHANGED. If `postal_codes_us.csv` shows as modified, the postal guard in Task 3 is wrong — fix before continuing.

- [ ] **Step 6: Commit**

```bash
git add api/cmd/server/etl.go api/internal/etl/us/us.go api/internal/etl/ca/ca.go
git commit -m "etl: add --target flag; download skips Optional + off-target sources"
```

---

## Task 5: Add the `just seed-check` recipe and wire it into `ci`

**Files:**
- Modify: `justfile`

- [ ] **Step 1: Add the recipe** (place in the `[group('data')]` block, after `etl-regenerate`)

```justfile
# Fail if the committed HUD-free seed (US/CA region files) drifts from a
# fresh regenerate of the pinned public sources. Mirrors api-gen-check /
# web-gen-check. HUD-dependent US postal + CA postal are covered by the
# golden determinism tests under `just api-test`, not here (HUD is
# account-gated; the CA FSA source is 155 MB).
[group('data')]
[doc('fail if committed region seed drifts from a regen of public sources')]
seed-check:
    @cd api && go run ./cmd/server etl download --country US --target regions --src ../etl/sources --out seed
    @python3 etl/scripts/xlsx_to_csv.py etl/sources/us/list1_2023.xlsx etl/sources/us/list1_2023.csv
    @cd api && go run ./cmd/server etl download --country CA --target regions --src ../etl/sources --out seed
    @cd api && go run ./cmd/server etl regenerate --country US --target regions --src ../etl/sources --out seed
    @cd api && go run ./cmd/server etl regenerate --country CA --target regions --src ../etl/sources --out seed
    @git diff --exit-code -- api/seed/regions_us_msas.toml api/seed/regions_ca_cmas.toml \
        || (echo "seed drift: region files differ from a fresh regen. Stage sources, run \`just etl-regenerate US\` / \`CA\`, and commit." && exit 1)
```
> NOTE on the python line: `etl regenerate` reads `list1_2023.csv`, but `etl download` fetches `list1_2023.xlsx`. The xlsx→csv conversion (Census ships only xlsx) must run between them. The path is relative to repo root; the `@cd api` recipes do not change the recipe's own working directory between lines (each line is its own shell), so the python line runs from repo root — verify with `just --evaluate` if unsure.

- [ ] **Step 2: Wire into the top-level `ci` aggregate**

Find the `ci:` recipe (currently `ci: api-check web-check`) and add `seed-check`:
```justfile
ci: api-check web-check seed-check
```

- [ ] **Step 3: Run it locally (sources staged) — expect green**

Run: `just seed-check`
Expected: downloads run, regenerates run, `git diff --exit-code` exits 0 (no output, success). If it fails, the committed region files are stale — regenerate and commit them first (that IS the gate doing its job).

- [ ] **Step 4: Prove the gate BITES (negative test)**

```bash
# Temporarily corrupt a region file and confirm seed-check fails:
printf '\n# drift probe\n' >> api/seed/regions_ca_cmas.toml
just seed-check; echo "exit=$?"
git checkout -- api/seed/regions_ca_cmas.toml
```
Expected: `just seed-check` prints the "seed drift" message and exits non-zero. Restore leaves the tree clean.

- [ ] **Step 5: Commit**

```bash
git add justfile
git commit -m "just: add seed-check region drift gate; wire into ci"
```

---

## Task 6: Add the `data` CI job

**Files:**
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Add a `data` job** (model it on the existing `api` job: `actions/checkout@v6`, `jdx/mise-action@v4`, the `mise env --dotenv >> "$GITHUB_ENV"` step). Insert after the `web` job, before `deploy-api`. Gate it on the `detect` job's `api` output (the ETL binary lives in `api/`).

```yaml
  data:
    name: seed determinism
    needs: detect
    if: needs.detect.outputs.api == 'true'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
      - name: Cache ETL upstream sources
        uses: actions/cache@v5
        with:
          path: etl/sources
          key: ${{ runner.os }}-etlsrc-${{ hashFiles('etl/SOURCES.md') }}
      - name: Cache Go modules + build cache
        uses: actions/cache@v5
        with:
          path: |
            ~/.cache/go-build
            ~/go/pkg/mod
          key: ${{ runner.os }}-go-${{ hashFiles('api/go.sum') }}
          restore-keys: |
            ${{ runner.os }}-go-
      - uses: jdx/mise-action@v4
      - name: Export mise [env] to GITHUB_ENV
        run: mise env --dotenv >> "$GITHUB_ENV"
      - name: Install xlsx→csv deps
        run: pip install -r etl/scripts/requirements.txt
      - run: just seed-check
```
> NOTE: `seed-check` is also in the `ci` aggregate (Task 5), but the `ci` recipe isn't what runs in CI — the workflow calls `just api-check` / `just web-check` per job. This dedicated `data` job is what actually runs `seed-check` in GitHub Actions. Keeping it in the `ci` aggregate too lets a developer run the whole gate locally with `just ci`.

- [ ] **Step 2: Make `deploy-api` wait on the new job**

Change `deploy-api`'s `needs: [api, web]` to `needs: [api, web, data]` so a drifted seed blocks deploy.

- [ ] **Step 3: Validate the workflow YAML**

Run: `cd /Users/mrossi/dev/urbanist-atlas && python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/ci.yml')); print('yaml ok')"`
Expected: `yaml ok`.

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add seed-determinism job; block deploy on seed drift"
```

---

## Task 7: US generator golden determinism test (covers postal logic incl. HUD)

This is the gate that protects the US **postal** path (reconcile + HUD backfill) that the real-data gate can't run in CI. Uses the Go golden-file idiom: a `-update` flag regenerates committed goldens; normal runs assert byte-equality.

**Files:**
- Create: `api/internal/etl/us/regenerate_golden_test.go`
- Create: `api/internal/etl/us/testdata/golden/` (synthetic sources + golden outputs)

- [ ] **Step 1: Write the synthetic source fixtures**

Create small, hand-authored files under `api/internal/etl/us/testdata/golden/sources/`. They must exercise the interesting paths: a normal metro ZIP, a CT legacy-county ZIP that the reconcile repairs (so the golden proves the reconcile), and a HUD-only (non-ZCTA) ZIP for the backfill.
- `list1_2023.csv` — header + a couple of MSAs incl. a CT metro (e.g. Hartford CBSA 25540) and NYC (35620).
- `tab20_zcta520_place20_natl.txt`, `tab20_zcta520_county20_natl.txt` — a handful of ZCTAs incl. a CT one keyed by the legacy Fairfield/Hartford county GEOID that is absent from the CBSA county set (to trigger reconcile).
- `hud_zip_county_2025q4.csv` — rows for the CT ZIP (current-vintage county → metro) and one non-ZCTA backfill ZIP.
- `regions_us_msa_overrides.toml` — copy the relevant override(s) (e.g. bridgeport-ct-metro parents) or leave empty if the chosen CBSAs need none.
> Derive exact column layouts by reading `cbsa.go`, `zcta.go`, `hud.go` parsers and `etl/SOURCES.md`. Keep each file to <10 data rows.

- [ ] **Step 2: Write the golden test**

Create `api/internal/etl/us/regenerate_golden_test.go`:
```go
package us

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/mjrossi/urbanist-atlas/api/internal/etl"
)

var update = flag.Bool("update", false, "regenerate golden files")

func TestRegenerate_GoldenDeterminism(t *testing.T) {
	srcDir := "testdata/golden/sources"
	goldenDir := "testdata/golden/expected"
	outDir := t.TempDir()

	// Overrides are read from outDir; copy the fixture override in.
	copyFile(t, filepath.Join(srcDir, "regions_us_msa_overrides.toml"),
		filepath.Join(outDir, "regions_us_msa_overrides.toml"))

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	if err := Regenerate(context.Background(), srcDir, outDir, etl.TargetAll, logger); err != nil {
		t.Fatalf("Regenerate: %v", err)
	}

	for _, name := range []string{"regions_us_msas.toml", "postal_codes_us.csv"} {
		got, err := os.ReadFile(filepath.Join(outDir, name))
		if err != nil {
			t.Fatalf("read output %s: %v", name, err)
		}
		goldenPath := filepath.Join(goldenDir, name)
		if *update {
			if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
				t.Fatalf("update golden %s: %v", name, err)
			}
			continue
		}
		want, err := os.ReadFile(goldenPath)
		if err != nil {
			t.Fatalf("read golden %s (run with -update first): %v", name, err)
		}
		if string(got) != string(want) {
			t.Errorf("%s drifted from golden. Run `go test ./internal/etl/us -run GoldenDeterminism -update` and review the diff.", name)
		}
	}
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read %s: %v", src, err)
	}
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		t.Fatalf("write %s: %v", dst, err)
	}
}
```

- [ ] **Step 3: Generate + sanity-check the goldens**

```bash
cd api && mkdir -p internal/etl/us/testdata/golden/expected
go test ./internal/etl/us -run GoldenDeterminism -update
```
Then OPEN `api/internal/etl/us/testdata/golden/expected/postal_codes_us.csv` and confirm: the CT fixture ZIP resolved to its `*-ct-metro` (reconcile worked), and the non-ZCTA ZIP appears (HUD backfill worked). If the CT ZIP is still bare `ct`, your fixtures don't trigger the reconcile — fix the county GEOIDs and regenerate.

- [ ] **Step 4: Run without `-update` to confirm it passes deterministically**

Run: `cd api && go test ./internal/etl/us -run GoldenDeterminism -v 2>&1 | tail`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/etl/us/regenerate_golden_test.go api/internal/etl/us/testdata/
git commit -m "etl/us: golden determinism test over full Regenerate (incl. HUD reconcile)"
```

---

## Task 8: CA generator golden determinism test (synthetic zip+DBF)

CA reads DBF tables inside zip files. Reuse the DBF-bytes builder from the existing `TestDBFReader` (`api/internal/etl/ca/etl_test.go`) and wrap the bytes in an in-memory zip written to the fixture source dir.

**Files:**
- Create: `api/internal/etl/ca/regenerate_golden_test.go`
- Create: `api/internal/etl/ca/testdata/golden/expected/`

- [ ] **Step 1: Identify the DBF builder + zip member names**

Read `api/internal/etl/ca/etl_test.go` `TestDBFReader` for the DBF byte-construction helper, and `api/internal/etl/ca/ca.go` `ParseCMAs`/`ParseFSAs` for the exact DBF member name expected inside each zip (e.g. `lcma000b21a_e.dbf`). Note the required DBF columns (CMA: CMAUID/CMANAME-ish; FSA: CFSAUID/…) from the parsers.

- [ ] **Step 2: Write the test** (helper builds a zip containing one `.dbf` member)

Create `api/internal/etl/ca/regenerate_golden_test.go`:
```go
package ca

import (
	"archive/zip"
	"bytes"
	"context"
	"flag"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/mjrossi/urbanist-atlas/api/internal/etl"
)

var update = flag.Bool("update", false, "regenerate golden files")

// writeZipWithDBF writes a zip at zipPath containing a single member
// (dbfName) whose bytes are dbf. Build dbf with the same helper used by
// TestDBFReader in etl_test.go.
func writeZipWithDBF(t *testing.T, zipPath, dbfName string, dbf []byte) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(dbfName)
	if err != nil {
		t.Fatalf("zip create member: %v", err)
	}
	if _, err := w.Write(dbf); err != nil {
		t.Fatalf("zip write member: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	if err := os.WriteFile(zipPath, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write zip %s: %v", zipPath, err)
	}
}

func TestRegenerate_CAGoldenDeterminism(t *testing.T) {
	srcDir := t.TempDir()
	goldenDir := "testdata/golden/expected"

	// Build synthetic CMA + FSA DBFs (a couple of rows each) and zip them
	// under the member names ParseCMAs/ParseFSAs expect.
	cmaDBF := buildSyntheticCMADBF(t) // construct via the etl_test.go DBF builder
	fsaDBF := buildSyntheticFSADBF(t)
	writeZipWithDBF(t, filepath.Join(srcDir, "lcma000b21a_e.zip"), "lcma000b21a_e.dbf", cmaDBF)
	writeZipWithDBF(t, filepath.Join(srcDir, "lfsa000b21a_e.zip"), "lfsa000b21a_e.dbf", fsaDBF)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	if err := Regenerate(context.Background(), srcDir, srcDir, etl.TargetAll, logger); err != nil {
		t.Fatalf("Regenerate: %v", err)
	}

	for _, name := range []string{"regions_ca_cmas.toml", "postal_codes_ca.csv"} {
		got, err := os.ReadFile(filepath.Join(srcDir, name))
		if err != nil {
			t.Fatalf("read output %s: %v", name, err)
		}
		goldenPath := filepath.Join(goldenDir, name)
		if *update {
			if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
				t.Fatalf("update golden %s: %v", name, err)
			}
			continue
		}
		want, err := os.ReadFile(goldenPath)
		if err != nil {
			t.Fatalf("read golden %s (run with -update first): %v", name, err)
		}
		if string(got) != string(want) {
			t.Errorf("%s drifted from golden. Run with -update and review.", name)
		}
	}
}
```
> `buildSyntheticCMADBF` / `buildSyntheticFSADBF` must produce DBFs with the columns the parsers read. Factor the DBF-byte builder out of `TestDBFReader` into a shared test helper (e.g. `dbf_testhelpers_test.go`) so both tests use it — DRY.

- [ ] **Step 3: Generate + sanity-check goldens**

```bash
cd api && mkdir -p internal/etl/ca/testdata/golden/expected
go test ./internal/etl/ca -run CAGoldenDeterminism -update
```
Open the two golden files; confirm the synthetic CMA produced a region row and the synthetic FSA resolved to it.

- [ ] **Step 4: Run without `-update`**

Run: `cd api && go test ./internal/etl/ca -run CAGoldenDeterminism -v 2>&1 | tail`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/etl/ca/regenerate_golden_test.go api/internal/etl/ca/testdata/ api/internal/etl/ca/dbf_testhelpers_test.go
git commit -m "etl/ca: golden determinism test over full Regenerate (synthetic zip+DBF)"
```

---

## Task 9: Documentation

**Files:**
- Modify: `etl/SOURCES.md`, `docs/deploy.md`

- [ ] **Step 1: Update `etl/SOURCES.md`**

In the "Known data-vintage gaps" / Seed note area, replace the "CI doesn't gate on `etl regenerate`" caveat with the new reality: a `seed-check` CI job now gates the HUD-free region files on regenerate drift, and the US/CA postal logic is gated by the golden determinism tests. Document the two new source-descriptor conventions: `Optional: true` (download skips; fetch by hand) and `Targets: []Target` (download fetches only what a `--target` needs). Note `etl download US --target regions` now succeeds without HUD.

- [ ] **Step 2: Add a runbook subsection to `docs/deploy.md`**

Add a "Regenerating seed data" subsection documenting the chained operator flow the gate is built around:
```sh
# Maintainer machine with etl/sources/{us,ca}/ staged (incl. HUD for US postal):
just etl-regenerate US        # full: regions + postal (needs HUD)
just etl-regenerate CA        # full: regions + postal (needs FSA)
just seed-check               # verify committed == regenerated (region files)
cd api && go test ./internal/etl/... -run GoldenDeterminism   # verify generator logic
git add api/seed/ && git commit -m "seed: regenerate <CC>"
git push                      # → merge to main → GHA deploy-api ships the new image
```
Call out the sharp edges from #67: sources must match `SOURCES.md` vintages; HUD must be staged for a full US regenerate or the CT reconcile silently reverts; the diff should be country-scoped — churn elsewhere ⇒ stop and investigate.

- [ ] **Step 3: Commit**

```bash
git add etl/SOURCES.md docs/deploy.md
git commit -m "docs: document the seed-determinism gate + regenerate runbook"
```

---

## Task 10: Full local gate run + PR

- [ ] **Step 1: Run the complete CI aggregate locally**

Run: `just ci`
Expected: `api-check`, `web-check`, and `seed-check` all pass. (Requires sources staged + python deps for `seed-check`.)

- [ ] **Step 2: Run the full Go test suite**

Run: `cd api && go test ./... 2>&1 | tail -20`
Expected: all PASS, including both `GoldenDeterminism` tests and the `Target` tests.

- [ ] **Step 3: Open the PR** (closes #67)

```bash
git push -u origin ci/seed-determinism-gate
gh pr create --title "CI: gate seed determinism with an etl regenerate drift check" \
  --body "Implements #67. Two gates: (1) just seed-check real-data drift gate for the HUD-free region files (new ci.yml \`data\` job); (2) golden determinism tests covering US/CA postal logic (incl. the CT reconcile) without needing HUD/the 155MB FSA in CI. Adds an etl \`--target\` selector and Optional/target-aware download. CA region header backfilled so the gate starts green. Closes #67."
```

---

## Self-Review (completed against #67 acceptance criteria)

- [x] **Decide the gate scope** → real-data gate (regions, opt 1) + fixture gate (generator logic, opt 2). US/CA postal real-data deferred (HUD account-gated; FSA 155 MB) — covered by goldens. (Tasks 5–8)
- [x] **Vendor or sha256-pin the public sources** → fetch-in-CI via `etl download` (already sha256-validates), cached on `SOURCES.md` hash. No vendoring (155 MB FSA). (Task 6)
- [x] **Add `just seed-check` + wire into `ci`** (Task 5) and into the workflow (Task 6).
- [x] **Make HUD/source presence + vintage explicit** → `Optional` (download skips, fetch by hand) + `Targets` (fetch only what `--target` needs); download skip is logged. (Tasks 2, 4)
- [x] **Document the regenerate → verify → commit → deploy runbook** (Task 9, `docs/deploy.md`).
- [x] **Backfill the committed CT seed** → US done in #66 (prerequisite); CA region header backfilled in Task 1.

**Type-consistency check:** `Target` / `TargetAll|Regions|Postal` / `ParseTarget` / `Target.Regions()` / `Target.Postal()` / `SourceDescriptor.FeedsTarget` / `SourceDescriptor.Optional` / `SourceDescriptor.Targets` used identically across Tasks 2–8. `Country.Regenerate(ctx, srcDir, outDir, target, logger)` signature matches in etl.go (Task 3 Step 3), us.go (Step 4), ca.go (Step 5), and the cmd call site (Task 4 Step 3).

**Open item to resolve during execution (not a blocker):** confirm the `just` recipe's per-line working directory for the python xlsx→csv step (Task 5 Step 1 NOTE) — each recipe line is its own shell, so `@cd api && ...` does not persist; the python path is repo-root-relative as written, but verify with `just --evaluate`.
