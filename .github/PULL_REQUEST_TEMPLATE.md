<!--
  Thanks for the PR. A few quick checks before you click submit:

  - One topic per PR. Refactors, behavior changes, and doc updates
    are easier to review separately.
  - New behavior needs tests. See docs/testing-strategy.md.
  - If you edited api/openapi.yaml, regenerate both halves:
    `just api-oapi-gen && just web-oapi-gen`.
  - `just ci` passes locally before pushing.
  - Full-stack PRs (API + frontend in the same PR): the Cloudflare
    preview URL points at QA, not your branch — review the change
    locally via `just preview` (see CONTRIBUTING.md).
-->

## Summary

<!-- 1–3 sentences. What changed and why. The diff covers the "what";
     this is your chance to explain the "why" so reviewers don't have
     to reverse-engineer it. -->

## Related issues

<!-- Closes #N / refs #N. Skip if standalone. -->

## Test plan

<!-- How you verified this. Be specific — "ran tests" is less useful
     than "added X test, ran `just api-test-integration` against the
     QA seed, smoke-checked /metros for the new field". -->

- [ ]
- [ ]

## Notes for reviewers

<!-- Anything reviewers should pay particular attention to:
     - tricky logic or a non-obvious tradeoff
     - a deliberate departure from convention
     - a follow-up you've decided to defer
     Skip if there's nothing to flag. -->
