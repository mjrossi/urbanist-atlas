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
    overrides.region ?? 'brooklyn-ny',
  );
  await user.type(
    screen.getByLabelText(/one-line description/i),
    overrides.oneLineDesc ?? 'Advocates for safer streets in Anytown.',
  );
}

// Build a Response that the form's fetch helper will see. The
// ApiError wrapper reads `Content-Type` to decide whether to parse
// the body as a problem document, so we set it deliberately.
function jsonResponse(body: unknown, init: ResponseInit & { problem?: boolean } = {}) {
  const { problem, headers, ...rest } = init;
  return new Response(JSON.stringify(body), {
    status: init.status ?? 200,
    headers: {
      'Content-Type': problem
        ? 'application/problem+json'
        : 'application/json',
      ...(headers ?? {}),
    },
    ...rest,
  });
}

const SUCCESS_BODY = {
  id: '01928200-3344-7000-9abc-000000000001',
  status: 'pending' as const,
  payload: {
    name: 'Strong Towns Sample',
    short_desc: 'Advocates for safer streets in Anytown.',
    website_url: 'https://example.org',
    tags: [],
    region_slugs: ['brooklyn-ny'],
  },
  created_at: '2026-05-28T15:00:00Z',
};

describe('Submit', () => {
  let fetchSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    fetchSpy = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(jsonResponse(SUCCESS_BODY, { status: 201 }));
  });

  afterEach(() => {
    fetchSpy.mockRestore();
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
    const button = screen.getByRole('button', { name: /send to editorial queue/i });
    expect((button as HTMLButtonElement).disabled).toBe(true);
  });

  it('submit button is enabled once required fields are filled', async () => {
    const user = userEvent.setup();
    renderSubmit();
    await fillRequired(user);
    await user.tab();
    const button = screen.getByRole('button', { name: /send to editorial queue/i });
    await waitFor(() => {
      expect((button as HTMLButtonElement).disabled).toBe(false);
    });
  });

  it('happy-path submit POSTs to the API and shows the receipt card', async () => {
    const user = userEvent.setup();
    renderSubmit();
    await fillRequired(user, { name: 'Sample Riders Alliance' });
    await user.tab();
    const button = screen.getByRole('button', { name: /send to editorial queue/i });
    await waitFor(() => {
      expect((button as HTMLButtonElement).disabled).toBe(false);
    });
    await user.click(button);

    // Receipt card appears, with the first 8 hex chars of the UUIDv7
    // surfaced as the human reference.
    await waitFor(() => {
      expect(screen.getByRole('heading', { level: 1, name: /tip received/i })).toBeDefined();
    });
    // Short id appears in both the headline and the receipt row — at
    // least one match is enough.
    expect(screen.getAllByText('#01928200').length).toBeGreaterThan(0);

    // Fetch was called with the API payload.
    expect(fetchSpy).toHaveBeenCalledTimes(1);
    const [url, init] = fetchSpy.mock.calls[0];
    expect(String(url)).toContain('/api/v1/submissions');
    expect((init as RequestInit).method).toBe('POST');
    const body = JSON.parse((init as RequestInit).body as string);
    expect(body.payload.name).toBe('Sample Riders Alliance');
    // region_slugs is optional on the wire (see openapi.yaml) — the
    // SPA always sends an empty array. The user-typed Region text
    // flows into submitter_note for editor context.
    expect(body.payload.region_slugs).toEqual([]);
    expect(typeof body.submitter_note).toBe('string');
    expect(body.submitter_note).toMatch(/new organization/i);
    expect(body.submitter_note).toMatch(/brooklyn-ny/);
  });

  it('clears the form when "File another tip" is clicked after a submit', async () => {
    const user = userEvent.setup();
    renderSubmit();
    await fillRequired(user, { name: 'Sample Riders Alliance' });
    await user.tab();
    const button = screen.getByRole('button', { name: /send to editorial queue/i });
    await waitFor(() => {
      expect((button as HTMLButtonElement).disabled).toBe(false);
    });
    await user.click(button);

    await waitFor(() => {
      expect(
        screen.getByRole('heading', { level: 1, name: /tip received/i }),
      ).toBeDefined();
    });

    await user.click(screen.getByRole('button', { name: /file another tip/i }));

    // Back on the form: every field the submitter just typed is blank
    // again, not carried over from the previous submission.
    const nameInput = await screen.findByLabelText(/organization name/i);
    expect((nameInput as HTMLInputElement).value).toBe('');
    expect((screen.getByLabelText(/primary website/i) as HTMLInputElement).value).toBe('');
    expect((screen.getByLabelText(/region served/i) as HTMLInputElement).value).toBe('');
    expect(
      (screen.getByLabelText(/one-line description/i) as HTMLInputElement).value,
    ).toBe('');
  });

  it('correction submissions hide the new-org fields and POST a valid wire shape', async () => {
    const user = userEvent.setup();
    renderSubmit();
    await user.click(
      screen.getByRole('radio', { name: /a correction to an existing entry/i }),
    );
    // Region and one-line description are hidden for corrections; the
    // org is already in the Atlas, so the wire ships synthetic
    // placeholders that point moderators at submitter_note.
    expect(screen.queryByLabelText(/region served/i)).toBeNull();
    expect(screen.queryByLabelText(/one-line description/i)).toBeNull();

    await user.type(
      screen.getByLabelText(/organization name/i),
      'Existing Org',
    );
    await user.type(
      screen.getByLabelText(/primary website/i),
      'https://example.org',
    );
    await user.type(
      screen.getByLabelText(/what needs correcting/i),
      'Their website moved to example.org last March.',
    );
    await user.tab();
    const button = screen.getByRole('button', { name: /send to editorial queue/i });
    await waitFor(() => {
      expect((button as HTMLButtonElement).disabled).toBe(false);
    });
    await user.click(button);

    await waitFor(() => {
      expect(fetchSpy).toHaveBeenCalledTimes(1);
    });
    const [, init] = fetchSpy.mock.calls[0];
    const body = JSON.parse((init as RequestInit).body as string);
    // Wire contract: short_desc is required but synthesized for
    // correction; region_slugs is optional (always sent as []).
    expect(body.payload.name).toBe('Existing Org');
    expect(body.payload.short_desc).toMatch(/correction request/i);
    expect(body.payload.region_slugs).toEqual([]);
    expect(body.submitter_note).toMatch(/correction to an existing entry/i);
  });

  it('shows the rate-limit message with a Retry-After countdown when the API returns 429', async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse(
        {
          type: 'https://urbanistatlas.com/problems/rate-limited',
          title: 'Too Many Requests',
          status: 429,
        },
        { status: 429, problem: true, headers: { 'Retry-After': '600' } },
      ),
    );
    const user = userEvent.setup();
    renderSubmit();
    await fillRequired(user);
    await user.tab();
    await user.click(screen.getByRole('button', { name: /send to editorial queue/i }));

    // Server-provided Retry-After: countdown drives the copy.
    await waitFor(() => {
      expect(screen.getByText(/try again in 600 seconds/i)).toBeDefined();
    });
    // Submit button is disabled and reflects the same countdown.
    expect(
      screen.getByRole('button', { name: /retry in 600s/i }),
    ).toHaveProperty('disabled', true);
  });

  it('falls back to the static "breather" copy when Retry-After is missing on a 429', async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse(
        {
          type: 'https://urbanistatlas.com/problems/rate-limited',
          title: 'Too Many Requests',
          status: 429,
        },
        { status: 429, problem: true },
      ),
    );
    const user = userEvent.setup();
    renderSubmit();
    await fillRequired(user);
    await user.tab();
    await user.click(screen.getByRole('button', { name: /send to editorial queue/i }));

    await waitFor(() => {
      expect(screen.getByText(/breather/i)).toBeDefined();
    });
  });

  it('routes per-field validation errors to the matching form fields', async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse(
        {
          type: 'https://urbanistatlas.com/problems/validation',
          title: 'Bad Request',
          detail: 'one or more fields failed validation',
          status: 400,
          errors: {
            name: 'required',
            website_url: 'must be a valid URL',
          },
        },
        { status: 400, problem: true },
      ),
    );
    const user = userEvent.setup();
    renderSubmit();
    await fillRequired(user);
    await user.tab();
    await user.click(screen.getByRole('button', { name: /send to editorial queue/i }));

    await waitFor(() => {
      // Per-field errors render at the field, not as a top-level banner.
      expect(screen.getByText(/must be a valid URL/i)).toBeDefined();
    });
    expect(screen.getByText(/^required$/i)).toBeDefined();
    // The top-level `detail` banner is suppressed when field errors handled it.
    expect(screen.queryByText(/one or more fields failed validation/i)).toBeNull();
  });

  it('surfaces unmapped/hidden-field validation errors in the top-level banner', async () => {
    // `tags` has no entry in FIELD_NAME_MAP; `submitter_email` maps to
    // the `contact` form field, which renders no inline error slot.
    // Both would be swallowed if only `setError`-mapped fields showed.
    fetchSpy.mockResolvedValueOnce(
      jsonResponse(
        {
          type: 'https://urbanistatlas.com/problems/validation',
          title: 'Bad Request',
          detail: 'one or more fields failed validation',
          status: 400,
          errors: {
            tags: 'too many tags (max 5)',
            submitter_email: 'must be a valid email address',
          },
        },
        { status: 400, problem: true },
      ),
    );
    const user = userEvent.setup();
    renderSubmit();
    await fillRequired(user);
    await user.tab();
    await user.click(screen.getByRole('button', { name: /send to editorial queue/i }));

    // Neither error has a home in the form, so both must appear in the
    // banner rather than vanishing.
    await waitFor(() => {
      expect(screen.getByText(/too many tags/i)).toBeDefined();
    });
    expect(screen.getByText(/must be a valid email address/i)).toBeDefined();
  });

  it('shows the validation message when the API returns 400', async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse(
        {
          type: 'https://urbanistatlas.com/problems/validation',
          title: 'Bad Request',
          detail: 'region_slugs contains unknown slug "anytown-usa"',
          status: 400,
        },
        { status: 400, problem: true },
      ),
    );
    const user = userEvent.setup();
    renderSubmit();
    await fillRequired(user, { region: 'anytown, usa' });
    await user.tab();
    await user.click(screen.getByRole('button', { name: /send to editorial queue/i }));

    await waitFor(() => {
      expect(screen.getByText(/unknown slug/i)).toBeDefined();
    });
  });

  it('shows the GitHub-issue fallback link on 5xx', async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse(
        {
          type: 'https://urbanistatlas.com/problems/internal',
          title: 'Internal Server Error',
          status: 500,
        },
        { status: 500, problem: true },
      ),
    );
    const user = userEvent.setup();
    renderSubmit();
    await fillRequired(user, { name: 'Fallback Org' });
    await user.tab();
    await user.click(screen.getByRole('button', { name: /send to editorial queue/i }));

    const fallback = await screen.findByRole('link', {
      name: /open this as a github issue instead/i,
    });
    const href = fallback.getAttribute('href') ?? '';
    expect(href).toContain('github.com/mjrossi/urbanist-atlas/issues/new');
    const url = new URL(href);
    expect(url.searchParams.get('body') ?? '').toContain('Fallback Org');
  });

  it('sets the browser tab title', async () => {
    renderSubmit();
    await waitFor(() => {
      expect(document.title).toMatch(/submit.*urbanist atlas/i);
    });
  });
});
