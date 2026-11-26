# SafeRM

SafeRM gives AI coding agents a safer alternative to `rm` and `mv`. An agent runs `saferm delete` or `saferm move` like any other shell command. A delete moves the item to a trash directory and records where it came from. A move records the item's original location and—when necessary—makes overwrites restorable so that no action is permanently destructive. Each action is recorded with the agent's stated reason, so that it can be reviewed and restored later if necessary. Review rules add a measure of protection for sensitive paths, by holding a move or delete until you explicitly approve or deny it.

SafeRM runs on macOS, Linux, and Windows. It consists of three programs that share one per-user configuration and action history:

| Program | Purpose |
| --- | --- |
| **`saferm`** | CLI utility for both agents and humans |
| **SafeRM App** | Desktop app for reviewing requests and managing recovery. |
| **SafeRM Notify** | Optional background process for pending requests. |

The CLI and the desktop app have full feature parity. Agents use the CLI. You can use either one to monitor actions, decide pending requests, and restore items.

## What SafeRM does not protect

SafeRM does not intercept `rm`, `mv`, or any other program that changes files. Only deletes and moves made through `saferm` are recoverable, so tell your agent to use it.

## Install

### macOS and Linux

```sh
curl -fsSL https://github.com/shdeen/saferm/releases/latest/download/install.sh | sh
```

Requires `curl` or `wget` for downloads and `sha256sum` or `shasum` for verification.

| System | Installer behavior |
| --- | --- |
| macOS 13 or later, Intel or Apple Silicon | Installs `/Applications/SafeRM.app` and links `/usr/local/bin/saferm` to the CLI inside the app. |
| Ubuntu 24.04 or a compatible APT system | Installs the DEB package and its runtime dependencies. |
| Fedora 44 or a compatible DNF system | Installs the RPM package and its runtime dependencies. |

On **Linux**, .rpm and .deb packages support AMD64 and ARM64. All packages install to `/usr/lib/saferm`, link the CLI at `/usr/bin/saferm`, and add SafeRM to the applications menu. Other distributions must provide the GTK 4 and WebKitGTK 6 runtime dependencies that the packages require.

### Windows

Run this in PowerShell 5.1 or later:

```powershell
irm https://github.com/shdeen/saferm/releases/latest/download/install.ps1 | iex
```

The installer selects AMD64 or ARM64, runs the per-user Windows installer, and installs WebView2 if it is missing. The installation directory is added to your `PATH` and to the current PowerShell session.

The default location is the `AppData\Local\Programs\SafeRM` directory in your user profile.

[Installation](docs/user-docs/installation.md) lists the installer variables for choosing a version or an install location.

### Upgrade

Close SafeRM, then run the same install command again. A download or checksum failure leaves the existing installation unchanged. Installing and upgrading keep your SafeRM history, configuration, and retained files.

### Install a downloaded package

