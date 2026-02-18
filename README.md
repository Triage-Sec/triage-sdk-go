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

    // 3. Pass ctx to your LLM calls — spans created with this context
    //    will automatically include all triage.* attributes
    _ = ctx // use ctx with your OpenAI/Anthropic client
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
