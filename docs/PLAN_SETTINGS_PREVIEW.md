# Settings preview loading and content selection

User request: improve settings-background asset loading and keep previews useful
when a mod replaces the unit catalog. This work changes settings presentation,
not battle rules, retail evidence, save data or the film capture fixtures.

Base: main at cdcb1279. The settings-preview integration branch combined two
independent implementation units from reviewed composition seams. The user
approved squash landing on main on 2026-09-30 after testing the combined build.

## Shared interface

- `(*contentSet).nlPreviewCatalog() (*content.Catalog, error)` returns one
  immutable authored catalog per content set, shared by preview construction and
  mutator examples. The loading unit owns its implementation. Cloned battle
  catalogs, mutator transforms and phase-7 texture cursors remain session-owned.
- `stageNLPreviewFixture(preset nlPreset, sess *session.Session, authored
  *content.Catalog) (*nlStage, error)` stages a settings-only fixture. The roster
  unit owns it. The supplied authored catalog precedes rule preparation and
  mutators, so compared scenes choose the same units. A nil authored catalog may
  use the session catalog as a fixture fallback.
- `(*nlStage).assetUnitNames() []string` lists every unit the fixture may create:
  initial armies, commanders, offscreen sentinels/storage, placement ghosts,
  scheduled wrecks, builders and all queued products. The roster unit owns it.
  Canonical sorted unique names are required. Nil means conservative full
  catalog preparation; empty means no units. The loading unit derives linked
  weapons, projectile models and corpse/successor art from these roots.
- `composeBattleEntryDetachedWithModels` accepts a registry prepared by the
  preview loader. Root owns this small adaptation in battle.go. The existing
  composeBattleEntryDetached wrapper passes nil and retains full preparation
  for every ordinary battle.

## Loading unit

Owns cmd/nanolathe/content.go, cmd/nanolathe/nlscreen_preview.go,
cmd/nanolathe/nlscreen_loading.go, cmd/nanolathe/nlscreen_loading_test.go,
internal/client/model_textures.go, internal/client/model_texture_scope.go,
internal/client/model_texture_scope_test.go, internal/client/feature_warm_scope.go,
internal/client/feature_warm_scope_test.go.

Cache authored catalog compilation for one content set; remove duplicate
texture-bank decoding; add preview-only asset preparation from the supplied unit
roots where it preserves model/texture resolution, animation ownership and
feature dependency closure. Keep ordinary battle entry unchanged. Do not crop
the authoritative terrain or add filesystem reads to simulation ticks. A first
usable increment may retain full texture-bank decoding and full map/detail art
if deeper scoping requires more infrastructure; report those remaining costs.

## Content roster unit

Owns cmd/nanolathe/nlscreen_roster.go, cmd/nanolathe/nlscreen_roster_test.go,
cmd/nanolathe/nlscreen_roster_retail_test.go, cmd/nanolathe/nlscreen_scenes.go,
cmd/nanolathe/nlscreen_scenes_test.go, cmd/nanolathe/nlscreen_stats.go,
cmd/nanolathe/nlscreen_pages.go, cmd/nanolathe/nlscreen_demo.go.

Preserve working stock compositions. Select deterministic substitutes by actual
catalog capabilities for missing stock units; derive faction identities from
SIDEDATA. Adapt construction products, unit-dependent spacing and artillery
range/timing where necessary. Use the same resolved units for portraits and
mutator examples. Missing capabilities get a compatible scene and an explicit
explanation, rather than silently creating an empty demonstration. Do not add
mod-specific unit tables or change film/benchmark fixtures or gameplay rules.

## Integration and review

Root owns this plan and the design-document updates. Agents do not modify design
documents; they report their precise contracts, evidence, remaining limits and
commands. Each agent commits its own unit and stops before landing. Root reads
both diffs, runs affected checks independently, merges the commits into the
integration worktree, captures and inspects stock and installed TA Zero previews,
checks loading timing and applies relevant performance/design gates. A runnable
integration build and concise manual test steps are delivered for user testing.
Main is not landed as part of the initial test handoff.

## Progress

- Composition seams prepared; stock behavior is unchanged.
- Focused stock scene and citation baseline checks passed.
- Loading and roster agents dispatched in separate worktrees with exclusive
  file ownership. Root handles the ordinary-battle wrapper, portrait and
  capability-limit UI wiring and design-document updates.
