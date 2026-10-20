import type { ReactNode, RefObject } from 'react';
import { useEffect, useRef } from 'react';
import {
  NavigationType,
  ScrollRestoration,
  useLocation,
  useNavigationType,
} from 'react-router';

import { BroadsheetNav } from './BroadsheetNav.tsx';
import { Footer } from './Footer.tsx';
import { Masthead } from './Masthead.tsx';

const MAIN_ID = 'main';

/**
 * The broadsheet "sheet" chrome shared by every full-page view: skip
 * link, masthead, primary nav, a `<main>` content well, and the footer.
 * The error and 404 pages render this themselves, because React Router
 * swaps the whole element tree for an `errorElement`.
 */
export function SheetLayout({ children }: { children: ReactNode }) {
  const mainRef = useRef<HTMLElement>(null);
  useFocusOnNavigation(mainRef);
  return (
    <div className="sheet">
      <ScrollRestoration />
      <a
        className="skip-link"
        href={`#${MAIN_ID}`}
        onClick={(event) => {
          // A real fragment navigation would add #main to the URL and to
          // history; moving focus is all the reader asked for.
          event.preventDefault();
          mainRef.current?.focus();
        }}
      >
        Skip to content
      </a>
      <Masthead />
      <BroadsheetNav />
      <main id={MAIN_ID} ref={mainRef} tabIndex={-1}>
        {children}
      </main>
      <Footer />
    </div>
  );
}

/**
 * After a client-side navigation, focus the new page's `<h1>` so screen
 * readers announce the page and keyboard focus starts at the content.
 * Pages that render their heading after a query resolves get `<main>`
 * first, then the heading once it mounts, unless the reader has moved
 * focus in the meantime.
 */
function useFocusOnNavigation(mainRef: RefObject<HTMLElement | null>) {
  const { pathname } = useLocation();
  const navigationType = useNavigationType();
  // A POP on mount is the document's first load (or a reload), where
  // focus belongs to the browser. A PUSH on mount is a navigation that
  // landed on the error page, which renders its own SheetLayout.
  const focusedForRef = useRef<string | null>(
    navigationType === NavigationType.Pop ? pathname : null,
  );

  useEffect(() => {
    const main = mainRef.current;
    if (!main || focusedForRef.current === pathname) return;
    focusedForRef.current = pathname;

    const heading = main.querySelector('h1');
    if (heading) {
      focusWithoutScroll(heading);
      return;
    }
    focusWithoutScroll(main);
    const observer = new MutationObserver(() => {
      const late = main.querySelector('h1');
      if (!late) return;
      observer.disconnect();
      if (document.activeElement === main) focusWithoutScroll(late);
    });
    observer.observe(main, { childList: true, subtree: true });
    return () => {
      observer.disconnect();
    };
  }, [pathname, mainRef]);
}

function focusWithoutScroll(el: HTMLElement) {
  if (!el.hasAttribute('tabindex')) el.setAttribute('tabindex', '-1');
  // ScrollRestoration owns the scroll position.
  el.focus({ preventScroll: true });
}
