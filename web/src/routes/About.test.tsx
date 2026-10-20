import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { About } from './About.tsx';

function renderAbout(path = '/about') {
  return render(
    <MemoryRouter initialEntries={[path]}>
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

  describe('collapsible sections', () => {
    function setPhone(matches: boolean) {
      vi.spyOn(window, 'matchMedia').mockImplementation(
        (query: string) =>
          ({
            matches,
            media: query,
            onchange: null,
            addEventListener: () => undefined,
            removeEventListener: () => undefined,
            addListener: () => undefined,
            removeListener: () => undefined,
            dispatchEvent: () => false,
          }) as MediaQueryList,
      );
    }

    afterEach(() => {
      vi.restoreAllMocks();
    });

    const hidden = (text: RegExp) => screen.getByText(text).closest('[hidden]') !== null;

    it('uses no <details>, whose summaries announce a toggle that does nothing on desktop', () => {
      const { container } = renderAbout();
      expect(container.querySelector('details, summary')).toBeNull();
    });

    it('shows every section as plain headings and text on desktop', () => {
      setPhone(false);
      renderAbout();
      expect(screen.queryAllByRole('button', { expanded: false })).toHaveLength(0);
      expect(screen.queryAllByRole('button', { expanded: true })).toHaveLength(0);
      expect(hidden(/independent reference work/i)).toBe(false);
    });

    it('collapses all but the first section behind disclosure buttons on phones', async () => {
      setPhone(true);
      const user = userEvent.setup();
      renderAbout();

      const curate = screen.getByRole('button', { name: /how we curate/i });
      const honest = screen.getByRole('button', { name: /keep ourselves honest/i });
      expect(curate.getAttribute('aria-expanded')).toBe('true');
      expect(honest.getAttribute('aria-expanded')).toBe('false');
      const body = document.getElementById(honest.getAttribute('aria-controls') ?? '');
      expect(body).not.toBeNull();
      expect(hidden(/independent reference work/i)).toBe(true);

      await user.click(honest);
      expect(honest.getAttribute('aria-expanded')).toBe('true');
      expect(hidden(/independent reference work/i)).toBe(false);
    });

    it('opens the section a link points at on phones', () => {
      setPhone(true);
      renderAbout('/about#corrections');
      expect(
        screen
          .getByRole('button', { name: /keep ourselves honest/i })
          .getAttribute('aria-expanded'),
      ).toBe('true');
    });
  });
});
