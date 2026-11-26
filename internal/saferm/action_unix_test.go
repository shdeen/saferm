//go:build unix

package saferm

// Invariants tested:
//  1. Unreadable directories: A delete waits for approval when a directory that a pattern could apply inside cannot be read and no visible item is protected. A directory that no pattern could apply inside is never read.
//  2. Move warnings: A move waits for approval with the warning for an unreadable directory inside its source, and a replacing move also carries the warning for an unreadable directory inside the occupant.

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"testing"

	"github.com/shdeen/saferm/internal/appstrings"
	"github.com/shdeen/saferm/internal/store"
)

// TestUnreadableDirectories verifies invariant #1: Unreadable directories.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// The source Downloads/old holds the file a.txt, the directory docs, and the
// directory locked, which cannot be opened. Each case deletes the source.
//   - With no patterns, with one empty pattern, and with "~/Contracts/" alone,
//     the delete completes without a warning and the source is in trash.
//   - With "*.pem" alone, the delete is pending and records "*.pem". The result
//     carries the warning that names locked. The source keeps its contents, and
//     approving the action completes the delete.
//   - With "*.pem" and a file a.pem beside locked, the delete is pending and
//     records "*.pem", and the result carries no warning.
//
// The test fails when it runs as root, because root can open every directory.
func TestUnreadableDirectories(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Fatal("💣 run this test as a regular user; root can open every directory")
	}

	for _, example := range []struct {
		name         string
		patterns     []string
		hasKeyFile   bool
		recordedRule string
		hasWarning   bool
	}{
		{"no patterns", []string{}, false, "", false},
		{"one empty pattern", []string{""}, false, "", false},
		{"pattern for another directory", []string{"~/Contracts/"}, false, "", false},
		{"pattern that could apply inside", []string{"*.pem"}, false, "*.pem", true},
		{"visible protected file", []string{"*.pem"}, true, "*.pem", false},
	} {
		t.Run(example.name, func(t *testing.T) {
			home := isolateHome(t)
			source := filepath.Join(home, "Downloads", "old")
			lockedDirectory := filepath.Join(source, "locked")

			reviewDocument, err := json.Marshal(example.patterns)
			if err != nil {
				t.Fatalf("💣 review configuration: %v", err)
			}

			writeConfig(t, home, fmt.Sprintf("trash-path = %q\nrequire-review = %s\n", filepath.Join(home, "trash"), reviewDocument))
			writeFile(t, filepath.Join(source, "a.txt"), "text")
			writeFile(t, filepath.Join(source, "docs", "page"), "page")
			writeFile(t, filepath.Join(lockedDirectory, "hidden.pem"), "hidden")

			if example.hasKeyFile {
				writeFile(t, filepath.Join(source, "a.pem"), "key")
			}

			lockDirectory(t, home, lockedDirectory)

			result, err := Run(t.Context(), &Request{Command: Delete, Source: source, Reason: "remove old downloads"})
			if err != nil || len(result.Actions) != 1 {
				t.Fatalf("💣 no action available for inspection: %+v, %v", result, err)
			}

			action := result.Actions[0]

			var warnings []string
			if example.hasWarning {
				warnings = []string{fmt.Sprintf(appstrings.Text("warning.unreadable"), filepath.Join(action.Source, "locked"))}
			}

			if action.ReviewRule != example.recordedRule || !reflect.DeepEqual(result.Problems, warnings) {
				t.Errorf("✗ review outcome: %+v", result)
			}

			if example.recordedRule != "" {
				if action.Status != store.Pending {
					t.Errorf("✗ pending record: %+v", action)
				}

				assertFile(t, filepath.Join(source, "a.txt"), "text")

				if lockedInfo, err := os.Lstat(lockedDirectory); err != nil || lockedInfo.Mode().Perm() != 0 {
					t.Errorf("✗ unreadable directory changed: %v", err)
				}

				action = performAction(t, Request{Command: Dispose, ID: strconv.FormatInt(action.ID, 10), Decision: store.ApproveDecision})
			}

			if action.Status != store.Completed {
				t.Errorf("✗ completed record: %+v", action)
			}

			assertAbsent(t, source)
			assertFile(t, filepath.Join(home, "trash", action.TrashID, "a.txt"), "text")

			if !t.Failed() {
				t.Log("✓ the delete gives its listed result")
			}
		})
	}

	if !t.Failed() {
		t.Log("✓ unreadable directories hold a request only where a pattern could apply")
	}
}

