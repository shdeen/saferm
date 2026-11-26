# Command-line reference

Run `saferm help` for a list of commands. Run `saferm help <command>` or `saferm <command> -h` for a command's arguments and options.

## Syntax

```text
saferm <command> [<arguments>] [<options>]
```

These options work with every command, except as noted:

| Option         | Meaning                                                                                                             |
| -------------- | ------------------------------------------------------------------------------------------------------------------- |
| `-j`, `--json` | Output results as JSON. Every command except `help` accepts it. See [CLI output and JSON reference](cli-output.md). |
| `-h`, `--help` | Show the command's help.                                                                                            |

## Commands

| Command               | Purpose                                    |
| --------------------- | ------------------------------------------ |
| [`delete`](#delete)   | Move an item to trash.                     |
| [`move`](#move)       | Move an item and record its original path. |
| [`list`](#list)       | List actions.                              |
| [`view`](#view)       | Show one action's details.                 |
| [`dispose`](#dispose) | Approve or deny a pending action.          |
| [`restore`](#restore) | Return an item to its original path.       |
| [`purge`](#purge)     | Permanently erase trashed items.           |
| [`cleanup`](#cleanup) | Remove eligible history records.           |
| [`config`](#config)   | Show, create, or reset the configuration.  |
| [`help`](#help)       | Show help.                                 |

## delete

```text
saferm delete <path> <reason> [--requester <name>] [--description <text>]
```

`delete` moves an item to trash. If a [review rule](review.md#which-requests-need-approval) requires approval, `delete` records a pending request and leaves the item in place until you approve or deny it.

If SafeRM can't read a folder inside the item, and a review rule could protect something in that folder, the request is pending. The command prints a warning that names each folder that SafeRM couldn't read. See [Folders that SafeRM can't read](review.md#folders-that-saferm-cant-read).

| Option                 | Meaning                                         |
| ---------------------- | ----------------------------------------------- |
| `--requester <name>`   | Record the person or agent making the request. |
| `--description <text>` | Record more context for the request.            |

## move

```text
saferm move <source> <destination> <reason> [--overwrite [--force]] [--requester <name>] [--description <text>]
```

`move` relocates an item from `<source>` to `<destination>` and records its original path. The destination's parent folder must exist. Review rules apply as they do for `delete`.

| Option | Meaning |
| --- | --- |
| `--overwrite` | Replace an existing item at the destination after confirmation. The existing item is deleted to trash first. |
| `-f`, `--force` | With `--overwrite`, skip the confirmation. Review rules still apply. |
| `--requester <name>`, `--description <text>` | As for `delete`. |

A replacement returns two actions: the move, then the delete of the existing item. The move names that delete as its blocker. See [Moves onto an existing item](file-operations.md#moves-onto-an-existing-item).

## list

```text
saferm list [--type <type>] [--status <status>]
```

`list` shows the action history, oldest first.

| Option | Meaning |
| --- | --- |
| `--type <type>` | Show only `delete` or only `move` actions. |
| `--status <status>` | Show only actions with this status, given by name or number: `completed` (1), `pending` (2), `denied` (3), `restored` (4), `purged` (5), or `failed` (6). |

## view

```text
saferm view <id>
```

`view` shows one action's paths, status, reason, requester, description, dates, review rule, blocker, decision, and recorded errors.

## dispose

```text
saferm dispose <id> (approve|deny)
```

`dispose` approves or denies a pending action. Approving carries out the request without checking the review rules again. For a move whose blocker is still pending, `dispose` asks whether to decide both. See [Replacements](review.md#replacements).

## restore

```text
saferm restore <id> [--overwrite [--force]]
```

`restore` returns the item of a completed or failed action to its original path. If another item now occupies that path, `--overwrite` can replace it only when the action is completed.

| Option          | Meaning                                                                                                          |
| --------------- | ---------------------------------------------------------------------------------------------------------------- |
| `--overwrite`   | Replace an existing item at the original path after confirmation. The existing item is deleted to trash first. |
| `-f`, `--force` | With `--overwrite`, skip the confirmation. Review rules still apply.                                             |

The delete of the existing item is checked against the review rules like any other delete. If it is pending, approve it, then run `restore` again. See [Restore an item](recovery.md#restore-an-item).

## purge

```text
saferm purge <id>...
```

`purge` permanently erases the trashed items of completed or failed deletes. It doesn't ask for confirmation, and it keeps the history records. Each ID is handled separately, so one failure doesn't stop the others.

## cleanup

```text
saferm cleanup [--orphans]
```

`cleanup` removes history records once their retention period has passed. [Cleanup eligibility](recovery.md#cleanup-eligibility) lists the records that it keeps longer or removes sooner. It never removes files.

| Option | Meaning |
| --- | --- |
| `--orphans` | Also remove eligible records whose items are missing. |

## config

```text
saferm config [--default | --setup [--reset]]
```

`config` shows the settings in effect and the source of each one.

| Option | Meaning |
| --- | --- |
| `--default` | Show the built-in values without reading the configuration file. |
| `--setup` | Write the built-in values to `~/.saferm/config.toml` if the file doesn't exist. |
| `--reset` | With `--setup`, replace an existing file after confirmation. |

`--reset` without `--setup` is invalid, and so is `--default` with another option. When the settings hold a malformed review rule, `config` shows them, reports the rule as an error, and exits with status 1. See [Configuration](configuration.md) and [Write review rules](review.md#write-review-rules).

## help

```text
saferm help [<command>]
```

`help` lists the commands, or shows the syntax and options of `<command>`. Running `saferm` with no command also lists the commands. `help` reads neither the configuration nor the history, so it works when either one is unavailable.

## Confirmations

The CLI asks for confirmation in these cases:

- Before it replaces an item with `--overwrite`, unless `--force` is given. The default answer is no.
- Before it decides a move together with its pending blocker. The default answer is no.
- Before `config --setup --reset` replaces the configuration file. The default answer is yes.

The question goes to standard error. The answer is one line read from standard input.

| Answer | Effect |
| --- | --- |
| `y` or `yes`, in any letter case | Confirm. |
| Empty line | Use the default, which the prompt shows in capitals: `(y/N)` or `(Y/n)`. |
| Anything else | Decline. |
| End of input before a full line | Decline, even when the default is yes. |
| Ctrl-C | Cancel with an error. |

Declining changes nothing, prints `Canceled.`, and exits with status 0. With `--json`, the question is a JSON object and the result is `{"notice": "Canceled."}`, but the answer is still a plain line.

## Exit status

| Status | Meaning |
| --- | --- |
| `0` | Success. This includes a declined confirmation and a recorded pending request. |
| `1` | Failure, including a partial failure and Ctrl-C. A command that records a pending request also exits with status 1 when the review rules hold a malformed rule. |

Results go to standard output. Errors and questions go to standard error. A failed command can still print actions that it completed, especially a replacement or a purge of several items. A notification that couldn't be delivered doesn't make a command fail.

In scripts, use `--json` and check each action's `status` as well as the exit status. A command that records a pending request changes no files, but it exits with status 0. See [Troubleshooting](troubleshooting.md) for failures.

Revised 2026-11-26
