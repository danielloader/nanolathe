import {demoManifest, loadDemo, importedContent} from './content.js';
import {BrowserHost} from './host.js';
import {BrowserStorage} from './storage.js';
import {collectEntry} from './folder.js';

const $ = id => document.getElementById(id);
let content = null, manifest = null, storage = null, storageEpoch = 0, loading = null, launching = false, launchEpoch = 0;
let measurement = null, profiling = false, pendingMeasureTick = null, activeConfig = null, performanceRuns = [];
let storedURLs = [], heapURL, measurementsURL;
const failedWrites = new Map();
const setStatus = text => { $('status').textContent = text; };
const appendLog = text => { $('log').textContent = ($('log').textContent + text + '\n').slice(-40000); };
const host = new BrowserHost($('game'), (type, payload) => {
  if (type === 'status') setStatus(payload);
  if (type === 'log') appendLog(payload);
  if (type === 'storage') refreshStorage();
  if (type === 'persistence') persistence(payload);
  if (type === 'sample') sample(payload);
  if (type === 'profile') profileReady(payload);
  if (type === 'stopped') { retireView(); measurement = null; profiling = false; pendingMeasureTick = null; diagnostics(false); setStatus(payload); }
});
const modulePromise = () => host.compiledModule();
function diagnostics(ready) {
  $('measure').disabled = !ready || profiling || Boolean(measurement);
  $('heap').disabled = !ready || profiling || Boolean(measurement);
}
function selectContent(next) {
  storage?.close(); content = next; failedWrites.clear(); $('storage-error').hidden = true;
  // Preserve existing prototype retail saves. Demo data has a separate namespace.
  storage = new BrowserStorage(next.kind === 'retail' ? 'nanolathe-browser-poc' : 'nanolathe-browser-demo-v1');
  activeConfig = null; measurement = null; pendingMeasureTick = null;
  $('content-name').textContent = next.name;
  const demo = next.kind === 'demo';
  for (const option of $('entry').options) option.disabled = demo ? ['skirmish','stress'].includes(option.value) : option.value === 'demo';
  $('entry').value = demo ? 'demo' : 'menus'; entryChanged();
  refreshStorage();
}
function entryChanged() {
  $('maplabel').hidden = !['skirmish','stress'].includes($('entry').value);
  if ($('entry').value === 'stress') $('map').value = 'town & country';
}
$('entry').onchange = entryChanged;
async function start() {
  if (!content) return;
  const epoch = ++launchEpoch;
  launching = true; $('start').disabled = true;
  try {
    measurement = null; profiling = false; diagnostics(false); $('metrics').textContent = '';
    pendingMeasureTick = $('entry').value === 'stress' && $('auto-measure').checked ? 750 : null;
    activeConfig = {entry:$('entry').value, map:['skirmish','stress'].includes($('entry').value) ? $('map').value : null,
      seed:['skirmish','stress'].includes($('entry').value) ? 7 : null, memory:$('memory').value, content:content.kind};
    // Begin fetching/compiling before the iframe asks for the module.
    const module = modulePromise(); module.catch(() => {});
    setStatus('Preparing ' + content.name + '…');
    await host.start({...activeConfig, renderer:$('renderer').value, name:content.name,
      storage:storage.name, files:content.files, module});
    if (epoch !== launchEpoch) return;
    $('welcome').hidden = true; $('controls').hidden = false; $('game-view').hidden = false;
  } catch (error) { if (epoch === launchEpoch && error.name !== 'AbortError') { setStatus(error.message); appendLog(error.message); } }
  finally { if (epoch === launchEpoch) { launching = false; $('start').disabled = false; } }
}
function retireView() {
  if (document.fullscreenElement === $('game-view')) document.exitFullscreen().catch(() => {});
  $('game-view').hidden = true;
}
function back() {
  launchEpoch++; launching = false; $('start').disabled = false;
  loading?.abort(); loading = null; host.stop(); diagnostics(false);
  measurement = null; profiling = false; pendingMeasureTick = null;
  $('welcome').hidden = false; $('controls').hidden = true; retireView();
  $('demo').disabled = !manifest; $('demo').textContent = 'Try the demo';
  $('measure').textContent = 'Measure 20 seconds'; setStatus('Choose the demo or your game folder.');
}
$('start').onclick = start;
$('stop').onclick = back;
$('fullscreen').onclick = () => $('game-view').requestFullscreen().catch(error => setStatus(error.message));
$('leave-fullscreen').onclick = () => document.exitFullscreen().catch(error => setStatus(error.message));
document.addEventListener('fullscreenchange', () => {
  // Keep durable-write failures visible inside the fullscreen game.
  if (document.fullscreenElement === $('game-view')) $('game-view').append($('storage-error'));
  else $('status').after($('storage-error'));
});
$('demo').onclick = async () => {
  if (!manifest || loading || launching) return;
  const controller = new AbortController(); loading = controller; $('demo').disabled = true;
  $('controls').hidden = false; $('start').disabled = true;
  try {
    modulePromise().catch(() => {});
    const next = await loadDemo(manifest, controller.signal, (done, total) => { if (loading === controller && !controller.signal.aborted) setStatus(
      'Preparing demo: ' + (done/1024**2).toFixed(1) + ' / ' + (total/1024**2).toFixed(1) + ' MiB'); });
    controller.signal.throwIfAborted(); selectContent(next); await start();
  } catch (error) { if (loading === controller && error.name !== 'AbortError') { setStatus(error.message); appendLog(error.message); } }
  finally {
    if (loading === controller) { loading = null; $('demo').disabled = false; $('start').disabled = false; }
  }
};
async function accept(files) {
  loading?.abort(); loading = null;
  let epoch = launchEpoch;
  try {
    const next = importedContent(files);
    epoch = ++launchEpoch; launching = false;
    await host.stop();
    if (epoch !== launchEpoch) return;
    selectContent(next); await start();
  } catch (error) { if (epoch === launchEpoch) setStatus(error.message); }
}
$('folder').onchange = event => {
  const files = Array.from(event.target.files, file => [file.webkitRelativePath || file.name, file]);
  event.target.value = ''; // Selecting the same folder again must still dispatch.
  if (files.length) accept(files);
};
$('drop').ondragover = event => { event.preventDefault(); $('drop').classList.add('over'); };
$('drop').ondragleave = () => $('drop').classList.remove('over');
$('drop').ondrop = async event => {
  event.preventDefault(); $('drop').classList.remove('over'); setStatus('Reading folder names…');
  const epoch = ++launchEpoch; launching = false;
  loading?.abort(); loading = null;
  try {
    const entries = Array.from(event.dataTransfer.items, item => item.webkitGetAsEntry?.()).filter(Boolean);
    const files = [];
    for (const entry of entries) files.push(...await collectEntry(entry));
    if (epoch === launchEpoch) await accept(files);
  } catch (error) { if (epoch === launchEpoch) setStatus(error.message); }
};
function sample(data) {
  if (pendingMeasureTick !== null && data.tick >= pendingMeasureTick) {
    pendingMeasureTick = null; measurement = {started:performance.now(), samples:[]}; $('measure').textContent = 'Measuring…';
  }
  diagnostics(true);
  if (!measurement && !profiling) setStatus('Running ' + content.name + '. Click the game to focus it.');
  $('metrics').textContent = data.renderer + ' | ' + data.width + '×' + data.height +
    ' | ' + data.fps.toFixed(1) + ' FPS | ' + data.tps.toFixed(1) + ' ticks/s | ' + data.units +
    ' units | tick ' + data.tick + ' | Go heap ' + data.heap_mib.toFixed(0) + ' MiB / Wasm ' +
    data.wasm_mib.toFixed(0) + ' MiB | draw ' + data.draw_ms.toFixed(1) + ' ms | sim ' + data.sim_ms.toFixed(1) + ' ms';
  if (!measurement) return;
  measurement.samples.push(data);
  if (performance.now() - measurement.started < 20000) return;
  const samples = measurement.samples, avg = key => samples.reduce((n,s) => n+s[key], 0)/samples.length;
  const result = {...activeConfig, renderer:data.renderer, resolution:[data.width,data.height],
    seconds:(performance.now()-measurement.started)/1000, samples:samples.length,
    avg_fps:avg('fps'), avg_tps:avg('tps'), avg_draw_ms:avg('draw_ms'), avg_sim_ms:avg('sim_ms'),
    peak_heap_mib:Math.max(...samples.map(s => s.heap_mib)), peak_wasm_mib:Math.max(...samples.map(s => s.wasm_mib)),
    units:[samples[0].units,data.units], ticks:[samples[0].tick,data.tick], samples_detail:samples};
  performanceRuns.push(result); appendLog(JSON.stringify(result)); measurement = null;
  $('measure').textContent = 'Measure 20 seconds'; diagnostics(true);
}
$('measure').onclick = () => {
  pendingMeasureTick = null; measurement = {started:performance.now(), samples:[]};
  $('measure').textContent = 'Measuring…'; diagnostics(true);
};
$('export').onclick = () => {
  if (measurementsURL) URL.revokeObjectURL(measurementsURL);
  measurementsURL = URL.createObjectURL(new Blob([JSON.stringify(performanceRuns,null,2)], {type:'application/json'}));
  $('measure-download').href = measurementsURL; $('measure-download').download = 'nanolathe-browser-performance.json'; $('measure-download').hidden = false;
};
$('heap').onclick = () => { profiling = true; diagnostics(true); setStatus('Recording heap profile…'); host.profile(); };
function profileReady({bytes,error}) {
  profiling = false; diagnostics(true);
  if (error) { setStatus('Heap profile failed: ' + error); return; }
  if (heapURL) URL.revokeObjectURL(heapURL);
  heapURL = URL.createObjectURL(new Blob([bytes], {type:'text/plain'}));
  $('heapdownload').href = heapURL; $('heapdownload').download = 'nanolathe-browser-heap.txt';
  $('heaptext').textContent = new TextDecoder().decode(bytes); $('profile').hidden = false;
  setStatus('Heap profile ready. Click the game to resume.');
}
function persistence({paths, error}) {
  for (const path of paths) {
    if (error) failedWrites.set(path, error); else failedWrites.delete(path);
  }
  $('storage-error').hidden = !failedWrites.size;
  $('storage-error').textContent = 'Browser storage failed for ' +
    [...failedWrites].map(([path, code]) => path.split('/').pop() + ' (' + code + ')').join(', ') +
    '. Your latest changes have not been stored. Try saving again before restarting; the previous browser save may be older.';
  if (error) appendLog('nanolathe: browser storage commit failed: logical path ' + paths.join(', ') +
    ', providers searched [IndexedDB], expected committed browser files (' + error + ')');
}
async function refreshStorage() {
  const epoch = ++storageEpoch, source = storage;
  if (!source) return;
  try {
    const records = (await source.records()).filter(r => !r.dir && !r.path.split('/').pop().startsWith('.'));
    if (epoch !== storageEpoch) return;
    storedURLs.forEach(url => URL.revokeObjectURL(url)); storedURLs = []; $('stored').replaceChildren();
    const info = document.createElement('p');
    info.textContent = content.name + ': ' + records.filter(r => /\.sav$/i.test(r.path)).length +
      ' saved games. Stored in this browser at this address.';
    $('stored').append(info);
    for (const record of records) {
      const link = document.createElement('a'), name = record.path.split('/').pop();
      link.textContent = 'Download ' + name; link.download = name;
      link.href = URL.createObjectURL(new Blob([record.bytes])); storedURLs.push(link.href); $('stored').append(link, ' · ');
    }
  } catch (error) { if (epoch === storageEpoch) $('stored').textContent = 'Local storage unavailable: ' + error.message; }
}
try {
  manifest = await demoManifest(new URL('demo/manifest.json', location.href));
  $('demo').disabled = !manifest;
  $('demo').textContent = manifest ? 'Try the demo' : 'Demo unavailable in this build';
  $('demo-info').textContent = manifest ? 'Three campaign missions · ' +
    (manifest.files.reduce((n,f) => n+f.size,0)/1024**2).toFixed(1) + ' MiB, cached for your next visit' : 'You can still play using your own folder.';
  setStatus('Choose the demo or your game folder.');
} catch (error) { $('demo').textContent = 'Demo unavailable'; $('demo-info').textContent = error.message; setStatus('Choose your game folder to play.'); }
