# Desktop app reference

The desktop app is where you watch what your agents did: review their actions, decide pending requests, restore items, permanently delete trash, and edit settings. It has the same features as the CLI and shares its history and configuration.

Only one copy of the app runs at a time; opening it again brings its window to the front. On macOS, closing the window quits the app. The notifier is a separate program and keeps showing pending requests.

`saferm-app --review <id>` opens the details of one action, in the running app or in a new window. The notifier's **Review** action uses it.

## Sidebar

The sidebar holds **Operations** and four controls: **History cleanup**, **Help** (the built-in Using SafeRM page), a dark and light mode switch, and **Settings**. The app follows the system appearance until you pick a mode, then remembers your choice.

## Operations page

The **Operations** page lists every action. Pending actions come first, then the newest. The page refreshes on its own and after each change you make, and **Refresh** reloads it on demand.

- **Status** filter: all statuses, or one status.
- **Operation type** filter: all types, Delete, or Move.
- **Page size**: 20, 50, or 100 actions. Changing a filter or the page size returns to the first page.

Each row shows the item's name and ID, its source path, the reason and requester, the status and type, the request time, and the buttons that apply:

| Status | Buttons |
| --- | --- |
| Pending | **Approve**, **Deny** |
| Completed or failed | **Restore**. For a delete, also permanent deletion. |
| Denied, restored, or purged | None |
| In progress or interrupted | None |

Every row also opens its details. Completed and failed deletes have checkboxes; **Permanently delete selected (N)** acts on the checked rows of the current page.

## Action details

Selecting an action's name opens a panel with its status, type, and reason, plus whichever of these it has: source and destination paths, recorded trash directory, trash item ID, requester, description, review rule, blocker, decision, last operation, timestamps, and recorded errors. The panel has the same buttons as the row. An action marked in progress shows a warning; see [Interrupted or ambiguous operations](troubleshooting.md#interrupted-or-ambiguous-operations).

## Create a delete or move

**New operation** creates a delete or move, as the CLI's `delete` and `move` do. Enter the source path, the destination for a move, and a reason; the requester and description are optional. The folder buttons open a file picker that shows hidden files.

For a move onto an existing item, check **Allow replacement at the destination**. The app asks before replacing, and the occupant goes to trash first. Review rules still apply. The app has no equivalent of `--force`.

A request that needs approval is recorded as pending and appears at the top of the **Operations** page. If SafeRM couldn't read a folder inside the item, the app shows the warning that names the folder. While the review rules hold a malformed rule, the app shows the error that names the rule, and the request is recorded as pending. See [Which requests need approval](review.md#which-requests-need-approval).

If the delete or move fails, the form stays open with what you entered.

## Approve, deny, and restore

**Approve** and **Deny** act at once. For a move whose blocker is still pending, the app first asks whether to decide both; see [Replacements](review.md#replacements).

**Restore** shows the original path. To replace an item that now occupies it, check **Allow replacement at the original path**; the app asks before replacing. If the occupant's delete needs approval, approve it and restore again. See [Restore an item](recovery.md#restore-an-item).

## Permanently delete

**Permanently delete**, in an action's details or for the checked rows, lists the IDs and paths and asks for confirmation. The items are erased from trash and can't be restored. Their history records stay. If some items fail, an error dialog lists them.

## History cleanup

**History cleanup** removes eligible history records; check **Include orphaned records** to also remove records whose items are missing. Cleanup never removes files. The app reports how many records were removed. See [Clean up history](recovery.md#clean-up-history).

## Settings

The Settings page edits `~/.saferm/config.toml` and saves each change automatically. Saving rewrites the whole file in the app's layout, which drops comments and custom formatting. If no file exists, the first save creates one. See [Configuration](configuration.md).

- **Configuration source** shows the file's path, or **Embedded defaults** when there is no file. **Open** opens the file in your default editor.
- **Trash directory** accepts only an existing folder, chosen with **Change** or typed in. A missing folder isn't saved. Before changing it, read [Change the trash directory](configuration.md#change-the-trash-directory).
- **History retention (days)** accepts a whole number, 0 or more.
- When the file sets a trash directory or retention period that SafeRM can't use, the page shows the built-in value in that field and an error below it that names the value from the file. The next save writes the values shown. See [Missing and unusable values](configuration.md#missing-and-unusable-values).
- **Paths requiring review** has one row per rule. The plus button adds a row, and the minus button removes the row you last selected. The app doesn't save a malformed rule: the row keeps your text and is marked once you leave it, and the file keeps the rule that the row last saved. A malformed rule that the file already holds is marked when the page opens. See [Write review rules](review.md#write-review-rules).
- **Reset to defaults** replaces the settings with the built-in values, after confirmation when a file exists. It works even when the current file can't be read, so it can repair a broken file.

Revised 2026-11-26