- Stock and installed TA Zero cache captures recorded outside the repository.
  Stock first scene/request-to-frame was about 1.65 seconds, the next scene
  1.45 seconds and its cached revisit 5 milliseconds. TA Zero reproduces the
  absent army and empty stock-name portraits.
- Classic and Enhanced live-battle baselines captured sequentially with matching
  scene metadata and workload, including moving units, construction, projectiles
  and burning features. Candidate comparison follows integration.
- Settings and presentation design contracts updated; documentation checks pass.
- Both implementation commits integrated and reviewed independently. Scoped art
  preserves definition identities, texture precedence and independent animation
  cursors; roster checks pass against stock and the installed TA Zero package.
- Candidate stock cache captures show 1.21 seconds to the first frame and
  0.84 seconds to the next scene, against 1.65 and 1.45 seconds at the baseline.
  Cached revisit remains a few milliseconds. These are single local samples,
  with remaster loading disabled in both builds, not a latency guarantee.
- Candidate TA Zero captures show fighting units, faction commanders and mod
  portraits. Reviewed forward corrections preserve full authored membership and
  valid slot zero, remove the incidental placement capability note, and provide
  visible ground combat when naval or aircraft roles are absent.
- Classic and Enhanced benchmark scene metadata, opening/end state exports and
  captured PNGs match the baseline exactly. Median record time was 20.17 versus
  19.46 ms for Classic and 4.37 versus 4.24 ms for Enhanced; measured cadence
  remained 33.33 ms. Enhanced allocations were 1.09 versus 1.02 MB per frame in
  the single pair, with variation in renderer buffer growth in the profiles.
- Latest main through 2cfe4fdb integrated without conflicts before final gates.
- Stock and TA Zero each completed 31 focused captures covering construction,
  salvage, placement, water, finish, lighting, blast, fire and wreck effects.
  Inspected construction comparisons show progressing nanoframes under both
  factors; missing beam families have visible compatible units and a readable
  limitation note. Placement retains its authored weapon range.
- Whole-tree fast gate passed. Retail lint's method-call comparison spelling in
  the cache test was fixed forward; the focused cache check passed and retail
  staticcheck/deadcode passed on the correction. The whole-tree short retail
  tier and real GPU device fixtures passed. The reviewed result is squash-landed
  on main; temporary implementation worktrees are removed after landing checks.

## Manual test build

The combined executable and isolated preferences are outside the repository:

```sh
/private/tmp/nanolathe-settings-preview-review.SpMBOD/test-preview --mod ta-zero
```

Open the Nanolathe settings screen and check Mutators → Build speed → Compare,
Effects → Placement weapon rings, Water and Glow. Switch content to Total
Annihilation and apply,
then return to previous cards to check scene reuse. Alternatively launch with
`--mod none` for stock content. The launcher uses a separate settings file;
installed content remains available. Artifacts and full logs are in
`/private/tmp/nanolathe-settings-preview-review.SpMBOD`.

## Follow-up: map cost and menu responsiveness

The original units were squash-landed as `6eb14707` on 2026-09-30. Both
pre-landing and post-landing `tools/check` and short `tools/check-retail`,
including real GPU fixtures, passed. Their four clean implementation/landing
worktrees and branches were removed; the external test artifacts remain.

The user then requested further map-loading reductions and supplied reports of
very low menu frame rates during scene loading on Linux and Windows. The new
integration worktree is `/private/tmp/nanolathe-wt-settings-map-loading`, based
on that landed commit. These reports motivate host responsiveness work; they
do not establish which GPU or loading path caused another user's slowdown.
Reported hardware includes a Ryzen 9 8945HX/RTX 5070 laptop on Linux and an
i5-10400F/RTX 2060 desktop with 64 GB RAM on Windows 11. Neither platform has
been reproduced locally; native measurements below use this macOS host.

### Measured costs

Local probes use stock assets, four Go workers and no remaster preparation.
They are single-host observations, not Linux/Windows latency guarantees.
Map discovery already reads bounded header ranges. Complete preview staging
without GPU rendering takes about 0.39–0.64 seconds and allocates about
171–542 MB cumulatively per scene; allocated bytes are not retained heap size.
Terrain loading is about 7% of the focused staging CPU profile, while archive
art reads and strategic-icon preparation take more. Maps alone do not explain
all of the staging cost.

