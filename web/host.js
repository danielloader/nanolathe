// One runtime per origin prevents competing snapshots from overwriting saves.
export class BrowserHost {
  constructor(frame, receive) {
    this.frame = frame; this.receive = receive; this.run = null; this.module = null; this.generation = 0; this.lockTask = null;
    window.nanolatheBrowserHost = {getLaunch: id => this.run?.id === id ? this.run.config : null};
    window.addEventListener('message', event => {
      if (!this.run || event.origin !== location.origin || event.source !== frame.contentWindow ||
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
        const wasm = await fetch(new URL('../' + build.wasm, import.meta.url));
        if (!wasm.ok) throw new Error('WebAssembly download failed (' + wasm.status + ').');
        const module = wasm.headers.get('content-type')?.split(';')[0] === 'application/wasm'
          ? await WebAssembly.compileStreaming(wasm) : await WebAssembly.compile(await wasm.arrayBuffer());
        if (!/^wasm_exec\.[a-f0-9]{16}\.js$/.test(build.runtime)) throw new Error('Invalid browser runtime manifest.');
        return {module, runtime:new URL('../' + build.runtime, import.meta.url).href, version:build.version};
      })().catch(error => { this.module = null; throw error; });
    }
    return this.module;
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
  stop() {
    this.generation++; this.run = null;
    this.frame.removeAttribute('src'); this.frame.hidden = true;
    this.release();
    // Reacquisition must wait for the Web Locks callback to finish, not just
    // for our promise to resolve: an immediate ifAvailable request races it.
    return this.lockTask?.catch(() => {});
  }
  profile() { if (this.run) this.frame.contentWindow.postMessage({channel:'nanolathe',run:this.run.id,type:'heap-profile'}, location.origin); }
}
