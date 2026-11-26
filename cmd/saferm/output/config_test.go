package output

// Invariants tested:
//  1. Configuration layout: A titled display aligns settings and wraps values without loss.
//  2. Configuration presence: Embedded sources, omitted settings, and empty values remain distinct.
//  3. Replaced values: A setting that replaced a value from the file shows an error naming that value, in red only on a terminal.

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
	"github.com/shdeen/saferm/internal/appstrings"
	"github.com/shdeen/saferm/internal/config"
	"github.com/shdeen/saferm/internal/saferm"
)

// TestConfigurationLayout verifies invariant #1: Configuration layout.
// Test class: Core.
// Test kind: Permanent.
// What is under test:
// The active configuration has its catalog title and separator, aligned labels,
// and complete values that fit within 60 terminal columns, including wide text.
// These properties hold for templates checked out with LF or CRLF line endings.
func TestConfigurationLayout(t *testing.T) {
	for _, lineEnding := range []struct{ caseName, newline string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
		t.Run(lineEnding.caseName, func(t *testing.T) {
			originalTemplate := resultTemplate

			t.Cleanup(func() { resultTemplate = originalTemplate })

			resultTemplate = strings.ReplaceAll(strings.ReplaceAll(originalTemplate, "\r\n", "\n"), "\n", lineEnding.newline)

			trashPath := "/trash/" + strings.Repeat("界", 30)
			retentionDays := 17
			settings := config.Settings{
				Path: "/home/" + strings.Repeat("directory/", 6) + "config.toml", External: true,
				TrashPath: &trashPath, RetainPeriod: &retentionDays,
				RequireReview: []string{"~/documents requiring review/**", "**/important files/**", "**/.git/"},
			}

			var buffer bytes.Buffer
			if err := render(&buffer, false, saferm.Config, saferm.Result{Config: &settings}); err != nil {
				t.Errorf("✗ configuration display: %v", err)
			}

			configurationText := strings.ReplaceAll(buffer.String(), "\r\n", "\n")
			if !strings.HasPrefix(configurationText, appstrings.Text("config.title")+"\n"+appstrings.Text("config.separator")+"\n") {
				t.Error("✗ configuration title or separator missing")
			}

			for displayLine := range strings.SplitSeq(configurationText, "\n") {
				if runewidth.StringWidth(displayLine) > 60 {
					t.Errorf("✗ configuration exceeds 60 columns: %q", displayLine)
				}
			}

			assertConfigurationField(t, configurationText, "source", settings.Path)
			assertConfigurationField(t, configurationText, "trash", trashPath)
			assertConfigurationField(t, configurationText, "retention", "17")
			assertConfigurationField(t, configurationText, "review", strings.Join(settings.RequireReview, ", "))

			if !t.Failed() {
				t.Log("✓ configuration aligns and wraps complete values")
			}
		})
	}

	if !t.Failed() {
		t.Log("✓ configuration aligns and wraps complete values")
	}
}

// TestConfigurationPresence verifies invariant #2: Configuration presence.
// Test class: Core.
// Test kind: Permanent.
// What is under test:
// The embedded source is identified; absent settings remain unset; explicitly
// empty paths and rule lists display the empty marker, and zero days stays zero.
func TestConfigurationPresence(t *testing.T) {
	emptyPath := ""

	zeroDays := 0
	for _, testCase := range []struct {
		settings                             config.Settings
		trashText, retentionText, reviewText string
	}{
		{config.Settings{}, appstrings.Text("display.unset"), appstrings.Text("display.unset"), appstrings.Text("display.unset")},
		{config.Settings{TrashPath: &emptyPath, RetainPeriod: &zeroDays, RequireReview: []string{}}, appstrings.Text("config.empty"), "0", appstrings.Text("config.empty")},
	} {
		var buffer bytes.Buffer
		if err := render(&buffer, false, saferm.Config, saferm.Result{Config: &testCase.settings}); err != nil {
			t.Errorf("✗ configuration display: %v", err)
		}

		assertConfigurationField(t, buffer.String(), "source", appstrings.Text("display.embedded"))
		assertConfigurationField(t, buffer.String(), "trash", testCase.trashText)
		assertConfigurationField(t, buffer.String(), "retention", testCase.retentionText)
		assertConfigurationField(t, buffer.String(), "review", testCase.reviewText)
	}

	if !t.Failed() {
		t.Log("✓ configuration preserves missing and empty settings")
	}
}

