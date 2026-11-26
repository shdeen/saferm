# Reviewing requests

Review rules name the sensitive items that SafeRM escalates to you. When an agent's delete or move would affect an item that a rule protects, SafeRM records the request as `pending` and leaves the item in place until you approve or deny it.

## Decide a pending request

The notifier shows each pending request as a desktop notification with **Approve**, **Deny**, and **Review**. **Review** opens the request in the desktop app, where pending requests are listed first on the **Operations** page. See [Notifications](notifications.md).

In the CLI, a pending request appears in `saferm list` with the status `pending`:

```sh
saferm list --status pending
saferm view <id>
saferm dispose <id> approve    # or: saferm dispose <id> deny
```

The request's details show the source, the destination of a move, the reason, the requester, and the rule that requires approval.

Approving carries out the delete or move. If the source has disappeared or the destination has been taken since the request, approval fails and the request stays pending, so you can still deny it. Denying leaves the item untouched. A request can be decided only once.

Approval doesn't check the rules again, and changing the rules doesn't affect requests already recorded.

## Write review rules

Rules are the `require-review` list in `~/.saferm/config.toml`. The desktop app edits the same list under **Settings → Paths requiring review**. The list starts empty, so no request needs approval until you add a rule.

```toml
require-review = ["~/Documents/contracts/", "*.pem", "**/{secrets,credentials}/"]
```

A rule describes an item by its name or by its path. A rule that matches a folder protects everything inside that folder. A request needs approval when it would delete or move a protected item.

### Name rules and path rules

A **path rule** starts with `/` or `~/`, is exactly `~`, or has a `/` anywhere before its last character. On Windows, a rule that starts with a drive, such as `C:/`, is also a path rule. SafeRM compares a path rule with an item's full path.

Every other rule is a **name rule**. SafeRM compares it with an item's own name, so it matches an item of that name in any folder. `*.pem` and `.git/` are name rules. A `/` at the very end doesn't make a rule a path rule.

A path rule is measured from one of these folders:

| Rule | Measured from |
| --- | --- |
| Starts with `/` | The root of the filesystem. `/` alone names the root folder. |
| Is `~`, or starts with `~/` | Your home folder. `~` alone names the home folder. |
| Any other path rule | The root of the filesystem. `build/output` means `/build/output`, and doesn't match `/home/me/build/output`. |
| Starts with a drive, on Windows | The root of that drive. The rule applies only to that drive. |

On Windows, every other path rule that is measured from the root uses the root of the drive or network share that holds the item.

To match in the home folder, write `~/build/output`. To match in any folder, write `**/build/output`.

SafeRM compares rules with an item's real path, after it resolves the symbolic links in the folders above the item.

### Wildcards, sets, and alternatives

| In a rule | Matches |
| --- | --- |
| `*` | Any run of characters within one name, including none. It doesn't match `/`. |
| `?` | Exactly one character. It doesn't match `/`. |
| `[abc]` | One of the listed characters. |
| `[a-z]` | One character in the range. |
| `[!abc]` or `[^abc]` | One character that is not listed. |
| `{a,b}` | `a` or `b`. Each alternative may contain wildcards. |
| `**` as a whole name, in a path rule | Zero or more folders. |

A `.` is an ordinary character to `*` and `?`: `*.p*` matches `.p`, `xyz.p`, and `a.pop`, and `*` matches names that start with `.`.

`*` and `?` always stand for part of one name. `/a/*` matches each item directly inside `/a`, and not `/a` itself.

In a path rule, the one character that a set matches can be a `/` when the set covers it. `[!abc]` covers `/`, because `/` is not listed, so `/work/a[!x]b` matches `/work/a/b`. In a name rule this can't happen, because a name holds no `/`.

`**` works like this in a path rule:

- `**/x` matches `x` in any folder.
- `/a/**/b` matches `/a/b`, `/a/x/b`, `/a/x/y/b`, and so on.
- `/a/**` matches `/a` and everything inside it. It protects the same items as `/a`.

Anywhere else, including in a name rule, `**` works like `*`.

### Folders and their contents

A rule that ends in `/` matches folders only. A symbolic link that points to a folder counts as a folder for this purpose, and so does a link whose target SafeRM can't inspect.

A rule that matches a folder also protects everything inside it, at any depth. An item is protected when a rule matches the item itself or any folder above it.

### Symbolic links

