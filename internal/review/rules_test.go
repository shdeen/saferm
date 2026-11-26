package review

// Invariants tested:
//  1. Pattern rules: A pattern protects the items that its kind, wildcards, sets, alternatives, and trailing slash describe, and everything inside a matched directory.
//  2. Spelled-out names: The symbolic links in the leading names that a path pattern spells out are resolved, a pattern that spells out a link's path protects the link and its target, and any other pattern that matches a link protects the link alone.
//  3. Letter case: Letter case matters for a name exactly when the directory that holds the name treats two spellings as two names.
//  4. Malformed patterns: A malformed pattern protects every item, and list order still decides which pattern is reported.
//  5. Malformed text: A pattern is malformed when its text, without a trailing slash, holds a set or alternatives that are not closed, a set that lists no character, or a closing brace with no opening brace.
//  6. Malformed list: Checking a list reports its first malformed pattern as an identifiable error, and reports nothing for a list that has none.
//  7. Directory contents: A directory that no pattern protects reports the first listed pattern that protects an item inside it, and a symbolic link inside it is judged as a link.
//  8. Directory boundary: A wildcard that stands for one name does not match the directory before it, and ** does.

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/shdeen/saferm/internal/errs"
)

// Kinds of item that a fixture creates.
const (
	kindFile          = "file"
	kindDirectory     = "directory"
	kindDirectoryLink = "link to a directory"
	kindFileLink      = "link to a file"
)

// TestPatternExamples verifies invariant #1: Pattern rules.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// Each case names one pattern, one item under a fixture root whose home is
// home/me, and whether the pattern must protect the item. A rooted pattern is
// written with the fixture root in place of the filesystem root. An item that
// starts with a slash is used as written and is not created.
func TestPatternExamples(t *testing.T) {
	for _, example := range []struct {
		pattern     string
		item        string
		itemKind    string
		isRooted    bool
		isProtected bool
	}{
		{".git/", "home/me/proj/.git/objects/ab", kindFile, false, true},
		{".git/", "home/me/plain/.git", kindFile, false, false},
		{".git/", "home/me/work/.git", kindDirectoryLink, false, true},
		{".git/", "home/me/work/.git", kindFileLink, false, false},
		{".*/", "home/me/.ssh/id_rsa", kindFile, false, true},
		{".*/", "home/me/proj/.cache/data", kindFile, false, true},
		{".*/", "home/me/.zshrc", kindFile, false, false},
		{"*.pem", "home/me/keys/a.pem", kindFile, false, true},
		{"*.pem", "home/me/keys/a.pem.bak", kindFile, false, false},
		{"*.bak", "home/me/.zshrc.bak", kindFile, false, true},
		{"*.p*", "home/me/.p", kindFile, false, true},
		{"*.p*", "home/me/a.pop", kindFile, false, true},
		{"*log*", "home/me/catalog2", kindFile, false, true},
		{"photo?.jpg", "home/me/photo1.jpg", kindFile, false, true},
		{"photo?.jpg", "home/me/photo10.jpg", kindFile, false, false},
		{"~/*.txt", "home/me/a.txt", kindFile, false, true},
		{"~/*.txt", "home/me/notes/a.txt", kindFile, false, false},
		{"~", "home/me/notes/a.txt", kindFile, false, true},
		{"/", "/etc/hosts", "", false, true},
		{"a[!x]b", "home/me/a-b", kindFile, false, true},
		{"a[!x]b", "home/me/a/b", kindFile, false, false},
		{"~/work/a[!x]b", "home/me/work/a/b", kindFile, false, true},
		{"build/output/", "build/output/x/f", kindFile, true, true},
		{"build/output/", "home/me/build/output/f", kindFile, true, false},
		{"~/build/output/", "home/me/build/output/f", kindFile, false, true},
		{"**/build/output/", "home/me/build/output/f", kindFile, true, true},
		{"~/private/**", "home/me/private", kindDirectory, false, true},
		{"~/private", "home/me/private/deep/f", kindFile, false, true},
		{"{secrets,keys}/", "home/me/app/secrets/token", kindFile, false, true},
		{"/{secrets,keys}/", "home/me/app/secrets/token", kindFile, false, false},
		{"~/Projects/**/secrets/", "home/me/Projects/a/b/secrets/token", kindFile, false, true},
		{"~/Projects/*/secrets/", "home/me/Projects/a/b/secrets/token", kindFile, false, false},
		{"!draft", "home/me/!draft", kindFile, false, true},
		{"#notes", "home/me/#notes", kindFile, false, true},
	} {
		t.Run(example.pattern+" "+example.item+" "+example.itemKind, func(t *testing.T) {
			root, home := fixtureRoot(t)

			pattern := example.pattern
			if example.isRooted {
				pattern = rootedPattern(root, pattern)
			}

			item := filepath.FromSlash(example.item)
			if !strings.HasPrefix(example.item, "/") {
				item = filepath.Join(root, item)
				createItem(t, root, item, example.itemKind)
			}

			reportedPattern := matchItem(t, []string{pattern}, item, home)
			if example.isProtected && reportedPattern != pattern {
				t.Errorf("✗ %q left %q unprotected: %q", pattern, item, reportedPattern)
			}

			if !example.isProtected && reportedPattern != "" {
				t.Errorf("✗ %q protected %q", reportedPattern, item)
			}

			if !t.Failed() {
				t.Log("✓ the example gives its listed result")
			}
		})
	}

	if !t.Failed() {
		t.Log("✓ patterns protect the listed items and no others")
	}
}

