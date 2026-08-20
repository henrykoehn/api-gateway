# Architecture

## Request flow

Every request passes through a chain of middleware wrapping the reverse proxy. Two layers (`recover`, `logging`) wrap the whole router; the rest are built per-route in `internal/router/router.go`, since auth/rate-limit/breaker configuration differs per route.

```
Client
  │
  ▼
recover            — catch panics, return 500 instead of crashing
  │
  ▼
logging             — structured slog line: method, path, status, latency
  │
  ▼
┌─ per route ──────────────────────────────────────────────┐
│  metrics           — records final status/latency for this route,
│                       wrapping everything below it        │
│  auth (if configured)                                    │
│    — reject 401 on missing/invalid/expired JWT           │
│    — inject claims into request context on success       │
│  rate limit (if configured)                               │
│    — token bucket keyed by JWT subject (if authed) or IP  │
│    — reject 429 with Retry-After if exhausted             │
│  circuit breaker (if configured)                           │
│    — reject 503 immediately if backend marked open,       │
│      without attempting the call                          │
│  reverse proxy                                             │
│    — forward to backend target                            │
│    — 5xx/connection error → breaker.RecordFailure()        │
│    — other response → breaker.RecordSuccess()              │
└─────────────────────────────────────────────────────────┘
  │
  ▼
Backend
```

Independently of any request, a background goroutine per breaker-configured route polls the backend's health endpoint (`internal/breaker/healthcheck.go`) and can force the breaker open or closed directly — this is what lets the gateway detect an outage before the next real request would have failed against it.

## Concurrency model

Three pieces hold state shared across goroutines, and each is tested with `-race`:

- **Token bucket** (`internal/ratelimiter`) — one `sync.Mutex` per client bucket, plus a package-level mutex guarding the bucket map itself (creation/eviction). Contention is per-client, not global, so a plain mutex is enough — no need for lock-free structures, which would just be unjustifiable complexity here.
- **Circuit breaker** (`internal/breaker`) — a single `sync.RWMutex` per breaker. Reads (`Allow()`, `State()`) are far more frequent than writes (state transitions), which is exactly what `RWMutex` is for.
- **Health checker** — runs in its own goroutine per backend, calling `Breaker.ForceState` through the same locked setter as the passive path. It never touches breaker internals directly, so there's exactly one code path that mutates breaker state, regardless of which signal triggered it.

## Design trade-offs

**Router: stdlib `net/http.ServeMux`, not chi/gorilla/echo.** Go 1.22+ added method- and wildcard-based routing to the standard library, which covers everything this gateway needs. Using it directly — rather than a router library — is a better demonstration of understanding the underlying mechanism, and keeps `go.mod` close to dependency-free. A route-heavy production gateway with regex constraints or shared per-group middleware might outgrow this; this one doesn't.

**Rate limiting: token bucket, not sliding window.** O(1) memory per client (one struct: tokens, last-refill time) versus a sliding-window log's O(n) timestamps per client, and it allows a legitimate burst instead of punishing it — the same trade-off real gateways (Kong, Envoy, AWS API Gateway) make by default.

**Circuit breaker: hand-rolled, not an imported library.** A three-state machine with a handful of transitions is small and well-bounded enough to implement and unit test directly, and it's exactly the kind of code an interviewer might ask "walk me through this" about — importing `sony/gobreaker` here would hide the part of the project meant to demonstrate understanding. JWT validation, by contrast, *is* imported (`golang-jwt/jwt/v5`) — rolling your own signature verification is a security anti-pattern, not a good learning exercise.

**Two independent failover signals.** Passive (a live request fails → count toward the threshold) reacts only after real traffic hits a broken backend. Active (a background poller checks a health endpoint on its own schedule) can catch — and recover from — an outage the moment it happens, independent of traffic volume. Relying on only one would mean either slow detection during low traffic (passive-only) or false confidence when the health endpoint lies about real request-path failures (active-only).

**Config: YAML, loaded once at startup, no hot-reload.** Fail-fast validation before the listener even starts (missing routes, invalid rate limits, etc.) catches config mistakes immediately rather than mid-traffic. Hot-reload (e.g. `fsnotify` + SIGHUP) would add real complexity — safely swapping live rate-limiter/breaker state — for a feature this project doesn't need; restart-on-change is the honest choice here, not an oversight.

**Durations as seconds (float), not duration strings.** `gopkg.in/yaml.v3` doesn't parse `"5s"`-style strings into `time.Duration` without a custom `UnmarshalYAML`. Plain numbers avoid that entirely, at the minor cost of less familiar-looking config.

## What's out of scope, and why

- **Single-instance only.** Rate-limiter and breaker state live in process memory. Running multiple gateway replicas behind a load balancer means each replica enforces its own limits independently — a client could get up to N× the configured rate by hitting different replicas. At scale, this becomes a Redis-backed (or similar shared-store) limiter and breaker state.
- **No TLS termination.** The gateway speaks plain HTTP. For anything exposed past a home network or LAN, put a real edge proxy or tunnel in front (Caddy, Traefik, Tailscale, Cloudflare Tunnel) rather than teaching this project to do TLS as well — that's a separate, well-solved problem.
- **No distributed tracing.** Structured logs and Prometheus metrics cover single-hop observability; a multi-service deployment (e.g. this gateway in front of several ML inference backends) would benefit from OpenTelemetry trace propagation to follow a request across hops, which isn't implemented here.
