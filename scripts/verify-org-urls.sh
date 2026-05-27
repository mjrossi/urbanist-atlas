#!/usr/bin/env bash
#
# Walk api/seed/orgs.toml and probe every website_url + contact_url
# for liveness, off-domain redirects, and parked-page signatures.
# Writes a Markdown report grouped by severity plus a TSV with raw
# probe data. Contact-form rows are labelled `<slug> (contact)` in
# the report so they don't get conflated with the main website check.
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
#
# Future: there's overlap with `urbanist-atlas-server linkcheck` (see
# api/internal/linkcheck/). The Go version uses the typed `seed`
# package so schema typos can't silently drop orgs; this script adds
# VPN egress, redirect-domain comparison, parked/SEO-spam title
# detection, body-size threshold, and Markdown buckets on top. A
# future slice should fold the classifier + Markdown report into the
# Go subcommand (e.g. `linkcheck --out-format=md`) and shrink this
# Dockerfile to a Go-binary wrapper, collapsing `just linkcheck` and
# `just verify-org-urls` into one path.

set -euo pipefail

# `wait -n` needs bash 4.3+. macOS ships bash 3.2 at /bin/bash; if
# /usr/bin/env picks it up, the parallel probe loop below dies with
# `wait: -n: invalid option`. Inside the Alpine container we're on
# bash 5.x, so this only bites bare-metal invocations on macOS without
# homebrew bash on PATH.
if ! { [[ "${BASH_VERSINFO[0]}" -gt 4 ]] \
    || { [[ "${BASH_VERSINFO[0]}" -eq 4 ]] && [[ "${BASH_VERSINFO[1]}" -ge 3 ]]; }; }; then
  echo "verify-org-urls: needs bash >= 4.3 for \`wait -n\` (current: ${BASH_VERSION})." >&2
  echo "  macOS default is 3.2 — \`brew install bash\` and make sure it's first on PATH," >&2
  echo "  or use the container path: \`just verify-org-urls\`." >&2
  exit 1
fi

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ORGS_TOML="${ORGS_TOML:-${REPO_ROOT}/api/seed/orgs.toml}"
REPORT="${REPORT:-${REPO_ROOT}/tmp/org-url-report.md}"
DETAILS="${REPORT%.md}.tsv"
CONCURRENCY="${CONCURRENCY:-8}"
TIMEOUT="${TIMEOUT:-20}"
UA="${UA:-urbanist-atlas-url-verifier/0.1 (+https://urbanistatlas.com)}"

mkdir -p "$(dirname "$REPORT")"
: > "$DETAILS"

# ── parse: emit TSV rows for every probe-worthy URL per [[org]] ───────
#   slug \t name \t kind \t url
# kind ∈ {website, contact}. Both fields are probed because a stale
# contact page (broken form, lapsed donate target) is just as
# actionable as a dead website_url. Orgs with only one URL emit one
# row; orgs with both emit two.
parse_orgs() {
  awk '
    function emit() {
      if (slug == "") return
      if (website != "") print slug "\t" name "\twebsite\t" website
      if (contact != "") print slug "\t" name "\tcontact\t" contact
    }
    /^\[\[org\]\]/ {
      emit()
      slug=""; name=""; website=""; contact=""
    }
    /^slug = "/         { match($0, /"[^"]*"/); slug    = substr($0, RSTART+1, RLENGTH-2) }
    /^name = "/         { match($0, /"[^"]*"/); name    = substr($0, RSTART+1, RLENGTH-2) }
    /^website_url = "/  { match($0, /"[^"]*"/); website = substr($0, RSTART+1, RLENGTH-2) }
    /^contact_url = "/  { match($0, /"[^"]*"/); contact = substr($0, RSTART+1, RLENGTH-2) }
    END { emit() }
  ' "$1"
}

# ── probe: one URL → one TSV row in $DETAILS (atomic append) ─────────
probe() {
  local slug="$1" name="$2" kind="$3" url="$4"
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
    # `|| true` swallows SIGPIPE (141) from `head -n1` closing the pipe early
    # under `set -o pipefail`; we don't care if extraction came up empty.
    title="$(tr '\n\r\t' '   ' < "$body" \
      | grep -oiE '<title[^>]*>[^<]{0,500}</title>' \
      | head -n1 \
      | sed -E 's/<[^>]+>//g; s/^[[:space:]]+//; s/[[:space:]]+$//; s/[[:space:]]+/ /g' \
      | cut -c1-200 || true)"
    [[ -z "$title" ]] && title="(no <title>)"
  fi

  # \t-safe: name is the only field that could realistically contain a
  # tab and the seed has none — verified by inspection.
  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
    "$slug" "$name" "$kind" "$url" "$status" "$final" "$bytes" "$title" >> "$DETAILS"
}

