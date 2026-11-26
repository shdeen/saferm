# Installation

Each SafeRM package contains the CLI, the desktop app, and the notifier.

## macOS and Linux

```sh
curl -fsSL https://github.com/shdeen/saferm/releases/latest/download/install.sh | sh
```

Run the script as your normal user. It asks for administrator access when it needs it. It requires `curl` or `wget`, and `sha256sum` or `shasum`.

| System | Result |
| --- | --- |
| macOS 13 or later, Intel or Apple Silicon | `/Applications/SafeRM.app`, with the CLI linked at `/usr/local/bin/saferm`. |
| Ubuntu 24.04 or a compatible APT system, AMD64 or ARM64 | DEB package. Programs in `/usr/lib/saferm`, with the CLI linked at `/usr/bin/saferm`. |
| Fedora 44 or a compatible DNF system, AMD64 or ARM64 | RPM package, with the same locations as the DEB package. |

Other Linux distributions must provide GTK 4 and WebKitGTK 6.

## Windows

In PowerShell 5.1 or later:

```powershell
irm https://github.com/shdeen/saferm/releases/latest/download/install.ps1 | iex
```

SafeRM installs for the current user, by default in the `AppData\Local\Programs\SafeRM` directory of your profile. That directory is added to your user `PATH`, including in the current PowerShell session. The script installs WebView2 if it is missing.

## Installer variables

| Variable | Platform | Effect |
| --- | --- | --- |
| `SAFERM_VERSION` | All | Install the given release instead of the latest. The version has three numbers and may start with `v`. |
| `SAFERM_INSTALL_DIR` | macOS, Windows | Absolute path of the directory for `SafeRM.app` on macOS, or for the installation on Windows. |
| `SAFERM_BIN_DIR` | macOS | Absolute path of the directory for the `saferm` link. The directory must be on your `PATH`. |

On Linux, the packages always use the locations above.

```sh
curl -fsSL https://github.com/shdeen/saferm/releases/latest/download/install.sh | SAFERM_VERSION=<version> sh
```

```powershell
$env:SAFERM_VERSION = '<version>'
irm https://github.com/shdeen/saferm/releases/latest/download/install.ps1 | iex
```

## Upgrade

Close SafeRM and run the install command again. Your configuration, history, and trash are kept. If the download or checksum check fails, the installed version stays in place. On Windows, an upgrade keeps the existing installation directory unless `SAFERM_INSTALL_DIR` names another one.

## Install a package yourself

The [Releases page](https://github.com/shdeen/saferm/releases) lists each release's packages by operating system and processor. A completed GitHub Actions run lists its packages under **Artifacts**, which requires signing in to GitHub. On Linux, `uname -m` reports `x86_64` for AMD64 and `aarch64` for ARM64.

| System | Package | Install it with |
| --- | --- | --- |
| Ubuntu 24.04 or a compatible APT system | `.deb` | `sudo apt install ./<file>.deb` |
| Fedora 44 or a compatible DNF system | `.rpm` | `sudo dnf install ./<file>.rpm` |
| macOS | `.dmg` | Drag SafeRM to Applications. The CLI is at `/Applications/SafeRM.app/Contents/MacOS/saferm`; link it onto your `PATH` yourself. |
| Windows | `.exe` | Run the installer. |

## After installation

Open SafeRM once from your applications menu. Its first launch sets the notifier to start when you log in. On macOS, approve the login item and notifications in System Settings if asked. See [Notifications](notifications.md).

## Uninstall

- **Linux:** `sudo apt remove saferm-app` or `sudo dnf remove saferm-app`.
- **Windows:** remove SafeRM in **Installed apps**. This also removes its directory from your user `PATH`.
- **macOS:** delete `/Applications/SafeRM.app` and the `saferm` link.

Uninstalling leaves `~/.saferm` in place, with your configuration, history, and the default trash directory.

Revised 2026-11-24
