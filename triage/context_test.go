package triage

import (
	"context"
	"encoding/json"
	"testing"
)

// ---------------------------------------------------------------------------
// WithUser
// ---------------------------------------------------------------------------

func TestWithUser_SetsUserID(t *testing.T) {
	ctx := WithUser(context.Background(), "u1")
	attrs := attrMap(getTriageAttrs(ctx))
	if attrs[AttrUserID] != "u1" {
		t.Errorf("got %v, want %q", attrs[AttrUserID], "u1")
	}
}

func TestWithUser_SetsRole(t *testing.T) {
	ctx := WithUser(context.Background(), "u1", UserRole("admin"))
	attrs := attrMap(getTriageAttrs(ctx))
	if attrs[AttrUserID] != "u1" {
		t.Errorf("got %v, want %q", attrs[AttrUserID], "u1")
	}
	if attrs[AttrUserRole] != "admin" {
		t.Errorf("got %v, want %q", attrs[AttrUserRole], "admin")
	}
}

func TestWithUser_OmittedRoleDoesNotOverwrite(t *testing.T) {
	ctx := WithUser(context.Background(), "u1", UserRole("admin"))
	ctx = WithUser(ctx, "u2") // no UserRole — should preserve "admin"
	attrs := attrMap(getTriageAttrs(ctx))
	if attrs[AttrUserID] != "u2" {
		t.Errorf("got %v, want %q", attrs[AttrUserID], "u2")
	}
	if attrs[AttrUserRole] != "admin" {
		t.Errorf("got %v, want %q", attrs[AttrUserRole], "admin")
	}
}

// ---------------------------------------------------------------------------
// WithTenant
// ---------------------------------------------------------------------------

func TestWithTenant_SetsTenantID(t *testing.T) {
	ctx := WithTenant(context.Background(), "org_1")
	attrs := attrMap(getTriageAttrs(ctx))
	if attrs[AttrTenantID] != "org_1" {
		t.Errorf("got %v, want %q", attrs[AttrTenantID], "org_1")
	}
}

func TestWithTenant_SetsName(t *testing.T) {
	ctx := WithTenant(context.Background(), "org_1", TenantName("Acme"))
	attrs := attrMap(getTriageAttrs(ctx))
	if attrs[AttrTenantID] != "org_1" {
		t.Errorf("got %v, want %q", attrs[AttrTenantID], "org_1")
	}
	if attrs[AttrTenantName] != "Acme" {
		t.Errorf("got %v, want %q", attrs[AttrTenantName], "Acme")
	}
}

// ---------------------------------------------------------------------------
// WithSession
// ---------------------------------------------------------------------------

func TestWithSession_SetsSessionID(t *testing.T) {
	ctx := WithSession(context.Background(), "s1")
	attrs := attrMap(getTriageAttrs(ctx))
	if attrs[AttrSessionID] != "s1" {
		t.Errorf("got %v, want %q", attrs[AttrSessionID], "s1")
	}
}

func TestWithSession_SetsTurnAndHash(t *testing.T) {
	ctx := WithSession(context.Background(), "s1", TurnNumber(3), HistoryHash("abc123"))
	attrs := attrMap(getTriageAttrs(ctx))
	if attrs[AttrSessionTurn] != int64(3) {
		t.Errorf("got %v, want %d", attrs[AttrSessionTurn], 3)
	}
	if attrs[AttrSessionHash] != "abc123" {
		t.Errorf("got %v, want %q", attrs[AttrSessionHash], "abc123")
	}
}

func TestWithSession_OmittedOptionalsDoNotOverwrite(t *testing.T) {
	ctx := WithSession(context.Background(), "s1", TurnNumber(1))
	ctx = WithSession(ctx, "s2") // no TurnNumber — should preserve 1
	attrs := attrMap(getTriageAttrs(ctx))
	if attrs[AttrSessionID] != "s2" {
		t.Errorf("got %v, want %q", attrs[AttrSessionID], "s2")
	}
	if attrs[AttrSessionTurn] != int64(1) {
		t.Errorf("got %v, want %d", attrs[AttrSessionTurn], 1)
	}
}

// ---------------------------------------------------------------------------
// WithInput
// ---------------------------------------------------------------------------

func TestWithInput_SetsRaw(t *testing.T) {
	ctx := WithInput(context.Background(), "hello <script>")
	attrs := attrMap(getTriageAttrs(ctx))
	if attrs[AttrInputRaw] != "hello <script>" {
		t.Errorf("got %v, want %q", attrs[AttrInputRaw], "hello <script>")
	}
}

func TestWithInput_SetsSanitized(t *testing.T) {
	ctx := WithInput(context.Background(), "hello <script>", Sanitized("hello"))
	attrs := attrMap(getTriageAttrs(ctx))
	if attrs[AttrInputRaw] != "hello <script>" {
		t.Errorf("got %v, want %q", attrs[AttrInputRaw], "hello <script>")
	}
	if attrs[AttrInputSanitized] != "hello" {
		t.Errorf("got %v, want %q", attrs[AttrInputSanitized], "hello")
	}
}

// ---------------------------------------------------------------------------
// WithTemplate
// ---------------------------------------------------------------------------

