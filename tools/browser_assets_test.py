"""Authored fixtures for the demo distribution boundary; no original bytes."""
import hashlib
import tempfile
import unittest
from pathlib import Path
from browser_assets import demo_assets


class DemoAllowlist(unittest.TestCase):
    def test_only_archive_and_original_readme_are_admitted(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            archive = b'HAPI' + b'authored header fixture'
            (root / 'tademo.HPI').write_bytes(archive)
            (root / 'TADemoReadme.txt').write_text('authored readme')
            (root / 'TADemo.exe').write_bytes(b'not distributable')
            (root / 'save.SAV').write_bytes(b'local save')
            manifest, sources = demo_assets(root)
            self.assertEqual({f['path'] for f in manifest['files']}, {'TADemo.hpi', 'TADemoReadme.txt'})
            row = manifest['files'][0]
            self.assertEqual(row['sha256'], hashlib.sha256(archive).hexdigest())
            self.assertEqual(row['size'], len(archive))
            self.assertEqual(len(sources), 2)
            self.assertNotIn(root / 'TADemo.exe', sources.values())

    def test_missing_invalid_and_escaping_files_are_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / 'demo'
            root.mkdir()
            (root / 'TADemoReadme.txt').write_text('authored readme')
            with self.assertRaises(ValueError):
                demo_assets(root)
            archive = root / 'TADemo.hpi'
            archive.write_bytes(b'not an archive')
            with self.assertRaises(ValueError):
                demo_assets(root)
            archive.unlink()
            outside = Path(directory) / 'outside.hpi'
            outside.write_bytes(b'HAPIauthored')
            archive.symlink_to(outside)
            with self.assertRaises(ValueError):
                demo_assets(root)


if __name__ == '__main__':
    unittest.main()
