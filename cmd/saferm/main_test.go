package main

// Invariants tested:
//  1. CLI configuration: Setup and reset route to configuration with terminal consent.
//  2. CLI recovery: Parsed paths and metadata reach recoverable operations and their results.
//  3. CLI maintenance: Decisions, multiple purge IDs, and orphan cleanup reach their operations.
//  4. JSON aliases: Both JSON flag spellings select the same result for every operation.
//  5. CLI move replacement: Both force spellings request recoverable replacement without bypassing review.
//  6. Flag placement: The CLI parses options before, between, and after positional inputs.
//  7. Option scope: Operational options are available only on applicable commands.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

	"github.com/urfave/cli/v3"
)

// TestCLIConfiguration verifies invariant #1: CLI configuration.
// Test class: Core.
// Test kind: Permanent.
// What is under test:
// Setup creates defaults; reset reads scripted consent; refusal preserves bytes
// and affirmation restores defaults. Both text and JSON routes report results.
func TestCLIConfiguration(t *testing.T) {
	for _, asJSON := range []bool{false, true} {
		home := cliHome(t)
		invokeCLI(t, "", asJSON, "config", "--setup")

		configPath := filepath.Join(home, config.Directory, config.Filename)
		assertCLIFile(t, configPath, config.DefaultDocument)

		altered := "retain-period = 5\n"
		writeCLIFile(t, configPath, altered)
		invokeCLI(t, "n\n", asJSON, "config", "--setup", "--reset")
		assertCLIFile(t, configPath, altered)
		invokeCLI(t, "\n", asJSON, "config", "--setup", "--reset")
		assertCLIFile(t, configPath, config.DefaultDocument)
		invokeCLI(t, "", asJSON, "config")
	}

	if !t.Failed() {
		t.Log("✓ CLI configuration follows consent")
	}
}

// TestCLIRecovery verifies invariant #2: CLI recovery.
// Test class: Core.
// Test kind: Permanent.
// What is under test:
// Delete records metadata, list filters select it, view preserves its source,
// restore returns its contents, and move restoration honors overwrite and force.
// Every operational result is rendered in the selected text or JSON format,
// including source paths containing characters that JSON must escape.
func TestCLIRecovery(t *testing.T) {
	for _, asJSON := range []bool{false, true} {
		home := configuredCLIHome(t)
		source := filepath.Join(home, "source & review")
		writeCLIFile(t, source, "payload")
		invokeCLI(t, "", asJSON, "delete", source, "reason", "--requester", "agent", "--description", "details")

		action := latestCLIAction(t)
		if action.Source != source || action.Requester != "agent" || action.Description != "details" || action.Reason != "reason" {
			t.Errorf("✗ CLI deletion metadata: %+v", action)
		}

		assertCLIFile(t, filepath.Join(home, "trash", action.TrashID), "payload")

		if _, err := os.Lstat(source); !os.IsNotExist(err) {
			t.Errorf("✗ deleted source remains: %v", err)
		}

		listed := invokeCLI(t, "", asJSON, "list", "--type", "delete", "--status", appstrings.Text("status.1"))
		if asJSON {
			var listedActions []store.Action
			if err := json.Unmarshal([]byte(listed), &listedActions); err != nil || len(listedActions) != 1 || listedActions[0].ID != action.ID || listedActions[0].Source != source {
				t.Errorf("✗ list did not preserve the selected action: %s, %v", listed, err)
			}
		} else if !strings.Contains(listed, source) {
			t.Error("✗ list omitted selected action")
		}

		id := strconv.FormatInt(action.ID, 10)

		viewed := invokeCLI(t, "", asJSON, "view", id)
		if asJSON {
			var viewedAction store.Action
			if err := json.Unmarshal([]byte(viewed), &viewedAction); err != nil || viewedAction.ID != action.ID || viewedAction.Source != source {
				t.Errorf("✗ view did not preserve the selected action: %s, %v", viewed, err)
			}
		} else if !strings.Contains(viewed, source) {
			t.Error("✗ view omitted source")
		}

		invokeCLI(t, "", asJSON, "restore", id)
		assertCLIFile(t, source, "payload")

		destination := filepath.Join(home, "destination")
		invokeCLI(t, "", asJSON, "move", source, destination, "relocate")

		action = latestCLIAction(t)
		if action.Destination != destination {
			t.Errorf("✗ CLI move destination: %+v", action)
		}

		assertCLIFile(t, destination, "payload")
		writeCLIFile(t, source, "occupant")
		restoreOutput := invokeCLI(t, "", asJSON, "restore", strconv.FormatInt(action.ID, 10), "--overwrite", "--force")

		occupyingDelete := latestCLIAction(t)
		if occupyingDelete.ID == action.ID || occupyingDelete.Type != saferm.Delete || occupyingDelete.Status != store.Completed {
			t.Errorf("✗ CLI restore omitted occupying delete: %+v", occupyingDelete)
		}

		assertCLIFile(t, filepath.Join(home, "trash", occupyingDelete.TrashID), "occupant")

		if asJSON {
			var restoredResult saferm.Result
			if err := json.Unmarshal([]byte(restoreOutput), &restoredResult); err != nil || len(restoredResult.Actions) != 2 || restoredResult.Actions[0].ID != occupyingDelete.ID || restoredResult.Actions[1].ID != action.ID {
				t.Errorf("✗ CLI restore lost action order: %q, %v", restoreOutput, err)
			}
		}

		assertCLIFile(t, source, "payload")
	}

	if !t.Failed() {
		t.Log("✓ CLI recovery routes arguments and flags")
	}
}

