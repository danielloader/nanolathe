"""Explicit demo allowlist shared by local build and preview tools."""
import hashlib
from pathlib import Path

def demo_assets(root):
    root = Path(root).expanduser().resolve()
    by_name = {p.name.lower(): p for p in root.iterdir() if p.is_file()}
    files, sources = [], {}
    for name in ('TADemo.hpi', 'TADemoReadme.txt'):
        source = by_name.get(name.lower())
        if source is None:
            raise ValueError(f'missing demo input: {root / name}')
        if not source.resolve().is_relative_to(root):
            raise ValueError(f'demo input escapes its root: {name}')
        data = source.read_bytes()
        if not data or len(data) > 64 * 1024 * 1024:
            raise ValueError(f'invalid demo file size: {name}')
        if name.endswith('.hpi') and data[:4] != b'HAPI':
            raise ValueError('TADemo.hpi is not a HAPI archive')
        digest = hashlib.sha256(data).hexdigest()
        url = f'files/{digest}/{name}'
        files.append(dict(path=name, url=url, size=len(data), sha256=digest))
        sources[url] = source
    return dict(schema=1, kind='ta-demo', files=files), sources
