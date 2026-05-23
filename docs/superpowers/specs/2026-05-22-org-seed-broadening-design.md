# Slice 7.8 — Org Seed Broadening (Geographic Reach)

## Context

`api/seed/orgs.toml` shipped with **134 orgs** after slice 7.7
(2026-05-21), shaped by the slice 7.6 / 7.7 coverage gates: a
universal state/province floor of ≥1, a top-20 US metro / top-5 CA
CMA floor of ≥3, and a top-21–30 floor of ≥1. Those gates were
deliberate "v1 launch" floors and left whole swaths of the supported
geography under-seeded:

- **US #31–50 metros** — outside the canvas entirely.
- **CA CMAs #6–10** — outside the canvas.
- **Big-state floor stuck at ≥1.** CA, TX, NY, FL, IL, PA, OH, GA,
  NC, MI sat at the same ≥1 floor as WY/VT, despite each having
  multiple genuinely-distinct statewide orgs.
- **No city-leaf orgs outside the metro canvas.** State capitals,
  college towns, and isolated mid-sized cities had no local-tier
  entry; ZIPs there fell through to the state floor.

This slice broadens geographic reach to **203 orgs (+73 net-new)**
without changing what counts as an admissible org. The scope
precedents (public-agency oversight bodies, multi-issue 501(c)(3)s
with named programs, publication/advocacy hybrids, chapters-must-be-
independent) and the 12-month activity bar locked in by 7.6/7.7
stay intact.

Out of scope for this slice (explicit, to keep it shippable): refilling
the documented state/CA gaps (WV, AR, OK, KS, ND, SD, NV, WY, PR; PE,
SK, NB, YT/NT/NU), refilling top-20 third-org gaps (Miami, Inland
Empire, Tampa, Denver), and federation expansion (chicagoland,
rta-service-area). Each is a worthwhile follow-up but a different
research surface.

## Coverage gates (slice 7.8)

| Gate | Floor | Anchor | Source |
|---|---|---|---|
| **State / province floor** | ≥1 org per US state + CA province | State/province | slice 7.6 (unchanged) |
| **Big-state depth (US top-10 pop)** | ≥2 statewide orgs (≥3 for CA, NY, TX) where genuinely-distinct candidates exist | State | **slice 7.8** |
| **Top-20 US / top-5 CA metro depth** | ≥3 | Metro / city leaf | slice 7.7 (unchanged) |
| **Top-21–30 metro depth** | ≥1 | Metro | slice 7.6 (unchanged) |
| **Top-31–50 US metro gate** | ≥1 metro-anchored org | MSA | **slice 7.8** |
| **CA CMA #6–10 gate** | ≥1 CMA-anchored org | CMA | **slice 7.8** |
| **City-leaf canvas** | ≥1 city-leaf org per city on the canvas | MSA (city dominates) | **slice 7.8** |

The city-leaf canvas was implemented by anchoring orgs directly at
existing MSA slugs for prominent non-top-50 metros (Madison, Boise,
Anchorage, etc.) where the city dominates the MSA population, rather
than adding new city-leaf entries to `regions_us.toml`. No
region-tree changes were needed.

## Top-31–50 US metro results

