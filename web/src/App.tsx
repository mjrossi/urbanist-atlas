import { Outlet } from 'react-router';
import { BroadsheetNav } from './components/BroadsheetNav.tsx';
import { Footer } from './components/Footer.tsx';
import { Masthead } from './components/Masthead.tsx';

export function App() {
  return (
    <div className="sheet">
      <Masthead />
      <BroadsheetNav />
      <main>
        <Outlet />
      </main>
      <Footer />
    </div>
  );
}
