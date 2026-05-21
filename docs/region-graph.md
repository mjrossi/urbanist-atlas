# Region graph

Urbanist Atlas models geography as a **directed acyclic graph of
regions**. Postal codes point at a leaf region; every advocacy
organization attaches to one or more regions. A `/lookup` walks the
graph upward from the leaf to gather every region whose orgs should
surface.

This document is the user-facing reference for curators and new
contributors. The design discussion lives at
[`docs/superpowers/specs/2026-05-16-region-graph-design.md`](./superpowers/specs/2026-05-16-region-graph-design.md);
this doc is shorter and skips the alternatives we considered.

---

## Why a graph

The earlier model put four fixed "tier" foreign keys (city, county,
metro, state) on every postal-code row. That works for US/CA postal
codes, where every ZIP has roughly those four meaningful ancestors.
It breaks for:

- **City-states.** Berlin is a *Land* (state-equivalent) but functions
  as a city. Forcing it into the "state" slot would bucket every
  Berlin org as regional rather than local.
- **Multi-state metros.** NYC spans NY/NJ/CT. If we parent the metro
  under all three states, every Brooklyn lookup would incorrectly
  inherit NJ and CT as ancestors.
- **Transit federations.** Chicago's RTA service area, Berlin's VBB
  Verbund, the NY-NJ-CT Tri-State advocacy region — these are
  real, important regions that don't fit the administrative hierarchy.
- **Variable depth.** Germany has Gemeinde → (Kreis?) → Land. France
  has commune → département → région plus métropoles that overlap
  départements. The UK has town → district → county → region →
  nation. No fixed tier count fits all of them.

The graph handles all of these natively. Each region declares its
direct parents; the lookup walks transitively.

---

## Schema

| Table | Role |
|---|---|
| `regions` | One row per region. `kind` is free-form (`us:city`, `de:land`, `fr:metropole`, …). `scope_tier ∈ {local, regional}` is explicit (not derived from kind). `sort_priority` is a server-side hint for ordering within the Regional bucket. |
| `region_parents` | The DAG edges. Multi-parent allowed; `CHECK` blocks self-loops; longer cycles are caught at `loadregions` time. |
| `postal_codes` | Three columns: `(country, postal_code, leaf_region_id)`. The leaf is the most-specific region a postal code falls under. |
| `organization_regions` | Many-to-many between orgs and regions. Orgs can attach to any node in the graph (leaf city, intermediate metro, multi-state region, transit federation). |

The lookup algorithm: `postal_code → leaf region → recursive CTE up
`region_parents` → set of ancestor region IDs → orgs joined to any of
those IDs → bucket each org by `scope_tier` of its matched regions.

---

## Modeling conventions

These are the things you have to know to model a country correctly.
Without them the graph will produce wrong results.

### 1. State edges live on the leaf, not on the metro

When a metro spans multiple states (NYC across NY+NJ+CT, Chicago
across IL+IN+WI), do **not** parent the metro under each state.
Instead, parent each leaf city directly under its own state.

```mermaid
graph BT
  brooklyn[Brooklyn]
  newark[Newark]
  bridgeport[Bridgeport]
  nyc[NYC]
  nycmetro[NYC Metro]
  ny[New York]
  nj[New Jersey]
  ct[Connecticut]
  tristate[Tri-State]

  brooklyn --> nyc
  nyc --> nycmetro
  nyc --> ny
  newark --> nycmetro
  newark --> nj
  bridgeport --> nycmetro
  bridgeport --> ct
  nycmetro --> tristate
