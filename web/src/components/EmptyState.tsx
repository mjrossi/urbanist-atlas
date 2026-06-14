import type { ReactNode } from 'react';

import { NewsroomCatGlyph } from './NewsroomCats.tsx';

/**
 * Broadsheet "editor's note" card used wherever a list page resolves
 * to no entries: a small framed block with a short label, a one-
 * sentence body, and an optional call-to-action (typically a link
 * to the submissions desk).
 *
 * Renders the same `.editors-note` treatment the homepage already
 * uses for the editor's-note aside, so the visual language doesn't
 * fork between "we have nothing here" and "here's a sidebar".
 *
 * Call sites — Results, Region, Browse, Home — pass copy that
 * names the place or kind of thing missing; this component owns
 * none of that domain logic. The one piece of copy it does own is
 * the newsroom-cat sign-off below the card body — pure editorial
 * chrome, naming no place or domain concept.
 */
export function EmptyState({
  title,
  body,
  cta,
  className,
}: {
  /** Small-caps label above the body. */
  title: string;
  /** One- to two-sentence explanation; rendered as the card body. */
  body: ReactNode;
  /**
   * Optional call-to-action slot — usually a `<Link>` to the
   * submissions desk. Rendered inside its own `<p>` below `body`.
   */
  cta?: ReactNode;
  /**
   * Extra utility classes — pages often need spacing modifiers like
   * `mt-24` / `mt-48` to fit into their surrounding layout.
   */
  className?: string;
}) {
  const classes = ['editors-note', className].filter(Boolean).join(' ');
  return (
    <div className={classes}>
      <div className="label">{title}</div>
      <p>{body}</p>
      {cta ? <p>{cta}</p> : null}
      <div className="editors-note-cat">
        <NewsroomCatGlyph className="cat-glyph" />
        <span>The newsroom cat has checked under every desk.</span>
      </div>
    </div>
  );
}
