// Package store owns the SQLite action history and atomic record transitions.
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/shdeen/saferm/internal/errs"
	"github.com/shdeen/saferm/internal/filesystem"
	_ "modernc.org/sqlite" // Registers the SQLite driver used by database/sql.
)

// schema is the SQL that creates the history tables.
//
//go:embed schema.sql
var schema string

// databaseBusyTimeout is how long a statement waits for a database that another
// connection has locked.
const databaseBusyTimeout = 5 * time.Second

// selectActions selects every Action field, in the order that scanAction reads
// them, from each action joined with its deleted or moved record.
const selectActions = `SELECT a.action_id,a.action_type,a.status,a.reason,a.requester,a.description,
 a.created_at,a.updated_at,a.resolved_at,a.review_rule,a.decision,a.failure,a.operation,a.in_progress,a.blocker_id,
 COALESCE(d.source_path,m.source_path),COALESCE(d.trash_id,''),COALESCE(d.trash_path,''),COALESCE(m.destination_path,'')
 FROM actions a LEFT JOIN deleted d USING(action_id) LEFT JOIN moved m USING(action_id)`

// History reads and writes the action history through one database connection.
type History struct {
	// database is the open history database.
	database *sql.DB
	// path is the location of the database file.
	path string
}

// Open opens the history database at the path, creating the file, its
// directory, and the history tables when they are absent. It upgrades existing
// tables transactionally, serializing concurrent openers before inspection.
func Open(ctx context.Context, path string) (*History, error) {
	if err := os.MkdirAll(filepath.Dir(path), filesystem.PrivateDirectoryMode); err != nil {
		return nil, fmt.Errorf("%q: %w: %w", path, errs.ErrDatabaseOpen, err)
	}

	databasePath := filepath.ToSlash(path)
	if volume := filepath.VolumeName(path); len(volume) == 2 && volume[1] == ':' {
		// SQLite URI paths put a slash before a Windows drive letter.
		databasePath = "/" + databasePath
	}

	databaseURL := url.URL{Scheme: "file", Path: databasePath}
	query := url.Values{"_pragma": []string{"foreign_keys(1)", fmt.Sprintf("busy_timeout(%d)", databaseBusyTimeout.Milliseconds())}, "_txlock": []string{"immediate"}}
	databaseURL.RawQuery = query.Encode()

	database, err := sql.Open("sqlite", databaseURL.String())
	if err != nil {
		return nil, fmt.Errorf("%q: %w: %w", path, errs.ErrDatabaseOpen, err)
	}

	database.SetMaxOpenConns(1)

	history := &History{database: database, path: path}
	if err = history.upgrade(ctx); err != nil {
		return nil, fmt.Errorf("%q: %w: %w", path, errs.ErrDatabaseOpen, errors.Join(err, database.Close()))
	}

	return history, nil
}

// Close releases the database connection.
func (history *History) Close() error {
	if err := history.database.Close(); err != nil {
		return fmt.Errorf("%q: %w: %w", history.path, errs.ErrDatabaseClose, err)
	}

	return nil
}

// Create inserts the action together with its deleted or moved record, in one
// transaction. On success, it sets the action's ID, CreatedAt, and UpdatedAt.
func (history *History) Create(ctx context.Context, action *Action) (resultErr error) {
	transaction, err := history.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%q: %w: %w", action.Source, errs.ErrDatabaseWrite, err)
	}
	defer rollback(transaction, history.path, &resultErr)

	createdAction := *action
	if err := insertAction(ctx, transaction, &createdAction); err != nil {
		return err
	}

	if err = transaction.Commit(); err != nil {
		return fmt.Errorf("%q: %w: %w", action.Source, errs.ErrDatabaseWrite, err)
	}

	*action = createdAction

	return nil
}

