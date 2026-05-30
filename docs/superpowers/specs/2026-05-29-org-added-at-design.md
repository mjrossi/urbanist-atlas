# Org `added_at` — honest recency for the "Recently indexed" strip

**Date:** 2026-05-29
**Status:** Approved (design)
**Author:** maintainer + Claude

## Problem

The homepage "Recently indexed" strip (`web/src/routes/Home.tsx`, fed by
`GET /api/v1/recent`) is supposed to show the most-recently-added
organizations. It does not.

`Org` carries a server-side-only `CreatedAt time.Time` field
(`api/pkg/atlas/atlas.go:95`, tagged `json:"-" toml:"-"`) that
`MemStore.ListRecent` sorts on (`api/pkg/atlas/memstore.go:297-298`,
`ORDER BY CreatedAt DESC, ID DESC`). But `CreatedAt` is **never
populated** — it is not a field in `orgs.toml`, and the seed loader
(`api/internal/seedfiles/build.go`) never sets it. All 202 seed orgs
therefore carry the zero-value time, tie on the primary sort key, and
fall through to the `ID DESC` tiebreaker. ID is assigned in file order
at boot, so:

> **"Recently indexed" today = the last four `[[org]]` blocks at the
> bottom of `orgs.toml`** — an editorial artifact of file position, not
> a recency signal.

This is misleading-by-implication at launch. It self-corrects only once
real approved submissions start appending to the file (higher ID → top),
so it is fundamentally a cold-start problem: meaningless until organic
submissions flow.

## Goal

Give every org a real, honest "added" date, sort the strip on it, expose
it on the API, and guarantee the dataset can never silently regress to
the undated state. Backfill the existing 202 orgs with best-guess dates
derived from auditable evidence (slice-section comments + git blame), not
fabricated precision.

## Decisions (locked during brainstorming)

| Decision | Choice |
|---|---|
| Backfill source | **Slice-section dates** from inline `orgs.toml` comments, with git blame drawing the one boundary the comments lack |
| Field strictness | **Required** — loader fails loudly if any org lacks `added_at` |
| Wire exposure | **Exposed now** in the `Org` schema (`format: date`), surfaced on cards |
| Submission date | **Approval date** — stamped by the GitHub-PR worker at approval time |
| Date format | **Date-only** (TOML local date / OpenAPI `format: date`); no fake time-of-day precision |
| `CreatedAt` | **Fully renamed to `AddedAt`** — no dead field, no lingering references |

## Design

### 1. Data model (Go)

Rename `Org.CreatedAt` → `Org.AddedAt` everywhere; do not leave a dead
field. `AddedAt` is a `time.Time` holding the date at midnight UTC.

- `api/pkg/atlas/atlas.go:95`: field becomes
  `AddedAt time.Time `json:"added_at" toml:"-"`` (tag changes from
  `json:"-"` because it is now exposed — see §3; `toml:"-"` stays
  because the seed entry struct does the TOML parsing — see §2).
- `api/pkg/atlas/atlas.go:81-84`: rewrite the doc comment to describe
  `AddedAt` as the populated, wire-exposed recency field sourced from
  `orgs.toml`'s `added_at`.
- All sort sites switch to `AddedAt`:
  `MemStore.ListRecent` (`memstore.go:297-298`) and
  `MemStore.OrgsForRegions` (per the existing field comment; verify and
  update any `.CreatedAt` usage found by `grep -rn CreatedAt api/`).
- Sort shape is unchanged: `AddedAt DESC, ID DESC`. The `ID DESC`
  tiebreaker resolves same-day ties deterministically (common, since
  backfill yields ~4 distinct dates).

### 2. TOML schema + loader (required field)

- Each `[[org]]` gains a date-only `added_at`, e.g. `added_at = 2026-05-21`.
- The seed-entry struct in `api/internal/seedfiles/` gains an
  `AddedAt toml.LocalDate` field (go-toml/v2 native local date).
- The loader (`api/internal/seedfiles/build.go`) converts the
  `toml.LocalDate` to `time.Time` at midnight UTC and assigns it to
  `Org.AddedAt` before `store.AddOrg`.
- **Required, fail-loud:** if any org's `added_at` is missing/zero, boot
  fails with an error naming the offending slug. No fallback default.
  This is the guarantee that the strip can never silently regress.
- `api/seed/README.md` documents `added_at` as required and states the
  convention (slice date for seed orgs; approval date for submissions).

### 3. Wire contract (exposed on the API)

- `api/openapi.yaml`: the `Org` schema gains
  `added_at: { type: string, format: date }`, added to the schema's
  `required` list.
- Spec edit lands in **its own commit**, ahead of the feature commit,
  per the repo's wire-contract rule.
- Regenerate both sides:
  - Go: `just api-gen` → `api/internal/httpapi/oapi/types.gen.go`
    (oapi-codegen emits `openapi_types.Date` for `format: date`); keep
    the embedded `api/internal/httpapi/openapi.yaml` copy in sync so
    `just api-check` passes.
  - TS: `npm run generate:api` → `web/src/lib/api.gen.ts`
    (`added_at: string`).
