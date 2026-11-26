# Installation

Each SafeRM package contains the CLI, the desktop app, and the notifier.

## macOS and Linux

**Requires**: `curl` or `wget`, and `sha256sum` or `shasum`.

```sh
curl -fsSL https://github.com/shdeen/saferm/releases/latest/download/install.sh | sh
```

Run the script as your regular user. It asks for administrator access when it needs it.

| System                                | Installation                                                                               |
| ------------------------------------- | ------------------------------------------------------------------------------------------ |
| macOS 13 or later                     | `/Applications/SafeRM.app`, with the CLI linked at `/usr/local/bin/saferm`.                |
| Ubuntu 24.04 or compatible APT system | DEB package. Programs go in `/usr/lib/saferm`, and the CLI is linked at `/usr/bin/saferm`. |
| Fedora 44 or compatible DNF system    | RPM package, same locations as DEB.                                                        |

Other Linux distributions must provide GTK 4 and WebKitGTK 6.

On Linux, the script doesn't add an APT repository or an RPM signing key.

## Windows

In PowerShell 5.1 or later:

```powershell
irm https://github.com/shdeen/saferm/releases/latest/download/install.ps1 | iex
```

SafeRM installs for the current user, by default in `AppData\Local\Programs\SafeRM` in your profile. That directory is added to your user `PATH`, including in the current PowerShell session. The script installs WebView2 if it is missing.

## Choose a version or install location

The script uses the defaults below. To change one, set its environment variable before you run the install command.

| Variable             | Platforms      | Default                                                                                                         |
| -------------------- | -------------- | --------------------------------------------------------------------------------------------------------------- |
| `SAFERM_VERSION`     | All            | Latest release                                                                                                  |
| `SAFERM_INSTALL_DIR` | macOS, Windows | `/Applications` on macOS. On Windows, `AppData\Local\Programs\SafeRM`, or an existing installation's directory. |
| `SAFERM_BIN_DIR`     | macOS          | `/usr/local/bin`                                                                                                |

Paths must be absolute. On macOS, if both directories already exist and are writable, the script doesn't ask for administrator access.

## Install a package yourself

Download the package for your operating system and processor from the [Releases page](https://github.com/shdeen/saferm/releases). (GitHub sign-in required.)

| System                                  | Package | Install it with                                                                                                                 |
| --------------------------------------- | ------- | ------------------------------------------------------------------------------------------------------------------------------- |
| macOS                                   | `.dmg`  | Drag SafeRM to Applications. To run `saferm` by name, link `/Applications/SafeRM.app/Contents/MacOS/saferm` into a directory on your `PATH`. |
| Windows                                 | `.exe`  | Run the installer.                                                                                                              |
| Ubuntu 24.04 or a compatible APT system | `.deb`  | `sudo apt install ./<file>.deb`                                                                                                 |
| Fedora 44 or a compatible DNF system    | `.rpm`  | `sudo dnf install ./<file>.rpm`                                                                                                 |

## Install the CLI only

The CLI works without the desktop app and the notifier. To install only the CLI, [install Go](https://go.dev/dl/) 1.27 or later, then run:

```sh
go install github.com/shdeen/saferm/cmd/saferm@latest
```

Go installs `saferm` in its `bin` directory, usually `~/go/bin`. That directory must be on your `PATH`.

Without the notifier, check for pending requests with `saferm list --status pending`.

## After installation

Open SafeRM once from the applications menu. On its first launch, SafeRM sets the notifier to start when you log in. On macOS, approve the login item and notifications in System Settings if asked. See [Notifications](notifications.md).

## Upgrade

Close SafeRM and run the install command again. Your configuration, history, and trash are kept. If the download or checksum check fails, the installed version stays in place. On Windows, an upgrade keeps the existing installation directory unless `SAFERM_INSTALL_DIR` names another one.

## Uninstall

- **macOS:** delete `/Applications/SafeRM.app` and the `saferm` link.
- **Windows:** remove SafeRM in **Installed apps**. This also removes its directory from your user `PATH`.
- **Linux:** `sudo apt remove saferm-app` or `sudo dnf remove saferm-app`.
- **CLI installed with Go:** delete `saferm` from the directory Go put it in.

Uninstalling leaves `~/.saferm` in place, with your configuration, history, and the default trash directory.

Revised 2026-11-26
