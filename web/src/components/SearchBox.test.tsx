import { describe, expect, it } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router';
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
      <div
        data-testid="location"
        data-pathname={loc.pathname}
        data-search={loc.search}
      />
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
  return screen.getByLabelText(/postal code/i) as HTMLInputElement;
}

function getButton() {
  return screen.getByRole('button', { name: /look up/i });
}

function typeInto(input: HTMLInputElement, value: string) {
  fireEvent.change(input, { target: { value } });
}

describe('SearchBox', () => {
  it('normalizes a US ZIP and navigates with country=US', () => {
    renderWithRouter();
    typeInto(getInput(), '11217');
    fireEvent.click(getButton());

    const { pathname, search } = getLocation();
    expect(pathname).toBe('/r/11217');
    expect(search).toBe('?country=US');
  });

  it('uppercases and strips whitespace from a Canadian postal code', () => {
    renderWithRouter();
    typeInto(getInput(), 'm5v 2t6');
    fireEvent.click(getButton());

    const { pathname, search } = getLocation();
    expect(pathname).toBe('/r/M5V2T6');
    expect(search).toBe('?country=CA');
  });

  it('accepts a 3-char FSA for Canadian lookups', () => {
    renderWithRouter();
    typeInto(getInput(), 'm5v');
    fireEvent.click(getButton());

    expect(getLocation().pathname).toBe('/r/M5V');
  });

  it('shows an inline error and does not navigate when input is invalid', () => {
    renderWithRouter();
    typeInto(getInput(), '123');
    fireEvent.click(getButton());

    expect(screen.getByRole('alert').textContent).toMatch(/5 digits/i);
    expect(getLocation().pathname).toBe('/');
  });

});
