#!/usr/bin/env bash
#
# Walk api/seed/orgs.toml and probe every website_url for liveness,
# off-domain redirects, and parked-page signatures. Writes a Markdown
# report grouped by severity plus a TSV with raw probe data.
#
# Network-bound — hits ~200 third-party advocacy-org URLs. A handful
# resolve to parked/squatted domains that log requester IPs, so this is
# a script you want to think about *where* you run from. Recommended:
# a hardened Docker container behind a VPN.
#
# Usage:
#
#   # Bare metal (your laptop, your IP, your problem):
#   scripts/verify-org-urls.sh                 # writes tmp/org-url-report.{md,tsv}
#   REPORT=foo.md scripts/verify-org-urls.sh
#   CONCURRENCY=4 TIMEOUT=30 scripts/verify-org-urls.sh
#
#   # Hardened Docker + Proton WireGuard via gluetun (recommended):
#   #   1. Put WIREGUARD_PRIVATE_KEY in mise.local.toml [env]
#   #      (see mise.local.toml.example).
#   #   2. just verify-org-urls           # writes tmp/org-url-report.{md,tsv}
#   #      just verify-org-urls-down      # stops gluetun when finished
#   # The compose stack at scripts/verify-org-urls.compose.yml ties
#   # this script's container to gluetun's network namespace, so all
#   # egress goes through the VPN tunnel.
#
#   # If the VPN runs on the host (Tailscale, host-level WireGuard),
#   # the bare-metal invocation above is fine — host routing handles
#   # egress.
#
#   # Hardened Docker without compose, attached to your own VPN container:
#   docker build -t atlas-url-verify -f scripts/verify-org-urls.Dockerfile .
#   docker run --rm \
#     --read-only --tmpfs /tmp \
#     --cap-drop=ALL --security-opt no-new-privileges \
#     --user "$(id -u):$(id -g)" \
#     --network=container:<your-vpn-container> \
#     -v "$PWD/api/seed:/work/api/seed:ro" \
#     -v "$PWD/tmp:/work/tmp" \
#     atlas-url-verify
#
# Why curl, not httpie: `curl -w` gives us status + final URL + body
# size in one shot; matching that with httpie would need --print=h plus
# jq glue. If you'd rather drive httpie interactively for one-off
# re-checks of a flagged row, the rows in the TSV are copy-pasteable.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ORGS_TOML="${ORGS_TOML:-${REPO_ROOT}/api/seed/orgs.toml}"
REPORT="${REPORT:-${REPO_ROOT}/tmp/org-url-report.md}"
DETAILS="${REPORT%.md}.tsv"
CONCURRENCY="${CONCURRENCY:-8}"
TIMEOUT="${TIMEOUT:-20}"
UA="${UA:-urbanist-atlas-url-verifier/0.1 (+https://urbanistatlas.com)}"

mkdir -p "$(dirname "$REPORT")"
: > "$DETAILS"

# ── parse: emit one TSV row per [[org]] block ─────────────────────────
#   slug \t name \t website_url
parse_orgs() {
  awk '
    /^\[\[org\]\]/ {
      if (slug != "" && url != "") print slug "\t" name "\t" url
      slug=""; name=""; url=""
    }
    /^slug = "/        { match($0, /"[^"]*"/); slug = substr($0, RSTART+1, RLENGTH-2) }
    /^name = "/        { match($0, /"[^"]*"/); name = substr($0, RSTART+1, RLENGTH-2) }
    /^website_url = "/ { match($0, /"[^"]*"/); url  = substr($0, RSTART+1, RLENGTH-2) }
    END {
      if (slug != "" && url != "") print slug "\t" name "\t" url
    }
  ' "$1"
}

# ── probe: one URL → one TSV row in $DETAILS (atomic append) ─────────
probe() {
  local slug="$1" name="$2" url="$3"
  local body status final bytes title metrics rc
  body="$(mktemp)"
  trap 'rm -f "$body"' RETURN

  # Hardening:
  #   --max-filesize  cap body size so a 10 GB response can't fill the disk
  #   --max-redirs    cap redirect chains; some parkers bounce through trackers
  #   --proto-redir   refuse non-http(s) redirect targets (no file://, dict://, …)
  metrics="$(curl -sSL --max-time "$TIMEOUT" \
    --max-filesize 5242880 \
    --max-redirs 5 \
    --proto-redir =https,http \
    -A "$UA" \
    -o "$body" \
    -w '%{http_code}\t%{url_effective}\t%{size_download}' \
    "$url" 2>/dev/null)" && rc=0 || rc=$?

  if (( rc != 0 )); then
    status="000"; final="-"; bytes="0"
    title="(curl exit $rc)"
  else
    status="$(printf '%s' "$metrics" | cut -f1)"
    final="$(printf '%s' "$metrics" | cut -f2)"
    bytes="$(printf '%s' "$metrics" | cut -f3)"
    # First <title> tag, tags stripped, collapsed whitespace, capped at 200 chars.
    title="$(tr '\n\r\t' '   ' < "$body" \
      | grep -oiE '<title[^>]*>[^<]{0,500}</title>' \
      | head -n1 \
      | sed -E 's/<[^>]+>//g; s/^[[:space:]]+//; s/[[:space:]]+$//; s/[[:space:]]+/ /g' \
      | cut -c1-200)"
    [[ -z "$title" ]] && title="(no <title>)"
  fi

  # \t-safe: name is the only field that could realistically contain a
  # tab and the seed has none — verified by inspection.
  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
    "$slug" "$name" "$url" "$status" "$final" "$bytes" "$title" >> "$DETAILS"
}

