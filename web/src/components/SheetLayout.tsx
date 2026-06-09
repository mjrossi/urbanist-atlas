import type { ReactNode } from 'react';

import { useScrollToTop } from '../lib/useScrollToTop.ts';
import { BroadsheetNav } from './BroadsheetNav.tsx';
import { Footer } from './Footer.tsx';
import { Masthead } from './Masthead.tsx';

/**
 * The broadsheet "sheet" chrome shared by every full-page view: the
 * masthead, the primary nav, a `<main>` content well, and the footer.
 *
 * Owns `useScrollToTop` so scroll restoration is identical across the
 * normal routed pages (via {@link App}'s `<Outlet>`) and the error /
 * 404 pages, which render INSTEAD of `App` (React Router swaps the
 * whole element tree for an `errorElement`) and so can't lean on App's
 * Outlet — they wrap their content here instead.
 */
export function SheetLayout({ children }: { children: ReactNode }) {
  useScrollToTop();
  return (
    <div className="sheet">
      <Masthead />
      <BroadsheetNav />
      <main>{children}</main>
      <Footer />
    </div>
  );
}