| # | Metro | Orgs added | Notes |
|---|---|---|---|
| 26 | Pittsburgh | 3 | BikePGH, Pittsburghers for Public Transit, Pittsburgh Walks |
| 27 | Sacramento | 3 | SABA, Civic Thread (formerly WALKSacramento), STAR |
| 28 | Las Vegas | 1 | Southern Nevada Bicycle Coalition; PedSafe Vegas excluded (UNLV-housed program) |
| 29 | Cincinnati | 3 | Better Bus Coalition, Queen City Bike, Devou Good Foundation |
| 30 | Kansas City | 2 | BikeWalkKC, KC Regional Transit Alliance |
| 31 | Austin | 3 | Safe Streets Austin, Transit Forward, AURA |
| 32 | Columbus OH | 2 | Yay Bikes!, Transit Columbus |
| 33 | Indianapolis | 2 | Central Indiana Cycling (merger), Health by Design |
| 34 | Cleveland | 2 | Bike Cleveland, Clevelanders for Public Transit |
| 35 | Nashville | 3 | Walk Bike Nashville, Transit Now Nashville, Transit Alliance MTN |
| 36 | San Jose | — | Treated as SF Bay top-up (Round 5 deferred) |
| 37 | Virginia Beach | 1 | Bike Norfolk; HRPTA couldn't be verified |
| 38 | Providence | 2 | Providence Streets Coalition, Grow Smart RI |
| 39 | Jacksonville | gap | Bike Walk Jax pending 501(c)(3); FL state floor covers |
| 40 | Milwaukee | 1 | Better Streets MKE; Wisconsin Bike Fed covers via MilWALKee Walks |
| 41 | Oklahoma City | gap | OK is itself a state-floor gap; OBS recreational |
| 42 | Raleigh | 1 + 1 bonus | Oaks & Spokes (Raleigh) + Bike Durham (durham-nc-metro) |
| 43 | Memphis | 2 | Innovate Memphis, MICAH |
| 44 | Richmond | 2 | RVA Rapid Transit, Bike Walk RVA |
| 45 | Louisville | 1 | Streets for People (formerly Bicycling for Louisville) |
| 46 | New Orleans | 2 | Bike Easy, Ride New Orleans |
| 47 | Salt Lake City | 2 | Sweet Streets, Utah Transit Riders Union |
| 48 | Hartford | 1 | Bike West Hartford |
| 49 | Buffalo | 2 | GObike Buffalo, Citizens for Regional Transit |
| 50 | Birmingham | gap | No clean advocacy 501(c)(3); AlaBike covers via ancestor walk |

**Subtotal:** ~40 net-new metro orgs across 22 of the 25 metros.
3 documented gaps (Jacksonville, Oklahoma City, Birmingham).

## CA CMAs #6–10 results

| # | CMA | Orgs added |
|---|---|---|
| 6 | Edmonton | 2 (Paths for People, Bike Edmonton) |
| 7 | Québec City | 1 (Accès transports viables) |
| 8 | Winnipeg | 1 (Bike Winnipeg) |
| 9 | Hamilton | 1 (Cycle Hamilton) |
| 10 | Kitchener-Cambridge-Waterloo | 2 (CycleWR, TriTAG) |

**Subtotal:** 7 net-new CA CMA orgs.

## Big-state depth results

| State | Existing | Net-new | Final | Target |
|---|---|---|---|---|
| CA | 1 (CalBike) | +2 (California Walks, TransForm) | 3 | ≥3 ✓ |
| NY | 1 (NYBC) | +2 (Reinvent Albany, NYPIRG) | 3 | ≥3 ✓ |
| TX | 1 (BikeTexas) | +2 (Texas Streets Coalition, Texas Pedestrian Safety Coalition) | 3 | ≥3 ✓ |
| FL | 1 (FL Bicycle Assn) | +1 (1000 Friends of FL) | 2 | ≥2 ✓ |
| PA | 1 (PA Walks and Bikes) | +1 (Transit for All PA!) | 2 | ≥2 ✓ |
| OH | 2 (All Aboard OH, OH Bicycle Fed) | 0 | 2 | ≥2 ✓ |
| MI | 1 (LMB) | +1 (Trans4M) | 2 | ≥2 ✓ |
| GA | 1 (Georgia Bikes) | 0 | 1 | ≥2 (PEDS in dissolution-merger with ABC) |
| NC | 1 (BikeWalk NC) | 0 | 1 | ≥2 (Sustain Charlotte is metro-anchored) |
| IL | 1 (Ride Illinois) | 0 | 1 | ≥2 (ATA already at chicago-metro) |

**Subtotal:** 8 net-new statewide orgs. GA/NC/IL stayed at 1 due to
no clean genuinely-distinct candidate at the state level.

## Non-top-50 metro depth (city-leaf canvas) results

