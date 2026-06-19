import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { describe, expect, it } from 'vitest';

import { Colophon } from './Colophon.tsx';

function renderColophon() {
  return render(
    <MemoryRouter initialEntries={['/colophon']}>
      <Colophon />
    </MemoryRouter>,
  );
}

describe('Colophon', () => {
  it('renders the page heading', () => {
    renderColophon();
    const h1 = screen.getByRole('heading', { level: 1 });
    expect(h1.textContent).toMatch(/what the atlas/i);
    expect(h1.textContent).toMatch(/built from/i);
  });

  it('renders the six section headings', () => {
    const { container } = renderColophon();
    const h2s = screen.getAllByRole('heading', { level: 2 });
    const text = h2s.map((h) => h.textContent).join(' | ');
    // Six new-style section h2s.
    expect(text).toMatch(/where the geography comes from/i);
    expect(text).toMatch(/how the atlas runs/i);
    expect(text).toMatch(/broadsheet vocabulary/i);
    expect(text).toMatch(/what you can take/i);
    expect(text).toMatch(/how the directory stays current/i);
    expect(text).toMatch(/three surveyors who know the ground/i);
    // And the kicker numerals carry their topic labels.
    const kickers = Array.from(container.querySelectorAll('.section-kicker'))
      .map((k) => k.textContent)
      .join(' | ');
    expect(kickers).toMatch(/data sources/i);
    expect(kickers).toMatch(/stack/i);
    expect(kickers).toMatch(/licensing/i);
    expect(kickers).toMatch(/editorial cadence/i);
    expect(kickers).toMatch(/field staff/i);
  });

  it('credits the newsroom cats with decorative portraits', () => {
    const { container } = renderColophon();
    expect(screen.getAllByText(/pad thai/i).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/mrs peacock/i).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/cera/i).length).toBeGreaterThan(0);
    const portraits = container.querySelectorAll('.newsroom-cats svg');
    expect(portraits.length).toBe(3);
    for (const svg of portraits) {
      expect(svg.getAttribute('aria-hidden')).toBe('true');
      expect(svg.getAttribute('focusable')).toBe('false');
    }
  });

  it('names the upstream data providers with the right vintages', () => {
    renderColophon();
    expect(screen.getByText(/u\.s\. census bureau/i)).toBeDefined();
    expect(screen.getByText(/hud usps zip-to-county/i)).toBeDefined();
    expect(screen.getByText(/statistics canada/i)).toBeDefined();
    expect(screen.getAllByText(/2021 census/i).length).toBeGreaterThan(0);
  });

  it('credits Urbanist Lexicon for the broadsheet identity', () => {
    renderColophon();
    const link = screen.getByRole('link', { name: /urbanist lexicon/i });
    expect(link.getAttribute('href')).toBe('https://mjrossi.com/blog');
  });

  it('shows the ODbL attribution headers verbatim', () => {
    renderColophon();
    expect(screen.getByText(/X-Data-License: ODbL-1\.0/)).toBeDefined();
    expect(
      screen.getByText(/X-Data-Attribution: https:\/\/urbanistatlas\.com/),
    ).toBeDefined();
  });

  it('sets the browser tab title', async () => {
    renderColophon();
    await waitFor(() => {
      expect(document.title).toMatch(/colophon.*urbanist atlas/i);
    });
  });
});
