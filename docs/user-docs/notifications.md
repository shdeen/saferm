# Notifications

SafeRM Notify (`saferm-notify`) is installed with the desktop app. It runs in the background and shows each pending request as a desktop notification that you can act on without opening the app. Pending requests don't depend on it: they stay in the history, where `saferm list --status pending` and the app's **Operations** page show them.

## Start at login

The first launch of the desktop app sets the notifier to start when you log in. On Linux and Windows, it also starts the notifier right away.

| Platform | Login entry |
| --- | --- |
| macOS | A login item named **SafeRM Notify**, inside the SafeRM app bundle. You may need to approve it in System Settings. |
| Linux | An autostart entry named **SafeRM Notify** in your autostart folder, usually `~/.config/autostart`. |
| Windows | A startup entry for your user account. |

The app creates the login entry only once. If you later disable the entry, the app doesn't turn it back on.

On its first start, the notifier asks for permission to show notifications. If you refuse, it keeps receiving requests, and you can allow notifications later in your system settings without restarting it.

## When a notification appears

A notification appears whenever SafeRM records a pending request: a delete or move that needs approval under a review rule, or the pending delete of an occupant created by a replacing move or restore. A request that was decided before the notification could appear is skipped. Actions that complete at once don't produce notifications.

The title names the operation and the item, such as `Delete: id_rsa`. If the request named a requester, the text shows it.

| Action | Effect |
| --- | --- |
| **Approve** | Carries out the request. |
| **Deny** | Refuses the request. |
| **Review** | Opens the request's details in the desktop app. |

Clicking the notification also opens the details. For a move whose blocker is still pending, **Approve** and **Deny** first ask whether to decide both, with **No** as the default; see [Replacements](review.md#replacements). A failed decision opens an error dialog.

## Stop or disable the notifier

```sh
saferm-notify --quit
```

`--quit` stops the running notifier. The installer puts only `saferm` on your `PATH`, so you may need the full path to `saferm-notify`. If no notifier is running, the command exits with status 2. On Windows, if the notifier hasn't stopped after ten seconds, the command exits with status 1.

Running `saferm-notify` without options starts the notifier if it isn't already running.

Stopping the notifier doesn't stop it from starting at your next login. To prevent that, disable **SafeRM Notify** in your system's login or startup settings.

## If notifications don't appear

Check that the notifier is running and that your system allows its notifications. If the notifier isn't running, requests are still saved, without a notification. If it is running but can't be reached, the command that created the request prints a `Notification unavailable` warning to standard error, and the request is still saved.

The desktop app and the notifier append errors to `~/.saferm/notifications.log`, with the process ID and a UTC timestamp on each line. SafeRM never shortens or removes this file. If it can't write the file, errors go to standard error instead.

Revised 2026-11-25
