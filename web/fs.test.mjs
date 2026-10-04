import assert from 'node:assert/strict';
import test from 'node:test';
import {BrowserFS} from './fs.js';

const bytes = text => new TextEncoder().encode(text);
const text = data => new TextDecoder().decode(data);
const call = (install, method, ...args) => new Promise((resolve, reject) =>
  install.fs[method](...args, (error, result) => error ? reject(error) : resolve(result)));
const errno = code => error => error.code === code;
const turn = () => new Promise(resolve => setImmediate(resolve));

class Storage {
  constructor(records = []) {
    this.files = new Map(records.map(record => [record.path, structuredClone(record)]));
    this.commits = []; this.failure = null; this.hold = false; this.waiting = [];
  }
  async records() { return structuredClone([...this.files.values()]); }
  async commit(records = [], removed = []) {
    this.commits.push(structuredClone({records, removed}));
    if (this.hold) await new Promise(resolve => this.waiting.push(resolve));
    if (this.failure) { const error = this.failure; this.failure = null; throw error; }
    // Storage can yield before IndexedDB clones the input, awaiting its open.
    const snapshot = structuredClone({records, removed});
    for (const path of snapshot.removed) this.files.delete(path);
    for (const record of snapshot.records) this.files.set(record.path, record);
  }
  release() { this.waiting.shift()(); }
}

async function create(install, path, value) {
  const fd = await call(install, 'open', path, install.fs.constants.O_CREAT | install.fs.constants.O_RDWR, 0o600);
  await call(install, 'write', fd, bytes(value), 0, bytes(value).length, 0);
  return fd;
}

async function read(install, path) {
  const fd = await call(install, 'open', path, 0, 0);
  const {size} = await call(install, 'fstat', fd);
  const result = new Uint8Array(size);
  await call(install, 'read', fd, result, 0, size, 0);
  await call(install, 'close', fd);
  return text(result);
}

test('imported content stays read-only through file, metadata and namespace mutations', async () => {
  const storage = new Storage();
  const install = new BrowserFS([['data/archive.hpi', new Blob([bytes('original')])]], storage);
  const path = '/game/data/archive.hpi';
  const fd = await call(install, 'open', path, 0, 0);
  const c = install.fs.constants;
  const mutations = [
    ['open', path, c.O_WRONLY, 0], ['open', path, c.O_RDWR, 0],
    ['open', path, c.O_TRUNC, 0], ['open', '/game/new', c.O_CREAT | c.O_RDWR, 0],
    ['mkdir', '/game/new', 0], ['truncate', path, 0], ['unlink', path],
    ['rmdir', '/game/data'], ['rename', path, '/saves/archive.hpi'],
    ['chmod', path, 0], ['chown', path, 1, 1], ['lchown', path, 1, 1],
    ['utimes', path, 0, 0], ['fchmod', fd, 0], ['fchown', fd, 1, 1],
  ];
  for (const [method, ...args] of mutations) await assert.rejects(call(install, method, ...args), errno('EROFS'), method);
  await assert.rejects(call(install, 'ftruncate', fd, 0), errno('EBADF'));
  await assert.rejects(call(install, 'write', fd, bytes('x'), 0, 1, 0), errno('EBADF'));
  const saved = await create(install, '/tmp/source', 'replacement');
  await assert.rejects(call(install, 'rename', '/tmp/source', path), errno('EROFS'));
  await call(install, 'close', saved);
  await call(install, 'close', fd);
  assert.equal(await read(install, path), 'original');
  assert.deepEqual(await call(install, 'readdir', '/game/data'), ['archive.hpi']);
  assert.deepEqual(storage.commits, []);
});

test('imports cannot place browser files outside the read-only game mount', () => {
  for (const name of ['../saves/injected.SAV', '..', '../../settings/preferences']) {
    assert.throws(() => new BrowserFS([[name, new Blob(['content'])]], new Storage()), errno('EINVAL'));
  }
});

