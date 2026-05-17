import type { Org } from '../lib/api.ts';
import { TagChip } from './TagChip.tsx';

/**
 * One row in the classified-section list: org name (linked out to
 * `website_url`), short description, and the org's tags as small
 * pills.
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

export function Entry({ org }: { org: Org }) {
  const domain = domainOf(org.website_url);
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
