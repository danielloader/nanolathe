# path-lab-render

Draws what a pathfinding-laboratory snippet's units did, so a reviewer can
judge movement by eye: the recorded game beside replays of the same snippet
under named rule sets, on the ground they moved over. It writes still
pictures, a video, or a compact bundle for the HTML player in `player/`.

Everything it reads and writes is local research data. Recordings, frames,
maps and the outputs never enter the repository.

```
go build -o /tmp/path-lab-render ./tools/path-lab-render
path-lab-render tracks|sheet|video|pack [flags]
```

## Inputs

| Flag | What |
|---|---|
| `-snippet FILE` | snippet written by `path-lab snippet` (required) |
| `-map DIR` | map directory written by `nanolathe-pathlab map`: `terrain.png` and `grid.json.gz` (required) |
| `-log LABEL=FILE` | a frames file written by `path-lab frames`, repeated; panels follow the order given (at least one) |
| `-units FILE` | unit table (`units-ota.json`); footprints come from `foot_x`/`foot_z`, and units it lacks are drawn 2x2 cells |
| `-score FILE` | score file written by `path-lab score -out`; a log's line is the entry whose `log` or `rules` is the label, else the one named in the frames file's `<snippet>__<log>.` suffix |
| `-margin N` | world units shown around the snippet region (default 96) |
| `-scale F` | output pixels per world unit; the default makes a panel about 480 pixels wide, within 0.25..2 |
| `-out FILE` | output file |

Frames and JSON inputs may be gzip-compressed or not.

## Subcommands

- `tracks -out FILE.png` — one panel per log, side by side: every subject's
  path over the whole window in its colour, blocked stretches thicker in red,
  a dot where it started and a ring, labelled with its unit ID, at its goal
  (the goal of its first order in the window that was not already under way,
  the order `path-lab score` measures). Other units' paths are thin grey
  lines. Title: the label and, with `-score`, `near 12/13  mean 912 t
  blocked 2389 t  overlap 0` (subjects that came within 96 of their goal,
  mean ticks to get there, blocked ticks, overlapping unit-ticks).
