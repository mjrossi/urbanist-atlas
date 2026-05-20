# Slice 7.6 — Org Seed Growth Design

## Context

The Urbanist Atlas API currently ships with 23 hand-curated orgs (19
US/CA + 4 PT) in `api/seed/orgs.toml`, loaded by
`api/internal/seed/orgs.go`. The roadmap entry for slice 7.6 in
`docs/roadmap.md` calls for "Editorial work, not engineering" to expand
the dataset toward 30–50 orgs across the supported countries before
Phase 1 dogfooding begins.

This design extends slice 7.6 beyond the original count target. The
driving editorial decisions:

1. **No ZIP should return empty.** Every US state + every CA province
   gets ≥1 org (or a documented gap), so the recursive-CTE ancestor
   walk in `api/internal/store/postgres/queries/lookup.sql` always
   surfaces something for any postal code.
2. **Top-30 metro depth.** The 25 largest US CBSAs + 5 largest CA CMAs
   each get ≥1 metro-anchored org for richer launch signal where most
   users live.
3. **Coverage gate, not vibe.** State floor and metro gate are two
   independent coverage requirements with explicit fallback rules and
   an empty-state policy.

The slice stays editorial — no engineering changes, no schema
migrations, no loader changes. `seed.LoadFile` already upserts orgs
keyed on `region_slugs`; this slice fills the table.

## Two coverage gates

| Gate | Floor | Anchor |
|---|---|---|
| **State / province floor** | ≥1 org per US state + ≥1 per CA province | Org attached at the state/province region directly |
| **Top-30 metro gate** | ≥1 org per metro in the 25 US + 5 CA canvas | Org at the metro/MSA/CMA level, or city leaf if municipal |

- **Per-metro depth:** 1–3 new orgs per metro is the typical target;
  no hard cap. Existing metros (NYC at 5, others at 1–2) keep their
  current orgs and may grow further if quality candidates surface
  during research.
- **Per-state depth:** typically 1 statewide org; depth comes from
  metro entries underneath, not from stacking multiple statewide
  orgs.
- **PT held at 4** — validation fixture, not a launch-coverage
  country. A separate future slice handles PT public-surface
  visibility if/when that becomes a concern.

## Activity bar

Any one of — site updated / blog or news post / social post / event —
within the last 12 months as of the verification date. Maintainer
spot-checks each candidate during the review pass.

## Empty-state policy

If a state/province has no demonstrably-active statewide org, leave
the slot empty and add a
`# gap: no active statewide org as of YYYY-MM-DD` comment in
`orgs.toml` at the appropriate position. ZIPs in those regions
return `200 OK` with empty `local` and `regional` arrays — graceful
empty, not error.

**Likely-gap states (preliminary, subject to research):** WY, MT, ND,
SD, MS, AK; CA territories YT, NT, NU; possibly PR.

## Metro fallback

If a top-30 metro has no quality local org, the state-level org
satisfies the gate via ancestor walk for ZIPs in that metro. If even
state-level coverage would be empty, swap the metro for the
next-ranked metro with organizers and note the swap in the spec
under "Canvas adjustments."

## Top-30 metro canvas

**US — top 25 CBSAs by 2020 census** (existing coverage marked ✓;
slugs verified against `api/seed/regions_us_msas.toml` and
`api/seed/regions_us_msa_overrides.toml`):

```
 1. NYC                  nyc / nyc-metro / nyc-tristate     ✓ (5 orgs)
 2. Los Angeles          greater-la / los-angeles           ✓ (1 org)
 3. Chicago              chicago / chicago-metro            ✓ (2 orgs)
 4. Dallas-Fort Worth    dallas-tx-metro                    net-new
 5. Houston              houston-tx-metro                   net-new
 6. Washington DC        washington-dc / washington-dc-metro net-new
 7. Philadelphia         philadelphia-pa-metro              net-new
 8. Atlanta              atlanta-ga-metro                   net-new
 9. Miami                miami / greater-miami              ✓ (1 org)
10. Phoenix              phoenix-az-metro                   net-new
11. Boston               greater-boston / boston            ✓ (2 orgs)
12. SF Bay               sf / sf-bay-area                   ✓ (2 orgs)
13. Riverside-SB         riverside-ca-metro                 net-new
14. Detroit              detroit-mi-metro                   net-new
15. Seattle              seattle-metro / seattle            ✓ (1 org)
16. Minneapolis-St. Paul minneapolis-mn-metro               net-new
17. San Diego            san-diego-ca-metro                 net-new
18. Tampa                tampa-fl-metro                     net-new
19. Denver               denver-co-metro                    net-new
20. St. Louis            st-louis-mo-metro (MO+IL)          net-new
21. Baltimore            baltimore-md-metro                 net-new
22. Charlotte            charlotte-nc-metro (NC+SC)         net-new
23. Orlando              orlando-fl-metro                   net-new
24. San Antonio          san-antonio-tx-metro               net-new
25. Portland OR          portland-or-metro (OR+WA)          net-new
```

