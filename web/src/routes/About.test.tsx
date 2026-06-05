import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { describe, expect, it } from 'vitest';

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
    expect(h1.textContent).toMatch(/the people.*doing the work/i);
  });

  it('renders the major section headings', () => {
    const { container } = renderAbout();
    const h2s = screen.getAllByRole('heading', { level: 2 });
    const text = h2s.map((h) => h.textContent).join(' | ');
    // Loosely assert mission + curation/methodology + acknowledgments
    // are present as h2s.
    expect(text).toMatch(/why this exists/i);
    expect(text).toMatch(/how we curate/i);
    expect(text).toMatch(/who the directory rests on/i);
    // Section kickers carry the I/II/III labels.
    const kickers = container.querySelectorAll('.section-kicker');
    const kickerText = Array.from(kickers)
      .map((k) => k.textContent)
      .join(' | ');
    expect(kickerText).toMatch(/mission/i);
    expect(kickerText).toMatch(/methodology/i);
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
    // And links out to opendatacommons.org. Parse the URL and check
    // the hostname rather than a substring match so a path like
    // `/opendatacommons.org/` on a different host can't satisfy the
    // assertion (CodeQL js/incomplete-url-substring-sanitization).
    const odblLink = screen.getAllByRole('link').find((a) => {
      const href = a.getAttribute('href');
      if (!href) return false;
      try {
        return new URL(href).hostname === 'opendatacommons.org';
      } catch {
        return false;
      }
    });
    expect(odblLink).toBeDefined();
  });

  it('sets the browser tab title', async () => {
    renderAbout();
    await waitFor(() => {
      expect(document.title).toMatch(/about.*urbanist atlas/i);
    });
  });
});
