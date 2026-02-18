package triage

import (
	"context"
	"encoding/json"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// contextKey is an unexported type used as the key for storing triageContext
// in context.Context. Using a private type prevents collisions with keys
// from other packages.
type contextKey struct{}

// triageContext holds all triage annotation values attached to a context.
type triageContext struct {
	userID             string
	userRole           string
	tenantID           string
	tenantName         string
	sessionID          string
	sessionTurnNumber  *int
	sessionHistoryHash string
	inputRaw           string
	inputSanitized     string
	templateID         string
	templateVersion    string
	chunkACLs          string // JSON-serialized
}

// clone returns a shallow copy of the context so callers can mutate the copy
// without affecting the original.
func (tc triageContext) clone() triageContext {
	c := tc
	if tc.sessionTurnNumber != nil {
		n := *tc.sessionTurnNumber
		c.sessionTurnNumber = &n
	}
	return c
}

// ---------------------------------------------------------------------------
// Per-helper option types — idiomatic Go optional parameters
// ---------------------------------------------------------------------------

// UserOption configures optional fields for WithUser.
type UserOption func(*triageContext)

// UserRole sets the user role (e.g. "admin", "viewer").
func UserRole(role string) UserOption {
	return func(tc *triageContext) { tc.userRole = role }
}

// TenantOption configures optional fields for WithTenant.
type TenantOption func(*triageContext)

// TenantName sets the tenant/organization display name.
func TenantName(name string) TenantOption {
	return func(tc *triageContext) { tc.tenantName = name }
}

// SessionOption configures optional fields for WithSession.
type SessionOption func(*triageContext)

// TurnNumber sets the conversation turn number.
func TurnNumber(n int) SessionOption {
	return func(tc *triageContext) { tc.sessionTurnNumber = &n }
}

// HistoryHash sets the session history hash.
func HistoryHash(h string) SessionOption {
	return func(tc *triageContext) { tc.sessionHistoryHash = h }
}

// InputOption configures optional fields for WithInput.
type InputOption func(*triageContext)

// Sanitized sets the sanitized version of the user input.
func Sanitized(s string) InputOption {
	return func(tc *triageContext) { tc.inputSanitized = s }
}

// TemplateOption configures optional fields for WithTemplate.
type TemplateOption func(*triageContext)

// TemplateVersion sets the prompt template version.
func TemplateVersion(v string) TemplateOption {
	return func(tc *triageContext) { tc.templateVersion = v }
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// getFromContext extracts the triageContext from ctx, or returns a zero value
// if none is present.
func getFromContext(ctx context.Context) triageContext {
	if tc, ok := ctx.Value(contextKey{}).(triageContext); ok {
		return tc
	}
	return triageContext{}
}

// setInContext stores a triageContext in ctx and returns the new context.
func setInContext(ctx context.Context, tc triageContext) context.Context {
	return context.WithValue(ctx, contextKey{}, tc)
}

// getTriageAttrs reads the triageContext from ctx and returns a slice of
// non-zero-value OTel attributes. Used by the span processor.
func getTriageAttrs(ctx context.Context) []attribute.KeyValue {
	tc := getFromContext(ctx)

	var attrs []attribute.KeyValue
	if tc.userID != "" {
		attrs = append(attrs, attribute.String(AttrUserID, tc.userID))
	}
	if tc.userRole != "" {
		attrs = append(attrs, attribute.String(AttrUserRole, tc.userRole))
	}
	if tc.tenantID != "" {
		attrs = append(attrs, attribute.String(AttrTenantID, tc.tenantID))
	}
	if tc.tenantName != "" {
		attrs = append(attrs, attribute.String(AttrTenantName, tc.tenantName))
	}
	if tc.sessionID != "" {
		attrs = append(attrs, attribute.String(AttrSessionID, tc.sessionID))
	}
	if tc.sessionTurnNumber != nil {
		attrs = append(attrs, attribute.Int(AttrSessionTurn, *tc.sessionTurnNumber))
	}
	if tc.sessionHistoryHash != "" {
		attrs = append(attrs, attribute.String(AttrSessionHash, tc.sessionHistoryHash))
	}
	if tc.inputRaw != "" {
		attrs = append(attrs, attribute.String(AttrInputRaw, tc.inputRaw))
	}
	if tc.inputSanitized != "" {
		attrs = append(attrs, attribute.String(AttrInputSanitized, tc.inputSanitized))
	}
	if tc.templateID != "" {
		attrs = append(attrs, attribute.String(AttrTemplateID, tc.templateID))
	}
	if tc.templateVersion != "" {
		attrs = append(attrs, attribute.String(AttrTemplateVersion, tc.templateVersion))
	}
	if tc.chunkACLs != "" {
		attrs = append(attrs, attribute.String(AttrChunkACLs, tc.chunkACLs))
	}
	return attrs
}

// ---------------------------------------------------------------------------
// Public API — the 6 developer annotation helpers
// ---------------------------------------------------------------------------

// WithUser attaches user identity to the context. The returned context
// carries the user ID and any optional fields (e.g. UserRole) so that
// all spans created with this context include the triage.user.* attributes.
func WithUser(ctx context.Context, userID string, opts ...UserOption) context.Context {
	tc := getFromContext(ctx).clone()
	tc.userID = userID
	for _, o := range opts {
		o(&tc)
	}

	// Also set on current span for immediate effect on already-started spans.
	if span := trace.SpanFromContext(ctx); span.IsRecording() {
		span.SetAttributes(attribute.String(AttrUserID, tc.userID))
		if tc.userRole != "" {
			span.SetAttributes(attribute.String(AttrUserRole, tc.userRole))
		}
	}

	return setInContext(ctx, tc)
}

// WithTenant attaches tenant/organization identity to the context.
func WithTenant(ctx context.Context, tenantID string, opts ...TenantOption) context.Context {
	tc := getFromContext(ctx).clone()
	tc.tenantID = tenantID
	for _, o := range opts {
		o(&tc)
	}

	if span := trace.SpanFromContext(ctx); span.IsRecording() {
		span.SetAttributes(attribute.String(AttrTenantID, tc.tenantID))
		if tc.tenantName != "" {
			span.SetAttributes(attribute.String(AttrTenantName, tc.tenantName))
		}
	}

	return setInContext(ctx, tc)
}

// WithSession attaches conversation session metadata to the context.
func WithSession(ctx context.Context, sessionID string, opts ...SessionOption) context.Context {
	tc := getFromContext(ctx).clone()
	tc.sessionID = sessionID
	for _, o := range opts {
		o(&tc)
	}

	if span := trace.SpanFromContext(ctx); span.IsRecording() {
		span.SetAttributes(attribute.String(AttrSessionID, tc.sessionID))
		if tc.sessionTurnNumber != nil {
			span.SetAttributes(attribute.Int(AttrSessionTurn, *tc.sessionTurnNumber))
		}
		if tc.sessionHistoryHash != "" {
			span.SetAttributes(attribute.String(AttrSessionHash, tc.sessionHistoryHash))
		}
	}

	return setInContext(ctx, tc)
}

// WithInput attaches raw (and optionally sanitized) user input to the context.
func WithInput(ctx context.Context, raw string, opts ...InputOption) context.Context {
	tc := getFromContext(ctx).clone()
	tc.inputRaw = raw
	for _, o := range opts {
		o(&tc)
	}

	if span := trace.SpanFromContext(ctx); span.IsRecording() {
		span.SetAttributes(attribute.String(AttrInputRaw, tc.inputRaw))
		if tc.inputSanitized != "" {
			span.SetAttributes(attribute.String(AttrInputSanitized, tc.inputSanitized))
		}
	}

	return setInContext(ctx, tc)
}

// WithTemplate attaches prompt template metadata to the context.
func WithTemplate(ctx context.Context, templateID string, opts ...TemplateOption) context.Context {
	tc := getFromContext(ctx).clone()
	tc.templateID = templateID
	for _, o := range opts {
		o(&tc)
	}

	if span := trace.SpanFromContext(ctx); span.IsRecording() {
		span.SetAttributes(attribute.String(AttrTemplateID, tc.templateID))
		if tc.templateVersion != "" {
			span.SetAttributes(attribute.String(AttrTemplateVersion, tc.templateVersion))
		}
	}

	return setInContext(ctx, tc)
}

// WithChunkACLs attaches retrieved chunk access control metadata to the
// context. The acls slice is JSON-serialized and stored as a string attribute
// because OTel span attributes only support primitive types.
func WithChunkACLs(ctx context.Context, acls []map[string]any) context.Context {
	tc := getFromContext(ctx).clone()

	data, err := json.Marshal(acls)
	if err != nil {
		// Don't break the user's application for a telemetry failure.
		return setInContext(ctx, tc)
	}
	tc.chunkACLs = string(data)

	if span := trace.SpanFromContext(ctx); span.IsRecording() {
		span.SetAttributes(attribute.String(AttrChunkACLs, tc.chunkACLs))
	}

	return setInContext(ctx, tc)
}
