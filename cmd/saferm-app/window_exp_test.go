package main

// Invariants tested:
//  1. Review activation: Only a positive integer operation ID can be selected by an activation, and it is consumed once.

import (
	"strconv"
	"testing"
)

// FuzzReviewActivation verifies invariant #1: Review activation.
// Test class: Expanded.
// Test layer: Fuzzing.
func FuzzReviewActivation(f *testing.F) {
	for _, argument := range []string{"42", "0", "-1", "", "9223372036854775808", "../../private"} {
		f.Add(argument)
	}

	f.Fuzz(func(t *testing.T, argument string) {
		actionID := requestedReview([]string{"saferm-app", "--review", argument})

		parsedID, parseErr := strconv.ParseInt(argument, 10, 64)
		if parseErr != nil || parsedID <= 0 {
			if actionID != 0 {
				t.Errorf("✗ invalid activation selects %d", actionID)
			}
		} else if actionID != parsedID {
			t.Errorf("✗ activation selected %d instead of %d", actionID, parsedID)
		}

		service := &WindowService{reviewID: actionID}
		if selectedID := service.TakeReview(); selectedID != actionID || service.TakeReview() != 0 {
			t.Errorf("✗ activation was lost or repeated: %d", selectedID)
		}

		if !t.Failed() {
			t.Log("✓ activation validates and consumes the requested operation")
		}
	})
}
