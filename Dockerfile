# syntax=docker/dockerfile:1.7
#
# Multi-stage build for the urbanist-atlas API.
#
# Stage 1 compiles the Go binary against the same Go version mise pins
# for local dev (see mise.toml's [tools] section).
#
# Stage 2 is a barebones Alpine runtime with ca-certificates and a
# non-root user; it ships only the binary. The seed bundle
# (regions_*.toml, postal_codes_*.csv, orgs.toml) is embedded into
# the binary via //go:embed at api/seed/embed.go, so no separate
# COPY is needed and the image stays minimal.
#
# Design doc: docs/superpowers/specs/2026-05-21-fly-deploy-design.md

# ── build ─────────────────────────────────────────────
# Patch-pinned to match mise.toml. If the patch tag is unavailable on
# Docker Hub at build time, fall back to `golang:1.26-alpine` and note
# the skew in the build log.
FROM golang:1.26.3-alpine AS builder

WORKDIR /src

# Pull deps first so subsequent code edits don't bust this layer.
COPY api/go.mod api/go.sum ./api/
RUN cd api && go mod download

# Then the rest of the api source.
COPY api/ ./api/

# CGO off keeps the binary fully static so the runtime stage can be
# minimal; -s -w strips symbols + DWARF for a smaller image. These
# flags MUST stay in sync with `just api-build-prod` in the root
# justfile — same build flags, different output path. Drift check is
# a code-review concern; we don't install `just` inside the build
# stage to delegate, since the dependency cost outweighs the parity
# gain for a single command.
WORKDIR /src/api
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /out/urbanist-atlas-server \
    ./cmd/server

# ── runtime ───────────────────────────────────────────
FROM alpine:3.20

# ca-certificates for any outbound TLS the binary might do.
# Non-root user because there's no reason to run as root for a
# stateless API listening on a non-privileged port.
RUN apk add --no-cache ca-certificates sqlite && \
    addgroup -S app && \
    adduser -S app -G app

WORKDIR /app

# Pre-create /data so the non-root app user can write the SQLite DB
# even on the first boot before the Fly volume's lifecycle attaches.
# Fly mounts inherit the in-image ownership on first mount, so this
# also fixes the long-term permissions.
RUN mkdir -p /data && chown app:app /data
VOLUME ["/data"]

COPY --from=builder --chown=app:app /out/urbanist-atlas-server /usr/local/bin/urbanist-atlas-server

USER app
EXPOSE 8080

ENTRYPOINT ["urbanist-atlas-server"]
CMD ["serve"]
