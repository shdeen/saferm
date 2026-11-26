// Package saferm exposes SafeRM operations independently of their user interface.
package saferm

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/shdeen/saferm/internal/appstrings"
	"github.com/shdeen/saferm/internal/config"
	"github.com/shdeen/saferm/internal/errs"
	"github.com/shdeen/saferm/internal/store"
)

// Commands name the operations that Run accepts.
//   - Delete: moves an item into the trash directory
//   - Dispose: approves or denies a pending action
//   - Restore: returns a deleted or moved item to its source path
//   - Purge: permanently removes deleted items from the trash directory
//   - Cleanup: deletes the history records that are eligible for removal
//   - Config: returns the settings or installs the default configuration file
const (
	Delete  = store.DeleteType
	Move    = store.MoveType
	List    = "list"
	View    = "view"
	Dispose = "dispose"
	Restore = "restore"
	Purge   = "purge"
	Cleanup = "cleanup"
	Config  = "config"
)

// Request names one command and supplies its inputs.
//   - ExtraArguments: surplus positional arguments, which make the request invalid
//   - ID: the decimal ID of the action for view, dispose, or restore
//   - IDs: the decimal IDs of the actions for purge
//   - Decision: store.ApproveDecision or store.DenyDecision, for dispose
//   - Type: the action type that list returns, or empty for both types
//   - Status: the status name or code that list returns, or empty for every status
//   - Overwrite: whether move or restore may replace an occupant through a recoverable delete
//   - Force: whether move or restore skips replacement confirmation; review still applies
//   - Orphans: whether cleanup also deletes records whose files are missing
//   - Default: whether config returns the embedded defaults instead of the loaded settings
//   - Setup: whether config installs the default configuration file
//   - Reset: whether that installation may replace an existing configuration file
//   - Confirmed: whether the caller confirmed the Confirmation that an earlier Run returned
type Request struct {
	ExtraArguments []string
	Command        string
	Source         string
	Destination    string
	Reason         string
	Requester      string
	Description    string
	ID             string
	IDs            []string
	Decision       string
	Type           string
	Status         string
	Overwrite      bool
	Force          bool
	Orphans        bool
	Default        bool
	Setup          bool
	Reset          bool
	Confirmed      bool
}

// Result holds what one operation produced. A Result that accompanies an error
// retains recorded actions and attempted transfers, including partial failures.
//
// Note: the fields are listed by their Go names. The generated TypeScript
// bindings spell the same fields in a different letter case.
//   - Actions: the actions that the operation recorded, changed, or looked up
//   - Config: the settings that a config operation returns
//   - Document: the default configuration document, when config was asked for the defaults
//   - Notice: a message for the user
//   - Removed: the IDs of the history records that cleanup deleted
//   - Problems: the failure of each action that purge or cleanup could not process, and the warning for each directory that the review check of a delete or move could not read
//   - Confirmation: the confirmation that the operation needs before it can proceed
type Result struct {
	Actions      []store.Action   `json:"actions,omitempty"`
	Config       *config.Settings `json:"config,omitempty"`
	Document     string           `json:"document,omitempty"`
	Notice       string           `json:"notice,omitempty"`
	Removed      []int64          `json:"removed,omitempty"`
	Problems     []string         `json:"problems,omitempty"`
	Confirmation *Confirmation    `json:"confirmation,omitempty"`
}

// mergeActions retains each action once, replacing an earlier state when a
// later decision returns the same ID. Actions have committed IDs here.
func (result *Result) mergeActions(actions []store.Action) {
	for incomingIndex := range actions {
		action := &actions[incomingIndex]
		actionIndex := -1

		for index := range result.Actions {
			if result.Actions[index].ID == action.ID {
				actionIndex = index

				break
			}
		}

		if actionIndex < 0 {
			result.Actions = append(result.Actions, *action)
		} else {
			result.Actions[actionIndex] = *action
		}
	}
}

// Confirmation describes a replacement or linked decision needing consent.
//
// Note: the fields are listed by their Go names. The generated TypeScript
// bindings spell the same fields in a different letter case.
//   - Prompt: the question template without a terminal answer hint; it may refer to .Path, .MoveID, and .BlockerID
//   - Path: the item that would be replaced
//   - DefaultYes: whether an empty answer gives consent
//   - MoveID: the move named by a combined decision, or zero for other questions
//   - BlockerID: that move's occupying delete, or zero for other questions
type Confirmation struct {
	Prompt     string `json:"prompt"`
	Path       string `json:"path"`
	DefaultYes bool   `json:"defaultYes"`
	MoveID     int64  `json:"moveID,omitempty"`
	BlockerID  int64  `json:"blockerID,omitempty"`
}

// Run validates and carries out one request and returns its result, which may
// accompany an error. Run does not modify the request. When the operation needs
// confirmation, Run changes nothing and returns ErrConfirmation with
// Result.Confirmation set; the caller proceeds by resubmitting the request with
// Confirmed set.
//
// For every command except config, Run opens the history database in the
// SafeRM directory, creating it when it is absent.
func Run(ctx context.Context, request *Request) (result Result, resultErr error) {
	if request == nil {
		return Result{}, errs.ErrInputMissing
	}

	if err := request.validate(); err != nil {
		return Result{}, err
	}

	if err := ctx.Err(); err != nil {
		return Result{}, fmt.Errorf("%w: %w", errs.ErrOperationCanceled, err)
	}

	if request.Command == Config && request.Default {
		return defaultConfig()
	}

	home, err := config.Home()
	if err != nil {
		return Result{}, err
	}

	if request.Command == Config {
		return configure(ctx, request, home)
	}

	settings, err := config.Load(home)
	if err != nil {
		return Result{}, err
	}

	history, err := store.Open(ctx, filepath.Join(home, config.Directory, config.Database))
	if err != nil {
		return Result{}, err
	}
	defer closeHistory(history, &resultErr)

	return runOperation(ctx, request, home, settings, history)
}

