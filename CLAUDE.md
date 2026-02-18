# SDK — Design & Implementation Reference

## Overview

The Triage SDK captures telemetry from AI agents to enable security-focused observability. The SDK's job is **data collection only** — all heavy analysis (embeddings, PII detection, drift scoring, groundedness) happens later on backend workers. The SDK must never add meaningful latency to the customer's application.

## Architecture

### The SDK Has Two Layers

**Layer 1: OpenTelemetry Instrumentation**

Go does not have the same monkey-patching auto-instrumentation that Python and TypeScript get from OpenLLMetry/Traceloop. In Go, instrumentation is explicit — via middleware, interceptors, and wrapper clients. The OTel Go ecosystem provides `otelhttp`, `otelgrpc`, and similar middleware packages. For LLM providers, we provide thin instrumented wrapper functions that create spans around LLM API calls and extract `gen_ai.*` attributes (model, tokens, messages, tool calls). Users opt into instrumentation by wrapping their HTTP clients or using our provider-specific helpers.

**Layer 2: Triage context helpers (developer annotations)**

Six data points that live in the application layer above the LLM library boundary. Instrumentation cannot see these, so the developer must pass them in via our helper functions. In Go, these flow through `context.Context` — the idiomatic mechanism for request-scoped values.

The SDK is: OTel setup + a standard OTLP exporter pointed at our backend + 6 context helper functions + optional LLM provider wrappers.

## What Instrumentation Auto-Captures (via Provider Wrappers)

When using our instrumented wrappers (e.g., `triageopenai.Wrap(client)`), spans capture:

- **System Prompt** — From the messages array sent to the LLM client.
- **Chain-of-Thought / Hidden Thoughts** — From response (extended thinking, reasoning tokens).
- **Confidence/Uncertainty Scores** — From logprobs or model response metadata.
- **Step Count per Request** — From tool-use rounds in an agent loop.
- **Pre-Execution Tool Arguments** — The JSON payload before each tool call.
- **Tool Execution Status & Error Codes** — Return status from tool execution.
- **Tool Output (Post-Execution)** — Data returned by tools.
- **Model/Infrastructure Metadata** — Model name, provider, token usage, latency.

## What We Build: Developer Annotation Helpers

Everything above the LLM library boundary — auth, tenancy, session management, input filtering, prompt templating — is invisible to instrumentation. We provide helpers for these 6 data points:

1. **User ID & Role** — Lives in the app's auth middleware/JWT/session. The LLM SDK has no concept of users.
2. **Organization/Tenant ID** — Multi-tenancy is an app concern. The LLM API doesn't know about tenants.
3. **Session/Conversation History Hash** — Instrumentation sees the messages array but doesn't know which session or turn number this is.
4. **Raw vs. Sanitized User Input** — The app filters input before the LLM call. Instrumentation only sees the sanitized version.
5. **Template Version ID** — The app renders a template into a string. Instrumentation sees the string, not which template produced it.
6. **Retrieved Chunk ACLs** — Instrumentation sees retrieved chunks but not their access control metadata from the app's data layer.

These helpers attach the data as custom OTel span attributes via `context.Context`, so they flow through the same export pipeline as everything else.

## What the Backend Computes Later (NOT in the SDK)

Derived metrics computed asynchronously after trace ingestion. Do not build these into the SDK:

- Latent embeddings of prompts
- PII/sensitive entity flags
- Retrieval groundedness score
- Instruction drift score
- Prompt injection detection
- Cross-tenant leakage detection
- Privilege escalation detection

## Async Event Pipeline

All trace emission is async and non-blocking:
1. **Hot path (sync):** Capture raw data into span attributes — microseconds
2. **Background goroutine:** `BatchSpanProcessor` flushes every N spans or M milliseconds to Triage ingest API
3. **Backpressure:** If queue fills, `BatchSpanProcessor` drops spans — never blocks the customer's application

---

## Go SDK — Implementation Plan

### Wire Protocol Decision: OTLP/HTTP

Same as Python and TypeScript SDKs. We use the **standard OpenTelemetry Protocol over HTTP** (protobuf encoding) via `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp`.

**Why OTLP, not a custom exporter:**
- The Go OTel SDK ships a production-grade OTLP exporter with batching, retry, compression, and connection management.
- Customers already using OTel can point their existing exporter at our backend with an API key header — zero SDK dependency.
- Same wire format as our Python and TypeScript SDKs — one backend receiver serves all languages.

**Why HTTP, not gRPC:**
- Our backend is FastAPI on ECS Fargate — HTTP/1.1 is native.
- `BatchSpanProcessor` already batches spans, so we get sufficient throughput without gRPC streaming.
- Avoids pulling in the `google.golang.org/grpc` dependency tree (~15+ transitive deps).
- Can always add a gRPC exporter option later.

