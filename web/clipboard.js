// The clipboard bridge (DESIGN_BROWSER_HOST §4 contract 9). A page receives
// clipboard text only in a paste event, which the browser fires for its own
// paste shortcut unless the key-down was cancelled; the engine cancels every
// key-down its canvas receives. The page therefore leaves the browser's paste
// in place, hands each paste's text to the engine before its next update, and
// makes sure the engine sees one of its own paste keys in the same task:
// Ctrl+V or Shift+Insert as pressed, and anything else — Command+V, or the
// browser's menus — as Insert, the engine's other paste key [07 §2].
const isCanvas = event => event.target?.tagName === 'CANVAS';

export function installBrowserClipboard(win, deliver) {
  // Whether the engine received the key-down this paste follows.
  let engineKey = false;
  const keydown = event => {
    engineKey = false;
    if (!event.isTrusted || !isCanvas(event) || event.altKey) return;
    const v = event.code === 'KeyV';
    if (v && event.metaKey && !event.ctrlKey) {
      // The engine reads Command as no modifier in a browser and would take
      // this V as typed text. Keep the key from it; one paste per press, as
      // the engine pastes, so a held key repeats nothing.
      event.stopImmediatePropagation();
      if (event.repeat) event.preventDefault();
      return;
    }
    const paste = v && event.ctrlKey && !event.metaKey ||
      event.code === 'Insert' && event.shiftKey && !event.ctrlKey && !event.metaKey;
    if (paste && !event.repeat) {
      // The engine still receives its own paste key; only its cancellation of
      // this key-down is set aside, so the browser's paste follows it.
      engineKey = true;
      Object.defineProperty(event, 'preventDefault', {value() {}, configurable:true});
    }
  };
  const paste = event => {
    event.preventDefault();
    const data = event.clipboardData;
    // No text format is not empty text: the engine keeps its field as it is.
    deliver(data && Array.from(data.types ?? []).includes('text/plain') ? data.getData('text/plain') : null);
    if (!engineKey) {
      const canvas = win.document.querySelector('canvas');
      for (const type of ['keydown', 'keyup']) {
        canvas?.dispatchEvent(new win.KeyboardEvent(type, {code:'Insert', key:'Insert', bubbles:true, cancelable:true}));
      }
    }
    engineKey = false;
  };
  win.addEventListener('keydown', keydown, true);
  win.addEventListener('paste', paste, true);
  return () => {
    win.removeEventListener('keydown', keydown, true);
    win.removeEventListener('paste', paste, true);
  };
}
