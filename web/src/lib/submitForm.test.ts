import { describe, expect, it } from 'vitest';

import { ApiError } from './api.ts';
import {
  buildIssueBody,
  buildIssueUrl,
  buildNewSubmissionRequest,
  isServerOrUnreachable,
  shortSubmissionId,
  SUBMIT_FORM_DEFAULTS,
  type SubmitForm,
  titlePrefix,
} from './submitForm.ts';

// Per-type wire-shape coverage for the public submissions endpoint.
// The form's three submission types (`new` / `correction` / `removal`)
// adapt the visible fields but must always produce a payload the
// backend's `ValidateSubmissionPayload` accepts. `region_slugs` carries
// the slugs the submitter picked from the type-ahead; when none are
// picked the free-text fallback rides along in `submitter_note`.

function formFor(type: SubmitForm['type'], over: Partial<SubmitForm> = {}): SubmitForm {
  return {
    ...SUBMIT_FORM_DEFAULTS,
    type,
    name: 'Sample Org',
    website: 'https://example.org',
    region: 'brooklyn-ny',
    oneLineDesc: 'Advocates for safer streets in Anytown.',
    why: 'They led the citywide bus-lane campaign in 2025.',
    sources: 'https://news.example/article',
    affiliation: 'none',
    contact: '',
    ...over,
  };
}

describe('buildNewSubmissionRequest', () => {
  it('new: ships the real one-line description and an empty slug list', () => {
    const req = buildNewSubmissionRequest(formFor('new'));
    expect(req.payload.name).toBe('Sample Org');
    expect(req.payload.website_url).toBe('https://example.org');
    expect(req.payload.short_desc).toBe('Advocates for safer streets in Anytown.');
    expect(req.payload.region_slugs).toEqual([]);
  });

  it('correction: hides the org-description fields, synthesizes short_desc', () => {
    const req = buildNewSubmissionRequest(formFor('correction'));
    // Wire shape stays stable — short_desc is required, so we mint
    // a synthetic placeholder that points moderators at submitter_note.
    expect(req.payload.short_desc).toMatch(/correction request/i);
    expect(req.payload.region_slugs).toEqual([]);
  });

  it('removal: synthesizes short_desc with the removal-request marker', () => {
    const req = buildNewSubmissionRequest(formFor('removal'));
    expect(req.payload.short_desc).toMatch(/removal request/i);
    expect(req.payload.region_slugs).toEqual([]);
  });

  it('new: sends the picked region slugs and echoes them in the note', () => {
    const req = buildNewSubmissionRequest(
      formFor('new', { regionSlugs: ['queens', 'nyc'], region: '' }),
    );
    expect(req.payload.region_slugs).toEqual(['queens', 'nyc']);
    expect(req.submitter_note).toMatch(/queens/i);
    expect(req.submitter_note).toMatch(/nyc/i);
  });

  it('new: rolls the free-text region fallback into submitter_note', () => {
    const req = buildNewSubmissionRequest(formFor('new', { region: 'Brooklyn, NY' }));
    expect(req.submitter_note).toMatch(/Brooklyn, NY/);
  });

  it('correction/removal: drop region values left over from the new-org fields', () => {
    for (const type of ['correction', 'removal'] as const) {
      const req = buildNewSubmissionRequest(
        formFor(type, { regionSlugs: ['queens'], region: 'Ann Arbor, MI' }),
      );
      expect(req.payload.region_slugs).toEqual([]);
      expect(req.submitter_note).not.toMatch(/queens/i);
      expect(req.submitter_note).not.toMatch(/Ann Arbor/);
      expect(req.submitter_note).not.toMatch(/new-region request/i);
    }
  });

  it('submitter_note labels the submission type', () => {
    expect(buildNewSubmissionRequest(formFor('new')).submitter_note).toMatch(
      /new organization/i,
    );
    expect(buildNewSubmissionRequest(formFor('correction')).submitter_note).toMatch(
      /correction to an existing entry/i,
    );
    expect(buildNewSubmissionRequest(formFor('removal')).submitter_note).toMatch(
      /removal request/i,
    );
  });

  it('contact: an email-shaped value lands in submitter_email', () => {
    const req = buildNewSubmissionRequest(
      formFor('new', { contact: 'jane@example.org' }),
    );
    expect(req.submitter_email).toBe('jane@example.org');
    expect(req.submitter_name).toBeUndefined();
  });

  it('contact: a @handle lands in submitter_name (it is not a valid email)', () => {
    const req = buildNewSubmissionRequest(formFor('new', { contact: '@jane' }));
    expect(req.submitter_name).toBe('@jane');
    expect(req.submitter_email).toBeUndefined();
  });

  it('contact: empty value omits both submitter_name and submitter_email', () => {
    const req = buildNewSubmissionRequest(formFor('new', { contact: '' }));
    expect(req.submitter_name).toBeUndefined();
    expect(req.submitter_email).toBeUndefined();
  });

  it('trims whitespace on name and website', () => {
    const req = buildNewSubmissionRequest(
      formFor('new', { name: '  Padded Org  ', website: '  https://example.org  ' }),
    );
    expect(req.payload.name).toBe('Padded Org');
    expect(req.payload.website_url).toBe('https://example.org');
  });
});

