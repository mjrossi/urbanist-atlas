import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router';
import { Submit } from './Submit.tsx';

function renderSubmit() {
  return render(
    <MemoryRouter initialEntries={['/submit']}>
      <Submit />
    </MemoryRouter>,
  );
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
    openSpy = vi.spyOn(window, 'open').mockImplementation(() => null);
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
    const url = openSpy.mock.calls[0][0] as string;
    expect(url).toContain('github.com/mjrossi/urbanist-atlas/issues/new');
    expect(url).toContain('title=%5BNew%20org%5D');
    expect(url).toContain(encodeURIComponent('Sample Riders Alliance'));
    // template= would silently override our pre-filled body.
    expect(url).not.toContain('template=');
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

    const url = openSpy.mock.calls[0][0] as string;
    expect(url).toContain('title=%5BCorrection%5D');
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

    const url = openSpy.mock.calls[0][0] as string;
    expect(url).toContain('title=%5BRemoval%5D');
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
});
