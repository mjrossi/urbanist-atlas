import type { LookupOrg } from '../lib/api.ts';
import { TagChip } from './TagChip.tsx';

/**
 * One row in the classified-section list: org name (linked out to
 * `website_url`), short description, the org's tags as small pills,
 * and a "via X" subtitle naming the matched region(s) that caused
 * this org to surface for the current lookup.
 *
 * The domain is rendered next to the name as a visual hint that the
 * primary link leaves the site; we derive it from `website_url`
 * defensively so a malformed URL doesn't blow up the page.
 */
function domainOf(url: string): string | null {
  try {
    return new URL(url).hostname.replace(/^www\./, '');
  } catch {
    return null;
  }
}

export function Entry({
  org,
  regionNameBySlug,
}: {
  org: LookupOrg;
  regionNameBySlug: Map<string, string>;
}) {
  const domain = domainOf(org.website_url);

  const viaNames = org.matched_region_slugs
    .map((slug) => regionNameBySlug.get(slug) ?? slug)
    .join(', ');

  return (
    <li className="entry">
      <div className="entry-header">
        <h3 className="entry-name">
          <a href={org.website_url} target="_blank" rel="noopener noreferrer">
            {org.name}
          </a>
        </h3>
        {domain ? <span className="entry-domain">{domain}</span> : null}
      </div>
      {viaNames ? <p className="entry-via">via {viaNames}</p> : null}
      <p className="entry-desc">{org.short_desc}</p>
      {org.tags.length > 0 ? (
        <ul className="entry-tags" aria-label="Tags">
          {org.tags.map((tag) => (
            <li key={tag}>
              <TagChip label={tag} />
            </li>
          ))}
        </ul>
      ) : null}
    </li>
  );
}
