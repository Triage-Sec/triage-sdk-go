package triage

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Traceloop span kind constants — matches go-openllmetry / OpenLLMetry conventions.
const (
	spanKindWorkflow = "workflow"
	spanKindTask     = "task"
	spanKindAgent    = "agent"
	spanKindTool     = "tool"
)

// workflowNameKey is an unexported context key for propagating the workflow
// name to child spans (tasks, agents, tools).
type workflowNameKey struct{}

// workflowNameFromContext extracts the workflow name from ctx, or "".
func workflowNameFromContext(ctx context.Context) string {
	if name, ok := ctx.Value(workflowNameKey{}).(string); ok {
		return name
	}
	return ""
}

// ---------------------------------------------------------------------------
// Workflow
// ---------------------------------------------------------------------------

// Workflow represents a traced workflow span — the top-level grouping for a
// multi-step LLM pipeline. Child spans (tasks, agents, LLM calls) created
// from the returned context will be nested under this workflow.
type Workflow struct {
	span trace.Span
	ctx  context.Context
	name string
}

// StartWorkflow creates a new workflow span and returns it along with a
// derived context. Call workflow.End() when the workflow completes:
//
//	wf, ctx := triage.StartWorkflow(ctx, "chat-pipeline")
//	defer wf.End()
func StartWorkflow(ctx context.Context, name string) (*Workflow, context.Context) {
	tracer := otel.GetTracerProvider().Tracer(llmTracerName)
	ctx, span := tracer.Start(ctx, name)

	span.SetAttributes(
		attribute.String("traceloop.span.kind", spanKindWorkflow),
		attribute.String("traceloop.entity.name", name),
		attribute.String("traceloop.workflow.name", name),
	)

	// Store workflow name in context so child spans inherit it.
	ctx = context.WithValue(ctx, workflowNameKey{}, name)

	return &Workflow{span: span, ctx: ctx, name: name}, ctx
}

// End ends the workflow span.
func (w *Workflow) End() {
	if w != nil && w.span != nil {
		w.span.End()
	}
}

// Context returns the context carrying this workflow span.
func (w *Workflow) Context() context.Context {
	if w == nil {
		return context.Background()
	}
	return w.ctx
}

// ---------------------------------------------------------------------------
// Task
// ---------------------------------------------------------------------------

// Task represents a traced task span — a discrete step within a workflow.
type Task struct {
	span trace.Span
	ctx  context.Context
	name string
}

// StartTask creates a new task span. If the context carries a workflow, the
// task automatically inherits the workflow name:
//
//	task, ctx := triage.StartTask(ctx, "parse-input")
//	defer task.End()
func StartTask(ctx context.Context, name string) (*Task, context.Context) {
	tracer := otel.GetTracerProvider().Tracer(llmTracerName)
	ctx, span := tracer.Start(ctx, name)

	attrs := []attribute.KeyValue{
		attribute.String("traceloop.span.kind", spanKindTask),
		attribute.String("traceloop.entity.name", name),
	}
	if wf := workflowNameFromContext(ctx); wf != "" {
		attrs = append(attrs, attribute.String("traceloop.workflow.name", wf))
	}
	span.SetAttributes(attrs...)

	return &Task{span: span, ctx: ctx, name: name}, ctx
}

// End ends the task span.
func (t *Task) End() {
	if t != nil && t.span != nil {
		t.span.End()
	}
}

// Context returns the context carrying this task span.
func (t *Task) Context() context.Context {
	if t == nil {
		return context.Background()
	}
	return t.ctx
}

// ---------------------------------------------------------------------------
// Agent
// ---------------------------------------------------------------------------

// Agent represents a traced agent span — an autonomous entity that can make
// LLM calls and use tools.
type Agent struct {
	span trace.Span
	ctx  context.Context
	name string
}

// StartAgent creates a new agent span:
//
//	agent, ctx := triage.StartAgent(ctx, "research-agent")
//	defer agent.End()
func StartAgent(ctx context.Context, name string) (*Agent, context.Context) {
	tracer := otel.GetTracerProvider().Tracer(llmTracerName)
	ctx, span := tracer.Start(ctx, name)

	attrs := []attribute.KeyValue{
		attribute.String("traceloop.span.kind", spanKindAgent),
		attribute.String("traceloop.entity.name", name),
		attribute.String("llm.agent.name", name),
	}
	if wf := workflowNameFromContext(ctx); wf != "" {
		attrs = append(attrs, attribute.String("traceloop.workflow.name", wf))
	}
	span.SetAttributes(attrs...)

	return &Agent{span: span, ctx: ctx, name: name}, ctx
}

// End ends the agent span.
func (a *Agent) End() {
	if a != nil && a.span != nil {
		a.span.End()
	}
}

// Context returns the context carrying this agent span.
func (a *Agent) Context() context.Context {
	if a == nil {
		return context.Background()
	}
	return a.ctx
}

// ---------------------------------------------------------------------------
// Tool (execution span, not to be confused with ToolDef/ToolCall)
// ---------------------------------------------------------------------------

// ToolSpan represents a traced tool execution span — a function or API call
// made by an agent during processing.
type ToolSpan struct {
	span trace.Span
	ctx  context.Context
	name string
}

// StartTool creates a new tool execution span:
//
//	tool, ctx := triage.StartTool(ctx, "get-weather")
//	defer tool.End()
func StartTool(ctx context.Context, name string) (*ToolSpan, context.Context) {
	tracer := otel.GetTracerProvider().Tracer(llmTracerName)
	ctx, span := tracer.Start(ctx, name)

	attrs := []attribute.KeyValue{
		attribute.String("traceloop.span.kind", spanKindTool),
		attribute.String("traceloop.entity.name", name),
	}
	if wf := workflowNameFromContext(ctx); wf != "" {
		attrs = append(attrs, attribute.String("traceloop.workflow.name", wf))
	}
	span.SetAttributes(attrs...)

	return &ToolSpan{span: span, ctx: ctx, name: name}, ctx
}

// End ends the tool span.
func (t *ToolSpan) End() {
	if t != nil && t.span != nil {
		t.span.End()
	}
}

// Context returns the context carrying this tool span.
func (t *ToolSpan) Context() context.Context {
	if t == nil {
		return context.Background()
	}
	return t.ctx
}
