# SafeRM invariants

## All commands

- Operational commands return plain text by default and JSON with `-j` or `--json`, including failures. Help remains text. JSON result output contains no plain-text prompts.
- With `--json`, a `delete`, `move`, `dispose`, or `restore` prints the whole result, an object whose `actions` list holds every action, even when it holds one. A warning is an entry in that result's `problems` list. `view` prints one action object, and `list` prints a list of actions.
- Flags work before, between, and after positional arguments. `-j` and `--json` are equivalent for every operational command and follow its command name. Flags are registered only on commands they affect and listed in those commands' help pages.
- Missing required inputs, invalid IDs or filters, extra arguments, and conflicting configuration flags fail through the shared internal API before operational file changes.
- Domain validation and operations run through internal APIs. The CLI only selects operations, translates arguments, handles terminal interaction, and exits.
- Every user-facing message comes from a template or TOML/JSON catalog. Tests resolve presentation expectations from those same sources.
- Paths are resolved before storage. Home and path handling support macOS, Linux, and Windows.
- Ordinary delete and move operations create history after execution. Requests requiring approval are saved before execution. A failed result write returns recovery information and is distinguished from a failed filesystem operation.
- Ordinary commands inspect only records and files needed for the requested operation. Interrupted or ambiguous operations are reported without guessing their outcome.
- Move and restore replacement refuse before effects when the configured trash directory lies inside the directory being transferred. Existing or absent trash and review requirements do not permit relocating the retained occupant or any intermediate symbolic link used to reach it. Refused replacements leave files, history, and notifications unchanged; unrelated outside trash links still permit replacement.
- If a file occupies the configured trash directory or one of its parents, delete, move replacement, and restore replacement refuse before changing files or history. Both replacing items and the blocking file retain their contents. Path resolution rejects an existing file used as a directory parent.

## Help

- `saferm help`, `saferm -h`, and `saferm --help` show root help without configuration or database access. Every subcommand supports `help <command>`, `-h`, and `--help`. Help navigation accepts nested command names.
- Root and help commands reject `-j` and `--json` and omit them from their help pages. Operational command pages show both aliases in the options list; combining either alias with `-h` or `--help` still returns plain-text help.

## Configuration

- Without an active file, configuration comes entirely from embedded TOML. An active file replaces defaults completely; omitted keys remain unset. Malformed or unreadable files cause errors.
- `config` displays every recognized setting, explicit unset states, and the selected source beneath a title and separator. Labels and values align in a 60-column display; long values wrap without truncation, and empty settings remain distinct from omitted settings. Only settings needed for an invoked operation must be usable.
- `config --default` outputs the embedded document without reading active configuration or opening SQLite; JSON returns its structured values.
- The built-in configuration lists no review patterns. With no configuration file, no delete or move waits for approval because of a pattern.
- While the list of review patterns holds a malformed pattern, `saferm config` displays the configuration, writes an error that names the pattern, and exits with status 1.
- `config --setup` creates the active file, preserving an existing file with the specified notice. `--setup --reset` asks the specified confirmation before replacement; a submitted empty response approves, but refusal, cancellation, or EOF preserves the file.

## Delete and move

