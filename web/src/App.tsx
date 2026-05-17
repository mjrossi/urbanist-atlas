import { Outlet } from 'react-router';
import { BroadsheetNav } from './components/BroadsheetNav.tsx';
import { Footer } from './components/Footer.tsx';
import { Masthead } from './components/Masthead.tsx';

/**
 * Top-level layout shell. Mirrors the portfolio's `Base.astro`:
 * masthead, nav strip, the active route in `<main>`, footer at
 * the bottom. The router (see router.tsx) wires this in as the
 * parent route of every page.
 */
export function App() {
  return (
    <>
      <Masthead />
      <BroadsheetNav />
      <main>
        <Outlet />
      </main>
      <Footer />
    </>
  );
}
