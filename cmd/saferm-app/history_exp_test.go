package main

// Invariants tested:
//  1. Invalid list filters: Unsupported types and statuses fail before returning history.
//  2. Bounded pages: Arbitrary pagination input cannot panic or expose an invalid empty page.

import (
	"errors"
	"math"
	"testing"

	"github.com/shdeen/saferm/internal/errs"
)

// TestInvalidFilters verifies invariant #1: Invalid list filters.
// Test class: Expanded. Test layer: Hardening/adversarial. Test kind: Permanent.
func TestInvalidFilters(t *testing.T) {
	homeDirectory := t.TempDir()
	t.Setenv("HOME", homeDirectory)
	t.Setenv("USERPROFILE", homeDirectory)

	service := OperationService{}
	for _, filter := range []struct{ actionType, status string }{{"erase", ""}, {"", "unknown"}, {"delete", "-1"}} {
		_, err := service.List(t.Context(), filter.actionType, filter.status, 1, 20)
		if !errors.Is(err, errs.ErrInputInvalid) {
			t.Errorf("✗ filter %+v accepted: %v", filter, err)
		}
	}

	if !t.Failed() {
		t.Log("✓ unsupported filters retain shared validation")
	}
}

// FuzzEmptyPages verifies invariant #2: Bounded pages.
// Test class: Expanded. Test layer: Fuzzing. Test kind: Permanent.
func FuzzEmptyPages(f *testing.F) {
	f.Add(0, 0)
	f.Add(math.MaxInt, math.MaxInt)
	f.Add(math.MinInt, math.MinInt)
	f.Fuzz(checkEmptyPage)
}

func checkEmptyPage(t *testing.T, requestedPage, requestedSize int) {
	t.Helper()
	homeDirectory := t.TempDir()
	t.Setenv("HOME", homeDirectory)
	t.Setenv("USERPROFILE", homeDirectory)

	service := OperationService{}

	actionPage, err := service.List(t.Context(), "", "", requestedPage, requestedSize)
	if err != nil || actionPage.Page != 1 || actionPage.Total != 0 || len(actionPage.Actions) != 0 || actionPage.PageSize < 1 || actionPage.PageSize > 100 {
		t.Errorf("✗ invalid empty page: %+v, %v", actionPage, err)
	}

	if requestedSize <= 0 && actionPage.PageSize != 20 {
		t.Errorf("✗ missing default page size: %d", actionPage.PageSize)
	}

	if !t.Failed() {
		t.Log("✓ arbitrary pagination remains bounded")
	}
}
