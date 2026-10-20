import { describe, expect, it } from 'vitest';

import {
  buildNewSubmissionRequest,
  buildSubmitterNote,
  charCount,
  contactError,
  looksLikeEmail,
  MAX_SUBMITTER_NOTE_CHARS,
  nameError,
  noteFieldError,
  shortDescError,
  SUBMIT_FORM_DEFAULTS,
  type SubmitForm,
  websiteError,
} from './submitForm.ts';

// Client mirrors of the API's submission rules, so the form flags input
// the API would reject before a round trip.

// U+1F68C OMNIBUS: one code point, two UTF-16 units, four UTF-8 bytes.
const BUS = '\u{1F68C}';

function form(over: Partial<SubmitForm> = {}): SubmitForm {
  return { ...SUBMIT_FORM_DEFAULTS, name: 'Sample Org', ...over };
}

describe('charCount', () => {
  it('counts code points, not UTF-16 units', () => {
    expect(BUS.length).toBe(2);
    expect(charCount(BUS)).toBe(1);
    expect(charCount(`a${BUS}b`)).toBe(3);
  });
});

describe('nameError', () => {
  it('allows 200 characters, counting an emoji once', () => {
    expect(nameError(BUS.repeat(200))).toBeUndefined();
    expect(nameError(BUS.repeat(201))).toMatch(/200 characters or fewer/);
  });

  it('measures the trimmed value, as the API does', () => {
    expect(nameError(`  ${'a'.repeat(200)}  `)).toBeUndefined();
  });
});

describe('shortDescError', () => {
  it('caps the description at 1,000 characters', () => {
    expect(shortDescError('a'.repeat(1000))).toBeUndefined();
    expect(shortDescError(BUS.repeat(1000))).toBeUndefined();
    expect(shortDescError('a'.repeat(1001))).toMatch(/1,000 characters or fewer/);
  });
});

describe('websiteError', () => {
  it.each([
    'https://example.org',
    'http://example.org/path?q=1#top',
    'HTTPS://Example.org',
    'https://user@example.org',
    '  https://example.org  ',
  ])('accepts %s', (url) => {
    expect(websiteError(url)).toBeUndefined();
  });

  it('leaves an empty value to the required rule', () => {
    expect(websiteError('   ')).toBeUndefined();
  });

  it.each([
    'example.org',
    'www.example.org/path',
    'ftp://example.org',
    'mailto:hi@example.org',
    '//example.org',
  ])('rejects %s for its scheme', (url) => {
    expect(websiteError(url)).toMatch(/http:\/\/ or https:\/\//);
  });

  it.each([
    'https://',
    'https:///path',
    'https:example.org',
    'https:/example.org',
    'https://user@/x',
    'https://?q=1',
  ])('rejects %s for its missing host', (url) => {
    expect(websiteError(url)).toMatch(/address after/);
  });

  it('caps the URL at 500 bytes', () => {
    const base = 'https://example.org/';
    expect(websiteError(base + 'a'.repeat(500 - base.length))).toBeUndefined();
    expect(websiteError(base + 'a'.repeat(501 - base.length))).toMatch(/500 characters/);
    // Non-ASCII path text counts in UTF-8 bytes.
    expect(websiteError(base + BUS.repeat(121))).toMatch(/500 characters/);
  });
});

describe('looksLikeEmail', () => {
  it('accepts one @ with text on both sides and a dot in the domain', () => {
    expect(looksLikeEmail('jane@example.org')).toBe(true);
  });

  it.each(['x@y', '@jane', 'jane@', 'a@b@c.org', 'jane.example.org'])(
    'rejects %s',
    (value) => {
      expect(looksLikeEmail(value)).toBe(false);
    },
  );
});

describe('contact routing', () => {
  it('sends an x@y with no dot in the domain as a name, not an email', () => {
    const req = buildNewSubmissionRequest(form({ contact: 'jane@localhost' }));
    expect(req.submitter_email).toBeUndefined();
    expect(req.submitter_name).toBe('jane@localhost');
  });

  it('sends a value with two @ signs as a name', () => {
    const req = buildNewSubmissionRequest(form({ contact: '@jane@mastodon.social' }));
    expect(req.submitter_email).toBeUndefined();
    expect(req.submitter_name).toBe('@jane@mastodon.social');
  });
});

describe('contactError', () => {
  it('caps an email at 320 bytes', () => {
    const domain = '@example.org';
    expect(contactError('a'.repeat(320 - domain.length) + domain)).toBeUndefined();
    expect(contactError('a'.repeat(321 - domain.length) + domain)).toMatch(/320/);
  });

  it('caps a handle or name at 200 characters', () => {
    expect(contactError(`@${'a'.repeat(199)}`)).toBeUndefined();
    expect(contactError(`@${'a'.repeat(200)}`)).toMatch(/200/);
  });
});

describe('noteFieldError', () => {
  // The note's fixed lines (type, affiliation); the free-text fields
  // and their headings fill the rest.
  const fixed = charCount(buildSubmitterNote(form({ region: '', why: '', sources: '' })));

  it('passes a note at the cap', () => {
    const f = form({ why: 'w'.repeat(MAX_SUBMITTER_NOTE_CHARS - fixed - 40) });
    expect(charCount(buildSubmitterNote(f))).toBeLessThanOrEqual(
      MAX_SUBMITTER_NOTE_CHARS,
    );
    for (const field of ['region', 'why', 'sources'] as const) {
      expect(noteFieldError(field, f)).toBeUndefined();
    }
  });

  it('puts the error on the field that takes the note past the cap', () => {
    const f = form({ why: 'w'.repeat(1500), sources: 's'.repeat(600) });
    const over = charCount(buildSubmitterNote(f)) - MAX_SUBMITTER_NOTE_CHARS;
    expect(over).toBeGreaterThan(0);
    expect(noteFieldError('why', f)).toBeUndefined();
    expect(noteFieldError('region', f)).toBeUndefined();
    expect(noteFieldError('sources', f)).toBe(
      `Trim ${over.toLocaleString('en-US')} characters. Your notes to the editor share a 2,000-character limit.`,
    );
  });

  it('blames the first field when it alone is over', () => {
    const f = form({ why: 'w'.repeat(2500), sources: 'short' });
    expect(noteFieldError('why', f)).toMatch(/^Trim /);
    expect(noteFieldError('sources', f)).toBeUndefined();
  });

  it('counts emoji once, as the API does', () => {
    const room = MAX_SUBMITTER_NOTE_CHARS - fixed - 40;
    expect(noteFieldError('why', form({ why: BUS.repeat(room) }))).toBeUndefined();
  });

  it('ignores the region text on a correction, which does not send it', () => {
    const f = form({ type: 'correction', region: 'r'.repeat(3000) });
    expect(noteFieldError('region', f)).toBeUndefined();
  });
});