- `sheet -out FILE.png [-frames 4 | -ticks LIST]` — a contact sheet, one row
  per log and one column per moment: every unit at that moment as a square
  the size of its footprint (subjects in colour, others grey, blocked units
  outlined in red, a mark toward its heading), with goal rings. `-frames N`
  spreads N moments over the window, both ends included (one moment is the
  window's end). `-ticks` takes recording ticks (`91500`), ticks after the
  window opens (`+300`) or seconds after it opens (`10s`), comma-separated.
- `video -out FILE.mp4|FILE.gif [-speed 4] [-fps 30]` — the sheet's picture
  animated, every log side by side, with a clock and a two-second fading
  trail behind each subject. `-speed` is game seconds per video second. The
  MP4 is H.264 in yuv420p, encoded by the `ffmpeg` on `PATH`; a `.gif` needs
  no ffmpeg and defaults to 15 frames a second. The last moment is held for
  a second.
- `pack -out FILE.json [-js FILE.js] [-quality 70]` — the bundle for the HTML
  player. `-js` writes the same bundle as a script that pushes it onto
  `window.PATHLAB_BUNDLES`, for pages opened from disk, where a page may not
  fetch local files.

Sample, with the laboratory data under `~/nanolathe-bench/path-redesign`:

```
D=$HOME/nanolathe-bench/path-redesign
L="-log recorded=$D/out/replay-test/test-pd-13veh__recorded.frames.json.gz -log strict-3.1=$D/out/replay-test/test-pd-13veh__strict-3.1.frames.json.gz -log modern=$D/out/replay-test/test-pd-13veh__modern.frames.json.gz"
C="-snippet $D/data/snippets/test-pd-13veh.json -map $D/data/maps/painted_desert -units $D/data/units-ota.json -score $D/out/replay-test/test-pd-13veh.score.json"
/tmp/path-lab-render tracks $C $L -out $D/out/render-test/tracks.png
/tmp/path-lab-render sheet  $C $L -frames 4 -out $D/out/render-test/sheet.png
/tmp/path-lab-render video  $C $L -speed 6 -out $D/out/render-test/compare.mp4
/tmp/path-lab-render pack   $C $L -out $D/out/render-test/bundle.json -js $D/out/render-test/bundle.js
```

(In zsh, write `${=C} ${=L}` so the variables split into words.)

## Reading the pictures

- **Ground.** `terrain.png` is the map's pre-rendered ground in screen space:
  ground of height *h* at world (x, z) is drawn at picture pixel
  (x, z − h/2), *h* being the height byte of the cell holding the point.
  Units, structures, goals and the snippet region are placed the same way,
  so they sit where the game draws them. A unit crossing a cell border on a
  slope therefore steps by half the height difference. The terrain is
  dimmed and desaturated so the overlays read.
- **Blocking features** are small pale squares, one per cell. The grid marks
  a blocking feature's anchor cell and the fringe cells to its right and
  below. A feature the recording shows reclaimed before the window opened
  is left out, as the replay removes it, whichever of its cells the reclaim
  names. Features that do not block are not drawn.
- **Structures** are dark squares with a light edge, sized from the unit
  table and centred where the snippet places them. `tracks` draws every
  structure that stands at some time in the window; the moments draw those
  standing then.
- **Colours.** Each subject has its own colour, the same in every panel,
  picture, video and bundle of a snippet: subjects are ranked by unit ID
  and take the palette in order. The palette is chosen so the first colours
  are the most distinct, including for readers with red-green colour
  deficiency; with many subjects some colours are close, so goal rings carry
  the unit ID and the legend lists every ID. Past sixteen subjects the
  colours repeat. Red means blocked and grey means a unit that is not a
  subject; neither is a subject colour.
- **Between samples** a unit is placed on the straight line between its two
  neighbouring samples, turning the short way. Before a log's first frame a
  unit stands where that frame shows it (a replay's first frame can come a
  few ticks after the window opens). A unit missing from later frames is
  held for one sampling interval and then gone; at a log's end the units it
  still shows stay put.
- **Blocked** is the frames file's flag. `tracks` draws red through each run
  of consecutive blocked samples (a lone one is a red dot); a moment between
  samples takes the flag of the sample before it.

## Bundle

```
{id, map, class, t0, t1, scale, width, height,
 background: "data:image/jpeg;base64,...",
 structures: [[x, y, w, h], ...],
 goals: [[unitIndex, x, y], ...],
 units: [{id, name, subject, w, h, color}, ...],
 logs: [{label, score: {...}, ticks: [...], frames: [[...], ...]}, ...]}
```

Coordinates are pixels of the background picture (`width` x `height`, the
terrain at `scale` pixels per world unit), rounded to integers. The
background already carries the region outline and the blocking features.
A structure is its top-left corner and size; one that does not stand for
the whole window adds its first tick and the tick it is gone (`-1` for
never). `units` lists the subjects first, in colour order, each with its
`color`, then every other unit a log shows. `frames[i]` holds, for tick
`ticks[i]`, `[unitIndex, x, y, heading, flags]` flattened for every unit in
view, where heading is the 16-bit heading divided by 256 (0 faces north, 64
west) and flags bit 0 is blocked and bit 1 moving. `score` holds the score
file's aggregate fields for the log. The sample snippet's bundle, three
logs, is about 0.8 MB.

## HTML player

`player/player.js` (no dependencies, no network requests) and
`player/player.css`:

```html
<link rel="stylesheet" href="player.css">
<script src="player.js"></script>
<script src="bundle.js"></script>
<div id="p"></div>
<script>
  PathLabPlayer.mount(document.getElementById('p'), window.PATHLAB_BUNDLES[0], {speed: 4});
</script>
```

`mount(element, bundle, options)` draws one canvas per log, all on one
clock, with play/pause, a scrubber, speed (1x, 2x, 4x, 8x), the clock in
game seconds, each panel's label and score line, and a legend. Subjects are
footprint squares in their colours with a fading trail, other units grey,
blocked units outlined in red, with structures and labelled goal rings.
Units move between samples as in the pictures. Panels wrap on narrow
screens and canvases follow the device pixel ratio.

Options: `speed` (default 4), `autoplay` (false), `loop` (false), `trail`
(seconds, default 3; 0 for none), `start` (tick to open at), `title` (false
hides the snippet line). It returns `{play, pause, seek(tick),
setSpeed(s), refreshTheme(), destroy()}`.

Colours come from CSS custom properties on the element or an ancestor:
`--plp-bg`, `--plp-fg`, `--plp-muted`, `--plp-accent`, `--plp-warn`, and
`--plp-other` for units that are not subjects. Each falls back to a light or
dark value by the reader's colour scheme. The player follows changes to the
`class`, `style` or `data-theme` attribute of `<html>` and `<body>`, and to
the colour scheme; call `refreshTheme()` after any other change.

`player/demo.html?bundle=PATH` shows a bundle, `PATH` relative to the page.
Served over HTTP it fetches a `.json` bundle; opened from disk, where that
fetch is refused, it loads the `.js` bundle beside it (`pack -js`) instead,
or the `.js` named directly. Optional: `&t=SECONDS` to open at, `&speed=`,
`&autoplay=1`.