### Go-Specific Design Decisions

#### Context Propagation via `context.Context`

Go's standard library `context.Context` is the idiomatic mechanism for request-scoped data. Unlike Python (`contextvars.ContextVar`) or TypeScript (`AsyncLocalStorage`), Go context is **explicit** — it must be passed as the first function parameter. This is a fundamental difference that shapes the entire API.

**Implications:**
- Every context helper returns a new `context.Context` (contexts are immutable in Go).
- The user must pass `ctx` through their call chain — there is no implicit propagation.
- This is idiomatic Go. Fighting it (e.g., using goroutine-local storage hacks) would be non-idiomatic and fragile.

```go
ctx = triage.WithUser(ctx, "u_123", triage.UserRole("admin"))
ctx = triage.WithTenant(ctx, "org_456")
// Pass ctx to LLM calls — spans created within inherit context
resp, err := client.CreateChatCompletion(ctx, req)
```

#### No Monkey-Patching — Explicit Instrumentation

Go does not support monkey-patching. Auto-instrumentation works differently:

1. **HTTP middleware** (`otelhttp`): Wraps `http.Handler` and `http.Transport` to create spans for HTTP requests.
2. **gRPC interceptors** (`otelgrpc`): Wraps gRPC clients/servers.
3. **LLM provider wrappers**: We provide thin wrappers (e.g., `triageopenai`) that instrument calls to OpenAI, Anthropic, etc. These are opt-in — the user wraps their client explicitly.

This means Go users write slightly more setup code than Python/TypeScript users, but get explicit, predictable behavior with zero magic.

#### Error Handling

Go uses explicit error returns, not exceptions. Every fallible operation returns `(result, error)`. The SDK follows this pattern:

- `Init()` returns `(func(), error)` — the `func()` is the shutdown function, the `error` is any initialization failure.
- Context helpers never fail (they just attach values to context) — no error return needed.
- `Shutdown()` returns `error` to report flush/export failures.

#### Singleton via `sync.Once`

The SDK uses `sync.Once` to guarantee exactly-once initialization, safe for concurrent goroutine access. Unlike Python/TypeScript which use a boolean flag with a warning on double-init, Go's `sync.Once` is the idiomatic concurrency-safe pattern.

#### Graceful Shutdown

Go has no `atexit` equivalent. Shutdown is handled by:
- Returning a shutdown function from `Init()` that the user defers: `shutdown, err := triage.Init(...); defer shutdown()`
- Or the user calls `triage.Shutdown(ctx)` explicitly in their signal handler.
- The shutdown function calls `TracerProvider.Shutdown(ctx)` which flushes pending spans with a context deadline.

#### Exported vs Unexported

Go uses capitalization for visibility. Public API is `Exported`, internal implementation is `unexported`:
- `triage.Init()`, `triage.WithUser()`, `triage.Shutdown()` — exported (public API)
- `triageContext`, `contextKey`, `getTriageAttrs()` — unexported (internal)

#### Struct Tags for Configuration

Go uses struct tags for JSON/env unmarshaling. Config fields use `env:"TRIAGE_API_KEY"` tags for environment variable binding.

### End-User Experience

A developer integrates Triage observability with explicit setup and context passing:

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/openai/openai-go"
    "github.com/triageai/triage/sdk/go/triage"
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

    // 3. Use any LLM provider normally — pass ctx through
    client := openai.NewClient()
    resp, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
        Model: openai.ChatModelGPT4o,
        Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
            openai.UserMessage("Explain prompt injection in 2 sentences"),
        }),
    })
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(resp.Choices[0].Message.Content)
}
```

### Package Structure

```
sdk/go/
├── go.mod                              # Module: github.com/triageai/triage/sdk/go
├── go.sum                              # Dependency checksums
├── CLAUDE.md                           # This file
├── README.md                           # Public user-facing documentation
├── triage/                             # Main package — public API
│   ├── triage.go                       # Init(), Shutdown(), top-level orchestration
│   ├── config.go                       # Config struct, functional options, env resolution
│   ├── config_test.go                  # Config tests (arg > env > default, validation)
│   ├── constants.go                    # Span attribute keys (triage.*), env var names
│   ├── context.go                      # 6 context helpers (WithUser, WithTenant, etc.)
│   ├── context_test.go                 # Context helper tests
│   ├── processor.go                    # TriageSpanProcessor — injects context into spans
│   ├── processor_test.go              # Processor tests
│   ├── triage_test.go                  # Init/shutdown integration tests
│   ├── version.go                      # Version constant
│   └── e2e_test.go                     # End-to-end tests
└── internal/                           # Internal packages (not importable by consumers)
    └── testutil/                       # Shared test helpers
        └── testutil.go                 # InMemoryExporter setup, span assertion helpers
