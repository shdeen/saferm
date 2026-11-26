package output

// Invariants tested:
//  1. Text results: Rendering selects the appropriate template and preserves relevant action data.
//  2. JSON results: Every operation returns machine-readable data in JSON mode.
//  3. Restore results: Multiple actions retain their order and fields in text and JSON.
//  4. Blocker presentation: Text places blocker identity beside review, and JSON preserves its optional reference.
//  5. Linked questions: Prompt templates substitute both action IDs while preserving the literal occupant path.
//  6. Action results: The JSON of a delete, move, dispose, or restore result holds its actions in a list, for one action as for several, with each warning beside them. In text, a warning follows its action.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/shdeen/saferm/internal/appstrings"
	"github.com/shdeen/saferm/internal/config"
	"github.com/shdeen/saferm/internal/errs"
	"github.com/shdeen/saferm/internal/saferm"
	"github.com/shdeen/saferm/internal/store"
)

// TestTextResults verifies invariant #1: Text results.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// View includes full action information; list includes source, status, and local
// creation date; config exposes unset values and notices use their catalog text.
func TestTextResults(t *testing.T) {
	action := store.Action{ID: 42, Type: saferm.Delete, Status: store.Pending, Source: "/source/item", TrashID: "identity", TrashPath: "/historical/trash", Reason: "reason text", Requester: "requester text", Description: "description text", ReviewRule: "**/item", CreatedAt: time.Now().UTC(), InProgress: true}
	for _, command := range []string{saferm.View, saferm.List, saferm.Delete} {
		var buffer bytes.Buffer

		terminal := Terminal{Output: &buffer}
		if err := render(terminal.Output, terminal.JSON, command, saferm.Result{Actions: []store.Action{action}}); err != nil {
			t.Errorf("✗ rendering %s: %v", command, err)
		}

		for _, value := range []string{action.Source, appstrings.Text("status.2")} {
			if !strings.Contains(buffer.String(), value) {
				t.Errorf("✗ missing result value %q", value)
			}
		}

		if command == saferm.View {
			for _, value := range []string{action.Reason, action.Requester, action.Description, action.TrashID, action.TrashPath, action.ReviewRule} {
				if !strings.Contains(buffer.String(), value) {
					t.Errorf("✗ missing view value %q", value)
				}
			}
		}

		if command == saferm.List && !strings.Contains(buffer.String(), action.CreatedAt.Local().Format(appstrings.Text("display.date"))) {
			t.Error("✗ local date missing")
		}
	}

	var buffer bytes.Buffer

	terminal := Terminal{Output: &buffer, Errors: &buffer}
	if err := render(terminal.Output, terminal.JSON, saferm.Config, saferm.Result{Config: &config.Settings{}}); err != nil {
		t.Errorf("✗ configuration rendering: %v", err)
	}

	if !strings.Contains(buffer.String(), appstrings.Text("display.unset")) {
		t.Error("✗ omitted configuration not displayed")
	}

	buffer.Reset()

	if err := terminal.Failure(errs.ErrInputMissing); err != nil {
		t.Errorf("✗ failure rendering: %v", err)
	}

	if !strings.Contains(buffer.String(), appstrings.Text("error.inputMissing")) {
		t.Error("✗ failure copy missing")
	}

	if !t.Failed() {
		t.Log("✓ text selects templates and preserves action data")
	}
}

// TestJSONResults verifies invariant #2: JSON results.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// Lists, actions, purge batches, cleanup counts, configuration, notices, and errors
// produce valid JSON. A viewed action exposes its ID directly, without a wrapper.
func TestJSONResults(t *testing.T) {
	action := store.Action{ID: 42, Type: saferm.Move, Status: store.Completed, Source: "source", Destination: "destination"}
	for _, testCase := range []struct {
		command string
		result  saferm.Result
	}{
		{saferm.List, saferm.Result{Actions: []store.Action{action}}},
		{saferm.View, saferm.Result{Actions: []store.Action{action}}},
		{saferm.Purge, saferm.Result{Actions: []store.Action{action}, Problems: []string{errs.ErrFileMissing.Error()}}},
		{saferm.Cleanup, saferm.Result{Removed: []int64{42}}},
		{saferm.Config, saferm.Result{Config: &config.Settings{}}},
		{saferm.Config, saferm.Result{Notice: appstrings.Text("notice.configExists")}},
	} {
		var buffer bytes.Buffer

		terminal := Terminal{Output: &buffer, Errors: &buffer, JSON: true}
		if err := render(terminal.Output, terminal.JSON, testCase.command, testCase.result); err != nil || !json.Valid(buffer.Bytes()) {
			t.Errorf("✗ JSON %s: %v, %q", testCase.command, err, buffer.String())
		}

		if testCase.command == saferm.View {
			var decoded store.Action
			if err := json.Unmarshal(buffer.Bytes(), &decoded); err != nil || decoded.ID != action.ID {
				t.Errorf("✗ direct action JSON: %+v, %v", decoded, err)
			}
		}

		buffer.Reset()

		if err := terminal.Failure(errs.ErrInputMissing); err != nil || !json.Valid(buffer.Bytes()) {
			t.Errorf("✗ JSON failure: %v", err)
		}
	}

	if !t.Failed() {
		t.Log("✓ machine-readable command results")
	}
}

