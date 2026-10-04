import assert from 'node:assert/strict';
import test from 'node:test';
import {BrowserStorage} from './storage.js';

const record = (path, value) => ({path, dir:false, mtime:123, bytes:new TextEncoder().encode(value)});
const turn = () => new Promise(resolve => setImmediate(resolve));

// Only the IndexedDB boundary used by BrowserStorage is modeled here. Requests
// clone their values, transactions publish together, and aborts publish nothing.
function indexedDBFixture(t) {
  const previous = Object.getOwnPropertyDescriptor(globalThis, 'indexedDB');
  const fixture = {databases:new Map(), opens:[], failNext:null, blockNext:false};
  fixture.open = (name, version) => {
    fixture.opens.push([name, version]);
    const request = {}, blocked = fixture.blockNext; fixture.blockNext = false;
    let state = fixture.databases.get(name), upgrade = !state;
    if (!state) { state = {files:new Map(), connections:[]}; fixture.databases.set(name, state); }
    const db = {closed:false,
      createObjectStore(storeName, options) { assert.equal(storeName, 'files'); assert.deepEqual(options, {keyPath:'path'}); },
      close() { this.closed = true; },
      transaction(storeName, mode, options) {
        assert.equal(storeName, 'files');
        if (this.closed) throw new DOMException('connection closed', 'InvalidStateError');
        if (mode === 'readwrite') assert.deepEqual(options, {durability:'strict'});
        const operations = [], requests = [], failure = fixture.failNext; fixture.failNext = null;
        const tx = {error:null, aborted:false,
          abort() { this.aborted = true; },
          objectStore() { return {
            getAll() { const result = {}; requests.push(result); return result; },
            delete(path) { operations.push({remove:path}); },
            put(value) { operations.push({put:structuredClone(value)}); },
          }; },
        };
        setImmediate(() => {
          if (failure || tx.aborted) {
            tx.error = failure;
            for (const request of requests) request.error = failure;
            tx.onabort?.(); return;
          }
          for (const operation of operations) {
            if ('remove' in operation) state.files.delete(operation.remove);
            else state.files.set(operation.put.path, operation.put);
          }
          for (const request of requests) request.result = structuredClone([...state.files.values()]);
          tx.oncomplete?.();
        });
        return tx;
      },
    };
    state.connections.push(db); request.result = db;
    queueMicrotask(() => {
      if (blocked) { request.onblocked?.(); setImmediate(() => request.onsuccess?.()); }
      else { if (upgrade) request.onupgradeneeded?.(); request.onsuccess?.(); }
    });
    return request;
  };
  Object.defineProperty(globalThis, 'indexedDB', {value:fixture, configurable:true});
  t.after(() => { if (previous) Object.defineProperty(globalThis, 'indexedDB', previous); else delete globalThis.indexedDB; });
  return fixture;
}

test('demo and original retail namespaces preserve their own committed snapshots', async t => {
  const fixture = indexedDBFixture(t), notifications = [];
  const retail = new BrowserStorage('nanolathe-browser-poc', () => notifications.push('retail'));
  const demo = new BrowserStorage('nanolathe-browser-demo-v1', () => notifications.push('demo'));
  const saved = record('/saves/BATTLE.SAV', 'retail-save');
  await retail.commit([saved]); saved.bytes.fill(0);
  await demo.commit([record('/saves/BATTLE.SAV', 'demo-save')]);
  assert.deepEqual(await retail.records(), [record('/saves/BATTLE.SAV', 'retail-save')]);
  assert.deepEqual(await demo.records(), [record('/saves/BATTLE.SAV', 'demo-save')]);
  const replacement = record('/saves/BATTLE.SAV', 'replacement');
  await retail.commit([replacement], ['/saves/BATTLE.SAV']);
  assert.deepEqual(await retail.records(), [replacement], 'replacement is published in one transaction');
  assert.deepEqual(notifications, ['retail', 'demo', 'retail']);
  retail.close(); demo.close(); await turn();
  const reopened = new BrowserStorage('nanolathe-browser-poc');
  assert.deepEqual(await reopened.records(), [replacement]);
  assert.deepEqual(fixture.opens, [['nanolathe-browser-poc', 1], ['nanolathe-browser-demo-v1', 1], ['nanolathe-browser-poc', 1]]);
  reopened.close();
});

test('aborted writes preserve previous saves, reject the error and do not notify', async t => {
  const fixture = indexedDBFixture(t); let changed = 0;
  const storage = new BrowserStorage('authored-storage-test', () => changed++);
  const original = record('/saves/BATTLE.SAV', 'original'); await storage.commit([original]);
  const failure = new DOMException('quota full', 'QuotaExceededError'); fixture.failNext = failure;
  await assert.rejects(storage.commit([record('/saves/BATTLE.SAV', 'replacement')], [original.path]), error => error === failure);
  assert.deepEqual(await storage.records(), [original]); assert.equal(changed, 1);
  await storage.commit([record('/saves/BATTLE.SAV', 'retry')], [original.path]);
  assert.equal(changed, 2);
  storage.close();
});

test('a synchronous request failure aborts earlier deletes in the same transaction', async t => {
  indexedDBFixture(t); let changed = 0;
  const storage = new BrowserStorage('authored-storage-test', () => changed++);
  const original = record('/saves/BATTLE.SAV', 'original'); await storage.commit([original]);
  const invalid = {...record('/saves/new.SAV', 'new'), cannotClone() {}};
  await assert.rejects(storage.commit([invalid], [original.path]), {name:'DataCloneError'});
  await turn();
  assert.deepEqual(await storage.records(), [original]);
  assert.equal(changed, 1);
  storage.close();
});

test('readonly transaction failures reject instead of returning an empty save list', async t => {
  const fixture = indexedDBFixture(t), storage = new BrowserStorage('authored-storage-test');
  await storage.ready;
  const failure = new DOMException('read aborted', 'AbortError'); fixture.failNext = failure;
  await assert.rejects(storage.records(), error => error === failure);
  storage.close();
});

test('close before opening and version changes release storage connections', async t => {
  const fixture = indexedDBFixture(t), storage = new BrowserStorage('authored-storage-test');
  storage.close(); await storage.ready; await turn();
  assert.equal(fixture.databases.get(storage.name).connections[0].closed, true);
  const second = new BrowserStorage(storage.name); const db = await second.ready;
  assert.equal(db.closed, false); db.onversionchange(); assert.equal(db.closed, true);
});

test('a blocked open that later succeeds does not leak an unused connection', async t => {
  const fixture = indexedDBFixture(t); fixture.blockNext = true;
  const storage = new BrowserStorage('authored-storage-test');
  await assert.rejects(storage.ready, /Close other Nanolathe tabs/); await turn();
  assert.equal(fixture.databases.get(storage.name).connections[0].closed, true);
});