| Current map | Attribute cells | Distinct tiles | Terrain load |
| --- | ---: | ---: | ---: |
| Coast To Coast | 210×126 | 1,292 | 8 ms |
| Great Divide | 160×256 | 2,820 | 19 ms |
| SHERWOOD | 202×204 | 2,950 | 21 ms |
| Greenhaven | 512×512 | 2,347 | 23 ms |
| Crystal Cracked | 640×640 | 9,698 | 92 ms |
| Gasbag Forests | 608×608 | 7,371 | 60 ms |
| Ice Scream | 450×1,224 | 8,861 | 85 ms |
| Red River | 672×960 | 7,200 | 61 ms |

At 1600×900, nine focused native menu captures made 414 render passes and
104 simulation ticks across 219 display draws while another scene was loading.
Steady comparison views rendered both halves on every display draw. Cold scene
activation held `nlScreen.Draw` for up to 904 ms in the paired construction
case. The CPU profile attributes 2.14 of 2.72 sampled seconds beneath preview
rendering to whole-map water/shadow-mask preparation on the UI thread. Terrain
atlas packing and shader construction remain separate first-use costs.

### Bounded candidate contracts

- The responsiveness agent owns `nlscreen.go`, `nlscreen_preview.go`, a new
  cadence contract test and the corresponding interface-design subsection in
  `/private/tmp/nanolathe-wt-settings-menu-responsiveness`. Ordinary backgrounds
  use 30 FPS while controls and cursor keep their existing host cadence. The
  Frame rate card overrides that budget, including its uncapped choice. Loading
  pauses the outgoing background without catch-up; both comparison pictures
  share a cadence and edits invalidate immediately.
- The terrain agent owns `gpurender/water.go`, a new preparation contract test
  and a small GPU-design contract in
  `/private/tmp/nanolathe-wt-settings-water-preparation`. It splits existing mask
  CPU preparation from GPU upload without changing its arithmetic or ordinary
  battle fallback. The opaque prepared result belongs to a terrain identity.
  Root wires it into the preview after staging/lead-in, while the staging worker
  exclusively owns the terrain, and releases the CPU pixels after upload.
- Root owns integration, this plan, temporary profiling instrumentation and
  independent checks/captures. Instrumentation is removed before committing the
  candidate. Gameplay, ordinary battle pacing and retail research are unchanged.

### Further map options

Prefer smaller authored fixtures for Crystal Cracked, Gasbag Forests,
Ice Scream and Red River, which have both large cell grids and many tiles.
The current coast, hillside and forest maps are already relatively small.
Greenhaven has more cells but fewer tiles than Great Divide or SHERWOOD, so a
cell-only ranking can increase GPU atlas cost. A suitable replacement needs a
two-player Network schema, starts, enough formation/camera space and the relevant
shore, snow, forest, contrast or hillside treatment. No fixture replacements
are implemented in this candidate.

Caching immutable decoded TNT/detail sources would avoid repeated decoding for
twins, rules, mutators and restarts but still read each full map on its first
visit. Mutable world, feature, movement and visibility state must stay separate.
To avoid that first full read, the VFS already supports range access through
content-layout views; compressed archives decode only intersecting chunks.
A bounded loader must group overlapping range reads, remap referenced tiles,
preserve feature-footprint margins and adapt authored starts, anchors, waves,
offscreen support and camera bounds. A post-read crop would reduce world/GPU
work without avoiding initial decompression. Neither crop path is implemented.

### Candidate validation

Both bounded units are committed and integrated (`99263f31`, `ebaff49f`).
Root independently reviewed their diffs, checked the unchanged liquid/void
classification and mask resolution arithmetic, and ran the affected scheduling,
water-source and documentation checks. Temporary profiling instrumentation is
preserved only with the external artifacts and removed from the production tree.

In the matching nine-state native capture pair, paired construction activation
fell from 904 to 102 ms of `nlScreen.Draw` time. Total background render passes
fell from 2,920 to 1,145 (about 61%), with 1,785 versus 1,943 UI draws; the runs
have different loading durations, so this is work reduction across the capture
sequence rather than an FPS guarantee. The candidate's old scene made zero
simulation ticks during the sampled loading frames, against 104 before. The
whole-sequence maximum UI draw was 107 ms, so stalls are reduced but remain.
First pictures still take approximately 0.6–1.8 seconds after request. Work was
moved into loading; the paired request-to-picture delay has not disappeared.
Reviewed stock construction, water and smooth-edge comparisons retain useful
visible differences and working units.

