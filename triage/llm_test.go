package triage

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// setGlobalTestProvider sets the global OTel TracerProvider to a test provider
// so that LogPrompt (which uses otel.GetTracerProvider()) works in tests.
func setGlobalTestProvider(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	tp, exporter := newTestProvider(t)
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		otel.SetTracerProvider(prev)
	})
	return exporter
}

// ---------------------------------------------------------------------------
// LogPrompt — span creation and request attributes
// ---------------------------------------------------------------------------

func TestLogPrompt_CreatesSpanWithVendorAndModel(t *testing.T) {
	exporter := setGlobalTestProvider(t)

	_, llmSpan := LogPrompt(context.Background(), PromptParams{
		Vendor: "openai",
		Model:  "gpt-4o",
	})
	llmSpan.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name != "openai.chat" {
		t.Errorf("span name: got %q, want %q", spans[0].Name, "openai.chat")
	}
	attrs := attrMap(spans[0].Attributes)
	if attrs[AttrGenAISystem] != "openai" {
		t.Errorf("gen_ai.system: got %v, want %q", attrs[AttrGenAISystem], "openai")
	}
	if attrs[AttrGenAIRequestModel] != "gpt-4o" {
		t.Errorf("gen_ai.request.model: got %v, want %q", attrs[AttrGenAIRequestModel], "gpt-4o")
	}
}

func TestLogPrompt_NoVendorDefaultsSpanName(t *testing.T) {
	exporter := setGlobalTestProvider(t)

	_, llmSpan := LogPrompt(context.Background(), PromptParams{
		Model: "gpt-4o",
	})
	llmSpan.End()

	spans := exporter.GetSpans()
	if spans[0].Name != "llm.chat" {
		t.Errorf("span name: got %q, want %q", spans[0].Name, "llm.chat")
	}
}

func TestLogPrompt_AnthropicVendor(t *testing.T) {
	exporter := setGlobalTestProvider(t)

	_, llmSpan := LogPrompt(context.Background(), PromptParams{
		Vendor: "anthropic",
		Model:  "claude-sonnet-4-20250514",
	})
	llmSpan.End()

	spans := exporter.GetSpans()
	if spans[0].Name != "anthropic.chat" {
		t.Errorf("span name: got %q, want %q", spans[0].Name, "anthropic.chat")
	}
	attrs := attrMap(spans[0].Attributes)
	if attrs[AttrGenAISystem] != "anthropic" {
		t.Errorf("gen_ai.system: got %v, want %q", attrs[AttrGenAISystem], "anthropic")
	}
}

func TestLogPrompt_RequestParams(t *testing.T) {
	exporter := setGlobalTestProvider(t)

	temp := 0.7
	topP := 0.9
	maxTok := 1024
	_, llmSpan := LogPrompt(context.Background(), PromptParams{
		Vendor:      "openai",
		Model:       "gpt-4o",
		Temperature: &temp,
		TopP:        &topP,
		MaxTokens:   &maxTok,
		Stop:        []string{"STOP", "END"},
	})
	llmSpan.End()

	attrs := attrMap(exporter.GetSpans()[0].Attributes)
	if attrs[AttrGenAIRequestTemperature] != 0.7 {
		t.Errorf("temperature: got %v, want %v", attrs[AttrGenAIRequestTemperature], 0.7)
	}
	if attrs[AttrGenAIRequestTopP] != 0.9 {
		t.Errorf("top_p: got %v, want %v", attrs[AttrGenAIRequestTopP], 0.9)
	}
	if attrs[AttrGenAIRequestMaxTokens] != int64(1024) {
		t.Errorf("max_tokens: got %v, want %v", attrs[AttrGenAIRequestMaxTokens], 1024)
	}
}

// ---------------------------------------------------------------------------
// LogPrompt — message attributes
// ---------------------------------------------------------------------------

