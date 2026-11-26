//go:build unix

package review

// Invariants tested:
//  1. Literal characters: A one-character set or a backslash makes a wildcard character literal, in an item's name and in a leading directory name.
//  2. Unreadable leading directory: A pattern whose leading directory cannot be looked into makes matching neither fail nor protect an unrelated item.
//  3. Trailing backslash: A pattern that ends in a backslash is malformed.
//  4. Unreadable contents: When no visible item is protected, a directory source reports the first listed pattern that could protect an item inside a directory that cannot be read, together with every such directory. A directory that no pattern could apply inside is not read.
//  5. Backslash positions: A backslash before an ordinary character makes that character literal in the first directory name, in a later directory name, and in the final name alike, and a doubled backslash matches a backslash in a name.
//  6. Unreadable link target: A pattern for directories only protects a symbolic link whose target cannot be inspected, and not a link whose target does not exist.

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// TestLiteralCharacters verifies invariant #1: Literal characters.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// "star[*]" and "star\*" protect a file named "star*", and "star[*]" does not
// protect "starx". A pattern whose leading directory is written "Album\[1\]"
// protects a file under the directory "Album[1]" and not one under "Album1".
func TestLiteralCharacters(t *testing.T) {
	root, home := fixtureRoot(t)
	starFile := filepath.Join(home, "star*")
	plainFile := filepath.Join(home, "starx")
	bracketFile := filepath.Join(root, "photos", "Album[1]", "private", "f")
	digitFile := filepath.Join(root, "photos", "Album1", "private", "f")

	for _, file := range []string{starFile, plainFile, bracketFile, digitFile} {
		createItem(t, root, file, kindFile)
	}

	albumPattern := rootedPattern(root, `/photos/Album\[1\]/private/**`)

	for _, testCase := range []struct {
		pattern     string
		item        string
		isProtected bool
	}{
		{"star[*]", starFile, true},
		{"star[*]", plainFile, false},
		{`star\*`, starFile, true},
		{albumPattern, bracketFile, true},
		{albumPattern, digitFile, false},
	} {
		reportedPattern := matchItem(t, []string{testCase.pattern}, testCase.item, home)
		if testCase.isProtected != (reportedPattern == testCase.pattern) || (!testCase.isProtected && reportedPattern != "") {
			t.Errorf("✗ %q on %q: %q", testCase.pattern, testCase.item, reportedPattern)
		}
	}

	if !t.Failed() {
		t.Log("✓ sets and backslashes keep wildcard characters literal")
	}
}

// TestUnreadableLeadingDirectory verifies invariant #2: Unreadable leading directory.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// The first pattern leads through a directory that exists and cannot be looked
// into. A later pattern still protects the file that it matches, and a file
// that no pattern matches stays unprotected. The test fails when it runs as
// root, because root can look into every directory.
func TestUnreadableLeadingDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Fatal("💣 run this test as a regular user; root can look into every directory")
	}

	root, home := fixtureRoot(t)
	lockedDirectory := filepath.Join(root, "locked")
	keyFile := filepath.Join(home, "other", "key.pem")
	unrelatedFile := filepath.Join(home, "other", "notes.txt")

	createItem(t, root, filepath.Join(lockedDirectory, "inner"), kindDirectory)
	createItem(t, root, keyFile, kindFile)
	createItem(t, root, unrelatedFile, kindFile)

	if err := os.Chmod(lockedDirectory, 0); err != nil {
		t.Fatalf("💣 fixture permissions: %v", err)
	}

	t.Cleanup(func() {
		if err := os.Chmod(lockedDirectory, 0o700); err != nil { //nolint:gosec // G302: the directory needs its owner permissions back so that cleanup can remove it.
			t.Errorf("✗ restore fixture permissions: %v", err)
		}
	})

	if _, err := os.ReadDir(lockedDirectory); !os.IsPermission(err) {
		t.Fatalf("💣 fixture directory can still be read: %v", err)
	}

	patterns := []string{filepath.ToSlash(lockedDirectory) + "/inner/secret*", "*.pem"}

	if reportedPattern := matchItem(t, patterns, keyFile, home); reportedPattern != "*.pem" {
		t.Errorf("✗ later pattern lost after an unreadable leading directory: %q", reportedPattern)
	}

	if reportedPattern := matchItem(t, patterns, unrelatedFile, home); reportedPattern != "" {
		t.Errorf("✗ unrelated file protected: %q", reportedPattern)
	}

	if !t.Failed() {
		t.Log("✓ an unreadable leading directory leaves other matching intact")
	}
}

