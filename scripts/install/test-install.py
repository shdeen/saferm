#!/usr/bin/env python3
"""Permanent native installer checks using actual packaged SafeRM applications.

Invariants tested (permanent): downloaded packages install all three executables;
the CLI runs and is reachable by name; reinstall preserves user data; invalid
checksums and unavailable downloads leave the installation unchanged; APT can
read a download even when the invoking user's home is private. Only the two
GitHub URLs in the installer copy are redirected to the local HTTP server.
Linux and Windows runs change installed programs and require disposable runners.
"""
import argparse
import functools
import hashlib
import http.server
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import threading


def checksum(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def run_installer(installer_script, environment, system, output_path):
    if system == 'windows':
        script_text = installer_script.read_text()
        command_text = "$ErrorActionPreference = 'Stop'\ntry {\n" + script_text + "\n(Get-Command saferm -CommandType Application).Source\n} catch { Write-Error $_; exit 1 }"
        installer_command = [environment.get('SAFERM_TEST_POWERSHELL', 'pwsh'), '-NoProfile', '-NonInteractive', '-Command', command_text]
        installer_result = subprocess.run(installer_command, env=environment, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    else:
        # A pipe is the public invocation: package-manager prompts must not consume script input.
        installer_result = subprocess.run(['sh'], input=installer_script.read_text(), env=environment, text=True,
                                stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    output_path.write_text(installer_result.stdout)
    print(installer_result.stdout, flush=True)
    return installer_result


def verify_installation(system, environment, version, installer_output):
    if system == 'darwin':
        application_bundle = Path(environment['SAFERM_INSTALL_DIR']) / 'SafeRM.app'
        cli_path = application_bundle / 'Contents/MacOS/saferm'
        executables = [cli_path, application_bundle / 'Contents/MacOS/saferm-app', application_bundle / 'Contents/Library/LoginItems/SafeRM Notify.app/Contents/MacOS/saferm-notify']
        assert (Path(environment['SAFERM_BIN_DIR']) / 'saferm').resolve() == cli_path.resolve()
        subprocess.run(['codesign', '--verify', '--deep', '--strict', str(application_bundle)], check=True)
        import plistlib
        with (application_bundle / 'Contents/Info.plist').open('rb') as plist_file:
            assert plistlib.load(plist_file)['CFBundleShortVersionString'] == version
        with (application_bundle / 'Contents/Library/LoginItems/SafeRM Notify.app/Contents/Info.plist').open('rb') as plist_file:
            assert plistlib.load(plist_file)['CFBundleShortVersionString'] == version
    elif system == 'windows':
        install_dir = Path(environment['SAFERM_INSTALL_DIR'])
        cli_path = install_dir / 'saferm.exe'
        executables = [install_dir / name for name in ['saferm.exe', 'saferm-app.exe', 'saferm-notify.exe', 'Uninstall.exe']]
        import winreg
        with winreg.OpenKey(winreg.HKEY_CURRENT_USER, r'Software\Microsoft\Windows\CurrentVersion\Uninstall\SafeRM', 0, winreg.KEY_READ | winreg.KEY_WOW64_64KEY) as registry_key:
            assert winreg.QueryValueEx(registry_key, 'DisplayVersion')[0] == version
        with winreg.OpenKey(winreg.HKEY_CURRENT_USER, 'Environment') as registry_key:
            user_path = winreg.QueryValueEx(registry_key, 'Path')[0]
        assert [directory.lower().rstrip('\\') for directory in user_path.split(';')].count(str(install_dir).lower().rstrip('\\')) == 1
    else:
        cli_path = Path('/usr/bin/saferm')
        executables = [Path('/usr/lib/saferm') / name for name in ['saferm', 'saferm-app', 'saferm-notify']]
        assert cli_path.resolve() == Path('/usr/lib/saferm/saferm')
        if shutil.which('dpkg-query'):
            installed_version = subprocess.check_output(['dpkg-query', '-W', '-f=${Version}', 'saferm-app'], text=True)
        else:
            installed_version = subprocess.check_output(['rpm', '-q', '--qf', '%{VERSION}', 'saferm-app'], text=True)
        assert installed_version == version, installed_version
        for executable in executables[1:]:
            library_report = subprocess.check_output(['ldd', str(executable)], text=True, stderr=subprocess.STDOUT)
            assert 'not found' not in library_report, library_report
    for executable in executables:
        assert executable.is_file(), executable
    subprocess.run([str(cli_path), 'help'], env=environment, check=True, stdout=subprocess.DEVNULL)
    if system != 'windows':
        shell_cli = shutil.which('saferm', path=environment['PATH'])
        installed_cli = Path(environment.get('SAFERM_BIN_DIR', '/usr/bin')) / 'saferm'
        assert shell_cli and installed_cli.samefile(shell_cli), (shell_cli, installed_cli)
        shadow_warning = json.loads(Path(__file__).with_name('copy.json').read_text())['shell']['shadowed_path']
        assert (shadow_warning % (shell_cli, installed_cli)) not in installer_output, installer_output
    return {str(executable): checksum(executable) for executable in executables}


def run_cases(release_dir, version, work_dir, previous_release_dir=None, previous_version=None):
    system = platform.system().lower()
    server_dir = work_dir / 'server'
    download_dir = server_dir / 'download' / ('v' + version)
    download_dir.mkdir(parents=True)
    for release_file in release_dir.iterdir():
        if release_file.is_file():
            shutil.copy2(release_file, download_dir / release_file.name)
    if previous_release_dir:
        previous_download_dir = server_dir / 'download' / ('v' + previous_version)
        previous_download_dir.mkdir()
        for release_file in previous_release_dir.iterdir():
            if release_file.is_file():
                shutil.copy2(release_file, previous_download_dir / release_file.name)
    (server_dir / 'latest.json').write_text(json.dumps({'tag_name': 'v' + version}))
    request_handler = functools.partial(http.server.SimpleHTTPRequestHandler, directory=str(server_dir))
    http_server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), request_handler)
    server_thread = threading.Thread(target=http_server.serve_forever, daemon=True)
    server_thread.start()
    server_url = f'http://127.0.0.1:{http_server.server_port}'
    script_extension = 'ps1' if system == 'windows' else 'sh'
    installer_script = work_dir / ('install.' + script_extension)
    installer_script.write_text((Path(__file__).parent / installer_script.name).read_text().replace(
        'https://api.github.com/repos/shdeen/saferm/releases/latest', server_url + '/latest.json').replace(
        'https://github.com/shdeen/saferm/releases', server_url))
    environment = os.environ.copy()
    for name in ['SAFERM_VERSION', 'SAFERM_INSTALL_DIR', 'SAFERM_BIN_DIR']:
        environment.pop(name, None)
    isolated_home = work_dir / 'home'
    isolated_home.mkdir(mode=0o700)
    data_home = Path(os.environ['USERPROFILE']) if system == 'windows' else isolated_home
    preservation_file = data_home / '.saferm' / 'installer-preservation-check'
    preservation_file.parent.mkdir(exist_ok=True)
    assert not preservation_file.exists(), 'Refusing to overwrite an existing test marker'
    preservation_file.write_text('Existing SafeRM data must survive installation.')
    environment['HOME'] = str(isolated_home)
    environment['TMPDIR'] = str(isolated_home)  # A private temp parent must not cause APT's warning.
    if system == 'darwin':
        applications = work_dir / 'Applications'
        binaries = work_dir / 'bin'
        applications.mkdir()
        binaries.mkdir()
        # Exercise shells that find the installed CLI through a directory symlink, as on Fedora.
        bin_alias = work_dir / 'sbin'
        bin_alias.symlink_to(binaries, target_is_directory=True)
        environment['SAFERM_INSTALL_DIR'] = str(applications)
        environment['SAFERM_BIN_DIR'] = str(binaries)
        environment['PATH'] = str(bin_alias) + os.pathsep + environment['PATH']
    elif system == 'windows':
        environment['SAFERM_INSTALL_DIR'] = str(work_dir / 'SafeRM Programs')
    try:
        if previous_release_dir:
            environment['SAFERM_VERSION'] = previous_version
            installer_result = run_installer(installer_script, environment, system, work_dir / 'previous.log')
            assert installer_result.returncode == 0, f'Previous version: installer exit {installer_result.returncode}'
            verify_installation(system, environment, previous_version, installer_result.stdout)
            environment.pop('SAFERM_VERSION')
        for case_name, selected_version in [('latest', None), ('reinstall', 'v' + version), ('explicit', version)]:
            if selected_version:
                environment['SAFERM_VERSION'] = selected_version
            installer_result = run_installer(installer_script, environment, system, work_dir / (case_name + '.log'))
            assert installer_result.returncode == 0, f'{case_name}: installer exit {installer_result.returncode}'
            assert 'unsandboxed' not in installer_result.stdout.lower(), installer_result.stdout
            installed_hashes = verify_installation(system, environment, version, installer_result.stdout)
            assert preservation_file.read_text() == 'Existing SafeRM data must survive installation.'
            print('PASS', case_name, flush=True)

        checksums_path = download_dir / f'saferm_{version}_checksums.txt'
        valid_checksums = checksums_path.read_text()
        checksums_path.write_text('\n'.join('0' * 64 + '  ' + record.split('  ')[1] for record in valid_checksums.splitlines()) + '\n')
        installer_result = run_installer(installer_script, environment, system, work_dir / 'checksum.log')
        assert installer_result.returncode != 0, 'Bad checksum was accepted'
        assert installed_hashes == verify_installation(system, environment, version, installer_result.stdout)
        print('PASS checksum failure preserves installation', flush=True)

        checksums_path.write_text(valid_checksums)
        environment['SAFERM_VERSION'] = '999999.0.0'
        installer_result = run_installer(installer_script, environment, system, work_dir / 'unavailable.log')
        assert installer_result.returncode != 0, 'Missing release was accepted'
        assert installed_hashes == verify_installation(system, environment, version, installer_result.stdout)
        print('PASS unavailable release preserves installation', flush=True)

        environment['SAFERM_VERSION'] = '../invalid-version'
        installer_result = run_installer(installer_script, environment, system, work_dir / 'invalid-version.log')
        assert installer_result.returncode != 0, 'Invalid version was accepted'
        assert installed_hashes == verify_installation(system, environment, version, installer_result.stdout)
        print('PASS invalid version preserves installation', flush=True)

        if system == 'windows':
            install_dir = Path(environment['SAFERM_INSTALL_DIR'])
            # _?= prevents the NSIS uninstaller from detaching; wait for actual completion.
            installer_result = subprocess.run(f'"{install_dir / "Uninstall.exe"}" /S _?={install_dir}', env=environment)
            assert installer_result.returncode == 0, 'Uninstall failed'
            assert not (install_dir / 'saferm.exe').exists()
            import winreg
            with winreg.OpenKey(winreg.HKEY_CURRENT_USER, 'Environment') as registry_key:
                user_path = winreg.QueryValueEx(registry_key, 'Path')[0]
            assert str(install_dir).lower() not in [directory.lower() for directory in user_path.split(';')]
            assert preservation_file.read_text() == 'Existing SafeRM data must survive installation.'
            print('PASS uninstall removes programs and PATH, preserving user data', flush=True)
    finally:
        http_server.shutdown()
        http_server.server_close()
        server_thread.join()


if __name__ == '__main__':
    argument_parser = argparse.ArgumentParser(description=__doc__)
    argument_parser.add_argument('release_dir', type=Path)
    argument_parser.add_argument('version')
    argument_parser.add_argument('work_dir', type=Path)
    argument_parser.add_argument('--previous-release-dir', type=Path)
    argument_parser.add_argument('--previous-version')
    arguments = argument_parser.parse_args()
    arguments.work_dir.mkdir(parents=True, exist_ok=False)
    if bool(arguments.previous_release_dir) != bool(arguments.previous_version):
        argument_parser.error('Previous release directory and version must be supplied together')
    run_cases(arguments.release_dir.resolve(), arguments.version, arguments.work_dir.resolve(), arguments.previous_release_dir, arguments.previous_version)