- `delete <path> <reason>` and `move <source> <destination> <reason>` accept files and directories and optional requester and description metadata.
- Creation stores an action and its operation-specific record together. Deletions have a unique trash identity and historical trash directory; moves have a resolved destination.
- A review pattern that matches a folder protects everything inside it, at any depth, and a request on any of those items needs approval.
- A pattern with no `/` before its last character matches an item of that name in any folder. A pattern with one applies from the filesystem root, and a pattern that is `~` or starts with `~/` applies from the home folder.
- A path pattern that is written through a symbolic link protects the real location. SafeRM follows the links in the folder names that the pattern spells out, up to the first name that holds a `*`, `?`, `[`, `{`, or `\`. A folder whose real name contains `[` is protected under exactly that name when a link leads to it.
- A path pattern in which no name holds one of those characters, and which names a symbolic link, protects the link and the item it points to. Any other pattern that matches a link protects the link alone. A pattern that ends in `/` matches a symbolic link that points to a folder.
- Where a folder ignores letter case for a name, matching ignores it for that name. Where it doesn't, case must match.
- On Windows, a pattern that starts with a drive applies only to that drive. Any other path pattern that is measured from the root applies from the root of the drive that holds the item.
- A delete or move of a folder that contains a protected item needs approval. So does the occupying delete created by move replacement or restore replacement. Links inside the folder are not followed. The action records the first pattern in the list that protects the source. When none does, it records the first pattern in the list that protects an item inside the source.
- When no item SafeRM can see is protected and it couldn't read a folder that some pattern could apply inside, the request is pending. The action records the first pattern in the list that could apply inside that folder, and SafeRM writes a warning that names the folder. A folder that no pattern could apply inside is never read, and an empty pattern counts as no pattern.
- While the list holds a malformed pattern, every delete and move is saved as a pending action. Each one also writes an error that names the pattern and exits with status 1.
- An operation cannot move the active history database or a directory containing it, including through a symbolic home path.
- Ordinary unprotected operations execute immediately and record completion or failure. Moves require an existing destination parent and refuse an occupied destination unless replacement is explicitly requested and consented to; failed transfers retain a recoverable copy.
- Ordinary requests are created as pending only when they require separate approval; otherwise they are recorded with their completed or failed outcome. A replacing move is saved pending with its blocker before its own transfer.
- Consented move replacement preserves the occupant under its own recorded delete, returns the move before its delete with lower ID and immutable blocker ID, and retains metadata. No or EOF changes nothing; force alone cannot permit replacement.
- Source and occupant review apply independently. A completed delete releases an unprotected move but leaves a protected source pending. Only saved actions still pending send notifications, delete first and move second.
- Two moves to one occupied destination create separate deletes. Either approval order preserves the original occupant and the first moved item in separate trash items, with the last moving source at the destination.
- Failure to save the move and delete atomically records neither, exposes no assigned IDs, leaves the moving source untouched, and identifies an already-retained occupant by its trash ID and paths with a database-result error.
- Existing database upgrades preserve records, diagnostics, and IDs; repeated or concurrent opens behave identically. Removed blocker history does not change the move's reference or permit ID reuse. Pending-dependent reads release the database connection before decisions.
- An empty destination permits an ordinary move without trash. Missing trash for replacement and overlapping paths change no files or history. A missing moving source leaves its pending request unchanged and deniable after its occupying delete completes.

## List, view, and dispose

- `list` returns recorded IDs, readable local dates, operation, status, and source, filtered by type and status when requested.
- `view <id>` returns all recorded action information, including metadata, review rule, destination or trash identity and historical root, decision, and failure.
- `dispose <id> approve` executes an unresolved pending operation after rechecking its source and destination. `dispose <id> deny` leaves files untouched. A resolved action cannot execute twice.
- Approving or denying a blocked move first asks about both actions and the occupying path. Explicit approval permits both named actions even with source review; denial closes both. No or EOF changes neither record nor file.
- Direct blocker denial or transfer failure denies dependent moves with decision, resolution time, and an appended diagnostic identifying the blocker. Preliminary refusal leaves both pending and deniable.

## Restore

- `restore <id>` reverses a deletion using only the currently configured trash directory, or a move using its recorded destination. Relocating the trash directory does not require rewriting history.
- An occupied original path is preserved unless `--overwrite` is supplied and confirmed. `--overwrite --force` skips confirmation; force alone never permits replacement.
- Confirmed restore first records a recoverable delete of the occupant, preserving files, directories, and symbolic links. Results put that delete before the restored original. Status becomes restored only after successful restoration.
- No, EOF, or an unfinished confirmation answer changes no files or actions.
- An occupant requiring review creates a pending delete and an identifiable error even with force. The original remains unchanged and unclaimed; approving the delete permits a separate ordinary restore.
- Recording or transfer failures retain recovery locations and partial results. A completed occupying delete remains recoverable if the original claim, transfer, or final recording fails; failures before its claim leave the original unchanged.

## Purge

- `purge <id>...` permanently removes selected deleted items from the current trash directory and marks only successful removals purged. Move destinations cannot be purged.
- Missing items and failed removals are reported accurately. Purged records remain available for the configured history retention period.

## Cleanup

- `cleanup` removes eligible actions and their operation-specific records transactionally, without removing filesystem contents.
- Removing a record never frees its ID: every later action receives an ID greater than every ID issued before it.
- Retention runs from restoration, purge, denial, or failure, or from completion for completed moves. Restoring a move restarts retention.
- Pending review requests and records needed to recover retained files are preserved regardless of age.
- `cleanup --orphans` additionally removes records whose status-specific expected items are missing. Permission errors do not establish absence. Restored and purged records are not orphans merely because their prior trash item is absent.
- Ambiguous interruptions are reported and retained. Maintenance uses the current trash directory and never searches historical trash directories or the filesystem for manually moved items.

## Desktop interface

- The desktop interface executes the same shared operations as the CLI, using the same configuration, history, review rules, and recovery behavior.
- The operations list places pending actions before other actions across all pages. Each group is newest first. Pages default to 20 operations, with 50 and 100 also available; type and status filters use the existing shared API.
- Pending requests can be approved or denied directly from the list. In-progress actions cannot be submitted for another operation through the interface.
- Details expose recorded metadata, paths, review rules, timestamps, decisions, and errors. New delete and move forms preserve required inputs and optional requester/description metadata.
- Restore replacement and configuration reset retain the backend confirmation prompt and path. The interface resubmits with consent only after the user confirms; canceling preserves the files.
- Partial failures retain successful action results and individual diagnostics. A partial purge closes its dialog and refreshes history; a subsequent purge excludes items already purged.
- Cleanup exposes the orphan option. Configuration exposes current values, embedded defaults, setup, and reset. Help describes the available operations.
- Older history responses cannot overwrite newer requests. Refresh failures remain visible. Empty and out-of-range pages stay usable without panics.
- Standard action success produces no floating popup. Operation failures use native dialogs attached to the desktop window.
- The overwrite checkbox in the new-operation form applies only to move requests. Confirmation displays supplied IDs and literal paths without a terminal hint, and only Yes resubmits with consent. Details expose both pending causes; linked decisions and partial errors refresh history. The adapter preserves actions, confirmation metadata, and errors together.

## Desktop notifications and installation

- The packaged GUI includes SafeRM Notify, an optional notifier that runs independently of the GUI. CLI-only installations perform no registration. Disabling SafeRM Notify leaves CLI and GUI operations, including manual decisions, available.
- Creating a pending request sends one local message after history is committed. Notifications use this push; there are no history scans, reminders, or repeated checks.
- Notifications show operation type, filename, and optional requester. Approve and Deny use the shared decision logic. Review opens the requested operation drawer, launching or focusing the GUI as needed.
- First launch of the packaged GUI registers SafeRM Notify for the user. Development launches perform no registration. Subsequent launches preserve the user's decision to disable startup.
- macOS bundles contain SafeRM Notify as an embedded login item. Windows and Linux packages install both programs. Windows upgrades restart SafeRM Notify only when it was running before the upgrade; uninstall waits for its shutdown before removing its files and startup registration.
- Native Approve and Deny on a blocked move ask before deciding either action. Yes uses current records and produces the linked outcome; No and closing preserve pending records and files. Errors remain visible and append to diagnostic history.

## Release packages and download installers

- Permanent: stable `vMAJOR.MINOR.PATCH` tags supply package versions. Malformed tags and components outside Windows' 16-bit version range are rejected before release assembly.
- Permanent: assembly produces both architectures' macOS, Windows, DEB, and RPM packages, both installer scripts, and matching SHA-256 checksums. A signed release requires every signed macOS and Windows input; an absent signed input fails before any release directory is created.
- Permanent: Test/Package retains each final package as an individually downloadable file under its release filename. Each native installer test downloads its own package. The tag release verifies all eight packages and both installer scripts against the checksums from assembly before publishing.
- Permanent: the Unix and PowerShell scripts select the native platform and architecture and install the complete GUI, CLI, and SafeRM Notify from actual release packages. Latest selection, explicit versions with or without `v`, and reinstallation leave a working CLI and matching package metadata.
- Permanent: installation and reinstallation preserve existing SafeRM data. An optional previous-release input exercises an upgrade between real package versions. Bad checksums, unavailable downloads, and malformed requested versions fail without changing installed executables.
- Permanent: macOS bundles retain valid signatures and a CLI link. Linux package installation resolves runtime dependencies and exposes the CLI at `/usr/bin/saferm`; a private invoking home or temporary-directory preference does not cause APT's unsandboxed-download warning.
- Permanent: the Unix installer accepts a PATH entry that reaches the installed CLI through a directory symlink. It warns when PATH selects a different executable.
- Permanent: Windows installs in a directory containing spaces, adds that directory exactly once to user PATH, and exposes the CLI in the invoking PowerShell session. Uninstall removes the programs and the PATH directory while preserving SafeRM data.
- Permanent: preparing application bundles and binary archives for signing creates no DMG, NSIS installer, DEB, or RPM. Packaging later uses those compiled binaries without rebuilding them.
- Permanent: application tests can run without producing or signing distribution packages. Installer tests use newly built packages and include DMG/NSIS installation; enabling installer tests also enables their required packaging.
- Permanent: distribution signing applies to the executables, application bundles, and final macOS/Windows packages. Signed builds produce no separate unsigned installers. Unsigned binary archives are retained only by explicit selection during signed builds; temporary transfers and assembly inputs are deleted after successful assembly. If assembly fails or cleanup cannot finish, those inputs remain available for up to one day so failed jobs can be retried.
- Permanent: Test/Package owns application tests, packaging, signing, and installer tests. Release calls it with all required checks, then publishes its resulting files without rebuilding or duplicating them.

Release assembly isolation checks run through `python3 -B -m unittest discover -s scripts -p release_test.py -v`. Packaging stage checks run through `python3 -B scripts/package-checks.py /absolute/binary/directory /absolute/new/package/directory` with real native executables. Native installer checks run through `python3 -B scripts/install/test-install.py /absolute/release/directory VERSION /absolute/new/test/directory` and Test/Package. Linux and Windows installation checks require disposable runners. Workflow syntax and expressions are checked with actionlint; runner scheduling and artifact cleanup require a GitHub run.
