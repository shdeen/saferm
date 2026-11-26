"""Permanent isolation tests for release versions, complete assets, and checksums."""
import hashlib
import importlib.util
from pathlib import Path
import tempfile
import unittest

module_spec = importlib.util.spec_from_file_location('release', Path(__file__).with_name('release.py'))
release = importlib.util.module_from_spec(module_spec)
module_spec.loader.exec_module(release)


class ReleaseTests(unittest.TestCase):
    def test_release_tag_version(self):
        for tag, version in [('v1.2.3', '1.2.3'), ('v26.11.23', '26.11.23'), ('v0.0.0', '0.0.0')]:
            with self.subTest(tag=tag):
                self.assertEqual(release.release_version(tag), version)

    def test_invalid_release_tag(self):
        for tag in ['', 'main', '1.2.3', 'v1.2', 'v01.2.3', 'v1.2.3-rc1', 'v1.2.3\n', 'v1.2.3/../../x', 'v65536.0.0']:
            with self.subTest(tag=tag):
                with self.assertRaises(ValueError):
                    release.release_version(tag)

    def test_complete_signed_release(self):
        with tempfile.TemporaryDirectory() as temporary_directory:
            artifact_directory = Path(temporary_directory) / 'packages'
            release_directory = Path(temporary_directory) / 'release'
            package_contents = populate_packages(artifact_directory)
            release.assemble(artifact_directory, release_directory, '1.2.3', signed=True)
            for filename, package_bytes in package_contents.items():
                self.assertEqual((release_directory / filename).read_bytes(), package_bytes)
            for filename in ['install.sh', 'install.ps1']:
                self.assertEqual((release_directory / filename).read_bytes(), (Path(__file__).parent / 'install' / filename).read_bytes())
            checksum_records = (release_directory / 'saferm_1.2.3_checksums.txt').read_text().splitlines()
            self.assertEqual(len(checksum_records), len(package_contents) + 2)
            for checksum_record in checksum_records:
                digest, filename = checksum_record.split('  ')
                self.assertEqual(digest, hashlib.sha256((release_directory / filename).read_bytes()).hexdigest())

    def test_missing_signed_asset_stops_before_assembly(self):
        with tempfile.TemporaryDirectory() as temporary_directory:
            artifact_directory = Path(temporary_directory) / 'packages'
            release_directory = Path(temporary_directory) / 'release'
            populate_packages(artifact_directory, omitted_artifact='signed-darwin-arm64')
            with self.assertRaises(FileNotFoundError):
                release.assemble(artifact_directory, release_directory, '1.2.3', signed=True)
            self.assertFalse(release_directory.exists())

    def test_manual_build_uses_native_packages(self):
        with tempfile.TemporaryDirectory() as temporary_directory:
            artifact_directory = Path(temporary_directory) / 'packages'
            release_directory = Path(temporary_directory) / 'release'
            populate_packages(artifact_directory)
            release.assemble(artifact_directory, release_directory, '1.2.3', signed=False)
            self.assertEqual((release_directory / 'saferm_1.2.3_darwin_arm64.dmg').read_bytes(), b'local macOS arm64')
            self.assertEqual((release_directory / 'saferm_1.2.3_windows_amd64.exe').read_bytes(), b'local Windows amd64')


def populate_packages(artifact_directory, omitted_artifact=None):
    """Unit-only package bytes distinguish selected artifacts without running installers."""
    published_assets = {}
    for architecture in ['amd64', 'arm64']:
        for package_format in ['deb', 'rpm']:
            artifact_path = artifact_directory / f'saferm-linux-{architecture}'
            artifact_path.mkdir(parents=True, exist_ok=True)
            package_bytes = f'Linux {architecture} {package_format}'.encode()
            (artifact_path / f'saferm-app-linux-{architecture}.{package_format}').write_bytes(package_bytes)
            published_assets[f'saferm_1.2.3_linux_{architecture}.{package_format}'] = package_bytes
        for platform, extension, local_filename in [('darwin', 'dmg', 'SafeRM.dmg'), ('windows', 'exe', f'saferm-app-windows-{architecture}-setup.exe')]:
            label = 'macOS' if platform == 'darwin' else 'Windows'
            artifact_path = artifact_directory / f'saferm-{platform}-{architecture}'
            artifact_path.mkdir(parents=True)
            (artifact_path / local_filename).write_bytes(f'local {label} {architecture}'.encode())
            signed_name = f'signed-darwin-{architecture}' if platform == 'darwin' else 'signed-windows'
            if signed_name == omitted_artifact:
                continue
            signed_artifact = artifact_directory / signed_name
            signed_artifact.mkdir(parents=True, exist_ok=True)
            signed_filename = f'saferm-app-darwin-{architecture}.dmg' if platform == 'darwin' else local_filename
            package_bytes = f'signed {label} {architecture}'.encode()
            (signed_artifact / signed_filename).write_bytes(package_bytes)
            published_assets[f'saferm_1.2.3_{platform}_{architecture}.{extension}'] = package_bytes
    return published_assets


if __name__ == '__main__':
    unittest.main()
