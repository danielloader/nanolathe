# Native integration and verification

Use when the user requests a game-ready unit, native export, animation checks
or an in-game test. Art review alone can end with source, renders and clearly
stated limits. Earlier authorization to integrate remains authorization; there
is no extra art-approval gate in this skill.

## Reuse the actual content pipeline

Locate the named content worktree and inspect its status before changes. Check
the requested unit's identifiers, piece ABI, FBI, weapons, effects, sound
references, menus, portraits and packaging scripts. Preserve unrelated units
and behavior. If the worktree is foreign and dirty, leave those changes alone
and use the repository's worktree/coordination rules.

The existing Human pilot may have `probes/human-units/blender-package`,
`blender-pilot-run`, `blender-unitpics`, `check` and `rigid-preview`. These are
optional project-context tools, **not guaranteed paths in main**. Read their
current flags and source. Reuse them when present; do not copy stale absolute
paths, old HPI hashes or assumed menu pages from a previous conversation.

For an update, retain the existing unit ID and gameplay definitions unless the
request includes gameplay changes. Refresh its build portrait from the actual
new model. Do not invent an entire faction, command structure or balanced unit
statistics merely to exercise one asset. Missing new-unit settings should be
resolved from the user's intended role/content contract, with consequential
unknowns reported under the repository's rules.

## Export and inspect the result

First run the read-only preflight helper in this skill. Then invoke the installed
exporter's own CLI, with an external palette and a new output directory:

```sh
blender --background --disable-autoexec /path/to/unit.blend --python-exit-code 1 \
  --python "$NANOLATHE_EXPORTER/cli/export_blend.py" -- \
  --profile /path/to/unit.profile.json \
  --palette /path/to/external/install-palette.pal --out /path/to/new-export
```

Use a discovered Blender executable if `blender` is not on PATH. On the current
Mac installation, background Blender still initializes Metal and may need the
narrow host-graphics permission described by the tool; a startup crash is not
evidence of invalid geometry. Do not install a second Blender to evade it.

Inspect the actual outputs and `engine-report.json`, not merely exit status:

- `objects3d/<unit>.3do`: expected piece tree, face counts and selection plate.
- `textures/<unit>.gaf`: every ordinary reference resolves, correct orientation
  and palette, no accidental transparent/key pixels or emitted `colorsmd` entry.
- `scripts/<unit>.cob`, `source/<unit>.bos`: piece names/indices match, firing
  queries return correct origins, and existing custom responsibilities survive.
- `<unit>.rigid.json` and report: native decode, pose agreement and applicable
  aim/recoil/walk callback checks pass. Understand each warning; no claim of
  mechanical collision clearance follows from a transform comparison.

Source images must remain editable and portable; pack them in the `.blend` and
preserve the faction atlas/manifest externally. Preserve the export profile and
source-to-native provenance. Do not distribute the external retail palette,
LOGOS bank, extracted models or other retail inputs.

## Game verification

Use an isolated additive mod/content overlay and local settings/save directory.
Mount only the intended version of the test archive. Do not replace the retail
installation or publish globally as part of a local test.

Check the affected contracts with existing probes before inventing another test
harness. For an armed ground unit, the useful checks are:

1. Catalog/texture/COB load, intended build menu and portrait; factory construction
   and exit where applicable; rest scale, facing, footprint and ground contact.
2. Movement, turns, start/stop, weapon aiming in front/side/rear and while moving;
   pitch/recoil clearance, muzzle origin, shot direction and visible recoil/return.
3. Repeated targets/shots, interruption/target loss and ready-to-fire timing;
   damage/death and save/load of active script state when the session supports it.
4. Applicable Modern and Strict 3.1 behavior through the existing content test
   path. Rendering the model in two cameras is not this simulation check.
5. Actual engine views at native game size and a diagnostic enlargement, multiple
   headings and at least two team owners. Look for hidden/back-facing pieces,
   collapsed material values, UV seams, palette loss, missing textures, incorrect
   team lookup, shadows and visual clipping. Preserve the native unscaled image.

Use Blender renders to review source art, and production engine captures to
judge export fidelity. Label isolated model diagnostics separately from a live
battle. A staged screenshot alone does not prove that construction, combat or
animation works. When headless-running the window build, follow AGENTS.md's
Ebitengine skill requirement; locate/read the relevant installed or upstream
`run-ebitengine-app-headless` skill first.

Follow the applicable visual/performance checks in
[AGENTS.md](../../../../AGENTS.md) and
[ARCHITECTURE §6](../../../../docs/ARCHITECTURE.md#6-verification).
Use existing benchmark scenes and sequential runs with comparable metadata when
the change affects their workload; distinguish a custom-unit performance
measurement from an unchanged retail benchmark. Do not imply a performance pass
from a single screenshot. Whole-tree landing gates belong to the integrated
candidate, per the repository rules, not every intermediate modeling iteration.

## Handoff

Commit the scoped source changes in the worktree. Provide source and native
paths, verified face counts, exporter/version information, the test result and
any remaining limitation (for example static treads). Give one exact command
using the binary, overlay and settings directory actually tested, plus where the
unit appears or which prepared battle to load. Verify the command's paths exist.
State how to return to the ordinary launcher. Do not claim main integration,
retail executable interoperability or publication unless actually performed.