Each release on the [Releases page](https://github.com/shdeen/saferm/releases) lists its packages individually, one for each operating system and processor. A completed GitHub Actions run lists its packages under **Artifacts**; downloading them requires signing in to GitHub. On Linux, `uname -m` reports `x86_64` for AMD64 and `aarch64` for ARM64.

| System | Package | How to install it |
| --- | --- | --- |
| Ubuntu 24.04 or a compatible APT system | `.deb` | In the download directory, run `sudo apt install ./FILENAME.deb` with the downloaded filename. |
| Fedora 44 or a compatible DNF system | `.rpm` | In the download directory, run `sudo dnf install ./FILENAME.rpm` with the downloaded filename. |
| macOS | `.dmg` | Open the disk image and drag SafeRM to Applications. The CLI is inside the app at `/Applications/SafeRM.app/Contents/MacOS/saferm`. |
| Windows | `.exe` | Run the downloaded installer. |

### After installation

Open SafeRM once from your applications menu. Its first launch sets the notifier to start when you log in. On macOS, approve the login item and notifications in System Settings if asked.

### Uninstall

On Linux, run `sudo apt remove saferm-app` or `sudo dnf remove saferm-app`. On Windows, use **Installed apps**; this also removes SafeRM's directory from your user `PATH`. On macOS, delete `/Applications/SafeRM.app` and the `saferm` link. Uninstalling leaves `~/.saferm` in place, with your configuration, history, and the default trash directory.

## Give `saferm` to your agent

Add an instruction like this to your agent's instruction file, such as `AGENTS.md` or `CLAUDE.md`:

```markdown
Use `saferm` for file deletes and moves. It keeps them recoverable.

- Delete: `saferm delete "<path>" "<reason>"`
- Move: `saferm move "<source>" "<destination>" "<reason>"`
- The reason should say why the change is needed.
- A `pending` result means the path needs the user's approval. Report the action ID to the user.
- Run `saferm help` for detailed usage.
```

Each command prints the action's ID and status. An agent can add `--requester "<name>"` to record which agent made the request; SafeRM shows the name with the action but does not verify it.

A delete or move whose source matches a [review rule](#review-rules) returns `pending`, and the item stays in place until you approve or deny the request.

With `-j` (`--json`), the CLI writes results and errors as JSON. A pending request and a declined confirmation both exit with status 0, so check each action's `status` as well as the exit status. See the [CLI output and JSON reference](docs/user-docs/cli-output.md).

## Monitor and restore

The desktop app's **Operations** page lists every action with its source path, reason, and requester, with pending requests first. From there you can approve or deny a pending request, restore a deleted or moved item to its original path, and permanently delete items from trash. The notifier, which comes with the desktop app, also shows each pending request as a desktop notification with **Approve**, **Deny**, and **Review**.

In the CLI, a pending request appears in `saferm list` with the status `pending`:

```sh
saferm list --status pending    # requests waiting for a decision
saferm view <id>                # one action's details
saferm dispose <id> approve     # or: saferm dispose <id> deny
saferm restore <id>             # undo a delete or move
```

## Review rules

Review rules name the sensitive paths that SafeRM escalates to you. A delete or move of a matching path stays `pending`, with the item in place, until you approve or deny it. Rules are patterns in the `require-review` list of `~/.saferm/config.toml`, which the desktop app's **Settings** page also edits. See [Reviewing requests](docs/user-docs/review.md) for the pattern syntax.

## Configuration

SafeRM's settings live in `~/.saferm/config.toml`. Run `saferm config --setup` to write the file with the built-in values, then edit it, or use the desktop app's **Settings** page. `saferm config` shows the settings in effect.

| Setting | What it does |
| --- | --- |
| `trash-path` | Directory that deleted items go to. Initially `~/.saferm/trash`. |
| `retain-period` | Days that a resolved action stays in history before `saferm cleanup` can remove its record. It never erases items from trash. |
| `require-review` | Patterns for sensitive paths that need your approval. |

Once the file exists, a key missing from it is unset; it doesn't fall back to its built-in value. Changing `trash-path` doesn't move existing trash; see [Change the trash directory](docs/user-docs/configuration.md#change-the-trash-directory).

## Recovery and permanent deletion

- To restore an item to a path that a new item now occupies, use `--overwrite` in the CLI, or **Allow replacement at the original path** in the desktop app. The new item goes to trash first.
- `saferm purge <id>...` permanently erases deleted items from trash. It does not ask for confirmation, and it cannot be undone through SafeRM.
- `saferm cleanup` removes history records according to `retain-period`. It never removes files. Once a completed move's record is removed, SafeRM can no longer reverse that move.

SafeRM does not erase trash when the retention period expires. Read [Recovery and history](docs/user-docs/recovery.md) before you purge trash or clean up history.

## Commands

| Command | Purpose |
| --- | --- |
| [`delete`](docs/user-docs/cli.md#delete) | Move an item into recoverable trash. |
| [`move`](docs/user-docs/cli.md#move) | Move an item to a recorded destination. |
| [`list`](docs/user-docs/cli.md#list) | List action history. |
| [`view`](docs/user-docs/cli.md#view) | Show one action's details. |
| [`dispose`](docs/user-docs/cli.md#dispose) | Approve or deny a pending action. |
| [`restore`](docs/user-docs/cli.md#restore) | Return an item to its original path. |
| [`purge`](docs/user-docs/cli.md#purge) | Permanently erase one or more deleted items. |
| [`cleanup`](docs/user-docs/cli.md#cleanup) | Remove eligible history records. |
| [`config`](docs/user-docs/cli.md#config) | Display, create, or reset configuration. |
| [`help`](docs/user-docs/cli.md#help) | Show general or command-specific help. |

Run `saferm help <command>` or `saferm <command> -h` for a command's syntax and options.

## Documentation

- [Overview](docs/user-docs/overview.md)
- [Quickstart](docs/user-docs/quickstart.md)
- [Installation](docs/user-docs/installation.md)
- [How deletes and moves work](docs/user-docs/file-operations.md): trash, recoverable overwrites, paths, and moves between filesystems.
- [Reviewing requests](docs/user-docs/review.md): review rules and deciding pending requests.
- [Recovery and history](docs/user-docs/recovery.md): restoring items, statuses, purging trash, and cleaning up history.
- [Configuration](docs/user-docs/configuration.md): settings and the trash location.
- [Command-line reference](docs/user-docs/cli.md): commands, options, confirmations, and exit status.
- [CLI output and JSON reference](docs/user-docs/cli-output.md): JSON fields and result shapes.
- [Desktop app reference](docs/user-docs/desktop.md): pages, controls, and settings.
- [Notifications](docs/user-docs/notifications.md): starting the notifier, notification actions, and diagnostics.
- [Troubleshooting](docs/user-docs/troubleshooting.md): error messages and interrupted operations.

Revised 2026-11-24