// CreatePair saves a pending move and its occupying delete in one transaction,
// giving the move the lower ID and recording the delete as its blocker. It
// updates the supplied actions only after commit; failures expose no new IDs.
func (history *History) CreatePair(ctx context.Context, moved, deleted *Action) (resultErr error) {
	transaction, err := history.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%q: %w: %w", moved.Source, errs.ErrDatabaseWrite, err)
	}
	defer rollback(transaction, history.path, &resultErr)

	createdMove, createdDelete := *moved, *deleted
	if err := insertAction(ctx, transaction, &createdMove); err != nil {
		return err
	}

	if err := insertAction(ctx, transaction, &createdDelete); err != nil {
		return err
	}

	createdMove.BlockerID = createdDelete.ID
	if _, err = transaction.ExecContext(ctx, `UPDATE actions SET blocker_id=? WHERE action_id=?`, createdMove.BlockerID, createdMove.ID); err != nil {
		return fmt.Errorf("%q: %w: %w", moved.Source, errs.ErrDatabaseWrite, err)
	}

	if err = transaction.Commit(); err != nil {
		return fmt.Errorf("%q: %w: %w", moved.Source, errs.ErrDatabaseWrite, err)
	}

	*moved, *deleted = createdMove, createdDelete

	return nil
}

// List returns the actions in creation order. A nonempty type and a nonzero
// status each restrict the actions that it returns.
func (history *History) List(ctx context.Context, actionType string, status int) ([]Action, error) {
	actionRows, err := history.database.QueryContext(ctx, selectActions+` WHERE (?='' OR a.action_type=?) AND (?=0 OR a.status=?) ORDER BY a.action_id`, actionType, actionType, status, status)
	if err != nil {
		return nil, fmt.Errorf("%q, %q, %d: %w: %w", history.path, actionType, status, errs.ErrDatabaseRead, err)
	}

	return readActions(actionRows, history.path)
}

// PendingDependents returns pending actions that name blockerID, in creation
// order. The query is drained and closed before returning so callers can claim
// and finish the returned actions on the history's one connection.
func (history *History) PendingDependents(ctx context.Context, blockerID int64) ([]Action, error) {
	actionRows, err := history.database.QueryContext(ctx, selectActions+` WHERE a.blocker_id=? AND a.status=? ORDER BY a.action_id`, blockerID, Pending)
	if err != nil {
		return nil, fmt.Errorf("%d: %w: %w", blockerID, errs.ErrDatabaseRead, err)
	}

	return readActions(actionRows, history.path)
}

