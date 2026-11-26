package main

// Invariants tested:
//  1. Operation pages: Pending actions precede other actions across pages, with newest actions first within each group.
//  2. Page bounds: Empty and out-of-range pages remain usable, and omitted page size means twenty actions.

import (
	"path/filepath"
	"strconv"
	"testing"

	"github.com/shdeen/saferm/internal/config"
	"github.com/shdeen/saferm/internal/store"
)

// TestOperationPages verifies invariant #1: Operation pages.
// Test class: Core. Test kind: Permanent.
func TestOperationPages(t *testing.T) {
	homeDirectory := t.TempDir()
	t.Setenv("HOME", homeDirectory)
	t.Setenv("USERPROFILE", homeDirectory)

	history, err := store.Open(t.Context(), filepath.Join(homeDirectory, config.Directory, config.Database))
	if err != nil {
		t.Fatalf("💣 history setup: %v", err)
	}

	for actionIndex := range 45 {
		status := store.Completed
		if actionIndex < 23 {
			status = store.Pending
		}

		action := store.Action{Type: store.DeleteType, Status: status, Source: homeDirectory, Reason: "pagination", TrashID: strconv.Itoa(actionIndex)}
		if err := history.Create(t.Context(), &action); err != nil {
			t.Fatalf("💣 action setup: %v", err)
		}
	}

	if err := history.Close(); err != nil {
		t.Fatalf("💣 close setup: %v", err)
	}

	service := OperationService{}

	firstPage, err := service.List(t.Context(), "", "", 1, 0)
	if err != nil || firstPage.Total != 45 || firstPage.Pending != 23 || len(firstPage.Actions) != 20 {
		t.Fatalf("💣 unusable first page: %+v, %v", firstPage, err)
	}

	if firstPage.Actions[0].ID != 23 || firstPage.Actions[19].ID != 4 {
		t.Errorf("✗ pending ordering: %+v", firstPage.Actions)
	}

	secondPage, err := service.List(t.Context(), "", "", 2, 20)
	if err != nil || len(secondPage.Actions) != 20 {
		t.Fatalf("💣 unusable second page: %+v, %v", secondPage, err)
	}

	if secondPage.Actions[0].ID != 3 || secondPage.Actions[2].ID != 1 || secondPage.Actions[3].ID != 45 {
		t.Errorf("✗ ordering across pages: %+v", secondPage.Actions)
	}

	filteredPage, err := service.List(t.Context(), "delete", "completed", 1, 20)
	if err != nil || filteredPage.Total != 22 || filteredPage.Pending != 0 {
		t.Errorf("✗ shared filters: %+v, %v", filteredPage, err)
	}

	if !t.Failed() {
		t.Log("✓ pending operations precede completed operations across pages")
	}
}

// TestPageBounds verifies invariant #2: Page bounds.
// Test class: Core. Test kind: Permanent.
func TestPageBounds(t *testing.T) {
	homeDirectory := t.TempDir()
	t.Setenv("HOME", homeDirectory)
	t.Setenv("USERPROFILE", homeDirectory)

	service := OperationService{}
	for _, requestedPage := range []int{-1, 0, 1, 999999} {
		actionPage, err := service.List(t.Context(), "", "", requestedPage, 0)
		if err != nil || actionPage.Page != 1 || actionPage.PageSize != 20 || actionPage.Total != 0 || actionPage.Actions == nil {
			t.Errorf("✗ empty page %d: %+v, %v", requestedPage, actionPage, err)
		}
	}

	if !t.Failed() {
		t.Log("✓ empty pages normalize without losing the default page size")
	}
}
