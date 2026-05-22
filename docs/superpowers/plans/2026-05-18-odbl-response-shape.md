# ODbL response shape — implementation plan (slice #24)

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Use `superpowers:test-driven-development` for every code-bearing step.

**Goal:** Ship the ODbL attribution wire-contract change (two response headers on every `/api/v1/**` response, a `{ meta, data }` envelope on collection responses) with the OpenAPI spec, Go middleware/helper, and frontend client all updated and tested.

**Architecture:** A new `odbl.go` houses two pieces: `odblHeadersMiddleware` (mounted inside `/api/v1`) and `respondCollection[T]` (called by collection handlers, replaces `writeJSON` for the two endpoints that return arrays). `meta.generated_at` is per-request `time.Now().UTC()` formatted as RFC3339. OpenAPI gets three new schemas (`Meta`, `MetroSummariesEnvelope`, `RecentEnvelope`) and two new header refs (`XDataLicense`, `XDataAttribution`). Frontend's `listMetros` / `listRecent` unwrap `.data` once so consumers see the same shape they always have.

**Tech Stack:** Go 1.26, chi, oapi-codegen, openapi-typescript, Vitest. No new dependencies.

**Spec:** [`docs/superpowers/specs/2026-05-18-odbl-response-shape-design.md`](../specs/2026-05-18-odbl-response-shape-design.md). Read **§4 (wrapping strategy)**, **§5 (helper signature)**, **§6 (OpenAPI changes)**, and **§8 (tests)** before starting.

**Preconditions:**

