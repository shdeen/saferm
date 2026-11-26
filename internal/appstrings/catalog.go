// Package appstrings owns the shared, embedded product copy.
package appstrings

import (
	_ "embed"
	"strings"

	"github.com/BurntSushi/toml"
)

// document is the embedded copy catalog, in TOML.
//
//go:embed app-strings.toml
var document string

// catalog holds the copy of each catalog section, keyed by section name and
// then by the copy's name within the section.
var catalog = readCatalog() //nolint:gochecknoglobals // Parsed once from embedded copy and read without mutation.

// readCatalog decodes the embedded catalog. It panics when the catalog is
// malformed.
func readCatalog() map[string]map[string]string {
	var messages map[string]map[string]string
	if _, err := toml.Decode(document, &messages); err != nil {
		panic(err)
	}

	return messages
}

// Text returns the copy for a key of the form "section.name". It panics when
// the catalog has no such key.
func Text(key string) string {
	section, name, _ := strings.Cut(key, ".")

	message, ok := catalog[section][name]
	if !ok {
		panic(key)
	}

	return message
}
