package triage

import (
	"context"
	"testing"
)

// ---------------------------------------------------------------------------
// Workflow basics
// ---------------------------------------------------------------------------

func TestStartWorkflow_CreatesSpanWithTraceloopAttrs(t *testing.T) {
	exporter := newGlobalTestProvider(t)

	wf, _ := StartWorkflow(context.Background(), "chat-pipeline")
	wf.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	attrs := attrMap(spans[0].Attributes)

	if attrs["traceloop.span.kind"] != "workflow" {
		t.Errorf("span.kind: got %v, want %q", attrs["traceloop.span.kind"], "workflow")
	}
	if attrs["traceloop.entity.name"] != "chat-pipeline" {
		t.Errorf("entity.name: got %v, want %q", attrs["traceloop.entity.name"], "chat-pipeline")
	}
	if attrs["traceloop.workflow.name"] != "chat-pipeline" {
		t.Errorf("workflow.name: got %v, want %q", attrs["traceloop.workflow.name"], "chat-pipeline")
	}
	if spans[0].Name != "chat-pipeline" {
		t.Errorf("span name: got %q, want %q", spans[0].Name, "chat-pipeline")
	}
}

// ---------------------------------------------------------------------------
// Task
// ---------------------------------------------------------------------------

func TestStartTask_InheritsWorkflowName(t *testing.T) {
	exporter := newGlobalTestProvider(t)

	_, ctx := StartWorkflow(context.Background(), "my-workflow")
	task, _ := StartTask(ctx, "parse-input")
	task.End()

	spans := exporter.GetSpans()
	// Find the task span.
	var taskAttrs map[string]any
	for _, s := range spans {
		a := attrMap(s.Attributes)
		if a["traceloop.span.kind"] == "task" {
			taskAttrs = a
			break
		}
	}
	if taskAttrs == nil {
		t.Fatal("task span not found")
	}

	if taskAttrs["traceloop.entity.name"] != "parse-input" {
		t.Errorf("entity.name: got %v", taskAttrs["traceloop.entity.name"])
	}
	if taskAttrs["traceloop.workflow.name"] != "my-workflow" {
		t.Errorf("workflow.name: got %v, want %q", taskAttrs["traceloop.workflow.name"], "my-workflow")
	}
}

func TestStartTask_WithoutWorkflow(t *testing.T) {
	exporter := newGlobalTestProvider(t)

	task, _ := StartTask(context.Background(), "standalone-task")
	task.End()

	attrs := attrMap(exporter.GetSpans()[0].Attributes)

	if attrs["traceloop.span.kind"] != "task" {
		t.Errorf("span.kind: got %v", attrs["traceloop.span.kind"])
	}
	// No workflow name should be set.
	if _, ok := attrs["traceloop.workflow.name"]; ok {
		t.Error("task without workflow should not have workflow.name")
	}
}

// ---------------------------------------------------------------------------
// Agent
// ---------------------------------------------------------------------------

func TestStartAgent_SetsAgentName(t *testing.T) {
	exporter := newGlobalTestProvider(t)

	agent, _ := StartAgent(context.Background(), "research-agent")
	agent.End()

	attrs := attrMap(exporter.GetSpans()[0].Attributes)

	if attrs["traceloop.span.kind"] != "agent" {
		t.Errorf("span.kind: got %v", attrs["traceloop.span.kind"])
	}
	if attrs["traceloop.entity.name"] != "research-agent" {
		t.Errorf("entity.name: got %v", attrs["traceloop.entity.name"])
	}
	if attrs["llm.agent.name"] != "research-agent" {
		t.Errorf("llm.agent.name: got %v", attrs["llm.agent.name"])
	}
}

func TestStartAgent_InheritsWorkflowName(t *testing.T) {
	exporter := newGlobalTestProvider(t)

	_, ctx := StartWorkflow(context.Background(), "agent-workflow")
	agent, _ := StartAgent(ctx, "my-agent")
	agent.End()

	var agentAttrs map[string]any
	for _, s := range exporter.GetSpans() {
		a := attrMap(s.Attributes)
		if a["traceloop.span.kind"] == "agent" {
			agentAttrs = a
			break
		}
	}
	if agentAttrs == nil {
		t.Fatal("agent span not found")
	}

	if agentAttrs["traceloop.workflow.name"] != "agent-workflow" {
		t.Errorf("workflow.name: got %v", agentAttrs["traceloop.workflow.name"])
	}
}

// ---------------------------------------------------------------------------
// Tool
// ---------------------------------------------------------------------------

func TestStartTool_SetsToolAttributes(t *testing.T) {
	exporter := newGlobalTestProvider(t)

	tool, _ := StartTool(context.Background(), "get-weather")
	tool.End()

	attrs := attrMap(exporter.GetSpans()[0].Attributes)

	if attrs["traceloop.span.kind"] != "tool" {
		t.Errorf("span.kind: got %v", attrs["traceloop.span.kind"])
	}
	if attrs["traceloop.entity.name"] != "get-weather" {
		t.Errorf("entity.name: got %v", attrs["traceloop.entity.name"])
	}
}