1. Working in worktree `.worktrees/odbl-backend` on branch `slice-24-odbl-response-shape`, branched from `main` at commit `94275b6`.
2. Docker is running and the dev Postgres on `:55432` is healthy (`just pg-up` is fine; data isn't strictly required for unit tests but is required for the curl smoke at the end).
3. `just ci` is green on baseline. If not, stop and report.
4. `git diff` reports clean before starting.

---

## File Structure

### New

| Path | Responsibility |
|---|---|
| `api/internal/httpapi/odbl.go` | `odblHeadersMiddleware`, `respondCollection[T]`, `newMeta`, the two header/license constants. |
| `api/internal/httpapi/odbl_test.go` | Middleware presence/absence, `respondCollection` wrap behavior + nil-safe, `newMeta` shape. |

### Modified

| Path | Change |
|---|---|
| `api/openapi.yaml` | Add `Meta`, `MetroSummariesEnvelope`, `RecentEnvelope` schemas; add `XDataLicense`, `XDataAttribution` headers; switch `/api/v1/metros` and `/api/v1/recent` 200 schemas to the envelope; add header refs to every `/api/v1/**` 200 response declaration; update `info.description`. |
| `api/internal/httpapi/oapi/types.gen.go` | Regenerated. |
| `api/internal/httpapi/openapi.yaml` | Refreshed copy (kept in sync with canonical via `//go:generate`). |
| `web/src/lib/api.gen.ts` | Regenerated. |
| `api/internal/httpapi/router.go` | One `r.Use(odblHeadersMiddleware)` line inside the `/api/v1` route group. |
| `api/internal/httpapi/metros.go` | `listMetrosHandler` calls `respondCollection` instead of `writeJSON`. |
| `api/internal/httpapi/recent.go` | `recentHandler` calls `respondCollection`. |
| `api/internal/httpapi/metros_test.go` | Happy-path test decodes envelope, asserts headers + meta block. |
| `api/internal/httpapi/recent_test.go` | Same as metros. |
| `web/src/lib/api.ts` | Import envelope types; `listMetros` and `listRecent` unwrap `.data`; re-export `Meta` / envelope types. |
| `web/src/lib/api.test.ts` | Existing tests return enveloped JSON from the fetch mock; one new test per helper asserts unwrap on a non-empty body. |

---

## Tasks

### Phase 0 — baseline

- [ ] **Step 0.1: Confirm worktree state**

```bash
cd .worktrees/odbl-backend
git rev-parse --abbrev-ref HEAD
git status --short
```

Expected:
- branch: `slice-24-odbl-response-shape`
- status: clean

- [ ] **Step 0.2: Confirm baseline `just ci` is green**

```bash
cd .worktrees/odbl-backend
just ci
```

Expected: all green. If anything fails, STOP and report — the baseline is broken and this plan can't proceed.

- [ ] **Step 0.3: Confirm generated files are clean**

```bash
cd .worktrees/odbl-backend
just api-oapi-gen
git diff --stat
```

Expected: no diff. (Confirms the openapi-yaml ↔ generated types are in sync at baseline.)

---

### Phase 1 — Commit 1: OpenAPI spec changes + regenerate types

The goal of this commit is to lock in the new wire contract types in both languages with NO behavior change. After this commit, the handlers still emit bare arrays — but the existing handler tests still decode `[]oapi.MetroSummary` so they pass. The generated types compile against the existing handler code because the new envelope types are additive (the bare array types still exist).

- [ ] **Step 1.1: Add `Meta` schema to `api/openapi.yaml`**

Open `api/openapi.yaml`. Find the `components/schemas` block (starts around line 432). Add `Meta` immediately after `Country` and before `ScopeTier` (alphabetical clustering isn't strict in this file, but proximity to the other small primitive schemas reads best). Insert before the line containing `    ScopeTier:`:

```yaml
    Meta:
      type: object
      description: |
        Attribution block included on every collection response.
        Carries the ODbL license obligation in-band so consumers
        that strip headers still see the share-alike requirement.
      required: [license, attribution_url, generated_at]
      properties:
        license:
          type: string
          example: ODbL-1.0
          description: |
            SPDX identifier of the data license. Stable for the
            lifetime of v1 (`ODbL-1.0`).
        attribution_url:
          type: string
          format: uri
          example: https://urbanistatlas.com
          description: |
            URL downstream consumers should link to when crediting
            the source.
        generated_at:
          type: string
          format: date-time
          example: "2026-05-18T12:34:56Z"
          description: |
            RFC 3339 timestamp recording when the response was
            produced server-side. Per-request — there is no caching
            layer in v1.

```

- [ ] **Step 1.2: Add `MetroSummariesEnvelope` and `RecentEnvelope` schemas**

Still in `api/openapi.yaml`, find the `MetroDetail:` schema (around line 618). Add the two envelopes immediately after it (and before `SubmissionPayload:`):

```yaml
    MetroSummariesEnvelope:
      type: object
      description: |
        Collection envelope for `GET /api/v1/metros`. Wraps the
        metro list in a `meta` + `data` shape so every list
        response carries the ODbL attribution alongside its
        payload, even when transport-level headers are stripped.
      required: [meta, data]
      properties:
        meta:
          $ref: '#/components/schemas/Meta'
        data:
          type: array
          items:
            $ref: '#/components/schemas/MetroSummary'

    RecentEnvelope:
      type: object
      description: |
        Collection envelope for `GET /api/v1/recent`. Same shape
        contract as `MetroSummariesEnvelope`.
      required: [meta, data]
      properties:
        meta:
          $ref: '#/components/schemas/Meta'
        data:
          type: array
          items:
            $ref: '#/components/schemas/Org'

```

- [ ] **Step 1.3: Add `XDataLicense` and `XDataAttribution` to `components/headers`**

Still in `api/openapi.yaml`, there is no `headers:` block under `components:` yet. Find the `components:` line (around line 335), then the `responses:` sub-block (around line 380). Add a new `headers:` sub-block immediately before `responses:`:

```yaml
  headers:
    XDataLicense:
      description: |
        SPDX identifier of the dataset license. Stable for the
        lifetime of v1.
      schema:
        type: string
        example: ODbL-1.0
    XDataAttribution:
      description: |
        URL downstream consumers should link to when crediting
        the source.
      schema:
        type: string
        format: uri
        example: https://urbanistatlas.com

```

- [ ] **Step 1.4: Update `/api/v1/lookup` 200 response to advertise the two headers**

In `api/openapi.yaml`, find the `/api/v1/lookup:` path block. Locate the `'200':` response (around line 119). The current shape is:

```yaml
        '200':
          description: Lookup succeeded.
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/LookupResult'
```

Replace with:

```yaml
        '200':
          description: Lookup succeeded.
          headers:
            X-Data-License:
              $ref: '#/components/headers/XDataLicense'
            X-Data-Attribution:
              $ref: '#/components/headers/XDataAttribution'
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/LookupResult'
```

- [ ] **Step 1.5: Update `/api/v1/metros` 200 response — headers + envelope schema**

In `api/openapi.yaml`, find the `/api/v1/metros:` path block. Locate the `'200':` response (around line 144). The current shape is:

```yaml
        '200':
          description: Metros with counts, ordered by org count descending.
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: '#/components/schemas/MetroSummary'
```

Replace with:

```yaml
        '200':
          description: |
            Metros with counts, ordered by org count descending.
            Wrapped in a `{ meta, data }` envelope; the
            `MetroSummary[]` lives at `data`.
          headers:
            X-Data-License:
              $ref: '#/components/headers/XDataLicense'
            X-Data-Attribution:
              $ref: '#/components/headers/XDataAttribution'
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/MetroSummariesEnvelope'
```

- [ ] **Step 1.6: Update `/api/v1/metros/{slug}` 200 response — headers only (single resource, no envelope)**

In `api/openapi.yaml`, find the `/api/v1/metros/{slug}:` path block. The current 200 response (around line 163) is:

```yaml
        '200':
          description: The metro region and its organizations.
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/MetroDetail'
```

Replace with:

```yaml
        '200':
          description: The metro region and its organizations.
          headers:
            X-Data-License:
              $ref: '#/components/headers/XDataLicense'
            X-Data-Attribution:
              $ref: '#/components/headers/XDataAttribution'
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/MetroDetail'
```

- [ ] **Step 1.7: Update `/api/v1/recent` 200 response — headers + envelope schema**

In `api/openapi.yaml`, find the `/api/v1/recent:` path block. The current 200 response (around line 184) is:

```yaml
        '200':
          description: Recently approved organizations.
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: '#/components/schemas/Org'
```

Replace with:

```yaml
        '200':
          description: |
            Recently approved organizations. Wrapped in a
            `{ meta, data }` envelope; the `Org[]` lives at `data`.
          headers:
            X-Data-License:
              $ref: '#/components/headers/XDataLicense'
            X-Data-Attribution:
              $ref: '#/components/headers/XDataAttribution'
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/RecentEnvelope'
```

- [ ] **Step 1.8: Update `info.description` to mention the headers + envelope**

In `api/openapi.yaml`, find the `info:` block at the top. The `description:` paragraph currently lists conventions in a bullet list (around line 17). Locate this bullet:

```yaml
    - Every response includes an `X-Request-ID` header; error response
      bodies also echo it as the `request_id` extension field so
      clients can quote it when reporting bugs.
```

Add a NEW bullet immediately after it:

```yaml
    - Every `/api/v1/**` response carries `X-Data-License: ODbL-1.0`
      and `X-Data-Attribution: https://urbanistatlas.com` headers
      announcing the dataset license (ODbL 1.0; see `LICENSE-DATA`
      in the repo). Collection responses additionally carry a `meta`
      envelope with `license`, `attribution_url`, and `generated_at`
      (RFC3339 UTC) — the array lives at `data`. The envelope keeps
      the license obligation in-band when transport headers are
      stripped.
```

- [ ] **Step 1.9: Regenerate Go types and refresh embedded copy**

```bash
cd .worktrees/odbl-backend
just api-oapi-gen
```

Expected:
- exits 0
- `git status` shows changes in `api/internal/httpapi/oapi/types.gen.go` and `api/internal/httpapi/openapi.yaml`

Quick sanity check:

```bash
cd .worktrees/odbl-backend
grep -c "type Meta " api/internal/httpapi/oapi/types.gen.go
grep -c "type MetroSummariesEnvelope " api/internal/httpapi/oapi/types.gen.go
grep -c "type RecentEnvelope " api/internal/httpapi/oapi/types.gen.go
```

Expected: each grep returns `1`.

- [ ] **Step 1.10: Regenerate TS types**

```bash
cd .worktrees/odbl-backend
npm --prefix web run generate:api
```

Expected:
- exits 0
- `git status` shows changes in `web/src/lib/api.gen.ts`

Quick sanity check:

```bash
cd .worktrees/odbl-backend
grep -c "MetroSummariesEnvelope" web/src/lib/api.gen.ts
grep -c "RecentEnvelope" web/src/lib/api.gen.ts
grep -c "Meta:" web/src/lib/api.gen.ts
```

Expected: each grep returns at least `1`.

- [ ] **Step 1.11: Run `just ci` — must stay green**

```bash
cd .worktrees/odbl-backend
just ci
```

Expected: all green. The handlers still emit bare arrays (which the OpenAPI now says are envelopes), but `just ci` doesn't run the live server — it runs unit tests against compiled Go types and against the lint/test stack. The existing handler tests decode `[]oapi.MetroSummary` directly, which still compiles because that bare-array type still exists in the generated file. The mismatch between handler output and the spec's declared shape is the bug we'll fix in Phase 2.

If `just ci` fails, STOP and inspect — most likely cause is a typo in the YAML.

- [ ] **Step 1.12: Commit**

```bash
cd .worktrees/odbl-backend
git add api/openapi.yaml \
        api/internal/httpapi/oapi/types.gen.go \
        api/internal/httpapi/openapi.yaml \
        web/src/lib/api.gen.ts
git status
git diff --cached --stat
git commit -m "$(cat <<'EOF'
feat(api): openapi — Meta schema + collection envelope (slice #24)

Adds the wire-contract pieces of slice #24 without changing any
behavior:

- New `Meta` schema (license / attribution_url / generated_at).
- New `MetroSummariesEnvelope` and `RecentEnvelope` schemas wrapping
  the `/api/v1/metros` and `/api/v1/recent` 200 bodies.
- New `XDataLicense` and `XDataAttribution` headers in
  `components/headers`, referenced from every `/api/v1/**` 200
  response declaration.
- `/api/v1/metros` and `/api/v1/recent` 200 schemas switch from
  bare arrays to the envelope.
- `info.description` documents the new convention.
- Regenerated `oapi/types.gen.go` and `web/src/lib/api.gen.ts`.

No handler or client code changes in this commit; the next commit
(middleware + helper + handler updates) flips the wire to match the
declared shape. `just ci` is green at this checkpoint because the
existing bare-array types still exist in the generated file and the
existing handler tests still decode them.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Expected: commit succeeds, pre-commit hook (if any) passes.

---

### Phase 2 — Commit 2: ODbL middleware + `respondCollection` helper

The goal of this commit is to actually emit the new shape: headers on every `/api/v1/**` response, envelope on the two collection endpoints. TDD: failing tests first, then implementation, then update existing tests to decode the new shape.

- [ ] **Step 2.1: Write `api/internal/httpapi/odbl_test.go` (RED phase)**

Create the file with the full test contents below. Do NOT create `odbl.go` yet; we want to see the tests fail with "undefined" errors before implementing.

```go
package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mjrossi/urbanist-atlas/api/internal/httpapi/oapi"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// TestODbLHeaders_PresentOnAPISuccessResponse asserts the middleware
// puts both attribution headers on every /api/v1/** 200.
func TestODbLHeaders_PresentOnAPISuccessResponse(t *testing.T) {
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/api/v1/metros")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if got, want := resp.Header.Get("X-Data-License"), dataLicense; got != want {
		t.Errorf("X-Data-License: want %q, got %q", want, got)
	}
	if got, want := resp.Header.Get("X-Data-Attribution"), dataAttributionURL; got != want {
		t.Errorf("X-Data-Attribution: want %q, got %q", want, got)
	}
}

// TestODbLHeaders_AbsentOnHealthz pins the path-scoping decision:
// /healthz isn't a data endpoint and must NOT carry the attribution
// headers. If someone reroutes the middleware to the router root,
// this test fails.
func TestODbLHeaders_AbsentOnHealthz(t *testing.T) {
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("X-Data-License"); got != "" {
		t.Errorf("X-Data-License on /healthz: want empty, got %q", got)
	}
	if got := resp.Header.Get("X-Data-Attribution"); got != "" {
		t.Errorf("X-Data-Attribution on /healthz: want empty, got %q", got)
	}
}

// TestODbLHeaders_PresentOnAPIErrorResponse documents the
// headers-on-every-response decision: even a 404 problem document
// under /api/v1 carries the attribution headers. Cheaper than
// status-sniffing and arguably more honest.
func TestODbLHeaders_PresentOnAPIErrorResponse(t *testing.T) {
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/api/v1/metros/totally-bogus")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status: want 404, got %d", resp.StatusCode)
	}
	if got, want := resp.Header.Get("X-Data-License"), dataLicense; got != want {
		t.Errorf("X-Data-License: want %q, got %q", want, got)
	}
	if got, want := resp.Header.Get("X-Data-Attribution"), dataAttributionURL; got != want {
		t.Errorf("X-Data-Attribution: want %q, got %q", want, got)
	}
}

// TestRespondCollection_WrapsItemsAndSetsHeaders covers the helper's
// shape contract against a httptest.ResponseRecorder so we don't have
// to spin up a server. Asserts:
//   - status 200, Content-Type application/json
//   - meta.license / meta.attribution_url / meta.generated_at present
//   - data is the items passed in, in order
func TestRespondCollection_WrapsItemsAndSetsHeaders(t *testing.T) {
	w := httptest.NewRecorder()
	items := []oapi.MetroSummary{
		{Region: oapi.Region{Slug: "a"}, OrgCount: 3},
		{Region: oapi.Region{Slug: "b"}, OrgCount: 1},
	}
	respondCollection(w, items)

	resp := w.Result()
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: want 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type: want application/json prefix, got %q", ct)
	}
	var env oapi.MetroSummariesEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if env.Meta.License != dataLicense {
		t.Errorf("meta.license: want %q, got %q", dataLicense, env.Meta.License)
	}
	if env.Meta.AttributionUrl != dataAttributionURL {
		t.Errorf("meta.attribution_url: want %q, got %q", dataAttributionURL, env.Meta.AttributionUrl)
	}
	if _, err := time.Parse(time.RFC3339, env.Meta.GeneratedAt); err != nil {
		t.Errorf("meta.generated_at: not RFC3339-parseable (%q): %v", env.Meta.GeneratedAt, err)
	}
	if len(env.Data) != 2 {
		t.Fatalf("data length: want 2, got %d", len(env.Data))
	}
	if env.Data[0].Region.Slug != "a" || env.Data[1].Region.Slug != "b" {
		t.Errorf("data order: want [a, b], got [%s, %s]",
			env.Data[0].Region.Slug, env.Data[1].Region.Slug)
	}
}

// TestRespondCollection_NilSlice_EncodesEmptyArray guards against a
// future regression where a handler hands `nil` to respondCollection
// and the JSON encoder writes `"data": null`. Downstream clients can
// assume `data` is always an array.
func TestRespondCollection_NilSlice_EncodesEmptyArray(t *testing.T) {
	w := httptest.NewRecorder()
	respondCollection[oapi.Org](w, nil)

	body := w.Body.String()
	if !strings.Contains(body, `"data":[]`) {
		t.Errorf("body should contain \"data\":[], got %s", body)
	}
}

// TestNewMeta_EmitsRFC3339UTC asserts the helper produces a UTC
// RFC3339 timestamp matching wall-clock now (within a small window).
func TestNewMeta_EmitsRFC3339UTC(t *testing.T) {
	m := newMeta()

	if m.License != dataLicense {
		t.Errorf("license: want %q, got %q", dataLicense, m.License)
	}
	if m.AttributionUrl != dataAttributionURL {
		t.Errorf("attribution_url: want %q, got %q", dataAttributionURL, m.AttributionUrl)
	}
	parsed, err := time.Parse(time.RFC3339, m.GeneratedAt)
	if err != nil {
		t.Fatalf("generated_at: not RFC3339 (%q): %v", m.GeneratedAt, err)
	}
	if parsed.Location() != time.UTC {
		t.Errorf("generated_at: want UTC, got %s", parsed.Location())
	}
	if d := time.Since(parsed); d < 0 || d > 5*time.Second {
		t.Errorf("generated_at: want within 5s of now, got delta %s", d)
	}
}

// Compile-time check: this package's existing newTestServer (in
// lookup_test.go) already provides what we need. The atlas/slog
// imports above are here for the helper test; if go vet complains
// they're unused, drop them.
var _ = atlas.NewMemStore
var _ = slog.LevelInfo
```

(The two `var _ =` lines at the bottom keep `go vet` happy if a refactor later removes the in-test dependencies on those imports; they're harmless. If `goimports` strips them, that's also fine.)

- [ ] **Step 2.2: Run tests to verify they fail with the expected "undefined" errors**

```bash
cd .worktrees/odbl-backend
cd api && go test ./internal/httpapi/... -run "TestODbL|TestRespondCollection|TestNewMeta" -count=1 2>&1 | head -40
```

Expected: compilation failures referencing `dataLicense`, `dataAttributionURL`, `respondCollection`, `newMeta` as undefined. (If you see "no test files" or unrelated test failures, STOP and investigate.)

- [ ] **Step 2.3: Create `api/internal/httpapi/odbl.go` with constants, middleware, helper**

Create the file:

```go
package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/mjrossi/urbanist-atlas/api/internal/httpapi/oapi"
)

// dataLicense is the SPDX identifier of the Urbanist Atlas dataset
// license. The source of truth for the license itself is
// LICENSE-DATA at the repo root; this constant only echoes the
// identifier into response headers and the collection-meta envelope.
const dataLicense = "ODbL-1.0"

// dataAttributionURL is the URL downstream consumers should link to
// when crediting the source. Hardcoded here so the production
// frontend domain is the canonical attribution target even if a
// future config field overrides the CORS origins or the serving host.
const dataAttributionURL = "https://urbanistatlas.com"

// odblHeadersMiddleware sets the two attribution headers on every
// response that passes through it. Mount inside the /api/v1 route
// group so /healthz (which is not a data endpoint) stays clean.
//
// Headers are set BEFORE delegating to next.ServeHTTP so they're
// already in the response writer's header map by the time a handler
// calls WriteHeader — net/http flushes headers on the first body
// write, so ordering matters.
//
// Headers are set unconditionally (success or error). Distinguishing
// 2xx from non-2xx in middleware would require response-status
// sniffing, and an error response that quotes the dataset license
// is harmless. See spec §1 for the rationale.
func odblHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Data-License", dataLicense)
		h.Set("X-Data-Attribution", dataAttributionURL)
		next.ServeHTTP(w, r)
	})
}

// newMeta returns the meta block for a single response: license,
// attribution URL, and a fresh RFC3339 timestamp in UTC.
//
// `generated_at` is per-request `time.Now().UTC()` formatted as
// time.RFC3339. There is no caching layer in v1; the timestamp
// records when this response was produced.
func newMeta() oapi.Meta {
	return oapi.Meta{
		License:        dataLicense,
		AttributionUrl: dataAttributionURL,
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
	}
}

// respondCollection writes a JSON `{ meta, data }` envelope for a
// list response. Use this from any collection handler instead of
// writeJSON; single-resource handlers continue to use writeJSON.
//
// T uses generics so the call site keeps its typed slice (e.g.
// []oapi.MetroSummary) without an explicit any conversion. The
// helper encodes whatever T is, so adding a new collection
// endpoint is one call-site change plus a new *Envelope schema in
// openapi.yaml.
//
// A nil input slice is coerced to an empty slice so the encoded
// JSON has `"data": []`, never `"data": null`. Downstream clients
// can rely on `data` always being an array.
func respondCollection[T any](w http.ResponseWriter, items []T) {
	if items == nil {
		items = []T{}
	}
	body := struct {
		Meta oapi.Meta `json:"meta"`
		Data []T       `json:"data"`
	}{
		Meta: newMeta(),
		Data: items,
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(body)
}
```

- [ ] **Step 2.4: Wire the middleware into the router**

Open `api/internal/httpapi/router.go`. Find the `r.Route("/api/"+apiVersion, ...)` block. Add `r.Use(odblHeadersMiddleware)` as the FIRST line inside the block, before the existing route declarations:

```go
	r.Route("/api/"+apiVersion, func(r chi.Router) {
		r.Use(odblHeadersMiddleware)
		r.Get("/openapi.yaml", openapiHandler())
		r.Get("/lookup", lookupHandler(cfg.Store, logger))
		r.Get("/metros", listMetrosHandler(cfg.Store, logger))
		r.Get("/metros/{slug}", getMetroHandler(cfg.Store, logger))
		r.Get("/recent", recentHandler(cfg.Store, logger))
	})
```

- [ ] **Step 2.5: Switch `listMetrosHandler` to `respondCollection`**

Open `api/internal/httpapi/metros.go`. Find `listMetrosHandler`. Replace:

```go
		writeJSON(w, http.StatusOK, toOAPIMetroSummaries(metros))
```

with:

```go
		respondCollection(w, toOAPIMetroSummaries(metros))
```

`getMetroHandler` is unchanged (single resource).

- [ ] **Step 2.6: Switch `recentHandler` to `respondCollection`**

Open `api/internal/httpapi/recent.go`. Find `recentHandler`. Replace:

```go
		writeJSON(w, http.StatusOK, toOAPIOrgs(orgs))
```

with:

```go
		respondCollection(w, toOAPIOrgs(orgs))
```

- [ ] **Step 2.7: Run the new odbl_test.go tests to GREEN**

```bash
cd .worktrees/odbl-backend
cd api && go test ./internal/httpapi/... -run "TestODbL|TestRespondCollection|TestNewMeta" -count=1 -v 2>&1 | tail -30
```

Expected: all six new tests PASS. If any fail, fix before continuing — don't update the existing tests yet.

- [ ] **Step 2.8: Run the FULL existing test suite to see what breaks**

```bash
cd .worktrees/odbl-backend
cd api && go test ./internal/httpapi/... -count=1 2>&1 | tail -30
```

Expected: `TestListMetros_HappyPath_ReturnsOAPIShape`, `TestListRecent_HappyPath_ReturnsOAPIShape`, and `TestListRecent_ExcludesNationalTier` all FAIL because they decode `[]oapi.MetroSummary` / `[]oapi.Org` from a body that is now an envelope. We fix them in the next steps.

Other tests (lookup, metros/{slug}, healthz, openapi) should still PASS — they're unaffected by the envelope.

- [ ] **Step 2.9: Update `metros_test.go` happy-path to decode the envelope**

Open `api/internal/httpapi/metros_test.go`. Replace `TestListMetros_HappyPath_ReturnsOAPIShape` (the entire test function) with:

```go
func TestListMetros_HappyPath_ReturnsOAPIShape(t *testing.T) {
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/api/v1/metros")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: want 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type: want application/json prefix, got %q", ct)
	}
	// ODbL attribution headers ride every /api/v1/** response.
	if got, want := resp.Header.Get("X-Data-License"), "ODbL-1.0"; got != want {
		t.Errorf("X-Data-License: want %q, got %q", want, got)
	}
	if got, want := resp.Header.Get("X-Data-Attribution"), "https://urbanistatlas.com"; got != want {
		t.Errorf("X-Data-Attribution: want %q, got %q", want, got)
	}

	var env oapi.MetroSummariesEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// meta block populated.
	if env.Meta.License != "ODbL-1.0" {
		t.Errorf("meta.license: want %q, got %q", "ODbL-1.0", env.Meta.License)
	}
	if env.Meta.AttributionUrl != "https://urbanistatlas.com" {
		t.Errorf("meta.attribution_url: want %q, got %q",
			"https://urbanistatlas.com", env.Meta.AttributionUrl)
	}
	if _, err := time.Parse(time.RFC3339, env.Meta.GeneratedAt); err != nil {
		t.Errorf("meta.generated_at: not RFC3339 (%q): %v", env.Meta.GeneratedAt, err)
	}

	got := env.Data
	if len(got) == 0 {
		t.Fatal("want at least one metro, got 0")
	}
	// Ordering: org_count DESC, then name ASC.
	for i := 1; i < len(got); i++ {
		if got[i].OrgCount > got[i-1].OrgCount {
			t.Errorf("not descending by org_count at [%d]: %d > %d",
				i, got[i].OrgCount, got[i-1].OrgCount)
		}
		if got[i].OrgCount == got[i-1].OrgCount && got[i].Region.Name < got[i-1].Region.Name {
			t.Errorf("not alphabetical within count tie at [%d]: %q < %q",
				i, got[i].Region.Name, got[i-1].Region.Name)
		}
	}
	// Region must have parent_slugs as a non-null array (even if empty).
	for _, m := range got {
		if m.Region.ParentSlugs == nil {
			t.Errorf("metro %s has nil parent_slugs (must be at least [])", m.Region.Slug)
		}
	}
}
```

You also need to add `"time"` to the imports at the top of `metros_test.go`. The current imports are:

```go
import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/mjrossi/urbanist-atlas/api/internal/httpapi/oapi"
)
```

Change to:

```go
import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mjrossi/urbanist-atlas/api/internal/httpapi/oapi"
)
```

- [ ] **Step 2.10: Update `recent_test.go` happy-path to decode the envelope**

Open `api/internal/httpapi/recent_test.go`. Replace `TestListRecent_HappyPath_ReturnsOAPIShape` with:

```go
func TestListRecent_HappyPath_ReturnsOAPIShape(t *testing.T) {
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/api/v1/recent")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: want 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type: want application/json prefix, got %q", ct)
	}
	if got, want := resp.Header.Get("X-Data-License"), "ODbL-1.0"; got != want {
		t.Errorf("X-Data-License: want %q, got %q", want, got)
	}
	if got, want := resp.Header.Get("X-Data-Attribution"), "https://urbanistatlas.com"; got != want {
		t.Errorf("X-Data-Attribution: want %q, got %q", want, got)
	}

	var env oapi.RecentEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Meta.License != "ODbL-1.0" {
		t.Errorf("meta.license: want %q, got %q", "ODbL-1.0", env.Meta.License)
	}
	if _, err := time.Parse(time.RFC3339, env.Meta.GeneratedAt); err != nil {
		t.Errorf("meta.generated_at: not RFC3339 (%q): %v", env.Meta.GeneratedAt, err)
	}

	got := env.Data
	// LoadDevFixtures seeds plain orgs only (no national-tier). Empty
	// is technically legal; non-empty is what we ship with.
	if len(got) > 10 {
		t.Errorf("len: want <= 10, got %d", len(got))
	}
	for _, o := range got {
		if len(o.Regions) == 0 {
			t.Errorf("org %s missing Regions hydration", o.Slug)
		}
	}
}
```

- [ ] **Step 2.11: Update `TestListRecent_ExcludesNationalTier` to decode the envelope**

Still in `api/internal/httpapi/recent_test.go`. The test currently decodes `var got []oapi.Org` (around line 96). Replace this section:

```go
	var got []oapi.Org
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want exactly 1 org (plain-org); got %d (%v)", len(got), oapiOrgSlugs(got))
	}
```

with:

```go
	var env oapi.RecentEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := env.Data
	if len(got) != 1 {
		t.Fatalf("want exactly 1 org (plain-org); got %d (%v)", len(got), oapiOrgSlugs(got))
	}
```

- [ ] **Step 2.12: Run the full httpapi test suite — must be all green**

```bash
cd .worktrees/odbl-backend
cd api && go test ./internal/httpapi/... -race -count=1
```

Expected: PASS. If anything fails, fix before continuing.

- [ ] **Step 2.13: Run `just api-check` — must stay green**

```bash
cd .worktrees/odbl-backend
just api-check
```

Expected: green. Note that this also runs the gen-check (regenerates types and ensures no diff); that should still pass because the openapi.yaml change was committed in Phase 1.

- [ ] **Step 2.14: Commit**

```bash
cd .worktrees/odbl-backend
git add api/internal/httpapi/odbl.go \
        api/internal/httpapi/odbl_test.go \
        api/internal/httpapi/router.go \
        api/internal/httpapi/metros.go \
        api/internal/httpapi/recent.go \
        api/internal/httpapi/metros_test.go \
        api/internal/httpapi/recent_test.go
git status
git commit -m "$(cat <<'EOF'
feat(api): ODbL headers + collection envelope middleware/helper (slice #24)

The behavior half of slice #24. Now matches the OpenAPI spec
introduced in the previous commit:

- New `odblHeadersMiddleware`: sets `X-Data-License: ODbL-1.0`
  and `X-Data-Attribution: https://urbanistatlas.com` on every
  response that passes through it. Mounted inside the /api/v1
  route group so /healthz stays clean.
- New `respondCollection[T]` helper: wraps an items slice in a
  `{ meta, data }` envelope and writes JSON. Uses generics so
  the call site preserves its typed slice. A nil items value is
  coerced to `[]T{}` so the encoded body always has
  `"data": []`, never `"data": null`.
- New `newMeta()`: per-request RFC3339-UTC timestamp +
  the two attribution constants.
- `listMetrosHandler` and `recentHandler` swap `writeJSON` for
  `respondCollection`. `getMetroHandler` and `lookupHandler` are
  unchanged (single resource / container, no envelope).
- New tests cover header presence on /api/v1, absence on /healthz,
  header presence on /api/v1 error responses, envelope shape,
  nil-safe data, and RFC3339-UTC `generated_at`.
- Existing `/metros` and `/recent` handler tests updated to decode
  the envelope.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Expected: commit succeeds.

---

### Phase 3 — Commit 3: Frontend unwrap

The goal of this commit is to keep the SPA working: `listMetros` / `listRecent` consumers still see arrays, even though the wire is now an envelope.

- [ ] **Step 3.1: Update `web/src/lib/api.test.ts` — RED phase (envelope shape)**

Open `web/src/lib/api.test.ts`. The existing tests use a `jsonResponse` helper that returns a Response with arbitrary body. We need to update the three relevant tests (`listMetros`, `listRecent`, `getMetro` happy paths) so the fetch mock returns the envelope shape for the collection endpoints.

Replace the test block

```ts
  it('listMetros calls GET /api/v1/metros and returns the parsed body', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse([]));
    const result = await listMetros();
    expect(result).toEqual([]);
    const [url] = fetchMock.mock.calls[0]!;
    expect(String(url)).toContain('/api/v1/metros');
  });
```

with:

```ts
  it('listMetros calls GET /api/v1/metros and unwraps the envelope', async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({
        meta: {
          license: 'ODbL-1.0',
          attribution_url: 'https://urbanistatlas.com',
          generated_at: '2026-05-18T12:00:00Z',
        },
        data: [],
      }),
    );
    const result = await listMetros();
    expect(result).toEqual([]);
    const [url] = fetchMock.mock.calls[0]!;
    expect(String(url)).toContain('/api/v1/metros');
  });

  it('listMetros returns the unwrapped data array on a non-empty body', async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({
        meta: {
          license: 'ODbL-1.0',
          attribution_url: 'https://urbanistatlas.com',
          generated_at: '2026-05-18T12:00:00Z',
        },
        data: [
          {
            region: {
              id: 1,
              kind: 'us:metro',
              name: 'New York Metro',
              slug: 'nyc-metro',
              country: 'US',
              scope_tier: 'regional',
              parent_slugs: [],
            },
            org_count: 7,
          },
        ],
      }),
    );
    const result = await listMetros();
    expect(result).toHaveLength(1);
    expect(result[0]!.region.slug).toBe('nyc-metro');
    expect(result[0]!.org_count).toBe(7);
  });
```

Replace the test block

```ts
  it('listRecent calls GET /api/v1/recent', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse([]));
    const result = await listRecent();
    expect(result).toEqual([]);
    const [url] = fetchMock.mock.calls[0]!;
    expect(String(url)).toContain('/api/v1/recent');
  });
```

with:

```ts
  it('listRecent calls GET /api/v1/recent and unwraps the envelope', async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({
        meta: {
          license: 'ODbL-1.0',
          attribution_url: 'https://urbanistatlas.com',
          generated_at: '2026-05-18T12:00:00Z',
        },
        data: [],
      }),
    );
    const result = await listRecent();
    expect(result).toEqual([]);
    const [url] = fetchMock.mock.calls[0]!;
    expect(String(url)).toContain('/api/v1/recent');
  });

  it('listRecent returns the unwrapped data array on a non-empty body', async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({
        meta: {
          license: 'ODbL-1.0',
          attribution_url: 'https://urbanistatlas.com',
          generated_at: '2026-05-18T12:00:00Z',
        },
        data: [
          {
            id: 1,
            slug: 'transalt',
            name: 'Transportation Alternatives',
            short_desc: 'NYC advocacy',
            website_url: 'https://transalt.org',
            tags: ['transit'],
            regions: [
              {
                id: 1,
                kind: 'us:metro',
                name: 'New York Metro',
                slug: 'nyc-metro',
                country: 'US',
                scope_tier: 'regional',
                parent_slugs: [],
              },
            ],
          },
        ],
      }),
    );
    const result = await listRecent();
    expect(result).toHaveLength(1);
    expect(result[0]!.slug).toBe('transalt');
  });
```

`getMetro` tests do NOT change — `/metros/{slug}` is a single resource, not an envelope.

- [ ] **Step 3.2: Run the frontend tests — should fail on listMetros/listRecent**

```bash
cd .worktrees/odbl-backend
cd web && npm test -- --run 2>&1 | tail -40
```

Expected: the four updated/added test cases fail. The current `listMetros` / `listRecent` cast the envelope object as an array; `.length` is undefined; the first test's `toEqual([])` fails because the result is `{ meta, data: [] }` and not `[]`. (Other tests should still pass.)

- [ ] **Step 3.3: Update `web/src/lib/api.ts` — unwrap and re-export envelope types**

Open `web/src/lib/api.ts`. Locate the re-exports near the top:

```ts
export type Country = components['schemas']['Country'];
export type Region = components['schemas']['Region'];
export type Org = components['schemas']['Org'];
export type LookupOrg = components['schemas']['LookupOrg'];
export type LookupQuery = components['schemas']['LookupQuery'];
export type LookupResult = components['schemas']['LookupResult'];
export type MetroSummary = components['schemas']['MetroSummary'];
export type MetroDetail = components['schemas']['MetroDetail'];
export type Submission = components['schemas']['Submission'];
export type SubmissionPayload = components['schemas']['SubmissionPayload'];
export type NewSubmissionRequest = components['schemas']['NewSubmissionRequest'];
export type ProblemDetails = components['schemas']['ProblemDetails'];
```

Add three new exports after the `MetroDetail` line:

```ts
export type Meta = components['schemas']['Meta'];
export type MetroSummariesEnvelope = components['schemas']['MetroSummariesEnvelope'];
export type RecentEnvelope = components['schemas']['RecentEnvelope'];
```

Now find `listMetros`. Current:

```ts
export function listMetros(init?: RequestInit): Promise<MetroSummary[]> {
  return apiFetch<MetroSummary[]>('/api/v1/metros', init);
}
```

Replace with:

```ts
export function listMetros(init?: RequestInit): Promise<MetroSummary[]> {
  return apiFetch<MetroSummariesEnvelope>('/api/v1/metros', init).then(
    (env) => env.data,
  );
}
```

Find `listRecent`. Current:

```ts
export function listRecent(init?: RequestInit): Promise<Org[]> {
  return apiFetch<Org[]>('/api/v1/recent', init);
}
```

Replace with:

```ts
export function listRecent(init?: RequestInit): Promise<Org[]> {
  return apiFetch<RecentEnvelope>('/api/v1/recent', init).then(
    (env) => env.data,
  );
}
```

Also update the JSDoc comment on each so future readers know the shape changed:

For `listMetros`, change the block comment from:

```ts
/**
 * `GET /api/v1/metros` — list every metro region with its
 * approved-org count. Feeds the `/browse` page and the homepage
 * "Browse by metro" aside.
 */
```

to:

```ts
/**
 * `GET /api/v1/metros` — list every metro region with its
 * approved-org count. Feeds the `/browse` page and the homepage
 * "Browse by metro" aside.
 *
 * The wire shape is `{ meta, data: MetroSummary[] }`; this helper
 * unwraps `data` so callers continue to receive the bare array.
 * Read `meta` (license, attribution_url, generated_at) by calling
 * `apiFetch<MetroSummariesEnvelope>` directly if you need it.
 */
```

For `listRecent`, change the block comment from:

```ts
/**
 * `GET /api/v1/recent` — recently approved organizations, newest
 * first. Feeds the homepage "Recently added" aside.
 */
```

to:

```ts
/**
 * `GET /api/v1/recent` — recently approved organizations, newest
 * first. Feeds the homepage "Recently added" aside.
 *
 * The wire shape is `{ meta, data: Org[] }`; this helper unwraps
 * `data` so callers continue to receive the bare array.
 */
```

- [ ] **Step 3.4: Run the frontend tests — must be all green**

```bash
cd .worktrees/odbl-backend
cd web && npm test -- --run 2>&1 | tail -20
```

Expected: all tests pass.

- [ ] **Step 3.5: Run `just web-check` — must be green**

```bash
cd .worktrees/odbl-backend
just web-check
```

Expected: green (covers npm ci, lint, test, build, gen-check).

- [ ] **Step 3.6: Commit**

```bash
cd .worktrees/odbl-backend
git add web/src/lib/api.ts web/src/lib/api.test.ts
git status
git commit -m "$(cat <<'EOF'
feat(web): unwrap data envelope in api.ts helpers (slice #24)

The frontend half of slice #24. The collection endpoints
(`/api/v1/metros` and `/api/v1/recent`) now return a
`{ meta, data }` envelope; the typed helpers `listMetros` and
`listRecent` unwrap `.data` once so consumers (`Home`, `Browse`,
`Metro`) see the same bare array they always have.

- `apiFetch<T>` is unchanged — still generic; some endpoints
  don't have envelopes (`getMetro`, `lookup`).
- `Meta`, `MetroSummariesEnvelope`, `RecentEnvelope` types are
  re-exported from `api.ts` so any advanced consumer can read
  the meta block without reaching into the generated file.
- `api.test.ts` mocks updated to return enveloped JSON; new test
  per helper asserts unwrap on a non-empty body.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Expected: commit succeeds.

---

### Phase 4 — Final verification

- [ ] **Step 4.1: Run the full `just ci`**

```bash
cd .worktrees/odbl-backend
just ci
```

Expected: green.

- [ ] **Step 4.2: Curl smoke against a live API**

Start the dev Postgres and load the dev fixtures (skip if already loaded), then run the server in background and curl the new shape.

```bash
cd .worktrees/odbl-backend
just pg-up
just migrate-up
just loaddata
```

In one terminal:

```bash
cd .worktrees/odbl-backend
just api-run
```

In another:

```bash
curl -sI http://localhost:8080/api/v1/metros | grep -i "X-Data-"
curl -sI http://localhost:8080/healthz | grep -i "X-Data-" || echo "no ODbL on /healthz (correct)"
curl -s http://localhost:8080/api/v1/metros | jq '{meta, first: .data[0]}'
curl -s http://localhost:8080/api/v1/recent | jq '{meta, first: .data[0]}'
curl -s 'http://localhost:8080/api/v1/lookup?postal_code=11217&country=US' | jq 'has("meta")'
```

Expected:
- First curl: both `X-Data-License: ODbL-1.0` and `X-Data-Attribution: https://urbanistatlas.com` lines present.
- Second curl: empty match (no ODbL headers on `/healthz` — correct).
- Third curl: shows `meta` with `license`/`attribution_url`/`generated_at` and a `first` MetroSummary.
- Fourth curl: same shape for recent.
- Fifth curl: `false` (lookup is NOT enveloped — non-goal preserved).

Stop the server (Ctrl-C in the first terminal).

- [ ] **Step 4.3: Manual SPA smoke**

```bash
cd .worktrees/odbl-backend
just api-run &  # restart if you stopped it
cd web && npm run dev
```

Visit `http://localhost:5173/`. Confirm:
- Home page renders, "Browse by metro" and "Recently added" cards populate.
- `/browse` lists metros.
- `/m/nyc-metro` renders the metro with orgs.
- `/r/11217?country=US` renders lookup results.

Stop both processes.

- [ ] **Step 4.4: Inspect git log**

```bash
cd .worktrees/odbl-backend
git log main..HEAD --oneline
```

Expected: three commits, in order:
1. `feat(api): openapi — Meta schema + collection envelope (slice #24)`
2. `feat(api): ODbL headers + collection envelope middleware/helper (slice #24)`
3. `feat(web): unwrap data envelope in api.ts helpers (slice #24)`

- [ ] **Step 4.5: Push and open PR**

```bash
cd .worktrees/odbl-backend
git push -u origin slice-24-odbl-response-shape
gh pr create --title "feat: ODbL attribution headers + collection envelope (slice #24)" \
  --body "$(cat <<'EOF'
## Summary

Implements slice #24 of the launch-readiness cluster: every
`/api/v1/**` success response now carries `X-Data-License: ODbL-1.0`
and `X-Data-Attribution: https://urbanistatlas.com` headers, and the
two collection endpoints (`/api/v1/metros`, `/api/v1/recent`) wrap
their bodies in a `{ meta, data }` envelope that announces the same
ODbL obligation in-band.

Three commits, in spec → behavior → frontend order:

1. **`feat(api): openapi — Meta schema + collection envelope`** —
   `api/openapi.yaml` adds `Meta`, `MetroSummariesEnvelope`,
   `RecentEnvelope` schemas; adds `XDataLicense` / `XDataAttribution`
   to `components/headers`; switches `/api/v1/metros` and
   `/api/v1/recent` 200 schemas to the envelopes; adds header refs
   to every `/api/v1/**` 200 declaration. Regenerated
   `oapi/types.gen.go` and `web/src/lib/api.gen.ts`. **No behavior
   change.**
2. **`feat(api): ODbL headers + collection envelope middleware/helper`** —
   new `odbl.go` with `odblHeadersMiddleware` (mounted inside
   `/api/v1`), `respondCollection[T]` (used by the two list
   handlers), and `newMeta()` (RFC3339 UTC per-request). Tests
   cover header presence on `/api/v1`, absence on `/healthz`,
   presence on `/api/v1` error responses, envelope shape, nil-safe
   data, and the meta block.
3. **`feat(web): unwrap data envelope in api.ts helpers`** —
   `listMetros` and `listRecent` switch their internal fetch type
   to the envelope and unwrap `.data`. Public signatures unchanged
   so `Home`, `Browse`, `Metro` need zero updates.

## Design notes

- **`/lookup` is intentionally NOT enveloped.** It returns a
  `LookupResult` container (single resource with arrays inside),
  not a list. ODbL headers ride it; the body shape is unchanged.
- **`/metros/{slug}` is intentionally NOT enveloped** for the same
  reason (single resource).
- **Headers fire on every response, success or error** under
  `/api/v1`. Distinguishing 2xx in middleware would require status
  sniffing, and an error response that quotes the dataset license
  is harmless.
- **`respondCollection` is generic (`[T any]`)** so call sites keep
  their typed slice. A nil items value is coerced to `[]T{}` so
  the wire is always `"data": []`, never `"data": null`.
- See `docs/superpowers/specs/2026-05-18-odbl-response-shape-design.md`
  for the full design.

## Test plan

- [x] `just api-check` green
- [x] `just web-check` green
- [x] `just ci` green
- [x] `curl -sI :8080/api/v1/metros` shows both X-Data-* headers
- [x] `curl -sI :8080/healthz` shows neither (path-scoping intact)
- [x] `curl :8080/api/v1/metros | jq '.meta, .data[0]'` returns both halves
- [x] `curl :8080/api/v1/recent | jq '.meta, .data[0]'` returns both halves
- [x] `curl ':8080/api/v1/lookup?postal_code=11217&country=US' | jq 'has("meta")'` is `false`
- [x] SPA still renders Home / Browse / Metro / Results

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Expected: PR opens, returns a URL.

---

## Non-goals

Per spec §"Non-goals":

- No envelope on `/lookup` or `/metros/{slug}`.
- No `Cache-Control` / `Vary` / `Link` headers (Phase-2 concern).
- No `info.version` bump.
- No changes to `apiFetch` generic signature.
- No changes to error response shape.
- No seed-data changes.

## Risks

Per spec §"Risks & mitigations". The big two:

1. **Middleware ordering / path scoping.** The middleware is mounted
   inside the `/api/v1` route group, not at router root, so
   `/healthz` stays clean. Two tests pin this (`TestODbLHeaders_PresentOnAPISuccessResponse`,
   `TestODbLHeaders_AbsentOnHealthz`).
2. **Direct `apiFetch<T[]>` callers on the frontend.** Anyone reaching
   past the typed helpers and calling `apiFetch<MetroSummary[]>` will
   get an envelope object cast to an array — `.length` will be
   `undefined`. Grep confirms only `listMetros` and `listRecent` are
   used in the SPA. The envelope types are re-exported for any future
   direct caller.
