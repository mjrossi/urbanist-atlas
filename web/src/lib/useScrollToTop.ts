import { useEffect } from 'react';
import { useLocation } from 'react-router';

/**
 * On pathname change, snap the viewport back to the top. Plain React
 * Router has no built-in scroll restoration for client-side
 * navigations, so a long page (About) linking to a short page (Submit)
 * leaves the viewport stuck at the previous offset.
 *
 * When the destination URL carries a hash, defer to the browser's
 * native anchor scroll — clicking an "On this page" rail link should
 * land on the target, not the top.
 */
export function useScrollToTop(): void {
  const { pathname, hash } = useLocation();
  useEffect(() => {
    if (hash) return;
    window.scrollTo(0, 0);
  }, [pathname, hash]);
}
