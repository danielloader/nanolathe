# TAUCP and original Star Wars TA packages

## Scope

This reference records the release identities and installation requirements
of two historical TA content packages. It establishes no retail mechanics,
new Nanolathe rules or completed compatibility. They are the preservation
candidates after [TA: Twilight](twilight-engine.md).

## TAUCP 2.3

**Established — author documentation.** The
[author's files page](https://taucp.tauniverse.com/taucp.htm) identifies the
final release as December 23, 2002 and separates Normal PC, TA:M/TA:C and Mac
distributions. Normal PC requires TA 3.1 plus Core Contingency or the author's
replacement model files. TA:M/TA:C uses its own modified `Rev31.gp3` setup;
its installation cannot be substituted for Normal PC without inspection.
The author's [features page](https://taucp.tauniverse.com/features.htm) and
[release announcements](https://taucp.tauniverse.com/) describe selectable
content sections, AI choices and frontend resource options. The optional
OverLoad Pack is a separate release, outside the initial 2.3 package scope.

**Established — downloaded archive identity.** The preserved Normal PC
[taucp23.zip mirror](https://totala.strategie.pl/download/taucp23.zip) is
17,926,367 bytes with SHA-256
`cf7b467d0c408e548297a3a4d10479b8ffee83667a30298c8fdc4321cc9eee35`.
Its MD5 and SHA-1 match the independently published
[AusGamers listing](https://www.ausgamers.com/files/download/31427/total-annihilation-units-compilation-pack-v23).
The ZIP contains only `taucpsetup.exe`, 17,943,193 bytes with SHA-256
`0f51d349b183d1c6e7aa1cf900d609312077b7e57115966014bf4c198a663d0a`.

**Unknown — authored payload and selected installation.** Container tools
available during the October 4, 2026 inspection did not recover any authored
files from the installer. No installer code was analyzed. A compatible
container extractor or a provenance-matched authored-data distribution is
needed to settle the selected files, archive precedence, resource completeness
and executable dependencies. Selecting every optional component is not
established as the default installation. No package config or playable
catalogue entry is authored before those inputs can be verified.

## Original Star Wars TA

**Established — author release identity.** The
[original SWTA downloads page](https://swta.tauniverse.com/downloads.htm)
identifies Advanced Fighter Pack v1.0 as the project's fifth release,
advertised as 11.7 MB. It links
[swtapack5_full.zip](https://swta.tauniverse.com/swtapack5_full.zip) and
[swtapack5_full_tam.zip](https://swta.tauniverse.com/swtapack5_full_tam.zip),
describing the Normal and TA:M forms as compatible. This scope excludes the
retail demo bundle and later Spring/Imperial Winter projects.

**Established — older manual, bounded scope.** The
[installation manual](https://swta.tauniverse.com/readme_install.htm)
requires TA 3.1, recommends Core Contingency and names `.swx` content,
`rev40.gp4` and `SWTA.exe`. Its package names predate the fifth release, so
that inventory cannot establish the fifth release's contents. The
[FAQ](https://swta.tauniverse.com/readme_faq.htm) describes a custom
executable archive selection that excludes `.ufo`. Nanolathe's existing VFS
supports `.swx` and `.gp4`; that format support does not establish that archive
selection is the executable's only modification.

**Unknown — artifact and engine contract.** Both primary fifth-release ZIP
URLs returned HTTP 403 during the October 4, 2026 inspection. Exact bytes,
hashes and content layout require the original ZIP or a traceable mirror.
Matching release documentation, licensed source or scoped manual observations
must settle any additional executable requirements before dependent behavior
is implemented. No modern Community table is assumed for this older package.

## Acceptance boundary

**Unknown — Nanolathe playability.** Neither package has reached authored
content compilation or a battle acceptance run. After recovering its data,
preserve readmes and credits and exclude executable and retail base files.
Establish the original installation's winning paths and compare their bytes
and compiled catalog hash with the cleaned package. Then check linked
commander programs and build membership, ordinary construction and weapons,
and a bounded skirmish under evidenced limits. Record remaining mechanics
in the owning section; content admission alone does not prove historical
engine equivalence.
