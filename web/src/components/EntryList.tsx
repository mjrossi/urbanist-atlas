import type { LookupOrg } from '../lib/api.ts';
import { Entry } from './Entry.tsx';

/**
 * Classified-section list grouped into Local (city / county) and
 * Regional (metro / state / province / multi-state) blocks. Each
 * section renders the broadsheet `.section-break` header (roman
 * numeral + title + entry count) and an inline org-entry list
 * driven by the shared `Entry` component.
 *
 * Empty sections render nothing — the section label only appears
 * when there's at least one entry. The Results page's empty-state
 * copy lives outside this component (the page renders an
 * editors-note card when BOTH buckets are empty).
 */
export function EntryList({
  local,
  regional,
  regionNameBySlug,
}: {
  local: LookupOrg[];
  regional: LookupOrg[];
  regionNameBySlug: Map<string, string>;
}) {
  return (
    <>
      <Section
        roman="I."
        title="Local"
        orgs={local}
        regionNameBySlug={regionNameBySlug}
      />
      <Section
        roman="II."
        title="Regional"
        orgs={regional}
        regionNameBySlug={regionNameBySlug}
      />
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
  if (orgs.length === 0) return null;
  return (
    <section className="org-section" style={{ marginTop: 32 }}>
      <header className="section-break" style={{ marginTop: 0 }}>
        <span className="num">{roman}</span>
        <h2 className="title">
          {title}
          <span className="accent">.</span>
        </h2>
        <span className="aside">
          {orgs.length} {orgs.length === 1 ? 'entry' : 'entries'}
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
