# Changelog

## [0.1.0] - 2026-11-26

First public release. SafeRM gives AI coding agents a safer alternative to `rm` and `mv`, with deletes and moves that can be reviewed and restored, on macOS, Linux, and Windows.

### Features

- Recoverable deletes and moves from the terminal. `saferm delete` moves an item to a trash directory, and `saferm move` records the item's original path. `saferm restore` returns the item to that path.
- Each action is recorded with the reason that the agent states, and optionally with a requester name and a description given with `--requester` and `--description`. `saferm list` and `saferm view` show the history, and `saferm list` filters it by type and status.
- Restorable overwrites. With `--overwrite`, `saferm move` first moves the item at the destination to trash, so the replaced item can be restored too. The CLI asks for confirmation unless `--force` is given. `saferm restore --overwrite` does the same for an item that now occupies the original path.
- Review rules for sensitive paths. A delete or move that matches a pattern in the `require-review` list stays pending, with the item in place, until you approve or deny it with `saferm dispose` or in the desktop app. A pattern names an item or a path and accepts `*`, `?`, `**`, character sets, and `{a,b}` alternatives.
- Trash and history maintenance. `saferm purge` permanently erases selected items from trash, and `saferm cleanup` removes history records whose retention period has passed. Cleanup never removes files.
- Output for agents and scripts. Each command prints the ID and status of its actions, and `-j`/`--json` writes results and errors as JSON.
- SafeRM App, a desktop app with the same features as the CLI. It lists every action with pending requests first, and you can approve or deny requests, restore items, purge trash, and edit settings there.
- SafeRM Notify, an optional background program that shows each pending request as a desktop notification with **Approve**, **Deny**, and **Review**. The desktop app's first launch sets SafeRM Notify to start at login.
- Configuration file at `~/.saferm/config.toml` that sets the trash directory, the retention period, and the review rules. `saferm config` shows the settings in effect, and `saferm config --setup` writes the file with the built-in values. The desktop app's **Settings** page edits the same file.
- Install scripts for macOS and Linux (`install.sh`) and Windows (`install.ps1`), which verify each package's checksum before they install it.
- Native packages: a DMG for macOS 13 or later on Intel and Apple Silicon, DEB and RPM packages for Linux on AMD64 and ARM64, and a per-user installer for Windows on AMD64 and ARM64.

[0.1.0]: https://github.com/shdeen/saferm/releases/tag/v0.1.0

Revised 2026-11-26
