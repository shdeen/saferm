package output

// Invariants tested:
//  1. Configuration layout: A titled display aligns settings and wraps values without loss.
//  2. Configuration presence: Embedded sources, omitted settings, and empty values remain distinct.

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
