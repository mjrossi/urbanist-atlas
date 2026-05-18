# Urbanist Atlas — Web

React + Vite SPA for [urbanistatlas.com](https://urbanistatlas.com). Deployed
to Cloudflare Pages.

## Layout

```
web/
├── src/
│   ├── main.tsx        # createRoot + QueryClientProvider + RouterProvider
│   ├── router.tsx      # react-router v7 route table
│   ├── App.tsx         # layout shell: Masthead + Outlet + Footer
│   ├── routes/         # one file per route (Home, Results,
│   │                   #   Browse, Metro, About, NotFound)
│   ├── components/     # Masthead, BroadsheetNav, Footer, …
│   ├── styles/         # global.css ported from mjrossi.com
│   └── lib/
│       ├── api.ts      # typed client for /api/v1
│       ├── api.gen.ts  # ← generated; do not edit
│       └── queryKeys.ts
└── public/             # static assets (favicon, noise.webp)
```

## Conventions

See the root `CLAUDE.md` and the approved plan. In short:

- TypeScript, strict mode (`strict` + `noUncheckedIndexedAccess`).
- Routing: `react-router` v7 (SPA / data mode).
- Server state: `@tanstack/react-query` v5. No global client state lib.
- Forms: `react-hook-form` is the pre-approved choice; not yet installed (the submission form, slice #13, is deferred to Phase 2 alongside the account model).
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
`.env.local` (e.g. `VITE_API_BASE=https://api.urbanistatlas.com`). The
default is `http://localhost:8080`, which matches `cd ../api && just api-run`.

## API client and types

`src/lib/api.gen.ts` is **machine-generated** from
[`api/openapi.yaml`](../api/openapi.yaml) — never edit by hand. Whenever
the spec changes upstream, regenerate it:

```
npm run generate:api
```

`src/lib/api.ts` imports the wire shapes (`LookupResult`, `Org`,
`Region`, `ProblemDetails`, `MetroSummary`, `MetroDetail`, `Meta`,
etc.) from `api.gen.ts`, so they stay in lockstep with the
contract. It exposes:

- `apiFetch<T>(path, init?)` — low-level fetch wrapper that throws
  `ApiError` on non-2xx responses, parsing `application/problem+json`
  bodies into a typed `ProblemDetails`.
- `lookup(postal_code, country)` — typed wrapper for
  `GET /api/v1/lookup`.
- `listMetros()`, `getMetro(slug)`, `listRecent()` — typed wrappers
  for the browse + recent endpoints. Collection responses arrive as
  `{ meta, data }` envelopes (slice #24); these helpers unwrap
  `data` so call sites see plain arrays.

`src/lib/queryKeys.ts` centralizes the `@tanstack/react-query` keys so
cache invalidation has a single source of truth.
