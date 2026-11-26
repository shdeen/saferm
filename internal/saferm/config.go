package saferm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/shdeen/saferm/internal/appstrings"
	"github.com/shdeen/saferm/internal/config"
	"github.com/shdeen/saferm/internal/errs"
	"github.com/shdeen/saferm/internal/filesystem"
	"github.com/shdeen/saferm/internal/review"
)

// defaultConfig returns the embedded default configuration, as decoded settings
// and as its TOML document.
func defaultConfig() (Result, error) {
	settings, err := config.Decode(config.DefaultDocument)

	return Result{Config: &settings, Document: config.DefaultDocument}, err
}

// configure carries out a config request. Without Setup, it returns the loaded
// settings, together with ErrRuleMalformed when they hold a malformed review
// pattern. With Setup, it installs the default configuration file and returns
// the settings loaded from it.
//
// When a configuration file already exists, Setup alone keeps the file and
// returns a notice. Setup with Reset returns ErrConfirmation with a
// Confirmation, and replaces the file once the request is confirmed.
func configure(ctx context.Context, request *Request, home string) (Result, error) {
	if !request.Setup {
		settings, err := config.Load(home)
		if err != nil {
			return Result{}, err
		}

		return Result{Config: &settings}, review.Validate(settings.RequireReview)
	}

	configPath := filepath.Join(home, config.Directory, config.Filename)

	exists, err := filesystem.Exists(configPath)
	if err != nil {
		return Result{}, err
	}

	if exists && !request.Reset {
		return Result{Notice: appstrings.Text("notice.configExists")}, nil
	}

	if exists && !request.Confirmed {
		return Result{Confirmation: &Confirmation{Prompt: appstrings.Text("prompt.configReset"), Path: configPath, DefaultYes: true}}, errs.ErrConfirmation
	}

	if err = config.Setup(ctx, home, request.Confirmed && request.Reset); err != nil {
		if errors.Is(err, os.ErrExist) {
			return Result{Notice: appstrings.Text("notice.configExists")}, nil
		}

		return Result{}, err
	}

	settings, err := config.Load(home)
	if err != nil {
		return Result{}, fmt.Errorf("%q: %w: %w", configPath, errs.ErrConfigRead, err)
	}

	return Result{Config: &settings, Notice: appstrings.Text("notice.configSaved")}, nil
}
