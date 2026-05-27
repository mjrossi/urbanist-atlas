import { useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router';
import { ApiError, listRegions } from '../lib/api.ts';
import type { RegionSummary } from '../lib/api.ts';
import { queryKeys } from '../lib/queryKeys.ts';
import { useDocumentTitle } from '../lib/useDocumentTitle.ts';
import { regionKindLabel } from '../lib/regionKind.ts';

const ALPHABET = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ'.split('');

const COUNTRY_TITLES: Record<string, string> = {
  US: 'United States',
  CA: 'Canada',
};

interface AnchorWithChildren {
  anchor: RegionSummary;
  children: RegionSummary[];
}

interface ByLetter {
  letter: string;
  anchors: AnchorWithChildren[];
}

interface ByCountry {
  country: string;
  total: RegionSummary[];
  letters: ByLetter[];
}

function letterOf(name: string): string {
  const m = name.trim().toUpperCase().match(/[A-Z]/);
  return m ? m[0] : '#';
}

/**
 * Groups the flat /regions response into the broadsheet's
 * country → letter → anchor → children shape. Cities whose
 * `browse_parent_slug` points to a visible anchor (metro, CMA,
 * etc.) render nested beneath that anchor; cities with no
 * browseable ancestor render as their own anchor rows.
 *
 * The anchor letter wins for grouping — Hoboken (parent: NYC
 * Metro) nests under New York Metro in the "N" section, not under
 * its own "H" section. For the v1 seed every parent/child pair
 * shares a first letter so this is moot in practice, but the
 * convention reads cleaner than splitting children across letters.
 */
function groupForBrowse(all: ReadonlyArray<RegionSummary>): ByCountry[] {
  const slugSet = new Set<string>(all.map((r) => r.region.slug));

  const byCountry: Record<string, RegionSummary[]> = {};
  for (const p of all) {
    const c = p.region.country;
    if (!byCountry[c]) byCountry[c] = [];
    byCountry[c].push(p);
  }
  return Object.entries(byCountry)
    .sort(([a], [b]) => (a === 'US' ? -1 : b === 'US' ? 1 : a.localeCompare(b)))
    .map(([country, regions]) => {
      // Children: regions whose browse_parent_slug points at a
      // sibling in the visible set. Anchors: everything else.
      const childrenByAnchor = new Map<string, RegionSummary[]>();
      const anchors: RegionSummary[] = [];
      for (const r of regions) {
        const parentSlug = r.browse_parent_slug;
        if (parentSlug && slugSet.has(parentSlug)) {
          const arr = childrenByAnchor.get(parentSlug) ?? [];
          arr.push(r);
          childrenByAnchor.set(parentSlug, arr);
        } else {
          anchors.push(r);
        }
      }
      // Sort children alphabetically within each anchor.
      for (const arr of childrenByAnchor.values()) {
        arr.sort((a, b) => a.region.name.localeCompare(b.region.name));
      }
      // Sort anchors alphabetically (within-letter rule); letter-group.
      anchors.sort((a, b) => a.region.name.localeCompare(b.region.name));
      const grouped: Record<string, AnchorWithChildren[]> = {};
      for (const a of anchors) {
        const letter = letterOf(a.region.name);
        if (!grouped[letter]) grouped[letter] = [];
        grouped[letter].push({
          anchor: a,
          children: childrenByAnchor.get(a.region.slug) ?? [],
        });
      }
      const letters = Object.entries(grouped)
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([letter, anchors]) => ({ letter, anchors }));
      return { country, total: regions, letters };
    });
}

