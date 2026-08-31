// Package config holds runtime configuration resolved from flags and
// environment variables. Every knob has a safe default so that `nudge`
// launched with no arguments works out of the box.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the resolved server configuration.
type Config struct {
	// Addr is the listen address, e.g. ":8080".
	Addr string
	// DataDir stores the WAL, snapshot and state file.
	DataDir string
	// BaseURL is the public URL used to build links and VAPID audience hints.
	BaseURL string
	// AdminToken, when non-empty, is the bootstrap admin bearer token.
	// When empty a random token is generated on first boot and persisted.
	AdminToken string
	// VAPIDSubject is the VAPID "sub" claim (mailto: or https:).
	VAPIDSubject string
	// MaxEvents caps retained events (oldest are evicted first).
	MaxEvents int
	// MaxAge evicts events older than this duration; 0 disables age eviction.
	MaxAge time.Duration
	// RatePerMinute is the per-IP publish rate limit (0 disables).
	RatePerMinute int
	// MaxBodyBytes caps inbound request body size.
	MaxBodyBytes int64
	// QueueSize is the outbound delivery worker queue length.
	QueueSize int
}

// env reads an environment variable with a fallback default.
func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envInt64(key string, def int64) int64 {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

// Default returns configuration resolved from environment variables,
// applying defaults for anything unset.
func Default() Config {
	return Config{
		Addr:          env("NUDGE_ADDR", ":8080"),
		DataDir:       env("NUDGE_DATA_DIR", "./data"),
		BaseURL:       strings.TrimRight(env("NUDGE_BASE_URL", ""), "/"),
		AdminToken:    os.Getenv("NUDGE_ADMIN_TOKEN"),
		VAPIDSubject:  env("NUDGE_VAPID_SUBJECT", "mailto:admin@nudge.local"),
		MaxEvents:     envInt("NUDGE_MAX_EVENTS", 5000),
		MaxAge:        envDuration("NUDGE_MAX_AGE", 0),
		RatePerMinute: envInt("NUDGE_RATE_PER_MIN", 120),
		MaxBodyBytes:  envInt64("NUDGE_MAX_BODY_BYTES", 64*1024),
		QueueSize:     envInt("NUDGE_QUEUE_SIZE", 256),
	}
}

// WithAddr returns a copy with Addr overridden (used by the CLI flags).
func (c Config) WithAddr(addr string) Config {
	if strings.TrimSpace(addr) != "" {
		c.Addr = addr
	}
	return c
}

// WithDataDir returns a copy with DataDir overridden.
func (c Config) WithDataDir(dir string) Config {
	if strings.TrimSpace(dir) != "" {
		c.DataDir = dir
	}
	return c
}
