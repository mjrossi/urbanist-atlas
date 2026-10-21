import { Link } from 'react-router';

import type { Org } from '../lib/api.ts';
import { domainOf, prettyTag } from '../lib/format.ts';

/**
 * One `.org-entry` row in a classified-section list. A non-empty
 * `matchedRegionSlugs` adds a "Matched via X · Y" footer; a slug missing
 * from `regionNameBySlug` renders raw.
 */
export function Entry({
  org,
  matchedRegionSlugs,
  regionNameBySlug,
}: {
  org: Org;
  matchedRegionSlugs?: readonly string[];
  regionNameBySlug?: Map<string, string>;
}) {
  const domain = domainOf(org.website_url);
  const matchedNames =
    matchedRegionSlugs && matchedRegionSlugs.length > 0
      ? matchedRegionSlugs.map((slug) => regionNameBySlug?.get(slug) ?? slug).join(' · ')
      : null;

  return (
    <article className="org-entry">
      <div>
        <div className="head">
          <h3 className="name">
            <Link to={`/orgs/${encodeURIComponent(org.slug)}`}>{org.name}</Link>
          </h3>
        </div>
        {org.website_url ? (
          <div className="url">
            <a href={org.website_url} target="_blank" rel="noopener noreferrer">
              {domain ?? org.website_url}
            </a>
          </div>
        ) : null}
        <p className="desc">{org.short_desc}</p>
        {org.tags.length > 0 ? (
          <ul className="tag-list">
            {org.tags.map((tag) => (
              <li key={tag}>
                <span className="tag">{prettyTag(tag)}</span>
              </li>
            ))}
          </ul>
        ) : null}
        {matchedNames ? (
          <div className="foot">
            <span className="via">
              Matched via <span className="em">{matchedNames}</span>
            </span>
          </div>
        ) : null}
      </div>
    </article>
  );
}