// TestUnreadableMoves verifies invariant #2: Move warnings.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// With "*.pem" as the only pattern, the directory moving holds the directory
// locked and the directory occupant holds the directory sealed. Neither locked
// nor sealed can be opened.
//   - A move of moving to a free destination is pending, records "*.pem", and
//     carries the warning that names locked.
//   - A move of moving that replaces occupant returns the move and the
//     occupying delete, both pending, with the warnings that name locked and
//     sealed.
//
// Both directories keep their contents. The test fails when it runs as root,
// because root can open every directory.
func TestUnreadableMoves(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Fatal("💣 run this test as a regular user; root can open every directory")
	}

	home := isolateHome(t)
	movingDirectory, occupantDirectory := filepath.Join(home, "moving"), filepath.Join(home, "occupant")
	lockedDirectory, sealedDirectory := filepath.Join(movingDirectory, "locked"), filepath.Join(occupantDirectory, "sealed")

	writeConfig(t, home, fmt.Sprintf("trash-path = %q\nrequire-review = [%q]\n", filepath.Join(home, "trash"), "*.pem"))
	writeFile(t, filepath.Join(movingDirectory, "a.txt"), "moving")
	writeFile(t, filepath.Join(occupantDirectory, "b.txt"), "occupant")

	for _, unreadableDirectory := range []string{lockedDirectory, sealedDirectory} {
		writeFile(t, filepath.Join(unreadableDirectory, "hidden.pem"), "hidden")
		lockDirectory(t, home, unreadableDirectory)
	}

	freeMove, err := Run(t.Context(), &Request{Command: Move, Source: movingDirectory, Destination: filepath.Join(home, "archive"), Reason: "archive"})
	if err != nil || len(freeMove.Actions) != 1 {
		t.Fatalf("💣 no move available for inspection: %+v, %v", freeMove, err)
	}

	lockedWarning := fmt.Sprintf(appstrings.Text("warning.unreadable"), filepath.Join(freeMove.Actions[0].Source, "locked"))
	if moveAction := freeMove.Actions[0]; moveAction.Status != store.Pending || moveAction.ReviewRule != "*.pem" || !reflect.DeepEqual(freeMove.Problems, []string{lockedWarning}) {
		t.Errorf("✗ move with an unreadable directory in its source: %+v", freeMove)
	}

	replacingMove, err := Run(t.Context(), &Request{Command: Move, Source: movingDirectory, Destination: occupantDirectory, Reason: "replace", Overwrite: true, Force: true})
	if err != nil || len(replacingMove.Actions) != 2 {
		t.Fatalf("💣 no replacing move available for inspection: %+v, %v", replacingMove, err)
	}

	sealedWarning := fmt.Sprintf(appstrings.Text("warning.unreadable"), filepath.Join(replacingMove.Actions[1].Source, "sealed"))
	if !slices.Equal(slices.Sorted(slices.Values(replacingMove.Problems)), slices.Sorted(slices.Values([]string{lockedWarning, sealedWarning}))) {
		t.Errorf("✗ replacing move lost a warning: %q", replacingMove.Problems)
	}

	for _, action := range replacingMove.Actions {
		if action.Status != store.Pending || action.ReviewRule != "*.pem" {
			t.Errorf("✗ pending record: %+v", action)
		}
	}

	assertFile(t, filepath.Join(movingDirectory, "a.txt"), "moving")
	assertFile(t, filepath.Join(occupantDirectory, "b.txt"), "occupant")

	if !t.Failed() {
		t.Log("✓ moves carry the warning for each unreadable directory")
	}
}

// lockDirectory removes every permission from the directory. Before the test
// ends, it restores the permissions of every such directory under the home
// directory, wherever a delete has put it.
func lockDirectory(t *testing.T, home, directory string) {
	t.Helper()

	if err := os.Chmod(directory, 0); err != nil {
		t.Fatalf("💣 fixture permissions: %v", err)
	}

	t.Cleanup(func() {
		restoreErr := filepath.WalkDir(home, func(path string, directoryEntry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if directoryInfo, infoErr := directoryEntry.Info(); infoErr == nil && directoryEntry.IsDir() && directoryInfo.Mode().Perm() == 0 {
				return os.Chmod(path, 0o700) //nolint:gosec // G302: the directory needs its owner permissions back so that cleanup can remove it.
			}

			return nil
		})
		if restoreErr != nil {
			t.Errorf("✗ restore fixture permissions: %v", restoreErr)
		}
	})

	if _, err := os.ReadDir(directory); !os.IsPermission(err) {
		t.Fatalf("💣 fixture directory can still be read: %v", err)
	}
}
