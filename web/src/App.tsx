import { Outlet } from 'react-router';

import { SheetLayout } from './components/SheetLayout.tsx';

export function App() {
  return (
    <SheetLayout>
      <Outlet />
    </SheetLayout>
  );
}