test('range reads use foreign-realm Blob slices, preserve offsets and stop at EOF', async () => {
  const block = 1024 * 1024, data = new Uint8Array(block + 8), slices = [];
  data.set(bytes('head'), 0); data.set(bytes('boundary'), block - 4); data.set(bytes('tail'), block + 4);
  // The import is deliberately duck-typed, as Files from another iframe are.
  const blob = {size: data.length, arrayBuffer() { throw new Error('whole-file read'); },
    slice(start, end) { slices.push([start, end]); return {arrayBuffer: async () => data.slice(start, end).buffer}; }};
  const install = new BrowserFS([['archive.hpi', blob]], new Storage());
  const fd = await call(install, 'open', '/game/archive.hpi', 0, 0);
  const result = new Uint8Array(8).fill(33);
  assert.equal(await call(install, 'read', fd, result, 2, 4, null), 4);
  assert.equal(text(result), '!!head!!');
  assert.equal(await call(install, 'read', fd, result, 0, 8, block - 4), 8);
  assert.equal(text(result), 'boundary');
  assert.equal(await call(install, 'read', fd, result, 0, 8, block + 4), 4);
  assert.equal(text(result.subarray(0, 4)), 'tail');
  assert.equal(await call(install, 'read', fd, result, 0, 4, null), 4);
  assert.deepEqual(result.subarray(0, 4), new Uint8Array(4), 'positioned reads do not move the descriptor');
  const beforeEOF = slices.length;
  assert.equal(await call(install, 'read', fd, result, 0, 8, data.length + 1), 0);
  assert.equal(slices.length, beforeEOF);
  assert.deepEqual(slices, [[0, block], [block - 4, block + 4], [block, block * 2]]);
  for (const range of [[-1, 1, 0], [0, 9, 0], [0, 1, -1], [0, 1, 0.5]]) {
    await assert.rejects(call(install, 'read', fd, result, ...range), errno('EINVAL'));
  }
  await call(install, 'close', fd);
});

test('settings, save bytes and sidecars restore; uncommitted and temporary files do not', async () => {
  const storage = new Storage(), install = new BrowserFS([], storage);
  await call(install, 'mkdir', '/saves/nested', 0o700);
  const settings = await create(install, '/settings/settings.json', '{"renderer":"classic"}');
  const save = await create(install, '/saves/nested/BATTLE.SAV', 'authored-save');
  const sidecar = await create(install, '/saves/nested/BATTLE.SAV.nanolathe', 'authored-sidecar');
  const temporary = await create(install, '/tmp/staging', 'session-only');
  await create(install, '/saves/uncommitted.SAV', 'not-yet-closed');
  await call(install, 'fsync', settings);
  await call(install, 'close', save); await call(install, 'close', sidecar); await call(install, 'close', temporary);
  const restored = new BrowserFS([], storage); await restored.restore();
  assert.equal(await read(restored, '/settings/settings.json'), '{"renderer":"classic"}');
  assert.equal(await read(restored, '/saves/nested/BATTLE.SAV'), 'authored-save');
  assert.equal(await read(restored, '/saves/nested/BATTLE.SAV.nanolathe'), 'authored-sidecar');
  for (const path of ['/tmp/staging', '/saves/uncommitted.SAV']) await assert.rejects(call(restored, 'stat', path), errno('ENOENT'));
  assert.ok(storage.commits.every(commit => commit.records.every(record => !record.path.startsWith('/tmp/'))));
});

test('restore admits only canonical settings and saves paths', async () => {
  const storage = new Storage([
    {path:'/saves/../game/archive.hpi', bytes:bytes('injected')},
    {path:'/tmp/staging', bytes:bytes('temporary')},
    {path:'/game/other.hpi', bytes:bytes('injected')},
    {path:'/saves', bytes:bytes('invalid mount file')},
    {path:'/settings/settings.json', bytes:bytes('settings'), mtime:123},
  ]);
  const install = new BrowserFS([['archive.hpi', new Blob(['original'])]], storage);
  await install.restore();
  assert.equal(await read(install, '/game/archive.hpi'), 'original');
  assert.equal(await read(install, '/settings/settings.json'), 'settings');
  assert.equal((await call(install, 'stat', '/settings/settings.json')).mtimeMs, 123);
  await assert.rejects(call(install, 'stat', '/tmp/staging'), errno('ENOENT'));
  await assert.rejects(call(install, 'stat', '/game/other.hpi'), errno('ENOENT'));
  assert.equal((await call(install, 'stat', '/saves')).isDirectory(), true);
});

