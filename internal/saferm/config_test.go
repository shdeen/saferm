package saferm

// Invariants tested:
//  1. Malformed rule report: Displaying a configuration that holds a malformed review pattern returns the settings together with an error that names the pattern.

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/shdeen/saferm/internal/errs"
)

// TestMalformedRuleReport verifies invariant #1: Malformed rule report.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// With "*.pem" and "[abc" configured, the config command returns the loaded
// settings and ErrRuleMalformed naming "[abc". With "*.pem" alone, it returns
// the settings and no error.
func TestMalformedRuleReport(t *testing.T) {
	home := isolateHome(t)

	for _, testCase := range []struct {
		patterns    []string
		isMalformed bool
	}{
		{[]string{"*.pem", "[abc"}, true},
		{[]string{"*.pem"}, false},
	} {
		writeConfig(t, home, fmt.Sprintf("trash-path = %q\nrequire-review = [%s]\n", filepath.Join(home, "trash"), quoteList(testCase.patterns)))

		result, err := Run(t.Context(), &Request{Command: Config})
		if result.Config == nil || !reflect.DeepEqual(result.Config.RequireReview, testCase.patterns) {
			t.Errorf("✗ settings with %q: %+v", testCase.patterns, result)
		}

		if testCase.isMalformed != errors.Is(err, errs.ErrRuleMalformed) || (!testCase.isMalformed && err != nil) {
			t.Errorf("✗ error with %q: %v", testCase.patterns, err)
		}

		if testCase.isMalformed && (err == nil || !strings.Contains(err.Error(), strconv.Quote("[abc"))) {
			t.Errorf("✗ error does not name the malformed pattern: %v", err)
		}
	}

	if !t.Failed() {
		t.Log("✓ config reports a malformed rule beside the settings")
	}
}

// quoteList returns the patterns as the elements of a TOML array.
func quoteList(patterns []string) string {
	quoted := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		quoted = append(quoted, strconv.Quote(pattern))
	}

	return strings.Join(quoted, ", ")
}
