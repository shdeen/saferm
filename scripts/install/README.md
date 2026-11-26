# Install SafeRM

## Install a GitHub Actions build

Open the completed workflow run while signed in to GitHub. Under **Artifacts**, download the package for your operating system and processor. On Linux, `uname -m` reports `x86_64` for AMD64 or `aarch64` for ARM64.

| Operating system | Package | Installation |
| --- | --- | --- |
| Ubuntu 24.04 or compatible APT system | `.deb` | In the download directory, run `sudo apt install ./FILENAME.deb` with the downloaded filename. |
| Fedora 44 or compatible DNF system | `.rpm` | In the download directory, run `sudo dnf install ./FILENAME.rpm` with the downloaded filename. |
| macOS | `.dmg` | Open the disk image and drag SafeRM to Applications. For the CLI, the executable is inside the app at `/Applications/SafeRM.app/Contents/MacOS/saferm`. |
| Windows | `.exe` | Run the downloaded installer. |

## Install a published release

The commands below become available when SafeRM publishes its first public GitHub release in `shdeen/saferm`. They install the CLI, desktop application, and SafeRM Notify. A manual Actions run makes its packages available through the run's **Artifacts** section.

### macOS and Linux

```sh
curl -fsSL https://github.com/shdeen/saferm/releases/latest/download/install.sh | sh
```

Run this as your normal user. The installer requests administrator access when needed. Close SafeRM before upgrading, then run the same command again.

- **macOS 13 or later:** installs `/Applications/SafeRM.app` and links `/usr/local/bin/saferm` to the CLI inside the app. Supports Intel and Apple Silicon.
- **Ubuntu 24.04 or compatible APT systems:** installs the DEB package and its runtime dependencies. The programs reside in `/usr/lib/saferm`, with the CLI linked at `/usr/bin/saferm` and SafeRM available from the applications menu.
- **Fedora 44 or compatible DNF systems:** installs the RPM package and its runtime dependencies, using the same program locations as the DEB package. Linux packages support AMD64 and ARM64. Other distributions must provide the packages' GTK 4 and WebKitGTK 6 runtime dependencies.

The installer needs `curl` or `wget` for downloads and `sha256sum` or `shasum` for verification. Linux downloads are placed where the package manager can read them, including when your home directory is private.

To select a version:

```sh
curl -fsSL https://github.com/shdeen/saferm/releases/download/v1.2.3/install.sh | SAFERM_VERSION=1.2.3 sh
```

On macOS, `SAFERM_INSTALL_DIR` changes the directory containing `SafeRM.app`; `SAFERM_BIN_DIR` changes the directory containing the CLI link. Both must be absolute paths. Custom CLI directories must be on your `PATH`. Linux uses the package's standard locations.

### Windows

Run in PowerShell 5.1 or later:

```powershell
irm https://github.com/shdeen/saferm/releases/latest/download/install.ps1 | iex
```

The installer selects AMD64 or ARM64, runs the per-user Windows installer, and installs WebView2 if needed. SafeRM appears in the Start menu. Its installation directory is added to your user `PATH` and the current PowerShell session, so `saferm` works by name. The default location is the current user's `AppData\Local\Programs\SafeRM` directory. Upgrades reuse the registered installation directory.

To select a version:

```powershell
$env:SAFERM_VERSION = '1.2.3'
irm https://github.com/shdeen/saferm/releases/download/v1.2.3/install.ps1 | iex
```

Set `SAFERM_INSTALL_DIR` to an absolute local directory to choose the installation location. Close SafeRM before upgrading, then run the installer again.

## After installation

Run `saferm help` or open SafeRM from your applications menu. Opening the GUI registers SafeRM Notify for your user; installation does not start the GUI or enable notifications on its own. macOS may request approval for the login item or notifications.

Both scripts accept `SAFERM_VERSION` with or without the leading `v`. A version has three numeric components, such as `1.2.3`; the default is the latest published release. Packages are checked against the release's SHA-256 checksums before installation. Download or checksum failures leave an existing installation unchanged. Installation and upgrades preserve your SafeRM history, configuration, and retained files.

On Linux, remove the application with your package manager. On Windows, use Installed apps; uninstall also removes the application's directory from your user `PATH`. On macOS, remove the application and its CLI link. Application removal does not purge retained files or history.

Revised 2026-11-24
