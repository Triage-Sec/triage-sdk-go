package triage

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// newGlobalTestProvider creates a TracerProvider, registers it as the global
// OTel provider (so LogPrompt/StartWorkflow etc. pick it up), and returns
// the in-memory exporter for assertions.
func newGlobalTestProvider(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(&triageSpanProcessor{}),
		sdktrace.WithSyncer(exporter),
	)
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		otel.SetTracerProvider(trace.NewNoopTracerProvider())
		globalCfg = nil
	})
	return exporter
}

// ---------------------------------------------------------------------------
// LogPrompt basics
// ---------------------------------------------------------------------------

func TestLogPrompt_CreatesSpanWithGenAiAttributes(t *testing.T) {
	exporter := newGlobalTestProvider(t)

	ctx := context.Background()
	llmSpan, _ := LogPrompt(ctx, Prompt{
		Vendor: "openai",
		Model:  "gpt-4o",
		Messages: []Message{
			{Role: "user", Content: "Hello, world!"},
		},
	})
	llmSpan.LogCompletion(Completion{
		Model: "gpt-4o",
		Messages: []Message{
			{Role: "assistant", Content: "Hi there!"},
		},
	}, Usage{
		PromptTokens:     10,
		CompletionTokens: 5,
		TotalTokens:      15,
	})

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	attrs := attrMap(spans[0].Attributes)

	// gen_ai.* attributes.
	if attrs["gen_ai.system"] != "openai" {
		t.Errorf("gen_ai.system: got %v, want %q", attrs["gen_ai.system"], "openai")
	}
	if attrs["gen_ai.request.model"] != "gpt-4o" {
		t.Errorf("gen_ai.request.model: got %v, want %q", attrs["gen_ai.request.model"], "gpt-4o")
	}
	if attrs["gen_ai.response.model"] != "gpt-4o" {
		t.Errorf("gen_ai.response.model: got %v, want %q", attrs["gen_ai.response.model"], "gpt-4o")
	}
	if attrs["gen_ai.usage.input_tokens"] != int64(10) {
		t.Errorf("gen_ai.usage.input_tokens: got %v, want %d", attrs["gen_ai.usage.input_tokens"], 10)
	}
	if attrs["gen_ai.usage.output_tokens"] != int64(5) {
		t.Errorf("gen_ai.usage.output_tokens: got %v, want %d", attrs["gen_ai.usage.output_tokens"], 5)
	}

	// llm.* backward-compat attributes.
	if attrs["llm.vendor"] != "openai" {
		t.Errorf("llm.vendor: got %v, want %q", attrs["llm.vendor"], "openai")
	}
	if attrs["llm.request.model"] != "gpt-4o" {
		t.Errorf("llm.request.model: got %v, want %q", attrs["llm.request.model"], "gpt-4o")
	}
	if attrs["llm.usage.total_tokens"] != int64(15) {
		t.Errorf("llm.usage.total_tokens: got %v, want %d", attrs["llm.usage.total_tokens"], 15)
	}
}

