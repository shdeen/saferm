# SafeRM documentation

SafeRM gives AI coding agents a safer alternative to `rm` and `mv`. Every delete and move made through `saferm` is recorded and can be restored. Review rules hold changes to sensitive paths until you approve or deny them.

## Get started

- [Overview](overview.md): what SafeRM does and its three programs.
- [Quickstart](quickstart.md): install SafeRM, give `saferm` to your agent, review its actions, and configure SafeRM.
- [Installation](installation.md): supported systems, installer variables, upgrades, and uninstalling.
- [Agent setup](agents.md): instructions for your agent, its results, and blocking direct `rm` and `mv`.

## Guides

- [How deletes and moves work](file-operations.md): trash, recoverable overwrites, paths, and moves between filesystems.
- [Reviewing requests](review.md): review rules and deciding pending requests.
- [Recovery and history](recovery.md): restoring items, statuses, purging trash, and cleaning up history.
- [Configuration](configuration.md): settings and the trash location.
- [Notifications](notifications.md): starting the notifier, notification actions, and diagnostics.
- [Troubleshooting](troubleshooting.md): error messages and interrupted operations.

## Reference

- [Command-line reference](cli.md): commands, options, confirmations, and exit status.
- [CLI output and JSON reference](cli-output.md): JSON fields and result shapes.
- [Desktop app reference](desktop.md): pages, controls, and settings.

Revised 2026-11-26
