import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { act } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { ThemeToggle } from './ThemeToggle.tsx';

const root = document.documentElement;

function pressed(name: RegExp) {
  return screen.getByRole('button', { name }).getAttribute('aria-pressed');
}

describe('ThemeToggle', () => {
  afterEach(() => {
    vi.restoreAllMocks();
    localStorage.clear();
    root.removeAttribute('data-theme');
  });

  it('renders three labelled buttons with Auto pressed by default', () => {
    render(<ThemeToggle />);

    expect(screen.getByRole('group', { name: 'Colour theme' })).toBeDefined();
    expect(pressed(/match system theme/i)).toBe('true');
    expect(pressed(/^light theme$/i)).toBe('false');
    expect(pressed(/^dark theme$/i)).toBe('false');
  });

  it('reads the initial choice from data-theme set by theme.js', () => {
    root.setAttribute('data-theme', 'dark');
    render(<ThemeToggle />);

    expect(pressed(/^dark theme$/i)).toBe('true');
    expect(pressed(/match system theme/i)).toBe('false');
  });

  it('Dark stores the choice and sets data-theme', async () => {
    const user = userEvent.setup();
    render(<ThemeToggle />);

    await user.click(screen.getByRole('button', { name: /^dark theme$/i }));

    expect(localStorage.getItem('theme')).toBe('dark');
    expect(root.getAttribute('data-theme')).toBe('dark');
    expect(
      await screen.findByRole('button', { name: /^dark theme$/i, pressed: true }),
    ).toBeDefined();
    expect(pressed(/match system theme/i)).toBe('false');
    expect(pressed(/^light theme$/i)).toBe('false');
  });

  it('Light stores the choice and sets data-theme', async () => {
    const user = userEvent.setup();
    render(<ThemeToggle />);

    await user.click(screen.getByRole('button', { name: /^light theme$/i }));

    expect(localStorage.getItem('theme')).toBe('light');
    expect(root.getAttribute('data-theme')).toBe('light');
    expect(
      await screen.findByRole('button', { name: /^light theme$/i, pressed: true }),
    ).toBeDefined();
  });

  it('Auto clears the stored choice and removes data-theme', async () => {
    const user = userEvent.setup();
    render(<ThemeToggle />);

    await user.click(screen.getByRole('button', { name: /^dark theme$/i }));
    await user.click(screen.getByRole('button', { name: /match system theme/i }));

    expect(localStorage.getItem('theme')).toBeNull();
    expect(root.hasAttribute('data-theme')).toBe(false);
    expect(
      await screen.findByRole('button', { name: /match system theme/i, pressed: true }),
    ).toBeDefined();
  });

  it('still applies the theme when storage throws', async () => {
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new DOMException('blocked', 'SecurityError');
    });
    const user = userEvent.setup();
    render(<ThemeToggle />);

    await user.click(screen.getByRole('button', { name: /^dark theme$/i }));

    expect(root.getAttribute('data-theme')).toBe('dark');
    expect(
      await screen.findByRole('button', { name: /^dark theme$/i, pressed: true }),
    ).toBeDefined();
  });

  it('follows data-theme when theme.js re-applies it (another tab, bfcache)', async () => {
    render(<ThemeToggle />);

    act(() => {
      root.setAttribute('data-theme', 'light');
    });

    expect(
      await screen.findByRole('button', { name: /^light theme$/i, pressed: true }),
    ).toBeDefined();
  });
});
