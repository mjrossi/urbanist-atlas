import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useState } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { renderWithProviders } from '../test/renderWithProviders.tsx';
import { RegionCombobox } from './RegionCombobox.tsx';

const QUEENS = {
  region: {
    id: 1,
    kind: 'us:borough',
    name: 'Queens',
    slug: 'queens',
    country: 'US',
    scope_tier: 'local',
    parent_slugs: ['ny'],
  },
  context_label: 'New York',
};

function searchResponse(data: unknown[]) {
  return new Response(JSON.stringify({ data }), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  });
}

// Controlled host so the test can observe the emitted slug list.
function Host() {
  const [value, setValue] = useState<string[]>([]);
  return (
    <>
      <RegionCombobox id="rc" value={value} onChange={setValue} />
      <output data-testid="val">{value.join(',')}</output>
    </>
  );
}

describe('RegionCombobox', () => {
  let fetchSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(searchResponse([QUEENS]));
  });

  afterEach(() => {
    fetchSpy.mockRestore();
  });

  it('does not search until the query reaches two characters', async () => {
    const user = userEvent.setup();
    renderWithProviders(<Host />);
    await user.type(screen.getByRole('combobox'), 'q');
    // Wait past the debounce window to be sure no request fires.
    await new Promise((r) => setTimeout(r, 300));
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it('searches, selects an option, shows a chip, and emits the slug', async () => {
    const user = userEvent.setup();
    renderWithProviders(<Host />);

    await user.type(screen.getByRole('combobox'), 'queens');
    await user.click(await screen.findByRole('option', { name: /queens/i }));

    // The slug is emitted to the controlled parent.
    expect(screen.getByTestId('val').textContent).toBe('queens');
    // Chip shows the human name + state context, not the raw slug.
    expect(screen.getByText('Queens')).toBeDefined();
    expect(screen.getByText('New York')).toBeDefined();

    // The request went to the search endpoint.
    expect(fetchSpy).toHaveBeenCalled();
    const firstCall = fetchSpy.mock.calls[0];
    expect(String(firstCall?.[0])).toContain('/api/v1/regions/search');
  });

  it('removes a selected region via its chip remove button', async () => {
    const user = userEvent.setup();
    renderWithProviders(<Host />);

    await user.type(screen.getByRole('combobox'), 'queens');
    await user.click(await screen.findByRole('option', { name: /queens/i }));
    expect(screen.getByTestId('val').textContent).toBe('queens');

    await user.click(screen.getByRole('button', { name: /remove queens/i }));
    expect(screen.getByTestId('val').textContent).toBe('');
  });

  it('does not re-offer an already-selected region', async () => {
    const user = userEvent.setup();
    renderWithProviders(<Host />);

    await user.type(screen.getByRole('combobox'), 'queens');
    await user.click(await screen.findByRole('option', { name: /queens/i }));
    expect(screen.getByTestId('val').textContent).toBe('queens');

    // Type again — the only result is already picked, so no option shows.
    await user.type(screen.getByRole('combobox'), 'queens');
    await new Promise((r) => setTimeout(r, 300));
    expect(screen.queryByRole('option')).toBeNull();
  });

  it('highlights with ArrowDown and selects with Enter', async () => {
    const user = userEvent.setup();
    renderWithProviders(<Host />);

    await user.type(screen.getByRole('combobox'), 'queens');
    await screen.findByRole('option', { name: /queens/i });
    await user.keyboard('{ArrowDown}{Enter}');

    expect(screen.getByTestId('val').textContent).toBe('queens');
  });

  it('selects the first option on Enter when nothing is highlighted yet', async () => {
    const user = userEvent.setup();
    renderWithProviders(<Host />);

    // Type, see the list, then press Enter without arrowing — picks the
    // first suggestion rather than letting Enter escape to a form submit.
    await user.type(screen.getByRole('combobox'), 'queens');
    await screen.findByRole('option', { name: /queens/i });
    await user.keyboard('{Enter}');

    expect(screen.getByTestId('val').textContent).toBe('queens');
  });

  it('ignores Enter while the shown options belong to an earlier query', async () => {
    fetchSpy.mockImplementation((input: RequestInfo | URL) => {
      const url = new URL(input instanceof Request ? input.url : input.toString());
      const q = url.searchParams.get('q');
      return Promise.resolve(searchResponse(q === 'qu' ? [QUEENS] : []));
    });
    const user = userEvent.setup();
    renderWithProviders(<Host />);

    await user.type(screen.getByRole('combobox'), 'qu');
    await screen.findByRole('option', { name: /queens/i });
    // "qux" is still inside the debounce window, so the Queens option on
    // screen is the answer to "qu", not to what is now typed.
    await user.keyboard('x{Enter}');

    expect(screen.getByTestId('val').textContent).toBe('');
  });

  it('exposes the highlighted option via aria-activedescendant', async () => {
    const user = userEvent.setup();
    renderWithProviders(<Host />);

    const input = screen.getByRole('combobox');
    await user.type(input, 'queens');
    const option = await screen.findByRole('option', { name: /queens/i });
    await user.keyboard('{ArrowDown}');

    expect(input.getAttribute('aria-activedescendant')).toBe(option.id);
  });

  it('announces the result count in a polite live region', async () => {
    const user = userEvent.setup();
    renderWithProviders(<Host />);

    await user.type(screen.getByRole('combobox'), 'queens');
    const live = await screen.findByText(/1 matching region/i);
    expect(live.getAttribute('aria-live')).toBe('polite');
    expect(live.className).toContain('sr-only');
  });

  it('announces when a search has no matches', async () => {
    fetchSpy.mockResolvedValue(searchResponse([]));
    const user = userEvent.setup();
    renderWithProviders(<Host />);

    await user.type(screen.getByRole('combobox'), 'zzz');
    const live = await screen.findByText(/no matching regions/i);
    expect(live.getAttribute('aria-live')).toBe('polite');
  });

  it('removes the last chip when Backspace is pressed on an empty input', async () => {
    const user = userEvent.setup();
    renderWithProviders(<Host />);

    await user.type(screen.getByRole('combobox'), 'queens');
    await user.click(await screen.findByRole('option', { name: /queens/i }));
    expect(screen.getByTestId('val').textContent).toBe('queens');

    // The input is empty after a pick, so Backspace deletes the last chip.
    await user.type(screen.getByRole('combobox'), '{Backspace}');
    expect(screen.getByTestId('val').textContent).toBe('');
  });

  it('surfaces a status message when the search request fails', async () => {
    fetchSpy.mockReset();
    fetchSpy.mockRejectedValue(new Error('network down'));
    const user = userEvent.setup();
    renderWithProviders(<Host />);

    await user.type(screen.getByRole('combobox'), 'queens');

    // A failed search shows an explicit status, not a silent empty list.
    // (Queried by text because the test Host's <output> also has the
    // implicit `status` role.)
    const status = await screen.findByText(/region search is unavailable/i);
    expect(status.getAttribute('role')).toBe('status');
    expect(screen.queryByRole('option')).toBeNull();
  });
});
