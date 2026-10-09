# Design — Multiplayer

Battles between several people — on one LAN, by direct connection, or through
a hosted service — together with the replay recorder, spectators and the relay
server they need. Every participating client runs the complete simulation.
Only player commands travel: a relay puts them in one order, tells every
client which tick each one runs on, and decides how far the battle may
advance. The model is **relayed deterministic lockstep**.

**Status: adopted 2026-10-01; M1 verified; M2 implemented; M3 U2 and the first U3 owner group implemented on the multiplayer worktree.** The maintainer accepted
this design and decided its original questions on 2026-10-01, after two
revisions of the 2026-09-30 proposal. Networking, replays and the multiplayer
lobby are in scope (ARCHITECTURE §1), and implementation is authorized in the
order of §16, each milestone behind the one before it. §16.1 records M1's
implementation and verification; its native-platform gate is complete.
§16.2 records M2's schemas, implemented work units and carried gaps.
§16.3 records M3's complete owner writers, capture lifecycle, fault diagnosis
and local cost evidence. Its native-platform comparison remains pending;
recorder/playback is the next implementation milestone (M4).
Per maintainer direction, further multiplayer work stays on the
`multiplayer-m3-u2` worktree branch until play testing, without merging to main.
The original audit measurements were taken on
2026-09-30 and 2026-10-01 against
main `193abfde`. Co-op is the first delivery, not the architecture's limit:
an eventual competitive mode is required. The interoperability target is
Nanolathe-to-Nanolathe; other engines are deferred without a commitment to
support them (§8.5). The four follow-up policies Q22–Q25 were approved on 2026-10-02 (§15).
Later that day the maintainer decided what final removal does to hosted
computers and that it goes through a vote (Q26, Q27, revised Q4 and Q6),
and the online `Give` policy was recorded under Q8. Q28 collects the protocol
and design details this document had proposed on its own; the maintainer
approved them the same day, decided Q29 (a finally removed seat is absent
from the end-condition sweeps, and the perspective its computers borrow keeps
updating) and settled O23 (a Strict resignation deletes the leaver's units
silently, as retail's menu quit does on the other machines).

The first revision added the protocol, identity and snapshot requirements of
an earlier review, the competitive path (§4.7, §12.6) and shared match
settings (§8.4). The second folded in a measured review: it added the effect
pool to the determinism contract (L9), defined the portable arithmetic by
what retail's own routines compute (L1), made pause and speed pacing instead
of world state (§4.4), said what a simulation-free relay can and cannot know
(§12.2), gave casual rooms a desync policy that does not end the battle for
everyone (§9.3), restarted Modern AI controllers instead of serializing them
(§9.1), and put a hosted room-code relay beside LAN (§16).

This document owns the lockstep model and its determinism
contract, the command stream and its wire form, battle identity, the state
digest and desync handling, replays, joining and spectating, the relay and
lobby protocol, and the threat model. It restates none of the mechanisms it
builds on: the tick, the clock and the random streams
([DESIGN_RUNTIME_DETERMINISM](DESIGN_RUNTIME_DETERMINISM.md)); sessions,
skirmish setup, results and saves
([DESIGN_SESSIONS_AI_SAVE](DESIGN_SESSIONS_AI_SAVE.md)); rule selection
([DESIGN_GAMEPLAY_RULES](DESIGN_GAMEPLAY_RULES.md)); the input queue and the
paused-input boundary
([DESIGN_INTERFACE_HUD_INPUT §3.12](DESIGN_INTERFACE_HUD_INPUT.md));
mutators ([DESIGN_MODS_MUTATORS §6](DESIGN_MODS_MUTATORS.md)); and
Survival ([DESIGN_SURVIVAL](DESIGN_SURVIVAL.md)).

## 1. Purpose and boundary

**In scope.**

1. Battles of two to ten seats, any mix of humans and computer players
   (Classic or Modern), in skirmish and Survival, under whichever gameplay
   mode and mutators the lobby selects.
2. LAN and direct-IP play through a relay embedded in the hosting client,
   and online play through a hosted relay, `cmd/nanolathe-server`.
3. Spectators.
4. Replays of single-player and multiplayer battles. Replays are the first
   player-facing deliverable (§16): they exercise the whole command layer and the
   determinism contract without a network.
5. Rejoining a battle after a disconnect.
6. Eventual competitive play, with explicit fairness, command authorization,
   result authority and spectator policies. Ranked admission has its own
   release gate (§16); a completed co-op milestone is not that gate.
7. Matching content, mods and effective settings across all Nanolathe players,
   including agreed restrictions on views that affect play, such as disabling
   tactical zoom-out for the match (§8.4; maintainer direction 2026-10-01).

**Out of scope, deliberately.**

- Interoperating with retail `TotalA.exe`, TA Forever, or DirectPlay, and
  the retail packet formats (§3.2 says why).
- Retail-format multiplayer saves `[08 "Multiplayer saves"]`, and saving a
  multiplayer battle at all in the first versions (§11.5).
- Co-op campaign missions.
- Server-side fog in the first releases. A server that simulates and sends
  only permitted state is a possible later architecture (§12.6), requiring
  a state protocol and a different client driver. Lockstep clients receive
  hidden state; a trusted referee alone cannot prevent map hacks (§13).
- Rollback, client-side prediction, and client anti-cheat software.

## 2. The design in one page

```
  client A ──commands──▶ ┌──────────────────────┐ ──stream──▶ client A
  client B ──commands──▶ │ relay                │ ──stream──▶ client B
  client C ──commands──▶ │ orders, stamps, paces│ ──stream──▶ client C
                         └──────────────────────┘ ──stream──▶ spectators, replay file
```

- **Same inputs, same battle.** Every client runs the whole battle under the
  same simulation contract, the same content, the same configuration and seed
  pair and the same command stream, and therefore holds the same state after
  every tick (§5). Initially only tested builds of one source release are
  admitted; build provenance and simulation compatibility are separate (§8).
- **One stream.** A client sends its player's commands to the relay. The relay
  stamps each with the sending seat and the tick it executes on, appends it to
  the battle's single **stream**, and broadcasts the stream. It also
  **grants** ticks: no client simulates past the last granted tick, so the
  relay is the battle's clock. Pause and game speed are relay pacing: they
  decide when ticks run and never what a tick computes (§4.4). The first
  releases offer neither online (§15 Q5).
- **Commands only.** Units, COB scripts, projectiles and effects are all
  computed on each client. Traffic follows commands and their actor lists,
  not periodic updates for every world object. Ordering a large selection
  still sends many unit references; server fan-out and spectators multiply
  that traffic. §17 measures the supported workload before sizing servers.
- **Checked, not trusted.** Every client hashes its canonical state every
  30 ticks, once a second at normal speed, and the relay compares (§9); a
  cheaper per-tick record locates a mismatch afterwards. Every client
  validates every command inside its own simulation (§7.2), including its
  permissions.
  Hashes detect disagreements; they do not prove that clients are honest or
  that an identically accepted command was authorized (§13).
- **Content-free server.** The relay runs no simulation and never touches
  Cavedog assets (§12). The same relay code runs inside the hosting client
  for LAN and direct-IP play, so multiplayer never depends on a service
  staying up.
- **Replays use the same inputs.** A replay records configuration, seeds and
  stream (§10). Complete recording and deterministic playback are explicit
  deliverables, including single-player pump boundaries.
- **One simulation on every host.** A window, a headless replay and a test
  process compute the same battle from the same inputs. Nothing a host, its
  renderer or its audio device supplies may decide a draw, a pool slot or a
  lifetime (§5.3 L9).
- **Interface state stays local.** Selection, camera, build-menu browsing and
  the other interface state never enter the stream (§7.3).

## 3. Retail baseline

### 3.1 What retail does

The research describes retail multiplayer closely. In summary, every point
**Established** unless marked:

- **Each machine owns its players.** A player record's control byte is 1 for
  the local human, 2 for a computer player and 3 for a remote peer
  `[05 R-SHARE-01 §1]`. A machine runs the full per-unit work only for owners of
  control 1 or 2, and after each such player's units it emits the unit-state
  synchronization bitstream, packet kind 44 `[04 R-MOV-03 §1]`
  `[08 "Unit-sync ownership and body structure"]`. A remote peer's units are
  driven by that stream.
- **No orders travel.** Each machine enters and executes its own players'
  orders and sends their results `[08 "Command canonicalization"]`.
- **Live-battle receivers trust seated peers.** After the session-state and
  seated-peer admission gate, live-battle receive arms do not check that a named unit or player belongs
  to the sender, or recheck host, watcher, alliance or cheat permissions
  `[08 "Packet framing and dispatch"]`.
- **The shooter's machine resolves hits.** A projectile whose shooter belongs
  to an occupied remote-peer row applies no damage locally; the shooter's
  machine resolves the hit and sends the damage `[06 R-DMG-01 §9]`.
- **Economy is overwritten, not compared.** Each participant periodically
  sends its stocks, capacities and totals, and receivers copy them
  `[08 "Economy and integrity checks"]`.
- **Peers do not agree on randomness.** Battle entry reseeds the simulation
  stream from the machine's own performance counter and the CRT stream from
  its own clock; no packet carries a seed `[08 "Lockstep advancement"]`
  `[01 §7.1]`.
- **No input barrier.** No packet carries a per-tick input bundle and no fixed
  input delay exists. A machine that runs 900 or more ticks ahead of the
  slowest remote peer throttles its own tick production. The progress value
  is the tick that peer states in its unit-state records
  `[08 "Soft pacing — no per-tick input barrier"]`.
- **No state check or repair.** No comprehensive world hash, no rollback and
  no snapshot resynchronization exist `[08 "Full-world hash"]`
  `[08 "No rollback or world snapshot resync"]`.
- **Peer-to-peer over DirectPlay** `[08 "DirectPlay transport"]`.
- **Unit identities follow transport order.** Multiplayer allocates each
  participant's block of unit identities in ascending transport-identity
  order, watchers included `[08 "Unit-sync ownership and body structure"]`.

### 3.2 Why Nanolathe does not reproduce it

1. **There is no exact retail multiplayer outcome to match.** Each retail
   machine ran its own random streams and saw other players' units through a
   lossy state stream, so the machines' worlds differed and a battle's outcome
   was assembled from several partial simulations. A bit-exact clone would
   reproduce that divergence, not a single answer.
2. **It is a second simulation mode.** Units driven by packets instead of by
   their own movers, pumps and scripts are exactly what the research marks as
   multiplayer transport outside Nanolathe's scope `[04 R-MOV-03 §1]`; the
   single-player boundary keeps only the packets the local path still
   constructs `[08 R-OOS-01 §1]`.
3. **It trusts admitted clients with other players' state too.** Live-battle receivers
   apply payload identities without ownership checks, including removal and
   player-record changes. Nothing compares the results
   `[08 "Packet framing and dispatch"]`.
4. **It needs every machine to reach every other.** A full mesh through home
   routers is what made retail hard to play online, and it exposes every
   player's address to every other.
5. **It buys no cross-play.** TA Forever admits only clients whose executable
   and DLL hashes match, so a compatible Nanolathe still could not join it.

### 3.3 What Nanolathe keeps

The **player-visible** multiplayer rules — the lobby options and what they
do, alliances and sharing, how a battle ends — are rules, not transport. A
Strict 3.1 online battle keeps the established baseline except the explicit
online policies in §15. Unsettled retail behavior remains a `TODO(question)`;
open product decisions authorize no departure. Established unless marked:

| Rule | Retail | Lockstep form |
|---|---|---|
| Host options | At battle entry every peer copies the host's commander-death rule, cheat gate, mapping and line-of-sight bits and unit limit `[08 R-ENTRY-01 §2]`. Two further host options are not copied there: *watching allowed*, off in a new game, gates watcher joins, the side control's watcher choice and the defeat watch offer; *game closed* gates joins and added computer players `[08 R-SKIR-01 §12]`. No command-line word presets a host option; an online service's configuration file can preset all of them but *game closed* `[01 R-PLAT-01 §2]` `[08 R-SKIR-01 §7]`. | Part of the agreed configuration (§8.1), watching allowed included. Game closed is lobby admission and stays with the room (§12.4). |
| Commander death | *Game continues*, *game ends* (the commander's death sweeps its owner's remaining units through ordinary self-damage) or *deathmatch* (the same sweep, then a respawn after elimination; never ends by elimination) `[08 R-SKIR-01 §3]`. | `SkirmishConfig.CommanderDeath`, already modelled; the sweep runs for every seat (§6). |
| Mapping, line of sight | Mapped or unmapped; permanent, circular or true `[03 R-VIS-01 §1]`. | Configuration, already modelled. |
| Unit limit | The battleroom's maximum, an equal slice per player `[08 R-SKIR-01 §6]` `[05 R-SHARE-01 §7]`. | Configuration, already modelled. |
| Starting resources | Every player's storage bonus is the host's value times 100, and at least 200 `[08 R-ENTRY-01 §5]`. | Per-row metal and energy, already modelled. |
| Unit restrictions | Multiplayer only: a per-unit count of 0–100 or none, edited by the host. At lobby exit a count of 0 clears the definition's creatable bit and the battle-entry compile removes it; a positive count survives as each player's cap, enforced only by the allocator at creation; no planner reads either. A lobby seeds `wacky` definitions at 0 — removed once the host closes the screen, kept and refused at every creation if it never does; `norestrict` definitions are never offered `[08 R-SKIR-01 §10]` `[05 R-SHARE-01 §8]` `[05 R-SHARE-01 §9]` `[05 R-SHARE-01 §10]`. | Modelled for single-player skirmish and Survival in every mode as `content.Restrictions` on the battle's catalog clone, nothing seeded ([DESIGN_MODS_MUTATORS §15](DESIGN_MODS_MUTATORS.md#15-unit-restrictions)). Online it is field 12 (§8.6), carried from the host since the first online lobby (§16.6). A restriction lobby screen follows later (Q16) and must decide retail's seeded-but-never-closed `wacky` state; no field value expresses it. |
| Unit roster | Peers keep only units every peer selected and holds compatibly `[08 "Unit-data negotiation and catalog retention"]`. | Identical catalogs are required instead (§8.2); negotiating a common roster is §15 Q15. |
| Cheats | The typing machine checks the entry-time cheat permission, but the ungated developer phrase unlocks its cheat and developer commands regardless of the host switch. Commands execute only there; received lines are chat text, never dispatched `[07 R-CAM-01 §6]` `[08 "Lockstep advancement"]`. | Agreed cheat permission enforced by every receiving simulation; neither the developer phrase nor a local developer flag expands the allowed stream commands (§7.1, §7.2). |
| Alliances | Row A holds declarations and row B mirrors declarations toward the seat. A seated non-eliminated, non-watching human can toggle a declaration toward a seated non-eliminated, non-watching remote human outside its team; its own row changes at once and only the target machine is told. Third machines keep stale copies. Computers have no toggle: battleroom teams set mutual rows on every machine and remain locked in battle. Consumers read row A one-sidedly; victory additionally tests mutuality and the survivor's row toward other seats `[05 R-SHARE-01 §1]` `[07 R-FE-01 §7]`. | Declarations are seat commands. Q22 adopts one shared directed matrix, with B its transpose; all replicas apply each declaration on its assigned tick (§6.7). |
| Allied sight | Current sight is per player and never merged `[03 §3.2]`. Two opt-in shares exist, off at the start and toggled in battle by chat commands. *Share mapping* copies the sharer's explored tiles, every 450 ticks, into each surviving remote human it has declared alliance toward — or once, from the share screen, toward anyone — and under Permanent line of sight an explored tile is also a visible one `[05 R-SHARE-01 §6]`. *Share radar* marks the sharer's own units as friendly contacts for viewers it has declared alliance toward, and passes on none of its detections `[03 R-VIS-01 §7]`. | Retail share eligibility (§6.3), with the approved history scope and request-tick application of Q24/Q25 (§6.7). Shared allied vision, as Survival already does for its team, would be a separate Modern policy. |
| Automatic sharing | Every 60 ticks, for the local player only, surplus above a threshold flows to an allied, surviving remote human with less stock; computer players never receive it `[05 R-SHARE-01 §3]`. | Runs for every human seat, slots ascending (§6). |
| Giving | The share screen gives resources (no alliance test) and units other than airborne, transported and commander units `[05 R-SHARE-01 §5]`. A transferred unit is always replaced by a fresh record without its kills, orders or groups; for a remote new owner the sender only kills its copy and sends the transfer, the receiver creates the replacement, and the stockpile bytes travel under a different gate `[05 R-WORK-01 §15]`. | Seat commands; a transfer between seats of different machines takes the remote branch's copy rules (§6). |
| Pause and speed | The pause key is available to watchers too; speed keys and slider refuse watchers. Receivers apply pause and speed from any admitted peer. Speed is 1–20; unpause gives no catch-up burst `[08 "Lockstep advancement"]` `[07 R-CAM-01 §2]` `[01 §4.3]`. Entry is unpaused at speed 10 `[08 R-ENTRY-01 §2]` `[08 R-ENTRY-01 §3]`. | Normal speed and no pause in the first releases (§15 Q5). Any later support uses relay pacing (§4.4). |
| Ending | Each machine judges its own human every 30 ticks; authored triggers are not checked online. Defeat requires no live units. Each surviving opponent must have shared victory enabled on both sides, mutual alliance, and its own row covering every seated non-eliminated seat, including itself and watchers; an opponent that has created nothing prevents victory. Won and lost paths share a countdown that false checks do not reset `[08 R-SKIR-01 §3]` `[08 R-TRIG-01 §6]`. Outside Deathmatch, the no-live-human site `[08 R-SESS-01 §1]` runs each tick and ends the battle on its sixth consecutive true tick from an unarmed countdown `[08 R-TRIG-01 §6]`. | Per human seat (§6). Shared victory reads the common directed matrix under Q22; the other victory requirements remain (§6.7). |
| Defeated players | Where the host allows watching, a defeated player becomes a watcher at its defeat latch and is then asked "You're out!  Continue Watching?": *Yes* changes nothing further and *No* leaves `[07 R-FE-01 §9]`. Where it does not, the player takes the ordinary lost ending, as in skirmish. One hosting live computer players is put in watch mode without the question, whatever the option says, because departure removes their machine group and destroys its units `[08 R-SKIR-01 §3]` `[08 R-LEAVE-01 §9]`. | Per seat (§6.6, §11.1). |
| Watchers | A watcher holds a seat with no commander — its placement draws are still taken — sees everything, and is left out of scores and elimination `[08 R-ENTRY-01 §5]`. Joining as one needs the host's *watching allowed* option; the host can switch it off in battle, which removes the remote watchers `[08 R-SKIR-01 §12]` `[07 R-HUD-04 §1]`. | Observer rows in the configuration (§11.4). |
| Resign, host loss, timeout | Resign tears down only the leaving machine. In a launched battle host loss migrates the host flag and does not end play `[08 R-LEAVE-01 §7]` `[08 R-LEAVE-01 §8]`. Timeout opens a dialog; REJECT or timeout plus 120 seconds removes one silent machine, and when the silent slots span two or more machines the dialog closes, so the automatic path never removes them `[08 R-LEAVE-01 §6]`. The host can also reject a player through its control window `[08 R-LEAVE-01 §1]`. Removal destroys units and empties the seat without transferring stocks, scores or its unit-limit slice `[08 R-LEAVE-01 §3]` `[08 R-LEAVE-01 §4]` `[08 R-LEAVE-01 §5]`; removing a human removes every slot of its machine, its hosted computers included, in ascending slot order `[08 R-LEAVE-01 §9]`. | Rejoin and final removal are distinct. A seat out of play stays idle until a removal vote passes; no timer removes it (§11.1, Q27). Strict destroys a voted-out seat's units as retail's peer removal does and removes its hosted computers with it; a Strict resignation deletes the leaver's units silently, as the other machines do for retail's menu quit (§19 O23); Modern/Community retain the units idle and keep the computers running (§11.1, Q6, Q26). |
| Computer players | Each not-ready human may add one computer on its machine, when the game is open and not Deathmatch; the host's first click on an open row blocks it, and the next reopens it and enters the add path. The creator controls side/team and readiness; teams provide computer alliances. Difficulty is unsynchronized hosting-machine state `[08 R-SKIR-01 §13]` `[08 R-AI-01 §21]`. | Every replica runs each computer, with its creator's fixed perspective (§6.6). Strict permits one computer per human and removes it with its human's final removal; Modern/Community permit multiple within available seats and keep them running (Q26). Difficulty is explicit per computer; computers are excluded from Deathmatch initially (Q23, §6.6). |
| Saving | Disabled in multiplayer `[08 R-SAVE-02 §4]` `[08 "Multiplayer saves"]`. | Not offered (§11.5). |
| Replays | Retail has none `[08 "Replay"]` `[08 "Bounded absence"]`; the community recorder's files are `[fmt tad]`. | A Nanolathe feature, not a rule (§10). |
| Chat | All, allies, enemies or chosen players; recipients may include watchers. A watcher cannot open ordinary battle chat or type commands; the timeout dialog can send text without command dispatch `[07 §5]` `[07 R-CAM-01 §6]`. | Relay messages with sender-resolved recipients (§12.2); non-authoritative replay entries. Enforce the watcher role restriction. |
| Unit identities | Allocated in transport-identity order `[08 "Unit-sync ownership and body structure"]`. | Seat order, as in single player: there is no transport identity. |

## 4. The lockstep model

### 4.1 Terms

| Term | Meaning |
|---|---|
| Tick | One authoritative sub-tick, 30 per second at normal speed `[01 §4.4]`. |
| Seat | A player slot 0–9. A human seat belongs to one connection; a computer seat to the battle. |
| Stream | The battle's ordered list of entries, identical for every client, spectator and replay. |
| Entry | One command, session event, grant, note or pump end with a monotonically increasing stream position. Commands and session events are bound to a tick. |
| Grant | The relay's statement that the commands and session events for ticks through T are complete and those ticks may run. A client never runs an ungranted tick. |
| Pump | One host batch of sub-ticks followed by the executor tail `[01 R-PLAT-02 §7]`. |
| Checkpoint | The state after a completed tick and its executor tail; the tick number identifies it (§9.1). |
| Digest | A hash of the canonical authoritative state at a checkpoint (§9). |

### 4.2 The stream

The relay owns the stream. Every entry has a battle identity and stream
position. It holds five kinds of entry:

1. **Command** — `{tick, seat, clientSequence, payload}`. A seat's gameplay command (§7.1).
   The relay sets `seat` from the connection; a client cannot claim another
   seat.
2. **Session event** — `{tick, kind, seat}`: a seat resigning, a seat's
   final removal after a passed removal vote (§11.1), and the controller
   restart that accompanies a snapshot rejoin (§9.1). A resignation is the
   seat's own request; the relay authors the others. Each is bound to a
   tick, as a command is. The removal vote itself is relay and lobby
   traffic (§12.2): its call, ballots and tally never enter the stream, and
   the final-removal event is the only thing of it a simulation sees.
3. **Grant** — `{through, sealedPosition}`: the prefix of commands and
   session events for ticks up to `through` is sealed and those ticks may
   run. No later entry may be assigned to a sealed tick.
4. **Note** — `{kind, argument}`: a pacing decision — pause, resume or
   game speed (§4.4) — or a membership change — a seat entering the
   reconnectable-drop state or completing its rejoin (§11.1, §11.2). A note
   records the change for the HUD, the relay's participant set and replays,
   and changes no simulation state: a dropped seat simply issues nothing
   while no connection is active in it, and its rejoin asks nothing of the
   simulation. Single-player recordings carry pacing notes; online battles
   carry no pacing note until §15 Q5 is revisited.
5. **Pump end** — in single-player replays only, the tick after which the
   host's executor tail ran (§4.5).

**Casual tick assignment.** On receiving a command, or authoring a session
event, the relay assigns it the earliest tick no client can yet have run,
`lastGrant + 1`, and appends it. Entries bound to one tick keep the relay's
arrival order; one seat's commands keep their sending order. The stream is append-only. Acceptance acknowledges the
client sequence and assigned stream position; reconnects retain this identity
so a retransmitted command cannot execute twice (§11.2). A competitive
scheduler may assign a later tick under the room's declared policy (§4.7).

**Client sequence (§15 Q28, approved 2026-10-02).** These are Nanolathe protocol rules.
They replace the earlier retransmission contract, under which a repeated
sequence returned its original receipt and a conflicting payload under an
accepted sequence was refused. Each human seat numbers its commands with an
unsigned 64-bit client sequence that belongs to the seat for the whole
battle, not to one connection. The seat's first command carries 1, and every
later command carries the previous value plus one. The relay accepts a
command only when its sequence is exactly one more than the seat's last
accepted value:

- a sequence at or below the last accepted value is a duplicate — a
  retransmission — and is dropped silently: it makes no stream entry and no
  second receipt, and its payload is not compared, so whatever was accepted
  under that number stands;
- a sequence above the next expected value is a gap: the relay refuses that
  command with the expected value (§12.2 "Refused"), stamps nothing, and
  accepts nothing more from the seat until the expected value arrives;
- on resume the relay tells the client the last value it accepted, and the
  client resends from the next value under the original numbers (§11.2).

One seat's commands therefore enter the stream in the order they were
numbered and can never be reordered, skipped or applied twice. The relay
never buffers a command that arrives ahead of its turn.

**Relay-authored entries.** The relay supplies some of the battle's inputs
itself: the seed pair (§8.3), every tick assignment, the final-removal
events, and the controller restarts that accompany a snapshot rejoin (§9.1);
it also writes the membership notes, which change nothing in a simulation.
Each input is agreed: it is carried in the start message or in the stream,
every client applies it identically at its stated tick, and a replay
records it. None is a value a tick computes from host-local
state. The rule that nothing a relay, a lobby or a host supplies may change
what a tick computes concerns that second kind: a tick's arithmetic, draws,
pool occupancy and lifetimes depend only on the configuration, the seeds
and the stream entries bound to ticks up to it — never on when or how a
host received them, its wall clock, its pacing, its renderer or its local
preferences. A relay that authors a different entry produces a different
stream, which every client then runs alike, exactly as a different player
command would be.

**Application.** A client applies the commands and session events bound to
tick T in phase 1 of tick T, in stream order, through the existing typed
command path that drains the input queue today
([DESIGN_RUNTIME_DETERMINISM §2.5](DESIGN_RUNTIME_DETERMINISM.md), phase 1).
Nothing else in the stream changes simulation state, so there is one
application point and one order. An earlier draft gave pause, resume, speed,
resignation and drop a second application stage between ticks. It is
withdrawn: the first three are not world state (§4.4), a drop changes no
simulation state (a note, above), and a resignation needs nothing a
tick-bound entry lacks.

In a lockstep battle **every command and session event received while paused
waits for phase 1 of the next granted tick**. The client may preview accepted
orders, but does not apply any to authoritative state early. A resignation
or final removal made during a pause is bound to that same tick at its
stream position: the seat's entries ahead of it apply, and those behind it
are refused, identically by every simulation. A drop made during a pause is
a note and refuses nothing; its seat simply sends nothing until it rejoins,
and a rejoined seat's later commands apply as any other's. Local network
buffers never enter the state digest.

The single-player session keeps its queue: a local source assigns `DueTick`
as it does today and records the same entries, so a single-player battle and
a lockstep battle run one code path from phase 1 inward. Single-player also
keeps its existing paused-input boundary
([DESIGN_INTERFACE_HUD_INPUT §3.12](DESIGN_INTERFACE_HUD_INPUT.md)). That
boundary applies the queue's prefix through phase 1's own path, with the
number of the tick that has not yet run (`Session.stepPausedInput`), so its
authoritative result is the one phase 1 of that tick would have produced.
The recorder writes those commands at that tick and playback applies them in
phase 1, once; no separate record of the early application is needed. The
online rule above reaches the same state without applying anything early,
and is part of Q2's session contract.

### 4.3 Latency and playout

A seat's command latency is its round trip to the relay plus its **playout
buffer**: the granted ticks the client holds in reserve so that network
jitter does not stall it. The client runs ticks to keep that reserve near a
target, speeding up slightly when the reserve grows and stalling when it
empties — a jitter buffer with adaptive playout. Each seat pays only its own
latency; nobody waits for the slowest connection except when the relay
must slow down for them (§4.4). With a 60 ms round trip and a two-tick
reserve a command takes effect about 130 ms after the click (illustrative;
§16 measures it).

Real-time strategy hides that delay with immediate local feedback, and so
does this design: the acknowledgement sound, the move marker and the queued
build outline play when the click is sent, before the order runs. The
session already exposes commands that are queued but not yet applied to
presentation (`Session.PendingHumanCommands`), which is the hook.

Full-world prediction and rollback are not first-release requirements. Local
feedback and a retained pending-command view still hide some input delay;
the window may retain its existing presentation interpolation. Running the
whole simulation does not by itself remove the round trip or the need to
measure responsiveness.

### 4.4 Pacing, pause and speed

**Pacing.** The relay grants ticks at the battle's speed in real time. Each
client reports the last tick it ran; when a client falls behind its grants by
more than a bound, the relay slows its grants for everyone, and after a
longer bound it shows the waiting-for-player state. A seat whose connection
is lost enters the reconnectable-drop state, the grants go on without it,
and once it has been out of play for the agreed cumulative grace the other
seats may vote on its final removal (§11.1); no vote removes an active seat. M6's pacing state
table (§19 O17) must set a bound after which a connected seat that has made
no progress is moved into the drop state, so that it becomes votable after
the grace without any vote-kick (§15 Q28; the bound's value
belongs to that table). Pacing is host policy: it decides *when* ticks run,
never *what* they compute. A battle therefore runs no faster than its slowest
participant can simulate, and the seat and unit limits a lobby offers are
advertised only after §17 measures them on the weakest supported hardware.

**Pause and speed are pacing, not world state.** No phase of the tick reads
the pause bit or either speed word. Their only readers today are the host
loop, the HUD's publication and the single-player paused-input boundary (a
type-resolved census of every use, 2026-10-01). In a lockstep battle the
relay therefore owns them outright. To pause, it seals its last grant,
appends a Pause note and issues no further grants until Resume; clients
finish the ticks already granted and then stand still. A speed change alters
the rate of grants. The relay never retracts a grant. The notes travel in
the stream so that every HUD and every replay shows who paused and when, but
no client's simulation state depends on having received one, and no
checkpoint needs to say whether the battle was paused. Commands and session
events received during a pause are bound to the next tick the relay grants
(§4.2), and their receipts keep travelling while it lasts.

**First releases: neither is offered online** (§15 Q5). An online battle
runs at normal speed — retail's speed 10, thirty ticks a second — and the
relay accepts no pause or speed request. Its grants still slow for a
participant that falls behind and stop while a room waits for one; that is
pacing for a machine, not a pause a player controls. The paragraph above is
the mechanism for when the decision is revisited, and single-player replays
use pacing notes from the start.

Who may pause and change speed, and how often, would be a new policy
decision. Retail permits the pause key even for watchers, refuses watcher
speed changes, and applies either from any admitted peer without a receiver
authority check `[08 "Lockstep advancement"]`. Q5 remains unchanged.
Whatever the policy, the relay can apply only rules expressed in facts it
holds (§12.2).

**Host pacing state stays out of the battle.** The clock's anchor, delta,
fractional carry, pause bit and requested and active speeds describe a host's
timing, not the world; all of them are excluded from the digest (§9.1), in
every session kind. So is the load hysteresis that lowers the active speed
when a host cannot keep up `[01 §4.3]`: in a lockstep battle the relay's
pacing replaces it. Today pause and speed are host calls
(`Session.SetPaused`, `Session.AdjustSpeed`), and more than the pause key
reaches them: opening the battle options menu pauses and closing it resumes,
a debug capture pauses, and battle entry copies the machine's preferred game
speed into the clock (`cmd/nanolathe/battle_menu.go`, `battle_settings.go`).
In a lockstep battle none of these touches the session. Each becomes a
pacing request to the relay or stays local to the client, and the HUD reads
pause and speed from the lockstep driver. The retail multiplayer clock path
— `AdvanceMP` and its lag throttle, implemented and unreachable
([DESIGN_RUNTIME_DETERMINISM §3.4](DESIGN_RUNTIME_DETERMINISM.md)) — paced
retail's transport, which this design replaces; it stays unused.

### 4.5 Pumps are part of the stream

The executor tail runs once per host pump, after the whole catch-up batch,
using the batch's last tick `[01 R-PLAT-02 §7]`, and its last step expires
temporary sight `[03 R-COMP-02 §2]` `[01 R-PLAT-02 §5]`
(`Session.runRetailPostLoopTail` in `internal/session/post_loop.go`).
Temporary sight is visibility state, so the tick at which it expires depends
on how many ticks the host ran per pump: a client at 144 frames per second
and one catching up five ticks per pump expire it at different ticks, and the
battles diverge. Retail's own behaviour here varies with the machine's frame
pacing.

Lockstep therefore fixes pump boundaries in the stream: **in a lockstep
battle every granted tick is its own pump**, and the tail runs after every
tick. A one-tick pump is an ordinary retail pump, the shape a fast machine
produces at normal speed. A single-player replay records the host's actual
pump boundaries as pump-end entries, so it reproduces a battle exactly
however its frames were paced (§10). The executor tail of a pump that runs no
tick changes no authoritative state — the barriers are empty, the message ring is
presentation, and a second expiry pass at an unchanged tick removes nothing —
so such pumps need no record.

The clock needs none either. `clock.AdvanceSP` moves its anchor, carry and
speed hysteresis on every unpaused pump, zero-tick pumps included, but those
fields are host pacing in every session kind (§4.4): no tick reads them, the
digest excludes them (§9.1), and playback steps the recorded pumps directly
instead of deriving them from a clock. A replay keeps the original pause and
speed changes as pacing notes, for a viewer who wants the battle at the pace
it was played, and they affect nothing else.

### 4.6 Alternatives considered

| Model | Decision and tradeoff |
|---|---|
| Retail's owner-authoritative peer-to-peer | §3.2. |
| Peer-to-peer lockstep | Direct connectivity and distributed membership/order decisions add complexity. A relay simplifies those responsibilities, at the cost of an extra route, hosting, and a single service failure point. |
| Relay plus a trusted replayer or referee | The planned competitive extension (§12.6). A replayer certifies results after the match; a live referee adds a trusted checkpoint source during it. Neither changes the clients' command stream, both need content and simulation capacity where they run, and neither hides enemy state. |
| Server simulates, clients receive permitted state | Preserved as a future option (§12.6). Can withhold hidden information and admit clients that do not reproduce the simulation. Requires a state/event protocol, a different presentation driver, more bandwidth and a simulation service. A transport swap cannot provide it. |
| Rollback | Re-simulates several ticks per frame to hide input delay; real-time strategy tolerates the delay (§4.3), and the cost grows with army size. |

Relayed lockstep is also the model in service elsewhere. Recoil sends only
player inputs, and Factorio makes its server the sole authority for the tick
an input runs on
([Recoil netcode overview](https://recoilengine.org/articles/netcode-overview/),
[Factorio FFF-147](https://factorio.com/blog/post/fff-147)). Two limits
Recoil states for itself are things this design attempts anyway and must
therefore prove: it supports only x86-64 because of hardware floating-point
differences (L1), and it has no save-based mid-game join because a restored
save desyncs (§11.3). Planetary Annihilation is the precedent for the
state-delivery row: its server simulates and sends time-indexed state to
clients that render what they are given
([ChronoCam](https://forrestthewoods.com/the-tech-of-planetary-annihilation-chronocam/)).
These are **Established external descriptions**, not retail evidence.

### 4.7 Competitive scheduling and admission

Competitive play is an eventual requirement. The casual arrival scheduler
is not a fairness guarantee: a nearby player, especially an embedded host,
can see an event and return a command sooner than a distant player. Different
playout reserves also change how old each player's view is. The comparable
tradeoff is described by [Factorio's multiplayer design](https://factorio.com/blog/post/fff-147).

Keep tick assignment behind the relay's scheduling boundary. Before ranked
admission, select and test a declared policy for input lead, playout limits,
late commands, speed, pause and disconnects. A shared input horizon with
server-enforced deadlines is a candidate; assigning every arrival a fixed
extra delay alone does not equalize different network delays. The policy
must not trust a client's claimed issue tick to grant retrospective inputs,
let the embedded host bypass the stream, or let one player manipulate it by
withholding progress. Latency cannot be made irrelevant; supported latency
and CPU ranges and the accepted residual advantage must be published.

The room records its scheduling and authorization policy separately from
its gameplay rule set. Those policies govern command admission and timing;
they do not select new movement, combat or economy rules. Any such departure
uses the existing `session.RuleSet` seams and owning design contract. Rated
rooms require authenticated identities, results verified by a trusted
replayer or referee (§12.6), cheats disabled, explicit target-visibility
admission (§7.2), spectator and replay embargoes, and a fixed policy for
pauses, drops and adjudication.
The first co-op release need not implement ratings, but its envelopes, seat
identity and application boundaries must accommodate these later policies.

## 5. Lockstep identity

### 5.1 The contract

Two admitted Nanolathe clients that start from the same simulation build,
content identity, battle configuration and seed pair, and apply the same
stream, hold equivalent authoritative state after every tick. Its canonical
representation is identical at the checkpoint boundary (§9.1). A Modern
controller's private memory lies outside that representation; every command
it has had applied lies inside it. Nothing else may matter:

- which seat is local, the viewing slot, or whether the client is a spectator;
- the camera, the renderer and its options, audio, window size and frame rate;
- whether the host has a window, an art cache or an audio device at all (L9);
- the host operating system and processor architecture;
- host pump sizes, catch-up, and how long a frame takes; recorded
  single-player pump ends remain inputs (§4.5);
- pause and game speed (§4.4);
- goroutine scheduling, including how long the Modern AI thinks.

### 5.2 What already holds

- **Ordering.** No map iteration or scheduling dependence in authoritative
  packages, enforced by source-inspecting guards [I1].
- **Arithmetic.** Fixed point for world state, an exhaustive floating-point
  allowlist, and no fused multiply-add in authoritative packages, or in the
  load-time packages whose output the simulation reads, on arm64 or
  `GOAMD64=v3`, enforced by `TestAuthoritativeArithmeticIsNotFused` [I2].
- **Random streams.** Two session-owned streams seeded from an explicit seed
  pair; presentation draws only private copies [I4] (DET-01 in
  DESIGN_RUNTIME_DETERMINISM §3.3).
- **Presentation boundary.** The simulation reads no wall clock, input state,
  camera or renderer state [I6], with the one exception L9 records.
- **One input path.** Typed `HumanCommand` values carrying handles and values
  only, sequence-ordered, each with a `DueTick`, drained by one consumer in
  phase 1 (`internal/session/commands.go`).
- **Computer players.** The Modern AI thinks on a background goroutine but its
  commands are applied at a fixed reaction deadline, and its private generator
  is seeded from the battle seed and the seat [I1] [I4]. Both computer
  players have engine upkeep that takes simulation-stream draws inside the
  tick. The initial design replicates both their upkeep and their decisions;
  deterministic command application does not make in-flight worker memory
  safe to hash (§9.1).
- **Across architectures, as far as the existing fingerprint sees.** The
  locked scenes — a computer-player game checked at 6,000 ticks and at its
  end, up to 54,000, and the three-army benchmark fight checked at three
  points, under each reserved rule set — produce the same partial
  fingerprints from an arm64 build and from an amd64 build run under
  Rosetta 2: fifteen of fifteen on 2026-10-01
  (`internal/headless/rules_lock_retail_test.go`). That fingerprint covers
  unit and movement state, the economy's exact float bits, routes, features
  and both streams' states and draw counts, and omits projectiles,
  visibility and the computer players (L5). It bounds L1; it does not
  dismiss it.

### 5.3 Audit findings and their resolution

L1 and the timing/package-ownership portion of L9 describe the pre-M1
audit. M1 resolves those parts through the kernel, content timing and
`internal/effects`; the perspective and event-window work in L9 remains for
M5. The other findings retain their milestone gates below.

**L1 — Floating-point library results differ by architecture.** The
authoritative packages call seven standard-library functions: `math.Hypot`
(COB query ports, aiming, projectile motion, flight, altitude and ground
movement), `math.Atan2` (the bearing conversion behind COB ports, combat
bearing and pitch, movement, guard and `StartBuilding`, and the Community
kickout), `math.Acos` (ballistic aim), `math.Sincos` (the flight lean),
`math.Sin` and `math.Cos` (the sine-table build, the piece-rotation chain of
`internal/model/model.go` — whose results combat and COB read as piece world
positions — and the Community kickout), and `math.Tan` (the Community
kickout). They were measured on 2026-09-30 by
evaluating each over deterministic inputs on arm64 and on amd64 (the amd64
binary under Rosetta 2; Go 1.27.1), comparing every result bit for bit, and
then applying the narrowing the engine applies:

| Function and inputs | Results differing in any bit | Still differing after the engine's narrowing |
|---|---|---|
| `Hypot`, 2,000,000 raw 16.16 deltas of 2^8–2^28 | 86,867 (up to 2 ulp) | **11** (integer truncation) |
| `Hypot`, every integer pair in 0–3000 | — | **98 of 9,006,001**: `Hypot(165, 52)` is 173.00000000000003 on arm64 and 172.99999999999997 on amd64, so it truncates to 173 and to 172 |
| `Acos`, 2,000,000 values in −1…1 | 109,585 | 0 (16-bit angle) |
| `Atan2`, 2,000,000 raw deltas | 14,739 (1 ulp) | 0; also 0 of 20,000,000 through the bearing conversion's round-half-even |
| `Sincos`, all 65,536 headings | yes | not measured |
| `Sin`, `Cos`, 2,000,000 values in ±2π | 20,055 and 20,088 (1 ulp) | 0 |
| `Sin`, `Cos`, the 65,536 engine angles (2026-10-01) | 1,315 angles | 0 of 160,000,000 piece rotations of integer coordinates up to 2^28 |
| `Tan`, 2,000,000 values in ±1.5 | 82,826 (up to 2 ulp) | 0 |
| The 512-entry sine table | identical | identical |

The Go distribution implements `Hypot` in assembly on amd64 and in Go
elsewhere, and on arm64 the compiler fuses multiply-adds inside package
`math` itself (the arm64 `math.sin` holds sixteen fused instructions, the
amd64 one none). The fusion guard compiles only this module's packages, so it
cannot see either. Every one of the 98 integer flips is a Pythagorean pair,
whose exact answer is a whole number; such deltas are ordinary on a grid, and
the battle calls `Hypot` for movement, flight and aiming every tick. One
flipped distance is enough to separate two machines. How often that happens
in play is not established: the locked battles agree across the two
architectures as far as the partial fingerprint sees (§5.2), so this is a
rare event to be closed, not one that has been observed to end a game. A
`GOAMD64=v3` build could not run under Rosetta and was unmeasured in that audit; the
installer builds with that setting cleared.

**Which results are right is a retail question, not a portability one.**
Retail's two-argument distance routine runs with a 64-bit significand
`[01 R-DET-01 §3]`, and `[01 R-DET-01 §7]` records its arithmetic step by
step from two independent traces: both operands are divided by the larger,
the two squares are summed in one extended-precision expression, the root is
taken at that precision, and each of those results and the final product is
stored to a double. Six of its thirty-nine call sites keep the double, so the
whole result is observable and not only its truncation. A software model of
the sequence, compared with the library after the engine's truncation:

| Inputs | arm64 library | amd64 library |
|---|---|---|
| 4,504,501 unordered integer pairs in 0–3000 | 0 differ | 49 differ — the 98 ordered pairs above |
| 6,000,000 raw 16.16 deltas of 2^8–2^28 | 0 differ | 17 differ |

After truncation the arm64 build's fused `1 + q·q`, rounded once, agrees with
retail's sequence on every pair sampled; the amd64 assembly rounds twice and
is the outlier.
`Hypot(165, 52)` is 173.00000000000003 in the model, as on arm64. The model
is itself one below the exact whole answer on 722 ordered pairs, as retail's
routine then is, so exactness is not the target either. Before truncation
the arm64 library and the model differ in a bit on 0.04% of the integer pairs
and 2% of the raw deltas, which matters to the callers that keep the float —
the flight brake.

The fix (§16, M1) is therefore defined by retail, function by function:

- **Distance.** An in-repository routine performs retail's rounding sequence
  in integer arithmetic, identical on every architecture by construction. A
  version "with every product rounded before it is added" would standardise
  every platform on the amd64 values and move the arm64-recorded locks away
  from retail. A prototype of the retail sequence (2026-10-01) matched a
  big-number model of `[01 R-DET-01 §7]` on twenty-one million operand
  pairs and, substituted at all thirteen call sites, left the fifteen locks
  unchanged on arm64 and on amd64. It costs about 50 ns a call against the
  library's 2 ns, which in the simulation benchmark is about eight percent
  of a tick's process CPU, so M1 gives the truncating sites a proven
  shortcut (§16.1, M1-C6).
- **Sine and cosine.** The engine's angles are 65,536 values. A table of the
  results, built once by a routine compiled under the fusion guard, replaces
  the calls on the piece-rotation chain and the flight lean and takes the
  library off the busiest float path in the tick.
- **`Atan2`, `Acos`, `Tan`.** In-repository implementations compiled under
  the fusion guard. No narrowed result differed in the measurements above, so
  these should move no lock; their retail forms are recorded where a stored
  value could change.

The library implements sine, cosine, tangent, arctangent and arccosine in
portable Go on both architectures; the two differ only where the arm64
compiler fuses a multiply-add inside them. An in-repository form with every
product rounded therefore returns the amd64 library's bits on every
platform, and the amd64 build already holds every lock.

**Conversions are part of the same hazard.** Go leaves a floating-point to
integer conversion implementation-defined when the value does not fit, and
the two architectures disagree: `int32(3e9)` is 2147483647 on arm64 and
−2147483648 on amd64, a NaN converts to 0 and to the minimum integer, and
`uint16(3e9)` is 65535 and 0. The shared truncation helper
(`numeric.TruncateFloat64ToLow32`) defines those cases and most arithmetic
goes through it; 38 conversions in the authoritative packages do not, 21 of
them in the Modern AI's observation code, where an authored or mutated value
can be large. The bit pattern of a NaN an operation produces differs too.
M1 routes every such conversion through a helper with one defined result,
and the digest refuses NaN (§9.1).

**L2 — The local seat is a simulation input.** Retail ran one viewing player
per machine, and the faithful single-machine tick inherits places where that
player decides authoritative state. §6 lists them and generalizes each.

**L3 — Host pump batching reaches the world.** §4.5.

**L4 — Commands assume one seat.** No command names its seat, and some apply
paths read the session's current selection rather than the units the player
meant. §7.

**L5 — No complete state hash exists.** `Session.PartialStateFingerprint`
deliberately excludes projectiles, visibility, the computer players and more
([DESIGN_RUNTIME_DETERMINISM §4](DESIGN_RUNTIME_DETERMINISM.md)), and the
`HashState` helper is test-only. Both format text through `fmt` into
SHA-256. A desync detector needs a complete, cheap digest. §9.

**L6 — A restored battle does not continue identically.** Retail-format saves
omit both random streams
(`TestRetailBattleSaveLoadContinuationReport` reports the first divergence),
so rejoining from a save is not available. §11.3.

**L7 — AI work has no complete checkpoint representation.** A Modern AI
worker mutates its brain, private generator and pending command batch before
their application deadline (`internal/aikit/host.go`). Existing saves join
the worker to read its generator, but deliberately omit brain memory
(`internal/session/ai_save.go`): a restored controller starts again from
observation. §9.1 builds on that instead of fighting it. The regular
checkpoint leaves controller memory out and hashes what the controller has
had applied, and a snapshot restarts every replica's controllers at one tick.

**L8 — Existing hashes are not multiplayer admission identities.** Unit
scripts and model heights are deliberately absent from `Catalog.Hash`, and
`SimArt` is separate from it. Build revision alone also omits modified source
and build choices. §8.2 defines a complete simulation-content digest and a
verified release identity rather than reusing diagnostic hashes as proof.

**L9 — The effect pool is simulation state, and the host feeds it.** The
fixed active-effect pool `[03 §1]` — 300 records under Strict 3.1, more under
the Community table — is advanced in phase 4 and kept in the session's
publication state, which the code and I6 treat as presentation. It is not: a
full pool refuses work that draws from both streams. A shattering piece
stops at the pool's capacity before the eight simulation-stream draws of
each refused fragment `[04 R-COB-04 §3]` (`FixedEffectPool.AdmitShatter`,
reached from `internal/session/shatter.go`), and a script explosion or a
debris impact adds its land-dust puffer, with its CRT draws, only when its
own record was admitted `[03 R-FX-01 §3]`. What fills the pool depends on
the host in three ways:

- **Art timing.** A record lives as long as its animation players. The
  primary player's per-frame holds come from the art entry the event names
  `[06 R-WFX-01 §1]`, through a resolver that only the windowed host installs
  (`Session.SetEffectTimingResolver`, bound in `cmd/nanolathe/battle.go` to
  the client's art cache). A host without it leaves that player inactive, so
  the same records retire sooner.
- **The viewer.** The COB effect gate (§6.3) decides whether an emit produces
  a frame event, and that event is admitted to the same pool.
- **The event window.** Events reach the pool through a per-tick window of
  4,096 that positional audio and status events share. An audio event is
  emitted only where the viewing seat can hear it, so the window's occupancy
  follows the viewer as well. In play it stays far from its bound (below).

Measured on 2026-10-01 with the Strict 3.1 benchmark fight (three 250-unit
armies, seed 7, the scene as it stood before the O22 correction) and a
resolver supplying the per-frame holds the window reads from the same art:
the pool first reaches 300 at tick 3,283, where without the resolver it
peaks at 286. In that tick's unit phase it refuses three shatter fragments
and ten other admissions, which leaves the simulation stream 24 draws and
the CRT stream 10 draws behind the other run. The two runs' partial
fingerprints are equal through step 3,000 and different at step 4,500 and at
every later checkpoint. A window and a headless host therefore already
compute different battles from the same inputs, and every fingerprint lock
is headless.

A census of the same day shows how narrow the exposure is. Each scene ran
with and without that resolver under each reserved rule set:

| Scene | Strict 3.1 (300 records) | Community 3.9 (3,000) | Modern (3,000) |
|---|---|---|---|
| Benchmark fight (seed 7, before O22), 15,000 steps | full from tick 3,283, in 30 ticks; 30 fragments and 81 other admissions refused | peak 392 | peak 431 |
| Computer-player game, to 54,000 ticks | peak 18 | peak 43 | peak 159 |
| Survival, to its defeat | peak 126 | peak 260 | peak 124 |

Only the Strict fight diverges. Every other pair of runs keeps equal
fingerprints to its end, and no existing lock constant changes with the
resolver. The event window peaks at 48 events in a tick against its bound of
4,096 and drops nothing. After the O22 correction the seed-7 Strict pool
peaks at 296 records through 15,000 steps; the pool lock therefore uses seed
5, whose pool first fills at tick 2,255 (§16.1 M1-C9). ARCHITECTURE §2
already states the intent for `SimArt` — "so headless and windowed battles
run one simulation" — and this reader of art was left behind when the others
moved there.

The fix (§16, M1): the holds become simulation content, compiled into
`SimArt` beside the entry lengths and feature sequences it already carries
and covered by the content identity (§8.2). Every host times the pool from
that table and the resolver seam is removed. The pool, its fragment geometry
and the strip, debris and event-window state that feed it join the canonical
snapshot (§9.1), and their code — today in `internal/render`, which the
determinism guards exempt as presentation by package — comes under those
guards (§14). The viewer gate and the audio share of the window are the
multi-seat half and belong to §6.3. The windowed timing is the researched
one, so M1 moves a headless fingerprint only where a pool fills — in none of
today's locked scenes — and leaves the window's battles as they are.

### 5.4 Invariant amendments

Adoption amended four invariants; INVARIANTS.md carries the text and says
which milestone brings each into force:

- **I2** gains two rows. Authoritative packages call no standard-library
  floating-point function other than exactly rounded ones (`math.Sqrt`,
  `math.Abs`, `math.Floor`, `math.Ceil`, `math.Trunc`, `math.Round`,
  `math.RoundToEven`); everything else goes through the in-repository
  implementations of L1. And a floating-point value becomes an integer only
  through a helper that defines the result for a value that does not fit or
  is not a number. Source guards enforce both.
- **I6** gains two sentences. Which seat is local, the viewing slot, and the
  host's pump sizes are presentation inputs; no authoritative state, draw
  count or draw order may depend on them, outside the single-seat
  equivalences of §6.5. And nothing a host supplies after composition — a
  renderer, an art cache, an audio service, a preference — may change a
  lifetime, a pool's occupancy or a draw; the effect pool is named as
  authoritative state (L9).
- **I4** gains the seed handoff: in a lockstep battle the relay draws the seed
  pair and every client receives it in the start message (§8.3).
- **I5** distinguishes retail pool identities from delayed human-command
  references: the latter additionally carry a deterministic allocation serial
  (§7.2). This does not change slot allocation or internal retail references.
  The serial and its counter are authoritative metadata retained by snapshots.

## 6. One world for every player

### 6.1 Owner-machine equivalence

In retail every machine ran the work of its own players — its local human
(control 1) and the computer players it hosted (control 2) — and saw everyone
else through packets. The truth about a unit in a retail battle was what its
owner's machine computed. A lockstep client therefore runs, **for every seat,
the work that seat's own machine would have run for it**, drawing from the
shared streams. Retail has no common player-row order: each machine has its
own human first and the other rows in its admission order. Nanolathe uses a
canonical seat order, with each human's block before the computer seats it
hosts so the settlement freeze starts on the same due
`[03 R-VIS-01 §4]` `[05 R-ECO-01 §1]`. Work falls into four kinds:

1. **Per-owner work** that retail gates on control 1 or 2 — weapons, order
   pumps, movement, water damage, self-repair, cloak upkeep, settlement,
   commander creation, planner records, the death latch, kill filing, storage
   bonus clear, commander sweep and cloak-proximity pass
   `[08 R-SKIR-01 §3]` `[03 R-VIS-01 §4]` `[04 R-MOV-03 §1]` `[05 R-ECO-01 §1]`
   `[08 R-ENTRY-01 §3]` `[08 R-ENTRY-01 §5]`. Nanolathe already runs it for
   every seat, because no seat is ever control 3.
2. **Per-viewer work** that retail runs once per machine for that machine's
   viewing player. It runs once per **perspective** (§6.2), one for each human
   seat. This is the work §6.3 lists, and the work that has to change.
3. **Per-machine work** that retail runs once per machine whoever its player
   is — wind, meteors, features, projectiles, effect strips. It runs once per
   battle, as it does today. Retail's split of feature damage and fire spread
   between an authority peer and the others `[05 R-FEAT-01 §8]` falls away
   with the puppets: there is one world, and it is authoritative everywhere.
   Machine-local countdowns, ending bits, visibility modes and mapping
   history instead belong to perspectives where they affect owner work
   (§6.3); Q24 defines the shared per-player explored-history projection (§6.7).
4. **Presentation** — what a machine showed its player — stays on the client.

Puppet-only machinery — remote units' reduced controllers, their release of
map cells, the unit-state and economy packets `[04 R-COLL-01 §4]`
`[08 "Economy and integrity checks"]` — has no lockstep counterpart.

### 6.2 Perspective slots

Retail keeps two slots per machine, the local player's own slot and the
viewing slot. On the ordinary online path both stay zero from the player-table
reset, and row
zero is that machine's human; the sensor phase, visibility probes and minimap
contacts read the viewing slot `[03 R-VIS-01 §4]`. The `+Control` developer
command changes the controlling slot; `+View` changes only the viewing slot
`[07 R-CAM-01 §6]`. A multiplayer player who becomes a watcher at defeat
sees everything because its machine clears its own mapping and line-of-sight
mode bits; that branch writes neither slot `[08 R-SKIR-01 §3]`
`[03 R-VIS-01 §4]`.

A lockstep battle keeps one **perspective** per human seat: an own slot (the
seat) and a viewing slot (the seat, until `+View`, a stream entry, moves it).
Every authoritative read of "the local
player" or "the viewing slot" becomes a read of a perspective, chosen by who
is acting:

- a unit's own decision — target acquisition, the direct-visibility
  predicate, cloak — reads the perspective of the machine that would have
  simulated the unit: its owner's, or for a computer seat its host's (§6.6);
- a per-seat block reads its own seat's perspective;
- presentation reads the local client's.

A battle with no human seat (the arena) keeps a single perspective, the
session's viewing slot, exactly as today.

### 6.3 What changes

The session holds the local seat three ways today: `Session.LocalOwner` and
`Session.ViewingOwner`, both seeded from the last human row, with
`LocalOwner` re-derived every tick from the controller rows
(`internal/session/step.go`, `trigger_adapter.go`); the visibility service's
own `local` slot, which is the viewing owner; and the control byte, where 1
means "the local human". Every authoritative reader below moves to a
perspective or a seat. Code sites are as of `193abfde`. A 2026-10-01 audit
of every reference to those three in the authoritative packages found no
direct reader outside these rows and §7.1. What it added are consequences of
the reads — the effect pool, the end countdown, the respawn rebuild and the
sensor source gate — which the rows now carry. The machine option bits row
(2026-10-02) reads none of the three; it is there because retail keeps that
word per machine.

| Work | Contract | Today | Lockstep form |
|---|---|---|---|
| Sensor phase | Five unit walks that write the seen, sonar, jammed and decloak-timer bits from the viewing slot, inside that player's 30-tick settlement deadline, only when more than one player is active; a defeated or watching viewer marks every unit friendly `[03 R-SENSOR-01]` `[03 R-VIS-01 §4]`, and the first walk also marks the units of an owner that has declared alliance toward the viewer and shares radar `[03 R-VIS-01 §7]`. Every unit's fallback targeting reads the seen set `[06 §3.1]`; the sonar bit exempts underwater rejection; the decloak bit feeds cloak upkeep. | Runs in the viewing owner's due block (`step.go`); passes in `internal/visibility/sensors.go`; new units of the viewing owner get the sonar bit at creation (`session/visibility.go`). | One pass per perspective, in that seat's own deadline block. The bits it writes are held per perspective; each reader reads its acting unit's perspective. The alliance rows and radar-share options the first walk reads are per-seat battle state. Pass 4's source gate, which retail writes as "the owner's controller is 1 or 2" and means "simulated on this machine" (`ownerLocallySimulated`), becomes "this perspective's seat or a computer seat it hosts": with every human seat control 1, the unchanged gate would let each perspective's pass write every seat's decloak bit. |
| Direct-visibility predicate | Reads the local player's coverage bit even for a query on behalf of another record `[03 §3.2]`. | `internal/visibility/predicate.go`, reached by every side's target-list rebuild, Modern targeting, danger and repair-pad queue, the Classic computer player's rally sight `[08 R-AI-01 §7]`, and the COB effect gate below. | Reads the querying record's perspective. |
| COB effects | The effect opcode first tests whether the viewing player can see the unit; a failed test does nothing at all. A passing test spawns strip objects that take pool slots and, in the sprinkle and smoke families, CRT draws `[04 R-COB-03 §6]` `[03 R-STRIP-01 §1]`. Draw counts are behaviour [I4], so on one machine the viewer decides the CRT stream's position. | The gate in `composition.go` reads the viewing owner; the passing path also emits a frame event that the fixed effect pool `[03 §1]` admits in phase 4, so the viewer reaches the pool's occupancy and, through it, the simulation stream as well (L9). | Spawned when the unit is visible to **any** perspective. The union is taken ahead of the strips and the pool, so both hold the same records on every client. Each client then draws only what its own seat may see: that filter is presentation and runs after the pool. |
| Effect pool | A fixed pool whose records live for their animation players' authored holds `[03 §1]` `[06 R-WFX-01 §1]`; at capacity it refuses shatter fragments and land-dust puffers before their draws `[04 R-COB-04 §3]` `[03 R-FX-01 §3]`. | Since M1, timed on every host from the holds compiled into `SimArt` and bound at composition; the resolver seam is gone (M1-C8) and the pool lives in the guarded `internal/effects`. Still fed through an event window that positional audio shares; an audio event is emitted only where the viewing seat can hear it (L9). | Timing is already host-independent (M1). Admission of the events that feed the pool does not depend on audio or status events: those are produced for the local seat after the pool, or counted in a window of their own. |
| Temporary sight | Recorded only for victims the viewing slot owns, at most 20 `[08 R-SESS-01 §3]`; expired in the executor tail `[01 R-PLAT-02 §5]`. | `internal/session/eyeball.go`; expiry in `post_loop.go`, once per host pump. | One list of 20 per perspective; expiry after every tick in lockstep (§4.5). |
| End condition | Each machine evaluates its own player in that player's deadline block; online, authored map triggers are never polled `[08 R-TRIG-01 §6]` `[08 R-SKIR-01 §3]`. | `endConditionBlock` runs only on the local owner's due (`step.go`, `result.go`): with two humans the lower seat's defeat is never evaluated and the higher seat's ends the battle for everyone. | Every human seat's block evaluates that seat (§6.5). |
| End countdown | One countdown and ending latch per machine gate its human and hosted computer settlement. Won and lost paths share it; a false due never resets it. Quit and Continue-Watching No set that same ending bit `[05 R-ECO-01 §1]` `[08 R-SKIR-01 §3]`. | `publishEndCountdown` (`result.go`) mirrors one latch onto all ten player records. | Per human perspective; gates that human and its hosted computers for victory as well as defeat. Human work precedes hosted-computer work on the arming due. The kind-3 after-loop no-human site runs every tick, skips Deathmatch and uses the same countdown, ending all perspectives on its sixth consecutive true tick when starting unarmed `[08 R-TRIG-01 §6]`. |
| Commander death | Only the owner's machine clears the storage bonus, files the commander death and sweeps its units. Received deaths do not sweep. A human respawns only in its own block, with the host's starting-resource values; its visibility rebuild resets that machine's complete mapping history and every eligible sight grid `[08 R-SKIR-01 §3]` `[08 R-ENTRY-01 §7]`. | `processPendingCommanderDeaths` runs per owner; respawn is the local owner's only (`commander_death.go`); `RebuildEntry` refills all grids. | Sweep each owner once. Respawn each eligible human through its own block. Rebuild that perspective's own and hosted-computer visibility/history; retain unrelated players' explored history under Q24 (§6.7). Other perspectives do not rebuild because a remote seat respawns. |
| Watcher entry | Clears mapping and line-of-sight mode bits for the whole watching machine, hosted computer included, and rebuilds its complete visibility/history grids `[08 R-SKIR-01 §3]` `[08 R-ENTRY-01 §7]`. | `clearWatcherVisibilityMasks` in `ai_entry.go`, at entry and restore. | Changes the entering perspective and the hosted computers that borrow it; other perspectives keep their configured modes. Unrelated players' explored history is retained under Q24, as for respawn (§6.7). |
| Automatic sharing | The local slot only, every 60 ticks, multiplayer only `[05 R-SHARE-01 §3]`. | Inert: the economy's networked flag is never set. | Every human seat, ascending. |
| Explored-map sharing | Every 450 ticks the opted-in sharer asks surviving remote human allies to copy its explored bits; the target applies the request on its own mapping-grid copy after transport delay. The share screen offers a one-time gift to anyone and applies it locally at once `[05 R-SHARE-01 §3]` `[05 R-SHARE-01 §6]`. History affects path probes, placement and Permanent sight. | Not implemented; `internal/economy/tick.go` only counts eligibility. | Q25: periodic shares apply on the request tick, humans in canonical seat order, copying to eligible human recipients; computers neither send nor receive automatic shares. They use Q24's per-player history. Explicit gifts apply in command-stream order on their assigned tick (§6.7, §7.1). |
| Unit transfer | A local and a remote branch with different copy rules `[05 R-WORK-01 §15]`. | Local branch only. | Chosen by whether old and new owner share a machine. |
| Player-record cheats | `+Give`, `+ATM` and their kin act for the local player; `Give` takes its source from the own/controlling slot that `Control` changes, not the viewing slot `[07 R-CAM-01 §6]`. | Give's source is the own/controlling slot, resolved at drain time `[07 R-CAM-01 §6]` (the code through `41a09a60` still debits the viewing owner; a separate correction moves it); ATM and the Modern spawn command act for the local owner (`commands.go`, `spawn_command.go`). | The issuing seat (§7.1). |
| Machine option bits | `DoubleShot` and `HalfShot` toggle the double and half damage gates, bits 7 and 8 of one options word whose bit 10 is the `ShootAll` target-admission bit; a fresh battle starts with both gates clear, and the session-settings block that carries the word out to other machines is never copied back over a machine's own word `[06 §9.2]` `[06 §3.2]`. A hit is resolved on the shooter's machine, a death explosion's on the victim owner's, and a shooterless record's on every machine `[06 R-DMG-01 §9]`. | One battle-local gate pair in `combat.Service` (`ToggleDoubleShot`, `ToggleHalfShot`); `ShootAll` is a field nothing sets. | Per seat: each seat's bits govern the work owner-machine equivalence gives its machine (§6.1) — damage from its own and its hosted computers' shots and death explosions, and those units' target admission. Which seat's bits apply to damage retail resolves on several machines (shooterless records) is settled in M5 against `[06 R-DMG-01 §9]`, or left a `TODO(question)` at its code site. |
| Builder options | The Community builder preference belongs to the host player (DESIGN_COMMUNITY_PATCH §4.3). | Installed only for the local owner (`builder_options.go`). | Each seat's own, as a seat command. |
| Survival waves | Waves patrol to the nearest unit of the human team (DESIGN_SURVIVAL §6.7). | `survival.go` targets the local owner and its allies. | All human survivors. |
| Loss statistics | — | The old cause-3 implementation incorrectly uses the local alliance row. Current `[06 §12.1]` identifies a requested-snapshot receipt latch. | File the victim owner's loss once; lockstep has no overwrite-sync receipt latch. Preserve the preexisting single-player path pending its separate fingerprint-reviewed correction. |
| Known-site gate | The placement check consults the viewer's map knowledge `[04 R-P0-08-B §1]`. | Authoritative only through the Community order drag (`placement.go`, `community_order_drag.go`). | The issuing seat's perspective. |

### 6.4 Session kind

Retail branches on the session kind in many places beyond transport: kind 3
starts at speed 10, takes its cheat gate and options from the host, offers
watchers, restrictions and sharing, never polls authored triggers, and
disables saving `[08 R-ENTRY-01 §2]` `[08 R-OOS-01 §5]` `[08 R-TRIG-01 §1]`
`[08 R-SAVE-02 §4]`. Its interface differs too: only in kind 3 does Tab open
the strip of Options, Allies, Share and, on the host's machine, Control, and
only there does `h` open the share screen; in a single-player battle Tab is
the options key `[07 R-CAM-01 §2]` `[07 §11]`. The engine never builds kind 3
today (`skirmish.go`), and the session's network preload state exists but is
unreachable. **A lockstep
battle is kind 3** for every branch the research establishes, minus the
transport; a single-player battle stays kind 2. Each kind-3 branch an
implementation unit meets is checked against its research section, and one
the research leaves open is a `TODO(question)` (§19). The elimination
announcement shows what that audit has to catch: its kind-3 form takes one
CRT draw masked to eight entries and posts the line on the machine that ran
it `[08 R-CAMP-01 §9]`, where the engine has only the kind-2 form. Every
retail machine takes that draw locally whenever any human or computer live
count falls to zero, including after another Deathmatch loss; nothing is
sent. The shared world takes one CRT draw per such elimination event. The
retail own-human removal-reason check that can suppress the announcement
has no shared-world counterpart.

### 6.5 Single-player battles do not move

In a single-player session every perspective read resolves to its human seat, and every
computer seat borrows it — today's behaviour, unchanged. `+View` in a
single-player battle still moves the one viewing slot. So the generalization
leaves every single-player battle, and therefore every fingerprint lock,
bit-identical; that is M5's exit test (§16). Nothing in it is a gameplay
departure in single-player.

When several seats each evaluate their own end, the session's result
becomes per seat: each seat's result is the one its own block latched, with
its own latch and countdown (§6.3). A defeated seat may watch or leave; the
battle goes on while any human seat is still playing. It ends for every
client once every human seat has latched a result or been finally removed,
or through the no-human countdown `[08 R-SESS-01 §1]` `[08 R-TRIG-01 §6]`. A
seat out of play has not left, and its units count as live until a vote
removes it, in every mode, by design. A finally removed seat is absent from
the end-condition sweeps in every mode, as retail's cleared record is
`[08 R-LEAVE-01 §3]` `[08 R-LEAVE-01 §5]`: under Modern and Community its
retained idle units are still live — they can be attacked and destroyed —
but no survivor has to destroy them to win (§15 Q29, decided 2026-10-02). If
every human still playing is out of play and none has returned within
`RejoinGraceMilliseconds`, the relay ends the battle without a result
(§15 Q28; §11.1).

### 6.6 Computer seats

Retail allows at most one computer per human machine. Its creator hosts it
for the whole battle; only that machine runs its planner and full unit work,
while peers receive results `[08 R-SKIR-01 §13]` `[08 R-AI-01 §21]`.
The computer borrows that machine's sensor picture. A defeated human remains
watching while its computer lives, even when watching is forbidden. Removing
the human removes the machine's computer too and destroys both players' units
`[08 R-LEAVE-01 §9]` `[08 R-LEAVE-01 §10]`.

Initially every client runs every computer seat. Its **host seat** is the
human that added it, fixed in the configuration (§15 Q4). While that human
is out of play but not finally removed, its computers keep running in every
mode with the host's perspective: every client runs every computer, and the
reconnectable window is itself a Nanolathe addition (§11.1). At the human's
final removal the modes differ (§11.1, §15 Q26):

- **Strict 3.1** removes the human seat and every computer seat it hosts
  together, in ascending slot order, the human's own slot taking its place
  in that order, each through the destruction contract of §11.1. For a
  voted removal this follows retail's peer removal of the machine group
  `[08 R-LEAVE-01 §2]` `[08 R-LEAVE-01 §9]`; for a resignation it is the
  silent deletion the other machines perform for retail's menu quit
  `[08 R-LEAVE-01 §7]` (§19 O23, decided 2026-10-02).
- **Modern and Community** keep its computers running with the host's
  perspective, which goes on updating: the removed human's own per-seat
  block and sensor pass keep running for it (§15 Q29, decided 2026-10-02).
  This is a Nanolathe Modern policy, selected with the retention of the
  removed human's units through the existing `session.RuleSet` — by one
  answer (§11.1, §15 Q28).

Retail difficulty belongs to the hosting machine, is not synchronized and
is consulted only for its computers: the room creator sets hard at START;
other hosts use their existing value `[08 R-AI-01 §21]`. Q23 instead agrees
difficulty explicitly per computer seat, for Classic and Modern computers
in every online mode. All consumers use that seat's value; no machine-local
preference overrides it. Classic keeps its bound rule set's difficulty
behavior; Modern keeps its persona selection and full-income contract.

Today every reader takes the session's one difficulty word through
`sessionDifficultyWord` (`internal/session/composition.go`), directly or
through the shared profile's plan difficulty (`session.ControllerDifficulty`);
one researched reader is dormant in the code. Each concerns exactly one
computer seat, so under owner-machine equivalence each reads the word of the
machine that hosts that computer `[08 R-AI-01 §21]`, which Q23 replaces with
the computer's own agreed value:

| Reader (as of `41a09a60`) | Researched use | Seat whose difficulty applies |
|---|---|---|
| Classic profile plan gate, `profile.SetDifficulty` at entry (`ai_entry.go`); one `*ai.Profile` is shared by every slot today | `plan` directives `[08 R-AI-01 §12]`, evaluated with effect only where a planner record exists `[08 R-AI-01 §21]` | The computer whose planner it gates |
| Modern parameter layering, `AIOverrides.For` through `ControllerDifficulty` (`ai_entry.go`) | None: Nanolathe's parameter layer | That Modern computer |
| Modern persona selection, `personaFor` (`mods/aikit/rules.go`), also used by the Survival buddy host (`mods/aikit/survival.go`), reading `ControllerDifficulty` of the profile | None: Nanolathe's persona | That Modern computer, Survival buddies included |
| Ledger production discount, `Econ.SetEconomySelector`, read by the contribution and reclaim credits and by the feature table's `AIDifficultyIncome` credit (`internal/economy/maker.go`) | A computer player's production credit `[05 R-ECO-01 §3]` `[05 R-ECO-01 §11]`, settled by its own machine `[05 R-ECO-01 §1]` | The producing computer |
| Construction refund discount, `construction.Service.ModeSelector` (`bindConstructionEconomy`) | Refund credits `[05 R-ECO-01 §11]` | The refunded computer owner |
| Transfer helper's computer-recipient discount, through the same contribution credit (`economy.Service.Transfer`) | Applied at credit to a recipient whose control byte is 2 `[05 R-SHARE-01 §2]`; that recipient's own machine settles the credited slot | The recipient computer |
| Computer-income projection, `projectComputerIncome` and the `ComputerIncomeRules` answer it feeds (`computer_income.go`) | Projects the word above onto the ledger and refunds | Each computer, before its full-income mark |
| Commander-respawn grant: dormant, since `respawnLocalCommander` (`commander_death.go`) adds no grant while the lobby's resource shorts are zero on every reachable path | Listed among the word's readers `[08 R-AI-01 §12]`; the respawned commander's added stored resources are scaled × 0.5 or × 0.7 by difficulty when the owner is a computer `[08 R-SKIR-01 §3]` | The respawning computer; unreachable online while computers are excluded from Deathmatch (Q23) |

A human seat consults no difficulty, and the Survival attacker has no
economy and runs no planner tasks (DESIGN_SURVIVAL §4.1), so no reader
reaches it. **Rule (§15 Q28, approved 2026-10-02): an online configuration
carries no battle-wide difficulty word.** Each added computer's row carries its value
(§8.6), and every reader above takes the seat it concerns. Validating a
global word against the per-seat values was rejected: retail has no
battle-wide difficulty to preserve, two computers at different levels admit
no single valid word, and a second copy is a second source of truth that a
partly migrated reader could silently read.

**Nanolathe Modern policy — online computer-seat cap (approved 2026-10-02).**
Strict 3.1 retains one computer per human. Modern and Community allow a
human to add multiple computers within the session's available lobby seats,
including the existing Survival layout constraints. Each has the fixed host
association above. The cap is selected through the existing
`session.RuleSet` composition by the session-owned `SeatRules` seam
(`ComputerSeatsPerHuman`: `StrictSeats` answers one, `ModernSeats` the
available seats; Community composes Modern's answer), added by U5 under
DESIGN_GAMEPLAY_RULES §9 because no simulation package asks the question; the
final-removal answer of §11.1 joins that seam in M6. There is no room flag
or second registry. All online modes
exclude computers from Deathmatch in the first releases. That admission
restriction does not define computer respawn behavior. Single-player
admission and difficulty remain unchanged.

M2 admission tests cover a second computer refused under Strict and admitted
under Modern/Community when seats permit, exhaustion of available seats,
Deathmatch refusal in every mode, and fixed host attribution. Change only
one seat's difficulty and require a configuration mismatch; under the
rule above (Q28), a configuration carrying a battle-wide difficulty
word is malformed. M5 verifies that every
reader in the table above uses the configured value of the seat it
concerns — Classic plan gates, Modern parameters and persona selection,
production and refund discounts, and a transfer to a computer recipient —
including two
differently configured computers with one host; preserve
existing income/RNG rules and single-player locks.

The configured host seat is a simulation association, not the machine that
happens to have the fastest connection. Reconnects cannot change it. Its
effects on targeting when that human becomes a watcher must be covered by
the multi-seat tests and explicitly accepted in the online rule contract.
A Modern policy using each computer player's own sensor perspective is a
separate gameplay decision, through the existing rule seams, not a transport
optimization or an inferred retail rule (§15 Q4).

One alternative is to run a Modern brain on a designated authority and stream
its decisions, while every replica still runs shared-RNG engine upkeep.
`aikit.Host.Step` already separates those responsibilities. This could reduce
duplicate CPU work but needs authority, deadlines and failover contracts;
it is not selected initially. Replicating all brains is a design choice,
not a consequence of their upkeep drawing from the shared stream.

### 6.7 Approved online alliance and exploration policies

**Nanolathe online policy, approved 2026-10-02 (Q22, Q24, Q25).** These
apply to multiplayer sessions in every gameplay mode, Strict 3.1 included.
They define the common-world projection of retail's separate machine
copies; they do not rewrite the retail research or change single-player.
No room switch or second gameplay registry selects them. The mode-dependent
computer cap is a separate policy under the existing RuleSet (§6.6).

**Alliance declarations (Q22).** Keep one directed matrix of declarations
in the common world; the incoming-declaration rows are its transpose, not
an independently mutable copy. Apply an admitted declaration at its command's
tick and stream position on every replica. A declaring alliance toward B
does not declare B toward A. Team initialization, declaration admission,
one-sided consumers, mutuality and the other shared-victory requirements
retain their researched contracts (§3.3, §7.1). This grants neither shared
current sight nor automatic resource sharing. The intentional difference
from retail is that a third seat's victory evaluation sees the same
declarations as the sender and recipient, rather than stale machine copies.

M5 tests use at least three human seats: A declares toward B, all replicas
see that directed edge, the reverse stays unchanged, and C's shared-victory
evaluation sees it. Exercise removal of the edge, team-locked declarations,
mutuality and the other victory requirements. Assert identical outcomes
across local-view choices, no extra RNG draws from matrix maintenance, and
unchanged single-player locks. A two-seat agreement alone cannot verify the
third-seat correction.

**Explored history (Q24).** Keep one canonical explored-history grid per
player, identical on every replica. This is not one grid merged across all
players. On respawn or watcher entry, apply that transition's established
visibility rebuild to the entering human and the computer seats borrowing
its perspective, resetting their histories as required by that rebuild.
Preserve unrelated players' histories and perspective modes. Do not model
separate stale remote-history copies inside every perspective. Retail resets
the entering machine's copies of all players' histories (§6.3); that broader
remote-copy wipe is deliberately omitted. Current sight, radar sharing,
ordinary sharing eligibility and Survival's existing team sight contract
remain unchanged.

M5 tests give three players different explored regions, reset one human
with a hosted computer, and verify only the own/hosted reset scope at watcher
entry. Separately exercise Deathmatch human respawn without computers,
as Q23 requires. Include later eligible map sharing and
Permanent LOS, where exploration affects visibility. All replicas must
agree independently of their local viewer. Preserve the normal transition's
RNG/resource effects and show no additional effects from remote-history
bookkeeping; retain single-player behavior.

**Sharing time (Q25).** Periodic explored-map shares run at their existing
due site on the request tick, processing humans in canonical seat order.
Each copies the source's history as it stands when that request applies to
the researched eligible recipients. Explicit share-screen gifts are seat
commands, applied on their assigned tick in stream order. No wall-clock
arrival or guessed transport delay changes either application. A share
that precedes another share can therefore affect the later source history;
there is no implicit simultaneous snapshot of all senders.

M5 tests cover the periodic due boundary, opted-out and ineligible seats,
one-time gifts, and a three-seat sharing chain whose same-tick result proves
the declared order. Deliver the same commands to replicas with different
network/pump batching and require the same history at the application tick.
No new RNG draw or resource mutation belongs to copying explored bits.
The policy does not change current sight sharing or add computer recipients
to the researched automatic human share.

## 7. Commands

### 7.1 Classification

Every `HumanCommandKind` falls into one of four classes:

| Kinds | Today | Lockstep |
|---|---|---|
| `SelectionReplace`, `SelectionToggle`, `SelectionClear`, `GroupRecall`, `BuildPage` | Selection is bit `0x10` of each unit's status word, with two visited bits, and the build page lives in the same word. Only command application and phase 2 read them: phase 2 drops units that stop being ready from the selection `[04 R-MOV-03 §1]` and runs the interface pass of the next row. | **Local.** Selection and the build page become client state; the client applies the same readiness rule to its own selection. Nothing enters the stream. |
| `BigBrother`, `ShiftState` | Interface state written into the selection in phase 2 `[07 R-CAM-01 §12]`. `+BigBrother` is ungated in retail, in every session kind, and in the engine `[07 R-CAM-01 §6]`. | **Local**. It writes only the issuing client's selection, which gives no command over units the seat does not own (§7.2), so it needs no online permission. |
| `NoShake` | A preference that stops the shake driver, and with it the driver's two CRT draws per active tick (DET-04). | **Local** in a lockstep battle: the driver always runs and the client declines to apply the published offset. Single-player keeps today's behaviour. |
| `GroupAssign` | Writes each unit's group number, which is also the computer player's task-group index: its classifier skips grouped units, and a human builder's products inherit its group (`internal/ai/groups.go`, `internal/construction/inheritance.go`). | **Seat command** carrying the group's complete new membership: assignment also clears the number from the seat's other units. |
| `Order`, `Stop`, `SelfDestruct`, `Stance`, `Cloak` | Read the selection when no units are given; `Stance` and `Cloak` always do. | **Seat commands**, always with explicit units. |
| `Activation`, `MobileBuild`, `FactoryBuild`, `CancelProduction`, `Stockpile`, `CancelQueuedMove`, `CommunityKickout`, `CommunityOrderDrag` | Explicit units owned by the local owner; the two Community commands also match a publication identity, and the order drag runs the viewer's known-site gate. | **Seat commands**; references by handle and allocation serial (§7.2), the known-site gate on the seat's perspective. |
| `BuilderOptions` | Player state, live mid-battle. The payload names its owner, which is checked against the local owner. | **Seat command**; the owner field leaves the payload (§7.2). |
| `Give` | `+Give p n metal`, open in every session kind and with no alliance test: a transfer from the issuing machine's own/controlling slot (the slot `Control` changes, not the viewing slot) to seat `p` through the sharing transfer, the typed integer converted to single precision `[07 R-CAM-01 §6]`. A negative amount passes every gate of that transfer, raising the source's stock and lowering the recipient's production slot `[05 R-SHARE-01 §2]`. The single-player source is resolved at drain time. | **Seat command** whose source is the issuing seat. **Nanolathe online policy (§15 Q8, every mode):** a positive whole amount within that producer's range is ordinary resource sharing, as in retail. Any other amount — zero, negative, non-finite, not a whole number, or outside the signed 32-bit range of the only producer (§7.4.1) — is rejected at the validation boundary unless the room's cheat permission is on, in which case the signed retail transfer applies to any amount that producer can generate (that range under the permission is §15 Q28, approved 2026-10-02). Without this rule any seat could take another seat's resources with every replica agreeing. Single-player keeps the retail behavior, negative amounts included. |
| `SetResource` | `+NoMetal p n` and `+NoEnergy p n`, also open in every session kind, write any player's stock `[07 R-CAM-01 §6]`. In retail multiplayer a write to another player's stock lasted only until that player's next economy snapshot overwrote it `[08 "Economy and integrity checks"]`. | **Seat command limited to the issuing seat's own stock and gated by the online cheat permission.** This authorization policy applies in every online gameplay mode, including Strict 3.1 (§15 Q8); it is Nanolathe's and not a claim about retail. |
| `SetLogo` | `+Logo n p` sets any seat's insignia `[07 R-CAM-01 §6]` by writing the player record's logo, which saves and result rows carry. | **Local**: a presentation override on the issuing client. The record keeps its configured logo. |
| *(new)* share toggles and thresholds, the share screen's gifts | `+ShareMetal`, `+ShareEnergy`, `+ShareMapping`, `+ShareRadar`, `+ShareAll`, `+SetShareMetal`, `+SetShareEnergy` are network-only; each toggle flips the issuing player's own option, clear at the start `[07 R-CAM-01 §6]` `[05 R-SHARE-01 §3]`. The share screen is `[05 R-SHARE-01 §5]`; declarations have their own row below. Retail's receiver does not check that a share names the player that sent it `[05 R-SHARE-01 §4]`. | **New seat commands** acting on the issuing seat; the relay's seat stamp is the source (§7.2). The share options are per-seat battle state inside the digest (§9.1), not agreed configuration. |
| `View`, `ATM`, `DoubleShot`, `HalfShot`, `Visibility`, `Meteor`, `MakeSelectable`, `Spawn` | Retail cheats, most of them behind the cheat gate `[08 R-OOS-01 §5]`, and the Modern testing spawn. The engine holds no cheat flag: the host refuses the gated ones outside a skirmish session and enqueues the rest unconditionally. Two are ungated in retail too, in every session kind `[07 R-CAM-01 §6]`: `MakeSelectable`, whose bit order resolution, trigger evaluation and the computer player read, and the line-of-sight-type half of `Visibility`, which rewrites the typing machine's one mode word — the battle's only one in single-player. `DoubleShot` and `HalfShot` toggle the double and half damage gates of the typing machine's options word `[06 §9.2]`, which in single-player governs every shot; the argument-free meteor arms a storm with four CRT draws; the spawn creates a unit and is gated only on the Modern rule set. | **Seat commands only when the lobby enabled cheats** (§15 Q8), the two ungated ones included. The sending UI refuses them when disabled, and every receiving simulation independently enforces that permission before dispatch. What each acts on follows retail's per-machine state (§6.1): `View` and `Visibility` change only the issuing seat's perspective — its viewing slot and its mode bits (§6.2, §6.3, §6.7, §7.4.3); `DoubleShot` and `HalfShot` set the issuing seat's own bits of the per-machine options word, which govern the work its machine would have resolved `[06 R-DMG-01 §9]` (§6.3 "Machine option bits"); `ATM` and `Spawn` act for the issuing seat; `Meteor` and `MakeSelectable` act on the whole battle. |
| *(new)* `DeclareAlliance`, `SharedVictory` | Allies-window declarations and shared-victory control `[05 R-SHARE-01 §1]` `[07 R-FE-01 §7]`. | **Seat commands**: declaration `{target, value}` requires a seated non-eliminated, non-watching issuer and a distinct seated non-eliminated, non-watching human target outside the issuer's team. Shared victory `{on}` refuses watchers and seats in teams of two or more. Declarations update Q22's shared directed matrix (§6.7). |
| *(not yet a command)* `ShootAll` | A retail toggle, ungated in every session kind `[07 R-CAM-01 §6]`, of bit 10 of the same per-machine options word, which target admission reads `[06 §3.2]`. The engine has the seam (`combat.Service.ShootAll`) and no typed command. | **Seat command** when it is added, setting that seat's bit: on a retail machine the bit governs the units that machine simulates (§6.3 "Machine option bits"). It needs the agreed cheat permission under Q8. |
| `Gameplay` | Switches the rule set at the phase-1 boundary (DESIGN_GAMEPLAY_RULES §5). | **Lobby only**; refused in a lockstep battle. |

### 7.2 Attribution and validation

Receiver-side authorization is Nanolathe's contract. Retail checks these
roles only in the sending interface, then trusts admitted live-battle payloads
`[08 "Packet framing and dispatch"]`. Its ordinary watcher interface offers
no chat, `+` commands, speed change, sharing or alliance changes.

- **The relay names the seat.** A payload never carries one (§4.2).
- **Every receiver authorizes the kind and arguments.** Before dispatch,
  validate the command's online class, issuing seat's role, agreed cheat
  permission, gameplay availability, bounded arguments and current session
  state. `Gameplay` remains lobby-only. A sender-side check is convenience,
  not enforcement: the relay is payload-opaque, and an unauthorized command
  accepted by all clients would produce matching hashes. Competitive rooms
  never grant cheat/debug capabilities. The developer phrase and mask-4
  developer commands cannot widen the stream command class or bypass the
  agreed permission, even if a local developer flag is already set.
  Invalid commands have the same rejection and no partial effects on every honest replica; presentation
  drains any diagnostics outside the tick.
- **The session validates the actors.** Phase 1 already admits only units the
  local owner owns (`Session.humanUnit`); it admits only units the entry's
  seat owns. Because validation runs inside the tick on identical state, a
  forged or stale command is refused identically on every client.
- **References survive the delay.** Pools reuse a freed slot at once and carry
  no generation [I5], and a command now waits a round trip before it applies,
  so a command names its unit by handle plus an allocation serial. A session
  counter assigns a fresh serial on each successful unit creation, including
  forced-slot and transfer creation; failure does not advance it. The serial
  never derives from mutable position, owner or type, and never silently wraps.
  Both it and the counter are serialized. A reference whose serial fails is
  dropped rather than redirected to the slot's new occupant. Targets use the same form and may
  belong to any seat. Publication identities are presentation bookkeeping
  and never validate a command.
- **Ordering is the stream's.** Commands and session events assigned to the
  same tick apply in stream order (§4.2). The session's command sequence,
  which tracked moves record in their order nodes, is numbered from stream
  position so it is the same everywhere.

**Visibility admission.** Casual rooms initially retain the researched
target-admission behavior (§15 Q9). Rated play needs a separate explicit
contract for direct targets, radar contacts, stale sight and area commands.
Testing only current visibility can reject an honest delayed order; trusting
a client's claimed earlier sight permits fabricated targets. Research the
existing target semantics, then specify a bounded history keyed to an
admitted observation tick and the competitive scheduler, or another verified
admission rule. Until that contract is approved and tested, ranked play is
blocked. Enforce any gameplay departure through the existing owning rule
interface; wire/session policy must not become a second gameplay registry.

### 7.3 Selection and control groups

Moving selection out of the session changes no gameplay — no phase reads it
— but it does change the status words the partial fingerprint hashes, so M2
moves the fingerprint locks once, by exactly the removed interface bits
(§16). The client keeps the selection, the build page and the visited bits,
and resolves every order's units from its own selection when the order is
sent, as dispatchers already resolve their actors from the committed frame
(DESIGN_INTERFACE_HUD_INPUT §3.7). Control-group assignment stays a seat
command because the group number is also computer-player state; recalling a
group is selection, and stays local.

### 7.4 Wire form

The session encodes and decodes command payloads, because it owns their
types. A payload is a kind byte followed by the kind's fields — integers as
variable-length values, unit references as handle and allocation serial, world
positions as raw 16.16 values. The encoding is versioned with the protocol,
an unknown kind is refused, and every kind has a round-trip and fuzz test.
The relay never decodes a payload; it limits payload size and rate.

Publish stable kind numbers, field order, signedness, ranges, collection
bounds and malformed-input behavior before implementation. These are wire
schemas, not Go struct layouts, `gob`, pointers or implicit enum ordinals.
Lengths are bounded before allocation and actor counts before dispatch.
The command size budget must accommodate the advertised maximum selection,
with any splitting preserving its declared order and tick semantics.

**Payload audit (2026-10-01, `193abfde`).** All 35 command kinds are
classified in §7.1. The schema must also settle what today's payloads leave
to the host that builds them:

- `Order` carries values the host computes from a local setting or gesture —
  the interface type that selects the resolution branch, and the
  assigned-position, tracked-move and area-target flags. The wire form
  carries each one, so no receiver consults a preference. Its staged count
  is a replay-staging field and stays off the wire.
- `MobileBuild` carries a site height the sender validated. Every receiver
  checks it against its own world instead of trusting it.
- `GroupAssign` carries complete membership (§7.1).
- `BuilderOptions` drops its owner field (§7.1).
- `CancelQueuedMove` names an order by the sequence the session stamped into
  its nodes at enqueue. With sequences numbered from stream position (§7.2),
  the client names the stream position its receipt reported.

**Implementation audit (2026-10-02, `fc032410`).** These are observations
about Nanolathe's current code, not additional retail findings. They refine
the schema and migration work in §16.2:

| Input or state | Current consumer | Required M2 treatment |
|---|---|---|
| Order position | `orders.ResolvePos` contains three fixed coordinates, `InterfaceType`, `HasFeature`, `IsWreck` and `FeatureResurrectable`; area orders carry another such value per target. | The schema accounts for every member, including any presently unused member. Separate captured gesture intent from claims about the world. Audit the latter's producers and resolution branches before deciding which are transmitted or recomputed; do not introduce a current-visibility requirement under the guise of decoding (§7.2). |
| Mobile construction | `commands.go` writes the supplied height into the new order and handles repeated-site cancellation before insertion. `AppendOnly`, `Queued` and `Facing` also change the result. | Carry each intent field. Validate the site/height through the owning placement contract before any unauthorized payload can purge or cancel orders. Preserve the researched repeated-click and rule-selected facing behavior. |
| Community queue drag | The receipt includes unit, list index, descriptor, creation tick, target, old goal, build product and facing; the command adds a publication `InstanceID` and new destination. | Replace the unit's publication identity with an allocation serial; retain the queue receipt's complete identity checks. Audit its target reference as well. A stream position alone cannot identify an order produced by a script or planner. |
| Group assignment | `applyHumanGroup` scans the owner's units and reads selected flags through `hud.AssignGroup`. | Capture the complete replacement membership before enqueue, including the empty set that clears the group. Preserve the operation's clearing of the same group on nonmembers. Recall/filter/Shift remain local. |
| Resource commands | `SetResource` and `Give` carry binary32 amounts and explicit player fields. Single-player `Give` takes its source from the own/controlling slot at drain time `[07 R-CAM-01 §6]` (the code through `41a09a60` still debits `ViewingOwner`; a separate correction moves it); `SetResource` may name another player. | The online source is the stamped seat; a gift still names its recipient. Preserve binary32 values explicitly in the schema, with the finite-value and range policy of §7.4.1. Online `Give` admits only a positive whole amount within its producer's range unless the room permits cheats (§7.1, Q8); `SetResource` stays cheat-gated. Keep the single-player source/target behavior and signed amounts. Integer-only encoding of amounts would lose existing inputs. |
| Local command metadata | Enqueue supplies `Sequence` and `DueTick`, copies actor/area slices, and drains due commands in queue order, including the single-player paused prefix. | Keep a local enqueue adapter and a distinct stamped-entry adapter. Neither payload may author its seat, due tick or sequence. Both reach one phase-1 implementation; the online adapter cannot invoke the paused drain. |
| Interface flags | Selection and BigBrother use selected/visited flags; build pages use their own status-word fields. `Unit.Group`, selectable/CTRL_F flags and readiness inputs have other consumers. | Inventory each bit before moving it. Remove only interface-owned fields; preserve group, eligibility and gameplay flags. Update the local selection from committed state, including transport readiness, death and reuse. |

The schema must distinguish a malformed or unauthorized payload from a
well-formed command whose actors have since died. It specifies rejection
granularity for each kind before implementation: stale references never
become references to replacement units, and a stale explicit target never
silently becomes an intentional targetless order. Codec rejection and
authorization rejection occur before order queues, resources or either RNG
can change. Once an authorized command reaches an owning gameplay service,
that service retains its researched ordering and failure behavior; for
example Community order drag can interrupt movement before its placement
test (DESIGN_COMMUNITY_PATCH §7; `[community patch engine behavior §5.11]`).

**Single-player replay context.** The online classification is not a list
of everything a single-player recorder may omit. `NoShake` changes the
single-player CRT draw schedule, and `Gameplay` can change its rule set;
both must be reproducible in M4. U0 publishes explicit single-player replay
schemas for such inputs, including any retained target-player fields that
online authorization forbids. Their decoding context comes from the agreed
session/replay kind, never from a payload's request to elevate itself to a
single-player command. Online admission refuses them. Pure selection,
page browsing and camera changes need no authoritative stream entry.

#### 7.4.1 Version 1 primitives and limits

The following are **Nanolathe protocol contracts**, not retail findings.
The command schema version is `1`, negotiated outside the payload. The
payload starts with one kind byte. Context is supplied by the admitted
session or replay header, never by that byte or another payload field.

| Notation | Encoding and accepted domain |
|---|---|
| `u8` / `bool` | One byte; a boolean is exactly 0 or 1. Enumerations accept only the listed values. |
| `u16`, `u32`, `u64` | Shortest unsigned base-128 varint, limited to the named width; reject overflow and overlong encodings. |
| `s32`, `s64` | Zigzag of the named signed width, then shortest unsigned varint. No narrowing through Go `int`. |
| `fixed` | `s64`, the raw value of `numeric.Fixed`. Its 16 fractional bits do **not** imply a 32-bit storage width. |
| `point` | X, Y, Z, each `fixed`, in that order. A decoder accepts the full representation. Preserve each owning command's coordinate semantics as specified below; there is no generic on-map test. |
| `ref` | Handle `u16`, then allocation serial `u64`. Both zero is null; exactly one zero is invalid. Non-null references have handle 1..65535 and serial 1..MaxUint64. |
| `actors` | Count `u16`, then that many non-null `ref` values in captured order. At most the agreed per-player unit limit; duplicate references or repeated handles are invalid. Empty means no actors, never implicit selection. |
| `key` | Byte length `u16`, then 1..255 bytes of the canonical content key returned by `content.CanonicalKey`, with no NUL. Preserve bytes above ASCII literally, as that function does; keys need not be UTF-8. Encode canonical keys; refuse noncanonical input on decode. An explicitly optional key permits zero length. This length ceiling is protocol admission policy: reject content whose command-addressable keys exceed it before ready. |
| `amount` | Four little-endian bytes of IEEE binary32. Online: finite, signed, and positive zero only (the encoder canonicalizes negative zero). Per kind, online `Give` narrows this further: without the room's cheat permission the amount must be a whole number from 1 to 2^31 (2,147,483,648), the positive values its only producer can generate — the typed signed 32-bit integer converted to single precision `[07 R-CAM-01 §6]`, whose largest value rounds to 2^31. With the cheat permission it may be any whole number from −2^31 to 2^31, zero included, and the signed transfer applies `[05 R-SHARE-01 §2]` (§7.1, §15 Q8); a value no producer can generate — not finite or not whole — is rejected in every online room (both §15 Q28, approved 2026-10-02). Single-player replay: preserve all 32 bits, including the existing local input's exceptional values. Never round through decimal or integer amounts. |
| `position` | `point`, InterfaceType `u8` (0 left, 1 right), HasFeature `bool`. The latter records the captured contextual feature intent. |

`ResolvePos.IsWreck` and `FeatureResurrectable` have producers but no
authoritative readers in the audited tree. They have no wire fields; the
adapter sets them false. `HasFeature` does select a resolver branch and
remains explicit. It does not prove a feature exists or confer visibility:
feature work still resolves through the owning world service. In particular,
do not replace the captured click with a new visibility test; that would
change delayed orders and belongs to the separately gated ranked contract.
`StagedCount` remains an internal replay-staging input, not a recorded
human command field.

Version 1 admits the existing startup unit-limit range **20..3276**. Ten
owner slices then contain at most 32760 units, within the positive signed
occupancy domain as well as `pool.Handle`. Definition limits are a different
quantity. Each area list permits **65535 entries**, independently of the
unit pool: feature targets are not unit allocations. Its count is `u16`;
each entry is `{Target ref, Position position}`. Entries, including repeated
entries, retain their captured order. A command permits at most **4 MiB**
(4,194,304 bytes); check this before parsing, then every count and remaining
byte requirement before allocating a collection. These are representation
ceilings, not a claim that every room can run the largest battle (§17).
An oversized gesture is refused visibly before submission; no implicit
split, truncation, next-tick continuation or area-to-click fallback exists.
The context-only codec checks the absolute 3276-actor ceiling; receiver
admission checks the smaller agreed unit limit and, online, the work limits
below before dispatch.

**Per-command work (§15 Q28, approved 2026-10-02).** The representation ceilings do
not bound the work one command asks of a tick. The area applier
(`applyHumanOrderBatch`, `internal/session/commands.go`) visits every actor
for every entry, and each visit can push one order node, while one unit's
queue holds at most 10,000 nodes before its guard drops the rest
(`orders.OOMGuardQueue`, `internal/orders/pump.go`). At the ceilings one
command could demand 3,276 × 65,535 visits. The client's producers generate
far less (`cmd/nanolathe/battle_command_drag.go`): the actor list is the
selection, at most the unit limit; a repair drag lists the issuing player's
own damaged or unfinished units inside the box, also at most the unit
limit; a reclaim drag lists the visible reclaimable blocking features inside
the box. Online admission therefore adds two **Nanolathe protocol limits,
not retail constants**:

- an area list holds at most **10,000** entries — the queue guard's
  capacity, so no admitted entry is one that no actor's queue could hold;
- the actor count times the entry count is at most **1,048,576** (2^20),
  which admits the largest repair drag at the default skirmish unit limit
  of 1,000 (1,000 × 1,000) and, for example, 104 builders reclaiming
  10,000 features.

An online command over either limit is rejected whole at the validation
boundary, before any queue, resource or random stream changes; it is never
split, truncated or continued on a later tick (M2-C5). The local sender
refuses such a gesture visibly before submission. The single-player replay
context keeps the representation ceilings, so a recording reproduces
whatever the local producer applied. U6 tests an area list of 10,000 entries
and one of 10,001; a product of exactly 2^20 (128 actors × 8,192 entries)
and the smallest buildable product above it in range (163 × 6,433 =
1,048,579; 2^20 + 1 itself needs 17 actors × 61,681 entries); and that each
rejected command leaves queues, resources and both streams unchanged. §17
measures the tick cost of a command at these limits before M6 offers them.

The largest `Order` is bounded even with maximum serials and signed
coordinates: one ref uses at most 3+10=13 bytes; one position at most
30+2=32; an area entry at most 45. At the representation ceilings — the
single-player replay context — the schema below therefore needs at most
`1 + 2 + 3276*13 + 1 + 2 + 32 + 3 + 3 + 65535*45 = 2,991,707`
bytes (the actor count uses two bytes at this admitted limit, and the outer
target beside an area list is the 2-byte null ref, §7.4.3). Under the
online work limits the largest admissible `Order` is an area order of 10,000
entries and 104 actors:
`1 + 1 + 104*13 + 1 + 2 + 32 + 3 + 2 + 10000*45 = 451,394` bytes; the
largest actor list beside an area list, 3,276 actors with 320 entries, needs
57,031, and an ordinary order of 3,276 actors, whose outer target is a full
13-byte ref, 42,641. (Corrected 2026-10-02 by U6's measured encoder: the
earlier sums counted a 13-byte outer target beside the area list, 11 bytes
too many; the conclusion is unchanged.) All other
kinds are smaller, including the 255-byte queue-receipt product. This
guarantees one maximum advertised selection and maximum admissible area list
fit in one atomic command. U6 tests these maxima, not just small examples.

#### 7.4.2 Kind and payload table

Numbers are explicit protocol constants. Existing numbers are retained for
recognition, but the codec must not derive them from `iota` or Go layout.
`S` means supported seat schema in M2; `D` means a reserved seat schema whose
online application is deferred to M5; `L` means local-only, with no online
or authoritative replay payload; `R` means single-player replay only.
All S and D commands also have a single-player replay form where the local
operation exists. M2 can test their codec without claiming D application.
Unimplemented contexts reject the kind before mutation.

Fields below are in wire order. An `actors` field replaces every selected-
unit fallback and the stance/cloak selection scan. Singular actors are
non-null `ref`. A target is nullable and may belong to any player. The
labels are the payload record names under `session.SeatCommand`; records
contain exactly the named semantic fields, using the primitive Go types
above (`[]pool.UnitRef` for actors, `numeric.Fixed` for fixed values).

| Number | Kind / class | Fields after kind byte |
|---|---|---|
| 1 | SelectionReplace / L | None; reserved, reject in codecs |
| 2 | SelectionToggle / L | None; reserved, reject in codecs |
| 3 | SelectionClear / L | None; reserved, reject in codecs |
| 4 | Order / S | Actors `actors`, Code `u8` 1..14, Target `ref`, Position `position`, Queued `bool`, AssignedPosition `bool`, TrackQueuedMove `bool`, Targets area list |
| 5 | Stop / S | Actors `actors` |
| 6 | Activation / S | Unit `ref`, Activate `bool`, Queued `bool` |
| 7 | MobileBuild / D | Builder `ref`, Product `key`, Position `point`, Facing `u8` 0..3, Queued `bool`, AppendOnly `bool` |
| 8 | FactoryBuild / S | Builder `ref`, Product `key`, Count `s32` (nonzero, bounded below) |
| 9 | CancelProduction / S | Unit `ref` |
| 10 | Stockpile / S | Unit `ref`, Count `s32` (nonzero, bounded below) |
| 11 | BuildPage / L | None; reserved, reject in codecs |
| 12 | GroupAssign / S | Group `u8` 1..9, Members `actors` |
| 13 | GroupRecall / L | None; reserved, reject in codecs |
| 14 | Stance / S | Actors `actors`, Fire `bool`, Value `u8` 0..2 |
| 15 | Cloak / S | Actors `actors`, Cloak `bool` |
| 16 | SelfDestruct / S | Actors `actors`, Queued `bool` |
| 17 | NoShake / R | No fields; toggles the single-player driver. Online uses a local visual preference only. |
| 18 | ATM / S, cheat | No fields; issuing seat's existing credit operation |
| 19 | SetResource / S, cheat | Resource `u8` (0 metal, 1 energy), Amount `amount`; replay adds Player `u8` 0..9 **before** Resource |
| 20 | SetLogo / R | Player `u8` 0..9, Logo `u8` 0..255; online is a local override only |
| 21 | View / D, cheat | Player `u8` 0..9 |
| 22 | Give / S; cheat for any amount but a positive whole one | Player `u8` 0..9 (recipient), Resource `u8` (0 metal, 1 energy), Amount `amount` with `Give`'s per-kind bound (§7.4.1; the range under the cheat permission is §15 Q28) |
| 23 | MakeSelectable / S, cheat | No fields |
| 24 | Visibility / D, cheat | ToggleMask `u8` 0..7, ClearMask `u8` 0..7; overlapping bits retain toggle-then-clear semantics |
| 25 | DoubleShot / D, cheat | No fields; toggles the issuing seat's own gate (§6.3 "Machine option bits") |
| 26 | HalfShot / D, cheat | No fields; toggles the issuing seat's own gate (§6.3 "Machine option bits") |
| 27 | Meteor / S, cheat | ArgumentPresent `bool`, Enabled `bool`; Enabled must be false when ArgumentPresent is false |
| 28 | BigBrother / L | None; reserved, reject in codecs |
| 29 | ShiftState / L | None; reserved, reject in codecs |
| 30 | CancelQueuedMove / S | Sequence `u64` 1..MaxUint64, Actors `actors` |
| 31 | Spawn / S, cheat | Unit `key`, Position `point`; still requires the owning rule's existing spawn permission |
| 32 | BuilderOptions / S | Guard[0..2], then Patrol[0..2], six `u8` values each 0..2; replay adds Owner `u8` 0..9 first |
| 33 | CommunityOrderDrag / D | Unit `ref`, Index `u16`, DescriptorID `s32`, CreationTick `u32`, Target `ref` (must be null in v1), Goal `point`, BuildProduct optional `key`, BuildFacing `u8` 0..3, Destination `point` |
| 34 | CommunityKickout / S | Unit `ref`, Destination `point`; requires the existing Community feature |
| 35..43 | ShareMetal, ShareEnergy, ShareMapping, ShareRadar, ShareAll, SetShareMetal, SetShareEnergy, ShareGift, DeclareAlliance / D | Reserved in the listed order; no v1 payload, reject even when a receiver has local helpers |
| 44 | SharedVictory / D | Reserved; no v1 payload |
| 45 | ShootAll / D, cheat | Reserved; no v1 payload |
| 46 | DeveloperSpawn / R | Pattern `key` (wildcards allowed), Owner `u8` (the typed integer’s low byte), Position `point`; accepted local developer submission, replay only (DESIGN_DEVELOPER_TOOLS §8). Online codecs and receivers reject it. |
| 255 | Gameplay / R; lobby-only online | Mode name `key`; resolve through the existing registered rule sets, never a payload-defined registry |

Every unlisted number is invalid. New sharing/alliance schemas require M5's
owning service contract and a new supported command-schema version; reserving
their numbers does not authorize guessed threshold or gift arithmetic.
`GroupAssign` has no Preserve or filter Mask: those belong only to recall.
Replay `Give` takes its source from the own/controlling slot, resolved at
drain time `[07 R-CAM-01 §6]`; online uses the stamp. Replay `SetLogo`,
`NoShake`, `View`, `BuilderOptions` and `Gameplay` retain their existing
authoritative effects and local authorization. Online context cannot
request these replay-specific fields or bypass cheat checks.
Replay `Give` keeps signed amounts and the researched transfer direction,
the negative-amount edge included `[05 R-SHARE-01 §2]`. Online `Give` is
the Nanolathe online policy of §7.1 and Q8: a positive whole amount within
its producer's range is ordinary sharing for every seat, and any other
amount the producer can generate needs the room's cheat permission, under
which the same signed transfer applies. Neither context replaces the
transfer with a different gift operation. SetResource still requires the
cheat permission. `DoubleShot` and `HalfShot` are D because their per-seat
gates need M5's per-seat state (§6.3); until then a receiver rejects them
before mutation, as for every unimplemented context.

Online counted production accepts -32767..-1 and 1..32767. This is a bounded
command-input policy, not a new queue capacity; the gameplay producer still
owns accumulation/cancellation. The local adapter normalizes Count=0 to 1
before recording. Single-player replay accepts other nonzero signed-32
counts except MinInt32 (whose negation is not representable), retaining
existing valid local producers. Never coalesce or split counted commands.

`AssignedPosition` accepts only a single actor, Code=2, null Target, empty
Targets and TrackQueuedMove=false. `TrackQueuedMove` requires Code=2,
Queued=true, null Target, empty Targets and AssignedPosition=false.
An area list requires null outer Target and both special flags false;
the ordinary Code and Queued retain their existing meaning. The outer
Position remains encoded for fidelity even when that path does not use it.
These conditions describe current gesture producers, not new gameplay.

#### 7.4.3 Application and stale data

Validate the complete payload and issuer first. A live foreign actor rejects
the **whole command**, before any friendly actor mutates. A dead or
serial-mismatched actor is stale, not foreign; remove stale actors while
preserving the relative order of survivors. Empty survivor lists do nothing
except `GroupAssign`: its complete surviving membership, including empty,
replaces that group's membership among the issuer's units. Capability gates
then run in the existing gameplay order. In particular `SelfDestruct` keeps
its whole-selection cancellation pass before deciding whether to issue.
Ordinary `Order` actors are a set visited in ascending handle order, as the
current consumer sorts them; its encoder emits that order and decoder refuses
an unsorted list. Area orders preserve actor order as well as target order.
The local adapter deduplicates ordinary captured selections before encoding;
the wire never accepts duplicates. Stance/cloak producers capture their
former ascending selection scan in that same order.

A stale singular actor makes that command a no-op. A stale ordinary explicit
target makes the entire order a no-op; it must not turn into a ground click.
For an area order, drop only stale explicit-target entries, keep targetless
feature entries and their order, and retain area semantics even when no
entries survive. A changed queue receipt makes the drag a no-op. Compare its
entire receipt before interruption; no publication `InstanceID` is accepted.
Current draggable orders are targetless, so Target must be the null ref;
expanding that set requires a future schema and target-lifetime contract.

**Single-player stale targets differ today.** These stale-target rules are
the online seat-command contract. Today's single-player applier does
otherwise for an ordinary order (the order case of `applyHumanCommand`,
`internal/session/commands.go`): when the named target has died, the order
resolves as a ground order at the captured position and its node still
carries the dead handle. M2's declared single-player changes cover only
the removed interface bits (§7.3), so U2 preserves that single-player result
through the local adapter and its replay context unless a cited research
section establishes what retail does. This document does not decide the
retail answer. Any change to the single-player result is listed with its
fingerprint evidence under M2-C7, beside the interface bits.

Mobile build is D because its known-site admission needs the issuing seat's
M5 perspective. Its receiver must derive the canonical placement centre and
height through the owning placement service and reject mismatching input
before queue mutation, including the owning known-site/occupancy check for
both a new click and cancellation. Once placement is admitted, keep the
existing repeated-click match and cancellation before insertion; a delayed
click whose site is no longer admissible does nothing. Community drag retains interruption **before** its
gameplay placement test once the payload, ownership and receipt are valid.
Neither command may borrow a client's viewer or temporarily swap `LocalOwner`.
`View` and `Visibility` likewise await M5 perspective/history application.

Order and CommunityKickout retain their existing coordinate semantics,
including targetless points outside the map; do not add build-placement or
current-visibility gates to them. U2 audits their arithmetic/indexing for
the full accepted representation and rejects unrepresentable intermediate
values before mutation rather than narrowing accidentally. Spawn retains
`spawnCommandPlacement`: X/Z cell coordinates must be within terrain bounds,
then the existing mission position fixup; it does not acquire the ordinary
build site's occupancy test. MobileBuild and Community drag use the placement
predicates above, at their distinct specified points in application.

Malformed encoding, forbidden kind/role, invalid scalar/combination, missing
content key and live foreign actors reject before any gameplay service, queue,
resource or RNG mutation. Authorized gameplay failure keeps the owning
service's partial-work semantics. Every admitted stream entry consumes its
position even if rejected or entirely stale. A receipt distinguishes rejected,
no-op and applied; it does not claim every actor achieved the requested order.

#### 7.4.4 Public boundary for U1, U2 and U6

The following names and signatures are the shared contract; implementations
stay in their owning packages. `pool.UnitRef` is only a value type, not a
generation added to the allocator. `frame.UnitView` adds `AllocationSerial
uint64` beside Slot; `InstanceID` remains presentation-cache identity.

```go
// internal/pool
type UnitRef struct { Handle Handle; Serial uint64 }

// internal/units
// Unit adds AllocationSerial uint64. World owns the battle counter.
func (w *World) Reference(h pool.Handle) pool.UnitRef
func (w *World) LookupReference(r pool.UnitRef) *Unit
func (w *World) LastAllocationSerial() uint64

// internal/session
type CommandContext uint8 // explicit constants: OnlineCommand=1, SinglePlayerReplay=2
type SeatCommandKind uint8 // explicit numbers from the table
type SeatCommand struct { /* Kind plus one named typed payload from the table */ }
type CommandStamp struct { Seat uint8; Tick uint32; Position uint64 }
type CommandReceipt struct { Stamp CommandStamp; Outcome CommandOutcome; Diagnostic string } // Diagnostic: the rejection's text, empty otherwise (added by U2)
type CommandOutcome uint8 // explicit constants: CommandApplied=1, CommandNoOp=2, CommandRejected=3
func EncodeSeatCommand(context CommandContext, c SeatCommand) ([]byte, error)
func DecodeSeatCommand(context CommandContext, payload []byte) (SeatCommand, error)
func (s *Session) EnqueueSeatCommand(stamp CommandStamp, c SeatCommand) error
func (s *Session) DrainCommandReceipts() []CommandReceipt
```

The tagged value has `Kind` and payload fields named exactly as the table's
kinds (fieldless kinds need no payload record); each payload type is named
`<Kind>Payload`, with the table's field names, widths and order. `Actors` and
`Members` are `[]pool.UnitRef`; `Position` is `CommandPosition` containing
`X,Y,Z numeric.Fixed`, `InterfaceType uint8`, `HasFeature bool`. Plain points
use `CommandPoint { X,Y,Z numeric.Fixed }`. `Targets` is
`[]CommandTarget { Target pool.UnitRef; Position CommandPosition }`.
Replay-only Player/Owner fields are retained in their typed records, must be
zero in online values, and are absent from online bytes. Unselected payload
records must be zero, preventing silently discarded data. Go arrays represent
Guard/Patrol, and `gameplay.Mode` represents the typed Gameplay choice; only
its registered name is encoded. Resource fields use `economy.Res` with the
explicit wire mapping above. Amount is `float32`, Count is `int32`.

`Reference` returns null for an absent/dead unit; `LookupReference` returns
nil for null, dead or mismatched references. A new World starts at counter
zero. Allocate the serial at the successful-creation boundary before
`OnCreate`; check exhaustion before allocation or creation RNG work.
Creation failures leave the counter unchanged and preserve the allocator's
existing failed-creation draws. Only the future exact snapshot restore may
set the counter/live serials; no public setter permits reuse. A retail-save
load creates a new session; discard prior commands and local reference maps
even if the same numeric pair appears in that new namespace.

The session's admitted kind chooses context; callers cannot pass context to
`EnqueueSeatCommand`. It copies variable payload storage, validates stamp
order/bounds and queues for phase 1. Stamps have seat 0..9, nonzero position,
and a future unsealed tick; the stream driver owns grants and battle identity.
Position gaps for non-command entries are legal; repeats/backward positions
are not. Queue-time errors do not advance simulation, and phase-1 rejection
still yields a receipt. Drain receipts outside the tick and deep-copy them.
Keep `EnqueueHumanCommand` as the single-player compatibility adapter while
migrating producers: capture explicit references/membership at submission,
retain paused-prefix behavior only there, and call the same phase-1 payload
implementation. Tracked-order Sequence uses the accepted stream Position;
an unacknowledged local client sequence is never a cancellation target.

## 8. Battle configuration and identity

### 8.1 What the seats agree

The agreed configuration includes every battle-entry input and the match
policies that constrain its participating clients:

1. **The skirmish configuration**, `session.SkirmishConfig`
   (`internal/session/skirmish.go`): map; the seat rows (nickname,
   controller, side, colour, ally group, starting metal and energy, Classic or
   Modern); difficulty, which online is each computer row's own and never
   a battle-wide word (§6.6, §8.6, §15 Q28); start-location
   mode; commander-death mode; mapping, line of sight and its type; unit
   limit; Survival options. The relay fills the two seed fields (§8.3).
2. **The match selection** of DESIGN_MODS_MUTATORS §3 — the mod's id,
   version and archive SHA-256, the content configuration, the rule set's
   name and base, the Community sources and entry table, and the mutators.
   That document already designs the selection "to be the future handshake".
3. **The entry options** battle entry reads beside the configuration
   (`session.SkirmishEntryOptions`): each option that can change the battle
   is either part of the agreed configuration or held at its default online.
   `AutomatedPlayers`, the arena's switch, is never set in a lockstep battle.
4. **The seat assignment**: which connection owns each human row.
5. **The remaining online configuration**: computer host seats; team symbols
   and starting shared-victory bits; explicit per-computer difficulty
   under Q23; unit restrictions, cheat permission, watching permission, pause/drop policy,
   scheduling policy,
   spectator/replay release policy and the match view policy (§8.4).
   Its canonical encoding is versioned and covers every effective field,
   including defaults. No battle-affecting option may remain an unagreed
   machine-local default.

Same-team pairs start mutually allied with shared victory enabled for teams
of two or more; a single team holding every player prevents START
`[08 R-SKIR-01 §3]` `[05 R-SHARE-01 §1]`. Q22 and Q23 define the shared
alliance representation and computer admission/difficulty policy explicitly
(§6.6–§6.7).

Audio volume, key bindings and ordinary renderer quality preferences remain local.
Presentation settings that the room restricts for play, such as tactical
zoom-out, are agreed match policy instead (§8.4). The client computes its
effective local preferences within those common limits.
Classify information/coverage controls as match policy even when their current
implementation lives under presentation settings; do not use that package
boundary as a reason to omit a play-affecting setting from the agreement.

Today the session seats exactly one human: `LocalOwnerForConfig` picks the
last human row, and phase 5 re-derives the local owner from the controller
rows every tick. A lockstep battle has several human rows, each owned by one
seat, and the local seat becomes a presentation fact (§6).

### 8.2 Identity

Before a seat may ready, its client and the relay compare the following
identities. Protocol version, simulation build identity, content identity
and configuration identity are separate fields with separate diagnostics:

| Identity | Source | Rule |
|---|---|---|
| Protocol | published envelope and command schema version | Exact supported version; reject unknown required fields or kinds. |
| Simulation build | release manifest covering source identity, pinned dependencies/toolchain and simulation-relevant build choices, plus the tested platform variants | One admitted Nanolathe release initially. A VCS revision, `vcs.modified` boolean or `version.Profile` alone cannot identify source changes. Reject unstamped/dirty builds from normal rooms; development rooms require an explicit common build manifest. Platform binaries may differ, but their cross-platform equivalence must pass §17. |
| Content | a new simulation-content digest covering the complete effective inputs described below | Exact match, with missing/differing logical inputs reported before ready. `Catalog.Hash` alone is insufficient. |
| Map | SHA-256 of the map files the battle reads | Exact match. |
| Rules | rule set name and base; `community.Features.Digest` | Exact match. |
| Mod | id, version and archive SHA-256 | Exact match; a client missing the host's mod may fetch it through the existing catalogue (§15 Q14). |
| Configuration | SHA-256 over a versioned canonical encoding of every effective field in §8.1 and §8.4, including seeds, entry options and defaults | Exact match; all configuration changes invalidate readiness. `SkirmishConfig.NormalizedBytes` is an existing equivalence helper, not the complete wire/configuration schema. |

**Complete content identity.** Hash canonical compiled definitions and their
ordering, scripts for every admitted unit (including units not initially in
the world), authoritative model geometry and derived heights, simulation
animation metadata (`SimArt`, which after M1 includes the per-frame holds of
every effect entry an event can name, L9), map/mission inputs used by this
session kind, AI data, extension tables and mutators. Include missing-input
states where the loader has a defined fallback. Audit all composition and later unit
creation reads; a file's name is not its contents. Render-only pixels and
unrelated installed files need not match, but a resource must be classified
from its consumers, not assumed cosmetic because it is a model or animation.
Content is frozen for the battle after admission; changing a loose resource
cannot silently alter subsequent unit creation.

The existing catalog hash deliberately omits `UnitDef.Script`, `ModelTop`
and `ModelTopFixed` (`internal/content/compile_unit.go`), and `SimArt` is
compiled separately. Retain that hash's existing regression contract; add
the complete network identity rather than silently changing what it means.
Record source/build diagnostics separately: an identity reported by a client
is compatibility information, not proof that it is unmodified (§13).

**A simulation change is a compatibility change.** Two releases that differ
in any authoritative behaviour cannot share a battle, and a replay plays
only on the release that wrote it (§10). The engine's simulation changes
often, so this is an operating cost, not an edge case. Rooms are listed by
simulation build, the client says plainly which release a room or a replay
needs, and a release is published only after the cross-architecture and
host-kind suites of §17 pass on it. The installer already builds each
release from pinned source with a pinned toolchain and cleared architecture
settings (`tools/installer`), which is what makes "the same release" a
checkable statement on its six platform targets. Whether it also keeps
earlier releases for old replays is §15 Q20.

The archive manifest hash (`vfs.FS.ManifestHash`) is not used: it covers paths,
providers, priorities and sizes rather than bytes, and it differs between
installs that carry unrelated extra archives.

**The initial digest.** After composing the battle and before tick 1, every
client reports the complete digest (§9) of its composed state, and the relay
grants tick 1 only when all agree. This catches divergent composed state,
but cannot replace content identity: changing an unbuilt unit's script may
have no effect until minutes later, and definitions are referenced by key
in a snapshot. Both checks are required.

### 8.3 Seeds

The relay draws the seed pair from `crypto/rand` when it freezes a start
proposal, before the final ready acknowledgements, and sends it with that
proposal (§12.2). Clients pass it through the existing explicit
handoff (`FreshBattleRequest.SimulationSeed` and `CRTSeed`), which is already
"the only RNG handoff into composition" [I6]. The relay is not an
authoritative package, so the authoritative ban on `crypto/rand` is
untouched. Single-player battles keep choosing their own seeds as they do
today (`cmd/nanolathe/main.go`); a replay records the pair it used.

### 8.4 Match-wide view restrictions

The match has one effective configuration for every setting that affects
simulation or that the room restricts for fair play. **Maintainer requirement
(2026-10-01):** the host can, for example, disable zoomed-out tactical play
for all players. This is a shared view restriction, separate from gameplay
rule selection and from each player's ordinary graphics preferences.

The match view policy specifies permitted tactical zoom bounds and
whether the full-map tactical view is available. A no-zoom-out preset uses
the camera's existing native 1× boundary as its minimum permitted tactical
scale, regardless of a player's custom zoom lock. It blocks equivalent
zoom-out routes through the wheel, keys, battle-entry flags, settings changes
and renderer switches; it also disables an otherwise available Community
megamap/full-map tactical route. The ordinary minimap remains available.
Its controls and sight limits retain the selected rule set's contract.
The final field encoding and UI wording belong to M2/M6; do not implement
this as an untransmitted local `settings.Presentation.Zoom` preference.

Every player sees these restrictions in the lobby. They participate in the
configuration hash, readiness, reconnect and replay metadata, remain fixed
during the battle, and constrain the effective settings at every relevant
input/presentation boundary. The client preserves personal preferences and
restores their unrestricted effect when leaving the battle. A client unable
to honor the policy refuses ready rather than silently substituting a mode.
Use the existing camera/presentation controls; do not add a gameplay seam.

A scale limit is not an equal-visible-area guarantee: monitor resolution
and aspect ratio can still change the world area shown. Competitive policy
must explicitly accept that difference or define and test a common maximum
world viewport, including resize/fullscreen changes. No such viewport size
is invented here. Likewise, disabling strategic icons alone would not
disable zoom-out. §17 verifies the actual permitted view, not just a toggle.
Spectator and replay views have separately declared privileges; a player
who is still in the match cannot acquire them by switching local UI mode.

These controls constrain ordinary Nanolathe clients. A modified lockstep
client already has the whole world and can bypass its camera restrictions
without changing simulation hashes. The lobby must not present equal settings
as an anti-cheat guarantee; competitive integrity retains the limits in §13.

### 8.5 Other engines

The implementation target is Nanolathe-to-Nanolathe play. There is no
cross-engine milestone, compatibility certification scheme or promise to
relax the initial release gate. Reconsider other engines only as a separate
future decision; support may be dropped entirely.

An open wire protocol alone would not supply cross-play. Lockstep peers
must also agree on content interpretation, arithmetic, random draws, phase
ordering, allocation, scripts, AI and every gameplay rule. A translator
cannot generally reconcile different simulations. For context, RWE's own
[FAQ](https://www.robotwarengine.com/faq/) describes TA data compatibility
alongside selected behavior changes; Recoil's
[netcode overview](https://recoilengine.org/articles/netcode-overview/)
describes input-only lockstep and its determinism requirements. These are
**Established external descriptions**, not retail evidence or evidence of
compatibility with Nanolathe. No external engine is admitted by this design.

The protocol remains documented for Nanolathe clients, relays and tools.
Using explicit schemas rather than Go memory layouts is sufficient hygiene
now; it does not commit the project to a general engine interoperability
layer. The distinct future state-delivery architecture in §12.6 serves the
competitive requirement and likewise promises no cross-engine adapters.

### 8.6 Effective configuration contract for U5

Configuration version 1 is a fully explicit value. Resolve host preferences
once, then validate, freeze, encode and compose **that same value**. Decoding
never calls `SkirmishConfig.ApplyDefaults` or `Normalize`: their private
missing-value state and zero-resource defaults can change a deliberately
selected Easy, randomized, off or zero choice. A local request adapter may
use those functions before freezing; a decoded effective configuration may
not. Unused rows and irrelevant fields are canonical zero values, not a
second source of defaults.

Use §7.4.1 integer/bool encodings. `text(N)` means `u32` byte length then
valid UTF-8 without NUL, at most N bytes; lengths are checked before
allocation. `digest` is exactly 32 bytes, `id` exactly 16. Collections below
have a `u32` count and the stated order; reject duplicate or unsorted map
keys. The complete configuration is at most 2 MiB. Its identity is SHA-256
of the literal UTF-8 domain `nanolathe/match-config/1` followed by the
following positional encoding; the domain has no trailing NUL. There are
no optional unknown fields or trailing bytes. A field addition requires a
new version, even if its Go zero value would appear harmless.

| Order / effective field | Representation and normalization |
|---|---|
| 1. SessionKind | `u8`: 1 online skirmish, 2 online Survival. Campaign and single-player recording headers have separate admission; this constructor cannot create them. |
| 2. RuleName, RuleBase | `key`, then `u8` (1 Strict 3.1, 2 Modern, 3 Community 3.9). Custom named sets use their registered base; resolve through the existing registry, reject unknown names or disagreement rather than normalizing an unknown name to Modern. |
| 3. MapName, MapSchema | Canonical logical map `text(1024)`, selected schema `u32`. Selection is resolved by the existing map-entry code, not a client-local rescan at composition. |
| 4. NumPlayers, Players | `u8` 2..10, then exactly that many rows in slot order (row format below). Clear rows beyond this count. At least one human; watcher admission requires WatchingAllowed. Survival keeps its attacker-last and survivor-alliance invariants and permits multiple human survivors; do not import the single-player convenience constructor's human/computer menu layout into online admission. |
| 5. Location, CommanderDeath, Mapping, LineOfSight, LOSType | Five `u8`: location 0 randomized/1 identity; commander-death 0..2; the last three 0..1. There is no battle-wide difficulty word (§15 Q28): each added computer's row carries its own difficulty (row field 2), and every reader of today's global word takes the computer seat it concerns (§6.6). The single-player `SkirmishConfig.Difficulty` is not encoded, and composition must not read it for an admitted online battle. |
| 6. UnitLimit, SimulationSeed, CRTSeed | `u16` 20..3276, then two `u32`. The effective Community unit-limit override has already been applied. Seeds are explicit, including zero if supplied; composition uses the existing seed constructors. |
| 7. Survival | Pace `u8` (0 normal, 1 relaxed, 2 relentless), NoAir `bool`, NoNaval `bool`. All zero for SessionKind=1; no redundant Enabled bit. |
| 8. Mod | ID `text(255)`, Version `text(255)`, Archive `digest`. All empty/zero for base content; a selected mod requires all three. These are public mod identities, not local archive paths. |
| 9. Content profile | Name `text(255)`, at most 64 directory pairs `{From text(255), To text(1024)}` in canonical logical-key order, then effective Units `u32`, Weapons `u32`, TNTBytes `u64`, LOSBytes `u64`. Units 1..65536, Weapons 1..MaxInt32, byte caps 1..MaxInt64; use content defaults 512/256/16 MiB/1 MiB for absent local inputs. Reject redundant identity directory mappings. These are metadata limits, never remote instructions to allocate those sizes. Validate against the locally admitted content profile before loading/allocating. Detect markers and presentation defaults are not simulation fields. |
| 10. Community | Length `u32` and at most 16 KiB of the existing canonical `community.Features` JSON used by `Features.Digest`; that closed field vocabulary, integer validation and named `RepairRate` subrecord are the version-1 schema. Encode the resolved value, refuse unknown fields and require exact re-encoding. Strict requires exactly the zero feature value, a separate case from non-Strict complete-table validation (whose repair multipliers must be positive). Preserve that existing digest contract; source-layer spelling/provider paths are diagnostic only. |
| 11. Mutators | Eleven `u8` step indices in this order: BuildSpeed, BuildCost, Health, Damage, AreaOfEffect, Sight, Radar, Income, Salvage, FireRate, UnitSpeed. Indices 0..7 mean ¼, ½, ¾, 1, 1½, 2, 3, 4; missing/zero Factor and 1/1 both encode 3. Apply once to the battle's catalog clone. |
| 12. Unit restrictions | At most 65535 records `{DefinitionID u16, Unit key, Limit u8}` in ascending nonzero definition-ID order. `DefinitionID` is the record's index in the unrestricted compiled catalog and `Unit` its canonical name key; `Limit` 0 removes the record from the battle catalog and 1..100 caps each player's records of it at the allocator `[05 R-SHARE-01 §8]` `[05 R-SHARE-01 §9]`. Omitted means unrestricted, not zero, and nothing is seeded: a `wacky` definition is restricted only by an explicit record. The value is `content.Restrictions` mapped as [DESIGN_MODS_MUTATORS §15.3](DESIGN_MODS_MUTATORS.md#153-names-and-resolution) states — every record carrying a named key present with one limit — and composition applies it to the catalog clone before the mutators (§15.4 there). Verify each key against its immutable record, preserving duplicate-name identities; reject duplicate IDs, a partial name group, `norestrict` definitions and the removal of a side's commander. Online enforcement begins with the first online lobby (§16.6, Q16 brought forward 2026-10-08), which carries the host's set. |
| 13. Permissions | CheatsAllowed `bool`, WatchingAllowed `bool`. Neither local developer state nor interface preferences add permissions. GameClosed is room admission and excluded from battle identity. |
| 14. View | Player, Spectator, Replay view records, each `{MinimumScale u16, MaximumScale u16, FullMap bool}`; 64 ≤ minimum ≤ maximum ≤ 2048 in existing 1/1024 zoom units. Native-no-zoom-out uses minimum 1024 and FullMap=false. Enforce camera's map-dependent feasibility separately in M6; ordinary minimap is unaffected. |
| 15. Online policies | Policy revision `u16` =1, Scheduling `u8` =1 (casual earliest-unsealed tick), Pacing `u8` =1 (normal speed/no pause), Drop `u8` =1, Audience `u8` =1 (§11.4). Drop policy 1 is: retain a seat out of play idle, allow a removal vote once it has been out of play for a cumulative RejoinGraceMilliseconds, and on a passed vote apply the mode's final-removal rule (§11.1); its vote window, cooldown and other details are fixed protocol values of policy 1, not configuration fields (§15 Q28). Explicit RejoinGraceMilliseconds `u32`, never a removal timer, has three uses (§11.1): the cumulative time a seat must have been out of play — dropped or rejoining — since it was last active before a removal vote may be called against it; the time an admitted rejoin has to complete before the seat returns to the drop state; and the wait before a battle with no active playing human ends without a result (the last two §15 Q28). Then SpectatorDelayMilliseconds `u32`, ReplayReleaseDelayMilliseconds `u32`. No implicit timeout is invented here: the room must supply these values before ready; M6 validates service bounds and enforces them. Competitive or alternative policies are unsupported. |

Each player row is positional:

1. Role `u8`: 1 human, 2 computer, 3 watcher, 4 Survival scenario attacker;
   Side `u8` (0..min(admitted side count−1,255)), Color `u8` 0..9, AllyGroup `u8`
   0..5, Nickname `text(16)`, Metal `s32`, Energy `s32`. Finalized resources
   are nonnegative; do not replace explicit zero with 1000. Protocol names
   are valid UTF-8 within 16 bytes; local byte-truncated invalid names must
   be corrected before ready, not hashed differently by platform.
2. Participant `id`, HostSeat `u8`, ComputerKind `u8`, Difficulty `u8`.
   Human/watcher participant IDs are nonzero and unique public opaque IDs,
   not credentials. Others use zero. Added computers have HostSeat 0..9
   naming an initially human row, ComputerKind 0 Classic/1 Modern, and
   difficulty 0..2, the only difficulty an online battle has under the
   rule of §6.6 (§15 Q28). Other
   roles use HostSeat=255, ComputerKind=0, Difficulty=0, a required zero
   that no reader consults; the scenario attacker is not an added computer
   counted by Q23. Validate Strict's one-computer cap through the owning RuleSet;
   reject computers in Deathmatch. No socket, token or machine name enters
   the value.
3. SharedVictory `bool`, then the six Guard/Patrol bytes of §7.4.2.
   Teams initialize Q22's directed matrix from the agreed team rows; there
   is no independently editable initial matrix. Same-team shared victory
   and the all-in-one-team refusal remain the owning entry checks.
4. EffectiveAIParams: at most 128 `{Key text(32), Value text(32)}` pairs in
   lexical key order, at most 8192 encoded bytes per row. Resolve existing
   All → difficulty → player precedence; validate names/values through
   the controller's owning validator. Do not flatten to guessed defaults:
   omitted and explicit values are equivalent only when that controller's
   contract says so. Preserve applicability of effective parameters to
   existing managers until the consumer migration proves them irrelevant.

The row, pair and string bounds above are protocol admission limits, not
retail claims. Every `SkirmishEntryOptions` member is accounted for:
BuilderOptions becomes the row's six values; CommunitySources becomes the
resolved table; ContentLimits and Mutators are explicit above; AIOverrides
becomes the merged per-row values; Restrictions become field 12's records
through `MatchUnitRestrictions` against the unrestricted catalog, and a set in
the options must be the set those records describe (§16.6). AutomatedPlayers
is false. Progress is
local and excluded. SimArt is in frozen content identity; pointer presence
is not a configuration distinction. Presentation profile recommendations,
local provenance and override-layer text do not enter the identity.

U5 must not import `mods/aikit` into session to validate AI parameters (that
would cycle). Put the controller's vocabulary validator in its existing
lower owning AI package and reuse it from both the mod and admission.
M5 must give computers distinct effective profiles: the current shared
`*ai.Profile` and global difficulty mutation cannot implement Q23 merely by
adding fields to the hash. Until those consumers land, an admitted config
can be inspected/compared but cannot start a multi-seat battle.

### 8.7 Frozen content and build contracts for U4/U5

Capture sources **before** compiling the catalog. `content.SimulationSources`
owns that closed source snapshot; catalog compilation, map/schema resolution
and rule/mutator preparation consume its read-only VFS. Then
`content.SimulationInputs` clones the prepared catalog and freezes SimArt, all admitted unit
scripts and authoritative models, and retains the selected map/schema and
AI inputs. Its VFS view never falls through to live providers, including
failed/missing lookups. Late creation consumes only this view and the frozen
compiled objects. A second battle reads/revalidates provider bytes before
reusing a parsed cache; path/provider metadata alone cannot validate cached
content after a loose-file edit. The capture covers the known simulation
resource families and the selected map's inputs, using the existing loader
discovery order and recording misses. It exposes no arbitrary resource
registry. Supplied catalog/SimArt values from another capture are rejected;
neither a previously cached catalog nor a matching path is provenance for
this capture. U4 must carry the capture identity through compilation and
preparation. Do not reread live sources to "validate" an old compiled value
and then continue using it.

Manifest entries are `{Family u8, Key text(1024), Ordinal u32,
Presence u8, SemanticDigest digest}` in family, ordinal, key order.
Presence is 0 defined absence, 1 present, 2 owning-loader fallback;
fallback entries also contain the resolved fallback key `text(1024)` (empty
for states 0/1). Families are explicit: 1 catalog, 2 COB, 3 model,
4 simulation art, 5 map, 6 AI, 7 extension/mutator inputs. Ordinal preserves
record identity where names repeat. Hash domain `nanolathe/sim-content/1`,
then entry count `u32` and entries. Diagnostic provider/path/byte-size data
is returned separately and excluded. This manifest composes semantic family
digests; it does not reinterpret `Catalog.Hash` as complete.

| Family | Complete identity/consumer requirement |
|---|---|
| Catalog | Unit record-ID order, including duplicate names; weapon slots; side order; feature/movement/category definitions; build membership/download placement order; LOS and meteor definitions; compiled SightShapes from the authored visibility-mask GAF. Keep the current regression hash unchanged. U4's family encoder inventories every compiled field read by simulation, including scripts and derived model heights omitted by that hash. |
| COB/model | Every admitted unit's resolved program and missing/fallback state, including units not yet created; parsed model hierarchy, piece origins, authoritative geometry and derived heights. Compile/bind from the frozen result. Never hash only a filename. |
| SimArt | Canonical sequence names and defined misses, ordered frame geometry/holds and feature animation metadata consumed by simulation. Nil-versus-present cache pointers with equal effective content agree. |
| Map | Exact selected OTA/TNT inputs and schema used by entry, plus any scenario inputs actually consumed. `Map` identity remains separately diagnosable in admission. |
| AI | Selected profile and default-fallback state; preserve ordered directives and argument order. Runtime `ai.LoadProfile` rereads authored directives whose repeated multipliers do not commute; unordered `AIProfile.Plans` is insufficient. The frozen VFS supplies these reads without a content→ai import. |
| Extensions/mutators | Effective Community table and the applied mutator vector, tied to the prepared clone; never apply a mutator a second time when composing admitted content. |

The common build manifest uses domain `nanolathe/sim-build/1` and these
fields in order: SourceTree `digest`; GoVersion `text(64)`; GoMod and GoSum
digests; module count (at most 4096) and `{Path text(1024), Version text(255),
Sum text(255)}` sorted by path/version; build tags (at most 64 keys, sorted);
GOEXPERIMENT `text(1024)`; CGOEnabled `bool`; ordered build arguments
(at most 64 `text(1024)`); variant count (1..16) and sorted records
`{GOOS key, GOARCH key, ArchitectureLevel key, ToolchainArchive digest}`.
Whole manifest ≤1 MiB. Platform variant differs within that common manifest,
not between common build identities. Explicitly include installer settings
such as readonly modules, trimpath, buildvcs and ldflags; clear uncontrolled
environment inputs as the installer already does.

SourceTree hashes the canonical build-source inventory: relative UTF-8 paths
in lexical order, file kind/executable mode, lengths and file bytes, including
all actual build inputs and dependency replacements. The stamp generated
from that inventory is excluded to avoid self-reference; no other source
file may be silently excluded because it is dirty or untracked. Installer
archive hashes and revision are provenance diagnostics. A release manifest
is stamped only from the pinned verified source; a development manifest
must be an explicit common source inventory, never an inferred clean flag.
Missing/dirty/unstamped release provenance rejects normal-room admission.

Binary hashes and native-equivalence evidence are **detached attestations**
keyed by common build digest and variant. They cannot be embedded into a
manifest that their own binary contains. The release gate supplies this
evidence after building/testing all admitted variants; its absence prevents
normal release admission. M2 does not create a signature/attestation service
or claim a client-reported digest proves integrity.

### 8.8 Public identity API

These values are package-owned, with private effective storage and copying
constructors/accessors. No method returns a mutable map/slice alias into a
frozen value. Catalog/model pointers follow the existing immutable-definition
convention; instances and mutator transforms use per-battle clones.

Field 12 adds two session functions (§16.6): `MatchUnitRestrictions(cat
*content.Catalog, r content.Restrictions) ([]MatchUnitRestriction, error)`
maps a host's set against the unrestricted catalog, and
`RestrictionsFromMatch(cat *content.Catalog, records []MatchUnitRestriction)
(content.Restrictions, error)` reads records back, refusing what field 12
rejects. `FreezeMatchInputs` performs the second against the install's own
unrestricted catalog.

```go
// internal/content
type SimulationSources struct { /* private immutable captured source set */ }
func CaptureSimulationSources(fs vfs.FSOps, mapName string) (*SimulationSources, error)
func (s *SimulationSources) Filesystem() vfs.FSOps
type SimulationInputRequest struct {
    Catalog *Catalog // prepared once under the selected rules/mutators
    SimArt *SimArt   // optional precompiled value, validated against snapshot
    MapOTA, MapTNT string
    MapSchema uint32
    AIProfile string
    CommunityDigest [32]byte
    Mutators Mutators
}
type SimulationInputs struct { /* private frozen storage */ }
func FreezeSimulationInputs(sources *SimulationSources, r SimulationInputRequest) (*SimulationInputs, error)
func (i *SimulationInputs) Digest() [32]byte
func (i *SimulationInputs) Manifest() []SimulationInput
func (i *SimulationInputs) Catalog() *Catalog
func (i *SimulationInputs) SimArt() *SimArt
func (i *SimulationInputs) Model(key string) (*model.Model, bool)
func (i *SimulationInputs) Filesystem() vfs.FSOps

// internal/session
type MatchConfigRequest struct { /* public positional fields of §8.6, no defaults */ }
type EffectiveMatchConfig struct { /* private validated copy */ }
func ResolveMatchConfig(r MatchConfigRequest) (EffectiveMatchConfig, error)
func EncodeMatchConfig(c EffectiveMatchConfig) ([]byte, error)
func DecodeMatchConfig(payload []byte) (EffectiveMatchConfig, error)
func (c EffectiveMatchConfig) Digest() [32]byte
func ValidateMatchInputs(c EffectiveMatchConfig, inputs *content.SimulationInputs) error
func NewAdmittedSkirmish(inputs *content.SimulationInputs, c EffectiveMatchConfig, progress content.Progress) (*Session, error)

// internal/version
type BuildManifest struct { /* public positional fields of §8.7 */ }
func CurrentBuildManifest() (BuildManifest, error)
func EncodeBuildManifest(m BuildManifest) ([]byte, error)
func DecodeBuildManifest(payload []byte) (BuildManifest, error)
func (m BuildManifest) Digest() ([32]byte, error)
```

`SimulationInput` is the typed manifest record of §8.7: Family and Presence
are `uint8`, Key and FallbackKey strings, Ordinal `uint32`, SemanticDigest
`[32]byte`; diagnostic provenance has a separate accessor. Configuration
fields/types mirror §8.6's explicit names and widths, with named `MatchSeat`,
`MatchView`, `MatchPolicies`, `MatchMod`, `MatchContentProfile` records and
`[]AIParam` per row. Codecs own validation and return no partially valid
value. Resolve/decode establish **schema validity** (closed values, canonical
form and known registered rules). `ValidateMatchInputs` then establishes
**admission validity** against the same frozen content: map/schema, side
ordinals, definition restrictions, content profile, effective rule/mutator
inputs and supported consumer policies. `NewAdmittedSkirmish` must call it
before allocating a world or consuming either RNG; a hash match alone is not
admission. `Digest` for effective configuration/content is infallible because
construction already validated them; the public build request remains
fallible. Identity comparisons in U6 report protocol, build, content, map,
rule, mod and configuration mismatches separately. M2's constructor refuses
multi-seat execution until M5's entry/consumer gate is satisfied; do not
silently compose it through today's single-human constructor.

## 9. State digest and desync

### 9.1 The canonical snapshot

One versioned serializer writes the complete authoritative state in canonical
order —
players 0–9, pool slots ascending, projectiles in pool order, the effect
pool with its fragment geometry, the strip objects and the debris arena
(L9), both stream states and draw counts, and the global tick but none of
the clock's pacing fields (§4.4) — with every
float as its exact bit pattern and every definition as a canonical content
reference, including its immutable record identity where names repeat.
Those keys are bound to the complete content identity (§8.2). A NaN in
authoritative state is refused at the checkpoint and reported as a defect:
its bit pattern differs by processor (L1), so hashing one would add a false
mismatch to the real fault. Owners publish
logical field schemas and deterministic iteration order; runtime pointers,
Go layout, goroutine/channel state and presentation caches are not a schema.
The session's publication state is not a presentation cache wholesale: the
effect pool lives there and is listed above.
Three things use it:

1. **The digest** is SHA-256 over the serialization, truncated to 128 bits
   on the wire.
2. **The desync bundle** stores the serialization itself (§9.4).
3. **Rejoin by snapshot**, later (§11.3).

**Boundary.** A lockstep checkpoint is the state after a tick's executor
tail and before any entry of the next tick is applied. The tick number
identifies it, and a pause changes nothing about it (§4.4). In a
single-player replay the tail belongs to the last tick of its recorded pump:
a checkpoint at tick 30 inside a pump spanning ticks 28–32 is taken before
that pump's tail, which runs only after tick 32. A live single-player host
takes its checkpoint when the tick completes, before its paused-input
boundary applies anything for the next tick, so a recording and its playback
hash the same state (§4.2). Compare only checkpoints of the same tick.
Network and input queues stay outside world state and are rebuilt from the
stream position that accompanies a snapshot.

**Modern AI.** A controller's memory — its brain, private generator,
observations and any batch not yet applied — is mutated by its worker
between deadlines and is **not** part of the regular checkpoint. Nothing a
controller holds reaches the world except as commands applied on the
simulation thread at its reaction deadline, so a controller that has
diverged shows as different commands. The checkpoint therefore carries, per
computer seat, a running hash of the commands applied for it so far and the
tick of its next deadline. Both are simulation-thread values: reading them
joins no worker, and a checkpoint costs the same whatever the workers are
doing. Engine upkeep for both kinds of computer player, and the Classic
planner's records, are world state and are serialized like any other.

A snapshot does not serialize controller memory either. Saves already
restart a Modern controller from observation, carrying only its generator's
position (`internal/session/ai_save.go`); a snapshot rejoin runs the same
rebuild on **every** replica at once. The relay binds a controller-restart
session event to the first tick after the snapshot's checkpoint (§4.2). In
phase 1 of that tick each client joins its workers, keeps each generator's
position, discards the controllers with whatever they had pending, and
builds them again from observation, exactly as a load does. The snapshot
carries the generator positions, which is the one moment its provider must
join a worker. The computer players lose their plans at that tick — a
rejoin costs them a moment's thought — and the entry is in the stream, so a
replay repeats it. A brain that cannot restart from observation cannot be
offered in a room that allows snapshot rejoin; none is registered today.
This is a visible effect on play and is decided in §15 Q21. Exact
serialization of controller memory remains possible later and no milestone
here requires it.

**Completeness.** Maintain an owner-by-owner inventory of fields that can
affect future behavior, with a reason and reconstruction rule for every
exclusion. Review it when authoritative state changes. Then test serialize /
restore / serialize byte equality and run original and restored sessions
through targeted and long continuation scenarios with matching checkpoints.
Tests complement that review: an omitted field may never affect an exercised
scenario. The canonical writer/digest is delivered before verified replay;
the exact restore reader can follow. Partial fingerprint locks retain their
separate regression contract, apart from the explicit M1/M2 changes (§16);
that fingerprint goes on hashing the clock's pacing fields for its
single-host purpose, and the canonical digest never does (§4.4).

### 9.2 Cadence

Each client computes the digest at the checkpoint after every tick whose
number is a multiple of 30 and sends it with that tick. The relay
compares the required participant connections for that checkpoint, not one
vote per computer seat. That participant set follows the relay's ordered
membership: a resignation or final removal, or a drop's membership note,
removes a participant, and the completed rejoin of §11.2 restores one.
Catching-up clients and spectators are excluded.
Each client also keeps the **sub-digests** of the
last 64 digest ticks, one per owner — units, projectiles, effects, features,
economy, visibility, movement and paths, COB, computer players, random
streams — so a mismatch names the subsystem as well as the tick. The cadence
and window are initial values for §16 to measure; the serializer's cost bounds
them.

**The tick ring.** A digest every 30 ticks says that two clients differ, not
when they began to. Each client therefore also keeps, for every tick of a
recent window (600 ticks to begin with), a cheap record written without
serializing anything: both streams' states and draw counts, each pool's live
count, and a rolling sum over each owner's fixed-width fields. It is never
sent during play. On a mismatch the rings go into the desync bundles (§9.4),
and comparing two of them names the first differing tick and owner even when
the divergence cannot be reproduced on one machine — the ordinary case for a
cross-platform defect. Its contents and cost are M3 measurements.

Each required report has a relay-side deadline independent of Progress.
Withholding digests while reporting progress is a protocol failure; it
cannot postpone comparison indefinitely. Bound outstanding reports and
retained windows. Cadence and deadlines are configuration/protocol values
measured before release, not wall-clock inputs read by the simulation.

### 9.3 What a desync does

When digests disagree, the relay announces the checkpoint, requests its
sub-digests and tick rings, and holds its grants at the last sealed one.
Each client writes a desync bundle. What happens next is room policy,
declared in the configuration:

- **Casual rooms: the battle goes on for those who agree.** If the
  participants reporting one digest are a strict majority of those required,
  they continue. Each dissenting seat enters the reconnectable-drop state
  (§11.1) and is offered the rejoin path (§11.2). Before that path exists
  (in M6, ahead of M7's fast-forward rejoin) the seat stays in that state:
  its seat and units are retained idle exactly as any out-of-play seat's
  are, and it is finally removed only by a passed removal vote, never by a
  timer. A rejoin replays the stream from the start, which cures a
  transient fault and repeats a deterministic one; a seat that disagrees
  again at the same checkpoint never completes its rejoin and returns to the
  drop state, so its cumulative grace keeps running and a removal vote
  becomes possible as for any seat out of play (§11.1); its bundle names the
  platform. The hosting client of an
  embedded relay is a participant like any other: its relay keeps running
  while its seat rejoins. With no strict majority — every two-participant battle — the
  battle ends without a result. This is a deliberate trade. Ending the
  battle for everyone lets one faulty or dishonest client end an
  eight-player game, which is the commoner harm in rooms that rank nothing.
  Once exact snapshots exist (M8), a dissenting seat is resynchronized from
  an agreeing participant's snapshot instead of replaying, and a battle with
  no majority continues from the room creator's state instead of ending:
  where nothing is ranked, one consistent battle matters more than which
  copy was right.
- **Rated rooms: no client majority decides anything.** Colluding clients
  and platform-specific defects both make that unsafe where a result is at
  stake. The battle stops without a certified result until a trusted
  replayer or referee says which state is right (§12.6).

Neither policy silently replaces a client's state, and a seat moved to
rejoin is not thereby a loser. Replaying on the same incompatible client is
not assumed to cure a deterministic bug.

### 9.4 Desync bundles and diagnosis

A bundle holds the replay, complete build/platform/content/configuration
identities, the sub-digest window, the tick ring and a snapshot with its
checkpoint tick. Diagnose snapshots only at matching checkpoints. The headless
tool replays at finer checkpoint cadence to localize reproducible divergence
and compares canonical fields when matching snapshots are available.
It must also report when it cannot reproduce the mismatch: replaying the
same inputs twice on one platform does not reproduce every scheduling or
cross-platform defect. Retain source-machine evidence and arrange a common
future checkpoint for diagnostic captures where possible. Every explained
desync becomes a regression scenario for the multi-seat harness (§17).

## 10. Replays

A replay file holds a header — format version, build identity, content
identity, battle configuration, seed pair — and then the stream: commands
and session events with their ticks, pump ends, pacing and membership
notes, and digests at
the digest cadence. Playback composes the battle exactly as battle entry
does, applies commands and session events through phase 1, and runs each
executor tail where the recording ran it (§4.5, §9.1). It checks each
recorded digest, so a replay that a build cannot reproduce says so at the
first mismatch rather than showing a different battle. A recording made in
a window must play back headless, and the reverse: that is L9's exit test,
and nothing in a replay may assume the kind of host that made it.

- **Single-player battles** record the local source's entries and the host's
  pump boundaries (§4.5).
- **Multiplayer battles** record the relay stream; the relay can keep the
  server copy (§12.5).
- **Compatibility** initially requires the admitted Nanolathe release and
  complete content identity that wrote it, including its platform-equivalence
  checks (§8.2). Source provenance and schema versions stay in the header.
  Preserving older releases or later proving compatibility between releases
  is a separate decision, not an automatic consequence of unchanged protocol.
- **Viewing** is presentation: any seat's perspective, fog on or off, any
  speed the host can sustain, and jumps forward by fast simulation.
  Rewinding is a later feature built on snapshots.
- **Chat** is non-authoritative and retains its audience/access metadata;
  public replay release and private team chat follow the room's declared
  archive policy. A replay archive must not bypass the live spectator embargo.

## 11. Seats over time

### 11.1 Leaving

A resignation and a voted final removal are session events, assigned a tick
by the relay (§4.2); a resignation is itself a final removal. A
reconnectable drop and a completed rejoin are membership notes that change
no simulation state (§4.2). No client may remove another seat by naming it
in a payload. Retail instead accepts a peer's removal notice without a host
check `[08 R-LEAVE-01 §1]` `[08 R-LEAVE-01 §2]`.

- **Resign** is the seat's own immediate final removal, with no vote. It
  ends that seat's battle without a win. Allowing its client to stay as a
  watcher is Nanolathe's interface choice. Retail has no single resignation
  path to match `[08 R-LEAVE-01 §7]`. Established: the surrender question is
  the only in-battle way to leave; its menu variant runs only the leaving
  machine's teardown, which sends a death record for every unit that machine
  simulates, and peers delete a positive-health unit with no script,
  explosion or wreck, but run the kill script, an effect-only explosion and
  the wreck for a unit whose health is not positive; its exit variant first
  applies 30000 self-damage to every own unit, so a unit that damage leaves
  at non-positive health takes the second path. No departure packet is
  built: peers learn of the departure from those death records and, when the
  session closes, from the transport. Supported inference only: closing the
  session destroys the leaver's players and peers then empty its slots,
  hosted computers included, in an order not traced. **A Strict resignation
  is retail's menu variant as the other machines see it** (§19 O23, decided
  2026-10-02): the relay-authored resignation event deletes every unit of
  the resigning seat and, under Strict, of the computer seats it hosts,
  silently — no `Killed` script, no explosion, no wreck, no kill or loss
  credit — exactly as a receiver treats a positive-health unit's cause-8
  death record `[08 R-LEAVE-01 §7]`; a unit whose health is already
  non-positive at that tick keeps the researched receiver path for it (the
  `Killed` query, an effect-only explosion and the wreck). The 30000
  self-damage of the exit variant is not applied. Each seat's record is then
  cleared as a voted removal clears it (below), with the same freezing of
  stocks and nothing transferred. Replicas take the seats in the order a
  voted removal takes them — ascending slot order, the human's own slot in
  its place — and each seat's units in ascending pool order, the order of
  the voted removal's sweep `[08 R-LEAVE-01 §4]`; retail's order for the
  menu variant is only the Supported inference above, so this is the
  protocol's order, not a researched one. Modern and Community follow the
  final-removal policy below.
- **A reconnectable drop** is the state of a seat that has no connection in
  play: its connection was lost or closed, the desync policy took it out of
  play (§9.3), or it made no progress past M6's lag bound (§4.4). A seat
  with a connection in play is **active**. A seat whose resume has been
  admitted but whose rejoin (§11.2) has not completed is **rejoining**, and
  is still out of play; a rejoin that has not completed within
  `RejoinGraceMilliseconds` of its admitted resume returns the seat to the
  drop state (§15 Q28). An out-of-play seat and its units are
  retained idle in every mode and issue no new commands, and the computer
  seats it hosts keep running in every mode, because every client runs every
  computer and the reconnectable window, with its rejoin, is itself the
  approved Nanolathe addition: no retail reconnect protocol has been found
  `[08 R-LEAVE-01 §10]`. No timer ends the state: without a passed removal
  vote an out-of-play seat stays idle until the battle ends. Before the
  rejoin path exists (M6, ahead of M7's fast-forward rejoin) the same holds:
  the seat is retained idle and can be finally removed only by a passed
  vote.
- **Final removal** closes the rejoin window and applies Q6. It comes only
  from the seat's own resignation or from a passed removal vote (below). In
  Strict 3.1 a voted removal destroys that seat's units using the
  established peer-removal contract: effect-only self-destruct and no kill
  credit; the ordinary positive-health case leaves no wreck, while an
  already zero-health unit preserves the researched full preamble and
  script-selected wreck. Stocks freeze when the record is removed;
  resources, scores, identities and the unit-limit slice are not gifted or
  redistributed `[08 R-LEAVE-01 §3]` `[08 R-LEAVE-01 §4]` `[08 R-LEAVE-01 §5]`.
  A Strict resignation clears the record the same way but deletes the units
  silently, with no self-destruct effect (Resign bullet, §19 O23).
- **Computer seats** follow their human host's final removal by mode
  (§6.6, Q26). When a remaining machine removes a departed human, retail
  removes every slot carrying that human's machine number — its hosted
  computers too — in ascending slot order, each through the same
  peer-removal path that destroys its units `[08 R-LEAVE-01 §2]`
  `[08 R-LEAVE-01 §9]`. At a human seat's final removal Strict 3.1 removes
  its hosted computer seats with it, the human's own slot taking its place
  in ascending slot order, each by the destruction contract above: for a
  voted removal this is retail's peer removal of the machine group; for a
  resignation it is the silent deletion of the Resign bullet (§19 O23).
  Modern and Community keep them running with the host's perspective, which
  keeps updating, under the policy below. Defeat alone removes no computer
  in any mode.
- **The hosting client** of an embedded relay is the relay: losing it ends
  the battle for everyone without a result. This is an infrastructure limit;
  retail migrates its host flag and continues a launched battle
  `[08 R-LEAVE-01 §8]`. A hosted relay has no such player seat.

**Nanolathe online policy — the removal vote (approved 2026-10-02, §15
Q27; its details §15 Q28, approved the same day).** Retail removes a silent
machine through its time-out dialog: any machine's `REJECT` removes it at
once, and the dialog removes it by itself at the time-out plus 120 seconds;
when the silent slots span two or more machines the dialog closes instead,
so two or more silent machines are never removed that way
`[08 R-LEAVE-01 §6]`. `+Drop 0` turns the monitor off `[07 R-CAM-01 §6]`.
The host can also reject any remote player at once through its control
window (`Reject: <name>`) `[08 R-LEAVE-01 §1]`, and every receiver applies
the resulting removal notice without a host check `[08 R-LEAVE-01 §1]`
`[08 R-LEAVE-01 §2]`. Each machine therefore decides for itself. A lockstep
battle needs one decision applied by every replica at one tick, and in
Strict a final removal destroys the seat's units, so Nanolathe replaces
the dialog and the host's reject with a vote in every gameplay mode:

- **Subject.** Only a seat out of play: in the reconnectable-drop state or
  rejoining. An active seat cannot be voted out: there is no vote-kick in
  the first releases, because a majority could otherwise destroy a present
  opponent's units. A present client that the casual desync policy took
  out of play (§9.3) is out of play here too; §13 records that exception.
- **Caller.** Any active human seat that is still playing — not a watcher,
  not a spectator, not the subject.
- **Electorate and threshold.** The active human seats still playing,
  excluding the subject, fixed when the vote opens. The vote
  passes on a strict majority of that electorate: a tie fails, and a sole
  remaining eligible seat decides alone. A ballot not cast counts as no.
  The call casts the caller's yes ballot, and each other voter casts at most
  one ballot, which it cannot change. "Active" is the relay's own
  fact; "still playing" is the agreed summary of §12.2, which may lag by one
  digest interval.
- **Timing.** A vote may be called only once the subject has been out of
  play — dropped or rejoining — for a cumulative `RejoinGraceMilliseconds`
  since it was last active in the battle (§8.6 field 15). Cumulative, so a
  seat that keeps reconnecting without completing a rejoin cannot reset it,
  and a seat the desync policy keeps out of play (§9.3) is covered by the
  same clock. Only a completed rejoin (§11.2) cancels an open
  vote and resets the grace; an admitted resume does neither. A
  vote stays open for **30 seconds** and closes earlier once its outcome can
  no longer change; after a failed or tied vote, another vote against the
  same subject may not be called for **60 seconds**; at most one vote per
  subject is open at a time (Nanolathe protocol values, not retail
  constants; all §15 Q28). A refused call is answered with its reason (§12.2
  "Refused").
- **Mechanism.** Calls, ballots and results are relay and lobby messages
  (§12.2). They are never simulation commands and never enter the replay's
  command stream. The relay tallies; on a pass it authors one final-removal
  session event and assigns it a tick as it assigns any other (§4.2). That
  event is the only part of the vote a simulation sees, and every replica
  applies it identically (§4.2 "Relay-authored entries"). An embedded relay
  runs the same tally code as a hosted one; its hosting client is trusted
  with the tally as §13 states.
- **A battle with nobody left to vote** (§15 Q28). When no human
  seat that is still playing is active, the relay holds its grants. If none
  has become active again once `RejoinGraceMilliseconds` has passed since
  the last one left play, the relay ends the battle without a result, as it
  does when an embedded relay's host is lost (above) or a hosted relay's
  service stops (§12.5). Connected watchers and spectators do not keep it
  running.

**Nanolathe Modern policy — final seat removal (Q6, revised 2026-10-01;
hosted computers Q26, approved 2026-10-02).** The approved departure is
bounded to a finally removed human seat in a multiplayer battle. Strict 3.1
keeps the machine-group removal above. Modern and Community retain the
removed seat's units under that owner, idle, with no new commands, and keep
the computer seats it hosts running with its perspective. That perspective
is live, not a snapshot (§15 Q29, decided 2026-10-02): the removed human's
own per-seat block — its sensor pass, temporary sight and the other per-seat
work of §6.3 — keeps running for its seat exactly as while it was only out
of play, so the picture its computers borrow goes on updating; only its
end-condition evaluation stops, because a finally removed seat is absent
from the end-condition sweeps in every mode (§6.5). Its retained units are
live and can be destroyed, but destroying them is not required for any
survivor's victory. Retention transfers neither
units nor stocks to another player and adds no RNG draw of its own. Ordinary
unit simulation and economy remain under their existing rules.
Single-player, commander defeat and reconnectable absence do not select this
policy; the removal vote decides only whether and when a final removal
happens, in every mode.

Select the answer through the existing central `gameplay.Mode` and
`session.RuleSet` composition, following DESIGN_GAMEPLAY_RULES §9. **One
answer covers both halves (§15 Q28, approved 2026-10-02).**
Retail's machine-group removal is a single rule `[08 R-LEAVE-01 §2]`, so the
decision can be one choice between the Strict removal — destroy the human's
units and remove its hosted computer seats with theirs — and the Modern
retention — retain the units idle and keep the computers running. Two
answers would admit a mixed combination nobody approved. Extend the existing
owning rule interface if it can express the removal decision; if none can,
justify a narrow session-owned seam before implementation and compose it in
that same registry. Strict, Community and Modern defaults must all supply
the answer; no room flag or second registry chooses it. Mutable seat
lifecycle and vote state belong to the session and the relay respectively,
not to cached rules.

Required M6 tests cover a reconnectable drop and a final removal in all
three reserved modes, repeated removal, and removal while grants are held.
Assert Strict destruction, the positive-health no-wreck path and zero-health
exception, no kill credit or resource transfer, and the researched loss-stat,
effect and RNG consequences; assert Modern/Community retention avoids
those removal effects and draws. Compare later RNG positions and stocks,
and preserve single-player fingerprints. For hosted computers:

- in every mode, a computer keeps running while its human is out of play;
- under Strict, a voted final removal removes the human and its hosted
  computers in ascending slot order, each with the researched destruction,
  loss-stat, elimination-line and RNG effects, and a later removal of an
  already removed seat changes nothing;
- under Strict, a resignation clears the human and its hosted computers in
  that same order but deletes their units silently: no `Killed` script,
  explosion, wreck, credit or self-destruct draw for a positive-health unit,
  the researched receiver path for a unit already at non-positive health,
  and no self-damage (§19 O23);
- under Modern and Community, either removal leaves its computers running
  and issuing commands with the host's perspective, which keeps updating
  because the removed seat's per-seat block still runs; the removed seat is
  absent from the end-condition sweeps, so a survivor wins without
  destroying its retained units (Q29).

For the vote: one that passes, one that fails, a tie (which fails), one
cancelled by the subject's completed rejoin, one that survives an admitted
resume that does not complete, a call refused before the cumulative grace
(including a seat that reconnects repeatedly), a call refused against an
active seat, a call refused during the cooldown, a call by an ineligible
caller, a rejoin that misses its completion bound and returns to the drop
state, and a battle with a single eligible voter, who decides alone. Assert
that no call or ballot enters the stream, that each refusal carries its
reason, that a pass produces exactly one final-removal event at the next
assignable tick, that embedded and hosted relays tally the same inputs
identically, and that a battle whose every playing human is out of play
ends without a result after the grace. Keep unit orders already in flight
and the exact meaning of idle explicit in the lifecycle contract before
implementation; Q6 does not invent queue cancellation or a new per-tick
suspension rule.

### 11.2 Rejoining by fast-forward

The relay keeps the battle's configuration, seeds and stream. A rejoining
client receives them, composes the battle, and runs the stream at full speed
without presentation or audio until it reaches the live grant, then resumes
its seat. Admission repeats build/content/configuration checks, including
view restrictions. A per-match seat credential proves ownership without
requiring an account in casual rooms. A new connection epoch fences off the
old connection; only one connection can command a seat at a time.

The relay retains each seat's last accepted client sequence and its stream
position across disconnects, under the sequence rules of §4.2. A command at
or below that value is dropped silently, whatever its payload: no second
stream entry or receipt is made, and the entry already accepted under that
number stands. A command that skips ahead is refused and nothing is
stamped. The relay's reply to a resume names the last accepted sequence and
its stream position, and the client resends its unacknowledged commands
from the next value under their original numbers, handling a connection
loss between acceptance and echo without either duplicating production or
silently losing an order. Sequence retention covers the whole rejoin
window, which lasts until the seat's final removal or the battle's end.

Required M7 tests (M6 for the parts without rejoin): commands accepted in
sequence order; a duplicate at and below the last accepted value dropped
with no stream entry or receipt; a gap refused with nothing stamped, the
refusal naming the expected value, and the seat's later numbers refused, not
buffered, until the expected value arrives; a resume that reports the last
accepted value, after which the resent commands apply exactly once; a
connection lost between acceptance and echo; one seat's commands never
reordered across a reconnect; and two seats' sequences independent of each
other.

Catch-up clients do not pace the battle or vote on its digests. They become
active through an ordered membership transition after matching a required
checkpoint and receiving its sealed stream continuation. Commands are
admitted only after that transition; one sent earlier is refused with the
expected next sequence (§12.2 "Refused"). The transition is the completed
rejoin of §11.1: only it cancels an open removal vote and resets the seat's
grace, and a rejoin that has not completed within `RejoinGraceMilliseconds`
of its admitted resume returns the seat to the drop state (§15
Q28). Authentication, takeover, deduplication and checkpoint handoff are
part of reconnect, not later account features.

The displayless simulation benchmark measures replay throughput. With an
initial backlog of N ticks, replay rate r and ongoing grant rate g, catch-up
takes approximately `N / (r - g)` seconds when rates are steady and `r > g`,
plus loading/transfer time. `N / r` applies only to a stationary target.
On the development machine (2026-10-01) the three-army benchmark fight runs
headless at 490–800 ticks per second, sixteen to twenty-six times real time,
so thirty minutes of that battle would replay in about two minutes; at only
twice real time it would take another thirty. The rate falls with army size
and with the client's processor, and nothing has been measured at the
advertised seat and unit limits. If the client cannot catch up within the
advertised window, the feature is unavailable or requires an agreed
pause/snapshot;
never promise reconnection merely because replay is faster than real time.
Measure large/late battles early and move §11.3 forward if needed (§16).

### 11.3 Rejoining from a snapshot

Once the canonical snapshot (§9.1) passes exact restore and continuation
tests, the relay arranges a common checkpoint and obtains a snapshot with its
stream cursors and subsequent entries. Casual rooms may use a participant
whose digest agrees; competitive rooms require the trusted referee's
checkpoint. An agreed client hash alone does not certify honesty. Validate
the snapshot's envelope, lengths, identities and digest before restoring it,
and verify continuation before activating the seat. Keep transfer traffic
and buffers bounded and separate from live control progress. A historical
checkpoint is available only if retained; otherwise schedule a new one.
The retail-format save is not used for this (L6). The snapshot carries no
Modern controller memory: the relay binds a controller restart to the tick
after the snapshot's checkpoint, and every client, the rejoining one
included, rebuilds its controllers there (§9.1).

### 11.4 Watchers and spectators

Two different things, kept apart:

- **A watcher** is retail's: an observer row in the configuration
  (`SkirmishControllerObserver` already exists), chosen in the lobby, with
  everything retail gives it — no commander, its placement draws still taken,
  the whole map visible, no score `[08 R-ENTRY-01 §5]`. Watchers are part of
  the battle and fixed at its start, and a room offers the row only when its
  host allows watching `[08 R-SKIR-01 §12]`. A defeated seat becomes one at
  its defeat tick where watching is allowed or it still hosts a live computer;
  otherwise it takes the ordinary lost ending `[08 R-SKIR-01 §3]`.
- **A spectator** is Nanolathe's: a stream consumer with no row. It receives
  the stream delayed by a lobby-chosen interval to reduce live intelligence
  sharing, sends nothing but permitted chat, may join at any time by
  fast-forward (§11.2), and may view any seat's perspective or the whole map.
  Its views are presentation and cannot move the battle.

The relay enforces spectator release times on live frames, catch-up history,
snapshots and replay downloads; a client-side delay is bypassable. A referee
or participant must not expose unrestricted live snapshot URLs. Ranked rooms
also need an explicit rule for defeated players and full-vision watchers,
who otherwise bypass the spectator embargo, and for spectator-to-player chat.
Even a delayed stream does not prevent inference about future events in a
deterministic simulation. A playing client's full local state remains the
larger limitation (§13).

### 11.5 Saving

Retail disables saving in multiplayer `[08 R-SAVE-02 §4]`, so a Strict 3.1
online battle offers none, and the first versions offer none in any mode. A
later multiplayer save would be the canonical snapshot plus the
configuration, resumed only by the same seats; it waits on §11.3. Survival
battles cannot be saved in any case (DESIGN_SURVIVAL §11).

## 12. The relay and the lobby

### 12.1 Shape

One Go package implements the relay; it runs in two hosts:

- **Embedded** in the hosting player's client for LAN and direct-IP battles.
  The host listens on a port; the others connect to it. Over the internet the
  host must accept inbound connections.
- **Hosted** by `cmd/nanolathe-server`, a headless program that adds rooms
  reached by a short room code, then a lobby list and (later) accounts.
  Clients connect outward, so nobody opens a port. This is the ordinary way
  to play over the internet from the first networked release (§16): an
  embedded relay needs an inbound connection that many home networks do not
  allow and few players will set up, and a content-free relay costs little
  to run.

The relay holds no simulation, no catalog and no Cavedog asset. It sees
battle configurations, commands, digests, removal votes and chat. The
protocol is open and documented here so anyone can run a server.

### 12.2 Messages

Client to relay:

| Message | Content |
|---|---|
| Hello | protocol version, release/build manifest identity, content identity, display name, room access credential if required |
| Lobby | join or leave a room; seat, side, colour and team changes; ready; the host's configuration changes |
| Command | the client's sequence number (§4.2) and the command payload (§7.4) |
| Resume seat | battle identity, seat credential, connection epoch and last acknowledged client sequence/stream position |
| Progress | the last tick run (for pacing) |
| Digest | checkpoint tick, digest and the relay-visible summary (below); sub-digests and the tick ring on request |
| Pacing request | pause, resume, speed — not accepted in the first releases (§15 Q5) |
| Seat request | own resignation, the seat's immediate final removal (§11.1) |
| Removal call | the subject seat; opens a removal vote, casting the caller's yes ballot (§11.1) |
| Removal ballot | the vote's identity and yes or no |
| Chat | recipient seats, resolved by the sender, or the spectator audience; text |
| Ping | for round-trip measurement |

Relay to client:

| Message | Content |
|---|---|
| Welcome | accepted versions, room list (hosted) |
| Lobby state | the room's configuration, seats and readiness |
| Prepare start | frozen configuration, its version/hash, seed pair, seat assignment and required readiness participants |
| Start | confirmation of the acknowledged configuration and agreed initial checkpoint; permits the first grant |
| Accepted | client sequence and immutable stream position; a duplicate gets no second receipt (§4.2) |
| Refused | §15 Q28: the refused message's kind and reason. For a command — a gap in its sequence, or a command sent before the rejoin membership transition — the expected next sequence (§4.2, §11.2). For a removal call — the cumulative grace not yet reached, the subject's cooldown still running, a subject that is active, or a caller that is not an active human seat still playing (§11.1) |
| Resumed | the seat's last accepted client sequence and its stream position, the new connection epoch, and the stream position from which delivery continues (§11.2) |
| Frame | ordered stream entries, optionally with a grant sealing a complete prefix; notes and receipts can be delivered without granting ticks |
| Desync | checkpoint tick, sub-digest and tick-ring request, last sealed grant, and the room policy's outcome for each seat (§9.3) |
| Seat status | lag, waiting, out of play (the reconnectable-drop state) with its cumulative out-of-play time, rejoining, finally removed |
| Removal vote | opened: vote identity, subject, caller, electorate and closing time; closed: passed, failed or cancelled, the tally, and on a pass the final-removal event's stream position (§11.1) |
| Chat | sender seat, recipients and text |
| Pong | |

**What the relay knows.** The relay runs no simulation, so it holds only
what connections tell it: the configuration, which connection owns which
seat, who is a spectator, the stream it wrote, and each client's reports. It
does not know who is allied, who has been defeated or whether the battle is
over. A rule that needs such a fact gets it in one of two ways, and never by
the relay guessing:

- **The sender resolves it.** A chat line to allies or to enemies carries the
  recipient seats the sending client computed from its own simulation, and
  the relay routes to seats. A wrong list gains the sender nothing it could
  not have typed to everyone.
- **The clients agree it.** Every digest report carries a small summary
  derived from the same state: which seats are still playing, and whether
  the battle has ended and how. The relay adopts a summary only from reports
  whose digests agree (§9.2). That is how it learns that a defeated seat has
  become a watcher, for a pause rule that admits only playing seats and
  for the removal vote's electorate (§11.1), and that a battle is over, so
  the room can close and the recording can end. The summary lags the world
  by at most one digest interval. A pause rule or a vote can afford that;
  command authority cannot, and is decided inside the simulation (§7.2),
  where the facts are.

A start is two-stage. The relay freezes and distributes the complete
configuration and seeds; required participants acknowledge that exact hash,
compose their sessions and report the initial checkpoint. Only after all
required checks agree does it confirm Start and grant tick 1. Any configuration
change cancels readiness and requires a new proposal. Loading failure or a
missing acknowledgement has a bounded failure path; an unknown identity is
never treated as a match. Reconnect credentials travel over the authenticated
connection, not in public lobby or replay records.

Messages are length-prefixed binary frames; the protocol
version is checked in Hello and a mismatch is refused with a message that
names both versions.

### 12.3 Transport

TCP with `TCP_NODELAY` is the initial transport, through `crypto/tls` to a
hosted relay, from the standard library [I12]. The stream needs reliable
ordered delivery. Loss still stalls a TCP stream; whether the playout reserve
hides it is a measurement, not a guarantee. Internet direct hosting needs an
explicit server-authentication/credential-protection setup too; a trusted-LAN
plaintext mode must be distinguished from public hosting.

Bound per-connection read/write queues and never block the room loop on a
slow spectator or bulk transfer. Prioritize live frames and progress over
catch-up/snapshot traffic, using separate connections if needed. A lost TCP
segment holds that one client until it is retransmitted; the others play on.
`internal/netproto` messages are whole frames that assume no byte stream, so
a datagram transport that repeats recent frames could later carry the frame
and command path with the protocol unchanged, if impaired-network runs
(§17) show the stalls matter. QUIC is a later dependency decision if
measurements justify it: its head-of-line
benefit is across independent streams, while missing earlier gameplay input
still blocks that simulation's progress
([RFC 9000 §13](https://www.rfc-editor.org/rfc/rfc9000.html#section-13)).

### 12.4 Lobby and matchmaking

A room is a battle configuration being assembled: the host chooses the map,
rule set, mutators and options; players choose seats, sides, colours and
teams; the host adds computer seats. The retail battleroom `[07 §12]` is the
reference for what the lobby offers (§3.3); its screens are designed with
M6. The first hosted lobby is a room reached by its code; a room list
follows. Accounts, ratings and matchmaking
come last (§16) and are ordinary web services beside the relay.

Two of retail's host options are room state rather than battle
configuration: *game closed* stops further joins and added computer seats,
and *watching allowed* decides whether a watcher may join
`[08 R-SKIR-01 §12]`. The room keeps the first for itself and passes the
second into the configuration (§8.1). Retail's host presets its options from
preferences or from an online service's configuration file, never from the
command line `[08 R-SKIR-01 §7]`; a Nanolathe room has its own defaults.

### 12.5 Operations

The relay's cost follows command volume, actor-list sizes, fan-out, history
and spectator counts. Measure ordinary and maximum advertised workloads;
do not size the service from a constant bytes-per-player estimate. Bound
rooms, input rates, outstanding digests, send queues, replay retention and
concurrent catch-ups. The relay keeps each stream for the promised rejoin
window and archives it only under the declared release/access policy.
Process metrics use `expvar`; service restart/crash initially ends its live
battles without a certified result unless a later durable recovery contract
is implemented. Admission, overload and failed replay storage have explicit
failure paths. The trusted replayer and the referee are §12.6.

### 12.6 Competitive authority and future state delivery

An initial relay orders input but cannot establish which world is correct.
Two trusted roles can, and they differ in what they cost:

- **A trusted replayer** takes the relay's recorded stream after the match,
  plays it headless on an admitted build with the same content and
  configuration, and reports the terminal result and the digests. A battle
  is a function of its configuration, seeds and stream, so this is all that
  certifying a result needs. It runs faster than real time, off the match's
  critical path, on whatever machine the operator trusts, and only for
  matches that need it: every rated match, or those whose clients disagree
  or are disputed.
- **A live referee** runs the same admitted simulation and validation path
  during the match, as a service-side participant. Only it can adjudicate a
  desync while play continues or supply a trusted snapshot for recovery
  (§9.3, §11.3).

Ranked admission requires the replayer; the referee is an addition for rooms
that promise recovery mid-match. The result service binds its authenticated
report to the match identity, participants, configuration and stream; player
votes cannot manufacture or replace it. Loss of that authority makes a match
uncertified under a declared policy, not an automatic win for a participant.
Both roles need the game content and simulation capacity wherever they run,
which the content-free relay never does (§12.1): who operates them, and on
whose licensed copy of the content, is part of the ranked decision (§15
Q17). Both also depend on a headless host computing what a window computes
(L9).

Refereed lockstep still supplies every client with the whole world. If the
competitive product later requires withholding hidden enemy state, a
**server-authoritative state mode** remains an option. The server would
validate commands, run simulation and publish only each seat's permitted
state/events; clients would present that feed rather than depend on a complete
local simulation. This needs its own visibility-safe state schema, references,
recovery, bandwidth budgets and presentation driver. A referee, TLS or an
account system cannot supply that property to the existing command stream.

Preserve the existing semantic command admission and committed-frame
presentation boundaries so this can be designed without changing the gameplay
rules. Avoid exposing session pointers or Go serialization as public APIs.
Do not implement speculative state replication in the first milestones.
Before promising a ranked integrity level, explicitly decide whether
full-state clients are acceptable or whether this additional architecture is
required; acknowledge its substantial cost. Neither route commits Nanolathe
to cross-play with other engines (§8.5).

## 13. Threat model

Retail does not provide these receiver-side defences: its admitted peers may
name other players in removal and player-record payloads
`[08 "Packet framing and dispatch"]`.

| Threat | Defence |
|---|---|
| Removing another seat | Only the relay authors another seat's final-removal event, and only on a passed removal vote against a seat out of play; a client can request its own resignation and call or vote in a removal vote, nothing more (§11.1). |
| A majority voting out a present opponent | An active seat cannot be a vote's subject, and there is no vote-kick (§11.1). The exception is a present client that the casual desync policy took out of play: it is out of play for the vote too, and before M7 it cannot rejoin, so a majority that out-voted it at a checkpoint can later vote it out — see the row "A client majority expelling an honest peer" (§9.3). |
| Rewriting another seat's options or host status | The relay holds the agreed configuration; payloads cannot replace another seat's record or authority (§8.1). |
| Ordering another seat's units | The relay stamps the seat; every client's simulation drops commands whose actors the seat does not own (§7.2). |
| Taking another seat's resources with a negative or otherwise abnormal gift | Online `Give` admits only a positive whole amount within its producer's range unless the room permits cheats; every receiver rejects anything else before resources change (§7.1, §7.4.1, §15 Q8). |
| Changing one's own state (resources, health, build time) | Honest replicas do not apply uncommanded changes. Reported digests detect honest disagreements at the checkpoint cadence, but a malicious client can lie about its digest or maintain an honest copy. Receiver-side admission prevents unauthorized commands from becoming shared state; a trusted replayer certifies ranked results (§12.6). |
| Seeing through fog or bypassing match view restrictions | Every lockstep client holds the whole state, whether its source is open or closed. Honest clients enforce §8.4; hashes cannot prove camera compliance. Moderation/reports are limited measures; withholding hidden state needs §12.6's different delivery architecture. |
| Ordering attacks on units one cannot see | Casual behavior and the required competitive admission contract are distinct (§7.2, §15 Q9). |
| Automation, macros | Not preventable; community moderation. |
| Stalling, lag switching | Pacing and the waiting-for-player state while a seat is active; a connected seat that makes no progress past M6's bound is moved into the drop state (§15 Q28), and once a seat is out of play the grants go on without it and the others may vote on its final removal after the grace (§4.4, §11.1). |
| Attacking an opponent's connection | Hosted relays avoid publishing player addresses. An embedded/direct host necessarily exposes its listening address; hosting remains a service attack surface. |
| Flooding the relay | Per-connection rate and size limits; tokens on hosted relays. |
| Forged results or replays | Casual streams are diagnostic records, not independently certified results. Ranked reporting requires the authenticated replayer/result service and a declared archive authentication policy (§12.6). |
| Seat theft or duplicate input on reconnect | Protected per-match seat credentials, fenced connection epochs, the per-seat client sequence and its deduplication (§4.2, §11.2). |
| A client majority expelling an honest peer, or withholding hashes | Casual rooms accept a strict majority's digest and say so in the lobby (§9.3); rated rooms never do, and rely on bounded report deadlines and trusted authority. |
| Ending or stalling a battle by reporting a false digest | Casual: the dissenter is moved to rejoin and the others play on (§9.3). Rated: the match is uncertified, never awarded. |

**The embedded relay's host is trusted.** Whoever runs a relay holds
everything it does: the stream's order, tick assignment, the seed pair,
digest comparison and the room's desync outcome, and the removal-vote tally
(§11.1). The clients cannot check from the stream that a tally was honest or
that the seeds were drawn at random. An embedded relay runs the same code
as a hosted one, the tally included, and its hosting client — also a
player — is trusted with all of it: with the vote tally exactly as with the
stream's order and the seeds. A hosted relay removes that trust from every
player and places it with the service operator.

Client anti-cheat software is not planned. This does not make authoritative
validation, authenticated services or moderation unnecessary. Co-op comes
first because it is a useful smaller release; eventual competition has
additional fairness and integrity gates (§16), not merely more playtesting.

## 14. Packages

The relay must stay content-free and simulation-free, so it never decodes a
command: commands cross it as opaque payloads, encoded and decoded by the
session that owns their type.

| Package | Layer (ARCHITECTURE §3) | Responsibility | Imports |
|---|---|---|---|
| `internal/netproto` | leaf | Versioned message envelopes and binary codec (§12.2); explicit stream/checkpoint identities and opaque command payloads | standard library |
| `internal/relay` | leaf | Rooms and room codes, acknowledged configuration, seat credentials and fencing, client-sequence admission and receipts (§4.2), scheduling, grants, pause and speed, bounded digest comparison with the agreed summary and the room's desync policy, removal votes and the final-removal events they author (§11.1), and stream retention | `netproto` |
| `internal/replay` | leaf | The replay file: header, configuration bytes, stream, digests | `netproto` |
| `internal/session` | composition | Seat-attributed command authorization and payload codec (§7), perspective slots and per-seat blocks (§6), fixed online pump tails (§4.5), the effect pool timed from content (L9), the canonical checkpoint/digest, the tick ring and later exact restore (§9), the controller restart, the stream-application entry point | as today |
| `internal/content` | content | Effect frame holds in `SimArt` (L9); the complete simulation-content digest (§8.2) | as today |
| `internal/sim/numeric` | leaf | The retail-defined distance routine, the angle tables, the in-repository `Atan2`/`Acos`/`Tan` and the defined float-to-integer conversions (L1) | standard library |
| `internal/lockstep` | composition | The client driver: connection lifecycle, playout, grant-gated ticks, the pause and speed the HUD shows, checkpoint cadence, desync bundles and rejoin | `session`, `netproto`, `replay` |
| `cmd/nanolathe-server` | platform | The hosted relay | `relay`, `netproto`, `version` |
| `cmd/nanolathe` | platform | The main menu's MULTI entry (greyed out today in `cmd/nanolathe/frontend.go`), lobby and agreed view-policy wiring into existing camera/client controls, embedded relay and battle host | adds `relay`, `lockstep`, `replay` |
| `cmd/nanolathe-headless` | platform | Replay playback, scripted streams, desync-bundle comparison | adds `lockstep`, `replay` |

Guards in `internal/architecture` grow with it:

- **Network boundary.** `TestNetworkStaysInTheModFetcher` keeps `net/http` in
  `internal/modfetch`. It gains a rule for `net` and `crypto/tls`: only
  `internal/relay`, `internal/lockstep` and the commands may import them, and
  no authoritative package ever does.
- **Content-free relay.** `internal/relay` and `cmd/nanolathe-server` have
  dependency closures without `content`, `vfs`, `session` or any
  authoritative package, enforced the way
  `TestHeadlessCommandHasNoDesktopDependency` keeps the headless command off
  the desktop.
- **Authoritative list.** Any new authoritative package joins
  `authoritativeDirs` and inherits the determinism guards.
- **Effect pool.** The fixed pool, admission service, fragment and whole-piece
  debris arithmetic live in `internal/effects` after M1. The package joins
  `authoritativeDirs`, so all determinism guards cover it. Authoritative
  packages cannot import `internal/render`; presentation-only trails and
  drawing stay there. Effect timing is immutable `SimArt` bound at composition.
- **Conversions and library calls.** The two I2 source guards of §5.4.

DESIGN_MODS_MUTATORS decision D5 — the client uses the network for the mod
manifest and downloads "and for nothing else" — was widened with adoption to
admit a relay connection (§15 Q1).

## 15. Decisions

The maintainer decided the original questions on 2026-10-01 and approved
the follow-up policies Q22–Q25 on 2026-10-02 after the second research round.
Later on 2026-10-02 the maintainer decided Q26 and Q27, which revise Q4 and
Q6, and the online `Give` correction was recorded under Q8. Q28 collects
the protocol and design details this document had proposed on its own; the
maintainer approved them as written later that day ("looks reasonable for
now"), decided Q29, and settled §19 O23, which replaces Q28's resignation
item. A later decision or revision is recorded here first.
Adoption authorizes §16 in order and no gameplay departure beyond those named
here. M1 implementation is expressly approved, with Go's BSD-licensed
pure-Go trigonometric routines adapted unfused and credited in `NOTICE.md`.
The integer-root fidelity audit O22 is authorized separately after M1.

| ID | Question | Decision (2026-10-01 unless dated otherwise) |
|---|---|---|
| Q1 | Adopt relayed deterministic lockstep, move networking, replay and the lobby out of ARCHITECTURE §1 "Deliberately out of scope", and widen DESIGN_MODS_MUTATORS D5 so the client may also connect to a relay? | Yes (§2, §4, §14). |
| Q2 | Is multiplayer a mode-independent session kind, available under every gameplay mode, Strict 3.1 included, like Survival and the Modern AI? | Yes, with owner-machine equivalence and the online command-authorization exception of Q8; the agent instructions and the invariants were amended with this decision. Session and view policy cannot introduce a second gameplay registry: mechanical departures still use `session.RuleSet`. Single-player behavior is preserved except the declared M1/M2 changes. |
| Q3 | Is "Strict 3.1 online" defined by owner-machine equivalence (§6)? | Yes. Retail has no single multiplayer outcome to match (§3.2), and this is the reading that keeps every single-seat battle bit-identical. |
| Q4 | Which seat lends each computer seat its perspective? | The human that added it, recorded and fixed in configuration, matching the established retail host association (§6.6). Test that human becoming a watcher. **Revised 2026-10-02 by Q26:** while that human is out of play its computers keep running with its perspective in every mode; at its final removal Strict removes them with it and Modern/Community keep them running with its perspective, which keeps updating because the removed seat's per-seat block still runs (Q29, §11.1). A Modern policy using the computer's own sensor perspective requires a separate researched gameplay contract. |
| Q5 | Who may pause and change speed? | Nobody in the first releases: normal speed 10, thirty ticks a second, no pause or speed change. Relay grants still slow or hold for lag or disconnect. Retail authority is established: watchers may pause, only non-watchers may change speed, and receivers trust either from an admitted peer. Any later policy is decided before enabling §4.4. |
| Q6 | What happens to a dropped seat's units? | **Revised 2026-10-01:** retain the seat idle while rejoin is possible. On final removal Strict 3.1 destroys that seat's units; Modern and Community keep them idle. This is the Nanolathe Modern policy of §11.1, selected through `gameplay.Mode` and `session.RuleSet`. **Revised 2026-10-02 (Q26, Q27):** drop policy 1 is: retain the seat out of play idle, allow a removal vote once it has been out of play for the agreed grace (measured cumulatively, Q28), then apply the mode's final-removal rule. No timer removes a seat. Strict's final removal of a human also removes its hosted computers in ascending slot order — retail's peer removal for a voted removal, the silent deletion of retail's menu quit for a resignation (§19 O23); Modern/Community keep them running. One rule answer selects both halves (Q28, §11.1). |
| Q7 | Order of delivery: one simulation on every host, commands and identity, the digest, replays, one world per player, LAN co-op with a room-code relay, the public service, snapshots, then ranked? | Yes (§16). The canonical writer precedes digest-verified replay; exact restore has its own milestone and may move earlier if fast-forward rejoin misses its budget. |
| Q8 | Are cheat and debug commands available online? | Only with the agreed cheat permission, enforced on every receiving simulation in every online gameplay mode; competitive rooms always disable it. This includes the commands retail leaves ungated in every session kind that change the world — own-stock `+NoMetal`/`+NoEnergy`, `+Selectable` and `+LOSType` (§7.1) — and is an explicit Nanolathe online authorization exception, not historical Strict behavior. `+ShootAll`, when it is implemented, is classified under the same decision. The developer phrase and developer-only commands cannot bypass this permission or expand the allowed stream classes. Single-player permissions remain unchanged. **Recorded 2026-10-02 — online `Give`, Nanolathe online policy in every mode:** retail `+Give` is open in every session kind with no alliance test, and a negative amount reverses the transfer `[07 R-CAM-01 §6]` `[05 R-SHARE-01 §2]`, so an ungated online gift would let any seat take another's resources with every replica agreeing. A positive whole amount within the only producer's range is ordinary resource sharing for every seat — the play the share screen also offers `[05 R-SHARE-01 §5]` — so the cheat gate for world-changing commands does not apply to it. Any other amount is rejected at the validation boundary unless the room's cheat permission is on, in which case the signed retail transfer applies to every amount that producer can generate — whole numbers from −2^31 to 2^31, zero included; an amount no producer generates (non-finite, not whole, or beyond that range) stays rejected (Q28, approved 2026-10-02). Single-player keeps the retail behavior, negative amounts included; its source is the own/controlling slot at drain time (§7.1, §7.4.1). `SetResource` stays cheat-gated. |
| Q9 | Validate attack-target visibility? | Casual retains the researched admission. Ranked admission requires an approved delayed-observation/radar/direct-target contract with tests (§7.2); neither arbitrary client claims nor a current-sight-only check is accepted as the solution. Any new gameplay decision uses its existing owning rule seam. |
| Q10 | Who runs public relays? | The project runs one, reached by room code; the protocol is open and anyone may run more (§12). |
| Q11 | Identity: display names only, or accounts? | Display names until ranked play needs accounts. |
| Q12 | Do replays require the Nanolathe release that recorded them? | Yes, with matching simulation content and tested platform variants (§10). No cross-release compatibility program is planned. |
| Q13 | What decides a desync? | Casual rooms: a strict majority of participants plays on and each dissenting seat rejoins, or is held in the drop state — idle, removable only by vote (§11.1) — where rejoin is not yet offered; with no strict majority the battle ends without a result until snapshots exist (M8), and afterwards continues from the room creator's state. Rated rooms: no client majority decides, and the match is uncertified until a trusted replayer or referee rules (§9.3, §12.6). |
| Q14 | May a client that lacks the host's mod fetch it from the nanolathe.gg catalogue in the lobby? | Yes, through the existing verified download path (DESIGN_MODS_MUTATORS §5). |
| Q15 | Require identical catalogs, or negotiate a common roster as retail does? | Identical catalogs first; roster negotiation later, as a filter over the battle catalog like the restriction table. |
| Q16 | Implement retail's multiplayer unit-restriction table? | Yes, for Strict 3.1 fidelity; it is a configuration field and a lobby screen (§3.3). Brought forward by the maintainer on 2026-10-08: the first online lobby carries the host's restrictions as field 12 (§16.6); a restriction-editing lobby screen, and retail's seeded `wacky` state with it, follow later. |
| Q17 | Must the architecture support eventual competition? | **Yes, maintainer direction 2026-10-01.** Preserve scheduling/authorization boundaries and add verified results before ranked release: a trusted replayer, and a live referee only where rooms promise recovery mid-match. Who runs them, with the game content they need, and the integrity level, including whether filtered state delivery is required, remain explicit product decisions (§12.6). |
| Q18 | May a match restrict tactical views for every player? | **Yes, maintainer direction 2026-10-01.** Include shared view policy in the agreed configuration and enforce it in ordinary clients; no-zoom-out is the concrete first example (§8.4). A scale cap does not by itself equalize world viewport area or prevent modified-client bypass. |
| Q19 | Is cross-play with other engines a delivery goal? | **No planned support, maintainer direction 2026-10-01.** Focus on Nanolathe-to-Nanolathe with matching effective content/mods/settings. Other engines may be reconsidered later or dropped entirely (§8.5). |
| Q20 | How are releases, rooms and replays kept compatible? | One simulation build per room, shown in the lobby; a replay or a desync bundle names the release it needs (§8.2, §10). The installer keeps earlier releases on request so that old recordings still play, and no release is published before the cross-architecture and host-kind suites pass on it. |
| Q21 | May a snapshot rejoin restart the Modern AI's controllers on every client? | Yes (§9.1), subject to the play-test of §19 O21 before M8. The computer players lose their plans at that tick, as they already do across a save and load, in exchange for not requiring exact serialization of every brain. A room that forbids snapshot rejoin never restarts them. |
| Q22 | One shared alliance matrix or retail's per-machine stale copies? | **Approved 2026-10-02:** one shared directed matrix in every online mode, with row B its transpose. Declarations remain one-way; mutuality and all other victory requirements remain. Shared victory uses consistent third-seat knowledge instead of retail's stale copies (§6.7). |
| Q23 | Computer-seat cap, difficulty and Deathmatch admission? | **Approved 2026-10-02:** Strict retains one computer per human; Modern/Community allow multiple within available lobby seats. Every computer has explicit agreed difficulty and its fixed human host. Computers remain excluded from Deathmatch in every mode initially. Preserve Classic/Modern difficulty and income semantics; single-player is unchanged (§6.6). An online configuration carries no battle-wide difficulty word; every reader takes the computer seat it concerns (§6.6, §8.6; Q28, approved 2026-10-02). |
| Q24 | Respawn/watcher visibility rebuild: preserve retail's erased copies of remote explored history? | **Approved 2026-10-02:** one canonical explored history per player, shared by replicas. Respawn/watcher rebuild affects the entering human's own/hosted-computer perspective and histories, preserving unrelated players' exploration. This applies in every online mode, including Strict; current sight is not merged (§6.7). |
| Q25 | Apply explored-map sharing on its request tick or reproduce transport-delay timing? | **Approved 2026-10-02:** periodic map shares apply on the request tick in canonical human-seat order; explicit gifts apply on their assigned tick in command-stream order. No transport-delay emulation. Eligibility and the copied data retain their researched contracts (§6.7). |
| Q26 | When a human seat is finally removed, what happens to the computer seats it hosts? | **Approved 2026-10-02.** When a remaining retail machine removes a departed human, it removes every slot carrying that human's machine number — its hosted computers too — in ascending slot order, each through the peer-removal path that destroys its units `[08 R-LEAVE-01 §2]` `[08 R-LEAVE-01 §3]`–`[08 R-LEAVE-01 §5]` `[08 R-LEAVE-01 §9]`. Strict 3.1 online: at a human's final removal its hosted computer seats are removed with it, in ascending slot order, each by the §11.1 destruction contract — retail's peer removal for a voted removal; for a resignation it is the silent deletion the other machines perform for retail's menu quit, because the leaving machine itself takes a different path `[08 R-LEAVE-01 §7]` (§19 O23, decided 2026-10-02). Modern and Community keep them running with the host's perspective, which keeps updating (Q29), as a Nanolathe Modern policy selected with Q6's unit retention through the existing `gameplay.Mode`/`session.RuleSet` composition; one answer for both (Q28). In every mode, while the human is only out of play, its computers keep running: every client runs every computer, and the reconnectable window is itself the approved Nanolathe addition (§6.6, §11.1). |
| Q27 | How is a seat finally removed? | **Approved 2026-10-02.** The maintainer: "triggering final removal was supposed to go through a vote, anybody can trigger unless you think that's unwise." A Nanolathe online policy in every mode, with these rules from the landing owner's recommendation: only a seat in the reconnectable-drop state (disconnected) can be a vote's subject; there is no vote-kick of a connected seat, because a Strict removal destroys units. Any connected human seat still playing may call; the electorate is the connected human seats still playing, excluding the subject; a strict majority passes, a tie fails, a sole eligible seat decides alone, and an uncast ballot counts as no. A vote may be called only after the subject has been absent for at least `RejoinGraceMilliseconds`, and the subject's rejoin cancels it. No timer removes a seat. Votes are relay/lobby messages, never stream entries; the relay tallies and authors the one final-removal event. Resignation is the seat's own immediate final removal with no vote. Q28 records the approved refinements: a rejoining seat counts as still absent, absence is measured cumulatively, only a completed rejoin cancels, and the bounds and timings of §11.1. |
| Q28 | Details this document proposed on its own | **Approved 2026-10-02** as written, the maintainer: "q28 looks reasonable for now." All are Nanolathe protocol or design values, not retail constants, and may be revised here later. One item is superseded: the *Resignation* entry below was replaced the same day by §19 O23 (a Strict resignation deletes the leaver's units silently, as retail's menu quit does on the other machines, rather than applying the voted-removal contract). *Removal vote and absence (§11.1, §11.2):* a seat that is rejoining counts as out of play; the grace is the cumulative time out of play — dropped or rejoining — since the seat was last active, so repeated reconnects or a desync-held seat (§9.3) cannot reset it; only a completed rejoin cancels an open vote and resets the grace; a rejoin not completed within `RejoinGraceMilliseconds` of its admitted resume returns the seat to the drop state; a 30-second vote window that closes early once its outcome cannot change; a 60-second cooldown after a failed or tied vote against the same subject; one open vote per subject; the electorate fixed when the vote opens; the call casts the caller's yes ballot and no ballot can be changed; a battle with no active playing human ends without a result once `RejoinGraceMilliseconds` passes with none returned. *Lag (§4.4, O17):* M6's pacing state table sets a bound after which a connected seat that makes no progress enters the drop state. *Protocol (§4.2, §12.2):* the client-sequence rules, which replace the earlier retransmission contract (a repeated sequence returned its original receipt, a conflicting payload was refused), and the Refused message. *Resignation (§11.1, O23) — superseded:* the proposal that a Strict resignation apply the voted-removal contract gave way to O23's silent deletion. *Rule seam (§11.1, Q6, Q26):* one `session.RuleSet` answer selects both unit retention and the hosted computers' fate. *Per-command work (§7.4.1):* at most 10,000 area entries and an actor-times-entry product of at most 1,048,576 online. *Online `Give` (§7.1, §7.4.1):* with the cheat permission, admit any whole amount from −2^31 to 2^31 and still reject non-finite or non-whole amounts. *Difficulty (§6.6, §8.1, §8.6):* no battle-wide difficulty word in the online configuration. |
| Q29 | How does a finally removed seat take part in ending the battle, and what is "the host's last perspective"? | **Decided 2026-10-02**, the maintainer taking the landing owner's recommendation. (1) A finally removed seat is absent from the end-condition sweeps in every mode, as retail's cleared record is `[08 R-LEAVE-01 §3]` `[08 R-LEAVE-01 §5]`. Under Modern and Community its retained idle units stay live and can be destroyed, but no survivor has to destroy them to win (§6.5, §11.1). Under Strict the removal destroys them, so nothing changes there. (2) Unchanged by design: a seat that is out of play but not removed blocks victory in every mode until a vote removes it, because its units are live (§6.5). (3) The perspective its hosted computers borrow under Modern and Community is live, not frozen: the removed human's own per-seat block — sensor pass, temporary sight and the other per-seat work of §6.3 — keeps running for its seat as it did while the seat was only out of play; only its end-condition evaluation stops (§6.6, §11.1). A frozen snapshot was rejected as a second perspective representation nothing else needs. |

## 16. Delivery plan

The original milestone sequence below remains the full delivery and acceptance
plan. On 2026-10-07 the maintainer approved bringing forward a constrained
local two-client play test (§16.4), before full M3 platform acceptance and M4
replays. Prototype work may take the necessary M5/M6 pieces in dependency order;
it does not claim those milestones complete. Every single-player battle stays
bit-identical except where an existing contract explicitly says otherwise.

| Milestone | Delivers | Done when |
|---|---|---|
| **M0 Adoption** | The §15 decisions; scope changes in ARCHITECTURE and the agent instructions; the online authorization and view policies; the invariant amendments (§5.4). | Done 2026-10-01: the maintainer decided §15 and the documents agree. |
| **M1 One simulation on every host** | Effect holds in `SimArt`, the pool timed from them on every host and the resolver seam removed (L9). The retail-defined distance routine, the angle tables, the in-repository `Atan2`/`Acos`/`Tan`, the defined conversions, their moved call sites and the two I2 source guards (L1). Two standing ratchets in the retail gate: the fingerprint locks from an amd64 build as well as an arm64 one, and a lock scene long enough to fill the Strict effect pool. | The distance routine implements `[01 R-DET-01 §7]`. Bit-pattern and engine-narrowing vectors agree on native darwin/arm64, linux/amd64 and windows/amd64. The fifteen existing locks are unchanged on both architectures, or each move is explained. A new Strict lock past the tick at which the pool fills equals, at M1 landing, the step-4,500 fingerprint of the 2026-10-01 window-like probe on the seed-7 scene — a headless run with the old timing resolver installed, supplying the per-frame holds the window read; no windowed run was made. After O22 the lock moved to seed 5, whose pool fills (M1-C9). With the resolver seam removed no host can supply timing, and the guard forbidding authoritative packages, `internal/effects` included, from importing `internal/render` (`TestAuthoritativePackagesDoNotImportHostOrNondeterministicRuntime`) holds that. A windowed run against a headless one is §17's host-kind test, after M4. Complete cross-platform world equivalence is tested after M3, not claimed from these vectors alone. |
| **M2 Commands, configuration and identity** | Seat-attributed commands, receiver-side permissions, allocation serials, local interface state, explicit wire schemas with the payload audit of §7.4, complete content/build/configuration identities and match view policy fields. | Codec fuzz/round-trip and hostile-input tests; script/model/SimArt mismatches refused; every effective match field changes identity as intended. Existing fingerprints move only by removed interface bits, demonstrated with those bits masked, and by any single-player change declared with its evidence under M2-C7 (§7.4.3). |
| **M3 Canonical checkpoints and digest** | Reviewed state inventory with the effect pool in it, canonical writer and sub-digests, the tick ring, pump ends and the computer seats' applied-command hashes (§9). | Scripted single-seat scenes agree per checkpoint across supported platforms and host kinds, with host schedules replaying the same explicit pump boundaries (§16.3); a seeded one-field fault is located to its tick and owner from two rings alone; a checkpoint costs the same with AI workers idle and busy; state-owner checks and measured encoding budgets pass. Exact restore is not yet promised. |
| **M4 Replays** | Recorder, playback, pump ends, pacing notes, digest checks and a headless replay command. | Long Strict and Modern replays agree across platforms and between a windowed recording and headless playback, through pauses, speed changes and late AI workers. Reproducible desync diagnostics retain common-boundary evidence. |
| **M5 One world per player** | Perspective slots, per-seat blocks, latches and countdowns, the union gate ahead of the strips and the effect pool, and every other generalization in §6; the tail after every tick in lockstep battles (§4.5); the multi-seat harness (§17). | The harness passes for two to four human seats with computer seats under every registered rule set; single-seat fingerprint locks unchanged. |
| **M6 LAN and room codes** | Embedded relay; `cmd/nanolathe-server` hosting the same relay, reached by room code over TLS with rate and size limits and no accounts; sealed grants, relay pacing at fixed normal speed, the agreed summary, LAN/direct lobby, chat, departures with removal votes and the mode's final-removal rule (§11.1), shared view restrictions, desync detection, bundles and the casual desync policy. | Mixed-platform battles finish on a LAN and through the hosted relay from behind ordinary home routers; hostile commands are refused; a resignation and a drop made while the relay holds its grants for a lagging seat are handled correctly; §11.1's removal-vote and hosted-computer tests pass in every reserved mode; a seeded desync in a three-seat battle leaves two seats playing; all zoom entry routes honor match restrictions. Impaired-network and CPU-stall runs meet declared budgets, set before acceptance. |
| **M7 Public service** | Room list, hardened and bounded rooms/queues, seat credentials and reconnect deduplication, measured fast-forward rejoin, delayed spectators and controlled archives. | Public play plus duplicate/half-open reconnect, slow-reader, digest-withholding and embargo-bypass tests pass. Advertise rejoin only within measured limits; move M8 earlier if necessary. |
| **M8 Exact snapshots and recovery** | Full world-state readers, the controller-restart event, checkpoint transfer and continuation (§9, §11.3). | Byte-identical round trip and matching long continuations across supported modes, with computer seats restarted at the snapshot tick on every replica; malformed snapshots refused; late-game rejoin meets a declared budget; a casual desync is repaired from an agreeing snapshot. These are Nanolathe snapshots, not retail saves. |
| **M9 Competitive admission** | Trusted replayer and authenticated results/accounts; a live referee where rooms promise recovery mid-match; approved scheduling, target-admission, pause/drop, watcher and view policies; integrity-level decision (§12.6). | Unequal-RTT/host-advantage tests, hostile-client admission, colluding-hash and result-forgery tests, loss of the result authority, spectator/replay isolation and published operating limits pass. Choose filtered state delivery before claiming hidden-state protection; refereed lockstep alone cannot claim it. |
| **Later product work** | Ratings/matchmaking UX, any multiplayer save product, and filtered state delivery if selected. | Scoped after the relevant correctness and integrity gates; no cross-engine milestone. |

### 16.1 M1 work units

M1 is five units. Each owns its files exclusively and lands
on its own through the gates of ARCHITECTURE §6. U1, U3 and U5 are
independent; U2 follows U1; U4 follows U2 and U3.

**Implementation status.** All five units are implemented. Local verification
covers native Darwin/arm64 and executed Darwin/amd64 builds under Rosetta:
all sixteen partial fingerprint locks agree, including Strict at tick 4,500,
and both renderer benchmarks retain identical captures and scene census.
The first [native CI run](https://github.com/nanolathe-gg/nanolathe/actions/runs/37032586632)
at `55e95018` passed Darwin/arm64, Linux/amd64 v1 and Windows/amd64 v1.
Linux/amd64 v3 passed the committed vectors and angle-table digest but failed
two live-library comparisons: its standard library fuses arithmetic and is
not the unfused v1 reference of M1-C2. The test-only correction in `184161da`
limits live comparisons to v1 and checks an independently generated digest
of the same 16,384 radian input pairs on every target; it changes no kernel
code or existing vector/digest constant. The [native rerun at `f5c71c22`](https://github.com/nanolathe-gg/nanolathe/actions/runs/37034107271)
passed all four numeric jobs: Darwin/arm64, Linux/amd64 v1 and v3, and
Windows/amd64 v1. M1's platform gate is complete. This is not complete
cross-platform world equivalence, which still awaits M3. Until the
effect-bank loader policy of M1-C7 was shared with the client, the
simulation table compiled banks under the default loader limits, so TA
Zero's `ModFX` bank and three Escalation banks had no holds. The pool lock
has since moved to seed 5 (M1-C9).

| Unit | Delivers | Files it owns |
|---|---|---|
| **U1 Numeric kernel** | The distance routine and its truncating shortcut, the radian functions, the angle table and the two new conversions, with their tests and vectors. No call site changes, so no behaviour changes. | New files in `internal/sim/numeric`: `distance.go`, `radians.go`, `angletable.go`, `convert.go`, their tests and `testdata/`, plus the attribution in `NOTICE.md`; declaration-scoped registration in the architecture and session arithmetic guards, and I2. |
| **U2 Call sites and guards** | Every library call and raw conversion in the authoritative packages moved to the kernel; the two I2 source guards; I2's rows marked in force. | `internal/cob/ports.go`, `internal/combat/aim.go`, `internal/combat/motion.go`, `internal/construction/community_kickout.go`, `internal/construction/reclaim.go`, `internal/model/model.go`, `internal/movement/altitude.go`, `internal/movement/flight.go`, `internal/movement/integrate.go`, `internal/orders/work.go`, `internal/session/script_ports.go`, `internal/session/survival.go`, `internal/survival/pool.go`, `internal/aikit/host.go`, `internal/aikit/info.go`, `internal/aikit/mapinfo.go`, `internal/aikit/obs.go`, `internal/sim/numeric/trig.go`, new guard tests in `internal/architecture`, `docs/INVARIANTS.md` (I2). |
| **U3 Effect timing from content** | Effect holds compiled into `SimArt`; the pool timed from them at composition on every host; the resolver seam and its late hydration removed; a Strict lock past the tick at which the pool fills. | `internal/content/sim_art.go` and its test, `internal/render/effect_service.go`, `internal/render/effects.go` and their tests, `internal/session/composition.go`, `internal/session/session.go`, `internal/session/publish.go`, and the session/render fixture constructor callers, `internal/client/presentation_resolver.go` and its two tests, `cmd/nanolathe/battle.go`, `internal/headless/rules_lock_retail_test.go`, `docs/INVARIANTS.md` (I6). |
| **U4 Pool under the guards** | The fixed pool, its admission adapter, the fragment and whole-piece debris state and their arithmetic moved from `internal/render` into the authoritative package `internal/effects`, which joins `authoritativeDirs`. Presentation-only code (draw lists, trail particles) stays in `internal/render`. No behaviour change. | `internal/effects` (new), the moved parts of `internal/render/{effects,effect_service,fragment,debris,effectmath,compose}.go` and their tests, the sixteen files that name those types, `internal/architecture/retail_only_test.go`, `internal/architecture/fusion_guard_test.go`, `docs/ARCHITECTURE.md` (§3). |
| **U5 Ratchets** | The fingerprint locks from an amd64 build beside the native one in `tools/check-retail`, on hosts that can execute both; the kernel's vectors in CI on linux/amd64, darwin/arm64 and windows/amd64. | `tools/check-retail`, `.github/workflows/ci.yml`, `docs/ARCHITECTURE.md` (§6). |

**Public API.** Package `internal/sim/numeric`:

```go
// Distance is retail's two-argument distance [01 R-DET-01 §7].
func Distance(x, y float64) float64

// TruncatedDistance is TruncateFloat64ToLow32(Distance(x, y)) for every
// operand pair, reached by a shortcut whose proof is in its comment.
func TruncatedDistance(x, y float64) int32

// The radian functions. Each returns the same bits on every platform.
func SinRadians(x float64) float64
func CosRadians(x float64) float64
func TanRadians(x float64) float64
func Atan2Radians(y, x float64) float64
func AcosRadians(x float64) float64

// AngleSinCos is the sine and cosine of a 16-bit angle word, from a table of
// all 65,536.
func AngleSinCos(a Angle) (sin, cos float64)

// The conversions that complete TruncateFloat64ToLow32.
func TruncateFloat64ToInt64(v float64) int64
func RoundFloat64ToInt32(v float64) int32
```

Package `internal/content`:

```go
// EffectEntryHolds reports the per-frame holds of one effect entry, each
// max(authored, 1), for the default bank or a bank a weapon names.
func (a *SimArt) EffectEntryHolds(bank, entry string) ([]int32, bool)
```

**Contracts.**

- **M1-C1 Distance.** `Distance` performs the sequence of
  `[01 R-DET-01 §7]` in integer arithmetic, its special operands, its
  overflow and its truncating underflow included. NaN class and precedence
  are established; payload/sign remain the explicit research Unknown and
  code-site placeholder. For finite operands it equals a big-number
  model of that sequence, kept in the test, bit for bit: on every ordered
  integer pair to 3,000 in the retail tier and a sample in the fast tier, on
  raw 16.16 deltas, on fractional and wide-exponent operands, and on the
  section's vectors. `TruncatedDistance` returns the truncation of `Distance`
  for every operand pair: its comment proves when the shortcut may answer
  and the routine answers otherwise, and its test adds operands whose
  distance lies within a few units in the last place of a whole number.
- **M1-C2 Radian functions.** Their results are the Go 1.27.1 unfused
  `GOAMD64=v1` library's, bit for bit: asserted against that library in a v1
  build and against independently committed reference data everywhere.
  A v3 standard library may fuse and is not a live reference for these bits.
  Every product is rounded before it is added, and the
  fusion guard holds with no new allowance.
- **M1-C3 Angle table.** Entry `a` is the two radian functions of
  `float64(a) * 2 * math.Pi / 65536`, the expression today's sites form.
- **M1-C4 Conversions.** `TruncateFloat64ToInt64` truncates toward zero and
  returns the processor's 64-bit indefinite for a NaN, an infinity or a value
  that does not fit `[01 R-DET-01 §1]`. `RoundFloat64ToInt32` rounds to
  nearest even and returns the 32-bit indefinite, `0x80000000`, in those
  cases `[01 R-DET-01 §2]`.
- **M1-C5 Call sites.** After U2 no authoritative package calls a library
  floating-point function outside I2's exactly rounded list, and none
  converts a floating-point value to an integer outside the kernel. Both are
  source guards, with no allowance outside `internal/sim/numeric`. The
  fifteen locks hold on arm64 and on amd64.
- **M1-C6 Cost.** The simulation benchmark's process CPU per tick grows by at
  most three percent across U2. Measured on 2026-10-01, the routine alone at
  all thirteen sites costs about eight percent in the Modern 750-unit fight
  (1.82 ms to 1.98 ms a tick) and changes no fingerprint there, so the five
  sites that truncate the result call `TruncatedDistance`.
  Verified on 2026-10-02 with Go 1.27.1, Darwin/arm64 and two runtime
  workers: three alternating matched pairs of the Modern 750-unit scene
  (1,200 warm-up and 3,000 measured ticks; profiling and phase/thread timing
  disabled) measured median process CPU of 2.051 → 2.095 ms/tick, +2.15%.
  Each pair was below 3%; all scene metadata, fingerprints, RNG counts and
  census samples matched. The baseline was `6258e24b`, the implementation
  candidate `1749396c`; this is local cost evidence, not a universal timing
  guarantee.
- **M1-C7 Holds.** `SimArt` holds every entry of the default effect bank and
  of every bank a weapon definition names, compiled in sorted order from the
  battle's own files. A missing bank or entry reports unknown. Banks are
  compiled under the effect-bank loader policy the client shares
  (`content.EffectBankMaxBytes`, `content.EffectBankGAFLimits`); a bank that
  does not compile is listed in `SimArt.Diagnostics`.
- **M1-C8 One timing source.** The pool reads holds from `SimArt`, bound at
  composition before the first unit script runs. No host can supply timing
  afterwards, and the late hydration of records admitted without it is gone.
- **M1-C9 The pool lock.** The Strict benchmark scene with seed 5 is locked
  at step 4,500. Its pool is first seen full at the end of the step reaching
  tick 2,255, and by step 4,500 the effect service has refused 143
  admissions because the pool was full (a cumulative count that no tick
  reads; shatter quads refused inside the pool are excluded). The test
  asserts a refusal at capacity before it compares the fingerprint, and
  forcing the timing lookup off moves the constant. At M1 landing the lock used seed 7, whose pool first filled
  at tick 3,283; that constant equalled the fingerprint of the 2026-10-01
  headless probe that ran with the old resolver installed. O22's dogfight
  threshold correction, recorded in DESIGN_MOVEMENT_PATH §3.4, left the
  seed-7 pool at a peak of 296 records through 15,000 steps, so no lock
  exercised a full pool, and on 2026-10-02 the lock moved to seed 5. The
  original fifteen locks remain unchanged.
- **M1-C10 Guards.** After U4 the map-order, float, fusion, goroutine and
  import guards read the pool's code, and `internal/render` holds no state
  that a tick reads. The numeric-portability and both fusion guards also
  read the load-time packages whose output the simulation reads: `vfs`,
  `formats`, `internal/content` (mutators and profiles included),
  `internal/community` and `internal/gameplay`. These are listed apart from
  `authoritativeDirs`, so the other audits do not read them. A closure test
  keeps every in-module import of `internal/session` classified as
  authoritative, a load-time input or a named presentation edge.
- **M1-C11 Ratchets.** `tools/check-retail` runs the locks from an amd64
  build where the host can execute one and says SKIPPED where it cannot.

Co-op comes first: humans allied against computer players, and Survival with
several human survivors — DESIGN_SURVIVAL §1 already plans for a buddy row to
become a human seat with nothing in the wave director changing. Competitive
play is required eventually and has explicit additional mechanisms and gates.
Co-op validates the shared simulation and transport; it does not certify the
fairness or integrity of rated matches.

### 16.2 M2 preparation and work units

**Status: U0 contracts published, 2026-10-02.** M1's native matrix passed
after the v3 reference-test correction (§16.1), opening M2's milestone gate.
§7.4.1–§7.4.4 publish the command schemas, size proof, stale-reference rules
and allocation/command APIs; §8.6–§8.8 publish configuration, frozen-input
and build contracts. U1 allocation references, U4 frozen simulation content
and U5's configuration and build identities are implemented as described
below, as are U5b, the admission half (`ValidateMatchInputs`,
`NewAdmittedSkirmish`), U2, the explicit seat commands, and U6, the command
codec, the wire primitives and the join comparison, and U3, the local
interface state. **Every M2 unit is implemented as of 2026-10-02.** No
multi-seat session is claimed: M2's constructor refuses it until M5, and the
open items each unit's paragraph lists below (the replay-widening proposal,
the three admission attestations, the visited-bit question) carry into M3–M6.

**U1 allocation references, 2026-10-02.** Both successful creation paths
assign a battle-wide serial after the fallible COB bind and before creation
callbacks. Nanoframes, forced-slot reconstruction and ownership-transfer
replacements participate in the same sequence; failure does not consume it.
Slot reuse rejects the old command reference while internal raw-slot damage
keeps its retail aliasing. Committed frames copy the serial independently
of the publication-only `InstanceID`, including reuse between publications.

Near counter exhaustion, in-flight creations reserve capacity before pool
allocation, so a nested binder cannot consume an outer creation's final
serial. Success commits and releases that reservation before subsequent
callbacks; failure only releases it and retains any existing allocator RNG
draws. The reservation count is transient call state, zero outside creation;
M3 must inventory the successful counter and each unit's serial, not this
temporary capacity accounting. M8 exact restore is still future work.

The three public reference/counter accessors have lifecycle contract tests;
U2 command admission and M3 serialization are their staged consumers.
No deadcode-baseline exception is required. U1 does not yet migrate human commands to serial validation and does not
expand the existing partial fingerprint to include the new metadata.

U1's simulation-cost check compared `2691a6e0` with `0ed0e5e8` in two
alternating pairs: Modern, three 250-unit armies, seed 7, two runtime workers,
1,200 warm-up and 300 measured ticks, profiling disabled. Process-CPU medians
were 2.298 ms/tick before and 2.197 after; no regression was observed, and
the short samples do not establish a speedup. Scene metadata, initial/warm/
final fingerprints, both RNG draw counts and every census sample matched.
The final census had 725 live units and 6,218 features, with seven burning,
eleven active builds and 53 deaths. All existing fingerprint locks remain
required by the landing gates; no expected fingerprint was changed for U1.

**U4 frozen simulation content, 2026-10-02.** Skirmish and Survival entry
capture their sources before the catalog compiles
(`content.CaptureSimulationSources`), compile, resolve the map and prepare
rules and mutators through the capture, then freeze
(`content.FreezeSimulationInputs`): every admitted unit's program and model
— units never built included — the animation table, the map's OTA and TNT,
the AI profile and the extension inputs. The sealed view answers a lookup it
never captured as a defined miss and records it (`UncapturedLookups`), so a
file edited during a battle cannot reach it; the next battle sees the edit.
The §8.8 content API exists as written, plus the family and presence
constants, `ErrSimulationInputNotCaptured`, a separate `Provenance()`
accessor and the audit list. Composition binds each unit's model and program
from the frozen inputs; a battle composed without them (campaign, restore,
hand-built fixtures) reads each model once per battle through a cache keyed
by provider entry **and** content digest, and every battle gets its own COB
loader, because the old process-wide caches were keyed by name or path alone
(M2-C8). `Catalog.Hash` and `Catalog.Manifest` are unchanged through a
capture, and all sixteen fingerprint locks hold with their constants. Battle
entry on "ashap plateau" costs about 33 ms more on the game's normal path
(602 → 636 ms) and about 53 ms more with a supplied catalog (267 → 320 ms),
chiefly parsing every admitted unit model once; a live battle holds 8–21 MiB
of captured bytes, animation-bank bytes excluded. Readings the unit made,
recorded in its code comments: a manifest entry carries its fallback key only
in presence state 2 (§8.7's "empty for states 0/1"); the catalog family
digests only what the simulation reads (sounds, aliases, other maps, the
profile table, the model-name index and `Catalog.Hash`, which mixes in
provider paths, are left out); unit models are digested as the parsed
hierarchy plus derived height, feature and weapon models as authored bytes,
because they are only checked for loadability; the selected map's header,
OTA and TNT bytes and schema index are the map family, a translated-name OTA
retry is presence 2; a supplied `SimArt` is validated file by file against
the capture and, under the single-player adapter, a stale one is recompiled
rather than refused; audio keeps the live mount. Not done: campaign missions
and restores are not frozen (their entry files were outside the unit), the
session holds no field for the inputs (recovered from the unit world's COB
source), and `Catalog.Clone()` drops `Limits`, an existing defect for the
catalog's owner.

**U5 build and configuration identities, 2026-10-02 (configuration and
build-manifest half).** The `internal/session` and `internal/version` blocks
of §8.8 exist as written except `ValidateMatchInputs` and
`NewAdmittedSkirmish`, which need U4's `SimulationInputs` and follow as a
short unit. `MatchConfigRequest` carries the fifteen fields of §8.6 and the
positional seat row; `ResolveMatchConfig` validates and freezes a deep copy
with its canonical encoding, `DecodeMatchConfig` refuses an oversize payload,
an overlong or overflowing varint, a boolean other than 0 or 1, a count or
length over its bound before allocating from it, every value resolution
refuses, trailing bytes and any payload that is not the canonical encoding of
what it decodes to; `Digest()` is SHA-256 over `nanolathe/match-config/1`
and that encoding. `NewMatchConfigRequest` is the local adapter from
`SkirmishConfig` and `SkirmishEntryOptions`, which it leaves unchanged.
`version.BuildManifest` encodes §8.7's fields in order under
`nanolathe/sim-build/1`; `internal/version/stampgen` is the generator both
installers run with `go run` on the verified archive before the release
build, writing `internal/version/stamp_generated.go` (ignored by git); an
unstamped development build reports itself so. The Modern AI parameter
vocabulary check lives in the new leaf `internal/aikit/brains/utiltac`, and
the online computer-seat cap is asked of the new session-owned `SeatRules`
seam (§6.6). Tests change one effective field at a time across the §16.2
inventory, resolve equivalent default spellings to one identity, refuse a
spliced battle-wide difficulty byte, and mutate every byte of a valid
encoding (truncations, flips under six masks, 20,000 deterministic random
edits, count and length attacks) requiring a refusal or an identical
re-encoding. The startup report now names the build manifest digest or
"unstamped"; the staged API no shipped binary reaches yet (the configuration
codec and adapter, the vocabulary check) is listed in the deadcode baseline
with that reason, for U6 and the `mods/aikit` switch to drop. Readings the
unit made, recorded in its code comments: neither payload carries an in-band
version (the domain and envelope name it); `Seats` holds exactly NumPlayers
rows, so unused rows cannot be represented; fields a role does not read must
hold their canonical value (non-computers HostSeat 255, ComputerKind 0,
Difficulty 0 and no parameters; computers and the attacker a zero
participant; the attacker zero resources; every non-human seat the patch
default builder options) or the request is rejected; Classic computers keep
their merged parameters; `SharedVictory` must equal the team rule
`[05 R-SHARE-01 §1]`; the all-in-one-team refusal counts watchers; a
Survival attacker is last with group 5 and every other row shares one team;
a nonzero Community unit limit must equal the field; `MapName` is nonempty
and trimmed with case kept; content-profile `From` is a canonical key,
`To` lower-case, identity rows rejected, `From` strictly ascending; unit
restrictions get schema checks only, and `ValidateMatchInputs` refused a
nonempty list until Q16 enforcement (§16.6, 2026-10-08); the three policy durations
accept any `u32`; the mod triple is all empty or all set; AI parameters are
canonical tokens in strictly ascending key order with the 8,192-byte bound
measured on the encoding; the adapter supports one local human hosting every
computer, drops only a truncated trailing character of a name, maps a
nonzero location to 1, takes the visibility words' low bit, requires
difficulty 0..2 and refuses `AutomatedPlayers`; a zero `SourceTree` marks an
unstamped manifest; the source inventory is SHA-256 of
`nanolathe/source-tree/1`, the entry count and each entry (path, kind 1
regular / 2 executable / 3 symlink, length, bytes) in path byte order, with
empty directories not inventoried, special files, hard links, any `.git` and
`replace` directives refused; the module list is `go.sum`'s non-`/go.mod`
lines; build arguments exclude `-o` and `-tags`; the six variants are the
cleared environment's levels (amd64 v1, arm64 v8.0) with the release's
toolchain hashes; the runtime stamp check compares toolchain, platform,
level, cgo, tags, GOEXPERIMENT, trimpath and the linked modules, and
`-ldflags` only for untrimmed builds, which are the only ones the toolchain
records them for; the Windows build now also passes `-ldflags=-s -w` so both
installers share one argument list. A headless `match_config_digest` was not
added: the report builder cannot reach the entry options, and a digest
rebuilt from the normalized session would misreport the battle.

**U5b match admission, 2026-10-02.** `ValidateMatchInputs` and
`NewAdmittedSkirmish` exist with their §8.8 signatures. Skirmish battle entry
is now two steps, `prepareSkirmishEntry` (capture, compile, prepare, select
the map, freeze) and `composeSkirmish` (compose from exactly those frozen
inputs, with the presentation audio mount as a parameter); the single-player
constructors call both and are unchanged in signature and behaviour, and all
sixteen locks hold. The frozen inputs now record the selection the freeze
made — map name and files, schema index, Community digest, mutators in
canonical spelling — behind read accessors; manifest and digest are
unchanged. Admission compares the configuration against those records and
reports every failed comparison at once (`errors.Join`), each wrapped in its
category — `ErrMatchConfigurationRejected`, `ErrMatchMapMismatch`,
`ErrMatchRulesMismatch`, `ErrMatchContentMismatch` — in that order; the
map-entry selection is re-run on the frozen map file for the configuration's
seat count and must equal both the recorded and the configured schema; side
ordinals are checked for every seat, attacker and watchers included; a
unit-restriction list was refused naming Q16 until §16.6 admitted it; permissions, views,
policies and participant identities are not composition inputs and are not
compared. `NewAdmittedSkirmish` admits first, then composes only the
single-seat shape — exactly one human, no watcher, one difficulty shared by
every computer (with none, the default word, which §6.6 says nothing reads) —
from the given inputs through the back half, never through a second capture
or freeze, passing the setup through no `Normalize` (an explicit zero
resource stays zero), the agreed Community table as one final command-line
base layer the constructor checks resolves back to itself under the bound
set, each computer's parameters as its own seat layer, and a nil audio mount
(the host binds its own at adoption); any other shape returns
`ErrMatchNeedsMultiSeat`, naming M5. A retail test composes four setups —
Strict, Modern with options and mutators, Survival with a buddy, Survival
alone — both ways and requires the same content digest and the same partial
state fingerprint at entry and every 600 ticks through 1,800. Three things
the inputs cannot yet attest are `TODO(question)` markers in
`match_admission.go`: the rule set that prepared the catalog (the combat seam
answers weapon reloads during preparation, and only the Community digest is
recorded — settle by recording the preparing set in the freeze request or by
U6's cross-seat content-digest comparison); the content limits after
`Catalog.Clone`, which drops `Limits` (U4's finding; settle in `Clone`); and
the content profile's name and directory table and the mod identity, which
the mount applies before capture (settle by recording them with the capture
or comparing the mod identity from the mod library in U6). U6 needs a public
form of the unexported `freezeMatchInputs`, the front half run from a decoded
configuration; and the session keeps no copy of the admitted configuration,
which U2's permission checks and M6's view enforcement will want, or the
host keeps the value. Seven staged rows join the deadcode baseline.

**U2 explicit commands and authorization, 2026-10-02.** The §7.4.4
`internal/session` block exists — `SeatCommand` with the §7.4.2 kind numbers
and one `<Kind>Payload` record per kind, `CommandStamp`, `CommandOutcome`,
`CommandReceipt` (with one field beyond the published struct, `Diagnostic`,
the rejection text, recorded in §7.4.4), `EnqueueSeatCommand` and
`DrainCommandReceipts`; `EncodeSeatCommand`/`DecodeSeatCommand` are U6's.
Stamped entries ride the existing phase-1 queue through an unexported field
of `HumanCommand`, so the drain, the paused boundary (which stops at a stamped
entry) and `step.go` are unchanged; the session gained one field,
`seatCommands`. The former per-kind appliers are one shared implementation
(`applyBound`) that both the local adapter and the stamped path reach, and
the local adapter now captures explicit references and membership at
submission, keeping its legacy semantics otherwise: foreign handles skipped,
duplicate actors processed, the ordinary order's explicit handles visited as
a set in pool order as before. Online, every seat kind of §7.4.2 is applied
(Order, Stop, Activation, FactoryBuild, CancelProduction, Stockpile,
GroupAssign, Stance, Cloak, SelfDestruct, CancelQueuedMove, BuilderOptions;
ATM, SetResource on the issuing seat's own stock, MakeSelectable and Meteor
behind the cheat permission; Spawn behind the permission and the Modern set;
`Give` under Q8/Q28; CommunityKickout when the feature is on) or refused
with the gate it waits for: MobileBuild and CommunityOrderDrag (known-site
admission), View and Visibility (perspective and history), DoubleShot and
HalfShot (per-seat damage gates) and the reserved kinds 35–45 name M5; the
local kinds, the replay-only NoShake and SetLogo, lobby-only Gameplay and
any unlisted number are never admitted online. In the single-player replay
context every S, D and replay-only kind applies with single-player
semantics. Readings, each marked in the code: a stream position must exceed
every position accepted so far from any seat (the stream is one append-only
sequence, §4.2, so queue order equals stream order without a sort);
"unsealed" means after `Clock.GlobalTick`, the granted-tick rule being M6's
driver's; an online stamp must name a human row of the configuration that is
not finally removed and whose economy record is neither Watcher nor
observer; in the replay context the stamped seat must equal `LocalOwner`;
`EnqueueHumanCommand` in an online session accepts only local interface
kinds; online coordinates must fit the signed 32-bit raw 16.16 range (a
`TODO(question)` at `validPoint`: a formation offset can still push a point
near the edge past it); online Spawn and Kickout are refused when their rule
or feature is off, where replay keeps the silent no-op; `CommandNoOp` is a
stale or empty actor list, a stale singular actor, a stale online ordinary
target or a changed drag receipt, and `CommandApplied` means dispatched even
if the gameplay service then refused; online `SetResource` amounts must be
finite and not negative zero; selection-derived actors (implicit Order, Stop,
SelfDestruct, Stance, Cloak, GroupAssign membership) are still read at the
input boundary, not at submission, because capturing at submission would
break selecting then ordering within one batch — U3 moves it; missing content
keys are refused in both contexts; `humanUnit` admits units of the command's
issuer through a separate attribution field, set only when the issuer differs
from `LocalOwner`, so `group_destinations.go` serves remote seats without
edits. **M2-C7 declaration:** the one single-player behaviour change is the
M2-C3 correction itself — a handle captured when a command is submitted is
no longer redirected to a unit created in the same slot before phase 1; no
current producer reaches that case, and all sixteen locks hold with their
constants. Open for later units: `group_destinations.go` should take the
issuer explicitly; the admitted multi-seat constructor (M5) calls
`setOnlineSeatCommands`, M6's final-removal event calls
`markSeatRemovedForCommands`, and its stream driver enqueues in stream
order; a dead or freed handle captured by the local adapter becomes a
reference with serial 0 because the freed-slot record drops
`AllocationSerial`, and local duplicate actor lists exist — neither has a
wire form, so M4's recorder and U6 decide their treatment. Two staged rows
join the deadcode baseline.

**U6 codecs and admission integration, 2026-10-02.** The version-1 wire
primitives of §7.4.1 — u8/bool, the varint widths, zigzag `s32`/`s64`,
`text(N)`, `key`, `digest`, `id`, `amount` with its canonical zero — live in
the new leaf `internal/netproto` (standard library only, audited by every
architecture guard as `internal/version` is), with a bounded reader that
refuses before allocating; the U5 configuration codec and the build manifest
now encode through them with byte-identical output, pinned by golden digests
of the two configuration fixtures and the manifest fixture.
`EncodeSeatCommand`/`DecodeSeatCommand` encode all 28 payload-carrying kinds
of §7.4.2 by the table (both contexts for 4–10, 12, 14–16, 18, 21–27, 30, 31,
33, 34; 19 and 32 in their online and replay forms; 17, 20 and 255 replay
only), refuse the local kinds, the reserved numbers and unlisted numbers in
both contexts, and establish schema validity before any gameplay code runs:
unknown kind, extra bytes, invalid booleans, overlong or overflowing
integers, non-canonical encodings, counts over their bounds before
allocation, and a replay-only field in an online payload. Every kind is
round-tripped, truncated at every byte, flipped under six masks and randomly
edited 20,000 times in both contexts, with a fuzz target besides; the
hostile online `Give` cases, the work limits (10,000 against 10,001 entries,
128 × 8,192 against 163 × 6,433) and M2-C11 (replay `NoShake` and `Gameplay`
keep their effects, the online codec refuses the same bytes, and a replay
decode delivered to an online session is still refused at phase 1) are
exercised through decode, enqueue and the tick, comparing both random
streams, every player record, queues, groups and settings. The M2-C5 size
proof is derived from the encoder, which corrected §7.4.1's sums by 11
bytes (an area order's outer target is the null ref, §7.4.3): 2,991,707 and
451,394 bytes, with an exhaustive search confirming 104 actors × 10,000
entries as the online maximum and every other kind smaller.
`CompareMatchIdentity(local MatchJoin, remote netproto.Identity)` reports
every mismatch at once, one wrapped category each in the order protocol,
build, content, map, rules, mod, configuration (the last four are
admission's own errors, so one `errors.Is` covers both), never
short-circuiting on a matching digest; an unstamped or dirty build is
refused in a normal room, a development room is an explicit parameter
carrying a stamped common manifest, and another admitted platform variant of
the same release is admitted. `FreezeMatchInputs` is the public front half
from a decoded configuration. The identity carries no variant or binary-hash
field; the map identity is SHA-256 of `nanolathe/match-map/1` over the
frozen manifest's map-family entries; the join compares the mounted mod
against the configuration's and the other seat's, settling the mod half of
U5b's open item (the content profile's name and directory table remain
open). Readings: both contexts refuse a reference with serial 0 and a
duplicate actor (`ErrSeatCommandNoWireForm`) rather than rewriting the
command, because version 1 cannot represent either; since a stale ordinary
target with serial 0 would not replay identically if dropped, a
`TODO(question)` proposes a replay-context-only widening with no layout
change — a `ref` may carry a nonzero handle with serial 0, and `actors` may
keep repeats in captured order for every kind but the ordinary Order — which
awaits the maintainer and M4; negative zero is written as zero and refused
on decode; the codec checks every context-only rule and the §7.4.2 flag
combinations, while work limits, the agreed unit limit, coordinate range,
permissions, key existence and ownership stay with the receiver;
`Gameplay` encodes only a registered rule-set name. Eighteen reader/writer
rows left the deadcode baseline and the staged codec, join and front-half
rows joined it, because nothing shipped can call them before M4's recorder
and M6's relay. Open: M6's stream driver needs an accessor for the session's
command context; U2's `onlineAmount` duplicates `netproto.OnlineAmount`.

**U3 local interface state, 2026-10-02.** Selection, the visited set, build
pages, BigBrother and held Shift, and the online logo and shake overrides
live in `hud.LocalInterface`, keyed by `pool.UnitRef` (sorted slices, no
ranged map), so a reused handle inherits neither a selection nor a page. The
session refuses the local kinds in both contexts, reads no selection — Stance,
Cloak and GroupAssign carry `Handles`, and a command with no units does
nothing — and no longer publishes the selection, the selected bit or the
command page; it publishes `CloakRequested`, `StockpileRounds`,
`RangeEligible` and, each tick, `frame.InterfaceFacts`. Those facts are step
7's readiness verdicts, heard through a session-installed observation sink on
the units sweep (`units.SetReadinessObserver`, nil by default) that reports
each visited unit's reference and verdict in visit order before the step's
own clear and writes nothing, so a sweep with an observer leaves every record
as one without — proven by status words and partial fingerprints compared
tick by tick with and without a consumer. The host applies local kinds at
once, fills selection-derived actors when it sends a command (Order, Stop,
SelfDestruct, Stance, Cloak, GroupAssign), consumes facts per publication and
composes the presentation-only frame sections at the end of each step;
control-group assignment stays a seat command, recall is local with the
not-yet-published assignment overlaid so assign-then-recall in one batch
works. The frame buffer keeps a per-tick fact queue of 256 ticks: when full it
keeps the oldest, refuses newer ones and counts them; the host then applies
the facts before the gap and resyncs from the first frame at or after it
(selected units not ready there leave the selection; a unit unready only
inside the gap stays selected, BigBrother cycles in the gap are skipped,
visited units and pages are kept) — unreachable in normal play, since the
host drains every step. **M2-C7 evidence:** the bits moved are the selected
bit, both visited bits and the page field (bits 22–25); with those masked on
every unit record, baseline `4bedbf0f` and the candidate agree at all 58
samples (ashap every 6,000 ticks and the end in three modes, the benchmark
scene every 300 steps, the Strict pool scene every 900 steps), with and
without a facts consumer, and so do the unmasked fingerprints — the lock
scenes take no human input and never set those bits, and the page field
keeps its creation seed in `units.initialStatusFlags` — so no lock constant
moved; moving one would have been updating from unexplained output. Six
renderer captures (plain, `--shot-select`, select with a build page and
Shift, through classic and modern) are byte-identical to the baseline.
Retail's save carries the selected bit, both visited bits and the page field
`[08 R-SAVE-02 §6]`: the host hands its local selection and moved pages to
the save path, which writes them into detached copies of the status words,
and a load seeds the new local state once from the restored words
(`AdoptStatusWords`); the visited bits are a `TODO(question)` — research
names 0x40 and 0x80 without saying which a visit sets and which the `n`
cycle tests — so they are neither written nor seeded and a restored word
keeps the file's. Readings: the host consumes every completed tick's facts
before the next input batch, so local state takes the same values tick for
tick, and a selection becomes visible one tick sooner, at input; online
`NoShake` and `SetLogo` are local only while single-player keeps the
authoritative toggle and record write; the commanded page is local and the
seeded page field is the default; BigBrother's cancel-follow notice goes out
with the next tick, or at once when paused; a dying unit is reported unready
at its last visit and leaves the selection there. Open: `step.go` and
`composition.go` still use the names `resetBigBrotherEvents`/`stepBigBrother`;
`units.initialStatusFlags` still seeds bits 22–25 (removing the seed would
move the locks); `debug_capture.go`'s `DebugUnit.Selected` is always false;
the visited-bit question needs an Unknown entry in research 07.

M2 makes the command boundary explicit and the battle inputs identifiable.
It does not enable a network battle: perspectives, multiplayer sharing,
kind-3 lifecycle work and the multi-seat harness still belong to M5. A
command kind whose application requires that work remains unavailable until
its dependency lands. Tests must not make it appear implemented by temporarily
changing `LocalOwner` or `ViewingOwner` around dispatch: services retain
their own viewer state, and that substitution cannot implement §6.

**Decisions and dependency gates.**

| Dependency | M2 work that can be specified independently | Work that remains gated |
|---|---|---|
| M1 native-platform checks — passed | Native matrix evidence in §16.1 | No remaining M1 platform gate on M2; U0 contracts still precede implementation |
| Q22 alliance representation — decided | Shared directed matrix and declaration checks (§6.7) | M5 application and multi-seat victory tests |
| Q23 computer seats — decided | Fixed host attribution, mode-selected cap, per-seat difficulty and initial Deathmatch exclusion (§6.6) | Exact field encoding in U0; admission in M2 and per-seat consumers in M5 |
| Q24 history scope; Q25 share timing — decided | Canonical per-player history and request-tick sharing (§6.7) | M5 history storage/reset and sharing application |
| Q26 hosted computers; Q27 removal vote; Q28 details; Q29 end condition and perspective; O23 resignation — all decided | Drop policy 1 and `RejoinGraceMilliseconds` fields (§8.6), relay vote message schemas (§12.2), the Refused message and client-sequence rules (§4.2, §12.2) | M6 lifecycle, the one-answer final-removal rule, the silent resignation, the removed seat's absence from the end sweeps, the relay tally and their tests (§11.1); M5 keeps a removed seat's per-seat block running (Q29) |
| M5 perspectives | Serial references, codecs, explicit actor lists and checks that need only the issuing seat | Correct online `View`, visibility refresh, known-site admission and all other perspective-dependent application |
| M6/M7 service | Versioned payload and identity primitives | Live start/pacing/reconnect state machines, readiness transport and view-policy enforcement at the online UI boundaries |

Q22–Q29 and O23 are decided, so no product decision blocks M2 (the units
that encode a Q28 value — the online `Give` bound and the per-command work
limits in U2 and U6, and a configuration without a battle-wide difficulty
word in U5 — may land) or the M5/M6 lifecycle work.
U0's published schemas and APIs are contracts, not a completed encoding or
implementation. Unsupported later-milestone commands
remain explicitly unavailable rather than falling through to a local path.

**Code ownership and sequence.** The table identifies implementation
surfaces, not concurrent permission to edit every file in a directory.
Each dispatch enumerates exact files, including tests and caller migrations,
after its dependencies land. The landing owner alone edits this design,
I5/I6, ARCHITECTURE and shared gate files. Shared `session` and client files
make U2, U3 and U6 sequential. U4 and U5 may be independent after U0, with
their composition/installer integrations landed separately from any unit
already editing those files.

| Unit | Depends on | Delivers and principal surfaces |
|---|---|---|
| **U0 Schema and consumer inventory** | M1 gate passed | Finish §7.4's field-by-field schemas and §8's effective-input inventory in this document before a codec is written. Enumerate all 35 existing command kinds plus proposed new kinds; distinguish local, lobby, single-player replay, supported seat and deferred seat kinds. Trace every field to its producer and consumer, set stable explicit numbers/bounds and publish the API contract for later units. |
| **U1 Allocation references** | U0 | Creation serials at both successful creation paths in `internal/units/units.go`, committed references in `internal/frame/frame.go` and `internal/session/publish.go`, and focused lifecycle tests. Keep presentation identities for their existing cache purpose. Do not alter internal pool references. |
| **U2 Explicit commands and authorization** | U1, U5 | Seat/stream metadata at the session input boundary, explicit actor and target references, role/cheat/rule checks from the agreed configuration, per-kind validation and local adapter. Own `internal/session/commands.go` and its command helpers/tests; migrate command producers while preserving single-player behavior. Perspective-dependent cases stay gated as above. |
| **U3 Local interface state** | U2 | Selection, visited flags, build-page state, BigBrother/Shift and local logo overrides move to the client. Migrate `internal/hud` adapters, the unit readiness sweep, session publication and `cmd/nanolathe` input consumers together. Keep authoritative group assignment and online shake draws. Masked-fingerprint evidence and local interaction checks are part of this unit. |
| **U4 Frozen simulation content** | U0 | Complete content identity and diagnostic inventory owned by `internal/content`, with composition integration in `internal/session/composition.go` and the script/model creation path. Freeze every later simulation resource read; keep `Catalog.Hash` unchanged. Test a resource changed after admission and a never-yet-created unit. |
| **U5 Build and configuration identities** | U0 | Explicit versioned effective configuration, build manifest and match view fields. Session owns match composition; `internal/version` and installer tooling own build provenance. Cover `SkirmishConfig`, every `SkirmishEntryOptions` field, match/mod selection and per-seat inputs. No relay or gameplay registry is introduced. |
| **U6 Codecs and admission integration** | U2–U5, final schemas | Session owns command payload codec; `internal/netproto` holds only the agreed leaf wire primitives/identity values needed at this milestone. Join build/content/map/rule/mod/configuration comparisons, with distinct mismatch diagnostics and hostile-input tests. Live relay messages and transport remain M6/M7. |

**Public API boundary (U0).** The shared reference is a value
containing a `pool.Handle` and a nonzero `uint64` allocation serial, exposed
by the unit world and copied into the committed frame. Session owns the
typed seat-command value, its payload encoder/decoder and the adapter that
accepts externally stamped seat/tick/stream-position metadata. Encoding
never accepts that metadata inside the payload. The local adapter supplies
equivalent metadata without network dependencies. Content exposes an
immutable admitted-input value and its digest/diagnostic manifest;
composition consumes that same value. Configuration and build values have
separate canonical encoders and digests. Exact names, signatures and fields
are in §7.4.4 and §8.8, backed by their preceding wire tables. Dispatches
cite those contracts and exclusive file ownership; do not invent alternate
reference types, identity registries or codecs in parallel.

**Contracts.**

- **M2-C1 Serial lifetime.** One counter per battle, never per owner or
  publication. Each successful ordinary, nanoframe, transfer or forced-slot
  creation gets a distinct nonzero serial before the successful creation
  notification can publish it. Failed creation consumes none. Audit both
  allocator bodies and their bind-failure paths; this requirement does not
  undo draws already taken by the existing failed allocator. Exhaustion
  fails deterministically before a serial can wrap. Tests cover immediate
  reuse between publications, a failed COB bind, forced-slot creation and
  transfer. The counter and each live serial become M3 state inventory;
  M8 preserves them exactly. A retail save has no serial: a new session
  reconstructed from it obtains a new reference namespace and cannot
  accept commands retained from the prior session.
- **M2-C2 Attribution.** The stamped seat, not a payload owner or local
  viewer, controls actor authorization and seat-owned mutations. Reject a
  watcher, removed seat, foreign actor and forbidden kind at the receiver.
  Cheats require the agreed online permission in every rule set, regardless
  of developer state. Retain the researched single-player cheat and
  `Give`/`View` behavior through the local adapter, including `Give`'s
  own/controlling-slot source `[07 R-CAM-01 §6]`, to which the code is being
  corrected. No command kind can fall through from
  an unsupported online case to the legacy local path.
- **M2-C3 References and order.** Never resolve a serial mismatch to the
  slot's new occupant. For each actor list and target list, U0 specifies
  duplicate, empty-list, stale-member and stale-target behavior and the
  order retained for processing. Sorting a set for encoding is not harmless
  when order changes formations, queue mutation or RNG calls. Stream
  positions supply tracked-order sequences; pure local UI events do not
  appear in the replay/online stream. Test cancellation after other seats'
  entries and distinguish a local pending receipt from the accepted stream
  receipt. Preserve deep copies at enqueue.
- **M2-C4 Validation boundary.** Decode, validate schema and authorize
  before gameplay mutation. Reject unknown kinds/versions, extra bytes,
  invalid booleans/enums, overflowing integers, noncanonical encodings and
  counts above the published bound before allocating from them. Publish
  signedness, binary32 amount representation, string encoding, fixed-point
  widths and per-kind argument bounds. Codec and authorization rejection
  leave queues, resources, RNG and simulation state unchanged; consuming
  a rejected entry still consumes its stream position. A gameplay service's
  researched partial work is not rolled back (§7.4). Hostile-input tests
  include online `Give` from a seat without cheat permission with a
  negative, zero, non-whole, non-finite and out-of-range amount — each
  rejected before any stock or production slot changes — beside a positive
  whole gift to a non-allied seat, which applies as retail's transfer does;
  with the permission, a negative gift applies the signed transfer. They
  also include area orders over the §7.4.1 work limits, rejected whole.
- **M2-C5 Command-size proof.** U0 derives a worst-case payload size from
  the admitted unit capacity and all variable-length fields, including area
  targets and the maximum serial width. The configured maximum selection
  must fit. A kind must not split implicitly into separately scheduled
  commands: that can change group centres, repeated-click cancellation and
  same-tick order. If limits or atomic fragmentation are necessary, specify
  them and their tests before admitting that configuration. Byte limits are
  Nanolathe protocol limits, not retail constants. §7.4.1 records the
  result: 2,991,707 bytes for the largest `Order` at the representation
  ceilings (the single-player replay context) and 451,394 bytes under the
  online per-command work limits, both within the 4 MiB command ceiling;
  U6's test derives both from the encoder and refuses an area order whose
  outer target is not null.
- **M2-C6 Local interface.** A local state value is keyed by allocation
  reference, so reused handles cannot inherit selection or a build page.
  Selection readiness uses the existing predicate's complete inputs,
  including carrier readiness. Group assignment still writes the owning
  seat's authoritative group numbers; group recall changes only local
  selection. BigBrother's timing follows simulated ticks, not renderer
  frames. Preserve the phase-2 readiness clear and the subsequent
  BigBrother sweep-tail observation, including a unit that becomes unready
  and ready again before tick end. Deliver sufficient ordered facts to
  advance local state across every intervening tick when a host presents
  only the latest publication of a catch-up batch. Sampling that latest
  frame alone is insufficient. Test transient readiness, skipped display
  frames and handle reuse across the batch; the local result must equal
  consuming every boundary. Online `NoShake` affects presentation only; single-player keeps
  its existing draw behavior. Record how retained local UI state is handled
  across a retail save/load, without putting it in the multiplayer digest.
- **M2-C7 Fingerprint evidence.** Name the exact interface bits moved,
  enumerate their readers/writers and compare baseline/candidate state with
  only those bits masked. Keep group, selectable, ownership and other
  gameplay bits unmasked. Do not update constants from unexplained output.
  Allocation serials are new metadata for M3, not grounds to expand the old
  partial fingerprint and obscure this comparison. Any other single-player
  change — such as a different stale-target result (§7.4.3) — is declared
  here with its own evidence; none is assumed. Exercise explicit
  commands with different local selections; no selection fallback may
  affect an admitted seat command.
- **M2-C8 Frozen content.** Digest the effective definitions in their
  semantic order, all admitted scripts, model geometry/derived heights,
  SimArt sequences and holds, relevant map/AI/extension inputs and applied
  mutators. Record defined missing-input fallbacks. The value consumed at
  later creation is the one admitted. A global model cache keyed only by
  provider/path metadata (`loadAuthoredModel` today) is insufficient proof
  of this: a loose file can change without a new key. Test both an edit
  during a battle and a second admitted battle after that edit; the first
  retains frozen content, the second sees the new identity. Separate local
  provenance paths from identity so identical inputs on different installs
  agree. Preserve existing catalog regression hashes.
- **M2-C9 Effective configuration.** Canonicalize once, validate, freeze,
  and hash the exact value composition consumes. Equivalent default
  spellings normalize to the same value; each effective difference changes
  identity. Version and domain-separate every identity encoding, frame
  variable-length fields unambiguously, and encode ordered collections
  explicitly. Reject unknown required fields instead of silently dropping
  them. The field inventory below is a review checklist, not a license to
  hash Go memory or `NormalizedBytes`.
- **M2-C10 Build and admission.** The common release manifest identifies
  source contents, dependency/toolchain inputs and simulation-relevant build
  choices, and enumerates tested platform variants. GOOS/GOARCH-specific
  binary hashes are variant evidence, not a reason to reject another
  admitted variant of that same release. An unstamped or dirty build fails
  normal admission; a development room needs an explicit common manifest.
  A revision or `version.Profile` string alone is never a substitute.
  Report protocol/build/content/map/rules/mod/configuration mismatches
  separately. A later initial-state digest cannot replace these checks.
- **M2-C11 Single-player replay inputs.** Account for every local command
  that changes authoritative state, RNG or subsequent commands, even where
  §7.1 makes it local or lobby-only online. In particular replaying
  single-player `NoShake` and `Gameplay` must retain their effects. Context
  validation rejects the same bytes in an online battle before dispatch;
  no payload-supplied mode can bypass that check. M2 tests the boundary and
  the single-player application equivalence; M4 records and replays it.

**Effective-input inventory, starting from `fc032410`.** U0 adds every
newly discovered composition or later-creation input to this table. Tests
change one effective field at a time and also cover equivalent defaults.

| Input family | Required inventory and treatment |
|---|---|
| `SkirmishConfig` | Gameplay selection, map, occupied seat rows in slot order, controller/side/colour/team/nickname/resources/Classic-or-Modern choice, per-computer difficulty (Q23), which replaces the single-player global word online (§8.6 field 5, §15 Q28), location mode, commander-death mode, mapping/LOS/type, unit limit, both seeds and every Survival option. Resolve private defaulting bookkeeping before encoding; it is not a wire field. State explicitly whether unused rows are rejected or canonicalized away. |
| `SkirmishEntryOptions.BuilderOptions` | Initial six-value preference for each applicable seat; runtime changes remain seat commands. No receiver reads its own host preference at composition. |
| `CommunitySources`, match/mod selection | Effective rule table and digest, base/name, content profile, mod identity/version/archive digest and mutators, with deterministic precedence. Provider paths and diagnostic provenance stay separate. |
| `ContentLimits` | All effective table sizes and read caps. Admission must use the agreed limits even where a particular small fixture composes identically under two values. |
| `AIOverrides` | Resolve All, difficulty and per-player layers into the effective canonical parameters each computer consumes. Check both Classic/Modern applicability and every difficulty consumer; source strings alone do not prove effective equality. |
| `SimArt` | Digest the compiled content, including a defined missing lookup, not a pointer or whether a host passed a precompiled value. Precompiled and freshly compiled values must agree when their effective contents agree. |
| `AutomatedPlayers`, `Progress` | Online automated-player override is fixed false; progress callbacks are excluded after confirming they observe only loading. Neither an arbitrary function identity nor a host default enters the hash. |
| Additional online fields (§8.1) | Fixed computer host seats, initial team/shared-victory inputs, restrictions, cheat/watching permissions, scheduling/drop/pacing and audience policies. Encode the approved Q22–Q25 policies wherever representation or application depends on them. |
| Seat assignment | Encode the agreed human row assignment using protocol identities; keep reconnect secrets and ephemeral socket/local-host details out of the public configuration, digest and replay. |
| View policy (§8.4) | Explicit tactical scale bounds and full-map permission, with declared player/spectator/replay applicability. The native-1× preset is common policy; ordinary minimap and local graphics preferences keep their existing contracts. M2 stores/compares the fields; M6 enforces all input routes and tests captures. No common world-viewport cap is chosen here. |

**Acceptance.** Run the affected contracts during each unit, then the
whole-tree gates before and after landing. M2 final acceptance requires U0's
published schemas, all supported kinds' round-trip and bounded fuzz tests,
receiver tests that bypass the UI, stale-reference lifecycle tests, the
masked-state proof, and one-field identity/missing-input tests. Check every
script/model/SimArt mismatch before composition is admitted, including an
unbuilt unit. Simulation changes use the displayless benchmark; U3's
presentation changes additionally require both renderer captures and local
input checks, per ARCHITECTURE §6. Report unsupported/deferred commands and
pending native-platform or later-milestone gates separately from tests that actually passed.

### 16.3 M3 preparation and work units

**Status: U0 published; U1–U6 implemented; U7 acceptance in progress, 2026-10-07.**
M1's native gate is complete and all M2 units are implemented (§16.1–§16.2).
M3 is the next milestone. U0's reviewed field dispositions, encoding and
public API are in §16.3.5–§16.3.8. U1 provides the shared encoder and
reference keys; U2 adds allocation, order and script writers (§16.3.10).
U3 terrain/features/visibility and path are implemented (§16.3.11–§16.3.12);
movement writers are integrated (§16.3.14). U4 economy, construction, combat
and effects/event writers are implemented (§16.3.13, §16.3.15–§16.3.17).
U5 computer state and application recording are integrated (§16.3.19–§16.3.36).
U6 composes full battle capture and both histories. U7's local cost measurements
pass the declared limits (§16.3.83–§16.3.84); native-platform comparison remains
pending. These are Nanolathe implementation contracts under §9, not new retail
findings or gameplay policies.

**Scope.** Deliver the writer, digest, owner sub-digests, tick ring and
single-seat verification. Leave the existing partial fingerprint and its
locks unchanged. Exact restore is M8; recorder/playback is M4; perspective
generalization and a multi-seat constructor are M5; network transport and
relay desync decisions are M6. A retail save, committed frame, debug capture
or `PartialStateFingerprint` is not a complete checkpoint. In particular,
`RecordAIControllers` is a save operation that joins workers, so it cannot
be reused by the regular checkpoint.

#### 16.3.1 State-owner inventory

This overview is refined by the reviewed field dispositions in §16.3.5,
including nested records, interface-held state and exclusions. It is not
permission to serialize every Go field. Every future change to an
authoritative type updates its disposition.

| Owner and current source surfaces | State the inventory must account for |
|---|---|
| Session/runtime — `session.go`, `step.go`, `result.go`, `community_schema.go`, `sim/rng/rng.go` | Global tick; battle lifecycle and pending result/commander-death/respawn state; active rule-set name and effective rule inputs, including single-player live switches; builder options; wind, meteor and shake drivers; both RNG states, initialization and draw counts. Classify single-player local/viewing identities by their current consumers; M3 must not erase the M5 dependencies by excluding them. |
| Units and allocation — `units/units.go`, `units/types.go`, `pool/pool.go` | Player slice bounds, occupancy and definition identities, live/created counters, the successful allocation-serial counter and every live serial; complete unit/weapon-slot/attachment state. Include the freed-slot residuals accessible through `rawUnits`, not only the live walk. Temporary serial reservations must be zero at a legal checkpoint. Retain raw internal references as raw references: M2 serials do not change retail aliasing. |
| Orders — `orders/pump.go`, `node.go`, `binding.go` and owned handler state | Both queues in their actual traversal order; node parameters, flags, wait/return state and per-unit handler state, including Modern and Community additions. Define logical identity for shared/referenced nodes rather than encoding pointers. A queued unit order is world state; an unconsumed external input is not. |
| COB/model — `cob/vm.go`, `bridge.go`, `units.ScriptState`, `model` | Thread slots, execution/call/value stacks, waits, signals, statics, animation lanes and piece state; thread allocation/return identities and outstanding completion bindings. Audit callback closures: write the logical continuation and its operands, never a function address. Model/program definitions are frozen content references; pose and animation progress are mutable state. Classify render flags by their consumers rather than their names. |
| Movement and paths — `movement/integrate.go`, `layer.go`, `route.go`, `path/queue.go`, `search.go`, `heap.go`, `workspace.go` | Occupancy, filing order, class-layer cells and age/revision gates; routes including readable residual storage, steering/collision/flight state and callback edge memories; scheduler cursors, allowances and requests; every suspended search's frontier, nodes, parent links, tie order and result progress. Include Modern learned terrain, traffic claims/arrival places and repair reservations wherever future reads depend on them, even if retail saves rebuild or omit them. |
| World/features — `world/terrain.go`, `features/service.go`, `active.go`, `cursor.go` | Mutable terrain/plot and feature-reference state, relevant revisions, feature instances and animation/burn/reclaim/reproduction progress; the active list in its actual LIFO order, arena occupancy and global scan cursor. Immutable map inputs use the admitted identity. A sorted instance lookup does not replace the active list. |
| Visibility — `visibility/grids.go`, `sensors.go`, `session/visibility.go`, `eyeball.go`, `post_loop.go` | Explored masks, current refcounts, observer footprints and throttle/stamp state, completed sensor status and its cadence, mode/team/viewer-dependent inputs, and temporary-sight records in list order. The temporary-sight list is authoritative although its owner is the executor tail. Fog/minimap caches and their presentation revisions are excluded after checking their consumers. |
| Economy/construction — `economy/ledger.go`, `construction/factory.go` and per-unit work state | Every player's resources, accounting buckets, deadlines, control/alliance/share/end-condition fields; construction and production progress, factory products/queues, and Community repair banks. References to units and orders do not cause those owners' records to be written twice. Configuration values are not silently replaced by host defaults. |
| Combat — `combat/pool.go`, `service.go`, `autonomous.go`, `target.go` and rule-owned state | Projectile count/capacity/dead flags and residual records in pool order; aim readiness/continuations, persistent scan cursors and periodically rebuilt target/base lists; damage-option gates, death-notification identity and deferred transport deaths; Modern incoming-shot and other retained predictions where future admission reads them. Do not reconstruct a deliberately stale registry from today's units. |
| Effects — `effects/effects.go`, `effect_service.go`, `fragment.go`, `debris.go`, `session/strips.go`, `publicationState` | Fixed-pool occupancy/order and animation players, fragment geometry, debris arena allocation/linkage, all ten strip families and their particles. Audit effect event admission, sequence suppression, pending records and identity exhaustion before excluding any publication/event fields. Pixel/material caches stay local only when their values cannot affect admission, lifetime, geometry or RNG. |
| Computer players — `ai/manager.go`, `strategic.go`, `groups.go`, `build.go`, `aikit/host.go`, `cmd.go` | Classic planner records, deadlines, ordered groups and engine upkeep for both controller kinds. Add the per-seat applied-command history and next reaction deadline of §9.1 on the simulation thread. Explicitly classify the Modern executor's APM bucket/refill time and retained placement reservations; the worker-memory exception is not a blanket exclusion of everything behind `Manager.Ext`. |
| Mission/scenario — `mission`, `triggers`, `session/mission.go`, `survival.go`, `internal/survival` | Trigger progress and event latches, scheduled schema spawns, Survival accounts, phase/deadlines/wave plan/spawn cursors/retarget state and unit membership; score state where it affects the result. Pure report history may be excluded with its readers recorded. Survival's ban on saves does not exclude it from digests. |

A field belongs to one canonical section even when several services hold
its pointer. U0 records the ownership edges, including the scheduler shared
by session/movement, COB state held by units, temporary sight held by the
session but affecting visibility, and effect storage beside publication.
Sub-digest labels name logical owners, not a reflection dump of packages.
The immutable keys already used by `content.SimulationInput` distinguish
family, record ordinal and key; use that identity rather than a name alone.
A dynamic definition or continuation that has no such key needs an explicit
logical representation before its writer can land.

**Exclusions require evidence.** Host clock anchors/carry/speed hysteresis,
network/input queues, local interface state, audio playback, frame buffers,
renderer caches, tracing and profiling are outside world state (§4.4, §9.1).
Keep consumed stream position and pump information with the checkpoint as
external metadata, without hashing unconsumed inputs. A scratch field is
excluded only if overwritten before every read; a derived field only if its
reconstruction preserves all values and iteration/tie order. Do not infer
this from a `Save` omission or a comment calling it a cache. The Modern
brain, private generator, observation and unapplied command contents retain
§9.1's explicit exclusion; M8 owns the coordinated restart.

#### 16.3.2 Capture and encoding contracts

**M3-C1 Boundary.** Capture on the simulation thread after all work belonging
to the checkpoint tick, before any next-tick input or paused-input mutation.
For single-player, an interior tick of a multi-tick pump is captured after
its publication; its final executed tick is captured after
`runRetailPostLoopTail`. A pump shortened by battle termination uses its
actual final tick. `publicationObserver` currently runs before that tail,
so installing the writer there alone is insufficient. Use the same boundary
for full digests and the tick ring. Capture entry state separately, after
battle-entry initialization, without labeling it a completed runtime tick.
Zero-tick pumps do not append or replace a tick checkpoint (§4.5).

M3 keeps single-player pump semantics. Tests compare different host schedules
while replaying **the same explicit pump boundaries and input positions**;
arbitrary regrouping of single-player ticks can legitimately change sight
expiry. A targeted test with a temporary-sight expiry inside a multi-tick
pump must expose that difference. M5 later gives each granted online tick
its own pump. A known pre-M5 host-kind dependency remains a reported gate;
M3 cannot make it disappear by leaving the affected state out of the hash.

**M3-C2 Canonical values.** One versioned byte schema, independent of Go
layout, native integer width, memory address and map iteration. Specify
section IDs/order, field widths, signed representation, lengths, presence
and definition/reference keys before writing a codec. Preserve semantic
sequence order, including heaps and linked lists; canonical sorting is only
for unordered lookup tables. Every floating field keeps its exact bit
pattern, including signed zero. Refuse NaNs with the owner and logical
field path, without returning a usable digest or changing the world (§9.1).
Do not reuse the online `amount` validator, which normalizes/rejects values
under a different contract. Do not add a new floating arithmetic operation.

**M3-C3 Identity and failure.** Bind the schema to M2's complete content
identity and effective configuration; keep build/platform provenance in the
comparison/bundle metadata. Name any missing admission attestation rather
than assuming equal initial state proves equal inputs. Missing owners,
unrepresentable references or non-quiescent transient state fail capture;
a nil optional owner has an explicit schema representation. No reflection,
`unsafe`, retail-save encoding or renderer accessor supplies the schema.

**M3-C4 One writer.** Stream the canonical encoding to an `io.Writer` so
hashing need not allocate a full snapshot. The complete digest is SHA-256
over those exact bytes; only its first 16 bytes go on the wire (§9.1).
Sub-digests use the same owner encodings and an explicit owner/schema domain,
not a second hand-maintained field list. Diagnostic byte capture and digest
capture must agree. Writer errors return failure, never a digest of a
silently truncated state. Capturing, repeating a capture or enabling history
changes no world value, RNG draw, queue, pending callback or worker state.

**M3-C5 Modern AI boundary.** Regular capture never calls `Join`,
`Generator`, `RecordAIControllers`, a brain method or a worker-owned reader.
The simulation thread publishes a value-only record for every computer seat:
controller kind, applied-command chain and next-deadline presence/tick, plus
any engine-side fields the inventory classifies as authoritative. Distinguish
an absent/pending deadline from tick zero. Record the actual application
order and allocation references, not Go unit pointers or the worker's batch
storage. U0 fixes the applied-command encoding and the precise commit sites,
including accepted no-ops and failed/partially applied actions, before U5
instruments them; hashing emitted intents or aggregate success counts alone
does not satisfy §9.1. Do not route Classic AI through a new command path
or change Modern AI's scheduling/APM behavior to make it easier to observe.

**M3-C6 Bounded diagnosis.** Initially retain the last 64 owner-digest
checkpoints at the 30-tick cadence and a 600-tick ring (§9.2). A ring row
contains its tick, both RNG states/draw counts, pool counts and per-owner
fixed-width summaries. Computing the row must not call the canonical writer
or retain a whole snapshot. U0 specifies the summary algorithm, included
fields and explicit blind spots; a cheap rolling sum is diagnostic evidence,
not a complete equality proof. Compare only overlapping ticks and report
when the first divergence predates the retained window or the owner is not
covered. Host reads copy records without draining data needed by a later
bundle. Measure both the per-tick cost and retained memory; bounded row count
alone does not establish a useful memory bound.

#### 16.3.3 Public API gate and work sequence

**The U0 API is published in §16.3.6–§16.3.7.** Session owns checkpoint
boundaries, capture configuration and bounded report/history access.
Each state owner writes its logical section through a shared encoder; it
must not export mutable internals or import session to do so. A value-only
Modern AI checkpoint record belongs at the existing `ai`/`aikit` boundary,
so session continues to avoid importing the controller implementation.
Content owns canonical definition-key resolution. Do not add a gameplay
seam, registry or selectable policy: checkpointing observes the bound rules.

The shared streaming encoder lives in the stdlib-only leaf
`internal/sim/checkpoint`; typed content keys remain content-owned. The
published block covers error propagation, graph references, owner methods,
AI history, boundary metadata, diagnostic byte requests and bounded history.
Exact snapshot readers and relay APIs remain outside it.

| Unit | Depends on | Deliverable and principal ownership |
|---|---|---|
| **U0 Inventory and schema — published** | M2 acceptance evidence reviewed | Reviewed dispositions and schema/API in §16.3.5–§16.3.8. Implementation prerequisites and unsupported cases are explicit; no writer is claimed complete. |
| **U1 Encoder and reference keys — implemented** | U0 | Streaming primitives, domain/version framing, error propagation, content-owned key resolution, authored byte vectors and a tiny test owner. No whole-world completeness claim. Exact files/new leaf package are named by U0. |
| **U2 Units, orders and COB — implemented** | U1 | Owner writers and focused mutation/exclusion tests in `internal/units`, `pool`, `orders`, `cob` and `model`. Cover slot reuse/residuals, suspended orders, thread reuse and logical callback state. |
| **U3 World, visibility, movement and paths — owner writers implemented** | U1 | Terrain/features/visibility and path implemented (§16.3.11–§16.3.12); movement metadata and writers integrated (§16.3.14). Owner writers/tests in `internal/world`, `features`, `visibility`, `movement`, `path`. Cover temporary-sight data through the session integration, stale grids/registries, list order, active searches and Modern policy state. This is a sequencing group: split it into bounded, exclusive-file dispatches after their shared key/API needs are resolved. |
| **U4 Economy, construction, combat and effects — owner writers implemented** | U1 | Owner writers/tests in `internal/economy`, `construction`, `combat`, `effects`. Session-owned strips and shared event-admission glue are integrated by U6. Include full pools, deferred work and feature/transport effects. Split along existing owners when dispatching. |
| **U5 Computer-player state** | U1; reference contract from U0 | Classic/engine-upkeep writer in `internal/ai`; Modern simulation-thread application/deadline record in `internal/aikit`. Worker-schedule and application-order tests. No brain-policy edits, worker joins or new commands. |
| **U6 Session capture and history** | U2–U5 | Session/runtime/mission/trigger/Survival writers; compose each owner once; capture at actual pump/tick boundaries; digest cadence, sub-digests and tick ring. Own shared `internal/session` integration, with exact external owner files assigned separately. No network battle, recording format or restore reader. |
| **U7 Acceptance and cost** | U6 | Scripted single-seat harness, native-platform and host-kind evidence, two-ring fault diagnosis and measurements. Reuse the existing gates and simulation benchmark; landing owner owns shared gate/CI edits. Record exclusions, unsupported cases and measured budgets here before marking M3 complete. |

These rows are sequencing/ownership groups, not permission for one agent to
edit every named directory. Every dispatch lists exact files and a reviewed
API; shared `session` files remain with U6. U2–U5 may proceed independently
only after U0/U1 are green. New exported staging APIs must have real consumers
or explicitly justified deadcode-baseline entries; no unused convenience API
is added for a speculative M8 reader.

#### 16.3.4 Acceptance and carried gaps

**M3-C7 Completeness evidence.** Review each owner's field dispositions
against its actual readers/writers. Use small authored fixtures to mutate
representative retained fields and verify the full digest and owning
sub-digest change; vary excluded caches, allocation addresses, lookup
insertion order and presentation settings and require no change. Include
same-name definitions with distinct record identities, a freed/reused unit
slot, a paused COB return/wait, a suspended search, stale target lists,
temporary-sight expiry, a capacity-full effect pool and Survival spawn
progress. Tests of a writer against its own output are insufficient.
Serialize/restore/serialize and long restored continuations remain M8 tests.

**M3-C8 Equivalence and diagnosis.** Scripted single-seat Strict, Modern
and Community scenes use the same content/configuration, input positions
and pump ends on native Darwin/arm64, Linux/amd64 v1/v3 and Windows/amd64.
Compare every requested checkpoint, including recorded pumps of one to five
ticks and pauses. Vary Modern worker completion timing while holding its
reaction contract fixed; checkpoint reads never wait and later application
histories agree. A seeded fixed-width fault between full-digest ticks must
be located to its first retained tick and owner from two rings alone. Also
test ring wrap, absent overlap, owner-summary blind spots and error reporting.
Exercise windowed/headless hosts and audio/presentation variations; a failure
owned by the already-staged M5 work is recorded as a remaining gate, not
"fixed" by omitting state or changing single-player behavior. M4 extends
these tests to actual replay files and windowed recordings.

**M3-C9 Cost and landing.** Measure bytes, encoding CPU/allocations per
checkpoint, digest-only versus byte capture, ring CPU per tick and retained
memory on matching busy scenes. Compare disabled capture with the baseline
and enabled capture at §9.2's initial cadence. Use sequential `tools/sim-bench`
runs and the host coordination in ARCHITECTURE §6; report matching scene
metadata and census. Measure both idle and busy AI workers separately from
ordinary deadline joins. Set numerical acceptance budgets after exploratory
measurements and before the acceptance run, rather than declaring whatever
was measured acceptable. Preserve all existing single-seat fingerprint
locks and RNG histories. Run affected contracts, then `tools/check` and
`tools/check-retail` before and after landing; use the full retail tier when
the change affects the long-run contracts ARCHITECTURE §6 names. This
U0 documentation and the unwired U1 foundation do not change the tick;
live capture cost is measured after U6 enables it.

**Carry-forward ledger.** M2's replay-only zero-serial/duplicate-actor
proposal remains for the maintainer and M4 (`seat_command_codec.go`); M3
must not silently widen the command schema. The visited-bit meaning remains
a research question, with no guessed mapping in the checkpoint inventory.
The preparing rule identity and cloned content limits are now admitted by
§16.3.34. Profile name/directory attestation remains an explicit admission gap
(`match_admission.go`); §16.3.8 assigns the fail-closed admission conditions
before that identity claim is enabled.
The mod comparison already added by U6 is not an open item again. Unit
restrictions added after M2 also need an inventory of their effective content
and limits; online field-12 admission remains separately staged (§8.6 and
DESIGN_MODS_MUTATORS §15). None of these is settled merely by producing an
initial-state digest. O10 remains open until the reviewed inventory and
cost evidence above exist; exact continuation stays with M8.

#### 16.3.5 U0 field dispositions

**Reviewed implementation inventory, 2026-10-06.** This is a contract for
writing the state present at `4230f6c36`, not evidence about retail behavior.
The owner tables below refine §16.3.1. **Retain** means actual stored values,
even when a supported rule switch is needed to expose a reader; **binding**
means an admitted immutable identity or a checked composition edge; **exclude**
means the stated reconstruction or no-reader reason applies. These decisions
must be reviewed again when the named implementation changes. Existing retail
gaps are not closed by preserving the implementation's current values.

The field names in these tables are schema names. Within a record, encode
retained fields in bytewise lexical order of their source spelling; expand
abbreviated coordinate/array families into their individual named fields.
Nested records use the same rule unless an explicit framing/order below overrides
it. Source declaration order, padding and
pointer layout are irrelevant. Source fixed-width scalar types give the wire
width; Go `int`/`uint` use 64 bits. Named numeric types use their underlying
width. Sequences carry their length and preserve their stored order; maps
sort by their encoded logical key (numeric keys numerically, strings by raw
bytes, compound keys componentwise). Fixed arrays omit a length. Presence is
explicit for optional values. Each writer documents its expanded field list
beside its implementation; a new retained field changes the schema version.

**Session, runtime and scenario.**

| Type/source | Retain or bind | Exclude or boundary condition |
|---|---|---|
| `Session`, `session.go`, `state.go`, `result.go` | `State`, `Clock.GlobalTick`; `Gameplay`, active `Rules` identity, effective `Community`, `EntryCommunity`, mutators/restrictions and builder options; RNG initialization, entry seeds, both stream states and draw counts; `LocalOwner`, `EnemyOwner`, `ViewingOwner`; `VictoryDone`, `DefeatDone`, `Latch` (`Countdown`, `Bits`, `Pending`); `resultPending`, winner/loser/reason/draw fields, `resultArmedTick`, commander-death array, all Deathmatch counters, `deathsWithNoRecordedCause`; latched result `Ended`, `Draw`, `Winners`, `Losers`, `Reason`, `Tick`, `ArmedTick`, `Countdown`. | No pending battle transition at capture. `result.Kind`, `WinnerTeam`, score presentation and column maxima are result views; retain their underlying accounting/scenario state instead. `Snapshot`, publication copies, scratch walks, trace/probe/observer fields, diagnostics, `bigBrother` facts, HUD/debug display and audio device state are excluded. |
| Session configuration | Retain the effective skirmish/player values, mode-independent options, campaign slot/side/known arrays, `Progress` bank fields, and `battleEntryTailDone`; immutable mission/map inputs are admitted bindings. `seatCommands.removed` belongs here when implemented. | Nicknames/colours and source provenance are metadata unless a live consumer affects work; strip colour selection is separately classified below. `rulesDefaultsApplied` is load bookkeeping. Human/network queues, sequences, receipts and last consumed stream position are external metadata; require no command dispatch in progress. Clock anchor, delta, carry, requested/active speed, pause and slew are host pacing (§4.4). |
| `Wind`, `MeteorState`, camera shake driver | Wind `Strength`, `Heading`, `Scalar`, `DirX`, `DirZ`, `NextChange`, `Changed`, effective `Min`/`Max`; every `MeteorState` scalar plus its weapon identity; shake active/duration/remaining/amplitudes/offsets and `noShake`, because the driver controls CRT work. | Wind `LastChange` has no runtime reader; `BriefingCountdown` is front-end state. Exclude camera/view transforms, not the driver's RNG gates. |
| `postLoopState`, `eyeballRecord`, visibility stamps | Temporary-sight records in actual list order: `owner`, `sightDistance`, `heightByte`, `x/y/z`, `expiry`, `cx/cz`, `emitter`, `published`. Stamp map by handle, each `cx/cz/radius`. | Message-retirement callbacks, tail traces and publication counts do not control sight. The boundary still distinguishes interior and final pump ticks (C1). |
| `communitySchemaState`, mission/triggers | `active`, mission binding, `playerByStart`, `neutralOwner`, ordered `deferredPlacements`, `nextDeferred`; ordered victory/defeat trigger lists, each `Kind`, `Type`, `Args`, `Completed`, `Celebrated`, `CenterReady`, `CenterX/Y/Z`. | Mission type, schema/start positions, placements, specials, initial features, wind bounds, authored triggers, difficulty, use-only and campaign selection are immutable input bindings. Diagnostics are excluded. Never replace mutable trigger progress with the authored trigger list. |
| `survivalState`, `survivalUnit`, `survivalClass` | `attacker`, slot-ordered `team`, `settled`, all account `Stock/Capacity/Earned` pairs; effective `tuning`, `opts`, pool records and indices; centre/start class/region, `classes` in allocation order; phase/end, wave/plan (group/pick order), spawn cursors, last spawn, wave-unit membership, retarget deadline; survived/wave points/clean-loss, per-player stats, removed-health map; each unit's `h`, `wave`, `target`, `infecting`, `shun`, `shunSerial`, shun cell/deadline. Classes retain key, copied profile, region dimensions/labels/sizes and base. | Region caches are created against then-current terrain: do not regenerate them from today's world. Pool definitions use content keys. `walk` is rebuilt scratch. Deposit/report history and coordinates have only setup/report consumers after entry; exclude them. |
| Survival shared AI input | `info` is one scenario-owned record, including ordered warnings and their data; managers bind that same record. Retain the wave budget, group angles/domains/picks, pool tier/domain/cost data and all effective tuning values. | Worker-owned copies remain under §9.1. Survival's result view is reconstructed from the retained director and stats. |

**Allocation, units, orders and COB.** Raw handles keep existing weak-handle
semantics; allocation references additionally preserve object identity. A
stale pointer is not silently redirected to the current occupant of its slot.

| Type/source | Retained logical fields | Exclusions and representation |
|---|---|---|
| `pool.Units`, `units.World` | Arena limit, alive/definition arrays, player slice start/end bounds; physical slots tagged never allocated/live/freed residual; live/created counters and last successful allocation serial. A freed raw record retains exactly `Handle`, `Owner`, `Kills`, `Remaining`. | Validate `used` from alive records; `slotIndex` is the identity index. Require zero pending allocation serials. Finalized definition-index maps derive from the admitted catalog; unfinalized fixture maps must be explicitly represented or refused. Iteration hints are scratch. |
| `units.Unit` | `AllocationSerial`, `Handle`, `Owner`, `X/Y/Z`, `Health`, `MaxHealth`, last damage side/cause, `Alive`, `Dying`, death cause/hooks, `Remaining`; `Flags`, build/busy/yard/bugger-off/armour/building/group/mover/restored-mode/pending state; occupancy/sight cells, footprint, structure facing, reveal deadline; bob phase, engagement target, metal spot, activation/cloak/hidden/kills/paralysis/stun; current/prior samples and move tier; placement index/identity/name. Retain physical piece flags. | Definition, scripts and orders are explicit edges. `LOSByte`, restored AI group and weapon target-fixup words are load staging; minimap `BlinkSuppress` and unit `Move.PendingHeading/PendingSpeed` are presentation/parity bookkeeping. Do not confuse those last two with authoritative movement steering. |
| Unit nested records | Every weapon slot's `Reload`, `Flags`, desired yaw/pitch, `Ammo`, muzzle/aim-origin pieces, distance, weapon key, target and aim readiness (`IssueBit`, `Ready`, `readyWord`); target kind/raw unit/X/Z. Move mode/mirror/heading/pitch/bank/speed/velocities. Attachment carrier/piece and ordered cargo. | Script/VM/bridge aliases must agree, not be encoded as unrelated copies. `readyWord` is separately restored and cannot be reduced to a Boolean. The unresolved desired-aim initialization question stays open. |
| `orders.Queue`, `Node` | Primary/secondary order; danger and firing-position state; `lastPumpTick`. Each node retains ID/phase/gates/deadline/owner/target, goal/guard/cache coordinates, parameters, creation/satisfied/flags/move/path state, build key/facing, caption flag, human move sequence, crowded-arrival and automatic-work/attack/next-target state. | Require no detached pump node and no detached-successor context. Installed nodes must have consumed `QueuedIssue`/`GoalSupplied`. Retail subtype words are restore/save staging after payload reconstruction. `secondaryTick` and diagnostics are debug-only. |
| Queue auxiliary state | Danger impacts/contacts in slot order with every validity, sector, tick, coordinate and failure-deadline field; response/resume/return nodes, anchor/withdrawal/decision/quiet/opportunity state. Contacts preserve unit allocation identity. Firing-position node/owner/target identities, active/attempt/start state. Crowded-arrival active/since/lastTick/X/Z/goal coordinates. | A node or old allocation retained outside the current queue is still a graph root. Queue index is not an object identity. Attested handlers/adapters and their owner bindings replace function pointers. |
| `cob.VM`, `Thread`, `axisAnim` | All eight thread slots, every stack cell (including cells above SP and in inactive threads), PC/status/SP/sleep/waits/signal mask; statics, pieces, animation lanes, dirty, active count, tick denominator; thread identities, next identity, last-return value/validity/identity arrays. Every move/turn/spin lane target/speed/busy/acceleration/active field. Piece rotations/translations. | Local allocation can reveal old stack cells. Exclude diagnostics, drain/pose caches, cache revisions and scratch busy flags. Piece shading/visibility cache booleans are recomputed; the active unit render-flag store is retained because COB/debris read it. Presentation-only VMs are unsupported (`presentationInstructionLimit` must be zero). |
| COB binding/bridge | Program/model identities; VM alias, `createInvoked`; pending gameplay return continuations, identified by VM/thread allocation identity, continuation kind/mode, target unit allocation/weapon slot, and captured raw deletion key. | Program code/model data are frozen inputs; piece links derive from those inputs. Lifecycle trace queues, link notes, immediate last-started/last-query scratch and transform caches are excluded. A trace-only return closure is not a gameplay continuation. |

The production asynchronous gameplay return is combat's slot-aim completion.
U2 adds its value descriptor at the existing callback installation site; it
does not execute, replace or cancel the callback. Trace-enabled return
closures must produce the same state as tracing disabled. An unrecognized
callback with a gameplay effect fails capture. `combat.pendingAims` itself
has writes/deletes but no reader and is excluded; the actual readiness and
continuation cannot be excluded with it. Feature sequence/geometry/burn/smoke/
steam/sound ports, and visibility/movement reader ports, are checked composition bindings;
non-nil function pointers alone do not establish the admitted binding.

**World, visibility, movement and paths.**

| Type/source | Retained logical fields | Exclusions and representation |
|---|---|---|
| Terrain/plots | Row-major occupancy words, metal, feature index/sentinel, anchor/damage, gameplay flags; `metalSeeded`; feature names/definitions in record order, including nil rows and runtime appends. | Immutable geometry/heights, physics/map constants and LOS words bind admitted map inputs. Entry void sweep must be finished. Never-explored marker and placer nibble are presentation-only; retain any otherwise unclassified flag bits. Static obstacle revision is diagnostic today. |
| `features.Service`, `Instance` | Global reproduction cursor, arena held, instances by sorted anchor index, exact active head-to-tail order; definition, cell/position/velocity/orientation, footprint, burning/animating/countdown/suppression, cursor frame/delay/sequence presence/key, active/arena/runtime-live flags, animation selector, damage accumulator. | `runtimeLive` affects replacement transforms. Lookup caches rebuild from sorted keys; active linkage reconstructs from the separately retained active order. Reject active-walk/pending-burn handoffs. Last reproduction index, reclaim/status/sinking/settled/shadow presentation and saved anchor staging are excluded. |
| `visibility.Service` | Semantic mode bits, dimensions, row-major word mask; byte grids by player then row; local/team/viewer-defeated; footprints by observer ID, retaining owner/cells/height/radius/quantized/live/stored cells/stored byte; effective Community inputs. | Fog caches, mode cache-valid bit, publication versions/identities and rebuild flags are excluded. Spokes derive from immutable ray tables. Sensor index rebuilds every sensor tick; `sensorInputs` and `sensorStatusByID` are diagnostics without production readers. Actual contact bits live in unit flags; cadence lives in session/ledger. |
| Movement base state | Per-handle routes, steers, collisions, flights, copied profiles/names, working sets, previous move tier/SFX band; layer registry, learned terrain, pending layers, active orders/next activation, arrival handles, move/record goals, provider, first requests, unreachable/jam/traffic/pocket state, work tick/smoothing budget, pilots, repair landings, air-base lists. Retain effective fallback/Community/path-player/unit-limit inputs and current tick. | Require tick ended, overlap scan inactive and provider eligibility cache invalid. Diagnostics, path failures, history/lab counters and per-call scratch walks are excluded. `passAlliance`/`trafficNow` are replaced at BeginTick for the recognized pure rules; unknown stateful rules cannot claim that exclusion. |
| Route/profile/steer | Entire route `Points[20]`, count/active/dirty/repath/request/status/first-hold/pending; all eight profile footprint/water/slope fields; steer X/Z, heading/pending heading, dirty/speed/max velocity/turn rate/height/sea-level/definition flags/acceleration/brake. | Inactive route storage remains readable. `Route.StaticRevision` has only diagnostic readers. |
| Collision/flight | All collision scalar fields, yard values, half-bias state, filing, saved/proposal/stamp/blocker/lean/turn fields; all flight scalar fields, including mode mirror, targets, gravity/bank/pitch, plus unit/command edges. Filing retains `Filed`, `OffMap`, `SX`, `SZ`, `Seq`, including cargo. Selected air sector is nil/record index/sentinel and is not recomputed from position. Wire tags are nil 0, record 1 followed by zero-based u32 index, sentinel 2. | Flight-command `Flags` is stream/publication bookkeeping. Flight command otherwise retains payload/owner/unit, position, velocity and heading. Immutable air-sector records bind terrain. Conservative retained collision residuals are not reconstructed from current transforms. |
| Occupancy/layers | Row-major ground/air cells, plane dimensions, link sequence, off-map and sector heads, all link next/prev/cell/linked/off-map values; pending filing rows and duplicate-suppression rows. Layer names in allocation order, membership, each copied profile/dimensions/packed cells/watermark and per-handle commit tick/set. | Encode logical occupants as presence plus signed i64 identity, not identity-plus-one storage. Ground/air counts derive from occupied entries. Grid revision is diagnostic. Plot occupancy and mover occupancy may differ legitimately. Class stamp buffers are overwritten scratch. |
| Movement goals/state | Active order/token, arrival order/goal/threshold/payload/border, move order/coordinates/goal, record goals in installation order; working-set search/goal/activation/through. Learned-grid dimensions/words. Clearance route order/cells; unreachable order/activation/since/goal; jam run/replan/until/limit/cooldown/pocket; traffic side/deadline/round/steering/ahead/through/routeless/start/goal presence/coordinates; pocket order/since/grants/token. | Goal pointer sharing affects working-set reuse. Preserve aliases. Repair landings retain admission order, exact unit/node/pad, piece/reserved/holding/anchor. Movement owns the stale combat air-base lists once. |
| Claim/arrival pilots | Claim dimensions/have/serial/own rows; nullable owner grids, all/slow directional counters and written lists. Arrival move-ground/standby, per-handle rows (all seen/node/place/footprint/coordinate/check/exchange/member/best/stood fields), claim/generation/reservation grids. Composite pilot has four ordered nullable child states. | Claim trail and arrival live/fresh/member walks reset before use. Counter wrap is not permission to discard marks: captured claim serials and arrival row membership tags remain readable. |
| Path provider/scheduler | Requests by player/handle with raw unit, player, start, goal and activation; provider cursors/started/tick/players/limit. Scheduler base/set/scales/call count/have-last, optional active request/player/scale, player cursor/service counts/accumulators/allowance/unit limit/player count. | Staged indexes derive from keys; sweep polls reset per call. Inactive request residual, traces and diagnostics are excluded. |
| Goals/searches | Point centre/radius/**radiusSq**; annulus centre/inner/outer/**innerSq/outerSq**; rectangle. Saved forms reconstruct those same three variants. Search config descriptor and scale; logical entries sorted `(Z,X)` with status/direction/node; nearest/distance/presence, tolerance/presence, notified/seeded/done/result points/status, popped/setup/expanded counts; nodes in allocation order with cell/G/H/F/terrain/run/parent/direction/open/closed/hSet; heap **array order** `(id,f)` and spent state. | Thresholds are independently stored, not recomputed. Heap positions/node index back-references are validated derivations. Workspace generation/capacity/lending/dense-versus-sparse storage is excluded after extracting every logically visible entry, including rays/terminal entries without nodes. Search fan is scratch. |
| Search wrappers/payloads | Straighten/smooth variant, wrapped search, config, probes, done/status/out. Air marker flags/radius/altitude/heading/attach piece/unit/raw target/goal/radial; velocity marker saved flags/aux/trailing, unit/position/velocity/commanded/steer. | Wrapper finishing buffers are synchronous scratch. Payloads preserve sharing with record goals and flight commands. Unknown goal/search/kernel/pilot/payload variants fail capture. |

Suspended path closures require metadata at their creation sites in U3:
selected class/layer and copied profile; requester/owner/footprint; captured
revision tick; base/learned/through/jam/static/hostile view variants and
bindings; wedge override's captured start and bounds; optional finishing-leg
view; claim owner grid, **all versus slow** row, shared own-row identity,
captured serial, dimensions/footprint/per/against; and the revision callback's
registry/class/profile/requester/tick. These are operands of the existing
closures, not a new rule interface. Current units cannot reconstruct them.
Capture neither invokes the closures nor snapshots mutable claim rows in
place of the shared references. Retail, Straighten and Smooth kernels are
the initial closed set; unsupported laboratory/custom variants return an
explicit error until their own disposition is reviewed.

**Economy, construction, combat and effects.**

| Type/source | Retained logical fields | Exclusions and representation |
|---|---|---|
| `economy.Player`, ledger | Slot-ordered stock/capacity/mirror/AI production/consumption/update/waste/totals/pass counters, kills/losses/commander counters, archived mirrors; all bucket production/requested/accepted/carry and archived production/requested pairs, Metal before Energy. Per-unit buckets by physical handle. Player exists/control/observer/option/full-income/ended/countdown, directed allies, autoshares/thresholds/storage bonus, side/watcher/rejection/result auxiliary. Service reference player, networked, optional selector and effective Community. | Archived accounting is deliberately retained as battle accounting, though some consumers are reports. Names/logos/rank/timers/sensor-call counts are presentation/diagnostic. `aiAggregatesPrepared` is reset before the next settlement consumer. Callbacks bind session/unit/world operands. |
| Construction | Builder links by product; placements by product with rectangle/definition/creation-oriented yard; two repair-bank entries per builder in slot order, target/remainder; repair-world binding; kick records X/Y/Z/valid; effective Community/rules/mode/limit/special-state bindings. | Rotation cache derives from immutable definition/facing; row-registration caches from immutable descriptor registry. Reject active reclaim/VTOL/completion pump contexts. Diagnostic admission/message/command/permanent/kill records and builder debug identity are excluded. Progress/products/queues are unit/order state. |
| Combat pool | Count/capacity/dead flags and **every projectile record through capacity**, including residual records beyond count; all stored record fields: weapon, position/start/target, unit/projectile/shooter/side/muzzle references, velocity/speed/distance/angles, creation/burst/expiry/smoke deadlines, burst flags/count, beam/two-phase/dead, orientation and cached cell/floor/state/marker values. | Reservation clears only part of a reused record. Do not serialize just the live prefix. Pool diagnostic payload and compaction scratch are excluded. Full projectile residuals are a deliberate conservative inclusion. |
| Combat service | Double/half shot, opaque liquid, effective Community/rules; target last-rebuild/gate and primary/secondary lists in stored order, scan cursors; death-notified map with allocation identities; Modern incoming live-span target/shooter/weapon/motion/beam-invalid, modern tick/next-projectile tick; ordered transport captures (allocation/handle/type/health), tick/presence. | Air-base lists belong to movement. Impact stack and transport pending handoff must be empty. Query/candidate scratch and one-visit firing observations are excluded. Model box centres and weapon lookup derive from immutable content. Process-global projectile presentation IDs are excluded. |
| Community area damage | Cells with stamp/head/tail/count; nodes in insertion order with unit/next; width/height/limit/built tick/built/stamp; hit-generation array and counter. | Require current generation zero outside a damage transaction. Counter wrap skips zero without clearing marks and admission uses `>=`; these are not disposable scratch. Rebuild walk and saturation count are excluded. |
| Effect admission | `EffectService.max`, `nextID`, `lastSequence`; bound fixed-pool identity. Event-buffer effective limits, next ID/sequence and exhausted flag. | Zero/exhausted identity can refuse future events. Diagnostics are excluded. Bound production service must have no ownerless pending fallback; unsupported nonempty fallback fails capture. Event-window handling and the current host-kind gap are below. |
| Fixed effects/fragments | Capacity, ordered records, fragment slots/cursors/round-robin, gravity/sea level and effective fragment step inputs; record source/target/kind, XYZ/velocity/gravity/expiry/model-presence/fragment slot/explode-on-hit; both complete animation players (index/countdown/loop/active/frame count/durations). Fragment live/base velocity/angles/angular rates/vertices. | Durations are stored values, including producer overrides. Record graphic/presentation identity/flash/material metadata is excluded after lifecycle operands are retained. In particular `FrozenFragmentMaterial` is host artwork and differs in headless fallback; it never controls admission, physics or RNG. |
| Debris | Storage charge/count/cursor/serial; ordered partition occupied/start/charge/slot/generation; each slot's live flag and live generation/point span/position/angles/velocity/angular rates/lifetime/fall/explode-on-hit; occupied geometry spans. | Dead-slot payload and free point spans are overwritten before reuse. Expired slots can leave charged partitions: retain those partition-generation links. Model/piece/material/smoke/fire drawing metadata is excluded. |
| Strips | Ten lists in strip order, container insertion order, live/capacity/steady limits; family/window/spawn deadlines/interval, source/destination/extents, particle life, phase modulus, frame-delay/count inputs, smoke selector; particles in order with coordinates/velocities/expiry/frame/delay/phase/last-frame and deferred frame-draw state. | Recycled particle capacity is scratch. Colour-only fields are classified below; do not omit a cursor that controls a later lifetime or draw merely because publication also reads it. |

**Computer players.** Manager and executor state is retained even for a
Modern seat; the exception covers only the worker-owned planner material.

| Owner | Retain | Exclude or bind |
|---|---|---|
| `ai.Manager`, `Strategic` | Player/passive/controller/modern-wave-air, deadlines/origin/surface metal/mission gate/factory allocation/countdown/loss deadline; all nine ordered groups; wave engagement/rally initialization, best/probe/drift coordinates/score/targets. Strategic centre/radius, ordered metal spots, land/water region dimensions/offsets, refresh/build-capable/live count/unit limit/max wind and bound/readiness flags. Sorted counts/class/init/single vectors. Effective controller parameters, battle seed, start positions/owners. | Sorted catalog type cache and per-pass unit walks rebuild deterministically. Strategic initialization draw ledgers/intermediates are not future state; retain their resulting regions and readiness. Shared analysis and Modern resume-generator material fall under §9.1/M8. Survival input binds the scenario record. |
| AI profile | Presence, plan, effective weight/limit maps and per-record maps; name, directive stream/text-loaded, all-plan and fixture tables; applied-catalog presence/key and record-ID mapping. | Encode these **values** until profile name/directory admission is complete. Do not call profile application during capture. Record IDs use admitted definition identity; equal names do not collapse. |
| `aikit.Host`, executor | Simulation-thread initialized/next-think/deadline presence/ticks, effective execution persona, batch serial, application chain; APM tokens/last fill; pending reservation ring and next index; four guard-grid slots with origin/factory/sealed/cost/seeds/built/seen/stamp/reach/reachStamp; self-grid seen/stamp; dedupe unit/cell stamps/indexes/generation; three free-cache slots, free sequence/slot/last tick and each cache's stamp/value/generation/tick/class/used/asked/component/region/seen fields. | Cache TTLs deliberately expose old placement pictures. Keep free caches conservatively; their tick-equality reuse is not unconditional overwrite. Grid search queues/distances/heaps, rebuilt blocking/placement walks, reset-valid row scratch, immutable placement geometry and aggregate stats are excluded. |
| Modern exception | Controller presence and pending deadline are mirrored on the simulation thread at existing assignments. | Never inspect brain/private generator/kit/observation/unapplied command arrays or map analysis. Map analysis includes initial observed live-world information, so it is an explicit worker exception, not an immutable-content claim. Flight/ready/probe/worker counters are scheduling or diagnostics. Unknown `Manager.Ext` fails capture. |

Generation-tagged arrays are retained wherever wrap can expose an old tag.
In particular AI dedupe/exit-grid marks and Community hit generations skip
zero without clearing all old marks. A reset that seems harmless for a short
match is not an exclusion proof. U0 specifies existing behavior, not a fix
for any wrap behavior.

#### 16.3.6 Canonical format and public API

**Leaf and files.** U1 implements `internal/sim/checkpoint/{encoder,format,refs}.go`
and focused tests, plus `internal/content/checkpoint_refs.go` and its tests.
The leaf imports only the standard library. Owners can import it without
pointing back to session/content; it performs no rule selection. It does not
reuse `netproto.Writer`, whose buffered varints and online-amount contract are
different. ARCHITECTURE §2–§3 records the leaf and content dependency.
The encoder, references, summaries and content-key APIs are implemented by U1.
Owner writers and session capture APIs below remain planned until U2–U6.

```go
// internal/sim/checkpoint
const SchemaVersion uint16 = 1
const OwnerCount = 13

type Owner uint16
type Digest [32]byte
type Definition struct { Family uint8; Ordinal uint32; Key string }
type Allocation struct { Handle uint32; Serial uint64 }
type ObjectID uint32 // zero is absent; IDs are local to a typed object table

type Encoder struct { /* sticky error, logical field path, streaming sink */ }
func NewEncoder(w io.Writer) *Encoder
func (e *Encoder) Field(path string) // error context only, emits no bytes
func (e *Encoder) Bool(v bool)
func (e *Encoder) U8(v uint8)
func (e *Encoder) U16(v uint16)
func (e *Encoder) U32(v uint32)
func (e *Encoder) U64(v uint64)
func (e *Encoder) I8(v int8)
func (e *Encoder) I16(v int16)
func (e *Encoder) I32(v int32)
func (e *Encoder) I64(v int64)
func (e *Encoder) F32(v float32)
func (e *Encoder) F64(v float64)
func (e *Encoder) Bytes(v []byte) // u32 byte length then bytes
func (e *Encoder) String(v string) // same framing; no normalization
func (e *Encoder) Count(n int) // checked u32, negative/overflow fails
func (e *Encoder) Definition(v Definition)
func (e *Encoder) Allocation(v Allocation)
func (e *Encoder) Fail(err error)
func (e *Encoder) Err() error

type Identity struct { Content, Config Digest }
type Digests struct { Full Digest; Owners [OwnerCount]Digest }
type Capture struct { /* section order, streaming hashes, optional sink */ }
func NewCapture(identity Identity, out io.Writer) (*Capture, error)
func (c *Capture) Section(owner Owner, present bool) (*Encoder, error)
func (c *Capture) Finish() (Digests, error)

// Typed, capture-local interners; never iterate a pointer-keyed map.
type References[T comparable] struct { /* lookup plus encounter-order list */ }
func (r *References[T]) Add(value T) (ObjectID, error)
func (r *References[T]) Find(value T) (ObjectID, bool)
func (r *References[T]) Values() []T // detached list in assigned-ID order
```

The all-zero value of `T` is absent; callers use only the reviewed pointer
types below. `Add` detects ID exhaustion; `Find` never adds. No address is
encoded. `Values` is for simulation-thread composition only, not host access.
Errors carry owner and logical field path; all writes stop after the first
failure, including short writes. Failed capture returns zero digest values
and invalidates any partial diagnostic bytes. NaNs fail; infinities and signed
zero retain their exact IEEE bits. Fixed-width integers are little-endian;
signed integers use two's complement. Floats use bit copies, not arithmetic.
Boolean bytes are exactly 0/1. Definition fields are Family, Ordinal, Key,
with M2 numerical family IDs;
allocation fields are Handle, Serial. Optional records emit presence then
payload. A raw handle is u32, not an allocation reference.

The stream header is the eight ASCII bytes `NLCPSTAT`, schema u16, content
SHA-256, configuration SHA-256, then section count u16. Sections appear once
in this exact order, each prefixed by owner u16 and present u8. Payloads are
self-delimiting from this schema; there is no padded record or trailing
section length requiring a second encoding pass. An absent section has no
payload. Entry/tick distinction and tick number are runtime payload fields.

| ID | Section | Ownership edges |
|---|---|---|
| 1 | runtime | Session configuration, tick/RNG/lifecycle/result/drivers; no scenario internals |
| 2 | units | Arena, current/freed slots, reachable allocation records and physical unit piece flags; no order/VM bodies |
| 3 | orders | Unit-to-queue roots, queue/node tables and auxiliary order state |
| 4 | scripts | Unit-to-VM/bridge roots, VM tables, continuations and unbound VM fallback piece flags |
| 5 | world | Mutable plots, feature table, feature instances/active order |
| 6 | visibility | Visibility service, session stamps and temporary sight |
| 7 | movement | Occupancy/layers/movers/pilots/goals/payloads; nested air-base registry |
| 8 | paths | Provider/scheduler, goal/search tables and suspended accessor descriptors |
| 9 | economy | Player and unit accounts |
| 10 | construction | Placement links, repair/kick state |
| 11 | combat | Projectiles, targeting, damage/death and prediction state |
| 12 | effects | Event admission, effect service/pools, debris and session strips |
| 13 | computers-scenario | Computer managers/executors/application history, mission/schema/Survival |

The full digest is SHA-256 of the complete stream. Each owner digest is
SHA-256 of ASCII `NLCPSECT`, schema u16, content/configuration digests, then
that section's exact owner/presence/payload bytes. Absent owners therefore
still have a defined digest. `Capture.Section` tees one encoding to the full
hash, current owner hash and optional sink. `Finish` rejects missing/repeated/
out-of-order sections and returns no usable result after any error. Only
`Full[:16]` is the future wire digest; histories retain all 32 bytes.

**Content keys.** Content owns the typed resolver, with these exact entry
points (all reject values without an admitted identity):

```go
// internal/content; constructed from the already-frozen battle inputs
func (in *SimulationInputs) CheckpointKeys() (*CheckpointKeys, error)
func (k *CheckpointKeys) Unit(v *UnitDef) (checkpoint.Definition, error)
func (k *CheckpointKeys) Weapon(v *WeaponDef) (checkpoint.Definition, error)
func (k *CheckpointKeys) Feature(v *FeatureDef) (checkpoint.Definition, error)
func (k *CheckpointKeys) Model(v *model.Model) (checkpoint.Definition, error)
func (k *CheckpointKeys) SightShapes(v *SightShapes) (checkpoint.Definition, error)
func (k *CheckpointKeys) LOSTables(v *LOSTables) (checkpoint.Definition, error)
func (k *CheckpointKeys) FeatureSequence(filename, sequence string, delays []int32) (checkpoint.Definition, error)
func (k *CheckpointKeys) FeatureSequenceAbsent(filename, sequence string) error
func (k *CheckpointKeys) ProgramForUnit(unit *UnitDef, v *cob.Program) (checkpoint.Definition, error)
type CheckpointFeature struct {
    Variant uint8 // 1 admitted definition, 2 normalized copy
    Base checkpoint.Definition
    FootprintX, FootprintZ, Damage, Metal, Energy int32 // variant 2 only
}
func (k *CheckpointKeys) NormalizedFeature(base, value *FeatureDef) (CheckpointFeature, error)
```

Nil is encoded by the caller's presence flag, not resolved as an invented
record. Keys use M2 manifest family/record ordinal/key. A normalized feature
copy records variant 2, its admitted base key and the exact
resulting footprint X/Z, damage, metal and energy values. U3 records that
provenance at `NormalizeDef` consumers. `Feature` resolves only admitted base
records; a feature edge writes tag 1 plus that key, or tag 2 plus the
`CheckpointFeature` payload (Base then the five values in the API order).
The normalized resolver verifies exactly the existing five-field transform
and unchanged remaining semantic fields, rather than trusting the caller.
`ProgramForUnit` resolves the unit's admitted COB manifest record and verifies
the program against its semantic digest. Fallback binding recompiles from the
sealed input and can have a new pointer; pointer membership alone would
incorrectly reject it. Validation never rereads a live provider. Other dynamic
definitions fail until a reviewed value variant exists. No name-only match, current filesystem read
or pointer-to-string fallback is allowed.

**Owner APIs and graph discovery.** Each stateful owner exposes
`WriteCheckpoint(e *checkpoint.Encoder, c *CheckpointContext) error` on its
existing state type. `CheckpointContext` is an owner-local type containing
only admitted keys and the typed reference tables it needs; lower packages
never import an upper context. Scalar leaf owners (`pool`, `rng`, `clock`,
wind, event buffer, trigger, animation player) instead use
`WriteCheckpoint(e *checkpoint.Encoder) error`. The exact shared context
constructors are:

```go
// In units, orders, path, movement respectively.
func NewCheckpointContext(keys *content.CheckpointKeys) *CheckpointContext
func NewCheckpointContext(u *units.CheckpointContext) *CheckpointContext
func NewCheckpointContext() *CheckpointContext
func NewCheckpointContext(o *orders.CheckpointContext, p *path.CheckpointContext) *CheckpointContext
```

The contexts expose these typed tables for simulation-thread composition;
none are host APIs:

```go
// units.CheckpointContext
Keys *content.CheckpointKeys
Allocations checkpoint.References[*Unit]
VMs checkpoint.References[*cob.VM]
// orders.CheckpointContext
Units *units.CheckpointContext
Queues checkpoint.References[*Queue]
Nodes checkpoint.References[*Node]
// path.CheckpointContext
Goals checkpoint.References[Goal]
Searches checkpoint.References[Search]
// movement.CheckpointContext
Orders *orders.CheckpointContext
Paths *path.CheckpointContext
Layers checkpoint.References[*ClassLayer]
Payloads checkpoint.References[GoalPayload]
// private typed tables: claim/arrival/composite pilots and claim-row holders
```

Interface tables admit only the closed pointer variants below; reject unknown
or typed-nil concrete values before calling the generic interner. Higher
services' contexts borrow the applicable lower contexts and keys. They add no
second interner for the same kind. COB's value context has resolved program
identity; its VM retains value descriptors beside the installed continuations.
It does not import content. Session owns
the context composition. Every state owner above implements
`CollectCheckpointReferences(c *CheckpointContext) (added int, err error)`
alongside its writer. The receiver is its existing `World`, `Service`,
`System`, `Pump`, `Scheduler`, `Manager` or `Session`, as applicable. A collector both
adds its own roots and scans already-discovered objects in its tables. It
calls the exposed `Add` tables for lower-owner edges. Internal movement roots
and private pilot/row-holder tables are discovered by `System` itself.

Session first adds arena allocations in physical order, then calls owner
collectors in section-ID order. It repeats in the same order until a pass adds
zero. Within an object, outgoing edges use field order and retained sequence
order. IDs are per typed table, starting at 1; reference cycles terminate
through lookup. `added` counts only newly registered objects, never repeated
edges. Overflow or unsupported state fails the collection. The world cannot
mutate during collection/write. Writers use `Find` and fail on an undiscovered
reference; only collectors call `Add`. Table storage is discarded after the
capture, never retained as authoritative state.

Object tables have fixed u16 IDs: allocations 1, queues 2, nodes 3, VMs 4,
class layers 5, pilots 6, claim-row holders 7, payloads 8, goals 9, searches 10,
and ground move-goal handles 11.
Each section writes its non-table record, then its ordered root-link sequences,
then its tables in ascending table ID. A table is ID u16, record count u32,
then records in assigned-ID order; record ID is the implicit 1-based ordinal.
A reference is target table ID u16 plus ObjectID u32; an absent edge is target
table ID plus zero. Root links follow physical-unit order and use allocation
object IDs, including additional retired allocations after arena roots. Any
other root uses the owner's retained sequence/key order. Present sections emit
their assigned tables, including empty ones: units 1; orders 2–3; scripts 4;
movement 5–8 and 11; paths 9–10. An absent section implicitly has empty tables and
emits no table bytes. Unknown table tags fail capture.

Union tags are u8 and precede the variant payload. Unit-slot tags are never
allocated 0, live 1, freed residual 2. Goals: point 1, annulus 2, rectangle 3.
Searches: retail session 1, straightener 2, smoother 3. Pilots: no-pilot 1,
claims 2, arrival 3, ordered composite 4 (nil is reference zero). Payloads:
air marker 1, air velocity 2. COB gameplay continuation: none 0, slot-aim 1;
slot-aim carries thread-slot u8, thread allocation identity u64, captured raw
unit key u32, target allocation, weapon slot u8 and existing aim-mode value.
Descriptor metadata is installed/cleared atomically with `onReturn`, including
thread claim/kill/return and nonnil program replacement. Nil-program replacement
retains the existing receiver semantics and makes capture unsupported. Trace-only wrappers carry
no gameplay descriptor. Known binding identities are absent 0 or the canonical
composed owner 1; any callback with different behavior needs a reviewed variant.

**Path accessor boundary.** Movement keeps construction-time value metadata
beside each suspended closure, then supplies this path-owned value type:

```go
// internal/path
// Nodes are topologically ordered; input index zero means absent.
type CheckpointAccessor struct {
    Kind uint8
    Inputs []uint32
    Layer, Learned, ClaimCounts, ClaimOwn checkpoint.ObjectID
    Requester uint32
    Owner uint8
    Profile [8]int32
    Tick, Serial uint32
    Width, Height, FootprintX, FootprintZ int32
    Start Cell
    Bounds Rect
    Through uint8
    Wall uint8
    Row uint8
    Per, Against int32
}
type CheckpointAccessors struct {
    Nodes []CheckpointAccessor
    Passable, Leg, Cost, Revise uint32
}
func (c *CheckpointContext) SetAccessors(search Search, values CheckpointAccessors) error
```

Kinds are class view 1, learned overlay 2, through-movers 3, static view 4,
hostile/keeps-ground 5, wedge override 6, claims cost 7, class revision 8.
Each writes every field in lexical field order, with irrelevant fields zero
and validated. Inputs use 1-based indexes into the preceding node records.
Profile order is footprint X/Z, max/min water depth, max/bad slope, max/bad
water slope. The kind-specific operands are:

| Kind | Inputs and meaningful operands |
|---|---|
| 1 class view | No inputs; Layer, Requester, Owner, Profile, Tick, footprint |
| 2 learned overlay | One base-view input; Learned (owner+1, zero absent), footprint |
| 3 through movers | One base-view input; Owner, footprint, Through |
| 4 static view | One base-view input; Layer, footprint |
| 5 hostile/keeps-ground | One base-view input; Owner; Wall 1 hostile-only, 2 keeps-ground |
| 6 wedge override | One base-view input and one override-view input; Start, Bounds, footprint |
| 7 claims cost | No inputs; ClaimCounts, ClaimOwn, Owner, Serial, Width/Height, footprint, Per/Against; Row 2 all, 3 slow |
| 8 class revision | No inputs; Layer, Profile, Requester, Tick |

Descriptor object operands are plain u32 values: their target tables are
fixed by this schema (Layer table 5, ClaimCounts/ClaimOwn table 7), so they do
not add the table tag used by ordinary graph edges. Learned is owner+1.
Kind 6 Bounds holds the captured strict overlap boundary operands, computed
with the existing int32 start±clamped-footprint arithmetic, rather than the
search bounds or a new inclusive rectangle. Its footprint retains the raw
profile footprint passed to the override view.

Layer refers to movement's layer table. ClaimCounts and ClaimOwn refer to
separate private comparable row-holder objects in table 7: holder tag 1 is an
own row (`[]uint32`), tag 2 is a count row (`[][8]uint8`). Pilot grids/own
state and suspended accessors reference the **same** holders. U3 attaches
holders when rows/closures are created and preserves the actual backing rows,
including older rows after replacement. Holders do not copy counts or intern
uncomparable slices. This retains independent aliasing of count and own rows.
The row selector records whether the captured count row was all or slow.
Only nodes reachable from the final callback roots are emitted: replacing a
passability or cost callback does not leave its superseded descriptor in the
DAG, while a wedge retains the immediately preceding view as its base.
Revision metadata privately retains its actual registry/class key so capture
can validate the canonical registry and selected layer without calling `For`
or `Revise`.
`SetAccessors` validates local node indexes/kinds and rejects conflicting
repeated registrations; movement/session validates cross-table IDs before
writing. Neither validation invokes a closure. Path writers read only these
values. Fixture callbacks without a reviewed descriptor are unsupported,
not an all-zero passability view.

Unit-owned `any` orders are checked by the orders collector and written as
unit-to-queue roots there; units need not import orders. Similarly, the
scripts section writes unit-to-VM roots and the VM table once. A reference to
a retired allocation keeps its own allocation identity and reachable logical
record; it is not required to resolve to a live arena slot. Retain detached
nodes reachable from danger, repair or movement even when absent from queues.
This typed discovery is diagnostic plumbing, not a new gameplay registry or
reflection walker. Unknown concrete values fail in the owning type switch.

**Session access.** U6 exposes the following on a composed session; methods
are called between pumps on its owning thread, not concurrently with a tick.

```go
// internal/session
const CheckpointOwnerCount = checkpoint.OwnerCount

type CheckpointBoundary uint8 // 1 entry, 2 interior tick, 3 final pump tick
type CheckpointPosition struct {
    Tick uint32
    Boundary CheckpointBoundary
    Pump uint64
    ConsumedInput uint64
}
type OwnerSummary struct { Words, Sum uint64 }
type CheckpointRingRow struct {
    Position CheckpointPosition
    SimulationState, CRTState uint32
    SimulationDraws, CRTDraws uint64
    UnitCount, ProjectileCount, EffectCount, FragmentCount, DebrisCount, StripCount uint32
    Owners [CheckpointOwnerCount]OwnerSummary
}
type CheckpointRecord struct {
    Position CheckpointPosition
    Digests checkpoint.Digests
}
type CheckpointHistory struct {
    Records []CheckpointRecord // at most 64; detached, oldest first
    Ticks []CheckpointRingRow  // at most 600; detached, oldest first
}
type CheckpointCaptureResult struct {
    Pending bool
    Record CheckpointRecord
    Err error
}
func (s *Session) EnableCheckpoints() error
func (s *Session) DisableCheckpoints()
func (s *Session) RequestCheckpointCapture(out io.Writer) error
func (s *Session) CheckpointCaptureResult() CheckpointCaptureResult
func (s *Session) CheckpointHistory() CheckpointHistory
```

Enable performs/records an entry capture only at completed battle entry;
otherwise it fails (no guessed partial history). It retains the constructor's
admitted identities, checks supported bindings, and enables the fixed cadence.
Disabled sessions execute no traversal/hash/history work. Disable clears
retained history and cancels a pending request with an explicit error.
Only one byte request may be pending; a nil sink, disabled session or second
request fails. A request runs at the next C1 boundary (not a zero-tick pump),
even off cadence, and replaces the previous capture result. The sink is used
synchronously and must not reenter the session. It receives bytes only, no
world reference; blocking I/O belongs to a host-owned memory spool followed
by an out-of-tick file write. Failure is reported through the result/history
status, never a simulation log or a world mutation. Regular cadence failures
are retained as the latest capture error even without a pending byte request;
failed records are not added. Off-cadence requests do not shift cadence or
consume one of the 64 cadence slots. Successful automatic captures update only
the cadence history; they do not replace the entry/request result, including
a later cadence tick in the same pump that delivered requested bytes. A host
reads cadence records from history. Repeated history/result reads do not drain.
Pump/consumed-input position and build/platform provenance are metadata,
excluded from the canonical stream; boundary kind and world tick are included.

Initial whole-session admission uses `NewAdmittedSkirmish`, with one human and
Classic/Modern computers or Survival. U6 must retain its frozen inputs and
resolved configuration; it must not synthesize a zero identity for the older
constructors. Ordinary campaign/retail-load constructors currently lack that
binding: their owner records are covered above, but enabling capture on those
sessions returns an explicit unsupported-admission error until a constructor
supplies equivalent immutable identity. This does not block single-seat M3
acceptance or authorize multiplayer campaign/save support.

#### 16.3.7 AI application records and the tick ring

**Application history.** Both controller kinds retain one per-player chain
and application count on the simulation thread. U5 adds the following
value boundary in `internal/ai`, implemented by the Modern host through the
existing manager extension. Session never imports a controller implementation.

```go
// internal/ai
type ControllerCheckpoint struct {
    Present, Initialized bool
    NextThinkPresent bool
    NextThinkTick uint32
    DeadlinePresent bool
    DeadlineTick uint32
    NextBatchSerial, ApplicationCount uint64
    ApplicationHash checkpoint.Digest
    Tokens int64
    LastFill uint32
}
type ControllerCheckpointProvider interface {
    ControllerCheckpoint() ControllerCheckpoint
    WriteControllerCheckpoint(*checkpoint.Encoder, *CheckpointContext) error
    AppendControllerCheckpointSummary(*checkpoint.Summary) error
}
```

The provider reads only simulation-thread fields and the executor values
listed above. Assign batch serials when the simulation thread marks a batch
pending/due, not when a worker finishes. Classic synchronous decisions use a
simulation-thread decision serial. Zero identifies no batch; incrementing a
serial/count at exhaustion fails checkpoint reporting rather than wrapping
an identity. Existing commands still execute normally if diagnostics fail.
Enabling checkpoints at entry initializes the chain as SHA-256 of ASCII
`NLCPAIST`, schema u16, content/configuration hashes, player u8 and controller
kind u8 (Classic 1, Modern 2). After each attempt, replace it with SHA-256 of
ASCII `NLCPAIAP`, schema u16, previous hash and the canonical attempt below.
Counts and serials use u64; this is Nanolathe diagnostic framing, not retail.

An attempt record contains player/controller, tick, batch/decision serial,
command ordinal, typed intent and operands, ordered observed actors as
allocation references, optional observed target, optional product definition,
APM verdict, ordered operations and terminal outcome. Actor order is emission
order, not sorted. Command kinds and resolved order IDs use the existing
vocabularies and retain their current numerical values. All ordinary scalar
encoding uses §16.3.6. The APM tag is unlimited 1, debited 2, rejected 3;
Classic uses unlimited. Each operation has a u16 tag and these operands:

| Tag | Operation | Operand payload |
|---|---|---|
| 1 | Queue purge | Actor allocation; actual purge invocation, including an empty queue |
| 2 | Drop leading automatic | Actor allocation; actual invocation |
| 3 | Insert order | Actor allocation; actual inserted node's retained value fields, with target raw-handle semantics |
| 4 | Coalesce stockpile | Actor allocation, resolved row, actual capped count and whether an existing node was changed |
| 5 | Typed build attempt | Actor allocation, product key, resolved site/facing/count, stable admission verdict |
| 6 | Activation | Actor allocation, requested Boolean, including an already-equal value |
| 7 | Placement reservation | Actual pending-ring slot and complete new reservation value |
| 8 | Guard-grid mutation | Cache slot, affected cell/index and new stored value, in mutation order |

Stable typed-build verdicts are success 1, rejected product 2, rejected site 3,
rejected owner/actor 4, rejected limit 5, unavailable binding 6, other failure
7. These classify existing return paths; they must not change admission or
expose error strings. The terminal tag is accepted no-op 1, success 2,
stale/rejected with no committed operation 3, partial application 4. A rejected
APM attempt has no world operation, but remains in the chain; APM debit happens
before later stale validation. New executor mutation kinds require a schema
update rather than reusing an unrelated tag. Graph IDs are capture-local and
never enter the long-lived chain: inserted node values use their command
operands, allocation identities and content keys.

Instrument existing commit sites: `executor.apply/exec` for attempt/APM and
per-actor disposition; `execBuild` for reservation/grid mutations **before**
a possible failed typed build; `execProduce` for normalized count; `execReplace`
for purge/reclaim preceding a failed replacement; `execStockpile` for actual
coalescing; `execUnblock` for accepted no-op and chosen blockers;
`execClear/reclaimFeature/reclaimUnit` for ordered partial work. Classic uses
`constructionPlacePass`, `doResourceGroup`, `queueExactResult`,
`issueMobileBuild`, `submitResolvedOrder`: retain purge/insertion even when
resolution gives row zero, and actual activation calls. Do not introduce a
new command dispatcher or include ordinary strategic/group upkeep in this
chain. Those owners are already encoded in full. Unknown error text is never
a protocol value; a new unclassified state makes checkpoint reporting fail.

**Cheap ring algorithm.** Each owner separately starts `Words=0, Sum=0`.
For each selected scalar word in the order below, increment Words and add
`Words * word` to Sum modulo 2^64. Signed scalars are sign-extended to 64 bits;
unsigned/bools are zero-extended; binary32/binary64 contribute their exact
bits. An optional record contributes its presence first; a variable list
contributes its length first. Iterate arrays/slots/lists in their existing
order. No strings, pointer values, map iteration, canonical encoder or SHA
work is used. A NaN selected by a summary reports the same capture
failure as C2. Metadata and the six pool counts have their own row fields.

| Owner | Ring words, in this order (coordinates expand X, Y, Z) |
|---|---|
| runtime | Tick; lifecycle state; RNG initialization and both stream state/draw pairs; end latch fields; pending/ended/draw result bits; commander-death slots; wind strength/heading/scalar/next-change; meteor active/next-strike/end/next-hit; shake active/remaining |
| units | Physical slot tag, raw handle, allocation serial, owner, health/remaining/flags, XYZ, heading/speed, pending/stun/paralysis; each weapon slot reload/ammo/readiness; live/created counters in player order |
| orders | Per physical live unit: queue presence, primary then secondary lengths and node ID/phase/target/goal/deadline/parameters/flags in traversal order; last pump tick |
| scripts | Per physical live unit: VM presence; each thread status/PC/SP/sleep/waits/signal mask and all 32 stack words; statics; active count, next identity and thread identities |
| world | Feature count, reproduction cursor, arena held; row-major plots' occupancy words/metal/feature index/anchor-damage/gameplay flags |
| visibility | Mode semantic bits/local/team/viewer-defeated; word mask then player byte grids row-major; temporary-sight list length and owner/coordinates/expiry/published |
| movement | Per-handle route count/active/dirty/status/request tick and all point coordinates; steer X/Z/heading/speed/dirty; collision stamp/plane/blocker/blocked; flight mode/XYZ/velocity; work tick/budget |
| paths | Scheduler base/set/scales/call count/player cursor/allowance, service counts/accumulators; active request presence/player/unit/start/activation; per-handle working-search presence, popped/setup/expanded counts, node count and heap count |
| economy | Ten players in slot order: Stock, Capacity, Mirror (each Production, Requested, Accepted, Carry), AIProduction, AIConsumption, UpdateTime, GameEnded, EndGameCountdown, Allies; resource pairs Metal then Energy. Then unit-bucket count and each physical row's Buckets, Metal then Energy, each Production, Requested, Accepted, Carry |
| construction | Per-handle repair-bank target/remainders and kick XYZ/valid; placement count and builder-link count |
| combat | Pool count/capacity; all projectile records' dead/weapon/XYZ/target/shooter/velocity/expiry/burst remaining; target rebuild gates/cursors, Modern next-projectile tick, area generation counter |
| effects | Event next ID/sequence/exhausted; service next ID/last sequence; ordered effect XYZ/velocity/expiry and both animation indices/countdowns; fragment/debris/strip counts; each strip's family/spawn/window and each particle XYZ/expiry/frame/delay/phase |
| computers-scenario | Slot manager presence/controller/deadlines/countdown; group lengths; application count and four little-endian u64 words of its chain; next-think/deadline presence/ticks, APM tokens/refill; trigger completed/celebrated; Survival phase/end/wave/spawn cursors/next-retarget/score and accounts |

These are direct read-only summary methods, with owner signature
`AppendCheckpointSummary(s *checkpoint.Summary) error`, called on existing
owner state; session walks live unit queues/VMs directly for their owners.
They must not collect the full reference graph each tick. Nested owners
append into the section's same accumulator, so word numbering continues
across owner fragments. U1 provides the leaf `Summary` type with methods
`Word(uint64)` and `Result() (words uint64, sum uint64)`; its zero value is
ready to use. No byte writer is involved.

Blind spots are deliberate and tested: unlisted fields, equal-length string
changes, detached order/VM objects not rooted in live slots, deep path frontier
changes without count changes, geometric/effect residuals omitted above and
weighted-sum collisions can escape a row. Thus a matching ring does not prove
equality or locate every possible fault. Full owner digests cover the retained
schema; diagnosis reports unresolved intervals/owners instead of inventing a
first divergence. The required seeded-fault test changes a selected field.
Row storage is fixed-size: at most 600 rows, 64 records and one capture result;
no geometry, nodes, strings or worker state is retained in history. U7 measures
actual struct sizes plus slice overhead and collection cost. Reference discovery
and full hashes run only on full-capture ticks or explicit byte requests.

#### 16.3.8 U0 decisions and remaining implementation gates

U0 is complete as an implementation inventory and schema/API decision.
U1 implements the encoder and admitted content references. Owner writers,
runtime reference metadata, application instrumentation and session captures
remain unimplemented; U2–U5 may start after U1 verification. This is not M3
acceptance evidence.

- **Quiescence:** entry capture follows opening publication; runtime capture
  follows successful tick publication and C1's tail. Event staging must be
  empty. `publishFrame` can return before reset if no writable frame exists;
  such a boundary fails capture. A tail/observer that adds events also fails.
  Never invent a pending next-tick queue: the current effect phase consumes
  its event window before later phases, and publication resets the window.
- **Event admission host dependency:** `emitPositional` admits audio only with
  an audio service and an audible viewer; those events share effect admission
  limits and IDs/sequences. Preserve the counters. U6/U7 must expose this
  mismatch, then resolve it under the staged host-equivalence work before
  claiming that gate passed. Viewport pan/volume alone does not gate admission.
  Direct strip producers remain independent of event-buffer acceptance.
- **Strip exclusions:** table nano-colour cursors, object owner colour/known/
  infected/cursor/colour selector, particle colour/sample/sequence and the
  write-only reserved word are presentation-only. Keep animation and deferred
  frame-draw state listed above. The existing zero-sentinel tick-wrap question
  in `readyToSpawn` remains open; encoding its words does not settle parity.
- **Admission fixes before enable:** U1 resolves keys from actual frozen input
  records; U6 verifies preparing rule identity, effective cloned catalog limits
  and restriction inputs. Profile values are explicitly encoded while profile
  name/directory attestation remains a pre-online identity gap. An attestation
  that is required but absent fails enable; a matching first digest is not a
  substitute. Ordinary unadmitted constructors remain unsupported as stated.
- **Metadata prerequisites:** U2 owns slot-aim completion descriptors and
  script alias checks; U3 owns captured path accessor descriptors and normalized
  feature provenance; U5 owns simulation-thread deadline/application records.
  These additions observe existing operands and do not choose new behavior.
- **Existing gaps:** feature physical-slot reuse (EC-G2), prior SFX-band retail
  save persistence, desired-aim initialization and visited-bit interpretation
  remain in their owning research/design documents. Preserve actual runtime
  values. The replay-only actor relaxation remains a maintainer/M4 question.
  O10 stays open through owner-writer review and measured U7 acceptance.

U2–U6 compare every added writer against this inventory and account for any
newly encountered field before claiming coverage. A missing disposition fails
that owner's review; it is not an implicit exclusion. Unknown custom callbacks,
searches, pilots, rules or extensions remain explicit unsupported states, not
silently empty canonical records. No gameplay policy, retail research claim,
module dependency, save format or existing fingerprint changes in U0.

#### 16.3.9 U1 implementation and verification

The stdlib-only `internal/sim/checkpoint` leaf streams the exact schema into
SHA-256 and an optional diagnostic sink. Independent authored byte/hash vectors
cover all thirteen owner domains, identity binding and hash-only equivalence.
Primitive tests cover integer widths, IEEE signed zero/subnormal/infinity bits,
NaN rejection, count bounds and sticky short-write errors. Lifecycle checks
reject missing, repeated, reordered, absent-payload and closed-section writes.
Typed references preserve encounter order and pointer aliasing; weighted
summary tests pin modulo-2^64 accumulation. These exercise authored test owners,
not live simulation state.

`SimulationInputs.CheckpointKeys` uses frozen manifest records and admitted
objects, retaining equal-name unit ordinals and rejecting foreign objects.
Fallback COB instances are checked against the owning unit's admitted semantic
digest even when compiled into a fresh pointer. Model admission retains the
already-computed definition-loader heights, so key validation needs no file
reads and does not change the content digest. Feature references validate the
existing malformed-definition gate and precisely its five normalized fields;
U3 retains each runtime copy's base relation as described in §16.3.11.

Focused tests pin these reference boundaries, including lookups against a
filesystem wrapper that cannot perform reads. The declaration-scoped I2 guard
allows only `Encoder.F64`'s bit-copy parameter. The new production-unreachable
entries in `tools/deadcode-baseline.txt` are explicit U1 staging: owners begin
consuming the published APIs in U2–U5, and U6 makes capture reachable from a
battle. Remove those entries as their consumers land. No synthetic production
caller is added to bypass the staging gate. U1 changes no tick, gameplay rule,
existing partial fingerprint, save codec or module dependency; live capture
cost and whole-state/native-platform acceptance remain U7 work.

U1 package, architecture and citation checks pass. The authored encoder
vectors also pass in the Darwin/amd64 build on this Darwin/arm64 host; that
additional build check is not the native multi-platform U7 acceptance gate.

#### 16.3.10 U2 implementation and verification

The unit, allocator, order, COB and model owners now write their assigned
records. These are staged capture APIs: live-session composition and acceptance
remain U6–U7. U2 accepts admitted authored fixtures with absent gameplay
bindings and refuses unverified callbacks, sources and owner adapters. It does
not enable whole-world capture or claim playable networking. U3 progress is
recorded in §16.3.11.

**Allocation and graph framing.** Section 2 writes the World record in lexical
order: `OnCapture`, `OnCreate`, `OnDeath`, `OnDeathExtra`, `attachmentObserver`,
`cobBinder`, `cobFS`, `cobLoader`, `createdCounters`, `extraction`,
`lastAllocationSerial`, `liveCounters`, `pool`, `pose`, `simulationRNG`.
Binding fields use explicit absent u8 tags. Pool fields are `alive`, `defID`,
`limit`, and player-ordered `slices` (each `end`, `start`); used count,
identity-index and sliced-state derivations are validated. The physical-slot
sequence includes the null sentinel, with the three tags in §16.3.6. A live
slot carries a table-1 reference; a freed residual carries `Handle` u32,
`Kills` i32, `Owner` u8, `Remaining` f32, in that order. Table 1 follows and
includes independently retained old allocations after the arena roots.
Creation must be idle, serials must be distinct successful allocations from
this battle, and catalog identity maps must match finalized admitted records.

`units.World.CollectCheckpointReferences` registers all physical allocations
first, then the discovered allocation/VM edges. `orders.Pump` discovers queues,
nodes and retired allocations from danger and firing-position edges. Repeated
owner passes reach a fixed point without reading a pointer as an address or
redirecting an old allocation to a reused slot. Section 3 writes the ordered
allocation-to-queue roots, then queue table 2 and node table 3. No collector
creates a missing queue, invokes a handler or advances a script. Writer-time
reference lookups never register new objects. Detached pump execution and
unconsumed input-only order fields fail capture.

Unit and nested records use the expanded lexical lists next to their writers.
Raw handles use the schema's u32 width; `numeric.Fixed` uses its full i64
storage, including high bits. Unit `RenderPieceFlags` stays in section 2 after
`Remaining`, with byte-sequence framing. Unit `statusCue` and `yardTransaction`
are explicit absent binding tags after the death latches. This keeps physical
unit flags separate from the VM's unbound fallback storage.

**Script roots and callback state.** `units.World.WriteScriptCheckpoint`
composes section 4 from the shared context. Each allocation root writes its
table-1 reference, table-4 VM reference, `ScriptState` presence, optional
`Binding`, then optional `Bridge`. A binding writes its admitted model key
and absent u8 tags for `PresentationSink`, `SFXSink`, `SFXVisible`,
`SimulationRNG`; its program/VM/bridge aliases, piece links and creation marker
are validated derivations. A bridge writes `createInvoked`. VM table 4 follows,
with an admitted program identity supplied through `cob.CheckpointContext`.
Each VM must belong to exactly one allocation; sharing a VM across different
unit owners is explicitly unsupported. A VM without a program is unsupported.
Neither validation nor writing reads a file or invokes a mutable pose cache.

VM fields are lexical as listed beside `VM.WriteCheckpoint`. All eight thread
slots and every stack cell are retained, as are animation lanes, return and
thread allocation identities, and raw aim-ready words. The fixed VM binding
record has twelve tags: `cargoContains`, `carrierIdentity`, `explosionSink`,
`portBindings`, `portFuncs`, `renderFlags`, `scriptTouched`, `sfxSink`,
`sfxVisible`, `simRng`, `transportAttach`, `transportDrop`. U2 accepts only
absent bindings, even if a nonnil function looks like the normal implementation.
VM `pieceFlags` writes source tag 1 plus the local byte sequence; source tag 0
is reserved for the attested external unit store. Presentation-only VMs fail.
Piece cache booleans, drain counters, trace queues and provenance are excluded.

`CallbackBridge.AimWithCheckpoint` records the existing combat receiver's
captured allocation, weapon slot and raw cleanup key. It uses the same deferred
aim path, without running the receiver. Thread slot/identity metadata is armed
and cleared with the receiver at claim, kill, return and nonnil program replacement;
on return it clears before the callback executes. Ordinary unknown gameplay
receivers fail capture, while a trace-only wrapper has the same descriptor as
no receiver. `VM.CheckpointContinuations` returns these detached values.
Script composition verifies that each slot-aim target is its VM's own unit
allocation. This describes the pending work; it does not grant readiness or
alter RNG draws, scheduling or callback results.

**Remaining binding work.** The `TODO(M3-U6)` refusals are implementation
staging, not unresolved retail mechanics. U6 must attest the actual production
composition before it can emit canonical binding tag 1: World hooks/COB source,
unit yard/status hooks, queue adapters/handlers, script binding and VM ports,
readers, RNG, render store and sinks. No nonnil-only acceptance or function
address identity is permitted. New production-unreachable context and owner entry points have explicit
baseline entries until U6 wires capture to sessions; tests exercise them now. No synthetic caller bypasses that gate.

Authored tests pin independent byte vectors, full-width values, slot reuse and
residuals, detached/retired references, aliases, suspended orders, inactive COB
stack storage, thread reuse and completion cleanup. Mutation tests distinguish
owner sub-digests; exclusion and purity tests vary addresses, caches and traces
without changing the output. The combat test checks the original captured
receiver even when another allocation uses its raw handle.

The displayless simulation-cost comparison used scene 1, Town & Country,
Modern rules, seed 7 for both streams, three 250-unit armies, 1,200 warm-up and
300 measured ticks, Go 1.27.1 on Darwin/arm64 with two runtime workers. Catalog,
scene, all seven census rows, RNG draws (15,645 simulation; 985,036 CRT), and
initial/warm/final partial fingerprints matched the unchanged-main baseline.
Baseline/candidate median tick time was 2.020/1.685 ms, p95 3.217/2.718 ms,
and process CPU 2.145/1.824 ms per tick; allocated objects were about 808/tick
in both. These single-run timings show no observed regression, not a speedup
claim. This measures the added callback metadata with capture disabled;
whole-checkpoint cost remains U7.

The integrated worktree passes `tools/check` and `tools/check-retail`,
including the amd64 fingerprint locks under Rosetta and real-device GPU
fixtures. Read-only reviews independently ran the affected owner/callback
contracts. Main remains at `c6f73cbb6`; U2 is committed only on the continuing
multiplayer branch pending play testing.

#### 16.3.11 U3 terrain, feature and visibility owner group

U3 is split into bounded owner groups. Terrain/features and visibility are
implemented on the continuing multiplayer worktree; path search/accessor
metadata and movement follow. The first group uses the published API below.
Whole-session composition, map admission and canonical callback attestation
remain U6, as with U2.

```go
// internal/world
func NewCheckpointContext(keys *content.CheckpointKeys) *CheckpointContext
// CheckpointContext exposes Keys *content.CheckpointKeys and Terrain *Terrain.
func (t *Terrain) CollectCheckpointReferences(c *CheckpointContext) (int, error)
func (t *Terrain) WriteCheckpoint(e *checkpoint.Encoder, c *CheckpointContext) error
func (t *Terrain) RecordCheckpointFeatureNormalization(base, value *content.FeatureDef)
func (t *Terrain) CheckpointFeature(keys *content.CheckpointKeys, value, base *content.FeatureDef) (content.CheckpointFeature, error)
// internal/features
func NewCheckpointContext(w *world.CheckpointContext) *CheckpointContext
// CheckpointContext exposes World *world.CheckpointContext.
func (s *Service) CollectCheckpointReferences(c *CheckpointContext) (int, error)
func (s *Service) WriteCheckpoint(e *checkpoint.Encoder, c *CheckpointContext) error
// internal/visibility
func NewCheckpointContext(keys *content.CheckpointKeys) *CheckpointContext
// CheckpointContext exposes Keys *content.CheckpointKeys.
func (s *Service) CollectCheckpointReferences(c *CheckpointContext) (int, error)
func (s *Service) WriteCheckpoint(e *checkpoint.Encoder, c *CheckpointContext) error
```

These owners add no object-reference tables. U6 frames section 5 as terrain
presence/payload then feature-service presence/payload. Section 6 frames the
visibility-service presence/payload then the session's retained visibility
stamps and temporary-sight records. An owner writer emits only its payload;
it never emits another owner's body or adds section framing on its own.

Terrain retains `ClassRestamp`, `FeatureDefs`, `FeatureNames`, `Movers`, `Plot`,
`metalSeeded` in lexical order. Binding fields use absent u8 tags until U6
attests the canonical movement owners. Plot rows write `AnchorWord`, `Feature`,
`Flags`, `Metal`, `OccupantA`, `OccupantB` (u16, u16, u8, u8, i16, i16).
`Flags` removes only the unexplored bit and placer nibble; unclassified bits
remain. Heights and geometry are admitted immutable map inputs. Capture
requires the entry void sweep complete and no active mission undo/stamp/replay
transaction, validates plot dimensions, and never runs fixup or recomputes
immutable LOS data. Feature names and definitions are separate stored sequences; runtime admission can append only definitions.
A definition edge is presence followed by the §16.3.6 feature variant.

At the existing normalization site, features preserves the original base on
the created instance; when the normalized object is appended to the terrain's
retained definition table it also records that exact base relation there.
The terrain collector binds the context's singleton terrain; features verifies
its service and instance aliases against that owner. No name lookup infers
provenance and no diagnostic map keeps an unretained
failed placement alive. `CheckpointFeature` resolves an optional explicit
instance base, or the terrain's retained relation, then delegates validation
to the admitted content keys. A stale/foreign or changed copy fails capture.
Re-normalizing an already normalized empty-key definition also refuses while
its base is itself unadmitted; an explicit original-base provenance contract
and its composition validation must precede support for that edge.
This metadata neither changes normalization nor selects a replacement.

Features writes `BurnFrameGeometry`, `BurnSmoke`, `BurnSound`, `BurnWeapon`,
`Crt`, `GeothermalSteam`, `SequenceFrames`, `Sim`, `Terrain`, `Wind`,
`arenaHeld`, `cursor` in that order, followed by instances in sorted anchor-key
order and the exact active head-to-tail key sequence. Links, map ownership, arena charge and captured terrain aliases are
validated without repairing caches or links. Advertised-valid lookup keys and
value rows must agree with the sorted map; caches marked for rebuilding are
excluded without refresh. Active-walk and pending-burn handoffs fail capture. Instance fields follow §16.3.5; its cursor writes delay,
sequence presence/key and frame in lexical order. A live cursor's delay words
must equal the frozen sequence named by its actual definition and selector.
Sequence lookup neither invokes `SequenceFrames` nor populates its cache.
Excluded cache rows validate their sequence key and exact delay values, not
definition provenance; stale normalized key objects can outlive their instances
without affecting that immutable timing identity. `FeatureSequenceAbsent`
validates cached misses against admitted absence/zero-frame metadata or the
consumer's explicit blank-argument result, refusing unrequested names.
Unknown callbacks and nonnil RNG/wind bindings remain explicit U6 refusals;
observation/provenance and shadow-only bindings follow the reviewed exclusions.

Visibility writes Community values/binding tags, dimensions, rule binding,
byte grids, sorted observer footprints, local player, semantic mode bits,
ray-table and shape presence/keys, teams, terrain presence and viewer-defeated,
then word mask in source-field lexical order. The fog-valid mode bit alone is
excluded. Sprite masks and ray tables resolve as the actual admitted catalog
objects, including the declared ray-table count. Unknown rules, alliance and
off-map readers fail until U6 attestation; capture invokes none of them.
Terrain presence is the single world edge; U6 verifies it refers to the
same admitted terrain. Derived fog, spokes, sensor indexes and
diagnostic/publication counters stay excluded, without being rebuilt by the
writer. The new provenance and in-progress markers are capture metadata and
boundary checks, excluded from the payload. They add no RNG draws or rules.

Authored byte vectors and retained-field mutations cover plot words, normalized
definitions, feature positions above 32-bit range, active-list order, sequence
identity/timing, player grids, inactive observer residuals and the complete mode
word except its fog-valid bit. Tests vary cache contents, map insertion order,
object addresses and excluded presentation fields; collection and writing
neither mutate state nor invoke producers. Invalid lists, arena charges, usable
lookup caches, foreign definitions and unattested callbacks refuse before
payload emission. Content tests freeze authored missing and zero-frame GAF
sequences, and reject empty nonnil delay arrays as present bindings.

The three new context constructors in `tools/deadcode-baseline.txt` are U3
staging until U6 composes these owners. The visibility key-collection loop and
its containing numeric sort are pinned by the I1 architecture audit. No new
module dependency, rule seam, save representation or partial fingerprint is
introduced.

The displayless comparison used scene 1, Town & Country, Modern rules, seed 7
for both streams, three 250-unit armies, 1,200 warm-up and 300 measured ticks,
Go 1.27.1 on Darwin/arm64 with two runtime workers. Baseline `9b438fa2c` and
the integrated candidate matched the catalog, scene, all seven census rows,
RNG draws (15,645 simulation; 985,036 CRT), and initial/warm/final partial
fingerprints. The final census held 6,213 features, 12 burning. Baseline/candidate
median tick time was 2.029/1.888 ms, p95 3.238/3.003 ms, and process CPU
2.152/2.030 ms per tick, with about 808 allocated objects per tick in both.
These single runs show no observed regression and do not establish a speedup.
They measure runtime provenance/transaction metadata with capture disabled;
whole-checkpoint cost remains U7.

The integrated worktree passes `tools/check` and `tools/check-retail`, including
retail static analysis, the amd64 fingerprint locks under Rosetta and real-device
GPU fixtures. Independent reviews reran the affected content, terrain, feature
and visibility checkpoint contracts. Main remains at `c6f73cbb6`; this owner
group is committed only on `multiplayer-m3-u2`, pending play testing.

#### 16.3.12 U3 path-owner framing

The path owner uses the context, descriptor DAG and tables published in
§16.3.6. Its scheduler record writes retained source fields lexically; the
optional active request includes `activePlayer` and `activeScale`, whose
inactive residuals are excluded. `activeReq` storage is not a second request.
The path writer follows the scheduler payload with goal table 9 and search
table 10. U6 writes movement's candidate-provider payload before this fragment;
the provider's state is not an opaque scheduler callback. Unsupported scheduler
search/publish/provider bindings initially refuse pending composition attestation.

Each search variant writes its descriptor `Nodes` immediately before its
`cfg` record. The callback fields `CostDir`, `LegValue`, `PassableValue`,
`Revise` become their corresponding descriptor-root u32 indexes in lexical
field order. Each wrapper retains its own configuration, which can differ
from its child's normalized configuration. Descriptor registration neither
calls nor identifies closures by address. Node-store presence is explicit;
its `nodes` sequence includes the reserved index-zero row, followed by `scale`.
Node-store index aliases and heap position/node indexes are checked derivations,
not separate copies. Dense and sparse workspace representations emit the same
logical entry sequence sorted by `(Z, X)`, including visible entries without
allocated nodes. Omit an all-zero logical entry, since an absent sparse entry
and a stale dense slot both read that value; retain every nonzero direction,
node or status even when status is zero. `SetAccessors` propagates a registration
through the closed straighten/smooth child chain because those constructors
retain the same closure captures. It validates each variant's own callback
presence and rejects conflicting registrations, without invoking `Config` or
any callback. The no-terrain fixture's constant-blocked callback has no reviewed
descriptor and remains unsupported. Unknown concrete goals/searches and
typed-nil variants refuse. `CheckpointKernelKind(Kernel) (uint8, error)` reads
only the closed concrete kind: effective Retail (including nil) is 1,
Straighten 2, Smooth 3; their value and nonnil pointer forms are equivalent.
Typed-nil or custom variants refuse without invoking `NewSession`.

#### 16.3.13 U4 economy-owner API

The complete resolved `community.Features` value has a shared leaf writer:

```go
// internal/community: all fields lexically, nested RepairRate lexically;
// Boolean bytes and signed 64-bit Go ints, with no profile application.
func (f Features) WriteCheckpoint(e *checkpoint.Encoder) error
// internal/economy
func NewCheckpointContext(w *world.CheckpointContext) *CheckpointContext
// CheckpointContext exposes World *world.CheckpointContext.
func (s *Service) CollectCheckpointReferences(c *CheckpointContext) (int, error)
func (s *Service) WriteCheckpoint(e *checkpoint.Encoder, c *CheckpointContext) error
```

Economy adds no graph objects. Service fields and player/bucket records follow
§16.3.5's dispositions and §16.3.6's lexical/source-width schema. Fixed arrays
have no count, variable unit-bucket storage has its exact count including
unused rows, and resource pairs are Metal then Energy. Preserve every floating
bit pattern and refuse NaNs through the encoder. The selector is presence plus
i64, not a default difficulty. Terrain is singleton presence with the world
context's exact terrain alias; canonical callback/wind attestation follows in
U6, and until then nonnil callbacks/wind refuse without invocation. Names,
logos, rank/display timers, sensor diagnostics and the reset-before-use
`aiAggregatesPrepared` flag retain their reviewed exclusions.

The direct economy summary uses the exact order in §16.3.7 and no count for
the fixed player array. Pass production/consumption, archived accounting,
totals and configuration are deliberate summary blind spots retained by the
full writer. The summary does not inspect or invoke callback/wind bindings;
it reads only the selected scalars and commits a stack-local accumulator copy
after their NaN checks succeed, so failure cannot partially append an owner.

#### 16.3.14 U3 movement composition and metadata

Movement keeps the construction-time accessor operands beside each working
set, and comparable claim-row holders beside the arrays when allocated.
Replacing a working set drops its descriptors; resuming one retains them.
The pilot search call borrows a temporary descriptor-root record alongside
its existing temporary SearchConfig, so ordered composite pilots retain the
last actual cost callback's operands. No capture reconstructs operands from
the current unit, calls a rule, or evaluates a callback.

The movement context and tables are those of §16.3.6, with private typed
table 11 for `*moveGoal`: record cleanup compares the per-handle move goal's
identity with `recordGoals.ground`, so identical values cannot replace their
alias relation. Root fields reference table 11, whose records retain their
ordinary lexical fields and edges; detached retained handles remain roots.
Movement writes table 11 after its tables 5–8. Its section-7 record
writes Scheduler and pathProvider presence, while section 8 starts with
`System.WritePathProviderCheckpoint(e, c) error`, then the path scheduler and
its tables. Provider requests follow sorted actual player/handle keys; a walk
of today's unit slices can omit retained requests and is not the serializer.
Provider system/world/eligibility and scheduler ports require U6 composition
attestation. Eligibility cache must be invalid at capture. Class-table identity
is an immutable U6 admission binding; copied profiles alone do not describe
future class lookups. `unreachableLive` and `pocketLive` retain their source
i64 values. The closed kernel-kind helper identifies the bound effective
kernel without opening a search.

The cheap movement section writes separate physical Routes, Steers, Collisions
and Flights lengths, with each slot presence followed by its selected values.
Routes retain all twenty point coordinates. Collision stamp expands HasStamp,
StampedAnchor.X/Z and LastStampTick, followed by StampedPlane, BlockerID and
Blocked. The section ends with workTick and workSmooth.

The cheap path section appends scheduler words first, then the physical working
set length and each row in handle order, with search presence. A missing working
set or missing search is absent. A present search adds
charged popped count (retail popped plus wrapper probes using the existing
Go-int arithmetic), retail setupSteps, expanded, allocated node count excluding
reserved zero (zero for absent/empty storage), and heap-entry count.
`path.AppendSearchCheckpointSummary(Search, *checkpoint.Summary) error` reads
the closed search chain directly; typed-nil, unknown and cyclic wrappers fail
without partially appending. It invokes no Search method. Scheduler summaries
read only selected scalars and do not inspect their unselected bindings.

#### 16.3.15 U4 construction-owner API

```go
// internal/construction
func NewCheckpointContext(o *orders.CheckpointContext, w *world.CheckpointContext) *CheckpointContext
// Exposes Orders *orders.CheckpointContext, World *world.CheckpointContext.
func (s *Service) CollectCheckpointReferences(c *CheckpointContext) (int, error)
func (s *Service) WriteCheckpoint(e *checkpoint.Encoder, c *CheckpointContext) error
func (s *Service) AppendCheckpointSummary(s *checkpoint.Summary) error
```

Construction adds no object tables. Builder links and placements retain sorted
numeric product keys. Placement records write definition presence/key, rectangle
and the exact creation-oriented yard sequence; do not regenerate a yard from
today's rules. Repair banks include the full variable per-handle array and both
fixed slots; their target handles retain weak-slot semantics. Kick records retain
the complete variable array, including invalid rows' stored XYZ. Lexical field
order and scalar widths follow §16.3.6. The shared resolved Community writer
covers its bound feature answers. Terrain validates the world-context singleton;
`world.FootprintRect.WriteCheckpoint(*checkpoint.Encoder) error` retains
anchor(cellX,cellZ), extent(depth,initialized,width), maxX, maxZ without
reconstructing endpoints. The initialization flag distinguishes missing from
authored empty extents. OnRefresh, StatusText and RepairBankFallbacks are
excluded presentation/diagnostics. Other owner/content/rule/function bindings require U6 attestation and initially
refuse when nonnil, with explicit absent tags. Presentation hooks that cause
strip/RNG work remain bindings; diagnostic-only callbacks stay excluded.
Active reclaim/VTOL/completion contexts fail capture.

The direct summary writes repair-bank row count then each pair in slot order
(target, remainder), kick-record count then each record (X, Y, Z, valid), then
placement and builder-link counts. It inspects no callbacks, keys, map values
or reference graph; all other retained values are full-digest-only blind spots.

#### 16.3.16 U4 combat-owner API

```go
// internal/pool: implemented scalar leaf, no allocation or compaction.
func (p *Projectiles) WriteCheckpoint(e *checkpoint.Encoder) error
// internal/combat
func NewCheckpointContext(u *units.CheckpointContext, w *world.CheckpointContext) *CheckpointContext
// Exposes Units *units.CheckpointContext, World *world.CheckpointContext.
func (s *Service) CollectCheckpointReferences(c *CheckpointContext) (int, error)
func (s *Service) WriteCheckpoint(e *checkpoint.Encoder, c *CheckpointContext) error
func (s *Service) AppendCheckpointSummary(s *checkpoint.Summary) error
```

Combat adds no graph tables; retained allocation edges use the shared unit
table and weapon/catalog edges use admitted content keys. The projectile pool
leaf writes stored capacity, count, then the complete dead-flag row (with its
count); neither the active prefix nor the diagnostic cookie row substitutes
for that storage. A lazy zero-value pool keeps its zero capacity word and
absent dead row without materializing its effective default capacity. Validate
count and row dimensions against the effective capacity. Compaction scratch
and diagnostic cookies are excluded.

Service and projectile records follow §16.3.5's dispositions and lexical
source-width framing. Every record through the initialized arena capacity is
retained, including residual records after the current count; malformed or
uninitialized arena storage refuses until composition supplies it. Incoming shots retain the physical count span, including dead-flagged rows,
in stored order; the overwritten tail beyond count is excluded. Its storage
must match capacity, except nil is allowed while count is zero. Death-notified keys are sorted
raw handles and their values retain allocation identity. Target lists retain
stored order, gates and cursors; movement writes the separate air-base lists.
Area-damage cells/nodes/generation rows retain their actual stored values.
Impact stacks, pending transport handoffs and nonzero current damage generation
refuse. No collector invokes target queries, ports or callbacks. Unverified
production owner/rule/function bindings remain explicit U6 refusals, with
absent tag zero in the initial writer.

The direct combat summary writes effective pool count/capacity, record count,
then each stored projectile's Dead, WeaponID, Pos(X,Y,Z), TargetUnit,
TargetProjectile, TargetPos(X,Y,Z), Shooter, Velocity(X,Y,Z), ExpiryTick,
BurstRemaining; then targets.lastRebuild[10], targets.gate[10],
scanCursor.next[10], modernNextProjectileTick and communityAreaGenCounter.
WeaponID is the actual signed scalar, not a content lookup; handles remain
unsigned words. It invokes no binding and collects no graph. Target-list
contents and modernTick are deliberate cheap-row blind spots retained in full.

#### 16.3.17 U4 effects-owner API

```go
// internal/frame
func (b *EventBuffer) WriteCheckpoint(e *checkpoint.Encoder) error
func (b *EventBuffer) AppendCheckpointSummary(s *checkpoint.Summary) error
// internal/effects
func NewCheckpointContext(keys *content.CheckpointKeys, p *FixedEffectPool) *CheckpointContext
// Exposes Keys *content.CheckpointKeys, Pool *FixedEffectPool.
func (s *EffectService) CollectCheckpointReferences(c *CheckpointContext) (int, error)
func (s *EffectService) WriteCheckpoint(e *checkpoint.Encoder, c *CheckpointContext) error
func (p *FixedEffectPool) CollectCheckpointReferences(c *CheckpointContext) (int, error)
func (p *FixedEffectPool) WriteCheckpoint(e *checkpoint.Encoder, c *CheckpointContext) error
func (p *DebrisPool) WriteCheckpoint(e *checkpoint.Encoder) error
func (a *EffectAnimPlayer) WriteCheckpoint(e *checkpoint.Encoder) error
// Each service/pool also exposes AppendCheckpointSummary(*checkpoint.Summary) error.
```

These owners introduce no object tables. Section 12 composes, in order,
EventBuffer, EffectService, FixedEffectPool, DebrisPool and the session strip
table, each with explicit presence. Their payloads follow §16.3.5's retained
fields and lexical source widths; raw handles use the explicit u32 override.
The event window and its derived effect count must be empty. Event-buffer
limits, exhausted flag and next identities remain; drop/overflow diagnostics
do not. A service's owner must be nil or the exact context Pool with the known
concrete type; an ownerless pending record is unsupported. Immutable art and
terrain/impact ports need U6 attestation and initially refuse when nonnil.
The stored fragment-stepping enable flag must equal TerrainHeight presence;
that existing presence byte represents it (§16.3.68). It is not an active
traversal marker. Root owns the completed-tick boundary. Stored duration rows and all
animation cursor fields remain, without advancing or resolving an animation.
Fragment material is excluded host artwork. Debris retains occupied partition
geometry and live-slot state; free spans and dead-slot payload remain excluded.
Only the active block prefix is emitted, with fields charge, generation, occupied,
slot and start. The slot array has its physical length followed by each live bit
and, when live, its retained lexical payload. Geometry writes the occupied-span
count then, in block order, the block index i64, point start i64, point count u32
and XYZ i64 triples. Point start is the stored block start divided by 12; count
is `(charge - 110) / 12`. Allocation charges 110 plus 12 per point and absorbs
only a remainder below 9, so the division recovers the retained span even after
its slot expired or was reused. A stale occupied block need not share the current
slot generation; that residual is part of arena state.

The cheap section appends EventBuffer nextID,nextSequence,exhausted; service
nextID,lastSequence; fixed record count and each record's X,Y,Z,VX,VY,VZ,
ExpiryTick,AnimA.Idx,AnimA.Countdown,AnimB.Idx,AnimB.Countdown, then live fragment
count; live debris slot count. The partition count is retained only in the full
writer. Session appends its strip summary afterward. These direct
summaries read selected values only and invoke no ports or full graph walk.
Other retained physics/geometry/animation values remain full-digest blind
spots of the cheap row. The full owner writer validates quiescence; session
owns the completed-tick boundary for the direct summary.


#### 16.3.18 U3–U4 integration evidence

The reviewed path, economy and construction writers are integrated on
`multiplayer-m3-u2`. Their authored vectors, alias/residual tests, refusal and
read-only checks pass independently. Whole-session composition remains U6.
`world.Wind.WriteCheckpoint` retains Changed, DirX, DirZ, Heading, Max, Min,
NextChange, Scalar and Strength in lexical source widths. Its direct summary
writes strength, heading, raw scalar bits and next-change; selected NaNs fail
atomically. LastChange and BriefingCountdown remain presentation-only.

A displayless comparison of baseline `270c1eca1` and movement-metadata candidate
`f0d6fe394` used the same scene 1, Town & Country, Modern rules, both seeds 7,
three 250-unit armies, 1,200 warm-up and 300 measured ticks, Go 1.27.1 on
Darwin/arm64 with two runtime workers. Catalog/configuration, the complete census,
all initial/warm/final partial fingerprints and RNG draws matched (15,645
simulation; 985,036 CRT). Median tick time was 1.988/1.768 ms, p95 4.329/2.919 ms,
and CPU 2.253/1.914 ms per tick. These shared-host runs do not establish a speedup.
Allocated bytes per tick increased from 86,992 to 103,326, and objects from
807.71 to 908.37: about 16.3 KB and 101 objects for captured accessor metadata.
Neither measured window collected garbage. This cost remains an input to U7's
explicit checkpoint budget; it is not acceptance evidence for full capture.
All integration is on the multiplayer worktree branch, pending play testing.


The assembled U3/U4 owner group and initial AI manager writer passed the
whole `tools/check` gate at candidate `fa8463aa` and `tools/check-retail` at
`202ff959`, after correcting two non-comparable fixture fields and diagnostic
capitalization caught by lint. The retail gate included the ordinary corpus,
amd64 fingerprint locks and GPU device fixtures. Logs are outside the repo at
`/private/tmp/nanolathe-m3-u3-u4-check.log` and
`/private/tmp/nanolathe-m3-u3-u4-retail.log`. No single-seat lock moved. This is
owner-group regression evidence, not the still-pending complete M3 gate.

#### 16.3.19 U5 computer-owner API and staging

```go
// internal/ai: shared U5 boundary implemented before the owner dispatches.
func NewCheckpointContext(u *units.CheckpointContext, w *world.CheckpointContext) *CheckpointContext
// Exposes Units *units.CheckpointContext, World *world.CheckpointContext.
func (m *Manager) CollectCheckpointReferences(c *CheckpointContext) (int, error)
func (m *Manager) WriteCheckpoint(e *checkpoint.Encoder, c *CheckpointContext) error
func (m *Manager) AppendCheckpointSummary(s *checkpoint.Summary) error
```

Managers introduce no graph tables. Factory is an allocation-table edge;
ordered groups and rally targets retain raw u32 handles. Manager, embedded
Strategic and profile fields follow lexical source order and §16.3.5. Profile
maps retain nil/present framing before their sorted key/count/value sequences:
`fixtureWeights != nil` gates future application, so an absent map and an empty
map cannot collapse. StartOwners also writes nil/present before its count because
the observation builder distinguishes an absent public assignment. Profile record-ID mappings sort by admitted definition
identity rather than pointer or canonical name. Strategic setupDrawsReady is
retained; the draw ledger and intermediate region draws remain excluded.
Catalog aliases, Survival input, gameplay rules, callbacks and external
controllers initially refuse pending explicit composition admission. No writer
invokes profile application, a rule, lookup producer, planner or worker.

The existing manager-state writer and application instrumentation are separate
U5 units. The shared ControllerCheckpoint and ControllerCheckpointProvider types
are those in §16.3.7. The owner writer alone does not claim application history
or Modern-controller coverage. U5 instrumentation supplies those records before
U6 admits a live manager. Unknown Manager.Ext values fail; an arbitrary provider
implementation is not sufficient proof of canonical composition.

The manager's cheap fragment writes Controller, all ten Deadlines, countdown,
then the nine group lengths in task-slot order (resource through rally). Session
writes manager presence and appends the controller record after that fragment:
application count, four little-endian u64 hash words, next-think presence/tick,
deadline presence/tick, tokens and last fill. Controller kind in the manager
retains its stored enum; application-chain framing uses Classic 1 and Modern 2.
Only selected scalars are read, with no graph or binding validation.


#### 16.3.20 U5 Modern executor writer API

The private executor writer is separate from host scheduling metadata and the
application journal. It never reads even the pointers to mapInfo, obs, kit,
brain, generator or batch; those retain the explicit Modern worker exception.
It neither joins nor samples worker readiness. The shared context is
`ai.CheckpointContext` (§16.3.19).

```go
// internal/aikit/checkpoint_executor.go
func (e *executor) writeCheckpoint(enc *checkpoint.Encoder, c *ai.CheckpointContext) error
func (p *Persona) writeCheckpoint(enc *checkpoint.Encoder) error
// internal/aikit/checkpoint_grids.go
func (d *gridDedupe) writeCheckpoint(enc *checkpoint.Encoder, path string) error
func (g *exitGrid) writeGuardCheckpoint(enc *checkpoint.Encoder, path string) error
func (g *exitGrid) writeSelfCheckpoint(enc *checkpoint.Encoder, path string) error
func (f *freeCache) writeCheckpoint(enc *checkpoint.Encoder, path string) error
func (p *pendingSite) writeCheckpoint(enc *checkpoint.Encoder, path string) error
```

The exact source-lexical records are:

| Record | Retained fields |
|---|---|
| executor | dedupe, freeSeq, freeSlot, frees, grids, lastFill, lastTick, m, nextPending, pending, selfGrid, spotCover, table, tokens |
| gridDedupe | cellIdx, cellStamp, gen, unitIdx, unitStamp |
| guard exitGrid | built, cost, fac, ox, oz, reach, reachStamp, sealed, seeds, seen, stamp |
| self exitGrid | cost length as i64, seen, stamp |
| freeCache | asked, class, comp, compGen, gen, regions, seen, seenStamp, stamp, tick, used, val |
| MoveClass | FootX, FootZ, MaxDepth, MaxSlope, MaxWaterSlope, MinDepth |
| freeRegion | wide, x0, x1, z0, z1 |
| pendingSite | cx, cz, fx, fz, g, tick |
| Persona | APM, Ambition, Attention, Burst, Omniscient, Reaction, Skill, ThinkEvery |

Fixed arrays have no count: four grids, three free caches and sixteen pending
reservations. Slices have their exact count and stored order, including old
slots and generation marks. Guard cost has an additional presence byte: its
nil test participates in picture reuse. Self-grid cost values are rebuilt,
but their length controls whether the next ensure clears the retained seen
array; encode that shape gate without walking cost. Executor spotCover is a
persistent simulation-thread bool slice reused by length, so it is retained;
the exception for MapInfo does not extend to this cache. Persona Name is a
label and Async is scheduling; neither is encoded or normalized at capture.
Raw factory handles use u32, Go ints use i64, and other scalars retain their
source widths. There are no graph tables or unordered maps.

The blockers who/blk and dd pointer are excluded: only the unblock command
consumes them, after rebuilding selfGrid, clearing blk, rewriting who and
assigning dd to this executor's dedupe. Guard grids do not consume them.
Distances, predecessor/queue/heap storage and free-cache queues are rebuilt
scratch. guardFacs, nGuard, keep, rowNear, cutBuf, blkBuf, rows, lanes,
allyTowers, featDist and featOrder reset before their consumers. Places is
immutable placement geometry and stats is reporting. Applied batch.rowNear
must enter the application-attempt framing because that persistent worker
input changes how an otherwise identical command executes.

The initial executor writer requires absent m/table tags, with TODO(M3-U6)
refusal for nonnil unverified bindings. U6 must bind the exact owning manager,
terrain and table/catalog/rules provenance without calling tableFor, BuildTable
or a placement producer. Host application instrumentation owns the quiescence
marker and controller summary; this private writer alone does not claim live
Modern-controller capture.

#### 16.3.21 U5 order application receipts

AI history observes the existing queue methods while an application is in
progress. It does not replay requests through a second dispatcher. The scoped
observer is diagnostic, simulation-thread-only, and absent by default. The
caller already owns the actor allocation; observation must not resolve it
through a new lookup, lazily create a queue, or rebind one.

```go
// internal/orders/checkpoint_receipts.go
type CheckpointOrderReceipt struct {
    Kind uint8
    Segment uint8
    Index int64
    Node Node
    PreviousCount, Added uint32
    Preparation uint8
}
type CheckpointOrderObserver interface {
    RecordCheckpointOrder(CheckpointOrderReceipt)
}
func (q *Queue) SetCheckpointObserver(CheckpointOrderObserver) CheckpointOrderObserver
func (n *Node) WriteCheckpointValue(*checkpoint.Encoder) error
```

SetCheckpointObserver returns the previous observer for scoped restoration.
Nested scopes replace and restore, never fan out. A nil queue returns nil.
The callback is synchronous and must consume the receipt before returning;
Node is a value snapshot but its opaque payload slices are borrowed read-only.
The observer must not change gameplay, invoke producer work, or return errors
into a gameplay path. It retains diagnostic failures itself. An active observer
makes a full queue checkpoint fail as an application still in progress. No
observer identity enters checkpoint bytes, and an absent observer introduces
no receipt allocation or additional binding call.

Receipt kinds are purge 1, drop-leading-auto 2, inserted-node 3, tail-coalesced
4 and primary-preparation 5. Purge and drop receipts precede their actual
invocation, including an empty queue. Primary preparation has subkind 1 before
the existing stop-firing-position call and subkind 2 before the existing rules
BeforeCommand call. These run before Push's allocation guard and remain
visible even if that guard refuses insertion. Insert receipts follow the
actual mutation and all node flag writes in Push, PushHead, PushSecondary and
appendTail; they contain the actual node, segment (primary 1, secondary 2),
and zero-based index. Coalescence emits only after the existing tail was
changed, retaining the old count, actual add (including zero-to-one conversion),
and final node value. The existing wrapping arithmetic is unchanged. A
coalescence fallback emits the ordinary Push receipts, with no fictitious
coalescence or insertion on refusal. Nested cleanup emissions keep their
actual order within the outer invocation.

WriteCheckpointValue reuses the retained node-value schema from §16.3.6,
including raw handles and the existing staging exclusions; no capture-local graph identity is
introduced. It rejects nil or unconsumed insertion inputs just as the graph
writer does. The application chain maps receipts 1–3 to existing operation
kinds 1–3. It adds operation 9 for primary preparation (actor, subkind), and
operation 10 for general tail coalescence (actor, segment, index, old count,
actual add, final retained node). Operation 3 additionally retains segment and
index. This distinguishes factory/mobile coalescence from the stockpile
producer's capped-count operation 4. These are refinements of the unreleased
M3 schema, not changes to order behavior. Node removal side effects remain
owned by their enclosing purge/drop/preparation operation; this journal is
an ordered application receipt, not a second world mutation log.

#### 16.3.22 U5 application-chain envelope API

The history core owns the common envelope and retains only player/controller,
next serial, count and digest between attempts. Nil disables it. Construction
binds the admitted content/configuration digests, player 0–9 and history kind
Classic 1 or Modern 2 (distinct from the existing Controller enum). It starts
with next serial 1, count zero and the §16.3.7 initial digest.

```go
// internal/ai/checkpoint_history.go
func NewApplicationHistory(checkpoint.Identity, uint8, uint8) (*ApplicationHistory, error)
func (h *ApplicationHistory) NextSerial() uint64
func (h *ApplicationHistory) BeginAttempt(tick uint32, serial uint64, ordinal uint32,
    writeIntent func(*checkpoint.Encoder) error) *ApplicationAttempt
func (a *ApplicationAttempt) Operation(kind uint16, write func(*checkpoint.Encoder) error)
func (a *ApplicationAttempt) Finish(apm, terminal uint8)
func (h *ApplicationHistory) Fail(error)
func (h *ApplicationHistory) Snapshot() (ApplicationHistoryState, error)
func (h *ApplicationHistory) WriteCheckpoint(*checkpoint.Encoder) error
func (h *ApplicationHistory) AppendCheckpointSummary(*checkpoint.Summary) error
```

The exact common attempt bytes are player u8, history kind u8, tick u32,
assigned serial u64, zero-based ordinal u32, the controller codec's typed
intent/observed actors/optional target/product, APM u8, operation count u32,
ordered operations (each tag u16 then its published operands), and terminal
u8. There is no extra byte-string length around the typed intent or operation
list. The synchronous writer callbacks are internal codec boundaries: they
only encode already-observed values and may not invoke gameplay, retain the
encoder, or inspect workers. This core does not substitute an untyped payload
for the controller codecs still required by U5.

Assigning a serial increments the next serial even for an empty batch. The
last representable next serial is refused before it could wrap; counts and
operation counts similarly refuse overflow. Overlapping attempts, unknown
operation tags, invalid outcome tags, a Classic APM tag other than unlimited,
and an APM rejection with operations or a non-rejection terminal invalidate
reporting. First failure is sticky; callers continue gameplay normally.
Finishing releases both temporary byte buffers even after an error. The chain
never retains nodes, allocations, commands or worker data between attempts.

Snapshot refuses an active attempt or a stored diagnostic failure. Full
history framing is presence then count u64, the fixed 32 hash bytes, history
kind u8, next serial u64, player u8. The ring fragment remains the five words
in §16.3.7: count and four little-endian words of the hash, continuing the
caller's accumulator. Nil history is an absent full record; a summary requires
an enabled history, since its presence is the composition owner's field.

#### 16.3.23 U5 typed intent and order-receipt codecs

The `ai` package owns value-only codecs shared by both controller adapters.
They accept already-observed allocation identities and already-admitted
content identities. The adapter resolves these from the existing objects
using the admission's CheckpointKeys; the codec performs no lookup, producer
call, pointer serialization, or input normalization.

```go
// internal/ai/checkpoint_intent.go
type ApplicationOperands struct {
    Actors []checkpoint.Allocation
    Target *checkpoint.Allocation
    Product *checkpoint.Definition
}
type ClassicApplicationIntent struct {
    Kind uint8
    Code int64
    ResolvedRow uint8
    Modifier uint8
    Argument int32
    X, Y, Z numeric.Fixed
    RawTarget uint32
    UnitKey string
    BuildKind int64
    Count int64
    RequestTick uint32
    Active bool
    Operands ApplicationOperands
}
type ModernApplicationIntent struct {
    Kind uint8
    Queued bool
    RawTarget uint32
    X, Z int32
    ProductIndex int32
    ProductKey string
    Slot, Count, Spot, Spacing int32
    Keep, Exact bool
    RowNear int32
    Operands ApplicationOperands
}
func (v ClassicApplicationIntent) WriteCheckpoint(*checkpoint.Encoder) error
func (v ModernApplicationIntent) WriteCheckpoint(*checkpoint.Encoder) error
// internal/ai/checkpoint_order_receipts.go
func (a *ApplicationAttempt) RecordOrder(actor checkpoint.Allocation, receipt orders.CheckpointOrderReceipt)
```

Classic intent kind 1 is an ordinary order: kind u8, original command Code
i64, actual ResolvedRow u8, Modifier u8, Argument i32, XYZ i64 each and raw
target u32. Kind 2 is a typed build: kind u8, BuildKind i64, UnitKey string,
XZ i64 each, requested Count i64 and the actual BuildRequest.Tick u32.
Kind 3 is activation: kind u8, Active Boolean. Each is followed by the same
Operands encoding: actor count u32, each allocation in observed emission
order (duplicates retained), target presence and allocation if present,
product presence and definition if present. Fields belonging only to another
Classic variant do not enter that variant's bytes. No unknown Classic kind
is accepted. Row zero remains a valid resolved order input.

Modern's existing CmdKind numbers 1–14 are retained. Its exact fixed operand
order is Kind u8, Queued Boolean, raw target u32, XZ i32 each, ProductIndex
i32, ProductKey string, Slot/Count/Spot/Spacing i32 each, Keep/Exact Boolean,
RowNear i32, then Operands. Actor-span offsets are replaced by that ordered
actor list, not sorted or encoded as batch-buffer offsets. The applied
batch.rowNear enters each attempt. ProductIndex and ProductKey are copied
from the observed UnitInfo if present, zero and empty otherwise; the adapter
validates that object's identity against its immutable table and admitted
definition without constructing either. Kind zero or a new unknown kind
invalidates reporting until its codec is reviewed.

Allocation values require nonzero handle and serial. Optional absence has its
own presence byte; it is not inferred from coordinates or a null target
handle. Product identities require the admitted catalog family, nonzero unit
ordinal and a nonempty `unit/` key; that structural check supplements, and does
not replace, the adapter's pointer-to-admitted-key check.

RecordOrder maps receipt kinds 1/2/3/4/5 to operation kinds 1/2/3/10/9.
Purge/drop payload is actor allocation. Insertion is actor, segment u8, index
i64, retained node values. Coalescence is actor, segment u8, index i64,
previous count u32, actual add u32, retained node values. Preparation is actor
and subkind u8. Unknown receipt kinds, invalid segments/indices or preparation
subkinds invalidate reporting. Node owner is retained as supplied, even when
it differs from the actor under nested cleanup; it is never repaired. The
receipt codec does not interpret receipt count as a committed-effect count:
controller terminal classification remains at the actual application sites.

#### 16.3.24 U5 manager binding and Modern scheduling metadata

A manager installs its optional diagnostic history only before creating its
controller. Session U6 owns the stricter admitted-entry check; this helper
rejects missing keys, unsupported controller kind/player, an existing history
or an existing Ext value, including a typed nil. It never resets a chain.

```go
// internal/ai/checkpoint_application.go
func (m *Manager) EnableCheckpointApplications(checkpoint.Identity, *content.CheckpointKeys) error
func (m *Manager) CheckpointApplicationHistory() *ApplicationHistory
func (m *Manager) CheckpointApplicationKey(*content.UnitDef) (checkpoint.Definition, error)
func CheckpointAllocation(*units.Unit) (checkpoint.Allocation, error)
// internal/ai/checkpoint_controller.go
func (v ControllerCheckpoint) WriteCheckpoint(*checkpoint.Encoder) error
func (v ControllerCheckpoint) AppendCheckpointSummary(*checkpoint.Summary) error
```

The manager's private checkpointHistory and checkpointKeys are instrumentation
bindings. Its full Manager writer does not duplicate them: application state
belongs to its separate U5 history/controller fragment and keys to frozen
content admission. A retired observed unit is still an allocation identity;
CheckpointAllocation reads its existing nonzero handle/serial, never a current
slot lookup. The product helper uses the frozen key lookup by object identity.

The controller provider additionally implements
`AppendControllerCheckpointSummary(*checkpoint.Summary) error`, so the cheap
ring can refuse active/failed applications without invoking a full writer.
ControllerCheckpoint itself is a detached value getter, not admission: it
returns Present for a nonnil host and its simulation-thread fields, filling
serial/count/hash only from a successful history snapshot. A nil host returns
zero. Full/summary writers validate before consuming this value.
The value record's full encoding follows its source-field lexical order:
ApplicationCount u64, fixed ApplicationHash bytes, DeadlinePresent Boolean,
DeadlineTick u32, Initialized Boolean, LastFill u32, NextBatchSerial u64,
NextThinkPresent Boolean, NextThinkTick u32, Present Boolean, Tokens i64.
Its selected 11 summary words are the count/hash five, next-think presence/tick,
deadline presence/tick, tokens and refill tick. Full and summary values preserve
stored ticks even when their presence is false; the provider validates the
boundary before passing a detached value to either leaf.

The Modern Host borrows its manager's history at NewHost construction. Its
inited and nextThink fields are already simulation-thread-only; use them for
Initialized and next-think presence/tick. Add simulation-thread-only mirrors
for pending deadline presence/tick, the assigned current batch serial, and an
application-in-progress marker. Set the deadline and assign a serial at the
existing Step site that marks a batch pending, including an empty batch, before
starting a worker. Clear only deadline presence after applyBatch finishes;
keep its assigned tick. Mirror the same path for Reaction zero. These writes
must not change joins, scheduling, command dispatch or worker data.

ControllerCheckpoint reads only those simulation-thread fields, executor
tokens/lastFill and the history snapshot. Full writing/cheap summaries first
refuse missing or failed history, an application-in-progress marker or a
history belonging to another manager; they never inspect ready, flight, kit,
brain, generator, observation, mapInfo or any batch field. The full Host payload
is the detached controller record, effective Persona, then executor record.
The initial full writer still refuses unverified executor manager/table
bindings under §16.3.20 until U6 attests them. This scheduling unit does not
claim to record applied attempts until the typed producer instrumentation lands.

#### 16.3.25 U5 typed-build return classification

`ai.WithCheckpointBuildVerdict(error, uint8) error` preserves the existing
error text and unwrap chain while tagging a known return branch.
`ai.CheckpointBuildVerdict(error) (uint8, bool)` reads only that tag (including
through ordinary wrapping); nil is success 1. It never parses diagnostics.
Constants CheckpointBuildSuccess/Product/Site/Owner/Limit/Binding/Other retain
§16.3.7 values 1–7. An unknown error or invalid tag remains unclassified and
must fail history reporting, not command execution.

The session's existing AI typed-build closure classifies these branches:
missing world/catalog or queue/descriptor is binding 6; missing/dead builder
is owner 4; empty/unknown product or missing product movement profile is
product 2; allocator exhaustion is limit 5; a nonpositive count is the known
other-failure branch 7. The existing mobile producer does not add a new site
check for diagnostics. Classification uses construction's existing sentinel
errors with errors.Is, preserving their text and identity. A previously
unknown construction error is returned unchanged and remains unclassified.

Stockpile alias admission keeps its existing Boolean wrapper for other
callers. A private result helper executes the same checks and operations once,
returning success 1, binding 6 for a missing descriptor/queue, owner 4 for a nil
unit, other 7 for a nonpositive count, or product 2 when the existing stockpile
slot predicate refuses. AI maps that result to its existing error string with
the verdict attached. Successful queue/coalescence behavior and the actual
BuildRequest.Tick are unchanged, including zero where a producer omits Tick.
Return success still does not prove an order was inserted when its existing
allocation guard refuses; actual order receipts determine that separately.
This return-classification unit does not yet install the queue observer or
emit operation 5; those belong to the enclosing application instrumentation.

#### 16.3.26 U5 producer observation helpers

```go
// internal/ai/checkpoint_producers.go
func (h *ApplicationHistory) ActiveAttempt() *ApplicationAttempt
func (a *ApplicationAttempt) ObserveQueue(*orders.Queue, *units.Unit) func()
func (a *ApplicationAttempt) CommittedOperations() uint32
func (a *ApplicationAttempt) IssuedOrders() uint32
func (a *ApplicationAttempt) CoalescedOrders() uint32
func (a *ApplicationAttempt) RecordStockpile(checkpoint.Allocation, orders.ID, int64, bool)
func (a *ApplicationAttempt) RecordActivation(checkpoint.Allocation, bool)
func (a *ApplicationAttempt) RecordBuild(checkpoint.Allocation, BuildRequest, error)
```

ObserveQueue scopes the already-existing queue to the issuing allocation and
returns restoration of its previous observer. It does not look up, allocate
or bind a queue. Nil attempt or queue returns a no-op restoration. Session
uses it only after its existing lazy queue binding, so a factory's first queue
is observable without inserting a new bind before product validation. Nested
scopes replace/restore and never duplicate receipts. All helpers remain
simulation-thread-only and disabled with a nil history/attempt.

Operation 6 is actor allocation then requested Boolean, recorded immediately
before the existing activation setter, even for an already-equal value.
Operation 5 refines the unreleased framing to preserve the actual request:
observed actor allocation, raw Builder u32, UnitKey string, XZ i64 each,
actual facing u8 (zero: BuildRequest has no facing input), requested Count
i64, Kind i64, Tick u32 and stable verdict u8. Intent carries the admitted
product identity when available; an invalid request key is still represented
by its exact request string. Unclassified errors poison history reporting.
These helpers never execute a setter or build callback themselves.

The attempt tracks derived receipt counts for producer-side classification:
IssuedOrders counts only actual insertions/coalescences (operations 3/10).
CoalescedOrders counts operation 10 alone. Operation 4 follows the stockpile
queue call and writes actor allocation, resolved row u8, actual capped count
i64, and whether that call produced a coalescence receipt. It is metadata;
it never substitutes for the insertion/coalescence operation itself.
CommittedOperations counts the semantic invocations/mutations 1/2/3/6/7/8/9/10,
including the explicitly recorded empty purge/drop/preparation calls. The
stockpile and typed-build metadata records 4/5 do not alone prove a committed
operation. Counts increment only after successful encoding and cannot outgrow
the u32 operation count. They are temporary application bookkeeping, not new
checkpoint payload or gameplay state. Callers still classify explicit no-op
and partial group outcomes at their actual branches; neither receipt count nor
a nil typed-build error is enough to infer successful order insertion.

#### 16.3.27 U5 Classic producer instrumentation

A Classic decision serial belongs to one concrete actor submission after the
existing caller's selection/admission gates; its ordinal is zero. Group
broadcasts retain their existing member order and assign each submitted
member a serial. No selection/search/upkeep or skipped rally resolution is
invented as a submission. The private resolved-order helper carries the
original command code as well as the resolved row; resolution and queue binds
keep their existing order and invocation counts.

Begin a submission before its existing queue mutation; observe a queue only
after the existing bind. Order submissions use Classic intent kind 1; mobile
and factory builds kind 2; activation kind 3. Actor/target allocations come
from the observed objects. A product found in the selected catalog must resolve
through the manager's frozen keys; an unknown product has no definition operand
and keeps its raw request key. Capture errors never gate gameplay. Activation
records the requested value immediately before the actual setter, even if it
already matches. Its terminal is success.

A typed call emits operation 5 after the call, retaining its actual request and
classified return. The session scopes the same active attempt after its existing
lazy queue bind, including a factory's first queue. Nested scopes replace and
restore, so each operation appears once. queueExactResult borrows the enclosing
mobile-build attempt; it never starts another serial. Its own invalid-site,
missing-builder and missing-callback errors receive site/owner/binding verdicts
without changing text, order or existing wrapped errors. Request Tick remains
zero where the current producer omits it.

Resolved nonzero order plus actual insertion/coalescence is success. A rejected
resolution (including its row-zero insertion), failed typed call, or missing
insertion is partial when any committed semantic operation ran, otherwise
rejected with no commit. A nil typed return alone is not success. Explicit
accepted no-op is reserved for a producer branch that actually accepts a no-op;
Classic's submission paths currently have none. All Classic attempts use the
unlimited APM tag. Restore queue observers before finishing the attempt. Existing
activation RNG, queue preparation, purge, insertion, count, callback and resource
behavior must match with diagnostics enabled, disabled or failed.

#### 16.3.28 U6 binding authority primitive

The stdlib checkpoint leaf supplies a local provenance token:

```go
type BindingAuthority struct { /* private, non-zero-sized */ }
func NewBindingAuthority() *BindingAuthority
func (a *BindingAuthority) Matches(expected *BindingAuthority) bool
```

Matches is true only for the same nonnil token. It emits no bytes and selects
no gameplay behavior. A session's admitted composition owns one private token;
owner contexts borrow it alongside the exact expected typed owner aliases.
Known installation sites pass it when installing their reviewed bindings.
Each callback slot retains its own private authority. Ordinary setters clear
only that slot's authority, even for a caller supplying the same function.
Canonical installers set only the slot they actually install; they cannot
bless another slot's current contents. A replacement therefore refuses capture
without needing Go function equality, addresses in the stream, reflection or
revision counters. A foreign token is not the session's token. No live-owner
getter, snapshot or host result exposes the private authority.

Callback storage that remains publicly writable cannot claim this proof: it
must first be encapsulated or replaced by an immutable callback/provenance
value. A copied callback used later must carry its installation provenance
with the copy, not consult its source's current stamp. Regular later canonical
installs may reuse the same authority; full capture still validates their
logical operands and exact owner aliases. Nil callbacks retain their absent
encoding. This primitive alone admits no owner and removes no U6 refusal;
each owner must have its installation API and wire binding tags reviewed.

#### 16.3.29 U5 exact insertion completion

Mutation receipts alone cannot classify an outer insertion: a preparation
callback can insert a node before the outer Push reaches its allocation guard.
Orders therefore supplies a separate, optional diagnostic completion interface:

```go
type CheckpointInsertionResult struct {
    Method uint8 // Push 1, CoalesceTail 2
    Row ID
    Inserted, Coalesced bool // mutually exclusive; both false means refused
}
type CheckpointInsertionObserver interface {
    RecordCheckpointInsertion(CheckpointInsertionResult)
}
```

The existing CheckpointOrderObserver may additionally implement this interface.
Public Push and CoalesceTail retain their signatures and execute the same private
insertion cores, now returning the actual local result. CoalesceTail's fallback
uses the Push core; only the public outer invocation reports its completion.
Completion runs once after every ordinary return, including an allocation
refusal, after all nested callbacks. A panic remains a panic, with no invented
completion. Nil queue or absent observer emits none. Existing mutation receipt
order/bytes remain unchanged. No result is encoded as a new operation: it is
transient classification evidence, not a second gameplay return channel.

The invocation captures its entry observer, so a nested temporary observer cannot
steal the outer completion. The AI observer exposes:

```go
func (a *ApplicationAttempt) InsertionIndex() uint64
func (a *ApplicationAttempt) InsertionAfter(uint64, checkpoint.Allocation, uint8) (orders.CheckpointInsertionResult, bool)
```

The AI observer remembers each completion's actor, value and an attempt-local
u64 position. A producer takes the position immediately before its existing
insertion/typed call, then accepts only a later completion for that observed
actor and expected method. Nested insertion results are superseded by the outer completion. No
completion, a refused completion, or a mismatched actor cannot establish that
the requested insertion succeeded. Position exhaustion fails diagnostics. The
result/position never affects queue admission, callbacks, statistics or gameplay
control flow and is discarded when the attempt finishes.

#### 16.3.30 Path binding admission preparation

Scheduler admission uses §16.3.28 at its existing private callback slots:

```go
// internal/path
func NewSchedulerWithCheckpointBindings(SearchFunc, PublishFunc, *checkpoint.BindingAuthority) *Scheduler
func (s *Scheduler) SetSearchWithCheckpointBinding(SearchFunc, *checkpoint.BindingAuthority)
func (s *Scheduler) SetPublishWithCheckpointBinding(PublishFunc, *checkpoint.BindingAuthority)
func (s *Scheduler) SetCandidateProviderWithCheckpointBinding(CandidateProvider, *checkpoint.BindingAuthority)
func (c *CheckpointContext) SetSchedulerBindings(*Scheduler, *checkpoint.BindingAuthority) error
func CheckpointProviderMatches[T any, P interface { *T; CandidateProvider }](s *Scheduler, expected P, a *checkpoint.BindingAuthority) bool
```

Keep separate private authority slots for search, publish and provider. Ordinary
setters clear only the changed slot before their existing work. Canonical siblings
perform that work exactly once, then stamp only their own present slot. In
particular provider installation still calls PlayerCount then UnitLimit once;
capture calls neither. Nil receives no authority. The old constructor remains
unattested. Context registration is private, rejects nil/conflicting registration,
and accepts an identical registration idempotently. Collection and writing both
check the exact registered scheduler and nonnil matching authority for every
present binding; absent slots retain tag 0 and verified present slots use tag 1
at their existing lexical positions.

The generic provider helper accepts only a nonnil concrete provider pointer and
type-asserts to that pointer type before comparing it. It checks the scheduler and matching
provider authority without invoking the provider or comparing arbitrary open
interfaces. Movement uses its concrete nonnil provider pointer and checks the
exact system/scheduler/provider aliases before section 8. Unknown noncomparable
provider implementations therefore refuse without a panic. These owner APIs
prepare U6; until canonical movement/session installation is wired, production
capture retains its unsupported-binding refusal.

#### 16.3.31 U5 Modern placement receipts

Operation 7 records the actual pending-ring write and adjacent cursor advance
as one compound operation: written slot i64; new row cx/cz/fx/fz i32, group u8,
tick u32; resulting nextPending i64. Emit it after both assignments in
placedRow, preserving all preceding guard operations. guardPlaced itself
writes no ring row; ordinary/extractor/exact site success must not invent one.

Operation 8 records a write to one of the four guard-grid slots: slot i64
(0–3), field ID u8, action u8, then the action operands. Field IDs and values:

| ID | Field | Value |
|---|---|---|
| 1 | built | u32 |
| 2 | cost | i32 slice with presence |
| 3 | fac | raw u32 handle |
| 4, 5 | ox, oz | i32 |
| 6 | reach | u32 slice |
| 7 | reachStamp | u32 |
| 8 | sealed | Boolean |
| 9 | seeds | i32 slice |
| 10 | seen | u32 slice |
| 11 | stamp | u32 |

Actions are scalar assignment 1 (typed value); zero-filled slice replacement 2
(u32 length, preceded by cost's presence Boolean for field 2); element assignment
3 (index i64 then typed value); seeds truncation 4 (resulting u32 length); seeds
append 5 (insertion index i64 then i32 value). Reject unknown field/action pairs
in diagnostic encoding. Executed equal-value assignments still emit; skipped
branches do not. No pointer or reconstructed/sorted final-state diff is encoded.

Instrument ensure's cost/seen/reach replacements in that order; buildExitGrid's
fac, ox, oz, cost writes and seeds truncation/appends; overlay's in-bounds cost
writes; flood's stamp increment and separate wrap fixup, all seen/reach writes
in traversal order, and final reachStamp. gridFor records failure fac=0, or
built followed by physical-order pending overlays, flood and sealed assignment.
guardPlaced visits actual eligible physical slots in order, each overlay,
flood and sealed assignment. Record immediately after each existing assignment.
These writes may precede a failed site, resolver or typed-build call and remain
in the attempt even when no order is installed.

```go
// internal/aikit; scopes only the four guard slots, never selfGrid.
func (e *executor) observeCheckpointLayout(a *ai.ApplicationAttempt) func()
```

The scope installs temporary attempt/slot observations and returns restoration
of prior observations. Nil attempt does no work. Guard writer capture refuses
an active observation. No worker pointers are inspected by this mechanism.
No ordinary behavior is chosen from the observer or an encoding error.

This is a selected semantic application journal, not a second complete mutation
log. selfGrid, dedupe, free caches/selectors, spotCover and batch setup/refill
retain their full-checkpoint coverage and do not use tags 7/8; APM also has the
attempt verdict. Scratch and worker-state exclusions remain as reviewed. Modern
command outcome reporting is a separate unit, using exact insertion completions
(§16.3.29), per-actor/per-selected-target outcomes, and the explicit already-open
Unblock no-op; the old any-issued return and statistics remain unchanged.

#### 16.3.32 U5 Modern command scope and outcome helpers

The executor's temporary checkpointApplication and checkpointBatchSerial fields
exist only during simulation-thread application and are excluded from payloads.
A full executor capture refuses either active field. Host copies its already
assigned batch serial into this scope around its existing apply call and restores
the previous value afterward; it does not assign another serial at application.

```go
// internal/aikit; diagnostic helpers, not a second command dispatcher.
func (e *executor) newCheckpointCommand(*Command, *batch, uint32, uint64, uint32) *checkpointCommand
func (e *executor) checkpointAttempt() *ai.ApplicationAttempt
func (e *executor) checkpointActor(*units.Unit) checkpoint.Allocation
func (e *executor) checkpointAccept()
func (e *executor) checkpointReject()
func (e *executor) checkpointNoop()
func (e *executor) checkpointOrderOutcome(uint64, checkpoint.Allocation, uint8)
func (e *executor) checkpointBuildOutcome(uint64, checkpoint.Allocation, ai.BuildRequest, error)
func (c *checkpointCommand) finish(uint8)
```

The adapter copies all §16.3.23 fixed command operands and applied rowNear,
then the observed actor span in its exact order, duplicates included. It checks
span bounds and raw/observed handle agreement without consulting the live world.
Retired observed allocations remain valid. A product must be the exact existing
table row at its Index and by its definition/key, and its definition must resolve
through frozen keys. The adapter never builds/prepares a table or content.
Disabled/failed history skips intent work. Begin each emitted command before the
existing APM test, using its batch ordinal, including later rejected commands.
An unrepresentable ordinal fails diagnostics rather than wrapping.

The caller scopes the temporary command pointer and §16.3.31 layout observer
around the existing command execution. Restore both on every unwind, but finish
only after ordinary return: a panic remains unchanged and leaves history
incomplete, so capture refuses. APM retains its existing debit/refusal ordering.
Per-actor and per-selected-target helpers record accepted/rejected outcomes;
the old return Boolean, stats and gameplay branches do not consult these flags.
Stop's actual purge/drop is success even when empty. The already-open Unblock
branch is the explicit no-op. Partial groups and reclaim-before-failed-replace
remain partial. Missing actor/target/queue, failed resolution/admission and an
actual insertion refusal mark rejection at their existing branches.

Take an insertion position immediately before each existing Push/CoalesceTail
or typed callback and capture actor identity beforehand. OrderOutcome accepts
only the exact fresh completion. BuildOutcome first records operation 5's actual
request/return and accepts only a nil return with that actor's fresh CoalesceTail
completion. Stockpile operation 4 follows its actual CoalesceTail and uses that
completion's Coalesced flag, never an aggregate count. Existing omitted request
Tick, count normalization/capping and callback order remain unchanged.

At finish, APM rejection is terminal 3 with no operations. With no rejection,
accepted work is success 2; explicit no-op with no committed operation is 1
(with committed operations it is success). Otherwise any committed semantic
operation means partial 4, and no commit means rejected 3. These flags carry no
independent wire data and are not retained between commands.


#### 16.3.33 Movement path binding installation

Movement supplies the scheduler's closed search/publish/provider composition
through the same private session authority (§16.3.28–§16.3.30):

```go
func NewSystemWithCheckpointBindings(*world.Terrain, Profile, *OccupancyGrid, *checkpoint.BindingAuthority) *System
func (s *System) ConfigurePathWithCheckpointBinding(int, int32, func(int) bool, *checkpoint.BindingAuthority)
func (c *CheckpointContext) SetPathBindings(*System, *units.World, *checkpoint.BindingAuthority) error
```

The ordinary and admitted constructors share the current construction body;
terrain/grid setup, scheduler defaults and callback installation occur once in
their existing order. The admitted variant stamps only the search, publisher,
provider and default eligibility closures it actually installs. The eligibility
slot is private on pathProvider. Ordinary ConfigurePath clears its authority
only after its existing receiver/range guards permit installation; the admitted
variant performs the same work and then stamps that one slot. Its scheduler
provider installation uses the matching path authority. An unrelated callback
replacement cannot be blessed by ConfigurePath.

SetPathBindings records the capture's exact system, world and nonnil authority,
registers the same scheduler with the shared path context, and validates the
provider's concrete pointer with CheckpointProviderMatches. Conflicting repeated
registration fails; identical registration is harmless. Capture revalidates the
same aliases and stamps, without invoking eligibility, search, publication or
any CandidateProvider getter. Provider system/world edges must match the
registered owners; eligibility emits absent 0 or verified-present 1 at its
existing lexical field. Ordinary unbound fixtures keep their prior admission.
This path-specific registration does not admit unrelated movement callbacks,
rules, terrain/content or copied mapping bindings; their existing refusals stay
until their own canonical installation is implemented.

#### 16.3.34 Frozen catalog preparation admission

SimulationInputRequest records PreparingRuleName and PreparingRuleBase as the
exact resolved strings used by skirmish preparation. SimulationInputs exposes
them through PreparingRule; absence remains distinguishable and is not defaulted.
These strings are admission metadata: configuration already names the selected
rule, and the content manifest already hashes the resulting prepared values.
The preparation metadata changes neither content digest nor identity encoding
nor gameplay. Match admission
requires the frozen name and base to agree with its registered selected set,
as well as the existing effective Community comparison. Older freeze callers
that omit the identity cannot admit a multiplayer configuration.

Catalog.Clone preserves Limits, including after weapon preparation, restrictions
and mutators. The manifest already encodes all four limits, so this corrects the
frozen content digest of a clone that previously lost them (and encoded zeros),
without changing the manifest schema. Admission compares all four effective compile limits even when
zero: missing evidence is a refusal rather than a bypass. A frozen restriction
set is admitted only when field 12 describes it, and an empty field 12 admits
only unrestricted content (§16.6; until then any frozen set was refused). Profile
name/directory provenance remains a separate pre-online identity requirement.


#### 16.3.35 Session checkpoint history storage

The public value types follow §16.3.6. Boundary names are CheckpointEntry (1),
CheckpointInteriorTick (2), and CheckpointFinalPumpTick (3). The private ring
uses fixed arrays of 64 digest records and 600 tick rows, preserving insertion
order even through uint32 tick wrap. History reads return detached oldest-first
copies without draining either ring. Append performs no allocation. Entry's
digest occupies the first record slot; only completed runtime ticks enter the
tick ring. Explicit off-cadence byte captures belong to the separate result,
not the cadence record array. This storage adds no capture or tick hook by
itself; session composition, boundary admission and request handling remain U6.

#### 16.3.36 U5 integration checks and disabled cost

The integrated Classic and Modern producer instrumentation preserves actual
order completion rather than inferring success from legacy return values.
Cleanup callbacks can change an actor's raw handle or allocation serial;
the journal keeps the allocation observed before cleanup while the existing
order and typed-build paths still read their actual operands afterward.
The exact insertion mark stays immediately before insertion, after cleanup.
Modern regression vectors cover Move, Build and Replace through both purge
and automatic-order cleanup, including disabled/enabled/failed diagnostic
behavior, typed metadata and partial results. Classic follows the same rule.

The full fast and short retail gates passed on the integrated branch on
2026-10-07, including lint, amd64 fingerprint locks under Rosetta and the
GPU device fixtures. The final Classic cleanup correction separately passed
the full AI package gate and was independently verified before integration.
This is owner-group evidence, not whole-session M3 acceptance.

Sequential displayless benchmark runs compared pre-U5 `202ff959` with
integrated `cb7972f5b`: scene 1, Town & Country, Modern gameplay with Classic
computer controllers, seed 7 for both streams, three 250-unit armies, 1,200
warmup and 300 measured ticks, Darwin/arm64 Go 1.27.1, GOMAXPROCS=2.
Scene/configuration/catalog identities, all seven census rows, initial/warm/
final partial fingerprints and both final draw counts matched exactly
(simulation 15,645; CRT 985,036). Allocation cost was 103,329.7 versus
103,326.1 bytes/tick and 908.41 versus 908.38 objects/tick. Median/p95 were
1.839/3.047 versus 1.722/2.817 ms; process CPU was 2.046 versus 1.873
ms/tick. A concurrent verification gate and differing GC windows make these
single timing samples unsuitable for a speedup claim. Raw reports and census
are outside the repository at `/private/tmp/nanolathe-m3-u5-baseline-classic`
and `/private/tmp/nanolathe-m3-u5-candidate-classic`. This checks disabled
instrumentation for Classic controllers; it does not measure the Modern
executor or complete enabled checkpoint cost, which remain U7 work.

#### 16.3.37 U6 session strip fragment

The session owns the final fragment of section 12, after the event buffer,
effect service, fixed pool and debris (§16.3.17). Its private entry points are:

```go
func (t *stripTable) writeCheckpoint(e *checkpoint.Encoder) error
func (t *stripTable) appendCheckpointSummary(s *checkpoint.Summary) error
```

The caller writes table presence; a nil table passed directly to either
method is an error. Neither method creates simulation objects, invokes a
callback, looks up content or draws RNG. The cheap summary allocates no heap
objects; full encoding may allocate diagnostic field paths. The full payload is `live i64`, `poolCapacity i64`, `steadyCap i64`,
then the fixed ten strip lists with no outer length. Each list writes its
u32 length and objects in insertion order. Each object's lexical fields are
`dst[3] i64`, `dstExtent[3] i64`, `family u8`, `frameCountBase i32`,
`frameDelayParam i32`, `nextSpawn u32`, `particleLife i32`, `particles`
(u32 length and ordered records), `phaseModulus i32`, `smokeSelector u8`,
`spawnInterval i32`, `src[3] i64`, `srcExtent[3] i64`, `windowEnd u32`.
Each particle's fields are `expiry u32`, `frame i32`, `frameDelay i32`,
`lastFrame i32`, `lastFrameDraw i32`, `lastFrameDrawn bool`, `phase i32`,
`vx/vy/vz i64`, `x/y/z i64`, in that order. These are stored values, not
recalculated animation state. Colour-only fields, `reservedWord` and recycled
particle storage remain excluded by §16.3.5.

Validate a nonnegative live count equal to the sum of list lengths,
nonnegative steady capacity, positive pool capacity with live no greater
than it, and the closed family set 1–6 before writing. Per-strip length need
not fit the current steady limit: that limit controls future eviction, and
the writer must not normalize retained lists. Count overflow is an explicit
encoding refusal. The cheap fragment appends the live strip-object count,
then each object in strip/list order: family, nextSpawn, windowEnd, particle
list length, and each particle's x/y/z, expiry, frame, frameDelay, phase.
It does not append extra list lengths or limits beyond that selected schema;
section composition appends the table presence once for both full and cheap
forms. The cheap fragment checks only table/accumulator presence; full-only
capacity, family and list-consistency validation does not run in that walk.
A refused absent input leaves the supplied summary unchanged. Tests use an
independently constructed byte vector, high fixed-point bits,
retained-field mutations, exclusions, order, summary blind spots and purity.

#### 16.3.38 U6 scenario fragment

Section 13 writes the existing computer-manager fragment first, then Mission
presence and its mutable `Defeat`, `Victory` trigger lists, the nonoptional
`communitySchema` record, and Survival presence and payload. Nil trigger rows
are retained with presence. Lists carry u32 lengths, fixed arrays none;
§16.3.5 lexical order and source widths apply, with raw handles u32 and Go
ints i64. The bounded private session API and lower-owner leaves are:

```go
type checkpointScenarioContext struct {
    keys *content.CheckpointKeys
    mission *mission.Mission // exact object retained by admitted composition
}
func (s *Session) writeScenarioCheckpoint(*checkpoint.Encoder, *checkpointScenarioContext) error
func (s *Session) appendScenarioCheckpointSummary(*checkpoint.Summary) error
// internal/triggers
func (t *Trigger) WriteCheckpoint(*checkpoint.Encoder) error
// internal/survival
func (r *Regions) WriteCheckpoint(*checkpoint.Encoder) error
// internal/ai
func (s *SurvivalInfo) WriteCheckpoint(*checkpoint.Encoder) error
```

The session methods live in `checkpoint_scenario.go`; nested helpers remain
private there. Each leaf's caller writes presence. No new global graph table
is needed. Context construction belongs to admitted session composition:
the constructor must retain its previously admitted Mission object, whose
frozen OTA/TNT and selected schema determine immutable inputs. The scalar
fragment cannot manufacture this proof from a terrain name. Reject a session
or community-schema mission pointer different from that exact object;
ordinary campaign/restore entry remains unsupported (§16.3.6). `IsRestore`,
mission type, OTA/schema, placements/specials/initial features, wind bounds,
use-only, difficulty and campaign selection are immutable entry bindings;
`Mission.order` is load diagnostic state. Mutable trigger progress is encoded
separately. Repeated nonnil trigger pointers across either list are refused:
notification can mutate Args, so equal value copies do not preserve aliasing.

Expanded retained records (nested values follow their own lexical order):

| Record | Lexical fields |
|---|---|
| communitySchemaState | active bool; deferredPlacements []i64; mission binding presence; neutralOwner i8; nextDeferred i64; playerByStart [10]i8 |
| Trigger | Args [3]i32; Celebrated bool; CenterReady bool; CenterX/Y/Z i32; Completed bool; Kind u8; Type string |
| survivalState | accts [10]Account; attacker u8; centreX/Z i32; classes ordered records; cleanLost bool; info presence/payload; lastSpawn u32; nextG/P i64; nextRetarget u32; opts; phase u8; phaseEnd u32; plan; pool; removed; settled [10]u32; startClass local u32 reference; startRegion i32; stats [10]SurvivalStats; survived i64; team []u8; tuning; units ordered records; wave i64; wavePoints i64; waveUnits []u32 |
| survivalClass | base i32; key string; profile; regions |
| survivalUnit | h u32; infecting bool; shun u32; shunCellX/Z i32; shunSerial u64; shunUntil u32; target u32; wave i64 |
| movement.Profile | BadSlope, BadWaterSlope u8; FootPrintX/Z i16; MaxSlope u8; MaxWaterDepth i32; MaxWaterSlope u8; MinWaterDepth i32 |
| survival.Regions | H, W i32; label presence then length/values []i32; sizes []i32 |
| survival.Account | Capacity, Earned, Stock F32 |
| SurvivalStats | Damage, Destroyed, Lost i64 |
| survival.Options | NoAir, NoNaval bool |
| survival.Pool | MaxTier, RatioE, RatioM, Tier1Median i64; Units ordered records |
| survival.Unit | Cost i64; Def presence/admitted key; Domain u8; Key string; Tier i64 |
| survival.Wave | Budget i64; Groups ordered records; Number i64 |
| survival.Group | Angle u16; Domain u8; Picks []i64 |
| ai.SurvivalInfo | Attacker u8; CentreX/Z i32; Computer []bool; Starts [][2]i32; Team []u8; warnings pointer presence then ordered records |
| ai.SurvivalWarning | Arrive u32; Groups ordered records; Tick u32; Wave i32 |
| ai.SurvivalApproach | Angle u16; Domain string; X/Z i32 |

Tuning's lexical fields are AirFrom u32, BaseUnits i64, BuddyRing i32,
CleanWaveBonus i64, DirectionEvery u32, Doubling u32, DowntimeBase u32,
DowntimeMax u32, DowntimePerUnit u32, EdgeInset i32, FastClearBonus i64,
FirstWaveDelay u32, MaxDirections i64, NewTierWeight i64, RetargetEvery u32,
SpawnPerTick i64, Straggle u32, ThemeWeights [5]i64, UnlockUnits i64,
WarningTime u32, WaveReward F32. Write stored values without applying defaults.

The account array is one physical [10]Account reused by Metal then Energy
income passes, not two resource arrays; preserve unused residual rows too.
Classes preserve allocation order and must be unique and nonnil. `startClass`
is zero when absent, otherwise the exact one-based class index; reject a
foreign pointer rather than resolving by its key. Regions preserve labels
and sizes without rebuilding against current terrain; nil labels have an
explicit absence tag because region readers distinguish them. `removed`
uses a u32 count followed by numerically sorted handle-u32/health-i32 pairs;
never redirect stale handles to live allocations. Pool definitions resolve
through `keys.Unit(actualDef)`, keeping stored Key separately. No tiers,
prices, plans or groups are reconstructed.

SurvivalInfo loads its atomic warnings pointer once and writes the complete
published list without the filtering/copying `Warnings` getter. The manager
binding validator must require the exact scenario-owned info alias.
`info.Team` remains independent of `team`: commander allocation failures can
leave the former shorter. Setup/report-only deposits and history and rebuilt
walk scratch remain excluded under §16.3.5.

After managers the cheap fragment appends: Mission presence; Defeat length
and each trigger presence, Completed, Celebrated; Victory likewise; Survival
presence; then phase, phaseEnd, wave, nextG, nextP, nextRetarget, survived,
wavePoints; stats[0..9] Damage/Destroyed/Lost; accts[0..9]
Capacity/Earned/Stock exact F32 bits. It performs no map, region, warning,
pool or full-reference walk. Validate selected NaNs before committing a local
summary copy. Unselected tuning NaNs still fail the full writer, not this
selected-only summary. Tests pin stored residuals, order, widths, signed zero,
alias refusals, map insertion independence, exclusions and read-only behavior.

#### 16.3.39 U6 runtime and visibility-tail fragments

These callback-free private session leaves encode stored values after the
parent's mandatory whole-session admission/binding/boundary preflight:

```go
func (s *Session) writeCheckpointRuntime(*checkpoint.Encoder, *content.CheckpointKeys, CheckpointBoundary) error
func (s *Session) appendCheckpointRuntimeSummary(*checkpoint.Summary) error
func (s *Session) writeCheckpointVisibilityTail(*checkpoint.Encoder) error
func (s *Session) appendCheckpointVisibilityTailSummary(*checkpoint.Summary) error
```

They introduce no reference table and make no rule selection. Runtime starts
with boundary u8 (1/2/3), followed by this exact source-lexical record:

```text
CampaignSlot i64; Clock.GlobalTick u32; Community; DefeatDone bool;
EnemyOwner u8; EntryCommunity; Gameplay string; Latch; LocalOwner u8;
Meteor; Mutators; Progress; RNGCrtSeed u32; RNGSimSeed u32; Restrictions;
Rules { Base string; Name string }; Skirmish; State u8; VictoryDone bool;
ViewingOwner u8; Wind presence/payload; battleEntryTailDone bool;
builderOptionsReady bool; campaignPlayerSide [10]i8;
campaignPlayerSideKnown [10]bool; deathmatchActive bool;
deathmatchAttempts u16; deathmatchExhausted bool;
deathsWithNoRecordedCause i64; noShake bool; pendingCommanderDeaths [10]bool;
playerBuilderOptions [10]BuilderOptions; result; resultArmedTick u32;
resultPending bool; resultPendingDraw bool; resultPendingLosers []i64;
resultPendingReason string; resultPendingWinner i64;
rngCrt { State u32; draws u64 }; rngInitialized bool;
rngSim { State u32; draws u64 }; seatCommands.removed [10]bool;
shakeActive bool; shakeAmpX i32; shakeAmpY i32; shakeDuration i32;
shakeOffsetX i32; shakeOffsetY i32; shakeRemaining i32.
```

Community and EntryCommunity reuse `community.Features.WriteCheckpoint`;
Wind reuses its existing writer. Read RNG state directly and use pure Draws,
never initialization-capable SimRNG/CrtRNG getters. Names in Rules identify
bindings already verified by preflight; matching them is not that verification.
Nested lexical records are:

| Record | Fields |
|---|---|
| Latch | Bits u16; Countdown i16; Pending u8 |
| Progress | BetweenMissions i64; Thumbs [25]u8; WL [10]u8 |
| BuilderOptions | Guard [3]u8; Patrol [3]u8 |
| Result | ArmedTick u32; Countdown i16; Draw bool; Ended bool; Losers []i64; Reason string; Tick u32; Winners []i64 |
| Meteor | Active bool; DurationTicks i32; Enabled bool; Initialized bool; IntervalTicks i32; NextHit u32; NextStrike u32; OriginX/Z i32; PerHitDelay i32; Radius i32; StrikeEnds u32; TargetX/Z i32; Weapon presence/admitted definition; WeaponName string |
| Skirmish | CommanderDeath i64; Difficulty i64; Gameplay string; LOSType i64; LineOfSight i64; Location i64; MapName string; Mapping i64; NumPlayers i64; Players [10]SkirmishPlayer; RNGCrtSeed u32; RNGSimSeed u32; Survival options; UnitLimit i64 |
| SkirmishPlayer | AI u8; AllyGroup i64; Controller i64; Energy i64; Metal i64; Side i64 |
| Survival options | Enabled bool; NoAir bool; NoNaval bool; Pace u8 |
| Mutators | AreaOfEffect, BuildCost, BuildSpeed, Damage, FireRate, Health, Income, Radar, Salvage, Sight, UnitSpeed; each Factor has Den u8 then Num u8 |
| Restrictions | u32 entry count, then each Count u8, Unit string in the canonical order returned by Entries |

Keep raw factors, duplicate/ordered result slots and every stored scalar;
normalization and defaults do not belong in capture. Meteor's actual Weapon
pointer resolves through `keys.Weapon`; no name lookup replaces it. Required
leaf refusals are missing session/encoder/keys/Clock, unknown boundary, pending
battle transition, active seat-command dispatch, invalid count/key encoding,
foreign meteor definition and NaN wind. Full preflight additionally establishes
quiescence and all actual bindings; no arbitrary admitted Boolean is accepted.
A terminal final-tick state need not still be Battle.

Section 6 writes the visibility service fragment and then this session tail:
postLoop presence, followed when present by the eyeball-record count and each
record's `cx i32`, `cz i32`, `emitter u8`, `expiry u32`, `heightByte u8`,
`owner u8`, `published bool`, `sightDistance i16`, `x/y/z i64`; then visStamps
count and numerically sorted entries `handle u32`, `cx/cz/radius i32`.
Reject stamp keys outside u32 instead of narrowing them. Nil/empty maps both
encode count zero; eyeballs preserve list order and live length. Never call
tail-allocation or publication convenience helpers during either capture.

Runtime's cheap words are: Clock.GlobalTick, State, rngInitialized,
rngSim.State/draws, rngCrt.State/draws, Latch.Bits/Countdown/Pending,
resultPending, result.Ended, result.Draw, pendingCommanderDeaths[0..9], Wind
presence and its existing summary, Meteor.Active/NextStrike/StrikeEnds/NextHit,
shakeActive/remaining. The tail appends eyeball count then each record's
owner, x/y/z, expiry, published; absent tail contributes zero count, with no
extra postLoop presence. These selected-only methods use a local summary
copy and commit on success (including Wind's NaN check). No map, full graph,
content, callback or heap allocation belongs in a cheap walk.

**Parent-owned binding preflight.** CommunitySources is future-behavior input:
SetRules/BindRules and command validation reread its slices and pointer-valued
overrides. The initial admitted constructor has empty sources in Strict and
otherwise one CommandLine override containing only Base equal to the frozen
admitted table. Validate that exact shape and every value without resolving
sources at capture. Arbitrary or mutated sources fail. Actual RuleSet seams
and Features declarations must likewise have verified provenance; equal
Name/Base strings do not bless changed interfaces, and unknown concrete
interfaces must fail without invoking them or comparing potentially
noncomparable values. This leaf unit does not remove those production refusals.

#### 16.3.40 U6 admitted session receipt

`NewAdmittedSkirmish` now retains a private checkpoint admission receipt after
its existing validation, before composition. The receipt contains the frozen
inputs, resolved configuration, exact admitted Mission pointer and one fresh
BindingAuthority; `skirmishEntry` passes it to the new Session before any
service wiring. Bind its owner to that exact Session before any service wiring;
only successful completed composition marks the receipt ready. Every canonical
installation requires that same owner, so a shallow copy cannot attest closures
capturing itself against the original's shared owners. A shallow copy cannot reuse
another object's receipt. Ordinary constructors carry no receipt and remain
unsupported for whole-session capture.

This is constructor provenance, not a declaration that all bindings have been
verified. It neither enables histories nor changes gameplay. U6 preflight
still must validate immutable mission inputs, actual owner aliases, rule and
callback installations, and the C1 boundary. Capture keys are built only when
diagnostics are enabled. The receipt is private, excluded from the stream,
and offers no getter for its authority or frozen configuration storage.

Entry lifecycle also has a pending implementation constraint: the existing
tick-zero player prime can create a Modern Host before composition returns.
The current manager-history API accepts only an unstarted controller, so it
cannot yet support EnableCheckpoints at that completed entry. U6 must provide
an explicit entry-time history attachment contract without joining a worker,
inspecting its private state, changing prime behavior, or inventing history
for commands already applied before the captured entry baseline.

#### 16.3.41 U6 visibility cheap fragment

`(*visibility.Service).AppendCheckpointSummary(*checkpoint.Summary) error`
appends the service part of section 6 before §16.3.39's session tail: semantic
mode bits (all stored mode bits except ModeFogCacheValid), local player,
team[0..9], viewerDefeated, word-mask length and words, then each of the ten
byte-grid lengths and their row-major bytes. Each scalar contributes one
unsigned summary word. The caller owns service presence; missing service or
accumulator is an error without a partial append. This selected-only walk
reads no footprint map, dimension, sensor cache, rule or reader callback,
allocates nothing and does not refresh visibility. Nil and empty grids both
contribute zero length. Independent literal words lock order and zero extension;
mutations prove selected refcounts and deliberate full-only blind spots.

#### 16.3.42 U6 closed stateless rule tags

The five owner rule interfaces in visibility, movement, orders, construction
and combat expose `CheckpointRulesKind(Rules) (uint8, error)`. AI exposes
`CheckpointPlannerKind(Planner) (uint8, error)`. Each uses an exhaustive type
switch over the existing reviewed stateless implementations, without invoking
methods, comparing arbitrary interfaces, or examining function addresses.
Rule tags are nil 0, StrictRules 1, CommunityRules 2, ModernRules 3; planner
tags are nil 0, RetailPlanner 1, ModernPlanner 2. Accept a value form where it
implements the interface and a nonnil pointer form; refuse typed nil, custom
embeddings, laboratory variants and all other concrete types. No gameplay
registry or policy selection is added.

Replace the existing zero rule/planner byte in the corresponding owner writer
with that classification, removing only its absent-rule refusal. AI's
ConstructionRules uses construction's classifier. Keep every unrelated owner,
callback, content and active-scope refusal. Orders still refuses the entire
QueueBinding and owned handlers until their ports are admitted: adding its
classifier alone does not bless a binding. Nil fixture bytes remain unchanged.
The same existing `path.CheckpointKernelKind` owns path kernel classification.

These leaves prove concrete implementation identity only. Session preflight
must separately require the current reserved Name/Base pair, matching Gameplay,
empty reserved Features declarations and the actual projected rule/Community
values on every owner. Live phase-1 switches can change the current reserved
set independently of the entry selection; validation cannot rebind or normalize
state. The private Modern controller planner installed by the existing mod
registration is not ai.ModernPlanner: it remains unsupported until its own
registration witness and host lifecycle contract is published. Unknown planner
implementations must not be admitted through arbitrary interface methods.

Tests cover every supported value/pointer, nil versus typed nil, hostile or
noncomparable implementations without callbacks, nonzero tag bytes, unchanged
nil payloads and unrelated bindings that still refuse. The stateless tag
addition encodes existing policy, not a new rule or gameplay departure.


#### 16.3.43 U6 completed-entry Modern history attachment

The battle-entry player prime may already have constructed and stepped a
Modern host when `NewAdmittedSkirmish` returns. Recording begins at the
completed entry boundary, not retroactively during that prime. The initial
full checkpoint describes its completed effects. Attaching instrumentation
must not join or read a worker, recreate the host, or reset gameplay state.

```go
// internal/ai; the concrete caller proves the controller implementation.
func EnableControllerCheckpointApplications[T any](m *Manager, expected *T, identity checkpoint.Identity, keys *content.CheckpointKeys) error
// internal/aikit
func (h *Host) EnableCheckpointApplications(checkpoint.Identity, *content.CheckpointKeys) error
```

The manager helper accepts only a nonnil manager, nonnil keys and expected
pointer, ControllerModern, no existing history, and `m.Ext.(*T)` equal to
expected. It performs no arbitrary interface equality or interface callback.
It creates a fresh kind-2 history only after all checks, then installs history
and keys together. The existing manager method keeps its no-Ext refusal.
The helper establishes pointer ownership, not planner/binding admission; the
session must still attest the registered concrete planner and actual host.

Host attachment first checks nonnil host/manager; exact Ext ownership; no
existing host or manager history; no active host application, executor command
scope or nonzero host/executor batch serial; executor manager equals the host's manager
when initialized and is nil before initialization. A pending deadline requires
an initialized host. All refusals precede mutation. It then calls the manager
helper, borrows the installed history, and reserves its first serial only if
`checkpointDeadlinePresent` is true: that already scheduled batch is serial 1,
and next serial is 2. Otherwise its diagnostic batch serial stays zero and
next serial stays 1. Count is zero and the chain retains the initial hash.
The next actual application records ordinary attempts from that pending batch.
Previously completed prime attempts are not fabricated.

Read only the host's simulation-thread ownership, initialization and diagnostic
scope/deadline fields. Do not inspect `b`, `flight`, `ready`, brain, private RNG,
observation or worker results. Do not invoke Step, begin, join, normalize or any
producer. The executor reads the manager's history for future applications;
there is no second chain to attach. Existing pre-start attachment remains valid.

Tests cover unstarted, started without pending work, and pending hosts, fresh
hash/count/serial values, mismatched or typed-nil Ext, repeated and active-scope
refusals with atomic state, and a genuinely blocked worker. The blocked-worker
case must attach and read its cheap controller summary before release, then
apply its pending batch normally with unchanged world/RNG outcomes. No public
session lifecycle or unknown controller admission is enabled by this unit.


#### 16.3.44 U6 Modern planner registration witness

Extend the existing single Modern AI registration slot; do not add a registry
or let the lobby select another controller capability. The reviewed mods/aikit
init uses `RegisterModernAIWithCheckpointBinding[T ai.ModernAIStep](step T)`.
Ordinary `RegisterModernAI` continues to register gameplay identically but
supplies no checkpoint witness, so arbitrary registrations remain unsupported
for full capture. Both paths use the same duplicate/nil/zero-size checks.

The attested path creates a private, generated type witness, never a callback
supplied by the caller. At registration only, require T to be the actual
concrete dynamic type of step, a zero-size struct. This rejects an interface
instantiation whose assertion would accept other implementations, as well as
pointer or stateful types. Its witness is only the exact `planner.(T)` type
assertion and does not call planner methods, compare arbitrary interfaces or
use reflection during capture. A different zero-size embedding is a different
type and fails the witness. Registration is trusted composition code reviewed
alongside its planner implementation; it is not evidence supplied by a lobby.

Resolve step and witness together under the existing registration mutex once
per session, before a Modern player starts. Keep the witness private beside
Session.modernAI; ordinary registration leaves it nil. The private
`checkpointModernPlannerMatches(ai.Planner) bool` requires a nonnil session
witness and matches both the retained session planner and supplied owner
planner. It is a read-only concrete-implementation check, without global
lookup, locking, callbacks or controller/worker reads at capture. The whole
preflight still validates exact manager/host ownership and executor bindings.
Tests pin ordinary-registration refusal, exact-type acceptance, interface-type
instantiation refusal, hostile/noncomparable and typed-nil replacements, and
unchanged duplicate-registration behavior without invoking any think method.


#### 16.3.45 U6 reserved rule and feature projection preflight

The private session leaf `validateCheckpointRuleBindings(checkpointRuleContext)
error` verifies the selected reserved set and the actual owner projections.
The context contains `entryMode gameplay.Mode`, `entryCommunity
community.Features`, and `defaultCommunity community.Features`. Whole-session
admission constructs it from the private entry receipt and the mainline table
frozen at entry; callers' public names or current fields are not provenance.
The leaf does not establish the rest of the receipt/owner graph itself.

Require nonnil Session; current Rules.Name exactly equal to its reserved Base
word and Gameplay; empty Rules.Features; and this closed matrix, using the
existing owner classifiers without gameplay calls:

| Seam | Strict 3.1 | Community 3.9 | Modern |
|---|---|---|---|
| Visibility, movement, orders, construction, combat | StrictRules | CommunityRules | ModernRules |
| Path | RetailKernel | RetailKernel | SmoothKernel |
| RuleSet Planner | RetailPlanner | RetailPlanner | ModernPlanner |
| UnitLimit | StrictUnitLimit | ModernUnitLimit | ModernUnitLimit |
| ScriptPorts | StrictScriptPorts | CommunityScriptPorts | ModernScriptPorts |
| ComputerIncome | StrictComputerIncome | CommunityComputerIncome | ModernComputerIncome |
| Seats | StrictSeats | ModernSeats | ModernSeats |

Accept the supported concrete value or nonnil pointer form, reject nil
(including Path's otherwise equivalent nil fallback), typed nil, custom
embeddings and laboratory implementations. The four session-owned interfaces
use direct type switches with the same discipline as §.42. Never look up a
rule set, build one, call its methods, resolve features, or rebind owners.

Validate retained CommunitySources against entry mode, not current mode:
Content and Player are empty; Strict entry has no CommandLine sources; other
entry has exactly one override containing only Base, whose pointed value is
entryCommunity. EntryCommunity equals that entry value (Strict must be zero).
The full comparable Overrides value may be compared with `{Base: actual.Base}`
to detect other fields; do not invoke arbitrary callbacks or interfaces.
Current Community is zero in Strict; outside Strict it is defaultCommunity
for a Strict entry with empty sources, otherwise entryCommunity. This allows
normal phase-1 switches without erasing their retained source semantics.

For each present owner, verify its actual concrete rule kind and the exact
stored projection built by `projectCommunity`: visibility's two scalar feature
fields; combat's complete projected Features; construction's projected Features;
movement's Rules, Kernel and GridClaimTieBreak-only Features; economy's
AIDifficultyIncome-only Features; shared Build.OrderBinding's Rules and
orderCommunity Features; each manager's ConstructionRules and aiCommunity.
Classic managers use the current set's planner; Modern managers require
`checkpointModernPlannerMatches` for their actual planner. Unknown Controller
values refuse. Do not call controller marker methods or read Ext/workers.
Nil services/rows are skipped only by this leaf; whole-session admission owns
required presence, exact singleton aliases and all callback proof. No callback
refusal in an owner writer is relaxed by this check.

Tests exercise all three entry modes across all three current reserved modes,
source-value and projection mutations, same-name custom seams, typed nil and
noncomparable hostile interfaces without method calls, Modern witness checks,
and purity. Construct expected projections independently from the documented
field lists; do not call projection helpers during validation or in expected
values. Report a new projected field as a contract change when it is added.


#### 16.3.46 U6 unit lifecycle binding ownership

Encapsulate the four World lifecycle callbacks without changing any call or
installation position. `DeathHook`, `DeathExtraHook`, `CreateHook` and
`CaptureHook` return their current callback; `SetDeathHook`,
`SetDeathExtraHook`, `SetCreateHook` and `SetCaptureHook` install an ordinary
callback and clear only that slot's checkpoint proof. Each setter also has a
`WithCheckpointBinding(callback, *checkpoint.BindingAuthority)` form used only
at reviewed session composition sites. Its private proof names the exact
World, slot and authority. A copied World still names the original World and
cannot claim the copy's ownership. Getting and reinstalling a function does
not transfer its proof, including when the function itself is unchanged.

`(*units.CheckpointContext).SetLifecycleBindings(*World,
*checkpoint.BindingAuthority) error` registers the expected world and private
admission authority for this capture; it does not mutate or bless a binding.
Reject missing/conflicting registrations. At the existing lexical four binding
bytes encode absent 0 or attested present 1. Before discovery/writing, every
present slot must carry its own exact world/authority proof. Unknown callbacks
still refuse without invocation. Keep every unrelated units/COB/owner refusal.

Session uses its private admission authority at the existing canonical primary
create/death/capture installations. The additional death wrapper keeps existing
gameplay chaining, but is attested only if its captured predecessor is absent.
A nonnil predecessor, even another canonical wrapper, is not the reviewed
initial composition and remains unsupported. Ordinary constructors and headless
observation wrappers keep their gameplay and receive no proof. The recorder
must reject an unreviewed wrapper, rather than silently assume it is harmless.

Tests pin independent slot replacement, unchanged-function reinstall refusal,
wrong world and authority, copied-world refusal, absence/presence bytes and
unchanged callback order/deduplication through the existing lifecycle tests.
Proof is local metadata excluded from all wire state and RNG behavior.


#### 16.3.47 U6 copied COB port installation proof

The VM already stores engine-port callbacks privately. Keep ordinary APIs and
all invocation/fallback order unchanged; add reviewed installation forms:

```go
func (v *VM) BindPortWithCheckpointBinding(Port, func([]int32) int32, checkpoint.Allocation, *checkpoint.BindingAuthority)
func (v *VM) BindPortBindingWithCheckpointBinding(Port, PortBinding, checkpoint.Allocation, *checkpoint.BindingAuthority)
func (c *CheckpointContext) SetOwnerBindings(*VM, checkpoint.Allocation, *checkpoint.BindingAuthority) error
```

The two installation methods perform the existing installation once, then
privately stamp that copied map entry with the exact VM, nonzero owning unit
allocation and authority. Ordinary BindPort/BindPortBinding clear only the
corresponding port's proof, even for a same-function reinstall. The two maps
have independent proof slots because explicit bindings can fall back to legacy
functions per read/write arm. Nil-only entries retain no proof and keep their
existing absent behavior. Missing authority or zero allocation cannot attest.
No callback is invoked by these methods beyond existing installation work
(which currently calls none). Do not change BindingRequest or its callers.

The context setter registers a nonnil expected VM, nonzero allocation and
nonnil authority. Missing/conflicting registration refuses without mutation;
repeating the same registration is allowed. It never stamps the live VM.
Capture requires each nonnil actual port entry to match its own exact VM,
allocation and authority; a copied VM cannot borrow the original's stamps.
Capture reads installed entries, never request maps or their later contents.
A context explicitly registered for another VM refuses even when ports are
absent. Parent admission owns the actual unit/VM/program graph and constructs
this context from that verified allocation; a caller's declaration alone is
not whole-session proof. All non-port callback refusals remain in place.

At the existing lexical `VM.bindings.portBindings` slot, encode absent byte 0
if no Read/Write is nonnil. Otherwise encode 1, u32 active-row count, and rows
in numeric Port order, each `port i64; readPresent bool; writePresent bool`.
The independent `portFuncs` slot similarly encodes 0 or 1 plus u32 active-row
count and each numeric `port i64` (all included rows have nonnil functions).
Omit nil-only rows; preserve negative and full-width port keys without narrowing.
Keep every other binding slot and all nil-fixture bytes unchanged. These rows
encode which actual entry shadows which fallback; one generic presence byte
would lose that distinction. Gather keys only, sort, then read values. Never
call readPort/writePort or any arbitrary callback to verify the installed row.

Tests cover independent explicit/legacy slots and read/write presence; nil rows
and unchanged nil fixture bytes; numeric map insertion independence; private
copy proof versus mutation of the input PortBinding; ordinary replacements;
wrong authority/allocation/VM, copied VM and missing/conflicting context;
unknown unrelated callbacks still refused; source-width literal vectors and
read-only capture. Report map-function audit changes to the parent. This unit
adds no gameplay behavior and does not admit the other VM bindings.

#### 16.3.48 U6 economy callback and wind ownership

Economy's three existing callbacks become private fields without changing their
installation or invocation order. Its ordinary getters and setters are:

```go
func (s *Service) CloakCostHook() func(*units.Unit) float32
func (s *Service) CloakDueHook() func(*units.Unit) bool
func (s *Service) EndConditionHook() func(int, uint32)
func (s *Service) SetCloakCost(func(*units.Unit) float32)
func (s *Service) SetCloakDue(func(*units.Unit) bool)
func (s *Service) SetEndCondition(func(int, uint32))
```

Each setter has a `WithCheckpointBinding` sibling taking the same callback
and `*checkpoint.BindingAuthority`. An ordinary installation clears only its
slot's private proof; an admitted installation performs that same installation
then stamps the exact Service and nonnil authority when the callback is nonnil.
A getter transfers no proof. Reinstalling the same function ordinarily loses
proof, and a copied Service cannot reuse the original owner's proof. No
callback is invoked or compared for identity.

```go
func (c *CheckpointContext) SetBindings(*Service, *world.Wind, *checkpoint.BindingAuthority) error
```

The context setter requires a nonnil context, Service and authority, records
that exact service, expected wind pointer (which may be nil) and authority,
and rejects conflicting registration atomically. Identical registration is
idempotent. Both collection and full writing check actual service identity,
each present callback's own proof, and actual Wind pointer equality. An
unregistered fixture with no callbacks or wind remains valid. Preserve the
existing Terrain equality against the world's context. A registered context
for a different Service refuses even when callbacks and wind are absent.

The canonical CloakCost closure reads only its unit argument. CloakDue captures
the exact Session and reads its live clock and the argument's flags/deadline.
EndCondition captures the exact Session and reads its current mission, player,
ledger, countdown and result state. Session installs the first two at their
existing composition sites and EndCondition lazily, only when absent, at its
existing tickPlayers site. Its authority helper requires exact receipt
ownership (§16.3.40); whole-session capture must also verify that owner graph.
Wind remains a public pointer because its actual target is checked directly.
Economy's synchronous call to ApplyCloakDebits does not retain another copy.

At the existing lexical CloakCost, CloakDue, EndCondition and Wind slots, write
validated presence booleans instead of absent-only zero bytes. Every other
field, nil-fixture byte and selected-summary contract remains unchanged.
Tests cover each slot independently, same-function reinstall, foreign
proof, copied Service, wrong or equal-valued wind pointers, context conflicts,
exact byte changes, read-only capture, and unchanged settlement ordering. This
unit adds no gameplay rule and does not itself admit a complete Session.

#### 16.3.49 U6 visibility reader and terrain ownership

The CommunityState projection retains two private readers, `allied` and
`offMap`, with ordinary methods:

```go
func (c *CommunityState) AlliedReader() func(PlayerID, PlayerID) bool
func (c *CommunityState) OffMapReader() func(uint16) bool
func (c *CommunityState) SetAllied(func(PlayerID, PlayerID) bool)
func (c *CommunityState) SetOffMap(func(uint16) bool)
func (s *Service) SetAlliedWithCheckpointBinding(func(PlayerID, PlayerID) bool, *checkpoint.BindingAuthority)
func (s *Service) SetOffMapWithCheckpointBinding(func(uint16) bool, *checkpoint.BindingAuthority)
func (c *CheckpointContext) SetBindings(*Service, *world.Terrain, *checkpoint.BindingAuthority) error
```

Each reader has private proof inside CommunityState, retaining its exact
owning Service and authority. Ordinary setters clear only their reader's proof;
getters transfer no proof. The Service installation forms run the ordinary
setter then stamp nonnil callbacks with nonnil authority. Copying the projection
retains the proof of the installed callback, but it is valid only on that same
Service; a copied Service or projection installed on another Service refuses.
Scalar feature edits do not disturb reader proof; the session's reserved rule
preflight verifies those scalars separately. Capture neither invokes readers
nor compares function addresses.

The context registration requires a nonnil context, Service and authority,
retains expected terrain (possibly nil), and rejects any conflicting tuple
without mutation; the same tuple is idempotent. Both full collection and writing
validate exact registered service and terrain identities and every present
reader's proof. A registered foreign service refuses even with absent readers.
Unregistered fixtures with absent readers preserve their current terrain
presence encoding; whole-session admission always registers actual terrain.
Immutable ray-table and sight-shape identity checks remain unchanged.

At the existing Community.Allied and Community.OffMap lexical positions, encode
validated presence booleans; preserve every other field and absent fixture byte.
A disabled reader still needs proof because later rule changes can consume it.
Session's projectCommunity installs admitted readers only at its existing
only-if-absent sites: the alliance method and off-map method each capture the
exact Session and read its current owners. They must receive the exact-owner
checked authority (§16.3.40), while full capture verifies the Session graph.
No gameplay, rule projection, read order or selected-summary behavior changes.

Tests cover independent slot proof, ordinary same-function reinstall, wrong
proof, copied projections/services, same-owner projection restore, context
conflicts, wrong/equal-valued terrain pointers, unchanged absent vectors,
exact presence-byte changes, and capture without callback invocation.

#### 16.3.50 U6 feature callback, RNG and wind ownership

Features retains six private callbacks with ordinary `NameHook()` getters and
`SetName(fn)` setters, where Name is SequenceFrames, BurnWeapon, BurnSound,
GeothermalSteam, BurnFrameGeometry or BurnSmoke. Their function signatures
stay unchanged. Each also has a `SetNameWithCheckpointBinding(fn, authority)`
method, where authority is `*checkpoint.BindingAuthority`. Private per-slot
proof retains the exact Service and authority. Ordinary installations clear
only their slot; canonical installation performs the same ordinary install and
stamps only a nonnil callback with nonnil authority. Getters transfer no proof;
copied Services and ordinary same-function reinstalls cannot borrow proof.

```go
func (c *CheckpointContext) SetBindings(*Service, *rng.Simulation, *rng.CRT, *world.Wind, *checkpoint.BindingAuthority) error
```

Context registration requires nonnil context, Service and authority, retaining
the three expected owner pointers, which may individually be nil. Repeated
identical registration is allowed; conflicting registration fails atomically.
Both collection and writing require actual Service and RNG/wind identities to
match and each present callback to retain its matching proof. A registered
foreign Service refuses even with absent callbacks. Unregistered fixtures with
all six callbacks and three owners absent preserve their previous behavior.
The existing exact Terrain context check, active-traversal/burn-handoff refusal,
content-definition/sequence identities, arena and cache validation remain.
No callback or RNG accessor is invoked by capture. The ordinary presence slots
for the six callbacks, Crt, Sim and Wind become validated booleans at their
existing lexical positions; every absent fixture byte is unchanged.

Canonical callbacks all capture Session, not a snapshot of its owners.
BurnSmoke and GeothermalSteam append the existing strip records; BurnWeapon
reads the current catalog, combat, units, terrain and clock; BurnSound publishes
the existing positional cue. SequenceFrames reads current immutable simArt,
while BurnFrameGeometry reads the current private featureSequence resolver.
Whole-session admission must verify those current owners and that resolver's
installation against the frozen art; the features token alone is insufficient.
ShadowSequenceResolved remains presentation-only under the reviewed inventory.
Its field and behavior do not change in this unit.

Root session wiring preserves every current installation site and only-if-nil
guard, using the exact-owner authority helper. Restore's temporary sound wrapper
uses ordinary setters and invalidates its proof, consistent with currently
unsupported capture on restored sessions. Feature cursor delay metadata remains
validated against admitted sequence keys; no capture operation fills a cache.
Tests cover each independent slot, copied Service, same-function ordinary
reinstall, foreign proof, context conflicts, actual RNG/wind/terrain aliases,
exact presence bytes, zero callback/draw effects, and the existing burn,
reproduction and sequence timing tests. No gameplay behavior changes.

#### 16.3.51 U6 provisional COB port installation receipts

Production unit creation installs its script ports before COB Create runs, but
commits the allocation serial only after that fallible bind succeeds. Both
ordinary and forced allocation do so before extraction, the mover tail,
activation and the creation hook. Do not predict the next serial or move its
commit: nested creation and failed binds must retain their current behavior.
The §16.3.47 immediate installation API remains available for known allocations;
pre-create installation adds:

```go
func (v *VM) BindPortWithPendingCheckpointBinding(Port, func([]int32) int32, *checkpoint.BindingAuthority) *CheckpointPortInstallation
func (v *VM) BindPortBindingWithPendingCheckpointBinding(Port, PortBinding, *checkpoint.BindingAuthority) *CheckpointPortInstallation
func (r *CheckpointPortInstallation) Seal(checkpoint.Allocation) error
```

A receipt has only private fields. Each installation runs the ordinary setter
once, then, for a present callback and authority, creates proof and a receipt
for that exact VM, map family, key and copied installation. A nil-only callback
or missing authority produces no receipt and no proof. Ordinary API nil-receiver
behavior is preserved. Pending proof never admits a full capture.

Seal requires a nonnil receipt, nonzero handle and serial, and the receipt's
original proof still installed in the named map entry of the original VM.
A replacement, including another canonical installation of the same callback,
invalidates the old receipt. Sealing affects only its own private proof; it
never blesses whichever callback now occupies the key. The first successful
seal records the actual allocation, the same repeated seal is idempotent, and
a different allocation refuses without mutation. No callback runs, no map row
is reconstructed, and refusal changes no VM or receipt state. It is diagnostic
bookkeeping, not a new gameplay operation or wire field; sealed rows use the
same §16.3.47 encoding and existing exact VM/allocation/authority validation.

The units owner will privately retain receipts alongside the exact *Unit during
pre-create composition and seal that unit's receipts immediately after the
existing successful allocation serial assignment in each allocator path.
Failed creation never seals; nested creations retain their own receipt lists.
A diagnostic proof refusal cannot change creation success or trigger additional
gameplay work; that installation remains unsupported at capture. Ordinary
fixtures and legacy asset-binding entry points remain unproved.

Tests pin refusal before sealing, final-identity matching, invalid/repeated/
conflicting seals, independent map slots, stale receipts after ordinary or
canonical replacement, nil-only installs, copied VM refusal, reverse completion
order for nested-created owners, unchanged encoded bytes after sealing and
zero callback/RNG effects. This closes installation timing only; the remaining
VM bindings and the actual unit/program owner graph still require admission.

#### 16.3.52 U6 closed frozen-filesystem identity

Whole-session admission must not call an arbitrary filesystem's
`SimulationInputs()` method to prove its source. The content owner supplies:

```go
func (in *SimulationInputs) CheckpointFilesystemMatches(vfs.FSOps) bool
```

This is a pure closed-type check for the exact sealed view retained by those
inputs. It accepts only the four private snapshot-view forms returned by the
freezer (plain, range, ordered, range-and-ordered), with nonnil wrapper/base,
non-transient base, the exact nonnil snapshot retained by the inputs’ sources,
and that exact inputs pointer. The actual
view and retained view must be the same concrete pointer, checked only after
concrete narrowing; neither interface is compared as an arbitrary interface.
Nil inputs/view, typed nil, a different snapshot, a copy or foreign wrapper,
transient capture views and user-defined implementations refuse. No filesystem
or open-interface method runs and no bytes, diagnostics or caches change.
The result emits no wire data. It proves only source ownership; full admission
still validates immutable identities, cached fallback inputs and owner aliases.

Tests cover all four view variants, typed nil and noncomparable hostile wrappers,
copied views, distinct frozen inputs, transient/unbound capture views, and pure
zero-allocation successful checks. Existing freezing and ordinary composition
behavior is unchanged; the current open helper remains a composition facility,
never capture-time proof.

#### 16.3.53 U6 session art resolver provenance

Session's private effect-frame and feature-sequence resolvers are admitted only
when both are present and installed from the exact nonnil frozen SimArt object.
Private installation helpers call each existing public setter once at its
current composition site and retain the exact Session and art pointer beside
that resolver. Ordinary public setters clear that resolver's proof before their
existing work, including the effect setter's smoke-frame resolution. They do
not invalidate the other resolver. Getters, copied Session fields, equal-valued
art and same-function ordinary reinstall cannot transfer ownership.

Capture preflight checks current simArt against the admitted art and both
resolver proofs against that exact Session/art. It calls no resolver, fills no
cache and changes no strip or feature state. Missing art, missing resolvers or
custom replacements refuse; canonical admitted entry always binds both. These
proof fields are diagnostic metadata and add no wire bytes. Tests preserve
existing setter behavior, reject copied/foreign/cleared/reinstalled resolvers,
and require pure, allocation-free repeated validation.

#### 16.3.54 U6 per-unit yard and status callback ownership

The two private unit callback slots retain independent exact-*Unit/authority
proof alongside their installation:

```go
func (u *Unit) SetYardOpenTransactionWithCheckpointBinding(YardOpenTransaction, *checkpoint.BindingAuthority)
func (u *Unit) SetStatusCueSinkWithCheckpointBinding(StatusCueSink, *checkpoint.BindingAuthority)
```

Each new method performs the existing ordinary setter once, then stamps a
nonnil callback and authority on a nonnil Unit. Ordinary setters clear only their
own proof. Preserve their nil-receiver no-op behavior and invoke no callback.
The existing `CheckpointContext.SetLifecycleBindings(world, authority)` supplies
the expected owning world and authority for unit callback validation as well.
Both collection and writing require that exact world registration and each
present callback's exact unit and authority proof. A copied Unit, foreign
world/context or ordinary same-function reinstall refuses. Missing callbacks
preserve existing unregistered authored fixtures.

The yard callback is installed before allocation commits its serial; proof
therefore retains the exact Unit pointer, not a guessed allocation serial. The
existing allocation collector still validates the actual committed handle and
serial before writing its record. Retired allocations keep their own callback
proof and follow existing graph discovery; they need not occupy a live slot.
Change only the two existing statusCue/yardTransaction bytes to validated
presence booleans. Other World, unit and VM bindings keep their current checks.
Root wires these methods at the existing session pre-Create yard installer,
status-sink sweep and creation hook with the exact-owner session authority.

Tests cover both slots independently, pre-serial installation followed by
successful allocation capture, copied units, foreign authority/world, nil
inputs, ordinary replacement, unchanged nil bytes and exact presence changes,
retired allocation references, and no callback or RNG activity during capture.
This metadata does not alter allocation, yard requests or status-cue behavior.

#### 16.3.55 U6 COB runtime binding admission

The VM's remaining runtime bindings are admitted as actual installations, not
by invoking a reader or sink. The existing twelve lexical binding slots stay
in place. Ports retain §16.3.47. The other slots use validated presence bytes,
except renderFlags, whose present byte is followed by kind 1 (direct external
slice) or 2 (mapped handlers). The pieceFlags source is 0 with no duplicate
bytes for a validated external unit store; unbound local storage keeps source
1 and its existing bytes. No proof, pointer or scratch buffer is wire data.

```go
// internal/cob; each receipt has private fields.
type CheckpointVMInstallation struct { /* private */ }
func (*CheckpointVMInstallation) Seal(checkpoint.Allocation) error
func (*VM) BindScriptTouchedWithPendingCheckpointBinding(func(), *checkpoint.BindingAuthority) *CheckpointVMInstallation
func (*VM) BindTransportQueriesWithPendingCheckpointBinding(func(int32) bool, func() int32, *checkpoint.BindingAuthority) [2]*CheckpointVMInstallation
func (*VM) BindTransportMutationsWithPendingCheckpointBinding(func(int32, int32, int32), func(int32), *checkpoint.BindingAuthority) [2]*CheckpointVMInstallation
func (*VM) SetSFXVisibleWithPendingCheckpointBinding(func(int, int32) bool, *checkpoint.BindingAuthority) *CheckpointVMInstallation
func (*VM) BindRenderFlagsWithPendingCheckpointBinding([]uint8, *checkpoint.BindingAuthority) *CheckpointVMInstallation
func (*VM) BindRenderFlagHandlersWithPendingCheckpointBinding(func() []uint8, func(int, uint8, bool) bool, []uint8, []int, *checkpoint.BindingAuthority) *CheckpointVMInstallation
func (*Binding) SFXVisibleReader() func(int, int32) bool
func BindStrictWithCheckpointBinding(vfs.FSOps, BindingRequest, *checkpoint.BindingAuthority) (*Binding, *CheckpointVMInstallation, error)
func (*Binding) SetSFXSinkWithPendingCheckpointBinding(SFXSink, func(int, int32) bool, *checkpoint.BindingAuthority) *CheckpointVMInstallation
func (*CheckpointContext) SetRuntimeSources(*Binding, *rng.Simulation, []uint8, []int) error
func SetCheckpointPresentationSink[T any, P interface { *T; PresentationSink }](*CheckpointContext, P) error
func SetCheckpointExplosionSink[T any, P interface { *T; ExplosionSink }](*CheckpointContext, P) error
func (*VM) ExplosionSink() ExplosionSink
func (*VM) ValidateCheckpointBindings(*CheckpointContext) error
```

Receipt lifecycle follows §16.3.51: exact original VM/slot/installation,
nonzero final allocation, idempotent same seal, atomic refusal on replacement,
copy or conflicting seal. Ordinary setters clear only affected proofs; paired
transport slots remain independent. Render proof is cleared by either render
setter, UnbindRenderFlags and both program-reset branches. Installation keeps
all existing work and nil-receiver behavior. It never calls a callback beyond
work the ordinary API already performs. Pending proof refuses capture.

Mapped render proof retains the exact flag and piece-map slices captured beside
the canonical closures in units.bindRenderFlags. Both handlers must be present,
renderFlagsBound true and direct storage nil. Capture compares their lengths
and nonempty backing starts with the actual unit flags and Binding.PieceMap;
empty slices have no readable storage to distinguish. Values in the current
piece map separately must match admitted program/model links. Direct storage
must alias the expected unit flag slice. Explicitly bound nil/empty storage
remains distinct from local fallback through the binding/source tags. Mixed
handler forms and the fallback loader's different live-field closures remain
unsupported. Capture never calls the getter; its scratch is excluded because
that getter overwrites every returned element before use.

Binding's visibility callback becomes private. BindingRequest remains a copied
installation input. The admitted constructor stamps the copied Binding and VM
reader before PreCreate, never after a callback could replace it. Ordinary
BindStrict remains unproved. Ordinary SetSFXSink clears the Binding/VM reader
proof; its admitted sibling records both exact installations on one receipt.
A later independent VM reader replacement must refuse even if the Binding
retains its original proof. Public RNG and sink fields remain subject to
actual structural validation, not installation claims.

Runtime-source registration requires the existing exact VM/allocation/authority
registration, checks Binding.VM when present, and records expected Binding,
RNG and external slices. Conflicting repeats refuse atomically. Both actual VM
and retained Binding RNG must equal the expected pointer. Sink registration
uses only nonnil concrete pointers through the closed generic functions; any
stored matcher is their private type-assertion/equality implementation, never
an application-supplied callback or interface method. Recheck actual sinks at
capture so replacement after registration is detected. VM SFX accepts only the
value PresentationSinkAdapter around the registered presentation pointer;
Binding.PresentationSink must match and Binding.SFXSink may be nil or that same
adapter. ExplosionSink must be the registered concrete pointer. Absent sinks
with no corresponding registration preserve fixtures; typed nil, custom,
noncomparable and partial mismatches refuse without calls.

Root verifies the session-owned sink before registration: exact Session,
publication and clock; source handle equal to the associated unit; copied sink
piece-map values equal to admitted links; explosion sink pointing at that same
presentation sink. These are simulation-relevant sinks: their current session
implementation appends authoritative strip records and performs synchronous
bounded-arena admission. Their names do not justify a presentation exclusion.
The unit owner writes Binding's existing model key followed by presence bytes
in the existing PresentationSink, SFXSink, SFXVisible, SimulationRNG order, only
after ValidateCheckpointBindings succeeds. Existing program, model, bridge,
continuation, allocation and cheap-summary contracts remain unchanged.

Tests pin stale/pending/copied proof, independent paired slots, program reset,
nonaliasing equal-valued render storage, changed map storage, direct empty-store
versus fallback vectors, Binding/VM reader mismatch, wrong RNG, hostile sinks,
exact lexical tags and purity. The root migrates units/session call sites and
retains/seals receipts at the existing successful allocator boundaries; this
leaf never changes allocation success or starts an extra script callback.

#### 16.3.56 U6 compiled-program future-allocation subset

The initial production checkpoint admission supports catalogs whose every
admitted unit record has a compiled, nonempty script. This is an explicit
subset, not a claim that absent/fallback scripts cannot occur in authored
content. A read-only diagnostic on 2026-10-07 exercised ordinary frozen entry
on the reference install, Ashap Plateau, seeds 7/7, no mutators/restrictions,
for all three reserved modes in both skirmish and Survival. Each had 278
nonnil records, no missing/empty script and no ProgramForUnit mismatch. Capture
must prove the condition every time; this census is not a substitute.

```go
// internal/content
func (k *CheckpointKeys) ValidateCompiledAllocationPrograms(*Catalog) error
```

Walk all nonnil current catalog records in stored order, including duplicate
names and definitions never allocated. Require precisely the admitted record
set, manifest ordinals and names, plus each stored UnitDefID exactly as admitted
(without assuming it equals the manifest ordinal); each admitted COB entry must be SimulationInputPresent.
Require each current Script pointer to be nonnil, its Code nonempty, and its
complete semantic digest to match that record's admitted program. Do not load
files, normalize the catalog, populate caches, or change ProgramForUnit's
existing general fallback behavior. CheckpointKeys construction checks every nonnil admitted catalog/COB record
against its manifest ordinal before retaining metadata. It snapshots current
holes and total slice length for later comparison; the manifest does not prove
the history of earlier trailing nil slots. Exact
Catalog pointer ownership is checked separately by the units context.

This condition makes both loader branches unreachable: hasLoadableCOB returns
on the nonempty compiled program, and the reviewed production binder consumes
that same program. The catalog is finalized and bounds future allocations.
Existing cache entries, including successful programs, cached nil misses and
unrelated later populations, are therefore irrelevant in this subset. Never
inspect, clear, rebuild or serialize that cache during capture. Missing, empty,
fallback or later-mutated programs refuse. The freezer currently discards the
separate LoadFromFS probe outcomes, whose name/path rules differ from the strict
binder's manifest. Supporting those outcomes later needs its own frozen-input
contract; equality with the strict binder's program is not sufficient evidence.

The following units ownership API is staged after that content helper:

```go
func (*CheckpointContext) SetWorldBindings(*World, *content.SimulationInputs, *world.Terrain, *rng.Simulation, *cob.CachedLoader, *checkpoint.BindingAuthority) error
func (*World) SetAttachmentObserverWithCheckpointBinding(AttachmentObserver, *checkpoint.BindingAuthority)
func (*World) SetCOBSourceWithCheckpointBinding(vfs.FSOps, *cob.CachedLoader, *checkpoint.BindingAuthority)
func (*World) SetCOBBinderWithCheckpointBinding(COBBinder, vfs.FSOps, *checkpoint.BindingAuthority)
func (*World) SetExtractionSamplerWithCheckpointBinding(ExtractionSampler, *checkpoint.BindingAuthority)
func (*World) SetCreationPoseWithCheckpointBinding(CreationPose, *checkpoint.BindingAuthority)
func (*World) SetSimulationRNGWithCheckpointBinding(*rng.Simulation, *checkpoint.BindingAuthority)
func (*World) AttachmentObserver() AttachmentObserver
func (*World) CreationPose() CreationPose
```

The binder's extra filesystem is its actual captured source; its canonical
model resolver must be built from that same source at the reviewed session
site. Each private installation slot retains exact World/authority proof.
Ordinary setters clear only their affected proofs, even for the same value;
SetCOBSource clears source and loader proof. Preserve existing callback calls,
nil-receiver behavior and installation timing. Getters transfer no proof.

Context registration is atomic, idempotent for the same tuple and consistent
with SetLifecycleBindings. It requires present context/world/inputs/authority;
expected terrain, RNG and loader are explicit pointers. At collection and
writing, verify exact catalog identity with inputs.Catalog(), the compiled
program condition, exact current loader/RNG, and both current source and
binder-captured source through CheckpointFilesystemMatches. Extraction must
be a nonnil concrete *world.Terrain matching the expected terrain when present.
Every present private slot needs its own exact proof. Root additionally checks
AttachmentObserver and CreationPose by closed *movement.System type assertion
against the session's actual movement owner, never arbitrary interface equality.
A registered foreign World refuses even when every slot is absent. Unregistered
all-absent authored fixtures retain existing behavior; whole-session admission
requires the complete production graph, including its attested binder.

Only the existing seven binding bytes become validated presence tags; no
loader cache payload or new table is introduced. Tests cover all future records,
never-created mutations, missing/empty/fallback programs, record order and name
changes, independent proof invalidation, copied worlds, foreign registrations,
exact source/loader/RNG/terrain aliases, poison caches left untouched and pure
repeated capture. This adds no gameplay behavior or alternate allocation path.

#### 16.3.57 U6 terrain movement installation ownership

Both terrain movement ports become private, retaining ordinary getters and
setters. A single immutable installation receipt covers their exact terrain,
authority and captured restamp owner. Either ordinary setter invalidates that
receipt, including a same-value reinstall. Getters transfer no proof.

```go
// internal/world
type FootprintRestampOwner interface {
    NoteFeatureFootprint(int32, int32, int16, int16)
}
func (*Terrain) ClassRestamp() FootprintRestamp
func (*Terrain) SetClassRestamp(FootprintRestamp)
func (*Terrain) Movers() MobileOccupancy
func (*Terrain) SetMovers(MobileOccupancy)
func (*Terrain) SetClassRestampOwnerWithCheckpointBinding(FootprintRestampOwner, *checkpoint.BindingAuthority)
func (*Terrain) CheckpointMovementOwner(*checkpoint.BindingAuthority) (FootprintRestampOwner, bool)
func (*CheckpointContext) SetMovementBindings(*Terrain, *checkpoint.BindingAuthority) error
// internal/movement
func (*CheckpointContext) SetTerrainBindings(*System, *world.CheckpointContext, *OccupancyGrid, *checkpoint.BindingAuthority) error
```

The admitted installer obtains owner.NoteFeatureFootprint itself and installs
that method value at the existing call site. It cannot accept a different
function plus a claimed owner. A nil interface installs nil without proof;
other owner implementations are opaque data until movement narrows them.
It invokes no method. Nonnil authority and owner create a receipt naming the
exact terrain and the current two-port installation. Ordinary terrain setters
and this installer are nil-receiver no-ops. The owner getter succeeds only for
the current receipt's exact terrain and matching nonnil authority; it neither
attests an unknown owner nor compares arbitrary interfaces.

World context registration requires nonnil context, terrain and authority and
a valid current receipt; it records that exact receipt. Repeating that same
tuple is allowed, while conflicting terrain/authority/receipt registration
refuses atomically. Full collection and writing check the current receipt is
still exactly that registered receipt and belongs to the actual terrain.
A copied terrain cannot borrow proof. Unregistered all-absent fixtures remain
valid; present ports without proof refuse. The existing ClassRestamp and Movers
zero bytes become validated presence bytes with no receipt data in the stream.
Every geometry, feature normalization, entry sweep and content check remains.

Movement registration/capture supplies the concrete proof that world cannot
perform across its package boundary: the receipt owner must be a nonnil
*System equal to the current system; actual Movers must be value-form
gridOccupancy with its stored grid equal to the registered grid; System.Terrain
and System.Grid must match the registered terrain/grid. A nonnil grid's plot
must be that terrain. No method on either interface is invoked. Register the
world context only after all movement checks pass. Both contexts must remain
unchanged on a conflict. Movement capture repeats these checks, including the
world receipt, so replacing System/Grid or terrain ports after registration
refuses. Existing Grid body and presence are unchanged; only movement's Terrain
byte becomes a validated presence tag. Other movement bindings remain refused.

The production constructor keeps its current order: SetMovers(gridOccupancy),
AttachPlot(terrain), then SetClassRestampOwnerWithCheckpointBinding(system,
authority). The ordinary constructor passes nil authority and remains unproved.
This does not create new layers, restamp cells or alter movement state. Later
Classes/AirSectors, damage/product callbacks, overlap queries, mapping readers
and queue handler proof are separate contracts; this unit admits none of them.

Tests cover copied terrains/systems, same-authority foreign owners, typed nil
and hostile/noncomparable occupancy, wrong grids, callback extraction and
ordinary reinstall, replacement after registration, conflicts and exact tags,
while existing absent fixtures and read-only callback/RNG assertions remain.

#### 16.3.58 U6 order callback storage boundary

QueueBinding and its five existing adapters (MovementGoalAdapter,
WorldQueryAdapter, WorkAdapter, WeaponAdapter and PresentationAdapter) retain
private callback storage. Every existing callback field Name has an ordinary
NameHook() getter returning that exact callback and SetName(fn) setter. Neither
invokes it or transfers checkpoint proof. Their signatures stay unchanged,
including MovementGoalAdapter.RunAir's AirLegRunner type. Existing direct
nil-receiver access panics remain ordinary method panics; these methods do not
introduce fallback behavior. Later admitted setters must independently clear
or replace the matching slot's proof.

For each of those six types T, TConfig contains the former public field set
with the same names and types, and NewT(TConfig) *T copies those inputs into a
fresh owner without invoking callbacks. Noncallback fields (rules, projected
Community, actual services/RNG and adapter pointers) remain directly visible
on T for existing live projection and exact capture validation. TConfig is only
construction input; editing it later cannot replace a copied callback. Empty
and noncallback-only literals remain valid ordinary owners. No registry, new
policy selection or second command path is introduced.

All ordinary callers migrate mechanically: callback reads/calls go through
the getter, assignments through the setter, and callback-bearing literals
through the copied-input constructor. Keep initialization, callback expression
evaluation, guarded replacement and invocation order. The root performs this
cross-package migration before a bounded orders-owner proof implementation;
existing QueueBinding/owned-handler capture refusals stay in force meanwhile.
The existing order, movement, construction, session and AI behavior tests are
the verification of this mechanical stage. This storage boundary itself adds
no wire bytes and does not attest custom callbacks.

#### 16.3.59 U6 order binding proof and payload

Every callback on §16.3.58's six types has a private, independent installation
proof naming its exact owning object and authority. The ordinary SetName clears
only that slot. SetNameWithCheckpointBinding(fn, authority) performs that same
ordinary installation once, then proves a nonnil callback when authority is
nonnil. NameHook returns only the function, so even a same-function reinstall
loses proof. Copied owners cannot borrow the original's proof. All nil-receiver
and callback invocation behavior remains as before; no function address is read.

For each of the six types T add NewTWithCheckpointBinding(TConfig,
*checkpoint.BindingAuthority) *T. Ordinary and admitted constructors share one
implementation, with the ordinary constructor passing nil authority. Each
configuration expression is evaluated as before, each callback installed once,
and no callback invoked. Preserve the independent live noncallback fields.

```go
// internal/orders
func (*CheckpointContext) SetBindings(*QueueBinding, *economy.Service, *rng.Simulation, *checkpoint.BindingAuthority) error
func (*CheckpointContext) ValidateBinding(*QueueBinding) error
func (*QueueBinding) WriteCheckpoint(*checkpoint.Encoder, *CheckpointContext) error
```

SetBindings requires a nonnil context, binding and authority; economy and RNG
are explicit expected pointers, possibly nil. Record the exact binding and all
five of its current adapter pointers. Reject conflicting repeats atomically.
At registration and at every collection/write, check each nonnil callback's
exact owner/authority proof and all current adapter pointers against the tuple.
Economy must be nil when the expected pointer is nil, otherwise a nonnil concrete
*economy.Service equal to it; reject typed nil and arbitrary implementations
without invoking them. SimRNG must equal the expected pointer. Classify Rules
with the existing closed CheckpointRulesKind; write Community through its
existing complete writer. Never call QueueBinding.Validate or a Ready callback
to validate capture. A foreign binding refuses even with all nil callbacks.
An absent binding is represented explicitly and remains valid for fixtures;
whole-session admission owns production completeness and required presence.

QueueBinding.WriteCheckpoint writes absent 0 or present 1 followed by its
fields in the original public-name lexical order: BuildList, BuilderOptions,
Community, CurrentTick, Damage, DangerCanRespond, DangerRouteFeasible,
DangerStepFeasible, DangerVisible, Economy, Hostility, Lookup, ModernAIPlayer,
Movement, Presentation, ReclaimFeature, Resources, Rules, SimRNG,
TransportAdmission, Weapons, Work, World. Each callback and verified service/RNG
is one presence bool, Rules its existing u8 kind, Community its existing body.
Each adapter uses a presence bool followed by callback presence bools in its
original public-name lexical order (including Ready and RunAir). There are no
pointer IDs or proof bytes. The queue's existing binding byte becomes this
payload at the same position; nil-fixture bytes remain identical. The owned
handler refusal and all existing queue/node/transient checks remain untouched.
A validation or writer failure is sticky and yields no usable digest.

Session's newOrderBinding is the only canonical factory. Root must retain a
private exact-Session/binding record of the five constructed adapters and the
movement System captured by its bound methods. Whole-session capture requires
those actual aliases unchanged. Work and Presentation closures capture the
factory's worldQueries object; merely proving their authority would miss a
later replacement of binding.World. The private factory record closes that
edge, while callback-slot proof detects mutation within the captured adapter.
The factory copies Lookup/Hostility from its freshly proved world adapter;
proof on the copied callback belongs to the destination installation, not the
source's later contents. Subsequent source edits do not rewrite that copy.
Mapping-word copies and owned row handlers need their own later value-transfer
contracts and remain unsupported by their current owners in this unit.

Tests cover independent proof slots on every owner, copied owners/configuration
mutation, same-function ordinary reinstalls, nil and foreign authority,
changed adapters/economy/RNG, atomic conflicting contexts, exact independent
lexical vectors and absent bytes, Rules/Community validation, and capture that
never invokes callbacks or changes RNG/state. Root performs canonical session
and late BuilderOptions/RunAir wiring after this leaf passes its gates. This
unit does not admit an entire Session or add a gameplay rule.

## 17. Verification

- **The multi-seat harness.** One test process composes the same battle N
  times, each session believing a different seat is local, with different
  pump sizes (one to five ticks per pump) and different presentation
  settings, some with a renderer and an audio service bound and some
  without, feeds all of them one scripted stream, and compares complete
  digests after every tick. This exercises the §6 and §4.5 dependencies
  without a network; coverage depends on the scenarios. Every explained
  desync becomes a regression scenario. Short synthetic battles run in
  `tools/check`; long battles with retail content run in `tools/check-retail`,
  the longest under `--full`.
- **Cross-architecture replays.** The same replay files play on darwin/arm64,
  linux/amd64 and windows/amd64 and must report identical canonical digests.
  Exercise native hardware and every supported CPU build level; Rosetta alone
  is not the amd64 acceptance environment. Deliberately delay AI workers: a
  checkpoint must read and cost the same whatever they are doing (§9.1).
- **Host kinds.** The same stream through a windowed client and a headless
  host, past the tick at which the Strict effect pool fills, with matching
  digests; a recording from either plays on the other. The 2026-10-01 probe
  is the failing baseline — a headless run with the window's timing resolver
  installed against one without, not a windowed run, on the seed-7 scene
  before the O22 correction: equal through step 3,000 and different from
  step 4,500, the first refusal falling at tick 3,283. M1 removed the
  resolver (§16.1), and the pool lock now uses seed 5, whose pool first
  fills at tick 2,255 (M1-C9); this test, after M4, is the first to compare
  an actual window with a headless host.
- **Standing ratchets from M1.** The fingerprint locks from an amd64 build
  and an arm64 build on every retail gate run whose host can execute both
  (they agreed fifteen of fifteen on 2026-10-01), and a lock scene that
  fills the Strict effect pool.
- **Codec and relay.** Round-trip and fuzz tests for every message; tick
  assignment, the per-seat client sequence (§4.2), ordering, grant sealing,
  resignations and drops while grants are held, removal votes and their
  tally (§11.1), the agreed summary, readiness invalidation, rate/queue
  limits and desync handling tested in-process. Inject forbidden
  cheat/rule-change commands, foreign actors, reused slots,
  excessive lengths/counts and invalid numeric arguments without using the UI.
  Check rejection leaves resources, RNG and command ordering unchanged.
- **Identity and match settings.** Change only an unbuilt unit's script,
  model geometry or simulation animation metadata and require a pre-start
  mismatch. Change each relevant setting, entry option, mod or mutator and
  require readiness invalidation. Cosmetic-only differences stay local only
  after their consumers prove them simulation-inert. Test view-policy clamps
  through wheel/keys, custom zoom locks, launch flags, both renderers,
  Community megamap, resize/fullscreen and reconnect; leaving restores local
  preferences. Inspect actual captures/visible extents, not only booleans.
- **Checkpoint completeness.** Review every state's owner and exclusions;
  exercise save/restore with computer seats restarted at the snapshot tick,
  paths, COB waits, temporary sight, a full effect pool, unit-slot reuse and
  paused input. Do not infer complete coverage from a single long battle or
  a serializer hashing its own output.
- **Reconnect and audience isolation.** Lose a connection before and after
  command acceptance/echo, resume alongside the old connection, and verify
  exactly-once stream admission. Catch-up consumers cannot stall live play.
  Check credential isolation, digest deadlines, snapshot bounds, spectator
  history/replay delays and private chat audiences.
- **Impaired networks.** Relay and clients in one process over links with
  added latency, jitter and stalls, supplemented by real-socket packet-loss
  and backpressure tests. Record command latency distributions, stall count,
  queue bounds and playout reserve for maximum selections and supported
  player/spectator counts. Declare acceptance budgets after exploratory runs
  and before release acceptance; passing is not defined by whatever it measures.
- **Competitive policy.** Exercise unequal latency, an embedded host,
  dishonest issue-tick/progress/digest reports, pause abuse, forbidden targets,
  full-vision watcher transitions, forged results and loss of the result
  authority. Validate the actual scheduling and result-authority contracts
  before ranked release.
- **Desync policy.** Seed a one-field fault into one of three participants
  and into one of two: the three-seat casual battle continues with two; the
  two-seat battle ends without a result before M8 and continues from the
  room creator's state after it; in a rated room nothing continues (§9.3).
  Report a false digest and a false summary and check that neither moves an
  honest seat.
- **Fingerprint locks.** M1 left the fifteen original locks unchanged on
  both architectures and added a sixteenth, the Strict effect-pool scene at
  step 4,500 (M1-C9). The O22 retail correction (§19 O22,
  DESIGN_MOVEMENT_PATH §3.4) left the seed-7 scene's pool unfilled, so on
  2026-10-02 that sixteenth lock moved to seed 5, whose pool fills, with its
  reason recorded at the lock; the fifteen remain unchanged. M2
  moves locks only by the interface bits that leave the hashed status
  words, plus any single-player change it declares under M2-C7 (§7.4.3).
  Later milestones move no single-seat lock except where §16 says so. Each
  move carries its reason.
- **Cost.** Measure serialization time per checkpoint and the tick ring's
  cost per tick, relay fan-out/retention, and live-target catch-up on large
  late battles. Run the simulation benchmark before/after M5 so per-seat
  generalizations do not slow the single-player tick. Run a battle at the
  advertised seat and unit limits on the weakest supported hardware before a
  lobby offers them (§4.4). Snapshot transfer/recovery has a separate budget.

## 18. Research map

| Research | Used for |
|---|---|
| `[08 "Lockstep advancement"]`, `[08 "Soft pacing — no per-tick input barrier"]`, `[08 "No rollback or world snapshot resync"]`, `[08 "Full-world hash"]`, `[08 "Synchronization and integrity checks"]`, `[08 "Economy and integrity checks"]`, `[08 "Unit-sync ownership and body structure"]`, `[08 "DirectPlay transport"]`, `[04 R-MOV-03 §1]`, `[06 R-DMG-01 §9]`, `[05 R-SHARE-01 §1]`, `[01 §7.1]` | The retail model and why it is not reproduced (§3.1, §3.2) |
| `[08 R-ENTRY-01 §2]`, `[08 R-ENTRY-01 §3]`, `[08 R-ENTRY-01 §5]`, `[08 R-SKIR-01 §3]`, `[08 R-SKIR-01 §6]`, `[08 R-SKIR-01 §10]`, `[08 R-TRIG-01 §1]`, `[08 R-TRIG-01 §6]`, `[08 R-SESS-01 §1]`, `[08 R-OOS-01 §5]`, `[08 R-SAVE-02 §4]`, `[08 "Multiplayer saves"]`, `[08 "Bounded absence"]`, `[08 "Session end and reporting"]`, `[08 "Disconnect, resign, and peer loss"]`, `[08 "Unit-data negotiation and catalog retention"]`, `[05 R-SHARE-01 §3]`, `[05 R-SHARE-01 §5]`, `[05 R-SHARE-01 §6]`, `[05 R-SHARE-01 §7]`, `[05 R-SHARE-01 §9]`, `[05 R-WORK-01 §15]`, `[07 R-CAM-01 §2]`, `[07 R-CAM-01 §6]`, `[07 §5]`, `[07 §11]`, `[07 R-FE-01 §9]`, `[07 R-HUD-04 §1]`, `[01 R-PLAT-01 §2]`, `[03 R-VIS-01 §1]`, `[03 R-VIS-01 §7]`, `[03 §3.2]`, `[05 R-SHARE-01 §4]`, `[08 R-SKIR-01 §7]`, `[08 R-SKIR-01 §12]` | The player-visible rules a Strict 3.1 online battle keeps (§3.3, §6.4, §7.1, §11) |
| `[03 R-SENSOR-01]`, `[03 R-VIS-01 §4]`, `[08 R-SESS-01 §3]`, `[08 R-ENTRY-01 §7]`, `[04 R-COB-03 §6]`, `[03 R-STRIP-01 §1]`, `[04 R-P0-08-B §1]`, `[08 R-AI-01 §7]`, `[06 §3.1]`, `[06 §12.1]`, `[05 R-ECO-01 §1]`, `[05 R-FEAT-01 §8]`, `[04 R-COLL-01 §4]`, `[07 R-CAM-01 §12]` | Per-viewer and per-owner work (§6) |
| `[01 §4.3]`, `[01 §4.4]`, `[01 R-PLAT-02 §5]`, `[01 R-PLAT-02 §7]`, `[03 R-COMP-02 §2]` | The clock, the pump and the executor tail (§4) |
| `[fmt tad]` | The community recorder's files, which Nanolathe replays do not read (§3.3) |
| `[01 R-DET-01 §3]`, `[01 R-DET-01 §7]`, `[03 §1]`, `[06 R-WFX-01 §1]`, `[04 R-COB-04 §3]`, `[03 R-FX-01 §3]`, `[06 §3.2]`, `[08 R-CAMP-01 §9]` | One simulation on every host: the distance routine's precision and arithmetic, the effect pool and what it gates, an ungated mode bit, and a kind-3 branch with a draw (§5.3, §6.4, §7.1) |

## 19. Open questions

Unsettled retail behavior becomes a `TODO(question)` at its dependent code
site and stays in the owning retail research document's Unknown list.
Engineering measurements and Nanolathe policy choices stay here and in their
owning design documents; they are not retail findings. None is permission to
invent a retail rule while implementing an independent transport feature.

| ID | Question | What settles it |
|---|---|---|
| O1 | Retail authority for pause, speed, resign, sharing, removal and cheats | **Settled.** Sender UI supplies the restrictions; admitted receivers trust payload identities and never redispatch chat commands `[08 "Packet framing and dispatch"]` `[08 "Lockstep advancement"]` `[07 R-CAM-01 §6]`. Residual transport and unchecked-body questions remain in the owning research Unknown lists. |
| O2 | Departed-player units/resources and hosted-computer termination | **Settled** by `[08 R-LEAVE-01]`: destruction and emptying, no redistribution, machine-group removal, timeout dialog and launched-battle host migration. Q6 records the revised final-removal policy; Q26 makes Strict remove hosted computers with their human's final removal, as retail does, while Modern/Community keep them running; Q27 makes final removal a vote outcome (§11.1). |
| O3 | Explored-map sharing: `[05 R-SHARE-01 §6]` described it behind the share-mapping bit; `[03 R-VIS-01 §7]` said nothing is shared | **Settled 2026-10-01.** Both held of different stores: current sight is never merged, explored memory is copied on request, and the radar share marks the sharer's own units (§3.3, §6.3). Whether the share options survive into a second battle of one session stays **Unknown** in the research. |
| O4 | Computer hosting and online difficulty | **Settled** by `[08 R-SKIR-01 §13]` `[08 R-AI-01 §21]`: creator hosts one computer, association fixed, difficulty belongs to that machine. Q23 records the approved online cap/difficulty/admission policies (§6.6). |
| O5 | Alliance protocol and declaration timing | **Settled** by `[05 R-SHARE-01 §1]` `[07 R-FE-01 §7]` `[08 R-SKIR-01 §3]`: immediate sender write, target-only receipt, team propagation and victory reads. Q22 adopts the shared directed matrix (§6.7). |
| O6 | Defeated player when watching is forbidden, including peer effects | **Settled** by `[08 R-SKIR-01 §3]` `[08 R-LEAVE-01 §10]`: ordinary lost ending unless a live hosted computer keeps the human watching; peers store the watch bit, and a later exit is ordinary departure. |
| O7 | Four pairs of research statements that conflicted: which host option the overlay's `Watching:` row reads; whether the lobby's command-line words preset the host's options; whether the Tab strip exists outside multiplayer; and which step makes a defeated player a watcher | **Settled 2026-10-01.** Bit 15 is *game closed* and the `Watching:` row reads bit 7 `[08 R-SKIR-01 §12]`. No command-line word presets the lobby; an online service's configuration file does `[01 R-PLAT-01 §2]` `[08 R-SKIR-01 §7]`. The Tab strip and the `h` key are multiplayer-only `[07 R-CAM-01 §2]`. The elimination block makes the watcher, and neither answer to the question changes that `[07 R-FE-01 §9]`. |
| O8 | `[04 R-COB-03 §6]` placed the effect gate outside the deterministic contract because a failed gate does nothing, while I4 counts the passing path's CRT draws as behaviour | **Settled 2026-10-01.** The section now says the failing branch is inert, the passing branch reaches the CRT stream and the strip pool, and the gate breaks no contract between retail's machines. |
| O9 | Floating-point results under `GOAMD64=v3` and on native amd64 hardware; only Rosetta was measured in the initial audit | **M1 gate settled 2026-10-02:** native Linux/amd64 v1/v3, Windows/amd64 v1 and Darwin/arm64 pass the kernel's independent vectors and digests (§16.1). The v3 standard library differs because it permits fusion; the reference is explicitly unfused v1. Full-world equivalence remains M3. |
| O10 | Checkpoint and tick-ring cost, and a complete state inventory | M3's reviewed field inventory, checkpoint/ring measurements and acceptance budgets (§16.3); M8's exact-restore and continuation evidence. |
| O11 | Rejoin time on the largest/late battles and whether snapshot recovery must precede hosted release | Live-target catch-up measurements before promising M7 rejoin; advance M8 if the supported window cannot be met. |
| O12 | Kind-3 branches beyond those §6.4 lists | Each implementation unit audits the code it touches against the research. |
| O13 | Competitive scheduling, input/view age limits and acceptable latency advantage | Compare arrival scheduling and a shared-horizon candidate under unequal RTT/jitter and malicious reports; approve the concrete policy before M9. |
| O14 | Fair admission of delayed direct targets, radar contacts and area orders | Research the existing contracts, specify bounded authoritative observation evidence and validate against the scheduler; any gameplay departure follows DESIGN_GAMEPLAY_RULES. |
| O15 | Ranked integrity level: full-state lockstep with verified results, or filtered state delivery | An explicit product decision before advertising ranked guarantees; prototype and budget the different presentation/state protocol if required. |
| O16 | Whether competitive view policy also equalizes world viewport coverage | Decide whether resolution/aspect advantages are acceptable; otherwise define a common world-area bound and test all view/resize routes. No numeric viewport cap is assumed. |
| O17 | Final wire field bounds, start/pacing/reconnect state tables and supported build manifests | M2 publishes command/configuration schemas and build manifests (§16.2), with codec and hostile-input tests. M6 owns live start/pacing state tables, including the bound after which a connected seat that makes no progress enters the drop state (a §15 Q28 requirement; the value is the table's); M7 owns reconnect tables and their state-machine tests. Implementation must not infer any of these from Go layouts or sender UI behavior. |
| O18 | Retail's two-argument distance routine: the rounding sequence read on 2026-10-01 (L1), and the retail forms of the other library calls wherever a stored value could change `[01 R-DET-01 §3]` | **Settled 2026-10-01** for the distance routine: `[01 R-DET-01 §7]`, from two independent traces, its exits included. Still open: a manual retail observation that would confirm it (a reach test at an offset such as 20 by 99, where the routine's truncated distance is 100 and the exact one 101), and the retail forms of the other library calls. |
| O19 | How often the effect pool fills in play under each rule set, and whether the 4,096-event window is ever reached | **Settled 2026-10-01** by the census in L9: only the Strict benchmark fight (seed 7, before O22) fills its pool, no existing lock moves, and the event window peaks at 48 of 4,096. After O22 that scene peaks at 296 records, and the pool lock uses seed 5, which fills (M1-C9). The path benchmark's players are all allied, so nothing there fires. |
| O20 | Machine scope of end state, respawn visibility and elimination draws | **Settled** by `[08 R-SKIR-01 §3]` `[08 R-ENTRY-01 §7]` `[08 R-CAMP-01 §9]` `[05 R-ECO-01 §1]`: per-machine countdown/latch, complete machine-grid rebuild, and one local CRT draw per elimination on each machine. Q24 adopts canonical per-player histories (§6.7); transport enumeration and remaining writer/caller scope questions stay in research. |
| O21 | Whether restarting Modern controllers from observation at a snapshot tick is acceptable play (§9.1, §15 Q21) | Play-test computer seats across forced restarts and compare with a save and load, which already restarts them. |
| O22 | Strict fidelity where the engine takes an exact integer root or compares squares and retail calls its distance routine on whole numbers — candidates seen while reading for M1 are the leash test, the guard and nano ranges and the air order distance. The routine truncates to one below the exact root on 722 ordered whole-number pairs up to 3,000, such as 20 by 99 `[01 R-DET-01 §7]`. This is not a lockstep hazard: integer code computes the same everywhere | **Settled.** `[04 R-STANCE-01 §4]`, `[04 R-AIR-01 §8]` and `[05 R-WORK-01 §2]` establish the caller widths and routine identity. Leash, work/construction reach and air-order distances now use the portable kernel. Ordinary guard arrival and unit reclaim remain genuinely squared tests. The dogfight reads a signed high word before comparing with 160; correcting it left the seed-7 Strict pool at a peak of 296 records through 15,000 steps, so the step-4,500 pool lock moved to seed 5 (M1-C9, DESIGN_MOVEMENT_PATH §3.4). The original fifteen locks are unchanged. M1-C6's CPU budget (§16.1) was measured before this change and re-measured on 2026-10-02 on a shared host (Go 1.27.1, Darwin/arm64, two runtime workers; 216 sequential runs of the M1-C6 setting, each build's fingerprints identical across its runs). On the windows where the build before O22 and the build with it still play the same battle, process CPU per tick differs by about 0 ± 2%; in the profiles the kernel takes about 2% of Strict and 1.6% of Modern CPU, all of it at call sites that predate O22, and the leash, reach and air-order sites O22 changed do not appear. Against M1-C6's baseline the quietest Modern session measured +1.2% (range −1.5% to +2.4%), consistent with the recorded +2.15%; the host's noise is as large as the three-percent budget, so this supports the budget and does not certify it, and Strict was never measured against it on a quiet host. Two things the first O22 record did not say: the correction is a shared retail mechanic, so it also changes Modern battles that reach its boundaries — the Modern benchmark scene's fingerprint already differs by tick 2,199, with no Modern lock moved, and giving the Modern repair-pad queue its own distance does not change that — and after O22 a full-window fingerprint match with the M1-C6 baseline is no longer possible, so a later cost comparison must use a window before the battles diverge. |
| O23 | Which retail leaving path a Nanolathe resignation corresponds to, and whether Strict should reproduce it. Retail's surrender question has a menu variant (teardown only; peers delete positive-health units with no script, explosion or wreck) and an exit variant (30000 self-damage first, so a unit it leaves at non-positive health goes with the kill script, an effect-only explosion and the wreck); the order in which peers then empty the leaver's slots is only a Supported inference `[08 R-LEAVE-01 §7]` | **Decided 2026-10-02**, the maintainer: "units can be deleted silently" — the menu variant, as the other machines see it. A Strict resignation deletes the resigning seat's units, and its hosted computers' units with them, with no `Killed` script, explosion, wreck or credit, keeping the researched receiver path for a unit already at non-positive health and applying no self-damage; the records are then cleared as a voted removal clears them (§11.1). This supersedes the §15 Q28 proposal that a resignation apply the voted-removal contract. Still open, as research rather than a decision: the slot order in which peers empty the leaver's records, which a trace of the transport's player-destroyed notices when the leaver's session closes would settle; until then the protocol uses the voted removal's order (§11.1). |


#### 16.3.60 U6 unit script installation and capture contexts

The unit owner retains pending COB receipts from §16.3.51 and §16.3.55 on
that exact Unit, never on the allocator's predicted next serial. The admitted
strict helper shares the ordinary binding path and all existing callback
ordering. A nil authority takes the ordinary path without proof metadata.

```go
// internal/units; same operands as the ordinary strict unit helper, then authority.
func BindCOBForUnitWithCheckpointBinding(vfs.FSOps, *Unit, *model.Model, *rng.Simulation, cob.PresentationSink, func(int, int32) bool, func(*cob.Binding) error, *checkpoint.BindingAuthority) (*cob.Binding, error)
func (*Unit) RetainCheckpointPortInstallation(*cob.CheckpointPortInstallation)
func (*Unit) RetainCheckpointVMInstallation(*cob.CheckpointVMInstallation)
func (*CheckpointContext) ScriptBindings(*Unit) (*cob.CheckpointContext, error)
```

The admitted helper records its exact Unit and authority before PreCreate,
and retains the exact admitted VM when PreCreate receives it. Transport
attestation and script preflight require that same current VM; clearing or
replacing it through an ordinary path cannot reuse old unit metadata.
Its port, script-touched and mapped-render installations use pending receipt
APIs at their original sites. BindStrictWithCheckpointBinding supplies the
Binding/VM visibility receipt; retain it only on success. The existing
post-binder transport query installation uses this unit's authority when
present and valid, and otherwise keeps the ordinary call. Neither the
fallback loader nor an arbitrary ordinary binder acquires proof.

Retain methods ignore nil receipts. A nonnil receipt without a prior admitted
unit installation is a sticky diagnostic failure. Before successful allocation
it is appended to that unit's private typed list; after successful allocation
it seals immediately against the actual Handle/AllocationSerial. Immediately
after each of the two existing successful serial assignments, seal all retained
receipts against that actual allocation. Nested creation therefore seals each
unit independently. Failed creation never reaches this boundary. A seal error
is retained as a diagnostic refusal only; it cannot fail or roll back creation,
change RNG, call gameplay code, or suppress an existing callback. Keep the
first error. A copied Unit cannot reuse the original unit's metadata. Receipt
lists may be released after sealing; their private VM proof remains installed.
All this metadata is excluded from checkpoint bytes.

ScriptBindings is capture-local preparation after reference discovery: require
a nonnil discovered allocation and VM, admitted keys and the context's exact
lifecycle World/authority. Resolve the actual program key, register that VM
and the unit's actual nonzero allocation on a new lower COB context, and retain
it by exact Unit pointer. Repeated calls return that same context only while
the VM, program key, allocation and Binding pointer are unchanged. It also
retains the current Unit.RenderPieceFlags header and rechecks its length and
nonempty backing start at repeated access and preflight; empty slices have no
readable storage. Root registers current
Binding/RNG/render slices and the closed session sinks on it (§16.3.55).
Preparation never calls a script, render getter or gameplay callback.

The script preflight rechecks current aliases, admitted program key, exact
unit/VM/allocation context and any sticky installation error. With a registered
lower context it calls ValidateCheckpointBindings before emitting any script
bytes, then emits Binding's four validated presence booleans and writes the
VM with that same context. Without registration, existing all-absent fixtures
keep their exact bytes and a present binding still refuses. Never discover a
reference during writing. Tests cover normal/forced allocation, failed and
nested creation, actual final serials, copied/stale metadata, receipt failures
that do not alter creation, ordinary-path refusal, capture context conflicts,
independent payload tags and capture purity.


#### 16.3.61 U6 movement auxiliary ownership and lazy occupancy

```go
// internal/movement
func (*System) BindWorldWithCheckpointBinding(*units.World, *checkpoint.BindingAuthority)
func (*System) AttachOverlapBindingWithCheckpointBinding(func(uint8) uint8, *checkpoint.BindingAuthority)
func (*CheckpointContext) SetAuxiliaryBindings(*System, *units.World, *checkpoint.BindingAuthority) error
```

BindWorld's admitted sibling shares its existing implementation and installs
that same System as the unit world's attachment observer through the admitted
setter at the original site. Ordinary BindWorld still clears observer proof.
All sizing, clearing and registry behavior stays in its existing order.

AttachOverlapBinding's admitted sibling shares its existing installation path.
The grid privately retains independent proofs for ownerState and claimConflict:
exact grid, exact captured System and authority. Ordinary AttachOverlap clears
only ownerState proof, and ordinary AttachOverlapBinding clears both affected
proofs. The admitted System helper stamps the actual claimConflict method it
installs; no caller can claim an unrelated callback is that method. Capture
checks the actual grid, overlap's nonnil concrete *System and the expected
System/world aliases without invoking either callback. Presence bytes replace
the existing absent tags at their lexical positions.

The admitted System constructor keeps a private immutable snapshot of its
freshly built AirSectors, with exact System/grid/terrain/authority identities,
dimensions, all records including padding and the sentinel. Snapshot after the
existing constructor builds the grid, without moving or repeating that build.
Capture validates the actual grid pointer and every stored value against this
snapshot; it never rebuilds from current terrain, whose mutable plot may have
changed since entry. Copied Systems, replaced grids, missing proof and mutated
records refuse. Absent AirSectors fixtures stay absent. The existing AirSectors
slot changes only to a validated presence byte; the immutable values are proved
by the admitted constructor and frozen-entry graph, not duplicated as state.
No caller-facing arbitrary snapshot attestation API is added.

SetAuxiliaryBindings records exact System, expected World and authority;
required context/System/authority must be nonnil, the World may be nil for
fixtures. It agrees with existing context System and SetPathBindings' World
and authority in both registration orders, refuses conflicts atomically, and
is rechecked at collection and writing. Registration proves no callback by
itself. AirSectors and present overlap callbacks require their own proofs.
Other unimplemented composition slots remain refused.

Occupancy plane dimensions are nonnegative, and each of air and cells is
independently either nil or exactly planeW*planeH elements. reindexPlane keeps
nil storage and planeRow allocates each on demand. Counts must still equal
nonzero stored occupants, including zero for nil storage. Capture retains the
actual zero or full slice length in the existing encoding and never calls
planeRow or creates absent planes. A nonnil empty slice with nonzero dimensions
is malformed. Add vectors for untouched, ground-only and air-only grids,
malformed lengths/counts, both registration orders, ordinary replacement,
wrong/copy owners and air snapshot mutation/purity.


#### 16.3.62 U6 copied owned-order handlers

Owned queue handlers are copied values whose provenance travels with the
function, including BindQueue's existing transfer from the prior queue. The
ordinary getter still returns only a function, and ordinary SetOwnedHandler
clears only that destination row's proof. Do not compare function addresses,
call a handler, or infer ownership from its current source slot.

```go
// internal/orders; all fields of the two values below are private.
type CheckpointHandlerSource struct { /* exact owner and authority */ }
type CheckpointOwnedHandler struct { /* function, kind, source */ }
const CheckpointConstructionWake uint8 = 1
const CheckpointGetBuilt uint8 = 2
const CheckpointAirStandby uint8 = 3
func NewCheckpointHandlerSource[T any](*T, *checkpoint.BindingAuthority) *CheckpointHandlerSource
func RegisterCheckpointHandlerSource[T any](*CheckpointContext, uint8, *CheckpointHandlerSource, *T, *checkpoint.BindingAuthority) error
func NewCheckpointOwnedHandler(OwnedHandler, uint8, *CheckpointHandlerSource) CheckpointOwnedHandler
func (CheckpointOwnedHandler) Handler() OwnedHandler
func (*Queue) SetOwnedHandlerWithCheckpointBinding(ID, CheckpointOwnedHandler)
```

Source construction requires a nonnil concrete owner pointer and authority;
nil operands return no source. Registration uses a closed generic type
assertion and pointer comparison with the expected concrete owner. It never
calls an interface or compares arbitrary interface values. Each of the three
kinds has one expected source per capture, with atomic conflicting refusal and
idempotent same registration. Source registration and binding registration use
the same nonnil authority in either order; neither can refresh an old capture.
An arbitrary caller cannot recover a live session's authority from a source.

The function/source/kind record is immutable after construction. The admitted
queue setter performs ordinary installation once at the same point, then keeps
that record as the row's proof. Unknown kind, missing source, nil handler or
mismatched row cannot gain admission. Kind 1 is limited to BuildingBuild,
MobileBuild, VTOL_MobileBuild, ReclaimUnit and VTOL_ReclaimUnit; kind 2 to
GetBuilt; kind 3 to VTOL_Standby. Preserve existing nil/invalid-ID allocation
and no-op behavior. BindQueue copies the proof slice alongside the existing
handler slice only in the existing inheritance branch; explicit replacement
queues retain their own installed rows. Clearing/replacing one row cannot
alter proof for another row, queue or previously copied value.

Capture checks every actual nonnil row's retained proof and its exact
registered source. A present handler with no proof refuses. Retained proof
without the corresponding actual handler also refuses. Empty/all-nil storage
keeps tag 0. Present storage uses tag 1, u32 active count, then ascending
numeric rows, each row ID u8 followed by kind u8. All other queue payload is
unchanged. Source pointers, authorities and function values are never bytes.
Construction and movement create their sources at their reviewed canonical
handler installation sites; root registers the actual owners and these exact
sources before collection. This leaf does not itself admit those producers.
Tests pin queue inheritance, independent replacements, wrong/copy owners,
wrong authorities and kind/row combinations, exact vectors, context conflicts,
ordinary getter/reinstall refusal and capture purity.


#### 16.3.63 U6 frozen input mutation validation

Cached content hashes and rounded canonical text do not prove that an admitted
object remains unchanged. FreezeSimulationInputs retains a private typed
snapshot after preparation and source sealing, before it returns. This is
local diagnostic metadata, not a new manifest, wire field or M2 identity.
No checkpoint may establish its baseline from the first capture's live values.

```go
// internal/content
func (*SimulationInputs) ValidateCheckpointInputs() error
func (*CheckpointKeys) ValidateMovementClasses(map[string]*MovementClass) error
```

CheckpointKeys validates the freeze-time snapshot before constructing keys;
root revalidates it before every full capture when reusing keys. The snapshot
retains original object pointers, exact detached scalar values (floating
values by bits), ordered slices including holes, and sorted raw-key lookup
rows. Validation checks lengths and those expected lookups without ranging
current maps, calling getters, reading a filesystem, relinking definitions,
normalizing values or rebuilding caches. Catalog.Clone is not a snapshot:
it reconstructs indexes and shares some immutable storage.

Validate the exact catalog, complete actual unitRecords and Units index,
Weapons/Features/Movement membership including nil entries, ordered Sides,
weaponRecords/weaponByID and CategoryRegistry.byName. Record full consumed
definition values, category-mask words, weapon damageOrder and Damage,
resolved weapon/feature links, limits, categories, build menus, download
placements, meteor, sight/LOS tables (including unused records), and Survival
roster Units/IncludeBuildTree. Keep all fields of movement classes: canonical
key and FootprintX/Z, Max/MinWaterDepth, MaxSlope, BadSlope, MaxWaterSlope,
BadWaterSlope. Side anchors keep every raw key and rectangle coordinate.
Unit extension flags such as NanolatheInfector and exact economy/cost values
must be checked even when an existing canonical writer omits or rounds them.
Compiled COB programs retain their existing semantic identity contract.

Also retain complete parsed-model and SimArt sequence membership, including
known misses, geometry, frame delays, effect entries/holds and visit order;
selected-map header pointer and actual header/schema values. Preserve actual
lookup topology. Other installed maps are outside the battle selection.
Provenance/providers, warnings, duplicate diagnostics, sounds/aliases,
Survival AttackerSkin and SimArt source bookkeeping remain excluded under
the existing consumer audit. Any further model-cache exclusion requires a
consumer audit; do not infer it from the cache's name.

ValidateMovementClasses compares a supplied map's complete raw-key membership,
exact admitted class pointers and the stored values above. The map container
itself may differ; a copied class with equal values cannot replace its admitted
pointer. Validation is read-only and may never repair the caller's map.

Tests mutate values both before the first CheckpointKeys call and afterward;
cover deletion, nil and same-name foreign replacements, stale stored hashes,
sub-rounding float changes, poisoned runtime indexes, and repeated validation
with hostile filesystem methods and unchanged caches. These checks detect
invalid local mutation; they do not change authored content or gameplay.


#### 16.3.64 U6 construction and movement handler producers

```go
// internal/construction
func NewServiceWithCheckpointBinding(*world.Terrain, *content.Catalog, *units.World, *economy.Service, *checkpoint.BindingAuthority) *Service
func (*Service) RegisterCheckpointOrderHandlers(*orders.CheckpointContext, *checkpoint.BindingAuthority) error
// internal/movement
func (*System) RegisterCheckpointOrderHandlers(*orders.CheckpointContext, *checkpoint.BindingAuthority) error
```

Construction's new constructor shares NewService's body; the old entry passes
nil authority. Movement uses its existing NewSystemWithCheckpointBindings
constructor. Each admitted constructor retains one exact owner/authority and
an opaque orders.CheckpointHandlerSource. Nil authority creates no proof.
Neither constructor initializes lazy handlers early or invokes one.
At the existing lazy handler creation sites only, retain the immutable owned
handler value alongside the existing cached function: construction wake kind
1, GetBuilt kind 2, movement standby kind 3 (§16.3.62). At every existing
queue installation site transfer that same value through the admitted setter
when valid proof exists, and preserve the ordinary setter otherwise. Repeated
queue registration never replaces the cached handler or creates new closures.
An ordinary cached handler cannot acquire proof later.

Registration requires the exact constructor owner, nonnil matching authority
and original source, then registers it against that concrete owner on the
orders context. It never creates a source, constructs rows, or calls a handler.
Copied services/systems, ordinary constructors and conflicting contexts refuse.
Lower owners revalidate registered handler sources at collection/writing.
Movement's existing airLegHandler field writes its actual presence after its
own cached handler proof is checked; no new bytes are added. Empty fixtures
with no handler keep the absent tag. Construction's row caches remain derived
metadata under their existing disposition; their source is checked without
requiring other unfinished construction bindings to be admitted first.
Root supplies the authority at canonical construction creation and registers
both producers before collecting queues. The old ordinary paths and function
bodies retain their behavior. Tests cover late queue creation/inheritance,
repeated registration, ordinary/copy refusal, changed authority and unchanged
dispatch; capture never runs a handler or creates lazy registration state.


#### 16.3.65 U6 canonical session script bindings

The session's existing strict unit binder uses §16.3.60's admitted helper.
Before Create, port 16, query ports 7–15, adopted ports 32/69–75 and both
transport mutation callbacks retain their pending installation receipts on
that exact unit. The unit owner seals them at its actual allocation boundary.
Nil authority keeps ordinary installation and does not retain metadata.
Callback bodies, order and creation failure behavior are unchanged.

The existing private per-unit presentation sink retains two diagnostic aliases
only for admitted composition: the exact Unit and the Terrain receiver captured
by port 16's method value. Capture compares those with the current session and
unit, alongside the sink's existing session/publication/clock/handle/piece-map
aliases. This bounds the metadata lifetime to the existing sink and prevents a
replaced terrain from concealing the old ground-height receiver. No source
pointer or receipt contributes bytes. Explosion's private concrete sink must
point to that same presentation sink.

After reference discovery, root obtains each unit's capture-local ScriptBindings
context, registers its actual Binding, simulation RNG, render backing and piece
map, and supplies the validated concrete presentation/explosion sinks through
the lower closed generic registration APIs. Lower preflight verifies every
installation before any script bytes are written. Capture calls no callback,
script, render getter or filesystem operation and never rebinds a VM. Tests use
the real session binder with authored model/COB bytes, including creation after
the first capture, ordinary callback replacement, copied sessions and changed
captured terrain, and prove repeated capture leaves RNG and script state alone.


#### 16.3.66 U6 copied mapping-word sources

```go
// internal/orders; private function and original installation proof.
type CheckpointMappingWord struct { /* immutable copied value */ }
func (*WorldQueryAdapter) CheckpointMappingWord() CheckpointMappingWord
func (CheckpointMappingWord) Reader() func(int32, int32) (uint16, bool)
func (*CheckpointContext) ValidateMappingWord(CheckpointMappingWord) error
// internal/movement
func (*ClassLayers) BindMappingWordWithCheckpointBinding(orders.CheckpointMappingWord)
```

The orders getter copies the current callback with its proof only when that
proof still names this exact adapter and its authority. An ordinary installation
returns its actual callback with no proof, so simulation behavior is unchanged
but checkpoint validation refuses it. Reader returns the function alone; passing
it through an ordinary installation never transfers provenance. The copied value
retains its original proof after the source slot changes; it does not claim to
be the adapter's current callback. ValidateMappingWord checks its exact original
adapter against the capture's registered WorldQueryAdapter and authority, without
calling the callback, comparing function pointers or refreshing proof from the
current slot. The zero value is absence; present callbacks need valid proof.

ClassLayers and each ClassLayer retain the immutable copied value beside their
private mapping callback. Ordinary BindMappingWord keeps its existing nil no-op
and allocation-order update behavior, clearing proof for each callback it
actually replaces. The admitted sibling performs that same work at the same
site and transfers the value to the registry and each affected layer. For's
existing inheritance copies it with the mapping callback before the original
stamp. Private movement helpers obtain the copied value through the requester's
actual queue World adapter. Every existing retained mapping installation uses
the admitted sibling, including diagnostic/laboratory paths; immediate reads
continue using the ordinary function getter. No new mapping read, registry,
layer, stamp, path request or source lookup is introduced.

Registry and layer collection/writing validate their retained copied value
against the shared orders context; a nonnil mapping with absent proof refuses.
Existing mapping bytes become actual presence, retaining zero for absence.
All other fields and lazy behavior stay unchanged. Tests cover inheritance,
registry rebinding, nil no-op, source-slot replacement, ordinary getter/reinstall,
copied adapters, conflicting captures, stale proof without a callback, pure
capture with panic readers, and unchanged classifier behavior.


#### 16.3.67 U6 construction callback storage and production bindings

Construction privately stores CRTRandom, IsSpecialSecondState, ModelForFactory
and ModelForUnit. Each retains its existing function signature and gains an
ordinary Set<Name>/ <Name>Hook pair plus Set<Name>WithCheckpointBinding(fn,
*checkpoint.BindingAuthority). Ordinary replacement clears only that slot's
private proof; the admitted setter performs it once and records exact Service
and authority for a nonnil function. Nil authority creates no proof. All
readers and callers migrate without changing callback bodies, invocation order,
arguments or results. There are four production installations, all at session
composition. Allocator and LimitChecker have no production installations and
retain their existing nonnil checkpoint refusals; nil keeps the current World
allocation/default-admission path. Do not encapsulate unused fixture-only seams.

```go
// internal/construction
func (*CheckpointContext) SetBindings(*Service, *content.SimulationInputs, *units.World, *economy.Service, *combat.Service, *movement.System, *orders.QueueBinding, *checkpoint.BindingAuthority) error
func SetCheckpointPresentationSink[T any, P interface { *T; EmitNanolathe(frame.Event) bool }](*CheckpointContext, P) error
```

SetBindings retains one exact tuple atomically and revalidates exact repeats.
The source must be §16.3.64's admitted constructor owner and authority. Current
Catalog equals inputs.Catalog(); World/Economy/Combat/Movement/OrderBinding
must equal the explicit expected pointers, and repairWorld is nil or that same
unit world. Terrain retains its existing comparison against the collected
world context. Root validates frozen inputs once for the whole capture; these
owner checks never rebuild or rehash them. Callback proofs are independent and
are checked without invocation. Ordinary and copied services cannot inherit
another service's installations. Keep all existing mid-pump refusals and the
ordinary temporary World/Economy/Terrain/Catalog swaps in StepUnit.

Presentation is checked by a closed generic concrete-pointer matcher, as in
the COB sink registration. Require a nonnil expected pointer, reject typed nil
and arbitrary foreign/noncomparable implementations without calling methods,
and permit only exact repeats. Root retains its actual installed
*buildPresentationSink pointer and exact Session/Service/authority, then checks
that sink's session field before registering it. The sink appends authoritative
strips independently of frame-event admission; it is not excluded as display
state. The CRT and special-state closures capture Session and read its current
owners; model callbacks capture no extra objects. Capture never calls CrtRNG,
which may initialize a stream, any callback, or the presentation sink.

Collection and writing revalidate the registered tuple and sink. Existing absent
binding bytes become actual booleans in the same lexical field positions; no
new table or pointer value is encoded. Empty all-absent fixtures preserve their
bytes without a registered tuple. Tests pin ordinary replacement per slot,
foreign authority/copied owners, every alias and sink replacement, atomic
conflicts, independent presence vectors and capture purity including RNG and
strip state. Root migrates canonical calls and registers exact owners before
construction collection; the leaf does not itself prove a complete session.


#### 16.3.68 U6 effect art and fragment bindings

The actual fixed pool retains fragmentContext between ticks. Its
fragmentStepping field is the stored TerrainHeight-presence predicate, not a
traversal marker; validation requires exact coherence and its existing
TerrainHeight byte represents that derived field. Root's completed-tick
boundary provides quiescence for the closed canonical callbacks. No new active
marker is needed. This corrects §16.3.17's former false-only requirement.

```go
// internal/effects
func (*EffectService) CheckpointFixedPool() *FixedEffectPool
func (*CheckpointContext) SetBindings(*EffectService, *content.SimArt, *world.Terrain, uint32, *checkpoint.BindingAuthority) error
func (*FixedEffectPool) SetFragmentStepContextWithCheckpointBinding(FragmentStepContext, *world.Terrain, uint32, *checkpoint.BindingAuthority)
func (*EffectService) SetFragmentStepContextWithCheckpointBinding(FragmentStepContext, *world.Terrain, uint32, *checkpoint.BindingAuthority)
```

Ordinary SetFragmentStepContext clears its private proof. The admitted sibling
performs that same installation and records exact pool, terrain, tick and
private authority for the retained ports. Nil authority carries no proof.
The service wrapper narrows its known fixed pool; other implementations keep
ordinary forwarding without proof. CheckpointFixedPool returns only the known
concrete nonnil pool and never invokes it. SetBindings requires nonnil context,
service, expected art, terrain, pool and authority, validates exact current
service/art/pool plus port proof, and atomically records one exact tuple;
exact repeats revalidate. Before any fragment context installation its absent
ports are valid. A present port requires proof for that exact pool, terrain,
completed tick and authority. Ordinary replacement and copied pools cannot
inherit proof. Capture revalidates without methods on interfaces, reflection,
interface equality, filesystem reads or callbacks. The frozen input owner
validates art contents once for the complete capture.

Root migrates the sole production installation in bindFragmentStepContext,
which constructs World.HeightAt and fragmentImpactSink{debrisImpactSink{s,tick}}
immediately before effect advancement, supplying those same terrain/tick/authority
operands. The sink can admit new effects and smoke strips and is authoritative.
Its retained tick must equal the completed GlobalTick encoded by section 1;
no duplicate wire word is needed. The actual service art comes from frozen
inputs and controls lifetime/capacity. Existing art, Impact and TerrainHeight
bytes become validated presence. heightAt has no production installations and
keeps its nonnil refusal. All-absent fixture bytes and cheap summaries remain
unchanged. Tests cover coherent enable state, presence vectors, copied pools,
ordinary reinstall, foreign art/terrain/tick/authority, foreign interface owners,
atomic registration, pure capture and unchanged callback/RNG consumption.


#### 16.3.69 U6 movement class and callback bindings

Movement privately stores its Damage and ProductFootprint functions, preserving
their current signatures through ordinary Set<Name>/ <Name>Hook methods and
Set<Name>WithCheckpointBinding(fn, *checkpoint.BindingAuthority). Ordinary
replacement clears that slot's proof. The admitted sibling records exact
System and authority for a nonnil callback after the same installation. Nil
authority carries no proof. Current calls, arguments, results, and callback
bodies remain unchanged. Root owns both production installations in composition:
Damage captures Session through its method value, and ProductFootprint captures
Session and consults its current Catalog. The private authority is supplied
only by the original admitted Session.

```go
// internal/movement
func (*CheckpointContext) SetCompositionBindings(*System, *content.CheckpointKeys, *checkpoint.BindingAuthority) error
```

SetCompositionBindings requires nonnil operands, exact §16.3.64 admitted
constructor System and authority, keys equal to the shared unit context's
Keys, and a class table accepted by keys.ValidateMovementClasses. It atomically
records one exact tuple and revalidates repeats. It agrees with already
registered terrain/path/auxiliary authorities and those registration methods
must symmetrically refuse a conflicting composition authority. Collection and
writing revalidate class membership, object identity and values using the frozen
snapshot; no callback, class rebuild or map iteration is performed. Full frozen
inputs validation remains root's once-per-capture responsibility. Classes stays
public with its existing ordinary SetClasses behavior because identity and all
values can be verified without function proof. Nonempty/present callbacks need
their exact slot proof; copied systems and ordinary getter/reinstall refuse.
Existing Classes, Damage and ProductFootprint bytes become actual presence;
empty all-absent fixtures retain their bytes without registration. Root adds
composition registration to prepareCheckpointMovement, before reference
discovery. Tests cover each slot independently, conflicting aliases/authorities,
changed classes, absent/stale/copy proof, pure capture and unchanged gameplay
callback results. The two callback readers retain their ordinary invocation
sites in cargo cascade and air build approach [06 §12.1][04 R-ORD-02 §2].


#### 16.3.70 U6 combat callbacks and reaction ownership

Combat's ten public Service callbacks and eight ReactionSeams callbacks become
private slots. Their ordinary Set<Name>/ <Name>Hook APIs preserve each existing
signature; ordinary replacement clears only that slot's proof. The admitted
Set<Name>WithCheckpointBinding(fn, authority) performs the same assignment and
retains exact slot owner and private authority for a nonnil function. Nil
authority creates no proof. Ordinary ServiceConfig/NewService and
ReactionSeamsConfig/NewReactionSeams copy initial public fields without invoking
callbacks or initializing a pool. The admitted ReactionSeams constructor copies
the same configuration through its With setters. Existing capacity construction
and invocation order are unchanged.

```go
// internal/combat
func NewReactionSeamsWithCheckpointBinding(ReactionSeamsConfig, *checkpoint.BindingAuthority) *ReactionSeams
func (*CheckpointContext) SetBindings(*Service, *content.SimulationInputs, *features.Service, *world.Wind, *ReactionSeams, *checkpoint.BindingAuthority) error
```

SetBindings requires nonnil context, Service, inputs and authority, records one
exact tuple atomically, and revalidates repeats. Actual Features, ProjectileWind
and Reaction must equal the explicit expected pointers. Each present Service
and Reaction callback requires its independent owner/authority proof. Copied
services or reaction records cannot borrow installations. Unregistered fixtures
keep their all-absent behavior; capture never calls callbacks, rule methods,
lookup caches or presentation sinks. Root validates frozen inputs once; the
private weapon lookup remains the existing content-derived cache, rebuilt only
by weaponLookupFor before use, not during capture.

Root supplies the actual installed combat owner and Reaction object. Of the ten
Service callbacks, InfectionThreat is a package function; VisitOffMapFiled is a
method value capturing the exact movement Grid; all others capture Session and
read its current owners. The eight Reaction closures capture Session except
UnderAttackSilenced, a package function. Root retains exact Session, Combat,
Grid, Reaction and authority at the canonical installation, checks these edges
before registration, and verifies the full session owners separately. Combat's
Events callback is authoritative: it can request shake and append strips, so it
is not omitted as presentation state. Callback code, arguments, ordering and
RNG consumption stay unchanged.

The existing ten callback and two pointer-edge bytes become actual presence.
Reaction retains its presence byte and, when present, eight booleans in logical
field-name order: Allied, ArmConstructionThrottle, ObserverNotice,
PurgeOrdersOnDamage, RetaliationOrder, SlotAcquisitionAdmits, UnderAttackNotice,
UnderAttackSilenced. Absence retains the original single zero. These bytes
represent the optional parts rather than treating a partially installed record
as a complete reaction. Tests cover each slot, presence vectors, exact aliases,
ordinary getter/reinstall, copies, conflicting registration and capture purity.
The existing active-impact, transport-handoff and area-transaction refusals stay.

The package-global paralyze task installation is a separate remaining root
check; this unit must not claim full-session admission until its original
orders installation is also validated. It does not add a second gameplay seam.


#### 16.3.71 U6 Classic manager composition bindings

The nine Manager callbacks (CanPursueAir, IsAlliance, JammerSuppresses,
QueueBuildTyped, RallyProbeKnown, RallyShotTimeAdmits, RallyVisible, UnitVisible
and WeaponMaintenance) become private slots with ordinary Set<Name>/<Name>Hook
APIs and admitted Set<Name>WithCheckpointBinding(fn, authority), preserving
existing signatures. Ordinary setters clear only their slot's exact-owner
proof. Ordinary ManagerConfig/NewManager copy initial public fields without
invoking callbacks or running constructor work. Strategic's existing private
energyEnvironment and rebuildRegistry setters gain independent exact-Strategic
proofs; copying the embedded Strategic cannot transfer them.

```go
// internal/ai
func (*Manager) InitializeBattleStateWithCheckpointBinding(*world.Terrain, RallyBattleBindings, *checkpoint.BindingAuthority) bool
func (*Strategic) BindEnergyEnvironmentWithCheckpointBinding(func() (float32, float32), *checkpoint.BindingAuthority)
func (*Strategic) BindTargetRegistryRebuildWithCheckpointBinding(func(uint32, uint8), *checkpoint.BindingAuthority)
func (*CheckpointContext) SetBindings(*Manager, *content.SimulationInputs, *rng.Simulation, *orders.QueueBinding, *SurvivalInfo, *checkpoint.BindingAuthority) error
```

InitializeBattleState retains its one-shot guards and all initial arithmetic;
the admitted sibling stamps its three rally slots only on the accepted
installation. Rejected reinitialization changes neither state nor proof. Nil
authority produces ordinary behavior without proof. Ordinary Strategic setters
clear their own proof and preserve nil receiver handling.

SetBindings requires nonnil context, Manager, inputs and authority. It retains
one exact Manager/inputs/catalog/RNG/order-binding/Survival tuple per capture
context, atomically and with repeat revalidation. Manager.Catalog and
Strategic.Catalog equal the frozen catalog. Profile remains mutable serialized
state: appliedCatalog may be nil or that same frozen catalog, including after
SetDifficulty clears application state. Do not apply a profile, initialize
Strategic, rebuild tables or rehash frozen inputs during capture. Each present
callback must match its exact Manager or embedded Strategic and authority.
Absent unregistered fixtures retain their current bytes and behavior.

Root records each original manager in its actual session slot and verifies the
Session-owned current operands before registration. The canonical closures
capture Session, and QueueBuildTyped additionally captures the manager. Three
rally functions arrive through the existing one-shot initializer. The
WeaponMaintenance, CanPursueAir and registry-rebuild installations retain their
existing only-if-absent timing. The headless observation wrapper's ordinary
QueueBuildTyped replacement invalidates admission; it is not silently accepted
as a transparent callback. Survival is nil or the exact scenario info for a
survivor; root validates membership and keeps the attacker absent. The scenario
owner already serializes that info, so Manager writes only validated presence.

Existing Manager callback/catalog/RNG/order/Survival tags, Strategic catalog and
two callback tags, and Profile.appliedCatalog tag become actual presence, with
all mutable payload unchanged. This unit leaves Ext and registered Modern
planner admission to the subsequent closed-host integration; it must not accept
an arbitrary ControllerCheckpointProvider or change worker handling. Tests cover
independent slot replacements, copied managers/Strategic values, foreign aliases,
atomic conflicts, failed rally initialization and capture without callbacks,
RNG draws or profile application. Existing active-step and stateless rule
validation remains in force.

#### 16.3.72 U6 static paralyze task installation

The package-global paralyze task callback remains a static composition seam
([06 §10], [06 R-DMG-01 §11]); it is not a new gameplay policy or session value.
Make its storage private. The single installing API assigns the function and
returns an opaque nonzero-size installation receipt for a nonnil callback;
every replacement, including the same extracted function, has a new identity.
Installing nil clears the receipt. A getter returns only the function. Orders'
existing initializer retains its original receipt privately when installing
PushParalyzeCredit. Root asks orders to validate that original receipt before
full combat/session admission; validation never invokes or reinstalls the
callback. A copied or forged receipt, absent callback, or later replacement
refuses. This build-wide invariant adds no payload byte. Standalone combat
fixtures keep ordinary callback behavior and may restore their private storage
and receipt in cleanup; no public restore or receipt getter transfers admission.

```go
// internal/combat
func SetParalyzeTaskPush(func(*units.Unit, uint32, uint32)) *CheckpointParalyzeTaskInstallation
func ParalyzeTaskPushHook() func(*units.Unit, uint32, uint32)
func ValidateCheckpointParalyzeTaskInstallation(*CheckpointParalyzeTaskInstallation) error
// internal/orders
func ValidateCheckpointParalyzeTaskBinding() error
```

#### 16.3.73 U6 Modern AI immutable table provenance

BuildTable retains a private detached snapshot after its existing final fighter
classification (and at its nil-catalog return). This changes no classification,
ordering, numeric arithmetic, RNG or rule calls. The snapshot records exact
Table self-pointer and source catalog, original closed construction-rule tag
(nil 0, Strict 1, Community 2, Modern 3), and all stored table values. Unknown
rules continue ordinary construction but mark checkpoint provenance unsupported.
The original rules need not equal a later manager rule selection: mode changes
may retain the existing table. No rules interface is retained for invocation.

```go
// internal/aikit
func (*Table) ValidateCheckpointBindings(*content.Catalog, *content.CheckpointKeys) (uint8, error)
```

Validation requires nonnil operands, original table/catalog identity, admitted
unit/feature identities and exact snapshot agreement, returning the original
rule tag. Root still validates frozen content once per complete capture. It
never calls BuildTable, tableFor, Shared.Value, a lookup/rule method, worker
code, filesystem, reflection or sorting, and never traverses a live map.
Compare map lengths and value/presence lookups against private snapshot rows
formed during construction's ordered unit walk. Preserve nil versus empty
collection shape. Capture must not repair or replace the snapshot.

Snapshot Units and Capped in order with original UnitInfo pointers; complete
byKey, byDef and defensiveFeatures membership (deduplicated constructor keys);
and every UnitInfo scalar, string, Def/FinishedFeature pointer and private
capSlot. Copy each Builds slice independently and preserve its ordered pointers
and duplicates. UnitInfo is not merely content metadata: its identity is used
by Buildable and its derived values are read during command application.
All snapshot storage is private validation metadata, not another graph table.
A copied Table, changed collection membership/order, changed UnitInfo value or
build edge, foreign definition, unsupported rules or foreign catalog refuses.
The later executor integration replaces its existing table tag with presence
plus this original-rule u8 only when present; absent fixture bytes stay intact.
Tests cover each surface, original/current rule divergence, source copies,
capture purity and unsupported-rule ordinary behavior. Host/controller
admission and the opaque Session-to-AI bridge are a separate following unit.

#### 16.3.74 U6 immutable mission provenance

The admitted mission pointer alone cannot prove unchanged authored inputs:
deferred Community placement rereads its unit records, and later lava, score
and meteor consumers read retained OTA values. Snapshot immutable mission and
OTA/TDF values at the existing pre-compose admission point; validate after
composition before ready and once during full-capture preflight. Do not parse,
call typed getters, resolve lookup indexes, read files or rebuild content at
capture. This validation adds no wire payload and changes no mission behavior.

```go
// internal/mission
func SnapshotCheckpointInputs(*Mission) (*CheckpointInputs, error)
func (*CheckpointInputs) Validate(*Mission) error
// formats
func SnapshotCheckpointOTAInputs(*OTA) (*CheckpointOTAInputs, error)
func (*CheckpointOTAInputs) Validate(*OTA) error
```

Mission owns the exact original pointer, Type, TerrainKey, selected Schema
(Name and StartPositions), ordered Units/Specials/Features with every stored
field, WindBounds, UseOnlyPath, IsRestore, CampaignPath/Index/MissionName and
Difficulty. Clone slices preserving nil presence, length, order and values;
backing-array identity and capacity do not matter. Victory/Defeat and all
trigger fields remain mutable payload under §16.3.38, not immutable snapshot
members. The private load-order diagnostic is excluded. Campaign and restore
remain unsupported admitted battle kinds, regardless of this snapshot.

Formats owns exact OTA, Document and Section pointers and retained topology:
OTA's scalar strings and ordered complete OTASchema values; Document.Root,
OTA.Global and every nested Section edge; Section Name, OriginalName, ordered
Items, private resolvedBuilt and resolved index words; every Item Kind, Key,
OriginalKey, Value and Section edge. Line/Column source locations are excluded
from both Item and Section. Preserve nil/empty distinctions. The raw authored
tree contains both presentation and future simulation values; freeze it as a
whole instead of introducing a key whitelist. Parsed production sections
already have resolved indexes. An unresolved authored fixture may be recorded
as-is; any later lazy resolution is a detectable mutation, never performed by
snapshot or validation.

Snapshot uses a visited-pointer set and fixed ordered record list, allowing
shared edges without unbounded traversal. Validation walks only that detached
list, comparing current fields and edges directly, so newly inserted cycles or
replacement nodes refuse without following them. Snapshots own no mutable
source slice. Nil optional OTA/document/section edges preserve their actual
presence; nil snapshot/mission must fail where the root requires a mission.
Tests cover each stored field group, duplicate assignment/index mutation,
reordered schemas, pointer replacements/copies, cycle insertion, allowed
trigger progress and diagnostic edits, and allocation-free repeated validation.
Root supplies real skirmish/Survival entry and deferred-placement evidence.

#### 16.3.75 U6 closed Modern planner and controller bridge

Move §16.3.44's generated planner witness into an opaque ai-owned value, so
Manager's writer can consume the same proof as Session. Keep its existing
registration-time concrete, zero-size struct check; capture uses only the
resulting concrete assertion, never reflection or planner/marker methods.

```go
// internal/ai
func NewCheckpointModernPlanner[T ModernAIStep](T) CheckpointModernPlanner
func (CheckpointModernPlanner) Matches(Planner) bool

type CheckpointControllerOwner interface {
    ControllerCheckpointProvider
    EnableCheckpointApplications(checkpoint.Identity, *content.CheckpointKeys) error
    ValidateCheckpointBindings(*Manager, *CheckpointContext) error
}
func NewCheckpointControllerSource[T any, P interface { *T; CheckpointControllerOwner }]() CheckpointControllerSource
func (CheckpointControllerSource) Valid() bool
func (CheckpointControllerSource) EnableApplications(*Manager, checkpoint.Identity, *content.CheckpointKeys) error
func (CheckpointControllerSource) WriteCheckpoint(*Manager, *checkpoint.Encoder, *CheckpointContext) error
func (CheckpointControllerSource) AppendSummary(*Manager, *checkpoint.Summary) error
func (*CheckpointContext) SetModernBindings(*Manager, Planner, CheckpointModernPlanner, CheckpointControllerSource, *checkpoint.BindingAuthority) error
func (*CheckpointContext) ValidateModernManager(*Manager) error
// internal/aikit
func CheckpointControllerSource() ai.CheckpointControllerSource
func (*Host) ValidateCheckpointBindings(*ai.Manager, *ai.CheckpointContext) error
// internal/session
func RegisterModernAIWithCheckpointBinding[T ai.ModernAIStep](T, ai.CheckpointControllerSource)
```

The controller source accepts no callback argument. It seals generated adapters
that first assert Ext to exactly P and reject typed nil, then invoke only the
retained concrete owner's diagnostic methods. Pointer identity comparisons also
narrow first; arbitrary noncomparable values, embeddings, provider-only objects
and RetailTimer refuse without calls. The one reviewed production instance is
[Host, *Host], returned by aikit and passed by mods/aikit at the existing Modern
registration. Session retains step, opaque witness and opaque source together
under the existing registration mutex. Ordinary registration carries neither
proof. No second registry, dynamic capability selection or Session import of
its controller implementation is introduced.

Each AI capture context belongs to one Manager and shares the lower unit/world
contexts. SetModernBindings stages one exact manager/authority/witness/source,
expected registered planner, current optional controller pointer, history and
keys tuple. Require Modern controller kind, valid player, both planner values
accepted by the witness, matching shared keys and an enabled successful inactive
history belonging to that player/kind. A nil Ext is valid before lazy Host
creation; nonnil requires the closed source. Verify any existing §71 authority
agrees, and §71 registration must symmetrically reject a conflicting authority.
Modern registration may precede §71 registration, avoiding a circular admission
requirement. Repeats revalidate the same tuple; copied context expectations
cannot authorize a changed owner. ValidateModernManager rechecks these direct
aliases without invoking the Host validator recursively. SetModernBindings then
calls only the narrowed Host validator against the staged context and commits
on success. Full Manager validation also revalidates its registered Modern row.

The existing stateless Planner tags remain 0–2; the registered Modern step is
3. Manager.Ext is absent 0 or the admitted Host 1, retaining the existing byte
position. Host payload remains the separate controller fragment. Source full
writing requires a present exact registered owner and preflights before any
bytes; source summary narrows the present owner and preserves its selected-only
summary boundary without a table walk. EnableApplications requires Modern
controller kind and handles absent Ext
through the existing Manager method and present Ext only through the closed
Host method. It remains an entry-time diagnostic operation, never a capture
operation. Fixture payloads keep their absent bytes; Host full-capture fixtures
must now supply explicit typed registration.

NewHost retains private exact self and manager provenance unconditionally,
because entry priming can precede checkpoint enabling. Its current manager and
Ext must still identify that Host; history must be the manager's exact history
and pass ValidateModernManager. Existing attachment after completed entry also
checks constructor provenance. In begin, immediately after the existing
executor assignment and before worker preparation, retain exact executor,
manager, table, catalog and terrain aliases. Before initialization these are
absent; afterward validate the exact original aliases and §73 table snapshot.
The executor's existing manager byte becomes presence. Its table byte becomes
presence followed, only when present, by the original construction-rule u8.
Current manager rules may differ from the table's original tag. Full Host
validation occurs before its controller/persona bytes. No code here reads kit,
obs, mapInfo, brain, rand, batch, ready, flight, Shared or worker readiness,
including their pointers. Cheap summaries keep their current selected fields.

Tests cover foreign/copy/typed-nil owners, all registration conflicts and both
orders, original/current rule divergence, active/failed history, aliases and
presence framing, unsupported provider purity, capture purity and active-worker
race safety. AI bridge, aikit Host integration and root registration are separate
exclusive-file units; §73 Table validation is the Host unit's prerequisite.

U6 integration evidence (2026-10-07): the stable owner-binding candidate
`9ba809ba4` passed whole `tools/check` and short `tools/check-retail`,
including amd64 fingerprint locks and GPU device fixtures. Subsequent Classic
AI composition retains the original Session/Manager/authority tuple and uses
the admitted callback setters at their existing installation points. The
retail admission fixture now collects and writes AI, world, features, visibility
and economy together with units, orders, movement, path, construction, combat
and effects, at entry and after 300 ticks, in Strict/Modern/Community skirmish
and Survival. All six cases pass. These are owner-fragment checks; the Session
capture lifecycle and playability gate remain outstanding.

The displayless Classic scene at candidate `3219b9a6` retained its prior
initial/warm/final fingerprints, complete census and both RNG draw counts
(15,645 simulation; 985,036 CRT). Median tick cost was 1.646 ms, p95 2.708 ms,
103,638 bytes and 908.37 objects per tick with no measured GC. Artifacts are
outside the repository under `/private/tmp/nanolathe-m3-u6-inputs-classic`;
the comparison is `/private/tmp/nanolathe-m3-u6-lifecycle-classic`. Shared-host
timing differences are observations, not a speedup claim.

#### 16.3.76 U6 materialized terrain input provenance

Frozen source bytes do not prove that their loaded terrain copies remain
unchanged. Heights and their derived floor bounds are immutable during a
battle: there is no terrain deformation [03 R-TERR-01 §3]. The LOS terrain
words are likewise built once [03 §3.5]. Validate these loaded inputs without
changing the world payload or introducing a late snapshot operation.

```go
// internal/world
func (*Terrain) ValidateCheckpointInputs(*content.SimulationInputs, string) error
```

The string is the original mission terrain key, not the display MapName.
At the successful end of world.Load, after LOS construction, feature stamping
and the first void fixup, privately retain the exact Terrain receiver,
filesystem, catalog, requested terrain key and resolved TNT logical path.
Snapshot CellW/CellH/Version, row-major plot height/min/max triples, SeaLevel,
Gravity/AuthoredGravity/OTAGravity, LavaWorld, WaterDoesDamage/WaterDamage,
WindMin/WindMax/Tidal, PlayRight/PlayBottom, losWords and losBuildCount.
Preserve the exact float32 Tidal bits, including signed zero; refuse NaN.
Validation requires the input set's CheckpointFilesystemMatches, exact Catalog
identity, selected MapFiles TNT path and the expected requested terrain key.
It compares detached values without reads from the filesystem, callbacks,
loaders, recomputation, map traversal or allocation on success. Copies and
hand-built terrains lack the original load proof and refuse this production
admission API; existing lower-owner fixtures retain their existing behavior.

Occupancy, metal, feature/sentinel, anchor/damage word, retained flags,
metalSeeded and ordered FeatureNames/FeatureDefs remain in the mutable payload.
Movers and class-restamp retain their existing binding proof. Renderer-only
tiles/pixels, static-obstacle diagnostic revision and entry undo metadata stay
excluded, with existing completed-entry checks still required. ApplySchema
and mission feature replay happen after the snapshot and change only retained
mutable values; replay restores the same playable insets.

Session retains the original loaded Terrain pointer and mission terrain key
in admission metadata. Require the same World and validate this snapshot
before readiness and at each full capture. Test copied/foreign source owners,
scalar and geometry/LOS mutation, same-area dimension changes, allowed mutable
plot edits, exact float bits and repeated validation purity. The world unit
owns terrain.go's private field/load hook and new provenance files; Session
integration is a separate root-owned unit. Correct stale deformation wording
in terrain.go and terrain_lifetime_test.go to match the cited established
contract, without changing fixture behavior.

#### 16.3.77 U6 remaining cheap summary leaves

The §16.3.7 signature is implemented on units.World, orders.Queue, cob.VM,
cob.AimSlot, features.Service and world.Terrain. These selected-only walks
use a local Summary copy and commit on success, allocating nothing and
invoking no callbacks, getters that synthesize values, graph collector or
canonical writer. Missing receiver/accumulator and selected NaNs refuse.

Unit words start with physical slot count (slot zero included). An empty slot
contributes tag 0 only. A freed residual contributes tag 2, raw Handle, Owner,
Remaining bits; its Kills is a deliberate full-only blind spot. A live slot
contributes tag 1, Handle, AllocationSerial, Owner, Health, Remaining bits,
Flags, X/Y/Z, Move.Heading/Speed, Pending, Stunned and ParalyzeExpire. For each
of its fixed weapon slots append Reload, Ammo, then AimSlot's IssueBit, Ready,
raw private readyWord; never call ReadyWord(). After slots append live then
created counter, paired per player. Signed fields extend their source type.

Queue words are primary length and selected nodes, then secondary length and
selected nodes, then lastPumpTick. Each node contributes ID, Phase, Target,
GoalX/Y/Z, Deadline, Param1/2/3 and Flags in that order. The existing retained
queue traversal is used with malformed topology refusal; detached nodes remain
a full-only blind spot. VM words are its eight physical threads in order:
Status, PC, SP, Sleep, WaitPiece, WaitAxis, WaitThread, SignalMask, all 32 Stack
words; then statics length/values, activeThreadCount, nextIdentity and the eight
threadIdentity values. Session appends presence and these fragments only for
physical live unit slots, without a reference graph or a slot-list length.

Section 5's cheap order is features then terrain: feature instance count,
reproduction cursor, arenaHeld; then plot length and each row-major cell's
OccupantA/B, Metal, Feature, AnchorWord and FlagByte with the same excluded
bits as the full world writer. Counts include stored records without resolving
content or following the feature lookup map. Unselected geometry/feature
progress, VM instruction/callback data and strings remain blind spots.

Independent literal word vectors, retained/excluded mutations, selected NaNs,
malformed queues and allocation checks lock these refinements. These methods
observe existing state and introduce no new simulation state or gameplay rule.

#### 16.3.78 U6 capture lifecycle composition

Pump metadata starts at zero at Enable and increments for each ExecuteStep
whose plan Runs, including zero-tick pumps. ConsumedInput is a diagnostic ordinal
of drained queue elements passed to applyHumanCommand after Enable, local or
stamped, including refusals and accepted no-ops. It is not an enqueue sequence
or the relay stream position; M4 records the mapping to its input stream.
Paused consumption advances the ordinal but produces no checkpoint. Overflow
fails diagnostics without changing command execution. These counters never
enter canonical bytes. Interior capture follows publication when another tick
will actually execute; the actual last executed tick follows the retail tail.

Full capture validates admitted inputs once, seeds physical allocations first,
then repeats owner collectors in section order until no additions. Sections
1–13 are present for an admitted complete battle. World contains terrain then
features presence/payload; visibility contains service presence/payload then
the session tail. Effects uses §16.3.17's five-fragment presence framing. The
computer fragment is ten fixed slot rows, each manager presence and, when
present, Manager payload, ApplicationHistory payload and controller presence;
a present Modern controller contributes its Host payload. Classic controllers
are absent; a lazy, absent Modern controller still has its enabled history.
Scenario follows the ten rows. History player/kind must match the manager.
Cheap computers append manager presence/summary then either the present Host's
eleven selected words or history's five words and six zero scheduling/APM
words for an absent controller. No history fields are hashed twice there.

Enable requires the original completed admitted entry, initialized RNG, no
executed tick, and a successful opening publication already made by the host.
A sticky admission flag distinguishes a wrapped runtime tick zero from entry.
It does not publish or step the world. Keys and the content/configuration
identity are retained once. Attach application histories using the existing
Classic/closed Modern entry APIs, then validate and capture the complete entry.
On failure, stop each history attached by this attempt. Disable likewise stops
those histories through their existing sticky Fail operation, making NextSerial
and BeginAttempt return before any codec/hash work. It clears Session rings and
pending sinks; a pending result receives an explicit cancellation error.
No controller, worker or gameplay state is reset. A stopped chain cannot be
restarted on that Session, including a battle with no computers: repeated Enable
refuses any earlier enable attempt that reached history attachment,
consistent with §24's no-reset contract. Fresh admitted entry is required.

Track active pump/capture scopes to refuse recursive capture API calls and
recursive Step/ExecuteStep from a sink. Scope cleanup also runs when a sink panics; the panic propagates and an
incomplete diagnostic result remains, without blocking later gameplay.
Capture errors affect only diagnostics;
failed full captures/selected summaries append neither tick row nor cadence
record. Requests at non-cadence boundaries update the result only. Validate
successful publication by the frame buffer's existing PublishedTick metadata
and the event buffer's empty staged window, without using presentation contents
as authoritative state. Disabled pumps do no checkpoint traversal or hashing.

FixedEffectPool.CheckpointCounts() (records, fragments int) supplies the ring's
pool counts by direct record length and live geometry-slot flags, without
copying presentation metadata. Session reuses its thirteen Summary accumulators
so the closed controller adapter does not allocate a fresh escaping accumulator
on each tick; no scratch state enters the ring or canonical stream.

#### 16.3.79 U7 measurement composition

The existing displayless simulation benchmark gains an opt-in
`SimBenchOptions.Checkpoints bool`, exposed as `--sim-benchmark-checkpoints`.
Its zero value keeps the existing entry and workload. Enabled runs use the
same scene, seeds, per-player resources, rules and content through the M2
single-seat admitted constructor, stage the same armies before any tick,
publish the opening, then EnableCheckpoints once. Unsupported admission is
reported, never bypassed; in particular the three-computer scene cannot use
Strict's online one-computer-per-human limit. Matching Modern scenes supply
the initial enabled/disabled cost comparison. The benchmark's fixture room
uses profile `retail`, fixed participant identity 1 for the passive human,
player view 1024..2048 without full-map and spectator/replay view 256..2048
with full-map; revision/scheduling/pacing/drop/audience are 1, cumulative
grace 90000 ms and audience delays zero. These fixture values are explicit
configuration inputs, not a live room or a new default.

The host checks the value-only capture result after each step and fails a run
if diagnostics failed. The report adds `checkpoints` and optional
`checkpoint_records`, `checkpoint_ticks`, `checkpoint_tick` and
`checkpoint_digest` (the retained latest full SHA-256 in hexadecimal), read
once after the measured window. A report never claims capture is enabled if
only a partial fingerprint ran. No wall-clock reader enters Session. Existing
per-tick/CPU/allocation series measure enabled cadence against the ordinary
scene; focused retail benchmarks within session additionally measure one
complete digest-only writer, byte capture and the selected row independently.
Those benchmarks use already admitted, warmed state and never include entry
or setup in the measured operation. Reports keep bytes and allocations as
measurements, not correctness thresholds. Numerical acceptance budgets will
be recorded after exploration and before the final acceptance runs (M3-C9).


The benchmark census reads `units.World.DeathDispatches() uint64` instead of
wrapping the admitted death callback. This diagnostic count increments once
immediately before the existing primary death-hook dispatch in FinalizeDeath;
it is not an allocation/free/loss counter. A host takes a starting value and
subtracts it at census time. It invokes no observer, changes no death decision
and is excluded from the unit stream and summary (no simulation reader).

#### 16.3.80 U7 bounded history comparison

`session.CompareCheckpointHistories(a, b CheckpointHistory)
(CheckpointComparison, error)` compares detached history values only; it never
reads a Session, runs capture or claims equality from a selected summary.
Inputs are bounded to the published 64 records/600 rows. Runtime rows have
interior/final boundaries; digest records additionally permit entry. Refuse
duplicate positions or repeated tick labels within either retained sequence,
invalid boundaries, and records/rows in nonchronological tick order (unsigned
forward delta must be nonzero and below half the u32 range). Entry is optional
and appears first at tick/pump/input zero. Pump ordinals never decrease and
input ordinals never decrease; runtime ticks within a pump can wrap normally.
The two peers must already have equal content/configuration identities;
history alone has no admission identity and this API does not invent one.

Only an exact `CheckpointPosition` match is comparable: tick, boundary, pump
ordinal and consumed input position. Incompatible pump/input schedules are
not diagnosed as a simulation divergence. Preserve retained sequence order,
including u32 tick wrap, and never sort labels numerically. A bounded nested
scan suffices; no pointer/map key or host clock participates.

```
type CheckpointDifference struct {
    Position CheckpointPosition
    Owners [CheckpointOwnerCount]bool // array index + 1 is the section ID
    RNG, Pools bool // tick-row evidence only
    Full bool // full digest differs; record evidence only
}
type CheckpointComparison struct {
    ComparedTicks, ComparedRecords int
    TickDifference, RecordDifference *CheckpointDifference
    MayPredateTicks bool
    UncoveredOwners [CheckpointOwnerCount]bool
}
```

The first differing common row compares each selected `(Words, Sum)`, both
RNG states/draw counts and all six pool counts. The first differing common
record compares the full digest and all owner digests. Both results are kept:
a full digest is evidence of divergence even when the summaries have a blind
spot. `MayPredateTicks` means the first common row already differs; the
retained data cannot establish onset before it. Zero ComparedTicks or
ComparedRecords explicitly reports no comparable window of that kind.
`UncoveredOwners` marks each owner whose full digest differs at a common
record while that owner's selected summary is equal in a common row at the
same position; it does not infer a hidden fault location between samples.
No difference pointer means only that the available comparable evidence was
equal, not that the whole worlds were proved equal.

U6 evidence: the fast whole-tree gate passed at 09864cac, and real Classic
lifecycle tests cover requested/cadence capture, zero/catch-up/shortened pumps,
input drain positions, disable/restart, output failures/reentry/panic and tick
wrap. Real Modern controller tests pass in all six reserved-mode/scenario
combinations, with per-tick RNG and gameplay unchanged by capture frequency.
The earlier staged mission/OTA provenance constructors and two absent-executor
writer adapters remain in the generated production deadcode baseline because
admitted construction has no shipped host until the staged replay/measurement
entry; owner writers newly reached through ExecuteStep were removed from it.
These facts do not claim U7 cost, native-platform or host-kind acceptance.

#### 16.3.81 U7 encoding cost without schema changes

Exploratory profiling found diagnostic path formatting in dense terrain,
occupancy, capacity and projectile arrays dominates allocation. Encoder adds
`FieldChild(prefix, name string)` and `FieldIndex(prefix string, index int,
suffix string)`: their error paths are respectively `prefix + "." + name`
and `prefix + "[" + decimal(index) + "]" + suffix`. They retain the parts and
format only on failure. Calling ordinary Field clears any retained parts.
These methods emit no bytes and keep sticky errors unchanged. Dense writers
use them where they reproduce exactly the existing logical error path; a nil
row needs no formatted nested path until a present payload writer uses it.

The two SHA-256 destinations share one bounded 4 KiB buffer, which copies
each primitive once and fans out complete chunks to both hashes. It is flushed
before closing each section; the owner identity domain and the full header
are written directly to their respective hashes before that section begins. A caller's output sink
remains synchronous and unbuffered, preserving immediate write/short-write
failure and its active logical field. This changes neither bytes, section
boundaries, hash domains, validation, nor simulation state. Existing literal
vectors and writer-error tests, plus paired retained checkpoint digests,
remain correctness gates. No full snapshot is retained to hash it.


The content semantic-digest helper also batches its existing byte stream into
one 512-byte scratch buffer before SHA-256 writes. This applies to immutable
program validation at capture as well as entry compilation; it does not cache
validation results or skip checking a mutable program. Strings copy directly
into that buffer. All existing varints, string lengths, float bits, domains and
ordering stay unchanged, and sum flushes any tail before returning a digest.


#### 16.3.82 U7 portable scene and live fault evidence

The asset-free `TestCheckpointPortableScript` authors an in-memory HPI containing
TNT/OTA, two weaponless commander definitions, one model, returning COB scripts,
visibility masks and minimal game tables. The real compiler, freeze and admitted
constructor compose two independent copies in each reserved mode. Thirteen
explicit pumps produce 30 ticks with one-to-five-tick batches, paused Stop, empty
accepted commands and zero-tick input retention; requested byte captures and
cadence records remain distinct. The test compares complete bytes, all thirteen
present owner digests, both RNG histories and exact pump/consumed-input positions.
Its 507 logged identity/checkpoint/owner rows are also compared between native
Darwin/arm64, Linux/amd64 v1/v3 and Windows/amd64 CI jobs. Merely passing two copies
on one machine does not establish that native gate. This small scene covers
entry, movement, input and pump boundaries, not busy combat or every content
record; retail owner tests and the busy benchmark supply separate coverage.

`TestCheckpointLiveFaultDiagnosis` injects a single int32 health decrement before
tick 17 in one admitted copy. Two detached histories, retained after disabling
capture, identify the first selected units difference at 17 and the first full
units difference at 30 in all three modes. The prefix through tick 16 and entry
record agree, RNG states/draws remain equal through tick 60, and the second full
record retains the divergence. The comparison does not inspect either world.

Local Darwin/arm64 checks pass. Native CI evidence remains pending until the
branch's matrix and cross-artifact comparison run; no windowed-host result is
claimed here. The actual window recording/headless replay gate belongs to M4
and the perspective-dependent host differences remain M5 work (§17).


The admitted benchmark now reaches the prior staged entry/provenance functions
from a shipped host, so their generated deadcode entries are removed. Five
comparison entries (the public comparator and its four private helpers) remain
staged for M4's headless replay/desync-bundle host, with detached-history and live
fault tests already exercising them. The obsolete movement layer convenience
writer is removed rather than retained in that baseline.

#### 16.3.83 U7 cost acceptance limits

The following limits are fixed before the final acceptance measurements, after
§16.3.81's exploratory profiling. They are diagnostic engineering limits for
M3, not M6's input-latency or impaired-network acceptance. Do not adjust a limit
to turn a failed run green. A failure needs an implementation change or a
reported remaining gate. Timing tests remain opt-in measurements, not flaky
wall-clock assertions in the correctness suite.

Reference environment: Apple M3 Pro, 18 GiB RAM, Darwin/arm64, Go 1.27.1.
The fixed-state probes use GOMAXPROCS=4; the sequential simulation benchmark
uses GOMAXPROCS=2 and its shared benchmark lock. Record other active gates,
scene metadata, census, complete digests where enabled and both RNG histories.
Do not compare different workloads. Run the ordinary and moving/building
fixed-state probes once each, the controller probe in all three barrier states,
and two alternating pairs of disabled baseline/candidate simulation runs,
followed by two enabled runs. Use the median process CPU for comparisons; each
enabled run must satisfy the absolute limits.

| Measurement | Limit and reason |
|---|---|
| Fixed-state digest, ordinary tick 900 and busy-modern-classic-v1 tick 300 | At most 50 ms/op: at the 30-tick cadence, at most 1.67 ms of amortized tick work for these scenes. |
| Same states, byte capture into a reused buffer | At most 66.667 ms/op (two normal tick periods), including synchronous byte delivery. |
| Selected row in those states | At most 1 ms/op and zero allocations; with the digest limit the recurring diagnostics stay below one tenth of a normal tick period on these scenes. |
| Fixed-state writer allocation | At most 8 MiB/op and 200,000 allocations/op for both digest and reused-buffer capture; canonical bytes at most 8 MiB on these fixed fixtures. Counts guard against restoring eager diagnostic formatting; they do not constrain a different map or army. |
| Retained 64-record/600-row history storage | At most 256 KiB, excluding a caller-owned requested byte buffer; no whole checkpoint is retained by the histories. |
| Modern controller preparation outstanding, think outstanding and completed before deadline | Identical canonical bytes, selected summaries and allocation count across states; full writer at most 33.333 microseconds/op (one thousandth of a normal tick), 8 KiB/op and 256 allocations/op; summary zero allocations. Outstanding means barrier-held work, not a CPU-contention benchmark. Capture must not wait for that work or cross a reaction deadline. |
| Disabled capture, scene 1, three 250-unit armies, Modern/Classic, seeds 7/7, 1,200 warmup + 300 measured ticks | Candidate median process CPU no more than 5% above pre-instrumentation `202ff9595`; bytes/tick and objects/tick no more than 1% above it. Match all scene metadata, census and fingerprints. |
| Enabled capture on that same large scene at the initial cadence | Process CPU at most 11.111 ms/tick (one third of the 30 Hz tick period), p95 elapsed tick at most 11.111 ms, and maximum elapsed tick at most 166.667 ms (five tick periods). Allocation at most 2 MiB/tick. This reserves average CPU capacity but explicitly permits a visible checkpoint hitch; M6 must set and meet its own interactive latency bound before offering multiplayer. |

The busy fixed-state fixture uses the ordinary admitted two-seat Ashap Plateau
entry and adds 67 units to each seat before opening publication. Six factory
queues and 88 ground move orders use the ordinary allocator, movement setup
and order queues. At tick 300 the probe requires at least 100 live units,
moving units, active routes, nanoframes and build orders. It verifies that
measurement changes neither RNG stream, canonical state, selected row, census
nor retained histories. This supplements the larger benchmark's combat and
feature workload; it is not a substitute for it.

Final local measurements are recorded in §16.3.84. Native platform comparison
remains the separate M3-C8 gate in §16.3.82.


#### 16.3.84 U7 final local cost evidence

The limits in §16.3.83 were committed at `47dfcf776` before these runs.
That candidate passes every local cost limit on the stated M3 Pro environment.
The fixed-state measurements report elapsed encoding time; the whole-battle
comparison reports measured process CPU as well as elapsed tick percentiles.
These measurements preceded this task's whole-tree gates; no concurrent gate
was reported by the coordinated builds.

| Fixed state | Operation | ms/op | Bytes/op | Allocations/op |
|---|---|---:|---:|---:|
| Ordinary, tick 900 | Digest | 28.887 | 3,262,938 | 80,828 |
| Ordinary, tick 900 | Reused byte buffer | 37.290 | 3,263,716 | 80,856 |
| Ordinary, tick 900 | Selected row | 0.682 | 0 | 0 |
| Moving/building, tick 300 | Digest | 41.730 | 6,279,004 | 152,195 |
| Moving/building, tick 300 | Reused byte buffer | 52.093 | 6,279,836 | 152,224 |
| Moving/building, tick 300 | Selected row | 0.777 | 0 | 0 |

Canonical sizes are 5,237,959 and 6,262,176 bytes respectively. The retained
history is 198,240 bytes. The busy state has 142 live units, 85 moving units,
41 active routes, six nanoframes, seven build orders, six effects and 351 strip
objects. Both RNG streams, complete canonical state, selected row, census and
retained histories remain unchanged by each measurement series.

The Modern controller's 879-byte leaf takes 5.790, 5.721 and 5.717 microseconds
in preparation-outstanding, think-outstanding and completed-before-deadline
states. All three retain identical bytes and summaries and use 7,689 bytes and
227 allocations per write. Summaries take 20.07–20.33 ns with zero allocations.
The barrier-controlled probe confirms capture does not join outstanding work;
it does not claim performance under CPU contention or a whole Modern-AI army.

Two alternating pairs compare pre-instrumentation `202ff9595` with the
candidate with capture disabled. Median process CPU is 1.806 versus 1.817
ms/tick (+0.63%). Allocation is 103,295 versus 103,701 bytes/tick (+0.39%),
with 908.372 versus 908.378 objects/tick. These short samples meet the stated
regression bounds; they do not establish a speedup.

The two enabled runs measure 9.578 and 9.324 ms of process CPU per tick;
p95 elapsed ticks are 6.111 and 6.195 ms, with maxima 147.373 and 149.209 ms.
Both allocate approximately 1,167,745 bytes/tick and 25,718 objects/tick.
The visible once-per-30-ticks spike remains a limitation for M6's interactive
latency work, even though these M3 diagnostic limits pass.

All six large-scene runs have identical complete scene/configuration/catalog
metadata, initial/warm/final partial fingerprints, both final RNG draw counts
(15,645 and 985,036), and every census row. They end with 734 live units,
67 projectiles, 262 effects, 112 fragments, 1,175 strips, 6,213 features,
12 burning features, 11 active builds and 44 deaths. The two enabled runs
also agree on the complete checkpoint at tick 1,470:
`02561516467458d8380f8349a6e2509d86e8a5544c1a4983eab14b2ca1aa5919`.
The final partial fingerprint is `partial-v1:5762eecdf523bf33`.

Raw local evidence is outside the repository under
`/private/tmp/nanolathe-m3-u7-final-{baseline,candidate,enabled}-{1,2}/`,
`/private/tmp/nanolathe-m3-u7-final-cost.log` and
`/private/tmp/nanolathe-m3-u7-final-worker-cost.log`.
The temporary baseline worktree was archived after the comparisons.

A further local architecture check executes the complete portable scene on
native Darwin/arm64 and Darwin/amd64 v1 under Rosetta. All three reserved modes
pass and all 507 ordered identity/checkpoint/owner rows agree. Raw JSON is in
`/private/tmp/nanolathe-m3-u7-portable-darwin-arm64.json` and
`/private/tmp/nanolathe-m3-u7-portable-darwin-amd64-rosetta.json`. Rosetta is
additional architecture evidence, not the required native Linux/Windows gate.

Both whole-tree gates, `tools/check` and `tools/check-retail`, pass at the
measured candidate. The retail gate includes the unchanged amd64 fingerprint
locks under Rosetta and the real-device GPU fixtures. Native platform comparison
is pending the branch's CI run; no M3 completion, window/headless replay
equivalence, or multiplayer play-test readiness is claimed. The branch stays
unmerged to main until play testing.


### 16.4 Two-client play-test slice (user-authorized 2026-10-07)

The immediate outcome is two local window clients controlling different human
seats in one fixed Modern skirmish through a minimal local relay. Reuse the M2
command/configuration identities and the small periodic unit checksum below.
Complete replay files, native Linux/Windows acceptance, AI seats, Survival,
polished lobby screens, public room codes, spectators and reconnect are outside
this first play test. They remain in the milestones above. This changes delivery
order, not the approved simulation rules or online departure policies.

The prototype must not hide disagreements by forcing both clients to use one
human's authoritative perspective. Both replicas simulate both human seats;
presentation selects the local seat. Required work is the §6 subset exercised
by those seats: owner-relative visibility/targeting, per-seat settlement and
end state, and host-independent effect admission. Keep the ordinary single-seat
path and fingerprint locks unchanged. Unsupported prototype configurations and
commands are explicitly refused; no unfinished mechanic is replaced by a guess.

The relay receives seat-attributed commands, assigns stream order and the next
unsealed tick, and sends the same sealed commands/grants to both clients (§4).
Each granted tick runs as one pump at normal speed. A client advances only when
it has the complete sealed prefix. Developer launch options may select the two
local seats and the relay address; a complete lobby is not needed to test battle
controls. The initial listener is local-only. Network code stays in the host,
relay and lockstep packages, outside authoritative simulation packages (§14).

At the existing 30-tick cadence, compare `Session.UnitStateChecksum()` at the
same completed tick. On 2026-10-07 the maintainer explicitly approved a small
check that may detect hidden RNG/order divergence later when it affects unit
positions or health. It is SHA-256 over the domain
`nanolathe/playtest-units/v1`, completed tick u32, then live allocations in
player-slice/slot order: handle u16, allocation serial u64, owner u8, dying u8,
X/Y/Z raw fixed-point i64 and health i32, all fixed-width little-endian. Live
nanoframes and dying records are included; freed records are absent. No sorting,
allocation, worker join, pointer address, host clock or local viewing state is
needed. This is a partial check with no guaranteed detection delay for hidden
state; equal checks do not prove complete equality.

A mismatch stops this two-seat prototype and reports the tick, consistent with
§9.3's no-majority case. The comprehensive M3 writer and histories remain
available diagnostics for admitted sessions that support them. Enabling them,
extending their schema for new seat state, replay support and richer desync
bundles are not prototype prerequisites. State checks neither synchronize a
world nor authorize a command; they detect observable divergence.

Implementation order within this slice:

1. Publish the minimum perspective/constructor and granted-step interfaces;
   prove two admitted human copies agree while selecting different presentation
   seats and receiving the same seat commands.
2. Connect the bounded local relay and driver to the window host's existing
   input, feedback and battle presentation. Refuse unsupported entry paths.
3. Verify movement, construction and combat commands from both seats,
   periodic agreement and commander loss/end state with headless sessions,
   real loopback connections and host-controller checks. The maintainer chose
   headless automated testing on 2026-10-07; hands-on controls and rendering
   remain part of their play test.
4. Deliver exact launch instructions and the play-test limitations. Keep all
   work on the multiplayer worktree branch; merging to main still awaits the
   maintainer's play testing.

Automated acceptance is these headless checks, affected contract checks,
existing single-player locks and the normal local gates. Remaining full-milestone
gates stay recorded as pending. This explicit sequencing exception also means the
pending CI branch-push permission and replay-only command-schema decision do
not block prototype work that uses neither.

#### 16.4.1 Prototype composition interfaces

The following bounded interfaces implement existing §6 behavior for two hostile
human seats with common visibility settings. They add no checkpoint admission
or schema work. The parent owns their session/host integration.

Visibility reuses the existing ten coverage grids. It adds
`SensorUnit.AllocationSerial uint64`, `Service.EnableOwnerPerspectives()`,
`Service.SensorTickForPerspective(owner PlayerID, defeated bool, tick uint32,
activePlayers int, units []SensorUnit)`, and
`Service.StatusForPerspective(owner PlayerID, id uint16, allocationSerial uint64,
unitOwner PlayerID, fallback uint32) uint32`. Disabled lookup returns fallback
unchanged. Enabled lookup overlays only the sensor mask 0x1700 from that
perspective's allocation-qualified bank; an unseen allocation gets constructor
status (sonar only for its own perspective). Serial zero returns that seed
without storing it, because initial COB creation precedes serial assignment.
The same existing sensor algorithm writes each bank at that human's deadline;
its locally-simulated source gate additionally requires source owner equal to
that human. The shared reveal deadline remains the actual unit field. History
sampling uses the querying owner online; single-player keeps its existing local
history reader. Consumer wiring selects the actor's perspective, including
fallback targeting and underwater visibility. No duplicate visibility service
or new rules registry is introduced.

`frame.NewEventBufferWithIndependentEffects(Limits) *EventBuffer` supplies a
second bounded effect-only channel; `EventBuffer.EffectEvents() []Event` borrows
its current window. Ordinary buffers return their existing staging events.
The online channel has independent IDs, sequences and exhaustion, and receives
valid routed effect events before any presentation-window refusal. It includes
COBSFX, Nanolathe, MuzzleFlash, SmokeStart, SmokeEnd, ProjectileTrail, Impact,
WaterImpact, Explosion, LHTFlash and Corpse. Audio, status, announcements,
music and shake never enter it. Preserve the presentation verdict of Admit and
all publication ordering. Reset clears both windows while retaining counters.
The session composes this buffer before unit scripts run and passes EffectEvents
to the existing effect consumer. The COB producer first uses the union of both
human views (§6.3); local drawing applies its own visibility afterward.

Session adds `onlineResults *onlineResultState`, nil in ordinary sessions.
That state has ten optional rows, each with its own EndLatch and pending/final
Result. `newOnlineResultState([10]bool)` initializes admitted rows.
`evaluateOnlineSeatResult(player int, tick uint32)` runs at that player's
existing due before settlement gates and publishes only that row's ending and
countdown fields. `stepOnlineNoHumanEnd(tick uint32)` runs after the complete
player loop using the same latches; `onlineSeatEnded(int)` controls command
admission, and `onlineBattleEnded()` controls the shared terminal transition.
`ResultForSeat(uint8)` returns a detached presentation result; GetResult selects
the local client's row. Committed frames select that same row and its latch
countdown, including the final publication before grants stop. Defeat precedes
victory, an opponent that has never created a unit cannot satisfy victory,
false due predicates retain countdown,
and crossing the signed countdown below zero is terminal. A mutual wipe remains
defeat with no winner. These are [08 R-TRIG-01 §6] and [08 R-SESS-01 §1], with
§6's canonical seat composition; single-player result/countdown behavior stays
unchanged. Commander sweeps must not stop because only one local row ended.
The kind-3 elimination announcement keeps its one CRT draw and eight-entry
selection [08 R-CAMP-01 §9]. Deathmatch/respawn is refused by prototype entry.

#### 16.4.2 Play-test entry and local transport interfaces

`session.NewPlaytestSkirmish(inputs, config, localSeat, progress)` admits only
this slice's two hostile humans and composes both with controller byte 1 in
canonical seat order. `PrepareGrantedBattle()` completes entry dispatch without
wall-clock stepping; since §16.6 composition calls it before presentation
exists, which is the same world because dispatch runs no tick, and the map
schema comes from the compiled catalog's headers that admission reads; `StepGranted(tick)` accepts exactly the next tick and runs
one pump; `OnlineBattleEnded()` is the shared termination condition. The local
seat affects presentation only. Mobile-build admission checks the issuing
seat's existing known-site predicate and the derived site height before any
queue cancellation or replacement. It does not replace construction's later
terrain/occupancy checks.

`Session.CaptureOnlineCommand(HumanCommand) (SeatCommand, error)` is the
quiescent host adapter after local selection resolves. It captures current
allocation references without enqueuing or mutating a world. The initial closed
vocabulary is Order, Stop, Activation, MobileBuild, FactoryBuild,
CancelProduction, Stockpile, GroupAssign, Stance, Cloak, SelfDestruct,
CancelQueuedMove and BuilderOptions. It preserves the existing local producer's
count defaults and ordering, then applies the online codec's validation.
Unsupported kinds and references fail explicitly. The driver translates a
local pending move receipt to its assigned stream position before transmitting
CancelQueuedMove; client sequence numbers are not global stream positions.

The loopback transport is a development-only, bounded two-connection driver.
It does not implement public-room authentication, reconnect or final removal.
A lost connection aborts this test without awarding a result or changing seats.
Both connections report the existing full `netproto.Identity` and tick-zero
unit checksum before grants begin; any difference refuses entry. Commands stay
opaque to the relay. It seals at most one tick per 1/30 second and waits for
both acknowledgments before another grant, so a slow local client slows both.
It compares the small unit checksum at every tick divisible by 30 and stops on
a mismatch. The normal command sequence policy remains next=last+1, duplicate
<=last ignored, gap refused; seat comes from the admitted connection.

The transport API, owned by `internal/relay`, is:

```go
type LocalHello struct { Seat uint8; Identity netproto.Identity; InitialChecksum [32]byte }
type LocalCommand struct { Seat uint8; Sequence, Position uint64; Payload []byte }
type LocalGrant struct { Tick uint32; Position uint64; Commands []LocalCommand }
func ListenLocal(address string) (*LocalRelay, error)
func ListenLocalWithCommandDelay(address string, delay time.Duration) (*LocalRelay, error)
func (r *LocalRelay) Addr() string
func (r *LocalRelay) Close() error
func DialLocal(ctx context.Context, address string, hello LocalHello) (*LocalClient, error)
func (c *LocalClient) Submit(payload []byte) (uint64, error)
func (c *LocalClient) ReadGrant() (LocalGrant, error)
func (c *LocalClient) Acknowledge(tick uint32, checksum [32]byte, ended bool) error
func (c *LocalClient) Close() error
```

Dial admits and sends hello without waiting for the second seat; ReadGrant waits
for the ready barrier. Acknowledgments name exactly the preceding grant; only
multiples of 30 carry a checksum. Both terminal acknowledgments finish the
stream without another tick. All socket reads/writes are bounded and cancellable
by Close. Prototype resource limits are at most 64 commands and 8 MiB of pending
payload per tick (at most the existing 4 MiB per command), with refusal rather
than unbounded buffering. Wire envelopes have a checked length before allocation;
these are transport limits, not simulation choices. LocalClient supports one
reader and serialized concurrent Submit/Acknowledge calls. No network dependency
enters session or other authoritative packages.


The prototype host driver API is `lockstep.NewLocalDriver(session, client)`
returning `(*LocalDriver, error)`, with `Pump() (bool, error)` (nonblocking,
one granted tick at most), `Submit(session.HumanCommand) (uint64, error)`
and `Close() error`. The constructor requires a prepared battle. Network
reads run separately; only the host calls Pump/Submit and touches session.
The host drains ordinary command receipts after Pump and reports refusals.
A terminal transport error freezes the test and reports it without awarding
victory. Pausing, speed changes, saves and async wall-clock stepping are disabled.
The resource double-click gesture has one pending tracked move; the driver
retains that one receipt and defers its cancellation until the grant assigns
a stream position. It never substitutes a client sequence for that position.


**Launching the local play test.** Use the same stamped `nanolathe` binary and
retail asset installation for both windows. Start seat 1 first, then seat 2:

```sh
nanolathe --root ~/TotalAnnihilation --mod none --map 'ashap plateau' --fullscreen=false --local-mp-listen 127.0.0.1:39731
nanolathe --root ~/TotalAnnihilation --mod none --map 'ashap plateau' --fullscreen=false --local-mp-join 127.0.0.1:39731
```

The first window waits without advancing until both clients pass entry checks.
The launch fixes Modern rules, two hostile human seats and seeds 7/11; identical
`--seed N` arguments can replace both streams. It ignores saved content selection
and rejects explicit mods, mutators, restrictions, AI and probe/benchmark entry
options. Presentation settings remain local. Normal selection, orders and build
controls submit to the relay. Menus continue receiving grants; pause, speed,
save, load, restart and world-changing chat commands are unavailable. Closing a
window stops this test on its peer. There is no lobby or LAN join yet, and the
whole-state fog limitation in §12.6–§13 applies.

**Prototype verification, 2026-10-07.** The two local session perspectives agree
through tick 930 (31 seconds) with orders and construction from both seats and
a commander self-destruct producing opposite local outcomes and identical
per-seat results on both replicas. The regression also checks each committed
frame's local pending countdown and terminal result: the final grant must
publish the result the host needs, without requiring another tick. A headless
host check confirms that both clients enter results from that final frame,
including when a battle menu is open. The socket driver test sends both seats'
commands through real loopback connections and checks a cancellation before
its stream position is assigned. Relay and driver race tests pass. A closed
driver cannot execute a buffered grant. Online fixed-effect and whole-debris
publication uses the viewing seat's existing point-visibility predicate after
copying the shared pools; strips keep their established drawing gates.

The quick displayless performance comparison uses the same Town & Country
scene, seed 7, two runtime workers, 1,200 warm-up and 300 measured ticks without
checkpoints. Baseline `47dfcf776` and candidate `16552de0` have identical scene,
content, initial/warm/final partial fingerprints, RNG totals and all censuses.
Mean tick cost was 1.840 / 1.830 ms, p95 2.722 / 2.734 ms, p99 3.656 / 3.546 ms;
allocation was 103,640 / 103,639 bytes per tick. These short local measurements
show no material single-player regression; they are not online scalability or
cross-platform acceptance. Evidence remains outside the repository at
`/private/tmp/nanolathe-mp-playtest-before` and `...-after`.

#### 16.4.3 Local responsiveness experiment

To answer the maintainer's 2026-10-07 latency concern before deploying a service,
`--local-mp-command-delay-ms N` on the listener adds a fixed 0..1000 ms wait to
both seats' orders. The default is zero. This is a host diagnostic, not a game
rule, simulated ping, or an internet transport acceptance claim. The ordinary
local command/host/tick overhead is additional; tick boundaries round release
up to the next grant. Selection and camera feedback remain local. Command
cancellation follows the existing stream-position receipt path and can therefore
pay another delay when it must first wait for that receipt.

`ListenLocalWithCommandDelay` retains accepted commands until their monotonic
host deadline, releasing only the ready prefix in stream order. Empty grants
continue at the ordinary pace and expose only the last released stream position.
The existing 64-command/8-MiB pending bounds cover delayed commands too. It adds
no worker or simulation state, changes no match identity, and does not delay
acknowledgments or grants. It does not emulate jitter, loss, asymmetric links,
remote view age or TCP retransmission. The prototype still waits for both
acknowledgments per tick; real-network pacing remains separate work (§4.3–§4.4).

Compare the same map and seed at 0, 50, 100, 150 and 250 ms of **additional order
delay**, starting with 0, 100 and 250. Try move then stop, repeated direction
changes, build placement/cancellation and attack/retarget orders. Judge both
first response and correcting a previous order. Close both windows between
presets. The command-line form adds the option to the listener only:

```sh
nanolathe --root ~/TotalAnnihilation --mod none --map 'ashap plateau' --fullscreen=false --local-mp-listen 127.0.0.1:39731 --local-mp-command-delay-ms 100
nanolathe --root ~/TotalAnnihilation --mod none --map 'ashap plateau' --fullscreen=false --local-mp-join 127.0.0.1:39731
```

Headless checks cover deadline equality, retained byte bounds and payload
release, empty-grant progress, sealed-prefix ordering, both-seat command
application and cancellation, and matching unit checksums with 150 ms delay.
The feel test establishes a responsiveness preference. It cannot establish the
ping or jitter budget of a hosted service; that needs a paced transport with
measured click-to-application delay, frame stalls and real remote clients.

### 16.5 First hosted relay (user-authorized 2026-10-07)

After the local responsiveness experiment, the maintainer reports that up to
150 ms of additional order delay feels acceptable and requests the hosted
relay next. Bring forward this bounded M6 increment before full M3/M4/M5;
keep the branch unmerged. That report is a preference, not a measured 150 ms
network-RTT budget. Preserve §16.4's fixed two-human Modern configuration,
small periodic unit checksum, command vocabulary and per-seat simulation.
The hosted path has no artificial order delay. Direct-connect UI, room lists,
AI/Survival, configuration editing, reconnect and removal votes remain later
work. As in the local prototype, a lost connection aborts the test without a
result or gameplay removal; this is not the full M6 departure policy.

#### 16.5.1 Transport and admission contract

`internal/relay` reuses the local grant/command/ack payloads and identity
comparison, adding a versioned hosted create/join handshake. The client chooses
an empty room string to create, or a returned code to join. The server assigns
creator seat 0 and joiner seat 1, refuses a conflicting hello seat, and confirms
the room code before the ordinary identity/tick-zero barrier. Both clients
still compose the same fixed map/configuration before joining. A mismatch
refuses the joining connection without replacing the creator's identity.

Public API for this increment:

```go
type HostedConfig struct {
    TLSConfig *tls.Config
    InsecureLoopback bool
    MaxRooms int // zero selects 16; admitted range 1..256
    MaxConnections int // zero selects 512; admitted 2..1024, two per room at least
}
type HostedDialOptions struct {
    TLSConfig *tls.Config
    InsecureLoopback bool
}
func ListenHosted(address string, config HostedConfig) (*HostedServer, error)
func (s *HostedServer) Addr() string
func (s *HostedServer) Close() error
func DialHosted(ctx context.Context, address, room string, hello LocalHello, options HostedDialOptions) (*LocalClient, string, error)
```

TLS is mandatory except an explicitly requested numeric-loopback test listener
and connection. Clients verify the server certificate; custom roots support
private test certificates, without an insecure certificate-verification flag.
The standalone `cmd/nanolathe-server` reads certificate/key files and serves
many independent rooms without assets, simulation, renderer or audio imports.
Use cryptographic randomness for ten-character room codes; these are private
invitations, not player accounts. Joining a full or unknown room is refused.

Provisional host limits for this test: at most 512 established/pending
connections (`--max-connections`), a 10-second handshake deadline, a two-minute
wait for a second ready seat, and five-second writes. Each room holds at most
64 commands and 256 KiB pending, accepts client frames of at most 256 KiB, and
queues at most 1 MiB plus 4 KiB per peer, with all existing wire
length/sequence checks. These bound one misbehaving room near 2.3 MiB: pending
commands, one queue of grant bodies shared by both peers, each writer's
in-flight copy and each reader's frame. The deployed image serves 128 rooms
with `GOMEMLIMIT=384MiB`, so every room failing at once stays under that soft
limit. Room failure is the blast radius of a bad client. The limits were
measured on 2026-10-08 against honest load: a 1,000-unit order with a
destination per unit encodes to about 21 KB, and 128 rooms of two players each
issuing ten such orders a second held 30 ticks/second at 31 MiB resident on
one core of the reviewer's Mac. Bandwidth, not memory or CPU, is the first
limit of such play. A hosted client refuses to send a larger command. Per-peer writers are bounded and separate
from room pacing; slow readers cannot block other rooms. All close/error paths
release connection and room capacity. No room codes or credentials in routine
server logs. A transient accept error, such as descriptor exhaustion, is
retried with bounded backoff; only Close ends the service. Once grants flow, a
client that receives no relay traffic for 25 seconds stops; that exceeds the
ten-second progress abort and the 20-second WebSocket ping, and it applies only
after the first grant, while a creator may still be waiting for its peer.
These are initial host limits, not retail behavior or advertised
public-service capacity; public-service admission hardening remains M7.

#### 16.5.2 Continuous grants and client playout

A room seals at most 30 ticks/second after its ready barrier without waiting
for an acknowledgment of every tick. Seals keep a 30 Hz phase: a timer that
wakes late does not delay the next seal, and only a gap of a whole interval or
more, such as a wait at the lead bound, restarts the phase without a burst. Each seat acknowledges consecutive
executed ticks; an acknowledgment ahead of the last grant, a duplicate or a
gap fails the room. Compare each 30-tick checksum at that same tick even when
reports arrive at different times. Only released stream positions are sealed.

A room may lead its slowest acknowledged seat by at most 30 ticks. At that
bound grants wait; a seat with no execution progress for ten seconds after
battle start aborts this prototype room. Other rooms continue. On the first
terminal acknowledgment stop new grants; both seats must report the same
terminal tick/outcome bit, then receive explicit normal completion. Already
sent surplus grants are drained without running any tick after the session's
shared terminal state. A discrepancy aborts instead of awarding a result.

`internal/lockstep` adds `Client` (Submit, ReadGrant, Acknowledge, Close with the
existing relay.LocalClient signatures) and `NewPacedDriver(*session.Session,
Client) (*LocalDriver, error)`. `Completed() bool` reports explicit normal
relay completion, never a local result, close or failure. The hosted result
overlay waits for this confirmation. NewLocalDriver keeps its existing behavior.
The paced driver uses a bounded 32-entry receive queue and monotonic host time,
a one-tick reserve (about 33 ms), and a maximum release rate of 30 ticks/second
in steady state. Start with two grants; after an underrun refill the reserve. A lone grant
waits at most two normal intervals, so a final tick cannot wait forever for a
second grant after the peer has ended. Catch-up uses at most 33 ticks/second
when more than two grants are waiting. The window host calls Pump once per
30 Hz host step, and its steps land on display refreshes: on a 75 Hz display
they alternate 26.7 and 40 ms apart, and a window updating at 20 Hz takes two
steps at one instant. Deadlines therefore retain phase through lateness of up
to two normal intervals and discard only lateness beyond that. One Pump runs
every due tick, at most three, and every simulation tick still runs through
its own StepGranted pump. The normal interval equals the host step period
exactly. Hosts updating below 20 Hz cannot sustain 30 ticks/second. No wall
clock enters session.
Input submission remains immediate; commands take effect only on their sealed
ticks. Record compact timing observations for the latency test without changing
command payloads or simulation state.

#### 16.5.3 Verification and delivery

Run headless real-socket matches with equal/asymmetric links at 0, 50, 100 and
150 ms RTT, plus bounded jitter and a temporary stall. Record executed tick
rate, command submission-to-application latency and checksum agreement. Target
steady-state 30 Hz within five percent on a stable link; at 100 ms RTT with
up to 10 ms one-way jitter target p95 command latency below 225 ms. These are
initial acceptance targets to test, not measurements or promises. Measure
stall/recovery separately. Include TLS trust/refusal, identity mismatch,
room isolation/full/expiry/close, bounded input/slow-reader behavior, delayed
checksum mismatch and final-grant handling. Use existing single-player locks.

Deliver the standalone server, a stamped client, exact TLS/local-test launch
instructions and measured limits. Deployment to a paid host or public domain
is separate; the first artifact must be locally runnable without such access.

#### 16.5.4 Starting the hosted play test

The server is a standalone standard-library program. It requires no retail
installation and can be cross-built for a Linux VM:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o nanolathe-server ./cmd/nanolathe-server
./nanolathe-server --listen :39032 --tls-cert /path/to/fullchain.pem --tls-key /path/to/privkey.pem
```

Use a certificate valid for the relay hostname. Clients make outbound TLS
connections; the server needs its chosen TCP port reachable. The first client
creates a room and prints its code in the game messages and standard error;
the second supplies that code. Both use the same stamped client, content,
map and optional seed. For example (the hostname and code are placeholders):

```sh
nanolathe --root ~/TotalAnnihilation --mod none --map 'ashap plateau' --fullscreen=false --relay-address relay.example.com:39032
nanolathe --root ~/TotalAnnihilation --mod none --map 'ashap plateau' --fullscreen=false --relay-address relay.example.com:39032 --relay-room ABCDEFGHJK
```

A private test certificate can be trusted explicitly with `--relay-ca cert.pem`
on each client. No option disables TLS certificate verification. For a test
entirely on one machine, run the server with `--listen 127.0.0.1:39032
--insecure-loopback` and pass `--relay-address 127.0.0.1:39032
--relay-insecure-loopback` to both clients instead. These plaintext switches
refuse non-loopback addresses. Local delay-test flags cannot combine with the
hosted path. Settings/content-selection protections match the local play test.

The opt-in real-socket latency sweep is:

```sh
NANOLATHE_RETAIL_ASSETS=~/TotalAnnihilation NANOLATHE_RELAY_LATENCY=1 GOMAXPROCS=4 GOFLAGS=-trimpath go test -p 4 -tags retail ./cmd/nanolathe -run '^TestHostedRelayLatencyRetail$' -count=1 -v
```

It uses two independently paced real sessions, a headless host that pumps on
30 Hz host steps over a 75 Hz display (`NANOLATHE_RELAY_DISPLAY_HZ` selects
another rate; §16.5.8),
loopback TCP proxies with per-direction scheduled delay, five seconds of
measured play after warm-up, and the existing small checksum. Samples measure
command submission through application, excluding physical input polling and
render/display latency. The probe adds ordered delivery delay and jitter; it
does not emulate TCP packet loss, congestion control, bandwidth limits or an
actual international route. It stays opt-in rather than adding real-time waits
to every ordinary test run.


#### 16.5.5 First measured hosted run (2026-10-07)

The opt-in sweep passed on macOS/arm64 with the retail reference assets. Each
seat applied 28 measured orders after warm-up. Values below are the range
across the two seats, not a network service guarantee:

| Injected RTT to relay | One-way jitter | Executed ticks/second | Command application p95 |
|---|---|---|---|
| 0 ms / 0 ms | none | 29.70 | 81–82 ms |
| 50 ms / 50 ms | none | 29.61–29.70 | 134 ms |
| 100 ms / 100 ms | ±10 ms | 29.71 | 182 ms |
| 150 ms / 150 ms | none | 29.61–29.71 | 234–249 ms |
| 50 ms / 150 ms, with a 250 ms stall | ±10 ms | 29.70 | 268 / 399 ms |

Every pair agreed on its tick-180 unit checksum. The zero-delay case then
self-destructed one commander through the ordinary command path; both copies
finished at tick 510, displayed results only after explicit relay confirmation,
and discarded surplus grants without advancing simulation. The asymmetric case
recovered from its brief stall and reached the same tick/checksum. Its latency
includes the stall and is reported separately from stable-link acceptance.

Both declared stable-link targets passed. This measures ordered socket delay,
not packet loss or a public Internet route. TLS trust and room/lifecycle faults
are separate socket contract tests. The next useful test is two actual computers
using a hosted server and a real connection; the delay knob's acceptable extra
150 ms should not be confused with a total input-to-display or RTT budget.


#### 16.5.6 Private App Platform deployment (2026-10-07)

The maintainer requested a small private relay repository and Dockerfile for
DigitalOcean App Platform. Its public ingress is HTTP, with platform-managed
TLS; raw TCP listeners are only available internally. Add a WebSocket transport
for the same hosted hello, room, commands, grants and acknowledgments. It does
not change the simulation, command schema, pacing or room admission identity.
Sources: [App Platform routing](https://docs.digitalocean.com/products/app-platform/how-to/manage-internal-routing/),
[deployment limits](https://docs.digitalocean.com/products/app-platform/details/limits/),
and [WebSocket RFC 6455](https://www.rfc-editor.org/rfc/rfc6455).

The server adds `ListenHostedWebSocket(address string, config HostedConfig,
behindTLSProxy bool) (*HostedServer, error)`; the client adds
`DialHostedWebSocket(context.Context, url, room string, LocalHello,
HostedDialOptions) (*LocalClient, string, error)`. Native TCP APIs stay intact.
An internal constructor may reuse the hosted room service with an already
secured stream listener. WebSocket bytes carry existing bounded wire frames;
no JSON, command interpretation or new module dependency is introduced.
Use RFC 6455 binary messages, subprotocol `nanolathe-relay-v1`, path `/relay`;
handle masking, fragmentation, control frames and close correctly, reject
extensions/text/oversized messages, and bound HTTP headers and pending sockets.
Maintain the hosted connection limit and close/deadline behavior. HTTP
headers are bounded at 8 KiB. WebSocket pings every 20 seconds keep a waiting
room active at the proxy; these provisional bounds affect host traffic only.
`GET /healthz` reports service readiness and contains no room information.
`--health-listen` serves the same response on a separate listener outside the
connection limit. The platform health check uses it, so connections that
fill the relay's slots cannot fail the check and get the instance restarted.

CLI server flags `--websocket` and `--behind-tls-proxy` explicitly select HTTP
behind the hosting platform's TLS terminator. Public plaintext is permitted only
with that proxy flag; standalone WebSocket TLS and explicit loopback test mode
remain possible. The client accepts `wss://host/relay` in `--relay-address`;
`ws://` requires the existing explicit numeric-loopback test switch. Reject
credentials, fragments, queries and other paths. Certificate verification stays
mandatory. The cloud container binds `:8080` for the relay and `:8081` for the
platform health check, and runs as a non-root user. Its Linux/amd64 multi-stage Docker build contains only the relay,
wire package and server command; no retail assets or simulation are copied.

Export a reproducible source snapshot into a private `nanolathe-relay` repository
with the upstream revision and source-file digests. The engine worktree remains
the source of truth for protocol code; do not create independently evolving
copies. Include the MIT license, Dockerfile, test/build CI, deployment instructions
and an App Platform spec. Use one always-running instance, disable automatic
redeploy on push, and document that a restart/redeploy ends active rooms. No
cross-instance room lookup, database, autoscaling or reconnect is added here.

Verification: existing TCP contracts, actual HTTP/WebSocket/TLS round trips,
malformed framing and handshake rejection, health/lifecycle/connection bounds,
and a headless two-session match through WebSocket. Build and run the exported
Docker image when a daemon is available; otherwise report that check unrun.
A published private repo and matching stamped client are the deliverables.
The user will create the paid DigitalOcean App, then supply its URL for the
real-cloud match test.

The WebSocket acceptance run on 2026-10-07 passed all five §16.5.5 socket-delay
cases with matching checksums: stable-link command p95 was 68–82 ms at zero
RTT, 134–149 ms at 50 ms, 183 ms at 100 ms with jitter, and 232–234 ms at
150 ms. The asymmetric stall case recovered (251/351 ms p95). Both seats
confirmed terminal tick 510. Run the same probe with
`NANOLATHE_RELAY_WEBSOCKET=1 NANOLATHE_RELAY_LATENCY=1`; it remains
a headless local network simulation, not evidence of the cloud route.

#### 16.5.7 Real cloud play-test acceptance (2026-10-07)

The existing latency probe also accepts `NANOLATHE_RELAY_ENDPOINT` containing a
certificate-verified `wss://host/relay` URL. That mode runs one two-seat battle
through the specified remote service, with no local relay or injected delay,
and exercises the same command receipts, tick-180 checksum and explicit
terminal-result gate as §16.5.5. It remains opt-in; ordinary test gates never
contact a cloud service. Reproduce with retail assets configured:

```sh
NANOLATHE_RELAY_ENDPOINT=wss://relay.nanolathe.gg/relay \
  go test -tags retail ./cmd/nanolathe -run '^TestHostedRelayLatencyRetail$' -count=1 -v
```

The first run through DigitalOcean App Platform at `relay.nanolathe.gg` passed:
both seats agreed on their checksums and terminal tick 510, and measured
29.04/29.40 simulation ticks per second. Each seat applied 28 measured commands;
command-to-simulation p95 was 101/116 ms. These measurements come from this
Mac's actual cloud route, exclude input polling and display time, and do not
establish concurrent-room capacity or another region's latency.

Custom hostnames need both the DNS CNAME and registration on the App Platform
app before its managed certificate can be issued. The initial CNAME already
resolved to the deployed app, whose default-domain health endpoint worked;
registering the custom domain and waiting for certificate issuance made
`https://relay.nanolathe.gg/healthz` return 200 with normal TLS verification.
No certificate verification bypass was used. Cloud runtime and both interactive
clients remain the previously tested snapshot; this follow-up changes only the
opt-in probe and this documentation. Engine main remains unmerged.

#### 16.5.8 Hosted relay review fixes (2026-10-08)

A review of the deployed play test found that the paced client could not keep
30 Hz in the real window. The window host pumps the driver once per 30 Hz host
step, on a display refresh, and the driver released at most one tick per pump.
A step arriving a full interval late also reset the deadline phase. Once more
than two grants queued, playout fell to 15 ticks/second on a 75 Hz display
and about 20 on 50 or 100 Hz. The relay's 30-tick lead then held the whole
match at that rate. On 60 and 120 Hz a backlog never drained. The §16.5.5 and
§16.5.7 measurements pumped from a 60 Hz test ticker the window never uses, so
they could not show this. §16.5.2 now states the corrected rule, and a
deterministic lockstep test models the window host at 20–240 Hz.

The same real-socket WebSocket probe, now pumping on 30 Hz host steps over a
75 Hz display, measured on macOS/arm64 with the retail reference assets:

| Injected RTT to relay | One-way jitter | Previous pacing ticks/s, p95 | Corrected ticks/s | Corrected command p95 |
|---|---|---|---|---|
| 0 ms / 0 ms | none | 15.00, 2.0–2.1 s | 29.99–30.00 | 68 ms |
| 50 ms / 50 ms | none | 15.00, 2.0–2.1 s | 30.00 | 134 ms |
| 100 ms / 100 ms | ±10 ms | 15.00, 2.0–2.1 s | 30.20–30.25 | 200 ms |
| 150 ms / 150 ms | none | 15.00, 2.0–2.1 s | 29.99–30.00 | 240 ms |
| 50 ms / 150 ms, with a 250 ms stall | ±10 ms | 15.00, 2.0–2.1 s | 29.77–30.00 | 306 / 401 ms |

Every corrected pair agreed on its checksums, and the zero-delay case confirmed
terminal tick 510 through the result gate. The corrected client against the
deployed `relay.nanolathe.gg` service passed at 29.44 ticks/second for both
seats, with command p95 of 133/134 ms. That service still ran the earlier
relay, whose seals drifted below 30 Hz. After the corrected relay (export of
2c1d47a03) was deployed, the same probe measured 30.00 ticks/second for both
seats, with command p95 of 107/108 ms.

The other fixes in this round are recorded in §16.5.1, §16.5.2 and §16.5.6:
the seal phase, accept-error retry, the client's 25-second idle bound after the
first grant, and the separate health listener. `deploy/relay/.do/app.yaml` is
the intended spec: autodeploy off, one 512 MiB instance with
`GOMEMLIMIT=384MiB`, and the HTTP health check on port 8081. Until it is
applied, the live app deploys on every push to the relay repository and uses
the platform's default health check on the relay port.

Public-service hardening remains M7. Still open from this review:
- One client can hold every room indefinitely by creating a room, joining it
  with a second socket and acknowledging at 30 Hz.
- Worst-case queued bytes per room exceeded the instance's memory long before
  32 rooms. Fixed the same day: §16.5.1's per-room byte bounds hold 112
  flooding rooms at 71 MiB, against 1,430 MiB under the previous limits.
- The final done or failure frame can be lost to a TCP reset on Linux, and
  the server never sends a WebSocket close frame.
- A browser's permessage-deflate offer is refused rather than ignored.

### 16.6 First online lobby (user-authorized 2026-10-08)

The maintainer asked for an in-game way to start online matches, so players
can play and give feedback. The main menu's MULTI entry opens an online
screen with a server (default `relay.nanolathe.gg`) and Create and Join
buttons. Create freezes the host's battle configuration and opens a lobby
with a short room code. A joiner enters the code. When both players are
ready, the host starts the match. This is a bounded M6 increment ahead of
§16's order, like §16.4 and §16.5. Keep it simple; room lists, names, chat,
configuration editing and reconnect remain later work.

Decisions recorded 2026-10-08:

- **Two human seats, Modern gameplay**, as §16.4. The session admits nothing
  else yet.
- **The host's map, mod, mutators and unit restrictions are locked into the
  configuration at Create.** Every joining client composes from that
  configuration, never from its own preferences, and the existing identity
  comparison at join refuses any difference. This brings Q16 forward: field 12
  carries the host's restrictions online. It is mapped from
  `content.Restrictions` as DESIGN_MODS_MUTATORS §15.3 states, with nothing
  seeded, the policy of the fifth mode-independent exception. Retail's
  seeded-but-never-closed `wacky` state cannot arise, because the lobby has no
  restriction screen. The host edits restrictions where single-player does.
- **Room codes are six characters** from the existing 32-symbol alphabet,
  replacing §16.5.1's ten. About 10^9 codes face at most 128 rooms; codes are
  invitations, not credentials.
- **No configuration change after Create.** A different map or mod means a new
  room, so readiness never has to reset (§12.2).
- **The host draws the seed pair** with `crypto/rand` when it freezes the
  configuration. This is a temporary departure from §8.3: the relay-drawn pair
  arrives with the two-stage Prepare start. The configuration digest covers
  the seeds, so both seats still agree on them.
- **A lobby waits at most 30 minutes before Start**, with the 20-second
  WebSocket pings. A joiner who leaves before Start frees seat 2. The host
  leaving closes the room. After Start, §16.5 applies unchanged.
- Delivery: land on main locally without pushing; the maintainer curates main
  before publishing.

#### 16.6.1 Relay lobby protocol

The hosted protocol becomes version 2; a version-1 hello is refused naming
both versions. The relay still treats configurations as opaque bytes and
interprets no gameplay.

- **Create**: the hello carries an empty code, the `LocalHello` and the host's
  encoded configuration (`session.EncodeMatchConfig`, at most 64 KiB). The
  welcome returns the new code and seat 0, and the room enters its lobby.
- **Describe**: a short-lived connection sends only a code and receives that
  room's configuration bytes, or a refusal for an unknown, full, closed or
  started room. The relay then closes the connection. A joiner uses it to learn
  what to compose.
- **Join**: the hello carries the code, the `LocalHello` and no configuration.
  Admission compares identity and initial checksum with the creator's, as
  today, so a joiner whose composition differs is refused, naming the field.
- **Lobby state** (relay to both): which seats are present and ready, sent
  after every change.
- **Ready** (client to relay): a ready flag. **Start** (host to relay): accepted
  only when both seats are present and ready; otherwise the relay repeats the
  lobby state and the lobby stays open. Then **Started** goes to both, followed
  by grants exactly as §16.5.2.
- Commands submitted before Start wait for the first grant, as they did
  before the lobby. An acknowledgment before Start fails the room. After
  Start, ready and start messages are ignored. A peer that leaves the lobby
  releases its seat (seat 2) or closes the room (seat 1).
- A creator's hello may set an auto-start flag: the room starts as soon as a
  second seat joins. `DialHosted` and `DialHostedWebSocket` set it when
  creating and report ready at once when joining, so a command-line client
  can also join a lobby room and play once its host starts.

Public API added to `internal/relay`; `DialHosted` and `DialHostedWebSocket`
keep their signatures and become a lobby that readies and starts
automatically, so the command-line play test and the latency probe are
unchanged:

```go
type HostedLobbyState struct {
    Present, Ready [2]bool
    Started bool
}
func DescribeHostedRoom(ctx context.Context, address, room string, options HostedDialOptions) ([]byte, error)
func OpenHostedLobby(ctx context.Context, address, room string, hello LocalHello, config []byte, options HostedDialOptions) (*HostedLobby, error)
func (l *HostedLobby) Code() string
func (l *HostedLobby) Seat() uint8
func (l *HostedLobby) State() (HostedLobbyState, error) // latest snapshot; never blocks
func (l *HostedLobby) SetReady(ready bool) error
func (l *HostedLobby) Start() error // seat 0 only
func (l *HostedLobby) Battle() *LocalClient // non-nil once Started; owns the connection
func (l *HostedLobby) Close() error
```

`address` is `host:port` for TLS or a `wss://host/relay` URL, chosen as
`--relay-address` is today. The lobby reads relay messages on its own
goroutine and stops reading exactly after Started, so the first grant reaches
the battle client.

#### 16.6.2 Client flow

- **MULTI** is enabled except in the browser build, which has no relay
  transport. It opens the online screen: a server field (a bare host expands
  to `wss://host/relay`), a room-code field (case and spaces ignored), Create
  Game, Join Game and Back. A status line reports refusals in plain words.
  Unstamped builds are told up front that online play needs a stamped build.
- **Create** uses the host's current skirmish map, with a button to change it
  through the ordinary map picker, and its current mod, mutators and
  restrictions. It composes and prepares the session off the game goroutine,
  then opens the lobby.
- **Join** describes the room, decodes its configuration, and adopts its map,
  mod, mutators and restrictions. If the room's mod is installed but not
  mounted, the client remounts it through the ordinary content reload,
  telling the player. A missing mod or map is reported by name and nothing is
  joined. It then composes and prepares, and opens the lobby.
- **Lobby**: the room code shown large, with Copy where the host clipboard
  allows; both seats with their readiness; Ready/Not ready; Start for the host,
  enabled when both are ready; Leave.
- **Started**: the client enters its prepared battle and drives it from the
  lobby's battle client, as the command-line path does after its dial. When
  the battle ends or the connection fails, leaving returns to the online
  screen.
- Details settled while building it (2026-10-08):
  - A `ws://` server is the plaintext test opt-in, accepted only on a numeric
    loopback address.
  - Only a mod installed from its archive can be hosted, because field 8 needs
    the archive digest. A folder install is refused, asking for a reinstall
    from the zip.
  - Field 10 holds only the mounted content's own Community table, never a
    player's overrides. Both seats use the default builder options, as the
    command line does.
  - Joining a room whose mod is installed but not mounted saves that mod as
    the player's selection, as any content reload does.

#### 16.6.3 Verification

- Relay socket tests for the lobby states, refusals, leaving, Describe and
  version 2.
- Session tests: field 12 admitted online, restrictions applied to both seats'
  catalog clones, and identities that differ when restrictions differ.
- Client tests: MULTI routing, configuration adoption, and refusal text.
- One headless two-client match through a real relay, started from a lobby,
  with a mutator and a restriction.
- The existing latency probe and both gates still pass.
