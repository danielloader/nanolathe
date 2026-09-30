/*
 * PathLabPlayer plays a bundle written by `path-lab-render pack`: one panel
 * per log (the recorded game and its replays), kept on the same clock, so
 * the movement of a snippet's units can be compared side by side. It has
 * no dependencies and makes no network requests; the bundle carries its
 * background picture as a data URL.
 *
 *   var player = PathLabPlayer.mount(element, bundle, {speed: 4});
 *
 * Options: speed (1, 2, 4 or 8 game seconds per second; default 4),
 * autoplay (default false), loop (default false), trail (seconds of fading
 * trail behind each subject; default 3), start (tick to open at; default
 * the window's first), title (false hides the snippet line).
 *
 * Colours are read from CSS custom properties on the element or any
 * ancestor: --plp-bg, --plp-fg, --plp-muted, --plp-accent and --plp-warn,
 * and --plp-other for units that are not subjects. Each has a fallback for
 * light and dark schemes, so a host page themes the player by setting them.
 */
(function (root) {
  'use strict';

  var TPS = 30; // game ticks per second
  var SPEEDS = [1, 2, 4, 8];

  function el(tag, cls, text) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text != null) e.textContent = text;
    return e;
  }

  function prefersDark() {
    return !!(root.matchMedia && root.matchMedia('(prefers-color-scheme: dark)').matches);
  }

  function readTheme(node) {
    var cs = root.getComputedStyle(node);
    var dark = prefersDark();
    function v(name, light, darkValue) {
      var s = cs.getPropertyValue(name).trim();
      return s || (dark ? darkValue : light);
    }
    return {
      bg: v('--plp-bg', '#fcfcfb', '#1a1a19'),
      fg: v('--plp-fg', '#0b0b0b', '#f4f4f2'),
      muted: v('--plp-muted', '#62615d', '#a3a29c'),
      accent: v('--plp-accent', '#2a78d6', '#3987e5'),
      warn: v('--plp-warn', '#e5372d', '#ff453a'),
      other: v('--plp-other', '#8e959d', '#8e959d')
    };
  }

  // floorIndex is the last index whose tick is at most t, or -1.
  function floorIndex(ticks, t) {
    var lo = 0, hi = ticks.length - 1, k = -1;
    while (lo <= hi) {
      var mid = (lo + hi) >> 1;
      if (ticks[mid] <= t) { k = mid; lo = mid + 1; } else { hi = mid - 1; }
    }
    return k;
  }

  // prepareLog indexes each frame by unit so a unit's entry in a frame is
  // found without a search.
  function prepareLog(log, nUnits) {
    var ticks = log.ticks || [], frames = log.frames || [];
    var index = new Array(ticks.length);
    for (var i = 0; i < ticks.length; i++) {
      var f = frames[i] || [];
      var idx = new Int32Array(nUnits);
      idx.fill(-1);
      for (var o = 0; o + 4 < f.length; o += 5) {
        var u = f[o];
        if (u >= 0 && u < nUnits && idx[u] < 0) idx[u] = o;
      }
      index[i] = idx;
    }
    var step = Infinity;
    for (var j = 1; j < ticks.length; j++) {
      var d = ticks[j] - ticks[j - 1];
      if (d > 0 && d < step) step = d;
    }
    if (!isFinite(step)) step = 1;
    return { label: String(log.label), score: log.score || null, ticks: ticks, frames: frames, index: index, step: step };
  }

  // stateAt places unit u at tick t, on the straight line between the two
  // samples around t. Before a log's first frame a unit stands where that
  // frame shows it; a unit missing from the next frame is held for one
  // sampling interval, and at the log's end it is held.
  function stateAt(L, u, t, out) {
    var ticks = L.ticks, n = ticks.length;
    if (!n) return false;
    var k = floorIndex(ticks, t), f, o;
    if (k < 0) {
      o = L.index[0][u];
      if (o < 0) return false;
      f = L.frames[0];
      out.x = f[o + 1]; out.y = f[o + 2]; out.h = f[o + 3]; out.flags = f[o + 4];
      return true;
    }
    f = L.frames[k]; o = L.index[k][u];
    if (o < 0) return false;
    if (k + 1 < n) {
      var ob = L.index[k + 1][u], span = ticks[k + 1] - ticks[k];
      if (ob >= 0 && span <= 1.5 * L.step) {
        var g = L.frames[k + 1], a = (t - ticks[k]) / span;
        var turn = ((g[ob + 3] - f[o + 3] + 384) % 256) - 128; // the short way round
        out.x = f[o + 1] + (g[ob + 1] - f[o + 1]) * a;
        out.y = f[o + 2] + (g[ob + 2] - f[o + 2]) * a;
        out.h = f[o + 3] + turn * a;
        out.flags = f[o + 4];
        return true;
      }
      if (t - ticks[k] >= L.step) return false;
    }
    out.x = f[o + 1]; out.y = f[o + 2]; out.h = f[o + 3]; out.flags = f[o + 4];
    return true;
  }

  // trailOf lists where unit u was over the last `span` ticks before t,
  // newest first, as [x, y, age] with age 0 now and 1 at the trail's end.
  function trailOf(L, u, t, span, cur) {
    var pts = [[cur.x, cur.y, 0]];
    var ticks = L.ticks, k = floorIndex(ticks, t);
    if (k >= 0 && ticks[k] === t) k--;
    var newer = t;
    for (var j = k; j >= 0 && t - ticks[j] <= span; j--) {
      var o = L.index[j][u];
      if (o < 0 || newer - ticks[j] > 1.5 * L.step) break;
      var f = L.frames[j];
      pts.push([f[o + 1], f[o + 2], (t - ticks[j]) / span]);
      newer = ticks[j];
    }
    return pts;
  }

  function scoreLine(s) {
    if (!s) return '';
    var parts = [];
    if (s.n != null) parts.push('near ' + s.reached + '/' + s.n);
    if (s.mean_near_ticks != null) parts.push('mean ' + Math.round(s.mean_near_ticks) + ' t');
    if (s.blocked_ticks != null) parts.push('blocked ' + s.blocked_ticks + ' t');
    if (s.overlap_unit_ticks != null) parts.push('overlap ' + s.overlap_unit_ticks);
    return parts.join(' · ');
  }

  function clockText(t, t0) {
    return 't+' + ((t - t0) / TPS).toFixed(1) + ' s';
  }

  function mount(container, bundle, options) {
    if (!container || !bundle) throw new Error('PathLabPlayer.mount: an element and a bundle are required');
    var opt = options || {};
    var B = bundle;
    var t0 = B.t0, t1 = B.t1;
    var units = B.units || [];
    var logs = (B.logs || []).map(function (l) { return prepareLog(l, units.length); });
    var speed = SPEEDS.indexOf(opt.speed) >= 0 ? opt.speed : 4;
    var trailTicks = (opt.trail == null ? 3 : Math.max(0, +opt.trail)) * TPS;
    var t = opt.start != null ? Math.min(t1, Math.max(t0, +opt.start)) : t0;
    var playing = false, raf = 0, last = 0;
    var theme = readTheme(container);
    var subjects = [], others = [];
    units.forEach(function (u, i) { (u.subject ? subjects : others).push(i); });

    // The background is only ever the bundle's own embedded picture.
    var bg = new Image(), bgReady = false;
    bg.onload = function () { bgReady = true; renderAll(); };
    if (typeof B.background === 'string' && B.background.indexOf('data:image/') === 0) bg.src = B.background;

    container.classList.add('plp');
    var wrap = el('div', 'plp-root');
    if (opt.title !== false) {
      var head = [B.id, B.map, B.class].filter(Boolean).join(' · ');
      wrap.appendChild(el('div', 'plp-title', head));
    }
    var bar = el('div', 'plp-bar');
    var playBtn = el('button', 'plp-play');
    playBtn.type = 'button';
    var scrub = el('input', 'plp-scrub');
    scrub.type = 'range';
    scrub.min = String(t0); scrub.max = String(t1); scrub.step = '1';
    scrub.setAttribute('aria-label', 'Time');
    var speedSel = el('select', 'plp-speed');
    speedSel.setAttribute('aria-label', 'Speed');
    SPEEDS.forEach(function (s) {
      var o = el('option', null, s + 'x');
      o.value = String(s);
      if (s === speed) o.selected = true;
      speedSel.appendChild(o);
    });
    var clock = el('span', 'plp-clock');
    var tickLabel = el('span', 'plp-tick');
    bar.appendChild(playBtn); bar.appendChild(scrub); bar.appendChild(speedSel);
    bar.appendChild(clock); bar.appendChild(tickLabel);
    wrap.appendChild(bar);

    var panelsEl = el('div', 'plp-panels');
    var panels = logs.map(function (L) {
      var fig = el('figure', 'plp-panel');
      var cap = el('figcaption', 'plp-caption');
      cap.appendChild(el('span', 'plp-label', L.label));
      var sl = scoreLine(L.score);
      if (sl) cap.appendChild(el('span', 'plp-score', sl));
      var cv = el('canvas', 'plp-canvas');
      cv.style.aspectRatio = B.width + ' / ' + B.height;
      cv.setAttribute('role', 'img');
      cv.setAttribute('aria-label', 'Units of ' + B.id + ' in ' + L.label);
      fig.appendChild(cap); fig.appendChild(cv);
      panelsEl.appendChild(fig);
      return { log: L, canvas: cv, ctx: cv.getContext('2d'), labels: null, labelScale: 0 };
    });
    wrap.appendChild(panelsEl);
    wrap.appendChild(legend());
    container.appendChild(wrap);

    function legend() {
      var lg = el('div', 'plp-legend');
      function key(cls, text, colour) {
        var item = el('span', 'plp-key');
        var sw = el('span', 'plp-sw ' + cls);
        if (colour) sw.style.setProperty('--plp-sw', colour);
        item.appendChild(sw);
        item.appendChild(el('span', null, text));
        lg.appendChild(item);
      }
      key('plp-sw-other', 'other unit');
      key('plp-sw-blocked', 'blocked');
      key('plp-sw-goal', 'goal');
      key('plp-sw-structure', 'structure');
      key('plp-sw-feature', 'blocking feature');
      key('plp-sw-region', 'snippet region');
      subjects.forEach(function (i) { key('plp-sw-subject', String(units[i].id), units[i].color); });
      return lg;
    }

    function setPlaying(p) {
      playing = p;
      playBtn.textContent = p ? '❚❚' : '▶';
      playBtn.setAttribute('aria-label', p ? 'Pause' : 'Play');
      if (p) {
        if (t >= t1) t = t0;
        last = 0;
        raf = root.requestAnimationFrame(step);
      } else if (raf) {
        root.cancelAnimationFrame(raf);
        raf = 0;
      }
    }

    function step(now) {
      if (!playing) return;
      if (last) t += (now - last) / 1000 * TPS * speed;
      last = now;
      if (t >= t1) {
        if (opt.loop) { t = t0; } else { t = t1; renderAll(); setPlaying(false); return; }
      }
      renderAll();
      raf = root.requestAnimationFrame(step);
    }

    playBtn.addEventListener('click', function () { setPlaying(!playing); });
    scrub.addEventListener('input', function () { t = +scrub.value; last = 0; renderAll(); });
    speedSel.addEventListener('change', function () { speed = +speedSel.value; });

    // Goal labels sit beside their rings where they overlap nothing placed
    // before them; recomputed when the label size in picture pixels changes.
    function placeLabels(ctx, u) {
      var rings = (B.goals || []).map(function (g) {
        var r = goalRadius(g[0], u) + 2 * u;
        return [g[1] - r, g[2] - r, g[1] + r, g[2] + r];
      });
      function overlap(a, b) {
        var w = Math.min(a[2], b[2]) - Math.max(a[0], b[0]);
        var h = Math.min(a[3], b[3]) - Math.max(a[1], b[1]);
        return w > 0 && h > 0 ? w * h : 0;
      }
      var placed = [];
      ctx.font = 'bold ' + (12 * u) + 'px system-ui, -apple-system, "Segoe UI", sans-serif';
      (B.goals || []).forEach(function (g, gi) {
        var text = String(units[g[0]] ? units[g[0]].id : '');
        var w = ctx.measureText(text).width, h = 10 * u;
        var gap = goalRadius(g[0], u) + 4 * u, x = g[1], y = g[2];
        var cands = [[x + gap, y - h / 2], [x - gap - w, y - h / 2], [x - w / 2, y - gap - h], [x - w / 2, y + gap],
          [x + gap * 0.7, y - gap * 0.7 - h], [x - gap * 0.7 - w, y - gap * 0.7 - h], [x + gap * 0.7, y + gap * 0.7], [x - gap * 0.7 - w, y + gap * 0.7]];
        var best = null, bestCost = -1;
        for (var c = 0; c < cands.length; c++) {
          var box = [cands[c][0], cands[c][1], cands[c][0] + w, cands[c][1] + h];
          var cost = 0;
          if (box[0] < 0 || box[1] < 0 || box[2] > B.width || box[3] > B.height) cost += w * h + 1;
          for (var r = 0; r < rings.length; r++) if (r !== gi) cost += overlap(box, rings[r]);
          for (var p = 0; p < placed.length; p++) {
            var q = placed[p].box;
            cost += 4 * overlap([box[0] - 2 * u, box[1] - 2 * u, box[2] + 2 * u, box[3] + 2 * u], q);
          }
          if (bestCost < 0 || cost < bestCost) { best = box; bestCost = cost; }
          if (cost === 0) break;
        }
        placed.push({ box: best, text: text, unit: g[0] });
      });
      return placed;
    }

    function goalRadius(ui, u) {
      var un = units[ui] || { w: 12, h: 12 };
      return Math.max(5 * u, 0.42 * Math.min(un.w, un.h));
    }

    var st = { x: 0, y: 0, h: 0, flags: 0 };

    function drawUnit(ctx, ui, u, fill) {
      var un = units[ui], hw = un.w / 2, hh = un.h / 2, x = st.x, y = st.y;
      ctx.fillStyle = fill;
      ctx.fillRect(x - hw, y - hh, un.w, un.h);
      ctx.lineWidth = 1.2 * u;
      ctx.strokeStyle = 'rgba(0,0,0,0.85)';
      ctx.strokeRect(x - hw, y - hh, un.w, un.h);
      if (st.flags & 1) {
        ctx.lineWidth = 2.2 * u;
        ctx.strokeStyle = theme.warn;
        ctx.strokeRect(x - hw - 1.6 * u, y - hh - 1.6 * u, un.w + 3.2 * u, un.h + 3.2 * u);
      }
      // Heading 0 faces north (up) and a quarter turn faces west.
      var a = st.h / 256 * 2 * Math.PI, dx = -Math.sin(a), dy = -Math.cos(a), r = Math.min(hw, hh);
      ctx.lineCap = 'round';
      ctx.lineWidth = 1.7 * u;
      ctx.strokeStyle = 'rgba(0,0,0,0.9)';
      ctx.beginPath(); ctx.moveTo(x, y); ctx.lineTo(x + dx * r * 0.95, y + dy * r * 0.95); ctx.stroke();
      ctx.strokeStyle = fill;
      ctx.beginPath(); ctx.moveTo(x + dx * (r + 1.5 * u), y + dy * (r + 1.5 * u)); ctx.lineTo(x + dx * (r + 5 * u), y + dy * (r + 5 * u)); ctx.stroke();
    }

    function drawPanel(P) {
      var cv = P.canvas, ctx = P.ctx, W = cv.width, H = cv.height;
      if (!W || !H) return;
      var dpr = root.devicePixelRatio || 1;
      var k = W / B.width, u = dpr / k; // device pixels per picture pixel; picture pixels per CSS pixel
      var L = P.log;
      ctx.setTransform(1, 0, 0, 1, 0, 0);
      ctx.fillStyle = theme.bg;
      ctx.fillRect(0, 0, W, H);
      if (bgReady) ctx.drawImage(bg, 0, 0, W, H);
      ctx.setTransform(k, 0, 0, k, 0, 0);
      ctx.lineJoin = 'round';

      (B.structures || []).forEach(function (s) {
        if (s.length > 5 && !(s[4] <= t && (s[5] < 0 || t < s[5]))) return;
        ctx.fillStyle = 'rgba(8,9,11,0.6)';
        ctx.fillRect(s[0], s[1], s[2], s[3]);
        ctx.lineWidth = u;
        ctx.strokeStyle = 'rgba(232,234,237,0.8)';
        ctx.strokeRect(s[0] + u / 2, s[1] + u / 2, s[2] - u, s[3] - u);
      });

      (B.goals || []).forEach(function (g) {
        var r = goalRadius(g[0], u), col = (units[g[0]] && units[g[0]].color) || theme.accent;
        ctx.beginPath(); ctx.arc(g[1], g[2], r, 0, 2 * Math.PI);
        ctx.lineWidth = 4.2 * u; ctx.strokeStyle = 'rgba(0,0,0,0.75)'; ctx.stroke();
        ctx.lineWidth = 2.2 * u; ctx.strokeStyle = col; ctx.stroke();
      });
      if (!P.labels || P.labelScale !== u) { P.labels = placeLabels(ctx, u); P.labelScale = u; }
      ctx.font = 'bold ' + (12 * u) + 'px system-ui, -apple-system, "Segoe UI", sans-serif';
      ctx.textBaseline = 'top';
      P.labels.forEach(function (l) {
        ctx.lineWidth = 3 * u; ctx.strokeStyle = 'rgba(0,0,0,0.9)';
        ctx.strokeText(l.text, l.box[0], l.box[1]);
        ctx.fillStyle = (units[l.unit] && units[l.unit].color) || theme.accent;
        ctx.fillText(l.text, l.box[0], l.box[1]);
      });

      if (trailTicks > 0) {
        ctx.lineCap = 'round';
        ctx.lineWidth = 2.4 * u;
        subjects.forEach(function (ui) {
          if (!stateAt(L, ui, t, st)) return;
          var pts = trailOf(L, ui, t, trailTicks, st);
          ctx.strokeStyle = units[ui].color || theme.accent;
          for (var i = 0; i + 1 < pts.length; i++) {
            ctx.globalAlpha = 0.85 * (1 - pts[i + 1][2]);
            ctx.beginPath(); ctx.moveTo(pts[i][0], pts[i][1]); ctx.lineTo(pts[i + 1][0], pts[i + 1][1]); ctx.stroke();
          }
          ctx.globalAlpha = 1;
        });
      }

      others.forEach(function (ui) { if (stateAt(L, ui, t, st)) drawUnit(ctx, ui, u, theme.other); });
      subjects.forEach(function (ui) { if (stateAt(L, ui, t, st)) drawUnit(ctx, ui, u, units[ui].color || theme.accent); });
    }

    function renderAll() {
      scrub.value = String(Math.round(t));
      clock.textContent = clockText(t, t0);
      tickLabel.textContent = 'tick ' + Math.floor(t);
      panels.forEach(drawPanel);
    }

    function resize() {
      var dpr = root.devicePixelRatio || 1;
      panels.forEach(function (P) {
        var w = P.canvas.clientWidth;
        if (!w) return;
        var W = Math.max(1, Math.round(w * dpr)), H = Math.max(1, Math.round(w * dpr * B.height / B.width));
        if (P.canvas.width !== W || P.canvas.height !== H) {
          P.canvas.width = W; P.canvas.height = H;
          P.labels = null;
        }
      });
      renderAll();
    }

    function refreshTheme() {
      theme = readTheme(container);
      renderAll();
    }

    var ro = root.ResizeObserver ? new root.ResizeObserver(resize) : null;
    if (ro) panels.forEach(function (P) { ro.observe(P.canvas); });
    root.addEventListener('resize', resize);
    // Hosts switch themes by class, attribute or media query; follow all.
    var mo = root.MutationObserver ? new root.MutationObserver(refreshTheme) : null;
    if (mo) {
      mo.observe(document.documentElement, { attributes: true, attributeFilter: ['class', 'style', 'data-theme'] });
      if (document.body) mo.observe(document.body, { attributes: true, attributeFilter: ['class', 'style', 'data-theme'] });
    }
    var mq = root.matchMedia ? root.matchMedia('(prefers-color-scheme: dark)') : null;
    if (mq && mq.addEventListener) mq.addEventListener('change', refreshTheme);

    setPlaying(false);
    resize();
    if (opt.autoplay) setPlaying(true);

    return {
      element: wrap,
      play: function () { setPlaying(true); },
      pause: function () { setPlaying(false); },
      seek: function (tick) { t = Math.min(t1, Math.max(t0, +tick)); last = 0; renderAll(); },
      setSpeed: function (s) { if (SPEEDS.indexOf(s) >= 0) { speed = s; speedSel.value = String(s); } },
      refreshTheme: refreshTheme,
      destroy: function () {
        setPlaying(false);
        if (ro) ro.disconnect();
        if (mo) mo.disconnect();
        if (mq && mq.removeEventListener) mq.removeEventListener('change', refreshTheme);
        root.removeEventListener('resize', resize);
        if (wrap.parentNode) wrap.parentNode.removeChild(wrap);
        container.classList.remove('plp');
      }
    };
  }

  root.PathLabPlayer = { mount: mount };
})(typeof window !== 'undefined' ? window : this);