// TestLinkedPatterns verifies invariant #2: Spelled-out names.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// MyDocuments is a link to the directory "My Documents". A pattern written through
// the link protects items under the directory. A pattern that names the link
// protects the link, the directory, and the directory's contents, and leaves a
// sibling directory alone. A pattern that matches the link through a wildcard
// protects the link alone. A pattern written through a link to "Album[1]"
// protects that directory's contents and not those of its sibling "Album1".
func TestLinkedPatterns(t *testing.T) {
	root, home := fixtureRoot(t)
	documents := filepath.Join(home, "My Documents")
	documentsLink := filepath.Join(home, "MyDocuments")
	linkedFile := filepath.Join(documents, "reports", "x")
	siblingFile := filepath.Join(home, "Other", "x")
	bracketFile := filepath.Join(root, "photos", "Album[1]", "private", "f")
	plainFile := filepath.Join(root, "photos", "Album1", "private", "f")

	for _, file := range []string{linkedFile, siblingFile, bracketFile, plainFile} {
		createItem(t, root, file, kindFile)
	}

	albumLink := filepath.Join(root, "photos", "alias")
	for link, target := range map[string]string{documentsLink: documents, albumLink: filepath.Dir(filepath.Dir(bracketFile))} {
		if err := os.Symlink(target, link); err != nil {
			t.Fatalf("💣 fixture link: %v", err)
		}
	}

	albumPattern := filepath.ToSlash(albumLink) + "/private/**"

	for _, testCase := range []struct {
		pattern     string
		item        string
		isProtected bool
	}{
		{"~/MyDocuments/reports/", linkedFile, true},
		{"~/MyDocuments/", linkedFile, true},
		{"~/MyDocuments/", documentsLink, true},
		{"~/MyDocuments/", documents, true},
		{"~/MyDocuments/", siblingFile, false},
		{"~/MyDocument?/", documentsLink, true},
		{"~/MyDocument?/", linkedFile, false},
		{albumPattern, bracketFile, true},
		{albumPattern, plainFile, false},
	} {
		reportedPattern := matchItem(t, []string{testCase.pattern}, testCase.item, home)
		if testCase.isProtected != (reportedPattern == testCase.pattern) || (!testCase.isProtected && reportedPattern != "") {
			t.Errorf("✗ %q on %q: %q", testCase.pattern, testCase.item, reportedPattern)
		}
	}

	if !t.Failed() {
		t.Log("✓ a linked pattern protects the link, its target, and exactly the named directory")
	}
}