**CA — top 5 CMAs by 2021 census:**

```
 1. Toronto              toronto / toronto-cma              ✓ (2 orgs)
 2. Montréal             montreal / montreal-cma            net-new
 3. Vancouver            vancouver / metro-vancouver        ✓ (2 orgs)
 4. Calgary              calgary-cma                        net-new
 5. Ottawa-Gatineau      ottawa-gatineau-cma                net-new
```

**Net-new metro orgs (floor of 1 each):** 18 US + 3 CA = **21**.

## State/province floor

**US:** all 50 states + PR each need ≥1 statewide-anchored org. DC's
coverage is satisfied by multi-anchoring two of the three
`washington-dc-metro` orgs (Coalition for Smarter Growth, Greater
Greater Washington) at both the metro and the `dc` region. DC ZIPs
anchor at the `washington-dc` city leaf; the ancestor walk reaches
`washington-dc-metro` and `dc` at depth 1, then surfaces the metro's
other state parents (`va`, `md`, `wv`) at depth 2 — this is the
intended DC-metro graph behavior (the MSA genuinely spans four
jurisdictions, so Arlington/Bethesda ZIPs need MD/VA state-floor
content). Putting DC-region orgs at both anchors keeps DC content in
front of MD/VA content in the breadcrumb.

**CA:** all 10 provinces need ≥1 org; QC already covered by Trajectoire
Québec. Territories YT/NT/NU expected to land as documented gaps.

**Net-new state/province orgs:** ~51 US + 12 CA = **~63**, of which
~3–8 expected to end as `# gap` comments rather than seeded entries.
(Closing the slice: 13 gaps landed — 9 US (WV, AR, OK, KS, ND, SD, NV,
WY, PR) and 4 CA blocks (PE, NL-province, NB, plus YT/NT/NU
consolidated). Research surfaced fewer demonstrably-active statewide
safe-streets nonprofits than the upper-bound estimate anticipated,
especially in low-density states.)

## Total scope estimate

```
Existing orgs:                        23
+ Net-new metro orgs (floor):         21
+ Net-new state/province orgs:        ~55–60
+ Optional existing-metro top-ups:    0–15
────────────────────────────────────────
Net-new total:                        ~75–95
Dataset after slice 7.6:              ~100–120 orgs
```

**Roadmap line update:** `docs/roadmap.md` slice 7.6 entry changes
from "the planned ~30–50 across the supported countries" to "the
planned ~100–120 (universal state/province floor + top-30 metro
gate) across the supported countries."

## Per-org data shape

Each new entry in `api/seed/orgs.toml`:

```toml
[[org]]
slug = "kebab-case-stable-id"
name = "Display Name"
short_desc = "One-sentence description (~150 chars)."
website_url = "https://..."           # required, verified to resolve
contact_url = "https://..."           # optional deep-link
tags = ["transit", "advocacy", ...]   # open vocabulary
region_slugs = ["one-anchor-slug"]    # almost always exactly one
```

- **Slug:** stable, kebab-case, no spaces, no country prefix.
- **`region_slugs`:** one slug per org typically. Multiple only when
  an org genuinely has multiple equally-primary scopes (rare).
- **Region anchoring rule:**
  - **State/province** if statewide (e.g., All Aboard Ohio → `oh`).
  - **Metro / MSA / CMA** if explicitly metropolitan in scope.
  - **City leaf** if truly municipal (e.g., DC-only org →
    `washington-dc`).
  - **Multi-state region** if the org is a federation across states
    (e.g., `nyc-tristate`).
- **Verification:** maintainer spot-checks each org during the
  Claude-assisted review pass. Global header date in `orgs.toml`
  updated to reflect the latest pass; per-org verification dates
  are not stored on the row.
- **ODbL attribution:** `website_url` is the source. No separate
  `source_url` field.
- **Tags:** open vocabulary (existing pattern). No taxonomy lockdown
  in this slice.

## Workflow

**Maintainer-led, Claude-assisted drafting:**

1. For each metro or state, Claude produces a research note in chat
   with 2–5 candidate orgs, each carrying:
   - Proposed `slug`, `name`, `short_desc`
   - `website_url` + `contact_url` (if available)
   - Suggested `tags`
   - Recommended `region_slugs`
   - **Evidence of last-12-months activity** (specific link or post)
