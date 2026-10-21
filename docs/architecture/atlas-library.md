# The `pkg/atlas` library

This note holds the design reasons behind `api/pkg/atlas` that are too
long for code comments. For the package layout and the library-first
split, see [`docs/api-architecture.md`](../api-architecture.md). For
the region graph, kind sets and editorial conventions, see
[`docs/region-graph.md`](../region-graph.md).

## Lookup and GetRegion

`Lookup` and `GetRegion` are two orchestrators built on the same
`Store` methods. They ask opposite questions, so they walk the region
graph in opposite directions.

- **`Lookup` asks "what works at this address?"** It resolves a postal
  code to its leaf region and walks **up** through `AncestorRegions`. A
  Brooklyn ZIP is also in New York City, New York Metro and New York
  State, so orgs attached to any of those surface.
- **`GetRegion` asks "what does this region contain?"** It walks
  **down** through `DescendantRegions`. A metro shows its cities' orgs,
  and a state shows every metro and city beneath it. Ancestor orgs are
  not pulled in, so a city's page isn't flooded with the state's
  catch-all coalitions. `GetRegion` still calls `AncestorRegions`, but
  only to build the breadcrumb (`RegionDetail.Ancestry`). It never uses
  the result for org scope.

Both orchestrators pass their in-scope set to `BucketOrgsByScope`, so
an org lands in the same Local, Regional or Statewide tier on either
surface.

The downward in-scope set matches the subtree that `ListRegions` counts
for `OrgCount`. The count on a browse card is therefore the count on
that region's detail page. Rollups (below) don't break this, because
rollup targets are state-equivalent kinds, and browse doesn't list
those.

`GetRegion` returns `(nil, nil)` for an unknown or national-tier slug,
and the HTTP layer turns that into a 404. The wire contract does not
distinguish the two cases.

## rollup_states

A stateless multi-state metro, such as `chicago-metro` under the
`chicagoland` coalition, has no state parent.
[`region-graph.md`](../region-graph.md) §1 explains why: a state edge
on the metro would let a ZIP on one side of the border pull in the
other state's orgs. Without some extra link, though, the metro's own
orgs would never appear on the Illinois page.

`Region.RollupStates` supplies that link. It names the rollup targets
(`IsRollupTargetKind`, which covers the state-equivalent kinds plus
`us:federal-district`) on whose detail pages the region's **own** orgs
also appear, in the Regional bucket.

- **It is not a graph edge.** It is never added to the parent map,
  cycle detection, or the ancestor walk. The seed loader turns it into
  a separate state → metros index (`MemStore.AddRollupState`) after
  every region is registered. Only `RollupMetrosFor` reads that index,
  and only `GetRegion` calls `RollupMetrosFor`. So a rollup can never
  leak orgs into a `/lookup`. The storetest suite
  (`AncestorRegions_RollupState_NoLeak`) pins this.
- **It adds the metro node only.** The metro's descendants are not
  added, so an out-of-state leaf under the metro (a county in
  Indiana, say) stays off the Illinois page
  (`GetRegion_RollupMetro_SurfacesOnStatePage`).
- **Rolled-up metros are named in `DescendantRegionNames`.** They can
  appear in an org's `matched_region_slugs` without being descendants,
  and the SPA needs their names to label "matched via …".

DC is a rollup target but not a state-equivalent kind. A DMV org still
buckets Regional, and the bi-state Washington metro can still surface
its own orgs on `/dc`.

## Stats

`GET /api/v1/stats` exists because the atlas-wide totals **cannot** be
derived from `ListRegions`. That endpoint returns only
`defaultBrowseKinds` (metros and cities). If you sum
`RegionSummary.DirectOrgCount` over it, you drop every org attached
only to a state, province, borough or multi-state coalition. A sum
also counts an org twice when it is attached to two browse regions.
The two errors can cancel out on a small fixture and give a total that
looks plausible but is wrong, which is why the storetest case checks
by org identity rather than by arithmetic.

`MemStore.Stats` therefore counts distinct orgs over the store's own
org list. It uses the same "at least one non-national attachment" rule
as `ListRecent`, so the two surfaces agree on what counts as a live
org. `BrowseRegionCount` is computed from `listRegionsLocked`, the
same function behind `ListRegions`, so there is one browse predicate
and the count cannot drift from the list the SPA renders.

The per-country rows are attributions, not a partition:

- An org is credited to every country where it has a non-national
  attachment. An org spanning the border would appear in two rows, so
  the `OrgCount` column could sum to more than `TotalOrgCount`. No such
  org exists in the v1 seed, but the contract permits one.
- A region or org with a blank `Country` still counts toward the
  totals but has no country row. The seed loader always stamps
  `Country`, so this happens only in hand-built fixtures.

## MemStore read paths

- **Tags are cloned on read.** `OrgsForRegions`, `GetOrgBySlug` and
  `ListRecent` return a copy of `Tags`. Without the copy, a caller
  mutating a returned slice would write into the store's backing
  array, and the store is only read-locked on those paths. A nil
  `Tags` stays nil, which keeps the wire shape. The storetest case
  `ReadPaths_CloneTagsNoAlias` pins this for any future `Store`.
- **There is no approval gate.** `Org` has no status field, so
  `MemStore` can't filter on approval. The seed bundle and
  `LoadDevFixtures` contain only approved orgs, and hand-built
  fixtures should do the same.
- **Indexes are built per call.** `ListRegions` builds the
  parent → children map and the region → orgs index once per call,
  not once per region. That keeps the call linear in regions, parent
  edges and attachments, rather than quadratic. The indexes are
  allocated fresh instead of being cached on the struct, so they are
  safe to build under a read lock.
