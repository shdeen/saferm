// Package config loads, decodes, installs, and updates the SafeRM configuration.
package config

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
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
// RequireReview means that the configuration omits that key.
//
// Note: the fields are listed by their Go names. The generated TypeScript
// bindings spell the same fields in a different letter case.
//   - TrashPath: the directory that receives deleted items
//   - RetainPeriod: the number of days after which cleanup may remove a resolved action
//   - RequireReview: the review patterns; a delete or move that affects an item they protect needs approval
//   - Path: the location of the configuration file, whether or not the file exists
//   - External: whether the settings came from that file instead of DefaultDocument
type Settings struct {
	TrashPath     *string  `json:"trashPath"     toml:"trash-path"`
	RetainPeriod  *int     `json:"retainPeriod"  toml:"retain-period"`
	RequireReview []string `json:"requireReview" toml:"require-review"`
	Path          string   `json:"path"          toml:"-"`
	External      bool     `json:"external"      toml:"-"`
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

	settings, err := Decode(string(document))
	settings.Path, settings.External = configPath, external

	return settings, err
}

// Decode parses a TOML configuration document. It returns ErrConfigDecode when
// the document is malformed.
func Decode(document string) (Settings, error) {
	var settings Settings
	if _, err := toml.Decode(document, &settings); err != nil {
		return Settings{}, fmt.Errorf("%w: %w", errs.ErrConfigDecode, err)
	}

	return settings, nil
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