export function Browse() {
  useDocumentTitle('Browse the atlas — Urbanist Atlas');
  const query = useQuery<RegionSummary[], ApiError>({
    queryKey: queryKeys.regions(),
    queryFn: ({ signal }) => listRegions({ signal }),
  });

  const grouped = useMemo(() => groupForBrowse(query.data ?? []), [query.data]);
  const availableLetters = useMemo(() => {
    const s = new Set<string>();
    for (const g of grouped) for (const l of g.letters) s.add(l.letter);
    return s;
  }, [grouped]);

  const totalRegions = query.data?.length ?? null;
  // Sum direct_org_count (orgs attached directly to each row, no
  // descendant walk) so the header reads a deduped total. Summing
  // org_count would double-count orgs that surface under both a
  // metro and one of its child cities.
  const totalOrgs =
    query.data?.reduce((sum, p) => sum + p.direct_org_count, 0) ?? null;

  return (
    <>
      <div className="kicker">
        <div>
          <Link to="/">Atlas</Link>
          <span className="crumb-sep">/</span>
          <span className="crumb-here">The index</span>
        </div>
        <div>
          {totalRegions !== null && totalOrgs !== null
            ? `${totalRegions} regions · ${totalOrgs} org entries`
            : 'The index'}
        </div>
      </div>

      <div className="spread lede-first" style={{ marginTop: 48 }}>
        <div className="lede" style={{ marginBottom: 0 }}>
          <div className="eyebrow">
            § The index<span className="eyebrow-rule" />
          </div>
          <h1>
            Every metro and city <span className="accent">we&rsquo;ve indexed</span> —
            alphabetical.
          </h1>
          <p className="deck">
            Useful when you want to wander rather than search. Open a region
            for the groups working there, or jump to a letter on the strip
            below. Big cities appear alongside their parent metro — clicking
            the city shows only its own groups, while clicking the metro
            pulls in everything across the broader region.
          </p>
        </div>
        <div className="rail-block muted" style={{ marginTop: 12 }}>
          <div className="rail-kicker">Sorting</div>
          <p>
            Regions are sorted alphabetically within each country. The number
            in italic is the count of organizations the Atlas currently lists
            for that region (the metro count includes orgs tagged to its
            cities and counties via the region graph).
          </p>
          <p style={{ marginBottom: 0 }}>
            Searching by ZIP or postal code?{' '}
            <Link to="/">Use the front-page lookup</Link>.
          </p>
        </div>
      </div>

      <BrowseBody query={query} grouped={grouped} availableLetters={availableLetters} />

      <section className="editors-note" style={{ marginTop: 56 }}>
        <div className="label">Don&rsquo;t see your region?</div>
        <p>
          If your region is missing,{' '}
          <Link to="/submit">file a tip at the submissions desk</Link>. See{' '}
          <Link to="/about#methodology">how we curate</Link>.
        </p>
      </section>
    </>
  );
}

function BrowseBody({
  query,
  grouped,
  availableLetters,
}: {
  query: ReturnType<typeof useQuery<RegionSummary[], ApiError>>;
  grouped: ByCountry[];
  availableLetters: Set<string>;
}) {
  if (query.isPending) {
    return <p className="results-state" role="status">Loading the index…</p>;
  }
  if (query.isError) {
    return (
      <p className="results-state error" role="alert">
        {query.error.message}
      </p>
    );
  }
  if (grouped.length === 0) {
    return <p className="results-state">No regions indexed yet.</p>;
  }
  return (
    <>
      <div className="az-index" aria-label="Jump to letter">
        <span className="az-label">Jump to</span>
        {ALPHABET.map((letter) =>
          availableLetters.has(letter) ? (
            <a key={letter} href={`#${letter}`}>
              {letter}
            </a>
          ) : (
            <span key={letter} className="dim">
              {letter}
            </span>
          ),
        )}
      </div>
      {grouped.map((country) => (
        <CountrySection key={country.country} country={country} />
      ))}
    </>
  );
}

function CountrySection({ country }: { country: ByCountry }) {
  const total = country.total.length;
  // Same as the page total: direct_org_count avoids double-counting
  // orgs that surface under both a metro and its child cities.
  const orgs = country.total.reduce((sum, p) => sum + p.direct_org_count, 0);
  const name = COUNTRY_TITLES[country.country] ?? country.country;
  return (
    <section className="country-section">
      <header className="country-head">
        <h2 className="cname">{name}</h2>
        <div className="crule" />
        <div className="cnum">
          <span className="em">{total}</span> regions ·{' '}
          <span className="em">{orgs}</span> orgs
        </div>
      </header>
      {country.letters.map((group) => (
        <LetterRow key={group.letter} letter={group.letter} anchors={group.anchors} />
      ))}
    </section>
  );
}

function LetterRow({
  letter,
  anchors,
}: {
  letter: string;
  anchors: AnchorWithChildren[];
}) {
  const totalRows = anchors.reduce((sum, a) => sum + 1 + a.children.length, 0);
  return (
    <div className="index-letter-row" id={letter}>
      <div className="index-letter">
        {letter}
        <span className="meta">
          {totalRows} {totalRows === 1 ? 'region' : 'regions'}
        </span>
      </div>
      <div className="index-rows">
        {anchors.map((a) => (
          <div key={a.anchor.region.slug} className="index-anchor-group">
            <IndexRow region={a.anchor} />
            {a.children.map((c) => (
              <IndexRow key={c.region.slug} region={c} isChild />
            ))}
          </div>
        ))}
      </div>
    </div>
  );
}

function IndexRow({ region: r, isChild }: { region: RegionSummary; isChild?: boolean }) {
  const { region, org_count } = r;
  // Subtitle reads "<Country> · <Kind label>" so adjacent rows like
  // "Chicago Metro" and "Chicago" — when a city is nested beneath
  // its parent metro — still read distinctly via the kind label.
  const subtitle = regionKindLabel(region.kind);
  const className = isChild ? 'index-row child' : 'index-row';
  return (
    <Link className={className} to={`/region/${region.slug}`}>
      <div>
        <span className="iname">{region.name}</span>
        <span className="imeta">
          {region.country} · {subtitle}
        </span>
      </div>
      <span className="icount">
        {org_count} <span className="total">{org_count === 1 ? 'group' : 'groups'}</span>
      </span>
    </Link>
  );
}
