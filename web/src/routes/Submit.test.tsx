import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { Submit } from './Submit.tsx';

function renderSubmit() {
  return render(
    <MemoryRouter initialEntries={['/submit']}>
      <Submit />
    </MemoryRouter>,
  );
}

describe('Submit (Phase 2 placeholder)', () => {
  it('renders an honest "coming with Phase 2" headline, not a 404 metaphor', () => {
    renderSubmit();
    const h1 = screen.getByRole('heading', { level: 1 });
    // Headline must signal "deliberate not-yet," not "page is broken."
    expect(h1.textContent).toMatch(/submissions desk opens with phase 2/i);
    expect(h1.textContent).not.toMatch(/page not in this edition/i);
  });

  it('points contributors at GitHub issues until the queue exists', () => {
    renderSubmit();
    const link = screen.getByRole('link', { name: /public repository/i });
    expect(link.getAttribute('href')).toBe(
      'https://github.com/mjrossi/urbanist-atlas',
    );
  });

  it('uses the .page single-column layout shell', () => {
    const { container } = renderSubmit();
    expect(container.querySelector('.page')).not.toBeNull();
    expect(container.querySelector('.page-header')).not.toBeNull();
  });
});
