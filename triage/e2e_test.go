package triage

import (
	"context"
	"encoding/json"
	"testing"

	"go.opentelemetry.io/otel/attribute"
)

// ---------------------------------------------------------------------------
// Full annotation flow
// ---------------------------------------------------------------------------

func TestE2E_AllHelpersLandOnExportedSpan(t *testing.T) {
	tp, exporter := newTestProvider(t)
	tracer := tp.Tracer("test")

	ctx := context.Background()
	ctx = WithUser(ctx, "u_42", UserRole("admin"))
	ctx = WithTenant(ctx, "org_7", TenantName("Acme Corp"))
	ctx = WithSession(ctx, "sess_1", TurnNumber(3), HistoryHash("sha256abc"))
	ctx = WithInput(ctx, "drop table users;", Sanitized("drop table users"))
	ctx = WithTemplate(ctx, "tmpl_chat", TemplateVersion("v2.1"))
	ctx = WithChunkACLs(ctx, []map[string]any{{"chunk_id": "c1", "acl": []any{"role:admin"}}})

	_, span := tracer.Start(ctx, "chat gpt-4o")
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	attrs := attrMap(spans[0].Attributes)

	if attrs[AttrUserID] != "u_42" {
		t.Errorf("user_id: got %v, want %q", attrs[AttrUserID], "u_42")
	}
	if attrs[AttrUserRole] != "admin" {
		t.Errorf("user_role: got %v, want %q", attrs[AttrUserRole], "admin")
	}
	if attrs[AttrTenantID] != "org_7" {
		t.Errorf("tenant_id: got %v, want %q", attrs[AttrTenantID], "org_7")
	}
	if attrs[AttrTenantName] != "Acme Corp" {
		t.Errorf("tenant_name: got %v, want %q", attrs[AttrTenantName], "Acme Corp")
	}
	if attrs[AttrSessionID] != "sess_1" {
		t.Errorf("session_id: got %v, want %q", attrs[AttrSessionID], "sess_1")
	}
	if attrs[AttrSessionTurn] != int64(3) {
		t.Errorf("session_turn: got %v, want %d", attrs[AttrSessionTurn], 3)
	}
	if attrs[AttrSessionHash] != "sha256abc" {
		t.Errorf("session_hash: got %v, want %q", attrs[AttrSessionHash], "sha256abc")
	}
	if attrs[AttrInputRaw] != "drop table users;" {
		t.Errorf("input_raw: got %v, want %q", attrs[AttrInputRaw], "drop table users;")
	}
	if attrs[AttrInputSanitized] != "drop table users" {
		t.Errorf("input_sanitized: got %v, want %q", attrs[AttrInputSanitized], "drop table users")
	}
	if attrs[AttrTemplateID] != "tmpl_chat" {
		t.Errorf("template_id: got %v, want %q", attrs[AttrTemplateID], "tmpl_chat")
	}
	if attrs[AttrTemplateVersion] != "v2.1" {
		t.Errorf("template_version: got %v, want %q", attrs[AttrTemplateVersion], "v2.1")
	}

	// Verify chunk ACLs are valid JSON with the expected content.
	raw, ok := attrs[AttrChunkACLs].(string)
	if !ok {
		t.Fatalf("chunk_acls: expected string, got %T", attrs[AttrChunkACLs])
	}
	var parsed []map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("chunk_acls: failed to parse JSON: %v", err)
	}
	if len(parsed) != 1 || parsed[0]["chunk_id"] != "c1" {
		t.Errorf("chunk_acls: got %v, want [{chunk_id:c1, ...}]", parsed)
	}
}

// ---------------------------------------------------------------------------
// Context scoping — Go equivalent of Python context() / TS withContext()
// ---------------------------------------------------------------------------

func TestE2E_AttributesPresentInsideGoneOutside(t *testing.T) {
	tp, exporter := newTestProvider(t)
	tracer := tp.Tracer("test")

	// "Inside" — context with user and tenant.
	scopedCtx := WithUser(context.Background(), "u_scoped")
	scopedCtx = WithTenant(scopedCtx, "org_scoped")

	_, insideSpan := tracer.Start(scopedCtx, "inside")
	insideSpan.End()

	// "Outside" — clean context with no triage data.
	_, outsideSpan := tracer.Start(context.Background(), "outside")
	outsideSpan.End()

	spans := exporter.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}

	insideAttrs := attrMap(spans[0].Attributes)
	if insideAttrs[AttrUserID] != "u_scoped" {
		t.Errorf("inside user_id: got %v, want %q", insideAttrs[AttrUserID], "u_scoped")
	}
	if insideAttrs[AttrTenantID] != "org_scoped" {
		t.Errorf("inside tenant_id: got %v, want %q", insideAttrs[AttrTenantID], "org_scoped")
	}

	outsideAttrs := attrMap(spans[1].Attributes)
	if _, ok := outsideAttrs[AttrUserID]; ok {
		t.Error("outside: expected no user_id attribute")
	}
	if _, ok := outsideAttrs[AttrTenantID]; ok {
		t.Error("outside: expected no tenant_id attribute")
	}
}