test('atomic rename replaces a save and leaves replaced open descriptors detached', async () => {
  const storage = new Storage(), install = new BrowserFS([], storage);
  const old = await create(install, '/saves/BATTLE.SAV', 'old-save'); await call(install, 'fsync', old);
  const staged = await create(install, '/tmp/staged', 'new-save');
  const before = storage.commits.length;
  await call(install, 'rename', '/tmp/staged', '/saves/BATTLE.SAV');
  assert.equal(storage.commits.length, before + 1);
  assert.deepEqual(storage.commits.at(-1).removed, []);
  assert.equal(text(storage.commits.at(-1).records[0].bytes), 'new-save');
  assert.equal(await read(install, '/saves/BATTLE.SAV'), 'new-save');
  await assert.rejects(call(install, 'stat', '/tmp/staged'), errno('ENOENT'));
  await call(install, 'write', old, bytes('detached'), 0, 8, 0); await call(install, 'close', old);
  await call(install, 'close', staged);
  assert.equal(storage.commits.length, before + 1);
  const restored = new BrowserFS([], storage); await restored.restore();
  assert.equal(await read(restored, '/saves/BATTLE.SAV'), 'new-save');
});

test('directory rename commits every descendant together and preserves open handles', async () => {
  const storage = new Storage(), install = new BrowserFS([], storage);
  await call(install, 'mkdir', '/saves/old', 0);
  const fd = await create(install, '/saves/old/BATTLE.SAV', 'save'); await call(install, 'fsync', fd);
  await call(install, 'rename', '/saves/old', '/saves/new');
  const commit = storage.commits.at(-1);
  assert.deepEqual(commit.removed.sort(), ['/saves/old', '/saves/old/BATTLE.SAV']);
  assert.deepEqual(commit.records.map(record => record.path).sort(), ['/saves/new', '/saves/new/BATTLE.SAV']);
  await call(install, 'write', fd, bytes('more'), 0, 4, 0); await call(install, 'close', fd);
  const restored = new BrowserFS([], storage); await restored.restore();
  assert.equal(await read(restored, '/saves/new/BATTLE.SAV'), 'more');
  await assert.rejects(call(restored, 'stat', '/saves/old'), errno('ENOENT'));
});

test('storage failures use Go errno strings and failed persistence remains retryable', async () => {
  const storage = new Storage(), install = new BrowserFS([], storage);
  const fd = await create(install, '/saves/source', 'save');
  storage.failure = new DOMException('quota full', 'QuotaExceededError');
  await assert.rejects(call(install, 'fsync', fd), errno('ENOSPC'));
  assert.equal(storage.files.size, 0);
  await call(install, 'fsync', fd);
  const target = await create(install, '/saves/target', 'previous-save'); await call(install, 'close', target);
  for (const [method, args] of [['rename', ['/saves/source', '/saves/target']], ['unlink', ['/saves/source']]]) {
    storage.failure = new DOMException('transaction aborted', 'AbortError');
    await assert.rejects(call(install, method, ...args), errno('EIO'));
    assert.equal(await read(install, '/saves/source'), 'save');
    assert.equal(text(storage.files.get('/saves/source').bytes), 'save');
    assert.equal(await read(install, '/saves/target'), 'previous-save');
    assert.equal(text(storage.files.get('/saves/target').bytes), 'previous-save');
  }
  await call(install, 'rename', '/saves/source', '/saves/target');
  await call(install, 'unlink', '/saves/target');
  assert.equal(storage.files.size, 0);
  await call(install, 'close', fd);
});

test('failed close reports the storage error and leaves bytes available for a retry', async () => {
  const storage = new Storage(), install = new BrowserFS([], storage);
  const fd = await create(install, '/saves/BATTLE.SAV', 'save');
  storage.failure = new DOMException('storage unavailable', 'UnknownError');
  await assert.rejects(call(install, 'close', fd), errno('EIO'));
  await assert.rejects(call(install, 'fstat', fd), errno('EBADF'));
  assert.equal(storage.files.size, 0);
  const retry = await call(install, 'open', '/saves/BATTLE.SAV', 0, 0);
  await call(install, 'close', retry);
  assert.equal(text(storage.files.get('/saves/BATTLE.SAV').bytes), 'save');
});

test('a synchronous write during fsync is persisted by the next flush', async () => {
  const storage = new Storage(), install = new BrowserFS([], storage);
  const fd = await create(install, '/saves/BATTLE.SAV', 'first');
  storage.hold = true;
  const first = call(install, 'fsync', fd); await turn();
  assert.equal(storage.waiting.length, 1);
  install.fs.writeSync(fd, bytes('later'));
  storage.release(); await first;
  assert.equal(text(storage.files.get('/saves/BATTLE.SAV').bytes), 'first');
  storage.hold = false; await call(install, 'fsync', fd);
  assert.equal(text(storage.files.get('/saves/BATTLE.SAV').bytes), 'later');
});