// Get returns the action that has the ID, or ErrActionMissing.
func (history *History) Get(ctx context.Context, id int64) (Action, error) {
	action, err := scanAction(history.database.QueryRowContext(ctx, selectActions+` WHERE a.action_id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Action{}, fmt.Errorf("%d: %w", id, errs.ErrActionMissing)
	}

	return action, err
}

// Claim marks the stored action as in progress with the supplied operation and
// decision, and updates the supplied action to match. It returns ErrActionBusy
// when the stored action is already in progress or has changed since the
// supplied action was read.
func (history *History) Claim(ctx context.Context, action *Action, operation, decision string) error {
	updated := time.Now().UTC()

	result, err := history.database.ExecContext(ctx, `UPDATE actions SET in_progress=1,operation=?,decision=?,updated_at=?
 WHERE action_id=? AND updated_at=? AND in_progress=0`, operation, decision, formatTime(updated), action.ID, formatTime(action.UpdatedAt))
	if err != nil {
		return fmt.Errorf("%d: %w: %w", action.ID, errs.ErrDatabaseWrite, err)
	}

	if err := changedAction(result, action.ID); err != nil {
		return err
	}

	action.InProgress, action.Operation, action.Decision, action.UpdatedAt = true, operation, decision, updated

	return nil
}

// Finish records the final status of an operation on the action, clears the
// in-progress mark, and updates the supplied action to match. It appends a
// non-nil failure to the action's diagnostics. An action that was never stored
// is created with that outcome instead. Finish returns ErrActionBusy when the
// stored action is not in progress or has changed since it was claimed.
func (history *History) Finish(ctx context.Context, action *Action, status int, failure error) error {
	resolvedAction := *action
	resolvedAction.Resolve(status, failure)

	if action.ID == 0 {
		*action = resolvedAction

		return history.Create(ctx, action)
	}

	result, err := history.database.ExecContext(ctx, `UPDATE actions SET status=?,failure=?,in_progress=0,updated_at=?,resolved_at=?
 WHERE action_id=? AND updated_at=? AND in_progress=1`, status, resolvedAction.Failure, formatTime(resolvedAction.UpdatedAt), formatTime(resolvedAction.ResolvedAt), action.ID, formatTime(action.UpdatedAt))
	if err != nil {
		return fmt.Errorf("%d: %w: %w", action.ID, errs.ErrDatabaseWrite, err)
	}

	if err := changedAction(result, action.ID); err != nil {
		return err
	}

	*action = resolvedAction

	return nil
}

// closeRows closes the query result and joins a failure to close it into the
// error that resultErr points to.
func closeRows(actionRows *sql.Rows, databasePath string, resultErr *error) {
	if err := actionRows.Close(); err != nil {
		*resultErr = errors.Join(*resultErr, fmt.Errorf("%q: %w: %w", databasePath, errs.ErrDatabaseRead, err))
	}
}

// Delete removes the stored action and its deleted or moved record. It returns
// ErrActionBusy when the stored action is in progress or has changed since the
// supplied action was read.
func (history *History) Delete(ctx context.Context, action *Action) (resultErr error) {
	transaction, err := history.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%d: %w: %w", action.ID, errs.ErrDatabaseWrite, err)
	}
	defer rollback(transaction, history.path, &resultErr)

	result, err := transaction.ExecContext(ctx, `DELETE FROM actions WHERE action_id=? AND updated_at=? AND in_progress=0`, action.ID, formatTime(action.UpdatedAt))
	if err != nil {
		return fmt.Errorf("%d: %w: %w", action.ID, errs.ErrDatabaseWrite, err)
	}

	if err := changedAction(result, action.ID); err != nil {
		return err
	}

	if err = transaction.Commit(); err != nil {
		return fmt.Errorf("%d: %w: %w", action.ID, errs.ErrDatabaseWrite, err)
	}

	return nil
}

// rollback rolls back the transaction unless it has already ended, and joins a
// rollback failure into the error that resultErr points to.
func rollback(transaction *sql.Tx, databasePath string, resultErr *error) {
	if err := transaction.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		*resultErr = errors.Join(*resultErr, fmt.Errorf("%q: %w: %w", databasePath, errs.ErrDatabaseWrite, err))
	}
}

// upgrade creates or extends the actual schema while an immediate transaction
// holds SQLite's write reservation. Existing records and the ID sequence stay
// in their original tables.
func (history *History) upgrade(ctx context.Context) (resultErr error) {
	transaction, err := history.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%q: %w: %w", history.path, errs.ErrDatabaseWrite, err)
	}
	defer rollback(transaction, history.path, &resultErr)

	if _, err = transaction.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("%q: %w: %w", history.path, errs.ErrDatabaseWrite, err)
	}

	var blockerColumns int
	if err = transaction.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('actions') WHERE name='blocker_id'`).Scan(&blockerColumns); err != nil {
		return fmt.Errorf("%q: %w: %w", history.path, errs.ErrDatabaseRead, err)
	}

	if blockerColumns == 0 {
		if _, err = transaction.ExecContext(ctx, `ALTER TABLE actions ADD COLUMN blocker_id INTEGER NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("%q: %w: %w", history.path, errs.ErrDatabaseWrite, err)
		}
	}

	if err = transaction.Commit(); err != nil {
		return fmt.Errorf("%q: %w: %w", history.path, errs.ErrDatabaseWrite, err)
	}

	return nil
}

// insertAction inserts an action and its operation-specific details within the
// caller's transaction. Its action must be a private copy until commit succeeds.
func insertAction(ctx context.Context, transaction *sql.Tx, action *Action) error {
	action.CreatedAt = time.Now().UTC()
	action.UpdatedAt = action.CreatedAt

	resolvedAt := ""
	if !action.ResolvedAt.IsZero() {
		resolvedAt = formatTime(action.ResolvedAt)
	}

	result, err := transaction.ExecContext(ctx, `INSERT INTO actions(action_type,status,reason,requester,description,created_at,updated_at,review_rule,operation,in_progress,resolved_at,failure,decision,blocker_id)
 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, action.Type, action.Status, action.Reason, action.Requester, action.Description, formatTime(action.CreatedAt), formatTime(action.UpdatedAt), action.ReviewRule, action.Operation, action.InProgress, resolvedAt, action.Failure, action.Decision, action.BlockerID)
	if err != nil {
		return fmt.Errorf("%q: %w: %w", action.Source, errs.ErrDatabaseWrite, err)
	}

	action.ID, err = result.LastInsertId()
	if err != nil {
		return fmt.Errorf("%q: %w: %w", action.Source, errs.ErrDatabaseWrite, err)
	}

	if action.Type == DeleteType {
		_, err = transaction.ExecContext(ctx, `INSERT INTO deleted(action_id,source_path,trash_id,trash_path) VALUES(?,?,?,?)`, action.ID, action.Source, action.TrashID, action.TrashPath)
	} else {
		_, err = transaction.ExecContext(ctx, `INSERT INTO moved(action_id,source_path,destination_path) VALUES(?,?,?)`, action.ID, action.Source, action.Destination)
	}

	if err != nil {
		return fmt.Errorf("%q: %w: %w", action.Source, errs.ErrDatabaseWrite, err)
	}

	return nil
}

// readActions drains and closes a query before returning its action records.
func readActions(actionRows *sql.Rows, databasePath string) (actions []Action, resultErr error) {
	defer closeRows(actionRows, databasePath, &resultErr)

	actions = make([]Action, 0)

	for actionRows.Next() {
		action, err := scanAction(actionRows)
		if err != nil {
			return nil, err
		}

		actions = append(actions, action)
	}

	if err := actionRows.Err(); err != nil {
		return nil, fmt.Errorf("%q: %w: %w", databasePath, errs.ErrDatabaseRead, err)
	}

	return actions, nil
}

// changedAction returns ErrActionBusy unless the statement changed exactly one
// stored action.
func changedAction(result sql.Result, actionID int64) error {
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%d: %w: %w", actionID, errs.ErrDatabaseWrite, err)
	}

	if count != 1 {
		return fmt.Errorf("%d: %w", actionID, errs.ErrActionBusy)
	}

	return nil
}

// sqlScanner is the Scan method that *sql.Row and *sql.Rows share.
type sqlScanner interface {
	Scan(destinations ...any) error
}

// scanAction reads one action from a result of the selectActions query and
// parses its timestamps.
func scanAction(scanner sqlScanner) (Action, error) {
	var (
		action                     Action
		created, updated, resolved string
	)

	err := scanner.Scan(&action.ID, &action.Type, &action.Status, &action.Reason, &action.Requester, &action.Description,
		&created, &updated, &resolved, &action.ReviewRule, &action.Decision, &action.Failure, &action.Operation, &action.InProgress, &action.BlockerID,
		&action.Source, &action.TrashID, &action.TrashPath, &action.Destination)
	if err != nil {
		return Action{}, fmt.Errorf("%w: %w", errs.ErrDatabaseRead, err)
	}

	for _, timestamp := range []struct {
		value  string
		target *time.Time
	}{{created, &action.CreatedAt}, {updated, &action.UpdatedAt}} {
		*timestamp.target, err = time.Parse(time.RFC3339Nano, timestamp.value)
		if err != nil {
			return Action{}, fmt.Errorf("%d: %w: %w", action.ID, errs.ErrDatabaseRead, err)
		}
	}

	if resolved != "" {
		action.ResolvedAt, err = time.Parse(time.RFC3339Nano, resolved)
		if err != nil {
			return Action{}, fmt.Errorf("%d: %w: %w", action.ID, errs.ErrDatabaseRead, err)
		}
	}

	return action, nil
}

// formatTime formats a timestamp in UTC, in the form that the database stores.
func formatTime(timestamp time.Time) string { return timestamp.UTC().Format(time.RFC3339Nano) }
