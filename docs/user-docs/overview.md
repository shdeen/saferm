# Overview

SafeRM gives AI coding agents a safer alternative to `rm` and `mv`. An agent runs `saferm delete` or `saferm move` like any other shell command. A delete moves the item to a trash directory and records where it came from. A move records the item's original location and—when necessary—makes overwrites restorable so that no action is permanently destructive. Each action is recorded with the agent's stated reason, so that it can be reviewed and restored later if necessary. Review rules add a measure of protection for sensitive paths, by holding a move or delete until you explicitly approve or deny it.

## How changes stay recoverable

- **Delete:** the item goes to SafeRM's trash, and its original path is recorded.
- **Move:** the original path is recorded, so the move can be reversed. If an item already occupies the destination, that item goes to trash first, as its own delete, instead of being overwritten.
- **Restore:** the item returns to its original path. If a new item now occupies that path, the new item goes to trash first.
- **Review rules:** a delete or move that affects an item a rule protects stays pending, with the item in place, until you approve or deny it.

Trash is never emptied automatically. Only purging items from trash and cleaning up history are permanent, and cleanup removes the records needed to reverse moves. See [Recovery and history](recovery.md).

## The three SafeRM programs

| Program | What it does |
| --- | --- |
| `saferm` | Command-line interface for agents and people. Agents use it to delete and move files. You can use it for everything the desktop app does. |
| SafeRM (`saferm-app`) | Desktop app for monitoring actions, deciding pending requests, restoring items, permanently deleting trash, and editing settings. |
| SafeRM Notify (`saferm-notify`) | Notifier installed with the desktop app. It shows pending requests as desktop notifications that you can approve or deny. |

The CLI and the desktop app have full feature parity. All three programs share the same per-user configuration and action history, so a request that an agent makes at the command line appears in the desktop app, and a decision made from a notification is reflected in both.

SafeRM does not intercept `rm`, `mv`, or other programs that change files. Direct your agent to use `saferm delete` and `saferm move`.

## Documentation

- [Quickstart](quickstart.md): install SafeRM, give `saferm` to your agent, review its actions, and configure SafeRM.
- [Installation](installation.md): supported systems, installer variables, upgrades, and uninstalling.
- [How deletes and moves work](file-operations.md): trash, recoverable overwrites, paths, and moves between filesystems.
- [Reviewing requests](review.md): review rules and deciding pending requests.
- [Recovery and history](recovery.md): restoring items, statuses, purging trash, and cleaning up history.
- [Configuration](configuration.md): settings and the trash location.
- [Command-line reference](cli.md): commands, options, confirmations, and exit status.
- [CLI output and JSON reference](cli-output.md): JSON fields and result shapes.
- [Desktop app reference](desktop.md): pages, controls, and settings.
- [Notifications](notifications.md): starting the notifier, notification actions, and diagnostics.
- [Troubleshooting](troubleshooting.md): error messages and interrupted operations.

Revised 2026-11-25
