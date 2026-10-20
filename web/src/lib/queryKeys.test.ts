import { describe, expect, it } from 'vitest';

import { queryKeys } from './queryKeys.ts';

describe('queryKeys.regionSearch', () => {
  it('includes the limit, defaulting to the combobox page size', () => {
    expect(queryKeys.regionSearch('port')).toEqual(['regionSearch', 'port', 8]);
    expect(queryKeys.regionSearch('port', 20)).toEqual(['regionSearch', 'port', 20]);
  });

  it('sits outside the regions prefix, so invalidating regions keeps searches', () => {
    const [regionsPrefix] = queryKeys.regions();
    expect(queryKeys.regionSearch('port')[0]).not.toBe(regionsPrefix);
  });
});
