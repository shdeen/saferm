// Package errs defines failures identifiable through errors.Is.
package errs

import (
	"errors"

	"github.com/shdeen/saferm/internal/appstrings"
)

// Errors for invalid input and for unreadable or unusable configuration.
//   - ErrRuleMalformed: a review pattern is not well formed, so every delete and move needs approval
var (
	ErrInputMissing  = errors.New(appstrings.Text("error.inputMissing"))
	ErrInputInvalid  = errors.New(appstrings.Text("error.inputInvalid"))
	ErrConfigRead    = errors.New(appstrings.Text("error.configRead"))
	ErrConfigDecode  = errors.New(appstrings.Text("error.configDecode"))
	ErrConfigWrite   = errors.New(appstrings.Text("error.configWrite"))
	ErrConfigMissing = errors.New(appstrings.Text("error.configMissing"))
	ErrConfigInvalid = errors.New(appstrings.Text("error.configInvalid"))
	ErrRuleMalformed = errors.New(appstrings.Text("error.ruleMalformed"))
	ErrHomeRead      = errors.New(appstrings.Text("error.homeRead"))
)

// Errors for resolving paths and for inspecting, transferring, and removing files.
//   - ErrFileOverlap: the source and destination are the same path, or one contains the other
//   - ErrFileUnsupported: the item is not a regular file, a directory, or a symbolic link
var (
	ErrPathResolve     = errors.New(appstrings.Text("error.pathResolve"))
	ErrFileInspect     = errors.New(appstrings.Text("error.fileInspect"))
	ErrFileTransfer    = errors.New(appstrings.Text("error.fileTransfer"))
	ErrFileRemove      = errors.New(appstrings.Text("error.fileRemove"))
	ErrFileExists      = errors.New(appstrings.Text("error.fileExists"))
	ErrFileMissing     = errors.New(appstrings.Text("error.fileMissing"))
	ErrFileOverlap     = errors.New(appstrings.Text("error.fileOverlap"))
	ErrFileUnsupported = errors.New(appstrings.Text("error.fileUnsupported"))
)

// Errors for the history database and for actions that cannot take an operation.
//   - ErrDatabaseResult: the filesystem operation succeeded, but its result was not recorded
//   - ErrActionResolved: the action's type or status does not permit the operation
//   - ErrActionBusy: the action is in progress, was interrupted, or changed after it was read
//   - ErrActionAmbiguous: the recorded paths do not show which outcome is safe
//   - ErrRestorePending: the occupying delete must be approved before restoring
//   - ErrBlockerDenied, ErrBlockerFailed: the blocker outcome that denies a dependent move
var (
	ErrDatabaseOpen    = errors.New(appstrings.Text("error.databaseOpen"))
	ErrDatabaseRead    = errors.New(appstrings.Text("error.databaseRead"))
	ErrDatabaseWrite   = errors.New(appstrings.Text("error.databaseWrite"))
	ErrDatabaseClose   = errors.New(appstrings.Text("error.databaseClose"))
	ErrDatabaseResult  = errors.New(appstrings.Text("error.databaseResult"))
	ErrActionMissing   = errors.New(appstrings.Text("error.actionMissing"))
	ErrActionResolved  = errors.New(appstrings.Text("error.actionResolved"))
	ErrActionBusy      = errors.New(appstrings.Text("error.actionBusy"))
	ErrActionAmbiguous = errors.New(appstrings.Text("error.actionAmbiguous"))
	ErrRestorePending  = errors.New(appstrings.Text("error.restorePending"))
	ErrBlockerDenied   = errors.New(appstrings.Text("error.blockerDenied"))
	ErrBlockerFailed   = errors.New(appstrings.Text("error.blockerFailed"))
)

// Errors for confirmation, cancellation, output, notifications, and the desktop
// programs.
//   - ErrConfirmation: the operation waits for the caller's confirmation
//   - ErrBatchPartial: an operation on several items failed for at least one of them
var (
	ErrConfirmation        = errors.New(appstrings.Text("error.confirmation"))
	ErrOutputWrite         = errors.New(appstrings.Text("error.outputWrite"))
	ErrTemplateRender      = errors.New(appstrings.Text("error.templateRender"))
	ErrInputRead           = errors.New(appstrings.Text("error.inputRead"))
	ErrOperationCanceled   = errors.New(appstrings.Text("error.operationCanceled"))
	ErrBatchPartial        = errors.New(appstrings.Text("error.batchPartial"))
	ErrNotificationSend    = errors.New(appstrings.Text("error.notificationSend"))
	ErrNotificationReceive = errors.New(appstrings.Text("error.notificationReceive"))
	ErrNotifierSetup       = errors.New(appstrings.Text("error.notifierSetup"))
	ErrNotifierStop        = errors.New(appstrings.Text("error.notifierStop"))
	ErrApplicationLaunch   = errors.New(appstrings.Text("error.applicationLaunch"))
)
