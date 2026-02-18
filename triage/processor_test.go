package triage

import (
	"context"
	"testing"
)

func TestProcessor_ContextInjectedIntoSpan(t *testing.T) {
	tp, exporter := newTestProvider(t)
	tracer := tp.Tracer("test")

	ctx := WithUser(context.Background(), "u1", UserRole("admin"))
	ctx = WithTenant(ctx, "org_1")

	ctx, span := tracer.Start(ctx, "llm-call")
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	attrs := attrMap(spans[0].Attributes)
	if attrs[AttrUserID] != "u1" {
		t.Errorf("got %v, want %q", attrs[AttrUserID], "u1")
	}
	if attrs[AttrUserRole] != "admin" {
		t.Errorf("got %v, want %q", attrs[AttrUserRole], "admin")
	}
	if attrs[AttrTenantID] != "org_1" {
		t.Errorf("got %v, want %q", attrs[AttrTenantID], "org_1")
	}
}

func TestProcessor_NoContextMeansNoTriageAttrs(t *testing.T) {
	tp, exporter := newTestProvider(t)
	tracer := tp.Tracer("test")

	// Start span without any triage context.
	_, span := tracer.Start(context.Background(), "clean-span")
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	for _, kv := range spans[0].Attributes {
		if len(kv.Key) > 7 && string(kv.Key)[:7] == "triage." {
			t.Errorf("unexpected triage attribute: %s", kv.Key)
		}
	}
}

func TestProcessor_DifferentContextPerSpan(t *testing.T) {
	tp, exporter := newTestProvider(t)
	tracer := tp.Tracer("test")

	ctx1 := WithUser(context.Background(), "u1")
	_, span1 := tracer.Start(ctx1, "span-1")
	span1.End()

	ctx2 := WithUser(context.Background(), "u2")
	_, span2 := tracer.Start(ctx2, "span-2")
	span2.End()

	spans := exporter.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}
	if attrMap(spans[0].Attributes)[AttrUserID] != "u1" {
		t.Errorf("span-1: got %v, want %q", attrMap(spans[0].Attributes)[AttrUserID], "u1")
	}
	if attrMap(spans[1].Attributes)[AttrUserID] != "u2" {
		t.Errorf("span-2: got %v, want %q", attrMap(spans[1].Attributes)[AttrUserID], "u2")
	}
}

func TestProcessor_MultipleFields(t *testing.T) {
	tp, exporter := newTestProvider(t)
	tracer := tp.Tracer("test")

	ctx := WithUser(context.Background(), "u1", UserRole("viewer"))
	ctx = WithTenant(ctx, "org_5")
	ctx = WithSession(ctx, "sess_9")

	_, span := tracer.Start(ctx, "multi")
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	attrs := attrMap(spans[0].Attributes)
	if attrs[AttrUserID] != "u1" {
		t.Errorf("got %v, want %q", attrs[AttrUserID], "u1")
	}
	if attrs[AttrUserRole] != "viewer" {
		t.Errorf("got %v, want %q", attrs[AttrUserRole], "viewer")
	}
	if attrs[AttrTenantID] != "org_5" {
		t.Errorf("got %v, want %q", attrs[AttrTenantID], "org_5")
	}
	if attrs[AttrSessionID] != "sess_9" {
		t.Errorf("got %v, want %q", attrs[AttrSessionID], "sess_9")
	}
}