```

**Why `triage/` subdirectory for the main package?**
Go convention: the import path is `github.com/triageai/triage/sdk/go/triage`. Having the package named `triage` gives clean usage: `triage.Init()`, `triage.WithUser()`. The alternative — putting code at the module root — would force `go.Init()` which is unclear.

**Why `internal/`?**
Go's `internal/` directory enforces that packages under it cannot be imported by external consumers. This is a hard guarantee from the Go toolchain, not a convention. Test utilities and internal helpers go here.

**Why no `cmd/` directory?**
The SDK is a library, not an executable. No `main` package needed.

### Implementation Steps

#### Step 1 · Module Scaffolding — `go.mod`

```
module github.com/triageai/triage/sdk/go

go 1.21

require (
    go.opentelemetry.io/otel                          v1.34.0
    go.opentelemetry.io/otel/sdk                      v1.34.0
    go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.34.0
)
```

**Minimum Go version: 1.21**
- Required for `log/slog` (structured logging, stdlib since 1.21).
- Required for `slices` and `maps` packages.
- Matches OTel Go SDK's minimum version.

**Runtime dependencies:**

| Package | Purpose |
|---------|---------|
| `go.opentelemetry.io/otel` | OTel API (tracer, span, context, attribute) |
| `go.opentelemetry.io/otel/sdk` | TracerProvider, BatchSpanProcessor, SpanProcessor interface |
| `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp` | Standard OTLP/HTTP exporter |
| `go.opentelemetry.io/otel/sdk/resource` | Resource attributes (sdk name, version, environment) |
| `go.opentelemetry.io/otel/attribute` | Attribute key-value pairs |
| `go.opentelemetry.io/otel/trace` | Trace API (SpanKind, StatusCode) |

**Dev/test dependencies:**

| Package | Purpose |
|---------|---------|
| `go.opentelemetry.io/otel/sdk/trace/tracetest` | `InMemoryExporter` for capturing spans in tests |

No external test framework needed — Go's stdlib `testing` package is sufficient. Use `t.Run()` for subtests (table-driven tests).

---

#### Step 2 · Version — `version.go`

```go
package triage

// Version is the semantic version of the Triage Go SDK.
const Version = "0.1.0"
```

Single constant. Referenced by resource attributes during init.

---

#### Step 3 · Constants — `constants.go`

All custom span attribute keys under the `triage.*` namespace. Using constants prevents typos and enables IDE autocomplete.

```go
package triage

// Triage context span attributes
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

// SDK metadata attributes
const (
    AttrSDKName    = "triage.sdk.name"
    AttrSDKVersion = "triage.sdk.version"
    SDKName        = "triage-sdk-go"
)

// Environment variable names
const (
    EnvAPIKey       = "TRIAGE_API_KEY"
    EnvEndpoint     = "TRIAGE_ENDPOINT"
    EnvAppName      = "TRIAGE_APP_NAME"
    EnvEnvironment  = "TRIAGE_ENVIRONMENT"
    EnvEnabled      = "TRIAGE_ENABLED"
    EnvTraceContent = "TRIAGE_TRACE_CONTENT"
)

// Defaults
const (
    DefaultEndpoint       = "https://api.triageai.dev"
    DefaultOTLPTracesPath = "/v1/traces"
)
```

---

#### Step 4 · Configuration — `config.go`

Uses the **functional options pattern** — idiomatic Go for optional configuration. This avoids the builder pattern, avoids a massive constructor signature, and gives clean call sites.

```go
package triage

// Config holds resolved SDK configuration. Immutable after creation.
type Config struct {
    apiKey       string
    endpoint     string
    appName      string
    environment  string
    enabled      bool
    traceContent bool
}

// Option configures the SDK.
type Option func(*Config)

// Functional option constructors:
func WithAPIKey(key string) Option       { return func(c *Config) { c.apiKey = key } }
func WithEndpoint(ep string) Option      { return func(c *Config) { c.endpoint = ep } }
func WithAppName(name string) Option     { return func(c *Config) { c.appName = name } }
func WithEnvironment(env string) Option  { return func(c *Config) { c.environment = env } }
func WithEnabled(b bool) Option          { return func(c *Config) { c.enabled = b } }
func WithTraceContent(b bool) Option     { return func(c *Config) { c.traceContent = b } }
```

**Resolution priority:** explicit option > environment variable > default (same as Python/TypeScript).

**`resolveConfig(opts ...Option) (*Config, error)`** (unexported):
1. Start with defaults: `endpoint=DefaultEndpoint`, `environment="development"`, `enabled=true`, `traceContent=true`, `appName=os.Args[0]`.
2. Override with env vars (if set): `os.Getenv(EnvAPIKey)`, etc.
3. Override with explicit options (applied last, highest priority).
4. Validate: return `error` if `apiKey` is empty.
5. Return immutable `*Config`.

**Boolean env parsing:** Accepts `true/false/1/0/yes/no` (case-insensitive), matching Python/TypeScript behavior. Uses a helper `parseBoolEnv(key string, fallback bool) bool`.

**Why functional options over a config struct literal?**
- Config struct fields are unexported (lowercase) — prevents mutation after creation.
- Functional options compose cleanly: `triage.Init(triage.WithAPIKey("k"), triage.WithAppName("app"))`.
- Env var fallback logic lives in `resolveConfig`, not scattered across the call site.
- This is the established Go pattern (used by OTel SDK, gRPC, etc.).

---

#### Step 5 · Context Helpers — `context.go`

Uses `context.WithValue()` from Go's standard library. Each helper returns a **new** `context.Context` with the triage data attached (contexts are immutable value types in Go).

**Internal context key type:**

```go
package triage

