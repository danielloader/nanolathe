import test from 'node:test';
import assert from 'node:assert/strict';
import {installFrameGate} from './frames.js';

function fixture() {
  const pending = [];
  let clock = 1000;
  const win = {
    performance:{now:() => clock},
    requestAnimationFrame(callback) { pending.push(callback); return pending.length; },
  };
  const gate = installFrameGate(win);
  // Run the animation frames the browser has queued, as one refresh does.
  const refresh = (at = clock + 16) => { clock = at; for (const callback of pending.splice(0)) callback(at); };
  return {win, gate, pending, refresh, advance:ms => { clock += ms; }};
}

test('a frame runs from its callback until it requests the next', () => {
  const f = fixture();
  assert.equal(f.win.nanolatheBrowserFrames, f.gate);
  // The engine's first frame runs before any request.
  assert.equal(f.gate.running, true);
  let frames = 0;
  const engineFrame = () => { frames++; assert.equal(f.gate.running, true); };
  f.advance(5);
  f.win.requestAnimationFrame(engineFrame);
  assert.deepEqual([f.gate.running, f.gate.idleSince], [false, 1005]);
  f.refresh(1021);
  assert.deepEqual([frames, f.gate.running], [1, true]);
  // The next request ends that frame.
  f.advance(3);
  f.win.requestAnimationFrame(engineFrame);
  assert.deepEqual([f.gate.running, f.gate.idleSince], [false, 1024]);
});

test('a held gate starts no frame until it is released', () => {
  const f = fixture();
  let frames = 0;
  f.win.requestAnimationFrame(() => { frames++; });
  f.gate.held = true;
  f.refresh();
  f.refresh();
  assert.deepEqual([frames, f.gate.running, f.pending.length], [0, false, 1]);
  f.gate.held = false;
  f.refresh();
  assert.deepEqual([frames, f.gate.running, f.pending.length], [1, true, 0]);
});
