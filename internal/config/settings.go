// Package config loads, decodes, installs, and updates the SafeRM configuration.
package config

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/shdeen/saferm/internal/errs"
	"github.com/shdeen/saferm/internal/filesystem"
)

// DefaultDocument is the embedded default configuration, in TOML.
//
//go:embed default.toml
var DefaultDocument string

// Names of the per-user SafeRM files.
//   - Directory: the directory under the home directory that holds all SafeRM data
//   - Filename: the configuration file inside Directory
//   - Database: the database of action history inside Directory
const (
	Directory = ".saferm"
	Filename  = "config.toml"
	Database  = "saferm.sqlite"
)

// Settings holds a decoded configuration. A nil TrashPath, RetainPeriod, or
// RequireReview means that the configuration omits that key. Settings returned
// by Load always have a TrashPath and a RetainPeriod.
//
// Note: the fields are listed by their Go names. The generated TypeScript
// bindings spell the same fields in a different letter case.
//   - TrashPath: the directory that receives deleted items
//   - RetainPeriod: the number of days after which cleanup may remove a resolved action
//   - RequireReview: the review patterns; a delete or move that affects an item they protect needs approval
//   - Path: the location of the configuration file, whether or not the file exists
//   - External: whether the settings came from that file instead of DefaultDocument
//   - InvalidTrashPath: the file's trash-path value, as written, when it can't be applied and TrashPath holds the value of DefaultDocument instead; empty otherwise
//   - InvalidRetainPeriod: the file's retain-period value, as written, when it can't be applied and RetainPeriod holds the value of DefaultDocument instead; empty otherwise
type Settings struct {
	TrashPath           *string  `json:"trashPath"                     toml:"trash-path"`
	RetainPeriod        *int     `json:"retainPeriod"                  toml:"retain-period"`
	RequireReview       []string `json:"requireReview"                 toml:"require-review"`
	Path                string   `json:"path"                          toml:"-"`
	External            bool     `json:"external"                      toml:"-"`
	InvalidTrashPath    string   `json:"invalidTrashPath,omitempty"    toml:"-"`
	InvalidRetainPeriod string   `json:"invalidRetainPeriod,omitempty" toml:"-"`
}

// fileSettings holds the keys of a configuration file before Load checks that
// their values can be applied. A nil TrashPath or RetainPeriod means that the
// file omits that key; any other value has the type that the file gave it.
type fileSettings struct {
	TrashPath     any      `toml:"trash-path"`
	RetainPeriod  any      `toml:"retain-period"`
	RequireReview []string `toml:"require-review"`
}

// Home returns the current user's home directory, or ErrHomeRead when it cannot
// be determined.
func Home() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("%w: %w", errs.ErrHomeRead, err)
	}

	return home, nil
}

// Load returns the settings decoded from the configuration file under the home
// directory, or from DefaultDocument when no such file exists. A dangling
// symbolic link in the file's place is a read error, not an absent file.
//
// The value of DefaultDocument takes the place of a trash-path that the file
// omits, sets blank, or sets to a value that can't be applied, and of a
// retain-period that the file omits or sets to a value that can't be applied.
// A trash-path can't be applied when it is not a string, when it does not
// resolve, or when it names something other than a directory; a directory that
// does not exist yet is created on the first delete. A retain-period can't be
// applied when it is not a whole number of 0 or more. Load reports each value
// that can't be applied, as written, in InvalidTrashPath or InvalidRetainPeriod.
func Load(home string) (Settings, error) {
	configPath := filepath.Join(home, Directory, Filename)
	document, err := os.ReadFile(configPath)
	external := true

	if errors.Is(err, os.ErrNotExist) {
		if _, linkErr := os.Lstat(configPath); !errors.Is(linkErr, os.ErrNotExist) {
			return Settings{}, fmt.Errorf("%q: %w: %w", configPath, errs.ErrConfigRead, err)
		}

		document, external, err = []byte(DefaultDocument), false, nil
	}

	if err != nil {
		return Settings{}, fmt.Errorf("%q: %w: %w", configPath, errs.ErrConfigRead, err)
	}

	settings, err := Decode(DefaultDocument)
	if err != nil {
		return Settings{}, err
	}

	settings.Path, settings.External = configPath, external
	if !external {
		return settings, nil
	}

	var file fileSettings
	if err := decodeDocument(string(document), &file); err != nil {
		return Settings{}, err
	}

	settings.RequireReview = file.RequireReview
	settings.TrashPath, settings.InvalidTrashPath = applicableTrashPath(file.TrashPath, home, settings.TrashPath)
	settings.RetainPeriod, settings.InvalidRetainPeriod = applicableRetainPeriod(file.RetainPeriod, settings.RetainPeriod)

	return settings, nil
}

