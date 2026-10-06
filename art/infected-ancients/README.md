# Infected Ancients

Original, MIT-licensed Survival attackers: the Ancient Crawler (`iacrawl`),
Spitter (`iaspitter`) and Siege organism (`iasiege`), with reclaimable relic
wrecks. They use ordinary movement, weapons and COB scripts. The Crawler is the
only authored infector (`NANOLATHE_INFECTOR`); its takeover is Modern policy
([DESIGN_UNITS_ORDERS_COB "Modern infection"](../../docs/DESIGN_UNITS_ORDERS_COB.md#modern-infection),
[DESIGN_SURVIVAL §5.1, §6.7](../../docs/DESIGN_SURVIVAL.md)).

This directory holds only the final in-game files:

- `content/`: unit FBIs, 3DO models, GAF textures indexed to the TA palette,
  compiled COB scripts, weapons, wreck features and the Survival roster.
  The roster adds these units to the active build tree's attackers
  (`IncludeBuildTree=1`). Every Survival attacker receives the engine's
  generated infected appearance; no retail-derived texture bank is needed.
- `nanolathe-mod.json`: the content configuration used when mounting the pack.
- `pack/`: writes `content/` as one UFO archive, because unit definitions are
  admitted only from archives.
- `play`: packs the content and starts Painted Desert Survival with two Modern
  AI allies. It uses the reference installation in `~/TotalAnnihilation`
  (override with `NANOLATHE_RETAIL_ASSETS`) and keeps its build, settings and
  saves in the ignored `local/` directory. Extra arguments override defaults,
  for example `--gameplay strict-3.1` or `--survival-pace relaxed`.

The models, textures and scripts were authored outside this repository; their
source scenes and review captures are not kept here.
