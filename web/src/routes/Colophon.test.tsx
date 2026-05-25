import { describe, expect, it } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
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
    expect(h1.textContent).toMatch(/^colophon$/i);
  });

  it('renders the five section headings', () => {
    renderColophon();
    const h2s = screen.getAllByRole('heading', { level: 2 });
    const text = h2s.map((h) => h.textContent ?? '').join(' | ');
    expect(text).toMatch(/data sources/i);
    expect(text).toMatch(/stack/i);
    expect(text).toMatch(/type/i);
    expect(text).toMatch(/licensing/i);
    expect(text).toMatch(/editorial cadence/i);
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
    expect(screen.getByText(/X-Data-Attribution: https:\/\/urbanistatlas\.com/)).toBeDefined();
  });

  it('uses the .page single-column layout class', () => {
    const { container } = renderColophon();
    expect(container.querySelector('.page')).not.toBeNull();
    expect(container.querySelector('.page-header')).not.toBeNull();
  });

  it('sets the browser tab title', async () => {
    renderColophon();
    await waitFor(() => {
      expect(document.title).toMatch(/colophon.*urbanist atlas/i);
    });
  });
});