// TestRestoreResultOrder verifies invariant #3: Restore results.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// Restore exposes its occupying delete before the original action. Multiple
// actions use the result's JSON array; each text action keeps its recovery data.
func TestRestoreResultOrder(t *testing.T) {
	deleted := store.Action{ID: 9, Type: saferm.Delete, Status: store.Completed, Source: "/original", TrashID: "preserved-occupant", TrashPath: "/trash"}
	restored := store.Action{ID: 3, Type: saferm.Move, Status: store.Restored, Source: "/original", Destination: "/stored"}
	result := saferm.Result{Actions: []store.Action{deleted, restored}}

	for _, asJSON := range []bool{false, true} {
		var buffer bytes.Buffer

		if err := render(&buffer, asJSON, saferm.Restore, result); err != nil {
			t.Errorf("✗ restore results: %v", err)
		}

		if asJSON {
			var decoded saferm.Result
			if err := json.Unmarshal(buffer.Bytes(), &decoded); err != nil || !reflect.DeepEqual(decoded.Actions, result.Actions) {
				t.Errorf("✗ JSON lost action order or fields: %+v, %v", decoded, err)
			}
		} else {
			deletePosition, restorePosition := strings.Index(buffer.String(), deleted.TrashID), strings.Index(buffer.String(), restored.Destination)
			if deletePosition < 0 || restorePosition < deletePosition {
				t.Errorf("✗ text lost ordered recovery locations: %q", buffer.String())
			}
		}
	}

	if !t.Failed() {
		t.Log("✓ multiple restore results retain order and recovery data")
	}
}

// TestBlockerResults verifies invariant #4: Blocker presentation.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// Move summaries and details show the blocker after the review rule. The JSON
// of a viewed action stays unwrapped and preserves the reference, while actions
// without blockers omit it. Multiple-action JSON keeps the move before its occupying delete.
func TestBlockerResults(t *testing.T) {
	moved := store.Action{ID: 13, Type: saferm.Move, Status: store.Pending, Source: "/source", Destination: "/destination", BlockerID: 123456789, ReviewRule: "/source"}
	deleted := store.Action{ID: moved.BlockerID, Type: saferm.Delete, Status: store.Pending, Source: "/destination", TrashID: "occupant", ReviewRule: "/destination"}

	for _, command := range []string{saferm.Move, saferm.View} {
		t.Run(command, func(t *testing.T) {
			var rendered bytes.Buffer
			if err := render(&rendered, false, command, saferm.Result{Actions: []store.Action{moved}}); err != nil {
				t.Errorf("✗ linked text: %v", err)
			}

			blockerPosition := strings.Index(rendered.String(), strconv.FormatInt(moved.BlockerID, 10))

			reviewPosition := strings.LastIndex(rendered.String(), moved.ReviewRule)
			if blockerPosition < 0 || reviewPosition < 0 || blockerPosition < reviewPosition {
				t.Errorf("✗ blocker missing beside review: %q", rendered.String())
			}

			if !t.Failed() {
				t.Log("✓ text associates blocker identity with its move's review")
			}
		})
	}

	for _, action := range []store.Action{moved, deleted} {
		t.Run(action.Type+" JSON", func(t *testing.T) {
			var rendered bytes.Buffer
			if err := render(&rendered, true, saferm.View, saferm.Result{Actions: []store.Action{action}}); err != nil {
				t.Errorf("✗ linked JSON: %v", err)
			}

			var decoded store.Action
			if err := json.Unmarshal(rendered.Bytes(), &decoded); err != nil || !reflect.DeepEqual(decoded, action) {
				t.Errorf("✗ direct action JSON lost fields: %+v, %v", decoded, err)
			}

			if strings.Contains(rendered.String(), "blockerID") != (action.BlockerID != 0) {
				t.Errorf("✗ optional blocker JSON is incorrect: %q", rendered.String())
			}

			if !t.Failed() {
				t.Log("✓ single action JSON preserves its optional blocker")
			}
		})
	}

	var rendered bytes.Buffer

	pair := saferm.Result{Actions: []store.Action{moved, deleted}}
	if err := render(&rendered, true, saferm.Move, pair); err != nil {
		t.Errorf("✗ multiple linked results: %v", err)
	}

	var decoded saferm.Result
	if err := json.Unmarshal(rendered.Bytes(), &decoded); err != nil || !reflect.DeepEqual(decoded.Actions, pair.Actions) {
		t.Errorf("✗ multiple action JSON lost ordering or fields: %+v, %v", decoded, err)
	}

	if !t.Failed() {
		t.Log("✓ text and JSON preserve blocker relationships and action order")
	}
}