// TestCLIMaintenance verifies invariant #3: CLI maintenance.
// Test class: Core.
// Test kind: Permanent.
// What is under test:
// Dispose approves a pending action, purge receives multiple IDs, and cleanup
// removes eligible and orphaned records without deleting restored source files.
func TestCLIMaintenance(t *testing.T) {
	for _, asJSON := range []bool{false, true} {
		home := configuredCLIHome(t)
		protected := filepath.Join(home, "protected")
		writeCLIFile(t, protected, "reviewed")
		invokeCLI(t, "", asJSON, "delete", protected, "review")

		action := latestCLIAction(t)
		if action.Status != store.Pending {
			t.Errorf("✗ review not pending: %+v", action)
		}

		assertCLIFile(t, protected, "reviewed")

		firstID := strconv.FormatInt(action.ID, 10)
		invokeCLI(t, "", asJSON, "dispose", firstID, "approve")

		second := filepath.Join(home, "second")
		writeCLIFile(t, second, "second payload")
		invokeCLI(t, "", asJSON, "delete", second, "remove")
		secondAction := latestCLIAction(t)
		invokeCLI(t, "", asJSON, "purge", firstID, strconv.FormatInt(secondAction.ID, 10))

		actions, err := saferm.Run(t.Context(), &saferm.Request{Command: saferm.List})
		if err != nil || len(actions.Actions) != 2 {
			t.Errorf("✗ purge records: %+v, %v", actions, err)
		}

		for _, record := range actions.Actions {
			if record.Status != store.Purged {
				t.Errorf("✗ batch ID not purged: %+v", record)
			}

			if _, err := os.Lstat(filepath.Join(home, "trash", record.TrashID)); !os.IsNotExist(err) {
				t.Errorf("✗ purge left item: %v", err)
			}
		}

		missing := filepath.Join(home, "missing")
		writeCLIFile(t, missing, "orphan")
		invokeCLI(t, "", asJSON, "delete", missing, "remove")
		orphan := latestCLIAction(t)

		recovered := filepath.Join(home, "manual recovery")
		if err := os.Rename(filepath.Join(home, "trash", orphan.TrashID), recovered); err != nil {
			t.Fatalf("💣 manual recovery fixture: %v", err)
		}

		invokeCLI(t, "", asJSON, "cleanup", "--orphans")

		actions, err = saferm.Run(t.Context(), &saferm.Request{Command: saferm.List})
		if err != nil || len(actions.Actions) != 0 {
			t.Errorf("✗ cleanup did not remove eligible records: %+v, %v", actions, err)
		}

		assertCLIFile(t, recovered, "orphan")
	}

	if !t.Failed() {
		t.Log("✓ CLI decisions and maintenance route all inputs")
	}
}

