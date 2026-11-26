#!/usr/bin/env python3
"""Permanent checks: preparing signing inputs creates no installers; packaging reuses binaries.

Pass a directory containing real native build outputs. All mutations stay in a new
work directory. The checks exercise the same Task targets used by hosted builds.
"""
import argparse
import hashlib
from pathlib import Path
import platform
import shutil
import subprocess
import zipfile


def check_packaging(binary_directory, work_directory):
    operating_system = platform.system().lower()
    architecture = {'arm64': 'arm64', 'aarch64': 'arm64', 'x86_64': 'amd64', 'amd64': 'amd64'}[platform.machine().lower()]
    executable_extension = '.exe' if operating_system == 'windows' else ''
    work_directory.mkdir(parents=True, exist_ok=False)
    binary_hashes = {}
    for executable_name in ['saferm', 'saferm-app', 'saferm-notify']:
        executable_filename = executable_name + executable_extension
        shutil.copy2(binary_directory / executable_filename, work_directory / executable_filename)
        binary_hashes[executable_filename] = hashlib.sha256((work_directory / executable_filename).read_bytes()).hexdigest()
    wails_version = subprocess.check_output(['go', 'list', '-m', '-f', '{{.Version}}', 'github.com/wailsapp/wails/v3'], text=True).strip()
    task_command = ['go', 'run', 'github.com/wailsapp/wails/v3/cmd/wails3@' + wails_version, 'task']
    task_variables = [f'BUILD_DIR={work_directory}', 'VERSION=0.1.1']
    subprocess.run(task_command + ['package:prepare'] + task_variables, check=True)
    assert not list(work_directory.glob('*.dmg'))
    assert not list(work_directory.glob('*-setup.exe'))
    assert not list(work_directory.glob('*.zip'))
    assert not list(work_directory.glob('*.deb'))
    assert not list(work_directory.glob('*.rpm'))
    if operating_system == 'darwin':
        import plistlib
        application_bundle = work_directory / 'SafeRM.app'
        for application_path in [application_bundle, application_bundle / 'Contents/Library/LoginItems/SafeRM Notify.app']:
            with (application_path / 'Contents/Info.plist').open('rb') as plist_file:
                assert plistlib.load(plist_file)['CFBundleShortVersionString'] == '0.1.1'
    if operating_system in ['darwin', 'windows']:
        subprocess.run(task_command + ['package:binaries'] + task_variables, check=True)
        archive_path = work_directory / f'saferm-app-{operating_system}-{architecture}.zip'
        with zipfile.ZipFile(archive_path) as binary_archive:
            assert binary_archive.testzip() is None
            assert any(archive_filename.endswith('/saferm') or archive_filename == 'saferm.exe' for archive_filename in binary_archive.namelist())
        assert not list(work_directory.glob('*.dmg'))
        assert not list(work_directory.glob('*-setup.exe'))
    subprocess.run(task_command + ['package:archive'] + task_variables, check=True)
    package_names = {'darwin': ['SafeRM.dmg'], 'windows': [f'saferm-app-windows-{architecture}-setup.exe'],
                     'linux': [f'saferm-app-linux-{architecture}.deb', f'saferm-app-linux-{architecture}.rpm']}[operating_system]
    for package_filename in package_names:
        assert (work_directory / package_filename).stat().st_size > 0, package_filename
    for executable_filename, binary_digest in binary_hashes.items():
        assert hashlib.sha256((work_directory / executable_filename).read_bytes()).hexdigest() == binary_digest
    print('PASS: signing preparation creates no installer; packaging preserves the compiled binaries')


if __name__ == '__main__':
    argument_parser = argparse.ArgumentParser(description=__doc__)
    argument_parser.add_argument('binary_directory', type=Path)
    argument_parser.add_argument('work_directory', type=Path)
    arguments = argument_parser.parse_args()
    check_packaging(arguments.binary_directory.resolve(), arguments.work_directory.resolve())
