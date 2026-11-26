package review

import (
	"path/filepath"
	"strings"
)

// rule is one review pattern, prepared for matching.
type rule struct {
	// pattern is the pattern as configured, which a pending action records.
	pattern string
	// malformed reports whether the pattern is malformed. A malformed rule
	// matches every item, and its other fields are unset.
	malformed bool
	// directoriesOnly reports whether the pattern ends in a slash.
	directoriesOnly bool
	// nameText is the text of a name pattern, which is compared with an item's
	// own name. It is empty for a path pattern.
	nameText string
	// locations are the places where a path pattern applies. A name pattern has
	// none.
	locations []location
}

// location is one place where a path pattern applies: a directory, and the
// remaining pattern text, which is compared with the path below that
// directory.
type location struct {
	// volume is the Windows drive or share that holds the directory, or empty.
	volume string
	// directory holds the plain names of the directory, from the root of the
	// volume down.
	directory []string
	// remainingPattern is the pattern text for the path below the directory.
	// Empty text matches the directory itself.
	remainingPattern string
	// fixedPart holds the plain names of the path that the pattern cannot match
	// outside of: the directory, followed by the final name when the pattern
	// spells out one item.
	fixedPart []string
}

// patternCharacters are the characters that give a name a meaning as pattern
// text: the wildcards, the start of a set, the start of alternatives, and the
// backslash.
const patternCharacters = `*?[{\`

// parseRules prepares the patterns for matching items on the volume, in list
// order. It leaves out empty patterns.
func parseRules(patterns []string, home, volume string) []rule {
	rules := make([]rule, 0, len(patterns))

	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}

		parsedRule := rule{pattern: pattern, malformed: Malformed(pattern)}
		if !parsedRule.malformed {
			patternText := filepath.ToSlash(pattern)
			if patternText != "/" && strings.HasSuffix(patternText, "/") {
				parsedRule.directoriesOnly = true
				patternText = strings.TrimSuffix(patternText, "/")
			}

			parsedRule.nameText, parsedRule.locations = parsePatternText(patternText, home, volume)
		}

		rules = append(rules, parsedRule)
	}

	return rules
}

// parsePatternText decides the kind of a pattern from its text, which has no
// trailing slash. For a name pattern it returns the text. For a path pattern
// it returns the places where the pattern applies, measured from the home
// directory, from the pattern's own Windows drive, or from the root of the
// volume.
func parsePatternText(patternText, home, volume string) (nameText string, locations []location) {
	drive := filepath.VolumeName(patternText)

	switch {
	case patternText == "~" || strings.HasPrefix(patternText, "~/"):
		return "", locatePattern(home, patternText[1:])
	case drive != "":
		return "", locatePattern(drive+"/", patternText[len(drive):])
	case strings.Contains(patternText, "/"):
		return "", locatePattern(volume+"/", patternText)
	default:
		return patternText, nil
	}
}

// locatePattern returns the places where a path pattern applies, given the
// directory that the pattern is measured from and the pattern text that
// follows that directory.
//
// The leading names are the names before the first name that holds a pattern
// character. They are plain names, and the symbolic links among them are
// resolved. The text from that first name to the end is kept exactly as
// written, so every pattern character reaches the matching library.
//
// A pattern in which no name holds a pattern character spells out one item.
// Its leading names are all the names but the last. When the item is a
// symbolic link, the pattern also applies to the item that the link points to.
func locatePattern(startDirectory, patternText string) []location {
	patternNames := strings.Split(strings.TrimLeft(patternText, "/"), "/")

	leadingCount := 0
	for leadingCount < len(patternNames) && !strings.ContainsAny(patternNames[leadingCount], patternCharacters) {
		leadingCount++
	}

	spellsOutItem := leadingCount == len(patternNames)
	if spellsOutItem {
		leadingCount--
	}

	directory := resolveLinks(filepath.Join(filepath.FromSlash(startDirectory), filepath.Join(patternNames[:leadingCount]...)))
	remainingPattern := strings.Join(patternNames[leadingCount:], "/")
	locations := []location{newLocation(directory, remainingPattern)}

	if !spellsOutItem {
		return locations
	}

	namedItem := filepath.Join(directory, remainingPattern)
	_, locations[0].fixedPart = splitPath(namedItem)

	if linkTarget := resolveLinks(namedItem); linkTarget != namedItem {
		locations = append(locations, newLocation(linkTarget, ""))
	}

	return locations
}

// newLocation returns the location of a directory given as a path.
func newLocation(directory, remainingPattern string) location {
	volume, directoryNames := splitPath(directory)

	return location{volume: volume, directory: directoryNames, remainingPattern: remainingPattern, fixedPart: directoryNames}
}

// resolveLinks returns the path with its symbolic links resolved, or the path
// as given when it cannot be resolved.
func resolveLinks(path string) string {
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path
	}

	return resolvedPath
}

// splitPath splits a path into its Windows drive or share, which is empty on
// other systems, and the names that follow it.
func splitPath(path string) (volume string, pathNames []string) {
	volume = filepath.VolumeName(path)

	namesText := strings.Trim(filepath.ToSlash(path[len(volume):]), "/")
	if namesText == "" {
		return filepath.ToSlash(volume), []string{}
	}

	return filepath.ToSlash(volume), strings.Split(namesText, "/")
}
