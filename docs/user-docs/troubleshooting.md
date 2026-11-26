# Troubleshooting

## Before retrying a failed operation

A failed command can still have changed files, especially a replacement or a batch purge. Before retrying:

1. Run `saferm view <id>`, or open the action's details in the desktop app, and read its recorded errors.
2. Check the actual locations: the source, the destination of a move, and the trashed item of a delete. If `trash-path` has changed since the delete, check both trash directories.
3. For a replacement, check the move and the occupant's delete separately.

Don't add `--overwrite` or `--force` just to get past a conflict. They create further deletes.

## Interrupted or ambiguous operations

An action marked **In progress or interrupted** is either still running in another SafeRM process, or was cut off by a crash or power loss. SafeRM also refuses an operation on an action that another process changed in the meantime.

Wait for any running operation to finish and refresh the history. If the mark stays, check the recorded locations to see what actually happened, and deal with the files directly. SafeRM has no command that clears the mark. A marked action can't be decided, restored, or purged, and cleanup keeps it.

A copy between filesystems that was cut off can leave a partial copy at its destination, with the source intact. Compare the two before keeping or discarding either.

If an error says the filesystem operation succeeded but its result couldn't be saved, the history doesn't match the files. Check the source and destination before running anything else, and keep the history database and the trash as they are.

## Common problems

### A delete or move returned pending

One of these holds:

- A review rule protects the item, a folder above it, or an item inside it.
- SafeRM couldn't read a folder inside the item, and a rule could protect something in that folder. The command prints a warning that names the folder.
- The list of review rules holds a malformed rule. The command reports it as an error.
- The move is waiting for its occupant's delete.

`saferm view <id>` shows the rule and the blocker. See [Which requests need approval](review.md#which-requests-need-approval).

### Approval fails and the request stays pending

The source no longer exists, or the destination has been taken. Fix the paths and approve again, or deny the request.

### A restore can't find a deleted item

Restore looks in the current `trash-path`, which may differ from the directory the item was deleted to. Compare **Historical trash path** in `saferm view` with `saferm config`. See [Change the trash directory](configuration.md#change-the-trash-directory). Don't run `cleanup --orphans` until the item is found.

### A restore asks you to approve another delete

The original path is occupied, and the occupant's delete needs approval. Approve that delete, then run the restore again with the original ID.

### Cleanup didn't free any trash

Cleanup removes history records, never files. A completed delete whose item is still in trash is kept at any age. Purge the item to erase it. See [Recovery and history](recovery.md).

### Every delete and move is pending and reports a malformed review rule

The list of review rules holds a malformed rule, such as one with an unclosed `[`. The error names it. Until you fix or remove that rule, every delete and move is recorded as pending and exits with status 1, and `saferm config` reports the same error. In the desktop app, the Settings page marks the rule's row. See [Empty and malformed rules](review.md#empty-and-malformed-rules).

The requests recorded in the meantime stay pending. Approve or deny each one.

### The configuration can't be read

Check `~/.saferm/config.toml` for permissions, a broken symbolic link, or invalid TOML. `saferm help` and `saferm config --default` still work. Fix the file by hand, or reset it with **Reset to defaults** in the desktop app or `saferm config --setup --reset`. A reset replaces every setting, including `trash-path` and `require-review`.

### No notification appears

See [If notifications don't appear](notifications.md#if-notifications-dont-appear). The request is saved either way.

## Error messages

These phrases can appear within a longer message.

### Input and configuration

| Message | Meaning |
| --- | --- |
| `required input is missing` | A path, reason, destination, ID, or decision is missing or blank. |
| `input is invalid` | Bad syntax, option combination, ID, decision, or filter. Also reported when the destination's parent isn't a folder. |
| `configuration read failed` | The configuration file, or a link at its path, can't be read. |
| `configuration decoding failed` | The configuration file isn't valid TOML. |
| `configuration write failed` | The configuration file can't be written. |
| `review rule ... is malformed` | A review rule is not well formed. Every delete and move is recorded as pending until the rule is fixed or removed. |
| `home directory lookup failed` | SafeRM can't determine your home directory. |

### Files and paths

| Message | Meaning |
| --- | --- |
| `path resolution failed` | A path, a parent folder, or a link in it can't be resolved. |
| `file inspection failed` | The source, the destination's parent, or a trashed item can't be inspected. |
| `file transfer failed` | The move or copy failed. Check both locations. |
| `file removal failed` | Purge couldn't remove an item. Check what remains. |
| `destination already exists` | The destination or original path is occupied. |
| `item does not exist` | Purge couldn't find the item in the current trash directory. |
| `source and destination overlap` | The paths are the same, one contains the other, or the operation would move SafeRM's trash directory or history database. |
| `item type cannot be transferred across filesystems` | Only regular files, folders, and symbolic links can be moved between filesystems. |

### History and actions

| Message | Meaning |
| --- | --- |
| `database open failed`, `database read failed`, `database write failed`, `database close failed` | The history database couldn't be used. Keep the database as it is. |
| `filesystem operation succeeded, but its result could not be saved` | The files changed, but the history wasn't updated. Check the locations before doing anything else. |
| `action does not exist` | No action has that ID, or cleanup removed it. |
| `action is not eligible for this operation` | The action's type or status doesn't allow the operation. |
| `an operation is in progress or was interrupted` | Another operation is running, the action changed in the meantime, or an interrupted operation needs checking. |
| `recorded files do not establish a safe outcome; the record was preserved` | SafeRM can't tell that the operation is safe, for example a replacing restore of a failed action. Check the files. |
| `is awaiting approval` | A restore is waiting for the occupant's delete. Approve that delete, then restore again. |
| `one or more requested operations failed` | Part of a purge or cleanup failed. Check the individual problems and results. |

### CLI

| Message | Meaning |
| --- | --- |
| `operation canceled` | Ctrl-C interrupted the command. If a transfer had started, check the results. |
| `confirmation input read failed` | The answer to a question couldn't be read. |
| `output write failed`, `template rendering failed` | The results couldn't be printed. Check the history; the operation may have run. |
| `Notification unavailable` | A warning only. The request was saved, but the running notifier couldn't be reached. |

## Concurrent operations

Several `saferm` commands can run at once, alongside the desktop app. A command waits up to five seconds for another one that is writing the history. If another process changed the action first, SafeRM refuses the operation; reread the action before deciding what to do.

Revised 2026-11-26
