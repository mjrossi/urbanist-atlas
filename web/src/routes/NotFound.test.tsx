import { describe, expect, it } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { NotFound } from './NotFound.tsx';

function renderNotFound() {
  return render(
    <MemoryRouter initialEntries={['/totally-fake-url']}>
      <NotFound />
    </MemoryRouter>,
  );
}

describe('NotFound', () => {
  it('renders the newspaper-style headline', () => {
    renderNotFound();
    const h1 = screen.getByRole('heading', { level: 1 });
    expect(h1.textContent).toMatch(/page not in this edition/i);
  });

  it('renders the italic subhead about the missing page', () => {
    renderNotFound();
    expect(screen.getByText(/find the page you were after/i)).toBeDefined();
  });

  it('renders a body paragraph for users who followed a stale link', () => {
    renderNotFound();
    expect(screen.getByText(/followed a link from another site/i)).toBeDefined();
  });

  it('provides a return-to-homepage link pointing to /', () => {
    renderNotFound();
    const link = screen.getByRole('link', { name: /return to the front page/i });
    expect(link.getAttribute('href')).toBe('/');
  });

  it('applies the .not-found-return class to the return link', () => {
    renderNotFound();
    const link = screen.getByRole('link', { name: /return to the front page/i });
    expect(link.classList.contains('not-found-return')).toBe(true);
  });

  it('sets the browser tab title', async () => {
    renderNotFound();
    await waitFor(() => {
      expect(document.title).toMatch(/page not in this edition.*urbanist atlas/i);
    });
  });
});