// TestTrailingBackslash verifies invariant #3: Trailing backslash.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// A pattern that ends in a backslash is malformed, with or without a trailing
// slash after it. A backslash followed by a character is well formed.
func TestTrailingBackslash(t *testing.T) {
	for pattern, isMalformed := range map[string]bool{`name\`: true, `name\/`: true, `name\*`: false, `name\\`: false} {
		if Malformed(pattern) != isMalformed {
			t.Errorf("✗ %q malformed: %v", pattern, !isMalformed)
		}
	}

	if !t.Failed() {
		t.Log("✓ a trailing backslash makes a pattern malformed")
	}
}

// TestUnreadableContents verifies invariant #4: Unreadable contents.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// The source old holds a.txt, the directory locked, and the directory
// docs/sealed. Neither locked nor sealed can be opened. Each case names the
// patterns, the pattern that must be reported, and the unreadable directories
// that must come with it.
//   - "~/Contracts/" and one empty pattern report nothing and no directory.
//   - "*.pem" reports itself with both directories.
//   - "~/Contracts/" before "*.key" and "*.pem" reports "*.key", the first
//     pattern that could apply inside an unreadable directory.
//   - A pattern whose fixed part is inside locked reports itself with locked
//     alone, because it could not apply inside sealed.
//   - "*.txt" reports itself with no directory, because a.txt is visible.
//
// The test fails when it runs as root, because root can open every directory.
func TestUnreadableContents(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Fatal("💣 run this test as a regular user; root can open every directory")
	}

	root, home := fixtureRoot(t)
	source := filepath.Join(home, "old")
	lockedDirectory := filepath.Join(source, "locked")
	sealedDirectory := filepath.Join(source, "docs", "sealed")
	createItem(t, root, filepath.Join(source, "a.txt"), kindFile)

	for _, unreadableDirectory := range []string{lockedDirectory, sealedDirectory} {
		createItem(t, root, filepath.Join(unreadableDirectory, "hidden.pem"), kindFile)

		if err := os.Chmod(unreadableDirectory, 0); err != nil {
			t.Fatalf("💣 fixture permissions: %v", err)
		}

		t.Cleanup(func() {
			if err := os.Chmod(unreadableDirectory, 0o700); err != nil { //nolint:gosec // G302: the directory needs its owner permissions back so that cleanup can remove it.
				t.Errorf("✗ restore fixture permissions: %v", err)
			}
		})
	}

	insidePattern := filepath.ToSlash(lockedDirectory) + "/secret/*.pem"

	for _, testCase := range []struct {
		patterns              []string
		reported              string
		unreadableDirectories []string
	}{
		{[]string{"~/Contracts/"}, "", nil},
		{[]string{""}, "", nil},
		{[]string{"*.pem"}, "*.pem", []string{sealedDirectory, lockedDirectory}},
		{[]string{"~/Contracts/", "*.key", "*.pem"}, "*.key", []string{sealedDirectory, lockedDirectory}},
		{[]string{insidePattern}, insidePattern, []string{lockedDirectory}},
		{[]string{"*.txt"}, "*.txt", nil},
	} {
		reportedPattern, unreadableDirectories := Match(testCase.patterns, source, true, home)
		if reportedPattern != testCase.reported || !slices.Equal(slices.Sorted(slices.Values(unreadableDirectories)), slices.Sorted(slices.Values(testCase.unreadableDirectories))) {
			t.Errorf("✗ %q: %q, %q", testCase.patterns, reportedPattern, unreadableDirectories)
		}
	}

	if !t.Failed() {
		t.Log("✓ unreadable directories are reported only where a pattern could apply")
	}
}

// TestBackslashPositions verifies invariant #5: Backslash positions.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// The file "My Docs/More Docs/My Doc.docx" has three neighbors, each with a
// backslash before the space in one of the three names. Three patterns write
// that backslash in the first directory name, in the later directory name, and
// in the final name. Each must protect exactly the items that the pattern
// without a backslash protects: the file, and none of the neighbors.
//
// Further cases name one pattern, one item, and whether the pattern must
// protect the item: a backslash in a directory name before a final name and
// before a wildcard, and a doubled backslash for a directory whose real name
// holds a backslash.
func TestBackslashPositions(t *testing.T) {
	root, home := fixtureRoot(t)
	nestedFile := filepath.Join(home, "My Docs", "More Docs", "My Doc.docx")
	nestedItems := []string{
		nestedFile,
		filepath.Join(home, `My\ Docs`, "More Docs", "My Doc.docx"),
		filepath.Join(home, "My Docs", `More\ Docs`, "My Doc.docx"),
		filepath.Join(home, "My Docs", "More Docs", `My\ Doc.docx`),
	}
	documentFile := filepath.Join(home, "My Docs", "My Doc.docx")
	backslashDirectoryFile := filepath.Join(home, `My\ Docs`, "My Doc.docx")

	for _, file := range append([]string{documentFile, backslashDirectoryFile}, nestedItems...) {
		createItem(t, root, file, kindFile)
	}

	plainPattern := "~/My Docs/More Docs/My Doc.docx"

	for _, escapedPattern := range []string{`~/My\ Docs/More Docs/My Doc.docx`, `~/My Docs/More\ Docs/My Doc.docx`, `~/My Docs/More Docs/My\ Doc.docx`} {
		for _, item := range nestedItems {
			plainProtects := matchItem(t, []string{plainPattern}, item, home) == plainPattern
			escapedProtects := matchItem(t, []string{escapedPattern}, item, home) == escapedPattern

			if plainProtects != (item == nestedFile) || escapedProtects != plainProtects {
				t.Errorf("✗ %q on %q: protected %v, and %v without the backslash", escapedPattern, item, escapedProtects, plainProtects)
			}
		}
	}

	for _, testCase := range []struct {
		pattern     string
		item        string
		isProtected bool
	}{
		{`~/My\ Docs/My\ Doc.docx`, documentFile, true},
		{`~/My Docs/My\ Doc.docx`, documentFile, true},
		{`~/My\ Docs/*.docx`, documentFile, true},
		{`~/My\ Docs/My\ Doc.docx`, backslashDirectoryFile, false},
		{`~/My\ Docs/*.docx`, backslashDirectoryFile, false},
		{`~/My\\ Docs/*.docx`, backslashDirectoryFile, true},
		{`~/My\\ Docs/My Doc.docx`, backslashDirectoryFile, true},
		{`~/My\\ Docs/*.docx`, documentFile, false},
	} {
		reportedPattern := matchItem(t, []string{testCase.pattern}, testCase.item, home)
		if testCase.isProtected != (reportedPattern == testCase.pattern) || (!testCase.isProtected && reportedPattern != "") {
			t.Errorf("✗ %q on %q: %q", testCase.pattern, testCase.item, reportedPattern)
		}
	}

	if !t.Failed() {
		t.Log("✓ a backslash means the same in every name of a pattern")
	}
}

// TestUnreadableLinkTarget verifies invariant #6: Unreadable link target.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// The link work/.git points to a directory inside a directory that cannot be
// looked into, so the link can be seen and its target cannot be inspected.
// ".git/" protects that link, because it may point to a directory. The link
// plain/.git points to nothing, and the link under/.git points to a path
// below a regular file, where nothing can exist. ".git/" protects neither.
// The test fails when it runs as root, because root can look into every
// directory.
func TestUnreadableLinkTarget(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Fatal("💣 run this test as a regular user; root can look into every directory")
	}

	root, home := fixtureRoot(t)
	lockedDirectory := filepath.Join(root, "locked")
	hiddenLink := filepath.Join(home, "work", ".git")
	danglingLink := filepath.Join(home, "plain", ".git")
	blockedLink := filepath.Join(home, "under", ".git")
	regularFile := filepath.Join(root, "regular")

	createItem(t, root, filepath.Join(lockedDirectory, "repository"), kindDirectory)
	createItem(t, root, regularFile, kindFile)

	for link, target := range map[string]string{hiddenLink: filepath.Join(lockedDirectory, "repository"), danglingLink: filepath.Join(root, "missing"), blockedLink: filepath.Join(regularFile, "repository")} {
		if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
			t.Fatalf("💣 fixture parent: %v", err)
		}

		if err := os.Symlink(target, link); err != nil {
			t.Fatalf("💣 fixture link: %v", err)
		}
	}

	if err := os.Chmod(lockedDirectory, 0); err != nil {
		t.Fatalf("💣 fixture permissions: %v", err)
	}

	t.Cleanup(func() {
		if err := os.Chmod(lockedDirectory, 0o700); err != nil { //nolint:gosec // G302: the directory needs its owner permissions back so that cleanup can remove it.
			t.Errorf("✗ restore fixture permissions: %v", err)
		}
	})

	if _, err := os.Stat(hiddenLink); !os.IsPermission(err) {
		t.Fatalf("💣 fixture link target can still be inspected: %v", err)
	}

	if reportedPattern := matchItem(t, []string{".git/"}, hiddenLink, home); reportedPattern != ".git/" {
		t.Errorf("✗ a link with a target that cannot be inspected is unprotected: %q", reportedPattern)
	}

	for _, missingTargetLink := range []string{danglingLink, blockedLink} {
		if reportedPattern := matchItem(t, []string{".git/"}, missingTargetLink, home); reportedPattern != "" {
			t.Errorf("✗ the link %q to a target that does not exist is protected: %q", missingTargetLink, reportedPattern)
		}
	}

	if !t.Failed() {
		t.Log("✓ a link whose target cannot be inspected needs approval")
	}
}