func TestLogPrompt_MessagesAsAttributes(t *testing.T) {
	exporter := setGlobalTestProvider(t)

	_, llmSpan := LogPrompt(context.Background(), PromptParams{
		Vendor: "openai",
		Model:  "gpt-4o",
		Messages: []Message{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "Hello"},
		},
	})
	llmSpan.End()

	attrs := attrMap(exporter.GetSpans()[0].Attributes)
	if attrs["gen_ai.prompt.0.role"] != "system" {
		t.Errorf("prompt.0.role: got %v, want %q", attrs["gen_ai.prompt.0.role"], "system")
	}
	if attrs["gen_ai.prompt.0.content"] != "You are helpful." {
		t.Errorf("prompt.0.content: got %v, want %q", attrs["gen_ai.prompt.0.content"], "You are helpful.")
	}
	if attrs["gen_ai.prompt.1.role"] != "user" {
		t.Errorf("prompt.1.role: got %v, want %q", attrs["gen_ai.prompt.1.role"], "user")
	}
	if attrs["gen_ai.prompt.1.content"] != "Hello" {
		t.Errorf("prompt.1.content: got %v, want %q", attrs["gen_ai.prompt.1.content"], "Hello")
	}
}

func TestLogPrompt_ToolDefinitions(t *testing.T) {
	exporter := setGlobalTestProvider(t)

	_, llmSpan := LogPrompt(context.Background(), PromptParams{
		Vendor: "openai",
		Model:  "gpt-4o",
		Tools: []ToolDefinition{
			{
				Name:        "get_weather",
				Description: "Get current weather",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"city": map[string]any{"type": "string"},
					},
				},
			},
		},
	})
	llmSpan.End()

	attrs := attrMap(exporter.GetSpans()[0].Attributes)
	if attrs["gen_ai.request.tools.0.name"] != "get_weather" {
		t.Errorf("tools.0.name: got %v, want %q", attrs["gen_ai.request.tools.0.name"], "get_weather")
	}
	if attrs["gen_ai.request.tools.0.description"] != "Get current weather" {
		t.Errorf("tools.0.description: got %v, want %q", attrs["gen_ai.request.tools.0.description"], "Get current weather")
	}
	if _, ok := attrs["gen_ai.request.tools.0.parameters"]; !ok {
		t.Error("tools.0.parameters: expected to be present")
	}
}

// ---------------------------------------------------------------------------
// LogCompletion — response attributes
// ---------------------------------------------------------------------------

func TestLogCompletion_SetsModelAndUsage(t *testing.T) {
	exporter := setGlobalTestProvider(t)

	_, llmSpan := LogPrompt(context.Background(), PromptParams{
		Vendor: "openai",
		Model:  "gpt-4o",
	})
	llmSpan.LogCompletion(CompletionParams{
		Model: "gpt-4o-2024-08-06",
		Usage: Usage{
			InputTokens:  150,
			OutputTokens: 42,
			TotalTokens:  192,
		},
		FinishReason: "stop",
	})

	attrs := attrMap(exporter.GetSpans()[0].Attributes)
	if attrs[AttrGenAIResponseModel] != "gpt-4o-2024-08-06" {
		t.Errorf("response.model: got %v, want %q", attrs[AttrGenAIResponseModel], "gpt-4o-2024-08-06")
	}
	if attrs[AttrGenAIUsageInputTokens] != int64(150) {
		t.Errorf("input_tokens: got %v, want %d", attrs[AttrGenAIUsageInputTokens], 150)
	}
	if attrs[AttrGenAIUsageOutputTokens] != int64(42) {
		t.Errorf("output_tokens: got %v, want %d", attrs[AttrGenAIUsageOutputTokens], 42)
	}
	if attrs[AttrGenAIUsageTotalTokens] != int64(192) {
		t.Errorf("total_tokens: got %v, want %d", attrs[AttrGenAIUsageTotalTokens], 192)
	}
	if attrs[AttrGenAIResponseFinishReason] != "stop" {
		t.Errorf("finish_reason: got %v, want %q", attrs[AttrGenAIResponseFinishReason], "stop")
	}
}

