package triage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// config holds resolved SDK configuration. Fields are unexported to enforce
// immutability after creation.
type config struct {
	apiKey       string
	endpoint     string
	appName      string
	environment  string
	enabled      bool
	traceContent bool
}

// Option configures the Triage SDK. Pass options to Init().
type Option func(*config)

// WithAPIKey sets the Triage API key for authentication.
func WithAPIKey(key string) Option {
	return func(c *config) { c.apiKey = key }
}

// WithEndpoint sets the Triage backend endpoint.
func WithEndpoint(ep string) Option {
	return func(c *config) { c.endpoint = ep }
}

// WithAppName sets the application name reported in traces.
func WithAppName(name string) Option {
	return func(c *config) { c.appName = name }
}

// WithEnvironment sets the deployment environment (e.g. "production", "staging").
func WithEnvironment(env string) Option {
	return func(c *config) { c.environment = env }
}

// WithEnabled enables or disables the SDK. When disabled, Init() is a no-op.
func WithEnabled(b bool) Option {
	return func(c *config) { c.enabled = b }
}

// WithTraceContent controls whether prompt/completion content is captured.
func WithTraceContent(b bool) Option {
	return func(c *config) { c.traceContent = b }
}

// resolveConfig merges explicit options > env vars > defaults and returns a
// validated config. Returns an error if the API key is missing.
func resolveConfig(opts ...Option) (*config, error) {
	cfg := &config{
		endpoint:     DefaultEndpoint,
		appName:      defaultAppName(),
		environment:  "development",
		enabled:      true,
		traceContent: true,
	}

	// Layer 2: env var overrides.
	if v := os.Getenv(EnvAPIKey); v != "" {
		cfg.apiKey = v
	}
	if v := os.Getenv(EnvEndpoint); v != "" {
		cfg.endpoint = v
	}
	if v := os.Getenv(EnvAppName); v != "" {
		cfg.appName = v
	}
	if v := os.Getenv(EnvEnvironment); v != "" {
		cfg.environment = v
	}
	if v, ok := envBool(EnvEnabled); ok {
		cfg.enabled = v
	}
	if v, ok := envBool(EnvTraceContent); ok {
		cfg.traceContent = v
	}

	// Layer 3: explicit options (highest priority).
	for _, opt := range opts {
		opt(cfg)
	}

	if cfg.apiKey == "" {
		return nil, fmt.Errorf(
			"triage: API key is required. Pass triage.WithAPIKey() to Init() "+
				"or set the %s environment variable", EnvAPIKey,
		)
	}

	return cfg, nil
}

// envBool reads a boolean from an environment variable.
// Returns (value, true) if the variable is set, or (false, false) if unset.
// Accepts true/false/1/0/yes/no (case-insensitive).
func envBool(key string) (bool, bool) {
	v := os.Getenv(key)
	if v == "" {
		return false, false
	}
	switch strings.ToLower(v) {
	case "true", "1", "yes":
		return true, true
	default:
		return false, true
	}
}

// defaultAppName returns the basename of os.Args[0], or "unknown" if unavailable.
func defaultAppName() string {
	if len(os.Args) > 0 && os.Args[0] != "" {
		return filepath.Base(os.Args[0])
	}
	return "unknown"
}