test('overlapping flush and delete cannot resurrect a deleted save', async () => {
  const storage = new Storage(), install = new BrowserFS([], storage);
  const fd = await create(install, '/saves/BATTLE.SAV', 'save');
  storage.hold = true;
  const flush = call(install, 'fsync', fd); await turn();
  const deletion = call(install, 'unlink', '/saves/BATTLE.SAV'); await turn();
  assert.equal(storage.waiting.length, 1, 'delete waits for the outstanding flush');
  storage.release(); await flush; await turn();
  assert.equal(storage.waiting.length, 1);
  storage.release(); await deletion;
  storage.hold = false; await call(install, 'fsync', fd); await call(install, 'close', fd);
  assert.equal(storage.files.size, 0);
  await assert.rejects(call(install, 'stat', '/saves/BATTLE.SAV'), errno('ENOENT'));
});

test('rename keeps intervening descriptor writes dirty and orders later path mutations', async () => {
  const storage = new Storage(), install = new BrowserFS([], storage);
  await call(install, 'mkdir', '/saves/old', 0);
  const fd = await create(install, '/saves/old/BATTLE.SAV', 'first');
  storage.hold = true;
  const rename = call(install, 'rename', '/saves/old', '/saves/new'); await turn();
  install.fs.writeSync(fd, bytes('later'));
  const opening = call(install, 'open', '/saves/old/extra', install.fs.constants.O_CREAT | install.fs.constants.O_RDWR, 0);
  // Register rejection handling before releasing the preceding transaction.
  const rejected = assert.rejects(opening, errno('ENOENT'));
  const deletion = call(install, 'unlink', '/saves/new/BATTLE.SAV'); await turn();
  assert.equal(storage.waiting.length, 1);
  storage.release(); await rename; await rejected; await turn();
  assert.equal(storage.waiting.length, 1, 'delete sees the renamed path');
  assert.equal(text(storage.files.get('/saves/new/BATTLE.SAV').bytes), 'first');
  storage.release(); await deletion;
  storage.hold = false; await call(install, 'close', fd);
  assert.equal(storage.files.has('/saves/new/BATTLE.SAV'), false);
  assert.deepEqual(await call(install, 'readdir', '/saves/new'), []);
});

test('a write during atomic rename is not lost on close', async () => {
  const storage = new Storage(), install = new BrowserFS([], storage);
  const fd = await create(install, '/tmp/staged', 'first');
  storage.hold = true;
  const rename = call(install, 'rename', '/tmp/staged', '/saves/BATTLE.SAV'); await turn();
  install.fs.writeSync(fd, bytes('later'));
  storage.release(); await rename;
  storage.hold = false; await call(install, 'close', fd);
  assert.equal(text(storage.files.get('/saves/BATTLE.SAV').bytes), 'later');
});

test('an ignored bank close failure remains visible after a successful sidecar until the bank retries', async () => {
  const storage=new Storage(), failed=new Map();
  const install=new BrowserFS([],storage,()=>{},({paths,error})=>{
    for (const path of paths) { if (error) failed.set(path,error); else failed.delete(path); }
  });
  let fd=await create(install,'/saves/BROWSER.SAV','previous-save'); await call(install,'close',fd);
  fd=await call(install,'open','/saves/BROWSER.SAV',install.fs.constants.O_RDWR | install.fs.constants.O_TRUNC,0);
  await call(install,'write',fd,bytes('newer-save'),0,bytes('newer-save').length,0);
  storage.failure=new DOMException('quota exhausted','QuotaExceededError');
  await assert.rejects(call(install,'close',fd),errno('ENOSPC'));
  // The retail bank writer ignores Close; the independently successful sidecar
  // must never clear the browser's warning for the bank it accompanies.
  const sidecar=await create(install,'/saves/BROWSER.SAV.nanolathe','newer-sidecar');
  await call(install,'close',sidecar);
  assert.equal(text(storage.files.get('/saves/BROWSER.SAV').bytes),'previous-save');
  assert.deepEqual([...failed],[['/saves/BROWSER.SAV','ENOSPC']]);
  fd=await call(install,'open','/saves/BROWSER.SAV',install.fs.constants.O_RDWR,0);
  await call(install,'close',fd);
  assert.equal(failed.size,0);
  assert.equal(text(storage.files.get('/saves/BROWSER.SAV').bytes),'newer-save');
});
