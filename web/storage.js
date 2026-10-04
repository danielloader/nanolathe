// Origin-local settings and saves, separate namespaces for demo and retail.
export class BrowserStorage {
  constructor(name, changed = () => {}) {
    this.name = name; this.changed = changed;
    this.ready = new Promise((resolve, reject) => {
      const request = indexedDB.open(name, 1);
      let failed = false;
      request.onupgradeneeded = () => request.result.createObjectStore('files', {keyPath:'path'});
      request.onsuccess = () => {
        if (failed) { request.result.close(); return; }
        request.result.onversionchange = () => request.result.close(); resolve(request.result);
      };
      request.onerror = () => { failed = true; reject(request.error); };
      request.onblocked = () => { failed = true; reject(new Error('Close other Nanolathe tabs to open local storage.')); };
    });
    // A caller can render an availability error without an unhandled rejection.
    this.ready.catch(() => {});
  }
  close() { this.ready.then(db => db.close()).catch(() => {}); }
  async records() {
    const db = await this.ready;
    return new Promise((resolve,reject) => {
      const tx = db.transaction('files','readonly'), request = tx.objectStore('files').getAll();
      tx.oncomplete = () => resolve(request.result);
      tx.onabort = () => reject(tx.error || request.error);
    });
  }
  async commit(records = [], removed = []) {
    const db = await this.ready;
    await new Promise((resolve,reject) => {
      const tx = db.transaction('files','readwrite',{durability:'strict'}), store = tx.objectStore('files');
      tx.oncomplete = resolve;
      tx.onabort = () => reject(tx.error || new Error('Local storage transaction aborted.'));
      try {
        for (const path of removed) store.delete(path);
        for (const record of records) store.put(record);
      } catch (error) {
        // A synchronous request error does not abort IndexedDB automatically.
        // Roll back earlier requests too (DESIGN_BROWSER_HOST §4 contract 2).
        tx.abort(); reject(error);
      }
    });
    this.changed();
  }
}
