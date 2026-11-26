package main

import (
	"cmp"
	"context"
	"slices"

	"github.com/shdeen/saferm/internal/saferm"
	"github.com/shdeen/saferm/internal/store"
)

// Page sizes that List applies.
const (
	defaultPageSize = 20
	maximumPageSize = 100
)

// ActionPage contains one page of the actions that match the List filters.
//
// Note: the fields are listed by their Go names. The generated TypeScript
// bindings spell the same fields in a different letter case.
//   - Total: the number of matching actions across all pages
//   - Pending: the number of matching actions that are pending
//   - Page: the 1-based page number, after clamping
//   - PageSize: the page size that List applied
type ActionPage struct {
	Actions  []store.Action `json:"actions"`
	Total    int            `json:"total"`
	Pending  int            `json:"pending"`
	Page     int            `json:"page"`
	PageSize int            `json:"pageSize"`
}

// List returns one page of the actions that match the type and status filters.
// Pending actions come first; pending actions and the remaining actions are each
// ordered newest first. A nonpositive page size becomes 20, a page size above
// 100 becomes 100, and the page number is clamped to the available pages.
func (*OperationService) List(ctx context.Context, actionType, status string, page, pageSize int) (ActionPage, error) {
	result, err := saferm.Run(ctx, &saferm.Request{Command: saferm.List, Type: actionType, Status: status})
	if err != nil {
		return ActionPage{}, err
	}

	if pageSize <= 0 {
		pageSize = defaultPageSize
	}

	pageSize = min(pageSize, maximumPageSize)
	pageCount := max(1, (len(result.Actions)+pageSize-1)/pageSize)
	page = min(max(1, page), pageCount)
	pendingCount := 0

	for actionIndex := range result.Actions {
		if result.Actions[actionIndex].Status == store.Pending {
			pendingCount++
		}
	}

	slices.SortFunc(result.Actions, compareActions)

	startIndex := (page - 1) * pageSize
	endIndex := min(startIndex+pageSize, len(result.Actions))

	return ActionPage{Actions: result.Actions[startIndex:endIndex], Total: len(result.Actions), Pending: pendingCount, Page: page, PageSize: pageSize}, nil
}

// compareActions orders pending actions before all other actions, and orders
// the actions within each of those two groups by descending ID.
//
//nolint:gocritic // slices.SortFunc requires the slice element type as value parameters.
func compareActions(firstAction, secondAction store.Action) int {
	firstPending := firstAction.Status == store.Pending
	secondPending := secondAction.Status == store.Pending

	if firstPending != secondPending {
		if firstPending {
			return -1
		}

		return 1
	}

	return cmp.Compare(secondAction.ID, firstAction.ID)
}
