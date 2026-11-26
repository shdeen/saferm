package main

// Invariants tested:
//  1. CLI failures: Invalid operational input produces identifiable errors in the selected format.
//  2. CLI option failures: Inapplicable flags fail in the format selected by the invoked command.

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/shdeen/saferm/internal/appstrings"
	"github.com/shdeen/saferm/internal/errs"
)

// TestCLIFailures verifies invariant #1: CLI failures.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// An invalid action ID reaches shared validation and produces no success output.
// Diagnostics contain catalog copy in text mode and valid JSON in JSON mode.
func TestCLIFailures(t *testing.T) {
	for _, asJSON := range []bool{false, true} {
		cliHome(t)

		command := newCommand()

		var output, diagnostics bytes.Buffer

		command.Writer = &output
		command.ErrWriter = &diagnostics

		arguments := []string{"saferm", "view", "invalid"}
		if asJSON {
			arguments = append(arguments, "--json")
		}

		if err := execute(t.Context(), command, arguments); !errors.Is(err, errs.ErrInputInvalid) {
			t.Errorf("✗ operational input error: %v", err)
		}

		if output.Len() != 0 {
			t.Error("✗ failure emitted success output")
		}

		if asJSON {
			if !json.Valid(diagnostics.Bytes()) {
				t.Errorf("✗ diagnostic JSON: %s", diagnostics.String())
			}
		} else if !strings.Contains(diagnostics.String(), appstrings.Text("error.inputInvalid")) {
			t.Error("✗ diagnostic omitted input error")
		}
	}

	if !t.Failed() {
		t.Log("✓ CLI failures honor output format")
	}
}

// TestCLIOptionFailures verifies invariant #2: CLI option failures.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// Root and help reject JSON flags with plain-text diagnostics. An operational
// command with either JSON alias returns JSON for a subsequent unknown flag.
// All rejected invocations return the input-error sentinel and no success output.
func TestCLIOptionFailures(t *testing.T) {
	cliHome(t)

	for _, jsonFlag := range []string{"-j", "--json"} {
		for _, failureCase := range []struct {
			arguments []string
			jsonError bool
		}{
			{arguments: []string{"saferm", jsonFlag}},
			{arguments: []string{"saferm", jsonFlag, "config"}},
			{arguments: []string{"saferm", "help", jsonFlag}},
			{arguments: []string{"saferm", "config", jsonFlag, "--unknown"}, jsonError: true},
		} {
			command := newCommand()

			var standardOutput, diagnostics bytes.Buffer

			command.Writer, command.ErrWriter = &standardOutput, &diagnostics
			if err := execute(t.Context(), command, failureCase.arguments); !errors.Is(err, errs.ErrInputInvalid) {
				t.Errorf("✗ option error for %v: %v", failureCase.arguments, err)
			}

			if standardOutput.Len() != 0 {
				t.Errorf("✗ rejected options emitted success output: %v", failureCase.arguments)
			}

			if json.Valid(diagnostics.Bytes()) != failureCase.jsonError || !strings.Contains(diagnostics.String(), appstrings.Text("error.inputInvalid")) {
				t.Errorf("✗ diagnostic format for %v: %q", failureCase.arguments, diagnostics.String())
			}
		}
	}

	if !t.Failed() {
		t.Log("✓ option failures honor command scope and format")
	}
}