// TestLinkedPrompt verifies invariant #5: Linked questions.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// Both combined questions substitute the move/delete IDs and preserve literal
// dollar signs, template braces and quotes in the path without a second pass.
func TestLinkedPrompt(t *testing.T) {
	for _, promptKey := range []string{"prompt.blockedApprove", "prompt.blockedDeny"} {
		t.Run(promptKey, func(t *testing.T) {
			confirmation := saferm.Confirmation{Prompt: appstrings.Text(promptKey), MoveID: 13, BlockerID: 17, Path: `/folder/$& $$ {{.MoveID}} "quoted"`}

			rendered, err := RenderString(confirmation.Prompt, confirmation)
			if err != nil || !strings.Contains(rendered, confirmation.Path) || !strings.Contains(rendered, strconv.FormatInt(confirmation.MoveID, 10)) || !strings.Contains(rendered, strconv.FormatInt(confirmation.BlockerID, 10)) {
				t.Errorf("✗ combined prompt lost literal values: %q, %v", rendered, err)
			}

			if !t.Failed() {
				t.Log("✓ linked prompt substitutes IDs and retains the complete path")
			}
		})
	}

	if !t.Failed() {
		t.Log("✓ both linked questions preserve their metadata")
	}
}

// TestActionResults verifies invariant #6: Action results.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// The JSON of a delete, move, dispose, or restore result with one action is an
// object whose only field is the list that holds the action. A result with
// two warnings has one more field, the list that holds both. The text of a
// delete result with a warning shows the warning after the action.
func TestActionResults(t *testing.T) {
	pendingDelete := store.Action{ID: 21, Type: saferm.Delete, Status: store.Pending, Source: "/source/old", TrashID: "identity", ReviewRule: "*.pem"}
	warning := fmt.Sprintf(appstrings.Text("warning.unreadable"), "/source/old/locked")
	warnedResult := saferm.Result{Actions: []store.Action{pendingDelete}, Problems: []string{warning, fmt.Sprintf(appstrings.Text("warning.unreadable"), "/source/old/sealed")}}

	for _, command := range []string{saferm.Delete, saferm.Move, saferm.Dispose, saferm.Restore} {
		for _, result := range []saferm.Result{{Actions: []store.Action{pendingDelete}}, warnedResult} {
			var renderedJSON bytes.Buffer
			if err := render(&renderedJSON, true, command, result); err != nil {
				t.Errorf("✗ %s JSON: %v", command, err)
			}

			var resultFields map[string]json.RawMessage

			var decodedResult saferm.Result
			if err := errors.Join(json.Unmarshal(renderedJSON.Bytes(), &resultFields), json.Unmarshal(renderedJSON.Bytes(), &decodedResult)); err != nil {
				t.Errorf("✗ %s JSON is not a result: %q, %v", command, renderedJSON.String(), err)
			}

			fieldNames := []string{"actions"}
			if len(result.Problems) > 0 {
				fieldNames = append(fieldNames, "problems")
			}

			if !reflect.DeepEqual(decodedResult, result) || !slices.Equal(slices.Sorted(maps.Keys(resultFields)), fieldNames) {
				t.Errorf("✗ %s JSON is not the list of actions with its warnings: %q", command, renderedJSON.String())
			}
		}
	}

	var renderedText bytes.Buffer
	if err := render(&renderedText, false, saferm.Delete, warnedResult); err != nil {
		t.Errorf("✗ warning text: %v", err)
	}

	sourcePosition, warningPosition := strings.Index(renderedText.String(), pendingDelete.Source), strings.Index(renderedText.String(), warning)
	if sourcePosition < 0 || warningPosition < sourcePosition {
		t.Errorf("✗ text lost the warning after its action: %q", renderedText.String())
	}

	if !t.Failed() {
		t.Log("✓ action results list their actions in JSON, and a warning accompanies its action")
	}
}