// ---------------------------------------------------------------------------
// Triage and gen_ai coexistence
// ---------------------------------------------------------------------------

func TestE2E_TriageAndGenAiCoexistOnSameSpan(t *testing.T) {
	tp, exporter := newTestProvider(t)
	tracer := tp.Tracer("test")

	ctx := WithUser(context.Background(), "u1")

	ctx, span := tracer.Start(ctx, "chat gpt-4o")
	// Simulate what OpenLLMetry auto-instrumentation would set.
	span.SetAttributes(
		attribute.String("gen_ai.system", "openai"),
		attribute.String("gen_ai.request.model", "gpt-4o"),
		attribute.Int("gen_ai.usage.input_tokens", 150),
		attribute.Int("gen_ai.usage.output_tokens", 42),
	)
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	attrs := attrMap(spans[0].Attributes)

	// Triage attributes present.
	if attrs[AttrUserID] != "u1" {
		t.Errorf("user_id: got %v, want %q", attrs[AttrUserID], "u1")
	}
	// gen_ai attributes also present.
	if attrs["gen_ai.system"] != "openai" {
		t.Errorf("gen_ai.system: got %v, want %q", attrs["gen_ai.system"], "openai")
	}
	if attrs["gen_ai.request.model"] != "gpt-4o" {
		t.Errorf("gen_ai.request.model: got %v, want %q", attrs["gen_ai.request.model"], "gpt-4o")
	}
	if attrs["gen_ai.usage.input_tokens"] != int64(150) {
		t.Errorf("gen_ai.usage.input_tokens: got %v, want %d", attrs["gen_ai.usage.input_tokens"], 150)
	}
	if attrs["gen_ai.usage.output_tokens"] != int64(42) {
		t.Errorf("gen_ai.usage.output_tokens: got %v, want %d", attrs["gen_ai.usage.output_tokens"], 42)
	}
}

// ---------------------------------------------------------------------------
// Multi-span trace
// ---------------------------------------------------------------------------

func TestE2E_ParentAndChildSpansAllGetContext(t *testing.T) {
	tp, exporter := newTestProvider(t)
	tracer := tp.Tracer("test")

	ctx := WithUser(context.Background(), "u1")
	ctx = WithSession(ctx, "sess_1", TurnNumber(1))

	ctx, parent := tracer.Start(ctx, "workflow")

	_, child1 := tracer.Start(ctx, "llm-call")
	child1.End()

	_, child2 := tracer.Start(ctx, "tool-call")
	child2.End()

	parent.End()

	spans := exporter.GetSpans()
	if len(spans) != 3 {
		t.Fatalf("expected 3 spans, got %d", len(spans))
	}

	for _, s := range spans {
		attrs := attrMap(s.Attributes)
		if attrs[AttrUserID] != "u1" {
			t.Errorf("span %q: user_id got %v, want %q", s.Name, attrs[AttrUserID], "u1")
		}
		if attrs[AttrSessionID] != "sess_1" {
			t.Errorf("span %q: session_id got %v, want %q", s.Name, attrs[AttrSessionID], "sess_1")
		}
		if attrs[AttrSessionTurn] != int64(1) {
			t.Errorf("span %q: session_turn got %v, want %d", s.Name, attrs[AttrSessionTurn], 1)
		}
	}
}

func TestE2E_ContextChangeBetweenChildSpans(t *testing.T) {
	tp, exporter := newTestProvider(t)
	tracer := tp.Tracer("test")

	ctx := WithSession(context.Background(), "sess_1", TurnNumber(1))

	ctx, parent := tracer.Start(ctx, "workflow")

	// First child — turn 1.
	_, child1 := tracer.Start(ctx, "turn-1")
	child1.End()

	// Update context — turn 2.
	ctx2 := WithSession(ctx, "sess_1", TurnNumber(2))

	// Second child — turn 2.
	_, child2 := tracer.Start(ctx2, "turn-2")
	child2.End()

	parent.End()

	spans := exporter.GetSpans()
	spanMap := make(map[string]map[string]any)
	for _, s := range spans {
		spanMap[s.Name] = attrMap(s.Attributes)
	}

	if spanMap["turn-1"][AttrSessionTurn] != int64(1) {
		t.Errorf("turn-1: got %v, want %d", spanMap["turn-1"][AttrSessionTurn], 1)
	}
	if spanMap["turn-2"][AttrSessionTurn] != int64(2) {
		t.Errorf("turn-2: got %v, want %d", spanMap["turn-2"][AttrSessionTurn], 2)
	}
}
