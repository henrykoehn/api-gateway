# api-gateway

A lightweight API gateway in Go: config-driven reverse proxying, JWT auth, per-route token-bucket rate limiting, and a circuit breaker with both passive (live-traffic) and active (health-check) failover signals. Built stdlib-first — routing and proxying use `net/http.ServeMux` and `net/http/httputil.ReverseProxy` directly rather than a framework, and the rate limiter and circuit breaker are hand-rolled rather than imported, since the point of this project is demonstrating how these pieces work, not gluing libraries together.

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the request flow, concurrency model, and design trade-offs.

## Features

- **Routing** — path-prefix routes to backend targets, config-driven.
- **JWT auth** — HMAC bearer token validation, gated per route, claims available downstream.
- **Rate limiting** — token bucket per client (JWT subject if authenticated, else IP), configurable burst/refill per route.
- **Circuit breaker** — closed/open/half-open state machine per backend, tripped by live-traffic failures *and* an independent active health-check poller, so a dead backend fails fast instead of every request timing out.
- **Observability** — structured JSON logs, Prometheus metrics (`/metrics`), liveness endpoint (`/healthz`).

## Quickstart

```
cd deploy
docker compose up --build
```

This starts the gateway plus two mock backend services and wires up routing, auth, rate limiting, and circuit breaking together — see `deploy/gateway.compose.yaml` for the config driving it.

```
# plain routing to backend-a and backend-b
curl localhost:8080/api/a/hello
curl localhost:8080/api/b/hello

# JWT-protected route (401 without a token)
curl localhost:8080/api/protected/hello
curl localhost:8080/api/protected/hello -H "Authorization: Bearer <token>"

# rate limit on /api/a/ (burst=10) — fire more than 10 quickly to see 429s
for i in $(seq 1 12); do curl -s -o /dev/null -w "%{http_code} " localhost:8080/api/a/hello; done

# circuit breaker — simulate an outage directly on the backend container,
# bypassing the gateway, then watch /api/a/ fast-fail with 503 instead of
# hanging on a dead backend, and recover once you undo it
docker exec <mock-backend-a container> wget -qO- http://localhost:9000/fail
docker exec <mock-backend-a container> wget -qO- http://localhost:9000/recover

curl localhost:8080/metrics
curl localhost:8080/healthz
```

### Running without Docker

```
go run ./cmd/gateway --config configs/gateway.yaml
```

`configs/gateway.yaml` points at `localhost:9001` by default — run anything on that port (e.g. `python3 -m http.server 9001`) to try it locally.

## Config reference

```yaml
server:
  addr: ":8080"          # gateway's own listen address

auth:
  jwt_secret: "..."      # required only if any route sets auth_required: true

routes:
  - path: "/api/orders/"           # path prefix to match (stripped before proxying)
    target: "http://localhost:9001" # backend base URL
    auth_required: true             # optional, default false

    rate_limit:                     # optional; omit for no rate limiting
      requests_per_second: 5        # sustained refill rate
      burst: 10                     # bucket capacity (max burst size)

    circuit_breaker:                        # optional; omit for no breaker
      failure_threshold: 2                  # consecutive failures to trip open
      reset_timeout_seconds: 5              # how long to stay open before a trial request
      success_threshold: 1                  # consecutive half-open successes to close again
      health_check:                         # optional active signal
        path: "/health"                     # appended to target
        interval_seconds: 2
        timeout_seconds: 1
```

Durations are plain seconds (float), not duration strings — keeps the YAML parsing dependency-free rather than needing a custom unmarshaler.

## Testing

```
go test -race -cover ./...
```

Unit tests cover the rate limiter and circuit breaker in isolation (no HTTP, deterministic via an injected clock) and every middleware layer via `httptest`. `-race` is non-negotiable — both the limiter and breaker are shared mutable state accessed concurrently, so a "goroutine-safe" claim without it in CI wouldn't be credible.

## What's deliberately out of scope

Single-instance by design — rate-limiter and breaker state live in process memory, not a shared store like Redis, so limits don't hold across multiple gateway replicas. No config hot-reload (restart on change). No TLS termination (put a reverse proxy or tunnel in front — Caddy, Traefik, Tailscale — for anything exposed past a LAN). See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the reasoning and what change at scale.

## License

[MIT](LICENSE)
