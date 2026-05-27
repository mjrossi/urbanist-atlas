# Security policy

## Reporting a vulnerability

**Please don't open a public issue for security problems.**

Use GitHub's [private vulnerability reporting][gh-pvr] for this
repository:

1. Go to the repo's **Security** tab on GitHub.
2. Click **Report a vulnerability**.
3. Fill in what you've found, how to reproduce, and the impact you
   see.

The report stays private to the maintainer until a fix is ready
and coordinated disclosure can happen. If you don't have a GitHub
account or PVR isn't available, open a public issue titled
"Security: request private contact" with no details, and the
maintainer will follow up.

## What's in scope

Anything that could compromise:

- **Read paths** — the public JSON API at `/api/v1/**`, the SPA at
  `urbanistatlas.com`.
- **Auth boundaries** — the Phase 1 `X-Atlas-Client` shared-secret
  gate; the future Phase 2 API-key system.
- **Data integrity** — anything that could let an unauthenticated
  user mutate the directory or admin queue.
- **Operational integrity** — anything that could disclose the
  database credentials, secrets, or deploy infrastructure.

Out of scope, generally:

- Reports that boil down to "you bundled `VITE_API_CLIENT_SECRET`
  into the SPA, so any user can read it." That's documented
  behavior of the Phase 1 gate
  (see [`docs/api-architecture.md`](./docs/api-architecture.md#phase-1-gate--x-atlas-client)
  and [`CLAUDE.md`](./CLAUDE.md#launch-strategy)) — the gate is a
  deterrent against casual scrapers, not a security boundary
  against motivated attackers. Phase 2 replaces it with real
  per-user keys.
- Reports against third-party services we depend on (Fly.io,
  Cloudflare, GitHub) — report to them directly.
- Theoretical issues without a proof of concept against the live
  QA endpoint or a local checkout.

## Response expectations

This is a personal-time project with one maintainer, not a team
on a rotation. Best-effort response targets:

| Severity | Initial reply | Fix in QA |
|---|---|---|
| Critical (active exploit, data exposure) | 2 business days | 7 days |
| High (auth bypass, integrity loss) | 5 business days | 30 days |
| Medium / Low | 14 business days | next milestone |

Coordinated disclosure preferred. After a fix is deployed, you're
welcome to write up the finding publicly; a credit line in the
release notes is the default if you'd like one.

## Supported versions

The deployed `main` branch is what runs at `urbanistatlas.com` (and
its QA companion). There are no point releases or LTS branches to
back-port to — fixes land on `main` and ship on the next deploy.

[gh-pvr]: https://docs.github.com/en/code-security/security-advisories/guidance-on-reporting-and-writing-information-about-vulnerabilities/privately-reporting-a-security-vulnerability