A path rule that is written through a symbolic link protects the real location. If `~/MyDocuments` is a link to `~/My Documents`, the rule `~/MyDocuments/reports/` protects `~/My Documents/reports`. On macOS, `/tmp` is a link to `/private/tmp`, so `/tmp/build-*` protects `/private/tmp/build-*`.

SafeRM follows links only in the folder names that a rule spells out:

- SafeRM follows them in the folder that the rule is measured from and in each name after it. It stops at the first name that holds a `*`, `?`, `[`, `{`, or `\`, because a name like that stands for many names and can't be looked up on disk. In `~/Projects/*/secrets`, SafeRM follows links in `~/Projects` only.
- A path rule in which no name holds one of those characters spells out the path of one item. When that item is a symbolic link, the rule protects both the link and the item it points to. The rule `~/MyDocuments/` protects the link `~/MyDocuments`, the folder `~/My Documents`, and everything inside that folder.
- A folder that a link leads to keeps its exact name. If the link points to a folder named `Album[1]`, the rule means that folder, and not a folder named `Album1`.

Any other rule that matches a link protects the link alone. That covers a name rule, and a path rule in which some name holds one of those characters.

### Letter case

The folder that holds a name decides whether letter case matters for that name. If the folder treats `Notes` and `notes` as one name, rules do too. If it treats them as two names, case must match. macOS sets this for a whole volume. Windows and Linux can set it folder by folder.

- A name rule is compared with one name, so that name's folder decides.
- In a path rule, each name that the rule spells out is decided by its own folder. [Symbolic links](#symbolic-links) describes which names a rule spells out.
- From the first name that holds a `*`, `?`, `[`, `{`, or `\`, the remaining names of a path rule are compared together. The deepest of them that has letters is the one whose folder decides for all of them.

### Separators and literal characters

A character means the same thing wherever it stands in a rule.

- `/` separates folders on every system.
- On macOS and Linux, `\` makes the next character literal, so `\*` matches a `*`.
- On Windows, `\` separates folders, as `/` does.
- On every system, a set that lists one character matches exactly that character: `[*]` matches a `*`, `[?]` matches a `?`, `[[]` matches a `[`, and `[{]` matches a `{`.
- Every other character matches itself, including `#`, spaces, and a `!` outside a set. Inside a set, a leading `!` means "not".

### Empty and malformed rules

An empty rule is ignored. The desktop app saves an empty rule when you add a row, and that row changes nothing until you type in it.

A rule is malformed when its text, apart from a `/` at the very end, has any of these:

- a `[` with no closing `]`;
- a set that lists no characters, such as `[]` or `[!]`;
- a `{` with no closing `}`, or a `}` with no opening `{`;
- on macOS and Linux, a `\` at its end.

SafeRM can't tell what a malformed rule was meant to protect, so it treats the rule as protecting every item. While the list holds a malformed rule:

- Every delete and move is recorded as pending. Each one also reports an error that names the rule, and `saferm delete` and `saferm move` exit with status 1. The desktop app shows the same error for a delete or move that you submit there.
- The request records a rule by the usual list order, with the malformed rule counted as protecting the source. With `*.pem` listed before `[abc`, deleting `a.pem` records `*.pem`, and deleting `notes.txt` records `[abc`.
- `saferm config` shows the settings, reports the same error, and exits with status 1.
- The desktop app's Settings page marks the rule's row and shows a message for the list. The page doesn't save a malformed rule that you type: the row keeps your text and is marked once you leave it, and the saved rule stays as it was.

Fix or remove the rule to end this. Requests that are already pending stay pending until you decide them.

### Examples

These examples use `/home/me` as the home folder. In the rows about links, `/home/me/MyDocuments` is a symbolic link to the folder `/home/me/My Documents`.

| Rule | Item | Protected |
| --- | --- | --- |
| `.git/` | `/home/me/proj/.git/objects/ab` | Yes |
| `.git/` | `/home/me/plain/.git`, a regular file | No |
| `.git/` | `/home/me/work/.git`, a link that points to a folder | Yes |
| `.*/` | `/home/me/.ssh/id_rsa` | Yes |
| `.*/` | `/home/me/.zshrc`, a regular file | No |
| `*.pem` | `/home/me/keys/a.pem` | Yes |
| `*.pem` | `/home/me/keys/a.pem.bak` | No |
| `photo?.jpg` | `/home/me/photo10.jpg` | No |
| `~/*.txt` | `/home/me/a.txt` | Yes |
| `~/*.txt` | `/home/me/notes/a.txt` | No |
| `~` | `/home/me/notes/a.txt` | Yes |
| `build/output/` | `/build/output/x/f` | Yes |
| `build/output/` | `/home/me/build/output/f` | No |
| `**/build/output/` | `/home/me/build/output/f` | Yes |
| `~/private` | `/home/me/private/deep/f` | Yes |
| `{secrets,keys}/` | `/home/me/app/secrets/token` | Yes |
| `/{secrets,keys}/` | `/home/me/app/secrets/token` | No |
| `~/Projects/**/secrets/` | `/home/me/Projects/a/b/secrets/token` | Yes |
| `~/Projects/*/secrets/` | `/home/me/Projects/a/b/secrets/token` | No |
| `~/MyDocuments/` | `/home/me/My Documents/reports/x` | Yes |
| `~/MyDocuments/` | `/home/me/MyDocuments`, the link itself | Yes |
| `star[*]` | `/home/me/star*` | Yes |
| `star[*]` | `/home/me/starx` | No |
| `star\*` | `/home/me/star*`, on macOS or Linux | Yes |
| `/photos/Album\[1\]/private/**` | `/photos/Album[1]/private/f`, on macOS or Linux | Yes |
| `C:/Data/**` | `D:\Data\x`, on Windows | No |
| `/Data/**` | `D:\Data\x`, on Windows | Yes |

## Which requests need approval

A delete or move needs approval when it affects a protected item. A request affects:

- its source;
- every folder above the source;
- when the source is a folder, every item inside it, at any depth.

So with the rule `.git/`, deleting a project folder that contains a `.git` folder needs approval, and so does moving it.

A pending request records one rule, chosen by the order of your list:

- the first rule in the list that protects the source;
- when no rule protects the source and the source is a folder, the first rule in the list that protects an item inside it.

With `*.pem` listed before `~/vault/`, deleting `~/vault/key.pem` records `*.pem`. With the two in the other order, it records `~/vault/`.

SafeRM doesn't follow symbolic links when it checks the items inside a folder. Each link is judged as an item under its own name and path. Deleting or moving a link leaves the item it points to in place, so SafeRM doesn't check that item.

### Folders that SafeRM reads

SafeRM reads inside the source only when no rule protects the source itself. It then reads a folder only when some rule could protect an item inside that folder:

- a name rule always could;
- a path rule could only when the path that it spells out is that folder, is inside it, or is above it. That path ends before the rule's first name that holds a `*`, `?`, `[`, `{`, or `\`.

With `~/Contracts/` as the only rule, SafeRM reads nothing inside `~/Downloads/old`. With no rules, SafeRM reads nothing inside any source.

### Folders that SafeRM can't read

SafeRM may be unable to open a folder it has to read. It still checks everything it can open:

- If an item it can see is protected, the request is pending and records that rule, as usual.
- If no item it can see is protected, the request is pending as well, because a protected item may sit inside the folder that couldn't be read. The request records the first rule in the list that could protect an item inside such a folder, and SafeRM prints a warning that names each folder it couldn't read. With `-j`, the warning is an entry in the result's `problems` array; see [Errors and warnings](cli-output.md#errors-and-warnings).

A request never fails because a folder can't be read. A folder that SafeRM has no reason to read never makes a request pending.

## Replacements

When a move replaces an existing item, SafeRM calls that item the occupant. The move creates two actions: the move and a delete of the occupant, which is called the move's blocker. Rules apply to each action separately. The occupant's delete is checked like any other delete, including the occupant's contents. The move's destination is checked only through that delete, with these results:

| Occupant's delete needs approval | Move needs approval | Result |
| --- | --- | --- |
| No | No | The occupant goes to trash, then the source moves. |
| No | Yes | The occupant goes to trash, and the move is pending. |
| Yes | No | The occupant's delete is pending, and the move waits for it. |
| Yes | Yes | Both are pending. |

If you decide the move while its blocker is still pending, SafeRM asks whether to decide both, with no as the default answer. Approving both sends the occupant to trash and then carries out the move, even if the move needed approval of its own. Denying both leaves both items in place.

If you decide the blocker by itself:

- Approving it sends the occupant to trash. A waiting move that needed no approval of its own then runs. A move that did stays pending.
- Denying it denies every move waiting for it. So does a failure while carrying it out.

Denying a move doesn't bring back an occupant that is already in trash. Restore the occupant's delete for that.

A restore that replaces an item creates a delete of the occupant in the same way. Restoring doesn't check the rules against the item being restored, and purge and cleanup don't use the rules.

Revised 2026-11-26
