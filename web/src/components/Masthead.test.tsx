import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { Masthead } from './Masthead.tsx';

describe('Masthead', () => {
  it('renders the Atlas wordmark with the amber surname accent', () => {
    render(
      <MemoryRouter initialEntries={['/']}>
        <Masthead />
      </MemoryRouter>,
    );

    // Home → wordmark rendered as <h1>. The accent half ("Atlas")
    // lives in a sibling <span class="surname">, so it's the heading
    // text that combines the two halves.
    const heading = screen.getByRole('heading', { level: 1 });
    expect(heading.textContent).toBe('Urbanist Atlas');

    const accent = heading.querySelector('.surname');
    expect(accent?.textContent).toBe('Atlas');
  });

  it('renders the wordmark as a back-to-home link on non-home routes', () => {
    render(
      <MemoryRouter initialEntries={['/about']}>
        <Masthead />
      </MemoryRouter>,
    );

    // Off-home → wordmark is a link with the same text.
    const link = screen.getByRole('link', { name: /urbanist atlas/i });
    expect(link.getAttribute('href')).toBe('/');
  });

  it('renders the italic tagline between two horizontal rules', () => {
    render(
      <MemoryRouter initialEntries={['/']}>
        <Masthead />
      </MemoryRouter>,
    );

    expect(
      screen.getByText(/find the people fighting for better streets/i),
    ).toBeDefined();

    // Two .masthead-rule spans should bracket the tagline.
    const rules = document.querySelectorAll('.masthead-rule');
    expect(rules.length).toBe(2);
  });
});
