import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router';
import { describe, expect, it } from 'vitest';

import { SearchBox } from './SearchBox.tsx';

/**
 * Helper: render the SearchBox under a MemoryRouter that exposes the
 * current location via a sentinel route, so tests can assert on the
 * URL the box navigated to.
 */
function renderWithRouter() {
  function LocationProbe() {
    const loc = useLocation();
    return (
      <div data-testid="location" data-pathname={loc.pathname} data-search={loc.search} />
    );
  }

  return render(
    <MemoryRouter initialEntries={['/']}>
      <Routes>
        <Route
          path="/"
          element={
            <>
              <SearchBox />
              <LocationProbe />
            </>
          }
        />
        <Route
          path="/r/:postalCode"
          element={
            <>
              <SearchBox />
              <LocationProbe />
            </>
          }
        />
      </Routes>
    </MemoryRouter>,
  );
}

function getLocation() {
  const node = screen.getByTestId('location');
  return {
    pathname: node.getAttribute('data-pathname') ?? '',
    search: node.getAttribute('data-search') ?? '',
  };
}

function getInput() {
  return screen.getByLabelText(/postal code/i);
}

function getButton() {
  return screen.getByRole('button', { name: /look up/i });
}

describe('SearchBox', () => {
  it('normalizes a US ZIP and navigates with country=US', async () => {
    const user = userEvent.setup();
    renderWithRouter();
    await user.type(getInput(), '11217');
    await user.click(getButton());

    const { pathname, search } = getLocation();
    expect(pathname).toBe('/r/11217');
    expect(search).toBe('?country=US');
  });

  it('uppercases and strips whitespace from a Canadian postal code', async () => {
    const user = userEvent.setup();
    renderWithRouter();
    await user.type(getInput(), 'm5v 2t6');
    await user.click(getButton());

    const { pathname, search } = getLocation();
    expect(pathname).toBe('/r/M5V2T6');
    expect(search).toBe('?country=CA');
  });

  it('accepts a 3-char FSA for Canadian lookups', async () => {
    const user = userEvent.setup();
    renderWithRouter();
    await user.type(getInput(), 'm5v');
    await user.click(getButton());

    expect(getLocation().pathname).toBe('/r/M5V');
  });

  it('shows an inline error and does not navigate when input is invalid', async () => {
    const user = userEvent.setup();
    renderWithRouter();
    await user.type(getInput(), '123');
    await user.click(getButton());

    expect(screen.getByRole('alert').textContent).toMatch(/five digits/i);
    expect(getLocation().pathname).toBe('/');
  });
});
