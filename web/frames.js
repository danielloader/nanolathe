// The engine's frame gate (DESIGN_BROWSER_HOST §4 contract 10). Ebitengine
// runs one frame per animation-frame callback: the callback starts the frame's
// update and draw, and the frame's last act is to request the next callback.
// The gate records whether a frame is running and when the last one ended, and
// lets the Go background step for online play hold the next frame back while
// it runs. Install it before the Go runtime starts: the engine keeps its own
// reference to requestAnimationFrame from then on.
export function installFrameGate(win) {
  const request = win.requestAnimationFrame.bind(win);
  const now = () => win.performance.now();
  // The engine's first frame starts without a callback, so a frame counts as
  // running until the first request.
  const gate = {running:true, held:false, idleSince:now()};
  win.requestAnimationFrame = callback => {
    gate.running = false;
    gate.idleSince = now();
    const frame = time => {
      // A background step is running: start this frame at the next one.
      if (gate.held) { request(frame); return; }
      gate.running = true;
      callback(time);
    };
    return request(frame);
  };
  win.nanolatheBrowserFrames = gate;
  return gate;
}
