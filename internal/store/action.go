package store

import (
	"strings"
	"time"
)

// Action types and decisions are stable values in action history.
const (
	DeleteType      = "delete"
	MoveType        = "move"
	ApproveDecision = "approve"
	DenyDecision    = "deny"
)

// Recorded action statuses are stable database and API values.
//   - Completed: the delete or move was carried out
//   - Pending: the action awaits an approve or deny decision, and nothing was transferred
//   - Denied: the pending action was denied
//   - Restored: the item was returned to its source path
//   - Purged: the deleted item was permanently removed from the trash directory
//   - Failed: the most recent operation on the action failed
const (
	Completed = 1
	Pending   = 2
	Denied    = 3
	Restored  = 4
	Purged    = 5
	Failed    = 6
)

// Action is the history record of one delete or move.
//
// Note: the fields are listed by their Go names. The generated TypeScript
// bindings spell the same fields in a different letter case.
//   - Type: DeleteType or MoveType
//   - Status: one of the recorded action statuses
//   - Source: the resolved path that the item was deleted or moved from
//   - Destination: the resolved path that a move transfers the item to
//   - TrashID: the name of a deleted item inside the trash directory
//   - TrashPath: the trash directory that was configured when the delete was requested
//   - UpdatedAt: when the record last changed; Claim, Finish, and Delete reject an Action whose UpdatedAt is out of date
//   - ResolvedAt: when the most recent operation on the action finished
//   - ReviewRule: the review pattern that made the action need approval
//   - BlockerID: the occupying delete a move waits for while that delete is pending; zero means none, and the reference never changes
//   - Decision: ApproveDecision or DenyDecision, once the action is decided
//   - Failure: the diagnostics of every failed operation on the action, one per line
//   - Operation: the operation that was most recently started on the action
//   - InProgress: whether an operation has claimed the action and not finished, because it is running or was interrupted
type Action struct {
	ID          int64     `json:"id"`
	Type        string    `json:"type"`
	Status      int       `json:"status"`
	Source      string    `json:"source"`
	Destination string    `json:"destination,omitempty"`
	TrashID     string    `json:"trashID,omitempty"`
	TrashPath   string    `json:"trashPath,omitempty"`
	Reason      string    `json:"reason"`
	Requester   string    `json:"requester,omitempty"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	ResolvedAt  time.Time `json:"resolvedAt,omitzero"`
	ReviewRule  string    `json:"reviewRule,omitempty"`
	BlockerID   int64     `json:"blockerID,omitempty"`
	Decision    string    `json:"decision,omitempty"`
	Failure     string    `json:"failure,omitempty"`
	Operation   string    `json:"operation,omitempty"`
	InProgress  bool      `json:"inProgress"`
}

// Resolve sets the in-memory outcome of an operation, clears its busy mark,
// and appends a non-nil diagnostic with the operation and resolution time.
// It does not save the action; callers persist it through History.
func (action *Action) Resolve(status int, diagnostic error) {
	action.Status, action.InProgress = status, false
	action.UpdatedAt = time.Now().UTC()
	action.ResolvedAt = action.UpdatedAt

	if diagnostic != nil {
		action.Failure = strings.TrimPrefix(action.Failure+"\n"+formatTime(action.ResolvedAt)+" "+action.Operation+": "+diagnostic.Error(), "\n")
	}
}