func TestLogCompletion_CompletionMessages(t *testing.T) {
	exporter := setGlobalTestProvider(t)

	_, llmSpan := LogPrompt(context.Background(), PromptParams{
		Vendor: "openai",
		Model:  "gpt-4o",
	})
	llmSpan.LogCompletion(CompletionParams{
		Messages: []Message{
			{Role: "assistant", Content: "Hello! How can I help?"},
		},
		Usage: Usage{InputTokens: 10, OutputTokens: 8, TotalTokens: 18},
	})

	attrs := attrMap(exporter.GetSpans()[0].Attributes)
	if attrs["gen_ai.completion.0.role"] != "assistant" {
		t.Errorf("completion.0.role: got %v, want %q", attrs["gen_ai.completion.0.role"], "assistant")
	}
	if attrs["gen_ai.completion.0.content"] != "Hello! How can I help?" {
		t.Errorf("completion.0.content: got %v, want %q", attrs["gen_ai.completion.0.content"], "Hello! How can I help?")
	}
}

func TestLogCompletion_ToolCalls(t *testing.T) {
	exporter := setGlobalTestProvider(t)

	_, llmSpan := LogPrompt(context.Background(), PromptParams{
		Vendor: "openai",
		Model:  "gpt-4o",
	})
	llmSpan.LogCompletion(CompletionParams{
		Messages: []Message{
			{
				Role: "assistant",
				ToolCalls: []ToolCall{
					{
						ID:        "call_abc123",
						Type:      "function",
						Name:      "get_weather",
						Arguments: `{"city":"SF"}`,
					},
					{
						ID:        "call_def456",
						Type:      "function",
						Name:      "get_time",
						Arguments: `{"tz":"PST"}`,
					},
				},
			},
		},
		FinishReason: "tool_calls",
		Usage:        Usage{InputTokens: 50, OutputTokens: 20, TotalTokens: 70},
	})

	attrs := attrMap(exporter.GetSpans()[0].Attributes)

	// First tool call.
	if attrs["gen_ai.completion.0.tool_calls.0.id"] != "call_abc123" {
		t.Errorf("tool_calls.0.id: got %v, want %q", attrs["gen_ai.completion.0.tool_calls.0.id"], "call_abc123")
	}
	if attrs["gen_ai.completion.0.tool_calls.0.type"] != "function" {
		t.Errorf("tool_calls.0.type: got %v, want %q", attrs["gen_ai.completion.0.tool_calls.0.type"], "function")
	}
	if attrs["gen_ai.completion.0.tool_calls.0.name"] != "get_weather" {
		t.Errorf("tool_calls.0.name: got %v, want %q", attrs["gen_ai.completion.0.tool_calls.0.name"], "get_weather")
	}
	if attrs["gen_ai.completion.0.tool_calls.0.arguments"] != `{"city":"SF"}` {
		t.Errorf("tool_calls.0.arguments: got %v, want %q", attrs["gen_ai.completion.0.tool_calls.0.arguments"], `{"city":"SF"}`)
	}

	// Second tool call.
	if attrs["gen_ai.completion.0.tool_calls.1.name"] != "get_time" {
		t.Errorf("tool_calls.1.name: got %v, want %q", attrs["gen_ai.completion.0.tool_calls.1.name"], "get_time")
	}

	// Finish reason.
	if attrs[AttrGenAIResponseFinishReason] != "tool_calls" {
		t.Errorf("finish_reason: got %v, want %q", attrs[AttrGenAIResponseFinishReason], "tool_calls")
	}
}

// ---------------------------------------------------------------------------
// Reasoning tokens (chain of thought auditing)
// ---------------------------------------------------------------------------

func TestLogCompletion_ReasoningTokens(t *testing.T) {
	exporter := setGlobalTestProvider(t)

	_, llmSpan := LogPrompt(context.Background(), PromptParams{
		Vendor: "openai",
		Model:  "o1-preview",
	})
	llmSpan.LogCompletion(CompletionParams{
		Model: "o1-preview",
		Messages: []Message{
			{Role: "assistant", Content: "The answer is 42."},
		},
		Usage: Usage{
			InputTokens:     100,
			OutputTokens:    50,
			TotalTokens:     150,
			ReasoningTokens: 30,
		},
		FinishReason: "stop",
	})

	attrs := attrMap(exporter.GetSpans()[0].Attributes)
	if attrs[AttrGenAIUsageReasoningTokens] != int64(30) {
		t.Errorf("reasoning_tokens: got %v, want %d", attrs[AttrGenAIUsageReasoningTokens], 30)
	}
}