// Decode parses a TOML configuration document. It returns ErrConfigDecode when
// the document is malformed.
func Decode(document string) (Settings, error) {
	var settings Settings
	if err := decodeDocument(document, &settings); err != nil {
		return Settings{}, err
	}

	return settings, nil
}

// decodeDocument parses a TOML document into the target. It returns
// ErrConfigDecode when the document is malformed or a value does not fit its
// field.
func decodeDocument(document string, target any) error {
	if _, err := toml.Decode(document, target); err != nil {
		return fmt.Errorf("%w: %w", errs.ErrConfigDecode, err)
	}

	return nil
}

// applicableTrashPath returns the trash path that applies for a file's
// trash-path value, together with that value as written when it can't be
// applied. The embedded trash path applies when the value is absent, blank, or
// can't be applied.
func applicableTrashPath(fileValue any, home string, embeddedPath *string) (applicablePath *string, invalidValue string) {
	if fileValue == nil {
		return embeddedPath, ""
	}

	trashPath, isString := fileValue.(string)
	if !isString {
		return embeddedPath, fmt.Sprint(fileValue)
	}

	if strings.TrimSpace(trashPath) == "" {
		return embeddedPath, ""
	}

	if !trashUsable(trashPath, home) {
		return embeddedPath, trashPath
	}

	return &trashPath, ""
}

// trashUsable reports whether the path can serve as the trash directory: it
// resolves, and it names an existing directory or a directory that does not
// exist yet.
func trashUsable(trashPath, home string) bool {
	resolved, err := filesystem.Resolve(trashPath, home)
	if err != nil {
		return false
	}

	fileInfo, err := os.Stat(resolved)
	if err == nil {
		return fileInfo.IsDir()
	}

	return errors.Is(err, os.ErrNotExist)
}

// applicableRetainPeriod returns the retention period that applies for a
// file's retain-period value, together with that value as written when it
// can't be applied. The embedded retention period applies when the value is
// absent or can't be applied.
func applicableRetainPeriod(fileValue any, embeddedDays *int) (applicableDays *int, invalidValue string) {
	if fileValue == nil {
		return embeddedDays, ""
	}

	fileDays, isInteger := fileValue.(int64)
	if !isInteger || fileDays < 0 || fileDays > math.MaxInt {
		return embeddedDays, fmt.Sprint(fileValue)
	}

	retentionDays := int(fileDays)

	return &retentionDays, ""
}

// Trash returns the absolute path of the trash directory, expanding a leading ~
// to the home directory. It returns ErrConfigMissing when trash-path is omitted
// and ErrConfigInvalid when it is blank.
func (settings Settings) Trash(home string) (string, error) {
	if settings.TrashPath == nil {
		return "", fmt.Errorf("%q: %w", "trash-path", errs.ErrConfigMissing)
	}

	if strings.TrimSpace(*settings.TrashPath) == "" {
		return "", fmt.Errorf("%q: %w", "trash-path", errs.ErrConfigInvalid)
	}

	trashPath := filesystem.ExpandHome(*settings.TrashPath, home)

	resolved, err := filepath.Abs(trashPath)
	if err != nil {
		return "", fmt.Errorf("%q: %w: %w", trashPath, errs.ErrPathResolve, err)
	}

	return resolved, nil
}

// Retention returns the retention period in days. It returns ErrConfigMissing
// when retain-period is omitted and ErrConfigInvalid when it is negative.
func (settings Settings) Retention() (int, error) {
	if settings.RetainPeriod == nil {
		return 0, fmt.Errorf("%q: %w", "retain-period", errs.ErrConfigMissing)
	}

	if *settings.RetainPeriod < 0 {
		return 0, fmt.Errorf("%q: %w", "retain-period", errs.ErrConfigInvalid)
	}

	return *settings.RetainPeriod, nil
}

// Setup writes DefaultDocument to the configuration file under the home
// directory, creating the SafeRM directory when it is absent. It replaces an
// existing file only when the caller has confirmed the replacement; otherwise
// it fails and leaves that file unchanged.
func Setup(ctx context.Context, home string, confirmed bool) error {
	configPath := filepath.Join(home, Directory, Filename)

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%w: %w", errs.ErrOperationCanceled, err)
	}

	if err := os.MkdirAll(filepath.Dir(configPath), filesystem.PrivateDirectoryMode); err != nil {
		return fmt.Errorf("%q: %w: %w", configPath, errs.ErrConfigWrite, err)
	}

	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if confirmed {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}

	file, err := os.OpenFile(configPath, flags, filesystem.PrivateFileMode)
	if err != nil {
		return fmt.Errorf("%q: %w: %w", configPath, errs.ErrConfigWrite, err)
	}

	_, writeErr := file.WriteString(DefaultDocument)

	err = errors.Join(writeErr, file.Sync(), file.Close())
	if err != nil {
		return fmt.Errorf("%q: %w: %w", configPath, errs.ErrConfigWrite, err)
	}

	return nil
}
