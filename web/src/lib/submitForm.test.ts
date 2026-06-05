import { describe, expect, it } from 'vitest';

import {
  buildIssueBody,
  buildIssueUrl,
  buildNewSubmissionRequest,
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

  it('every type rolls the free-text region fallback into submitter_note', () => {
    for (const type of ['new', 'correction', 'removal'] as const) {
      const req = buildNewSubmissionRequest(formFor(type, { region: 'Brooklyn, NY' }));
      expect(req.submitter_note).toMatch(/Brooklyn, NY/);
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
});
