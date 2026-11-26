package main

// Invariants tested:
//  1. Fresh decisions: A native answer uses current records and reports failures without discarding prior diagnostics.

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/shdeen/saferm/internal/appstrings"
	"github.com/shdeen/saferm/internal/config"
	"github.com/shdeen/saferm/internal/errs"
	"github.com/shdeen/saferm/internal/saferm"
	"github.com/shdeen/saferm/internal/store"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// TestChangedNotificationRequest verifies invariant #1: Fresh decisions.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// Yes after another decision reports a resolved action. Yes after the moving
// source disappears preserves the completed delete and pending move. Both
// errors appear in a native dialog and append to the existing diagnostic log.
func TestChangedNotificationRequest(t *testing.T) {
	application.New(application.Options{Name: appstrings.Text("notification.appName")})

	for _, change := range []string{"resolved", "missing_source"} {
		t.Run(change, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			source := filepath.Join(home, "source")

			destination := filepath.Join(home, ".occupant")
			for path, contents := range map[string]string{source: "moving contents", destination: "occupying contents"} {
				if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
					t.Fatalf("💣 create files: %v", err)
				}
			}

			writeReviewRules(t, home, destination)

			created, err := saferm.Run(t.Context(), &saferm.Request{Command: saferm.Move, Source: source, Destination: destination, Reason: "replace", Overwrite: true, Force: true})
			if err != nil || len(created.Actions) != 2 {
				t.Fatalf("💣 prepare pair: %+v, %v", created, err)
			}

			moved, deleted := created.Actions[0], created.Actions[1]

			logPath := filepath.Join(home, config.Directory, "notifications.log")
			if err = os.WriteFile(logPath, []byte("previous diagnostic\n"), 0o600); err != nil {
				t.Fatalf("💣 diagnostic history: %v", err)
			}

			var dialog *application.MessageDialog

			nativeShow := showMessageDialog
			showMessageDialog = func(message *application.MessageDialog) { dialog = message }

			t.Cleanup(func() { showMessageDialog = nativeShow })

			service := &Service{}
			service.respond(notifications.NotificationResult{Response: notifications.NotificationResponse{ID: strconv.FormatInt(moved.ID, 10), ActionIdentifier: store.ApproveDecision}})

			if dialog == nil || dialog.DialogType != application.QuestionDialogType {
				t.Fatalf("💣 no native question available: %+v", dialog)
			}

			expectedError := errs.ErrActionResolved

			if change == "resolved" {
				_, err = saferm.Run(t.Context(), &saferm.Request{Command: saferm.Dispose, ID: strconv.FormatInt(deleted.ID, 10), Decision: store.DenyDecision})
			} else {
				expectedError = errs.ErrFileInspect
				_, err = saferm.Run(t.Context(), &saferm.Request{Command: saferm.Delete, Source: source, Reason: "remove source before consent"})
			}

			if err != nil {
				t.Fatalf("💣 change pending request: %v", err)
			}

			for _, button := range dialog.Buttons {
				if button.Label == appstrings.Text("dialog.yes") && button.Callback != nil {
					button.Callback()
				}
			}

			if dialog.DialogType != application.ErrorDialogType || !strings.Contains(dialog.Message, expectedError.Error()) {
				t.Errorf("✗ current failure not shown: %+v", dialog.MessageDialogOptions)
			}

			for _, action := range created.Actions {
				status := store.Denied
				if change == "missing_source" {
					status = store.Completed
					if action.ID == moved.ID {
						status = store.Pending
					}
				}

				inspected, readErr := saferm.Run(t.Context(), &saferm.Request{Command: saferm.View, ID: strconv.FormatInt(action.ID, 10)})
				if readErr != nil || len(inspected.Actions) != 1 || inspected.Actions[0].Status != status {
					t.Errorf("✗ current state lost: %+v, %v", inspected, readErr)
				}
			}

			preservedPath := deleted.Source
			if change == "missing_source" {
				preservedPath = filepath.Join(deleted.TrashPath, deleted.TrashID)
			}

			contents, readErr := os.ReadFile(preservedPath)
			if readErr != nil || string(contents) != "occupying contents" {
				t.Errorf("✗ occupant lost after refusal: %q, %v", contents, readErr)
			}

			diagnostics, readErr := os.ReadFile(logPath)
			if readErr != nil || !strings.HasPrefix(string(diagnostics), "previous diagnostic\n") || !strings.Contains(string(diagnostics), expectedError.Error()) {
				t.Errorf("✗ diagnostic history lost: %q, %v", diagnostics, readErr)
			}

			if !t.Failed() {
				t.Log("✓ fresh consent preserves current outcomes and error history")
			}
		})
	}

	if !t.Failed() {
		t.Log("✓ native decisions reread current records")
	}
}
