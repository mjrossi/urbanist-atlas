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

interface ByLetter {
  letter: string;
  regions: RegionSummary[];
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

function groupForBrowse(all: ReadonlyArray<RegionSummary>): ByCountry[] {
  const byCountry: Record<string, RegionSummary[]> = {};
  for (const p of all) {
    const c = p.region.country;
    if (!byCountry[c]) byCountry[c] = [];
    byCountry[c].push(p);
  }
  return Object.entries(byCountry)
    .sort(([a], [b]) => (a === 'US' ? -1 : b === 'US' ? 1 : a.localeCompare(b)))
    .map(([country, regions]) => {
      const sorted = [...regions].sort((a, b) => a.region.name.localeCompare(b.region.name));
      const grouped: Record<string, RegionSummary[]> = {};
      for (const r of sorted) {
        const letter = letterOf(r.region.name);
        if (!grouped[letter]) grouped[letter] = [];
        grouped[letter].push(r);
      }
      const letters = Object.entries(grouped)
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([letter, regions]) => ({ letter, regions }));
      return { country, total: sorted, letters };
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
  const totalOrgs = query.data?.reduce((sum, p) => sum + p.org_count, 0) ?? null;

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
  const orgs = country.total.reduce((sum, p) => sum + p.org_count, 0);
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
        <LetterRow key={group.letter} letter={group.letter} regions={group.regions} />
      ))}
    </section>
  );
}

function LetterRow({ letter, regions }: { letter: string; regions: RegionSummary[] }) {
  return (
    <div className="index-letter-row" id={letter}>
      <div className="index-letter">
        {letter}
        <span className="meta">
          {regions.length} {regions.length === 1 ? 'region' : 'regions'}
        </span>
      </div>
      <div className="index-rows">
        {regions.map((p) => (
          <IndexRow key={p.region.slug} region={p} />
        ))}
      </div>
    </div>
  );
}

function IndexRow({ region: r }: { region: RegionSummary }) {
  const { region, org_count } = r;
  // Subtitle reads "<Country> · <Kind label>" so duplicate-looking
  // pairs like "Chicago Metro" and "Chicago" — which both surface
  // when a city has its own direct org attachments — read
  // distinctly without needing badges or separate sections.
  const subtitle = regionKindLabel(region.kind);
  return (
    <Link className="index-row" to={`/region/${region.slug}`}>
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
