package triage

import (
	"context"
	"testing"
)

// ---------------------------------------------------------------------------
// Init wiring
// ---------------------------------------------------------------------------

func TestInit_SucceedsWithValidConfig(t *testing.T) {
	t.Cleanup(func() { resetSDK(t) })

	shutdown, err := Init(WithAPIKey("tsk_test"))
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer shutdown()

	mu.Lock()
	defer mu.Unlock()
	if !initialized {
		t.Error("expected initialized to be true after Init")
	}
	if provider == nil {
		t.Error("expected provider to be non-nil after Init")
	}
}

func TestInit_MissingApiKeyReturnsError(t *testing.T) {
	t.Cleanup(func() { resetSDK(t) })

	_, err := Init()
	if err == nil {
		t.Fatal("expected error for missing API key, got nil")
	}
}

func TestInit_CustomEndpoint(t *testing.T) {
	t.Cleanup(func() { resetSDK(t) })

	shutdown, err := Init(
		WithAPIKey("tsk_test"),
		WithEndpoint("https://custom.io"),
	)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer shutdown()

	// If Init succeeded, the exporter was created with the custom endpoint.
	// We can't inspect the exporter's config directly, but no error means
	// the endpoint was accepted.
	mu.Lock()
	defer mu.Unlock()
	if !initialized {
		t.Error("expected initialized to be true")
	}
}

// ---------------------------------------------------------------------------
// Double init
// ---------------------------------------------------------------------------

func TestDoubleInit_SecondCallIsNoop(t *testing.T) {
	t.Cleanup(func() { resetSDK(t) })

	shutdown1, err := Init(WithAPIKey("tsk_test"))
	if err != nil {
		t.Fatal(err)
	}
	defer shutdown1()

	// Second call should return no error and a no-op shutdown.
	shutdown2, err := Init(WithAPIKey("tsk_other"))
	if err != nil {
		t.Fatalf("second Init returned error: %v", err)
	}

	// Calling the no-op shutdown should not panic.
	shutdown2()
}

func TestDoubleInit_InitializedRemainsTrue(t *testing.T) {
	t.Cleanup(func() { resetSDK(t) })

	shutdown, err := Init(WithAPIKey("tsk_test"))
	if err != nil {
		t.Fatal(err)
	}
	defer shutdown()

	// Second call.
	_, _ = Init(WithAPIKey("tsk_test"))

	mu.Lock()
	defer mu.Unlock()
	if !initialized {
		t.Error("expected initialized to remain true after double Init")
	}
}

// ---------------------------------------------------------------------------
// Disabled
// ---------------------------------------------------------------------------

func TestInit_DisabledSkipsInit(t *testing.T) {
	t.Cleanup(func() { resetSDK(t) })

	shutdown, err := Init(WithAPIKey("tsk_test"), WithEnabled(false))
	if err != nil {
		t.Fatalf("Init with enabled=false failed: %v", err)
	}
	defer shutdown()

	mu.Lock()
	defer mu.Unlock()
	if initialized {
		t.Error("expected initialized to be false when disabled")
	}
	if provider != nil {
		t.Error("expected provider to be nil when disabled")
	}
}

// ---------------------------------------------------------------------------
// Shutdown
// ---------------------------------------------------------------------------

func TestShutdown_WhenNotInitializedIsNoop(t *testing.T) {
	t.Cleanup(func() { resetSDK(t) })

	// Should not panic or return error.
	err := Shutdown(context.Background())
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestShutdown_ResetsInitialized(t *testing.T) {
	t.Cleanup(func() { resetSDK(t) })

	_, err := Init(WithAPIKey("tsk_test"))
	if err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	if !initialized {
		mu.Unlock()
		t.Fatal("expected initialized to be true after Init")
	}
	mu.Unlock()

	err = Shutdown(context.Background())
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if initialized {
		t.Error("expected initialized to be false after Shutdown")
	}
	if provider != nil {
		t.Error("expected provider to be nil after Shutdown")
	}
}

func TestShutdown_CanBeCalledMultipleTimes(t *testing.T) {
	t.Cleanup(func() { resetSDK(t) })

	_, err := Init(WithAPIKey("tsk_test"))
	if err != nil {
		t.Fatal(err)
	}

	// First shutdown.
	if err := Shutdown(context.Background()); err != nil {
		t.Fatalf("first Shutdown failed: %v", err)
	}

	// Second shutdown — should be a no-op.
	if err := Shutdown(context.Background()); err != nil {
		t.Fatalf("second Shutdown failed: %v", err)
	}
}
