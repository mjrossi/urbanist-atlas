// Self-hosted variable fonts. The @font-face rules these ship populate
// the family names ("Fraunces Variable", "Source Serif 4 Variable",
// "Inter Variable", "JetBrains Mono Variable") that the --font-* CSS
// variables in global.css reference.
import '@fontsource-variable/fraunces';
import '@fontsource-variable/source-serif-4';
import '@fontsource-variable/inter';
import '@fontsource-variable/jetbrains-mono';
import './styles/global.css';

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { RouterProvider } from 'react-router';

import { router } from './router.tsx';

// One QueryClient per browser session. `staleTime: 60_000` keeps the
// data fresh for a minute (the API is read-mostly), and `retry: 1`
// means one automatic retry on transient failures — the user can
// trigger more by re-running the search.
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 60_000,
      retry: 1,
    },
  },
});

const rootEl = document.getElementById('root');
if (!rootEl) {
  throw new Error('Root element #root not found in index.html');
}

createRoot(rootEl).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  </StrictMode>,
);
