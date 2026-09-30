# tad-extract

Offline reader for TA Demo Recorder recordings (`.tad`, and ProTA's `.pro`)
and extractor of **where and when each player started building each unit**.
It exists so the local recording mirror (`~/ta-demos`) can be pruned without
losing the one thing only the raw recordings hold: build positions. Build
orders, economy series and unit counts are available from the archive's own
decoded views; positions are not.

The reader is implemented from [research/formats/tad.md](../../research/formats/tad.md)
only. No third-party recorder, replayer or analyzer code was consulted or
copied, and nothing is fetched from the network. Recordings and extracts are
local research data: **never commit them.** Chat subpackets (`0x05`, `0xf9`)
and the chat-log sector are skipped without decoding; lobby-address sectors
are never read. Player names are kept in the legacy local extract only so it can be
joined to archive listings that carry no transport identity.

## Usage

```
go build -o /tmp/tad-extract ./tools/tad-extract
nice -n 15 /tmp/tad-extract -workers 4            # every source under ~/ta-demos
nice -n 15 /tmp/tad-extract -workers 4 4.8 tada   # selected sources
nice -n 15 /tmp/tad-extract -validate             # compare with the archive's views
/tmp/tad-extract -dump ~/ta-demos/v3.1/14.tad     # one recording, first builds, no names
nice -n 15 /tmp/tad-extract -moves ~/ta-demos/v3.1/1709.tad   # movement extract (see below)
```

Flags: `-root` (default `~/ta-demos`), `-out` (default `<root>/extract`),
`-workers` (1–4, default 2), `-force` (re-extract everything).

**Incremental.** A recording is read only when its size matches its
metadata (so a download in progress is skipped), and re-extracted only when
its size, modification time or candidate unit tables changed. Rerun the same
command after more of the mirror arrives. Every run rewrites
`<out>/summary.json` over all listed recordings.

**Sources.**

| layout | metadata | output |
| --- | --- | --- |
| `v<version>/<id>.tad\|.pro` | `<id>.json` (`file`, `bytes`); `mod_units.json`; optional `<id>.site.json` | `extract/v<version>/<id>.json.gz` |
| `tada/files/<party>.tad\|.pro` | `<party>.json` (`detail.fileSize`) | `extract/tada/<party>.json.gz` |

## Privacy-limited path evidence

The archive-to-path-corpus pipeline uses a separate mode. It runs sequentially
and accepts explicit recording paths, with an explicit output directory:

```
go build -o /tmp/tad-extract ./tools/tad-extract
/tmp/tad-extract -path-evidence -root ~/ta-demos \
  -evidence-out /tmp/tad-path-evidence \
  ~/ta-demos/v3.1/1709.tad
```

This mode cannot be combined with `-dump`, `-moves` or `-validate`. Legacy
extracts include participant names and transport identities; do not feed them
to a privacy-limited corpus workflow. No source files are downloaded or changed.
The legacy directory scanner still expects its original archive sidecars; this
mode handles explicit paths and nameless sidecars independently. Source bytes
are read once; both decode passes and the digest use that same immutable buffer. Candidate-table and grammar hashes likewise cover exactly
the parsed bytes. The authored decoder subtree originated at `b6c4656d`;
evidence records the running binary's version-control revision and dirty flag.
A build without VCS information reports `revision: "unknown"`.

Output is `<evidence-out>/<source>/<id>.json.gz`, plus `manifest.json` and
`summary.json` for the current invocation. Known archive IDs retain their
numeric version-directory ID or 40-hex TADA ID. Other paths use `external` and
the source SHA-256 as ID; filenames are not copied. A neighboring JSON sidecar
must give the byte length (`bytes` in either layout, or legacy TADA
`detail.fileSize`); its digest is recorded, and optional `sha256` is checked against the exact source buffer.
If both size fields exist, both must agree. No other metadata is exported.
Absent, invalid or mismatched sidecars reject admission. Outputs are rewritten; rejection is a successful
extraction result, and the caller must inspect `admitted`. I/O/CLI failures
return nonzero with categorical errors, without source paths or packet text.

Schema 1, field names used by the corpus miner:

| Field | Meaning |
| --- | --- |
| `schema`, `parser` | schema number; parser `name`, `version`, VCS `revision`, `modified` |
| `source` | `source`, `id`, `sha256`, `bytes`, sidecar `metadata_sha256`, `expected_bytes`, optional `expected_sha256` |
| `map`, `max_units`, `candidate_width` | header map name and slot capacity; candidate definition-index width |
| `tables[]`, `grammar_table` | relative `path`, exact `sha256`, `rows`, `selected`, categorical `error` if unavailable/invalid |
| `catalog_confidence` | always `candidate`: table counts/commanders and body agreement do not prove the original installed catalog |
| `admitted`, `rejections[]` | whole-file gate; each rejection has `code` and `count`; partial evidence is retained for rejected files |
| `counters` | `framing`, `sync`, `joins`, `lifecycle`, `unit_data`, `complete_unique_roster`, `commanders_checked` |
| `instances[]` | `id`, zero-based `owner_ordinal`, one-based per-owner creation `ordinal`, unit `net_id`, `type_index`, `unit_name_candidate`, nullable `air_candidate`, `build`, optional `finished`/`died`, `positions[]`, `prefixes[]` |
| lifecycle object | `wall_ms`, nullable `tick_lower`/`tick_upper`, `clock_segment`, `speed_segment`, `sender_owner_ordinal`; `build` also carries creation low-word `x`, `y`, `z` |
| `positions[]` | scheduled true nonair pose: `tick`, `x`, `z`, signed `x_raw_fixed`/`z_raw_fixed`, `health`, `remaining`, `flags`, `occupancy_mode` |
| `prefixes[]` | each retained ground-prefix observation: `tick`, `blocked`, `points` as zero to three whole-world `[x,z]` pairs |
| `reclaims[]` | `owner_ordinal`, lower-bound `tick`, attribute-tile anchor `x`, `z` |
| `caveats[]` | machine-carried evidence boundaries described below |

`manifest.json.files[]` gives relative output `path`, compressed-file `sha256`,
`source`, `admitted` and `rejections`. The summary counts files, instances,
positions and prefixes, plus rejected-file counts per reason. Neither contains
player names, transport IDs, chat, addresses, recorder strings or raw error text.

The grammar table is `<root>/derived/unit_table.json`, a candidate stock-role
summary: rows need `unitname` and an explicit `roles` array (`"air"` selects the
air grammar; `[]` means known nonair). Missing roles remain unknown. Candidate
numbering tables are `<source>/mod_units.json` followed, for original-TA inputs,
by `v3.1/mod_units.json` and the stock table where applicable. All attempted
candidates are listed with hashes or unavailability/error codes. No mod grammar
is inferred from absent names or roles.

Admission requires EOF, a unique complete checksummed roster and block joins,
matching candidate unit count/commander indices, at least 50 agreeing movement
definitions, at least one nonair pose, and zero framing/compression/unknown-ID,
tick-gap/rewind, grammar, truncation, definition or status-length errors.
Ownership transfers with unresolved layout, ambiguous lifecycle joins,
nonincreasing prefix/pose ticks, and idle markers outside the observed width
also reject. These extraction thresholds are policy, not retail mechanics.
A failed body publishes none of its partially decoded prefixes. Rejected files
keep valid observations before/after errors but are never admitted.

Owner ordinals use unsigned transport-identity block ranks, watchers included,
without exposing those identities. Instance IDs combine source SHA-256, owner
ordinal and creation ordinal. Recorded death then creation always starts a new
lifetime for a reused unit ID. Identical live type/owner/creation-XZ repetitions
are counted as duplicates; indistinguishable reuse after an unrecorded death
remains unknown. Lifecycle brackets require preceding and following syncs from
the event sender in the same uninterrupted clock and speed segments. Both
bounds are null otherwise, including events at capture boundaries. A death's
event sender can differ from its victim's owner. Every speed record starts a
new conservative segment; no speed-law conversion is inferred.

Positions are sparse scheduled observations, with no interpolation. Whole
coordinates use arithmetic `raw >> 16` (floor for negative fractions); raw
signed 16.16 words preserve the exact transmitted values. Occupancy mode 1
includes boats and does not mean moving. Attached poses and air trajectories
are omitted. The optional mover-speed word is accepted only by total length
and skipped; complete movement-state lifetime remains unproved. Ground
prefixes are retained path lists, **not** actual poses, fresh collisions, player
orders or final goals. All valid observations, including identical consecutive
lists, are kept. Each instance's prefix and pose ticks must strictly increase;
same-tick repeats reject admission instead of being silently coalesced.
Reclaim ticks are only the previous sender sync, not exact removal times.
Creation high words remain uninterpreted, and their low words are not a later
position observation. Original catalog/map assets, filtering/coverage and
player intent remain unknown: admission does not turn these into exact replays.

## Output

`extract/<source>/<id>.json.gz`, one JSON object:

- `status` `ok` / `truncated` / `failed`, `error`; `setup` (header: format
  version, maxUnits, map name; recorder and date strings; the status records'
  transport identity (`dpid`), checksum result and TA version bytes;
  unit-data counts, notably `enabled_keys`); `naming` (which unit table named
  the types, or why none did); `decode` and `extract` counters.
- `players[]`: `number` (recorder participant number = packet sender),
  `side` (0 ARM, 1 CORE, 2 watcher), `color`, `name`, `dpid`, `block`
  (unit-ID block rank) and `builds[]`, in capture order. Each build:

| field | meaning |
| --- | --- |
| `t_ms` | capture wall clock: sum of record deltas since the first match packet |
| `tick`, `tick_next` | last unit-sync tick of the same sender before the start, and the first after it (`-1` if none): legacy capture-order markers (without clock/speed-segment validation; use path evidence for §9 brackets). 30 ticks ≈ 1 s of game time at normal speed. |
| `type` | unit type index from `0x09` (1-based position in the recording's name-sorted catalog) |
| `unit` | unit name, only when `naming.table` is set |
| `x`, `z`, `y` | low 16 bits of the `0x09` coordinate words: map pixels east, south, and height |
| `hi` | the three high 16-bit halves `[x, y, z]`, uninterpreted, when any is nonzero |
| `net_id` | unit ID; `dup` counts repeated starts merged into this one |
| `finished_ms`, `finished_tick`, `builder` | from the first `0x12` for this unit |
| `died_ms`, `died_tick`, `killer_player`, `killer_unit` | from the first `0x0c`; killer player from the attacker transport identity |

`extract/summary.json`: last run (files, throughput) and per-source totals —
status and failure reasons, raw vs extract bytes, builds, naming coverage,
enabled-key histogram, recorder strings, status-record checks, decode and
extract counters, subpacket counts by id. `extract/validation.json`: the
comparisons below.

## What is decoded

- Record framing, header, version-5 sectors (recorder/date strings only),
  player records, encrypted status envelopes (checksum, XOR mask, LZ77) with
  the 0x20 fields at 145/187 (transport identity) and 166–169, and the stored
  `0x1a` unit data (enabled-key count). Only format version 5 is accepted;
  no other version occurs in the mirror.
- Match packets: plain and LZ77 streams; subpacket splitting by the §6 length
  table including Smartpak `0xfd`/`0xfe`/`0xff`, raw `0x2c`, `0xfb`, `0x42`
  and chat overflow; the recorder checksum-failure marker; the per-sender tick
  counter with a resync check at every `0xfe` (equal / rewind / gap).
- `0x09` start (type, net ID, coordinates), `0x12` finish (built, builder),
  `0x0c` death (victim, attacker transport identity, attacker unit). Unknown
  `0x09` fields (offsets 5 and 19) are skipped and counted; unknown ids end
  the split of that packet and are counted.
- Unit-ID block check: every start's net ID against the sender's block from
  transport identities sorted ascending, watchers included [fmt tad §4].

Robustness: every length is checked against the bytes present; LZ77 output
is capped at nine times its input; a cut or corrupt file keeps everything
decoded before the damage. The test (`go test ./tools/tad-extract`) builds an
authored two-player recording in memory and covers framing, the status
envelope (plain and compressed), LZ77 round trips including overlapping
copies, the Smartpak tick model, the lifecycle joins, a stored-twice packet,
every truncation point and random byte corruption.

### Reader tolerances beyond the format description

Each is counted in `decode` and described in the format document:

- **Repeated bundles.** A packet body identical to the same sender's previous
  one at zero delta is skipped (`repeated_packets`, ~10% of stored packets).
  Repeats that differ (typically by an added `0xfc`) are kept; they show up
  as tick `rewind`s and as `duplicate_starts`/`finish_repeat`, which the joins
  absorb.
- **In-match `0x20` length.** 192 bytes, else 200 (TAF recorders from
  `taf-2023.12.03`), else 186 (the retail length), whichever lets the rest
  of the stream split to its end.
- **Checksum marker after the flag byte** as well as at the body start.

## Movement extracts (`-moves`)

`-moves [recording.tad ... | source ...]` writes
`<root>/extract-moves/<source>/<id>.json.gz` (`-moves-out` to change): every
player's units with their start, completion and death, joined with the
movement the owner broadcast in unit-sync records [fmt tad §7], plus the
player's feature reclaims and resource snapshots. It carries no player
names. The replay brain (`internal/aikit/brains/replay`) reads it.

**What is decoded.** Reconstructed unit-sync bodies (`0xfd` and raw `0x2c`;
an `0xff` idle carries nothing): movement entries (local slot, definition
index, then the ground or air grammar) up to the terminator, then the
scheduled status of slot `tick mod maxUnits`. Local slots are joined to unit
IDs through the sender's own block [fmt tad §4]. Ground entries keep the
blocked flag and up to three path points; air entries keep the selector-1
goal position (flag bit 5) and followed unit (bit 0) and the selector-2
position; the other Established fields are read past. Scheduled statuses keep
position, health word, construction-remaining byte and status flags;
attached statuses have no position. `0x0f` records with code `0xff` are
feature reclaim transitions at the feature's anchor tile [fmt tad §6.4];
`0x28` records give cumulative metal and energy produced, stocks, kills and
losses [fmt tad §8]. Non-sync records take the sender's last sync tick, the
lower end of their capture bracket [fmt tad §9].

**Width and grammar.** The definition-index width is recording-specific and
**Unknown** in the format; the reader takes the labeled candidate (bit
length of enabled keys + 1) and keeps the movement only when the recording
passes the legacy threshold: at least 99% of at least 50 movement entries
must name the same definition index as their unit's own `0x09` start, and
at least 99% of counted statuses must fit a candidate body length. This is
not the whole-file path-evidence gate. A recording without a matching candidate
unit table, or failing either threshold, is written with builds only and the
reason. The grammar is chosen by the definition's `canfly`, read from the
engine stock catalog summary (`derived/unit_table.json`, whose `air` role is
set exactly when a definition authors `canfly`) through the candidate table's
unit names.

**Unknowns skipped and counted, never guessed** (`sync` in the extract): an
air entry with selector 3 (no audited sender encoding) or a definition with
no known `canfly` ends that body's decoding; the scheduled status's trailing
mover speed has no presence bit, so it is never read, and a body is counted
as carrying it only when its length fits that form; a body fitting neither
length is counted as a mismatch. Identical consecutive path entries (repeated
bundles) are dropped and counted.

**Historical tool audit (2026-09-24, imported from `b6c4656d`).** These are
that source commit's measurements, not the current archive inventory or a
new path-evidence admission run. The 56 original-TA recordings in its local
mirror with two combatants on a matching candidate table (278 or 282 enabled keys,
width 9 in every one): 5,612,055 bodies and 5,508,711 idle syncs; 18,763,086
movement entries joined to a start, of which 18,735,495 (99.85%) name their
start's definition (the other 27,591 are dropped, their cause not
investigated; 10,095 more name a slot with no started unit); 13,343,039
ground and 5,430,142 air entries; no selector 3, no
unknown grammar, no truncated body and no length mismatch. Of 2,602,691
positioned statuses, 1,170,250 fit the form with the trailing speed word and
1,432,441 the form without it; 2,918,314 statuses name an empty slot and
91,050 an attached unit.

## Unit names

The type index is a position in the recording's own catalog, which varies:
the historical mirror held 276 to 513 enabled unit keys per recording. The archive's
`mod_units.json` is a candidate numbering with a trailing `ZZZ` row. A table is used
only when the recording's enabled-key count equals the table's unit count
(without `ZZZ`) **and** every ARM/CORE player's first start is that table's
ARMCOM/CORCOM. Candidates: the source's own `mod_units.json`; for original-TA
sources also `v3.1/mod_units.json` and the engine's stock catalog
(`derived/unit_table.json`, 278 units, which the older recordings use). ProTA
4.3 (`tada/*.pro`) and sources whose table is not mirrored yet stay unnamed;
their `type` and `setup.unit_data.enabled_keys` are kept for later naming.
Compatibility keys do not name units: their stored order is not catalog
order, and although the enabled key set is stable for a given catalog (a
usable fingerprint), no key-to-definition mapping is established.
Count, commander and body agreement leave the names/profile join conditional;
they do not prove every row of the recording's original catalog.

## Historical tool audit (2026-09-23)

The following counts and performance figures are preserved from the authored
tool at `b6c4656d`. They describe that older mirror and legacy mode, not the
new privacy-limited extraction, the current 156-file selection, or a new run.

`nice -n 15 tad-extract -workers 4` over the mirror as it stood (downloads
still in progress for some versions):

| source | recordings | raw | extract | build starts | named |
| --- | ---: | ---: | ---: | ---: | ---: |
| `v3.1` (original TA) | 84 | 1.32 GB | 7.8 MB | 228,925 | 41 |
| `tada` (original TA `.tad` + ProTA 4.3 `.pro`) | 100 | 0.79 GB | 7.8 MB | 222,570 | 59 of 59 `.tad` |
| `v4.5` | 395 | 4.28 GB | 37.6 MB | 1,109,827 | 0 (no table yet) |
| `v4.6` | 820 | 12.06 GB | 106.0 MB | 3,032,487 | 692 |
| `v4.7` | 471 | 10.62 GB | 87.6 MB | 2,531,712 | 378 |
| `v4.8` | 1,471 | 9.64 GB | 86.4 MB | 2,429,223 | 1,364 |
| **total** | **3,341** | **38.73 GB** | **333.2 MB (0.86%)** | **9,554,744** | **2,534** |

All 3,341 decoded to a clean end (no failed or truncated file). A full
forced pass decoded 37.8 GB in 60.8 s (622 MB/s) with four niced workers,
about 2.5 cores on average; an incremental rerun that found 114 new
recordings took 54 s, most of it re-reading existing extracts for the
currency check and the summary. Original-TA recordings left unnamed have
catalogs no mirrored table matches (for example 500–513 enabled keys in
TA Forever modded games).

## Historical validation

`-validate` results from `b6c4656d` over its historical mirror (2026-09-23).
These comparisons are observations in that sample, not universal format
proof or results of the current path-evidence admission:

**Against the archive's build orders** (`<id>.site.json`). The archive lists
exactly the starts that received a `0x12`, without the commander; per player
the counts are equal for 3,786 of 3,910 v4.8 players. Players join by
`dpid` = status transport identity (3,910 of 3,910; the two v3.1 views
without `dpid` join by side and colour or list order).

| | v4.8 (1,432 recordings) | v3.1 (2) |
| --- | --- | --- |
| site entries | 2,231,320 | 253 |
| unit type counts agree (clock-independent) | 99.999% | 100% |
| start: same unit type and second | 95.18% | 89.3% |
| start: within one second | 99.05% | 98.4% |
| completion within one second (matched pairs) | 96.6% | 96.4% |

In that historical sample, `type` matched the archive's `unit_category_id`, and the archive's
`started_ms` is `floor(tick / 30) * 1000` with `tick` the sender's last
sync tick — the same clock as `tick` here, off by one second at boundaries.
The remaining ~1% are consistent offsets of several seconds concentrated in
a few long recordings (for example 7 s in one segment of v4.8/12196), where
the archive evidently takes its tick from a different sync; unit types and
order still agree there.

**Coordinates.** Map pixels (x east, z south), the unit's footprint centre:
of 70,234 structure starts named by an original-TA table, 70,155 (99.9%)
lie at the footprint centre on the 16-pixel attribute-cell lattice
(`x mod 16 = 8 × (footprint width mod 2)`, likewise z). Engine world
coordinates are 16.16 fixed point with one map pixel = 1.0
(65,536 raw units) [03 §2.1], so `world = x << 16`. `y` runs 0–255 in
original-TA recordings (up to 455 in ProTA); treat it as the terrain height
at the start, not a world-unit claim. Starts in the first five minutes lie
nearest their own player's start (the commander's first position) in 95–99.9%
of cases by source (97.9% of 374,570 overall). The high halves (`hi`) are zero for structures and
nonzero for most mobile units started by factories; their meaning is
**Unknown**, which is why only the low halves are used. Four starts have an
x low half ≥ 65,490 (small negatives if signed); signedness is **Unknown**.

**Player↔sender.** Sender numbers join player and status records for every
participant (11,587 status records, all passing the envelope checksum); all
9,554,744 kept starts fall in the sender's unit-ID block.

## Known gaps

- Unit names only for matching candidate catalogs (see above); ProTA 4.3 and v4.5/v4.6
  are unnamed until a matching table exists.
- Positions in the build extract are start positions: for mobile units,
  where the factory started them. Later movement is in the movement extract
  above, for recordings passing the legacy candidate-width/grammar threshold.
- Building orientation is not recorded by `0x09` as described; the Unknown
  fields at offsets 5 and 19 are not interpreted.
- The status pair at 140/142 consists of **Unknown** scalars, not established
  map extents or screen resolution. Obtain map extents from the map assets.
- Historical residual split failures in `b6c4656d`: 18 subpackets in 8 of 3,341 recordings (a
  recorder-inserted tick-base record inside a following ally or chat-relay
  record) and one LZ77 stream; they fall in lobby traffic.
- `t_ms` is the capturing peer's wall clock; prefer `tick` for game time.
