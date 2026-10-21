# HTTP API: platform notes

Rationale that the code in `api/internal/httpapi/` and `api/cmd/server/`
links to rather than carries inline.

## Metrics listener bind address

`newMetricsServer` (`api/cmd/server/serve.go`) serves `/metrics` on its
own listener, off the public mux.

- **Locally** (no `FLY_PRIVATE_IP`) it binds `127.0.0.1`, so a dev machine
  does not expose metrics on the LAN.
- **On Fly** it binds all interfaces (`:port`), not `FLY_PRIVATE_IP`.
  Fly's managed-Prometheus scraper reaches the machine on a different
  interface than the 6PN address. Binding only to `FLY_PRIVATE_IP` left
  `/metrics` reachable through `fly proxy` but never scraped, so no
  `atlas_*` series reached Fly's Prometheus.

Binding all interfaces does not make the port public. Fly's edge routes
only the ports declared in `[http_service]` / `[[services]]` in
`fly.toml`, and the metrics port (`[metrics] port = 9091`) is
deliberately not one of them. Keep it that way: adding the metrics port
to a service block would publish it.

This is observed platform behaviour, not something the test suite can
pin. Re-check it if the deploy target changes.
