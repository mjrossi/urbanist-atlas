import type { ReactNode } from 'react';

import { NewsroomCatGlyph } from './NewsroomCats.tsx';

/**
 * The "editor's note" card for a list with no entries, in the same
 * `.editors-note` style as the homepage aside.
 */
export function EmptyState({
  title,
  body,
  cta,
  className,
}: {
  /** Small-caps label above the body. */
  title: string;
  body: ReactNode;
  /** Rendered in its own `<p>` below `body`. */
  cta?: ReactNode;
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