- `toOAPIOrgs` (`api/internal/httpapi/`) maps `Org.AddedAt time.Time` →
  `openapi_types.Date`.

### 4. Backfill methodology (the 202 existing orgs)

A one-time, auditable pass. Date assignment, in priority order:

1. **Per-org inline date.** If an org's own comment cites a specific
   add/verification date, prefer it. (Most orgs have only a slice-level
   date; this rule covers the few with explicit per-org dates.)
2. **Below a `=== Slice 7.8 … ===` marker** → that section's date:
   - "top-31–50 US metros" section → `2026-05-22`
   - "Non-top-50 US metro depth" section → `2026-05-22`
   - "CA CMAs #6–10" section → `2026-05-23` (the Ottawa/STAR notes in
     that block are dated 05-23)
3. **In the pre-7.8 block**, split by git blame against the two clean
   commits:
   - lines from `402db80` ("Org seed growth: +88 net-new" — blame
     attributes exactly 23 org lines, matching the inline "Slice #7.7
     (2026-05-21) added 23 net-new orgs" comment) → **slice 7.7 →
     `2026-05-21`**
   - lines from `7693e07` (founding region-graph seed — 19 org lines) →
     **initial seed → `2026-05-17`**

   Git blame is used **only** to draw the 7.6/7.7 section boundary the
   inline comments do not. It is not a per-org precision source: the
   later `bf3d8ae` (postal coverage) and `a5a07776` (post-launch)
   commits reordered the file, so their blame dates are confounded and
   are ignored in favor of the inline slice structure.

Result: ~4 distinct dates (`2026-05-17`, `2026-05-21`, `2026-05-22`,
`2026-05-23`). The pass produces a **reviewable mapping table**
(slug → date → source rule) committed alongside the backfill so the
reasoning is auditable, then writes `added_at` into every `[[org]]` in
`orgs.toml`.

PT seed files (`api/seed/regions_pt.toml` companions /
`orgs` entries loaded by the integration suite) also receive `added_at`
using their own slice dates, since the integration suite loads them and
the required-field rule applies uniformly.

### 5. Submission worker (going forward)

- `api/internal/githubpr/` `RenderOrgBlock` (`toml.go`) emits
  `added_at = <approval date>` in the appended `[[org]]` block.
- The admin approval handler passes the approval date (date-only, server
  clock) into the worker. Approval date is the chosen "added" semantic:
  the moment of the editorial decision. (The org goes live only on PR
  merge + next deploy, but approval date avoids merge-time write-back and
  is the cleanest stamp.)
- Every new org is therefore always dated and always satisfies the
  required-field rule.

### 6. Frontend display

- `web/src/routes/Home.tsx` "Recently indexed" strip and the org detail
  page render `added_at` — a small "Added May 2026" / "Added May 21,
  2026" line styled to the broadsheet language. Exact wording/format is
  a minor detail to match the *Urbanist Lexicon* design.
- No query changes: `listRecent` already drives the strip; it now sorts
  on real dates.

## Testing

- **Loader required-field:** table test — an org with missing/zero
  `added_at` makes FileStore boot return an error naming the slug.
- **Loader happy path:** `added_at` parses from TOML local date and
  lands on `Org.AddedAt` at midnight UTC.
- **Ordering:** `ListRecent` with mixed dates returns newest-first;
  same-day orgs fall back to `ID DESC` deterministically.
- **Wire mapping:** `toOAPIOrgs` emits the correct `format: date` value.
- **No-regression invariant:** after loading the full seed bundle, no
  org has a zero `AddedAt` (asserts the backfill is complete).
- **Submission render:** golden test for `RenderOrgBlock` includes the
  `added_at = <approval date>` line.

## Scope guard

- No changes to rate-limiting, auth, region-graph, or postal-code paths.
- No new dependencies (`go-toml/v2`, `oapi-codegen`, `openapi-typescript`
  are all already in use).
- The rename is mechanical and total: zero `CreatedAt` references remain
  after this work (`grep -rn CreatedAt` returns nothing).

## Affected files (non-exhaustive)

- `api/pkg/atlas/atlas.go` — field rename + comment
- `api/pkg/atlas/memstore.go` — sort sites
- `api/internal/seedfiles/build.go` (+ entry struct) — TOML field, parse, required check
- `api/openapi.yaml` (+ embedded copy) — `Org.added_at`
- `api/internal/httpapi/oapi/types.gen.go` — regenerated
- `api/internal/httpapi/*.go` — `toOAPIOrgs` mapping
- `api/internal/githubpr/toml.go` (+ approval handler) — stamp approval date
- `api/seed/orgs.toml` — backfill all 202 entries
- `api/seed/postal*/PT seed orgs` — backfill PT entries
- `api/seed/README.md` — document the field
- `web/src/lib/api.gen.ts` — regenerated
- `web/src/routes/Home.tsx` (+ detail page) — render `added_at`
- backfill mapping table (new committed artifact)
