package main

import (
	"context"

	"github.com/shdeen/saferm/internal/saferm"
	"github.com/shdeen/saferm/internal/store"
)

// OperationService lets the frontend run SafeRM operations and edit settings.
type OperationService struct{}

// OperationResponse carries an operation's result together with its error
// message, which is empty on success. A Go error return would make Wails reject
// the call and discard the result.
type OperationResponse struct {
	Result saferm.Result `json:"result"`
	Error  string        `json:"error"`
}

// Run carries out the request through saferm.Run and returns the result with
// any error as text.
func (*OperationService) Run(ctx context.Context, request *saferm.Request) OperationResponse {
	result, err := saferm.Run(ctx, request)

	response := OperationResponse{Result: result}
	if err != nil {
		response.Error = err.Error()
	}

	return response
}

// Statuses returns each action status code keyed by its name.
func (*OperationService) Statuses() map[string]int {
	return map[string]int{
		"completed": store.Completed,
		"pending":   store.Pending,
		"denied":    store.Denied,
		"restored":  store.Restored,
		"purged":    store.Purged,
		"failed":    store.Failed,
	}
}