// unexported key type prevents collisions with other packages
type contextKey struct{}

// triageContext holds all triage annotation values.
type triageContext struct {
    UserID             string
    UserRole           string
    TenantID           string
    TenantName         string
    SessionID          string
    SessionTurnNumber  *int    // pointer to distinguish "not set" from zero
    SessionHistoryHash string
    InputRaw           string
    InputSanitized     string
    TemplateID         string
    TemplateVersion    string
    ChunkACLs          string  // JSON-serialized
}
```

**Why a single struct in context instead of multiple keys?**
- Fewer `context.WithValue()` calls (one vs 12). Each `WithValue` creates a new context layer — fewer layers = faster lookups.
- Atomic reads in the processor — one lookup gets all triage data.
- Matches Python's `TriageContext` dataclass and TypeScript's `TriageContext` interface.

**6 public helper functions:**

```go
// WithUser attaches user identity to the context.
func WithUser(ctx context.Context, userID string, opts ...UserOption) context.Context

// WithTenant attaches tenant/org identity to the context.
func WithTenant(ctx context.Context, tenantID string, opts ...TenantOption) context.Context

// WithSession attaches session metadata to the context.
func WithSession(ctx context.Context, sessionID string, opts ...SessionOption) context.Context

// WithInput attaches user input (raw and optionally sanitized) to the context.
func WithInput(ctx context.Context, raw string, opts ...InputOption) context.Context

// WithTemplate attaches prompt template metadata to the context.
func WithTemplate(ctx context.Context, templateID string, opts ...TemplateOption) context.Context

// WithChunkACLs attaches retrieved chunk ACL metadata to the context.
// The acls slice is JSON-serialized and stored as a string attribute.
func WithChunkACLs(ctx context.Context, acls []map[string]any) context.Context
```

**Optional parameters via typed option functions:**

Go doesn't have optional parameters. We use per-helper option types to provide optional fields cleanly:

```go
// UserOption configures optional fields for WithUser.
type UserOption func(*triageContext)

func UserRole(role string) UserOption {
    return func(tc *triageContext) { tc.UserRole = role }
}

// SessionOption configures optional fields for WithSession.
type SessionOption func(*triageContext)

func TurnNumber(n int) SessionOption {
    return func(tc *triageContext) { tc.SessionTurnNumber = &n }
}

func HistoryHash(h string) SessionOption {
    return func(tc *triageContext) { tc.SessionHistoryHash = h }
}

// TenantOption configures optional fields for WithTenant.
type TenantOption func(*triageContext)

func TenantName(name string) TenantOption {
    return func(tc *triageContext) { tc.TenantName = name }
}

// InputOption configures optional fields for WithInput.
type InputOption func(*triageContext)

func Sanitized(s string) InputOption {
    return func(tc *triageContext) { tc.InputSanitized = s }
}

// TemplateOption configures optional fields for WithTemplate.
type TemplateOption func(*triageContext)

func TemplateVersion(v string) TemplateOption {
    return func(tc *triageContext) { tc.TemplateVersion = v }
}
```

**Each helper function:**
1. Extracts the existing `triageContext` from `ctx` (or creates a new one if absent).
2. Copies the struct (value semantics — never mutate the original).
3. Updates the relevant fields.
4. Applies optional field functions.
5. Returns `context.WithValue(ctx, contextKey{}, updatedTriageContext)`.

**Key difference from Python/TypeScript:** Context is not mutated in-place. Each `WithXxx()` call returns a new `context.Context`. The caller must capture the return value and pass it forward. This is the idiomatic Go pattern and matches how `context.WithTimeout`, `context.WithCancel`, etc. work.

**`getTriageAttrs(ctx context.Context) []attribute.KeyValue`** (unexported):
Reads the `triageContext` from `ctx` and returns a slice of non-zero-value OTel attributes. Used by the processor.

```go
func getTriageAttrs(ctx context.Context) []attribute.KeyValue {
    tc, ok := ctx.Value(contextKey{}).(triageContext)
    if !ok {
        return nil
    }
    var attrs []attribute.KeyValue
    if tc.UserID != "" {
        attrs = append(attrs, attribute.String(AttrUserID, tc.UserID))
    }
    if tc.UserRole != "" {
        attrs = append(attrs, attribute.String(AttrUserRole, tc.UserRole))
    }
    // ... all 12 fields
    return attrs
}
```

---

#### Step 6 · Span Processor — `processor.go`

Implements `sdktrace.SpanProcessor` from `go.opentelemetry.io/otel/sdk/trace`.

```go
package triage

