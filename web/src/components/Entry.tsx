import { Link } from 'react-router';
import type { Org } from '../lib/api.ts';
import { domainOf, prettyTag } from '../lib/format.ts';

/**
 * One row in the classified-section list. Renders the broadsheet
 * `.org-entry` treatment: org name (linked to its `/orgs/:slug`
 * detail page), the website domain as a secondary external link,
 * a short description, the org's tags as small pills, and — when
 * the caller passed `matchedRegionSlugs` — a "Matched via X · Y"
 * footer naming the regions that surfaced this org for the
 * current lookup. The footer is only rendered by the postal-code
 * Results page; Region pages omit it entirely.
 *
 * Why the props split: Results consumes `LookupOrg` (which carries
 * `matched_region_slugs`) and a hydrated slug → display-name map
 * built from the resolved-ancestry walk; Region consumes plain
 * `Org` and has no lookup context. Keeping the via-rendering bits
 * optional lets both pages share one component without lying
 * about each one's data shape.
 */
export function Entry({
  org,
  matchedRegionSlugs,
  regionNameBySlug,
}: {
  org: Org;
  /**
   * Slugs whose membership caused `org` to surface for the current
   * lookup. Omit on pages that aren't a postal-code lookup; the
   * "Matched via X" footer is suppressed when this is undefined or
   * empty.
   */
  matchedRegionSlugs?: ReadonlyArray<string>;
  /**
   * Slug → display-name map for the matched-via footer. When a slug
   * is missing from the map, the raw slug renders.
   */
  regionNameBySlug?: Map<string, string>;
}) {
  const domain = domainOf(org.website_url);
  const matchedNames =
    matchedRegionSlugs && matchedRegionSlugs.length > 0
      ? matchedRegionSlugs
          .map((slug) => regionNameBySlug?.get(slug) ?? slug)
          .join(' · ')
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
