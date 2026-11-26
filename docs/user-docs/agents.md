# Agent setup

SafeRM protects only the deletes and moves that your agent makes through `saferm`. Tell the agent to use `saferm delete` and `saferm move`. 

## Add SafeRM to your agent's instructions

Add this to your agent's instruction file, such as `AGENTS.md` or `CLAUDE.md`:

```markdown
Use `saferm` for file deletes and moves. It keeps them recoverable.

- Delete: `saferm delete "<path>" "<reason>"`
- Move: `saferm move "<source>" "<destination>" "<reason>"`
- The reason should say why the change is needed.
- A `pending` result means the path needs the user's approval. Report the action ID to the user.
- Use `saferm help` for full usage instructions.
```

## What the agent gets back

Each command prints the action's ID and status. A `completed` delete or move has already happened, and you can restore it later. A `pending` request changes nothing until you approve or deny it, so the agent should report its ID to you. See [Reviewing requests](review.md).

| Option | Use |
| --- | --- |
| `--requester "<name>"` | Record which agent made the request. SafeRM shows the name with the action but doesn't verify it. |
| `--description "<text>"` | Record more context for the request. |
| `-j`, `--json` | Write results and errors as JSON. See [CLI output and JSON reference](cli-output.md). |

A pending request and a declined confirmation both exit with status 0. An agent that reads the results should check each action's `status`, not only the exit status.

`--overwrite` asks for confirmation on standard input. If the agent's command receives no input, the question counts as declined and nothing changes. `--force` skips the question.

## Block direct `rm` and `mv`

You may also wish to block direct `rm` and `mv` where your agent harness allows it.

SafeRM doesn't intercept `rm`, `mv`, or any other command. To stop your agent from running destructive commands directly, set its harness permissions:

- [Claude Code permissions](https://code.claude.com/docs/en/permissions)
- [Codex permissions](https://learn.chatgpt.com/docs/permissions)

For another harness, see its documentation on command permissions.

Revised 2026-11-26
