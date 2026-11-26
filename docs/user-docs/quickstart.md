# Quickstart

## 1. Install SafeRM

On macOS or Linux:

```sh
curl -fsSL https://github.com/shdeen/saferm/releases/latest/download/install.sh | sh
```

On Windows, in PowerShell:

```powershell
irm https://github.com/shdeen/saferm/releases/latest/download/install.ps1 | iex
```

Then open SafeRM once from your applications menu. Its first launch sets the notifier to start when you log in. On macOS, approve the login item and notifications in System Settings if asked.

[Installation](installation.md) covers supported systems, package downloads, and installer options.

## 2. Give `saferm` to your agent

Add an instruction like this to your agent's instruction file, such as `AGENTS.md` or `CLAUDE.md`:

```markdown
Use `saferm` for file deletes and moves. It keeps them recoverable.

- Delete: `saferm delete "<path>" "<reason>"`
- Move: `saferm move "<source>" "<destination>" "<reason>"`
- The reason should say why the change is needed.
- A `pending` result means the path needs the user's approval. Report the action ID to the user.
- Use `saferm help` for full usage instructions.
```

See [Agent setup](agents.md) for more info on agent setup.

## 3. Review your agent's actions

In the desktop app, the **Operations** page lists every action with its path, reason, and requester, with pending requests first. A pending request has **Approve** and **Deny** buttons. A completed delete or move has **Restore**. The notifier also shows each pending request as a desktop notification with **Approve**, **Deny**, and **Review**.

In the CLI, a pending request appears in `saferm list` with the status `pending`:

```sh
saferm list --status pending    # requests waiting for a decision
saferm view <id>                # one action's details
saferm dispose <id> approve     # or: saferm dispose <id> deny
saferm restore <id>             # undo a delete or move
```

## 4. Configure SafeRM

SafeRM's settings live in `~/.saferm/config.toml`. Run `saferm config --setup` to write the file with the built-in values, then edit it. Alternatively, use the desktop app's **Settings** page, which saves each change as you make it.

| Setting | What it does |
| --- | --- |
| `trash-path` | Directory that deleted items go to. Initially `~/.saferm/trash`. |
| `retain-period` | Days that a resolved action stays in history before `saferm cleanup` can remove its record. It never erases items from trash. |
| `require-review` | Review rules for sensitive items. Initially empty. A delete or move that affects an item a rule protects waits for your approval. See [Reviewing requests](review.md). |

`saferm config` shows the settings in effect. See [Configuration](configuration.md) for details.

## Next

- [How deletes and moves work](file-operations.md)
- [Recovery and history](recovery.md)
- `saferm help <command>` for any command's options

Revised 2026-11-26
