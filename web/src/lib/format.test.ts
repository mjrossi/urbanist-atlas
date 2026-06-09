import { describe, expect, it } from 'vitest';

import {
  domainOf,
  formatAddedAt,
  groupCountLabel,
  pluralize,
  prettyTag,
} from './format.ts';

describe('groupCountLabel', () => {
  it('uses the singular form for exactly one', () => {
    expect(groupCountLabel(1)).toBe('1 group');
  });

  it('uses the plural form for zero', () => {
    expect(groupCountLabel(0)).toBe('0 groups');
  });

  it('uses the plural form for many', () => {
    expect(groupCountLabel(42)).toBe('42 groups');
  });
});

describe('pluralize', () => {
  it('returns the singular form for exactly one', () => {
    expect(pluralize(1, 'entry', 'entries')).toBe('entry');
  });

  it('returns the plural form for zero and many', () => {
    expect(pluralize(0, 'entry', 'entries')).toBe('entries');
    expect(pluralize(2, 'group', 'groups')).toBe('groups');
  });
});

describe('domainOf', () => {
  it('returns the host for a plain https URL', () => {
    expect(domainOf('https://transitriders.org/about')).toBe('transitriders.org');
  });

  it('strips a leading www.', () => {
    expect(domainOf('https://www.example.org/')).toBe('example.org');
  });

  it('preserves subdomains other than www', () => {
    expect(domainOf('https://chapter.example.org/')).toBe('chapter.example.org');
  });

  it('handles http URLs', () => {
    expect(domainOf('http://example.org')).toBe('example.org');
  });

  it('returns null for malformed input', () => {
    expect(domainOf('not a url')).toBeNull();
    expect(domainOf('')).toBeNull();
  });
});

describe('prettyTag', () => {
  it('swaps hyphens for spaces', () => {
    expect(prettyTag('vision-zero')).toBe('vision zero');
    expect(prettyTag('rider-union')).toBe('rider union');
    expect(prettyTag('safe-streets')).toBe('safe streets');
  });

  it('passes through hyphen-free tags untouched', () => {
    expect(prettyTag('transit')).toBe('transit');
    expect(prettyTag('')).toBe('');
  });

  it('replaces multiple hyphens in one tag', () => {
    expect(prettyTag('bus-rapid-transit')).toBe('bus rapid transit');
  });
});

describe('formatAddedAt', () => {
  it('renders a date-only string as a broadsheet dateline', () => {
    expect(formatAddedAt('2026-05-21')).toBe('May 21, 2026');
  });

  it('keeps the displayed day calendar-correct (no UTC drift)', () => {
    // A bare-date `new Date('2026-01-01')` parses as UTC midnight and
    // prints "Dec 31, 2025" west of UTC. The manual parse must show
    // the calendar day regardless of the runner's timezone.
    expect(formatAddedAt('2026-01-01')).toBe('January 1, 2026');
  });

  it('falls back to the raw input for malformed dates', () => {
    expect(formatAddedAt('')).toBe('');
    expect(formatAddedAt('not-a-date')).toBe('not-a-date');
    expect(formatAddedAt('2026-05')).toBe('2026-05');
  });
});