The clean production candidate is `21cbefa8`. Whole-tree `tools/check` and
short `tools/check-retail` passed, including real GPU fixtures. An independent
review reran the affected cadence, mask and source-lifecycle checks without
finding a regression. Stock and installed TA Zero each completed 34 focused
native captures with remaster preparation enabled. Inspected construction,
renderer, frame-rate, water and wreck captures retain units, advancing
nanoframes and visible comparison differences. Cold activation still took
124–152 ms in the last full-detail wreck captures; the earlier 107 ms maximum
belongs to the remaster-disabled profiling sequence, not every menu scene.

Classic and Enhanced live-battle runs match main in scene metadata, every
measured frame census, all eight opening and eight closing state files, and
captured PNG bytes. Enhanced median record time was 4.33 versus 4.35 ms, with
33.33 ms cadence. The first Classic pair varied from 19.70 to 21.52 ms; a
matching repeat measured 19.80 versus 19.57 ms, with unchanged 4.01 MB per-frame
allocation and 33.33 ms cadence. These short samples support unchanged ordinary
battle behavior; they do not establish an improvement to battle performance.

The native executable and Windows amd64 cross-build are stamped `21cbefa8` and
preserved outside the repository. The Windows executable has not been run on
Windows. The test package contains instructions and no retail assets. This
follow-up remains on `settings-map-loading` for user testing; main contains the
original squash only.

The new test launcher is
`/private/tmp/nanolathe-settings-preview-review.SpMBOD/test-menu`, with separate
preferences:

```sh
/private/tmp/nanolathe-settings-preview-review.SpMBOD/test-menu --mod none
/private/tmp/nanolathe-settings-preview-review.SpMBOD/test-menu --mod ta-zero
```

Test rapid changes between Game, Mutators and Effects while a scene
loads; move the pointer and edit controls during the held picture. Check paired
Build speed comparisons, Water/Glow comparisons and the Frame rate card's Display
override. Repeat with stock and TA Zero. Windows/Linux user feedback is still
needed; local visual/performance validation uses macOS.

The remaining UI work includes cold shader/atlas creation and GPU uploads. An
Apply that changes content still waits for an unfinished staging worker; obsolete
requests are not cooperatively canceled. Neither limitation is fixed by the
ordinary scene-loading pause.

## Follow-up: full background and mouse responsiveness

The user tested the previous candidate, found some improvement but occasional
mouse lag, and requested keeping the entire animated background while pursuing
other optimizations. The committed candidate retains its full viewport, framing,
comparison divider, scene work and background resolution. No smaller-viewport
policy is included.

The software cursor is drawn after the complete screen, so slow control painting
also delays it. The previous remaster-disabled profile sampled 6.35 CPU seconds
under `nlScreen.Draw`, 0.65 under its background and 3.94 under its hero painter;
2.93 of the latter were under the stepper lamps. Native rows with a ready scene
and no background render had median draws of about 9.7–10.7 ms on Build speed,
versus 1.3–1.4 ms on Water. Those rows can include a synchronous preview tick;
they are host callback times, not GPU or physical mouse-presentation latency.

The bounded shape unit is owned in
`/private/tmp/nanolathe-wt-settings-menu-shapes` on `settings-menu-shapes`, based
on the reviewed integration tip `719e5b73`. It changes only screenkit's painter
and three new cache/coverage files. Its `54be1d81` commit reuses original small
antialiased shapes after repeated use, with exact float32 geometry/colour keys,
bounded entry/byte/dimension budgets, least-recent retirement and the original
vector fallback. Root read the complete diff and independently ran its focused
tests and hidden native device fixture: 1,520 direct/cached comparisons passed,
including clipped, translucent, fractional and layered lamp geometry, with a
maximum 1/255 channel difference. Contact sheets are preserved outside the repo.

Root integrated the unit, recorded matching native baseline/candidate menu runs,
removed temporary profiling code, and ran the whole-tree gates and applicable
visual/performance checks before producing an updated test build. The fresh pair
uses stock content, four Go workers, 1600×900 and remaster disabled, matching the
nine-state Build speed, Water and Smooth edges sequence. Full-detail stock and
TA Zero captures provide separate visual checks.

The fresh matching pair confirms the control-painting benefit. With a ready
scene, no background render and no preview tick in the callback, Build speed
median draw fell from 9.77 to 0.74 ms and its raised comparison from 10.10 to
0.85 ms. Their p95 values fell from 10.20/11.12 to 1.86/1.61 ms. Other cards
improved more modestly: Water compare median 1.38 to 0.98 ms and Smooth edges
compare 1.46 to 0.93 ms. These cohorts have different sample counts; they are
local host-callback observations, not cross-platform mouse-latency guarantees.
The full sequences contain 1,958 versus 2,017 display draws and 1,151 versus
1,180 background render passes, with differing load durations. Whole-sequence
maximum draw remains 124 versus 114 ms: cold scene work is still present. The
new profile removes repeated vector-stencil work from the dominant steady UI
cost without reducing background resolution or switching the cursor.