// closeHistory closes the history and joins a failure to close it into the
// error that resultErr points to.
func closeHistory(history *store.History, resultErr *error) {
	*resultErr = errors.Join(*resultErr, history.Close())
}

// runOperation carries out a validated request for any command except config.
// It returns ErrActionBusy for a dispose or restore of an action in progress.
func runOperation(ctx context.Context, request *Request, home string, settings config.Settings, history *store.History) (Result, error) {
	switch request.Command {
	case Delete, Move:
		return createAction(ctx, request, home, settings, history)
	case List:
		status, err := parseStatus(request.Status)
		if err != nil {
			return Result{}, err
		}

		actions, err := history.List(ctx, request.Type, status)

		return Result{Actions: actions}, err
	case Cleanup:
		return cleanup(ctx, request, home, settings, history)
	case Purge:
		return purge(ctx, request, home, settings, history)
	default:
		actionID, err := parseID(request.ID)
		if err != nil {
			return Result{}, err
		}

		action, err := history.Get(ctx, actionID)
		if err != nil {
			return Result{}, err
		}

		if request.Command == View {
			return Result{Actions: []store.Action{action}}, nil
		}

		if action.InProgress {
			return Result{Actions: []store.Action{action}}, fmt.Errorf("%d: %w", actionID, errs.ErrActionBusy)
		}

		if request.Command == Dispose {
			return decide(ctx, request, home, settings, history, &action)
		}

		return restore(ctx, request, home, settings, history, &action)
	}
}

// validate checks that the request names a known command, has no extra
// arguments, and carries the inputs that the command requires. It returns
// ErrInputMissing or ErrInputInvalid.
func (request *Request) validate() error {
	if len(request.ExtraArguments) > 0 {
		return fmt.Errorf("%q: %w", request.ExtraArguments, errs.ErrInputInvalid)
	}

	switch request.Command {
	case Delete, Move:
		return request.validateCreation()
	case List:
		return request.validateFilters()
	case View, Dispose, Restore:
		return request.validateDecision()
	case Purge:
		return validateIDs(request.IDs)
	case Config:
		if (request.Reset && !request.Setup) || (request.Default && (request.Setup || request.Reset)) {
			return fmt.Errorf("%q: %w", Config, errs.ErrInputInvalid)
		}
	case Cleanup:
	default:
		return fmt.Errorf("%q: %w", request.Command, errs.ErrInputInvalid)
	}

	return nil
}

// validateCreation requires a source and a reason, and also a destination for
// a move.
func (request *Request) validateCreation() error {
	if err := requireInput("source", request.Source); err != nil {
		return err
	}

	if err := requireInput("reason", request.Reason); err != nil {
		return err
	}

	if request.Command == Move {
		return requireInput("destination", request.Destination)
	}

	return nil
}

// validateFilters checks the list filters. The type must be empty, delete, or
// move, and the status must be one that parseStatus accepts.
func (request *Request) validateFilters() error {
	if request.Type != "" && request.Type != Delete && request.Type != Move {
		return fmt.Errorf("%q: %w", request.Type, errs.ErrInputInvalid)
	}

	_, err := parseStatus(request.Status)

	return err
}

// validateDecision requires a valid action ID for view, dispose, and restore,
// and also a decision of approve or deny for dispose.
func (request *Request) validateDecision() error {
	if _, err := parseID(request.ID); err != nil {
		return err
	}

	if request.Command == Dispose {
		if err := requireInput("decision", request.Decision); err != nil {
			return err
		}

		if request.Decision != store.ApproveDecision && request.Decision != store.DenyDecision {
			return fmt.Errorf("%q: %w", request.Decision, errs.ErrInputInvalid)
		}
	}

	return nil
}

// validateIDs requires at least one ID and requires every ID to be valid.
func validateIDs(ids []string) error {
	if len(ids) == 0 {
		return fmt.Errorf("%q: %w", "id", errs.ErrInputMissing)
	}

	for _, id := range ids {
		if _, err := parseID(id); err != nil {
			return err
		}
	}

	return nil
}

// requireInput returns ErrInputMissing, labeled with the input's name, when the
// value is blank.
func requireInput(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%q: %w", name, errs.ErrInputMissing)
	}

	return nil
}

// parseID converts decimal text to an action ID. It returns ErrInputMissing
// for blank text and ErrInputInvalid for text that is not a positive integer.
func parseID(value string) (int64, error) {
	if err := requireInput("id", value); err != nil {
		return 0, err
	}

	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%q: %w", value, errs.ErrInputInvalid)
	}

	return id, nil
}

// parseStatus converts a status name or a decimal status code to the status
// code. It returns zero, which stands for every status, for empty text.
func parseStatus(value string) (int, error) {
	if value == "" {
		return 0, nil
	}

	for status := store.Completed; status <= store.Failed; status++ {
		name := appstrings.Text("status." + strconv.Itoa(status))
		if value == name || value == strconv.Itoa(status) {
			return status, nil
		}
	}

	return 0, fmt.Errorf("%q: %w", value, errs.ErrInputInvalid)
}