| City | MSA slug | Orgs added |
|---|---|---|
| Madison WI | madison-wi-metro | Madison Bikes |
| Boise ID | boise-city-id-metro | Boise Bicycle Project |
| Anchorage AK | anchorage-ak-metro | Bike Anchorage |
| Ann Arbor MI | ann-arbor-mi-metro | Walk Bike Washtenaw, Bicycle Alliance of Washtenaw |
| Boulder CO | boulder-co-metro | Community Cycles |
| New Haven CT | new-haven-ct-metro | Safe Streets Coalition, NCAT |
| Tucson AZ | tucson-az-metro | Living Streets Alliance |
| Albany NY | albany-ny-metro | Albany Bicycle Coalition |
| Spokane WA | spokane-wa-metro | SpokAT |
| Tallahassee FL | tallahassee-fl-metro | Capital City Cyclists |
| Charleston SC | charleston-sc-metro | Charleston Moves |
| Grand Rapids MI | grand-rapids-mi-metro | Greater Grand Rapids Bicycle Coalition |
| Fresno CA | fresno-ca-metro | Fresno County Bicycle Coalition |
| Albuquerque NM | albuquerque-nm-metro | BikeABQ |
| Halifax NS | halifax-cma | Halifax Cycling Coalition (CA bonus) |
| Mississauga ON | toronto-cma | Mississauga Cycling Now! (CA bonus) |

**Subtotal:** 17 net-new orgs across 16 cities.

**Surveyed-but-excluded city-leaf candidates** (logged inline at the
relevant section header in `orgs.toml`): Honolulu (HBL state-floor
covers), Burlington VT (Local Motion state-floor covers), Lexington
KY (no activity verification), Reno NV (NV gap state), Worcester MA
(WalkBike Worcester is a working group, not 501c3), Tulsa OK
(Tulsa Hub redistribution-focused; OK gap state), Long Beach CA
(Bike Long Beach is a BikeLA chapter), Chattanooga + Knoxville TN
(both are Bike Walk TN regional sub-committees without independent
incorporation).

## Final tally

```
Existing orgs (after slice 7.7):          130
+ Top-31-50 US metro canvas:              ~+40 (with 3 gaps documented)
+ CA CMAs #6-10:                          +7
+ Big-state depth (CA/NY/TX/FL/PA/MI):    +8
+ Non-top-50 metro depth (city-leaf):     +16 net-new + 1 multi-anchor
                                          (BikeABQ pre-existed at nm state;
                                          slice 7.8 added albuquerque-nm-metro
                                          as a second anchor — see Per-org data
                                          shape below)
─────────────────────────────────────────────
Net-new total:                            +73
Dataset after slice 7.8:                  203 orgs
```

The per-category subtotals are approximate (e.g., the city-leaf line
counts Bike Durham as a metro-canvas Raleigh-bonus rather than a
city-leaf entry, and Halifax/Mississauga are CMA bonuses); the
dataset total is authoritative.

## Per-org data shape

Unchanged from slice 7.6. Multi-anchoring used sparingly per the
Street Trust precedent (`[portland-or-metro, or]`). One new
multi-anchor entry in slice 7.8: BikeABQ at `[albuquerque-nm-metro,
nm]`. BikeABQ pre-existed on the slice-7.8 baseline at `nm` state
only; slice 7.8 added the metro anchor so ABQ-metro ZIPs see BikeABQ
as a local-tier org while the rest of New Mexico continues to see it
via the state-floor anchor. One row, two anchors — the same role
the Street Trust plays for Portland + Oregon.

## Critical files edited

- `api/seed/orgs.toml` — editorial expansion (+73 entries). Header
  updated to reflect slice 7.8 work and the new total.
- `docs/roadmap.md` — slice 7.8 entry added under Done.
- `docs/superpowers/specs/2026-05-22-org-seed-broadening-design.md`
  — this design doc (new).

**No region-tree changes.** All city-leaf canvas orgs anchored at
existing MSA slugs from `regions_us_msas.toml` / `regions_ca_cmas.toml`.

**Reused, not modified:**
- `api/internal/seed/orgs.go` — loader unchanged.
- `api/internal/loadregions/` — unchanged.
- `justfile` — unchanged.

## Workflow used

Per-region candidate slates with maintainer-approved batch-commit
mode. Each metro/state research surfaced 2–5 candidates with
activity-bar evidence; Claude applied the slice 7.7 scope precedents
directly (admit/exclude with rationale) and committed per region or
per tight group. Maintainer spot-checks against the resulting PR
diff.