```

Why: if `nyc-metro → {ny, nj, ct}`, then a Brooklyn lookup walks
`brooklyn → nyc → nyc-metro → {nj, ct}`, incorrectly inheriting NJ
and CT as ancestors. The walk picks up *all* ancestors transitively.

### 2. Multi-state / federation regions parent the metro or the leaf, not the state

`nyc-tristate` is a parent of `nyc-metro`, not of NY/NJ/CT. The rule:
federations and multi-state regions sit *above* the metro tier (or
directly above leaves), never above the state tier.

Why: if `nyc-tristate → ny`, then a Buffalo ZIP (in NY but not in the
Tri-State advocacy area) would inherit `nyc-tristate` as an ancestor —
wrong.

### 3. Transit federations are siblings of states, parented under the leaves they serve

Chicago's RTA service area covers Cook + 5 collar counties — all in
Illinois. Model it as a top-level region; parent each RTA county
under it. A Cook County lookup walks `cook-county → rta-service-area`;
a Gary IN lookup never touches RTA because Gary's county is parented
under IN, not RTA.

```mermaid
graph BT
  chicago[Chicago]
  oakpark[Oak Park]
  gary[Gary]
  cook[Cook County]
  lakein[Lake County IN]
  chimetro[Chicago Metro]
  rta[RTA Service Area]
  il[Illinois]
  in[Indiana]
  chimulti[Chicagoland]

  chicago --> cook
  oakpark --> cook
  gary --> lakein
  cook --> chimetro
  cook --> rta
  lakein --> chimetro
  lakein --> in
  rta --> il
  chimetro --> chimulti