describe('titlePrefix', () => {
  it('maps each submission type to its issue-title bracket', () => {
    expect(titlePrefix('new')).toBe('[New org]');
    expect(titlePrefix('correction')).toBe('[Correction]');
    expect(titlePrefix('removal')).toBe('[Removal]');
  });
});

describe('buildIssueBody / buildIssueUrl (5xx fallback path)', () => {
  it('the issue body contains the org name and the type checkbox marked', () => {
    const body = buildIssueBody(formFor('new', { name: 'Fallback Org' }));
    expect(body).toContain('Fallback Org');
    expect(body).toContain('[x] New organization to add');
    expect(body).toContain('[ ] Correction to an existing entry');
  });

  it('the issue URL is a github.com/new link with the title prefixed by the type', () => {
    const href = buildIssueUrl(formFor('correction', { name: 'Existing Org' }));
    const url = new URL(href);
    expect(url.origin + url.pathname).toBe(
      'https://github.com/mjrossi/urbanist-atlas/issues/new',
    );
    expect(url.searchParams.get('title')).toBe('[Correction] Existing Org');
  });

  it('the issue body lists the picked region slugs alongside the free-text region', () => {
    const body = buildIssueBody(
      formFor('new', { regionSlugs: ['queens', 'nyc'], region: 'Ann Arbor, MI' }),
    );
    expect(body).toContain('**Region served:** queens, nyc, Ann Arbor, MI');
  });

  it('the issue body lists picked slugs when no free-text region was given', () => {
    const body = buildIssueBody(formFor('new', { regionSlugs: ['queens'], region: '' }));
    expect(body).toContain('**Region served:** queens\n');
  });

  it('the issue body leaves out new-org fields on a correction', () => {
    const body = buildIssueBody(
      formFor('correction', {
        regionSlugs: ['queens'],
        region: 'Ann Arbor, MI',
        oneLineDesc: 'Stale description',
      }),
    );
    expect(body).not.toContain('queens');
    expect(body).not.toContain('Ann Arbor');
    expect(body).not.toContain('Stale description');
  });
});

describe('isServerOrUnreachable', () => {
  it('is true for a 5xx, a status-0 ApiError, and a non-ApiError failure', () => {
    expect(isServerOrUnreachable(new ApiError(503, 'down', undefined, undefined))).toBe(
      true,
    );
    expect(isServerOrUnreachable(new ApiError(0, 'offline', undefined, undefined))).toBe(
      true,
    );
    expect(isServerOrUnreachable(new TypeError('Failed to fetch'))).toBe(true);
  });

  it('is false for 4xx responses, which have their own banners', () => {
    for (const status of [400, 422, 429]) {
      expect(isServerOrUnreachable(new ApiError(status, 'x', undefined, undefined))).toBe(
        false,
      );
    }
  });
});

describe('shortSubmissionId', () => {
  it('takes the random tail of the UUIDv7, not the timestamp prefix', () => {
    expect(shortSubmissionId('01928200-3344-7000-9abc-0000deadbeef')).toBe('deadbeef');
  });

  it('differs for two ids minted in the same timestamp window', () => {
    const a = shortSubmissionId('01928200-3344-7000-9abc-000000000001');
    const b = shortSubmissionId('01928200-3344-7000-9abc-000000000002');
    expect(a).not.toBe(b);
  });
});
