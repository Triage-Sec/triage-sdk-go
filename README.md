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

    // 2. Annotate context before LLM calls
    ctx := context.Background()
    ctx = triage.WithUser(ctx, "u_123", triage.UserRole("admin"))
    ctx = triage.WithTenant(ctx, "org_456")
    ctx = triage.WithSession(ctx, "sess_789", triage.TurnNumber(1))
    ctx = triage.WithInput(ctx, "Explain prompt injection")

    // 3. Instrument LLM calls with LogPrompt / LogCompletion
    llmSpan, ctx := triage.LogPrompt(ctx, triage.Prompt{
        Vendor:   "openai",
        Model:    "gpt-4o",
        Messages: []triage.Message{{Role: "user", Content: "Explain prompt injection"}},
    })

    // ... make your LLM API call here ...

    llmSpan.LogCompletion(triage.Completion{
        Model:    "gpt-4o",
        Messages: []triage.Message{{Role: "assistant", Content: "Prompt injection is..."}},
    }, triage.Usage{
        PromptTokens:     25,
        CompletionTokens: 100,
        TotalTokens:      125,
    })
    _ = ctx
}
```

## Context Helpers

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

## LLM Call Instrumentation

Wrap your LLM API calls with `LogPrompt` / `LogCompletion` to capture prompts, completions, tool calls, and token usage. Spans automatically include any triage context from the `ctx`:

```go
llmSpan, ctx := triage.LogPrompt(ctx, triage.Prompt{
    Vendor:   "openai",
    Model:    "gpt-4o",
    Messages: []triage.Message{
        {Role: "system", Content: "You are a helpful assistant."},
        {Role: "user", Content: userInput},
    },
    Tools: []triage.ToolDef{{
        Type: "function",
        Function: triage.ToolFunction{
            Name:        "search_docs",
            Description: "Search documentation",
            Parameters:  map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}},
        },
    }},
})

// Make your LLM API call...
resp, err := openaiClient.CreateChatCompletion(ctx, req)

llmSpan.LogCompletion(triage.Completion{
    Model: resp.Model,
    Messages: []triage.Message{{
        Role:    "assistant",
        Content: resp.Choices[0].Message.Content,
        ToolCalls: []triage.ToolCall{{
            ID:   resp.Choices[0].Message.ToolCalls[0].ID,
            Type: "function",
            Function: triage.ToolCallFunction{
                Name:      resp.Choices[0].Message.ToolCalls[0].Function.Name,
                Arguments: resp.Choices[0].Message.ToolCalls[0].Function.Arguments,
            },
        }},
    }},
}, triage.Usage{
    PromptTokens:     resp.Usage.PromptTokens,
    CompletionTokens: resp.Usage.CompletionTokens,
    TotalTokens:      resp.Usage.TotalTokens,
})
```

Sets both `gen_ai.*` (OTel standard) and `llm.*` (OpenLLMetry compat) span attributes. When `WithTraceContent(false)` is set, prompt/completion content is omitted while metadata (model, tokens) is still captured.

## Workflow Hierarchy

Organize traces into workflows, tasks, agents, and tools — matching the OpenLLMetry/Traceloop span hierarchy:

```go
// Top-level workflow
wf, ctx := triage.StartWorkflow(ctx, "chat-pipeline")
defer wf.End()

// Agent within the workflow
agent, ctx := triage.StartAgent(ctx, "research-agent")
defer agent.End()

// Tool execution
tool, toolCtx := triage.StartTool(ctx, "search-knowledge-base")
// ... execute tool ...
tool.End()

// LLM call — automatically nested under the agent span
llmSpan, ctx := triage.LogPrompt(ctx, prompt)
llmSpan.LogCompletion(completion, usage)
```

All hierarchy spans automatically inherit the workflow name via context propagation and set `traceloop.span.kind`, `traceloop.entity.name`, and `traceloop.workflow.name` attributes.

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

- Go 1.21+

## License

Apache 2.0 — see [LICENSE](LICENSE) for details.