```

Berlin's VBB similarly: a top-level region; both `berlin` and
`brandenburg` have it as a parent.

### 4. `scope_tier` is editorial, not derived

Berlin is `kind='de:land'` (formally correct) but `scope_tier='local'`
(functionally correct — Berliners experience it as a city). The
maintainer makes this judgment per region.

The rule of thumb:

> Local = what a resident calls their city or neighborhood.
> Regional = what they call their region, metro, or state.
> National = a country-wide umbrella that doesn't fit into any
> single regional unit.

### 5. The `national` tier exists, but the default lookup filters it

`scope_tier='national'` is a third bucket alongside `local` and
`regional`. National-tier regions are filtered from the default
`/api/v1/lookup` ancestor walk so they don't surface in postal-code
results.

**Per-country editorial policy** (load-bearing — please read before
adding a national region):

- **US / CA: do NOT create `us:national` or `ca:national` regions in
  v1 seed data.** The local-first ethos is the prime directive for US
  and Canadian results. National orgs that have state/provincial
  chapters get modeled as their chapters (e.g. Rail Passengers
  Association → state chapters). National orgs without local presence
  are simply excluded.
- **PT / UK / NL / MX / future countries: create `<cc>:nacional` (or
  `<cc>:national`) only when an org genuinely operates nationally
  without sufficient local presence.** Examples: MUBi national (PT
  cycling federation), Fietsersbond (NL), Living Streets (UK).

**National regions get no incoming parent edges from the leaf chain.**
They live as standalone attachment targets. The default lookup never
reaches them through the ancestor walk; the SQL-level filter in
`AncestorRegions` is defense-in-depth.

**Scale calibration.** The semantic weight of "national" varies by
country scale. A PT-national org (~10M-person country) is closer in
*effective scope and influence* to a US state-level org than to a
hypothetical US-national one. Schema treats `national` uniformly;
editorial judgment about how to *frame* national orgs is per-country.
This informs future UX (a PT user's "national umbrella" feels close,
a US user's would feel distant), not the default display rules.

### 6. Abstract regions don't need postal codes pointing at them

`nyc-tristate` and `vbb-region` are interior nodes with no postal codes
attached directly. The ancestor walk reaches them through their
children. Orgs attached to abstract regions still surface correctly.

### 7. Postal codes anchor at the smallest curated region

`postal_codes.leaf_region_id` is a misnomer — the schema doesn't
constrain that referenced row to be a leaf in the DAG sense. The
recursive ancestor walk works from any region. So the convention is:
**every postal code anchors at the smallest curated region for its
area**, and granularity grows organically per-ZIP as we curate more
leaves.

Resolution priority, applied at seed-time:

```
if curated city leaf exists for ZCTA's place        → anchor = city leaf
elif ZCTA is in an NYC borough county               → anchor = borough leaf
elif ZCTA is in a curated MSA                       → anchor = MSA region
else                                                → anchor = state/province
```

Worked examples:

| Postal code | Anchor       | Why                                                |
|---|---|---|
| 10001 (NYC) | `manhattan`  | NYC borough county = New York County               |
| 94110 (SF)  | `sf`         | SF is a curated city leaf                          |
| 33401 (WPB) | `miami-msa`  | West Palm Beach is in the Miami MSA, no city leaf  |
| 83702 (ID)  | `id` (state) | No leaf, not in a curated MSA; state fallback      |

The lookup-side ancestor walk is unchanged: from whichever anchor the
postal code points at, walk up via `region_parents`, gather ancestors,
join orgs. No app-level fallback logic.

The full design rationale is at
[`docs/superpowers/specs/2026-05-19-postal-coverage-design.md`](./superpowers/specs/2026-05-19-postal-coverage-design.md).

### 8. NYC is the only US city we model sub-municipally at v1

NYC's five boroughs (Manhattan, Brooklyn, Queens, Bronx, Staten
Island) exist as separate leaves under a single `nyc` regional
intermediate region. This is the **only sub-municipal split in the
US seed at v1**.

Three things converge to make NYC structurally splittable:

1. **NYC boroughs are counties.** Manhattan = New York County,
   Brooklyn = Kings County, Queens = Queens County, Bronx = Bronx
   County, Staten Island = Richmond County. The Census ZCTA-to-county
   crosswalk hands us borough resolution for free.
2. **Each borough has distinct civic identity** — Borough President,
   council districts, etc. Most other US "city subdivisions" (LA
   neighborhoods, Chicago community areas, DC wards, Boston
   neighborhoods) lack distinct civic governance.
3. **Borough-specific advocacy ecosystems exist** (e.g., Brooklyn
   Spoke is borough-only). The split gives them a natural
   attachment point.

The DAG shape:

```mermaid
graph BT
  manhattan[Manhattan<br/>local · 10]
  brooklyn[Brooklyn<br/>local · 10]
  queens[Queens<br/>local · 10]
  bronx[The Bronx<br/>local · 10]
  staten[Staten Island<br/>local · 10]
  nyc[NYC<br/>REGIONAL · 15]
  nycmetro[NYC Metro<br/>regional · 40]
  ny[New York<br/>regional · 60]

  manhattan --> nyc
  brooklyn --> nyc
  queens --> nyc
  bronx --> nyc
  staten --> nyc
  manhattan --> ny
  brooklyn --> ny
  queens --> ny
  bronx --> ny
  staten --> ny
  nyc --> nycmetro