func TestLogPrompt_SpanName(t *testing.T) {
	exporter := newGlobalTestProvider(t)

	llmSpan, _ := LogPrompt(context.Background(), Prompt{
		Vendor:   "anthropic",
		Model:    "claude-sonnet-4-5-20250929",
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	llmSpan.LogCompletion(Completion{Model: "claude-sonnet-4-5-20250929"}, Usage{})

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	want := "anthropic.chat claude-sonnet-4-5-20250929"
	if spans[0].Name != want {
		t.Errorf("span name: got %q, want %q", spans[0].Name, want)
	}
}

// ---------------------------------------------------------------------------
// Prompt content recording
// ---------------------------------------------------------------------------

func TestLogPrompt_RecordsPromptAndCompletionContent(t *testing.T) {
	exporter := newGlobalTestProvider(t)

	llmSpan, _ := LogPrompt(context.Background(), Prompt{
		Vendor: "openai",
		Model:  "gpt-4o",
		Messages: []Message{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "What is 2+2?"},
		},
	})
	llmSpan.LogCompletion(Completion{
		Model: "gpt-4o",
		Messages: []Message{
			{Role: "assistant", Content: "2+2 is 4."},
		},
	}, Usage{PromptTokens: 20, CompletionTokens: 10, TotalTokens: 30})

	attrs := attrMap(exporter.GetSpans()[0].Attributes)

	// Prompt messages.
	if attrs["gen_ai.prompt.0.role"] != "system" {
		t.Errorf("prompt.0.role: got %v, want %q", attrs["gen_ai.prompt.0.role"], "system")
	}
	if attrs["gen_ai.prompt.0.content"] != "You are helpful." {
		t.Errorf("prompt.0.content: got %v", attrs["gen_ai.prompt.0.content"])
	}
	if attrs["gen_ai.prompt.1.role"] != "user" {
		t.Errorf("prompt.1.role: got %v, want %q", attrs["gen_ai.prompt.1.role"], "user")
	}
	if attrs["gen_ai.prompt.1.content"] != "What is 2+2?" {
		t.Errorf("prompt.1.content: got %v", attrs["gen_ai.prompt.1.content"])
	}

	// Completion messages.
	if attrs["gen_ai.completion.0.role"] != "assistant" {
		t.Errorf("completion.0.role: got %v, want %q", attrs["gen_ai.completion.0.role"], "assistant")
	}
	if attrs["gen_ai.completion.0.content"] != "2+2 is 4." {
		t.Errorf("completion.0.content: got %v", attrs["gen_ai.completion.0.content"])
	}
}

func TestLogPrompt_TraceContentDisabled_OmitsContent(t *testing.T) {
	exporter := newGlobalTestProvider(t)

	// Simulate traceContent=false via globalCfg.
	globalCfg = &config{traceContent: false}

	llmSpan, _ := LogPrompt(context.Background(), Prompt{
		Vendor: "openai",
		Model:  "gpt-4o",
		Messages: []Message{
			{Role: "user", Content: "secret prompt"},
		},
	})
	llmSpan.LogCompletion(Completion{
		Model:    "gpt-4o",
		Messages: []Message{{Role: "assistant", Content: "secret response"}},
	}, Usage{PromptTokens: 5, CompletionTokens: 5, TotalTokens: 10})

	attrs := attrMap(exporter.GetSpans()[0].Attributes)

	// Model and usage should still be present.
	if attrs["gen_ai.system"] != "openai" {
		t.Errorf("gen_ai.system should be present")
	}
	if attrs["gen_ai.usage.input_tokens"] != int64(5) {
		t.Errorf("gen_ai.usage.input_tokens should be present")
	}

	// Content should NOT be present.
	if _, ok := attrs["gen_ai.prompt.0.content"]; ok {
		t.Error("prompt content should not be recorded when traceContent is false")
	}
	if _, ok := attrs["gen_ai.completion.0.content"]; ok {
		t.Error("completion content should not be recorded when traceContent is false")
	}
	// Roles should also not be present (entire prompt/completion recording is skipped).
	if _, ok := attrs["gen_ai.prompt.0.role"]; ok {
		t.Error("prompt role should not be recorded when traceContent is false")
	}
}

// ---------------------------------------------------------------------------
// Tool calls and tool definitions
// ---------------------------------------------------------------------------

func TestLogPrompt_RecordsToolDefinitions(t *testing.T) {
	exporter := newGlobalTestProvider(t)

	llmSpan, _ := LogPrompt(context.Background(), Prompt{
		Vendor:   "openai",
		Model:    "gpt-4o",
		Messages: []Message{{Role: "user", Content: "What's the weather?"}},
		Tools: []ToolDef{
			{
				Type: "function",
				Function: ToolFunction{
					Name:        "get_weather",
					Description: "Get current weather",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"location": map[string]any{"type": "string"},
						},
					},
				},
			},
		},
	})
	llmSpan.LogCompletion(Completion{Model: "gpt-4o"}, Usage{})

	attrs := attrMap(exporter.GetSpans()[0].Attributes)

	if attrs["gen_ai.request.tool.0.type"] != "function" {
		t.Errorf("tool.0.type: got %v", attrs["gen_ai.request.tool.0.type"])
	}
	if attrs["gen_ai.request.tool.0.function.name"] != "get_weather" {
		t.Errorf("tool.0.function.name: got %v", attrs["gen_ai.request.tool.0.function.name"])
	}
	if attrs["gen_ai.request.tool.0.function.description"] != "Get current weather" {
		t.Errorf("tool.0.function.description: got %v", attrs["gen_ai.request.tool.0.function.description"])
	}
	// Parameters should be JSON-serialized.
	if _, ok := attrs["gen_ai.request.tool.0.function.parameters"]; !ok {
		t.Error("tool parameters should be recorded")
	}
}

