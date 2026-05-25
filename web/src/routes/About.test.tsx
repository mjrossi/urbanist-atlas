import { describe, expect, it } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { About } from './About.tsx';

function renderAbout() {
  return render(
    <MemoryRouter initialEntries={['/about']}>
      <About />
    </MemoryRouter>,
  );
}

describe('About', () => {
  it('renders the page heading', () => {
    renderAbout();
    const h1 = screen.getByRole('heading', { level: 1 });
    expect(h1.textContent).toMatch(/about the urbanist atlas/i);
  });

  it('renders all four section headings', () => {
    renderAbout();
    const h2s = screen.getAllByRole('heading', { level: 2 });
    const text = h2s.map((h) => h.textContent ?? '').join(' | ');
    expect(text).toMatch(/mission/i);
    expect(text).toMatch(/methodology/i);
    expect(text).toMatch(/criteria/i);
    expect(text).toMatch(/acknowledg/i);
  });

  it('mentions the project scope (transit + safe-streets)', () => {
    renderAbout();
    // Multiple paragraphs reference each — use a flexible matcher.
    expect(screen.getAllByText(/transit/i).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/safe.streets/i).length).toBeGreaterThan(0);
  });

  it('mentions the ODbL license', () => {
    renderAbout();
    expect(screen.getAllByText(/odbl/i).length).toBeGreaterThan(0);
  });

  it('links to the companion publication at mjrossi.com/blog', () => {
    renderAbout();
    const link = screen.getByRole('link', { name: /urbanist lexicon/i });
    expect(link.getAttribute('href')).toBe('https://mjrossi.com/blog');
  });

  it('uses the .page single-column layout class', () => {
    const { container } = renderAbout();
    expect(container.querySelector('.page')).not.toBeNull();
    expect(container.querySelector('.page-header')).not.toBeNull();
  });

  it('sets the browser tab title', async () => {
    renderAbout();
    await waitFor(() => {
      expect(document.title).toMatch(/about.*urbanist atlas/i);
    });
  });
});
