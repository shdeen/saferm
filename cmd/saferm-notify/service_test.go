package main

// Invariants tested:
//  1. Notification content: A notification identifies the operation and filename, with optional requester.
//  2. Notification decisions: Approve and Deny perform the corresponding shared operation without a GUI window.
//  3. Linked consent: Native questions identify both actions and wait for Yes before deciding either.

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/shdeen/saferm/cmd/saferm/output"
	"github.com/shdeen/saferm/internal/appstrings"
	"github.com/shdeen/saferm/internal/saferm"
	"github.com/shdeen/saferm/internal/store"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// TestNotificationContent verifies invariant #1: Notification content.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// Delete and move notifications show their type and basename. Requester text
// is present only when supplied. The notification carries the operation ID.
func TestNotificationContent(t *testing.T) {
	for _, actionType := range []string{store.DeleteType, store.MoveType} {
		for _, requester := range []string{"", "Agent"} {
			t.Run(actionType+"/requester="+requester, func(t *testing.T) {
				action := store.Action{ID: 42, Type: actionType, Source: filepath.Join("private", "document.txt"), Requester: requester}

				options := requestNotification(&action)
				if options.ID != "42" || !strings.Contains(options.Title, "document.txt") || !strings.Contains(options.Title, appstrings.Text("operation."+actionType)) || options.CategoryID == "" {
					t.Errorf("✗ request is not identifiable: %+v", options)
				}

				if requester == "" && options.Body != "" || requester != "" && !strings.Contains(options.Body, requester) {
					t.Errorf("✗ optional requester: %+v", options)
				}

				if !t.Failed() {
					t.Log("✓ notification metadata is identifiable")
				}
			})
		}
	}

	if !t.Failed() {
		t.Log("✓ notification content and optional requester")
	}
}

// TestNotificationDecisions verifies invariant #2: Notification decisions.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// Approve executes a pending request and Deny preserves its file. Each decision
// is saved with the correct resulting status without starting the GUI.
func TestNotificationDecisions(t *testing.T) {
	for _, decision := range []string{store.ApproveDecision, store.DenyDecision} {
		t.Run(decision, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)

			source := filepath.Join(home, "document.txt")
			if err := os.Mkdir(filepath.Join(home, ".saferm"), 0o700); err != nil {
				t.Fatalf("💣 settings directory: %v", err)
			}

			settings := fmt.Sprintf("trash-path = %q\nrequire-review = [%q]\n", filepath.Join(home, "trash"), source)
			if err := os.WriteFile(filepath.Join(home, ".saferm", "config.toml"), []byte(settings), 0o600); err != nil {
				t.Fatalf("💣 settings: %v", err)
			}

			if err := os.WriteFile(source, []byte("document contents"), 0o600); err != nil {
				t.Fatalf("💣 source: %v", err)
			}

			created, err := saferm.Run(t.Context(), &saferm.Request{Command: saferm.Delete, Source: source, Reason: "remove"})
			if err != nil || len(created.Actions) != 1 {
				t.Fatalf("💣 request: %+v, %v", created, err)
			}

			actionID := strconv.FormatInt(created.Actions[0].ID, 10)
			service := &Service{}
			service.respond(notifications.NotificationResult{Response: notifications.NotificationResponse{ID: actionID, ActionIdentifier: decision}})

			result, err := saferm.Run(t.Context(), &saferm.Request{Command: saferm.View, ID: actionID})
			if err != nil || len(result.Actions) != 1 {
				t.Fatalf("💣 inspect decision: %+v, %v", result, err)
			}

			status := store.Denied
			if decision == store.ApproveDecision {
				status = store.Completed
			}

			if result.Actions[0].Status != status || result.Actions[0].Decision != decision {
				t.Errorf("✗ decision not performed: %+v", result.Actions[0])
			}

			_, sourceErr := os.Stat(source)
			if decision == store.ApproveDecision && !os.IsNotExist(sourceErr) || decision == store.DenyDecision && sourceErr != nil {
				t.Errorf("✗ decision file effect: %v", sourceErr)
			}

			if !t.Failed() {
				t.Log("✓ direct decision preserves the requested outcome")
			}
		})
	}

	if !t.Failed() {
		t.Log("✓ notification actions perform decisions directly")
	}
}

