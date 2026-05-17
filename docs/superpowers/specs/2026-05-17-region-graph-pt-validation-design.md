# Region-graph validation via Portugal (slice #4.6)

**Status:** Active — implementation of slice #4.6.
**Supersedes:** none. Builds on [`2026-05-16-region-graph-design.md`](./2026-05-16-region-graph-design.md).
**Related:** [`docs/region-graph.md`](../../region-graph.md) (user-facing reference).

## Why this exists

Slice #4.5 shipped the multi-parent region DAG and the loaders that consume it. The model has only been exercised against US and Canada. Before we accumulate more US/CA editorial work and freeze the data model by inertia, this slice runs a *deliberate stress test* against a non-US/CA admin geography to surface any structural gaps now — when the cost of a schema change is small — rather than after shipping.

Portugal is the probe. The maintainer is moving there, so PT is both the practically motivated dataset and a country whose administrative geography exercises every pattern we expect to encounter as the atlas grows toward MX, NL, UK, and others:

- **Multi-parent municípios** (Lisboa belongs to both Distrito de Lisboa and the Área Metropolitana de Lisboa).
- **Metros that cross top-tier admin boundaries** (AML spans two distritos and, post-2024 NUTS reorganization, two NUTS-II regions).
- **Autonomous regions as parallel hierarchies** (Açores, Madeira sit outside the continental NUTS structure).
- **Sub-municipal consolidation** (uniões de freguesias from the 2013 reform).
- **National-scope umbrellas** (MUBi, the cycling federation — the first time we encounter an org class that doesn't fit the current local/regional contract).

This is a **model validation exercise, not a shipping deliverable.** The PT seed data is intentionally small (~15 regions, ~7 postal codes, ~4 orgs) — enough to hit each model edge once and prove the model handles it. A full Portuguese directory is out of scope; that's a separate editorial effort if and when it becomes a priority. **The primary shipping decision remains US and Canada.**

## Strategic goal

> "Avoiding a substantial change to the data model after shipping and having something that is robust and expandable enough for additional regions as we go — eventually to include MX, NL, UK, etc over the coming months and years." — maintainer

The spec optimizes for forward compatibility. Every decision is held to the test: *will this still work when we add country #6 without re-deciding it?*

## Design

### 1. Portuguese taxonomy

Country-prefixed kinds, following the established `us:` / `ca:` convention:

| Kind | scope_tier | sort_priority | Notes |
|---|---|---|---|
| `pt:freguesia` | local | 10 | Civil parish; finest grain. Uniões de freguesias (post-2013 mergers) use the same kind. |
| `pt:municipio` | local | 15 | Município (continental) or concelho (autonomous regions). Lisboa-município ≈ Lisboa city. |
| `pt:cim` | regional | 30 | Comunidade Intermunicipal — non-metro inter-municipal grouping. |
| `pt:area-metropolitana` | regional | 40 | AML, AMP — inter-municipal metros. |
| `pt:distrito` | regional | 50 | Vestigial since 2003 but still formal; included so AML can multi-parent into 2 distritos. |
| `pt:nuts-ii` | regional | 60 | Statistical region (post-2024 reorg). |
| `pt:regiao-autonoma` | regional | 60 | Açores, Madeira — top-level, no NUTS-II parent. |
| `pt:nacional` | **national** | 90 | One row: "Portugal". Migration 0003 makes this scope_tier legal. |

### 2. National-orgs resolution: third `scope_tier` value

The existing model has two scope tiers: `local` (city/county-level) and `regional` (metro/state/multi-state). National-scope advocacy orgs have no clean home in this model. The US/CA decision was "no national orgs in the default lookup" — fine when modeling US/CA where national orgs are rare in transit advocacy, but a non-starter for PT/UK/NL where they're often the primary voice.

**Decision: add `national` as a third `scope_tier` value, ship now (migration 0003), preserve US/CA behavior via editorial policy.**

#### Schema

- `regions.scope_tier CHECK` constraint expands from `('local','regional')` to `('local','regional','national')`.
- New `Country.ScopeNational = "national"` constant in Go.
- OpenAPI `ScopeTier` enum gains `national`.
- TOML loader (`loadregions/toml.go`) accepts `scope_tier = "national"`.

#### Default lookup behavior

`/api/v1/lookup` filters `scope_tier='national'` from the ancestor walk by default. Implemented at the SQL level in `AncestorRegions` recursive CTE (`api/internal/store/postgres/queries/lookup.sql`):

```sql
-- Both the base case and the recursive step filter:
WHERE r.scope_tier <> 'national'
```

Result: clients calling `/api/v1/lookup` see exactly the same shape they see today — `local` + `regional` buckets, no national-tier presence in `resolved_ancestry`, no national orgs in either bucket. Default behavior is *uniform across countries*.

#### Data shape — no parent edges into national regions

National regions (`pt-nacional`, future `<cc>:national` rows) get **no incoming `region_parents` edges** from the leaf chain. The geographic DAG models geographic containment (Lisboa is in AML is in Grande Lisboa); "Portugal" isn't a geographic ancestor in that sense — it's a top-level political abstraction.

National orgs attach to national regions via `region_slugs` in `orgs.toml`. The default lookup never reaches those regions through the ancestor walk, so national orgs don't surface. The SQL-level filter is defense-in-depth: even if a future contributor misjudges and adds a parent edge to a national region, the filter prevents national orgs from leaking into default results.

#### Editorial policy: per-country, not per-schema

The schema treats `national` uniformly. The *editorial policy* differs by country.

**US/CA: do NOT create `us:national` or `ca:national` regions in v1 seed data.** The local-first ethos is the prime directive for US/CA results: most effective advocacy is local, and the directory should prioritize that. Concretely:
- National orgs that have state/provincial chapters get modeled as their chapters (Rail Passengers Association → state chapters, when added).
- National orgs without local presence are simply excluded from v1 — same de facto outcome as today.
- The schema *permits* `us:national`/`ca:national` regions. The policy *forbids* using them without a case-by-case maintainer judgment.

**PT/UK/NL/MX (and future countries): create `<cc>:nacional` (or `<cc>:national`) when an org genuinely operates nationally without sufficient local presence.** Examples that would qualify: MUBi national federation (PT); Fietsersbond (NL); Living Streets, Sustrans (UK).

**Sort priority for `*:national` is 90** — the highest band in the sort_priority scheme. If a future UX surfaces national orgs (via opt-in), they appear visually subordinated. Local-first ethos is reinforced by the band ordering, not just by filtering.

#### Editorial nuance: "national" doesn't mean the same thing across country scales

Portugal is ~10M people; a PT-national org is closer in *effective scope and influence* to a US state-level org than to a hypothetical US-national one (the US is 33× the population). NL similarly is ~17M; UK is ~67M; MX is ~130M — all larger than PT but still smaller than the US.

The schema treats `national` uniformly (top tier of a country), but contributors should think of the *semantic weight* of a national tag in terms of the country's own scale. A PT user seeing MUBi national in a future opt-in "national umbrella" section is not looking at something distant — they're looking at *the* cycling federation for their country, which is more naturally part of their decision-making than a hypothetical US-national equivalent would be.

This nuance informs *how we frame* national orgs editorially and in future UX. Default-hide behavior is preserved across all countries; this is about presentation and editorial judgment, not display rules.

### 3. Locked-in conventions

These are now load-bearing for the data model and should be followed for every future country.

#### 3.1 Slug convention

Bare slugs (`brooklyn`, `lisboa-municipio`, `metro-vancouver`). Suffix-on-collision (`lake-county-in` because Lake County also exists in CA, NY, FL). Don't country-prefix unless forced.

**Why:** Bare slugs match the existing US/CA pattern, keep eventual public URLs clean (`/r/brooklyn` not `/r/us-brooklyn`), and the collision risk is low (only ~10-100 regions per country, hand-curated). The suffix-on-collision pattern is already in the data and handles edge cases gracefully.

#### 3.2 Kind convention

Always country-prefixed: `pt:municipio`, `us:state`, `ca:province`, `de:land`. Already established; documented here as the rule, not just the practice.

**Why:** Region kinds are meaningful only in their country's administrative context. A "state" means something different in US vs Australia vs Germany. Prefixing makes intent unambiguous and keeps the vocabulary open.

#### 3.3 `sort_priority` bands

The canonical band scheme:

| Band | Tier | Examples |
|---|---|---|
| 10 | Neighborhood / borough / freguesia | Brooklyn, Mitte, Santa Maria Maior |
| 15 | Consolidated city / município | New York City, Lisboa, Berlin (as city) |
| 20 | County | Cook County, Lake County IN |
| 30 | CIM / regional district | Comunidade Intermunicipal da Lezíria do Tejo, Metro Vancouver (`ca:regional-district`) |
| 40 | Metro / area metropolitana / CMA | NYC Metro, AML, Greater Toronto Area |
| 50 | Transit federation / distrito | RTA Service Area, Distrito de Lisboa, VBB-Region |
| 60 | State / province / Land / NUTS-II / região autónoma | New York, BC, NRW, Grande Lisboa, Madeira |
| 80 | Multi-state / multi-region | Tri-State Region, Chicagoland |
| 90 | National | pt-nacional, future uk:national |

Future countries fit into these bands or document a deviation in the spec.

**Why:** The lookup orders orgs within the Regional bucket by `sort_priority` (lower = more specific = earlier). A consistent band scheme across countries means contributors can pick the right number without re-deciding, and the visual ordering is predictable.

#### 3.4 `scope_tier` editorial rule

> "Local = what a resident calls their city or neighborhood. Regional = what they call their region, metro, or state. National = a country-wide umbrella that doesn't fit into any single regional unit."

Berlin is `kind='de:land'` but `scope_tier='local'` because Berliners experience it as a city. Lisboa município is `local`. Distrito de Lisboa is `regional`. MUBi national is `national`. The rule is editorial judgment, not derivation from kind.

**Why:** This is the only "soft" part of the model — but it's the part that most directly drives user-perceived correctness. Codifying the rule means contributors have a consistent yardstick.

### 4. Loader engineering

Two small changes unblock the entire EU+MX+UK roadmap, not just PT:

#### 4.1 Portuguese postal code support (`api/pkg/atlas/postal.go`)

Portuguese postal codes use the format `NNNN-NNN` (e.g. `1000-001` for parts of Lisboa). Normalize to 7 raw digits (matching the existing "uppercase + strip whitespace" rule, just extended to also strip hyphens for PT):

```go
case "PT":
    // PT postal codes are 7-digit NNNN-NNN; canonicalize by stripping
    // the hyphen so storage is 7 raw digits. Lookups apply the same
    // normalization, so "1100-001" and "1100001" resolve identically.
    return strings.ReplaceAll(s, "-", "")
```

And the matching validation:

```go
case "PT":
    if len(code) != 7 {
        return fmt.Errorf("PT postal code %q: want 7 digits (after stripping hyphen)", code)
    }
    for _, r := range code {
        if r < '0' || r > '9' {
            return fmt.Errorf("PT postal code %q: non-digit character", code)
        }
    }
```

Per-country normalization in a switch statement scales — each new country adds one case. This is the expected pattern, not a workaround.

#### 4.2 Drop the `--country US|CA` whitelist (`api/cmd/server/loadpostal.go`)

The `loadpostal` subcommand currently rejects any country except `US` or `CA`. This was a v1-anchors guardrail; with country becoming an opaque string per `api/pkg/atlas/atlas.go:12-20`, the whitelist is the last code-level coupling. Replace it with a soft `slog.Warn` when `ValidatePostalCode` returns nil for an unrecognized country (so operators see they're in uncharted territory but loading proceeds).

After this change, every future country (ES, MX, NL, UK) is purely editorial: write the TOML, write the CSV, run the loader. The *only* code change per country is the `postal.go` per-country validation case, which is a copy-paste pattern.

### 5. Migration 0003

```sql
-- 0003_national_scope.sql
--
-- Expand regions.scope_tier to include 'national' as a third bucket.
-- See docs/superpowers/specs/2026-05-17-region-graph-pt-validation-design.md
-- for the editorial policy that distinguishes when 'national' applies
-- vs when an org should be modeled as 'regional' (with state chapter).
--
-- DEFAULT lookup behavior continues to bucket only 'local' + 'regional'.
-- The 'national' tier exists in the schema to allow national-scope
-- orgs (MUBi national, Living Streets UK, …) to be modeled without
-- distorting the local-first defaults.

-- +goose Up

-- +goose StatementBegin
ALTER TABLE regions DROP CONSTRAINT regions_scope_tier_check;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE regions ADD CONSTRAINT regions_scope_tier_check
    CHECK (scope_tier IN ('local','regional','national'));
-- +goose StatementEnd

-- +goose Down

-- Refuse the downgrade if any national rows exist — would otherwise
-- be a data-loss operation. Operator should delete those rows
-- explicitly before downgrading.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM regions WHERE scope_tier = 'national') THEN
        RAISE EXCEPTION 'Cannot downgrade: regions.scope_tier=national rows exist';
    END IF;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE regions DROP CONSTRAINT regions_scope_tier_check;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE regions ADD CONSTRAINT regions_scope_tier_check
    CHECK (scope_tier IN ('local','regional'));
-- +goose StatementEnd
```

### 6. Sample data shape

#### `api/seed/regions_pt.toml` (~15 regions)

The set covers every model edge:

- **Top-level (no parents):** `nuts-ii-norte`, `nuts-ii-centro`, `nuts-ii-grande-lisboa`, `nuts-ii-peninsula-setubal`, `regiao-autonoma-madeira`, `pt-nacional`.
- **Distritos:** `distrito-lisboa` (parent: `nuts-ii-grande-lisboa`), `distrito-setubal` (parent: `nuts-ii-peninsula-setubal`), `distrito-porto` (parent: `nuts-ii-norte`), `distrito-aveiro` (parent: `nuts-ii-centro`).
- **Áreas Metropolitanas:** `aml` (parents: `nuts-ii-grande-lisboa`, `nuts-ii-peninsula-setubal` — **multi-NUTS-II parent**), `amp` (parent: `nuts-ii-norte`).
- **CIM:** `cim-regiao-aveiro` (parent: `nuts-ii-centro`).
- **Municípios (multi-parent):** `lisboa-municipio` (parents: `distrito-lisboa`, `aml`), `setubal-municipio` (parents: `distrito-setubal`, `aml` — **cross-distrito metro membership**), `porto-municipio` (parents: `distrito-porto`, `amp`), `aveiro-municipio` (parents: `distrito-aveiro`, `cim-regiao-aveiro`), `funchal-concelho` (parent: `regiao-autonoma-madeira` — **autonomous parallel hierarchy**).
- **Freguesias:** `santa-maria-maior` (parent: `lisboa-municipio`), `alvalade` (parent: `lisboa-municipio`), `ribeira-porto` (parent: `porto-municipio`), `lordelo-massarelos` (parent: `porto-municipio` — **união de freguesias under the same `pt:freguesia` kind**).

#### `api/seed/postal_codes_pt.csv` (~7 codes, 7-digit hyphen-stripped)

```
postal_code,country,leaf_region_slug
1100001,PT,santa-maria-maior
1700001,PT,alvalade
2900001,PT,setubal-municipio
4050001,PT,ribeira-porto
4150001,PT,lordelo-massarelos
3800001,PT,aveiro-municipio
9000001,PT,funchal-concelho
```

#### `api/seed/orgs.toml` (4 new entries)

- `mubi-lisboa` → `["lisboa-municipio"]`, tags: `["cycling","advocacy","local-chapter"]`
- `mubi-porto` → `["porto-municipio"]`, tags: `["cycling","advocacy","local-chapter"]`
- `lisboa-para-pessoas` → `["lisboa-municipio"]`, tags: `["pedestrian","walkability","safe-streets"]`
- `mubi-nacional` → `["pt-nacional"]`, tags: `["cycling","national-federation"]` — hidden from default lookup; will surface via future opt-in mechanism.

### 7. Forward-looking analysis: predicted behavior for MX, NL, UK

The point of this slice is to be confident the model holds for upcoming countries. Predictions:

#### Mexico (MX)

**Geography:** 32 federal entities (31 estados + CDMX), municipios (~2,400), CDMX with alcaldías. Zona Metropolitana del Valle de México spans CDMX + Estado de México + Hidalgo. Major metros in Guadalajara (ZMG spans Jalisco + Nayarit), Monterrey (ZMM in Nuevo León), Puebla-Tlaxcala (cross-state).

**Stress test:** Multi-state metros (same pattern as US NYC across NY/NJ/CT, PT AML across two NUTS-II). CDMX-as-federal-district resembles Berlin-as-Land (`scope_tier='local'` despite kind being a federal entity).

**Prediction: handled cleanly** by the same patterns. Recommended kinds: `mx:estado`, `mx:cdmx`, `mx:alcaldia` (CDMX subdivision, behaves like NYC borough), `mx:municipio`, `mx:zona-metropolitana`. National orgs (BiciRed Mexico, ITDP Mexico) get `mx:nacional` per Section 2's editorial policy.

#### Netherlands (NL)

**Geography:** 12 provinces, ~340 gemeenten. No autonomous regions. Randstad — the megalopolis spanning Noord-Holland, Zuid-Holland, Utrecht, and parts of Flevoland — is an informal/statistical region, not a statutory entity. Transit governance is by OV-bureaus (per-province in most provinces, regional in some).

**Stress test:** Editorial-only mega-region (Randstad has no statutory backing but is culturally and economically real). Multi-parent gemeenten that sit in both Randstad and their province.

**Prediction: handled cleanly** with `nl:province`, `nl:gemeente`, `nl:randstad` (editorial kind, multi-parent across 4 provinces), plus `nl:national` for Fietsersbond and similar. The Randstad pattern matches PT's AML pattern at a structural level.

#### United Kingdom (UK)

**Geography:** Four constituent nations (England, Scotland, Wales, Northern Ireland). England has counties + unitary authorities + Greater London (with 32 boroughs + GLA combined authority); Scotland has 32 council areas; Wales has 22 principal areas; NI has 11 districts. Combined Authorities (Greater Manchester, West Midlands, West Yorkshire, …) span multiple local authorities and have transit powers.

**Stress test:** The most demanding case. UK-wide orgs (Living Streets, Sustrans, Campaign for Better Transport) have no clean home in the existing two-tier model — they require `uk:national`. Combined Authorities cross local-authority boundaries (same multi-parent pattern as AML/RTA). Greater London resembles NYC consolidated city.

**Prediction: handled cleanly**, but UK will be the heaviest user of `<cc>:national` of any country to date. Recommended kinds: `uk:nation` (England, Scotland, Wales, NI), `uk:region` (English regions like North West), `uk:combined-authority`, `uk:county`, `uk:unitary-authority`, `uk:london-borough`, `uk:district`, `uk:national`. The `uk:nation` tier is interesting — there's no "UK" between the four nations and the national orgs, so `uk:national` attaches directly with no intermediate.

### 8. Validation findings (post-implementation)

The model handled every predicted case cleanly. Verified end-to-end via the testcontainers integration test (`TestPipeline_PT_ValidationFixture`) and direct HTTP smoke tests against the dev server.

- [x] **Multi-parent município (Lisboa: distrito + AML):** loads cleanly; `1100-001` lookup walks `santa-maria-maior → lisboa-municipio → {aml, distrito-lisboa}`.
- [x] **Metro spans 2 NUTS-II (AML → Grande Lisboa + Península de Setúbal):** both NUTS-II appear in ancestry for any AML-attached leaf — even Setúbal's, via `setubal-municipio → aml → {nuts-ii-grande-lisboa, nuts-ii-peninsula-setubal}`.
- [x] **Cross-distrito metro membership (Setúbal município ∈ AML but distrito is Setúbal, not Lisboa):** Setúbal lookup walks `distrito-setubal` and AML; `distrito-lisboa` is correctly absent.
- [x] **Autonomous region as parallel top-level (Madeira → Funchal):** Funchal lookup walks `funchal-concelho → regiao-autonoma-madeira` and stops; no NUTS-II or mainland regions leak in.
- [x] **União de freguesias under same `pt:freguesia` kind (`lordelo-massarelos`):** loads cleanly, no special structural treatment needed.
- [x] **Postal code format extension:** PT case in `postal.go` round-trips `1100-001` ↔ `1100001` and rejects non-7-digit input. Same lookup hits identical orgs whether the user types the hyphen or not.
- [x] **National-tier filter in lookup:** `mubi-nacional` is filtered from every default lookup; `pt-nacional` never appears in `resolved_ancestry`. Confirmed via direct array membership check.
- [x] **US/CA behavior unchanged:** existing US/CA lookups return identical results (smoke-tested `11217 US` → Brooklyn local orgs unchanged); all pre-existing integration tests still pass.

**Unexpected finding (now fixed):** The HTTP handler (`api/internal/httpapi/lookup.go`) had a duplicate `country != US && country != CA` whitelist alongside the one in `loadpostal.go`. Removed in the same slice — the handler now treats `Country` as opaque, consistent with the rest of the stack. `TestLookup_InvalidCountry_ReturnsProblemJSON` renamed to `TestLookup_UnknownCountry_ReturnsNotFound` (404 fall-through instead of 400) and a new `TestLookup_MissingCountry_ReturnsProblemJSON` keeps the empty-country 400 path covered.

The model is ready for ES (#4.7) without further structural changes.

### 9. Roadmap follow-on

This spec includes a placeholder roadmap entry for **slice #4.7 — Second EU country (Spain) validation**: "Repeat the validation exercise for Spain. Adds `regions_es.toml`, `postal_codes_es.csv`, ~5 ES orgs. Specifically validates: autonomous communities (Catalonia, Basque Country with their own transit authorities), the comarca layer in some communities, and Ceuta/Melilla as the analogue of Açores/Madeira. Should be mostly mechanical given #4.6's conventions and loader changes."

If #4.6 exposes a model gap, #4.7 is the natural place to validate the fix.

## Out of scope (deliberate)

- Full Portuguese editorial dataset. The seed data is intentionally a probe, not a directory.
- Spain (queued as slice #4.7).
- Mexico, Netherlands, UK actual data (only forward-looking analysis in Section 7).
- A UI section or `?include=national` query param for surfacing national orgs (deferred — the model is ready; UX is a later slice).
- Reorganizing existing US/CA data to follow any new convention (US/CA already match the locked-in rules; no retroactive churn).
- Adding `international` as a fourth `scope_tier` (mentioned as future-proof; not implemented — wait for a real international org to need a home).
