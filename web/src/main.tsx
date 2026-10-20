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

import { shouldRetry } from './lib/api.ts';
import { installGlobalErrorLogging, reportClientError } from './lib/clientErrors.ts';
import { router } from './router.tsx';

// Surface render-tree-escaping failures (async callbacks, rejected
// promises) in the dev console with any request id attached. No-ops in
// production — there is no client error-tracking SaaS by design.
installGlobalErrorLogging();

// `staleTime: 60_000` because the API is read-mostly. The cache-level
// `onError` hooks are the one place every query/mutation failure is
// logged with its request id; components still render the error via
// QueryState.
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 60_000,
      retry: shouldRetry,
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
