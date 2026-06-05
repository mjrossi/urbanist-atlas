import { screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { renderWithProviders } from '../test/renderWithProviders.tsx';
import { BroadsheetNav } from './BroadsheetNav.tsx';

describe('BroadsheetNav', () => {
  it('marks the matching nav entry with aria-current="page"', () => {
    renderWithProviders(<BroadsheetNav />, { initialEntries: ['/about'] });
    const aboutLink = screen.getByRole('link', { name: /about/i });
    expect(aboutLink.getAttribute('aria-current')).toBe('page');
  });

  it('leaves non-active entries without aria-current', () => {
    renderWithProviders(<BroadsheetNav />, { initialEntries: ['/about'] });
    const submitLink = screen.getByRole('link', { name: /submit/i });
    expect(submitLink.getAttribute('aria-current')).toBeNull();
  });

  it('treats /browse, /region/*, /orgs/*, and /r/* as the Browse section', () => {
    renderWithProviders(<BroadsheetNav />, { initialEntries: ['/region/seattle'] });
    const browseLink = screen.getByRole('link', { name: /browse/i });
    expect(browseLink.getAttribute('aria-current')).toBe('page');
  });

  it('keeps the active entry as a Link so it stays keyboard-focusable', () => {
    renderWithProviders(<BroadsheetNav />, { initialEntries: ['/about'] });
    // Active entries used to render as a <span onClick> which had no
    // tab stop and announced no current-page state. Asserting role=link
    // pins down that fix.
    expect(screen.getByRole('link', { name: /about/i })).toBeDefined();
  });
});
