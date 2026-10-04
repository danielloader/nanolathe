import test from 'node:test';
import assert from 'node:assert/strict';
import {installBrowserGestures} from './gestures.js';

function fixture() {
  const handlers = new Map(), timers = new Map(), events = [];
  let id = 0;
  const win = {
    innerHeight:600,
    addEventListener(type, handler, options) {
      assert.deepEqual(options, {capture:true, passive:false});
      handlers.set(type, handler);
    },
    removeEventListener(type, handler, capture) {
      assert.equal(handlers.get(type), handler); assert.equal(capture, true); handlers.delete(type);
    },
    setTimeout(fn, delay) { assert.equal(delay, 180); timers.set(++id, fn); return id; },
    clearTimeout(id) { timers.delete(id); },
  };
  const stop = installBrowserGestures(win, event => events.push(event));
  const send = (type, fields = {}) => {
    const event = {type, target:{tagName:'CANVAS', focus() {}}, clientX:400, clientY:300,
      deltaX:0, deltaY:0, deltaMode:0, ctrlKey:false, touches:[],
      prevented:false, stopped:false,
      preventDefault() { this.prevented = true; }, stopPropagation() { this.stopped = true; }, ...fields};
    handlers.get(type)(event); return event;
  };
  const quiet = () => { const tasks = [...timers.values()]; timers.clear(); for (const fn of tasks) fn(); };
  return {events, timers, handlers, win, send, quiet, stop};
}
const touch = (identifier, clientX, clientY) => ({identifier, clientX, clientY});

test('pixel scroll pans both axes once, while line/page wheels remain zoom input', () => {
  const f = fixture();
  const event = f.send('wheel', {deltaX:2.5, deltaY:-5});
  assert.equal(event.prevented, true); assert.equal(event.stopped, false);
  assert.deepEqual(f.events, [{kind:'wheel', x:-2.5, y:5, pixels:true, atX:400, atY:300}]);
  for (const deltaMode of [1,2]) {
    f.send('wheel', {deltaMode, deltaY:1});
    assert.deepEqual(f.events.at(-1), {kind:'wheel', x:-0, y:-1, pixels:false, atX:400, atY:300});
  }
  const outside = f.send('wheel', {target:{tagName:'BUTTON'}});
  assert.equal(outside.prevented, false); assert.equal(f.events.length, 3);
});

test('full-window tool ownership preserves Ebiten wheel input and retires camera gestures', () => {
  const f = fixture();
  f.send('wheel', {ctrlKey:true, deltaY:10});
  f.win.nanolatheBrowserCancelGestures();
  f.win.nanolatheBrowserGestureEnabled = false;
  const before = f.events.length;
  const wheel = f.send('wheel', {deltaY:20});
  assert.equal(wheel.prevented, true); assert.equal(wheel.stopped, false);
  f.send('touchstart', {touches:[touch(1,100,100),touch(2,200,100)]});
  assert.equal(f.events.length, before); assert.equal(f.timers.size, 0);
  f.win.nanolatheBrowserGestureEnabled = true;
  f.send('wheel', {ctrlKey:true, deltaY:10});
  assert.equal(f.events.at(-1).began, true);
  const move = f.send('mousemove', {clientX:50,clientY:80});
  assert.equal(move.prevented, false); assert.equal(move.stopped, false);
  assert.deepEqual(f.events.at(-1), {kind:'pointer',atX:50,atY:80});
  f.stop(); assert.equal(f.win.nanolatheBrowserCancelGestures, undefined);
});

test('Ctrl-wheel pinch has ordered burst boundaries and never scrolls the page or GUI', () => {
  const f = fixture();
  f.send('wheel', {ctrlKey:true, deltaY:-20});
  f.send('wheel', {ctrlKey:true, deltaY:10});
  assert.deepEqual(f.events.map(e => [e.kind,e.delta,e.began,e.ended]),
    [['pinch',.1,true,false], ['pinch',-.05,false,false]]);
  assert.equal(f.timers.size, 1);
  f.quiet();
  assert.equal(f.events.at(-1).ended, true);
  f.send('wheel', {ctrlKey:true, deltaY:-40});
  assert.equal(f.events.at(-1).began, true);
  f.send('blur');
  assert.equal(f.events.at(-1).cancelled, true);
  assert.equal(f.timers.size, 0);
  f.send('wheel', {ctrlKey:true, deltaY:-1, deltaMode:1});
  assert.equal(f.events.at(-1).delta, .08);
  f.send('wheel', {deltaY:1});
  assert.equal(f.events.at(-2).ended, true);
  assert.equal(f.events.at(-1).kind, 'wheel');
  f.stop(); assert.equal(f.handlers.size, 0); assert.equal(f.timers.size, 0);
});

test('two touches pan by midpoint and spread by distance, independent of touch order', () => {
  const f = fixture();
  f.send('touchstart', {touches:[touch(1,100,100)]});
  assert.equal(f.events.length, 0);
  f.send('touchstart', {touches:[touch(2,200,100),touch(1,100,100)]});
  assert.deepEqual(f.events[0], {kind:'pinch', atX:150, atY:100, delta:0, began:true, ended:false, cancelled:false});
  f.send('touchmove', {touches:[touch(1,120,130),touch(2,220,130)]});
  assert.deepEqual(f.events[1], {kind:'pan', x:20, y:30, atX:170, atY:130});
  assert.equal(f.events[2].delta, 0);
  f.send('touchmove', {touches:[touch(2,270,130),touch(1,70,130)]});
  assert.equal(f.events.at(-1).delta, Math.log(2)/2);
  f.send('touchend', {touches:[touch(1,70,130)]});
  assert.equal(f.events.at(-1).ended, true);
  const before = f.events.length;
  f.send('touchmove', {touches:[touch(1,80,130)]});
  assert.equal(f.events.length, before);
});

test('cancellation, a third finger and coincident touches cannot leave a stale pinch', () => {
  const f = fixture(), two = [touch(1,100,100),touch(2,100,100)];
  f.send('touchstart', {touches:two});
  f.send('touchmove', {touches:two});
  assert.equal(f.events.at(-1).kind, 'pan');
  f.send('touchcancel', {touches:two});
  assert.equal(f.events.at(-1).cancelled, true);
  f.send('touchstart', {touches:two});
  assert.equal(f.events.at(-1).began, true);
  f.send('touchstart', {touches:[...two,touch(3,200,100)]});
  assert.equal(f.events.at(-1).ended, true);
  f.send('touchend', {touches:two});
  assert.equal(f.events.at(-1).began, true);
  f.stop(); assert.equal(f.events.at(-1).cancelled, true);
  assert.ok(f.events.every(e => e.delta === undefined || Number.isFinite(e.delta)));
});
