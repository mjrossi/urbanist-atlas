import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render } from '@testing-library/react';
import type { ReactElement } from 'react';
import { MemoryRouter, Route, Routes } from 'react-router';

interface Options {
  /** Defaults to `['/']`. */
  initialEntries?: string[];
  /** Mounts `ui` at this route path, so `useParams()` resolves. */
  routePath?: string;
  /** Defaults to a client with query retries off. */
  queryClient?: QueryClient;
}

/** Renders `ui` inside a QueryClientProvider and a MemoryRouter. */
export function renderWithProviders(ui: ReactElement, opts: Options = {}) {
  const client =
    opts.queryClient ??
    new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const body = opts.routePath ? (
    <Routes>
      <Route path={opts.routePath} element={ui} />
    </Routes>
  ) : (
    ui
  );
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={opts.initialEntries ?? ['/']}>{body}</MemoryRouter>
    </QueryClientProvider>,
  );
}
