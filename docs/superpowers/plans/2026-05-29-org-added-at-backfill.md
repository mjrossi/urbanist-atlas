# Backfill: `Org.AddedAt` for seed org files (2026-05-29)

One-time, auditable data backfill — **Phase 3** of the approved
`org-added-at` implementation plan. The Go loader already parses a
date-only `added_at` TOML field into `Org.AddedAt` (committed in
`5d8a43f`). This phase writes the `added_at` value into every
`[[org]]` block of `api/seed/orgs.toml` (202 orgs) and
`api/seed/orgs_pt.toml` (4 PT validation-fixture orgs).

The value is an **unquoted ISO date** (`added_at = 2026-05-21`), as
required by go-toml/v2 `toml.LocalDate`. It is inserted on its own
line immediately after each top-level `slug = "..."` line. The diff
is insertions-only (202 / 4 additions, 0 deletions); the rest of each
file is preserved byte-for-byte.

## Methodology and the slice-7.8 / `5e3f8c25` discovery

The plan specified deriving each org's add-date from `git blame` on
the `slug` line, mapping the blame hash to a slice date:

- `7693e07` founding region-graph seed -> **2026-05-17**
- `402db80` slice 7.7 -> **2026-05-21**
- `5e3f8c25` slice 7.8 -> **2026-05-22** (US) / **2026-05-23** (CA,
  at/below the `# === Slice 7.8 — CA CMAs #6–10 ===` marker)
- any other (confounded reorder) hash -> dated by **section
  position** (pre-7.8 -> -21; at/below first 7.8 marker -> -22;
  at/below CA marker -> -23).

**Discovery while executing.** At the current `HEAD`, naive
`git blame -s` (and even `git blame -M -C -C -C`) attributes the 202
`slug` lines almost entirely to the *later* same-day (2026-05-27)
reorder commits — `bf3d8ae` (postal coverage) and `a5a07776`
(post-launch round) — because each of those large commits
rewrote/moved org blocks wholesale. Blame no longer isolates the
founding / 7.7 / 7.8 add events, so applying the literal blame-hash
rule would mis-date most orgs and contradict the plan's own
spot-checks (e.g. `transportation-alternatives` must be 2026-05-17).

**Resolution (deviation from the literal instruction, faithful to
its intent).** The per-org *origin* commit is recovered with the
content pickaxe `git log -S 'slug = "<slug>"' -- api/seed/orgs.toml`,
taking the **earliest** (oldest) commit that introduced the slug
string. The plan's hash->date rules are then applied to that
pickaxe-derived origin hash; the plan's **section-position fallback**
(rule 4) is used verbatim for the two confounded reorder commits
`a5a07776` and `bf3d8ae`. Those reorders are the earliest *surviving*
introduction for the 7.8-era rows whose original slice-7.8 commit was
squashed/rewritten by the architecture-cutover history, so the
section-position bands stand in for the lost 7.8 hash. Origin-hash —
not raw line position — is the primary signal for the clean commits
`7693e07` and `402db80`; confounded reorder hashes fall back to
section position. This recovers the true per-slice add dates and
satisfies all of the plan's spot-checks.

Pickaxe earliest-introducing-commit distribution across the 202 orgs:

| earliest commit | committer date | meaning | orgs | assigned via |
| --- | --- | --- | --- | --- |
| `7693e07` | 2026-05-17 | founding region-graph seed (clean) | 19 | hash rule -> 2026-05-17 |
| `402db80` | 2026-05-27 | slice 7.7 org growth (clean) | 23 | hash rule -> 2026-05-21 |
| `a5a07776` | 2026-05-27 | post-launch feature round (confounded reorder) | 73 | section position -> -22/-23 |
| `bf3d8ae` | 2026-05-27 | postal coverage (confounded reorder) | 87 | section position -> -21/-22/-23 |
| | | **total** | **202** | |

