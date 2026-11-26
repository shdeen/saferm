# Configuration

SafeRM reads its settings from `~/.saferm/config.toml`. The CLI, the desktop app, and the notifier use the same file. Changes take effect without restarting any of them.

## Settings

| Key | Type | What it does |
| --- | --- | --- |
| `trash-path` | String | Directory that deleted items go to. SafeRM creates it on the first delete. |
| `retain-period` | Whole number, 0 or more | Days that a resolved action stays in history before cleanup can remove its record. With 0, resolved records are removable at once. It never erases items from trash; see [Cleanup eligibility](recovery.md#cleanup-eligibility). |
| `require-review` | Array of strings | Review rules for sensitive items. A delete or move that affects an item they protect waits for your approval; see [Reviewing requests](review.md). |

`trash-path` may start with `~`. A relative path is resolved against the working directory of whichever program runs the operation, so use an absolute or `~` path.

Without a configuration file, SafeRM uses its built-in values: trash goes to `~/.saferm/trash`, the retention period is 30 days, and the list of review rules is empty. `saferm config --default` prints those values:

```toml
trash-path = "~/.saferm/trash"
retain-period = 30
require-review = []
```

## View and edit the settings

```sh
saferm config            # Shows all settings in effect, and where they come from
saferm config --setup    # Create the config file at `~/.saferm/config.toml` using program defaults
```

`--setup` leaves an existing file unchanged. Edit the file in a text editor, or use **Settings** in the desktop app. The app saves each change automatically and rewrites the whole file in its own layout, which drops comments and custom formatting. The app accepts only an existing folder as the trash path, and it doesn't save a malformed review rule.

To replace the file with the built-in values, run `saferm config --setup --reset`, or use **Reset to defaults** in the app. Both ask first when a file exists. In the CLI, an empty answer means yes.

## A key missing from the file is unset

Once the file exists, SafeRM uses only what it contains. A missing key does not fall back to its built-in value.

| Missing key | Effect |
| --- | --- |
| `trash-path` | Deletes, replacements, approvals of pending deletes, and restores and purges of deleted items fail. Ordinary moves still work. |
| `retain-period` | Cleanup fails. |
| `require-review` | Nothing needs approval. An empty array has the same effect. |

SafeRM ignores keys it doesn't recognize, including misspelled ones. A blank `trash-path` or a negative `retain-period` makes whatever needs that setting fail with `setting is invalid`. A malformed review rule doesn't stop a request from being recorded: every delete and move is recorded as pending, reports the rule as an error, and exits with status 1; see [Empty and malformed rules](review.md#empty-and-malformed-rules). A file that isn't valid TOML stops every command except `saferm help`, `saferm config --default`, and `saferm config --setup`.

## Change the trash directory

Changing `trash-path` doesn't move existing trash. Restore, purge, and cleanup look for a deleted item in the directory configured now, not the one it was deleted to. `saferm view` shows the directory recorded at deletion as **Historical trash path**; the desktop app shows it as **Recorded trash directory**.

To move your trash:

1. Make sure no SafeRM operation is running.
2. Move the contents of the old trash directory into the new one, keeping every item's name.
3. Set `trash-path` to the new directory.

Until the items are moved, restores and purges of earlier deletes fail, and `saferm cleanup --orphans` can remove their records even though the items still exist. Setting `trash-path` back to the old directory makes SafeRM look there again.

## Files in ~/.saferm

SafeRM creates these files and directories readable only by you.

| Path | Contents |
| --- | --- |
| `config.toml` | Settings. |
| `saferm.sqlite` | Action history. |
| `trash/` | Deleted items, unless `trash-path` names another directory. |
| `notifications.log` | Errors from the desktop app and the notifier. |
| `notifications.json` | Connection details of the running notifier. Removed when it quits. |
| `notifications.setup` | Shows that the notifier's login entry was created, so the desktop app doesn't create it again. |

Uninstalling SafeRM leaves this directory in place.

Revised 2026-11-25
