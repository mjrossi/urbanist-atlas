import type { LookupOrg } from '../lib/api.ts';
import { pluralize } from '../lib/format.ts';
import { Entry } from './Entry.tsx';

/**
 * Classified-section list grouped into Local (city / county),
 * Regional (metro / CMA / regional-district / transit-federation /
 * multi-state), and State / Provincial (state / province) blocks.
 * Each section renders the broadsheet `.section-break` header (roman
 * numeral + title + entry count) and an inline org-entry list driven
 * by the shared `Entry` component.
 *
 * Empty sections render nothing — the section label only appears
 * when there's at least one entry. Roman numerals track the *visible*
 * sections in order (a results page with only Regional + Statewide
 * reads "I. Regional", "II. State / Provincial"), not a fixed
 * tier-to-numeral mapping. The Results page's empty-state copy lives
 * outside this component (the page renders an editors-note card when
 * ALL THREE buckets are empty).
 */
const ROMAN = ['I.', 'II.', 'III.'];

export function EntryList({
  local,
  regional,
  statewide,
  regionNameBySlug,
}: {
  local: LookupOrg[];
  regional: LookupOrg[];
  statewide: LookupOrg[];
  regionNameBySlug: Map<string, string>;
}) {
  const sections = [
    { title: 'Local', orgs: local },
    { title: 'Regional', orgs: regional },
    { title: 'State / Provincial', orgs: statewide },
  ].filter((s) => s.orgs.length > 0);

  return (
    <>
      {sections.map((s, i) => (
        <Section
          key={s.title}
          // sections is a filter of the 3 tiers, so i is always 0–2 and in
          // range; the `?? ''` only satisfies noUncheckedIndexedAccess.
          roman={ROMAN[i] ?? ''}
          title={s.title}
          orgs={s.orgs}
          regionNameBySlug={regionNameBySlug}
        />
      ))}
    </>
  );
}

function Section({
  roman,
  title,
  orgs,
  regionNameBySlug,
}: {
  roman: string;
  title: string;
  orgs: LookupOrg[];
  regionNameBySlug: Map<string, string>;
}) {
  return (
    <section className="org-section mt-32">
      <header className="section-break mt-0">
        <span className="num">{roman}</span>
        <h2 className="title">
          {title}
          <span className="accent">.</span>
        </h2>
        <span className="aside">
          {orgs.length} {pluralize(orgs.length, 'entry', 'entries')}
        </span>
      </header>
      {orgs.map((org) => (
        <Entry
          key={org.id}
          org={org}
          matchedRegionSlugs={org.matched_region_slugs}
          regionNameBySlug={regionNameBySlug}
        />
      ))}
    </section>
  );
}
