package triage

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// newTestProvider creates a TracerProvider wired with the triageSpanProcessor
// and a synchronous InMemoryExporter for deterministic test assertions.
// The provider is automatically shut down when the test completes.
func newTestProvider(t *testing.T) (*sdktrace.TracerProvider, *tracetest.InMemoryExporter) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(&triageSpanProcessor{}),
		sdktrace.WithSyncer(exporter),
	)
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
	})
	return tp, exporter
}

// attrMap converts a slice of OTel KeyValue attributes into a map for
// easier test assertions. String values become string, int values become
// int64 (the OTel Go SDK internal representation).
func attrMap(kvs []attribute.KeyValue) map[string]any {
	m := make(map[string]any, len(kvs))
	for _, kv := range kvs {
		m[string(kv.Key)] = kv.Value.AsInterface()
	}
	return m
}

// resetSDK resets the global SDK state between tests that call Init().
func resetSDK(t *testing.T) {
	t.Helper()
	mu.Lock()
	defer mu.Unlock()
	if provider != nil {
		_ = provider.Shutdown(context.Background())
	}
	initialized = false
	provider = nil
}
