import { describe, expect, it } from 'vitest';
import { domainOf, groupCountLabel } from './format.ts';

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