// TestJSONAliases verifies invariant #4: JSON aliases.
// Test class: Core.
// Test kind: Permanent.
// What is under test:
// Every operational command returns identical JSON with either flag spelling,
// after the command name. Missing inputs also produce JSON diagnostics.
func TestJSONAliases(t *testing.T) {
	cliHome(t)

	for _, operation := range newCommand().Commands {
		if operation.Name == commandHelp {
			continue
		}

		var referenceJSON string

		for _, jsonFlag := range []string{"-j", "--json"} {
			arguments := []string{"saferm", operation.Name, jsonFlag}
			command := newCommand()

			var standardOutput, diagnostics bytes.Buffer

			command.Writer, command.ErrWriter = &standardOutput, &diagnostics
			commandErr := execute(t.Context(), command, arguments)

			renderedJSON := standardOutput.String()
			if commandErr != nil {
				renderedJSON = diagnostics.String()
			}

			if !json.Valid([]byte(renderedJSON)) {
				t.Errorf("✗ %v did not return JSON: %q, %v", arguments, renderedJSON, commandErr)
			}

			if referenceJSON == "" {
				referenceJSON = renderedJSON
			} else if renderedJSON != referenceJSON {
				t.Errorf("✗ JSON aliases differ for %s", operation.Name)
			}
		}
	}

	if !t.Failed() {
		t.Log("✓ JSON aliases agree for every operation")
	}
}

// TestCLIMoveOverwrite verifies invariant #5: CLI move replacement.
// Test class: Core.
// Test kind: Permanent.
// What is under test:
// --force and -f work with move's --overwrite and metadata flags on either side
// of positional inputs. They skip replacement consent but leave both actions
// pending when the occupying destination requires review. Approving its delete
// completes the move and preserves the occupant in trash.
func TestCLIMoveOverwrite(t *testing.T) {
	for _, forceFlag := range []string{"--force", "-f"} {
		t.Run(forceFlag, func(t *testing.T) {
			home := configuredCLIHome(t)
			source, destination := filepath.Join(home, "source"), filepath.Join(home, "protected")
			writeCLIFile(t, source, "moving contents")
			writeCLIFile(t, destination, "occupying contents")

			arguments := []string{"move", "--overwrite", forceFlag, source, destination, "replace", "--requester", "Agent"}
			if forceFlag == "-f" {
				arguments = []string{"move", source, "--requester", "Agent", destination, "replace", "--overwrite", forceFlag}
			}

			output := invokeCLI(t, "", true, arguments...)

			var pair saferm.Result
			if err := json.Unmarshal([]byte(output), &pair); err != nil || len(pair.Actions) != 2 {
				t.Errorf("✗ move flags did not return a linked pair: %q, %v", output, err)

				return
			}

			moved, deleted := pair.Actions[0], pair.Actions[1]
			if moved.Type != saferm.Move || deleted.Type != saferm.Delete || moved.Status != store.Pending || deleted.Status != store.Pending || moved.ID >= deleted.ID || moved.BlockerID != deleted.ID || moved.Requester != "Agent" || deleted.Requester != "Agent" {
				t.Errorf("✗ force bypassed review or lost metadata: %+v", pair)
			}

			assertCLIFile(t, source, "moving contents")
			assertCLIFile(t, destination, "occupying contents")
			output = invokeCLI(t, "", true, "dispose", strconv.FormatInt(deleted.ID, 10), "approve")

			var approved saferm.Result
			if err := json.Unmarshal([]byte(output), &approved); err != nil || len(approved.Actions) != 2 {
				t.Errorf("✗ linked approval results missing: %q, %v", output, err)
			} else {
				for _, action := range approved.Actions {
					if action.Status != store.Completed {
						t.Errorf("✗ approval did not finish action: %+v", action)
					}
				}
			}

			assertCLIFile(t, destination, "moving contents")
			assertCLIFile(t, filepath.Join(home, "trash", deleted.TrashID), "occupying contents")

			if _, err := os.Lstat(source); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("✗ source remains after approved move: %v", err)
			}

			if !t.Failed() {
				t.Log("✓ move replacement flags preserve independent review")
			}
		})
	}

	if !t.Failed() {
		t.Log("✓ both force spellings reach recoverable move replacement")
	}
}

// TestFlagPlacement verifies invariant #6: Flag placement.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// Each flag placement yields the same source, reason, requester, and local
// JSON choice. Parser-only callbacks keep file operations outside this unit test.
func TestFlagPlacement(t *testing.T) {
	for _, arguments := range [][]string{
		{"saferm", "delete", "--json", "--requester", "agent", "source", "reason"},
		{"saferm", "delete", "--requester", "agent", "source", "--json", "reason"},
		{"saferm", "delete", "source", "--requester", "agent", "reason", "-j"},
		{"saferm", "delete", "source", "reason", "--requester", "agent", "--json"},
	} {
		command := newCommand()

		deleteCommand := command.Command("delete")
		if deleteCommand == nil {
			t.Error("✗ delete command unavailable")

			continue
		}

		deleteCommand.Action = inspectParsedDelete
		deleteCommand.Metadata = map[string]any{"test": t}
		command.Writer = &bytes.Buffer{}

		command.ErrWriter = &bytes.Buffer{}
		if err := command.Run(t.Context(), arguments); err != nil {
			t.Errorf("✗ parsing %v: %v", arguments, err)
		}
	}

	if !t.Failed() {
		t.Log("✓ flag placement")
	}
}

