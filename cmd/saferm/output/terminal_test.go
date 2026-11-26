package output

// Invariants tested:
//  1. JSON configuration: Default configuration is valid JSON even when active TOML is malformed.
//  2. Reset consent: Configuration resets only after affirmative terminal input.
//  3. Restore review output: A pending occupying delete is rendered alongside the identifiable restore error.
//  4. Confirmation input: Submitted defaults apply; refusal and EOF preserve the existing item.

import (
	"bytes"
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
)

// TestDefaultJSON verifies invariant #1: JSON configuration.
// Test class: Core.
// Test kind: Permanent.
// What is under test:
// The default configuration renders as valid structured JSON and contains the
// values decoded from the embedded source without reading malformed active TOML.
func TestDefaultJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	configPath := filepath.Join(home, config.Directory, config.Filename)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatalf("💣 fixture directory: %v", err)
	}

	if err := os.WriteFile(configPath, []byte("invalid = ["), 0o600); err != nil {
		t.Fatalf("💣 fixture file: %v", err)
	}

	var buffer bytes.Buffer

	terminal := Terminal{Input: strings.NewReader(""), Output: &buffer, Errors: &bytes.Buffer{}, JSON: true}

	err := terminal.Execute(t.Context(), &saferm.Request{Command: saferm.Config, Default: true})
	if err != nil || !json.Valid(buffer.Bytes()) {
		t.Errorf("✗ default JSON: %v, %q", err, buffer.String())
	}

	var settings config.Settings
	if err = json.Unmarshal(buffer.Bytes(), &settings); err != nil {
		t.Errorf("✗ decoding JSON: %v", err)
	}

	defaults, err := config.Decode(config.DefaultDocument)
	if err != nil {
		t.Fatalf("💣 embedded config: %v", err)
	}

	if settings.TrashPath == nil || defaults.TrashPath == nil {
		t.Error("✗ default path missing")
	} else if *settings.TrashPath != *defaults.TrashPath {
		t.Error("✗ default path altered")
	}

	if !t.Failed() {
		t.Log("✓ independent default JSON")
	}
}

// TestResetConsent verifies invariant #2: Reset consent.
// Test class: Core.
// Test kind: Permanent.
// What is under test:
// Reset replaces the file after an affirmative response or submitted default;
// refusal and EOF preserve its bytes. JSON prompts and results remain valid JSON.
func TestResetConsent(t *testing.T) {
	for _, testCase := range []struct {
		input         string
		approve, json bool
	}{
		{"n\n", false, false},
		{"", false, false},
		{"\n", true, false},
		{"y\n", true, true},
		{"n\n", false, true},
	} {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)

		configPath := filepath.Join(home, config.Directory, config.Filename)
		if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
			t.Fatalf("💣 fixture directory: %v", err)
		}

		original := "retain-period = 7\n"
		if err := os.WriteFile(configPath, []byte(original), 0o600); err != nil {
			t.Fatalf("💣 fixture config: %v", err)
		}

		var output, prompts bytes.Buffer

		terminal := Terminal{Input: strings.NewReader(testCase.input), Output: &output, Errors: &prompts, JSON: testCase.json}

		request := saferm.Request{Command: saferm.Config, Setup: true, Reset: true}
		if err := terminal.Execute(t.Context(), &request); err != nil {
			t.Errorf("✗ reset: %v", err)
		}

		if request.Confirmed {
			t.Error("✗ terminal confirmation changed the caller's request")
		}

		document, err := os.ReadFile(configPath)
		if err != nil {
			t.Errorf("✗ config disappeared: %v", err)
		}

		if testCase.approve {
			if string(document) != config.DefaultDocument {
				t.Error("✗ approved reset omitted defaults")
			}
		} else if string(document) != original {
			t.Error("✗ declined reset changed config")
		}

		if testCase.json && (!json.Valid(output.Bytes()) || !json.Valid(prompts.Bytes())) {
			t.Error("✗ JSON interaction emitted invalid JSON")
		}
	}

	if !t.Failed() {
		t.Log("✓ configuration replacement requires consent")
	}
}

