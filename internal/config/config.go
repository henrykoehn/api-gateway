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
	Routes []RouteConfig `yaml:"routes"`
}

// ServerConfig controls the gateway's own listener.
type ServerConfig struct {
	Addr string `yaml:"addr"`
}

// RouteConfig maps an incoming path prefix to a backend target.
type RouteConfig struct {
	// Path is the prefix to match, e.g. "/api/orders/".
	Path string `yaml:"path"`
	// Target is the backend base URL, e.g. "http://localhost:9001".
	Target string `yaml:"target"`
	// RateLimit is optional; when nil, the route is not rate limited.
	RateLimit *RateLimitConfig `yaml:"rate_limit,omitempty"`
}

// RateLimitConfig configures a token-bucket limiter for one route.
type RateLimitConfig struct {
	// RequestsPerSecond is the sustained refill rate.
	RequestsPerSecond float64 `yaml:"requests_per_second"`
	// Burst is the bucket capacity, i.e. the max requests allowed at once.
	Burst int `yaml:"burst"`
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
	}

	return nil
}
