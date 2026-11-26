// Command saferm provides recoverable deletion and reversible moves.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"

	"github.com/shdeen/saferm/cmd/saferm/output"
	"github.com/shdeen/saferm/internal/errs"
	"github.com/shdeen/saferm/internal/saferm"
	"github.com/urfave/cli/v3"
)

// main runs the command line, canceling it on an interrupt signal, and exits
// with status 1 on failure.
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	command := newCommand()
	err := execute(ctx, command, os.Args)

	stop()

	if err != nil {
		os.Exit(1)
	}
}

// newCommand builds the saferm root command with its operation and help
// commands. It also replaces cli.ShowCommandHelp with showCommandHelp.
func newCommand() *cli.Command {
	command := &cli.Command{
		Name: "saferm", Action: showHelp, ExitErrHandler: deferCommandExit,
		Commands: []*cli.Command{
			{Name: saferm.Delete, Action: runCommand, Arguments: commandArguments(argumentSource, argumentReason), Flags: metadataFlags()},
			{Name: saferm.Move, Action: runCommand, Arguments: commandArguments(argumentSource, argumentDestination, argumentReason), Flags: metadataFlags()},
			{Name: saferm.List, Action: runCommand, Flags: []cli.Flag{stringFlag(flagType), stringFlag(flagStatus)}},
			{Name: saferm.View, Action: runCommand, Arguments: commandArguments(argumentID)},
			{Name: saferm.Dispose, Action: runCommand, Arguments: commandArguments(argumentID, argumentDecision)},
			{Name: saferm.Restore, Action: runCommand, Arguments: commandArguments(argumentID)},
			{Name: saferm.Purge, Action: runCommand, Arguments: []cli.Argument{&cli.StringArgs{Name: argumentIDs, Max: -1}}},
			{Name: saferm.Cleanup, Action: runCommand, Flags: []cli.Flag{boolFlag(flagOrphans)}},
			{Name: saferm.Config, Action: runCommand, Flags: []cli.Flag{boolFlag(flagDefault), boolFlag(flagSetup), boolFlag(flagReset)}},
			{Name: commandHelp, Action: showHelp, Arguments: []cli.Argument{&cli.StringArgs{Name: argumentTopic, Max: -1}}},
		},
	}
	for _, operation := range command.Commands {
		if operation.Name == saferm.Move || operation.Name == saferm.Restore {
			operation.Flags = append(operation.Flags, boolFlag(flagOverwrite), &cli.BoolFlag{Name: flagForce, Aliases: []string{"f"}, Usage: helpText("flag.force"), Local: true})
		}

		if operation.Name != commandHelp {
			operation.Flags = append(operation.Flags, &cli.BoolFlag{Name: flagJSON, Aliases: []string{"j"}, Usage: helpText("flag.json"), Local: true})
		}
	}

	// urfave/cli exposes this callback for application-specific help routing.
	cli.ShowCommandHelp = showCommandHelp

	configureHelp(command)

	return command
}

// commandArguments declares one positional string argument for each name, in
// the order given.
func commandArguments(names ...string) []cli.Argument {
	declaredArguments := make([]cli.Argument, 0, len(names))
	for _, name := range names {
		declaredArguments = append(declaredArguments, &cli.StringArg{Name: name})
	}

	return declaredArguments
}

// metadataFlags declares the --requester and --description flags that delete
// and move share.
func metadataFlags() []cli.Flag {
	return []cli.Flag{stringFlag(flagRequester), stringFlag(flagDescription)}
}

// stringFlag declares a string flag that applies only to its own command. The
// flag's usage is the help copy for its name.
func stringFlag(name string) *cli.StringFlag {
	return &cli.StringFlag{Name: name, Usage: helpText("flag." + name), Local: true}
}

// boolFlag declares a boolean flag that applies only to its own command. The
// flag's usage is the help copy for its name.
func boolFlag(name string) *cli.BoolFlag {
	return &cli.BoolFlag{Name: name, Usage: helpText("flag." + name), Local: true}
}

// runCommand translates a parsed operation command into a saferm.Request and
// carries it out on the command's input, output, and error streams.
func runCommand(ctx context.Context, command *cli.Command) error {
	request := saferm.Request{
		Command:        command.Name,
		Source:         command.StringArg(argumentSource),
		Destination:    command.StringArg(argumentDestination),
		Reason:         command.StringArg(argumentReason),
		ID:             command.StringArg(argumentID),
		IDs:            command.StringArgs(argumentIDs),
		Decision:       command.StringArg(argumentDecision),
		ExtraArguments: command.Args().Slice(),
		Requester:      command.String(flagRequester),
		Description:    command.String(flagDescription),
		Type:           command.String(flagType),
		Status:         command.String(flagStatus),
		Overwrite:      command.Bool(flagOverwrite),
		Force:          command.Bool(flagForce),
		Orphans:        command.Bool(flagOrphans),
		Default:        command.Bool(flagDefault),
		Setup:          command.Bool(flagSetup),
		Reset:          command.Bool(flagReset),
	}
	terminal := output.Terminal{Input: command.Reader, Output: command.Root().Writer, Errors: command.Root().ErrWriter, JSON: command.Bool(flagJSON)}

	return terminal.Execute(ctx, &request)
}

// execute runs the command line. On failure, it writes the error to the
// command's error stream, as JSON when the operation was given --json, and
// returns the error joined with any failure to write it.
func execute(ctx context.Context, command *cli.Command, commandLineArguments []string) error {
	if err := command.Run(ctx, commandLineArguments); err != nil {
		terminal := output.Terminal{Errors: command.ErrWriter}
		if operation := command.Command(command.Args().First()); operation != nil {
			terminal.JSON = operation.Bool(flagJSON)
		}

		return errors.Join(err, terminal.Failure(err))
	}

	return nil
}

// commandUsageError wraps an error from parsing flags with the command's full
// name and ErrInputInvalid.
func commandUsageError(_ context.Context, command *cli.Command, err error, _ bool) error {
	return fmt.Errorf("%q: %w: %w", command.FullName(), errs.ErrInputInvalid, err)
}

// deferCommandExit handles a command error by doing nothing, which leaves
// reporting the error to execute and exiting to main.
func deferCommandExit(context.Context, *cli.Command, error) {}

// Names of the help command, the flags, and the positional arguments.
const (
	commandHelp         = "help"
	flagJSON            = "json"
	flagRequester       = "requester"
	flagDescription     = "description"
	flagType            = "type"
	flagStatus          = "status"
	flagOverwrite       = "overwrite"
	flagForce           = "force"
	flagOrphans         = "orphans"
	flagDefault         = "default"
	flagSetup           = "setup"
	flagReset           = "reset"
	argumentSource      = "source"
	argumentReason      = "reason"
	argumentDestination = "destination"
	argumentID          = "id"
	argumentIDs         = "ids"
	argumentDecision    = "decision"
	argumentTopic       = "topic"
)