Across the nine tested views, ready/non-loading draws average 4.597 ms in
`menu-baseline.json` versus 2.062 ms in `menu-shapes-candidate.json`, approximately
55% less host draw time. Giving each view equal weight instead of each callback
produces approximately 54.5% less time. The baseline already contains the landed
asset-loading changes; this aggregate measures the subsequent responsiveness
work. These local stock-content, remaster-disabled samples cover the tested view
mix, not every card, GPU execution, physical mouse latency or an overall FPS
percentage. The earlier asset-loading gains and scene-activation improvements
measure separate costs and must not be added to this percentage.

Further candidates should follow the remaining profile: reuse text measurement
and wrapping where bounds/content permit, then move CPU terrain-atlas packing
into staging or spread cold GPU uploads across frames. The software cursor still
depends on frame completion; an operating-system cursor would decouple motion
from draw pauses but change the existing cursor treatment. No cursor replacement
or additional background reduction is included here.

The clean production build is `805d298a`. Whole-tree `tools/check` and short
`tools/check-retail` passed, including real GPU fixtures. The independent native
shape fixture also passed. Stock and TA Zero each completed 39 full-detail
captures; inspected rules, construction and water comparisons retain the full
background, working units, control geometry and the intended comparison effects.

The Classic and Enhanced live-battle runs match main in scene/host metadata,
all 180 frame censuses, all 16 endpoint state files and captured PNG bytes.
Classic median record time was 19.70 versus 20.40 ms and Enhanced 4.33 versus
4.32 ms; both retained approximately 33.33 ms median cadence. Classic allocation
was 4.007 MB/frame in both runs. Enhanced allocation was 0.994 versus 1.034
MB/frame; saved allocation profiles show existing rendering/model buffer costs
and no sampled screenkit cache cost. These short diagnostic samples preserve
the observed battle workload; they establish no battle-performance improvement
or exact allocation invariance.

The new native launcher uses separate preferences:

```sh
/private/tmp/nanolathe-settings-preview-review.SpMBOD/test-menu-full-background --mod none
/private/tmp/nanolathe-settings-preview-review.SpMBOD/test-menu-full-background --mod ta-zero
```

The external Windows test package is
`/private/tmp/nanolathe-settings-preview-review.SpMBOD/nanolathe-menu-full-background-windows-test.zip`.
Both executables are stamped `805d298a` with a clean source tree. The Windows
amd64 cross-build is not Windows runtime validation. Its README explains asset
selection, isolated preferences and the focused test sequence. The previous
candidate's executables and launchers remain available for comparison.

Test pointer motion on Mutators → Build speed, with and without Compare and
while changing the multiplier; then switch rapidly between Game, Mutators and
Effects during loading. Revisit Water and Glow, and repeat with TA Zero. Record
whether any remaining lag occurs during loading, first use or steady animation.
At that test handoff the candidate was retained in the reviewed integration
worktree, while main contained the original preview-loading/replacement-roster
squash.

## Squash landing

The user approved landing the tested follow-up on 2026-09-30. Main at
`6eb14707` was integrated without conflicts. The isolated `settings-menu-land`
worktree assembles the reviewed cadence/loading pause, CPU water-mask preparation
and bounded control-shape reuse as one squash. Its production Go files match
the tested `805d298a` build; subsequent changes only record validation and
measurement scope. The entire animated background and its resolution are
retained. The landing owner refreshes `tools/check`, short `tools/check-retail`
and the native control-shape device fixture before and after landing. The prior
stock/TA Zero captures and both renderer benchmarks remain applicable to this
unchanged implementation. Only these completed performance worktrees and their
branches are retired after the landing checks; external test builds, profiles
and captures remain available.

The refreshed fast gate passed. The first retail attempt failed the unchanged
`TestClientCloseStopsRecordPool` process-wide goroutine-count assertion by one
goroutine. All 100 focused repetitions passed, and the whole short retail gate
passed on retry, including its real GPU fixtures. No client teardown code or
test was changed for this landing. The separate native control-shape fixture
also passed all 1,520 comparisons with maximum channel difference 1/255.