The 19 `7693e07` orgs map directly to 2026-05-17 and the 23 `402db80`
orgs to 2026-05-21 by hash rule. The 160 orgs whose earliest
surviving introduction is a confounded reorder (`bf3d8ae`=87,
`a5a07776`=73) are dated by section position: those above the first
7.8 marker (line 750) -> 2026-05-21, between the first 7.8 marker and
the CA-CMAs marker (line 2111) -> 2026-05-22, and at/below the
CA-CMAs marker -> 2026-05-23.

There are **no** per-org inline `added on YYYY-MM-DD` comments that
differ from these slice dates. The dated comments present in
`orgs.toml` are gap-research / verification / removal notes, not
add-dates, so no per-org rule-1-style overrides apply.

### Section markers found in `api/seed/orgs.toml`

(line numbers in the committed `HEAD:api/seed/orgs.toml`, pre-insertion)

| marker | line |
| --- | --- |
| `# === Slice 7.8 — top-31–50 US metros + CA CMAs #6–10 ===` (first 7.8) | 750 |
| `# === Slice 7.8 — Non-top-50 US metro depth (city-leaf canvas) ===` | 1253 |
| `# === Slice 7.8 — CA CMAs #6–10 ===` (CA boundary) | 2111 |

## Summary — orgs.toml (202 orgs)

### Count per `added_at` date

| added_at | orgs |
| --- | --- |
| 2026-05-17 | 19 |
| 2026-05-21 | 56 |
| 2026-05-22 | 113 |
| 2026-05-23 | 14 |
| **total** | **202** |

### Count per source rule

| source rule | orgs |
| --- | --- |
| `blame:founding-seed(7693e07)` | 19 |
| `blame:slice-7.7(402db80)` | 23 |
| `position:default-2026-05-21` | 33 |
| `position:default-2026-05-22` | 113 |
| `position:default-2026-05-23` | 14 |
| **total** | **202** |

Expanded with the confounded blame hash carried in each `position:*`
row's `source` column (as it appears in the full table below):

| source (full) | orgs |
| --- | --- |
| `blame:founding-seed(7693e07)` | 19 |
| `blame:slice-7.7(402db80)` | 23 |
| `position:default-2026-05-21(blame bf3d8ae)` | 33 |
| `position:default-2026-05-22(blame a5a0777)` | 65 |
| `position:default-2026-05-22(blame bf3d8ae)` | 48 |
| `position:default-2026-05-23(blame a5a0777)` | 8 |
| `position:default-2026-05-23(blame bf3d8ae)` | 6 |
| **total** | **202** |

## Summary — orgs_pt.toml (4 PT validation-fixture orgs)