func TestWithTemplate_SetsTemplateID(t *testing.T) {
	ctx := WithTemplate(context.Background(), "tmpl_1")
	attrs := attrMap(getTriageAttrs(ctx))
	if attrs[AttrTemplateID] != "tmpl_1" {
		t.Errorf("got %v, want %q", attrs[AttrTemplateID], "tmpl_1")
	}
}

func TestWithTemplate_SetsVersion(t *testing.T) {
	ctx := WithTemplate(context.Background(), "tmpl_1", TemplateVersion("v2.1"))
	attrs := attrMap(getTriageAttrs(ctx))
	if attrs[AttrTemplateID] != "tmpl_1" {
		t.Errorf("got %v, want %q", attrs[AttrTemplateID], "tmpl_1")
	}
	if attrs[AttrTemplateVersion] != "v2.1" {
		t.Errorf("got %v, want %q", attrs[AttrTemplateVersion], "v2.1")
	}
}

// ---------------------------------------------------------------------------
// WithChunkACLs
// ---------------------------------------------------------------------------

func TestWithChunkACLs_JSONSerialized(t *testing.T) {
	acls := []map[string]any{{"chunk_id": "c1", "acl": []any{"role:admin"}}}
	ctx := WithChunkACLs(context.Background(), acls)
	attrs := attrMap(getTriageAttrs(ctx))
	raw, ok := attrs[AttrChunkACLs].(string)
	if !ok {
		t.Fatalf("expected string, got %T", attrs[AttrChunkACLs])
	}
	var parsed []map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if len(parsed) != 1 || parsed[0]["chunk_id"] != "c1" {
		t.Errorf("got %v, want chunk_id=c1", parsed)
	}
}

// ---------------------------------------------------------------------------
// getTriageAttrs
// ---------------------------------------------------------------------------

func TestGetTriageAttrs_EmptyWhenNothingSet(t *testing.T) {
	attrs := getTriageAttrs(context.Background())
	if len(attrs) != 0 {
		t.Errorf("expected empty attrs, got %d", len(attrs))
	}
}

func TestGetTriageAttrs_ReturnsOnlyNonZero(t *testing.T) {
	ctx := WithUser(context.Background(), "u1") // no role
	attrs := attrMap(getTriageAttrs(ctx))
	if _, ok := attrs[AttrUserID]; !ok {
		t.Error("expected AttrUserID to be present")
	}
	if _, ok := attrs[AttrUserRole]; ok {
		t.Error("expected AttrUserRole to be absent (not set)")
	}
}

func TestGetTriageAttrs_MultipleHelpersMerge(t *testing.T) {
	ctx := WithUser(context.Background(), "u1")
	ctx = WithTenant(ctx, "org_1")
	attrs := attrMap(getTriageAttrs(ctx))
	if attrs[AttrUserID] != "u1" {
		t.Errorf("got %v, want %q", attrs[AttrUserID], "u1")
	}
	if attrs[AttrTenantID] != "org_1" {
		t.Errorf("got %v, want %q", attrs[AttrTenantID], "org_1")
	}
}

// ---------------------------------------------------------------------------
// Context scoping (Go equivalent of Python's context manager / TS withContext)
// ---------------------------------------------------------------------------

func TestContextScoping_DifferentContextsAreIsolated(t *testing.T) {
	base := context.Background()

	// Scoped context has user + tenant.
	scoped := WithUser(base, "u_scoped")
	scoped = WithTenant(scoped, "org_scoped")

	scopedAttrs := attrMap(getTriageAttrs(scoped))
	if scopedAttrs[AttrUserID] != "u_scoped" {
		t.Errorf("scoped: got %v, want %q", scopedAttrs[AttrUserID], "u_scoped")
	}

	// Base context still has nothing.
	baseAttrs := getTriageAttrs(base)
	if len(baseAttrs) != 0 {
		t.Errorf("base: expected empty attrs, got %d", len(baseAttrs))
	}
}

func TestContextScoping_ChildInheritsParent(t *testing.T) {
	ctx := WithUser(context.Background(), "u1")
	ctx = WithTenant(ctx, "org_1")
	attrs := attrMap(getTriageAttrs(ctx))
	if attrs[AttrUserID] != "u1" {
		t.Errorf("got %v, want %q", attrs[AttrUserID], "u1")
	}
	if attrs[AttrTenantID] != "org_1" {
		t.Errorf("got %v, want %q", attrs[AttrTenantID], "org_1")
	}
}

func TestContextScoping_OverrideDoesNotAffectParent(t *testing.T) {
	parent := WithTenant(context.Background(), "org_1")
	child := WithTenant(parent, "org_override")

	childAttrs := attrMap(getTriageAttrs(child))
	if childAttrs[AttrTenantID] != "org_override" {
		t.Errorf("child: got %v, want %q", childAttrs[AttrTenantID], "org_override")
	}

	parentAttrs := attrMap(getTriageAttrs(parent))
	if parentAttrs[AttrTenantID] != "org_1" {
		t.Errorf("parent: got %v, want %q", parentAttrs[AttrTenantID], "org_1")
	}
}
