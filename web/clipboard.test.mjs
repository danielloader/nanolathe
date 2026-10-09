import test from 'node:test';
import assert from 'node:assert/strict';
import {installBrowserClipboard} from './clipboard.js';

function fixture() {
  const handlers = new Map(), delivered = [], replayed = [];
  const canvas = {tagName:'CANVAS', dispatchEvent(event) { replayed.push([event.type, event.init.code, event.init.key]); }};
  const win = {
    document:{querySelector:selector => (assert.equal(selector, 'canvas'), canvas)},
    KeyboardEvent:class { constructor(type, init) { this.type = type; this.init = init; } },
    addEventListener(type, handler, capture) { assert.equal(capture, true); handlers.set(type, handler); },
    removeEventListener(type, handler, capture) { assert.equal(handlers.get(type), handler); assert.equal(capture, true); handlers.delete(type); },
  };
  const stop = installBrowserClipboard(win, text => delivered.push(text));
  // An engine canvas listener that cancels every key-down, as Ebitengine's does.
  const key = fields => {
    const event = {type:'keydown', isTrusted:true, target:canvas, code:'KeyV', ctrlKey:false, metaKey:false,
      shiftKey:false, altKey:false, repeat:false, defaultPrevented:false, engine:true,
      preventDefault() { this.defaultPrevented = true; }, stopImmediatePropagation() { this.engine = false; }, ...fields};
    handlers.get('keydown')(event);
    if (event.engine) event.preventDefault();
    return event;
  };
  const paste = (text = 'k7m-2px', types = ['text/plain']) => {
    const event = {type:'paste', prevented:false, preventDefault() { this.prevented = true; },
      clipboardData:{types, getData:type => (assert.equal(type, 'text/plain'), text)}};
    handlers.get('paste')(event);
    return event;
  };
  return {handlers, delivered, replayed, key, paste, stop};
}

test('Ctrl+V and Shift+Insert reach the engine and the browser still pastes', () => {
  for (const fields of [{ctrlKey:true}, {code:'Insert', shiftKey:true}]) {
    const f = fixture();
    const down = f.key(fields);
    assert.equal(down.engine, true, 'the engine saw its paste key');
    assert.equal(down.defaultPrevented, false, 'the browser still pastes');
    assert.equal(f.paste().prevented, true);
    assert.deepEqual(f.delivered, ['k7m-2px']);
    assert.deepEqual(f.replayed, [], 'the engine already has its paste key');
  }
});

test('Command+V and menu pastes reach the engine as Insert', () => {
  const f = fixture();
  const down = f.key({metaKey:true});
  assert.deepEqual([down.engine, down.defaultPrevented], [false, false], 'kept from the engine, pasted by the browser');
  f.paste();
  assert.deepEqual(f.delivered, ['k7m-2px']);
  assert.deepEqual(f.replayed, [['keydown', 'Insert', 'Insert'], ['keyup', 'Insert', 'Insert']]);
  // A paste from the browser's menus follows no key at all.
  f.key({code:'KeyA'});
  f.paste('CFH234');
  assert.deepEqual(f.delivered, ['k7m-2px', 'CFH234']);
  assert.equal(f.replayed.length, 4);
});

test('held paste keys paste once, and other keys keep their default handling', () => {
  const f = fixture();
  const repeatCtrl = f.key({ctrlKey:true, repeat:true});
  assert.deepEqual([repeatCtrl.engine, repeatCtrl.defaultPrevented], [true, true], 'a repeated Ctrl+V pastes nothing');
  const repeatCommand = f.key({metaKey:true, repeat:true});
  assert.deepEqual([repeatCommand.engine, repeatCommand.defaultPrevented], [false, true], 'a repeated Command+V types and pastes nothing');
  for (const fields of [{code:'KeyV'}, {ctrlKey:true, altKey:true}, {ctrlKey:true, metaKey:true}, {code:'Insert'},
    {ctrlKey:true, isTrusted:false}, {ctrlKey:true, target:{tagName:'BODY'}}]) {
    const event = f.key(fields);
    assert.deepEqual([event.engine, event.defaultPrevented], [true, true], JSON.stringify(fields));
  }
  assert.deepEqual(f.delivered, []);
});

test('a paste with no text leaves the engine field alone', () => {
  const f = fixture();
  f.key({ctrlKey:true});
  f.paste('', ['image/png']);
  f.key({ctrlKey:true});
  f.paste('');
  assert.deepEqual(f.delivered, [null, '']);
  f.stop();
  assert.equal(f.handlers.size, 0);
});