// ---------------------------------------------------------------------------
// Cache tokens
// ---------------------------------------------------------------------------

func TestLogCompletion_CacheTokens(t *testing.T) {
	exporter := setGlobalTestProvider(t)

	_, llmSpan := LogPrompt(context.Background(), PromptParams{
		Vendor: "anthropic",
		Model:  "claude-sonnet-4-20250514",
	})
	llmSpan.LogCompletion(CompletionParams{
		Usage: Usage{
			InputTokens:      200,
			OutputTokens:     100,
			TotalTokens:      300,
			CacheReadTokens:  150,
			CacheWriteTokens: 50,
		},
	})

	attrs := attrMap(exporter.GetSpans()[0].Attributes)
	if attrs[AttrGenAIUsageCacheReadTokens] != int64(150) {
		t.Errorf("cache_read_tokens: got %v, want %d", attrs[AttrGenAIUsageCacheReadTokens], 150)
	}
	if attrs[AttrGenAIUsageCacheWriteTokens] != int64(50) {
		t.Errorf("cache_write_tokens: got %v, want %d", attrs[AttrGenAIUsageCacheWriteTokens], 50)
	}
}

// ---------------------------------------------------------------------------
// Triage context propagation (Layer 1 + Layer 2 coexistence)
// ---------------------------------------------------------------------------

func TestLogPrompt_TriageContextPropagated(t *testing.T) {
	exporter := setGlobalTestProvider(t)

	ctx := WithUser(context.Background(), "u_42", UserRole("admin"))
	ctx = WithTenant(ctx, "org_7", TenantName("Acme"))

	_, llmSpan := LogPrompt(ctx, PromptParams{
		Vendor: "openai",
		Model:  "gpt-4o",
	})
	llmSpan.LogCompletion(CompletionParams{
		Usage: Usage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15},
	})

	attrs := attrMap(exporter.GetSpans()[0].Attributes)

	// Layer 1 attributes present.
	if attrs[AttrGenAISystem] != "openai" {
		t.Errorf("gen_ai.system: got %v, want %q", attrs[AttrGenAISystem], "openai")
	}

	// Layer 2 attributes present (injected by triageSpanProcessor).
	if attrs[AttrUserID] != "u_42" {
		t.Errorf("triage.user.id: got %v, want %q", attrs[AttrUserID], "u_42")
	}
	if attrs[AttrUserRole] != "admin" {
		t.Errorf("triage.user.role: got %v, want %q", attrs[AttrUserRole], "admin")
	}
	if attrs[AttrTenantID] != "org_7" {
		t.Errorf("triage.tenant.id: got %v, want %q", attrs[AttrTenantID], "org_7")
	}
	if attrs[AttrTenantName] != "Acme" {
		t.Errorf("triage.tenant.name: got %v, want %q", attrs[AttrTenantName], "Acme")
	}
}

// ---------------------------------------------------------------------------
// traceContent control
// ---------------------------------------------------------------------------