func TestLogPrompt_RecordsToolCallsInCompletion(t *testing.T) {
	exporter := newGlobalTestProvider(t)

	llmSpan, _ := LogPrompt(context.Background(), Prompt{
		Vendor:   "openai",
		Model:    "gpt-4o",
		Messages: []Message{{Role: "user", Content: "What's the weather in SF?"}},
	})
	llmSpan.LogCompletion(Completion{
		Model: "gpt-4o",
		Messages: []Message{
			{
				Role: "assistant",
				ToolCalls: []ToolCall{
					{
						ID:   "call_123",
						Type: "function",
						Function: ToolCallFunction{
							Name:      "get_weather",
							Arguments: `{"location": "San Francisco"}`,
						},
					},
				},
			},
		},
	}, Usage{PromptTokens: 20, CompletionTokens: 15, TotalTokens: 35})

	attrs := attrMap(exporter.GetSpans()[0].Attributes)

	if attrs["gen_ai.completion.0.tool_calls.0.id"] != "call_123" {
		t.Errorf("tool_call.0.id: got %v", attrs["gen_ai.completion.0.tool_calls.0.id"])
	}
	if attrs["gen_ai.completion.0.tool_calls.0.type"] != "function" {
		t.Errorf("tool_call.0.type: got %v", attrs["gen_ai.completion.0.tool_calls.0.type"])
	}
	if attrs["gen_ai.completion.0.tool_calls.0.function.name"] != "get_weather" {
		t.Errorf("tool_call.0.function.name: got %v", attrs["gen_ai.completion.0.tool_calls.0.function.name"])
	}
	if attrs["gen_ai.completion.0.tool_calls.0.function.arguments"] != `{"location": "San Francisco"}` {
		t.Errorf("tool_call.0.function.arguments: got %v", attrs["gen_ai.completion.0.tool_calls.0.function.arguments"])
	}
}

func TestLogPrompt_RecordsToolCallsInPrompt(t *testing.T) {
	exporter := newGlobalTestProvider(t)

	// Simulate a multi-turn conversation with tool calls in a prior assistant message.
	llmSpan, _ := LogPrompt(context.Background(), Prompt{
		Vendor: "openai",
		Model:  "gpt-4o",
		Messages: []Message{
			{Role: "user", Content: "What's the weather?"},
			{
				Role: "assistant",
				ToolCalls: []ToolCall{
					{ID: "call_1", Type: "function", Function: ToolCallFunction{
						Name: "get_weather", Arguments: `{"location":"NYC"}`,
					}},
				},
			},
			{Role: "tool", Content: `{"temp": 72}`, ToolCallID: "call_1"},
		},
	})
	llmSpan.LogCompletion(Completion{
		Model:    "gpt-4o",
		Messages: []Message{{Role: "assistant", Content: "It's 72°F in NYC."}},
	}, Usage{})

	attrs := attrMap(exporter.GetSpans()[0].Attributes)

	// Assistant message tool calls.
	if attrs["gen_ai.prompt.1.tool_calls.0.id"] != "call_1" {
		t.Errorf("prompt.1.tool_calls.0.id: got %v", attrs["gen_ai.prompt.1.tool_calls.0.id"])
	}
	if attrs["gen_ai.prompt.1.tool_calls.0.function.name"] != "get_weather" {
		t.Errorf("prompt.1.tool_calls.0.function.name: got %v", attrs["gen_ai.prompt.1.tool_calls.0.function.name"])
	}

	// Tool result message.
	if attrs["gen_ai.prompt.2.role"] != "tool" {
		t.Errorf("prompt.2.role: got %v", attrs["gen_ai.prompt.2.role"])
	}
	if attrs["gen_ai.prompt.2.tool_call_id"] != "call_1" {
		t.Errorf("prompt.2.tool_call_id: got %v", attrs["gen_ai.prompt.2.tool_call_id"])
	}
}

