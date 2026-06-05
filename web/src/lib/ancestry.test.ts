import { describe, expect, it } from 'vitest';

import { reverseAncestry } from './ancestry.ts';

describe('reverseAncestry', () => {
  it('reverses a leaf-first list into root-first order', () => {
    const leafFirst = ['brooklyn', 'nyc-metro', 'new-york-state'];
    expect(reverseAncestry(leafFirst)).toEqual([
      'new-york-state',
      'nyc-metro',
      'brooklyn',
    ]);
  });

  it('does not mutate the input', () => {
    const input = ['a', 'b', 'c'];
    reverseAncestry(input);
    expect(input).toEqual(['a', 'b', 'c']);
  });

  it('returns an empty array for an empty input', () => {
    expect(reverseAncestry([])).toEqual([]);
  });

  it('handles a single-element list', () => {
    expect(reverseAncestry(['only'])).toEqual(['only']);
  });
});