2. Maintainer reviews the note, marks include/exclude, edits if
   needed.
3. Selected orgs are written to `api/seed/orgs.toml`.
4. Commit per region — one commit per metro (with its 1–3 orgs) or
   per state (with its 1 statewide org). Commit message format:
   `feat(seed): add <metro/state> orgs (slice 7.6)`.
5. Iterate until both coverage gates are satisfied.

**Suggested order of work:**

1. **Round 1 — easy CA + new US metro warm-up.** Montréal, Calgary,
   Ottawa-Gatineau + Baltimore, Charlotte, Orlando, San Antonio,
   Portland OR.
2. **Round 2 — top-25 US metro fill.** DFW, Houston, DC, Philly,
   Atlanta, Phoenix, Riverside-SB, Detroit, MSP, San Diego, Tampa,
   Denver, St. Louis.
3. **Round 3 — US state floor.** Walk through the 50 states + PR;
   most will be 1 org each. Gap-likely states processed last; gap
   entries written as comments.
4. **Round 4 — CA province floor.** ON, BC, AB, MB, SK, NS, NB, NL,
   PE + territory gap entries.
5. **Round 5 (optional) — existing-metro depth.** Add a 2nd or 3rd
   org for under-deep existing metros (LA at 1, Miami at 1, Seattle
   at 1 are likely candidates) only if a clearly-strong org surfaces
   during research.

**End of slice:**

- Update `orgs.toml` header comment with the latest verification date.
- Update `docs/roadmap.md` slice 7.6 line to reflect the new target.
- Run `just pg-reset && just pg-up && just loaddata` to confirm clean
  load.
- Smoke `/lookup` against ~10 representative ZIPs across covered
  states + gap states (see Verification below).

## Critical files

**Edited during this slice:**
- `api/seed/orgs.toml` — the actual editorial expansion.
- `docs/roadmap.md` — slice 7.6 target line update.

**Read-only references during research:**
- `api/seed/regions_us_states.toml` — canonical state slugs (52
  entries: 50 + DC + PR).
- `api/seed/regions_ca_provinces.toml` — canonical province/territory
  slugs (13 entries).
- `api/seed/regions_us_msas.toml` — auto-generated MSA slugs for
  verification when drafting metro orgs (393 entries).
- `api/seed/regions_us_msa_overrides.toml` — editorial-pinned slugs
  for the 7 known metros.
- `api/seed/regions_us.toml` — US city leaves (incl. `washington-dc`,
  `los-angeles`, `boston`, `seattle`, `sf`, `chicago`, NYC boroughs).
- `api/seed/regions_ca_cmas.toml` — CA CMA slugs (41 CMAs, incl.
  overrides `toronto-cma`, `montreal-cma`, `metro-vancouver`,
  `ottawa-gatineau-cma`, `calgary-cma`).
- `api/seed/regions_ca.toml` — CA city leaves (`toronto`, `montreal`,
  `vancouver`, `burnaby`, `richmond`).

**Reused, not modified:**
- `api/internal/seed/orgs.go` — `LoadFile` already handles upserts
  and FK resolution.
- `api/cmd/server/seed.go` — `runSeed` driver.
- `justfile` — `just loaddata`, `just pg-reset` recipes.

## Verification

**End-of-slice smoke test (manual):**

1. `just pg-reset && just pg-up && just loaddata` — confirm clean
   load with no errors and no FK rejections (every `region_slugs`
   entry resolves).
2. Hit `/api/v1/lookup` against ~10 representative ZIPs:
   - **Metro ZIPs:** 10001 (NYC), 20001 (DC), 30303 (Atlanta), 19103
     (Philly), 75201 (Dallas), 77002 (Houston), 33101 (Miami), 28202
     (Charlotte), 97201 (Portland OR), 90001 (LA) — confirm metro
     org returns.
   - **Rural state ZIPs:** 82001 (WY), 59601 (MT), 73501 (OK), 04101
     (ME), 67501 (KS) — confirm statewide org returns (or empty if
     state is documented gap).
   - **Gap-confirmed ZIPs:** if WY/MT/etc. land as documented gaps,
     confirm `/lookup` returns 200 with empty `local` and `regional`
     arrays (graceful empty, not error).
3. CA postal codes: `M5V` (Toronto), `H2X` (Montréal), `T2P` (Calgary),
   `K1P` (Ottawa), `V6B` (Vancouver) — confirm CMA org. Plus rural
   province FSAs (e.g., `A1A` NL, `S4P` SK) — confirm province org or
   documented gap.