// TestOptionScope verifies invariant #7: Option scope.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// Root and help reject both JSON flag spellings, including JSON before an
// operational command name. Commands also reject other inapplicable options.
func TestOptionScope(t *testing.T) {
	cliHome(t)

	for _, arguments := range [][]string{
		{"saferm", "--overwrite"},
		{"saferm", "--force", "move"},
		{"saferm", "-f", "move"},
		{"saferm", "help", "move", "--overwrite"},
		{"saferm", "delete", "--overwrite"},
		{"saferm", "dispose", "--force"},
		{"saferm", "view", "-f"},
		{"saferm", "list", "--force"},
		{"saferm", "config", "--requester", "agent"},
		{"saferm", "-j"},
		{"saferm", "--json"},
		{"saferm", "-j", "config"},
		{"saferm", "--json", "config"},
		{"saferm", "help", "-j"},
		{"saferm", "help", "--json"},
		{"saferm", "help", "config", "-j"},
		{"saferm", "help", "config", "--json"},
	} {
		command := newCommand()
		command.Writer = &bytes.Buffer{}

		command.ErrWriter = &bytes.Buffer{}
		if err := command.Run(t.Context(), arguments); !errors.Is(err, errs.ErrInputInvalid) {
			t.Errorf("✗ invalid option accepted: %v", arguments)
		}
	}

	if !t.Failed() {
		t.Log("✓ command option scope")
	}
}

func inspectParsedDelete(_ context.Context, command *cli.Command) error {
	command.Metadata["test"].(*testing.T).Helper()
	t := command.Metadata["test"].(*testing.T)

	if command.StringArg("source") != "source" || command.StringArg("reason") != "reason" || command.String("requester") != "agent" || !command.Bool("json") {
		t.Errorf("✗ parsed input differs: %v", command)
	}

	return nil
}

// Test class: Core: Helper.
func cliHome(t *testing.T) string {
	t.Helper()

	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("💣 fixture home: %v", err)
	}

	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	return home
}

// Test class: Core: Helper.
func configuredCLIHome(t *testing.T) string {
	t.Helper()
	home := cliHome(t)

	configPath := filepath.Join(home, config.Directory, config.Filename)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatalf("💣 fixture config directory: %v", err)
	}

	writeCLIFile(t, configPath, fmt.Sprintf("trash-path = %q\nretain-period = 0\nrequire-review = [\"**/protected\"]\n", filepath.ToSlash(filepath.Join(home, "trash"))))

	return home
}

// Test class: Core: Helper.
func invokeCLI(t *testing.T, input string, asJSON bool, arguments ...string) string {
	t.Helper()

	command := newCommand()

	var output, diagnostics bytes.Buffer

	command.Reader = strings.NewReader(input)
	command.Writer = &output
	command.ErrWriter = &diagnostics
	commandLine := []string{"saferm"}

	commandLine = append(commandLine, arguments...)
	if asJSON {
		commandLine = append(commandLine, "--json")
	}

	if err := execute(t.Context(), command, commandLine); err != nil {
		t.Errorf("✗ CLI %v: %v; %s", arguments, err, diagnostics.String())
	}

	if asJSON && !json.Valid(output.Bytes()) {
		t.Errorf("✗ invalid JSON for %v: %s", arguments, output.String())
	}

	if output.Len() == 0 {
		t.Errorf("✗ empty output for %v", arguments)
	}

	return output.String()
}

// Test class: Core: Helper.
func writeCLIFile(t *testing.T, path, contents string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("💣 fixture file: %v", err)
	}
}

// Test class: Core: Helper.
func assertCLIFile(t *testing.T, path, contents string) {
	t.Helper()

	actual, err := os.ReadFile(path)
	if err != nil || string(actual) != contents {
		t.Errorf("✗ file %q: %q, %v", path, actual, err)
	}
}

// Test class: Core: Helper.
func latestCLIAction(t *testing.T) store.Action {
	t.Helper()

	result, err := saferm.Run(t.Context(), &saferm.Request{Command: saferm.List})
	if err != nil || len(result.Actions) == 0 {
		t.Errorf("✗ inspect created action: %v", err)

		return store.Action{}
	}

	return result.Actions[len(result.Actions)-1]
}
