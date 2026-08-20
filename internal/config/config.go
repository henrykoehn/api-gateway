// Package config loads and validates the gateway's YAML configuration.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the root of the gateway's configuration file.
type Config struct {
	Server ServerConfig  `yaml:"server"`
	Auth   AuthConfig    `yaml:"auth"`
	Routes []RouteConfig `yaml:"routes"`
}

// ServerConfig controls the gateway's own listener.
type ServerConfig struct {
	Addr string `yaml:"addr"`
}

// AuthConfig holds the shared JWT verification secret. Required only
// if at least one route sets auth_required: true.
type AuthConfig struct {
	JWTSecret string `yaml:"jwt_secret"`
}

// RouteConfig maps an incoming path prefix to a backend target.
type RouteConfig struct {
	// Path is the prefix to match, e.g. "/api/orders/".
	Path string `yaml:"path"`
	// Target is the backend base URL, e.g. "http://localhost:9001".
	Target string `yaml:"target"`
	// RateLimit is optional; when nil, the route is not rate limited.
	RateLimit *RateLimitConfig `yaml:"rate_limit,omitempty"`
	// CircuitBreaker is optional; when nil, the route has no breaker
	// protection (a slow/broken backend is called on every request).
	CircuitBreaker *BreakerConfig `yaml:"circuit_breaker,omitempty"`
	// AuthRequired gates the route behind JWT bearer auth when true.
	AuthRequired bool `yaml:"auth_required"`
}

// RateLimitConfig configures a token-bucket limiter for one route.
type RateLimitConfig struct {
	// RequestsPerSecond is the sustained refill rate.
	RequestsPerSecond float64 `yaml:"requests_per_second"`
	// Burst is the bucket capacity, i.e. the max requests allowed at once.
	Burst int `yaml:"burst"`
}

// BreakerConfig configures a circuit breaker for one route's backend.
// Durations are expressed in seconds (float64) rather than duration
// strings to keep YAML parsing dependency-free.
type BreakerConfig struct {
	// FailureThreshold is consecutive failures before tripping open.
	FailureThreshold int `yaml:"failure_threshold"`
	// ResetTimeoutSeconds is how long to stay open before a half-open trial.
	ResetTimeoutSeconds float64 `yaml:"reset_timeout_seconds"`
	// SuccessThreshold is consecutive half-open successes required to close.
	SuccessThreshold int `yaml:"success_threshold"`
	// HealthCheck is optional; when nil, only the passive (live-traffic)
	// signal drives the breaker.
	HealthCheck *HealthCheckConfig `yaml:"health_check,omitempty"`
}

// HealthCheckConfig configures active polling of a backend.
type HealthCheckConfig struct {
	// Path is appended to the route's target to form the health URL.
	Path string `yaml:"path"`
	// IntervalSeconds is the time between checks.
	IntervalSeconds float64 `yaml:"interval_seconds"`
	// TimeoutSeconds bounds each individual check.
	TimeoutSeconds float64 `yaml:"timeout_seconds"`
}

// Load reads and validates the config file at path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config %s: %w", path, err)
	}

	return &cfg, nil
}

func (c *Config) validate() error {
	if c.Server.Addr == "" {
		c.Server.Addr = ":8080"
	}

	if len(c.Routes) == 0 {
		return fmt.Errorf("no routes defined")
	}

	seen := make(map[string]bool, len(c.Routes))
	for i, r := range c.Routes {
		if r.Path == "" {
			return fmt.Errorf("routes[%d]: path is required", i)
		}
		if r.Target == "" {
			return fmt.Errorf("routes[%d] (%s): target is required", i, r.Path)
		}
		if seen[r.Path] {
			return fmt.Errorf("routes[%d]: duplicate path %q", i, r.Path)
		}
		seen[r.Path] = true

		if r.RateLimit != nil {
			if r.RateLimit.RequestsPerSecond <= 0 {
				return fmt.Errorf("routes[%d] (%s): rate_limit.requests_per_second must be > 0", i, r.Path)
			}
			if r.RateLimit.Burst <= 0 {
				return fmt.Errorf("routes[%d] (%s): rate_limit.burst must be > 0", i, r.Path)
			}
		}

		if r.AuthRequired && c.Auth.JWTSecret == "" {
			return fmt.Errorf("routes[%d] (%s): auth_required is true but auth.jwt_secret is not set", i, r.Path)
		}

		if cb := r.CircuitBreaker; cb != nil {
			if cb.FailureThreshold <= 0 {
				return fmt.Errorf("routes[%d] (%s): circuit_breaker.failure_threshold must be > 0", i, r.Path)
			}
			if cb.ResetTimeoutSeconds <= 0 {
				return fmt.Errorf("routes[%d] (%s): circuit_breaker.reset_timeout_seconds must be > 0", i, r.Path)
			}
			if cb.SuccessThreshold <= 0 {
				return fmt.Errorf("routes[%d] (%s): circuit_breaker.success_threshold must be > 0", i, r.Path)
			}
			if hc := cb.HealthCheck; hc != nil {
				if hc.Path == "" {
					return fmt.Errorf("routes[%d] (%s): circuit_breaker.health_check.path is required", i, r.Path)
				}
				if hc.IntervalSeconds <= 0 {
					return fmt.Errorf("routes[%d] (%s): circuit_breaker.health_check.interval_seconds must be > 0", i, r.Path)
				}
				if hc.TimeoutSeconds <= 0 {
					return fmt.Errorf("routes[%d] (%s): circuit_breaker.health_check.timeout_seconds must be > 0", i, r.Path)
				}
			}
		}
	}

	return nil
}
