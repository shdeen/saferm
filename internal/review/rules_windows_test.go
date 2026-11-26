package review

// Invariants tested:
//  1. Windows separators: A backslash in a pattern separates directories.
//  2. Windows drives: A pattern that starts with a drive applies only to that drive, and any other path pattern applies from the root of the drive that holds the item.
//  3. Directory letter case: Letter case follows the directory that holds a name, also where two directories of one volume differ.

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// TestWindowsSeparators verifies invariant #1: Windows separators.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// A pattern written with backslashes, ending in one, protects a file under the
// directory that it names and not a file under a sibling directory. A home
// pattern written with backslashes protects the same file.
func TestWindowsSeparators(t *testing.T) {
	root, home := fixtureRoot(t)
	privateFile := filepath.Join(home, "private", "notes.txt")
	publicFile := filepath.Join(home, "public", "notes.txt")

	createItem(t, root, privateFile, kindFile)
	createItem(t, root, publicFile, kindFile)

	directoryPattern := filepath.Join(home, "private") + `\`
	if Malformed(directoryPattern) {
		t.Errorf("✗ a trailing backslash made %q malformed", directoryPattern)
	}

	for _, testCase := range []struct {
		pattern     string
		item        string
		isProtected bool
	}{
		{directoryPattern, privateFile, true},
		{directoryPattern, publicFile, false},
		{`~\private\*.txt`, privateFile, true},
		{`~\private\*.txt`, publicFile, false},
	} {
		reportedPattern := matchItem(t, []string{testCase.pattern}, testCase.item, home)
		if testCase.isProtected != (reportedPattern == testCase.pattern) || (!testCase.isProtected && reportedPattern != "") {
			t.Errorf("✗ %q on %q: %q", testCase.pattern, testCase.item, reportedPattern)
		}
	}

	if !t.Failed() {
		t.Log("✓ backslashes separate directories in patterns")
	}
}

// TestWindowsDrives verifies invariant #2: Windows drives.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// The same path pattern is written three ways for a file under the fixture
// root: with the root's own drive, with another drive letter, and with no
// drive. The first and the third protect the file. The second does not, and
// it also reports nothing for the directory that holds the file.
func TestWindowsDrives(t *testing.T) {
	root, home := fixtureRoot(t)
	dataFile := filepath.Join(root, "Data", "x")
	createItem(t, root, dataFile, kindFile)

	drive := filepath.VolumeName(root)
	driveless := filepath.ToSlash(root[len(drive):]) + "/Data/**"

	otherDrive := "D:"
	if drive == otherDrive || drive == "d:" {
		otherDrive = "C:"
	}

	for _, testCase := range []struct {
		pattern     string
		isProtected bool
	}{
		{drive + driveless, true},
		{otherDrive + driveless, false},
		{driveless, true},
	} {
		reportedPattern := matchItem(t, []string{testCase.pattern}, dataFile, home)
		if testCase.isProtected != (reportedPattern == testCase.pattern) || (!testCase.isProtected && reportedPattern != "") {
			t.Errorf("✗ %q on %q: %q", testCase.pattern, dataFile, reportedPattern)
		}
	}

	if reportedPattern, unreadableDirectories := Match([]string{otherDrive + driveless}, filepath.Dir(dataFile), true, home); reportedPattern != "" || len(unreadableDirectories) != 0 {
		t.Errorf("✗ a pattern for another drive reported %q, %q", reportedPattern, unreadableDirectories)
	}

	if !t.Failed() {
		t.Log("✓ a drive in a pattern limits the pattern to that drive")
	}
}

// TestDirectoryLetterCase verifies invariant #3: Directory letter case.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// Two directories share one volume, and fsutil marks one of them as case
// sensitive. ".git/" protects a file under ".GIT" in the other directory and
// does not protect one under ".GIT" in the case-sensitive directory. A path
// pattern that reaches each ".GIT" through a wildcard gives the same two
// results, because the directory of the deepest name decides. The test fails
// when fsutil refuses to mark the directory.
func TestDirectoryLetterCase(t *testing.T) {
	root, home := fixtureRoot(t)
	caseSensitive := filepath.Join(root, "sensitive")
	createItem(t, root, caseSensitive, kindDirectory)

	marking := exec.CommandContext(t.Context(), "fsutil.exe", "file", "setCaseSensitiveInfo", caseSensitive, "enable") //nolint:gosec // G204: the only variable argument is the fixture directory that this test created.
	if output, err := marking.CombinedOutput(); err != nil {
		t.Fatalf("💣 fsutil refused to mark %q case sensitive: %v: %s", caseSensitive, err, output)
	}

	sensitiveFile := filepath.Join(caseSensitive, ".GIT", "config")
	insensitiveFile := filepath.Join(root, "insensitive", ".GIT", "config")
	createItem(t, root, sensitiveFile, kindFile)
	createItem(t, root, insensitiveFile, kindFile)

	if reportedPattern := matchItem(t, []string{".git/"}, insensitiveFile, home); reportedPattern != ".git/" {
		t.Errorf("✗ a directory that ignores case left .GIT unprotected: %q", reportedPattern)
	}

	if reportedPattern := matchItem(t, []string{".git/"}, sensitiveFile, home); reportedPattern != "" {
		t.Errorf("✗ a case-sensitive directory protected .GIT: %q", reportedPattern)
	}

	insensitivePattern := rootedPattern(root, "/ins*/.git/")
	if reportedPattern := matchItem(t, []string{insensitivePattern}, insensitiveFile, home); reportedPattern != insensitivePattern {
		t.Errorf("✗ a path pattern left .GIT unprotected in a directory that ignores case: %q", reportedPattern)
	}

	if reportedPattern := matchItem(t, []string{rootedPattern(root, "/sen*/.git/")}, sensitiveFile, home); reportedPattern != "" {
		t.Errorf("✗ a path pattern protected .GIT in a case-sensitive directory: %q", reportedPattern)
	}

	if !t.Failed() {
		t.Log("✓ letter case follows each directory of one volume")
	}
}
