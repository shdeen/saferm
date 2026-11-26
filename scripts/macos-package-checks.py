#!/usr/bin/env python3
"""Permanent package checks: signed macOS app and embedded SafeRM Notify identity."""
import plistlib
from pathlib import Path
import subprocess
import sys


def macos_bundle(bundle):
    app = Path(bundle).resolve(strict=True)
    notifier = app / 'Contents/Library/LoginItems/SafeRM Notify.app'
    with (app / 'Contents/Info.plist').open('rb') as document:
        app_info = plistlib.load(document)
    with (notifier / 'Contents/Info.plist').open('rb') as document:
        notifier_info = plistlib.load(document)
    assert app_info['CFBundleIdentifier'] == 'com.shdeen.saferm'
    assert notifier_info['CFBundleIdentifier'] == 'com.shdeen.saferm.notify'
    assert notifier_info['CFBundleName'] == 'SafeRM Notify'
    assert notifier_info['LSUIElement'] is True
    for path, metadata in [(app, app_info), (notifier, notifier_info)]:
        executable = path / 'Contents/MacOS' / metadata['CFBundleExecutable']
        architecture = subprocess.check_output(['lipo', '-archs', str(executable)], text=True).strip()
        assert architecture in ('arm64', 'x86_64'), architecture
        subprocess.run(['codesign', '--verify', '--strict', str(path)], check=True)
    subprocess.run(['codesign', '--verify', '--deep', '--strict', str(app)], check=True)
    print('PASS: signed GUI and embedded SafeRM Notify with matching installation identity')


if __name__ == '__main__':
    macos_bundle(sys.argv[1])
