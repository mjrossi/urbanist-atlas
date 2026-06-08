// Self-hosted variable fonts. The @font-face rules these ship populate
// the family names ("Fraunces Variable", "Source Serif 4 Variable",
// "Inter Variable", "JetBrains Mono Variable") that the --font-* CSS
// variables in global.css reference.
import '@fontsource-variable/fraunces';
import '@fontsource-variable/source-serif-4';
import '@fontsource-variable/inter';
import '@fontsource-variable/jetbrains-mono';
import './styles/global.css';

import {
  MutationCache,
  QueryCache,
  QueryClient,
  QueryClientProvider,
} from '@tanstack/react-query';
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { RouterProvider } from 'react-router';

import { installGlobalErrorLogging, reportClientError } from './lib/clientErrors.ts';
import { router } from './router.tsx';

// Surface render-tree-escaping failures (async callbacks, rejected
// promises) in the dev console with any request id attached. No-ops in
// production — there is no client error-tracking SaaS by design.
installGlobalErrorLogging();

// One QueryClient per browser session. `staleTime: 60_000` keeps the
// data fresh for a minute (the API is read-mostly), and `retry: 1`
// means one automatic retry on transient failures — the user can
// trigger more by re-running the search. The cache-level `onError`
// hooks give one central place to log every query/mutation failure
// (with its request id) for dev debugging; components still render the
// error via QueryState.
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 60_000,
      retry: 1,
    },
  },
  queryCache: new QueryCache({
    onError: (error) => {
      reportClientError('query', error);
    },
  }),
  mutationCache: new MutationCache({
    onError: (error) => {
      reportClientError('mutation', error);
    },
  }),
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