4. Confirm `orgs.toml` header date reflects the verification pass.
5. Confirm `docs/roadmap.md` slice 7.6 entry updated.

**No new test code** for this slice — it's editorial. Existing
integration tests in `api/internal/seed/` and `api/internal/httpapi/`
cover the loader + lookup paths; they fail at `pg-reset` time if any
`region_slugs` entry doesn't resolve.

## Out of scope (captured for future slices)

- **Hide PT from public `/lookup` and `/browse` surfaces.** Separate
  future slice; engineering change (filter by country whitelist).
- **`/browse` endpoints' state-level rendering.** If `/browse/states`
  exists or gets added, its empty-state UI for gap states is its own
  slice.
- **Tag taxonomy lockdown.** Open vocabulary preserved; controlled
  taxonomy is a v1.1+ refinement.
- **Per-org `verified_at` field on the schema.** Global header
  comment is enough for now.
- **ES expansion** — slice 4.7 territory.
- **National-tier orgs for US/CA.** Forbidden by editorial policy in
  `docs/region-graph.md` §5.

## Canvas adjustments

(This section captures any metros swapped or states demoted to `# gap`
during the editorial pass.)

### Metro gate gaps (satisfied via state-floor fallback)

- **San Antonio (US #24)** — no metro-anchored advocacy org meets the
  activity bar as of 2026-05-20. Bike San Antonio's website is
  unreachable; San Antonio Mobility Coalition skews highway-funding
  (out of scope); Earn-A-Bike and San Antonio Wheelmen are
  shop/club rather than advocacy. Metro gate satisfied via the
  Texas state-floor org (BikeTexas) through ancestor walk for SA ZIPs.
  Worth revisiting if a maintainer with local knowledge surfaces an
  active SA advocacy group.

### State floor gaps (documented; ZIPs in these states will return empty)

- **West Virginia (WV)** — no demonstrably-active statewide ped/bike
  advocacy nonprofit. Existing WV bike orgs (Country Roads Cyclists,
  WVMBA, WVTrails) are recreational, mountain-bike, or trail-focused
  rather than safe-streets advocacy.
- **Arkansas (AR)** — Bike/Walk Arkansas was organized 2015 but
  stalled; no active statewide advocacy successor. ARDOT's
  Bicycle-Pedestrian Coordinator is reportedly trying to seed a new
  effort.
- **Oklahoma (OK)** — BikeOklahoma (Oklahoma Bicycling Coalition) is
  dissolving and transferring assets to the Oklahoma Bicycle Summit,
  Inc. No replacement statewide advocacy 501(c)(3) active as of
  2026-05-20.
- **Kansas (KS)** — no clean statewide advocacy entity. BikeWalkKC
  does some Kansas statewide work but is structurally a KC-metro
  org; Biking Across Kansas is an annual tour, not advocacy.
- **North Dakota (ND)** — no independent statewide ped/bike or
  transit advocacy nonprofit found.
- **South Dakota (SD)** — no independent statewide ped/bike or
  transit advocacy nonprofit found.
- **Nevada (NV)** — no clean statewide entity. Southern Nevada Bicycle
  Coalition exists but is Las-Vegas-area only.
- **Wyoming (WY)** — no statewide ped/bike or transit advocacy
  nonprofit. Local advocates exist (Jackson Hole Community Pathway
  System) but no statewide 501(c)(3).
- **Puerto Rico (PR)** — Puerto Rico Bicycle Coalition has a Facebook
  presence but no clear website; recent activity not verifiable to
  the 12-month bar from search alone. Worth a Facebook-side check.

### Multi-anchored orgs (cover both metro + state in one entry)

- **The Street Trust** → `[portland-or-metro, or]`. Conducts statewide
  legislative work alongside Portland-metro programs; multi-anchored
  to satisfy both the Portland metro gate and the Oregon state floor
  without a duplicate entry.

### CA province floor gaps

- **Saskatchewan (SK)** — Saskatchewan Cycling Association is sport-
  focused (Cycling Canada provincial branch), not safe-streets
  advocacy.
- **New Brunswick (NB)** — no provincial advocacy nonprofit found.
- **Prince Edward Island (PE)** — Cycling PEI is sport-focused;
  Charlottetown has a 2025 active-transportation plan but no
  independent provincial advocacy nonprofit.
- **Yukon (YT) / Northwest Territories (NT) / Nunavut (NU)** — no
  territorial-tier ped/bike or transit advocacy nonprofits found.
  Expected given population density and advocacy ecosystem in the
  Canadian North.