func TestLogPrompt_TraceContentFalse_SkipsMessages(t *testing.T) {
	exporter := setGlobalTestProvider(t)

	// Simulate traceContent=false.
	mu.Lock()
	activeConfig = &config{traceContent: false}
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		activeConfig = nil
		mu.Unlock()
	})

	_, llmSpan := LogPrompt(context.Background(), PromptParams{
		Vendor: "openai",
		Model:  "gpt-4o",
		Messages: []Message{
			{Role: "user", Content: "secret prompt"},
		},
		Tools: []ToolDefinition{
			{Name: "secret_tool", Description: "does secret things"},
		},
	})
	llmSpan.LogCompletion(CompletionParams{
		Messages: []Message{
			{Role: "assistant", Content: "secret response"},
		},
		Usage: Usage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15},
	})

	attrs := attrMap(exporter.GetSpans()[0].Attributes)

	// Model/usage attributes should still be present.
	if attrs[AttrGenAISystem] != "openai" {
		t.Errorf("gen_ai.system should be present")
	}
	if attrs[AttrGenAIUsageInputTokens] != int64(10) {
		t.Errorf("input_tokens should be present")
	}

	// Content attributes should NOT be present.
	if _, ok := attrs["gen_ai.prompt.0.content"]; ok {
		t.Error("prompt content should not be captured when traceContent=false")
	}
	if _, ok := attrs["gen_ai.prompt.0.role"]; ok {
		t.Error("prompt role should not be captured when traceContent=false")
	}
	if _, ok := attrs["gen_ai.completion.0.content"]; ok {
		t.Error("completion content should not be captured when traceContent=false")
	}
	if _, ok := attrs["gen_ai.request.tools.0.name"]; ok {
		t.Error("tool definitions should not be captured when traceContent=false")
	}
}

// ---------------------------------------------------------------------------
// LLMSpan.End and LLMSpan.SetError
// ---------------------------------------------------------------------------

func TestLLMSpan_End_WithoutCompletion(t *testing.T) {
	exporter := setGlobalTestProvider(t)

	_, llmSpan := LogPrompt(context.Background(), PromptParams{
		Vendor: "openai",
		Model:  "gpt-4o",
	})
	llmSpan.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	// Span should exist with request attrs but no completion attrs.
	attrs := attrMap(spans[0].Attributes)
	if attrs[AttrGenAISystem] != "openai" {
		t.Errorf("gen_ai.system: got %v, want %q", attrs[AttrGenAISystem], "openai")
	}
	if _, ok := attrs[AttrGenAIResponseModel]; ok {
		t.Error("response model should not be present after End() without completion")
	}
}

func TestLLMSpan_SetError(t *testing.T) {
	exporter := setGlobalTestProvider(t)

	_, llmSpan := LogPrompt(context.Background(), PromptParams{
		Vendor: "openai",
		Model:  "gpt-4o",
	})
	llmSpan.SetError(errors.New("API rate limited"))
	llmSpan.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	// Span should have error status. codes.Error = 1 in OTel Go SDK.
	if spans[0].Status.Code != 1 {
		t.Errorf("expected error status code 1, got %d", spans[0].Status.Code)
	}
	if spans[0].Status.Description != "API rate limited" {
		t.Errorf("status description: got %q, want %q", spans[0].Status.Description, "API rate limited")
	}
}

// ---------------------------------------------------------------------------
// Nil span safety
// ---------------------------------------------------------------------------

func TestLLMSpan_NilSpan_NoPanic(t *testing.T) {
	// A zero-value LLMSpan should not panic on any method.
	s := &LLMSpan{}
	s.End()                                 // should not panic
	s.LogCompletion(CompletionParams{})      // should not panic
	s.SetError(errors.New("test"))           // should not panic
}

// ---------------------------------------------------------------------------
// Context propagation — child spans nested under LLM span
// ---------------------------------------------------------------------------

func TestLogPrompt_ChildSpansNested(t *testing.T) {
	exporter := setGlobalTestProvider(t)

	ctx := context.Background()
	ctx, llmSpan := LogPrompt(ctx, PromptParams{
		Vendor: "openai",
		Model:  "gpt-4o",
	})

	// Create a child span (e.g., tool execution).
	tracer := otel.GetTracerProvider().Tracer("test")
	_, child := tracer.Start(ctx, "tool-execution")
	child.End()

	llmSpan.End()

	spans := exporter.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}

	// Child's parent should be the LLM span.
	llmSpanCtx := spans[1].SpanContext // LLM span (ended last)
	childParent := spans[0].Parent     // child span's parent
	if childParent.SpanID() != llmSpanCtx.SpanID() {
		t.Errorf("child parent span ID %s != LLM span ID %s",
			childParent.SpanID(), llmSpanCtx.SpanID())
	}
}

// ---------------------------------------------------------------------------
// Full E2E: prompt → completion with all features
// ---------------------------------------------------------------------------

