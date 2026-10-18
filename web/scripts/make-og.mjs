// Regenerates public/og-image.png, the 1200×630 social link card.
//
// The card mirrors the masthead: the folio line between rules, the
// wordmark with "Atlas" in --amber-soft, the italic tagline between
// double rules, and a three-column foot. Colours are read from the Paper
// `:root` block of src/styles/global.css on every run, so a palette change
// can't leave the card behind. Fonts are the self-hosted @fontsource
// files the site ships.
//
// It renders by screenshotting a generated HTML page in headless Chrome.
// No npm dependency is needed: set CHROME to a Chrome or Chromium binary,
// or let the script find a usual install.
//
//   npm run og                 # writes public/og-image.png
//   npm run og -- out.png      # writes elsewhere
//
// After a redraw, bump the `?v=` on og:image in index.html. Scrapers cache
// cards by URL and rarely re-fetch, so an unchanged URL keeps the old card.
//
// The card states identity only, never a fact about the present (an
// edition, a year, a count). It is cached indefinitely, so such a fact
// would go stale where it can't be corrected. The family core keeps those
// out of images.
import { execFileSync } from 'node:child_process';
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const web = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const out = resolve(process.cwd(), process.argv[2] ?? join(web, 'public/og-image.png'));

// ── Tokens: the Paper :root block of global.css ──────────────────────
const css = readFileSync(join(web, 'src/styles/global.css'), 'utf8');
const start = css.indexOf(':root {');
const body = css
  .slice(start + ':root {'.length, css.indexOf('}', start))
  .replace(/\/\*[\s\S]*?\*\//g, '');
const paper = new Map();
for (const decl of body.split(';')) {
  const colon = decl.indexOf(':');
  if (colon !== -1) paper.set(decl.slice(0, colon).trim(), decl.slice(colon + 1).trim());
}
const tokens = [
  '--paper',
  '--ink',
  '--muted',
  '--rule',
  '--amber',
  '--amber-soft',
  '--amber-rule',
  '--amber-tagline',
]
  .map((name) => {
    const value = paper.get(name);
    if (!value) throw new Error(`global.css: Paper token ${name} not found`);
    return `${name}: ${value};`;
  })
  .join('\n  ');

// ── Fonts ─────────────────────────────────────────────────────────────
const font = (pkg, file) =>
  pathToFileURL(join(web, 'node_modules/@fontsource-variable', pkg, 'files', file)).href;

const html = `<!doctype html>
<html><head><meta charset="utf-8">
<style>
@font-face { font-family: 'Fraunces'; font-style: normal; font-weight: 100 900; src: url('${font('fraunces', 'fraunces-latin-full-normal.woff2')}') format('woff2'); }
@font-face { font-family: 'Fraunces'; font-style: italic; font-weight: 100 900; src: url('${font('fraunces', 'fraunces-latin-full-italic.woff2')}') format('woff2'); }
@font-face { font-family: 'Inter'; font-style: normal; font-weight: 100 900; src: url('${font('inter', 'inter-latin-wght-normal.woff2')}') format('woff2'); }
:root {
  ${tokens}
}
* { box-sizing: border-box; margin: 0; }
html, body { width: 1200px; height: 630px; background: var(--paper); overflow: hidden; }
.frame { position: absolute; inset: 26px; border: 1px solid var(--rule); }
.caps { font-family: 'Inter'; font-weight: 600; font-size: 16px; letter-spacing: 0.22em; text-transform: uppercase; }
.folio { position: absolute; left: 60px; right: 60px; top: 74px; display: flex; align-items: center; gap: 24px; color: var(--amber-tagline); }
.folio .rule { flex: 1; height: 1px; background: var(--amber-rule); }
.folio .dot { color: var(--amber-rule); margin: 0 0.7em; }
.name { position: absolute; left: 0; right: 0; top: 196px; text-align: center; font-family: 'Fraunces'; font-weight: 600; font-size: 156px; line-height: 1; letter-spacing: -0.04em; color: var(--ink); font-variation-settings: 'opsz' 144, 'SOFT' 30; }
.name .surname { color: var(--amber-soft); margin-left: 0.18em; }
.tag { position: absolute; left: 160px; right: 160px; top: 410px; display: flex; align-items: center; gap: 22px; }
.tag .rule { flex: 1; height: 4px; border-top: 1px solid var(--amber-rule); border-bottom: 1px solid var(--amber-rule); }
.tag span:not(.rule) { font-family: 'Fraunces'; font-style: italic; font-weight: 400; font-size: 29px; color: var(--amber-tagline); white-space: nowrap; }
.foot { position: absolute; left: 60px; right: 60px; top: 552px; display: flex; justify-content: space-between; color: var(--muted); }
.foot .url { color: var(--amber); }
</style></head>
<body>
  <div class="frame"></div>
  <div class="folio caps"><span class="rule"></span><span>A directory<span class="dot">·</span>Vol. I</span><span class="rule"></span></div>
  <div class="name">Urbanist<span class="surname">Atlas</span></div>
  <div class="tag"><span class="rule"></span><span>Transit &amp; safe-streets advocacy near you</span><span class="rule"></span></div>
  <div class="foot caps"><span>United States &amp; Canada</span><span class="url">urbanistatlas.com</span><span>Search by postal code</span></div>
</body></html>
`;

// ── Render ────────────────────────────────────────────────────────────
function findChrome() {
  if (process.env.CHROME) return process.env.CHROME;
  const candidates = [
    '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
    '/Applications/Chromium.app/Contents/MacOS/Chromium',
    '/usr/bin/google-chrome',
    '/usr/bin/chromium',
    '/usr/bin/chromium-browser',
  ];
  const found = candidates.find((path) => existsSync(path));
  if (!found) throw new Error('No Chrome or Chromium found; set CHROME to its binary.');
  return found;
}

const dir = mkdtempSync(join(tmpdir(), 'atlas-og-'));
try {
  const page = join(dir, 'og.html');
  writeFileSync(page, html);
  execFileSync(
    findChrome(),
    [
      '--headless',
      '--disable-gpu',
      '--hide-scrollbars',
      '--allow-file-access-from-files',
      '--force-device-scale-factor=1',
      '--window-size=1200,630',
      // Lets the @font-face files load before the screenshot is taken.
      '--virtual-time-budget=5000',
      `--screenshot=${out}`,
      pathToFileURL(page).href,
    ],
    { stdio: ['ignore', 'ignore', 'pipe'] },
  );
} finally {
  rmSync(dir, { recursive: true, force: true });
}
console.log(`wrote ${out}`);
