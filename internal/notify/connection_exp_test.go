package notify

// Invariants tested:
//  1. Invalid messages: Invalid operation IDs and unauthenticated messages cannot request a notification.

import (
	"encoding/json"
	"net"
	"testing"
)

// TestInvalidMessages verifies invariant #1: Invalid messages.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
func TestInvalidMessages(t *testing.T) {
	listener, err := Listen(t.Context(), t.TempDir())
	if err != nil {
		t.Fatalf("💣 listener: %v", err)
	}
	defer listener.Close()

	for _, request := range []message{{ID: 42}, {ID: 42, Token: "incorrect"}, {ID: -1, Token: listener.endpoint.Token}} {
		dialer := net.Dialer{}

		connection, dialErr := dialer.DialContext(t.Context(), "tcp4", listener.endpoint.Address)
		if dialErr != nil {
			t.Fatalf("💣 connect to listener: %v", dialErr)
		}

		if err = json.NewEncoder(connection).Encode(request); err != nil {
			t.Fatalf("💣 write message: %v", err)
		}

		if err = connection.Close(); err != nil {
			t.Fatalf("💣 close sender: %v", err)
		}

		if actionID, receiveErr := listener.Receive(); receiveErr == nil || actionID != 0 {
			t.Errorf("✗ invalid message delivered request %d: %v", actionID, receiveErr)
		}
	}

	if !t.Failed() {
		t.Log("✓ invalid messages rejected")
	}
}
