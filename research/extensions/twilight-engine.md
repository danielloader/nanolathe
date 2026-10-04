# TA: Twilight 2.0 Beta 98 package

## Evidence scope and artifacts

This document records the installation and content contract of TA: Twilight
2.0 Beta 98. It establishes neither retail behavior nor complete historical
patch parity. The author's project is [TA: Twilight](https://twilight.tauniverse.com/).
The inspected release is the TAF distribution, not the older 1.9 release.

**Established — primary distribution record.** The
[TAF featured-mod API](https://api.taforever.com/data/featuredMod) identifies
`tatw` as Twilight and lists these content packages in installation order:

| Upstream package | Bytes | SHA-256 |
|---|---:|---|
| [TAT Drop in b91.zip](https://sjc1.vultrobjects.com/mods/tatw/TAT%20Drop%20in%20b91.zip) | 115227139 | `52adb7cf335fb03d27c155564c4d786953275170c1fba8234a32a61cc671f319` |
| [TAT-v2.0 Beta 98.zip](https://sjc1.vultrobjects.com/mods/tatw/TAT-v2.0%20Beta%2098.zip) | 60314519 | `8671732cfd0e31eae07b392c1bde7a255bb938ac69a031ae3fabf4cc1eaca54f` |

The update's authored changelog identifies Beta 98 as June 19, 2026.
Nanolathe's version string `2.0-beta98` represents that release. The API also
lists a separately downloaded runtime package; Nanolathe does not package
that runtime or treat it as content evidence.

## Installation and archive replacement

**Established — installer source and authored files.** The MIT-licensed
[TAF installer](https://github.com/ta-forever/downlords-taf-client/blob/593abc1c0f4a9c1d19c89b9215b5bd795f12512d/src/main/java/com/faforever/client/mod/InstallFeaturedModTask.java),
inspected October 4, 2026, extracts the listed packages sequentially into the
same target directory, replacing existing files. Both content ZIPs contain
`rev31.gp3`. Beta 98 replaces the complete Beta 91 archive; it is not an
additional VFS overlay that inherits paths absent from the replacement.
Mounting the two ZIPs as separate roots would incorrectly retain the older
`anims/logos.gaf` and produce a different catalog identity.

The Nanolathe package retains the replacement `rev31.gp3`, the base's
`TADEMO.UFO`, `TA_Ais_2013.ccx`, `TA_Features_2013.ccx`, `Icon/`, the Beta 91
and Beta 98 summaries, the updated full changelog, `LICENSE`, and the authored
Beta 88 unit guide. The older guide is preserved documentation, not evidence
for Beta 98 unit values. Empty archive placeholders, executables, DLLs,
launcher INIs and Windows renderer files are excluded.

## Authored content and settings

**Established — authored archive data.** The package uses the retail logical
tree names for units, weapons, game data, interface definitions, unit pictures
and build downloads. ARM and CORE identify `ARMCOM` and `CORCOM` as their
commanders. Its `gamedata/los.tdf` is 2,718,022 bytes and declares 94 tables,
so it exceeds the retail LOS read cap without requiring a renamed layout.

**Established — authored configuration.** Beta 98's `totala.ini` declares
1,500 units per player, 66,650 pathfinding entries, 20,480 special effects,
and 16,000 each for unit and weapon identifier tables. The package config
carries those values through the existing content limits and Community
feature fields. Its 64 MiB terrain and 8 MiB LOS caps are Nanolathe host
admission limits, shared with the other large content packages; they are not
historical executable limits.

**Established — bounded Nanolathe acceptance.** The original installed
layout and the cleaned package have identical winning archive paths,
providers, sizes and SHA-256 values. Compilation produces the same catalog
hash, `a30fd768bf37c54e020c60c72b46f9e06da30f7b1eba7e834247f2abc23485dd`.
Commander programs and authored build membership link under the package
config. The retail LOS cap still refuses the enlarged table. Seven existing
downloadable-unit warnings remain visible rather than being repaired by
invented data. A 900-tick seeded Community skirmish has the same partial state hash and
both RNG states and draw counts for the original and cleaned installations.
Rendered battle captures were inspected through both existing renderers.
This checks startup and ordinary construction,
not long-match completion, every unit, campaign compatibility or save parity.

## Runtime boundary

**Established — file identity.** The update's `TotalA.exe` is 1,178,624 bytes
with SHA-256
`70e65265e28303609b42f0594af8eae5ad53b4ec998a30316010914cdee82253`.
It differs from the reference retail 3.1 executable. This is a byte-identity
observation only; no executable behavior was inferred from it.

**Established — current-source recommendation.** The existing
[Community patch reference](community-patch-engine.md#31-feature-matrix-at-the-pinned-revision)
records the MIT source's `twilight` build profile at its pinned revision.
Nanolathe's complete schema 2 config carries that profile through the existing
Community feature surface, recommends Community 3.9 and adds no setting locks
or keyboard preset. It selects no new engine mechanics. Strict 3.1 continues
to bypass Community features through the central rules mechanism.

**Unknown — historical runtime equivalence.** Matching source or versioned
primary documentation is needed to identify every modification in the shipped
executable and DLLs and establish their equivalence to the pinned source.
Additional gameplay and control paths need scoped acceptance checks before
compatibility can be described as complete. The catalogue therefore labels
Twilight compatibility experimental. No historical executable mechanics or
controls are invented to close this gap.