// TestLetterCase verifies invariant #3: Letter case.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// The fixture root first shows whether it treats two names that differ only
// in letter case as one item. Where it does, ".git/" protects a file under
// ".GIT" and "~/Notes/" protects a file under "notes". Where it does not,
// neither pattern protects that file. A pattern spelled as the directory is
// spelled protects the file in both cases. The same holds for a leading
// directory name of a path pattern, "~/Notes/Drafts/" against "notes/drafts".
//
// The directory records holds "Kayıt.TXT", whose dotless ı has no capital of
// its own, beside "Secret.PEM". That name does not change how the directory
// treats letter case: "*.txt" protects "Kayıt.TXT" and "*.pem" is reported for
// the directory exactly where the directory ignores letter case.
//
// The fixture root also shows whether it treats the capital ẞ and the small ß
// as one name. Exactly where it does, a name pattern and a path pattern
// written as either spelling protect a file named with the other one, and a
// path pattern protects a file in a directory named with the other one. A
// pattern always protects an item of its own spelling.
//
// The set "[0-z]" covers the underscore in "item_" on every volume, because
// its range is compared as written or in small letters and never in capitals.
func TestLetterCase(t *testing.T) {
	root, home := fixtureRoot(t)

	for _, name := range []string{"probe", "PROBE"} {
		createItem(t, root, filepath.Join(root, "spellings", name), kindFile)
	}

	spellings, err := os.ReadDir(filepath.Join(root, "spellings"))
	if err != nil {
		t.Fatalf("💣 fixture spellings: %v", err)
	}

	ignoresCase := len(spellings) == 1

	repositoryFile := filepath.Join(home, "proj", ".GIT", "config")
	notesFile := filepath.Join(home, "notes", "a.txt")
	draftsFile := filepath.Join(home, "notes", "drafts", "b.txt")
	recordsDirectory := filepath.Join(home, "records")
	dotlessFile := filepath.Join(recordsDirectory, "Kayıt.TXT")

	for _, file := range []string{repositoryFile, notesFile, draftsFile, dotlessFile, filepath.Join(recordsDirectory, "Secret.PEM")} {
		createItem(t, root, file, kindFile)
	}

	for _, testCase := range []struct {
		pattern     string
		item        string
		isProtected bool
	}{
		{".git/", repositoryFile, ignoresCase},
		{"~/Notes/", notesFile, ignoresCase},
		{".GIT/", repositoryFile, true},
		{"~/notes/", notesFile, true},
		{"~/Notes/Drafts/", draftsFile, ignoresCase},
		{"~/notes/drafts/", draftsFile, true},
		{"*.txt", dotlessFile, ignoresCase},
		{"*.TXT", dotlessFile, true},
	} {
		reportedPattern := matchItem(t, []string{testCase.pattern}, testCase.item, home)
		if testCase.isProtected != (reportedPattern == testCase.pattern) || (!testCase.isProtected && reportedPattern != "") {
			t.Errorf("✗ %q on %q with ignoresCase=%v: %q", testCase.pattern, testCase.item, ignoresCase, reportedPattern)
		}
	}

	sharpSpellings := []string{"ẞ", "ß"}
	for _, spelling := range sharpSpellings {
		createItem(t, root, filepath.Join(home, "sharp", "both", spelling), kindFile)
	}

	bothSpellings, err := os.ReadDir(filepath.Join(home, "sharp", "both"))
	if err != nil {
		t.Fatalf("💣 fixture sharp spellings: %v", err)
	}

	isOneName := len(bothSpellings) == 1

	for spellingNumber, spelling := range sharpSpellings {
		spelledFile := filepath.Join(home, "sharp", strconv.Itoa(spellingNumber), spelling)
		nestedFile := filepath.Join(home, "sharp", strconv.Itoa(spellingNumber), "in", spelling, "f")

		createItem(t, root, spelledFile, kindFile)
		createItem(t, root, nestedFile, kindFile)

		for _, patternSpelling := range sharpSpellings {
			for _, pattern := range []string{patternSpelling, "~/sharp/" + strconv.Itoa(spellingNumber) + "/" + patternSpelling, "~/sharp/*/[" + patternSpelling + "]"} {
				isProtected := matchItem(t, []string{pattern}, spelledFile, home) == pattern
				if isProtected != (patternSpelling == spelling || isOneName) {
					t.Errorf("✗ %q on a file named %q, one name=%v: protected %v", pattern, spelling, isOneName, isProtected)
				}
			}

			directoryPattern := "~/sharp/" + strconv.Itoa(spellingNumber) + "/in/" + patternSpelling + "/f"
			if isProtected := matchItem(t, []string{directoryPattern}, nestedFile, home) == directoryPattern; isProtected != (patternSpelling == spelling || isOneName) {
				t.Errorf("✗ %q on a file in a directory named %q, one name=%v: protected %v", directoryPattern, spelling, isOneName, isProtected)
			}
		}
	}

	rangeFile := filepath.Join(home, "ranges", "item_")
	createItem(t, root, rangeFile, kindFile)

	if reportedPattern := matchItem(t, []string{"item[0-z]"}, rangeFile, home); reportedPattern != "item[0-z]" {
		t.Errorf("✗ the range in \"item[0-z]\" lost the underscore: %q", reportedPattern)
	}

	if reportedPattern, _ := Match([]string{"*.pem"}, recordsDirectory, true, home); (reportedPattern == "*.pem") != ignoresCase {
		t.Errorf("✗ \"*.pem\" on %q with ignoresCase=%v: %q", recordsDirectory, ignoresCase, reportedPattern)
	}

	if !t.Failed() {
		t.Logf("✓ letter case follows the directory (ignores case: %v)", ignoresCase)
	}
}

