package triage

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

var (
	mu           sync.Mutex
	initialized  bool
	provider     *sdktrace.TracerProvider
	activeConfig *config // stored for runtime checks (e.g. traceContent)
)

// Init initializes the Triage SDK. It configures OpenTelemetry with a
// TriageSpanProcessor (injects triage.* context attributes) and a
// BatchSpanProcessor backed by an OTLP/HTTP exporter pointed at the Triage
// backend.
//
// Returns a shutdown function that flushes pending spans and releases
// resources. The caller should defer it:
//
//	shutdown, err := triage.Init(triage.WithAPIKey("tsk_..."))
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer shutdown()
//
// Calling Init more than once logs a warning and returns a no-op shutdown.
func Init(opts ...Option) (func(), error) {
	mu.Lock()
	defer mu.Unlock()

	noop := func() {}

	if initialized {
		slog.Warn("triage: Init() called more than once — ignoring")
		return noop, nil
	}

	cfg, err := resolveConfig(opts...)
	if err != nil {
		return noop, err
	}

	if !cfg.enabled {
		slog.Info("triage: SDK disabled via config — skipping initialization")
		return noop, nil
	}

	ctx := context.Background()

	// Create OTLP/HTTP exporter pointed at the Triage backend.
	exporterOpts := []otlptracehttp.Option{
		otlptracehttp.WithEndpointURL(cfg.endpoint + defaultOTLPTracesPath),
		otlptracehttp.WithHeaders(map[string]string{
			"Authorization": "Bearer " + cfg.apiKey,
		}),
	}

	exporter, err := otlptracehttp.New(ctx, exporterOpts...)
	if err != nil {
		return noop, fmt.Errorf("triage: failed to create OTLP exporter: %w", err)
	}

	// Build the resource with SDK metadata.
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			attribute.String(AttrSDKName, sdkName),
			attribute.String(AttrSDKVersion, Version),
			attribute.String("triage.environment", cfg.environment),
			semconv.ServiceName(cfg.appName),
		),
	)
	if err != nil {
		return noop, fmt.Errorf("triage: failed to create resource: %w", err)
	}

	// Create TracerProvider with:
	// 1. triageSpanProcessor — injects triage.* context attributes on span start
	// 2. BatchSpanProcessor — batches and exports spans via OTLP
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(&triageSpanProcessor{}),
		sdktrace.WithBatcher(exporter),
	)

	// Register as the global TracerProvider so any OTel-instrumented library
	// (HTTP middleware, gRPC interceptors, LLM wrappers) picks it up.
	otel.SetTracerProvider(tp)

	provider = tp
	activeConfig = cfg
	initialized = true

	slog.Info("triage: SDK initialized",
		"app", cfg.appName,
		"env", cfg.environment,
		"endpoint", cfg.endpoint,
	)

	shutdown := func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := Shutdown(shutdownCtx); err != nil {
			slog.Error("triage: shutdown error", "error", err)
		}
	}

	return shutdown, nil
}

// Shutdown flushes pending spans and releases resources. Pass a context with
// a deadline to control how long the flush waits.
//
// Safe to call multiple times — subsequent calls after the first are no-ops.
// This is also available as the function returned by Init() for use with defer.
func Shutdown(ctx context.Context) error {
	mu.Lock()
	defer mu.Unlock()

	if !initialized || provider == nil {
		return nil
	}

	err := provider.Shutdown(ctx)
	initialized = false
	provider = nil
	activeConfig = nil
	return err
}
