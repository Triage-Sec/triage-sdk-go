package triage

// Triage context span attributes — the 6 developer annotation helpers.
const (
	AttrUserID          = "triage.user.id"
	AttrUserRole        = "triage.user.role"
	AttrTenantID        = "triage.tenant.id"
	AttrTenantName      = "triage.tenant.name"
	AttrSessionID       = "triage.session.id"
	AttrSessionTurn     = "triage.session.turn_number"
	AttrSessionHash     = "triage.session.history_hash"
	AttrInputRaw        = "triage.input.raw"
	AttrInputSanitized  = "triage.input.sanitized"
	AttrTemplateID      = "triage.template.id"
	AttrTemplateVersion = "triage.template.version"
	AttrChunkACLs       = "triage.chunk_acls"
)

// SDK metadata span attributes.
const (
	AttrSDKName    = "triage.sdk.name"
	AttrSDKVersion = "triage.sdk.version"
	sdkName        = "triage-sdk-go"
)

// Environment variable names.
const (
	EnvAPIKey       = "TRIAGE_API_KEY"
	EnvEndpoint     = "TRIAGE_ENDPOINT"
	EnvAppName      = "TRIAGE_APP_NAME"
	EnvEnvironment  = "TRIAGE_ENVIRONMENT"
	EnvEnabled      = "TRIAGE_ENABLED"
	EnvTraceContent = "TRIAGE_TRACE_CONTENT"
)

// Defaults.
const (
	DefaultEndpoint       = "https://api.triageai.dev"
	defaultOTLPTracesPath = "/v1/traces"
)