// TestMalformedPatterns verifies invariant #4: Malformed patterns.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// With "[abc" alone, a file is protected by "[abc". With "*.pem" listed before
// "[abc", a .pem file is protected by "*.pem" and any other file by "[abc".
func TestMalformedPatterns(t *testing.T) {
	root, home := fixtureRoot(t)
	notesFile := filepath.Join(home, "notes.txt")
	keyFile := filepath.Join(home, "a.pem")

	createItem(t, root, notesFile, kindFile)
	createItem(t, root, keyFile, kindFile)

	for _, testCase := range []struct {
		patterns []string
		item     string
		reported string
	}{
		{[]string{"[abc"}, notesFile, "[abc"},
		{[]string{"*.pem", "[abc"}, keyFile, "*.pem"},
		{[]string{"*.pem", "[abc"}, notesFile, "[abc"},
	} {
		if reportedPattern := matchItem(t, testCase.patterns, testCase.item, home); reportedPattern != testCase.reported {
			t.Errorf("✗ %q on %q: %q", testCase.patterns, testCase.item, reportedPattern)
		}
	}

	if !t.Failed() {
		t.Log("✓ a malformed pattern protects every item in list order")
	}
}

// TestMalformedText verifies invariant #5: Malformed text.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// Each listed text is reported as malformed or as well formed. A trailing
// slash does not change the answer, and an empty pattern is well formed.
func TestMalformedText(t *testing.T) {
	for pattern, isMalformed := range map[string]bool{
		"[abc":    true,
		"[abc/":   true,
		"[]":      true,
		"[!]":     true,
		"{a,b":    true,
		"a}":      true,
		"~/x/[a":  true,
		"":        false,
		"/":       false,
		"~":       false,
		"*.pem":   false,
		".git/":   false,
		"[abc]":   false,
		"[!abc]/": false,
		"{a,b}":   false,
		"[{]":     false,
	} {
		if Malformed(pattern) != isMalformed {
			t.Errorf("✗ %q malformed: %v", pattern, !isMalformed)
		}
	}

	if !t.Failed() {
		t.Log("✓ malformed text is told apart from well-formed text")
	}
}

