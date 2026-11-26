package main

// Invariants tested:
//  1. Rule check: The desktop service reports a review pattern as malformed exactly when the shared review check does.

import (
	"testing"

	"github.com/shdeen/saferm/internal/review"
)

// TestRuleCheck verifies invariant #1: Rule check.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// The service reports "[abc" as malformed and "*.pem" as well formed, and its
// answer for each equals the answer of review.Malformed.
func TestRuleCheck(t *testing.T) {
	service := OperationService{}

	for pattern, isMalformed := range map[string]bool{"[abc": true, "*.pem": false} {
		if answer := service.RuleMalformed(pattern); answer != isMalformed || answer != review.Malformed(pattern) {
			t.Errorf("✗ %q malformed: %v", pattern, answer)
		}
	}

	if !t.Failed() {
		t.Log("✓ the desktop service uses the shared rule check")
	}
}
