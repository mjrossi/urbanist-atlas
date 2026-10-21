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

installGlobalErrorLogging();

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
