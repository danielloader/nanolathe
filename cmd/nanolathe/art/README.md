# Original infection overlay

`infection-overlay-tile.png` is original RGBA art. Its source, a 1254×1254
overlay generated with built-in ImageGen on 2026-10-05 for the user-requested
mod-compatible Survival infection treatment, contains no extracted retail or
mod pixels and is covered by this repository's MIT license. The exact
generation prompt is in `infection-overlay-prompt.txt`.

The compositor only samples a 64×64 coverage-aware box-filtered reduction of
that source, so the shipped file is that reduction, produced once by the
compositor's own box filter; a tile-sized image is read back without
resampling. Before the
full-size source left the tree, a test compared the two and found the tile
equal to the full reduction and the composites identical. The source remains
in Git history as `cmd/nanolathe/art/infection-overlay.png`. Arbitrary art can
still be reduced the same way for probes.

Transparent space preserves the underlying surface; the alpha channel is the
generated one. The engine blends the tile into active textures at battle
preparation and quantizes to the active palette; the RGBA source is never a
bypass around indexed rendering. Runtime strength and substrate shading are
controlled by the compositor in `internal/client`.