import (
    "context"

    sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// triageSpanProcessor injects triage context attributes into every span on start.
type triageSpanProcessor struct{}

func (p *triageSpanProcessor) OnStart(ctx context.Context, span sdktrace.ReadWriteSpan) {
    attrs := getTriageAttrs(ctx)
    if len(attrs) > 0 {
        span.SetAttributes(attrs...)
    }
}

func (p *triageSpanProcessor) OnEnd(span sdktrace.ReadOnlySpan) {}

func (p *triageSpanProcessor) Shutdown(ctx context.Context) error {
    return nil
}

func (p *triageSpanProcessor) ForceFlush(ctx context.Context) error {
    return nil
}
```

**Go-specific notes:**
- `SpanProcessor.OnStart` receives `context.Context` as first parameter — this is where our triage data lives. Python/TypeScript processors had to read from ContextVar/AsyncLocalStorage instead.
- `Shutdown` and `ForceFlush` accept `context.Context` for deadline/cancellation support.
- Both return `error` (Go convention) vs Python returning `None`/`bool` and TypeScript returning `Promise<void>`.
- The processor is unexported (`triageSpanProcessor`) — users never instantiate it directly.

**Why the processor approach still works in Go:**
In Python/TypeScript, the processor reads context from a thread-local/async-local variable because auto-instrumented spans start inside the LLM call (after context is set). In Go, OTel passes `context.Context` directly to `OnStart`, so the processor reads triage data from the same `ctx` the user passed to the LLM call. This is actually cleaner than the Python/TypeScript approach.

---

#### Step 7 · SDK Initialization — `triage.go`

```go
package triage

import (
    "context"
    "fmt"
    "log/slog"
    "sync"

    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
    "go.opentelemetry.io/otel/sdk/resource"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
    semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
    "go.opentelemetry.io/otel/attribute"
)

var (
    initOnce     sync.Once
    globalConfig *Config
    shutdownFunc func()
)
```

**`Init(opts ...Option) (shutdown func(), err error)`:**

1. Resolve config via `resolveConfig(opts...)`.
2. If `!config.enabled`, log info and return no-op shutdown.
3. Create OTLP HTTP exporter:
   ```go
   exporter, err := otlptracehttp.New(ctx,
       otlptracehttp.WithEndpointURL(config.endpoint + DefaultOTLPTracesPath),
       otlptracehttp.WithHeaders(map[string]string{
           "Authorization": "Bearer " + config.apiKey,
       }),
   )
   ```
4. Create resource with SDK metadata:
   ```go
   res, _ := resource.Merge(
       resource.Default(),
       resource.NewWithAttributes(
           semconv.SchemaURL,
           attribute.String(AttrSDKName, SDKName),
           attribute.String(AttrSDKVersion, Version),
           attribute.String("triage.environment", config.environment),
           semconv.ServiceName(config.appName),
       ),
   )
   ```
5. Create `TracerProvider` with `triageSpanProcessor` and `BatchSpanProcessor`:
   ```go
   tp := sdktrace.NewTracerProvider(
       sdktrace.WithResource(res),
       sdktrace.WithSpanProcessor(&triageSpanProcessor{}),
       sdktrace.WithBatcher(exporter),
   )
   ```
6. Register as global TracerProvider: `otel.SetTracerProvider(tp)`.
7. Return shutdown function:
   ```go
   shutdown = func() {
       ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
       defer cancel()
       if err := tp.Shutdown(ctx); err != nil {
           slog.Error("triage SDK shutdown error", "error", err)
       }
   }
   ```

**Why `sync.Once` is NOT used for `Init`:**
While `sync.Once` prevents double-init at the goroutine level, we use a simpler `initialized` boolean with a mutex because:
- We need to return errors (sync.Once doesn't propagate errors cleanly).
- We need to log a warning on double-init (sync.Once silently skips).
- We need `Shutdown()` to reset the state for testability.

Instead, use a `sync.Mutex` to protect the initialized flag:

```go
var (
    mu          sync.Mutex
    initialized bool
    provider    *sdktrace.TracerProvider
)

func Init(opts ...Option) (func(), error) {
    mu.Lock()
    defer mu.Unlock()

    if initialized {
        slog.Warn("triage SDK already initialized, skipping")
        return func() {}, nil
    }
    // ... setup ...
    initialized = true
    return shutdownFn, nil
}
```

**Shutdown function:**

The returned shutdown function:
1. Acquires the mutex.
2. Calls `provider.ForceFlush(ctx)` to flush pending spans.
3. Calls `provider.Shutdown(ctx)` to release resources.
4. Sets `initialized = false`.
5. Respects the context deadline (default 5 seconds).

**Why return a shutdown function instead of a global `Shutdown()`?**
Both are provided. The returned function is idiomatic Go (`defer shutdown()`), and a package-level `Shutdown(ctx)` is also available for signal handlers:

```go
// Package-level shutdown for signal handlers
func Shutdown(ctx context.Context) error {
    mu.Lock()
    defer mu.Unlock()
    if !initialized || provider == nil {
        return nil
    }
    initialized = false
    return provider.Shutdown(ctx)
}
```

---

#### Step 8 · No `__init__.py` / `index.ts` Equivalent

In Go, the package **is** the public API. Everything exported (capitalized) from any `.go` file in the `triage/` package is part of the public surface. There's no separate "re-export" file needed.

**Public API surface** (across all files in `triage/`):

```go
// Lifecycle
func Init(opts ...Option) (func(), error)
func Shutdown(ctx context.Context) error

// Configuration options
func WithAPIKey(key string) Option
func WithEndpoint(ep string) Option
func WithAppName(name string) Option
func WithEnvironment(env string) Option
func WithEnabled(b bool) Option
func WithTraceContent(b bool) Option

// Context helpers
func WithUser(ctx context.Context, userID string, opts ...UserOption) context.Context
func WithTenant(ctx context.Context, tenantID string, opts ...TenantOption) context.Context
func WithSession(ctx context.Context, sessionID string, opts ...SessionOption) context.Context
func WithInput(ctx context.Context, raw string, opts ...InputOption) context.Context
func WithTemplate(ctx context.Context, templateID string, opts ...TemplateOption) context.Context
func WithChunkACLs(ctx context.Context, acls []map[string]any) context.Context

// Context helper options
func UserRole(role string) UserOption
func TenantName(name string) TenantOption
func TurnNumber(n int) SessionOption
func HistoryHash(h string) SessionOption
func Sanitized(s string) InputOption
func TemplateVersion(v string) TemplateOption

// Constants (exported for advanced users)
const Version = "0.1.0"
const AttrUserID, AttrUserRole, ... // all attribute key constants
const EnvAPIKey, EnvEndpoint, ...   // all env var name constants

// Types
type Option func(*Config)
type UserOption func(*triageContext)
type TenantOption func(*triageContext)
type SessionOption func(*triageContext)
type InputOption func(*triageContext)
type TemplateOption func(*triageContext)
```

---

#### Step 9 · Tests

All tests use Go's stdlib `testing` package + `go.opentelemetry.io/otel/sdk/trace/tracetest` for `InMemoryExporter`.

**Test file naming:** Go convention — test files are `*_test.go` in the same package. They have full access to unexported symbols.

**tests/testutil.go** (in `internal/testutil/`):

```go
package testutil

import (
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
    "go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// SetupTestProvider creates a TracerProvider with an InMemoryExporter
// and the TriageSpanProcessor for testing.
func SetupTestProvider() (*sdktrace.TracerProvider, *tracetest.InMemoryExporter) {
    exporter := tracetest.NewInMemoryExporter()
    tp := sdktrace.NewTracerProvider(
        sdktrace.WithSpanProcessor(&triageSpanProcessor{}),
        sdktrace.WithSyncer(exporter), // sync export for deterministic tests
    )
    return tp, exporter
}
```

**Test structure — table-driven tests (idiomatic Go):**

```go
func TestResolveConfig(t *testing.T) {
    tests := []struct {
        name    string
        opts    []Option
        envs    map[string]string
        want    Config
        wantErr bool
    }{
        {
            name: "explicit api key",
            opts: []Option{WithAPIKey("tsk_123")},
            want: Config{apiKey: "tsk_123", endpoint: DefaultEndpoint, ...},
        },
        {
            name: "env fallback",
            envs: map[string]string{EnvAPIKey: "tsk_env"},
            want: Config{apiKey: "tsk_env", ...},
        },
        {
            name:    "missing api key",
            wantErr: true,
        },
        // ...
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            for k, v := range tt.envs {
                t.Setenv(k, v) // auto-cleaned up after test
            }
            got, err := resolveConfig(tt.opts...)
            if (err != nil) != tt.wantErr { ... }
            if got.apiKey != tt.want.apiKey { ... }
        })
    }
}
```

**Test files and coverage:**

| File | Tests | Coverage |
|------|-------|---------|
| `config_test.go` | ~15 | Arg > env > default precedence, missing key error, bool parsing, defaults |
| `context_test.go` | ~14 | Each helper sets correct fields, option functions, merged context, getTriageAttrs returns only non-zero, context immutability |
| `processor_test.go` | ~5 | Processor injects attrs on span start, no context = no attrs, different context per span, multiple fields |
| `triage_test.go` | ~8 | Init creates provider, double-init warns, shutdown flushes, disabled config skips, resource attrs correct |
| `e2e_test.go` | ~5 | Full flow: context helpers + processor + exporter, scoped context, attribute coexistence, multi-span |

**Go-specific test patterns:**
- `t.Setenv()` (Go 1.17+): Sets env vars that auto-restore after the test. No manual cleanup needed.
- `t.Parallel()`: Mark tests that can run concurrently. Config tests that set env vars should NOT be parallel.
- `t.Helper()`: Mark helper functions so test failures report the caller's line number.
- `t.Cleanup()`: Register cleanup functions that run after the test (like `defer` but for test scope).
- No assertion library needed — use `if got != want { t.Errorf(...) }`. Stdlib is sufficient.

---

### Implementation Order

| Order | File(s) | Rationale |
|-------|---------|-----------|
| 1 | `go.mod` | Enables `go mod tidy` — everything else needs deps |
| 2 | `version.go` | Trivial, referenced by other files |
| 3 | `constants.go` | No deps, referenced by all other files |
| 4 | `config.go` + `config_test.go` | Depends only on constants — testable immediately |
| 5 | `context.go` + `context_test.go` | Depends on constants + OTel attribute — testable in isolation |
| 6 | `processor.go` + `processor_test.go` | Depends on context — testable with mock spans |
| 7 | `triage.go` + `triage_test.go` | Depends on all above — integration point |
| 8 | `e2e_test.go` | Full integration validation |
| 9 | `internal/testutil/testutil.go` | Shared test helpers (can be built alongside step 6) |

---

## Key Differences from Python & TypeScript SDKs

| Aspect | Python | TypeScript | Go |
|--------|--------|------------|-----|
| Context propagation | `contextvars.ContextVar` (implicit) | `AsyncLocalStorage` (implicit) | `context.Context` (explicit, passed as param) |
| Context scoping | `with context(...)` generator | `withContext(overrides, fn)` callback | New `ctx` returned from `WithXxx()` — scoped by function call chain |
| Update in-place | `ContextVar.set()` | `AsyncLocalStorage.enterWith()` | Not possible — contexts are immutable; each helper returns new ctx |
| Optional params | `def f(x, y=None)` keyword args | `function f(x: string, y?: string)` | Functional option types (`UserRole("admin")`) |
| Immutability | `@dataclass(frozen=True)` | `Readonly<T>` + `Object.freeze()` | Unexported struct fields (lowercase) — can't be set outside package |
| Error handling | Exceptions (`raise ValueError`) | Exceptions (`throw Error`) | Return `(result, error)` tuples |
| Singleton guard | `if cls._initialized: warn` | `if (initialized) { warn }` | `sync.Mutex` + boolean flag |
| Shutdown hook | `atexit.register()` | `process.on('beforeExit')` | Return shutdown func from `Init()` + `Shutdown(ctx)` for signal handlers |
| Auto-instrumentation | Traceloop monkey-patches 37 providers | Traceloop monkey-patches providers | No monkey-patching — explicit middleware/wrappers |
| Test runner | `pytest` | `vitest` | `go test` (stdlib) |
| Package build | `hatchling` (wheel/sdist) | `tsup` (ESM + CJS bundles) | None needed — Go modules compile from source |
| SDK name constant | `triage-sdk-python` | `triage-sdk-typescript` | `triage-sdk-go` |
| Module registry | PyPI | npm | Go Modules (Git tag-based, no registry upload) |
| Publish command | `twine upload` | `pnpm publish` | `git tag sdk/go/v0.1.0 && git push --tags` |

## Data Flow Diagram

```
Developer's Go Application
    │
    ├── triage.Init(triage.WithAPIKey("tsk_..."))
    │       └── Creates TracerProvider with:
    │           ├── triageSpanProcessor (reads context, injects triage.* attrs)
    │           └── BatchSpanProcessor → OTLPTraceHTTPExporter
    │
    ├── ctx = triage.WithUser(ctx, "u_123", triage.UserRole("admin"))
    │       └── Stores triageContext in context.Context (immutable, value type)
    │
    ├── client.CreateChatCompletion(ctx, req)
    │   │   ↑ ctx carries triage data through the call chain
    │   │
    │   ├── [middleware/wrapper creates span, passing ctx]
    │   ├── [OnStart] triageSpanProcessor reads ctx → sets triage.* attributes
    │   ├── [wrapper] Sets gen_ai.* attributes (model, tokens, messages)
    │   ├── [OnEnd] BatchSpanProcessor queues span for export
    │   │
    │   └── [background goroutine] OTLPTraceHTTPExporter
    │           ├── Serializes spans to protobuf (ExportTraceServiceRequest)
    │           ├── POST /v1/traces with Authorization: Bearer tsk_...
    │           ├── Gzip compression, retry with backoff on 429/5xx
    │           └── Receives ExportTraceServiceResponse
    │
    v
