package review

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"unicode"

	"github.com/bmatcuk/doublestar/v4"
)

// requestMatcher tests the items of one request against the review rules.
type requestMatcher struct {
	// rules are the prepared patterns, in list order.
	rules []rule
	// volume is the Windows drive or share that holds every tested item, or
	// empty.
	volume string
	// ignoresCase records, for each directory checked so far, whether the
	// directory treats two letter cases of one name as the same item.
	ignoresCase map[string]bool
	// source is the path of the request's source.
	source string
	// protectingRule is the index of the first rule found to protect an item of
	// the request, or the number of rules when none has been found.
	protectingRule int
	// unreadableDirectories lists the directories inside the source that could
	// not be read.
	unreadableDirectories []string
}

// visitItem tests one item inside the source directory against the rules that
// could still change the result: the ones listed before the first rule found
// to protect an item. It does not enter a directory that none of those rules
// could apply inside, and it records a directory that cannot be read. The walk
// ends once the first rule of the list protects an item.
func (matcher *requestMatcher) visitItem(itemPath string, directoryEntry fs.DirEntry, err error) error {
	if err != nil {
		matcher.unreadableDirectories = append(matcher.unreadableDirectories, itemPath)

		return nil //nolint:nilerr // An unreadable directory is recorded, and the walk continues with what can be read.
	}

	_, pathNames := splitPath(itemPath)
	if itemPath != matcher.source {
		matcher.protectingRule = matcher.findRule(pathNames, matcher.protectingRule)
	}

	if matcher.protectingRule == 0 {
		return filepath.SkipAll
	}

	if directoryEntry.IsDir() && !matcher.readsDirectory(pathNames) {
		return filepath.SkipDir
	}

	return nil
}

// readsDirectory reports whether a rule listed before the first protecting
// rule could protect an item inside the directory with the names.
func (matcher *requestMatcher) readsDirectory(directoryNames []string) bool {
	for index := range matcher.protectingRule {
		if matcher.appliesInside(&matcher.rules[index], directoryNames) {
			return true
		}
	}

	return false
}

// appliesInside reports whether the rule could protect an item inside the
// directory with the names. A name pattern always could. A path pattern could
// only when its fixed part is that directory, is inside it, or is above it.
func (matcher *requestMatcher) appliesInside(testedRule *rule, directoryNames []string) bool {
	for index := range testedRule.locations {
		ruleLocation := &testedRule.locations[index]
		if !strings.EqualFold(ruleLocation.volume, matcher.volume) {
			continue
		}

		sharesPath := true
		for depth := 0; sharesPath && depth < min(len(directoryNames), len(ruleLocation.fixedPart)); depth++ {
			sharesPath = matcher.isSameName(directoryNames[:depth], directoryNames[depth], ruleLocation.fixedPart[depth])
		}

		if sharesPath {
			return true
		}
	}

	return len(testedRule.locations) == 0
}

// findRule returns the index of the first rule before the limit that matches
// the item with the names, or the limit when none does.
func (matcher *requestMatcher) findRule(pathNames []string, ruleLimit int) int {
	for index := range ruleLimit {
		if matcher.matchesRule(&matcher.rules[index], pathNames) {
			return index
		}
	}

	return ruleLimit
}

// matchesRule reports whether the rule matches the item with the names. A
// malformed rule matches every item. A rule that ends in a slash matches only
// a directory or a symbolic link that points to one. It also matches a link
// whose target cannot be inspected, because that target may be a directory. A
// target that does not exist is no directory.
func (matcher *requestMatcher) matchesRule(testedRule *rule, pathNames []string) bool {
	if testedRule.malformed {
		return true
	}

	isMatch := false

	if len(testedRule.locations) == 0 && len(pathNames) > 0 {
		// The root of a volume has no name, so no name pattern matches it.
		parentNames, itemName := pathNames[:len(pathNames)-1], pathNames[len(pathNames)-1]
		isMatch = matcher.matchText(testedRule.nameText, itemName, parentNames, itemName)
	}

	for index := range testedRule.locations {
		isMatch = isMatch || matcher.matchesLocation(&testedRule.locations[index], pathNames)
	}

	if !isMatch || !testedRule.directoriesOnly {
		return isMatch
	}

	itemInfo, err := os.Stat(matcher.itemPath(pathNames))
	if err != nil {
		return !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, syscall.ENOTDIR)
	}

	return itemInfo.IsDir()
}

