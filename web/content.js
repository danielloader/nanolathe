// Host-only content acquisition. Nothing uploads local game files.
const demoNames = new Set(['tademo.hpi', 'tademoreadme.txt']);
const hex = bytes => Array.from(new Uint8Array(bytes), n => n.toString(16).padStart(2, '0')).join('');
export async function digest(blob) {
  return hex(await crypto.subtle.digest('SHA-256', await blob.arrayBuffer()));
}
export function importedContent(files) {
  const marker = files.find(([path]) => /(^|\/)totala1\.hpi$/i.test(path)) ||
    files.find(([path]) => /(^|\/)tademo\.hpi$/i.test(path));
  if (!marker) throw new Error('Choose a TotalA installation or extracted Total Annihilation demo folder.');
  const root = marker[0].slice(0, marker[0].lastIndexOf('/') + 1);
  const mounted = files.filter(([path]) => path.startsWith(root)).map(([path, blob]) => [path.slice(root.length), blob]);
  for (const [path] of mounted) {
    if (!path || path.split('/').some(p => !p || p === '.' || p === '..') || /[\\\x00]/.test(path)) {
      throw new Error('Invalid local file path: ' + path);
    }
  }
  const kind = /tademo\.hpi$/i.test(marker[0]) ? 'demo' : 'retail';
  return {kind, id: kind, name: kind === 'demo' ? 'Local demo' : 'Your TotalA folder', files: mounted};
}
export async function demoManifest(url, signal) {
  const response = await fetch(url, {signal, cache: 'no-cache'});
  if (response.status === 404) return null;
  if (!response.ok) throw new Error('Demo manifest request failed (' + response.status + ').');
  const manifest = await response.json(), base = new URL('.', response.url || url);
  if (base.origin !== location.origin) throw new Error('Demo manifest must be same-origin.');
  if (manifest.schema !== 1 || manifest.kind !== 'ta-demo' || !Array.isArray(manifest.files) || manifest.files.length !== 2) {
    throw new Error('Unsupported demo manifest.');
  }
  const names = new Set();
  for (const file of manifest.files) {
    if (!file || typeof file.path !== 'string' || typeof file.url !== 'string') throw new Error('Invalid demo asset manifest.');
    const name = file.path.toLowerCase(), asset = new URL(file.url, base);
    if (!demoNames.has(name) || names.has(name) || !Number.isSafeInteger(file.size) || file.size <= 0 ||
        file.size > 64 * 1024 * 1024 || !/^[a-f0-9]{64}$/.test(file.sha256) ||
        asset.origin !== location.origin || !asset.pathname.startsWith(base.pathname) || asset.search || asset.hash) {
      throw new Error('Invalid demo asset manifest.');
    }
    names.add(name); file.url = asset.href;
  }
  return manifest;
}
export async function loadDemo(manifest, signal, progress) {
  // Storage denial/fullness affects caching, not the ability to try the demo.
  const cache = await globalThis.caches?.open('nanolathe-demo-v1').catch(() => null);
  const total = manifest.files.reduce((n, f) => n + f.size, 0);
  let loaded = 0;
  const files = [];
  for (const file of manifest.files) {
    signal.throwIfAborted();
    let blob = await cache?.match(file.url).then(r => r?.blob()).catch(() => null);
    if (blob && (blob.size !== file.size || await digest(blob) !== file.sha256)) {
      await cache?.delete(file.url).catch(() => {}); blob = null;
    }
    if (!blob) {
      const response = await fetch(file.url, {signal});
      if (!response.ok) throw new Error('Demo download failed (' + response.status + '): ' + file.path);
      const reader = response.body.getReader(), chunks = [];
      let count = 0;
      for (;;) {
        const {value, done} = await reader.read();
        if (done) break;
        count += value.byteLength;
        if (count > file.size) { await reader.cancel(); throw new Error('Demo size mismatch: ' + file.path); }
        chunks.push(value); progress(loaded + count, total);
      }
      blob = new Blob(chunks);
      if (blob.size !== file.size || await digest(blob) !== file.sha256) throw new Error('Demo integrity check failed: ' + file.path);
      signal.throwIfAborted();
      await cache?.put(file.url, new Response(blob)).catch(() => {});
    }
    loaded += blob.size; progress(loaded, total); files.push([file.path, blob]);
  }
  return {kind: 'demo', id: 'demo', name: 'Official TA demo', files};
}
