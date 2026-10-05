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

## Authored build-menu geometry

**Established — Beta 98 authored files.** In the replacement `rev31.gp3`,
`guis/armcom2.gui` and `guis/corcom2.gui` each place a three-piece factory cell
beside a four-piece factory cell in the first row. The left cell consists of
16-by-64 side strips and a 32-by-64 centre. The right cell uses the same side
strips with two 32-by-32 centre buttons. Each cell fills one 64-by-64 square,
at horizontal origins 0 and 64. Their GAF frames match those hit rectangles.
The second row places a complete air-factory button beside another four-piece
shipyard cell. Constructor menus reuse these split layouts.

**Established — bounded Nanolathe reproduction.** Considering a square at every
child's corner also finds shifted squares that combine pieces from the adjacent
factories. The original Modern sidebar's ambiguity fallback therefore split the
first row into individual cells. Nanolathe's host layout now accepts a complete
cover on the rail's two-column grid, retaining every child action and its shape.
This is presentation policy owned by DESIGN_INTERFACE_HUD_INPUT §3.3, not an
inference about Twilight's patched runtime.

**Established — overlay interaction.** Beta 98 overlays `guis/armlab2.gui` and
`guis/corcom4.gui` with zero-byte files. The reference base install's downloaded
unit records can still raise those builders' page counts and request those
pages. The HUD must give these empty files the absent-page DL fallback already
used by the catalog probe and documented TDF loader. It must not suppress
inherited download membership or treat nonempty malformed GUIs as absent.

**Established — authored state and retail baseline.** Some resolved factory
children in Beta 98's commander GUI files author `grayedout=1`. The retail
numbered-page opener replaces that low bit from product-name resolution,
enabling known definitions and disabling unknown names. Preserving the authored
bit in Nanolathe left those factory children unclickable; applying the documented
page-open writer corrects both sidebar presentations without inventing a
Twilight-specific enable rule.

## Nano platform and turret acceptance

**Established — Beta 98 authored definitions.** The replacement `rev31.gp3`
defines `ARMNANOB` and `CORNANOB` as structure builders (`BMcode=0`);
`ARMNANOB` authors `Digger=1`. Their nano turret products use `BMcode=1`, `Builder=1` and
`CanMove=0`; the regular `ARMNANOTC` and `CORNANOTC` each have build distance
1,000 and worker time 300. Their models and script programs resolve from the
mounted content. These authored classes matter independently of mobility:
the platforms receive factory rally patrols, while the turret products receive
ordinary repair patrols under the retail resolver `[04 R-ORD-02 §1]`.

**Established — bounded Nanolathe acceptance, October 4, 2026.** On Ashap
Plateau with seed 7, an ARM platform completed its regular turret through
ordinary factory construction. The product detached at the platform's origin.
Both renderers displayed only its shadow while the independent factory
occupant policy grouped it with the platform. Removing only that grouping in
the classic committed-frame capture restored the turret body. The confirmed
key-basis and clipping defect is owned by
[DESIGN_PRESENTATION_CLIENT §5](../../docs/DESIGN_PRESENTATION_CLIENT.md#5-divergences).
This is a Nanolathe limitation; it establishes no historical patch behavior.

**Established — corrected bounded acceptance, October 5, 2026 UTC.** Repeating
the same platform construction after excluding independent mixed-Digger
occupants restores the completed turret body in classic and GPU captures.
Actual attachments still use the researched carrier composition. The retail
contract `[03 R-REN-03A §4]` groups linked cargo, not unrelated overlapping
units; the [Community rendering evidence](community-patch-rendering.md#ordinary-model-composition-and-digger-boundary)
establishes no historical turret-specific depth exception.

**Established — bounded patrol acceptance.** In a separate seeded battle, the
platform-built ARM turret on Roam automatically completed a half-built friendly
solar collector 100 world units away with full resource stores and no script
diagnostics. With the mounted Community patrol filter enabled, Hold Position
left a reset half-built target unchanged with full stores; Roam likewise left
it unchanged with energy at 10% of storage. Returning to Roam with full stores
completed it. The two gates are the existing
[Community patrol preference](community-patch-engine.md) (CP-CON-3)
and retail energy-storage test `[04 R-ORD-01 §4]`. This acceptance does not
identify the settings or resource state of the reported live battle, prove
every turret variant, or establish Beta 98's historical runtime equivalence.

## Extractor bonuses and human shortcut admission

**Established — Beta 98 authored unit data and changelog.** `AAIMEXX` and
`CAIMEXX` are special ARM and CORE AI metal extractors. They share the normal
extractors' display name, but author 500 energy production, 8 passive metal,
3 energy upkeep, an extraction rate of 0.003, 6,000 energy storage and 2,000
metal storage. The packaged `TAT-v2.0-Beta-Changelog.txt` explicitly describes
the AI extractor bonuses and the increases to 500 energy and 8 passive metal.
Ordinary `ARMMEX` and `CORMEX` author no energy or passive metal production,
15 energy upkeep and an extraction rate of 0.001.

**Established — bounded authored membership observation.** The special units
occur in the corresponding basic builders' CANBUILD lists, allowing the
computer player to choose them, but not on those builders' resolved human GUI
pages. Their absence is established from product identities, not an inferred
filter for names containing `AI`.

**Established — bounded Nanolathe reproduction.** Before the host-input fix,
Shift-double-click's strongest-extractor selection read construction membership
alone and chose these hidden products for both factions. Ordinary production
accounting then correctly yielded their authored 500 energy. The fix intersects
rule-selected membership with active, enabled product buttons across all human
build pages, including generated downloads. It changes neither AI membership
nor authored resource values. Twilight's underwater extractors are stronger
than its ordinary land extractors; the shortcut also uses ordinary placement
and queue preview to select the strongest fitting human product at the deposit.
This is the existing input convenience's human
availability contract, owned by DESIGN_INTERFACE_HUD_INPUT §3.10; it is not a
new economy rule or a claim about Twilight's patched runtime.

**Established — bounded script-port inventory.** A static instruction and
literal-stack scan of the installed catalogue's 509 linked unit programs found
3,494 decoded port calls. Their selectors resolved only to retail ports or the
already adopted Community getters 69–75. It traversed every authored function
entry and both conditional branches; no unsupported extension selector was
observed. These are static call sites per linked unit, not runtime call counts.
Two authored death-script boundaries remain outside complete decoding: `CMGEO`
reaches an unmatched word and `CORCRW` can branch to the end of its code. This
inventory does not establish complete runtime compatibility.

**Unknown — scoped runtime acceptance.** Twilight's actual Galactic Gate
linking and transfer scripts, switchable Guardian/Punisher weapon modes, armed
transport passenger gates, and death-script boundary outcomes still need
bounded gameplay and save-continuation checks. Existing generic engine
primitives and the port inventory do not prove those complete unit behaviours.
Versioned primary documentation, appropriately licensed matching source or
manual observations are needed wherever those checks reveal an extension
contract not already established; third-party runtime disassembly is excluded.

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