// ---------------------------------------------------------------------------
// Full hierarchy: Workflow → Task → Agent → Tool → LLM call
// ---------------------------------------------------------------------------

func TestWorkflow_FullHierarchy(t *testing.T) {
	exporter := newGlobalTestProvider(t)

	ctx := context.Background()
	ctx = WithUser(ctx, "u_1")

	wf, ctx := StartWorkflow(ctx, "chat-pipeline")

	task, ctx := StartTask(ctx, "process-request")

	agent, ctx := StartAgent(ctx, "chatbot")

	tool, toolCtx := StartTool(ctx, "search-knowledge-base")
	tool.End()

	llmSpan, _ := LogPrompt(ctx, Prompt{
		Vendor:   "openai",
		Model:    "gpt-4o",
		Messages: []Message{{Role: "user", Content: "Hello"}},
	})
	llmSpan.LogCompletion(Completion{
		Model:    "gpt-4o",
		Messages: []Message{{Role: "assistant", Content: "Hi!"}},
	}, Usage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8})

	agent.End()
	task.End()
	wf.End()

	_ = toolCtx // suppress unused

	spans := exporter.GetSpans()
	if len(spans) != 5 {
		t.Fatalf("expected 5 spans, got %d", len(spans))
	}

	// Verify all spans have triage.user.id (from triageSpanProcessor).
	for _, s := range spans {
		attrs := attrMap(s.Attributes)
		if attrs[AttrUserID] != "u_1" {
			t.Errorf("span %q: user_id got %v, want %q", s.Name, attrs[AttrUserID], "u_1")
		}
	}

	// Verify the workflow, task, agent, tool spans have correct kinds.
	spansByName := make(map[string]map[string]any)
	for _, s := range spans {
		spansByName[s.Name] = attrMap(s.Attributes)
	}

	if spansByName["chat-pipeline"]["traceloop.span.kind"] != "workflow" {
		t.Error("chat-pipeline should be workflow kind")
	}
	if spansByName["process-request"]["traceloop.span.kind"] != "task" {
		t.Error("process-request should be task kind")
	}
	if spansByName["chatbot"]["traceloop.span.kind"] != "agent" {
		t.Error("chatbot should be agent kind")
	}
	if spansByName["search-knowledge-base"]["traceloop.span.kind"] != "tool" {
		t.Error("search-knowledge-base should be tool kind")
	}

	// Verify workflow name propagates.
	if spansByName["process-request"]["traceloop.workflow.name"] != "chat-pipeline" {
		t.Error("task should inherit workflow name")
	}
	if spansByName["chatbot"]["traceloop.workflow.name"] != "chat-pipeline" {
		t.Error("agent should inherit workflow name")
	}
	if spansByName["search-knowledge-base"]["traceloop.workflow.name"] != "chat-pipeline" {
		t.Error("tool should inherit workflow name")
	}

	// Verify LLM span has gen_ai attributes.
	llmAttrs := spansByName["openai.chat gpt-4o"]
	if llmAttrs["gen_ai.system"] != "openai" {
		t.Error("LLM span should have gen_ai.system")
	}
	if llmAttrs["gen_ai.usage.input_tokens"] != int64(5) {
		t.Error("LLM span should have gen_ai.usage.input_tokens")
	}
}

// ---------------------------------------------------------------------------
// Nil safety
// ---------------------------------------------------------------------------

func TestWorkflow_NilEnd(t *testing.T) {
	// Should not panic.
	var wf *Workflow
	wf.End()
}

func TestTask_NilEnd(t *testing.T) {
	var task *Task
	task.End()
}

func TestAgent_NilEnd(t *testing.T) {
	var agent *Agent
	agent.End()
}

func TestToolSpan_NilEnd(t *testing.T) {
	var tool *ToolSpan
	tool.End()
}

func TestWorkflow_NilContext(t *testing.T) {
	var wf *Workflow
	ctx := wf.Context()
	if ctx == nil {
		t.Error("Context() on nil Workflow should return non-nil context")
	}
}

// ---------------------------------------------------------------------------
// Span parent-child relationships
// ---------------------------------------------------------------------------

func TestWorkflow_SpansAreNested(t *testing.T) {
	exporter := newGlobalTestProvider(t)

	wf, ctx := StartWorkflow(context.Background(), "parent")
	task, taskCtx := StartTask(ctx, "child")
	_ = taskCtx
	task.End()
	wf.End()

	spans := exporter.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}

	// The task span's parent should be the workflow span.
	wfSpanID := spans[1].SpanContext.SpanID() // workflow ends last, so it's second in InMemory
	taskParentID := spans[0].Parent.SpanID()

	if taskParentID != wfSpanID {
		t.Errorf("task parent %v != workflow span %v", taskParentID, wfSpanID)
	}
}
