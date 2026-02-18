# Triage Go SDK

Security-focused observability for AI agents. Captures telemetry from LLM-powered applications and sends traces to the [Triage](https://triage-sec.com) backend for security analysis.

## Installation

```bash
go get github.com/Triage-Sec/triage-sdk-go/triage
```

## Quick Start

```go
package main

import (
    "context"
    "log"

    "github.com/Triage-Sec/triage-sdk-go/triage"
)

func main() {
    // 1. Initialize once at app startup
    shutdown, err := triage.Init(
        triage.WithAPIKey("tsk_..."),
        triage.WithAppName("my-chatbot"),
        triage.WithEnvironment("production"),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer shutdown()

    // 2. Annotate context with application metadata
    ctx := context.Background()
    ctx = triage.WithUser(ctx, "u_123", triage.UserRole("admin"))
    ctx = triage.WithTenant(ctx, "org_456")
    ctx = triage.WithSession(ctx, "sess_789", triage.TurnNumber(1))
    ctx = triage.WithInput(ctx, "What's the weather in SF?")

    // 3. Log LLM calls with full observability
    ctx, llmSpan := triage.LogPrompt(ctx, triage.PromptParams{
        Vendor: "openai",
        Model:  "gpt-4o",
        Messages: []triage.Message{
            {Role: "system", Content: "You are a helpful assistant."},
            {Role: "user", Content: "What's the weather in SF?"},
        },
    })

    // ... make your LLM API call here ...

    llmSpan.LogCompletion(triage.CompletionParams{
        Model: "gpt-4o-2024-08-06",
        Messages: []triage.Message{
            {Role: "assistant", Content: "I'd be happy to help..."},
        },
        Usage: triage.Usage{
            InputTokens:  150,
            OutputTokens: 42,
            TotalTokens:  192,
        },
        FinishReason: "stop",
    })
}
```

## LLM Instrumentation (Layer 1)

Log LLM calls to capture prompts, completions, tool calls, token usage, and reasoning tokens. Spans automatically include all `triage.*` context attributes set via the helpers below.

```go
// Start an LLM span — sets gen_ai.* request attributes
ctx, llmSpan := triage.LogPrompt(ctx, triage.PromptParams{
    Vendor:      "openai",
    Model:       "gpt-4o",
    Messages:    messages,
    Tools:       toolDefinitions,   // optional: available tools
    Temperature: &temp,             // optional: sampling params
})

// Complete the span — sets gen_ai.* response attributes and ends it
llmSpan.LogCompletion(triage.CompletionParams{
    Model:        "gpt-4o-2024-08-06",
    Messages:     completionMessages,
    Usage:        triage.Usage{
        InputTokens:     150,
        OutputTokens:    42,
        TotalTokens:     192,
        ReasoningTokens: 30,         // chain of thought (o1, Claude extended thinking)
        CacheReadTokens: 100,        // prompt caching
    },
    FinishReason: "stop",            // or "tool_calls", "length"
})

// On error — record and end without completion
llmSpan.SetError(err)
llmSpan.End()
```

### Tool Calls

Tool calls in completions are automatically captured as span attributes:

```go
llmSpan.LogCompletion(triage.CompletionParams{
    Messages: []triage.Message{
        {
            Role: "assistant",
            ToolCalls: []triage.ToolCall{
                {
                    ID:        "call_abc123",
                    Type:      "function",
                    Name:      "get_weather",
                    Arguments: `{"city":"San Francisco"}`,
                },
            },
        },
    },
    FinishReason: "tool_calls",
    Usage: triage.Usage{InputTokens: 50, OutputTokens: 20, TotalTokens: 70},
})
```

### Content Privacy

Set `WithTraceContent(false)` to disable capturing prompt/completion content while still recording model, usage, and metadata:

```go
triage.Init(
    triage.WithAPIKey("tsk_..."),
    triage.WithTraceContent(false), // prompts & completions not captured
)
```

## Context Helpers (Layer 2)

Six annotation helpers attach application-level metadata to all spans:

| Helper | Required Param | Optional Params |
|--------|---------------|-----------------|
| `triage.WithUser(ctx, userID)` | `userID` | `triage.UserRole(role)` |
| `triage.WithTenant(ctx, tenantID)` | `tenantID` | `triage.TenantName(name)` |
| `triage.WithSession(ctx, sessionID)` | `sessionID` | `triage.TurnNumber(n)`, `triage.HistoryHash(h)` |
| `triage.WithInput(ctx, raw)` | `raw` | `triage.Sanitized(s)` |
| `triage.WithTemplate(ctx, templateID)` | `templateID` | `triage.TemplateVersion(v)` |
| `triage.WithChunkACLs(ctx, acls)` | `acls` | — |

Each helper returns a new `context.Context` — contexts are immutable in Go.

## Configuration

Configuration follows **explicit option > environment variable > default** precedence:

| Option | Env Var | Default |
|--------|---------|---------|
| `WithAPIKey(key)` | `TRIAGE_API_KEY` | — (required) |
| `WithEndpoint(url)` | `TRIAGE_ENDPOINT` | `https://api.triageai.dev` |
| `WithAppName(name)` | `TRIAGE_APP_NAME` | `os.Args[0]` basename |
| `WithEnvironment(env)` | `TRIAGE_ENVIRONMENT` | `development` |
| `WithEnabled(bool)` | `TRIAGE_ENABLED` | `true` |
| `WithTraceContent(bool)` | `TRIAGE_TRACE_CONTENT` | `true` |

## Requirements

- Go 1.22+

## License

Apache 2.0 — see [LICENSE](LICENSE) for details.
