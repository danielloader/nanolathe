// One runtime per origin prevents competing snapshots from overwriting saves.
export class BrowserHost {
  constructor(frame, receive) {
    this.frame = frame; this.receive = receive; this.run = null; this.module = null; this.generation = 0; this.lockTask = null;
    window.nanolatheBrowserHost = {getLaunch: id => this.run?.id === id ? this.run.config : null};
    window.addEventListener('message', event => {
      if (!this.run || event.origin !== location.origin || event.source !== this.frame.contentWindow ||
          event.data?.run !== this.run?.id || event.data?.channel !== 'nanolathe') return;
      if (event.data.type === 'stopped') this.stop();
      receive(event.data.type, event.data.payload);
    });
  }
  async compiledModule() {
    if (!this.module) {
      this.module = (async () => {
        const response = await fetch(new URL('build.json', import.meta.url), {cache: 'no-cache'});
        if (!response.ok) throw new Error('Missing browser build. Run tools/browser-build first.');
        const build = await response.json();
        if (build.schema !== 1 || !/^nanolathe\.[a-f0-9]{16}\.wasm$/.test(build.wasm)) throw new Error('Invalid browser build manifest.');
        if (!/^wasm_exec\.[a-f0-9]{16}\.js$/.test(build.runtime)) throw new Error('Invalid browser runtime manifest.');
        const module = await this.compile(build);
        return {module, runtime:new URL('../' + build.runtime, import.meta.url).href, version:build.version};
      })().catch(error => { this.module = null; throw error; });
    }
    return this.module;
  }
  // The gzip variant is decompressed in the page, so a static host that does
  // not negotiate Content-Encoding for Wasm still transfers a quarter of the
  // bytes (DESIGN_BROWSER_HOST §5). A host that transparently decoded it, or
  // a browser without streams, falls back to the plain module.
  async compile(build) {
    if (build.wasm_gz === build.wasm + '.gz' && typeof DecompressionStream === 'function') {
      try {
        const gz = await fetch(new URL('../' + build.wasm_gz, import.meta.url));
        if (gz.ok && gz.body) {
          const stream = gz.body.pipeThrough(new DecompressionStream('gzip'));
          return await WebAssembly.compileStreaming(new Response(stream, {headers:{'content-type':'application/wasm'}}));
        }
      } catch {}
    }
    const wasm = await fetch(new URL('../' + build.wasm, import.meta.url));
    if (!wasm.ok) throw new Error('WebAssembly download failed (' + wasm.status + ').');
    return wasm.headers.get('content-type')?.split(';')[0] === 'application/wasm'
      ? WebAssembly.compileStreaming(wasm) : WebAssembly.compile(await wasm.arrayBuffer());
  }
  async start(config) {
    const stopped = this.stop(), generation = this.generation;
    await stopped;
    if (this.generation !== generation) throw new DOMException('Cancelled', 'AbortError');
    if (!navigator.locks) throw new Error('This experimental build requires a desktop browser with Web Locks on HTTPS. Chromium is currently verified.');
    const id = crypto.randomUUID(); this.run = {id, config: Object.freeze({...config, id})};
    await new Promise((resolve, reject) => {
      this.lockTask = navigator.locks.request('nanolathe-browser-runtime', {ifAvailable: true}, async lock => {
        if (!lock) { reject(new Error('A game is already running in another tab. Close it before starting here.')); return; }
        if (this.run?.id !== id) { reject(new DOMException('Cancelled', 'AbortError')); return; }
        await new Promise(release => {
          this.releaseLock = release;
          this.frame.hidden = false; this.frame.src = new URL('game.html?run=' + id, import.meta.url).href;
          resolve();
        });
      });
      this.lockTask.catch(reject);
    }).catch(error => { if (this.run?.id === id) this.run = null; throw error; });
  }
  release() { this.releaseLock?.(); this.releaseLock = null; }
  // Removing the element discards its browsing context synchronously, so the
  // old engine cannot commit after the lease is released. Clearing src alone
  // only queues a navigation, and a second src would also add a history entry
  // (DESIGN_BROWSER_HOST §4 contract 4).
  retireFrame() {
    const fresh = this.frame.cloneNode(false);
    fresh.removeAttribute('src'); fresh.hidden = true;
    this.frame.replaceWith(fresh); this.frame = fresh;
  }
  stop() {
    this.generation++; this.run = null;
    this.retireFrame();
    this.release();
    // Reacquisition must wait for the Web Locks callback to finish, not just
    // for our promise to resolve: an immediate ifAvailable request races it.
    return this.lockTask?.catch(() => {});
  }
  profile() { if (this.run) this.frame.contentWindow.postMessage({channel:'nanolathe',run:this.run.id,type:'heap-profile'}, location.origin); }
}
