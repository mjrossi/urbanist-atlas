/// <reference types="node" />
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { render } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { describe, expect, it } from 'vitest';

import { Footer } from './Footer.tsx';

// Read from disk: the Vitest config sets `css: false`.
const css = readFileSync(resolve(process.cwd(), 'src/styles/global.css'), 'utf8');

describe('Footer colophon strip', () => {
  it('names the paper of each theme in its own span', () => {
    const { container } = render(
      <MemoryRouter>
        <Footer />
      </MemoryRouter>,
    );
    const paper = container.querySelector('.colophon-paper:not(.night)');
    const night = container.querySelector('.colophon-paper.night');
    expect(paper?.textContent).toMatch(/cream.*#FAF6F1/);
    expect(night?.textContent).toMatch(/#17130E/);
  });

  it('shows only the active theme’s paper, under both Night triggers', () => {
    expect(css).toMatch(/\.colophon-paper\.night \{\s*display: none;/);
    expect(css).toContain(":root[data-theme='dark'] .colophon-paper.night");
    expect(css).toContain(":root:not([data-theme='light']) .colophon-paper.night");
  });
});
