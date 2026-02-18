package triage

import (
	"testing"
)

// ---------------------------------------------------------------------------
// API key resolution
// ---------------------------------------------------------------------------

func TestApiKey_ExplicitArg(t *testing.T) {
	cfg, err := resolveConfig(WithAPIKey("tsk_explicit"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.apiKey != "tsk_explicit" {
		t.Errorf("got %q, want %q", cfg.apiKey, "tsk_explicit")
	}
}

func TestApiKey_EnvFallback(t *testing.T) {
	t.Setenv(EnvAPIKey, "tsk_from_env")
	cfg, err := resolveConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.apiKey != "tsk_from_env" {
		t.Errorf("got %q, want %q", cfg.apiKey, "tsk_from_env")
	}
}

func TestApiKey_ArgBeatsEnv(t *testing.T) {
	t.Setenv(EnvAPIKey, "tsk_env")
	cfg, err := resolveConfig(WithAPIKey("tsk_arg"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.apiKey != "tsk_arg" {
		t.Errorf("got %q, want %q", cfg.apiKey, "tsk_arg")
	}
}

func TestApiKey_MissingReturnsError(t *testing.T) {
	_, err := resolveConfig()
	if err == nil {
		t.Fatal("expected error for missing API key, got nil")
	}
}

func TestApiKey_EmptyStringReturnsError(t *testing.T) {
	_, err := resolveConfig(WithAPIKey(""))
	if err == nil {
		t.Fatal("expected error for empty API key, got nil")
	}
}

// ---------------------------------------------------------------------------
// Endpoint resolution
// ---------------------------------------------------------------------------

func TestEndpoint_Default(t *testing.T) {
	cfg, err := resolveConfig(WithAPIKey("k"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.endpoint != DefaultEndpoint {
		t.Errorf("got %q, want %q", cfg.endpoint, DefaultEndpoint)
	}
}

func TestEndpoint_ExplicitArg(t *testing.T) {
	cfg, err := resolveConfig(WithAPIKey("k"), WithEndpoint("https://custom.io"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.endpoint != "https://custom.io" {
		t.Errorf("got %q, want %q", cfg.endpoint, "https://custom.io")
	}
}

func TestEndpoint_EnvFallback(t *testing.T) {
	t.Setenv(EnvEndpoint, "https://env.io")
	cfg, err := resolveConfig(WithAPIKey("k"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.endpoint != "https://env.io" {
		t.Errorf("got %q, want %q", cfg.endpoint, "https://env.io")
	}
}

func TestEndpoint_ArgBeatsEnv(t *testing.T) {
	t.Setenv(EnvEndpoint, "https://env.io")
	cfg, err := resolveConfig(WithAPIKey("k"), WithEndpoint("https://arg.io"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.endpoint != "https://arg.io" {
		t.Errorf("got %q, want %q", cfg.endpoint, "https://arg.io")
	}
}

// ---------------------------------------------------------------------------
// App name resolution
// ---------------------------------------------------------------------------

func TestAppName_ExplicitArg(t *testing.T) {
	cfg, err := resolveConfig(WithAPIKey("k"), WithAppName("my-app"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.appName != "my-app" {
		t.Errorf("got %q, want %q", cfg.appName, "my-app")
	}
}

func TestAppName_EnvFallback(t *testing.T) {
	t.Setenv(EnvAppName, "env-app")
	cfg, err := resolveConfig(WithAPIKey("k"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.appName != "env-app" {
		t.Errorf("got %q, want %q", cfg.appName, "env-app")
	}
}

func TestAppName_DefaultFromArgv(t *testing.T) {
	cfg, err := resolveConfig(WithAPIKey("k"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.appName == "" {
		t.Error("expected non-empty default app name")
	}
}

// ---------------------------------------------------------------------------
// Environment resolution
// ---------------------------------------------------------------------------

func TestEnvironment_Default(t *testing.T) {
	cfg, err := resolveConfig(WithAPIKey("k"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.environment != "development" {
		t.Errorf("got %q, want %q", cfg.environment, "development")
	}
}

func TestEnvironment_ExplicitArg(t *testing.T) {
	cfg, err := resolveConfig(WithAPIKey("k"), WithEnvironment("production"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.environment != "production" {
		t.Errorf("got %q, want %q", cfg.environment, "production")
	}
}

func TestEnvironment_EnvFallback(t *testing.T) {
	t.Setenv(EnvEnvironment, "staging")
	cfg, err := resolveConfig(WithAPIKey("k"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.environment != "staging" {
		t.Errorf("got %q, want %q", cfg.environment, "staging")
	}
}

// ---------------------------------------------------------------------------
// Boolean fields (enabled, traceContent)
// ---------------------------------------------------------------------------

func TestEnabled_EnvValues(t *testing.T) {
	tests := []struct {
		envVal   string
		expected bool
	}{
		{"true", true},
		{"True", true},
		{"1", true},
		{"yes", true},
		{"false", false},
		{"0", false},
		{"no", false},
	}
	for _, tt := range tests {
		t.Run("enabled_env_"+tt.envVal, func(t *testing.T) {
			t.Setenv(EnvEnabled, tt.envVal)
			cfg, err := resolveConfig(WithAPIKey("k"))
			if err != nil {
				t.Fatal(err)
			}
			if cfg.enabled != tt.expected {
				t.Errorf("env=%q: got %v, want %v", tt.envVal, cfg.enabled, tt.expected)
			}
		})
	}
}

func TestEnabled_DefaultIsTrue(t *testing.T) {
	cfg, err := resolveConfig(WithAPIKey("k"))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.enabled {
		t.Error("expected enabled to default to true")
	}
}

func TestEnabled_ExplicitOverridesEnv(t *testing.T) {
	t.Setenv(EnvEnabled, "true")
	cfg, err := resolveConfig(WithAPIKey("k"), WithEnabled(false))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.enabled {
		t.Error("expected explicit false to override env true")
	}
}

func TestTraceContent_EnvValues(t *testing.T) {
	tests := []struct {
		envVal   string
		expected bool
	}{
		{"true", true},
		{"false", false},
	}
	for _, tt := range tests {
		t.Run("trace_content_env_"+tt.envVal, func(t *testing.T) {
			t.Setenv(EnvTraceContent, tt.envVal)
			cfg, err := resolveConfig(WithAPIKey("k"))
			if err != nil {
				t.Fatal(err)
			}
			if cfg.traceContent != tt.expected {
				t.Errorf("env=%q: got %v, want %v", tt.envVal, cfg.traceContent, tt.expected)
			}
		})
	}
}

func TestTraceContent_DefaultIsTrue(t *testing.T) {
	cfg, err := resolveConfig(WithAPIKey("k"))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.traceContent {
		t.Error("expected traceContent to default to true")
	}
}
