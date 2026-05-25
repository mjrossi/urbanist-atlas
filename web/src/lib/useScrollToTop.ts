import { useEffect } from 'react';
import { useLocation } from 'react-router';

/**
 * On client-side navigation, restore the viewport in the way the user
 * expects. Plain React Router has no built-in scroll restoration, so
 * we drive it from the location:
 *
 *   - URL has a hash → wait for the new route's content to commit,
 *     then `scrollIntoView` on the matching element. Native hash
 *     scroll fails on an SPA because the target element isn't in
 *     the DOM at the moment the URL changes.
 *
 *   - No hash → snap to the top. A long page (e.g., About) linking
 *     to a shorter page (e.g., Submit) would otherwise leave the
 *     viewport stuck at the previous offset.
 */
export function useScrollToTop(): void {
  const { pathname, hash } = useLocation();
  useEffect(() => {
    if (hash) {
      const id = decodeURIComponent(hash.slice(1));
      const scrollToTarget = () => {
        const el = document.getElementById(id);
        if (el) {
          el.scrollIntoView({ block: 'start' });
          return true;
        }
        return false;
      };
      if (scrollToTarget()) return;
      // The target isn't mounted yet — try once more after the next
      // paint, which catches routes that commit content on a second
      // render (e.g., after a useQuery resolves).
      const raf = requestAnimationFrame(() => {
        scrollToTarget();
      });
      return () => cancelAnimationFrame(raf);
    }
    window.scrollTo(0, 0);
  }, [pathname, hash]);
}
