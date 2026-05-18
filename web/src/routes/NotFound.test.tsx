import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
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

  it('renders the italic subhead about the missing story', () => {
    renderNotFound();
    expect(screen.getByText(/story you were looking for/i)).toBeDefined();
  });

  it('renders a retraction body paragraph', () => {
    renderNotFound();
    expect(screen.getByText(/retract the link/i)).toBeDefined();
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

  it('uses the .page single-column layout class', () => {
    const { container } = renderNotFound();
    expect(container.querySelector('.page')).not.toBeNull();
    expect(container.querySelector('.page-header')).not.toBeNull();
  });
});
