package triage

import (
	"context"
	"encoding/json"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// ---------------------------------------------------------------------------
// Types for Layer 1 LLM instrumentation
// ---------------------------------------------------------------------------

// Message represents a chat message (prompt or completion).
type Message struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// ToolCall represents a tool/function call in a completion message.
type ToolCall struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolDefinition represents a tool available to the model.
type ToolDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

// Usage holds token usage information from an LLM response.
type Usage struct {
	InputTokens      int `json:"input_tokens"`
	OutputTokens     int `json:"output_tokens"`
	TotalTokens      int `json:"total_tokens"`
	ReasoningTokens  int `json:"reasoning_tokens,omitempty"`
	CacheReadTokens  int `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int `json:"cache_write_tokens,omitempty"`
}

// PromptParams configures an LLM call span.
type PromptParams struct {
	Vendor      string           // LLM provider: "openai", "anthropic", etc.
	Model       string           // Model name: "gpt-4o", "claude-sonnet-4-20250514", etc.
	Messages    []Message        // Prompt messages
	Tools       []ToolDefinition // Available tools/functions
	Temperature *float64
	TopP        *float64
	MaxTokens   *int
	Stop        []string
}

// CompletionParams captures the model response.
type CompletionParams struct {
	Model        string    // Response model (may differ from request model)
	Messages     []Message // Completion messages
	Usage        Usage
	FinishReason string // "stop", "length", "tool_calls", etc.
}

// LLMSpan wraps an OTel span for LLM call instrumentation.
type LLMSpan struct {
	span trace.Span
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// LogPrompt creates a new span for an LLM call, sets prompt attributes,
// and returns an LLMSpan. The caller MUST call LogCompletion() or End() on
// the returned span when the LLM call finishes.
//
// The returned context carries the new span, so child spans will be nested
// under this LLM span. The span also inherits all triage.* context attributes
// from the parent context via the TriageSpanProcessor.
//
// Example:
//
//	ctx, llmSpan := triage.LogPrompt(ctx, triage.PromptParams{
//	    Vendor:   "openai",
//	    Model:    "gpt-4o",
//	    Messages: []triage.Message{{Role: "user", Content: "Hello"}},
//	})
//	// ... make LLM call ...
//	llmSpan.LogCompletion(triage.CompletionParams{
//	    Model:    "gpt-4o",
//	    Messages: []triage.Message{{Role: "assistant", Content: "Hi!"}},
//	    Usage:    triage.Usage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15},
//	})
func LogPrompt(ctx context.Context, params PromptParams) (context.Context, *LLMSpan) {
	spanName := params.Vendor + ".chat"
	if params.Vendor == "" {
		spanName = "llm.chat"
	}

	tracer := otel.GetTracerProvider().Tracer(sdkName)
	ctx, span := tracer.Start(ctx, spanName)

	// Set request attributes.
	if params.Vendor != "" {
		span.SetAttributes(attribute.String(AttrGenAISystem, params.Vendor))
	}
	if params.Model != "" {
		span.SetAttributes(attribute.String(AttrGenAIRequestModel, params.Model))
	}
	if params.Temperature != nil {
		span.SetAttributes(attribute.Float64(AttrGenAIRequestTemperature, *params.Temperature))
	}
	if params.TopP != nil {
		span.SetAttributes(attribute.Float64(AttrGenAIRequestTopP, *params.TopP))
	}
	if params.MaxTokens != nil {
		span.SetAttributes(attribute.Int(AttrGenAIRequestMaxTokens, *params.MaxTokens))
	}
	if len(params.Stop) > 0 {
		span.SetAttributes(attribute.StringSlice(AttrGenAIRequestStopSequences, params.Stop))
	}

	// Set prompt messages and tool definitions if content tracing is enabled.
	if shouldTraceContent() {
		setMessageAttrs(span, "gen_ai.prompt", params.Messages)
		setToolDefinitionAttrs(span, params.Tools)
	}

	return ctx, &LLMSpan{span: span}
}

// LogCompletion sets completion attributes on the span and ends it.
// This should be called exactly once when the LLM response is received.
func (s *LLMSpan) LogCompletion(params CompletionParams) {
	if s.span == nil {
		return
	}
	defer s.span.End()

	if params.Model != "" {
		s.span.SetAttributes(attribute.String(AttrGenAIResponseModel, params.Model))
	}
	if params.FinishReason != "" {
		s.span.SetAttributes(attribute.String(AttrGenAIResponseFinishReason, params.FinishReason))
	}

	// Token usage.
	if params.Usage.InputTokens > 0 {
		s.span.SetAttributes(attribute.Int(AttrGenAIUsageInputTokens, params.Usage.InputTokens))
	}
	if params.Usage.OutputTokens > 0 {
		s.span.SetAttributes(attribute.Int(AttrGenAIUsageOutputTokens, params.Usage.OutputTokens))
	}
	if params.Usage.TotalTokens > 0 {
		s.span.SetAttributes(attribute.Int(AttrGenAIUsageTotalTokens, params.Usage.TotalTokens))
	}
	if params.Usage.ReasoningTokens > 0 {
		s.span.SetAttributes(attribute.Int(AttrGenAIUsageReasoningTokens, params.Usage.ReasoningTokens))
	}
	if params.Usage.CacheReadTokens > 0 {
		s.span.SetAttributes(attribute.Int(AttrGenAIUsageCacheReadTokens, params.Usage.CacheReadTokens))
	}
	if params.Usage.CacheWriteTokens > 0 {
		s.span.SetAttributes(attribute.Int(AttrGenAIUsageCacheWriteTokens, params.Usage.CacheWriteTokens))
	}

	// Completion messages.
	if shouldTraceContent() {
		setMessageAttrs(s.span, "gen_ai.completion", params.Messages)
	}
}

// End ends the span without logging completion attributes. Use this when
// the LLM call fails and no completion is available.
func (s *LLMSpan) End() {
	if s.span != nil {
		s.span.End()
	}
}

// SetError records an error on the LLM span and sets the span status to Error.
func (s *LLMSpan) SetError(err error) {
	if s.span != nil {
		s.span.RecordError(err)
		s.span.SetStatus(codes.Error, err.Error())
	}
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// shouldTraceContent checks if content tracing is enabled.
// Returns true by default (when SDK is not initialized or config not set).
func shouldTraceContent() bool {
	mu.Lock()
	defer mu.Unlock()
	return activeConfig == nil || activeConfig.traceContent
}

// setMessageAttrs sets indexed message attributes on a span.
// prefix is "gen_ai.prompt" or "gen_ai.completion".
func setMessageAttrs(span trace.Span, prefix string, messages []Message) {
	for i, msg := range messages {
		p := fmt.Sprintf("%s.%d", prefix, i)
		if msg.Role != "" {
			span.SetAttributes(attribute.String(p+".role", msg.Role))
		}
		if msg.Content != "" {
			span.SetAttributes(attribute.String(p+".content", msg.Content))
		}
		for j, tc := range msg.ToolCalls {
			tp := fmt.Sprintf("%s.tool_calls.%d", p, j)
			if tc.ID != "" {
				span.SetAttributes(attribute.String(tp+".id", tc.ID))
			}
			if tc.Type != "" {
				span.SetAttributes(attribute.String(tp+".type", tc.Type))
			}
			if tc.Name != "" {
				span.SetAttributes(attribute.String(tp+".name", tc.Name))
			}
			if tc.Arguments != "" {
				span.SetAttributes(attribute.String(tp+".arguments", tc.Arguments))
			}
		}
	}
}

// setToolDefinitionAttrs sets tool/function definition attributes on a span.
func setToolDefinitionAttrs(span trace.Span, tools []ToolDefinition) {
	for i, tool := range tools {
		p := fmt.Sprintf("gen_ai.request.tools.%d", i)
		if tool.Name != "" {
			span.SetAttributes(attribute.String(p+".name", tool.Name))
		}
		if tool.Description != "" {
			span.SetAttributes(attribute.String(p+".description", tool.Description))
		}
		if tool.Parameters != nil {
			data, err := json.Marshal(tool.Parameters)
			if err == nil {
				span.SetAttributes(attribute.String(p+".parameters", string(data)))
			}
		}
	}
}
