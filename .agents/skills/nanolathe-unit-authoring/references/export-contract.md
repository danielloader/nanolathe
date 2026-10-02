# Native export contract

Read before modeling, assigning materials or exporting. This is an authoring
snapshot of **Nanolathe Blender rigid exporter 0.1.2**, inspected on 2026-10-01
at exporter commit `9fe411c9c9be37d7af2910d416a15b89b3d8ddfc`. The external exporter's
`docs/AUTHORING.md`, `docs/SCHEMA.md`, `addon/nanolathe_export/core.py` and
`compiler.py` are the authorities for its current subset. This skill does not
vendor that tool or define new engine behavior.

## Separate engine facts from exporter policy

**Established format/implementation:** [3DO texturing](../../../../research/formats/3do.md#texturing)
stores a texture name for each primitive, with no arbitrary UV coordinates or
PBR material. Native model files retain pieces and parent translations, not
Blender's modifier stack, bones or stored normals. Inspect the format document
and the current renderer rather than assuming a general-purpose mesh importer.

**Exporter policy:** all exported visible faces are quads. Constant/team faces
may be arbitrary convex planar quads; image-textured faces require a geometric
parallelogram and a nondegenerate UV parallelogram. Nonplanar, concave,
self-intersecting and collapsed quads are errors. Four indices alone are not
enough. A textured octagonal wheel cap split into arbitrary quads can still fail
this rule: plan its texture-compatible topology during the blockout.

Tapered image faces must be reauthored within the supported subset, or explicitly
given an appropriate constant finish when that preserves the intended art.
Do not replace every rejected texture with a flat color, bypass validation,
triangulate or silently change the exporter. If preserving the requested art
requires an unsupported mapping, record the exact rejected faces and continue
independent work; an exporter extension is a separate, explicit scope decision.

## Scene and hierarchy

- One named export collection; cameras, floors, reference images and studio
  lights live elsewhere. `nanolathe_ignore=true` explicitly excludes an object.
- One root named `base`, at world origin. Every exported object descends from it.
  One mesh object per rigid piece; disconnected armor may share that mesh.
  Empties are supported pure pivots/locators, encoded as zero-face pieces.
- Unique lowercase ASCII names matching `[a-z][a-z0-9_]{0,30}`. Piece indices
  come from depth-first hierarchy traversal with lexical sibling ordering.
  Renaming/reparenting requires matching profile/script updates and re-export.
- Blender uses +X right, +Z up, −Y forward. `units_per_blender` controls scale.
  The exporter handles the engine coordinate conversion and winding/UV reversal;
  do not add a second flip. Confirm scale and facing against native views.
- Preserve mechanical origins. Apply scale to geometry while keeping the origin;
  exported scale is `(1,1,1)`. Static rest rotations are supported, but dynamic
  yaw/pitch pivot world axes must align with the global axes at rest.
- No negative determinant, shear, delta transforms, modifiers, constraints,
  skinning or shape keys. Resolve evaluated geometry deliberately before export.
- Outward winding is required. Split vertices where geometric hard edges matter;
  custom/smooth normals do not survive as Blender normals. Open surfaces warn;
  inspect their visibility. Edges with more than two faces fail validation.
- The exporter inserts the root selection quad. Set `selection_half_extents`
  from the intended ground footprint; do not turn the barrel overhang into the
  footprint or add a second visible ground plate. Model extent, FBI footprint,
  movement class and selection area serve different purposes.

Practical exporter caps, **not artistic targets**: 128 pieces, 16 hierarchy
levels, 4,096 quads per piece, 65,535 vertices per piece (base reserves four),
2,048 unique oriented tiles, source images at most 16,777,216 pixels in total
(4096 × 4096; non-square images are allowed), and square output tiles of
8/16/32/64/128 pixels. Prefer the requested much smaller asset budget.

## Materials and native texture references

Use a Principled BSDF directly connected to Material Output. Its Base Color is
either a constant or one directly connected Image Texture Color. Images use an
active UV map, FLAT projection, REPEAT/EXTEND and Closest/Linear filtering.
Procedural mapping, layered materials, normal/displacement/PBR maps, image
sequences and linked emission/roughness/metallic are not exported. Studio
lighting is not texture paint. Bake any needed authored form cues into the
ordinary albedo without baking a fixed world shadow across moving pieces.

The exporter resamples each face into an oriented square tile and deduplicates
matching indexed bytes. Choose tile resolution for native visual detail; a large
faction atlas does not guarantee a high-resolution native face. Inspect palette
quantization at actual size. The output GAF names are stable content-based
references used by the 3DO; reuse the exporter instead of building a second
atlas-to-GAF/3DO writer.

Use the intended installation's external 768/1024-byte PAL. The authored RGB332
example palette checks encoding, not installed colors. The exporter excludes
indices 0, 9 and 10–15 from ordinary quantization; preserve its current rules.
Do not invent a universal team-color palette range. Do not double gamma-encode
image samples or bundle extracted retail PAL/LOGOS/shading data.

Exported faces are opaque: sampled alpha below 0.999 fails. Unoccupied atlas
space may remain transparent, but no exported UV may sample it. Transparent
wheel/antenna cutouts cannot replace silhouette geometry through this path.

**Exact team binding:** set `material["nanolathe_team_texture"] = "colorsmd"`, or
use a material named `TEAM_COLOR` with no explicit property override. An explicit
property value takes precedence over the name, including an empty value.
Descriptive material names,
`nanolathe_texture` and `nanolathe_team_color` alone are not this exporter's API.
Its blue viewport swatch is separate from the faction atlas. Each team region
occupies a whole quad; no mixed painted panel/team mask within one face.

The 3DO must reference literal `colorsmd`. The ordinary emitted GAF must **not**
define it. Team frames resolve through the installed ten-frame fallback LOGOS
bank; a primary GAF entry can shadow that lookup. Verify two owners through the
native renderer. Relevant implementation: `internal/client/model_textures.go`.

## Lighting fidelity

Inspect the unit's FBI/rendering class in the target checkout. The current
Human mobile pilot's `BMcode=1` path bypasses directional SHD face shading;
see the `!current.BMCode` shading condition in `internal/render/model.go` and
`Structure = !v.BMCode` in `internal/client/model_staging.go`. A beautifully lit
Blender model may therefore
look flat in-game. Keep readable broad highlights, recesses and material values
in original albedo where necessary. Do not change FBI class or engine shading
to repair an art problem. Compare actual native pixels before repainting an
entire faction atlas used by other units.
