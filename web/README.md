# Urbanist Atlas — Web

React + Vite SPA for [urbanistatlas.com](https://urbanistatlas.com). Deployed
to Cloudflare Pages.

## Layout (once scaffolded)

```
web/
├── src/
│   ├── main.tsx        # createRoot + RouterProvider + QueryClientProvider
│   ├── router.tsx      # react-router v7 route table
│   ├── routes/         # one file per route (Home, Results, Submit, …)
│   ├── components/     # Masthead, Dateline, EntryList, SearchBox, TagChip…
│   ├── styles/         # global.css ported from mjrossi.com
│   └── lib/
│       ├── api.ts      # typed client for /api/v1
│       └── queryKeys.ts
└── public/             # static assets (favicon, etc.)
```

## Conventions

See the root `CLAUDE.md` and the approved plan. In short:

- TypeScript, strict mode.
- Routing: `react-router` v7 (SPA / data mode).
- Server state: `@tanstack/react-query` v5. No global client state lib.
- Forms: `react-hook-form` for the submission form.
- Styling: plain CSS via `src/styles/global.css`. No Tailwind, no CSS-in-JS.
- Fonts: Fraunces + Source Serif 4 + Inter via `@fontsource-variable/*`.
- Tests: Vitest + React Testing Library.
- Package manager: `npm`.

**Before adding any new dependency, confirm with the maintainer.**

## Local dev (once scaffolded)

```
npm install
npm run dev      # Vite on http://localhost:5173
npm test
npm run build
```
