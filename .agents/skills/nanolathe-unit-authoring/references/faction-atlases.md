# Reusable faction atlases

Use for creating a faction atlas or texturing a unit. The atlas is a shared
authoring resource; 3DO still references individual named textures. Native
deduplication does not necessarily share tile names across different unit IDs.

## Find and preserve the existing material library

Look in the requested content worktree and its faction source directories for
atlas images, manifests and existing UV users. Establish which atlas/version the
unit uses. Prefer its paint colors, scale of panel detail, edge treatment and
metal/rubber language over creating a near-duplicate palette per unit.

Keep occupied regions and names stable. Add surfaces in unused rectangles;
there is no requirement to fill the canvas. Repacking requires migrating and
checking every affected model. Keep a manifest beside the PNG with:

- Faction and atlas version; image dimensions and pixel origin convention.
- Stable tile names, content rectangles in pixels, and gutter/padding sizes.
- Reserved/unoccupied rectangles or cells; tile purpose and orientation where
  ambiguous (for example, tread direction).
- Model/UV consumers when available, and whether the image is authored albedo
  or a derived export bake. Record lineage rather than treating a per-unit
  lighting bake as a new shared canonical atlas.

A modest power-of-two atlas such as 1024 or 2048 square is a starting choice,
not a fixed requirement. Respect the current exporter's source-image cap. Use
enough texels for useful face detail, while keeping most of the surface reserved
when the first unit needs only a small set of tiles. No retail texture sampling.

## Paint for reuse and game size

Start with reusable armor paint, plain variants, worn edges, dark metal, rubber,
vents, access panels, optics, lamps, wheel hubs and tread strips as relevant.
Reuse existing tiles, including rotated UVs, before adding a tile. Add a new
surface only when the concept needs a material, proportion or detail the current
set cannot express well. Do not force a unique marking or distorted wheel into
an unsuitable existing tile merely to claim reuse.

Broad value separation survives palette quantization and small renders better
than dense noise. Geometry owns silhouette, holes and moving clearances;
paint owns small bolts, seams, slots, grilles and repeated tread divisions.
Compare real 1× native output and an enlarged inspection view. Report which
detail disappears, rather than inflating every texture or the quad budget.

Extrude ordinary edge pixels into gutters and keep UVs inside the intended
content rectangle. Do not sample transparent reserved space. A shared tile's
lighting cues should make sense on its intended face orientation; articulation
must not carry a world-space cast shadow painted from another moving part.

## Team color is a separate texture system

Never paint the current team's blue into the faction atlas, including as a
mask embedded in ordinary armor. Use complete dedicated quads and the exact
binding in [the export contract](export-contract.md#materials-and-native-texture-references).
Multiple panels may share that one team material. Broad top and side panels
make ownership legible at game size; enlarge them when requested while retaining
the concept's overall color balance. Do not put them only on a hidden underside
or a thin rim that disappears at the game camera angle.

Replace or partition armor faces cleanly; avoid coplanar overlapping decals and
z-fighting. Validate each resulting quad against the exporter's material-specific
shape constraints. Prove that swapping owner changes only the intended faces
and that the ordinary texture GAF does not shadow `colorsmd`.
