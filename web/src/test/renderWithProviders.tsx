import type { ReactElement } from 'react';
import { render } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

interface Options {
  /** Initial entry stack for the MemoryRouter. Defaults to `['/']`. */
  initialEntries?: string[];
  /**
   * If set, the rendered tree is wrapped in
   * `<Routes><Route path={routePath} element={ui} /></Routes>` so
   * `useParams()` resolves params from `initialEntries`. Leave undefined
   * for pages that don't read URL params.
   */
  routePath?: string;
  /**
   * Custom QueryClient if a test needs to seed cache or tweak defaults.
   * The default is `{ defaultOptions: { queries: { retry: false } } }`
   * so failing-query tests don't retry.
   */
  queryClient?: QueryClient;
}

/**
 * Renders `ui` wrapped in the providers every page test needs:
 * QueryClientProvider + MemoryRouter (+ optional Routes/Route to
 * activate URL params). Replaces six near-identical per-file render
 * helpers with one definition.
 */
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
      <MemoryRouter initialEntries={opts.initialEntries ?? ['/']}>
        {body}
      </MemoryRouter>
    </QueryClientProvider>,
  );
}