// TestConfigurationInvalidValues verifies invariant #3: Replaced values.
// Test class: Expanded.
// Test layer: Coverage.
// What is under test:
// A trash path or retention period that replaced a value from the file is
// followed by the catalog error naming that value, and the other setting shows
// no error. A display written to a destination other than a terminal holds no
// color codes; the error text for a terminal is red and holds the same words.
func TestConfigurationInvalidValues(t *testing.T) {
	trashPath, retentionDays := "~/.saferm/trash", 30

	for _, testCase := range []struct {
		name, invalidTrash, invalidRetention string
	}{
		{"trash", "/blocked/trash", ""},
		{"retention", "", "-5"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			settings := config.Settings{TrashPath: &trashPath, RetainPeriod: &retentionDays, InvalidTrashPath: testCase.invalidTrash, InvalidRetainPeriod: testCase.invalidRetention}

			var buffer bytes.Buffer
			if err := render(&buffer, false, saferm.Config, saferm.Result{Config: &settings}); err != nil {
				t.Fatalf("✗ configuration display: %v", err)
			}

			displayedWords := strings.Join(strings.Fields(buffer.String()), " ")
			if strings.Contains(buffer.String(), "\x1b[") {
				t.Errorf("✗ color codes outside a terminal: %q", buffer.String())
			}

			for _, field := range []struct{ labelKey, value, invalidValue, nextLabelKey string }{
				{"trash", trashPath, testCase.invalidTrash, "retention"},
				{"retention", "30", testCase.invalidRetention, "review"},
			} {
				expectedWords := []string{appstrings.Text("config." + field.labelKey), field.value}
				if field.invalidValue != "" {
					expectedWords = append(expectedWords, invalidValueWords(t, field.invalidValue))
				}

				expectedWords = append(expectedWords, appstrings.Text("config."+field.nextLabelKey))
				if !strings.Contains(displayedWords, strings.Join(strings.Fields(strings.Join(expectedWords, " ")), " ")) {
					t.Errorf("✗ %s shows the wrong error: %q", field.labelKey, buffer.String())
				}
			}
		})
	}

	coloredText, err := invalidValueFormatter{colored: true}.format("-5")
	if err != nil || !strings.Contains(coloredText, "\x1b[31m") || strings.Join(strings.Fields(strings.NewReplacer("\x1b[31m", " ", "\x1b[0m", " ").Replace(coloredText)), " ") != invalidValueWords(t, "-5") {
		t.Errorf("✗ terminal error text: %q", coloredText)
	}

	if !t.Failed() {
		t.Log("✓ replaced values show their error")
	}
}

// invalidValueWords returns the catalog error for the replaced value, with its
// words separated by single spaces.
func invalidValueWords(t *testing.T, invalidValue string) string {
	t.Helper()

	errorText, err := RenderString(appstrings.Text("config.invalidValue"), invalidValue)
	if err != nil {
		t.Fatalf("💣 error text: %v", err)
	}

	return strings.Join(strings.Fields(errorText), " ")
}

// Test class: Core: Helper.
func assertConfigurationField(t *testing.T, configurationText, labelKey, configuredValue string) {
	t.Helper()

	fieldLabel := appstrings.Text("config." + labelKey)
	continuationPrefix := strings.Repeat(" ", 18)

	var displayedValue strings.Builder

	readingField := false
	for displayLine := range strings.SplitSeq(configurationText, "\n") {
		if strings.HasPrefix(displayLine, fieldLabel) {
			readingField = true

			if !strings.HasPrefix(displayLine, fieldLabel+strings.Repeat(" ", 18-len(fieldLabel))) {
				t.Errorf("✗ misaligned configuration label %q", fieldLabel)
			}
		} else if !readingField || !strings.HasPrefix(displayLine, continuationPrefix) {
			readingField = false

			continue
		}

		if len(displayLine) >= 18 {
			displayedValue.WriteString(displayLine[18:])
		}
	}

	if displayedValue.String() != configuredValue {
		t.Errorf("✗ configuration %q: displayed %q; configured %q", fieldLabel, displayedValue.String(), configuredValue)
	}
}
