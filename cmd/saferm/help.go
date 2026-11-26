package main

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"strconv"
	"strings"
	"text/template"

	"github.com/shdeen/saferm/internal/appstrings"
	"github.com/shdeen/saferm/internal/errs"
	"github.com/shdeen/saferm/internal/store"
	"github.com/urfave/cli/v3"
)

// helpTemplates contains the layout of the help page and the help copy.
//
//go:embed templates/*.tmpl
var helpTemplates embed.FS

// helpTemplate holds the help copy as named templates: the usage and arguments
// of each command, and the usage of each flag. A flag that takes a value names
// that value between backquotes in its usage, which is where urfave/cli reads
// the name that it shows beside the flag.
//
//nolint:gochecknoglobals // Parsed once from embedded help and executed without mutation.
var helpTemplate = template.Must(template.New("appstrings").Funcs(template.FuncMap{"statuses": helpStatuses, "join": strings.Join}).ParseFS(helpTemplates, "templates/appstrings.tmpl"))

// showCommandHelp writes the help page for the named subcommand of the command.
// A command that has no subcommands is itself looked up by name in its parent.
func showCommandHelp(ctx context.Context, command *cli.Command, topic string) error {
	parents := command.Lineage()
	if len(command.Commands) == 0 && len(parents) > 1 {
		topic, command = command.Name, parents[1]
	}

	if err := cli.DefaultShowCommandHelp(ctx, command, topic); err != nil {
		return fmt.Errorf("%q: %w", topic, err)
	}

	return nil
}

// helpText returns the help copy of the named template. It panics when the
// template cannot be executed.
func helpText(name string) string {
	var buffer bytes.Buffer
	if err := helpTemplate.ExecuteTemplate(&buffer, name, nil); err != nil {
		panic(err)
	}

	return buffer.String()
}

// configureHelp sets the layout of the help page, the handler for usage errors,
// the help copy, and a --help flag on the command and on every command beneath
// it. It panics when the layout cannot be read.
func configureHelp(command *cli.Command) {
	page, err := helpTemplates.ReadFile("templates/page.tmpl")
	if err != nil {
		panic(err)
	}

	command.CustomHelpTemplate = string(page)
	command.CustomRootCommandHelpTemplate = string(page)
	command.HideHelpCommand = true
	command.OnUsageError = commandUsageError
	command.Usage = helpText(command.Name + ".usage")
	command.ArgsUsage = helpText(command.Name + ".arguments")

	command.Flags = append(command.Flags, &cli.BoolFlag{Name: commandHelp, Aliases: []string{"h"}, Usage: helpText("flag.help"), Local: true})
	for _, child := range command.Commands {
		configureHelp(child)
	}
}

// showHelp writes the help page for the command path that the arguments name,
// or the root help page when there are no arguments. It returns ErrInputInvalid
// for an unknown command name.
func showHelp(ctx context.Context, command *cli.Command) error {
	topics := command.Args().Slice()
	if command.Name == commandHelp {
		topics = command.StringArgs(argumentTopic)
	}

	selected := command.Root()

	parent := selected
	for _, topic := range topics {
		parent = selected

		selected = selected.Command(topic)
		if selected == nil {
			return fmt.Errorf("%q: %w", topic, errs.ErrInputInvalid)
		}
	}

	var err error
	if selected == command.Root() {
		err = cli.ShowRootCommandHelp(selected)
	} else {
		err = cli.ShowCommandHelp(ctx, parent, selected.Name)
	}

	if err != nil {
		return fmt.Errorf("%q: %w", selected.FullName(), err)
	}

	return nil
}

// helpStatuses returns the status names in the order of their status codes.
func helpStatuses() []string {
	names := make([]string, 0, store.Failed)
	for status := store.Completed; status <= store.Failed; status++ {
		names = append(names, appstrings.Text("status."+strconv.Itoa(status)))
	}

	return names
}
