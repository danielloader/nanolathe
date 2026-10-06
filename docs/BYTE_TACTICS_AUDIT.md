# Byte Tactics comparison audit

Audit date: 2026-10-05. External revision:
[`5f6435e547742ebfe1eef3f84d84188184adc102`](https://github.com/HectorBailey/byte-tactics/tree/5f6435e547742ebfe1eef3f84d84188184adc102).

This is an audit record, not a second retail specification. Corrected behavioral
contracts live in the owning `research/retail-executable-spec` category;
implementation decisions remain in their design documents.

The catalog audit found **four implementation defects, all corrected**, 38
bounded agreements (including two resolved external caveats), 24 mistaken or
overstated external interpretations, 59 host-boundary cases, 33 out-of-scope cases and 12 unresolved entries. Follow-through found
and corrected one additional anti-alias scratch defect. Two claims in the
supporting notes were also disproved. Among the remaining
work are the SweetSpot refresh chronology and Classic AI placement scratch
state; their dependent behavior remains explicitly incomplete.

The [expanded engine review](#expanded-engine-review) records the ongoing
function-inventory follow-up and additional corrections.

Read the [corrections and remaining work](#corrections-and-remaining-work) for
the practical outcome, the [catalog](#catalog-findings) for every B001–B170
entry, and [verification](#verification-and-limits) for the checks and limits.

## Scope and evidence

The external [bug catalog](https://github.com/HectorBailey/byte-tactics/blob/5f6435e547742ebfe1eef3f84d84188184adc102/docs/bugs.md)
contains 170 active entries: 38 named sections and 132 bullet entries. This
report assigns stable identifiers B001–B170 in their source order. Three
explicitly withdrawn introductory claims are excluded. Supporting-document
claims and the matching methodology were also inspected. This coverage is
exhaustive for that catalog at the pinned revision; it is not a function-by-
function equivalence proof of either complete engine.

The external target and our private retail executable have the same SHA-256:
`3b9c0fadabf3dc67ed5f05a70f1e1505a0c65deadd1a3c930adfe30e2a84995e`.
Findings were checked against our own executable analysis, then compared with
our research, implementation and relevant existing tests. External descriptions
were treated as leads. Names and inferred intent in a matching reconstruction
can be wrong even when the corresponding machine instructions match.

The external [license](https://github.com/HectorBailey/byte-tactics/blob/5f6435e547742ebfe1eef3f84d84188184adc102/LICENSE)
covers its tools; it does not license the reconstructed game source. No external
game code, raw executable analysis, executable layout or generated identifiers
were copied into Nanolathe. No external script or retail executable was run.
Private evidence retains the reproducibility trail outside this repository.

### What the matching claim establishes

The external project reports that all 3,267 game functions and the complete
executable match. Inspection of its checking tools and CI configuration found
two distinct checks: per-function comparison normalizes relocation operands
and padding, while the strict whole-executable build compares the resulting
file hash. The latter is materially stronger than an individual match label.
The ordinary placement tool also has an original-assisted path; that must not
be confused with the CI path that requires a build without the original file.

We inspected these checks but did not reproduce the compiler setup or complete
build. Consequently the advertised whole-file result remains an external
claim, not a result of this audit. Even a reproduced identical executable
would establish reconstruction fidelity, not the correctness of the prose bug
interpretations or the parity of Nanolathe's independently written engine.

### Disposition and confidence

- **Agreement:** the relevant behavior is already represented in Nanolathe.
- **Defect:** independent evidence establishes an implementation discrepancy.
- **Host boundary:** unsafe native-memory behavior or a replaced host facility
  does not define a portable gameplay rule to copy.
- **Misinterpretation:** the external claim misreads the independently checked
  operation, caller, control flow or consequence.
- **Unknown:** evidence does not yet settle the observable contract or its
  relevant reachability. The entry names what would settle it.
- **Out of scope:** the behavior belongs to an explicitly excluded subsystem,
  or to orphaned code with no established active caller.

Each disposition also has **Established**, **Supported inference** or
**Unknown** confidence. An established instruction or arithmetic operation does
not establish every claimed consequence. Host-boundary and out-of-scope rows
are not counted as proof of gameplay agreement. An unresolved gap is not
reported as a completed implementation.

## Expanded engine review

**Status: ongoing, not an entire-engine equivalence review.** The original
170-entry result remains exhaustive for the bug catalog only. The follow-up
uses the external inventory of 3,267 game-labelled function entries to look
for behavior the catalog never discussed. This inventory also contains host,
debug, library and excluded transport work; a game label is not a gameplay
contract. The older private research census has 86 missing entry starts, none
inside one of its previously indexed spans. These are coverage leads, not 86
proven missing engine behaviors. Some have rooted calls or address-taken
references; others have no direct reference in the preliminary census.

Each private row records fresh depth separately: pending, body inspected,
caller contract checked, or implementation compared. Even the last category
is bounded to named facets, not every path through that function. Previous
coverage labels, citation resolution, body reading and passing tests do not
establish equivalence. The raw row identities and traces remain outside the
repository. The category documents remain the behavioral authority.

### Practical value and review priority

The inventory count is a coverage aid, not a measure of player benefit. This
review has corrected normal interaction paths: cargo alignment and attachment
points, factory nanoframes entering repair scans, aircraft repair-patrol
admission, gamma changing when an options page opens, and music, camera and
minimap presentation. Production-path regressions cover these contracts;
passing them does not establish how often a player noticed the former error.

Many other rows concern unusual authored values, arithmetic wrap, unordered
floating-point inputs or malformed controls. Their practical stock-game impact
is unknown unless the row establishes a producer. They should not receive the
same implementation priority merely to reduce the pending count.

The next pass prioritizes ordinary stock-unit and screen behavior with a
concrete reproduction. Aircraft approach/facing has an ordinary stock-unit admission proof and is
the only new runtime work selected. Cached shadow source is deferred because
a visible stock-unit difference has not been demonstrated. Exceptional caption storage, malformed
campaign loading and other edge cases remain documented; broad implementation
work waits for evidence of practical value. The exhaustive inventory sweep is paused by the maintainer's direction. Its
remaining count is not a shipping gate or a target for further spending.

### Fresh coverage snapshot

This snapshot records review depth as of this report update. Each row counts
inventory entries assigned to that area; helpers can be routed imperfectly,
and cross-area supporting reads are not silently added. An implementation
comparison covers named facets only. **These counts are not percentages of
engine parity.** Remaining body and caller work still needs implementation
closure even when its pending column is zero.

| Area | Inventory | Pending | Body inspected | Caller checked | Implementation facets compared |
|---|---:|---:|---:|---:|---:|
| 01 Runtime | 277 | 145 | 121 | 0 | 11 |
| 02 Content/formats | 318 | 191 | 102 | 1 | 24 |
| 03 World/visibility/render/audio | 590 | 303 | 135 | 57 | 95 |
| 04 Units/orders/COB/movement | 404 | 93 | 246 | 19 | 46 |
| 05 Economy/features | 90 | 0 | 53 | 17 | 20 |
| 06 Combat | 83 | 0 | 43 | 20 | 20 |
| 07 Interface/front-end | 793 | 419 | 313 | 30 | 31 |
| 08 Sessions/AI/save | 712 | 526 | 132 | 28 | 26 |

The snapshot contains **1,677 entries still pending fresh inspection**.
Inspected bodies and caller contracts also need their remaining facet checks;
subtracting this count from 3,267 would not measure engine correctness.

### Newly verified implementation corrections

The following **Established** discrepancies were found beyond the catalog.
E004–E093 are corrected across the three implementation checkpoints, with
authored focused regressions. Whole-tree and performance validation is recorded
separately below. No stock-content impact is implied for explicitly bounded edge cases.

| ID | Correction and scope | Owning contract |
|---|---|---|
| E004 | Host milliseconds multiply by 30 with 32-bit wrap before division. The previous wide product disagreed after about 39.8 hours; the live input clock uses elapsed application time. | [01 §4.1] |
| E005 | Pending-speed means active below requested, sampled before hysteresis. Paused single-player passes and saves preserve that sample instead of recomputing it from the new speed pair. | [01 §4.3]; [08 "Scheduler and random state in saves"] |
| E006 | Signed-16 speed-slew counters wrap before threshold comparison. This distinguishes accepted restored extreme counters; ordinary production of those extremes is not established. | [01 §4.3] |
| E007 | Completed units seed current health from the signed low word of maximum health before script binding. Maximum health retains all 32 bits; nanoframes retain zero. | [04 §4.4] |
| E008 | COB sleep wraps the duration-times-30 product to signed 32 bits before division. Large authored durations can change sign and wake on a later zero-delta entry. | [04 §4.6] |
| E009 | An active path search continues while the combined budget is positive, charging its owner even below zero. Strict and Modern retain their separate carry and idle policies. | [04 R-PATH-01 §6] |
| E010 | Accuracy spread accepts a high-bit maximum-health word as an unsigned divisor. Only zero faults. A custom definition can reach the helper with positive current health. | [06 R-WPN-03 §4] |
| E011 | Reload uses signed low-word health, unsigned health division, a wrapped final product and signed truncation. High-bit maximum health is admitted; negative-health vectors establish consumer arithmetic without claiming every such state reaches launch. | [06 §4.2] |
| E012 | Allocated shooters whose death latch is set still receive their weapon visit before finalization. Tests preserve callbacks, resource costs, reload and RNG in Strict and Modern. | [04 R-COB-02 §2] |
| E013 | A retained allocated target whose death latch is set still reaches its synchronous target-point query. Autonomous acquisition retains its separate filters. | [06 R-WPN-04 §1] |
| E014 | Resource transfers cap first, then reject zero or NaN amounts. A NaN source stock refuses donor writes but does not suppress recipient credit for an ordered nonzero amount. Ordinary NaN producer reachability remains unknown. | [05 R-SHARE-01 §2] |
| E015 | Extractor footprint sums wrap to 16 bits and are interpreted signed for the rate. Authored large footprints distinguish the sign boundary from complete wrap; ordinary stock footprints are not asserted affected. | [05 R-PROD-01 §6/§6-A] |
| E016 | An unfinished TDF block comment retains its last body character, which can cause a parse diagnostic or close a section. The former blanket-to-end behavior changed that grammar result. The existing host policy preserving diagnostic line feeds remains. | [02 §4]; [fmt tdf] |
| E017 | COB translation preserves the signed script speed and wraps multiply/add before its inclusive signed arrival test. Authored negative-speed and overflow cases differ; shipped-script occurrence remains unknown. | [04 §4.6] |
| E018 | LOS compilation requests only the declared one-based line names, preserving missing slots; extra authored lines remain unreachable metadata. | [03 R-VIS-01 §3] |
| E019 | LOS quadrant expansion reflects the authored pair into the four specified quadrant blocks with signed-16 coordinate and negation widths. | [03 R-VIS-01 §3] |
| E020 | LOS line tokenization accepts commas and ASCII spaces independently, including space-only lists. | [03 R-VIS-01 §3] |
| E021 | A LOS ray skips an out-of-map point and can re-enter later, preserving the original point ordinal for the horizon test. | [03 R-VIS-01 §3] |
| E022 | LOS table and line counts narrow to signed 16 bits before their safe positive allocation and runtime use. An authored table count of 65538 selects two; negative allocation behavior remains unresolved. | [03 R-VIS-01 §3] |
| E023 | Each LOS value is limited to 511 bytes before tokenization. A complete last coordinate can be shortened without an incomplete list; larger content-file caps do not extend this grammar. | [03 R-VIS-01 §3] |
| E024 | Classic AI weapon scoring reads the unsigned low word of default damage. Authored 65536 and −1 previously reversed the low/high score outcome; the inactive sentinel and RNG schedule remain unchanged. | [08 R-P0-05 §5] |
| E025 | Meteor geometry widens random scaling products before division, stores signed-16 cells and wraps fixed-point shifts before the relevant divisions and stores. A large authored radius exposes a difference that survives the later fixed-point conversion; stock impact is not claimed. | [06 §6.5] |
| E026 | Definition bounds shift and wrap at signed 32-bit width before halving. Authored mobile footprints 2048 and 2049 distinguish the old shortcut; live placement and stock impact are not claimed. | [02 R-CAT-01 §7] |
| E027 | Classic AI class coefficients retain working precision between their actual single-precision stores. Fractional energy fields provide authored counterexamples; fractional metal-cost cases are arithmetic fixtures only because the retail cost loader reads integers. | [08 R-P0-05 §5] |
| E028 | The radar callback squares the full height-adjusted radius at signed 32-bit width, without narrowing it back to a signed word. Its spatial visitor still uses the unbonused range. | [03 R-VIS-01 §4] |
| E029 | Flame spawn-window and segment-expiry comparisons are signed; the current-tick due comparison remains unsigned. Signed-boundary tests include CRT draws. Zero-wrap sentinel lifecycle remains unresolved. | [03 R-STRIP-01 §3] |
| E030 | A GUI matrix result still permits the first gadget's ordinary visit before the surviving-result stop test. Pointer processing can replace it; rejected links can clear it. Callback, focus, capture and stage regressions exercise the production path. | [07 R-WGT-01 §1–§2] |
| E031 | An admitted label link to a non-button fires that target after focus setup; a missing target fires the label. Rejected targets consume an admitted quickkey but permit later visits. Production callback and modal-close tests cover the correction. | [07 R-WGT-01 §7] |
| E032 | Save routing tests case-insensitive scalar presence, not integer value 1. Campaign markers containing zero or strings now take continuation; binary boxes do not count. The distinct kind-2 route is recognized but explicitly rejected pending a verified retained setup/player/preference snapshot; it is not reported implemented. | [08 "Battle versus campaign continuations and timing"] |
| E033 | Failed static sound registrations keep their failure on playback and duplicate registration; files appearing later and independent direct-path loads do not repair the alias. Explicit host sample-cache supply remains supported. | [03 §8.3] |
| E034 | Classic AI extractor predicates consume the stored single value and reject unordered inputs; net-energy bonuses retain their distinct unordered admission. Tiny authored extraction values test the storage boundary; NaN cases are consumer fixtures without a claimed retail producer. | [08 R-P0-05 §5] |
| E035 | Point-selection cues follow the resulting selection, so Shift-toggle-off is silent. Rectangle cues scan eligible resulting selection, including retained units outside the rectangle. Production gesture tests cover both. | [07 §9] |
| E036 | The net-energy query keeps its product at working precision through the return. An authored tidal multiplier 1.98 and map strength 10 now yield class coefficient 99 instead of 98. Ordered input gates remain separate from the consumer's unordered gates. | [05 R-PROD-01 §1]; [08 R-P0-05 §5] |
| E037 | Music idle query/stop polarity, zero-track entry, sequential submission-before-wrap and retained-next deduplication now follow the controller contract. The host file backend retains its documented idle/error policy on failed opens; native device responses are not inferred. | [03 R-AUD-01 §4] |
| E038 | Drag-scroll entry clears tracking but preserves the pending glide. Runnable entry ticks can continue it; the first later step uses the entry anchor and replaces desired origin. Paused, zero-tick, multiple-tick, first-step and release paths are covered through the production controller. | [07 R-CAM-01 §11–§12] |
| E039 | Ordinary next/previous build-page keys request their cue even without a live page owner; an accepted digit-page request cues even when the page stays the same. Rejected digit requests remain silent, and adaptive-sidebar host policy is preserved. | [07 R-HUD-03 §6] |
| E040 | Weapon sounds retain anonymous path registration identity, including load-order admissions from overwritten and ID-less weapon sections. Named alias collisions cannot redirect them; shared capacity, failed-registration retention and ordinary battle reload lifetime are preserved. | [03 §8.3], [02 "Weapon record"] |
| E041 | Follow cycling uses the tracked slot even after deselection; null or out-of-owner tracking starts after the owner's actual first allocation slot. Forward/reverse wrap and permuted owner slices use committed bounds. | [07 R-CAM-01 §12] |
| E042 | Numbered-page product-name resolution excludes the final loaded child. That child retains its incoming grey state, including prior download clearing. No stock-page impact is claimed. | [07 R-HUD-03 §6] |
| E043 | Authored page discovery strips the final dotted suffix, continues past page 7 to the first missing or empty file, and narrows the final count to a byte. The selected page's three-bit field does not bound discovery. Pages 8 and above are counted but cannot be selected: the page producers operate modulo eight on that field, so with nine or more pages `.`/NEXT go from page 7 through a shown zero field to page 1; the HUD follows these field operations. | [02 R-CAT-01 §5]; [07 R-HUD-03 §6] |
| E044 | Translation values keep their first 254 bytes before forward or reverse lookup, including byte splits inside multibyte text. Oversized source-name residue is a separate Unknown with an explicit host placeholder. | [02 "Translation table"] |
| E045 | Repair-patrol thresholds and feature-fit sums retain working precision through the comparison, including inclusive and unordered admission. Finite authored resource cases distinguish the behavior; Modern guard's approved comparison is preserved. | [04 R-ORD-01 §4, §7] |
| E046 | Repair-patrol feature sampling retains raw fixed-point centres and odd-diameter halves, traverses Z before X, and preserves tournament tie/RNG order. Copied Y and pathological overflow outcomes remain explicit gaps; existing host bounds are retained. | [04 R-ORD-01 §4] |
| E047 | Aircraft repair patrol reaches feature tournaments with healthy stores; only ground patrol has the early both-stores hold. Community reclaim-only resumes each at its own verified boundary; assist-only remains unchanged. | [04 R-ORD-01 §7], [community-patch-engine.md CP-CON-3](../research/extensions/community-patch-engine.md) |
| E048 | Delayed stream callbacks cancel the recorded timer identity, leaving an older lost registration able to reopen the latest path. Missing/undecodable replacements preserve current playback. Shared CD/stream timer competition remains T23. | [03 R-AUD-02 §1] |
| E049 | Briefing narration follows the resulting widget stage, including nonzero-to-nonzero starts. Callback cue/stop order and accepted Start/Prev stops without a current narration key are preserved. | [07 R-FE-01 §4] |
| E050 | Side name, prefix, commander and font values retain their loader byte limits before consumers. Absent-font lifetime is now distinguished from a present font's fatal failed load; the host's existing absent/empty rejection remains explicit. | [02 "SIDE and battle interface data"] |
| E051 | Full visibility rebuilds clear the saved coverage byte but retain the tile pair before the ordinary ray throttle. Stationary emitter heights 1–5 therefore remain unpublished after the refill; height 6 or a changed tile publishes. Circular publication remains direct. Tests also check later retirement and overlapping observers. | [03 R-VIS-01 §1]; [08 R-ENTRY-01 §7] |
| E052 | Pickup lowering waits for the cargo heading through the ordinary marker. Live landing markers transform the selected piece with target orientation and animation; world-point addition wraps at signed 32 bits. Pickup offsets use the carrier orientation once. Tilted-carrier arithmetic is a consumer fixture, not a complete stock flight history. | [04 R-AIR-01 §9, §14.1]; [04 R-REV-02] |
| E053 | Caption acknowledgement stores the low byte of stage times five while retaining the live stage. Reopening derives the stage from the stored byte. An authored 53-stage control distinguishes this from a wide setting. | [07 R-FE-01 §6] |
| E054 | Scroll-slider initialization preserves the stored single-precision reciprocal. Its callback reads an entry knob beyond normal pointer travel without clamping it first, floors at one and stores a byte. Authored travel 66 distinguishes both the middle and endpoint. | [07 R-FE-01 §6] |
| E055 | Visual-page entry seeds sliders without invoking value callbacks. Opening a page no longer quantises the stored gamma; explicit changes still commit, and the speed page retains its callback pass. | [07 R-FE-01 §6] |
| E056 | Baked radar pictures reduce fixed two-by-two blocks at the stored stride. The former crop and ratio resize disagreed for safely sized authored sources. An undersized or incomplete authored source is treated as absent: the battle radar uses the generated terrain picture and reports a HUD asset warning, as explicit host policy without claiming retail's unsafe outcome. | [03 §3.7]; [fmt tnt] |
| E057 | Help, briefing and game-settings children request the Options cue for their recognised page controls and OK, before paging or closing. Unknown names and cleanup without a fired gadget request nothing; actual playback remains gated by audio. | [07 R-FE-01 §7] |
| E058 | Mission radius conditions use the shared spatial query's wrapped bounds and distance arithmetic. The session adapter excludes attached cargo, preserving the distinct annihilation predicate; scanning continues after completion with only one cue. | [08 R-TRIG-01 §5] |
| E059 | Smoke uses a signed spawn-window comparison and unsigned current-tick admission. Actual producer/sweep fixtures on both sides of the signed boundary lock changed CRT admission, while unsigned empty-container expiry and vent behavior stay separate. Full zero-deadline wrap remains unknown. | [03 R-STRIP-01 §3] |
| E060 | Speech verbosity stores the low byte of stage times five, retains the live stage, and derives enablement from the stage independently of the wrapped byte. Reopening derives a new stage from the stored byte. | [03 R-AUD-01 §2] |
| E061 | Entering Repeat copies the retained requested track into the music-page selection and updates the mode without immediately playing a track. The subsequent per-frame selection/request refresh is covered separately by E068. | [03 R-AUD-01 §4] |
| E062 | Sound MODE requests the Options cue after applying the updated sound state and controls; RESTORE and UNDO request it after reopening the panel. SPEECH retains its distinct cue-before-write order. A cue request does not guarantee audible playback. | [07 R-FE-01 §7] |
| E063 | Sound-category lookup scans retained names in file order, choosing the first duplicate rather than a map winner. The authored query is bounded to 99 bytes and the chosen or converted ordinal narrows to unsigned 16 bits before lookup. Authored fixtures cover duplicate/long names and positive/negative numeric wrap; no stock impact is claimed. | [02 R-CAT-01 §5] |
| E064 | Full-cell and masked dither fog write even checker parity, preserving the same phase through clipping. Software, atlas and ordered GPU paths now agree; authored device-pixel fixtures distinguish the old phase. | [03 §3.3] |
| E065 | Shared repair-radius scans exclude attached cargo before geometry or visitor dispatch. Production factory nanoframes no longer enter the repair tournament; actual release restores their eligibility and corresponding RNG draw. Canonical filing and detach order remain separate. | [04 R-COLL-01 §11] |
| E066 | The executor's message-ring tail retires an expired retained entry even when the current text-line setting is zero. Zero prevents new posts, not expiry of an existing span; ordinary UI production of that retained state is not asserted. | [01 R-PLAT-02 §8] |
| E067 | Elapsed-delta GAF stepping advances when the remaining hold is nonpositive, rather than one unit early. The separate tick-step rule remains distinct. Native low-word width and single-frame bypass are still implementation gaps. | [03 §4.4] |
| E068 | Both music-page input adapters poll displayed decimal text after widget service and before action dispatch, including no-action passes. Mismatch copies the logical next track, refreshes details without erasing advanced button stages, and writes the request in Repeat. Equality writes nothing. Missing or wrong-kind text controls retain an explicit host fallback. | [07 R-FE-01 §6] |

| E069 | Minimap drawing completes each unit's regular marker, hover marker, sensor circles and weapon circles before advancing to the next unit. The former grouped passes changed overlapping pixels. | [03 §3.9], [03 §3.10] |
| E070 | Minimap hover admission is independent of regular marker blink suppression. An otherwise visible contact can retain its hover marker during the suppressed regular phase. | [03 §3.9] |
| E071 | Minimap radar and sonar circles each consume their own signed nonzero authored distance; choosing only their maximum omitted an admitted circle. | [03 §3.9], [03 §3.10] |
| E072 | Admitted minimap radii reach solid or dashed drawing even when projection produces zero or a bounded negative radius. Zero can draw the centre pixel. Extreme trigonometric and clipping overflow remain outside the established implementation boundary. | [03 §3.9], [03 §3.10] |
| E073 | Fresh music initialization seeds requested track one independently of logical next zero. A later probe is a separate next-track writer. Repeated Open and Close/reopen remain a lifecycle gap. | [03 R-AUD-01 §4] |
| E074 | Music transport uses the controller selector: stopped/paused selection changes logical next without starting playback; playing selection follows the play helper. Play itself does not refresh page details or write a Repeat request. | [03 R-AUD-01 §4]; [07 R-FE-01 §6] |
| E075 | Music options retain selection across ordinary roots, snapshot the actual request, and preserve distinct page departure, Undo, Restore and Cancel ordering. Stored mode/enable preferences are not conflated with live controller state. Both production widget adapters have query-time lifecycle regressions. | [07 R-FE-01 §6] |
| E076 | Attachment and release update canonical spatial membership immediately. Release inserts at the retained sector head before the mode write, while carried units retain a reference without a top-level link. Rectangle traversal checks each parent's cargo independently in cargo-list order. Composed recursive-save restoration verifies the same owner and insertion chronology. | [04 R-COLL-01]; [04 R-AIR-01]; [08 R-SAVE-02 §6] |
| E077 | Carried movement quantises the committed hang position before copying carrier velocity. The previous extra velocity addition could change the occupied anchor. Extreme wrapped footprint-bias arithmetic remains a separate gap. | [04 R-COLL-01]; [04 R-AIR-01] |
| E078 | Rolling back a never-created factory product clears its movement/index ownership before freeing the unit slot. This is host lifecycle cleanup needed by the corrected canonical owner; the defensive attached rollback fixture does not establish a retail producer history. | [DESIGN_MOVEMENT_PATH](DESIGN_MOVEMENT_PATH.md) |
| E079 | Model waterline thresholds retain the unsigned low byte after the positive-depth gate, including active cutoff zero. Body tint/erase, mobile shadows and geometry packets share the correction; authored captures distinguish adjacent wrapped cutoffs. | [03 R-REN-03A §8]; [03 R-WATER-01 §2] |
| E080 | The statistics strip admits only a strictly later signed clock value and samples again for its next deadline, even at rest. Production uses the battle's existing monotonic source. Per-battle timer ownership remains an explicit host boundary pending full reset-lifetime research. | [07 §6] |
| E081 | Classic AI placement biases the raw origin in 32 bits before shifting to a cell. The former widened intermediate disagreed at the signed-coordinate boundary. Direct arithmetic tests establish the correction; stock producer reachability remains unknown. | [08 R-AI-03 §3] |
| E082 | Classic model staging chooses its final union and seeds cached planes once before reveal, live pieces and cargo. Equal-size copies are raw; resized color and key copies independently skip the transparent index. Live key-one pixels survive later cargo composition. Enhanced consumers are corrected separately in E091. | [03 R-REN-03A §4] |
| E083 | Classic AI net-resource queries retain working precision until the below-one comparison. A finite direct aggregate fixture changes the resulting energy score from 50 to 70, disproving the suggestion that later clamping always hides the difference. Economy storage is unchanged; stock-match incidence is not claimed. | [08 R-P0-05 §3] |
| E084 | Classic AI stock gates reject unordered values before any reservoir draw; net-resource and production bonus comparisons take their below-threshold arms for unordered values. Direct aggregate tests establish these consumers without claiming a retail nonfinite producer. | [08 R-P0-05 §3] |
| E085 | Primary handler results above nine drain both order segments. Full cancellation unlinks each record before cleanup and follows the live chains, including cancellation-created work. The rear default keeps its distinct single-record behavior. Injected tests establish consumers; a shipped above-nine result producer remains unknown. | [04 §3.3], [04 R-MOV-03 §6] |
| E086 | The queued-order duplicate test uses wrapped raw-coordinate arithmetic and unlinks the matching record before cancellation cleanup. Signed-boundary and callback fixtures reject the former implementation; ordinary stock coordinate reachability is not claimed. | [04 R-MOV-03 §6] |
| E087 | Path cursors begin on each assigned slice's first allocatable slot and advance before inspection. The battle-entry scheduler visit persists into ordinary ticks. Polling, idle batching and production entry composition now share the verified initialization. | [04 R-PATH-01 §6], [08 R-ENTRY-01 §8] |
| E088 | Label centering writes the live signed-16 X during initial paint, before later caption replacement. Empty captions still resolve; appended-page translation precedes resolution. Map-name, hover-help, modal-title and message-box fixtures retain that position. | [07 R-WGT-01 §7], [03 R-FONT-01 §6] |
| E089 | Message-box resizing gives every label the final panel width and centre attribute, including authored and hidden labels. Its final build makes empty-link labels inert again. Named-only resizing and the former active appended captions disagreed with this sequence. | [07 R-FE-01 §9], [07 R-WGT-01 §7] |
| E090 | Transport admission accepts zero or unordered construction scalars and rejects ordered nonzero values. Direct scalar tests establish the consumer. The earlier claim that only construction writes the scalar was disproved by save restoration; naturally generated NaN and a complete restore-to-command history remain unknown. | [04 §10.2], [04 "Simulation and identity"] |
| E091 | Enhanced staging preserves the original cached winner while applying resized key filtering before reveal, live drawing and ordered child merges. Seeded child shifts preserve saturation across the full signed-height difference in merges, clipping and reflections. Descriptor-absent behavior is unchanged; cached shadow source is established but its implementation is deferred; current-pose bounds remain open. | [03 R-REN-03A §4], DESIGN_GPU_RENDERER §22 |
| E092 | Queued hover attacks compute their approach from the target's current position when the attack starts, while retaining the stored order goal. Stock Brawlers and Rapiers admit this path; a queued attack behind movement demonstrates the stale-position defect with authored units. | [04 R-AIR-01 §8] |
| E093 | Dogfight facing comparisons use signed whole-unit direction components before multiplication. Finite near-sideways targets can consequently take the pursuit-counter or arrival-restart branch instead of the ahead branch. The ordinary evasion insertion clears its target and immediately completes in retail too; research now distinguishes that composition from the isolated executor. Stock fighters admit these paths; fixtures check resulting goals and RNG use, without claiming a measured match frequency. | [04 R-AIR-01 §8] |

**Stock LOS check:** reachable groups 1–8 in the reference install have the
same complete ray-sequence multisets before and after the correction,
including point ordinals. Projected coordinate sets and visited cells on an
authored asymmetric height field also agree. The asymmetric authored fixtures
establish the defects; this check found no stock-table effect. Malformed
negative counts and missing coordinate tokens remain outside the established
safe-input contract.

### Expanded validation

**Third checkpoint: correctness and performance validation complete.** The integrated candidate passes `tools/check` and
`tools/check-retail --full`, including native and AMD64 fingerprint locks and
actual GPU fixtures. The initial full retail run rejected changed Strict and
Community battle fingerprints; Modern's battle locks remained unchanged. Before correcting cursor initialization, the existing Strict
effect-pool scenario stopped reaching capacity, so merely changing its expected
hash would have invalidated its purpose.
An overlay restoring only the previous movement-binding time reproduces all
six Strict, Community and Modern warm/final locks while retaining the other
attachment corrections. Resetting only the physical path cursors after the
entry prime reproduces those locks and the original pool-exhaustion result.
The native initialization does include that scheduler visit; tracing its
constructor also exposed a separate first-poll offset in our provider. That
correction now passes focused checks and the original seed-five scenario again
reaches capacity: first sampled full at tick 2314, with 36 refused admissions
by step 4500. Disabling effect timing changes its fingerprint. Integrated
fingerprint validation confirmed the predicted changes, and the bounded locks
have been updated with their attribution. At that intermediate checkpoint, initial compositions, long
campaign-map trajectories and Modern benchmark locks remained unchanged; the
subsequent aircraft attribution is recorded below.
E079–E093 pass their
affected package checks; the AI and staging regressions reject the previous
implementations. The numeric architecture guard also passes.
The two ordinary-aircraft regressions reject their corresponding old implementations.
Correcting dogfight facing changes the Strict and Modern 1,500-step battle
locks and the Strict effect-pool lock. An independent overlay restoring only
the old facing calculation reproduces all three prior locks; restoring only
the old hover input does not. Initial states, 600-step checkpoints, Community,
and both 6,000/54,000-step Ashap trajectories remain unchanged. The same pool
scene still first samples full at tick 2314 and now refuses 151 admissions by
step 4500, preserving its timing coverage. These attributed locks are updated.
The integrated Enhanced correction passes actual-device fixtures, including
cold, warm and retained replay, native and doubled raster packets, both atlas
pages and forced parallel placement. Its authored fixture increases model
passes from two to six and vertices from 424 to 536, adds 158,976 bytes of merge
scratch and still uses one atlas page. This is a fixture cost, not a measured
battle regression. Third-checkpoint simulation, renderer and path checks are complete. The prior two landed checkpoints passed their
required pre- and post-landing gates.


The final third-checkpoint candidate passes `tools/check` and
`tools/check-retail --full`, including AMD64 locks and real-device GPU fixtures,
after both aircraft corrections and current-main integration. Paired live
renderer measurements use Expanded Confluence scene 5, seed 7, 1920×1080,
300 pre-window ticks and 180 measured draws. Classic DrawWork median/p95 is
28.911/38.184 ms on the baseline and 24.614/28.475 ms on the candidate; Modern
is 12.900/20.522 and 12.768/15.033 ms respectively. Both comparisons have
identical scene metadata, per-frame censuses and saved battle states: 314–344
units, 5,543–5,557 features, 2–5 burning features, 89–135 projectiles and eight
active nanoframes. Both candidate captures were inspected. Classic's captured
pixels are unchanged; Enhanced's capture differs at only 26 pixels. This
scene therefore does not establish a large visible benefit from staging.
Allocated bytes per frame rise from 1.247 to 1.306 MB in Classic and 1.304 to
1.398 MB in Modern. No timing regression appears in these samples; the earlier
baseline's higher timing variability prevents a speedup claim.

Two sequential quick simulation samples per revision reproduce their own
final fingerprint. The closer second pair measures baseline/candidate wall
median 1.860/1.719 ms, p95 2.941/2.789 ms, maximum 3.996/5.079 ms and process
CPU 1.967/1.864 ms per tick. Allocation is 104.4/87.0 kB per tick. The corrected
fighter decision changes the trajectory: final live units 733/734, projectiles
16/67, effects 241/262 and burning features 26/12. These are healthy combat
workloads, but their different work forbids attributing lower cost to an
implementation speedup. The first pair had wider baseline timing variability
and led to the matching repeat, not an optimization claim.


The final path matrix passes all 543 case/rule/size combinations, three
repeats each. Repeats have consistent inputs and outcomes within each revision.
The comparison tool correctly refuses a whole-matrix comparison because seven
`traffic/waves` groups have different dynamic input hashes; these are excluded
from cost comparisons. The remaining 536 groups match host/runtime, content
and authored input guards. Of these, 91 change their partial trajectory and
79 change at least one reported behavior measure. No group gains an unexpected
unit removal. Near-goal counts within the fixed windows decrease in 19 groups
and increase in ten; this is not a movement-quality improvement claim.

Two Modern cases have both a p95 increase above 20% and an absolute increase
above 100 microseconds. `avoid/open_big_b` at 256 units changes from 212 to 180
near goals and from 573.5 to 716.7 microseconds p95. `traffic/capacity_control`
at 16 units changes from six to one near goals and from 63.0 to 191.3
microseconds p95. These regressions are real bounded outcomes. A diagnostic
overlay restoring only the previous physical cursor initialization reproduces
the first case's full baseline outcome; restoring that initialization and the
previous battle-entry binding time reproduces both complete baseline outcomes.
The associated p95 measurements return to 565.5 and 61.3 microseconds. Thus the
corrected retail scheduling explains these tradeoffs; no Modern traffic policy
was removed or replaced to hide them. Any compensating Modern policy is a
separate decision, not an inferred retail fix. The median p95 ratio across
compatible groups is 0.962, which does not erase the individual regressions.


Focused regressions for the integrated corrections pass. The first full retail
run exposed the changed battle locks and a stale bomber test that stopped
weapon visits at the death latch. The test now checks final allocation removal.
Diagnostic overlays restoring only the prior path-budget and combat-lifetime
implementations reproduce every previous fingerprint lock; restoring either
family alone does not. The changes affect the benchmark warm/final states and
the effect-pool scene; the initial compositions and the short/long Ashap locks
remain unchanged in that comparison. The final integrated arithmetic candidate
repeated those same values, and the locks now cite the changed contracts. The full integrated retail gate passes on ARM64 and its AMD64 fingerprint
run, including the extended trajectories; real-device GPU fixtures also pass.

The full path matrix completed 543 case/rule/size combinations with three
repeats each on baseline and candidate. Host/runtime, content and scene inputs
match for 534 pairs. Nine `traffic/waves` pairs change their later relative
commands because those commands use the units' corrected positions; the
standard comparator correctly rejects treating these as identical inputs.
Among the compatible pairs, near-goal arrivals improve in 42, worsen in 38
and remain equal in 454; unexpected removals remain zero. Median per-case
CPU ratios across the repeated samples are 1.002 for p50 and 1.013 for p95.
These timing corrections have mixed fixed-window arrival effects; this is not
a claim of universally improved pathfinding or complete performance parity.

The first candidate simulation measurement overlapped verification work and
was discarded for performance assessment. A quiet repeat with matching scene,
seeds, catalog and two-worker runtime measured median 1.874 ms and p95 3.011 ms
per tick, against baseline 1.791/2.493 ms; process CPU was 2.005 versus 1.866
ms per tick. Both have active construction and thousands of features; the
changed battle ends the window with 733 versus 736 live units and 6217 versus
6214 features. A single short sample with divergent combat workload cannot
isolate an implementation cost. An alternating baseline/candidate repeat gave
1.838/2.104 ms process CPU per tick and 1.779/1.990 ms median wall time. A
diagnostic build restoring only the old path/combat behavior reproduced the
old final fingerprint, 1.828 ms CPU and 1.735 ms median, with allocation volume
also returning to baseline. This attributes the changed workload and cost to
those verified behavior corrections in this scene; it does not erase the
roughly 9–14% CPU increase observed across the two paired short samples.
The largest phase increases are orders, unit work and publication.

After the patrol and visibility follow-through and integration of current
main, both `tools/check` and `tools/check-retail --full` pass. The earlier
unused audio/AI helpers and stale hidden-gadget fixture were corrected without
relaxing behavior locks. A fresh quiet simulation sample on that integrated
candidate measured median 1.945 ms, p95 2.961 ms and process CPU 2.058 ms per
tick. Its final fingerprint and closing workload match the earlier corrected
candidate (733 live units, 6217 features, 26 burning features, 127 fragments,
11 builds and 45 cumulative deaths). This adds no evidence of a new trajectory
change from the subsequent patrol/visibility work in this particular scene.
That first expanded checkpoint is landed, and both gates also pass after
landing. The second E052–E068 checkpoint is also landed and passed both gates before
and after landing, including
full retail scenarios, ARM64 and AMD64 fingerprint locks and real-device GPU
pixel fixtures. No further fingerprint changes were needed.

The next transport/options/radar candidate also completed paired live battle
runs with both renderers: 180 measured frames after the documented warm-up,
Expanded Confluence scene 5, seed 7, 1920×1080 and two runtime workers. All
scene metadata match. Classic median/p95 host draw work was 25.316/29.811 ms
on the landed baseline and 25.156/29.639 ms on the candidate; Modern was
13.096/16.161 and 13.087/15.697 ms respectively. Each run retained 314–344
units, 5543–5557 features and 2–5 burning features. Both candidate battle
captures and the separate unobscured production radar picture were visually
inspected. Each renderer pair has identical ending state files, per-frame
censuses and PNG captures in this scene. These short samples show no measured host-work regression in that
scene; GPU and display presentation time are unavailable. Later smoke, mission-condition and sound changes still need their own
integrated validation. The first combined gate run found a radar read-cap
fixture whose tiny baked source depended on the removed ratio resize. The
fixture now supplies a safely sized fixed-half source and still checks both
exact read-cap admission and generated-picture fallback; its focused checks pass.
A subsequent combined run found two audio fixtures that populated only the
lookup map, without the category order supplied by the real compiler. Their
authored helper now supplies both, preserving the end-to-end assertions.
After both fixture corrections, the integrated E052–E068 candidate passes
`tools/check` and `tools/check-retail --full`.

Fresh sequential renderer pairs on the final candidate used the same scene,
worker count and 180 measured frames. Classic median/p95 host draw work was
26.632/30.426 ms on the landed baseline and 26.367/30.366 ms on the candidate;
Modern was 13.140/15.478 versus 12.838/15.357 ms. Each pair has identical
per-frame censuses, ending state files and battle PNGs; both candidate captures
were inspected. The workload remains 314–344 units, 5543–5557 features,
2–5 burning features and 198–238 moving units. The benchmark has dithered fog
disabled, so checker parity is established by the separate software and
real-device authored pixel fixtures, not by these unchanged battle images.

The final simulation sample preserves the earlier fingerprint, both RNG draw
counts and complete closing census. Its first median/p95/process-CPU sample
was 2.417/3.667/2.532 ms per tick. A fresh matched baseline/candidate repeat
measured 2.275/3.729/2.439 and 1.872/2.981/1.994 ms respectively, with identical
workloads and fingerprints. The timing variation does not establish a stable
new regression or a speedup. Candidate allocation remained approximately
104.5 kB per tick; these quick windows do not substitute for long-match gates.

### Research corrections and remaining questions

**Established, implementation follow-up pending:** parent shadows copy the
retained cached body before staging, reveal, live pieces and cargo. The
Classic implementation currently includes live/reveal output; Enhanced can
also include grouped cargo. Keyed Diggers additionally bypass the vehicle,
hover and floater shadow gates that still apply to keyless Diggers. The authored key-plane flag and construction are independent of Digger.
All stock Diggers in the checked census are already keyed; no stock allocation
impact is demonstrated. A visible stock shadow-source difference is also
unmeasured, so runtime changes are deferred. Approved softness and
supersampling remain separate [03 R-REN-03D §1, §6].

**Established consumers, ordinary-play reproduction under investigation:**
dogfight facing uses whole components after negation, while hover-attack's
initial jittered approach reads the live target instead of the retained goal.
Current host code differs at those points. The finite examples establish the
consumer discrepancy, not stock symptom frequency [04 R-AIR-01 §8].

**Established, lower-priority content edge:** campaign enumeration spends a
failed load attempt without advancing its current name. Persistent empty or
unreadable files can prevent later names from being visited while retaining
an admitted prefix. Our discovery/error boundary differs; syntax failure and
short-read histories must not be conflated with that predicate
[08 R-CAMP-01 §1].

**Established, classic implementation corrected:** enlarged model staging
copies color and height-key planes independently through the same transparent
index; unchanged dimensions copy both planes verbatim. An opaque parent pixel
with key one can therefore admit a child with key zero only after enlargement.
The correction preserves the native sequence: choose the final union, seed
the cached planes once, then apply reveal and live pieces. A copy-helper-only
counterexample loses all authored live pixels, while the corrected production
path retains them. Enhanced retained/direct staging now preserves the cached
winner separately from its seeded key, with actual-device checks of downstream
consumers. The parent shadow source is now established separately below; current-pose
bounds remain unknown [03 R-REN-03A §4].

**Established, implemented in the candidate:** model waterline
thresholds keep the unsigned low byte after the positive-depth gate. An
authored model feature at terrain height zero beneath sea level 210 reaches
the ordinary presenter with threshold four, not a saturated threshold. Stock
frequency is not claimed [03 R-REN-03A §8], [03 R-WATER-01 §2].

**Established, implemented in the candidate:** the statistics-strip
step uses a strict signed deadline and two clock samples; a resting detent
still advances the deadline. The former elapsed-time helper differed at
exact equality. The adapter uses the existing monotonic clock and exposes
both samples in production tests. Cross-session deadline ownership remains a bounded gap
[07 §6].

**Established consumer corrections, producer reachability Unknown:** an
unsigned primary handler result above nine drains both order segments; the
secondary default removes only its current record and reloads. The queued
order toggle compares wrapped coordinate differences. Host routing,
full-purge callback chronology and duplicate-goal subtraction are corrected
in E085–E086. Injected results and extreme stored coordinates establish
consumer differences, not ordinary stock-game histories [04 §3.3],
[04 R-MOV-03 §6].

**Established research corrections, bounded implementation agreement:** target
notifications follow current observer membership; the static observer flag is
only a constructor admission rule. Target removal calls the receiver before
conditionally clearing an unchanged target, then reloads the live list head.
The traced ordinary receivers do not rebind, so this review does not establish
a runtime discrepancy for them. Separately, code-two repair admission can
accept a live death-latched target with health zero; it rejects a negative
health value after unsigned conversion. The previous blanket statement about
latched targets was too broad [04 R-MOV-03 §7], [04 R-ORD-02 §7].

**Falsified comparison:** a proposed commander-centering mismatch compared a
retail Deathmatch-respawn helper with a host battle-entry helper. Their caller
boundaries differ; that observation does not authorize adding respawn filters
to startup. Entry normalization and respawn parity remain separate questions.

**Established, implementation pending:** ordinary button painting shortens the
live first caption until it fits the resolved artwork width minus six; equality
fits, and a later wider rectangle does not restore removed bytes. The shell and
battle painters currently measure and draw without this mutation. The unique
single-caption case is closed; staged captions, duplicate names and widths
below six retain explicit deciders [07 R-FE-02 §5]. A separate research
correction withdraws the claim that button-record append owns the executable's
only gadget-count cap: score-bar insertion also checks exactly 200, while label
append does not supply a universal guard.

**Falsified arithmetic suspicion:** briefing blink text intentionally rounds
its sampled clock to binary32 before comparing the deadline. The native
wrapped clock's range and the fixed periods make the suspected rounding
difference disappear. The research now specifies the sample and deadline
boundaries; no runtime change was made [07 R-FE-02 §7].


**Established overview reconciliation:** the audio summary now agrees with its
detailed music contract about initial request versus logical next, idle-query
polarity, sequential wrapping, Repeat request zero, and conditional category
reset. A fresh attempt to falsify failed-setup cleanup withdrew the blanket
claim that a time-format failure always stops and closes the device: cleanup
is conditional on the local open state. External re-entry and physical device
state remain Unknown. Speech and caption thresholds are separate stored bytes;
normal 0/5/10 examples no longer imply that authored stages cannot wrap. These
are research corrections, not additional runtime fixes [03 §8.3–§8.4].


**Established and implemented (E051):** full entry/respawn
visibility rebuilds retain the terrain-ray throttle after clearing the stored
coverage byte. An unchanged tile with emitter byte at most 5 is not republished;
height 6 or a changed tile passes, and circular publication remains direct
[03 R-VIS-01 §1], [08 R-ENTRY-01 §7]. No stock low-height impact is claimed.

**Established and landed (E052–E057):** transport
marker and offset behavior, options arithmetic and page entry, fixed-half radar
sampling, and child menu cue requests have focused regression coverage. Their
owning research is updated; the second batch has completed both integrated
gates and fresh paired visual and simulation checks.

**Established and landed (E058):** mission-radius scans use
wrapped partition bounds and only top-level spatial chains. Attached aircraft
are absent even when their airbase permits selection; the separate
`AllUnitsKilled` predicate can still count them. Finite authored radii 32768
and 32767 expose the negative-radius rejection and whole-pool scan discrepancy.
These contracts are recorded in [08 R-TRIG-01 §5]; no shipped-radius impact is
claimed. The adapter supplies top-level membership from the host overlap
index; broader cargo filing and detach-order parity remain separate research
work, not a claim of this correction.

**Established and landed (E060–E062):** sound
option storage, Repeat entry and callback cue ordering now follow the verified
callbacks. Focused production-widget tests pass. The page poll and narrow detail refresh are now implemented as E068.
Stopped transport selection, request snapshot/restore, root selection
retention and close/rebuild effects are now implemented in the third candidate
(E073–E075), with full integration and performance validation still pending.
Fresh falsification withdrew a suspected Open next-track defect: successful
Open ends with logical next zero after its internal probe. Its requested-track
seed of one is corrected separately as E073; a later probe can set
logical next to one. [03 R-AUD-01 §4] and [07 R-FE-01 §6] now distinguish
these boundaries.

**Established corrections with bounded gaps:** patrol arithmetic, traversal
and aircraft admission are implemented (E045–E047). Copied Y before the first
marker update and pathological wrapped traversal remain Unknown. Stream
identity and briefing producer corrections are implemented (E048–E049), while
shared native timer competition and device-specific outcomes remain bounded.

**Established ordinary sound lifetime:** anonymous sound identity, startup,
per-section admission and battle reload retention are now implemented (E040).
Arbitrary indirect reinitialization and device recovery remain Unknown [03
§8.3].

**Established arithmetic; authored reachability Unknown:** Classic nearest-
hostile/rally distance sums wrap at signed 32-bit width. The current wide sum
differs when both raw coordinate deltas are the minimum signed integer. This
is not a stock-impact claim; producer and pre-AI update constraints still need
closure [08 R-AI-01]. Group-centroid overflow also wraps; its existing
implementation agrees, and the old research description of a fault is removed.

**Falsified discrepancy:** the moving cloak-cost default retains the original
integer while also storing the first cost at single precision. Our existing
default matches; the hypothesized float-roundtrip defect was withdrawn.

**Established bounded profile agreement:** fresh review confirms the first-
argument-only `any` quirk, clear-on-empty `plan`, weight multiplication using a
stored single-precision factor before truncation/clamping, and separate
weight/limit locks. Both per-definition passes parse `ai_weight` with the gate
opened once per pass. Existing grammar and lock tests pass. This does not close
every manager-replay caller or pathological catalog width [08 R-AI-01 §12,
§18, §20].

**Established; implementation already agreed:** zero-speed script waits still
yield their issuing entry. Sleep resumes on a later interpreter entry after
subtracting that entry's delta; there is no extra timer decrement, and a
zero-delta wake can occur in the same simulation tick. Missing named feature
animations return absence and select the existing no-sequence behavior.

**Established developer-command availability:** `BurnAll` and `BurnOne`
reach forced feature teardown after developer authorization. This closes the
registration question and disproves the old claim that every teardown caller
honors indestructibility. Their absence in Nanolathe is explicitly deferred
by `DESIGN_DEVELOPER_TOOLS`, not an accidental regression; this audit does not
extend the developer-command implementation or multiplayer permission policy.

**Established bounded Classic AI history:** an immediately preceding regroup
broadcast can leave raw fixed-point height 3 for the first capture-capable
construction placement attempt. Other predecessor histories and failed-output
X/Z values remain unknown. This does not justify installing 3 as a general
replacement for the missing state model.

**Research-only precision clarification:** the retail remote-progress throttle
multiplies by a stored reciprocal. Nanolathe's legacy helper is called only
with zero lag, and online lockstep pacing has its own policy; no live nonzero-
lag discrepancy is asserted.

**Unknown:** complete retained SweetSpot materialization chronology, Classic
AI placement scratch outside the bounded history above, and a complete authored ballistic solver input that
exposes the stored reciprocal versus host-pi output conversion. Merely showing
an isolated floating-point difference is insufficient. The existing scoped
placeholders and the new deciders remain in their owning documents.

World/visibility, sessions/AI/save, remaining unit/order handlers, content and
runtime helpers, and interface/front-end behavior are still undergoing fresh
review. The report must not describe those whole areas as verified on the
strength of this first implementation batch.

## Corrections and remaining work

### Sound registry identity and capacity fallback

**Established; corrected.** Retail registers up to 255 sounds with identities
0–254. When full, registration returns 0, which names the first registered
sound; playback rejects the missing-sound sentinel, not zero. Nanolathe had
reserved zero and therefore silenced that fallback. The registry, ordered
export and service tests now preserve the first sound's identity and playback.

Contract: [03 §8.3]; design: DESIGN_PRESENTATION_CLIENT §2.6; implementation:
`internal/audio/registry.go`. Tests cover first registration, duplicates,
capacity, entry lookup, ordered export and playback after capacity exhaustion.

A separate **Unknown** remains for retained sound-path bookkeeping: retail
probes the original path before its short retained copy. Reproducing a shorter
copy without tracing later uses could introduce a false playback failure. The
existing retry storage remains an explicit placeholder pending that trace.

### Original's keyless composition textures

**Established; corrected (B027).** Both textured composition writers use raw
texels when the target lacks a height-key plane, even when the shaded renderer
was selected. At width 128 they overwrite their first pass with a 64-byte-stride
pass using unchanged UV coordinates. Nanolathe always used the authored width
and still applied shading. Both ordinary and diagnostic software writers now
preserve the retail result. Direct framebuffer models use a different writer
and retain normal stride; shaded flat polygons retain shading without a key
plane.

Contract: [03 R-RAST-01 §1 step 6], [03 R-REN-03A §5]; design:
DESIGN_GPU_RENDERER §5.1; implementation: `internal/client/model_spans.go` and
`model_compose.go`. Authored tests cover the source-width exception, horizontal
coordinates above 63, raw texture fallback, keyed shading, diagnostics and
direct-target provenance. An authored visual fixture confirms the expected row
mixing and identical diagnostic output. Enhanced retains its documented
texture-atlas and lighting approximation.

### Anti-alias scratch depth for a keyless final image

**Established; corrected (E003).** Following B027 through the caller revealed
that retail's doubled structure anti-alias scratch always carries a height-key
plane, even when the final resolved image does not. Nanolathe inherited the
final image's key-plane choice for its scratch. The scratch now always has its
required plane; the resolved image retains its authored choice. This preserves
scratch occlusion and selects keyed texture shading during anti-aliasing.

Contract: [03 R-REN-03A §6]; implementation: `internal/client/model_compose.go`.
The authored regression checks anti-aliasing on/off, scratch/final key-plane
independence and direct live-target provenance. This follow-through defect is
additional to the four defect entries in the 170-item external catalog.

### Signed palette indices in fade rectangles

**Established; corrected (B127).** The rectangle shader sign-extends each
destination palette byte before table lookup. High indices consequently read
the preceding row when the selected row is positive. Nanolathe used an unsigned
lookup. Original now preserves the defined in-table result for both shade and
light tables. Existing tests covered only low indices; new assertions cover
127/128, both tables and level clamping.

At selected row zero, high indices read before retail's table. That case is
**Unknown**, and the existing bounded row-zero lookup remains explicitly
marked. Contract: [03 R-COMP-02 §5]; design: DESIGN_PRESENTATION_CLIENT §5;
implementation: `internal/client/unitdraw.go`. Enhanced's RGB treatment remains
its separate presentation policy.

### Leading-separator help text

**Established; corrected (B131).** For a help value beginning with `|`, retail
writes a space and terminator over the first two bytes, then starts the
description after that terminator. Thus `|blank row text` displays
`lank row text`; Nanolathe previously kept the initial `b`. The helper and its
existing regression assertion now reflect the verified behavior. Separator-only
and missing-separator rows retain bounded host handling; native overreads are
not copied. Contract: [07 R-FE-01 §7]; code:
`cmd/nanolathe/battle_help_window.go`.

### SweetSpot bounds and retained model points

**Established correction; aim-time behavior Unknown.** The retail bounds helper
scans the selected instance's retained mutable points, initialized from model
geometry and subsequently rebuilt by the pose materializer. It does not always
scan pristine loaded vertices. Zero-seeded extrema and midpoint arithmetic
remain established. The former research assertion that transforms never affect
the helper was removed.

Nanolathe's immutable-geometry calculation remains a documented deterministic
placeholder. Completing it requires the refresh chronology at each targeting
call, including script callbacks, offscreen cases and presentation-triggered
materialization. Replacing it with a guessed current-pose transform would not
settle that question. Contract and Unknown: [06 R-WPN-04 §1], cross-reference
[04 R-COB-04 §3]; design: DESIGN_WEAPONS_PROJECTILES §7; code TODO:
`internal/combat/target.go`.

### Classic AI construction scratch state

**Established correction; scratch-dependent outcome Unknown.** The capture
placement pass copies a placement-result height into the shared task center
before checking success. That placement helper does not write the height; on
failure it also leaves horizontal coordinates untouched. A later ordinary
member can consume that retained height in a three-dimensional distance test.
The previous specification documented only the later pass's mutation and
incorrectly placed one cap test after success.

The research now records the store order and unresolved scratch provenance.
Nanolathe retains its explicit deterministic center-height and valid-placement
placeholders. A complete fix needs a reproducible definition of the consumed
scratch values, not a chosen constant. Contract: [08 R-AI-01 §3], [08 R-AI-03
§5]; design: DESIGN_SESSIONS_AI_SAVE; code TODOs: `internal/ai/manager.go`.

### Saved unit-type ordinal fallback

**Unknown.** When a saved order lacks a named unit type, the retail fallback
uses different counters for entries visited and entries eligible for comparison.
Before changing the result, the eligibility meaning and each caller's identity
domain must be established. Nanolathe retains the existing empty-name/raw-value
fallback and now marks the missing contract explicitly. Contract: [08
R-SAVE-ORDER-01]; code TODO: `internal/save/battle_image.go`.

### Additional presentation gaps made explicit

- **B079:** a non-window first GUI record changes the top-level retail hit
  rectangle. Loader acceptance, focus behavior and the corresponding service
  path need tracing. Ordinary stock-GUI parity is not disproved. [07 R-WGT-01 §1].
- **B111:** the structure-shadow punch can use a horizontal count extending
  past the destination row. Reachable projected extents and source coverage
  must establish whether ordinary structures can produce visible row spill.
  Per-pixel clipping remains the placeholder. [03 R-REN-03D §5].
- **B167:** render type 3 passes unwritten angle scratch. The existing
  recorded-angle/model-facing fallback now has an explicit code TODO and
  corrected research wording. A reproducible angle source is still missing.
  [03 §5.4], [06 R-WFX-01 §4].

## Supplemental findings

| ID | Disposition / confidence | Finding and action |
| --- | --- | --- |
| E001 | Misinterpretation / Established | The supporting notes allege a missing final PCX row. The retail loop restores the full positive row count before decoding and consumes every row. `formats/pcx.go` agrees; the existing two-row test checks the final row. No change. [02 §7], [fmt pcx]. |
| E002 | Misinterpretation / Established | The supporting notes suspect overflow in floater height. The final wrapping shift makes the expression exactly sea level minus authored draft, scaled to fixed point. Our research already explains the identity and movement follows it. No change. [04 R-MOV-01 §9]. |
| E003 | Defect — corrected / Established | Anti-alias structure scratch always has a key plane, independently of the final image. Corrected scratch allocation and added a caller-path regression. [03 R-REN-03A §6]. |

The additional E003 implementation defect is described above; unlike E001 and
E002 it was found by following the catalog lead into our caller allocation,
not by accepting an additional external bug claim.

## Upstream follow-up and adversarial recheck

On 2026-10-05, the 24 catalog disagreements originally marked Established and
the two supplemental disagreements received a second, adversarial review.
The three Supported-inference disagreements (B069, B073 and B100) were not
promoted or submitted. Upstream HEAD still matched the pinned revision, and
issue searches found no duplicate documentation correction.

The review tested alternative explanations against the independent retail
corpus, including callers, state writers, helper semantics and complete
arithmetic. These are static findings, not execution of the retail game.

| Item | Counterhypothesis tested | Bounded outcome |
|---|---|---|
| E001 | The restored PCX row count is bypassed or replaced. | Rejected: the positive full count reaches the loop and decreases after each completed row. |
| E002 | Overflow changes floating-unit height, especially for draft above sea level. | Rejected: the final wrapping scale cancels the large term. An authored arithmetic check passes all 65,536 byte-valued input pairs. |
| B021 | Host lookup follows the watched seat. | Rejected by the host-flag selection and other host-option consumers. Duplicate writes remain real. |
| B022 | Ordinary displacement prevents restoration. | Rejected for the examined motion paths: changed translations and rotations clear the rebuild gate; root and ancestor invalidation also participate. No blanket cache-correctness claim. |
| B024 | The destinations rename visible choices. | Rejected by the loader and keyboard consumers. The choices have distinct identities and actions within one dialog callback. |
| B055 | Count clearing occurs after the null-storage branch rejoins. | Rejected: that branch skips both clearing operations. |
| B058 | Counting includes classes skipped by compilation. | Rejected: both passes select populated classes. The progress overshoot remains real. |
| B062 | The duplicated destination is a battle unit. | Rejected by the metal-slider callback and starting-resource consumer. Only the record interpretation changes. |
| B077 | Equal inner branches erase selection differentiation. | Rejected: the outer branch already selects the row; the inner distinction is focus. Limited to ordinary text rows. |
| B102 | Disabling directories should disable file enumeration too. | Save-list callers rely on files with directories disabled. The tentative intent inference is unsupported; programmer intent itself is not established. |
| B105 | The animator is shared battlefield smoke code. | Rejected by main-menu installation and background surfaces. Spark interference remains real. |
| B106 | Another argument or alias supplies the alleged long name. | Rejected for the cited call: the short name is separate from dialog text. Unbounded copying remains a general hazard. |
| B110 | Scaling consumes the original position pointer. | Rejected by complete value flow: it consumes calculated horizontal speed. |
| B114 | A truncated header can retain the accepted version prefix. | Survives: the short-read hazard is real. Publish only the map-versus-campaign subsystem correction. |
| B115 | Signed comparisons make the interval empty for positive row counts. | Rejected: the visible interval is nonempty. Other advancement guards remain. |
| B117 | No eligible color necessarily means scan exhaustion. | Rejected: early upper-band exit can select another position. The final palette-permutation lookup must be retained. |
| B121 | Fatal cleanup returns to the subsequent stores. | Rejected for this termination path. Upstream already states the caveat: agreement with the caveat resolved. |
| B123 | Spacing is a timestamp, and the conditional display defect is spurious. | Spacing is font height; the dormant conditional defect is real. Static writer evidence establishes no ordinary entry path. |
| B125 | Both branches classify the same screen region. | Rejected: minimap and viewport use different conversions. Ordinary-input reachability of the unchecked off-map read remains unresolved. |
| B126 | Indexing mixes in the column, and zero occupancy identity is dereferenced. | Rejected: row-major traversal and a separate unit-definition input account for the operations. |
| B132 | Geometry writes overlap preserved caller state. | Rejected after complete temporary allocation/lifetime accounting, including both branches and early exit. |
| B133 | Fractional comparison destroys the scaled position. | Rejected in both routines and their conversion helper. No claim about malformed ranges. |
| B135 | Frame information replaces a vector, or expiry is omitted. | Both rejected by complete value flow and append tracing. |
| B136 | Device and software fills have the same Boolean return convention. | Rejected: device status is converted to Boolean. Resolve the existing question. |
| B141 | Ordinary final characters are omitted, including valid CRLF color runs. | Rejected for ordinary characters/CRLF. Bare LF can replace the preceding visible character. |
| B149 | Upstream alleges missing pictures. | Rejected: upstream already identifies the redundant test as harmless. Agreement; omit from the issue. |

For B117, an authored counterexample sets palette index 1 to black, every
other index to white, and the target to RGB (100, 100, 100). Increasing-sum
ordering puts original index 1 first and original index 0 second. Black is
below the brightness band; white is above it. The scan exits early and returns
original index 0, whereas the first ordered entry is index 1. This independently
evaluated example distinguishes the interpretations without retail bytes or
execution. The focused authored PCX tests also pass; those validate Nanolathe,
not an independent retail runtime oracle.

The review corrected this report before publication: B149 is agreement;
B121 and B136 resolve existing caveats; B117 returns a mapped palette index,
not the scan position itself, and its existing recovery tests were not evidence
for that fallback. B022 now distinguishes the rebuild gate from cache membership;
B126 uses unit occupancy terminology; B135 cites its detailed owning contract.
B114 preserves the short-read concern. The inaccurate dormant-mode parenthetical
in [07 R-HUD-03 §14.4] was corrected in place. No engine behavior changed during
this follow-up.

The resulting [upstream issue #5772](https://github.com/HectorBailey/byte-tactics/issues/5772)
contains 25 notes: 21 bounded catalog corrections/qualifications, two resolved
catalog questions, and two supplemental corrections. Three independent reviewers
checked the public wording for evidence scope and publication boundaries. The
posted body was read back and matched the reviewed draft after decoding the
read tool's HTML entities. It contains independently worded behavioral
explanations, an authored numeric counterexample, and public documentation links;
no game code, raw analysis, executable layouts or private artifacts were
published. It identifies the review as AI-assisted and distinguishes static
findings from runtime tests. These checks are not a legal-clearance opinion.

## Catalog findings

The descriptions below compare the pre-audit implementation unless the action
explicitly records a correction. “No change” applies only to the bounded finding,
not to the entire subsystem. Research references name documents and sections;
the private evidence index maps each audit ID to the independently inspected
retail operations. No executable locations are included here.

| Disposition | Entries |
| --- | ---: |
| Defect — corrected | 4 |
| Agreement | 35 |
| Misinterpretation | 27 |
| Host boundary | 59 |
| Out of scope | 33 |
| Unknown | 12 |

**Total: 170.** The two supplemental misinterpretations and E003 are outside these counts.

### B001 — Bitstream growth underallocates native storage

**Out of scope · Established.** The retail bit writer allocates one word on growth and copies the previous word capacity into it. This is native memory corruption, not a defined bitstream result. Nanolathe does not implement retail synchronization transport.

Evidence and comparison: Independent writer export shows a four-byte allocation followed by the capacity-sized copy. Repository transport policy expressly replaces retail synchronization.

Research: 04 packet synchronization discussion; DESIGN_MULTIPLAYER §1 and §5. Code: internal/session; docs/DESIGN_MULTIPLAYER.md.

Action: none: do not reproduce memory corruption; any future compatible codec must define safe storage.

### B002 — Untargeted network send has a conditional null read

**Out of scope · Supported inference.** An untargeted retail send contains a conditional null-target read. Whether its enclosing mode and direct-send mode can coexist has not been independently settled. Replacement lockstep does not use this native helper.

Evidence and comparison: Independent send export confirms the two different mode tests and null read; transport exclusion checked. The external play-test claim is not evidence.

Research: 08 unit synchronization; DESIGN_MULTIPLAYER §1. Code: internal/session; docs/DESIGN_MULTIPLAYER.md.

Action: none for current transport; settle constructor mode correlation before asserting the read is reachable.

### B003 — Raw-file association lookup keeps its painter route inactive

**Agreement · Established.** The raw-file association search returns no image on both found and exhausted paths. Its special associated-image tiling branch is therefore inactive; ordinary BackTile and Listbox fallback painting remains separate. This matches the documented inert raw-file gadget.

Evidence and comparison: Independent constant-zero lookup and sole recovered painter caller agree with the documented raw-file kind and its lack of runtime consumption. Typed GUI code retains its authored file path without enabling the inactive image route.

Research: 07 R-WGT-01 §8 and §12. Code: internal/gui/types.go: KindRawFile; internal/gui/load.go: raw-file build arm.

Action: No change.

### B004 — Surface-to-sprite conversion installs row pitch as width

**Unknown · Unknown.** The wrapper ultimately takes sprite width from the surface row pitch, replacing an earlier logical-width assignment. Generated images, decoded PCX images and loaded saved radar images have equal width and pitch, so the overwrite is harmless there. Whether the remaining live-window surface call can have unequal values remains unresolved.

Evidence and comparison: Independent conversion and source constructors identify width, height, row pitch and pixels. Five recovered direct calls use constructors that set pitch equal to width; the remaining live-window surface producer has not been closed. Nanolathe uses tightly packed decoded images rather than this native wrapper.

Research: 03 §2 framebuffer and §4 GAF sprite blitting; 07 save-preview and window painting. Code: internal/client/minimap_draw.go; cmd/nanolathe/retail_menu.go: panelBackground; cmd/nanolathe/battle_hud_modal.go.

Action: The owning renderer Unknown now records the remaining live-window producer and downstream clipping/read-width trace. No equivalent native wrapper exists in Nanolathe, so no unrelated code TODO was added.

### B005 — AI plan wildcard only works in the first argument

**Agreement · Established.** Only the first difficulty argument is tested for the wildcard; later wildcard arguments are ignored while named difficulty arguments retain their ordinary effect.

Evidence and comparison: Independent directive routine uses the first argument for the wildcard and the loop argument for named difficulties; the compiler implements that distinction.

Research: 08 R-AI-01 §12. Code: internal/content/ai_profile.go: aiPlanTableNames.

Action: No change.

### B006 — Construction assist uses the asymmetric footprint expression

**Agreement · Established.** The target footprint contribution is trunc(16*sqrt(X*X+2*Z))/2. The second footprint dimension is added twice, not squared. Existing assist code preserves this arithmetic and its truncation order.

Evidence and comparison: Independent arithmetic trace verifies the two additions before square root; implementation and focused assist-radius test agree.

Research: 04 R-ORD-01 §5 and §12; 05 R-WORK-01 §2. Code: internal/orders/work.go: assistApproachHalf.

Action: No change.

### B007 — Explore fallback may lack an enemy target

**Host boundary · Established.** Retail passes an absent enemy result into its exploration fallback without a null check. Nanolathe explicitly makes that case a no-op. The research documents the safe boundary and the bounded argument that normal surviving-unit stock play avoids it.

Evidence and comparison: Independent exploration export confirms the absent-target path; research and implementation explicitly preserve the safe no-op rather than fabricate a destination.

Research: 08 R-AI-01 §6. Code: internal/ai/manager.go: doExplore.

Action: none: retain the documented safe boundary; a different reachability claim requires tracing strategic-center failure with surviving own units.

### B008 — Wave merge subtracts the replacement member position

**Agreement · Established.** After moving the chosen member, the group replaces its slot with the last member and subtracts the replacement position from running coordinate sums. Nanolathe preserves the resulting center drift.

Evidence and comparison: Independent merge and vector-removal exports establish mutation before reread; code captures the replacement position and the focused wave-merge test passes.

Research: 08 R-P0-04 §3 Wave merge. Code: internal/ai/manager.go: mergeWaveGroupRecords.

Action: No change.

### B009 — Construction placement can carry an unwritten height into later work

**Unknown · Unknown.** A capture-capable builder copies placement output height into the shared center before checking placement success. The placement helper never writes that height, so subsequent ordinary reposition distances can depend on unresolved residue. Nanolathe retains the original center height as an explicit deterministic placeholder; no numeric replacement is justified.

Evidence and comparison: Independent caller and complete output-writer trace confirms the copy on both successful and failed placement, its ordering before the success test, and absence of a height producer. Both construction passes and their shared-center lifetime were traced.

Research: 08 R-AI-01 §3; 08 R-AI-03 §5 and Unknown list. Code: internal/ai/manager.go: doConstruction, constructionPlacePass.

Action: gap: identify the incoming output-height value on every caller path, or document the irreducible native residue; preserve deterministic placeholder until settled.

### B010 — Sound alias exhaustion returns the first alias index

**Defect — corrected · Established.** Retail registers aliases from zero through 254 and returns zero when full, selecting the first real alias. The pre-audit baseline reserved zero as null and started registration at one, making full-table playback silent. The correction restores zero-based identity and first-alias playback.

Evidence and comparison: Independent registration trace establishes zero-based IDs and the overflow return. Diff against main shows the old null-zero and one-based loops; current corrected registry plus ServiceAliasOverflowPlaysFirstRegisteredSample test pass.

Research: 03 §8.3. Code: internal/audio/registry.go: register, Entry, Load; internal/audio/service.go: Load.

Action: Corrected registry identity, full-table fallback and ordered export; regression and playback tests pass.

### B011 — Piece bounds include the origin on every axis

**Agreement · Established.** All minima and maxima begin at zero. Positive-only coordinates pull minima to the origin; negative-only coordinates also pull maxima to the origin. Nanolathe already uses both halves of this rule.

Evidence and comparison: Independent bounds routine initializes all six values to zero; current implementation and SweetSpot contract tests match. The catalog description omits the negative-only consequence.

Research: 06 R-WPN-04 §1. Code: internal/combat/target.go: pieceVertexBoxCentre.

Action: No change.

### B012 — Empty ordinary texture-bank lookup can reuse an unrelated pointer

**Host boundary · Established.** With no ordinary texture banks, retail can carry the input model pointer into texture-entry handling. That is an invalid native object interpretation. Nanolathe resolves textures through typed registries and reports or omits missing bindings safely.

Evidence and comparison: Independent texture-binding export confirms lookup initialization and zero-bank branch; typed model-texture registry has no model-as-texture fallback.

Research: 03 §2.4.1 and R-CRD-005 §1; DESIGN_GPU_RENDERER asset boundary. Code: internal/client/model_textures.go: ModelTextureRegistry.

Action: none: retain safe missing-texture handling; do not reproduce arbitrary pointer reads.

### B013 — Single ordinary texture file makes retail progress divide by zero

**Host boundary · Established.** The texture-load progress denominator subtracts one from the file count. A sole ordinary bank reaches division by zero; a sole logo bank bypasses that progress branch. This is a loader safety boundary, not a gameplay formula to reproduce.

Evidence and comparison: Independent loader export distinguishes logo handling from ordinary progress arithmetic. Nanolathe loads typed texture collections without this native progress divisor.

Research: 07 load-progress stages; 03 model texture resolution. Code: internal/client/model_textures.go; internal/ui/frontend.go.

Action: none: keep zero- and one-file resource sets safe.

### B014 — Build-option append admits a thirty-first item

**Host boundary · Established.** Retail allows a thirty-first build option into storage allocated for thirty. Nanolathe retains the logical thirty-first append using safe slice storage, without reproducing the overflow.

Evidence and comparison: Independent append guard and allocator establish the mismatch; current compiler explicitly allows the thirty-first logical item.

Research: 02 R-CAT-01 §8. Code: internal/content/compile_download_menu.go.

Action: No change.

### B015 — Flight lean uses the same rotated component twice

**Agreement · Established.** Both lean channels derive their horizontal input from the rotated X component. The apparent axis mix-up is already an explicit authoritative arithmetic contract.

Evidence and comparison: Independent lean routine confirms both reads; current implementation passes the same component to the two angle calculations.

Research: 04 R-AIR-01 §2. Code: internal/movement/flight.go: ApplyLean.

Action: No change.

### B016 — Rejected-player dialog formats into a fixed native buffer

**Host boundary · Established.** A long player name can overflow retail dialog formatting storage. Nanolathe uses managed strings and its own lobby protocol, so no corresponding native overwrite is required.

Evidence and comparison: Independent rejection-dialog export uses unrestricted formatting into a fixed local array; current UI state uses Go strings. Reachability depends on accepted player-name length.

Research: 07 modal dialogs; DESIGN_MULTIPLAYER lobby and identity validation. Code: internal/ui/frontend.go; docs/DESIGN_MULTIPLAYER.md.

Action: none: retain bounded protocol admission and safe formatting.

### B017 — Raw save record can be destructed as live pointer state

**Host boundary · Established.** Retail reads bytes over a temporary object and later runs its destructor, allowing file-provided native pointer state to reach cleanup. Nanolathe parses save records as data and never executes embedded pointers.

Evidence and comparison: Independent loader export establishes construct-read-destruct order. Typed retail-save records are handled without native object lifetime dispatch.

Research: 08 save loading and R-SAVE-ORDER-01; format save contracts. Code: internal/orders/retail_save.go.

Action: none: preserve data validation and pointer-free loading.

### B018 — Saved order type fallback uses mismatched counters

**Unknown · Unknown.** The fallback compares and advances different counters. The exact order-family identity and flag meaning needed to map the branch safely remain unresolved, so the existing save-order gap is still open.

Evidence and comparison: Independent save export confirms separate comparison and increment counters. Research already records the missing flag/caller evidence; current raw order handling does not assert a settled fallback.

Research: 08 R-SAVE-ORDER-01 Unknown list. Code: internal/save/battle_image.go: findUnitTypeName; internal/orders/retail_save.go.

Action: gap: identify the record family, flag writer and caller population before selecting fallback semantics.

### B019 — Paired sort helpers use compensating stack cleanup sizes

**Host boundary · Supported inference.** The native helpers use unusual individual argument cleanup sizes; recovered callers invoke them together. Go sorting has no equivalent stack-ABI artifact. No ordering defect follows from the cleanup observation alone.

Evidence and comparison: Independent assembly confirms differing return cleanup amounts and adjacent paired calls. Complete exceptional-call stack accounting was not established.

Research: 03 model painter ordering and R-RAST-01. Code: internal/client/model_compose.go; internal/render.

Action: none for Go behavior; a retail crash claim needs all-call-site argument accounting.

### B020 — AI report text is used as a formatting template

**Host boundary · Established.** The retail diagnostic writer treats computed text as a format string. Nanolathe diagnostic capture serializes values safely; reproducing native formatter interpretation is unnecessary.

Evidence and comparison: Independent diagnostic export passes the completed buffer as the formatting template. Current AI capture uses typed diagnostic serialization outside tick behavior.

Research: 08 AI diagnostic boundaries; DESIGN_SESSIONS_AI_SAVE diagnostics. Code: internal/ai/debug_capture.go.

Action: No change.

### B021 — Lobby unit limit follows the host seat

**Misinterpretation · Established.** The selected seat is found by the host flag, not by the viewed player. Copying its unit limit into the local record synchronizes the host setting; the duplicate local store has no extra effect.

Evidence and comparison: Independent host lookup searches occupied seats for the host flag; the limit updater reads that seat or local slider and writes the local setting. Design explicitly makes unit limit agreed host configuration.

Research: 08 R-ENTRY-01 §2; DESIGN_MULTIPLAYER host options. Code: internal/session/skirmish.go; docs/DESIGN_MULTIPLAYER.md.

Action: No change.

### B022 — Piece reset flag denotes invalidation rather than displacement

**Misinterpretation · Established.** Changed piece translations or rotations clear the rebuild gate before vertex restoration, and the transform pass subsequently rebuilds the vertices. Root orientation changes also invalidate this gate. The assertion that displaced vertices can never be restored does not follow from the isolated zero comparison.

Evidence and comparison: Independent motion setters and the refresh caller clear the rebuild gate; reset copies base vertices on zero or ancestor invalidation, and rebuilding uses that admission condition. This gate is distinct from cache membership. Nanolathe rebuilds posed geometry from immutable model vertices.

Research: 03 render-transform refresh cache; 03 model transforms. Code: internal/model; internal/client/model_compose.go.

Action: none: do not invert the comparison; complete flag lifecycle would be needed for a broader cache-parity claim.

### B023 — Large flat polygons can exceed retail local vertex storage

**Host boundary · Established.** Retail collects flat-polygon vertices into fixed local storage without the matching count bound. Nanolathe parses and uses bounded dynamic geometry, avoiding a native stack overwrite on larger authored polygons.

Evidence and comparison: Independent polygon export shows the fixed temporary arrays and count-driven copy. The parser and raster use validated indices and slices.

Research: 03 R-RAST-01 polygon raster; fmt 3do. Code: formats/three_do.go; internal/client/model_raster.go.

Action: none: retain parser limits and safe dynamic storage.

### B024 — Exit confirmation assigns both keyboard defaults to cancellation

**Misinterpretation · Established.** The duplicated choice name configures the Enter and Escape default bindings, not the two visible choice identities. Both default to cancellation for surrender confirmation, as research and UI behavior specify.

Evidence and comparison: Independent dialog setup passes the same cancellation name to the two default bindings; research describes both keys and initial focus as No. UI modal tests retain cancellation behavior.

Research: 07 R-FE-01 §7. Code: internal/ui/battle.go; internal/ui/battle_test.go.

Action: No change.

### B025 — Cloak HUD becomes mixed after a second capable unit

**Agreement · Established.** After the first cloak-capable selected unit sets the state, another capable unit sets the mixed state without checking agreement. The HUD fold already retains this retail quirk.

Evidence and comparison: Independent instruction trace confirms unconditional second-capable transition; the implementation uses the same fold.

Research: 07 R-HUD-03 §13. Code: internal/hud/localcompose.go.

Action: No change.

### B026 — Feature reproduction uses map height to derive one coordinate

**Agreement · Established.** The reproduction cell index is divided by map height for its row-like coordinate while the other coordinate uses width. This produces asymmetric behavior on rectangular maps, which Nanolathe already preserves.

Evidence and comparison: Independent reproduction export confirms the divisor; implementation and rectangular-map contract test agree.

Research: 05 R-FEAT-01 §12. Code: internal/features/reproduce.go.

Action: No change.

### B027 — Keyless composition textures use raw half-stride sampling at width 128

**Defect — corrected · Established.** Both textured composition writers bypass
SHD when their image has no key plane; at width 128, the second pass overwrites
the first using stride 64 with unchanged UVs. Direct framebuffer writers retain
normal stride. Nanolathe previously conflated these paths and always used the
authored width plus the selected shade table.

Evidence and comparison: Independent composition allocation, shaded/unshaded
mapper, direct framebuffer mapper and kernel traces establish the distinction.
A completed ZBuffer=0 structure can select shaded keyless composition when
anti-aliasing is off. Anti-alias scratch is always keyed; flat shaded spans
retain SHD without a key plane.

Research: 03 R-RAST-01 §1 step 6; 03 R-REN-03A §5–§6. Code:
internal/client/model_spans.go; internal/client/model_compose.go;
internal/client/model_raster.go.

Action: Corrected both software paths with explicit direct-target provenance;
regressions cover raw/keyed shading, width, upper horizontal coordinates,
diagnostics, direct callers and anti-alias scratch. Enhanced retains its
existing geometry policy. E003 records the additional scratch-allocation fix.

### B028 — Large focus tables read incompletely initialized scratch

**Host boundary · Established.** Retail initializes only fifty coordinate entries before a gadget-count-driven traversal. Nanolathe uses fully initialized per-window storage and bounds its earlier-entry scan. The exact retail outcome for larger windows remains residue-dependent.

Evidence and comparison: Independent assembly confirms the bounded initialization; implementation allocates by gadget count and checks earlier indices explicitly. Owning research already calls out the larger-window uncertainty.

Research: 07 R-WGT-01 §2 and Unknown list. Code: internal/ui/focus.go: focusCanonicalX.

Action: none for safe host behavior; an exact retail large-window contract needs scratch-lifetime evidence.

### B029 — Missing-disc warning does not stop the skirmish action

**Out of scope · Established.** The original skirmish action displays a missing-disc warning and continues. Nanolathe uses mounted asset availability and has no retail CD licensing gate to reproduce.

Evidence and comparison: Independent skirmish branch falls through after the warning. Repository content startup uses VFS roots, not a CD license probe.

Research: 07 front-end startup and skirmish entry; 02 VFS. Code: internal/ui/frontend.go; vfs/layout.go.

Action: No change.

### B030 — Standalone placement ignores the second special height class

**Agreement · Established.** The standalone footprint-height result includes the first special class maximum and ordinary contribution but does not use the computed second special-class maximum. Nanolathe already ignores that second maximum.

Evidence and comparison: Independent footprint routine retains an unused second accumulator; the implementation uses only the first special class in its final choice.

Research: 05 footprint-height producers and placement. Code: internal/world/placement.go: SiteHeight.

Action: No change.

### B031 — Alliance notification sound follows a local transition test

**Out of scope · Established.** The original lobby sound decision tests a local alliance-state transition rather than full bilateral agreement. Nanolathe intentionally replaces retail lobby transport and uses its documented directed-alliance policy; this observation alone is not a simulation discrepancy.

Evidence and comparison: Independent lobby instructions establish the local test. Current multiplayer contract defines one shared directed matrix rather than this native notification route.

Research: 08 alliance synchronization; DESIGN_MULTIPLAYER §6.6–§6.7. Code: docs/DESIGN_MULTIPLAYER.md; internal/session.

Action: none; future lobby sound parity work should cite this narrow local transition, not infer mutual alliance.

### B032 — Missing associated scrollbar can alias the window record

**Host boundary · Established.** The retail association lookup uses zero for absence while a consumer compares against a different sentinel, potentially writing through the window record. Nanolathe keeps gadget associations typed and checks whether a scrollbar exists.

Evidence and comparison: Independent lookup/consumer instructions confirm sentinel mismatch. Typed UI association traversal has no window-header fallback write.

Research: 07 R-WGT-01 list and scrollbar association. Code: internal/ui/panel.go; internal/ui.

Action: none: retain safe absent-association behavior; stock reachability would need an authored GUI census.

### B033 — Selected picture-list row may dereference an absent image

**Unknown · Unknown.** The selected picture-row painter can skip a missing-image branch and later read image dimensions. Nanolathe does not yet model the full record-list payload, so this remains part of an existing UI coverage gap rather than a proven matching path.

Evidence and comparison: Independent painter control flow confirms missing-image hazard on the selected branch. Panel code explicitly leaves record-list payload and variable-height geometry unresolved.

Research: 07 R-WGT-01 §4 and §5; Unknown record-list structures. Code: internal/ui/panel.go.

Action: gap: identify record producers, selected-row flags and absent-image admission; implement only the established safe data contract.

### B034 — Record-list hit testing enters an unconditional flag branch

**Unknown · Unknown.** The native list flag expression is always true, so the corresponding record-height path does not have the apparent attribute guard. The exact payload variants and their Nanolathe mapping remain unresolved.

Evidence and comparison: Independent instruction sequence combines a tested mask with a nonzero constant before the condition. Current UI retains an explicit variable-row/record-payload gap.

Research: 07 R-WGT-01 §4 and §5; record-list Unknown. Code: internal/ui/panel.go.

Action: gap: trace both record payload producers and the subsequent height walk before changing list behavior.

### B035 — Reclaim resolver assumes a supplied ground position

**Host boundary · Established.** Retail uses the reclaim position in a feature lookup before its later null test. Nanolathe checks optional inputs before resolving the ground-feature action, preserving safe behavior for malformed calls.

Evidence and comparison: Independent resolver export shows the dereference ordering. Typed resolver guards position availability in its feature route; all retail caller preconditions were not exhaustively proved.

Research: 04 R-ORD-01 resolver; 04 §3.4. Code: internal/orders/resolve.go: resolveName case 12.

Action: none: keep safe missing-position rejection; a reachability claim needs every code-twelve caller.

### B036 — Build-caption accelerator spacing may use a different caption variant

**Unknown · Unknown.** The build-attribute caption branch draws the selected prefix but measures the first stored caption for the colored key position. It does not underline that key. Ordinary staged-button construction clears the key and rewrites alignment, excluding this combination. Whether a bypass or later mutation admits a keyed build-style later caption remains unresolved. Current painters measure the selected prefix.

Evidence and comparison: Independent caption routine traces selected-string advancement, prefix truncation and first-string width measurement. The owning builder contract removes build alignment for ordinary staged captions. Both current caption painters use selected-prefix width, while retailGadgetText selects a staged label.

Research: 07 R-WGT-01 §3, painter frame choice, quickkey assignment and preclear lifetime; 03 R-FONT-01 §6. Code: cmd/nanolathe/button_caption.go: drawRetailButtonCaption; cmd/nanolathe/battle_hud_modal.go: modal caption painter; cmd/nanolathe/retail_menu_draw.go: retailGadgetText.

Action: The owning button research and both caption painters now mark the exceptional-admission gap. Retain selected-prefix spacing until all builder/runtime writers establish whether that combination is reachable.

### B037 — Order-button fallback can reuse an argument as a record pointer

**Host boundary · Supported inference.** The native fallback can retain an incoming argument where a button record is expected. Nanolathe dispatches typed gadget actions, so it has no equivalent arbitrary-pointer interpretation. Legitimate reachability of the malformed route is not established.

Evidence and comparison: Independent fallback instruction path retains the argument value; current UI identifies commands through typed actions and gadget indices.

Research: 07 R-HUD-03 command buttons and R-WGT-01. Code: internal/hud; internal/ui/panel.go.

Action: none: preserve typed dispatch; settle missing-gadget caller invariants before claiming a retail failure.

### B038 — Overlong directive tokens fail to advance the native parser

**Host boundary · Established.** At its storage limit, the retail tokenizer can stop copying without advancing the input and repeatedly append terminators. Nanolathe uses managed strings and an explicit token-count limit, so long lines do not reproduce native memory corruption.

Evidence and comparison: Independent tokenizer instruction trace confirms stationary input at the capacity branch. Current profile tokenizer safely splits strings and keeps at most twenty tokens.

Research: 08 R-AI-01 §20. Code: internal/content/ai_profile.go: aiLineTokens.

Action: none: retain safe token storage; do not add an infinite loop or native overwrite.

### B039 — Some legacy wrappers adjust only the low stack word

**Host boundary · Established.** Several native wrappers use a narrow stack adjustment that would mishandle a boundary crossing. This is an implementation-ABI hazard, not an engine algorithm, and has no counterpart in Go calls.

Evidence and comparison: Independent instructions use narrow rather than full stack adjustment. The claim that actual call depth never crosses the boundary is not independently proved and is not needed for Nanolathe.

Research: 01 runtime/platform boundary. Code: Go host call stack; no corresponding engine behavior.

Action: No change.

### B040 — Fatal-log writer uses the wrong file-handle failure check

**Host boundary · Established.** The native fatal logger tests an opened file handle against zero rather than the API failure sentinel. Nanolathe uses host file errors, so it should not attempt follow-up writes using a failed handle.

Evidence and comparison: Independent fatal-handler assembly confirms the zero test. This is outside deterministic simulation and does not define useful output on failure.

Research: 01 platform errors and diagnostics. Code: internal/client/art_diagnostics.go; Go file error handling.

Action: No change.

### B041 — Unreferenced legacy line wrapper has an incomplete call contract

**Out of scope · Supported inference.** The recovered legacy line wrapper supplies an incomplete clipping call. No direct caller of its enclosing wrapper was found in the bounded assembly search; no active rendering contract can be inferred from it.

Evidence and comparison: Independent wrapper instructions confirm the missing surface input. Bounded direct-call search found only the internal wrapper link, not a live engine caller.

Research: 03 primitive drawing; 01 host boundary. Code: internal/client line drawing; no active equivalent identified.

Action: none; an active-behavior claim requires an indirect-reference and caller trace.

### B042 — Briefing replacement clears ownership before loading

**Agreement · Established.** Although the low-level loader would retain freed storage on a zero-size replacement, its recovered owning caller clears the buffer before invoking it. That call path avoids the proposed dangling-pointer result; Nanolathe uses owned briefing text.

Evidence and comparison: Independent loader and owning caller show prior release and explicit clear before load. The conclusion is bounded to recovered call sites, not an assertion about unknown callers.

Research: 08 campaign briefing; 07 front-end briefing. Code: internal/ui/frontend.go.

Action: No change.

### B043 — No-recipient send returns an unused residual value

**Out of scope · Established.** When no send occurs, a retail transport helper returns an old argument value. The recovered caller ignores its return, so this produces no observed decision on that path; replacement transport does not use the helper.

Evidence and comparison: Independent helper shows result storage assigned only during sends; recovered caller does not branch on the return.

Research: 08 network transport boundary; DESIGN_MULTIPLAYER §1. Code: docs/DESIGN_MULTIPLAYER.md.

Action: No change.

### B044 — Unreferenced stepping helper accumulates an unused vertical value

**Out of scope · Supported inference.** A recovered stepping helper divides horizontal differences but repeatedly adds the full vertical difference. The vertical accumulator is not consumed in the recovered loop, and no direct caller was found. This does not establish a movement defect.

Evidence and comparison: Independent helper export confirms the unused vertical accumulation; bounded assembly call search found no direct call.

Research: 04 movement; bounded unused-code boundary. Code: internal/movement; no active equivalent identified.

Action: none; active relevance requires an indirect-reference or caller trace.

### B045 — Resurrection computes an unused absent-feature pointer

**Agreement · Established.** Late resurrection states can compute the absent-feature address but do not dereference it. Nanolathe likewise skips feature-definition access in the later phases; there is no reason to manufacture an invalid pointer.

Evidence and comparison: Independent state-handler export confines feature reads to earlier states; current handler obtains the definition only for those phases.

Research: 04 R-ORD-01 resurrection; 05 R-WORK-01 §7. Code: internal/orders/work.go: resurrection phases.

Action: No change.

### B046 — Resurrection failures retain two distinct spellings

**Agreement · Established.** The two failure paths emit different spellings, including the historical misspelling. The research and implementation retain the exact diagnostic text.

Evidence and comparison: Independent handler strings and failure branches match the two implemented messages.

Research: 04 R-ORD-01 §5; 05 R-WORK-01 §7. Code: internal/orders/work.go.

Action: No change.

### B047 — AI rally creates and destroys an unused temporary container

**Agreement · Established.** The temporary container has no elements or observable engine effect. Nanolathe implements the rally decisions without reproducing the dead allocation artifact.

Evidence and comparison: Independent rally export constructs an empty local and destroys it without population; current task has no corresponding gameplay state.

Research: 08 R-AI-01 rally task. Code: internal/ai/manager.go: doRally.

Action: No change.

### B048 — Search-grid constructor frees a pointer immediately after clearing it

**Host boundary · Established.** The native constructor clears a pointer and then frees the cleared value before allocation. This has no engine-state effect and requires no managed-memory counterpart.

Evidence and comparison: Independent constructor export establishes clear-before-free; current workspace is managed storage.

Research: 04 R-PATH-01 workspace construction. Code: internal/path/workspace.go.

Action: No change.

### B049 — Zero-cell search grid underflows native initialization length

**Host boundary · Established.** A zero-cell native grid makes a size-minus-one initialization underflow. Nanolathe rejects zero terrain dimensions at parsing and does not reproduce an enormous memory write.

Evidence and comparison: Independent allocator export confirms the subtract-before-fill expression; TNT parsing explicitly rejects zero width or height.

Research: 04 R-PATH-01 workspace; fmt tnt limits. Code: formats/tnt.go; internal/path/workspace.go.

Action: No change.

### B050 — Touched-cell clearing remains inside rounded allocation

**Agreement · Established.** The last-block clear repeats its initial bound check, but storage is rounded to complete eight-cell groups. Within the traced allocation and marking contract the clear does not cross allocated storage; Nanolathe clears logical search state safely.

Evidence and comparison: Independent clear and constructor exports establish eight-cell rounding. Current workspace generation/reset mechanism preserves the logical cleared state without copying native padding.

Research: 04 R-PATH-01 workspace reset. Code: internal/path/workspace.go.

Action: No change.

### B051 — Repeated capability test does not add a second condition

**Agreement · Established.** The same unit capability is tested twice without an intervening change. Removing the redundant branch preserves the admitted command set; Nanolathe does not need to reproduce the duplicated instruction.

Evidence and comparison: Independent instruction trace proves both tests use the unchanged capability. Typed command admission evaluates the corresponding capability once.

Research: 07 command admission; 04 order capabilities. Code: internal/hud; internal/orders/resolve.go.

Action: No change.

### B052 — Initial path-node terrain cost is not read before closing

**Agreement · Established.** The initial node leaves a terrain-cost field unwritten, but the first expansion closes it before neighbor relaxation can use that field. Nanolathe may initialize its storage safely without changing the traced search arithmetic.

Evidence and comparison: Independent search-start and expansion flow confirm the start-node lifetime; owning research explicitly identifies the unread field.

Research: 04 R-PATH-01 §4 step 11. Code: internal/path/search.go; internal/path/workspace.go.

Action: No change.

### B053 — Path scheduler collapses identical upper pressure tiers

**Agreement · Established.** The two highest written pressure cases have the same multiplier, and the redundant removal call is unreachable. Nanolathe uses the effective three multipliers six, three and one.

Evidence and comparison: Independent scheduler export shows identical final branches; current switch implements below-one, below-two and otherwise tiers.

Research: 04 R-PATH-01 §6; 04 §7.2 and §7.3. Code: internal/path/queue.go: Scheduler.replenish.

Action: No change.

### B054 — CD path builder emits a repeated separator

**Out of scope · Supported inference.** The original CD path builder uses a repeated directory separator. Nanolathe resolves logical asset paths through VFS and has no need to retain this native drive-path spelling.

Evidence and comparison: Independent path-builder export confirms use of a fixed formatting string; VFS path normalization and disc-independent resource mounting provide the host boundary. Exact displayed drive-path byte interpretation was not re-decoded from the executable data section.

Research: 02 VFS; 01 platform startup. Code: vfs/path.go; vfs/layout.go.

Action: none; byte-exact CD path claims require decoding the embedded format string.

### B055 — Build-option teardown leaves the count untouched when no storage exists

**Misinterpretation · Established.** The null-storage branch skips both pointer clearing and count clearing. The claim that the companion count is reset even when storage is already null contradicts the actual branch. Nanolathe uses immutable compiled catalogs and managed lifetime.

Evidence and comparison: Independent export and assembly show the null branch jumping over both stores. No teardown compatibility change is warranted.

Research: 02 R-CAT-01 build-option lifetime. Code: internal/content/compile_download_menu.go.

Action: No change.

### B056 — Tokenizer minus-sign test is redundant after punctuation dispatch

**Out of scope · Supported inference.** The recovered legacy tokenizer handles punctuation before a later minus-sign condition, making the later condition redundant under its character-class contract. This is not the active line-directive parser used by current AI profiles.

Evidence and comparison: Independent tokenizer control flow confirms punctuation precedence. The recovered parser entry set has no established active call chain, so its legacy grammar is not substituted for current profile syntax.

Research: 08 AI profile parsing and unreachable tokenizer boundary. Code: internal/content/ai_profile.go.

Action: none; establish a live caller and runtime character classification before treating this as an active grammar requirement.

### B057 — Legacy parser formats an error string without emitting it

**Out of scope · Established.** The recovered parser error paths build a local message then set an error flag without passing the message to a display or log. No live caller of this old parser family was established, and current profile directives use a separate tokenizer.

Evidence and comparison: Independent error-helper exports confirm the local formatting result is unused; bounded recovered-caller search found no external calls to these helpers.

Research: 08 AI profile parsing and unreachable tokenizer boundary. Code: internal/content/ai_profile.go.

Action: none; do not add an inferred retail diagnostic channel.

### B058 — Passability compilation progress overshoots its final value

**Misinterpretation · Established.** Only populated movement classes enter the divisor. For n populated classes, the kth completed class stores 100*(k+1)/n before the final 100. The alleged inclusion of skipped classes is false; the overshoot is real and duplicates B089.

Evidence and comparison: Own static count pass tests the same nonzero class condition as the build pass; current loading attribution explicitly uses catalog-family progress.

Research: 07 "The loading screen"; 04 §6.1. Code: cmd/nanolathe/loading.go.

Action: No change — retain the documented host loading attribution; do not change the class count to include empty slots.

### B059 — Unused display-mode allocation

**Host boundary · Established.** The legacy display-mode dialog allocates an auxiliary string buffer that its visible destructor path does not release. This resource leak is not an output contract for the native display backend.

Evidence and comparison: Own constructor and teardown export inspected; current monitor-mode enumeration is host-native and does not allocate the legacy object.

Research: 07 R-FE-01 §6. Code: cmd/nanolathe/retail_menu_options.go.

Action: No change — keep ordinary ownership and cleanup.

### B060 — Redundant lobby player-kind test

**Out of scope · Established.** The lobby update already admits only the three playing controller kinds before excluding a fourth kind. Removing that second test does not expand admission. The old transport lobby is replaced by Nanolathe's own multiplayer contract.

Evidence and comparison: Own lobby update branch inspected; session player representation and the replacement multiplayer policy searched.

Research: 08 "Synchronization and integrity checks"; docs/DESIGN_MULTIPLAYER.md. Code: internal/session/player_record.go.

Action: No change — no requirement to reproduce a redundant test or legacy lobby transport.

### B061 — Repeated recursive piece-walk flag assignment

**Agreement · Established.** The branch continuing recursive piece traversal assigns the traversal flag its already-required nonzero value. The extra assignment changes neither the visited pieces nor the resulting transform.

Evidence and comparison: Own transform control flow examined together with the current recursive piece composition; the target-point lifecycle gap is separate.

Research: 03 §2.4. Code: internal/model/model.go:ComposeInto.

Action: none.

### B062 — Repeated lobby starting-metal assignment

**Misinterpretation · Established.** The duplicate store belongs to the local lobby player's starting metal, not a battle unit. Both stores use the same slider-derived value with no intervening mutation.

Evidence and comparison: Own slider callback and energy sibling inspected; current skirmish setup has explicit initial resource fields.

Research: 08 R-SKIR-01 §3. Code: cmd/nanolathe/retail_menu.go.

Action: No change — retain one semantic assignment.

### B063 — Unused repeated lobby player lookup

**Out of scope · Established.** A player lookup is repeated and its result discarded while checking lobby slot availability. It contributes no new result or world-state mutation.

Evidence and comparison: Own whole helper inspected; the repeated scan has no calls or writes. The old network lobby is not Nanolathe's relay protocol.

Research: 08 "Synchronization and integrity checks"; docs/DESIGN_MULTIPLAYER.md. Code: internal/session/player_record.go.

Action: none.

### B064 — Effect strips retain 401 records

**Agreement · Established.** Eviction occurs only when the pre-insertion count exceeds 400, then the new record is appended. The steady maximum is therefore 401, which Nanolathe preserves.

Evidence and comparison: Own insertion assembly verifies strict greater-than and eviction-before-append; the existing boundary test passed in this audit.

Research: 03 "Strip storage and lifecycle"; 03 R-STRIP-01 §1. Code: internal/session/strips.go; internal/session/strips_test.go:TestStripAppendEvictsOldestAbove400.

Action: No change — do not change the cap to 400.

### B065 — Unused campaign scan and repeated dialog invalidation

**Agreement · Established.** The scan for an unplayed mission does not select it; current progression remains the selection source. The duplicated load/save helper merely writes the same invalidation sentinel twice.

Evidence and comparison: Own campaign list filler, load/save branch, and the called invalidation helper inspected. Existing progression code derives selection from campaign progression, not an unused scan.

Research: 08 R-CAMP-01 §8; 07 R-FE-01 §10. Code: internal/mission/campaign_progression.go; cmd/nanolathe/loadgame.go.

Action: No change — no automatic first-unplayed selection inferred.

### B066 — Repeated patrol attachment release

**Agreement · Established.** The first reclaim branch releases the current order attachment twice. The release clears the attachment; the second release has no attachment to destroy and introduces no additional behavioral step.

Evidence and comparison: Own patrol branch and attachment installer/releaser inspected; the releaser gates destruction on a nonempty attachment before clearing it.

Research: 04 R-ORD-01 §4. Code: internal/orders/patrol.go.

Action: none.

### B067 — Mission diagnostic preserves historical spelling

**Agreement · Established.** The missing-mission diagnostic contains the original spelling error. Nanolathe already reproduces that diagnostic verbatim, including the spacing.

Evidence and comparison: Read the diagnostic bytes from our own executable; compared current formatter and exact-string tests. Mission package tests passed.

Research: 08 R-CAMP-01 §1; 02 R-MALF-01. Code: internal/mission/load.go; internal/mission/load_test.go.

Action: none.

### B068 — Repeated pure repair predicate

**Agreement · Established.** Both successful repair checks choose the same cursor. The earlier special case adds no distinct outcome because the predicate is read-only and the unconditional repeated check covers it.

Evidence and comparison: Own cursor case and complete repair predicate inspected; current repair-admission contract uses the same underlying eligibility result. HUD tests passed.

Research: 07 §8. Code: internal/hud/cursor.go; internal/session/cursor_repair_admits.go.

Action: none.

### B069 — Restriction-list name-address check

**Misinterpretation · Supported inference.** The restriction-list producer tests the address of each catalog entry's inline name, not whether its text is empty. This does not establish the proposed intent to skip unused entries or prove a null catalog element is reachable.

Evidence and comparison: Own list-producing loop inspected and current catalog/menu paths searched; no corresponding nullable per-row type input was established.

Research: 08 R-SKIR-01 §10. Code: internal/content/compile_unit.go; cmd/nanolathe/retail_menu.go.

Action: gap — only if implementing the legacy restriction screen: trace its populated catalog range and filter rules; do not infer an empty-name filter.

### B070 — Empty queue branch guarded by a successful peek

**Out of scope · Supported inference.** The apparent empty-pop null read occurs behind a successful queue peek. The inspected caller does not demonstrate a reachable empty-pop failure. The legacy packet queue is outside the replacement transport.

Evidence and comparison: Own packet-service peek/pop control flow inspected. No concurrent queue mutation proof was made; no claim that every indirect caller is safe.

Research: 08 "Synchronization and integrity checks"; docs/DESIGN_MULTIPLAYER.md. Code: internal/session/player_record.go.

Action: none for the replacement transport; a retail crash claim needs a mutation path between peek and pop.

### B071 — Repeated battle-entry player guard

**Agreement · Established.** The active-player, controller-kind and excluded-side conditions are repeated without intervening work. The second group of tests does not change which players enter the body.

Evidence and comparison: Own consecutive guard blocks inspected; current session admission applies semantic player eligibility rather than reproducing repeated machine tests.

Research: 08 R-ENTRY-01 §8; 08 R-SKIR-01 §3. Code: internal/session/player_record.go; internal/session/skirmish.go.

Action: none.

### B072 — Veteran kill line always uses the plural

**Agreement · Established.** The veteran branch is entered only above four kills, so its singular alternative cannot be taken. Nanolathe formats one kill in the singular and every veteran count in the plural.

Evidence and comparison: Own unsigned kill-count comparisons inspected; current KillsLine applies singular only at one and appends Veteran above four. HUD tests passed.

Research: 07 R-HUD-03 §2. Code: internal/hud/footer.go:KillsLine.

Action: none.

### B073 — Temporary restriction-entry value is replaced before use

**Misinterpretation · Supported inference.** The copied unwritten temporary belongs to a restriction-tree entry and is overwritten with zero after insertion. Calling it a world-position height is unsupported; the inspected routine does not move units.

Evidence and comparison: Own insertion helper and store sequence inspected; no world-position consumer identified. Current catalog uses typed definitions, not this transient legacy record.

Research: 08 R-SKIR-01 §10. Code: internal/content/compile_unit.go.

Action: none for world simulation; a visible defect claim needs a reader between insertion and the final overwrite.

### B074 — Sound aliases ignore capitalization

**Agreement · Established.** The differently capitalized skirmish sound names resolve through a case-insensitive alias comparison. Nanolathe's alias registry likewise folds case.

Evidence and comparison: Own menu call sites and alias-lookup contract inspected; current aliasEqual uses EqualFold.

Research: 03 §8.3. Code: internal/audio/registry.go:aliasEqual.

Action: none.

### B075 — Unnecessary expired-entry compaction is harmless

**Host boundary · Established.** An unwritten change flag can invoke a compaction pass when nothing expired, but that pass finds no hole and preserves entries and count. Nanolathe initializes its state and preserves the same retained-entry result.

Evidence and comparison: Own expiry and complete compaction loop inspected; current expiry logic uses deterministic initialized state.

Research: 01 R-PLAT-02 §5. Code: internal/session/eyeball.go.

Action: No change — do not introduce an indeterminate local.

### B076 — Unused player scan at teardown

**Agreement · Established.** The final teardown loop copies controller bytes into a local byte that is never read before return. It cannot change player state or the result of teardown.

Evidence and comparison: Own assembly from the final scan through the return inspected; all writes are to the discarded local.

Research: 08 R-ENTRY-01 §9. Code: internal/session/campaign_teardown.go.

Action: none.

### B077 — Selected list rows use the same shade with or without focus

**Misinterpretation · Established.** The two identical shade calls distinguish focused from unfocused selection, not selected from ordinary rows. The selected row still receives a shade that other ordinary rows do not.

Evidence and comparison: Own surrounding selected-row predicate inspected; current painter shades the selected non-heading row independently of focus.

Research: 07 R-WGT-01 §4. Code: cmd/nanolathe/retail_menu_list.go:drawRetailListRows.

Action: No change — do not add a focus-dependent shade on this evidence.

### B078 — Text-focus refresh is repeated

**Host boundary · Supported inference.** The legacy immediate-mode focus tail repeats font/color setup, text refresh and key-queue clearing. Nanolathe retains focus/editor state and renders from that state; it does not reproduce duplicate host redraw calls.

Evidence and comparison: Own two tail blocks inspected and current focus lifecycle read. Exact transient pixels and host key arrival between the repeated calls were not independently established.

Research: 07 R-WGT-01 §2. Code: internal/ui/focus.go:MoveFocus; internal/ui/panel.go:SetFocus.

Action: none for retained rendering; a visible mismatch claim needs an observable state or input difference between the two refreshes.

### B079 — Non-window first GUI record changes the root hit rectangle

**Unknown · Unknown.** Retail's root hit test doubles the first record's position when that record is not a window. Nanolathe accepts the first record as its window rectangle but its gadget hit test skips index zero. Stock window-header behavior is unaffected; malformed-header admission and downstream behavior remain unresolved.

Evidence and comparison: Own root rectangle branch inspected; current loader's first-record fallback and HitTest index-zero exclusion read. No malformed-header service/callback trace or reference fixture establishes the complete visible effect.

Research: 07 R-WGT-01 §1; 02 §6. Code: internal/gui/load.go; internal/ui/panel.go:HitTest.

Action: Gap, now documented: trace loader acceptance and root service handling for a first record of another kind; the audit adds the owning UI Unknown and HitTest TODO.

### B080 — Lens displacement allocation reserves extra storage

**Host boundary · Established.** The lens initializer reserves more storage than its displacement cells alone require and installs two internal plane pointers. Allocation size is not itself a rendering contract, and the assertion that every extra byte is unnecessary needs its consumers.

Evidence and comparison: Own allocator and displacement-cell initialization inspected; current lens representation is independently typed and sized.

Research: 03 R-FX-01 §4. Code: internal/client/projectile_lens.go; internal/drawlist/lens.go.

Action: No change — preserve displacement behavior rather than a legacy allocation shape.

### B081 — Archive copyright text used as a fixed format string

**Host boundary · Established.** The archive writer supplies a generated copyright line as the formatting template, but that line contains only fixed text and year digits. This call is not a demonstrated user-controlled format-string path.

Evidence and comparison: Own string construction and formatting call inspected; current writer uses Go I/O and independently builds archive data.

Research: 02 "Archive"; docs/DESIGN_CONTENT_VFS.md. Code: vfs/hpi_write.go:WriteArchive.

Action: No change — do not imitate the formatting API choice.

### B082 — Ignored return value from a surface effect

**Host boundary · Supported inference.** The supplied-surface branch leaves an incidental return value unrelated to its pixel operation. The direct call sites consume the side effect, so Nanolathe needs the compositing result rather than this incidental legacy return convention.

Evidence and comparison: Own helper and five direct-call locations inspected; current indexed compositor exposes its pixel operation without that legacy ABI.

Research: 03 R-COMP-01 §2. Code: internal/client/classic_sink.go.

Action: No change — no behavioral API should be added for an unused return value.

### B083 — Repeated compression-buffer null check

**Host boundary · Supported inference.** The compression helper checks a buffer after successful allocation and checks it again during cleanup. A redundant diagnostic guard adds no compression output; a whole helper-side mutation census was not used to claim absolute unreachability.

Evidence and comparison: Own compression initializer and cleanup inspected; current archive compression uses separate Go-owned buffers.

Research: 02 R-VFS-01; research/formats/hpi.md. Code: vfs/hpi.go; vfs/hpi_write.go.

Action: No change — retain safe cleanup, not obsolete diagnostic branches.

### B084 — Diagnostic buffer is cleared just before replacement

**Host boundary · Established.** Writing an empty string immediately before copying a newline has the same final text as copying the newline once. There is no intervening reader.

Evidence and comparison: Own adjacent diagnostic-buffer writes inspected. Current host diagnostic formatting does not expose the temporary string state.

Research: 01 "Platform"; docs/ARCHITECTURE.md. Code: internal/platform.

Action: none.

### B085 — Compression error-name table has malformed final entries

**Host boundary · Established.** The compression error-name table combines two names and leaves the final accepted slot null. This concerns error diagnostics, not unpacked data. Ordinary unpack-error entries precede the malformed tail.

Evidence and comparison: Own executable table bytes and lookup range inspected; current decoder returns typed Go errors rather than pointers from the old table. No claim that every possible error-code caller was exhaustively excluded.

Research: 02 R-VFS-01; research/formats/hpi.md. Code: vfs/hpi.go.

Action: No change — keep defined errors; proving a retail-visible malformed message requires a caller producing the final error codes.

### B086 — Unused argument-shift utility has reversed bounds

**Out of scope · Supported inference.** The argument-shift helper clears arguments on the ordinary count relation and can copy beyond the intended range on the inverse relation. No direct call to this helper occurs in our executable scan; indirect reachability is not proven absent.

Evidence and comparison: Own helper inspected and complete assembly direct-call search returned no calls. Nanolathe parses its own command line.

Research: 01 R-DET-01 §4. Code: cmd/nanolathe/flags.go.

Action: No change — no live gameplay behavior established; an indirect caller would be needed to reopen this lead.

### B087 — Save compression allocation failure falls back to raw data

**Agreement · Established.** When the compression buffer cannot be allocated, the writer emits the uncompressed bank. The stale compression-result value is not used to select compressed output without a buffer.

Evidence and comparison: Own full compression-result condition shows the buffer-presence guard and raw-write branch; current save writer uses explicit compression/error handling.

Research: 08 R-ENTRY-02 §3; research/formats/save.md. Code: internal/save/writer.go.

Action: No change — preserve a defined raw fallback without an indeterminate local.

### B088 — Unused caller-buffer file loader has incorrect failure ownership

**Out of scope · Supported inference.** The caller-buffer variant can free supplied storage after an empty read. No direct calls were found in our executable, so this is not evidence of a live loading failure. Nanolathe's reader ownership is explicit.

Evidence and comparison: Own whole helper read; complete direct-call search found none. Current VFS caller-owned read interfaces do not transfer ownership on an empty read.

Research: 02 R-VFS-01. Code: vfs/vfs.go.

Action: No change — require an indirect caller before describing a live retail path.

### B089 — Passability progress is one completed class ahead

**Host boundary · Established.** The movement-class passability progress counter advances before division and can exceed 100 until the final assignment. This duplicates B058; Nanolathe documents different catalog-family loading progress.

Evidence and comparison: Own count/build loops and final store inspected; compared the explicit host loading-attribution comment.

Research: 07 "The loading screen"; 04 §6.1. Code: cmd/nanolathe/loading.go.

Action: No change — no gameplay discrepancy; count only populated classes.

### B090 — Missing unit section exits without all legacy cleanup

**Host boundary · Established.** A missing UNITINFO section takes an early exit that skips file-handle and shared-parser cleanup present on the normal path. Nanolathe already models the semantic early stop using managed data and does not need to leak resources.

Evidence and comparison: Own missing-section branch and normal cleanup compared; current SC24 test locks that following unit files are not compiled after this failure.

Research: 02 R-MALF-01 §5; docs/SPEC_CONFLICTS.md SC24. Code: internal/content/compile_unit.go; internal/content/sc24_test.go.

Action: No change — preserve the stop behavior and safe cleanup.

### B091 — Two aircraft recovery waypoints are never installed

**Agreement · Established.** The air attack recovery and transport climb branches construct a waypoint without attaching it. The existing contracts explicitly preserve the old marker and the relevant random draws; Nanolathe already follows that behavior.

Evidence and comparison: Own two order branches compared with the existing detailed research and current comments/branches around discarded markers. Focused air-order tests passed.

Research: 04 R-AIR-01 §8; 04 R-AIR-01 §10. Code: internal/movement/airorders.go.

Action: No change — do not install these waypoints as an apparent bug fix.

### B092 — Attachment-order allocation failure is unchecked

**Host boundary · Established.** The retail attachment-order constructor dereferences the result even when allocation failed. Nanolathe must retain defined allocator failure behavior rather than reproduce a null read.

Evidence and comparison: Own constructor's null branch and following metadata read inspected; current order ownership/queue interfaces searched.

Research: 04 R-FAC-02 §1. Code: internal/orders/transport.go; internal/orders/queue_handlers.go.

Action: No change — allocation failure is a host boundary, not an extra order transition.

### B093 — CD path builder assumes a nonnull extension

**Host boundary · Supported inference.** The CD path builder reads the extension unconditionally, unlike its sibling. A null-extension caller was not established; current path construction uses Go strings and cannot reproduce the C null-pointer distinction.

Evidence and comparison: Own helper and direct-call search inspected; only one direct call located, but no complete parameter/indirect-caller proof was claimed.

Research: 08 R-CAMP-01 §5. Code: internal/mission/load.go.

Action: none for host path handling; a retail crash claim needs a caller supplying a null extension.

### B094 — Oversized self-destruct counts exceed the announcement table

**Host boundary · Established.** Countdown values six and seven are representable and can reach a six-entry announcement table without a retail bounds check. This is already an explicit research unknown for the resulting local cue. Nanolathe preserves countdown progression and safely omits those undefined cue kinds.

Evidence and comparison: Own handler and current three-bit countdown/announcement guard compared. Existing code documents the exact safe fallback and the retained count progression.

Research: 04 R-ORD-01 §14; 04 R-SPEC-01 §13; 04 "Missing and unknown". Code: internal/orders/selfdestruct.go.

Action: No change — existing documented gap; runtime cue behavior would require bounded stack/cue observation, not an invented seventh sound.

### B095 — Unused piece-explosion helper leaves flag bits unwritten

**Out of scope · Supported inference.** An alternate piece-explosion helper sets only part of a temporary flag value. No direct caller was found, and no consumer of its remaining bits was established. This does not change the verified live explosion path.

Evidence and comparison: Own helper inspected and full assembly direct-call search found no calls. Existing research distinguishes the unused variant from live script explosion handling.

Research: 01 R-DET-01 §4. Code: internal/render/debris.go; internal/cob.

Action: No change — require a reachable caller and flag consumer before adding behavior.

### B096 — Truncated mover saves can leave unread retail bytes

**Host boundary · Established.** The unchecked fixed-length restore belongs to a mover record, not a unit definition. A short record can leave some retail temporary bytes unspecified. Nanolathe checks the mover record length before projection.

Evidence and comparison: Own restore helper compared with the current exact 35-byte mover check and its error return.

Research: 08 R-SAVE-02 §8. Code: internal/save/retail_projection.go.

Action: No change — retain defined rejection of truncated saves.

### B097 — Missing video-slider gadget is dereferenced before its check

**Host boundary · Established.** The legacy options helper reads a slider subobject before checking whether lookup found the gadget. Nanolathe uses named optional controls without reproducing this null dereference.

Evidence and comparison: Own VIDSLDR lookup/read order inspected; current options helpers and missing-widget handling searched.

Research: 07 R-FE-01 §6. Code: cmd/nanolathe/retail_menu_options.go:syncRetailVideoLabel.

Action: No change — preserve safe behavior for incomplete GUI content.

### B098 — Legacy restriction files assume successful file opening

**Host boundary · Established.** The restriction-list reader and writer pass an unchecked file-open result to subsequent I/O. This is a host I/O failure, not a defined restriction policy or unit eligibility rule.

Evidence and comparison: Own two file helpers read; current catalog/menu search found no matching unchecked C-style file API or ownership assumption.

Research: 07 R-FE-02 §1; 08 R-SKIR-01 §10. Code: internal/content/compile_unit.go; cmd/nanolathe/retail_menu.go.

Action: No change — any implementation of this file route should return a defined I/O error.

### B099 — Heading-matching markers require a target

**Host boundary · Established.** Ordinary air-marker constructors pair heading matching with following. The owning contract establishes that target removal clears marker references, including displaced and restored markers; the follow check therefore rejects a missing target before the heading read. Nanolathe preserves this behavior and also safely rejects an inconsistent restored marker that requests heading matching without following. The local unchecked read does not establish an ordinary target-lifetime defect.

Evidence and comparison: Compared the own executable arrival branch with the established constructor and target-reference lifetime contract in section 4. Current constructors pair the two flags; ordinary flag setters only add altitude, radius or explicit heading. Current restore retains saved flags and Arrived rejects a missing target. Normal displaced and reconstructed marker lifetime is already settled.

Research: 04 R-AIR-01 §4; 08 R-SAVE-02 §10. Code: internal/movement/airorders.go:airMarker.Arrived; internal/movement/retail_restore.go:restoreRecordGoal.

Action: No change — retain safe handling of inconsistent restored flags. Only a claim about malformed saved markers remains unverified: it requires a retail restore-admission trace for a heading-match-only flag combination and its target identifier, or a concrete additional live flag writer. This is not an unresolved ordinary target-lifetime contract.

### B100 — Restriction image-list guard does not prove a nullable unit type

**Misinterpretation · Supported inference.** The image-list producer indexes the unit catalog and tests an inline name address. The alleged null unit-type input is not a freely supplied function argument, so the local ineffective check does not prove a reachable null access.

Evidence and comparison: Own whole image-list producer inspected together with its catalog indexing; compared current typed catalog construction. B069 covers the related text-list guard.

Research: 08 R-SKIR-01 §10. Code: internal/content/compile_unit.go; cmd/nanolathe/retail_menu.go.

Action: gap — if the legacy restriction UI is implemented, verify populated index bounds and callback callers; no null-entry failure is established here.

### B101 — Legacy packet checksum omits the payload tail

**Out of scope · Established.** The inspected sender transforms and checksums a prefix that excludes the last three payload bytes. Its receiver's corresponding domain was not independently established, so a transport corruption claim is unwarranted. Nanolathe does not interoperate with this transport.

Evidence and comparison: Own sender loop bounds inspected; replacement deterministic relay transport is an explicit product policy, not retail packet compatibility.

Research: 08 "Synchronization and integrity checks"; docs/DESIGN_MULTIPLAYER.md. Code: internal/session.

Action: none for Nanolathe transport; retail protocol interpretation needs the receiver loop and packet framing.

### B102 — Directory enumeration flag does not suppress files

**Misinterpretation · Established.** The flag selects whether directories are enumerated first; the subsequent file enumeration runs independently. The shared branch destination does not establish an ignored file-enumeration flag.

Evidence and comparison: Own directory-then-file walker inspected; current saved-game listing uses host directory enumeration and explicit entry selection.

Research: 08 R-SAVE-02 §1. Code: cmd/nanolathe/loadgame.go; cmd/nanolathe/savegame.go.

Action: No change — do not make file enumeration conditional on the directory flag.

### B103 — Legacy movie-call argument width is unresolved

**Out of scope · Unknown.** A legacy movie DLL call receives a value whose upper bits are not cleared. Without the imported routine's parameter contract, it is unknown whether those bits matter. Nanolathe's movie decoder does not use that DLL ABI.

Evidence and comparison: Own call-site argument construction inspected; no foreign DLL execution or third-party disassembly performed.

Research: 08 R-OOS-01 §4. Code: internal/film; internal/audiobackend/movie.go.

Action: none for the native movie path; resolving the historical call requires an authoritative API signature or bounded runtime evidence.

### B104 — Empty command-path animation can divide by zero

**Host boundary · Established.** The command-path animation clamps its timing interval but assumes a nonempty frame sequence for modulo selection. An empty art record violates that assumption. Nanolathe's presentation can reject or skip missing animation frames safely.

Evidence and comparison: Own two consecutive divisions inspected; current queued-order overlay/art handling searched. No normal nonempty asset reaches the zero divisor.

Research: 03 R-FX-01 §5; 07 R-HUD-03 §12. Code: internal/hud/queueoverlay.go; internal/render/gaf_cursor.go.

Action: No change — preserve defined missing-art handling rather than a host divide fault.

### B105 — Main-menu sparks can erase and block each other

**Misinterpretation · Established.** This routine animates main-menu background sparks, not battlefield smoke or burnt features. It restores each spark's previous pixel, then tests the updated shared image before drawing the next pixel. Nanolathe follows that ordered shared-image behavior.

Evidence and comparison: Own complete routine identifies the menu bitmap, fixed spark array and shared-image nibble test; current tick erases, moves, tests and draws in record order.

Research: 07 §5 "Main-menu background shimmer (SPARKS) is closed". Code: cmd/nanolathe/menu_sparks.go:tick; cmd/nanolathe/menu_sparks_test.go.

Action: No change — do not change feature fire or smoke behavior based on this lead.

### B106 — Claimed controller-name overflow uses the wrong literal

**Misinterpretation · Established.** The cited message-dialog caller passes the short name TEXT, not the claimed long controller-format string. The helper's unbounded copy is a general C buffer hazard, but the stated overflowing call is disproved by the executable.

Evidence and comparison: Read the actual caller argument in own assembly and decoded the referenced PE bytes; compared the typed runtime-label representation.

Research: 07 R-FE-02 §5. Code: cmd/nanolathe/retail_panel_test.go:TestShowRetailMessageBuildsRuntimeLabels; internal/gui/names.go.

Action: No change — no controller-label geometry correction is supported.

### B107 — Label shortening can stall for a width below its inset

**Host boundary · Established.** The fitting loop compares measured text width with gadget width minus six; it is not a height comparison. At a width below six, even an empty string cannot satisfy the retail loop. This malformed-size hang is not a rendering rule to reproduce.

Evidence and comparison: Own text-measure and truncation loop inspected, including the width field read; current bounded Go text/layout paths searched.

Research: 07 R-FE-02 §5. Code: cmd/nanolathe/retail_font.go; cmd/nanolathe/retail_menu_draw.go.

Action: No change — keep terminating text fitting and safe malformed geometry handling.

### B108 — GAF font baseline without a capital-I frame is undefined

**Unknown · Unknown.** When the capital-I frame is absent, retail uses an unwritten baseline value. The repository already records this malformed-font case as Unknown; no deterministic baseline can be inferred from that load.

Evidence and comparison: Own GAF-font loader inspected; compared existing Unknown text and current baseline helper. This is an existing documented gap, not a newly closed contract.

Research: 03 R-FONT-01 §6; 03 "Missing and unknown"; 07 §4. Code: cmd/nanolathe/retail_font.go:retailGAFBaselineHeight.

Action: gap — existing: a bounded loader/runtime observation is needed to characterize a particular malformed font; retain a defined host fallback meanwhile.

### B109 — Sunken-frame helper uses two edge colors

**Agreement · Established.** The sunken outline helper reads two edge colors. An extra value passed by its callers does not establish an omitted highlight: current research separately specifies the outline and optional fill operations.

Evidence and comparison: Own outline helper and its two call sequences inspected; compared the documented two-color sunken-frame contract and current drawing helpers.

Research: 07 R-FE-02 §4. Code: cmd/nanolathe/retail_menu_draw.go.

Action: No change — do not invent a third-color highlight.

### B110 — Projectile velocity uses computed speed, not a pointer

**Misinterpretation · Established.** The projectile creator reuses argument storage for its calculated horizontal speed. The later trigonometric scale call reads that calculated scalar, not the earlier position pointer. The apparent pointer-length bug is a data-flow misreading.

Evidence and comparison: Own assembly followed the horizontal-speed result store and subsequent stack-relative read while accounting for pushed arguments; compared ordinary projectile velocity construction.

Research: 06 §6.3. Code: internal/combat/service.go.

Action: No change — no projectile velocity correction is supported.

### B111 — Structure-shadow mask copying has an unresolved horizontal overrun

**Unknown · Unknown.** Retail's shadow punch computes its horizontal copy length without subtracting the destination column and does not reject a negative length. Nanolathe clips each destination pixel. The sole direct caller uses ordinary projected body and shadow images, so malformed-input-only reachability cannot be assumed; whether valid extents produce visible row spill remains unresolved.

Evidence and comparison: Own full copy helper, sole direct caller and shadow bounds producer inspected. Current punchOut safely clips both axes. Projection formulas alone were not sufficient to establish all reachable image extents and opaque pixel overlap.

Research: 03 R-RAST-01 §4; 03 R-REN-03D §5. Code: internal/client/model_shadow_pass.go:modelTarget.punchOut.

Action: Gap, now documented: trace body/shadow extents and original scratch storage for a reachable model, then establish whether excess opaque writes alter committed pixels. The audit adds the owning Unknown and punchOut TODO; do not add unsafe writes.

### B112 — Unused movie statistics routine has questionable timing arithmetic

**Out of scope · Unknown.** The movie-statistics writer contains suspicious repeated timing arithmetic, but its recovered data types are unreliable and no direct caller was found. Neither an always-1000 report nor a reachable zero-divide is established for a live movie session.

Evidence and comparison: Own helper/export inspected and full direct-call search found no caller. A complete imported statistics structure and instruction-level value trace were not established.

Research: 08 R-OOS-01 §4. Code: internal/film.

Action: none for native playback; the historical diagnostic needs a reachable caller and exact statistics data-flow proof.

### B113 — Campaign error formatting can exceed its legacy buffer

**Host boundary · Supported inference.** The campaign-open error combines a path with fixed text in a smaller C buffer. A sufficiently long accepted path can exceed that buffer; Nanolathe uses dynamically sized diagnostic strings. Complete path-length reachability was not established.

Evidence and comparison: Own error formatter and source-name capacity inspected; current diagnostics use Go formatting and preserve required text without a fixed stack buffer.

Research: 08 R-CAMP-01 §1. Code: internal/mission/load.go.

Action: No change — retain bounded or dynamically sized host error handling.

### B114 — Map identity reader trusts a complete TNT header

**Misinterpretation · Established.** The cited routine computes TNT map identity, not campaign-file identity. Its header read result and later extents are unchecked. A truncated header retaining the expected version prefix can pass the comparison, so the external short-read concern remains valid; the correction is the subsystem identity. Nanolathe validates TNT input before using dimensions and offsets.

Evidence and comparison: Own complete identity routine identifies the TNT version, terrain/feature data and checksum walk; current TNT parser performs size and offset checks.

Research: 02 R-MAP-01 §3; 08 "Map/resource identity"; research/formats/tnt.md. Code: formats/tnt.go; internal/content.

Action: No change — retain defined malformed-map rejection; do not claim every truncated header passes or changes campaign behavior.

### B115 — List keyboard selection uses a reachable visible-row interval

**Misinterpretation · Established.** The normal downward-selection branch requires the selection to lie inside the visible interval; the alleged contradictory comparisons reverse an operand. The positive-row branch is reachable.

Evidence and comparison: Independent branch-target and operand reading establishes top <= selected < top+rows. Panel.keyboardList advances selection and adjusts top using the font metric. Existing keyboard fixtures exercise downward movement.

Research: 07 R-WGT-01 §4. Code: internal/ui/keyboard_service.go; internal/ui/keyboard_service_test.go.

Action: No change.

### B116 — Named button artwork bypasses fallback size selection

**Agreement · Established.** A successful named-art lookup uses its initial frame. Only fallback art scans size alternatives and resets frame offsets; both paths subsequently update geometry from the selected frame.

Evidence and comparison: Independent named-lookup branches jump over the fallback scan. The implementation resolves named frames before fallback selection; focused fixtures distinguish named art and fallback sizing.

Research: 07 R-WGT-01 §3, art and frame base. Code: cmd/nanolathe/retail_menu.go; cmd/nanolathe/newwidget_art_service_test.go.

Action: No change.

### B117 — Nearest-colour failure maps the search exit position through the palette permutation

**Misinterpretation · Established.** If no candidate wins, the selected permutation position is the scan exit position modulo 256. The returned color is the original palette index stored at that position. Exhausting the scan selects permutation position zero; an early upper-band exit can select another position. The external claim that the fallback always selects the first permutation entry is too broad.

Evidence and comparison: Own search export includes the early brightness-band break and subsequent permutation lookup. nearestBySum implements both steps. Existing recovery fixtures cover other palette contracts; they do not independently establish this fallback.

Research: 03 §4.3.4, R-PAL-RECOVERY §1. Code: internal/palette/palette.go; internal/palette/recovery_test.go.

Action: No change.

### B118 — Submerged tint has an unreachable blue clamp

**Agreement · Established.** The target blue channel is half the source blue plus 50. The separate half-blue-plus-60 comparison cannot reach 256 for a byte input, so its saturation arm is dead.

Evidence and comparison: Own tint-builder export confirms the distinct comparison and target constants. buildBlueTable explicitly documents the unreachable guard and uses the established target.

Research: 03 R-WATER-01 §2. Code: internal/palette/palette.go.

Action: No change.

### B119 — Missing scrollbar artwork can leave a null frame access

**Host boundary · Established.** Retail skips the first draw for a missing frame but still reads its dimensions afterward; subsequent frames also assume valid artwork. Nanolathe suppresses unavailable art rather than manufacturing a null-memory result.

Evidence and comparison: Own drawer export shows dimensions consumed after the null draw guard. Current art resolution and drawing check absent frames. No valid-art difference established.

Research: 07 R-WGT-01 §5; DESIGN_PRESENTATION_CLIENT §2.3 missing-art boundary. Code: cmd/nanolathe/retail_menu_draw.go; cmd/nanolathe/retail_menu_options.go.

Action: No change.

### B120 — Temporary list-rewrite storage is not released

**Host boundary · Established.** The retail list-rewrite helper allocates scratch storage, copies the rewritten list back, and returns without a release. This is a host allocation leak, not list-content semantics to reproduce.

Evidence and comparison: Entire independent assembly body was read, including its allocation and all return paths. Panel list state uses Go-owned strings and slices with no manual scratch allocation lifetime.

Research: 07 R-WGT-01 §4; DESIGN_INTERFACE_HUD_INPUT §3 widget ownership. Code: internal/ui/panel.go.

Action: No change.

### B121 — Missing required gadgets terminate before subsequent stores

**Agreement · Established (external caveat resolved).** The external entry explicitly conditions its crash concern on whether the error reporter returns. It does not: the helper exits the process, so the apparent subsequent null writes are unreachable on the normal fatal-error path. This resolves the stated caveat rather than disproving an unconditional external claim.

Evidence and comparison: Own lookup exports were followed through the message-box helper into the runtime exit routine and ExitProcess. Research already identifies these lookups as fatal. Nanolathe unavailable-panel handling is a host boundary.

Research: 07 R-FE-02 §5, Gadget lookup, mutation and synthesis helpers. Code: internal/gui/load.go; cmd/nanolathe/frontend_availability.go.

Action: No change.

### B122 — Command registration mixes folding and exact equality

**Unknown · Unknown.** Registration locates insertion positions with a case-insensitive comparison but recognizes an existing entry with exact byte equality. Case variants can therefore coexist. The inspected callers register fixed command names; a reachable conflicting registration is not established.

Evidence and comparison: Own helper assembly confirms both comparators. Complete direct-reference search finds three registrations from one bootstrap, with distinct fixed names plan, weight and limit. A byte search of the independent executable found no stored absolute pointer to this registration helper. Computed references are not exhaustively excluded. Nanolathe parses the established grammar directly.

Research: 01 R-PLAT-01 §9; 08 R-AI-01 §12. Code: internal/ai/profile.go.

Action: The owning session/AI Unknown now records the missing computed-call census. No mutable command-registration seam exists in Nanolathe; no dependent behavior changed.

### B123 — Dormant message filtering and ordinary font-height spacing

**Misinterpretation · Established.** The special presenter mode can retain an unwritten display decision for one class, as the external entry correctly observes. Initialization selects a different mode, and the static writer census establishes no ordinary path to the problematic mode. The vertical increment is the font height, not a timestamp. The correction concerns spacing and demonstrated reachability, not denial of the conditional defect.

Evidence and comparison: Own drawer export reads the font-height helper before its loop; research records process initialization to mode 3 with no other writer. Existing client message-ring behavior follows the reachable mode.

Research: 07 R-HUD-03 §14, class filter and screenchat polarity. Code: internal/client/audio.go.

Action: No change.

### B124 — Loading progress assumes at least one eligible player

**Out of scope · Established.** The retail waiting-player layout divides the available width by the eligible-player count without a zero guard. A zero-player path is not established; the DirectPlay waiting screen is outside the implemented transport contract.

Evidence and comparison: Own waiting-screen export confirms the participant filter and division. Nanolathe loading presentation and relay readiness do not use this DirectPlay routine.

Research: 07 R-FE-01 loading screen; 08 DirectPlay transport; DESIGN_MULTIPLAYER §3.2. Code: cmd/nanolathe/loading.go; docs/DESIGN_MULTIPLAYER.md.

Action: none: retain transport boundary; a retail zero-participant caller trace would be needed to establish the crash reachability.

### B125 — Cursor regions use separate coordinate conversions

**Misinterpretation · Established.** The two flag branches describe minimap and main-view regions, so their different coordinate conversions are expected. The aggregate flag reflects whether either region matched. A separate unchecked off-map feature read is a malformed-coordinate host boundary.

Evidence and comparison: Own resolver branches were followed from each distinct region test through scaling. Current cursor conversion distinguishes minimap/world view, and the documented SC20 boundary handles map edges.

Research: 07 §8; 07 §10; SPEC_CONFLICTS SC20. Code: cmd/nanolathe/battle_placement.go; internal/world/placement.go.

Action: No change.

### B126 — Footprint scan uses the map row and a scalar occupancy identity

**Misinterpretation · Established.** The initial cell is at row times map width plus column. After scanning a footprint row, traversal skips map width minus footprint width to reach the next row. The second argument is a unit occupancy identity; the fallback slope read uses the separate unit-definition argument. The alleged wrong-row and null-pointer bugs do not follow.

Evidence and comparison: Independent instruction operands establish both the row arithmetic and argument identity. Current placement loops use the same footprint and terrain concepts.

Research: 04 R-P0-08; 04 §8. Code: internal/world/placement.go.

Action: No change.

### B127 — Classic rectangle shading treats high palette indices as signed

**Defect — corrected · Established.** For source indices 128–255 and a selected table row above zero, retail reads the preceding row at that same high index. The audit found Nanolathe read the selected row; the reviewed correction now matches the defined in-table behavior. At row zero the retail read precedes the table and remains an explicit bounded host fallback. Missing-table returns also omit legacy surface unlock, a separate host issue.

Evidence and comparison: Independent assembly explicitly sign-extends each source pixel before indexing the selected row. Reviewed the corrected uiShadeRectRaw, owning research/design and high-index fixture against that arithmetic. go test ./internal/client -run TestUIShadeRect -count=1 passed, including both tables, row clamping and the row-zero placeholder.

Research: 03 R-COMP-02 §5, rectangle shader. Code: internal/client/unitdraw.go; internal/client/palette_test.go; internal/palette/palette.go.

Action: Corrected defined in-table Original lookup; both tables and high-index boundaries tested. Row-zero underflow retains the explicit bounded placeholder.

### B128 — Redundant fallback surface branch is unreachable

**Host boundary · Established.** A branch tests the address of a local surface descriptor, which cannot be null; its secondary lock path is dead. The ordinary locked-surface draw still runs, so this does not establish lost visible output.

Evidence and comparison: Own drawer export and control flow show the first successful lock feeds the live branch. Nanolathe owns its framebuffer without the legacy surface-lock fallback.

Research: 03 §4.2; 03 R-COMP-02 §5. Code: internal/client/unitdraw.go; internal/platform/ebitenapp.

Action: No change.

### B129 — Unit synchronization assumes an owner link

**Out of scope · Established.** A retail synchronization writer tests the unit controller but then reads an owner-link identity without checking that link. Whether an admitted synchronized unit can lack it remains unproved. Nanolathe does not emit retail synchronization packets.

Evidence and comparison: Own synchronization export confirms the missing link test and its packet writer calls. Current multiplayer is command lockstep with no retail packet interoperability.

Research: 08 Lockstep advancement; DESIGN_MULTIPLAYER §3.2. Code: docs/DESIGN_MULTIPLAYER.md; internal/session.

Action: none: retail allocation/link lifecycle would settle reachability; no transport parity change.

### B130 — Placement instantiation trusts its stored count

**Host boundary · Established.** The placement-instantiation helper bounds a negative allocation size but passes the original count to initialization. A negative stored count can make those operations disagree. The caller is battle placement initialization; calling the value inherently network-supplied is not established.

Evidence and comparison: Own helper and battle-entry caller exports were read. Current placement parsing uses bounded records and slices rather than this signed-count allocator.

Research: 08 R-ENTRY-01; 02 R-MALF-01; DESIGN_CONTENT_VFS §2 parser limits. Code: internal/mission; internal/session.

Action: none: preserve bounded parsing; a writer trace producing a negative placement count is needed before claiming ordinary retail reachability.

### B131 — Leading help separators consume the first description byte

**Defect — corrected · Established.** Retail replaces the opening separator and following byte with a space and terminator, then starts the description after that terminator. An authored leading-separator row therefore loses its next byte. The audit found Nanolathe preserved it; the reviewed correction drops it. A separator-only row and a missing separator remain explicit safe host boundaries.

Evidence and comparison: Independent filler assembly and the byte-scanning line helper establish the two-byte overwrite and description start. Reviewed splitBattleHelpLine, the focused fixture and owning research. go test ./cmd/nanolathe -run ^TestHelpLineSplit$ -count=1 passed, including ordinary lines, leading separators, one-character descriptions and bounded empty input.

Research: 07 R-FE-01 §7, HELP.GUI filler; fmt tdf, HELP.TDF. Code: cmd/nanolathe/battle_help_window.go; cmd/nanolathe/battle_info_window_test.go.

Action: Corrected the leading-separator split and existing test; empty and missing-separator inputs retain bounded host handling.

### B132 — Options animation keeps its temporary writes within bounds

**Misinterpretation · Established.** The alleged local-buffer overlap is absent. Accounting for the complete allocation and lifetime shows that the final quad write stays within its own temporary storage.

Evidence and comparison: Independent storage-lifetime analysis and all quad writes were checked. No caller workaround or state-corruption fix is required.

Research: 07 R-FE-01 §6, options pages. Code: cmd/nanolathe/retail_menu_options.go.

Action: No change.

### B133 — Slider opening keeps the scaled value through rounding

**Misinterpretation · Established.** The fractional comparison consumes a temporary duplicate; the original scaled position remains available for the final integer conversion. The position is not reduced to zero or one.

Evidence and comparison: Independent arithmetic and conversion analysis establishes preservation of the original scaled value. retailSliderKnob implements the documented ceiling operation.

Research: 07 R-FE-01 §6, slider arithmetic; 03 R-AUD-01 §2, §4. Code: cmd/nanolathe/retail_menu_options.go; cmd/nanolathe/options_slider_layout_test.go.

Action: No change.

### B134 — Legacy ping reply uses an unchecked missing-player index

**Out of scope · Established.** The transport reply handler can resolve an unknown identity to the spare player index and continue without a range check. Validity of such messages is not established. This is part of retail ping bookkeeping.

Evidence and comparison: Own reply-handler export confirms lookup fallback and later access. Nanolathe relay transport does not implement this handler.

Research: 08 DirectPlay transport; DESIGN_MULTIPLAYER §3.2. Code: docs/DESIGN_MULTIPLAYER.md.

Action: No change.

### B135 — Particle append retains all position and lifetime fields

**Misinterpretation · Established.** The complete append operation retains the position, frame count and lifetime inputs. The claimed overwrite and omission result from an incorrect reading of temporary storage movement.

Evidence and comparison: Independent input-to-output tracing accounts for temporary storage changes and the complete append. No particle-field corruption is established.

Research: 03 R-FX-01 §3, flame-stream trail admission. Code: internal/effects.

Action: No change.

### B136 — Device colour fill uses a success status result

**Agreement · Established (return-convention question resolved).** The external entry offers alternative interpretations of the return test. The device operation is a DirectDraw colour-fill call whose zero status denotes success; the wrapper converts that status to a Boolean. The software path returns Boolean success directly. This resolves the differing conventions without establishing an inverted fade.

Evidence and comparison: Own export shows the surface operation arguments and zero-success conversion; existing backend research identifies the DirectDraw colour-fill branch. Nanolathe uses its own framebuffer/backend lifecycle.

Research: 03 §4.2; 03 R-COMP-02 §5. Code: internal/client/unitdraw.go; internal/platform/ebitenapp.

Action: No change.

### B137 — Transport group assignment trusts a resolved destination

**Out of scope · Established.** After a successful legacy send, one group-assignment path uses the missing-player sentinel without rejecting it. A valid call naming no active destination is not established.

Evidence and comparison: Own group-assignment export confirms its send-success gate and unchecked destination fallback. Current relay lobby does not execute this packet path.

Research: 08 DirectPlay transport; DESIGN_MULTIPLAYER §3.2. Code: docs/DESIGN_MULTIPLAYER.md.

Action: none: establish destination call domain before a retail reachability claim.

### B138 — Exhausted transport groups leave an outgoing byte unwritten

**Out of scope · Established.** If every group conflicts, the legacy search exits without setting the outgoing value byte. Reachability needs both a full group set and a destination not excluded by the search; ordinary active-player assignment does not by itself establish that combination.

Evidence and comparison: Own group-assignment export confirms assignment only on successful search and the ten-attempt limit. Current transport has a different explicit lobby schema.

Research: 08 DirectPlay transport; DESIGN_MULTIPLAYER §3.2. Code: docs/DESIGN_MULTIPLAYER.md.

Action: none: trace destination and group population if historical crash reproduction is needed.

### B139 — Broadcast deduplication reads the group index before validating it

**Out of scope · Established.** Retail reads its already-sent group table using the stored group index before applying a bounds check to a later write. An invalid group producer is not established. Nanolathe uses no such retail group table.

Evidence and comparison: Own broadcast export confirms read-before-check ordering. Current relay transport boundary excludes retail packet/group interoperability.

Research: 08 DirectPlay transport; DESIGN_MULTIPLAYER §3.2. Code: docs/DESIGN_MULTIPLAYER.md.

Action: No change.

### B140 — Side display names preserve the first byte and transform the suffix

**Agreement · Established.** The first byte is retained; every later nonzero byte receives byte addition by 32, even punctuation or lowercase letters. Nanolathe already preserves this odd display conversion. Retail fixed-buffer overflow for long names is a separate host boundary.

Evidence and comparison: Own side-list constructor and save/load callers were read. retailSideDisplayNames and its focused test include punctuation, lowercase and byte wrap cases; Go-owned strings avoid the legacy buffer overrun.

Research: 08 R-SAVE-02 §3. Code: cmd/nanolathe/loadgame.go; cmd/nanolathe/save_side_names_test.go.

Action: No change.

### B141 — Briefing normalization copies the last character and replaces a carriage return

**Misinterpretation · Established.** For ordinary text, the loop copies the current character before testing the following terminator, so it retains the final character. Inside a coloured run, a newline rewrites the preceding carriage return into a closing marker and emits a new line plus reopening marker. For bare newlines inside a colored run, the preceding visible character can be replaced; this remains a separate limitation.

Evidence and comparison: Own copy loop and newline branch were checked. Existing briefingSplitBlinkRuns fixture locks the carriage-return/newline case and reopening colour; the alleged final-character omission is absent.

Research: 07 R-FE-02 §7; 07 R-HUD-03 §10. Code: cmd/nanolathe/briefing.go; cmd/nanolathe/briefing_test.go.

Action: No change.

### B142 — Developer spawn-file lookup formats an unbounded path

**Out of scope · Established.** The developer command formats a caller argument into a fixed local path buffer without a length bound. This is a debug-file lookup overflow, not simulation behavior to reproduce.

Evidence and comparison: Own command export confirms formatted debug path and later file lookup. The unsafe retail debug-file execution helper is not a Nanolathe content or command API.

Research: 01 R-PLAT-01 §9; 07 R-CAM-01 developer commands. Code: docs/DESIGN_INTERFACE_HUD_INPUT.md.

Action: No change.

### B143 — Developer path arrows can inherit an unset colour

**Out of scope · Established.** The debug arrow path masks flags for admission but selects colours using the unmasked value; combined flags can therefore use a previous or unwritten colour. This affects a retail diagnostic overlay.

Evidence and comparison: Own overlay export confirms masked admission and exact-value colour assignment. Nanolathe diagnostic overlays are authored host tools, not the retail debug painter.

Research: 07 R-CAM-01 developer mode; 03 R-COMP-01 §5. Code: cmd/nanolathe/battle_benchmark_diagnostics.go.

Action: none: no gameplay change; writer coverage would settle occurrence of the combined flags.

### B144 — Network spawn records assume a nonzero unit identity

**Out of scope · Established.** A zero unit identity selects a null unit and later accesses it after only a player-side eligibility gate. Whether a valid sender can produce that record is unproved. Nanolathe does not decode this retail spawn packet.

Evidence and comparison: Own spawn-record consumer export confirms zero-identity handling and the subsequent unguarded access. Current multiplayer commands use allocation identity validation.

Research: 08 Lockstep advancement; 04 unit allocation; DESIGN_MULTIPLAYER §3.2. Code: internal/session; docs/DESIGN_MULTIPLAYER.md.

Action: none: a retail spawn packet writer trace is needed to claim ordinary reachability.

### B145 — Fragment geometry exhaustion leaves a claimed retail effect

**Host boundary · Established.** Retail claims an effect and writes its position before finding a geometry slot; failure leaves the claimed entry with stale remainder. This behavior is already documented. Nanolathe paired ownership prevents geometry exhaustion while an effect record remains available.

Evidence and comparison: Own shatter-spawner export confirms operation order. FixedEffectPool fragment admission explicitly checks paired ownership and declines impossible stale admission; existing tests lock pairing and release. No reachable current pool-capacity difference established.

Research: 04 R-COB-04 §3; DESIGN_UNITS_ORDERS_COB fragment ownership. Code: internal/effects/fragment.go; internal/effects/fragment_test.go.

Action: No change.

### B146 — Resource setup writes prefix maxima but caller reachability is unresolved

**Unknown · Unknown.** The helper walks eligible slots in order and immediately installs each running resource maximum, with a minimum of 200. It does not apply the eventual global maximum to earlier slots. No direct caller was found in the independent assembly search, so it cannot yet establish a battle-entry mismatch.

Evidence and comparison: Own complete helper assembly confirms maxima, floors and per-slot stores. Full assembly search found no direct caller; whole independent executable scan found no stored absolute pointer to the helper. Computed references remain unresolved. Current skirmish entry uses the documented per-row resource setup.

Research: 08 R-ENTRY-01 §5; 08 Skirmish configuration. Code: internal/session/skirmish.go.

Action: The owning session Unknown now records the missing indirect/computed caller and option mode. No battle-entry behavior is inferred from the orphaned helper.

### B147 — Rectangle-outline helper has an inconsistent return value

**Host boundary · Established.** The explicit-surface branch leaves a coordinate as its return value whereas the locked-screen branch returns lock success. The drawing calls still occur. No caller using that value to alter output was established.

Evidence and comparison: Own outline export confirms all four edge draws and differing return source. Current UI rectangle painting has no corresponding success-value contract.

Research: 03 R-COMP-02 §5. Code: internal/client/unitdraw.go.

Action: none: caller use of the legacy return would be needed to establish a visual discrepancy.

### B148 — Empty image lists can produce a zero scrollbar divisor

**Host boundary · Established.** One retail image-list sizing branch divides by the total item extent without guarding empty lists; the text-list branch has different guards. Nanolathe bounded list geometry does not reproduce an integer fault.

Evidence and comparison: Own sizing export confirms a zero-initialized divisor is retained for nonpositive item count. Current list/slider service uses explicit empty and extent guards. No valid nonempty-list arithmetic discrepancy established.

Research: 07 R-WGT-01 §4, §5. Code: internal/ui/panel.go; internal/ui/service.go.

Action: No change.

### B149 — Unit-picture enumeration tests an embedded name address

**Agreement · Established.** Both the external entry and our analysis identify the embedded-name address test as always true for a valid definition. It does not reject empty names; the separate unit-eligibility flag still controls admission. The original audit misclassified this agreement as a disagreement. No upstream correction is warranted for this item.

Evidence and comparison: Own enumeration export confirms address rather than first-character comparison and the independent eligibility branch. No Nanolathe parity change follows without a valid unnamed definition and observable caller result.

Research: 07 R-FE-01 unit restriction screen; 02 §4 unit definitions. Code: cmd/nanolathe/retail_menu.go; internal/content.

Action: No change.

### B150 — Both scrollbar orientations assume complete artwork

**Host boundary · Established.** Both vertical and horizontal painters continue to read dimensions after a missing initial frame, and later frames lack guards. This is the same malformed-art assumption as the other scrollbar painter.

Evidence and comparison: Own full scrollbar export confirms both orientations. Current frame resolution guards absent art and extents. No valid-art discrepancy established.

Research: 07 R-WGT-01 §5; DESIGN_PRESENTATION_CLIENT §2.3. Code: cmd/nanolathe/retail_menu_draw.go; cmd/nanolathe/retail_menu_options.go.

Action: No change.

### B151 — Missing or rejected GUI data can reach invalid layer bookkeeping

**Host boundary · Established.** A missing or empty retail GUI skips layer setup but reaches common layer writes; parser failure can likewise free the layer before the common tail. Nanolathe returns load failure and keeps unavailable panels out of the active UI.

Evidence and comparison: Own complete loader export confirms length-zero and parser-failure edges to the common tail. Current GUI loading errors and availability fixtures cover required missing or malformed UI assets.

Research: 02 R-MALF-01; 07 R-WGT-01 GUI loading; DESIGN_INTERFACE_HUD_INPUT unavailable menu actions. Code: internal/gui/load.go; cmd/nanolathe/frontend_availability.go; cmd/nanolathe/frontend_availability_test.go.

Action: No change.

### B152 — Missing associated scrollbar falls back to the header index

**Host boundary · Established.** Retail association lookup returns zero on failure, and some consumers treat it as an ordinary gadget or compare against a different sentinel. Nanolathe associates actual slider/list gadgets and does not reinterpret its window header as a slider.

Evidence and comparison: Own selection-sync export confirms the zero miss and header use; the pointer-service sibling was searched for its mismatched check. Current association loops require matching gadget kinds. No ordinary associated-scrollbar discrepancy established.

Research: 07 R-WGT-01 §4, §5. Code: internal/ui/panel.go; internal/ui/service.go.

Action: No change.

### B153 — Legacy DirectX detection compares the wrong requested component

**Out of scope · Established.** When the first installed-version component differs, the retail check compares it against the second requested component. This is legacy Windows startup capability detection, not an engine behavior used by Nanolathe.

Evidence and comparison: Own detector export confirms the alternate comparison and Windows registry/library paths. Nanolathe does not run this DirectX version detector.

Research: 01 R-PLAT-01; 03 §4.2. Code: internal/platform/ebitenapp.

Action: No change.

### B154 — Polygon filling assumes a positive vertex count

**Host boundary · Established.** With no vertices, the retail extremum search leaves its chosen indices unwritten and later edge walks use them. The cited caller adds one to a stored count; a negative reachable count remains unproved. Nanolathe bounded geometry omits invalid polygons.

Evidence and comparison: Own polygon routine confirms skipped extrema initialization and subsequent indexing. Current raster code works from bounded vertex slices.

Research: 03 R-RAST-01 §1; 03 R-COMP-02 §5. Code: internal/client/model_raster.go.

Action: none: trace the sole caller and count writer to establish a valid-input retail trigger; do not emulate uninitialized geometry.

### B155 — Unused SQSH decoder methods do not provide a decoder

**Host boundary · Established.** Methods zero and three pass the initial method check but invoke no decoder; the final length comparison uses retained input state. Typical chunks fail, but an accidental numeric equality means failure is not logically guaranteed. Nanolathe rejects unsupported methods safely.

Evidence and comparison: Own decoder export and previous independent assembly trace establish dispatch only for methods one and two. Archive/save readers enforce supported methods and output bounds.

Research: fmt hpi, malformed-input behavior; 02 R-MALF-01. Code: vfs/hpi.go; internal/save/compression.go.

Action: none: retain explicit unsupported-method rejection; qualify any wording that says these methods always fail.

### B156 — Unused SQSH encoder methods retain the input value as a length

**Host boundary · Established.** Methods zero and three skip compression and then use retained input state in output-size arithmetic. Ordinary addresses make the output-size test fail, but it is not a universal numeric guarantee. Nanolathe does not expose these broken encoder methods.

Evidence and comparison: Own packer export confirms only two compression dispatch arms and subsequent size arithmetic. Current save encoding selects its supported method.

Research: fmt hpi; 08 Save-file organization. Code: internal/save/writer.go; internal/save/compression.go; vfs/hpi.go.

Action: none: preserve explicit method support and bounded output; do not recreate address-dependent behavior.

### B157 — Legacy allocator failure reaches unchecked tree-node writes

**Host boundary · Established.** If the runtime allocation path returns null without an installed recovery handler, its tree-node users continue writing. This is host out-of-memory failure behavior, not a defined content or simulation result.

Evidence and comparison: Own initialization export and allocator path confirm the conditional null return and unguarded node setup. Go storage and parser limits have no corresponding manual node-allocation contract.

Research: 01 R-PLAT-01 §5, tagged allocation; DESIGN_CONTENT_VFS §2 limits. Code: internal/content; vfs.

Action: No change.

### B158 — Crash reporting uses the wrong file-handle failure test

**Out of scope · Established.** The crash-header builder tests a file handle against zero rather than the file API failure sentinel, allowing failed opens into metadata calls. Subsequent success tests suppress the metadata line. Nanolathe does not reproduce this Windows crash-report helper.

Evidence and comparison: Own crash-header export confirms the handle test, guarded metadata formatting and close. This affects post-failure diagnostics only.

Research: 01 R-PLAT-01 §8, exception filter. Code: cmd/nanolathe.

Action: No change.

### B159 — Skirmish player-count loading preserves the stored value

**Agreement · Established.** Both nominal validation branches store the same supplied count. Only a missing setting supplies the default. Nanolathe session normalization already preserves this retail input rule; its settings UI recovery clamp is a separate host policy.

Evidence and comparison: Independent earlier assembly check confirms identical stores on both branches. Current SkirmishConfig comments and normalization retain raw supplied values; settings recovery is explicitly distinct.

Research: 02 R-KEYS-01 §5; 08 Skirmish configuration. Code: internal/session/skirmish.go; internal/settings.

Action: No change.

### B160 — Save order fallback scans through the descriptor end boundary

**Host boundary · Established.** When a saved order lacks a descriptor name, its legacy ordinal fallback permits an extra descriptor iteration and can carry an invalid result into later name use. This is an order-descriptor fallback, separate from the unit-type-name fallback later in the same loader. Nanolathe validates resolved descriptors.

Evidence and comparison: Own complete constructor export distinguishes its descriptor fallback from later build-unit mapping. retail_restore resolves known names or validated legacy identifiers and returns errors for unknown descriptors.

Research: 08 R-SAVE-02 §8, saved orders. Code: internal/save/battle_image.go; internal/orders/retail_restore.go.

Action: none: retain safe invalid-descriptor rejection; valid legacy mapping remains governed by the existing saved-order contract.

### B161 — Retail player registration copies transport names without bounds

**Out of scope · Established.** The DirectPlay registration path copies returned names into fixed fields without checking their lengths. Transport or UI restrictions on those strings were not established. Nanolathe uses a separate relay/lobby protocol.

Evidence and comparison: Own player-registration export confirms the unbounded copies after transport name retrieval. No corresponding retail name-buffer ABI is implemented.

Research: 08 DirectPlay transport; DESIGN_MULTIPLAYER §3.2. Code: docs/DESIGN_MULTIPLAYER.md.

Action: none: establish sender/UI string bounds only if historical exposure must be assessed.

### B162 — Retail packet validation contains dead rejection and unsafe target lookup

**Out of scope · Established.** One command-range rejection predicate cannot be true; a packet-mode lookup occurs before the later dispatch bound. A separate unit-related command can reach a missing-player dereference after resolving a live unit. These are retail packet-decoder hazards, outside Nanolathe transport compatibility.

Evidence and comparison: Own full handler export and the specific dispatch branches were inspected. Current multiplayer validates its own command schema and allocation identity rather than accepting retail packets.

Research: 08 DirectPlay transport, Lockstep advancement; DESIGN_MULTIPLAYER §3.2. Code: docs/DESIGN_MULTIPLAYER.md; internal/session.

Action: none: retain protocol boundary; sender/caller traces are needed before claiming ordinary retail trigger reachability.

### B163 — Unclosed coloured briefing runs can overread their text

**Host boundary · Established.** The retail inner coloured-run scan stops at its closing marker or a length cap, without checking the text terminator or newline. Nanolathe bounds runs to the actual line and has an existing malformed-run fixture.

Evidence and comparison: Own pager export confirms the bounded write but unbounded-to-input read. TestBriefingLayLineUnterminatedRun explicitly covers marker-only and unfinished runs. No balanced-text discrepancy established.

Research: 07 R-FE-02 §7; 07 R-HUD-03 §10. Code: cmd/nanolathe/briefing.go; cmd/nanolathe/briefing_test.go.

Action: No change.

### B164 — Map helper allocation failures are not propagated

**Host boundary · Established.** Several retail map-helper allocations can return null and are then used by initialization or border loops. This is an out-of-memory host failure; the ordinary grid-building algorithm does not require reproducing it.

Evidence and comparison: Own map-grid initializer export confirms unchecked allocation consumers. Current terrain data uses bounded Go-owned slices and validated content dimensions.

Research: 03 §1 world grids; 02 R-MALF-01; DESIGN_CONTENT_VFS §2 limits. Code: internal/world/terrain.go; internal/world/placement.go.

Action: No change.

### B165 — Save-preview difficulty is unchecked while required-gadget failure is fatal

**Host boundary · Established.** Retail uses the raw saved difficulty to select among three labels without a bounds check. Nanolathe already renders no difficulty label outside the known values. The alleged later missing-RADAR null write is preceded by a process-exiting required-gadget error.

Evidence and comparison: Own preview export and assembly establish the label selection; its gadget lookup was followed to the fatal exit. retailSummaryPanelFields explicitly documents its safe difficulty divergence.

Research: 08 R-SAVE-02 §3; 07 R-FE-02 §5. Code: cmd/nanolathe/loadgame.go; cmd/nanolathe/load_dialog_failure_test.go.

Action: No change.

### B166 — Projectile feature followers trust the anchor definition index

**Host boundary · Established.** The direct feature path checks the catalog bound; the follower path resolves an anchor and accepts its non-sentinel identity without that same bound. Corrupt feature cells can therefore reach invalid retail catalog data. Current feature resolution checks both anchor and catalog identity.

Evidence and comparison: Own collision helper export confirms the asymmetric bounds check. Current FeatureDefAt and feature-anchor handling use bounded definition lookup. No valid stamped-feature difference established.

Research: 06 R-DMG-01 §14; 05 R-FEAT-01; 03 world feature cells. Code: internal/world/terrain.go; internal/world/feature_stamp.go; internal/combat.

Action: No change.

### B167 — Type-three projectile drawing consumes an unwritten angle block

**Unknown · Supported inference.** The type-three branch passes orientation scratch that the inspected dispatcher never initializes. Existing weapon research already records this uncertainty. Current rendering supplies deterministic recorded angles, now explicitly qualified as a retained host fallback. This must not become stack-residue emulation.

Evidence and comparison: Own complete dispatcher export and targeted assembly show no writer for the scratch used by type three, while other model branches supply different initialized angle blocks. The existing research census identifies disintegrator use; BuildProjectileModelPiecesInto uses recorded angles.

Research: 06 R-WFX-01 §4; 03 §5.4, projectile rendering; DESIGN_PRESENTATION_CLIENT C7. Code: internal/render/model.go; internal/render/projectile_view.go.

Action: Corrected research wording and documented the existing deterministic orientation fallback in code and design. Reproducible scratch orientation remains unresolved.

### B168 — Legacy synchronization sender search can run past its vector

**Out of scope · Established.** A synchronization helper continues after a sender search reaches its end and uses that result in selected message arms. A message from an untracked sender is the needed trigger; valid sender admission is not established.

Evidence and comparison: Own synchronization export confirms search termination and unguarded uses. Current relay membership and command validation do not use this retail helper.

Research: 08 DirectPlay transport; DESIGN_MULTIPLAYER §3.2. Code: docs/DESIGN_MULTIPLAYER.md.

Action: none: trace sender admission if historical reachability is required.

### B169 — Debris flag residue is outside all consumed bits

**Agreement · Established.** The explode adapter rebuilds only the meaningful low flag bits and leaves upper bits as residue. Existing research follows the consumers and establishes that they read none of those upper bits. Nanolathe models meaningful flags explicitly.

Evidence and comparison: Own explode-adapter export confirms the read-modify-write construction. The existing consumer census marks the residue inert; current typed debris requests preserve the meaningful flags and random draws.

Research: 04 R-COB-04 §1–§4. Code: internal/effects/debris.go; internal/session/cob_explosion.go.

Action: No change.

### B170 — Short piece-state reads leak a temporary retail allocation

**Host boundary · Established.** After validating the total account size, the retail loader allocates piece-state scratch and returns on a short read without releasing it. This is a failed-I/O allocation leak, not a saved-state rule. Nanolathe stages bounded bytes with Go-owned storage.

Evidence and comparison: Own script-state loader export confirms the allocation, short-read return and release only on successful completion. Current save decoding rejects short data and has no manual scratch lifetime to leak.

Research: 08 R-SAVE-02 §8; 04 COB save/restore. Code: internal/save/battle_image.go; internal/save/bank.go; internal/cob.

Action: No change.


## Verification and limits

Validation includes `tools/check`, `tools/check-retail` (retail fingerprints
and real-device GPU fixtures), `go test ./internal/docs`, and the focused audio,
rendering, help-text, AI, targeting, flight and placement contracts. The complete
gates are run on the integrated candidate and repeated after landing; the final
handoff records their result. The original catalog phase did not change
authoritative gameplay arithmetic. The expanded review did; its changed locks,
diagnostic attribution and simulation measurements are recorded in
[expanded validation](#expanded-validation-in-progress).

Static analysis does not reproduce arbitrary native stack or heap contents.
Malformed-input crashes, stale-memory reads and excluded retail transport
behavior therefore need explicit dispositions, not invented portable outputs.
Approved Modern gameplay, the Modern AI controller, multiplayer session policy
and Enhanced presentation remain governed by their existing design contracts.

### Presentation and performance verification

Sequential baseline/candidate live battle runs used Expanded Confluence, seed 7,
scene version 5, 1920×1080, native scale, 300 lead-in ticks, 60 warm-up draws
and 180 measured draws at 30 draws/s. Both runs used matching metadata and
GOMAXPROCS=2. Every census matched at every measured draw for both renderers;
final captures were byte-identical within each renderer and were visually
inspected. The final census included 320 units, 207 moving units, eight builds,
5,558 features, two burning features, 136 projectiles and 834 effects. Thus the
fixture exercised the intended battle workload; its unchanged capture is not
proof that it exercised the new width-128 exception.

| Renderer | Median draw work, baseline → candidate | p95 draw work, baseline → candidate | Median cadence, baseline → candidate |
| --- | --- | --- | --- |
| Original | 24.060 → 23.837 ms | 29.787 → 28.480 ms | 33.363 → 33.367 ms |
| Enhanced | 12.581 → 13.096 ms | 15.640 → 16.039 ms | 33.333 → 33.333 ms |

These short host measurements show no material regression; they are not
measurements of GPU execution or display scanout. The authored texture fixture
separately exercised the corrected width-128 path: the keyed reference retained
its normal rows, while unkeyed ordinary and diagnostic output showed identical
retail stride mixing. The temporary fixture source was removed after capture.
Benchmark captures, metrics and logs remain outside the repository.
