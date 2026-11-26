package main

// Invariants tested:
//  1. Help routing: All conventional help forms select the same page without operational setup.
//  2. Help option scope: Each page lists only its applicable options.
//  3. Help with arguments: Help flags select the command page even after its arguments.
//  4. Help page content: A help page holds the command line and its options, and no text after the options.
//  5. Option forms: A page shows its command's arguments and options in the usage line, a name for the value of each option that takes one, and the values that the option accepts.

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	"github.com/shdeen/saferm/internal/appstrings"
	"github.com/shdeen/saferm/internal/store"
)

// TestHelpRouting verifies invariant #1: Help routing.
// Test class: Core.
// Test kind: Permanent.
// What is under test:
// Root and every command return matching pages for help, -h, and --help;
// adding JSON leaves help as text. No home directory is needed for help.
func TestHelpRouting(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")

	for _, name := range []string{"", "delete", "move", "list", "view", "dispose", "restore", "purge", "cleanup", "config"} {
		arguments := []string{"saferm", "help"}
		if name != "" {
			arguments = append(arguments, name)
		}

		reference := renderTestHelp(t, arguments)
		if reference == "" {
			t.Errorf("✗ no help for %q", name)
		}

		for _, flag := range []string{"-h", "--help"} {
			arguments = []string{"saferm"}
			if name != "" {
				arguments = append(arguments, name, "--json")
			}

			arguments = append(arguments, flag)
			if rendered := renderTestHelp(t, arguments); rendered != reference {
				t.Errorf("✗ inconsistent help for %q with %s", name, flag)
			}
		}
	}

	if !t.Failed() {
		t.Log("✓ conventional help routing")
	}
}

// TestHelpOptions verifies invariant #2: Help option scope.
// Test class: Core.
// Test kind: Permanent.
// What is under test:
// Root help omits operational options; restore includes overwrite and JSON.
// Expected option texts are read from the maintained help templates.
func TestHelpOptions(t *testing.T) {
	root := renderTestHelp(t, []string{"saferm", "help"})

	restore := renderTestHelp(t, []string{"saferm", "help", "restore"})
	if strings.Contains(root, helpText("flag.overwrite")) {
		t.Error("✗ root shows restore-only option")
	}

	if strings.Contains(root, helpText("flag.json")) || strings.Contains(renderTestHelp(t, []string{"saferm", "help", "help"}), helpText("flag.json")) {
		t.Error("✗ help advertises JSON where only help text is produced")
	}

	if !strings.Contains(restore, helpText("flag.overwrite")) || !strings.Contains(restore, helpText("flag.json")) || !strings.Contains(restore, helpText("flag.force")) {
		t.Error("✗ restore lacks applicable options")
	}

	if !t.Failed() {
		t.Log("✓ help option scope")
	}
}

// TestHelpAfterArguments verifies invariant #3: Help with arguments.
// Test class: Core.
// Test kind: Permanent.
// What is under test:
// A help flag after source and reason arguments displays delete help without
// trying to inspect either supplied path or operational configuration. Either
// JSON alias preserves the same plain-text help.
func TestHelpAfterArguments(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")

	reference := renderTestHelp(t, []string{"saferm", "help", "delete"})

	for _, flag := range []string{"-h", "--help"} {
		for _, jsonFlag := range []string{"", "-j", "--json"} {
			arguments := []string{"saferm", "delete", "source", "reason", flag}
			if jsonFlag != "" {
				arguments = append(arguments, jsonFlag)
			}

			if rendered := renderTestHelp(t, arguments); rendered != reference {
				t.Errorf("✗ help after arguments: %v", arguments)
			}
		}
	}

	if !t.Failed() {
		t.Log("✓ help ignores operation arguments")
	}
}

// TestHelpPageContent verifies invariant #4: Help page content.
// Test class: Core.
// Test kind: Permanent.
// What is under test:
// The root page and the page of every command end with a line of the options
// list, so no text follows the options.
func TestHelpPageContent(t *testing.T) {
	for _, name := range []string{"", "delete", "move", "list", "view", "dispose", "restore", "purge", "cleanup", "config", "help"} {
		arguments := []string{"saferm", "help"}
		if name != "" {
			arguments = append(arguments, name)
		}

		pageLines := strings.Split(strings.TrimRight(renderTestHelp(t, arguments), "\n"), "\n")
		if lastLine := pageLines[len(pageLines)-1]; !strings.HasPrefix(strings.TrimLeft(lastLine, " "), "-") {
			t.Errorf("✗ help for %q has text after its options: %q", name, lastLine)
		}
	}

	if !t.Failed() {
		t.Log("✓ help pages end with their options")
	}
}

// TestHelpOptionForms verifies invariant #5: Option forms.
// Test class: Core.
// Test kind: Permanent.
// What is under test:
// The list page shows its two filters in the usage line and names the value of
// each filter. The line of each filter lists the values that it accepts, read
// from the status catalog for the statuses. No page shows a value under the
// library's generic name.
func TestHelpOptionForms(t *testing.T) {
	listPage := renderTestHelp(t, []string{"saferm", "help", "list"})
	shownTexts := []string{
		"Usage: saferm list [--type <type>] [--status <status>]\n",
		"--type <type>",
		"--status <status>",
		"delete or move",
	}

	for status := store.Completed; status <= store.Failed; status++ {
		shownTexts = append(shownTexts, appstrings.Text("status."+strconv.Itoa(status)))
	}

	for _, shownText := range shownTexts {
		if !strings.Contains(listPage, shownText) {
			t.Errorf("✗ list help lacks %q", shownText)
		}
	}

	for _, name := range []string{"delete", "move", "list", "restore", "cleanup", "config"} {
		commandPage := renderTestHelp(t, []string{"saferm", "help", name})
		if strings.Contains(commandPage, " value") || strings.Contains(commandPage, "VALUE") || strings.Contains(commandPage, " string") {
			t.Errorf("✗ %s help names a value generically: %q", name, commandPage)
		}
	}

	if !t.Failed() {
		t.Log("✓ help names each value and lists what it accepts")
	}
}

// Test class: Core: Helper.
func renderTestHelp(t *testing.T, arguments []string) string {
	t.Helper()

	command := newCommand()

	var buffer bytes.Buffer

	command.Writer = &buffer

	command.ErrWriter = &buffer
	if err := command.Run(t.Context(), arguments); err != nil {
		t.Errorf("✗ help %v: %v", arguments, err)
	}

	return buffer.String()
}
