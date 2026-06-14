import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { EmptyState } from './EmptyState.tsx';

describe('EmptyState', () => {
  it('renders the title label and body inside the editors-note card', () => {
    const { container } = render(
      <EmptyState title="Nothing filed yet" body="The desk has been quiet." />,
    );
    const card = container.querySelector('.editors-note');
    expect(card).not.toBeNull();
    expect(screen.getByText('Nothing filed yet').classList.contains('label')).toBe(true);
    expect(screen.getByText('The desk has been quiet.')).not.toBeNull();
  });

  it('renders the cta in its own paragraph only when provided', () => {
    const { rerender } = render(<EmptyState title="Empty" body="Body" />);
    expect(screen.queryByRole('link')).toBeNull();

    rerender(
      <EmptyState title="Empty" body="Body" cta={<a href="/submit">File a tip</a>} />,
    );
    expect(screen.getByRole('link', { name: 'File a tip' })).not.toBeNull();
  });

  it('appends extra utility classes after the base class', () => {
    const { container } = render(
      <EmptyState title="Empty" body="Body" className="mt-24" />,
    );
    const card = container.querySelector('.editors-note');
    expect(card?.classList.contains('mt-24')).toBe(true);
  });

  it('signs off with the newsroom cat', () => {
    const { container } = render(<EmptyState title="Empty" body="Body" />);
    expect(screen.getByText(/checked under every desk/i)).not.toBeNull();
    const glyph = container.querySelector('.editors-note-cat svg');
    expect(glyph).not.toBeNull();
    expect(glyph?.getAttribute('aria-hidden')).toBe('true');
    expect(glyph?.getAttribute('focusable')).toBe('false');
  });
});