All four Portugal orgs were part of the founding region-graph seed
(slice #4.6, commit `7693e07`, 2026-05-17), so all receive
**2026-05-17**.

| slug | added_at | source |
| --- | --- | --- |
| `mubi-lisboa` | 2026-05-17 | `founding-seed-pt(slice#4.6, 2026-05-17)` |
| `mubi-porto` | 2026-05-17 | `founding-seed-pt(slice#4.6, 2026-05-17)` |
| `lisboa-para-pessoas` | 2026-05-17 | `founding-seed-pt(slice#4.6, 2026-05-17)` |
| `mubi-nacional` | 2026-05-17 | `founding-seed-pt(slice#4.6, 2026-05-17)` |

## Full mapping — orgs.toml (202 rows, file order)

| slug | added_at | source |
| --- | --- | --- |
| `transportation-alternatives` | 2026-05-17 | `blame:founding-seed(7693e07)` |
| `riders-alliance` | 2026-05-17 | `blame:founding-seed(7693e07)` |
| `streetspac` | 2026-05-17 | `blame:founding-seed(7693e07)` |
| `transitcenter` | 2026-05-17 | `blame:founding-seed(7693e07)` |
| `tri-state-transportation-campaign` | 2026-05-17 | `blame:founding-seed(7693e07)` |
| `sf-transit-riders` | 2026-05-17 | `blame:founding-seed(7693e07)` |
| `walk-sf` | 2026-05-17 | `blame:founding-seed(7693e07)` |
| `sf-bicycle-coalition` | 2026-05-21 | `blame:slice-7.7(402db80)` |
| `bike-east-bay` | 2026-05-21 | `blame:slice-7.7(402db80)` |
| `transitmatters` | 2026-05-17 | `blame:founding-seed(7693e07)` |
| `livablestreets-alliance` | 2026-05-17 | `blame:founding-seed(7693e07)` |
| `boston-cyclists-union` | 2026-05-21 | `blame:slice-7.7(402db80)` |
| `a-better-city` | 2026-05-21 | `blame:slice-7.7(402db80)` |
| `mbta-advisory-board` | 2026-05-21 | `blame:slice-7.7(402db80)` |
| `transit-alliance-miami` | 2026-05-17 | `blame:founding-seed(7693e07)` |
| `bike-walk-coral-gables` | 2026-05-21 | `blame:slice-7.7(402db80)` |
| `seattle-subway` | 2026-05-17 | `blame:founding-seed(7693e07)` |
| `seattle-streets-alliance` | 2026-05-21 | `blame:slice-7.7(402db80)` |
| `transit-riders-union` | 2026-05-21 | `blame:slice-7.7(402db80)` |
| `streets-for-all` | 2026-05-17 | `blame:founding-seed(7693e07)` |
| `streets-are-for-everyone` | 2026-05-21 | `blame:slice-7.7(402db80)` |
| `act-la` | 2026-05-21 | `blame:slice-7.7(402db80)` |
| `active-transportation-alliance` | 2026-05-17 | `blame:founding-seed(7693e07)` |
| `better-streets-chicago` | 2026-05-17 | `blame:founding-seed(7693e07)` |
| `commuters-take-action` | 2026-05-21 | `blame:slice-7.7(402db80)` |
| `chicago-bike-grid-now` | 2026-05-21 | `blame:slice-7.7(402db80)` |
| `bikemore` | 2026-05-21 | `position:default-2026-05-21(blame bf3d8ae)` |
| `central-maryland-transportation-alliance` | 2026-05-21 | `position:default-2026-05-21(blame bf3d8ae)` |
| `sustain-charlotte` | 2026-05-21 | `position:default-2026-05-21(blame bf3d8ae)` |
| `bike-walk-central-florida` | 2026-05-21 | `position:default-2026-05-21(blame bf3d8ae)` |
| `street-trust` | 2026-05-21 | `position:default-2026-05-21(blame bf3d8ae)` |
| `bikeloud-pdx` | 2026-05-21 | `position:default-2026-05-21(blame bf3d8ae)` |
| `opal-environmental-justice` | 2026-05-21 | `position:default-2026-05-21(blame bf3d8ae)` |
| `bike-dfw` | 2026-05-21 | `position:default-2026-05-21(blame bf3d8ae)` |
| `dallas-bicycle-coalition` | 2026-05-21 | `position:default-2026-05-21(blame bf3d8ae)` |
| `dallas-area-transit-alliance` | 2026-05-21 | `blame:slice-7.7(402db80)` |
| `bikehouston` | 2026-05-21 | `position:default-2026-05-21(blame bf3d8ae)` |
| `link-houston` | 2026-05-21 | `position:default-2026-05-21(blame bf3d8ae)` |
| `a-tale-of-two-bridges` | 2026-05-21 | `blame:slice-7.7(402db80)` |
| `waba` | 2026-05-21 | `position:default-2026-05-21(blame bf3d8ae)` |
| `coalition-for-smarter-growth` | 2026-05-21 | `position:default-2026-05-21(blame bf3d8ae)` |
| `greater-greater-washington` | 2026-05-21 | `position:default-2026-05-21(blame bf3d8ae)` |
| `bicycle-coalition-greater-philadelphia` | 2026-05-21 | `position:default-2026-05-21(blame bf3d8ae)` |
| `5th-square` | 2026-05-21 | `position:default-2026-05-21(blame bf3d8ae)` |
| `clean-air-council` | 2026-05-21 | `blame:slice-7.7(402db80)` |
| `propel-atl` | 2026-05-21 | `position:default-2026-05-21(blame bf3d8ae)` |
| `marta-army` | 2026-05-21 | `position:default-2026-05-21(blame bf3d8ae)` |
| `beltline-rail-now` | 2026-05-21 | `blame:slice-7.7(402db80)` |
| `phoenix-spokes-people` | 2026-05-21 | `position:default-2026-05-21(blame bf3d8ae)` |
| `friends-of-transit` | 2026-05-21 | `position:default-2026-05-21(blame bf3d8ae)` |
| `urban-phoenix-project` | 2026-05-21 | `blame:slice-7.7(402db80)` |
| `tempe-bicycle-action-group` | 2026-05-21 | `blame:slice-7.7(402db80)` |
| `inland-empire-biking-alliance` | 2026-05-21 | `position:default-2026-05-21(blame bf3d8ae)` |
| `friends-of-cv-link` | 2026-05-21 | `blame:slice-7.7(402db80)` |
| `transportation-riders-united` | 2026-05-21 | `position:default-2026-05-21(blame bf3d8ae)` |
| `detroit-greenways-coalition` | 2026-05-21 | `position:default-2026-05-21(blame bf3d8ae)` |
| `detroit-disability-power` | 2026-05-21 | `blame:slice-7.7(402db80)` |
| `move-minnesota` | 2026-05-21 | `position:default-2026-05-21(blame bf3d8ae)` |
| `our-streets-minneapolis` | 2026-05-21 | `position:default-2026-05-21(blame bf3d8ae)` |
| `streets-mn` | 2026-05-21 | `blame:slice-7.7(402db80)` |
| `circulate-san-diego` | 2026-05-21 | `position:default-2026-05-21(blame bf3d8ae)` |
| `san-diego-county-bike-coalition` | 2026-05-21 | `position:default-2026-05-21(blame bf3d8ae)` |
| `bikesd` | 2026-05-21 | `position:default-2026-05-21(blame bf3d8ae)` |
| `walk-bike-tampa` | 2026-05-21 | `position:default-2026-05-21(blame bf3d8ae)` |
| `denver-streets-partnership` | 2026-05-21 | `position:default-2026-05-21(blame bf3d8ae)` |
| `greater-denver-transit` | 2026-05-21 | `position:default-2026-05-21(blame bf3d8ae)` |
| `trailnet` | 2026-05-21 | `position:default-2026-05-21(blame bf3d8ae)` |
| `citizens-for-modern-transit` | 2026-05-21 | `position:default-2026-05-21(blame bf3d8ae)` |
| `paraquad` | 2026-05-21 | `blame:slice-7.7(402db80)` |
| `bikepgh` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `pittsburghers-for-public-transit` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `pittsburgh-walks` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `sacramento-area-bicycle-advocates` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `civic-thread` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `sacramento-advocates-for-rail-and-transit` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `southern-nevada-bicycle-coalition` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `better-bus-coalition` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `queen-city-bike` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `devou-good-foundation` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `bikewalkkc` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `kc-regional-transit-alliance` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `safe-streets-austin` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `transit-forward` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `aura-austin` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `yay-bikes` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `transit-columbus` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `central-indiana-cycling` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `health-by-design` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `bike-cleveland` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `clevelanders-for-public-transit` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `walk-bike-nashville` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `transit-now-nashville` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `transit-alliance-middle-tennessee` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `bike-norfolk` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `providence-streets-coalition` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `grow-smart-ri` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `better-streets-mke` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `oaks-and-spokes` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `bike-durham` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `innovate-memphis` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `micah-memphis` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `rva-rapid-transit` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `bike-walk-rva` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `streets-for-people-louisville` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `ride-new-orleans` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `sweet-streets` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `utah-transit-riders-union` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `bike-west-hartford` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `gobike-buffalo` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `citizens-for-regional-transit` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `madison-bikes` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `boise-bicycle-project` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `bike-anchorage` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `walk-bike-washtenaw` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `bicycle-alliance-of-washtenaw` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `community-cycles` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `new-haven-safe-streets-coalition` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `ncat-new-haven` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `living-streets-alliance` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `albany-bicycle-coalition` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `spokane-active-transportation` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `capital-city-cyclists` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `charleston-moves` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `greater-grand-rapids-bicycle-coalition` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `fresno-county-bicycle-coalition` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `bike-walk-connecticut` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `bicycle-coalition-of-maine` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `massbike` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `walk-massachusetts` | 2026-05-21 | `blame:slice-7.7(402db80)` |
| `bike-walk-alliance-nh` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `new-jersey-bike-walk-coalition` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `new-york-bicycling-coalition` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `reinvent-albany` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `nypirg` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `transit-for-all-pa` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `rhode-island-bicycle-coalition` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `local-motion` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `bike-delaware` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `bike-maryland` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `virginia-bicycling-federation` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `bikewalk-nc` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `palmetto-walk-bike` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `georgia-bikes` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `florida-bicycle-association` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `1000-friends-of-florida` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `bike-walk-kentucky` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `bike-walk-tennessee` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `alabike` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `bike-walk-mississippi` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `bike-easy` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `biketexas` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `texas-streets-coalition` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `texas-pedestrian-safety-coalition` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `ride-illinois` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `bicycle-indiana` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `league-of-michigan-bicyclists` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `trans4m` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `all-aboard-ohio` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `ohio-bicycle-federation` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `wisconsin-bike-fed` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `iowa-bicycle-coalition` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `bike-minnesota` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `mobikefed` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `bike-walk-nebraska` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `alaska-mobility-coalition` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `coalition-of-arizona-bicyclists` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `calbike` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `california-walks` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `transform-ca` | 2026-05-22 | `position:default-2026-05-22(blame a5a0777)` |
| `bicycle-colorado` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `hawaii-bicycling-league` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `idaho-walk-bike-alliance` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `bike-walk-montana` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `bike-abq` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `bike-utah` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `washington-bikes` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `ttcriders` | 2026-05-17 | `blame:founding-seed(7693e07)` |
| `walk-toronto` | 2026-05-17 | `blame:founding-seed(7693e07)` |
| `hub-cycling` | 2026-05-17 | `blame:founding-seed(7693e07)` |
| `movement-metro-vancouver` | 2026-05-17 | `blame:founding-seed(7693e07)` |
| `centre-ecologie-urbaine-montreal` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `coalition-mobilite-active-montreal` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `cre-montreal` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `bike-calgary` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `calgary-climate-hub` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `bike-ottawa` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `ottawa-transit-riders` | 2026-05-22 | `position:default-2026-05-22(blame bf3d8ae)` |
| `paths-for-people` | 2026-05-23 | `position:default-2026-05-23(blame a5a0777)` |
| `bike-edmonton` | 2026-05-23 | `position:default-2026-05-23(blame a5a0777)` |
| `acces-transports-viables` | 2026-05-23 | `position:default-2026-05-23(blame a5a0777)` |
| `cycle-hamilton` | 2026-05-23 | `position:default-2026-05-23(blame a5a0777)` |
| `cyclewr` | 2026-05-23 | `position:default-2026-05-23(blame a5a0777)` |
| `tritag` | 2026-05-23 | `position:default-2026-05-23(blame a5a0777)` |
| `halifax-cycling-coalition` | 2026-05-23 | `position:default-2026-05-23(blame a5a0777)` |
| `mississauga-cycling-now` | 2026-05-23 | `position:default-2026-05-23(blame a5a0777)` |
| `trajectoire-quebec` | 2026-05-17 | `blame:founding-seed(7693e07)` |
| `share-the-road-cycling-coalition` | 2026-05-23 | `position:default-2026-05-23(blame bf3d8ae)` |
| `bc-cycling-coalition` | 2026-05-23 | `position:default-2026-05-23(blame bf3d8ae)` |
| `alberta-cycling-coalition` | 2026-05-23 | `position:default-2026-05-23(blame bf3d8ae)` |
| `bike-winnipeg` | 2026-05-23 | `position:default-2026-05-23(blame bf3d8ae)` |
| `bicycle-nova-scotia` | 2026-05-23 | `position:default-2026-05-23(blame bf3d8ae)` |
| `bicycle-newfoundland-and-labrador` | 2026-05-23 | `position:default-2026-05-23(blame bf3d8ae)` |
