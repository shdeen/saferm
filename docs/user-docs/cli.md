# Command-line reference

`saferm help` lists the commands. `saferm help <command>`, or `saferm <command> -h`, shows a command's arguments and options.

## Syntax

```text
saferm <command> [<arguments>] [<options>]
```

Options may appear before, between, or after the arguments. Put `--` before an argument that starts with a dash, as in `saferm delete -- -draft.txt "Remove the draft"`. IDs are positive whole numbers. An unknown command or option, an extra argument, or an invalid combination of options fails with `input is invalid`.

| Option | Meaning |
| --- | --- |
| `-j`, `--json` | Write results and errors as JSON. Every command except `help` accepts it. See [CLI output and JSON reference](cli-output.md). |
| `-h`, `--help` | Show the command's help. |

## Commands

| Command | Purpose |
| --- | --- |
| [`delete`](#delete) | Move an item to trash. |
| [`move`](#move) | Move an item and record its original path. |
| [`list`](#list) | List actions. |
| [`view`](#view) | Show one action's details. |
| [`dispose`](#dispose) | Approve or deny a pending action. |
| [`restore`](#restore) | Return an item to its original path. |
| [`purge`](#purge) | Permanently erase trashed items. |
| [`cleanup`](#cleanup) | Remove eligible history records. |
| [`config`](#config) | Show, create, or reset the configuration. |
| [`help`](#help) | Show help. |

## delete

```text
saferm delete <path> <reason> [--requester <name>] [--description <text>]
```

Moves the item to trash. If the request needs approval under a [review rule](review.md#which-requests-need-approval), records a pending request instead and leaves the item in place. A pending result counts as success and exits with status 0, except while the review rules hold a malformed rule, when it exits with status 1.

If SafeRM couldn't read a folder inside the item and a rule could apply in that folder, the request is pending and the command prints a warning that names the folder. See [Folders that SafeRM can't read](review.md#folders-that-saferm-cant-read).

`--requester <name>` names the person or agent making the request. `--description <text>` adds context. Both are optional and stored with the action.

## move

```text
saferm move <source> <destination> <reason> [--overwrite [--force]] [--requester <name>] [--description <text>]
```

Moves the item to the destination, which is its full new path. The destination's parent folder must exist. If the request needs approval under a review rule, records a pending request instead, as `delete` does.

| Option | Meaning |
| --- | --- |
| `--overwrite` | Allow replacing an item at the destination. The occupant is deleted to trash first. Asks for confirmation, with no as the default. |
| `-f`, `--force` | Skip the confirmation. Has no effect without `--overwrite`, and review rules still apply. |
| `--requester <name>`, `--description <text>` | As for `delete`. |

A replacement returns two actions: the move, then the occupant's delete, which the move names as its blocker. See [Moves onto an existing item](file-operations.md#moves-onto-an-existing-item).

## list

```text
saferm list [--type <type>] [--status <status>]
```

Lists actions, oldest first. `--type` and `--status` filter the list and can be combined. `<type>` is `delete` or `move`. `<status>` is a name or its number: `completed` (1), `pending` (2), `denied` (3), `restored` (4), `purged` (5), and `failed` (6).

## view

```text
saferm view <id>
```

Shows an action's paths, status, reason, requester, description, dates, review rule, blocker, decision, and recorded errors.

## dispose

```text
saferm dispose <id> (approve|deny)
```

Approves or denies a pending action. Approving carries out the request without checking the review rules again. For a move whose blocker is still pending, asks whether to decide both, with no as the default. See [Replacements](review.md#replacements).

## restore

```text
saferm restore <id> [--overwrite [--force]]
```

Returns the item of a completed or failed action to its original path. The review rules aren't checked for the restored item.

| Option | Meaning |
| --- | --- |
| `--overwrite` | Allow replacing an item at the original path. The occupant is deleted to trash first. Completed actions only. Asks for confirmation, with no as the default. |
| `-f`, `--force` | Skip the confirmation. Has no effect without `--overwrite`, and review rules still apply. |

The occupant's delete is checked against the review rules like any other delete. If it is pending, approve it and run the restore again. See [Restore an item](recovery.md#restore-an-item).

## purge

```text
saferm purge <id>...
```

Permanently erases the trashed items of completed or failed deletes, without asking. The history records stay. Each ID is handled separately, and one failure doesn't stop the others.

## cleanup

```text
saferm cleanup [--orphans]
```

Removes history records whose retention period has passed, subject to [Cleanup eligibility](recovery.md#cleanup-eligibility). Never removes files. `--orphans` also removes eligible records whose items are missing.

## config

```text
saferm config [--default | --setup [--reset]]
```

| Option | Meaning |
| --- | --- |
| None | Show the settings in effect and their source. |
| `--default` | Show the built-in values without reading the configuration file. |
| `--setup` | Write the built-in values to `~/.saferm/config.toml`. Leaves an existing file unchanged. |
| `--reset` | With `--setup`, replace an existing file after confirmation, with yes as the default. |

`--reset` without `--setup` is invalid, and so is `--default` with another option. When the settings hold a malformed review rule, `saferm config` shows them, reports the rule as an error, and exits with status 1. See [Configuration](configuration.md) and [Write review rules](review.md#write-review-rules).

## help

```text
saferm help [<command>]
```

`saferm` with no command also prints the general help. Help reads neither the configuration nor the history, so it works when either is unavailable.

## Confirmations

The CLI asks before a replacement unless `--force` is given, before deciding a move together with its pending blocker, and before resetting the configuration file. The question goes to standard error, and the answer is one line read from standard input.

| Answer | Effect |
| --- | --- |
| `y` or `yes`, in any letter case | Confirm. |
| Empty line | The default shown: `(y/N)` means no, `(Y/n)` means yes. |
| Anything else | Decline. |
| End of input before a full line | Decline, even when the default is yes. |
| Ctrl-C | Cancel with an error. |

Declining changes nothing, prints `Canceled.`, and exits with status 0. With `--json`, the question is a JSON object and the result is `{"notice": "Canceled."}`, but the answer is still a plain line.

## Exit status

| Status | Meaning |
| --- | --- |
| `0` | Success, including a declined confirmation and a recorded pending request, except a pending request recorded while the review rules hold a malformed rule. |
| `1` | Failure, including a partial failure and Ctrl-C. Also a delete or move that was recorded as pending while the review rules hold a malformed rule. |

Results go to standard output. Errors and questions go to standard error. A failed command can still print actions it completed, especially a replacement or a batch purge. A notification that couldn't be delivered doesn't make a command fail.

In scripts, use `--json` and check each action's `status`, not just the exit status: a pending request changes no file, and it exits with 0, or with 1 while the review rules hold a malformed rule. See [Troubleshooting](troubleshooting.md) for failures.

Revised 2026-11-25