# ── classify: stdin TSV row → one-word severity ──────────────────────
classify() {
  local slug name url status final bytes title
  IFS=$'\t' read -r slug name url status final bytes title <<<"$1"

  if [[ "$status" == "000" ]]; then echo "dead"; return; fi
  if (( status == 403 || status == 429 )); then echo "blocked"; return; fi
  if (( status >= 400 )); then echo "dead"; return; fi

  # Off-domain redirect: compare last-two-label registrable domain.
  # Approximation — wrong for .co.uk etc., but the seed is .org/.com-heavy.
  local oh fh oreg freg
  oh="$(printf '%s' "$url"   | sed -E 's|^[a-z]+://([^/]+).*|\1|' | sed -E 's/^www\.//')"
  fh="$(printf '%s' "$final" | sed -E 's|^[a-z]+://([^/]+).*|\1|' | sed -E 's/^www\.//')"
  oreg="$(printf '%s' "$oh" | awk -F. 'NF>=2{print $(NF-1)"."$NF}')"
  freg="$(printf '%s' "$fh" | awk -F. 'NF>=2{print $(NF-1)"."$NF}')"
  if [[ -n "$oreg" && -n "$freg" && "$oreg" != "$freg" ]]; then
    echo "redirected"; return
  fi

  # Parked-page signatures in <title>. Body scan adds noise; keep it tight.
  if printf '%s' "$title" | grep -qiE \
    '(domain.*for sale|buy this domain|this domain (is for sale|may be for sale)|parked|godaddy|sedo|bodis|hugedomains|namecheap|dan\.com|expired|domain is available|host_not_allowed)'; then
    echo "parked"; return
  fi

  # Tiny body — placeholder, holding page, or content-less redirect target.
  if (( bytes < 1024 )); then echo "tiny"; return; fi

  echo "ok"
}

# ── run ──────────────────────────────────────────────────────────────
echo "→ parsing $ORGS_TOML" >&2
mapfile -t ROWS < <(parse_orgs "$ORGS_TOML")
TOTAL="${#ROWS[@]}"
echo "→ probing $TOTAL URLs at concurrency=$CONCURRENCY timeout=${TIMEOUT}s" >&2

inflight=0
done_count=0
for row in "${ROWS[@]}"; do
  IFS=$'\t' read -r slug name url <<<"$row"
  probe "$slug" "$name" "$url" &
  (( inflight++ )) || true
  if (( inflight >= CONCURRENCY )); then
    wait -n
    (( inflight-- )) || true
    (( done_count++ )) || true
    if (( done_count % 25 == 0 )); then
      echo "  …$done_count/$TOTAL" >&2
    fi
  fi
done
wait
echo "  done. raw rows → $DETAILS" >&2

# ── report ───────────────────────────────────────────────────────────
echo "→ classifying + writing $REPORT" >&2

declare -A BUCKETS=( [dead]="" [parked]="" [redirected]="" [blocked]="" [tiny]="" [ok]="" )
declare -A COUNTS=(  [dead]=0  [parked]=0  [redirected]=0  [blocked]=0  [tiny]=0  [ok]=0  )

while IFS= read -r line; do
  sev="$(classify "$line")"
  BUCKETS[$sev]+="${line}"$'\n'
  COUNTS[$sev]=$(( COUNTS[$sev] + 1 ))
done < <(sort -t$'\t' -k1,1 "$DETAILS")

fmt_section() {
  local sev="$1" heading="$2"
  local rows="${BUCKETS[$sev]}"
  local n="${COUNTS[$sev]}"
  printf '\n## %s (%d)\n\n' "$heading" "$n"
  if (( n == 0 )); then
    printf '_None._\n'
    return
  fi
  printf '%s' "$rows" | while IFS=$'\t' read -r slug name url status final bytes title; do
    [[ -z "$slug" ]] && continue
    printf -- '- **%s** [`%s`] — `%s`\n  - status `%s`, %s bytes\n' \
      "$name" "$slug" "$url" "$status" "$bytes"
    if [[ "$final" != "$url" && "$final" != "-" ]]; then
      printf -- '  - final: `%s`\n' "$final"
    fi
    if [[ -n "$title" && "$title" != "(no <title>)" ]]; then
      printf -- '  - title: %s\n' "$title"
    fi
  done
}

{
  printf '# Org URL verification report\n\n'
  printf '_Generated %s from `%s` (%d orgs)._\n\n' \
    "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "${ORGS_TOML#$REPO_ROOT/}" "$TOTAL"
  printf '## Summary\n\n'
  printf -- '- **dead**: %d (connection failed or 4xx/5xx)\n' "${COUNTS[dead]}"
  printf -- '- **parked**: %d (title looks like a parking page)\n' "${COUNTS[parked]}"
  printf -- '- **redirected**: %d (final URL on a different registrable domain)\n' "${COUNTS[redirected]}"
  printf -- '- **blocked**: %d (403/429 — could be UA filtering, manual check)\n' "${COUNTS[blocked]}"
  printf -- '- **tiny**: %d (response body < 1 KB)\n' "${COUNTS[tiny]}"
  printf -- '- **ok**: %d\n' "${COUNTS[ok]}"

  fmt_section dead       "Dead"
  fmt_section parked     "Parked / suspicious"
  fmt_section redirected "Redirected off-domain"
  fmt_section blocked    "Blocked — manual review"
  fmt_section tiny       "Tiny body"
  fmt_section ok         "OK"
} > "$REPORT"

echo "✓ report → $REPORT" >&2
