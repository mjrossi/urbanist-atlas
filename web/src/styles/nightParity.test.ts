/// <reference types="node" />
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { describe, expect, it } from 'vitest';

// Read from disk: the Vitest config sets `css: false`, which also empties
// `?raw` CSS imports.
// Vitest runs from web/ (the jsdom environment rewrites import.meta.url).
const css = readFileSync(resolve(process.cwd(), 'src/styles/global.css'), 'utf8');

// The Night palette is written twice in global.css — once for an explicit
// `data-theme="dark"` and once for the system preference — because CSS
// can't share one declaration block between a selector and a media query.
// The Lexicon Family core requires the two copies to stay identical; this
// test is what holds them to it.

// Font stacks are theme-independent; every other Paper token needs a Night value.
const themeIndependent = ['--serif', '--serif-text', '--sans', '--mono'];

function declarations(selector: string): Map<string, string> {
  const start = css.indexOf(`${selector} {`);
  if (start === -1) throw new Error(`missing block ${selector}`);
  const end = css.indexOf('}', start);
  // Strip comments first: one containing `:` or `;` would split a declaration.
  const body = css
    .slice(start + selector.length + 2, end)
    .replace(/\/\*[\s\S]*?\*\//g, '');
  const decls = new Map<string, string>();
  for (const line of body.split(';')) {
    const colon = line.indexOf(':');
    if (colon === -1) continue;
    decls.set(line.slice(0, colon).trim(), line.slice(colon + 1).trim());
  }
  return decls;
}

describe('Night theme blocks', () => {
  const explicit = declarations(":root[data-theme='dark']");
  const system = declarations(":root:not([data-theme='light'])");

  it('declare the same tokens with the same values', () => {
    expect([...system.entries()].sort()).toEqual([...explicit.entries()].sort());
  });

  it('redefine every Paper token, and nothing Paper lacks', () => {
    const paper = [...declarations(':root').keys()].filter(
      (k) => !themeIndependent.includes(k),
    );
    expect([...explicit.keys()].sort()).toEqual(paper.sort());
    expect(explicit.get('color-scheme')).toBe('dark');
  });

  it('are screen-only, so print always gets Paper', () => {
    expect(css).toContain("@media screen {\n  :root[data-theme='dark'] {");
    expect(css).toContain(
      "@media screen and (prefers-color-scheme: dark) {\n  :root:not([data-theme='light']) {",
    );
  });
});
