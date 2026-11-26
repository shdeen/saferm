"""Assemble versioned release assets from the native packaging jobs."""
import argparse
import hashlib
from pathlib import Path
import re
import shutil


def release_version(tag):
    if not re.fullmatch(r'v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)', tag):
        raise ValueError(f'Expected a release tag such as v1.2.3; received {tag!r}')
    version = tag[1:]
    # Windows installer version fields are unsigned 16-bit integers.
    if any(int(component) > 65535 for component in version.split('.')):
        raise ValueError('Release version components must be between 0 and 65535')
    return version


def assemble(artifact_directory, release_directory, version, signed):
    release_version('v' + version)
    asset_sources = {}
    for architecture in ['amd64', 'arm64']:
        for package_format in ['deb', 'rpm']:
            asset_sources[f'saferm_{version}_linux_{architecture}.{package_format}'] = (
                artifact_directory / f'saferm-linux-{architecture}' / f'saferm-app-linux-{architecture}.{package_format}')
        mac_directory = f'signed-darwin-{architecture}' if signed else f'saferm-darwin-{architecture}'
        mac_filename = f'saferm-app-darwin-{architecture}.dmg' if signed else 'SafeRM.dmg'
        asset_sources[f'saferm_{version}_darwin_{architecture}.dmg'] = artifact_directory / mac_directory / mac_filename
        windows_directory = 'signed-windows' if signed else f'saferm-windows-{architecture}'
        asset_sources[f'saferm_{version}_windows_{architecture}.exe'] = (
            artifact_directory / windows_directory / f'saferm-app-windows-{architecture}-setup.exe')
    for filename in ['install.sh', 'install.ps1']:
        asset_sources[filename] = Path(__file__).parent / 'install' / filename
    for source_path in asset_sources.values():
        if not source_path.is_file():
            raise FileNotFoundError(source_path)
    release_directory.mkdir(parents=True, exist_ok=False)
    checksum_records = []
    for filename, source_path in sorted(asset_sources.items()):
        release_path = release_directory / filename
        shutil.copy2(source_path, release_path)
        checksum_records.append(f'{hashlib.sha256(release_path.read_bytes()).hexdigest()}  {filename}\n')
    (release_directory / f'saferm_{version}_checksums.txt').write_text(''.join(checksum_records), encoding='utf-8')


if __name__ == '__main__':
    argument_parser = argparse.ArgumentParser(description=__doc__)
    subcommands = argument_parser.add_subparsers(dest='command', required=True)
    version_parser = subcommands.add_parser('version')
    version_parser.add_argument('tag')
    assemble_parser = subcommands.add_parser('assemble')
    assemble_parser.add_argument('artifact_directory', type=Path)
    assemble_parser.add_argument('release_directory', type=Path)
    assemble_parser.add_argument('version')
    assemble_parser.add_argument('--signed', action='store_true')
    arguments = argument_parser.parse_args()
    if arguments.command == 'version':
        print(release_version(arguments.tag))
    else:
        assemble(arguments.artifact_directory, arguments.release_directory, arguments.version, arguments.signed)