// TestMalformedList verifies invariant #6: Malformed list.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// A list with two malformed patterns returns ErrRuleMalformed, and the
// message quotes the first of them and not the second. A list of well-formed
// and empty patterns, and an empty list, return nil.
func TestMalformedList(t *testing.T) {
	err := Validate([]string{"*.pem", "", "[abc", "{open"})
	if err == nil || !errors.Is(err, errs.ErrRuleMalformed) {
		t.Fatalf("💣 no malformed-rule error to inspect: %v", err)
	}

	if !strings.Contains(err.Error(), strconv.Quote("[abc")) || strings.Contains(err.Error(), "{open") {
		t.Errorf("✗ error does not name the first malformed pattern: %v", err)
	}

	for _, patterns := range [][]string{nil, {""}, {"*.pem", ".git/", "~/vault/**"}} {
		if err := Validate(patterns); err != nil {
			t.Errorf("✗ well-formed list %q reported: %v", patterns, err)
		}
	}

	if !t.Failed() {
		t.Log("✓ a list reports its first malformed pattern")
	}
}

// TestDirectoryContents verifies invariant #7: Directory contents.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// The directory proj holds .git/config and deep/key.pem, and the directory
// linker holds only a link to proj. Each case names the patterns, the source
// directory, and the pattern that must be reported, which is empty when the
// source needs no approval. A path pattern is reported for proj when its fixed
// part is proj, is inside proj, or is above proj, and it matches an item
// inside. The two last cases repeat the first with no patterns and with only
// empty patterns.
func TestDirectoryContents(t *testing.T) {
	root, home := fixtureRoot(t)
	project := filepath.Join(home, "proj")
	linker := filepath.Join(home, "linker")
	createItem(t, root, filepath.Join(project, ".git", "config"), kindFile)
	createItem(t, root, filepath.Join(project, "deep", "key.pem"), kindFile)
	createItem(t, root, linker, kindDirectory)

	if err := os.Symlink(project, filepath.Join(linker, "ref")); err != nil {
		t.Fatalf("💣 fixture link: %v", err)
	}

	for _, testCase := range []struct {
		patterns []string
		source   string
		reported string
	}{
		{[]string{".git/"}, project, ".git/"},
		{[]string{"*.pem", ".git/"}, project, "*.pem"},
		{[]string{".git/", "*.pem"}, project, ".git/"},
		{[]string{"~/Contracts/", "*.pem"}, project, "*.pem"},
		{[]string{"~/proj/deep/"}, project, "~/proj/deep/"},
		{[]string{"~/proj/deep/key.*"}, project, "~/proj/deep/key.*"},
		{[]string{"~/**/key.pem"}, project, "~/**/key.pem"},
		{[]string{"~/*/deep"}, project, "~/*/deep"},
		{[]string{"~/proj/**"}, project, "~/proj/**"},
		{[]string{"~/Contracts/"}, project, ""},
		{[]string{"~/proj/deep/other"}, project, ""},
		{[]string{"*.key"}, project, ""},
		{[]string{".git/"}, linker, ""},
		{[]string{"ref"}, linker, "ref"},
		{nil, project, ""},
		{[]string{"", ""}, project, ""},
	} {
		reportedPattern, unreadableDirectories := Match(testCase.patterns, testCase.source, true, home)
		if reportedPattern != testCase.reported || len(unreadableDirectories) != 0 {
			t.Errorf("✗ %q on %q: %q, %q", testCase.patterns, testCase.source, reportedPattern, unreadableDirectories)
		}
	}

	if !t.Failed() {
		t.Log("✓ a directory reports the first listed pattern that protects its contents")
	}
}

