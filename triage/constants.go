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

// Layer 1: gen_ai semantic convention attributes (matching Python/TypeScript OpenLLMetry).
const (
	AttrGenAISystem               = "gen_ai.system"
	AttrGenAIRequestModel         = "gen_ai.request.model"
	AttrGenAIResponseModel        = "gen_ai.response.model"
	AttrGenAIRequestTemperature   = "gen_ai.request.temperature"
	AttrGenAIRequestTopP          = "gen_ai.request.top_p"
	AttrGenAIRequestMaxTokens     = "gen_ai.request.max_tokens"
	AttrGenAIRequestStopSequences = "gen_ai.request.stop_sequences"
	AttrGenAIUsageInputTokens     = "gen_ai.usage.input_tokens"
	AttrGenAIUsageOutputTokens    = "gen_ai.usage.output_tokens"
	AttrGenAIUsageTotalTokens     = "gen_ai.usage.total_tokens"
	AttrGenAIUsageReasoningTokens = "gen_ai.usage.reasoning_tokens"
	AttrGenAIUsageCacheReadTokens = "gen_ai.usage.cache_read_tokens"
	AttrGenAIUsageCacheWriteTokens = "gen_ai.usage.cache_write_tokens"
	AttrGenAIResponseFinishReason = "gen_ai.response.finish_reason"
)

// Defaults.
const (
	DefaultEndpoint       = "https://api.triageai.dev"
	defaultOTLPTracesPath = "/v1/traces"
)
