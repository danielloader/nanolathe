# Total Mayhem 11.3.0 package

## Evidence scope and artifact

This document records the installation and content contract of Total Mayhem
11.3.0. It does not establish retail behavior or approve new Nanolathe rules.
The primary release page is [Mayhem Inc's Total Mayhem page](https://mayhem.tauniverse.com/totalm.htm),
which identifies version 11.3.0 and the `TotalM1130.zip` download. The
inspected archive is 26,035,480 bytes with SHA-256
`f485e8950a4b71940f414159ab6375a3cc32f35c2e8a962918e5984d801363af`.
Its bundled `1092 to 113 changelog.txt` and `mayhem.ini` provide the
package-specific engine and preference evidence below.

## Executable and runtime

**Established — file identity.** The archive's `TotalA.exe` is 1,178,624
bytes with SHA-256
`3b9c0fadabf3dc67ed5f05a70f1e1505a0c65deadd1a3c930adfe30e2a84995e`.
It is byte-identical to the reference retail 3.1 executable identified in
[ProTA 4.8 engine package](prota-engine.md#evidence-scope-and-sources).
Total Mayhem 11.3.0 therefore does not require an on-disk Mayhem-specific
change to `TotalA.exe` relative to that reference.

**Established — documented runtime extension.** The archive also ships
`dplayx.dll`, `tplayx.dll`, and `tdraw.dll`. The bundled changelog names the
11.2 update to those components and documents engine changes including
construction-site snapping, team start positions, order-queue editing,
constructor patrol behavior, and wind synchronization. These are runtime
patch behaviors, not properties of the byte-identical executable. The
separately researched current community-patch source includes a `mayhem`
build profile ([Community patch engine behavior](community-patch-engine.md#3-artifacts-and-build-profiles));
that source's revision is newer than this 2024 package. Its exact equivalence
to the shipped `tdraw.dll` is **Unknown**. Matching licensed source or a
versioned release record for the shipped DLL would settle that version boundary.

## Authored content and settings

**Established — authored archive.** `mayhem.gp3` contains the mod's units,
weapons, scripts, maps, sound and presentation assets. It keeps `units` and
`gamedata` under retail names and names four other top-level trees
`weaponM`, `guiM`, `unitpicM`, and `downloadsM`. The separate `TADemoM.ufo`
also contains authored data. These two archives and `Icon/` are the content
and presentation inputs selected for Nanolathe's package; the executable,
DLLs, launch configuration and renderer shaders are not needed as content.

**Established — documented limits.** `mayhem.ini` declares defaults of
1,500 units per player, 66,650 pathfinding entries, 20,480 special effects,
and 16,000 each for unit and weapon identifier tables. It describes the
larger weapon table as a single-player setting. These declarations support
the load-time content profile's table sizes and the existing `mayhem`
Community feature table; they do not establish that every documented engine
extension is implemented by Nanolathe.

**Established — bounded host acceptance.** With the two authored archives
mounted over the reference content, Nanolathe's `mayhem` profile compiles
the catalog and starts a skirmish. Packaging those same archives with the
two changelogs and `Icon/` preserves the catalog hash. This checks content
admission and one battle startup, not complete historical gameplay or
controls parity.
