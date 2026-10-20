/// <reference types="node" />
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { describe, expect, it } from 'vitest';

const html = readFileSync(resolve(process.cwd(), 'index.html'), 'utf8');
const doc = new DOMParser().parseFromString(html, 'text/html');

describe('index.html', () => {
  it('advertises the production OpenAPI document', () => {
    const link = doc.querySelector('link[rel="alternate"][type="application/yaml"]');
    expect(link?.getAttribute('href')).toBe(
      'https://api.urbanistatlas.com/api/v1/openapi.yaml',
    );
  });

  it('carries one theme-color meta per scheme for theme.js to switch', () => {
    const metas = [...doc.querySelectorAll('meta[name="theme-color"]')];
    expect(metas.map((m) => m.getAttribute('data-scheme'))).toEqual(['light', 'dark']);
    for (const m of metas) {
      expect(m.getAttribute('media')).toBe(
        `(prefers-color-scheme: ${m.getAttribute('data-scheme') ?? ''})`,
      );
    }
  });
});