```

Note: `nyc.scope_tier = regional` even though `kind = us:city` —
similar to Berlin's `kind = de:land` / `scope_tier = local`
exception. The scope_tier-vs-kind decoupling earns its keep here.

The state edge (`ny`) lives on each borough leaf, **not** on `nyc`
itself, per [rule §1](#1-state-edges-live-on-the-leaf-not-on-the-metro).
A Manhattan lookup walks `manhattan → {nyc, ny} → nyc-metro →
nyc-tristate`; the orgs at each tier surface in the right bucket.

**Citywide NYC orgs attach to `nyc`** (regional) — TransitCenter,
Transportation Alternatives, Riders Alliance. **Borough-only orgs
attach to the specific borough leaf** (Brooklyn Spoke → `brooklyn`).
**Metro-wide orgs attach to `nyc-metro`** (Regional Plan
Association).

Other US cities (LA, Chicago, Boston, SF, Miami, etc.) stay as a
single leaf at v1, even where neighborhood identity is strong. The
ZCTA-to-county crosswalk would give them a single shared county, not
neighborhood-level granularity. Promotion to multi-leaf would
require additional editorial work + ZIP-to-neighborhood crosswalks
with fuzzy boundaries. Deferred until org density warrants.

### 9. Sort priority is a hint, not a contract

`sort_priority` orders orgs within the Regional bucket. Lower = more
specific = sorts earlier. Recommended ranges:

| Range | Tier |
|---|---|
| 10 | borough, neighborhood, freguesia |
| 15 | consolidated city (NYC), município, city-state acting as city (Berlin) |
| 20 | county |
| 30 | CIM, regional district |
| 40 | metro, área metropolitana, CMA |
| 50 | transit federation, distrito, RTA-style |
| 60 | state, province, Land, NUTS-II, região autónoma |
| 80 | multi-state, multi-province, multi-Land |
| 90 | national umbrella (only surfaced via opt-in) |

These are guidelines. The lookup doesn't care about the specific
numbers — only their relative order.

---

## Worked examples

### NYC (multi-state metro + Tri-State)

```mermaid
graph BT
  brooklyn[Brooklyn<br/>local · 10]
  manhattan[Manhattan<br/>local · 10]
  nyc[NYC<br/>local · 15]
  nycmetro[NYC Metro<br/>regional · 40]
  ny[New York<br/>regional · 60]
  tristate[Tri-State<br/>regional · 80]

  brooklyn --> nyc
  manhattan --> nyc
  nyc --> nycmetro
  nyc --> ny
  nycmetro --> tristate
```

**Lookup `11217 US`:**
- Ancestors: `{brooklyn, nyc, nyc-metro, ny, nyc-tristate}`
- **Local:** Brooklyn Spoke, TransAlt, StreetsPAC, Riders Alliance
- **Regional:** TransitCenter (nyc-metro), NY LCV (ny), Tri-State (nyc-tristate)

### Chicago (transit federation IL-only)

See above; the RTA pattern. A Chicago ZIP surfaces RTA-attached orgs;
a Gary IN ZIP doesn't, because Gary's county is parented under IN,
not RTA.

### Vancouver (no county layer)

```mermaid
graph BT
  vancouver[Vancouver<br/>local · 10]
  burnaby[Burnaby<br/>local · 10]
  metvan[Metro Vancouver<br/>regional · 40]
  bc[British Columbia<br/>regional · 60]

  vancouver --> metvan
  burnaby --> metvan
  metvan --> bc
```

The model gracefully omits tiers a country doesn't have. CA has no
county equivalent; the graph just doesn't have those nodes.

### Berlin (city-state + transit federation across Länder)

```mermaid
graph BT
  mitte[Mitte<br/>local · 10]
  berlin[Berlin<br/>LOCAL · 15<br/>kind: de:land]
  potsdam[Potsdam<br/>local · 10]
  brandenburg[Brandenburg<br/>regional · 60]
  vbb[VBB-Region<br/>regional · 50]

  mitte --> berlin
  potsdam --> brandenburg
  berlin --> vbb
  brandenburg --> vbb
```

Berlin's `scope_tier='local'` despite `kind='de:land'`. A Berlin lookup
walks `mitte → berlin → vbb-region`. A Potsdam lookup walks
`potsdam → brandenburg → vbb-region`. VBB-attached orgs surface for
both; berlin-attached orgs only surface for Berlin lookups.

### Lisboa (multi-parent município + cross-distrito metro)

```mermaid
graph BT
  smm[Santa Maria Maior<br/>local · 10]
  lisboa[Lisboa<br/>local · 15<br/>kind: pt:municipio]
  setubal[Setúbal<br/>local · 15<br/>kind: pt:municipio]
  aml[Área Met. de Lisboa<br/>regional · 40]
  dlx[Distrito de Lisboa<br/>regional · 50]
  dst[Distrito de Setúbal<br/>regional · 50]
  gl[Grande Lisboa<br/>regional · 60<br/>NUTS-II]
  ps[Península de Setúbal<br/>regional · 60<br/>NUTS-II]

  smm --> lisboa
  lisboa --> aml
  lisboa --> dlx
  setubal --> aml
  setubal --> dst
  aml --> gl
  aml --> ps
  dlx --> gl
  dst --> ps
