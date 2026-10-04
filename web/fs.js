// Node-style filesystem for Go's js/wasm runtime. Content files remain read-only
// browser Blobs; settings and saves use origin-local IndexedDB, tmp stays in RAM.
const blockSize = 1024 * 1024, cacheLimit = 64 * 1024 * 1024;
export class BrowserFS {
  constructor(files, storage, log = () => {}, persistence = () => {}) {
    this.storage = storage; this.persistence = persistence;
    this.nodes = new Map(); this.handles = new Map(); this.nextFD = 10; this.nextInode = 1;
    this.cache = new Map(); this.cacheBytes = 0; this.loading = new Map();
    this.pending = Promise.resolve();
    this.path = {resolve: (...parts) => this.clean(parts.join('/'))};
    this.process = {cwd: () => '/', chdir: () => {}, getuid: () => 0, getgid: () => 0,
      geteuid: () => 0, getegid: () => 0, getgroups: () => [], pid: 1, ppid: 0, umask: () => 0};
    for (const path of ['/', '/game', '/settings', '/saves', '/tmp']) this.mkdir(path);
    for (const [name, file] of files) {
      const p = this.clean('/game/' + name);
      if (!p.startsWith('/game/')) throw this.error('EINVAL', name);
      this.add(p, file);
    }
    const wrap = fn => (...args) => {
      const cb = args.pop();
      try {
        const result = fn(...args);
        if (result && typeof result.then === 'function') result.then(v => cb(null, v), e => cb(this.normalizeError(e)));
        else cb(null, result);
      } catch (e) { cb(this.normalizeError(e)); }
    };
    const fs = {constants: {O_WRONLY: 1, O_RDWR: 2, O_CREAT: 64, O_TRUNC: 512, O_APPEND: 1024, O_EXCL: 128, O_DIRECTORY: 65536}};
    fs.open = wrap((p, flags) => this.serialize(() => this.open(p, flags)));
    fs.close = wrap(fd => this.serialize(async () => { const h = this.handle(fd); try { await this.flush(h.node); } finally { this.handles.delete(fd); } }));
    fs.stat = fs.lstat = wrap(p => this.stat(this.node(p)));
    fs.fstat = wrap(fd => this.stat(this.handle(fd).node));
    fs.readdir = wrap(p => { const n = this.node(p); if (!n.dir) throw this.error('ENOTDIR', p); return Array.from(n.children.keys()).sort(); });
    fs.mkdir = wrap(p => this.serialize(async () => {
      p = this.clean(p); this.writable(p);
      if (this.nodes.has(p)) throw this.error('EEXIST', p);
      this.directory(this.dirname(p)); const n = this.mkdir(p); n.dirty = true; await this.flush(n);
    }));
    fs.read = (fd, buffer, offset, length, position, cb) => {
      try {
        const h = this.handle(fd), n = h.node;
        if (h.flags & 1) throw this.error('EBADF', fd);
        if (n.dir) throw this.error('EISDIR', n.path);
        const start = position == null ? h.pos : position;
        this.range(buffer, offset, length, start);
        const count = Math.max(0, Math.min(length, n.size - start));
        const done = bytes => { buffer.set(bytes.subarray(0, count), offset); if (position == null) h.pos += count; cb(null, count); };
        if (!count) done(new Uint8Array());
        else if (n.bytes) done(n.bytes.subarray(start, start + count));
        else {
          // A cached block completes synchronously, so the Go caller continues
          // without a microtask hop for every small archive read.
          const cached = this.cachedBlock(n, start, count);
          if (cached) done(cached);
          else this.readBlob(n, start, count).then(done, e => cb(this.normalizeError(e)));
        }
      } catch (e) { cb(this.normalizeError(e)); }
    };
    fs.writeSync = (fd, buf) => {
      if (fd === 1 || fd === 2) { log(new TextDecoder().decode(buf).trimEnd()); return buf.length; }
      return this.write(fd, buf, 0, buf.length, null);
    };
    fs.write = wrap((fd, buf, offset, length, pos) => this.serialize(() => {
      if (fd === 1 || fd === 2) return fs.writeSync(fd, buf.subarray(offset, offset + length));
      return this.write(fd, buf, offset, length, pos);
    }));
    fs.rename = wrap((a, b) => this.serialize(() => this.rename(a, b)));
    fs.unlink = wrap(p => this.serialize(() => this.remove(p, false)));
    fs.rmdir = wrap(p => this.serialize(() => this.remove(p, true)));
    fs.fsync = wrap(fd => this.serialize(() => this.flush(this.handle(fd).node)));
    fs.chmod = fs.chown = fs.lchown = fs.utimes = wrap(p => { this.writable(this.node(p).path); });
    fs.fchmod = fs.fchown = wrap(fd => { this.writable(this.handle(fd).node.path); });
    fs.readlink = wrap(p => { this.node(p); throw this.error('EINVAL', p); });
    fs.truncate = wrap((p, len) => this.serialize(async () => { const n = this.node(p); this.truncate(n, len); await this.flush(n); }));
    fs.ftruncate = wrap((fd, len) => this.serialize(() => { const h = this.handle(fd); if (!(h.flags & 3)) throw this.error('EBADF', fd); this.truncate(h.node, len); }));
    this.fs = fs;
  }
  async restore() {
    const records = (await this.storage.records()).filter(r => typeof r.path === 'string' &&
      r.path === this.clean(r.path) && this.persistent(r.path) &&
      (r.dir || (r.path !== '/settings' && r.path !== '/saves')));
    records.sort((a, b) => a.path.length - b.path.length || a.path.localeCompare(b.path));
    for (const r of records) {
      const n = r.dir ? this.mkdir(r.path) : this.add(r.path, new Uint8Array(r.bytes));
      n.mtime = r.mtime; n.dirty = false;
    }
  }
  serialize(fn) {
    // Namespace changes and durable publication share one order
    // (DESIGN_BROWSER_HOST §4 contract 2); failures must not stop later retries.
    const result = this.pending.then(fn);
    this.pending = result.catch(() => {});
    return result;
  }
  clean(p) { const out = []; for (const part of p.split('/')) { if (part === '..') out.pop(); else if (part && part !== '.') out.push(part); } return '/' + out.join('/'); }
  dirname(p) { return p.slice(0, p.lastIndexOf('/')) || '/'; }
  basename(p) { return p.slice(p.lastIndexOf('/') + 1); }
  persistent(p) { return ['/settings', '/saves'].some(root => p === root || p.startsWith(root + '/')); }
  writable(p) { if (!this.persistent(p) && p !== '/tmp' && !p.startsWith('/tmp/')) throw this.error('EROFS', p); }
  error(code, p) { const e = new Error(code + ': ' + p); e.code = code; return e; }
  normalizeError(e) {
    // DOMException.code is numeric; Go expects a known Node errno string.
    const known = ['ENOENT','EBADF','EEXIST','ENOTDIR','EISDIR','EROFS','EINVAL','ENOTEMPTY','EIO','ENOSPC','EACCES','EBUSY'];
    if (known.includes(e?.code)) return e;
    return this.error(e?.name === 'QuotaExceededError' ? 'ENOSPC' : 'EIO', e?.message || 'browser filesystem');
  }
  node(p) { p = this.clean(p); const n = this.nodes.get(p); if (!n) throw this.error('ENOENT', p); return n; }
  directory(p) { const n = this.node(p); if (!n.dir) throw this.error('ENOTDIR', p); return n; }
  handle(fd) { const h = this.handles.get(fd); if (!h) throw this.error('EBADF', fd); return h; }
  mkdir(p) {
    if (this.nodes.has(p)) return this.directory(p);
    if (p !== '/') this.mkdir(this.dirname(p));
    const n = {path: p, dir: true, children: new Map(), size: 0, ino: this.nextInode++, mtime: Date.now()};
    this.nodes.set(p, n);
    if (p !== '/') this.directory(this.dirname(p)).children.set(this.basename(p), n);
    return n;
  }
  add(p, data) {
    p = this.clean(p); this.mkdir(this.dirname(p));
    // Imported Files belong to the parent iframe's realm, so instanceof Blob fails.
    const blob = typeof data.arrayBuffer === 'function';
    const n = {path: p, dir: false, size: data.size ?? data.length, blob: blob ? data : null,
      bytes: blob ? null : data, ino: this.nextInode++, mtime: data.lastModified || Date.now()};
    this.nodes.set(p, n); this.directory(this.dirname(p)).children.set(this.basename(p), n); return n;
  }
  stat(n) { return {dev: 1, ino: n.ino, mode: n.dir ? 16877 : 33188, nlink: 1, uid: 0, gid: 0, rdev: 0,
    size: n.size, blksize: 4096, blocks: Math.ceil(n.size / 512), atimeMs: n.mtime, mtimeMs: n.mtime, ctimeMs: n.mtime, isDirectory: () => n.dir}; }
  open(p, flags) {
    p = this.clean(p); let n = this.nodes.get(p);
    if ((flags & 64) && (flags & 128) && n) throw this.error('EEXIST', p);
    if ((flags & 3) || (flags & 512) || (!n && (flags & 64))) this.writable(p);
    if (!n) {
      if (!(flags & 64)) throw this.error('ENOENT', p);
      this.directory(this.dirname(p)); n = this.add(p, new Uint8Array()); n.dirty = true;
    }
    if ((flags & 65536) && !n.dir) throw this.error('ENOTDIR', p);
    if (n.dir && ((flags & 3) || (flags & 512))) throw this.error('EISDIR', p);
    if (flags & 512) this.truncate(n, 0);
    const fd = this.nextFD++; this.handles.set(fd, {node: n, flags, pos: flags & 1024 ? n.size : 0});
    // Archive directories and small loose files live in the first block; one
    // slice at open serves the reads that follow instead of one slice each.
    if (n.blob && n.size > 0) this.readBlob(n, 0, Math.min(blockSize, n.size)).catch(() => {});
    return fd;
  }
  range(buffer, offset, length, start) {
    if (![offset, length, start].every(Number.isSafeInteger) || offset < 0 || length < 0 || start < 0 || offset + length > buffer.length) throw this.error('EINVAL', 'read/write range');
  }
  // A range inside one block is served from the block cache; larger or
  // straddling ranges read the Blob directly.
  block(n, start, length) {
    const base = Math.floor(start / blockSize) * blockSize;
    if (length > blockSize || start + length > base + blockSize) return null;
    return {base, key: n.ino + ':' + base};
  }
  cachedBlock(n, start, length) {
    const block = this.block(n, start, length);
    const data = block && this.cache.get(block.key);
    if (!data) return null;
    this.cache.delete(block.key); this.cache.set(block.key, data);
    return data.subarray(start - block.base, start - block.base + length);
  }
  async readBlob(n, start, length) {
    const block = this.block(n, start, length);
    if (!block) return new Uint8Array(await n.blob.slice(start, start + length).arrayBuffer());
    const cached = this.cachedBlock(n, start, length);
    if (cached) return cached;
    // Concurrent reads of one block share a single slice and one accounting.
    let loading = this.loading.get(block.key);
    if (!loading) {
      loading = n.blob.slice(block.base, block.base + blockSize).arrayBuffer().then(buffer => {
        const data = new Uint8Array(buffer);
        this.cache.set(block.key, data); this.cacheBytes += data.length;
        while (this.cacheBytes > cacheLimit) { const k = this.cache.keys().next().value; this.cacheBytes -= this.cache.get(k).length; this.cache.delete(k); }
        return data;
      }).finally(() => this.loading.delete(block.key));
      this.loading.set(block.key, loading);
    }
    const data = await loading;
    return data.subarray(start - block.base, start - block.base + length);
  }
  truncate(n, len) {
    this.writable(n.path);
    if (n.dir) throw this.error('EISDIR', n.path);
    if (!Number.isSafeInteger(len) || len < 0) throw this.error('EINVAL', n.path);
    const bytes = new Uint8Array(len); if (n.bytes) bytes.set(n.bytes.subarray(0, len));
    n.blob = null; n.bytes = bytes; n.size = len; n.mtime = Date.now(); this.changed(n);
  }
  write(fd, buf, offset, length, position) {
    const h = this.handle(fd), n = h.node;
    if (!(h.flags & 3)) throw this.error('EBADF', fd);
    this.writable(n.path);
    const start = h.flags & 1024 ? n.size : position == null ? h.pos : position;
    this.range(buf, offset, length, start);
    if (start + length > n.size) this.truncate(n, start + length);
    n.bytes.set(buf.subarray(offset, offset + length), start); n.mtime = Date.now(); this.changed(n);
    if (position == null) h.pos = start + length; return length;
  }
  changed(n) { n.revision = (n.revision || 0) + 1; n.dirty = true; }
  record(n) { return {path: n.path, dir: n.dir, mtime: n.mtime, bytes: n.dir ? null : n.bytes.slice()}; }
  async commit(records = [], removed = []) {
    const paths = [...new Set([...records.map(r => r.path), ...removed])];
    try {
      await this.storage.commit(records, removed);
      this.persistence({paths, error:null});
    } catch (error) {
      const normalized = this.normalizeError(error);
      // Retail bank writes may ignore Close errors. The browser must still
      // expose failed durable publication (DESIGN_BROWSER_HOST §4 contract 2).
      this.persistence({paths, error:normalized.code});
      throw normalized;
    }
  }
  async flush(n) {
    if (!n.dirty || this.nodes.get(n.path) !== n) return;
    const revision = n.revision;
    if (this.persistent(n.path)) await this.commit([this.record(n)]);
    // writeSync cannot await the queue. Preserve writes made while the captured
    // bytes were being committed so close/fsync can publish them afterward.
    if (n.revision === revision) n.dirty = false;
  }
  async rename(a, b) {
    a = this.clean(a); b = this.clean(b); this.writable(a); this.writable(b);
    if (['/settings','/saves','/tmp'].includes(a) || ['/settings','/saves','/tmp'].includes(b)) throw this.error('EBUSY', a);
    const n = this.node(a); this.directory(this.dirname(b));
    if (a === b) return;
    if (b.startsWith(a + '/')) throw this.error('EINVAL', b);
    const target = this.nodes.get(b);
    if (target) {
      if (n.dir !== target.dir) throw this.error(n.dir ? 'ENOTDIR' : 'EISDIR', b);
      if (target.dir && target.children.size) throw this.error('ENOTEMPTY', b);
    }
    const subtree = Array.from(this.nodes.values()).filter(x => x.path === a || x.path.startsWith(a + '/'));
    const revisions = subtree.map(x => x.revision);
    const removed = subtree.map(x => x.path).filter(p => this.persistent(p));
    const stored = subtree.filter(x => this.persistent(b + x.path.slice(a.length)))
      .map(x => ({...this.record(x), path: b + x.path.slice(a.length)}));
    if (removed.length || stored.length) await this.commit(stored, removed);
    this.directory(this.dirname(a)).children.delete(this.basename(a)); this.nodes.delete(b);
    for (const [i, x] of subtree.entries()) {
      const old = x.path; this.nodes.delete(old); x.path = b + old.slice(a.length);
      if (x.revision === revisions[i]) x.dirty = false;
      this.nodes.set(x.path, x);
    }
    this.directory(this.dirname(b)).children.set(this.basename(b), n);
  }
  async remove(p, directory) {
    p = this.clean(p); this.writable(p); const n = this.node(p);
    if (['/settings','/saves','/tmp'].includes(p)) throw this.error('EBUSY', p);
    if (directory && !n.dir) throw this.error('ENOTDIR', p);
    if (!directory && n.dir) throw this.error('EISDIR', p);
    if (n.dir && n.children.size) throw this.error('ENOTEMPTY', p);
    if (this.persistent(p)) await this.commit([], [p]);
    this.nodes.delete(p); this.directory(this.dirname(p)).children.delete(this.basename(p));
  }
}
