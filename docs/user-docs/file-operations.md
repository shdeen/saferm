# How deletes and moves work

SafeRM records every delete and move as an action, with a numeric ID, the reason given, and the paths involved. `saferm help delete` and `saferm help move` give the command syntax.

## Delete

A delete moves the item into the trash directory under a unique name, so items with the same original name never collide. The action records the original path and the trashed item's ID. SafeRM creates the trash directory on the first delete.

SafeRM refuses to delete its trash directory, its history database, or a folder that contains either one.

## Move

A move records its source and destination, so `saferm restore` can move the item back. The destination is the item's full new path, including its name. The destination's parent folder must exist. The source and destination must differ, and neither may contain the other. SafeRM refuses to move its history database or a folder that contains it.

An ordinary move doesn't use the trash.

## Moves onto an existing item

Moving onto an existing item takes `--overwrite` in the CLI, or **Allow replacement at the destination** in the desktop app. SafeRM deletes the occupying item to trash as a separate action, then moves the source into place. Nothing is overwritten. Without the option, the move stops with `destination already exists`.

The CLI asks before replacing, with no as the default answer. `-f` (`--force`) skips that question, and has no effect without `--overwrite`. The desktop app always asks.

The occupant's delete is the move's **blocker**. It carries the move's reason, requester, and description. The move gets the lower ID and is listed first in the result. Review rules apply to the occupant and the source separately; see [Replacements](review.md#replacements). If the blocker is denied or fails, SafeRM denies the move.

The two actions do not succeed or fail together. The occupant can already be in trash while the move is pending, denied, or failed. Check both actions before retrying, and see [Restore after a replacement](recovery.md#restore-after-a-replacement) to undo one.

SafeRM refuses a replacement that would move the trash directory, including through a symbolic link inside the source.

## Paths

- A relative path is resolved against the working directory of the program that runs the operation.
- A leading `~` stands for the home directory.
- Symbolic links in parent folders are resolved. A symbolic link at the end of the path is not followed: SafeRM deletes or moves the link itself. A broken link counts as an existing item.
- History stores the resolved absolute path. For how review rules are checked against an item, see [Reviewing requests](review.md#write-review-rules).

## Moves between filesystems

Within one filesystem, SafeRM renames the item in one step. Across filesystems, it copies the item and removes the source only after the copy is complete and written to disk. If the copy fails, the source stays, but a partial copy may remain at the destination or in trash.

A copy between filesystems handles regular files, folders, and symbolic links. It refuses special files such as named pipes and devices. It keeps permissions, as narrowed by your `umask`, but not timestamps or ownership.

Ctrl-C stops a copy in progress, and SafeRM still records the outcome. A crash or power loss during an operation can leave the action marked in progress; see [Interrupted or ambiguous operations](troubleshooting.md#interrupted-or-ambiguous-operations).

Revised 2026-11-26