```

AML is the **first metro in our data that multi-parents into two
top-tier admin regions** — both NUTS-II Grande Lisboa AND NUTS-II
Península de Setúbal. Setúbal-município is in Distrito de Setúbal but
also belongs to AML, so its lookup picks up *both* NUTS-II regions via
AML — even though Setúbal isn't geographically in Distrito de Lisboa.
This is the same pattern Zona Metropolitana del Valle de México will
use when MX is added; it scales.

### Madeira (autonomous region as parallel hierarchy)

```mermaid
graph BT
  funchal[Funchal<br/>local · 15<br/>kind: pt:municipio]
  madeira[Região Aut. Madeira<br/>regional · 60]

  funchal --> madeira
```

Madeira has no NUTS-II parent — it parallels the NUTS-II tier rather
than living inside it. The graph naturally accommodates: just no
outgoing parent edge from the autonomous region. Funchal lookup walks
`funchal → madeira` and stops.

### Portugal national umbrella (default-filtered)

```mermaid
graph BT
  ptn[Portugal<br/>NATIONAL · 90<br/>kind: pt:nacional]

  ptn -.->|attached via region_slugs:<br/>mubi-nacional| ptn
```

`pt-nacional` has **no incoming parent edges** from the geographic
chain — it's a standalone attachment target for `mubi-nacional` and
similar national-scope orgs. The default `/lookup` filter excludes
`scope_tier='national'` from the ancestor walk, so a Lisboa postal
code lookup never reaches this region. A future opt-in surface can
explicitly query for national-tier orgs.

---

## Adding a new country

Worked example: Germany. The region taxonomy now splits across
multiple files per country, mirroring how slices #7.5.1–#7.5.4
structured US + CA. Files at minimum:

| File | Purpose |
|---|---|
| `regions_de_lander.toml` (or `_states`) | Top-tier hand-defined Länder. `scope_tier=regional`. No parents. |
| `regions_de_multistate.toml` *(optional)* | Multi-Länder advocacy regions or transit federations (e.g., VBB, MVV). |
| `regions_de_msas.toml` *(generated, optional)* | Metro-equivalent regions from a Census-style upstream source via the ETL pipeline. Skip if you start hand-curated. |
| `regions_de.toml` | Hand-curated city/Gemeinde leaves. Parents reference state/multi-state/MSA slugs from the files above (cross-file resolution handled by `internal/loadregions/write.go`'s `RegionIDBySlug` fallback). |

Per-country file lists live in `api/internal/loaddata/loaddata.go`'s
`countries` table — add a `{code, regionFiles, postal}` entry there
when adding a country, in the right load order.

### Step-by-step

1. **Write `api/seed/regions_de_lander.toml`.** All 16 Länder.
   `scope_tier=regional`, `kind=de:land`, no parents. Mirror the
   structure of `regions_us_states.toml`.

2. **(Optional) Write `api/seed/regions_de_multistate.toml`.** Transit
   federations (VBB, VRR, MVV, …) and any multi-Länder advocacy
   regions. Parent under the Länder slugs from step 1 where
   appropriate per rule §3.

3. **(Optional) Build the ETL plan.** If Germany has a Census-style
   reference for metros (e.g., the EU's NUTS-3 codes mapped to
   FUAs / Stadtregionen), add `api/internal/etl/de/` with parsers +
   plan registration. Otherwise skip — hand-curating one
   `regions_de.toml` is fine for low cardinality.

4. **Write `api/seed/regions_de.toml`.** Cities. For city-states
   (Berlin, Hamburg, Bremen), use `kind=de:land` + `scope_tier=local`
   (the editorial override per rule §4). Other cities get
   `kind=de:kreisfreie-stadt` or `de:gemeinde` + `scope_tier=local`.

5. **Generate `api/seed/postal_codes_de.csv`.** Reshape an upstream
   source (OpenGeoDB, Geonames, official Bundespost data) into the
   3-column format `postal_code,country,leaf_region_slug` using the
   smallest-anchor pattern: prefer city leaves, fall through to
   Länder for un-curated postcodes. The ETL pipeline can do this if
   you build the DE plan in step 3.

6. **Add DE orgs to `api/seed/orgs.toml`.** Use `region_slugs` to
   attach them.

7. **Update `api/internal/loaddata/loaddata.go`.** Add a
   `{"DE", []string{"de_lander", "de_multistate", "de"}, "de"}`
   entry so `just loaddata` picks up the new country.

8. **Run `just loaddata` against a fresh dev DB.**

9. **Add a per-country postal normalizer if needed.** Edit
   `api/pkg/atlas/postal.go`. The default normalizer
   (`uppercase + strip whitespace`) works for DE (5-digit numeric).
   UK and CA need special handling (outward code / FSA truncation);
   those are already in place.

10. **Smoke-test:**

    ```sh
    just lookup 10115 DE     # Berlin-Mitte
    just lookup 14467 DE     # Potsdam
    ```

That's the flow. No schema changes; no code changes beyond the
single-line addition in `loaddata.go` (and the ETL plan if you
choose to build one).

---

## Open vocabulary

`RegionKind` and `Country` are free-form strings on the wire. The
recommended vocabulary uses country-prefixed values:

| Country | Recommended kinds |
|---|---|
| US | `us:borough`, `us:city`, `us:county`, `us:metro`, `us:state`, `us:multi-state`, `us:transit-federation` |
| CA | `ca:city`, `ca:regional-district`, `ca:cma`, `ca:province` |
| PT | `pt:freguesia`, `pt:municipio`, `pt:cim`, `pt:area-metropolitana`, `pt:distrito`, `pt:nuts-ii`, `pt:regiao-autonoma`, `pt:nacional` |
| DE | `de:bezirk`, `de:kreisfreie-stadt`, `de:kreis`, `de:land`, `de:transit-federation` |
| FR | `fr:commune`, `fr:departement`, `fr:region`, `fr:metropole` |
| UK | `uk:town`, `uk:unitary-authority`, `uk:county`, `uk:region`, `uk:nation`, `uk:national` |
| AU | `au:suburb`, `au:lga`, `au:gccsa`, `au:state` |

Clients should treat unknown kinds gracefully — fall back to
displaying `name`.

`scope_tier` is a closed enum on the wire: exactly `'local'`,
`'regional'`, or `'national'`. The default `/lookup` surface buckets
results into Local + Regional; `national` regions are filtered out of
the ancestor walk so they don't surface for postal-code queries. A
future opt-in path (query param or separate endpoint) is anticipated
for the national tier; until then, national-tier orgs exist in the
schema but are not visible through the default API.

---

## Locked-in conventions

These are documented here so that adding a new country is mechanical —
contributors don't need to re-decide them.

1. **Slug**: bare (`brooklyn`, `lisboa-municipio`, `metro-vancouver`).
   Suffix on collision (`lake-county-in`, `ca-state`). Don't
   country-prefix unless forced.
2. **Kind**: always country-prefixed (`pt:municipio`, `us:state`,
   `ca:province`).
3. **`sort_priority`**: use the bands documented in §9 above. New
   country fits or documents a deviation in its design spec.
4. **`scope_tier`**: editorial per rule §4 (local = city/neighborhood;
   regional = region/metro/state; national = country-wide umbrella per
   the per-country policy in §5).

The full validation report for the PT model probe (which exercised
each of these conventions against a non-US/CA admin geography) lives
at
[`docs/superpowers/specs/2026-05-17-region-graph-pt-validation-design.md`](./superpowers/specs/2026-05-17-region-graph-pt-validation-design.md).
