package triage

import (
	"context"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Compile-time check that triageSpanProcessor implements SpanProcessor.
var _ sdktrace.SpanProcessor = (*triageSpanProcessor)(nil)

// triageSpanProcessor injects triage context attributes into every span on
// start. It reads the triageContext stored in context.Context (set by the
// WithUser/WithTenant/etc. helpers) and writes non-zero values as span
// attributes.
//
// In Go, OTel passes context.Context directly to OnStart, so the processor
// reads triage data from the same ctx the user passed to their LLM call.
type triageSpanProcessor struct{}

func (p *triageSpanProcessor) OnStart(ctx context.Context, span sdktrace.ReadWriteSpan) {
	attrs := getTriageAttrs(ctx)
	if len(attrs) > 0 {
		span.SetAttributes(attrs...)
	}
}

func (p *triageSpanProcessor) OnEnd(_ sdktrace.ReadOnlySpan) {}

func (p *triageSpanProcessor) Shutdown(_ context.Context) error {
	return nil
}

func (p *triageSpanProcessor) ForceFlush(_ context.Context) error {
	return nil
}
