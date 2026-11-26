# CLI output and JSON reference

Without `-j` (`--json`), `saferm` prints text for people to read: a summary of each action a command created or changed, a table for `list`, one line per detail for `view`, and the settings for `config`. Text output shows times in local time, `(unset)` for a missing value, and `none` for an empty list of review rules. For scripts, use JSON.

## JSON output

With `-j`, results go to standard output as indented JSON. The shape depends on the command:

| Command | Result |
| --- | --- |
| `list` | An array of action objects. Empty when nothing matches. |
| `view` | One action object. |
| `delete`, `move`, `dispose`, `restore` | An object with an `actions` array of the actions that the command created or changed. The array is there for one action too. A `problems` array of warnings is added when the command warns about a folder it couldn't read. |
| `purge` | An object with an `actions` array of purged actions and a `problems` array of failures. Either array may be omitted when empty. |
| `cleanup` | An object with a `removed` array of IDs and a `problems` array. Either array is omitted when empty. |
| `config`, `config --default` | A configuration object. |
| `config --setup` | An object with `config` and `notice` when it created the file, or only `notice` when the file already existed. |
| A declined confirmation | `{"notice": "Canceled."}` |

The `actions` array of a `delete`, `move`, `dispose`, or `restore` holds one action for most requests. It holds more than one for a `move` or `restore` that replaces an item, and for a `dispose` that decides a move together with its blocker.

### Action object

`status` is a number. Times are UTC, in ISO 8601 format. Optional fields are omitted when empty.

| Field | Type | Always present | Meaning |
| --- | --- | --- | --- |
| `id` | Integer | Yes | Action ID. |
| `type` | String | Yes | `delete` or `move`. |
| `status` | Integer | Yes | Status code; see the table below. |
| `source` | String | Yes | Resolved absolute source path. |
| `destination` | String | No | Destination of a move. |
| `trashID` | String | No | The deleted item's unique name in trash. |
| `trashPath` | String | No | Trash directory when the delete was requested. Restore and purge use the current `trash-path` instead. |
| `reason` | String | Yes | Reason given with the request. |
| `requester` | String | No | Requester given with the request. |
| `description` | String | No | Description given with the request. |
| `createdAt` | String | Yes | When the action was created. |
| `updatedAt` | String | Yes | When the action last changed. |
| `resolvedAt` | String | No | When the latest operation on the action was resolved. |
| `reviewRule` | String | No | The review rule that made the action need approval. |
| `blockerID` | Integer | No | ID of a move's occupant delete. |
| `decision` | String | No | `approve` or `deny`. |
| `failure` | String | No | Recorded errors, one per line. |
| `operation` | String | No | Latest operation started: `delete`, `move`, `restore`, or `purge`. |
| `inProgress` | Boolean | Yes | Whether an operation is running or was interrupted. |

| Code | Status |
| --- | --- |
| 1 | `completed` |
| 2 | `pending` |
| 3 | `denied` |
| 4 | `restored` |
| 5 | `purged` |
| 6 | `failed` |

The action object of a completed delete, for example:

```json
{
  "id": 12,
  "type": "delete",
  "status": 1,
  "source": "/Users/me/notes/old.txt",
  "trashID": "6f1c2b8e-4f3a-4d6e-9a51-2c7d0e8b9f10",
  "trashPath": "/Users/me/.saferm/trash",
  "reason": "Remove stale notes",
  "createdAt": "2026-11-24T02:41:00Z",
  "updatedAt": "2026-11-24T02:41:00Z",
  "resolvedAt": "2026-11-24T02:41:00Z",
  "operation": "delete",
  "inProgress": false
}
```

### Configuration object

| Field | Type | Meaning |
| --- | --- | --- |
| `trashPath` | String or null | `trash-path` setting. |
| `retainPeriod` | Integer or null | `retain-period` setting. |
| `requireReview` | Array of strings or null | `require-review` setting. An empty array in the built-in values. |
| `path` | String | Path of the configuration file. Empty for `config --default`. |
| `external` | Boolean | `true` when read from the configuration file, `false` for the built-in values. |

A setting missing from the file is `null`.

### Confirmation questions

With `-j`, a confirmation question goes to standard error as a JSON object. The answer is still a plain line on standard input.

| Field | Type | Meaning |
| --- | --- | --- |
| `prompt` | String | The question, with the answer hint. |
| `path` | String | Affected path. |
| `defaultYes` | Boolean | Whether an empty line confirms. |
| `moveID` | Integer | The move, when deciding a move with its blocker. |
| `blockerID` | Integer | The blocker, when deciding a move with its blocker. |

### Errors and warnings

An error goes to standard error as `{"error": "<message>"}`. Results that accompany an error still go to standard output. For example, purge and cleanup report their completed items and their problems before exiting with status 1, and a replacement can return a completed occupant delete even when the move failed.

While the review rules hold a malformed rule, every `delete` and `move` reports an error that names the rule and exits with status 1. The pending action still goes to standard output. `config` prints the settings, reports the same error, and exits with status 1. See [Empty and malformed rules](review.md#empty-and-malformed-rules).

A request can be pending because SafeRM couldn't read a folder inside the item; see [Folders that SafeRM can't read](review.md#folders-that-saferm-cant-read). The command then gives a warning that names each such folder, and exits with the status of any pending request:

- In text output, the warning follows the action's summary on standard output.
- With `-j`, the warning is an entry in the result's `problems` array, beside the `actions` array.

For example, with `-j`:

```json
{
  "actions": [
    {
      "id": 14,
      "type": "delete",
      "status": 2,
      "source": "/Users/me/old",
      "trashID": "32563943-f11b-4feb-975b-16d086420948",
      "trashPath": "/Users/me/.saferm/trash",
      "reason": "Remove old downloads",
      "createdAt": "2026-11-25T18:01:48Z",
      "updatedAt": "2026-11-25T18:01:48Z",
      "reviewRule": "*.pem",
      "operation": "delete",
      "inProgress": false
    }
  ],
  "problems": [
    "<warning that names /Users/me/old/locked>"
  ]
}
```

A warning that a notification couldn't be delivered is timestamped text on standard error, not JSON. Don't parse standard error as one JSON value.

## In scripts

- Parse standard output and standard error separately.
- Handle every result shape above. A `delete`, `move`, `dispose`, or `restore` returns its actions in the `actions` array, for one action as for several. Only `view` returns a bare action object.
- Check the exit status, each action's `status` and `inProgress`, any `notice`, and any `problems`.
- Exit status 0 doesn't mean a file moved. A declined confirmation exits with 0, and so does a pending request, except while the review rules hold a malformed rule.
- Exit status 1 doesn't mean that nothing was recorded. While the review rules hold a malformed rule, a delete or move is recorded as pending and exits with 1.
- After exit status 1, inspect the partial results and the actual paths before retrying. See [Troubleshooting](troubleshooting.md).

Revised 2026-11-25