// TestRestorePendingOutput verifies invariant #3: Restore review output.
// Test class: Core.
// Test kind: Permanent.
// What is under test:
// Text and JSON both expose the pending occupant delete while returning the
// restore error. Force omits confirmation without bypassing the review rule.
func TestRestorePendingOutput(t *testing.T) {
	for _, asJSON := range []bool{false, true} {
		t.Run("json="+strconv.FormatBool(asJSON), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)

			source := filepath.Join(home, "source")
			if err := os.WriteFile(source, []byte("original"), 0o600); err != nil {
				t.Fatalf("💣 original file: %v", err)
			}

			if _, err := saferm.Run(t.Context(), &saferm.Request{Command: saferm.Config, Setup: true}); err != nil {
				t.Fatalf("💣 configuration: %v", err)
			}

			original, err := saferm.Run(t.Context(), &saferm.Request{Command: saferm.Delete, Source: source, Reason: "retain original"})
			if err != nil || len(original.Actions) != 1 {
				t.Fatalf("💣 original delete: %+v, %v", original, err)
			}

			if err := os.WriteFile(source, []byte("occupant"), 0o600); err != nil {
				t.Fatalf("💣 occupant file: %v", err)
			}

			settings := fmt.Sprintf("trash-path = %q\nrequire-review = [%q]\n", original.Actions[0].TrashPath, source)
			if err := os.WriteFile(filepath.Join(home, config.Directory, config.Filename), []byte(settings), 0o600); err != nil {
				t.Fatalf("💣 review configuration: %v", err)
			}

			var results, diagnostics bytes.Buffer

			terminal := Terminal{Input: strings.NewReader(""), Output: &results, Errors: &diagnostics, JSON: asJSON}

			err = terminal.Execute(t.Context(), &saferm.Request{Command: saferm.Restore, ID: strconv.FormatInt(original.Actions[0].ID, 10), Overwrite: true, Force: true})
			if !errors.Is(err, errs.ErrRestorePending) || diagnostics.Len() != 0 {
				t.Errorf("✗ force prompted or bypassed review: %v, %q", err, diagnostics.String())
			}

			if err == nil {
				return
			}

			if writeErr := terminal.Failure(err); writeErr != nil || !strings.Contains(diagnostics.String(), errs.ErrRestorePending.Error()) {
				t.Errorf("✗ restore error output: %q, %v", diagnostics.String(), writeErr)
			}

			if asJSON {
				var printedResult saferm.Result
				if decodeErr := json.Unmarshal(results.Bytes(), &printedResult); decodeErr != nil || len(printedResult.Actions) != 1 {
					t.Fatalf("💣 no single action in the restore JSON to inspect: %q, %v", results.String(), decodeErr)
				}

				if pending := printedResult.Actions[0]; pending.Type != saferm.Delete || pending.Status != store.Pending || pending.ID == original.Actions[0].ID {
					t.Errorf("✗ pending delete JSON: %+v", pending)
				}
			} else if !strings.Contains(results.String(), appstrings.Text("status.2")) || strings.Contains(results.String(), appstrings.Text("status.4")) {
				t.Errorf("✗ pending restore text claims wrong outcome: %q", results.String())
			}

			if !t.Failed() {
				t.Log("✓ restore case preserves its required outcome")
			}
		})
	}

	if !t.Failed() {
		t.Log("✓ pending delete is rendered with the restore error")
	}
}

// TestConfirmation verifies invariant #4: Confirmation input.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// A newline accepts a yes default, y and yes accept, n declines, EOF declines,
// and a no default declines an empty submission. The prompt includes the catalog
// question and the terminal hint for the actual default.
func TestConfirmation(t *testing.T) {
	for _, testCase := range []struct {
		input                string
		defaultYes, approved bool
	}{
		{"\n", true, true}, {"y\n", false, true}, {"YES\n", false, true}, {"n\n", true, false}, {"", true, false}, {"y", true, false}, {"\n", false, false},
	} {
		var buffer bytes.Buffer

		terminal := Terminal{Input: strings.NewReader(testCase.input), Output: &buffer, Errors: &buffer}
		prompt := appstrings.Text("prompt.configReset")

		approved, err := terminal.confirm(t.Context(), saferm.Confirmation{Prompt: prompt, DefaultYes: testCase.defaultYes})
		if err != nil || approved != testCase.approved {
			t.Errorf("✗ response %q default %v: %v, %v", testCase.input, testCase.defaultYes, approved, err)
		}

		hintKey := "prompt.defaultNoHint"
		if testCase.defaultYes {
			hintKey = "prompt.defaultYesHint"
		}

		if buffer.String() != prompt+" "+appstrings.Text(hintKey)+"\n" {
			t.Error("✗ configured question or default answer hint missing")
		}
	}

	if !t.Failed() {
		t.Log("✓ confirmation semantics")
	}
}
