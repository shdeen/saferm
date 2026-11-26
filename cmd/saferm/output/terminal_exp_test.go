package output

// Invariants tested:
//  1. Confirmation parsing: Only a complete affirmative line or a submitted affirmative default gives consent, and paths remain literal.

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/shdeen/saferm/internal/appstrings"
	"github.com/shdeen/saferm/internal/saferm"
)

// FuzzConfirmation checks invariant #1: Confirmation parsing.
// Test class: Expanded.
// Test layer: Fuzzing.
// Test kind: Permanent.
// What is under test:
// Arbitrary first-line answers follow yes/no/default semantics; EOF never gives
// consent. Valid Unicode paths, including template syntax and dollar signs,
// stay literal in rendered text and decoded JSON prompts.
func FuzzConfirmation(f *testing.F) {
	for _, answer := range []string{"y\n", "YES\n", "yes", "", "\n", "n\ny\n"} {
		f.Add(answer, "/original/$&/{{.Path}}", false, false)
		f.Add(answer, "/original/$&/{{.Path}}", true, true)
	}

	f.Fuzz(func(t *testing.T, answer, path string, defaultYes, asJSON bool) {
		if len(answer) > 4096 || len(path) > 4096 || !utf8.ValidString(path) {
			return
		}

		var promptOutput bytes.Buffer

		terminal := Terminal{Input: strings.NewReader(answer), Errors: &promptOutput, JSON: asJSON}
		confirmation := saferm.Confirmation{Prompt: appstrings.Text("prompt.restoreOverwrite"), Path: path, DefaultYes: defaultYes}
		approved, err := terminal.confirm(t.Context(), confirmation)
		answerLine, _, completeLine := strings.Cut(answer, "\n")
		normalizedAnswer := strings.ToLower(strings.TrimSpace(answerLine))

		validApproval := completeLine && (normalizedAnswer == "y" || normalizedAnswer == "yes" || (normalizedAnswer == "" && defaultYes))
		if err != nil || approved != validApproval {
			t.Errorf("✗ confirmation %q with default %v: %v, %v", answer, defaultYes, approved, err)
		}

		renderedPrompt := promptOutput.String()

		if asJSON {
			var decoded saferm.Confirmation
			if err := json.Unmarshal(promptOutput.Bytes(), &decoded); err != nil || decoded.Path != path {
				t.Errorf("✗ JSON path changed: %+v, %v", decoded, err)
			}

			renderedPrompt = decoded.Prompt
		}

		literalPrompt := strings.Join(strings.Split(confirmation.Prompt, "{{.Path}}"), path)

		hintKey := "prompt.defaultNoHint"
		if defaultYes {
			hintKey = "prompt.defaultYesHint"
		}

		literalPrompt += " " + appstrings.Text(hintKey)
		if renderedPrompt != literalPrompt && renderedPrompt != literalPrompt+"\n" {
			t.Errorf("✗ prompt changed path: %q", renderedPrompt)
		}

		if !t.Failed() {
			t.Log("✓ confirmation preserves paths and requires a complete affirmative answer")
		}
	})
}
