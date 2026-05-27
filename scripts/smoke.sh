#!/usr/bin/env bash
# End-to-end smoke against a live API host. Verifies:
#   /healthz reachable, /api/v1/lookup behind the X-Atlas-Client gate
#   (401 without, 200 with), ODbL attribution headers + meta envelope
#   present on the collection endpoint, OpenAPI YAML served.
#
# Defaults to the QA host. Invoked via `just smoke`; can also be
# called directly by CI without needing `just` on the runner:
#
#   URBANIST_CLIENT_SECRET=... ./scripts/smoke.sh
#   ./scripts/smoke.sh <secret> [host]
set -euo pipefail

SECRET="${1:-${URBANIST_CLIENT_SECRET:-}}"
HOST="${2:-qa-api.urbanistatlas.com}"

if [ -z "$SECRET" ]; then
    echo "smoke: URBANIST_CLIENT_SECRET is required (env var or first positional arg)" >&2
    exit 2
fi

BASE="https://$HOST"
fail=0

echo "→ GET $BASE/healthz"
code=$(curl -sS -o /dev/null -w '%{http_code}' "$BASE/healthz")
if [ "$code" != "200" ]; then echo "  FAIL: expected 200, got $code"; fail=1; else echo "  OK 200"; fi

echo "→ GET $BASE/readyz"
code=$(curl -sS -o /dev/null -w '%{http_code}' "$BASE/readyz")
if [ "$code" != "200" ]; then echo "  FAIL: expected 200, got $code"; fail=1; else echo "  OK 200"; fi

echo "→ GET $BASE/api/v1/lookup?postal_code=10001&country=US (no X-Atlas-Client)"
code=$(curl -sS -o /dev/null -w '%{http_code}' "$BASE/api/v1/lookup?postal_code=10001&country=US")
if [ "$code" != "401" ]; then echo "  FAIL: expected 401, got $code"; fail=1; else echo "  OK 401"; fi

echo "→ GET $BASE/api/v1/lookup?postal_code=10001&country=US (with secret)"
headers=$(mktemp)
body=$(mktemp)
code=$(curl -sS -o "$body" -D "$headers" -w '%{http_code}' \
    -H "X-Atlas-Client: $SECRET" \
    "$BASE/api/v1/lookup?postal_code=10001&country=US")
if [ "$code" != "200" ]; then echo "  FAIL: expected 200, got $code"; fail=1; else echo "  OK 200"; fi
if ! grep -qi '^X-Data-License: ODbL-1.0' "$headers"; then echo "  FAIL: missing X-Data-License header"; fail=1; else echo "  OK X-Data-License"; fi
if ! grep -qi '^X-Data-Attribution: ' "$headers"; then echo "  FAIL: missing X-Data-Attribution header"; fail=1; else echo "  OK X-Data-Attribution"; fi
rm -f "$headers" "$body"

# /lookup is a single-resource endpoint; the {meta, data} envelope
# only wraps collection responses (per the slice #24 ODbL design,
# see docs/api-architecture.md § Response envelope). Check meta on
# /regions, which is a collection endpoint.
echo "→ GET $BASE/api/v1/regions (collection meta envelope)"
headers=$(mktemp)
body=$(mktemp)
code=$(curl -sS -o "$body" -D "$headers" -w '%{http_code}' \
    -H "X-Atlas-Client: $SECRET" \
    "$BASE/api/v1/regions")
if [ "$code" != "200" ]; then echo "  FAIL: expected 200, got $code"; fail=1; else echo "  OK 200"; fi
if ! grep -qi '^X-Data-License: ODbL-1.0' "$headers"; then echo "  FAIL: missing X-Data-License header"; fail=1; else echo "  OK X-Data-License"; fi
if ! jq -e '.meta.license and .meta.attribution_url and .meta.generated_at' "$body" >/dev/null; then echo "  FAIL: meta envelope missing license/attribution_url/generated_at"; fail=1; else echo "  OK meta envelope"; fi
if ! jq -e '.data | type == "array"' "$body" >/dev/null; then echo "  FAIL: data is not an array"; fail=1; else echo "  OK data array"; fi
rm -f "$headers" "$body"

echo "→ GET $BASE/api/v1/openapi.yaml"
code=$(curl -sS -o /dev/null -w '%{http_code}' "$BASE/api/v1/openapi.yaml")
if [ "$code" != "200" ]; then echo "  FAIL: expected 200, got $code"; fail=1; else echo "  OK 200"; fi

if [ "$fail" -ne 0 ]; then echo "smoke: FAILED"; exit 1; fi
echo "smoke: PASS"
