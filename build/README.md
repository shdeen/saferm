# Build the desktop app

Build on the operating system and architecture that will run the app. Install Go 1.27, Node.js 24, and the [Wails platform dependencies](https://wails.io/quick-start/installation/). macOS needs Xcode command-line tools and macOS 13 or later. Windows packaging needs NSIS (`makensis`) on `PATH`; the installer supplies WebView2 when it is missing. Ubuntu 24.04 needs:

```sh
sudo apt-get install build-essential pkg-config libgtk-4-dev libwebkitgtk-6.0-dev
```

From the repository root, run:

```sh
go run github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0 task test
go run github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0 task package
```

The CLI version in these commands matches the Wails dependency. Binding generation and the development server resolve their CLI version from the module. `task dev` installs that version in `/Users/shulemdeen/Docs Home/Coding/saferm/bin/tools` before starting the watcher, independently of `BUILD_DIR`. Add `RACE=true` to `task test` for race detection; Windows ARM64 does not support it. Windows tests that create symlinks require Developer Mode or an administrator terminal. Windows AMD64 race tests also require MinGW-w64 GCC on `PATH`.

`task build` builds the GUI, SafeRM Notify, and CLI without running the tests; add `CHECK_FRONTEND=true` to include frontend tests and formatting checks. `task package` runs the frontend tests, formatting check, TypeScript check, and production frontend build before packaging. Set `BUILD_DIR` to change the output directory. The default output directory is `/Users/shulemdeen/Docs Home/Coding/saferm/bin` in this checkout. Set `VERSION=1.2.3` to set package versions; the local default is `0.1.0`.

| Platform | Installer | Contents |
| --- | --- | --- |
| macOS | `SafeRM.dmg` | `SafeRM.app` with SafeRM Notify embedded as a login item, signed ad hoc for local use |
| Windows | `saferm-app-windows-ARCH-setup.exe` | GUI, SafeRM Notify, and CLI installed for the current user, with the CLI directory on user PATH |
| Linux | `saferm-app-linux-ARCH.deb` and `.rpm` | GUI, SafeRM Notify, CLI, and desktop launcher; `/usr/bin/saferm` links to the bundled CLI |

`ARCH` is `amd64` or `arm64`. Linux packages declare the GTK and WebKit runtime dependencies. Packaging has separate tasks: `package:prepare` assembles the macOS bundle, `package:binaries` creates macOS/Windows ZIP archives when needed for signing, and `package:archive` creates native installers from the compiled executables. None of these three tasks rebuilds the executables; `task package` runs the build, preparation, and native installer tasks together.

## SafeRM Notify

The first launch of the packaged GUI registers SafeRM Notify for the current user. macOS uses an embedded login item, Windows uses the user's startup registration, and Linux uses desktop autostart. Later GUI launches preserve the user's choice to disable SafeRM Notify. CLI-only installations do not register or start SafeRM Notify.

SafeRM Notify receives pushed requests requiring approval, displays native notifications, handles Approve and Deny through the shared operation logic, and opens the GUI's operation drawer for Review. It does not evaluate rules, scan history, repeat alerts, or run ordinary operations as a service. Disabling it leaves CLI and GUI operations, including manual approval and denial, available.

Approve or Deny on a blocked move opens a native question naming the move and its occupying delete. Nothing changes until Yes. No or closing leaves both pending. After the pair is saved, a pending occupying delete sends a notification; the move sends one only if it remains pending when the command finishes. Immediately completed actions send none. Approving a delete directly executes an unprotected move, while a protected move still needs its own approval. The combined question gives consent for both named actions.

Use the operating system's login-item or startup-app settings to disable SafeRM Notify. Notification permission is managed separately in system notification settings. Linux notification buttons require a desktop notification service that supports actions.

## Run the GitHub workflow

Run **Test and package SafeRM** manually from GitHub Actions. It accepts these switches and a package version:

| Option | Default | Effect |
| --- | --- | --- |
| Run application tests | On | Runs Go tests and frontend tests/checks on all six native targets, plus release assembly tests. |
| Run install script and native installer tests | On | Builds packages and tests their installation, including DMG, NSIS, DEB, and RPM. |
| Build distribution packages | On | Creates native packages even when installer tests are off. |
| Sign macOS and Windows binaries and packages; notarize macOS | On | Signs the executables and final packages, with macOS notarization. Applies only when packages are built. |
| Keep unsigned binaries in addition to signed packages | Off | Retains the macOS/Windows binary ZIPs used for signing for 14 days. Applies only when signing runs. |

For application tests alone, turn off both packaging and installer tests. Installer tests require fresh packages, so selecting them also enables packaging. Packaging alone can run with both test options off; frontend compilation and TypeScript checks still run because they are required to build the app. Every selected Go test runs, including hardening tests and fuzz seeds; skipped Go tests fail the job. Race detection runs on all targets except Windows ARM64. Manual runs create no tags or releases.

Signing starts after every native build passes. macOS signs the CLI, embedded SafeRM Notify, GUI, and application bundle, then creates, signs, notarizes, and staples the DMG. Windows signs all three executables, then creates and signs the NSIS installer. A signed run creates no separate unsigned installers. With signing off, neither platform uses distribution certificates or notarization; macOS retains local ad hoc signatures so development bundles run. Windows signing uses an x64 runner because Azure Artifact Signing does not support ARM runners, and verifies every executable and installer signature and timestamp.

The default packaged run lists eight individual downloads under **Artifacts**. The DEB is tested on Ubuntu 24.04 and the RPM on Fedora 44. Each comes in AMD64 (Intel/AMD x86-64) and ARM64 (AArch64). macOS uses the DMG and Windows uses the EXE. The run summary links to installation instructions and lists SHA-256 checksums.

The workflow deletes temporary transfers after it uploads all eight packages. If assembly or cleanup fails, those transfers expire after one day. Explicitly retained binary ZIPs are named `saferm-binaries-PLATFORM-ARCH`. Go test results, installer logs, and notarization logs are uploaded only on failure; ordinary output remains in the job logs.

The `release` environment needs these secrets: `APPLE_ID`, `APPLE_TEAM_ID`, `APPLE_APP_SPECIFIC_PASSWORD`, `MACOS_CERTIFICATE_P12_BASE64`, `MACOS_CERTIFICATE_PASSWORD`, `AZURE_CLIENT_ID`, and `AZURE_TENANT_ID`. Repository secrets are also available to these jobs. Set `AZURE_SIGNING_ENDPOINT`, `AZURE_SIGNING_ACCOUNT`, and `AZURE_CERTIFICATE_PROFILE` as variables.

Azure's federated credential must trust issuer `https://token.actions.githubusercontent.com`, audience `api://AzureADTokenExchange`, and the actual repository's OIDC subject for its `release` environment. Configure the signing secrets, variables, and federated credential for `shdeen/saferm` when creating the public repository; the development repository's credentials and repository identity are not transferred by copying the workflow.

Grant the application the Artifact Signing Certificate Profile Signer role for the selected profile. Certificate material and the temporary keychain remain in the disposable hosted runner's temporary directory.

## Release tags and installation tests

Pushing a tag such as `v1.2.3` starts **Release SafeRM**. Tags must contain three numeric version components, without leading zeroes or a prerelease suffix. Each component is limited to 65535 by Windows package metadata. The tag supplies the versions inside the macOS bundles, DEB/RPM packages, and Windows installer registration.

The release workflow calls **Test and package SafeRM** with all tests, packaging, and signing enabled. That workflow builds, signs, assembles versioned files and SHA-256 checksums, and tests installation on all six native targets. Linux RPM installation also runs in Fedora 44 containers on both architectures. Release publishes those tested files without rebuilding them. Only a successful tag run in `shdeen/saferm` publishes the GitHub release. Manual **Release SafeRM** runs exercise the same sequence without publishing; `sign_artifacts=false` permits testing without signing credentials.

Each release contains `saferm_VERSION_darwin_ARCH.dmg`, `saferm_VERSION_linux_ARCH.deb`, `saferm_VERSION_linux_ARCH.rpm`, and `saferm_VERSION_windows_ARCH.exe`, plus both download scripts and `saferm_VERSION_checksums.txt`. `ARCH` is `amd64` or `arm64`. Release assets have no `unsigned` prefix. Linux package installation uses HTTPS downloads and release checksums; it does not configure an APT repository or RPM signing key.

Installation checks use actual release packages served over local HTTP. They exercise latest and explicit versions, reinstallation, CLI execution, package metadata, data preservation, and refusal of corrupt or unavailable downloads. macOS mounts the DMG, installs its application, and checks bundle signatures. Windows runs the NSIS installer and checks PATH updates and uninstall. Linux checks runtime library availability and APT access to the downloaded file. These checks do not automate interaction with GUI windows.

To run the checks locally on macOS with built release files:

```sh
python3 -B scripts/release.py version v1.2.3
python3 -B -m unittest discover -s scripts -p release_test.py -v
python3 -B scripts/install/generate.py --check
python3 -B scripts/install/test-install.py /absolute/release/directory 1.2.3 /absolute/new/test/directory
```

To check that preparing signing inputs creates no installers and that packaging preserves compiled binaries, run `python3 -B scripts/package-checks.py /absolute/binary/directory /absolute/new/package/directory` on the native build platform. This creates test packages from real executables without installing them.

Add `--previous-release-dir /absolute/previous/release --previous-version 1.2.2` to test an upgrade from another real package. Linux and Windows installation checks modify installed programs and are intended for disposable runners. Installer templates and wording are separate; after editing either, regenerate the standalone download scripts with `python3 scripts/install/generate.py`.

Revised 2026-11-24
