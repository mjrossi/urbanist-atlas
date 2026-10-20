/// <reference types="node" />
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

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

  describe('theme-color', () => {
    // Mirrors the two metas in index.html.
    function addThemeColorMetas() {
      for (const scheme of ['light', 'dark']) {
        const meta = document.createElement('meta');
        meta.setAttribute('name', 'theme-color');
        meta.setAttribute('data-scheme', scheme);
        meta.setAttribute('media', `(prefers-color-scheme: ${scheme})`);
        document.head.append(meta);
      }
    }

    function mediaOf(scheme: string) {
      return document
        .querySelector(`meta[name="theme-color"][data-scheme="${scheme}"]`)
        ?.getAttribute('media');
    }

    afterEach(() => {
      for (const m of document.querySelectorAll('meta[name="theme-color"]')) m.remove();
    });

    it('makes the chosen theme colour win over the system scheme', async () => {
      addThemeColorMetas();
      const user = userEvent.setup();
      render(<ThemeToggle />);

      await user.click(screen.getByRole('button', { name: /^dark theme$/i }));
      expect(mediaOf('dark')).toBe('all');
      expect(mediaOf('light')).toBe('not all');

      await user.click(screen.getByRole('button', { name: /^light theme$/i }));
      expect(mediaOf('light')).toBe('all');
      expect(mediaOf('dark')).toBe('not all');
    });

    it('hands the choice back to the system scheme on Auto', async () => {
      addThemeColorMetas();
      const user = userEvent.setup();
      render(<ThemeToggle />);

      await user.click(screen.getByRole('button', { name: /^dark theme$/i }));
      await user.click(screen.getByRole('button', { name: /match system theme/i }));
      expect(mediaOf('light')).toBe('(prefers-color-scheme: light)');
      expect(mediaOf('dark')).toBe('(prefers-color-scheme: dark)');
    });

    it('is set by theme.js before first paint from the stored choice', () => {
      addThemeColorMetas();
      localStorage.setItem('theme', 'dark');
      const script = readFileSync(resolve(process.cwd(), 'public/theme.js'), 'utf8');
      // eslint-disable-next-line @typescript-eslint/no-implied-eval
      new Function(script)();

      expect(root.getAttribute('data-theme')).toBe('dark');
      expect(mediaOf('dark')).toBe('all');
      expect(mediaOf('light')).toBe('not all');
    });
  });
});
