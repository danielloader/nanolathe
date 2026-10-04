"""Authored packaging fixtures; no original game assets."""
import json
import tempfile
import unittest
from pathlib import Path
from browser_bundle import browser_bundle


class BundleTests(unittest.TestCase):
    def test_old_launcher_keeps_its_child_modules_and_engine_manifest(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            source, out = root / 'web', root / 'out'
            source.mkdir()
            out.mkdir()
            (source / 'index.html').write_text('<link href="style.css"><script src="launcher.js"></script>')
            (source / 'launcher.js').write_text('import "./host.js";')
            (source / 'host.js').write_text('old host')
            (source / 'game.html').write_text('<script src="game.js"></script>')
            (source / 'game.js').write_text('old child')
            (source / 'host.test.mjs').write_text('test')
            build = dict(wasm='old.wasm', runtime='old.js', schema=1)
            first = browser_bundle(source, out, build)
            self.assertEqual(first, browser_bundle(source, out, build))
            (source / 'game.js').write_text('new child')
            second = browser_bundle(source, out, dict(build, wasm='new.wasm'))
            self.assertNotEqual(first, second)
            self.assertEqual((out / first / 'game.js').read_text(), 'old child')
            self.assertIn('old.wasm', (out / first / 'build.json').read_text())
            self.assertIn(second + '/launcher.js', (out / 'index.html').read_text())
            self.assertIn(second + '/style.css', (out / 'index.html').read_text())
            self.assertFalse((out / first / 'host.test.mjs').exists())
            root_manifest = json.loads((out / 'build.json').read_text())
            self.assertEqual(root_manifest['host'], second)
            self.assertEqual(root_manifest['files'], ['build.json', 'game.html', 'game.js', 'host.js', 'index.html', 'launcher.js'][1:] + ['build.json'])
            self.assertNotIn('host', json.loads((out / second / 'build.json').read_text()))


if __name__ == '__main__':
    unittest.main()
