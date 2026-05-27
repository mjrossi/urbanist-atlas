import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Submit } from './Submit.tsx';
import { renderWithProviders } from '../test/renderWithProviders.tsx';

function renderSubmit() {
  return renderWithProviders(<Submit />, { initialEntries: ['/submit'] });
}

async function fillRequired(
  user: ReturnType<typeof userEvent.setup>,
  overrides: Partial<{
    name: string;
    website: string;
    region: string;
    oneLineDesc: string;
  }> = {},
) {
  await user.type(
    screen.getByLabelText(/organization name/i),
    overrides.name ?? 'Strong Towns Sample',
  );
  await user.type(
    screen.getByLabelText(/primary website/i),
    overrides.website ?? 'https://example.org',
  );
  await user.type(
    screen.getByLabelText(/region served/i),
    overrides.region ?? 'Anytown, USA',
  );
  await user.type(
    screen.getByLabelText(/one-line description/i),
    overrides.oneLineDesc ?? 'Advocates for safer streets in Anytown.',
  );
}

describe('Submit', () => {
  let openSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    // Return a truthy stand-in for the new tab so the popup-blocked
    // branch only fires in tests that explicitly opt in.
    openSpy = vi.spyOn(window, 'open').mockImplementation(() => ({}) as Window);
  });

  afterEach(() => {
    openSpy.mockRestore();
  });

  it('renders all required field labels', () => {
    renderSubmit();
    expect(screen.getByLabelText(/organization name/i)).toBeDefined();
    expect(screen.getByLabelText(/primary website/i)).toBeDefined();
    expect(screen.getByLabelText(/region served/i)).toBeDefined();
    expect(screen.getByLabelText(/one-line description/i)).toBeDefined();
  });

  it('renders the page breadcrumb as a navigation landmark', () => {
    renderSubmit();
    const nav = screen.getByRole('navigation', { name: /breadcrumb/i });
    const current = within(nav).getByText('Submissions');
    expect(current.getAttribute('aria-current')).toBe('page');
  });

  it('submit button is disabled when required fields are empty', () => {
    renderSubmit();
    const button = screen.getByRole('button', { name: /open as github issue/i });
    expect((button as HTMLButtonElement).disabled).toBe(true);
  });

  it('submit button is enabled once required fields are filled', async () => {
    const user = userEvent.setup();
    renderSubmit();
    await fillRequired(user);
    // mode:'onBlur' — tab off the last field to trigger validation.
    await user.tab();
    const button = screen.getByRole('button', { name: /open as github issue/i });
    await waitFor(() => {
      expect((button as HTMLButtonElement).disabled).toBe(false);
    });
  });

  it('clicking submit opens a GitHub issue URL with the encoded title and body', async () => {
    const user = userEvent.setup();
    renderSubmit();
    await fillRequired(user, { name: 'Sample Riders Alliance' });
    await user.tab();
    const button = screen.getByRole('button', { name: /open as github issue/i });
    await waitFor(() => {
      expect((button as HTMLButtonElement).disabled).toBe(false);
    });
    await user.click(button);

    expect(openSpy).toHaveBeenCalledTimes(1);
    // Parse the URL and inspect decoded values so the test doesn't
    // care whether URLSearchParams encodes spaces as `+` or `%20`.
    const url = new URL(openSpy.mock.calls[0][0] as string);
    expect(url.origin + url.pathname).toBe(
      'https://github.com/mjrossi/urbanist-atlas/issues/new',
    );
    expect(url.searchParams.get('title')).toBe('[New org] Sample Riders Alliance');
    expect(url.searchParams.has('body')).toBe(true);
    // template= would silently override our pre-filled body.
    expect(url.searchParams.has('template')).toBe(false);
    expect(openSpy.mock.calls[0][1]).toBe('_blank');
    expect(openSpy.mock.calls[0][2]).toBe('noopener');
  });

  it('correction type produces a [Correction] title prefix', async () => {
    const user = userEvent.setup();
    renderSubmit();
    await user.click(screen.getByLabelText(/correction to an existing entry/i));
    await fillRequired(user);
    await user.tab();
    const button = screen.getByRole('button', { name: /open as github issue/i });
    await waitFor(() => {
      expect((button as HTMLButtonElement).disabled).toBe(false);
    });
    await user.click(button);

    const url = new URL(openSpy.mock.calls[0][0] as string);
    expect(url.searchParams.get('title')).toMatch(/^\[Correction\]/);
  });

  it('removal type produces a [Removal] title prefix', async () => {
    const user = userEvent.setup();
    renderSubmit();
    await user.click(screen.getByLabelText(/removal request/i));
    await fillRequired(user);
    await user.tab();
    const button = screen.getByRole('button', { name: /open as github issue/i });
    await waitFor(() => {
      expect((button as HTMLButtonElement).disabled).toBe(false);
    });
    await user.click(button);

    const url = new URL(openSpy.mock.calls[0][0] as string);
    expect(url.searchParams.get('title')).toMatch(/^\[Removal\]/);
  });

  it('sets the browser tab title', async () => {
    renderSubmit();
    await waitFor(() => {
      expect(document.title).toMatch(/submit.*urbanist atlas/i);
    });
  });

  it('intro paragraph mentions GitHub and a pre-filled issue', () => {
    renderSubmit();
    const header = screen.getByRole('heading', { level: 1 }).parentElement;
    expect(header?.textContent).toMatch(/github/i);
    expect(header?.textContent).toMatch(/pre-filled/i);
    expect(header?.textContent).toMatch(/issue/i);
  });

  it('a second submit within the cooldown is suppressed', async () => {
    const user = userEvent.setup();
    renderSubmit();
    await fillRequired(user);
    await user.tab();
    const button = screen.getByRole('button', { name: /open as github issue/i });
    await waitFor(() => {
      expect((button as HTMLButtonElement).disabled).toBe(false);
    });
    await user.click(button);
    // Button should be disabled by the justOpened cooldown — a second
    // click during the lockout window must not open a second tab.
    await waitFor(() => {
      expect((button as HTMLButtonElement).disabled).toBe(true);
    });
    await user.click(button);
    expect(openSpy).toHaveBeenCalledTimes(1);
  });

  it('shows an inline pop-up-blocked notice when window.open returns null', async () => {
    openSpy.mockImplementation(() => null);
    const user = userEvent.setup();
    renderSubmit();
    await fillRequired(user);
    await user.tab();
    const button = screen.getByRole('button', { name: /open as github issue/i });
    await waitFor(() => {
      expect((button as HTMLButtonElement).disabled).toBe(false);
    });
    await user.click(button);

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toMatch(/blocked/i);
    const manualLink = screen.getByRole('link', { name: /open the pre-filled issue manually/i });
    expect(manualLink.getAttribute('href')).toContain(
      'github.com/mjrossi/urbanist-atlas/issues/new',
    );
    // Button must remain clickable so the user can retry after allowing pop-ups.
    expect((button as HTMLButtonElement).disabled).toBe(false);
  });
});
