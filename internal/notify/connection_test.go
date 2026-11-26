package notify

// Invariants tested:
//  1. Local delivery: Pushed request IDs reach the running SafeRM Notify and shutdown releases the listener.
//  2. Optional notifier: An absent or stopped SafeRM Notify does not fail request submission.

import (
	"errors"
	"net"
	"os"
	"testing"
)

// TestLocalDelivery verifies invariant #1: Local delivery.
// Test class: Expanded.
// Test layer: Coverage.
func TestLocalDelivery(t *testing.T) {
	home := t.TempDir()

	listener, err := Listen(t.Context(), home)
	if err != nil {
		t.Fatalf("💣 start notifier listener: %v", err)
	}

	if err = Send(t.Context(), home, 42); err != nil {
		t.Errorf("✗ push request: %v", err)
	}

	actionID, err := listener.Receive()
	if err != nil || actionID != 42 {
		t.Errorf("✗ delivered request: %d, %v", actionID, err)
	}

	if err = listener.Close(); err != nil {
		t.Errorf("✗ stop notifier listener: %v", err)
	}

	if _, err = listener.Receive(); !errors.Is(err, net.ErrClosed) {
		t.Errorf("✗ stopped listener still receives: %v", err)
	}

	if !t.Failed() {
		t.Log("✓ request delivered and listener stopped")
	}
}

// TestOptionalNotifier verifies invariant #2: Optional notifier.
// Test class: Expanded.
// Test layer: Coverage.
func TestOptionalNotifier(t *testing.T) {
	home := t.TempDir()
	if err := Send(t.Context(), home, 42); err != nil {
		t.Errorf("✗ absent notifier prevents submission: %v", err)
	}

	listener, err := Listen(t.Context(), home)
	if err != nil {
		t.Fatalf("💣 start listener: %v", err)
	}

	discovery, err := os.ReadFile(listener.path)
	if err != nil {
		t.Fatalf("💣 read listener address: %v", err)
	}

	if err = listener.Close(); err != nil {
		t.Fatalf("💣 stop listener: %v", err)
	}

	if err = os.WriteFile(listener.path, discovery, 0o600); err != nil {
		t.Fatalf("💣 stopped notifier address: %v", err)
	}

	if err = Send(t.Context(), home, 42); err != nil {
		t.Errorf("✗ stopped notifier prevents submission: %v", err)
	}

	if !t.Failed() {
		t.Log("✓ submission works without a running notifier")
	}
}
