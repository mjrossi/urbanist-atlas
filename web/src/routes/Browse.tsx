import { useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router';
import { ApiError, listMetros } from '../lib/api.ts';
import type { MetroSummary } from '../lib/api.ts';
import { queryKeys } from '../lib/queryKeys.ts';
import { useDocumentTitle } from '../lib/useDocumentTitle.ts';

const ALPHABET = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ'.split('');

const COUNTRY_TITLES: Record<string, string> = {
  US: 'United States',
  CA: 'Canada',
};

interface ByLetter {
  letter: string;
  metros: MetroSummary[];
}

interface ByCountry {
  country: string;
  total: MetroSummary[];
  letters: ByLetter[];
}

function letterOf(name: string): string {
  const m = name.trim().toUpperCase().match(/[A-Z]/);
  return m ? m[0] : '#';
}

function groupForBrowse(all: ReadonlyArray<MetroSummary>): ByCountry[] {
  const byCountry: Record<string, MetroSummary[]> = {};
  for (const m of all) {
    const c = m.region.country;
    if (!byCountry[c]) byCountry[c] = [];
    byCountry[c].push(m);
  }
  return Object.entries(byCountry)
    .sort(([a], [b]) => (a === 'US' ? -1 : b === 'US' ? 1 : a.localeCompare(b)))
    .map(([country, metros]) => {
      const sorted = [...metros].sort((a, b) => a.region.name.localeCompare(b.region.name));
      const grouped: Record<string, MetroSummary[]> = {};
      for (const m of sorted) {
        const letter = letterOf(m.region.name);
        if (!grouped[letter]) grouped[letter] = [];
        grouped[letter].push(m);
      }
      const letters = Object.entries(grouped)
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([letter, metros]) => ({ letter, metros }));
      return { country, total: sorted, letters };
    });
}

export function Browse() {
  useDocumentTitle('Browse metros — Urbanist Atlas');
  const query = useQuery<MetroSummary[], ApiError>({
    queryKey: queryKeys.metros(),
    queryFn: ({ signal }) => listMetros({ signal }),
  });

  const grouped = useMemo(() => groupForBrowse(query.data ?? []), [query.data]);
  const availableLetters = useMemo(() => {
    const s = new Set<string>();
    for (const g of grouped) for (const l of g.letters) s.add(l.letter);
    return s;
  }, [grouped]);

  const totalMetros = query.data?.length ?? null;
  const totalOrgs = query.data?.reduce((sum, m) => sum + m.org_count, 0) ?? null;

  return (
    <>
      <div className="kicker">
        <div>
          <Link to="/">Atlas</Link>
          <span className="crumb-sep">/</span>
          <span className="crumb-here">The index</span>
        </div>
        <div>
          {totalMetros !== null && totalOrgs !== null
            ? `${totalMetros} metros · ${totalOrgs}+ org entries`
            : 'The index'}
        </div>
      </div>

      <div className="spread lede-first" style={{ marginTop: 48 }}>
        <div className="lede" style={{ marginBottom: 0 }}>
          <div className="eyebrow">
            § The index<span className="eyebrow-rule" />
          </div>
          <h1>
            Every metro <span className="accent">we&rsquo;ve indexed</span> —
            alphabetical.
          </h1>
          <p className="deck">
            Useful when you want to wander rather than search. Open a metro for
            the groups working there, or jump to a letter on the strip below.
          </p>
        </div>
        <div className="rail-block muted" style={{ marginTop: 12 }}>
          <div className="rail-kicker">Sorting</div>
          <p>
            Metros are sorted alphabetically within each country. The number in
            italic is the count of organizations the Atlas currently lists for
            that region.
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
          We index a metro once we&rsquo;ve verified at least one active group
          working there, and we add new entries as editors get to them. If your
          city is missing, tell us where to look —{' '}
          <Link to="/submit">file a tip at the submissions desk</Link>.
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
  query: ReturnType<typeof useQuery<MetroSummary[], ApiError>>;
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
    return <p className="results-state">No metros indexed yet.</p>;
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
  const orgs = country.total.reduce((sum, m) => sum + m.org_count, 0);
  const name = COUNTRY_TITLES[country.country] ?? country.country;
  return (
    <section className="country-section">
      <header className="country-head">
        <h2 className="cname">{name}</h2>
        <div className="crule" />
        <div className="cnum">
          <span className="em">{total}</span> metros ·{' '}
          <span className="em">{orgs}+</span> orgs
        </div>
      </header>
      {country.letters.map((group) => (
        <LetterRow key={group.letter} letter={group.letter} metros={group.metros} />
      ))}
    </section>
  );
}

function LetterRow({ letter, metros }: { letter: string; metros: MetroSummary[] }) {
  const half = Math.ceil(metros.length / 2);
  const colA = metros.slice(0, half);
  const colB = metros.slice(half);
  return (
    <div className="index-letter-row" id={letter}>
      <div className="index-letter">
        {letter}
        <span className="meta">
          {metros.length} {metros.length === 1 ? 'metro' : 'metros'}
        </span>
      </div>
      <div>
        {colA.map((m) => (
          <IndexRow key={m.region.slug} metro={m} />
        ))}
      </div>
      <div>
        {colB.map((m) => (
          <IndexRow key={m.region.slug} metro={m} />
        ))}
      </div>
    </div>
  );
}

function IndexRow({ metro }: { metro: MetroSummary }) {
  const { region, org_count } = metro;
  const subtitle = region.parent_slugs[0] ?? region.country.toLowerCase();
  return (
    <Link className="index-row" to={`/m/${region.slug}`}>
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
