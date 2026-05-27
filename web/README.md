# Urbanist Atlas — Web

React + Vite SPA for Urbanist Atlas. Phase 1 dogfooding is **live**
at [qa.urbanistatlas.com](https://qa.urbanistatlas.com), deployed to
Cloudflare Workers + Pages (Static Assets) via `wrangler.jsonc` at
the repo root. The production `urbanistatlas.com` hostname attaches
to the same project at Phase 2 cutover (slice #28). The SPA fallback
for direct navigation to client routes (`/about`, `/browse`,
`/region/:regionSlug`, `/r/:postalCode`, `/orgs/:slug`, `/submit`,
`/colophon`) is handled by `assets.not_found_handling =
"single-page-application"` in `wrangler.jsonc` — no `_redirects`
file is needed.

## Layout

```
web/
├── src/
│   ├── main.tsx        # createRoot + QueryClientProvider + RouterProvider
│   ├── router.tsx      # react-router v7 route table
│   ├── App.tsx         # layout shell: Masthead + Outlet + Footer
│   ├── routes/         # one file per route (Home, Results, Browse,
│   │                   #   Region, Org, About, Colophon, Submit,
│   │                   #   NotFound)
│   ├── components/     # Masthead, BroadsheetNav, Footer, …
│   ├── styles/         # global.css ported from mjrossi.com
│   ├── lib/
│   │   ├── api.ts      # typed client for /api/v1
│   │   ├── api.gen.ts  # ← generated; do not edit
│   │   ├── queryKeys.ts
│   │   └── submitForm.ts  # SubmitForm schema + buildIssueBody()
│   └── test/
│       └── renderWithProviders.tsx  # shared test helper
└── public/             # static assets (favicon, noise.webp)
```

## Conventions

See the root `CLAUDE.md` and the approved plan. In short:

- TypeScript, strict mode (`strict` + `noUncheckedIndexedAccess`).
- Routing: `react-router` v7 (SPA / data mode).
- Server state: `@tanstack/react-query` v5. No global client state lib.
- Forms: `react-hook-form`, used in `routes/Submit.tsx`. The Phase 1 submission form composes a pre-filled GitHub issue rather than POSTing to a backend; the Phase 2 in-app queue lands with slices #5 + #26.
- Styling: plain CSS via `src/styles/global.css`. No Tailwind, no CSS-in-JS.
- Fonts: Fraunces + Source Serif 4 + Inter via `@fontsource-variable/*`.
- Tests: Vitest + React Testing Library.
- Package manager: `npm`.

**Before adding any new dependency, confirm with the maintainer.**

## Local dev

```
npm install
npm run dev       # Vite on http://localhost:5173
npm test          # Vitest, watch mode (use `-- --run` for one-shot)
npm run lint
npm run build
```

Point the SPA at a non-default API base by setting `VITE_API_BASE` in
`.env.local` (e.g. `VITE_API_BASE=https://qa-api.urbanistatlas.com`).
The default is `http://localhost:8080`, which matches
`cd ../api && just api-run`.

During Phase 1 dogfooding (CLAUDE.md § Launch strategy), the API
checks an `X-Atlas-Client` header against a shared secret. The SPA
sources its copy of the secret from `VITE_API_CLIENT_SECRET`; if
unset the header isn't sent and the backend's empty-secret no-op
keeps local dev working. See [`.env.example`](./.env.example) for
the full list of env vars. **For Cloudflare Pages deploys these
two values live in the Cloudflare Workers project settings (or in
GitHub Actions secrets piped through `wrangler deploy`), not in any
committed `.env` file.**

## API client and types

`src/lib/api.gen.ts` is **machine-generated** from
[`api/openapi.yaml`](../api/openapi.yaml) — never edit by hand. Whenever
the spec changes upstream, regenerate it:

```
npm run generate:api
```

`src/lib/api.ts` imports the wire shapes (`LookupResult`, `Org`,
`Region`, `ProblemDetails`, `RegionSummary`, `RegionDetail`, `Meta`,
etc.) from `api.gen.ts`, so they stay in lockstep with the
contract. It exposes:

- `apiFetch<T>(path, init?)` — low-level fetch wrapper that throws
  `ApiError` on non-2xx responses, parsing `application/problem+json`
  bodies into a typed `ProblemDetails`.
- `lookup(postal_code, country)` — typed wrapper for
  `GET /api/v1/lookup`.
- `listRegions(init?)`, `getRegion(slug)`, `listRecent()` — typed
  wrappers for the browse + recent endpoints. `listRegions`
  returns the editorial default browse set (metros + cities), with
  each summary carrying a `browse_parent_slug` so the SPA can nest
  cities under their parent metro. `getRegion` returns the focus
  region plus the orgs in scope for it (the region itself or any
  DAG descendant), bucketed by `scope_tier` into `local` and
  `regional`, plus the breadcrumb `ancestry`. Ancestor orgs do NOT
  surface — that's `/lookup`'s job; see
  [`docs/superpowers/specs/2026-05-18-browse-endpoints-design.md`](../docs/superpowers/specs/2026-05-18-browse-endpoints-design.md)
  postscript for the rationale. The list endpoint deliberately
  ships without query parameters today. Collection responses arrive
  as `{ meta, data }` envelopes (slice #24); these helpers unwrap
  `data` so call sites see plain arrays.
- `getOrg(slug)` — typed wrapper for `GET /api/v1/orgs/{slug}`.

`src/lib/queryKeys.ts` centralizes the `@tanstack/react-query` keys so
cache invalidation has a single source of truth.