## Verification

End-of-slice smoke test:

1. `just pg-reset && just pg-up && just loaddata` — clean load with
   no FK rejections.
2. Hit `/api/v1/lookup` against representative ZIPs from each
   newly-added metro and city. Per the slice 7.8 plan smoke set:
   - Metro ZIPs: 15222 (Pittsburgh), 95814 (Sacramento), 89101
     (Vegas), 45202 (Cincinnati), 64108 (KC), 78701 (Austin),
     43215 (Columbus), 46204 (Indianapolis), 44113 (Cleveland),
     37203 (Nashville), 23510 (Norfolk), 02903 (Providence), 53202
     (Milwaukee), 27601 (Raleigh), 38103 (Memphis), 23219
     (Richmond), 40202 (Louisville), 70112 (New Orleans), 84111
     (SLC), 06103 (Hartford), 14202 (Buffalo).
   - City samples: 53703 (Madison), 83702 (Boise), 99501
     (Anchorage), 48104 (Ann Arbor), 80302 (Boulder), 06510 (New
     Haven), 85701 (Tucson), 12207 (Albany), 99201 (Spokane),
     32301 (Tallahassee), 29401 (Charleston SC), 49503 (Grand
     Rapids), 93721 (Fresno), 87102 (Albuquerque).
   - CA postal: T5J (Edmonton), G1R (Quebec City), R3C (Winnipeg),
     L8P (Hamilton), N2H (Kitchener), B3J (Halifax), L5B
     (Mississauga).
3. Confirm gap entries return graceful empty: 32202 (Jacksonville),
   73102 (OKC), 35203 (Birmingham), 82001 (WY), 25301 (WV).

No new test code — editorial slice.

## Out of scope (carried forward to future slices)

- **State / province gap re-research.** WV, AR, OK, KS, ND, SD, NV,
  WY, PR; PE, SK, NB, YT/NT/NU. Different research surface.
- **Top-20 metro third-org gap re-research.** Miami, Inland Empire,
  Tampa, Denver.
- **Federation depth.** chicagoland (0), rta-service-area (0),
  nyc-tristate (1).
- **Existing-metro top-ups (slice 7.8 Round 5, deferred).** NYC, SF,
  LA, Chicago, Boston could grow with additional quality candidates.
- **GA/NC/IL second-statewide.** No clean candidate surfaced in
  this pass; revisit when PEDS+ABC merger settles or Sustain
  Charlotte / similar incorporates statewide.
- **University-housed program precedent.** Las Vegas's PedSafe
  Vegas was excluded conservatively. A future slice could
  re-evaluate whether university-housed advocacy programs with
  named missions and dated content admit under an extended named-
  program precedent.
- **National-tier US/CA orgs.** Forbidden by editorial policy in
  `docs/region-graph.md` §5 — unchanged.
- **Scope precedent changes.** No new admitted org shapes; activity
  bar unchanged.
- **PT / ES / MX / NL / UK expansion.** v1.1+ territory.

## Precedents set by slice 7.8

- **University-housed advocacy programs** (e.g., PedSafe Vegas at
  UNLV TRC): excluded by extending the slice 7.7 chapter/affiliate
  precedent to academic-program parents. The university is treated
  as the structural equivalent of a parent 501(c)(3) without
  independent program incorporation. Future revisit possible.
- **Sub-committees of a state-level 501(c)(3)** (e.g., Bike Walk
  Chattanooga / Bike Walk Knoxville as Bike Walk Tennessee
  fiscally-sponsored RSCs): excluded per chapter/affiliate
  precedent. The parent's state-floor entry covers the metro via
  ancestor walk.
- **Mid-transition orgs** (e.g., Bike Walk Jax voted in 2025 to
  pursue 501(c)(3) but still meets at city facilities): excluded
  until incorporation is granted and an independent web surface
  exists.
- **Paused orgs** (e.g., Farm & City paused 2025-12-31): excluded
  per the activity bar even if their named programs were previously
  admitted.
- **Bonus geographic coverage** outside the strict canvas (e.g.,
  Bike Durham at durham-nc-metro, Halifax + Mississauga in CA): OK
  when a strong quality candidate surfaces during research.