// ---------------------------------------------------------------------------
// Optional request parameters
// ---------------------------------------------------------------------------

func TestLogPrompt_RecordsOptionalParams(t *testing.T) {
	exporter := newGlobalTestProvider(t)

	temp := 0.7
	topP := 0.9
	freqP := 0.5
	presP := 0.3

	llmSpan, _ := LogPrompt(context.Background(), Prompt{
		Vendor:           "openai",
		Model:            "gpt-4o",
		Messages:         []Message{{Role: "user", Content: "Hi"}},
		MaxTokens:        1024,
		Temperature:      &temp,
		TopP:             &topP,
		FrequencyPenalty: &freqP,
		PresencePenalty:  &presP,
		Stop:             []string{"END", "STOP"},
	})
	llmSpan.LogCompletion(Completion{Model: "gpt-4o"}, Usage{})

	attrs := attrMap(exporter.GetSpans()[0].Attributes)

	if attrs["gen_ai.request.max_tokens"] != int64(1024) {
		t.Errorf("max_tokens: got %v", attrs["gen_ai.request.max_tokens"])
	}
	if attrs["gen_ai.request.temperature"] != 0.7 {
		t.Errorf("temperature: got %v", attrs["gen_ai.request.temperature"])
	}
	if attrs["gen_ai.request.top_p"] != 0.9 {
		t.Errorf("top_p: got %v", attrs["gen_ai.request.top_p"])
	}
	if attrs["gen_ai.request.frequency_penalty"] != 0.5 {
		t.Errorf("frequency_penalty: got %v", attrs["gen_ai.request.frequency_penalty"])
	}
	if attrs["gen_ai.request.presence_penalty"] != 0.3 {
		t.Errorf("presence_penalty: got %v", attrs["gen_ai.request.presence_penalty"])
	}
}

// ---------------------------------------------------------------------------
// Integration with triage context
// ---------------------------------------------------------------------------

func TestLogPrompt_TriageContextOnLLMSpan(t *testing.T) {
	exporter := newGlobalTestProvider(t)

	ctx := context.Background()
	ctx = WithUser(ctx, "u_42", UserRole("admin"))
	ctx = WithTenant(ctx, "org_7")

	llmSpan, _ := LogPrompt(ctx, Prompt{
		Vendor:   "openai",
		Model:    "gpt-4o",
		Messages: []Message{{Role: "user", Content: "Hello"}},
	})
	llmSpan.LogCompletion(Completion{
		Model:    "gpt-4o",
		Messages: []Message{{Role: "assistant", Content: "Hi!"}},
	}, Usage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8})

	attrs := attrMap(exporter.GetSpans()[0].Attributes)

	// Triage context attributes should be present (injected by triageSpanProcessor).
	if attrs[AttrUserID] != "u_42" {
		t.Errorf("triage.user.id: got %v, want %q", attrs[AttrUserID], "u_42")
	}
	if attrs[AttrUserRole] != "admin" {
		t.Errorf("triage.user.role: got %v, want %q", attrs[AttrUserRole], "admin")
	}
	if attrs[AttrTenantID] != "org_7" {
		t.Errorf("triage.tenant.id: got %v, want %q", attrs[AttrTenantID], "org_7")
	}

	// gen_ai attributes should also be present.
	if attrs["gen_ai.system"] != "openai" {
		t.Errorf("gen_ai.system: got %v", attrs["gen_ai.system"])
	}
	if attrs["gen_ai.usage.input_tokens"] != int64(5) {
		t.Errorf("gen_ai.usage.input_tokens: got %v", attrs["gen_ai.usage.input_tokens"])
	}
}

// ---------------------------------------------------------------------------
// Nil safety
// ---------------------------------------------------------------------------

func TestLogCompletion_NilSpanIsNoop(t *testing.T) {
	// Should not panic.
	var ls *LLMSpan
	ls.LogCompletion(Completion{Model: "gpt-4o"}, Usage{})
}

func TestLLMSpan_NilContext(t *testing.T) {
	var ls *LLMSpan
	ctx := ls.Context()
	if ctx == nil {
		t.Error("Context() on nil LLMSpan should return non-nil context")
	}
}
