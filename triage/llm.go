package triage

import (
	"context"
	"encoding/json"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const llmTracerName = "triage.llm"

// ---------------------------------------------------------------------------
// Types — mirror go-openllmetry and OpenTelemetry GenAI semantic conventions
// ---------------------------------------------------------------------------

// Prompt represents an LLM request with messages and optional parameters.
type Prompt struct {
	Vendor   string    // LLM provider: "openai", "anthropic", etc.
	Model    string    // Model name: "gpt-4o", "claude-sonnet-4-5-20250929", etc.
	Messages []Message // Conversation messages
	Tools    []ToolDef // Available tool/function definitions

	// Optional request parameters.
	MaxTokens        int
	Temperature      *float64
	TopP             *float64
	FrequencyPenalty *float64
	PresencePenalty  *float64
	Stop             []string
}

// Message represents a single message in an LLM conversation.
type Message struct {
	Role       string     // "system", "user", "assistant", "tool"
	Content    string     // Message text content
	ToolCalls  []ToolCall // Tool calls in assistant messages
	ToolCallID string     // Tool call ID in tool-result messages
}

// ToolCall represents a tool/function call made by the model.
type ToolCall struct {
	ID       string           // Unique call ID
	Type     string           // "function"
	Function ToolCallFunction // Function name and arguments
}

// ToolCallFunction holds the function name and JSON-encoded arguments.
type ToolCallFunction struct {
	Name      string // Function name
	Arguments string // JSON-encoded arguments
}

// ToolDef defines a tool available to the model.
type ToolDef struct {
	Type     string       // "function"
	Function ToolFunction // Function definition
}

// ToolFunction describes a callable function for tool use.
type ToolFunction struct {
	Name        string // Function name
	Description string // Human-readable description
	Parameters  any    // JSON Schema object for the function parameters
}

// Completion represents an LLM response.
type Completion struct {
	Model    string    // Model that generated the response
	Messages []Message // Response messages
}

// Usage represents token counts for an LLM call.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// LLMSpan wraps an in-flight LLM call span. Call LogCompletion to record the
// response and end the span.
type LLMSpan struct {
	span trace.Span
	ctx  context.Context
}

// Context returns the context carrying this LLM span, suitable for creating
// child spans (e.g. tool execution spans nested under an LLM call).
func (ls *LLMSpan) Context() context.Context {
	if ls == nil {
		return context.Background()
	}
	return ls.ctx
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// LogPrompt starts a new span for an LLM call and records request attributes
// following both OpenTelemetry GenAI (gen_ai.*) and OpenLLMetry (llm.*)
// semantic conventions.
//
// Returns an LLMSpan that must be completed by calling LogCompletion after the
// LLM response is received:
//
//	llmSpan, ctx := triage.LogPrompt(ctx, triage.Prompt{
//	    Vendor:   "openai",
//	    Model:    "gpt-4o",
//	    Messages: []triage.Message{{Role: "user", Content: "Hello"}},
//	})
//	// ... make your LLM API call using ctx ...
//	llmSpan.LogCompletion(triage.Completion{...}, triage.Usage{...})
func LogPrompt(ctx context.Context, prompt Prompt) (*LLMSpan, context.Context) {
	tracer := otel.GetTracerProvider().Tracer(llmTracerName)

	spanName := prompt.Vendor + ".chat"
	if prompt.Model != "" {
		spanName = prompt.Vendor + ".chat " + prompt.Model
	}

	ctx, span := tracer.Start(ctx, spanName, trace.WithSpanKind(trace.SpanKindClient))

	var attrs []attribute.KeyValue

	// gen_ai.* — OpenTelemetry GenAI semantic conventions (primary).
	attrs = append(attrs,
		attribute.String("gen_ai.system", prompt.Vendor),
		attribute.String("gen_ai.request.model", prompt.Model),
	)

	// llm.* — OpenLLMetry / go-openllmetry conventions (backward compat).
	attrs = append(attrs,
		attribute.String("llm.vendor", prompt.Vendor),
		attribute.String("llm.request.model", prompt.Model),
		attribute.String("llm.request.type", "chat"),
	)

	// Optional request parameters.
	if prompt.MaxTokens > 0 {
		attrs = append(attrs, attribute.Int("gen_ai.request.max_tokens", prompt.MaxTokens))
	}
	if prompt.Temperature != nil {
		attrs = append(attrs, attribute.Float64("gen_ai.request.temperature", *prompt.Temperature))
	}
	if prompt.TopP != nil {
		attrs = append(attrs, attribute.Float64("gen_ai.request.top_p", *prompt.TopP))
	}
	if prompt.FrequencyPenalty != nil {
		attrs = append(attrs, attribute.Float64("gen_ai.request.frequency_penalty", *prompt.FrequencyPenalty))
	}
	if prompt.PresencePenalty != nil {
		attrs = append(attrs, attribute.Float64("gen_ai.request.presence_penalty", *prompt.PresencePenalty))
	}
	if len(prompt.Stop) > 0 {
		attrs = append(attrs, attribute.StringSlice("gen_ai.request.stop_sequences", prompt.Stop))
	}

	// Prompt messages — only when trace content is enabled.
	if isTraceContentEnabled() {
		for i, msg := range prompt.Messages {
			prefix := fmt.Sprintf("gen_ai.prompt.%d", i)
			attrs = append(attrs, attribute.String(prefix+".role", msg.Role))
			if msg.Content != "" {
				attrs = append(attrs, attribute.String(prefix+".content", msg.Content))
			}
			for j, tc := range msg.ToolCalls {
				tcPrefix := fmt.Sprintf("%s.tool_calls.%d", prefix, j)
				attrs = append(attrs,
					attribute.String(tcPrefix+".id", tc.ID),
					attribute.String(tcPrefix+".type", tc.Type),
					attribute.String(tcPrefix+".function.name", tc.Function.Name),
					attribute.String(tcPrefix+".function.arguments", tc.Function.Arguments),
				)
			}
			if msg.ToolCallID != "" {
				attrs = append(attrs, attribute.String(prefix+".tool_call_id", msg.ToolCallID))
			}
		}
	}

	// Tool definitions — always recorded (these are schema, not content).
	for i, tool := range prompt.Tools {
		prefix := fmt.Sprintf("gen_ai.request.tool.%d", i)
		attrs = append(attrs, attribute.String(prefix+".type", tool.Type))
		attrs = append(attrs, attribute.String(prefix+".function.name", tool.Function.Name))
		if tool.Function.Description != "" {
			attrs = append(attrs, attribute.String(prefix+".function.description", tool.Function.Description))
		}
		if tool.Function.Parameters != nil {
			if paramJSON, err := json.Marshal(tool.Function.Parameters); err == nil {
				attrs = append(attrs, attribute.String(prefix+".function.parameters", string(paramJSON)))
			}
		}
	}

	span.SetAttributes(attrs...)
	return &LLMSpan{span: span, ctx: ctx}, ctx
}

// LogCompletion records the LLM response and token usage, then ends the span.
// Safe to call on a nil LLMSpan (no-op).
func (ls *LLMSpan) LogCompletion(completion Completion, usage Usage) {
	if ls == nil || ls.span == nil {
		return
	}

	var attrs []attribute.KeyValue

	// Response model.
	if completion.Model != "" {
		attrs = append(attrs,
			attribute.String("gen_ai.response.model", completion.Model),
			attribute.String("llm.response.model", completion.Model),
		)
	}

	// Token usage — gen_ai.* conventions.
	attrs = append(attrs,
		attribute.Int("gen_ai.usage.input_tokens", usage.PromptTokens),
		attribute.Int("gen_ai.usage.output_tokens", usage.CompletionTokens),
	)

	// Token usage — llm.* conventions (backward compat).
	attrs = append(attrs,
		attribute.Int("llm.usage.prompt_tokens", usage.PromptTokens),
		attribute.Int("llm.usage.completion_tokens", usage.CompletionTokens),
		attribute.Int("llm.usage.total_tokens", usage.TotalTokens),
	)

	// Completion messages — only when trace content is enabled.
	if isTraceContentEnabled() {
		for i, msg := range completion.Messages {
			prefix := fmt.Sprintf("gen_ai.completion.%d", i)
			attrs = append(attrs, attribute.String(prefix+".role", msg.Role))
			if msg.Content != "" {
				attrs = append(attrs, attribute.String(prefix+".content", msg.Content))
			}
			for j, tc := range msg.ToolCalls {
				tcPrefix := fmt.Sprintf("%s.tool_calls.%d", prefix, j)
				attrs = append(attrs,
					attribute.String(tcPrefix+".id", tc.ID),
					attribute.String(tcPrefix+".type", tc.Type),
					attribute.String(tcPrefix+".function.name", tc.Function.Name),
					attribute.String(tcPrefix+".function.arguments", tc.Function.Arguments),
				)
			}
		}
	}

	ls.span.SetAttributes(attrs...)
	ls.span.End()
}

// isTraceContentEnabled returns whether prompt/completion content should be
// captured. Defaults to true if the SDK hasn't been initialized yet.
func isTraceContentEnabled() bool {
	if globalCfg == nil {
		return true
	}
	return globalCfg.traceContent
}