// TestDirectoryBoundary verifies invariant #8: Directory boundary.
// Test class: Expanded.
// Test layer: Coverage.
// Test kind: Permanent.
// What is under test:
// The directory work holds the file f and the directory sub, which holds a
// file of its own. "~/work/*/" protects the file inside sub and not f, and
// "~/work/*" protects both. For the empty directory box, "~/box/*" reports
// nothing and "~/box/**" reports itself.
func TestDirectoryBoundary(t *testing.T) {
	root, home := fixtureRoot(t)
	directFile := filepath.Join(home, "work", "f")
	nestedFile := filepath.Join(home, "work", "sub", "f")
	emptyDirectory := filepath.Join(home, "box")

	createItem(t, root, directFile, kindFile)
	createItem(t, root, nestedFile, kindFile)
	createItem(t, root, emptyDirectory, kindDirectory)

	for _, testCase := range []struct {
		pattern     string
		item        string
		isProtected bool
	}{
		{"~/work/*/", nestedFile, true},
		{"~/work/*/", directFile, false},
		{"~/work/*", nestedFile, true},
		{"~/work/*", directFile, true},
		{"~/box/*", emptyDirectory, false},
		{"~/box/**", emptyDirectory, true},
	} {
		reportedPattern := matchItem(t, []string{testCase.pattern}, testCase.item, home)
		if testCase.isProtected != (reportedPattern == testCase.pattern) || (!testCase.isProtected && reportedPattern != "") {
			t.Errorf("✗ %q on %q: %q", testCase.pattern, testCase.item, reportedPattern)
		}
	}

	if !t.Failed() {
		t.Log("✓ a wildcard for one name stops at the directory before it")
	}
}

// fixtureRoot creates a fixture root that stands in for the filesystem root,
// with the home directory home/me inside it. Both paths have their symbolic
// links resolved.
func fixtureRoot(t testing.TB) (root, home string) {
	t.Helper()

	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("💣 fixture root: %v", err)
	}

	home = filepath.Join(root, "home", "me")
	if err = os.MkdirAll(home, 0o700); err != nil {
		t.Fatalf("💣 fixture home: %v", err)
	}

	return root, home
}

// rootedPattern writes a pattern that is measured from the filesystem root so
// that it is measured from the fixture root instead. A pattern with no leading
// slash stays without one.
func rootedPattern(root, pattern string) string {
	rootText := filepath.ToSlash(root)
	if strings.HasPrefix(pattern, "/") {
		return rootText + pattern
	}

	return strings.TrimPrefix(rootText, "/") + "/" + pattern
}

// createItem creates an item of the kind at the path, with its parent
// directories. A link points to a new item of its own under the fixture root.
func createItem(t testing.TB, root, path, itemKind string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("💣 fixture parent: %v", err)
	}

	var err error

	switch itemKind {
	case kindFile:
		err = os.WriteFile(path, []byte("contents"), 0o600)
	case kindDirectory:
		err = os.Mkdir(path, 0o700)
	case kindDirectoryLink:
		target := filepath.Join(root, "targets", "directory")
		if err = os.MkdirAll(target, 0o700); err == nil {
			err = os.Symlink(target, path)
		}
	case kindFileLink:
		target := filepath.Join(root, "targets", "file")
		createItem(t, root, target, kindFile)
		err = os.Symlink(target, path)
	default:
		t.Fatalf("💣 unknown fixture kind %q", itemKind)
	}

	if err != nil {
		t.Fatalf("💣 fixture %s: %v", itemKind, err)
	}
}

// matchItem returns the pattern that Match reports for the item, telling Match
// whether the item is a directory as the filesystem shows it.
func matchItem(t testing.TB, patterns []string, item, home string) string {
	t.Helper()

	isDirectory := false
	if itemInfo, err := os.Lstat(item); err == nil {
		isDirectory = itemInfo.IsDir()
	}

	pattern, _ := Match(patterns, item, isDirectory, home)

	return pattern
}