// matchesLocation reports whether the item with the names lies at or under the
// location's directory and the path below that directory matches the rest of
// the pattern. The directory's names are compared one at a time. The directory
// itself matches only a remaining pattern that can stand for no name at all,
// as ** can.
func (matcher *requestMatcher) matchesLocation(ruleLocation *location, pathNames []string) bool {
	if !strings.EqualFold(ruleLocation.volume, matcher.volume) || len(pathNames) < len(ruleLocation.directory) {
		return false
	}

	for depth, patternName := range ruleLocation.directory {
		if !matcher.isSameName(pathNames[:depth], pathNames[depth], patternName) {
			return false
		}
	}

	remainingNames := pathNames[len(ruleLocation.directory):]
	if len(remainingNames) == 0 && ruleLocation.remainingPattern != "" {
		// The library matches * against no text, though * stands for one name.
		// With a name before the slash on both sides, it keeps that boundary.
		return matcher.matchText(standInName+"/"+ruleLocation.remainingPattern, standInName, nil, "")
	}

	remainingPath := strings.Join(remainingNames, "/")

	for depth, itemName := range slices.Backward(remainingNames) {
		// The deepest name that has letters decides letter case for the whole path.
		// A letter is a character that has another case, as refoldName finds it.
		if refoldName(itemName) != itemName {
			return matcher.matchText(ruleLocation.remainingPattern, remainingPath, pathNames[:len(ruleLocation.directory)+depth], itemName)
		}
	}

	return matcher.matchText(ruleLocation.remainingPattern, remainingPath, nil, "")
}

// matchText reports whether the pattern text matches the whole of the path
// text. When the directory with the parent names ignores letter case for the
// item name, both texts are compared in small letters. An empty item name
// means that letter case matters.
func (matcher *requestMatcher) matchText(patternText, pathText string, parentNames []string, itemName string) bool {
	if itemName != "" && matcher.ignoresLetterCase(parentNames, itemName) {
		patternText, pathText = strings.ToLower(patternText), strings.ToLower(pathText)
	}

	// For some names, the library rejects a pattern that ValidatePattern
	// accepted, such as a negated set that lists a brace. The pattern cannot be
	// judged for that item, so the request needs approval.
	isMatch, err := doublestar.Match(patternText, pathText)

	return isMatch || err != nil
}

// isSameName reports whether an item's name and a plain name from a pattern
// are the same name in the directory with the parent names.
func (matcher *requestMatcher) isSameName(parentNames []string, itemName, patternName string) bool {
	if itemName == patternName {
		return true
	}

	//lint:ignore SA6005 Names are compared in small letters, as pattern text is; strings.EqualFold folds some characters differently.
	return strings.ToLower(itemName) == strings.ToLower(patternName) && matcher.ignoresLetterCase(parentNames, itemName) //nolint:gocritic,staticcheck // The directive above gives the reason.
}

// ignoresLetterCase reports whether the directory with the parent names treats
// the item name and the same name in another letter case as one item. It asks
// the filesystem about each directory until one name settles the answer.
//
// The spelling from respellName settles it either way. The name in capitals,
// the name in small letters, and the spelling from refoldName can only show
// that the directory ignores letter case, since a letter such as the dotless ı
// has a capital that belongs to another letter. Letter case matters when the
// name has no letters or names no existing item.
func (matcher *requestMatcher) ignoresLetterCase(parentNames []string, itemName string) bool {
	directory := matcher.itemPath(parentNames)
	if ignoresCase, isKnown := matcher.ignoresCase[directory]; isKnown {
		return ignoresCase
	}

	itemInfo, err := os.Lstat(filepath.Join(directory, itemName))
	if err != nil {
		return false
	}

	settlingName := respellName(itemName)
	for _, respelledName := range []string{settlingName, strings.ToUpper(itemName), strings.ToLower(itemName), refoldName(itemName)} {
		if respelledName == itemName {
			continue
		}

		if respelledInfo, err := os.Lstat(filepath.Join(directory, respelledName)); err == nil && os.SameFile(itemInfo, respelledInfo) {
			matcher.ignoresCase[directory] = true

			return true
		}
	}

	if settlingName != itemName {
		matcher.ignoresCase[directory] = false
	}

	return false
}

// respellName returns the name with each letter in its other letter case,
// when changing the case back gives the same letter. Every other character
// stays as it is. That includes a letter such as the dotless ı, whose capital
// is the capital of another letter: a directory that ignores letter case can
// still tell those two letters apart.
func respellName(itemName string) string {
	letters := []rune(itemName)
	for index, letter := range letters {
		capitalLetter, smallLetter := unicode.ToUpper(letter), unicode.ToLower(letter)

		switch {
		case capitalLetter != letter && unicode.ToLower(capitalLetter) == letter:
			letters[index] = capitalLetter
		case smallLetter != letter && unicode.ToUpper(smallLetter) == letter:
			letters[index] = smallLetter
		}
	}

	return string(letters)
}

// refoldName returns the name with each character replaced by the next
// character that Unicode counts as the same letter in another case, such as
// the capital ẞ for the small ß. A character with no such partner stays as it
// is.
func refoldName(itemName string) string {
	letters := []rune(itemName)
	for index, letter := range letters {
		letters[index] = unicode.SimpleFold(letter)
	}

	return string(letters)
}

// standInName is a plain name that stands in for a directory when the library
// is asked whether a pattern matches the directory itself.
const standInName = "directory"

// itemPath returns the path of the item with the names on the matcher's
// volume.
func (matcher *requestMatcher) itemPath(pathNames []string) string {
	return filepath.FromSlash(matcher.volume + "/" + strings.Join(pathNames, "/"))
}
