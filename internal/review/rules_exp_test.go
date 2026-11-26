package review

// Invariants tested:
//  1. Normalized review paths: Absolute rules follow symbolic links in parent directories.
//  2. Unusable inputs: A source, a home directory, or a link target that does not exist makes matching neither fail nor loop; a directory source that cannot be inspected needs approval when a pattern could apply inside it.
//  3. Arbitrary review patterns: A reported pattern is one of the configured patterns, a pattern that ends in a slash protects a file only through a directory above the file, and a malformed pattern protects every item.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAbsolutePatternParent verifies invariant #1: Normalized review paths.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
// A rule using a symbolic parent directory still protects a normalized source.
func TestAbsolutePatternParent(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("💣 fixture home: %v", err)
	}

	parent := filepath.Join(home, "actual")
	alias := filepath.Join(home, "alias")

	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatalf("💣 parent: %v", err)
	}

	if err := os.Symlink(parent, alias); err != nil {
		t.Fatalf("💣 alias: %v", err)
	}

	pattern := filepath.ToSlash(filepath.Join(alias, "protected*"))

	reportedPattern, _ := Match([]string{pattern}, filepath.Join(parent, "protected.txt"), false, home)
	if reportedPattern != pattern {
		t.Errorf("✗ normalized source escaped review: %q", reportedPattern)
	}

	if !t.Failed() {
		t.Log("✓ symbolic parent paths preserve review requirements")
	}
}

// TestUnusableInputs verifies invariant #2: Unusable inputs.
// Test class: Expanded.
// Test layer: Hardening/adversarial.
// Test kind: Permanent.
// What is under test:
//   - A missing home directory is used as written: "~/x" protects x under it.
//   - A pattern that names a dangling link protects the link and nothing else.
//   - A directory that links back to its own parent is walked once: the walk
//     ends, and "*.pem" reports nothing.
//   - A missing source that is submitted as a directory reports "*.pem",
//     which could apply inside it, with the source as the unreadable directory.
//     With only "~/Contracts/", it reports nothing.
//   - An empty source and a relative source report nothing for "*.pem".
func TestUnusableInputs(t *testing.T) {
	root, home := fixtureRoot(t)
	missingHome := filepath.Join(root, "missing", "home")
	danglingLink := filepath.Join(home, "dangling")
	looped := filepath.Join(home, "looped")
	missingSource := filepath.Join(home, "absent")
	createItem(t, root, filepath.Join(looped, "notes.txt"), kindFile)

	for link, target := range map[string]string{danglingLink: filepath.Join(home, "nowhere"), filepath.Join(looped, "parent"): looped} {
		if err := os.Symlink(target, link); err != nil {
			t.Fatalf("💣 fixture link: %v", err)
		}
	}

	for _, testCase := range []struct {
		patterns              []string
		source                string
		isDirectory           bool
		home                  string
		reported              string
		unreadableDirectories []string
	}{
		{[]string{"~/x"}, filepath.Join(missingHome, "x"), false, missingHome, "~/x", nil},
		{[]string{"~/dangling"}, danglingLink, false, home, "~/dangling", nil},
		{[]string{"~/dangling"}, filepath.Join(home, "nowhere"), false, home, "", nil},
		{[]string{"*.pem"}, looped, true, home, "", nil},
		{[]string{"*.pem"}, missingSource, true, home, "*.pem", []string{missingSource}},
		{[]string{"~/Contracts/"}, missingSource, true, home, "", nil},
		{[]string{"*.pem"}, "", false, home, "", nil},
		{[]string{"*.pem"}, filepath.Join("relative", "notes.txt"), false, home, "", nil},
	} {
		reportedPattern, unreadableDirectories := Match(testCase.patterns, testCase.source, testCase.isDirectory, testCase.home)
		if reportedPattern != testCase.reported || len(unreadableDirectories) != len(testCase.unreadableDirectories) || (len(unreadableDirectories) == 1 && unreadableDirectories[0] != testCase.unreadableDirectories[0]) {
			t.Errorf("✗ %q on %q: %q, %q", testCase.patterns, testCase.source, reportedPattern, unreadableDirectories)
		}
	}

	if !t.Failed() {
		t.Log("✓ unusable inputs neither fail nor loop")
	}
}

// FuzzReviewPatterns checks invariant #3: Arbitrary review patterns.
// Test class: Expanded.
// Test layer: Fuzzing.
// Test kind: Permanent.
// What is under test:
// Arbitrary pattern and source text does not panic, and any pattern reported
// for it is the configured pattern. A malformed pattern is reported for every
// source. For a pattern that ends in a slash, two regular files with different
// names in one directory get the same answer, because only a directory above
// them can protect them.
func FuzzReviewPatterns(f *testing.F) {
	root, home := fixtureRoot(f)
	firstFile := filepath.Join(home, "folder", "first.txt")
	secondFile := filepath.Join(home, "folder", "second.dat")

	createItem(f, root, firstFile, kindFile)
	createItem(f, root, secondFile, kindFile)

	f.Add("**/.git/", "/project/.git")
	f.Add("*", "file")
	f.Add("[", "source")
	f.Add("*.txt/", "folder/first.txt")
	f.Add("first.txt/", "")
	f.Add("fold*/", "~")
	f.Add(filepath.ToSlash(firstFile)+"/", firstFile)
	f.Fuzz(func(t *testing.T, pattern, source string) {
		patterns := []string{"", pattern}

		reportedPattern, _ := Match(patterns, source, false, home)
		if reportedPattern != "" && reportedPattern != pattern {
			t.Errorf("✗ reported pattern %q was not configured", reportedPattern)
		}

		if Malformed(pattern) && reportedPattern != pattern {
			t.Errorf("✗ malformed pattern %q left %q unprotected", pattern, source)
		}

		if strings.HasSuffix(filepath.ToSlash(pattern), "/") {
			firstAnswer, _ := Match(patterns, firstFile, false, home)
			secondAnswer, _ := Match(patterns, secondFile, false, home)

			if firstAnswer != secondAnswer {
				t.Errorf("✗ pattern %q told two files of one directory apart: %q, %q", pattern, firstAnswer, secondAnswer)
			}
		}

		if !t.Failed() {
			t.Log("✓ arbitrary patterns preserve matching rules")
		}
	})

	if !f.Failed() {
		f.Log("✓ arbitrary review patterns handled")
	}
}