FastAPI Backend: POST /v1/traces
    │
    ├── Parse ExportTraceServiceRequest (protobuf or JSON)
    ├── Walk resource_spans → scope_spans → spans
    ├── Transform each span → traces table row
    ├── Bulk insert into PostgreSQL
    │
    v
Backend Workers (async, post-ingestion)
    ├── Embeddings, PII detection, drift scoring, groundedness
    └── Security analysis (prompt injection, cross-tenant leakage, privilege escalation)
```

## Go-Specific Nuances & Gotchas

### 1. `context.Context` Must Be Threaded Explicitly

Unlike Python/TypeScript where context is implicit (goroutine-local), Go requires passing `ctx` as the first parameter through the entire call chain. If a user forgets to pass the enriched `ctx`, spans won't have triage attributes. This is by design — explicit is better than magic in Go.

**Mitigation:** Clear documentation, examples showing `ctx` being passed through, and a `getTriageAttrs` that returns `nil` (not an error) when no triage context is present.

### 2. Goroutine Safety

- `context.Context` is safe for concurrent use (immutable).
- `sync.Mutex` protects the `initialized` flag and `provider` pointer.
- `triageSpanProcessor` is stateless — safe for concurrent `OnStart` calls.
- `Config` struct has unexported fields — immutable after creation.

### 3. No Generics Needed

The SDK doesn't require generics. All context values are concrete types (`string`, `int`, `[]map[string]any`). The functional options pattern uses specific types (`UserOption`, `SessionOption`) rather than generic options.

### 4. `encoding/json` for Chunk ACLs

`WithChunkACLs` accepts `[]map[string]any` and internally calls `json.Marshal()` to serialize to a string. This matches Python/TypeScript behavior where chunk ACLs are stored as a JSON string in the span attribute. If marshaling fails, the ACLs are silently dropped (don't break the user's application for a telemetry failure).

### 5. Zero Values vs Nil

Go has zero values for all types (`""` for strings, `0` for ints). To distinguish "not set" from "set to zero/empty":
- String fields: empty string `""` means "not set" (a user ID is never empty).
- Integer fields: use `*int` (pointer) — `nil` means "not set", `0` means "set to zero".
- This affects `SessionTurnNumber` which is `*int` in `triageContext`.

### 6. Go Module Versioning

Go modules use Git tags for versioning. Since the SDK is a subdirectory of the monorepo:
- Tag format: `sdk/go/v0.1.0` (prefix matches the module path suffix)
- Import path: `github.com/triageai/triage/sdk/go` for v0.x and v1.x
- For v2+: import path becomes `github.com/triageai/triage/sdk/go/v2` (Go major version suffix rule)

### 7. No Build Step

Go modules compile from source. There's no equivalent of `tsup build` or `python -m build`. Consumers run `go get` which downloads source and compiles it. This means:
- No `dist/` directory
- No build artifacts to publish
- No dual-format concerns (ESM/CJS)
- Version is a constant in source, not in a manifest

### 8. `log/slog` for Structured Logging

The SDK uses `log/slog` (stdlib since Go 1.21) for internal logging. This is structured, leveled, and replaceable — users can configure their own `slog.Handler` to control SDK log output. Avoids pulling in external logging dependencies (logrus, zap, zerolog).

### 9. Test Isolation with `t.Setenv`

`t.Setenv` (Go 1.17+) sets environment variables that are automatically restored after the test. This is critical for config tests that read from env vars. Tests using `t.Setenv` cannot be `t.Parallel()` — Go enforces this at runtime.

### 10. Interface Compliance Check

Use a compile-time interface compliance check to ensure `triageSpanProcessor` implements `SpanProcessor`:

```go
var _ sdktrace.SpanProcessor = (*triageSpanProcessor)(nil)
```

This fails at compile time (not runtime) if the interface changes.

## Verification

1. **Module:** `cd sdk/go && go mod tidy` — no errors, clean `go.sum`
2. **Build:** `go build ./...` — compiles all packages
3. **Vet:** `go vet ./...` — zero warnings
4. **Tests:** `go test ./... -v` — all tests pass
5. **Race detector:** `go test ./... -race` — no data races
6. **Import test:** Create a scratch `main.go` that imports `github.com/triageai/triage/sdk/go/triage` and calls `triage.Init()` — compiles and runs
7. **Parity:** Run test agent alongside Python/TypeScript test agents, verify identical `triage.*` attributes in traces table