# ── classify: stdin TSV row → one-word severity ──────────────────────
classify() {
  local slug name kind url status final bytes title
  IFS=$'\t' read -r slug name kind url status final bytes title <<<"$1"

  # status=000 = transport-layer failure (curl couldn't reach the origin):
  # DNS, timeout, TLS handshake, connection refused, etc. Over a VPN these
  # frequently fire on real, live sites — a single 000 is not a death
  # signal. Bucketed separately from real HTTP 4xx/5xx so manual triage
  # doesn't conflate the two.
  if [[ "$status" == "000" ]]; then echo "unreachable"; return; fi
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

  # SEO-spam takeovers — lapsed domains repurposed for gambling/crypto/pharma
  # spam. Distinct from parking pages: the site is "alive" (200 OK, big body)
  # but the content is unrelated to the original org. Caught pawalksandbikes
  # 2026-05-27 (Turkish slots SEO farm). Title-only — body scan is too noisy.
  if printf '%s' "$title" | grep -qiE \
    '(\bslot\b|\bslots\b|\bcasino\b|bonanza|\boyunu\b|\bbahis\b|\bpoker\b|\bbet[0-9]|sportsbook|\bcrypto\b|\bforex\b|viagra|cialis|pharmacy)'; then
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
  IFS=$'\t' read -r slug name kind url <<<"$row"
  probe "$slug" "$name" "$kind" "$url" &
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

declare -A BUCKETS=( [unreachable]="" [dead]="" [parked]="" [redirected]="" [blocked]="" [tiny]="" [ok]="" )
declare -A COUNTS=(  [unreachable]=0  [dead]=0  [parked]=0  [redirected]=0  [blocked]=0  [tiny]=0  [ok]=0  )

while IFS= read -r line; do
  sev="$(classify "$line")"
  BUCKETS[$sev]+="${line}"$'\n'
  COUNTS[$sev]=$(( COUNTS[$sev] + 1 ))
done < <(sort -t$'\t' -k1,1 -k3,3 "$DETAILS")

fmt_section() {
  local sev="$1" heading="$2"
  local rows="${BUCKETS[$sev]}"
  local n="${COUNTS[$sev]}"
  printf '\n## %s (%d)\n\n' "$heading" "$n"
  if (( n == 0 )); then
    printf '_None._\n'
    return
  fi
  printf '%s' "$rows" | while IFS=$'\t' read -r slug name kind url status final bytes title; do
    [[ -z "$slug" ]] && continue
    # Distinguish website vs contact in the slug tag so the report
    # reads unambiguously when both fail for the same org.
    local label="$slug"
    [[ "$kind" == "contact" ]] && label="$slug (contact)"
    printf -- '- **%s** [`%s`] — `%s`\n  - status `%s`, %s bytes\n' \
      "$name" "$label" "$url" "$status" "$bytes"
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
  printf '_Generated %s from `%s` (%d URLs across website_url + contact_url)._\n\n' \
    "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "${ORGS_TOML#$REPO_ROOT/}" "$TOTAL"
  printf '## Summary\n\n'
  printf -- '- **unreachable**: %d (curl couldn'\''t connect — DNS, timeout, TLS; often VPN-side noise, retry before acting)\n' "${COUNTS[unreachable]}"
  printf -- '- **dead**: %d (origin returned 4xx/5xx)\n' "${COUNTS[dead]}"
  printf -- '- **parked**: %d (title looks like a parking page or SEO-spam takeover)\n' "${COUNTS[parked]}"
  printf -- '- **redirected**: %d (final URL on a different registrable domain)\n' "${COUNTS[redirected]}"
  printf -- '- **blocked**: %d (403/429 — could be UA filtering, manual check)\n' "${COUNTS[blocked]}"
  printf -- '- **tiny**: %d (response body < 1 KB)\n' "${COUNTS[tiny]}"
  printf -- '- **ok**: %d\n' "${COUNTS[ok]}"

  fmt_section unreachable "Unreachable — retry before acting"
  fmt_section dead        "Dead"
  fmt_section parked      "Parked / suspicious"
  fmt_section redirected  "Redirected off-domain"
  fmt_section blocked     "Blocked — manual review"
  fmt_section tiny        "Tiny body"
  fmt_section ok          "OK"
} > "$REPORT"

echo "✓ report → $REPORT" >&2