// TestLinkedNotificationConsent verifies invariant #3: Linked consent.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// Both notification decisions show the named move, delete, and literal path
// without a terminal answer hint. Both records and files remain unchanged until
// Yes resolves the linked decision. No and closing leave both requests pending.
func TestLinkedNotificationConsent(t *testing.T) {
	application.New(application.Options{Name: appstrings.Text("notification.appName")})

	for _, decision := range []string{store.ApproveDecision, store.DenyDecision} {
		for _, answer := range []string{"yes", "no", "close"} {
			t.Run(decision+"/"+answer, func(t *testing.T) {
				home := t.TempDir()
				t.Setenv("HOME", home)
				t.Setenv("USERPROFILE", home)
				source := filepath.Join(home, ".source")

				destination := filepath.Join(home, ".$& {{.MoveID}} (yN)")
				for path, contents := range map[string]string{source: "moving contents", destination: "occupying contents"} {
					if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
						t.Fatalf("💣 create files: %v", err)
					}
				}

				writeReviewRules(t, home, source, filepath.Join(home, ".$& [{][{].MoveID[}][}] (yN)"))

				created, err := saferm.Run(t.Context(), &saferm.Request{Command: saferm.Move, Source: source, Destination: destination, Reason: "replace", Overwrite: true, Force: true})
				if err != nil || len(created.Actions) != 2 {
					t.Fatalf("💣 prepare linked requests: %+v, %v", created, err)
				}

				moved, deleted := created.Actions[0], created.Actions[1]

				var dialog *application.MessageDialog

				nativeShow := showMessageDialog
				showMessageDialog = func(question *application.MessageDialog) { dialog = question }

				t.Cleanup(func() { showMessageDialog = nativeShow })

				service := &Service{}
				service.respond(notifications.NotificationResult{Response: notifications.NotificationResponse{ID: strconv.FormatInt(moved.ID, 10), ActionIdentifier: decision}})

				if dialog == nil {
					t.Errorf("✗ native question was not shown")

					return
				}

				promptKey := "prompt.blockedApprove"
				if decision == store.DenyDecision {
					promptKey = "prompt.blockedDeny"
				}

				question := saferm.Confirmation{MoveID: moved.ID, BlockerID: deleted.ID, Path: deleted.Source}

				prompt, err := output.RenderString(appstrings.Text(promptKey), question)
				if err != nil {
					t.Fatalf("💣 render expected question: %v", err)
				}

				if strings.HasSuffix(dialog.Message, appstrings.Text("prompt.defaultNoHint")) {
					t.Error("✗ native question includes the terminal answer hint")
				}

				if dialog.DialogType != application.QuestionDialogType || dialog.Message != prompt {
					t.Errorf("✗ native question lost metadata: %+v", dialog.MessageDialogOptions)
				}

				for _, action := range created.Actions {
					inspected, readErr := saferm.Run(t.Context(), &saferm.Request{Command: saferm.View, ID: strconv.FormatInt(action.ID, 10)})
					if readErr != nil || len(inspected.Actions) != 1 || inspected.Actions[0] != action {
						t.Errorf("✗ action changed before answer: %+v, %v", inspected, readErr)
					}
				}

				for path, contents := range map[string]string{moved.Source: "moving contents", deleted.Source: "occupying contents"} {
					actual, readErr := os.ReadFile(path)
					if readErr != nil || string(actual) != contents {
						t.Errorf("✗ file changed before answer: %q, %v", actual, readErr)
					}
				}

				if answer != "close" {
					answered := false

					for _, button := range dialog.Buttons {
						if button.Label == appstrings.Text("dialog."+answer) {
							answered = true

							if answer == "no" && (!button.IsDefault || !button.IsCancel) {
								t.Errorf("✗ No is not the default cancellation answer")
							}

							if button.Callback != nil {
								button.Callback()
							}
						}
					}

					if !answered {
						t.Errorf("✗ native answer button missing: %s", answer)
					}
				}

				status := store.Pending
				if answer == "yes" {
					status = store.Denied
					if decision == store.ApproveDecision {
						status = store.Completed
					}
				}

				for _, action := range created.Actions {
					inspected, readErr := saferm.Run(t.Context(), &saferm.Request{Command: saferm.View, ID: strconv.FormatInt(action.ID, 10)})
					if readErr != nil || len(inspected.Actions) != 1 || inspected.Actions[0].Status != status {
						t.Errorf("✗ linked decision result: %+v, %v", inspected, readErr)
					}
				}

				preservedPath := deleted.Source
				if status == store.Completed {
					preservedPath = filepath.Join(deleted.TrashPath, deleted.TrashID)

					contents, readErr := os.ReadFile(deleted.Source)
					if readErr != nil || string(contents) != "moving contents" {
						t.Errorf("✗ move not completed: %q, %v", contents, readErr)
					}
				} else {
					contents, readErr := os.ReadFile(moved.Source)
					if readErr != nil || string(contents) != "moving contents" {
						t.Errorf("✗ source changed without approval: %q, %v", contents, readErr)
					}
				}

				contents, readErr := os.ReadFile(preservedPath)
				if readErr != nil || string(contents) != "occupying contents" {
					t.Errorf("✗ occupant lost: %q, %v", contents, readErr)
				}

				if !t.Failed() {
					t.Log("✓ native answer preserves the linked decision contract")
				}
			})
		}
	}

	if !t.Failed() {
		t.Log("✓ linked notification decisions wait for native consent")
	}
}

// writeReviewRules is shared with the tests of changed notification requests.
func writeReviewRules(t *testing.T, home string, patterns ...string) {
	t.Helper()

	configDirectory := filepath.Join(home, ".saferm")
	if err := os.MkdirAll(configDirectory, 0o700); err != nil {
		t.Fatalf("💣 settings directory: %v", err)
	}

	quotedPatterns := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		quotedPatterns = append(quotedPatterns, strconv.Quote(pattern))
	}

	settings := fmt.Sprintf("trash-path = %q\nrequire-review = [%s]\n", filepath.Join(home, "trash"), strings.Join(quotedPatterns, ", "))
	if err := os.WriteFile(filepath.Join(configDirectory, "config.toml"), []byte(settings), 0o600); err != nil {
		t.Fatalf("💣 settings: %v", err)
	}
}