func TestLogPrompt_FullE2EFlow(t *testing.T) {
	exporter := setGlobalTestProvider(t)

	// Set up triage context.
	ctx := WithUser(context.Background(), "u_42", UserRole("admin"))
	ctx = WithSession(ctx, "sess_1", TurnNumber(1))

	// Log prompt.
	temp := 0.7
	ctx, llmSpan := LogPrompt(ctx, PromptParams{
		Vendor:      "openai",
		Model:       "gpt-4o",
		Temperature: &temp,
		Messages: []Message{
			{Role: "system", Content: "You are a helpful assistant."},
			{Role: "user", Content: "What's the weather in SF?"},
		},
		Tools: []ToolDefinition{
			{Name: "get_weather", Description: "Get weather for a city"},
		},
	})

	// Log completion with tool call.
	llmSpan.LogCompletion(CompletionParams{
		Model: "gpt-4o-2024-08-06",
		Messages: []Message{
			{
				Role: "assistant",
				ToolCalls: []ToolCall{
					{
						ID:        "call_weather1",
						Type:      "function",
						Name:      "get_weather",
						Arguments: `{"city":"San Francisco"}`,
					},
				},
			},
		},
		Usage: Usage{
			InputTokens:  200,
			OutputTokens: 50,
			TotalTokens:  250,
		},
		FinishReason: "tool_calls",
	})

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	attrs := attrMap(spans[0].Attributes)

	// Layer 1: gen_ai attributes.
	if attrs[AttrGenAISystem] != "openai" {
		t.Errorf("gen_ai.system: got %v", attrs[AttrGenAISystem])
	}
	if attrs[AttrGenAIRequestModel] != "gpt-4o" {
		t.Errorf("gen_ai.request.model: got %v", attrs[AttrGenAIRequestModel])
	}
	if attrs[AttrGenAIResponseModel] != "gpt-4o-2024-08-06" {
		t.Errorf("gen_ai.response.model: got %v", attrs[AttrGenAIResponseModel])
	}
	if attrs[AttrGenAIRequestTemperature] != 0.7 {
		t.Errorf("gen_ai.request.temperature: got %v", attrs[AttrGenAIRequestTemperature])
	}
	if attrs[AttrGenAIUsageInputTokens] != int64(200) {
		t.Errorf("gen_ai.usage.input_tokens: got %v", attrs[AttrGenAIUsageInputTokens])
	}
	if attrs[AttrGenAIResponseFinishReason] != "tool_calls" {
		t.Errorf("gen_ai.response.finish_reason: got %v", attrs[AttrGenAIResponseFinishReason])
	}

	// Prompt messages.
	if attrs["gen_ai.prompt.0.role"] != "system" {
		t.Errorf("prompt.0.role: got %v", attrs["gen_ai.prompt.0.role"])
	}
	if attrs["gen_ai.prompt.1.content"] != "What's the weather in SF?" {
		t.Errorf("prompt.1.content: got %v", attrs["gen_ai.prompt.1.content"])
	}

	// Tool call in completion.
	if attrs["gen_ai.completion.0.tool_calls.0.name"] != "get_weather" {
		t.Errorf("tool_calls.0.name: got %v", attrs["gen_ai.completion.0.tool_calls.0.name"])
	}

	// Tool definitions.
	if attrs["gen_ai.request.tools.0.name"] != "get_weather" {
		t.Errorf("tools.0.name: got %v", attrs["gen_ai.request.tools.0.name"])
	}

	// Layer 2: triage attributes (injected by processor).
	if attrs[AttrUserID] != "u_42" {
		t.Errorf("triage.user.id: got %v", attrs[AttrUserID])
	}
	if attrs[AttrUserRole] != "admin" {
		t.Errorf("triage.user.role: got %v", attrs[AttrUserRole])
	}
	if attrs[AttrSessionID] != "sess_1" {
		t.Errorf("triage.session.id: got %v", attrs[AttrSessionID])
	}
	if attrs[AttrSessionTurn] != int64(1) {
		t.Errorf("triage.session.turn_number: got %v", attrs[AttrSessionTurn])
	}
}
