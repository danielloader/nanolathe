# Design — Multiplayer

Battles between several people — on one LAN, by direct connection, or through
a hosted service — together with the replay recorder, spectators and the relay
server they need. Every participating client runs the complete simulation.
Only player commands travel: a relay puts them in one order, tells every
client which tick each one runs on, and decides how far the battle may
advance. The model is **relayed deterministic lockstep**.

**Status: adopted 2026-10-01; not yet implemented.** The maintainer accepted
this design and decided its original questions on 2026-10-01, after two
revisions of the 2026-09-30 proposal. Networking, replays and the multiplayer
lobby are in scope (ARCHITECTURE §1), and implementation is authorized in the
order of §16, each milestone behind the one before it. Nothing here is built
yet. Measurements quoted here were taken on 2026-09-30 and 2026-10-01 against
main `193abfde`. Co-op is the first delivery, not the architecture's limit:
an eventual competitive mode is required. The interoperability target is
Nanolathe-to-Nanolathe; other engines are deferred without a commitment to
support them (§8.5). Four later decisions remain **OPEN** in §15; none blocks M1.

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
| Unit restrictions | Multiplayer only: a per-unit limit of 0–100 or none; units marked `wacky` default to 0; `norestrict` units are not offered `[08 R-SKIR-01 §10]` `[05 R-SHARE-01 §9]`. | **Not modelled today**: a new configuration field and lobby screen. |
| Unit roster | Peers keep only units every peer selected and holds compatibly `[08 "Unit-data negotiation and catalog retention"]`. | Identical catalogs are required instead (§8.2); negotiating a common roster is §15 Q15. |
| Cheats | The typing machine checks the entry-time cheat permission, but the ungated developer phrase unlocks its cheat and developer commands regardless of the host switch. Commands execute only there; received lines are chat text, never dispatched `[07 R-CAM-01 §6]` `[08 "Lockstep advancement"]`. | Agreed cheat permission enforced by every receiving simulation; neither the developer phrase nor a local developer flag expands the allowed stream commands (§7.1, §7.2). |
| Alliances | Row A holds declarations and row B mirrors declarations toward the seat. A seated non-eliminated, non-watching human can toggle a declaration toward a seated non-eliminated, non-watching remote human outside its team; its own row changes at once and only the target machine is told. Third machines keep stale copies. Computers have no toggle: battleroom teams set mutual rows on every machine and remain locked in battle. Consumers read row A one-sidedly; victory additionally tests mutuality and the survivor's row toward other seats `[05 R-SHARE-01 §1]` `[07 R-FE-01 §7]`. | Declarations are seat commands. A single matrix with B as its transpose is the **proposal**, pending Q22; reproducing per-perspective stale copies remains the alternative. |
| Allied sight | Current sight is per player and never merged `[03 §3.2]`. Two opt-in shares exist, off at the start and toggled in battle by chat commands. *Share mapping* copies the sharer's explored tiles, every 450 ticks, into each surviving remote human it has declared alliance toward — or once, from the share screen, toward anyone — and under Permanent line of sight an explored tile is also a visible one `[05 R-SHARE-01 §6]`. *Share radar* marks the sharer's own units as friendly contacts for viewers it has declared alliance toward, and passes on none of its detections `[03 R-VIS-01 §7]`. | Retail share eligibility (§6.3); history scope and application timing remain Q24 and Q25. Shared allied vision, as Survival already does for its team, would be a separate Modern policy. |
| Automatic sharing | Every 60 ticks, for the local player only, surplus above a threshold flows to an allied, surviving remote human with less stock; computer players never receive it `[05 R-SHARE-01 §3]`. | Runs for every human seat, slots ascending (§6). |
| Giving | The share screen gives resources (no alliance test) and units other than airborne, transported and commander units `[05 R-SHARE-01 §5]`. A transferred unit is always replaced by a fresh record without its kills, orders or groups; for a remote new owner the sender only kills its copy and sends the transfer, the receiver creates the replacement, and the stockpile bytes travel under a different gate `[05 R-WORK-01 §15]`. | Seat commands; a transfer between seats of different machines takes the remote branch's copy rules (§6). |
| Pause and speed | The pause key is available to watchers too; speed keys and slider refuse watchers. Receivers apply pause and speed from any admitted peer. Speed is 1–20; unpause gives no catch-up burst `[08 "Lockstep advancement"]` `[07 R-CAM-01 §2]` `[01 §4.3]`. Entry is unpaused at speed 10 `[08 R-ENTRY-01 §2]` `[08 R-ENTRY-01 §3]`. | Normal speed and no pause in the first releases (§15 Q5). Any later support uses relay pacing (§4.4). |
| Ending | Each machine judges its own human every 30 ticks; authored triggers are not checked online. Defeat requires no live units. Each surviving opponent must have shared victory enabled on both sides, mutual alliance, and its own row covering every seated non-eliminated seat, including itself and watchers; an opponent that has created nothing prevents victory. Won and lost paths share a countdown that false checks do not reset `[08 R-SKIR-01 §3]` `[08 R-TRIG-01 §6]`. Outside Deathmatch, the no-live-human site runs each tick and ends the battle on its sixth consecutive true tick from an unarmed countdown `[08 R-SESS-01 §1]`. | Per human seat (§6). The third-seat alliance knowledge used by shared victory awaits Q22. |
| Defeated players | Where the host allows watching, a defeated player becomes a watcher at its defeat latch and is then asked "You're out!  Continue Watching?": *Yes* changes nothing further and *No* leaves `[07 R-FE-01 §9]`. Where it does not, the player takes the ordinary lost ending, as in skirmish. One hosting live computer players is put in watch mode without the question, whatever the option says, because departure removes their machine group and destroys its units `[08 R-SKIR-01 §3]` `[08 R-LEAVE-01 §9]`. | Per seat (§6.6, §11.1). |
| Watchers | A watcher holds a seat with no commander — its placement draws are still taken — sees everything, and is left out of scores and elimination `[08 R-ENTRY-01 §5]`. Joining as one needs the host's *watching allowed* option; the host can switch it off in battle, which removes the remote watchers `[08 R-SKIR-01 §12]` `[07 R-HUD-04 §1]`. | Observer rows in the configuration (§11.4). |
| Resign, host loss, timeout | Resign tears down only the leaving machine. In a launched battle host loss migrates the host flag and does not end play `[08 R-LEAVE-01 §7]` `[08 R-LEAVE-01 §8]`. Timeout opens a dialog; REJECT or timeout plus 120 seconds removes one silent machine, but the automatic path does not remove two or more silent machines `[08 R-LEAVE-01 §6]`. Removal destroys units and empties the seat without transferring stocks, scores or its unit-limit slice `[08 R-LEAVE-01 §3]` `[08 R-LEAVE-01 §4]` `[08 R-LEAVE-01 §5]`. | Rejoin and final removal are distinct; Strict destroys the finally removed seat's units, Modern/Community retain them idle (§11.1). |
| Computer players | Each not-ready human may add one computer on its machine, when the game is open and not Deathmatch; the host's first click on an open row blocks it, and the next reopens it and enters the add path. The creator controls side/team and readiness; teams provide computer alliances. Difficulty is unsynchronized hosting-machine state `[08 R-SKIR-01 §13]` `[08 R-AI-01 §21]`. | Every replica runs each computer, with its creator's fixed perspective (§6.6). Cap, explicit difficulty and Deathmatch admission await Q23. |
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
| Entry | One command, session event, grant, pacing note or pump end with a monotonically increasing stream position. Commands and session events are bound to a tick. |
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
2. **Session event** — `{tick, kind, seat}`: a seat resigning or being
   dropped (§11.1), and the controller restart that accompanies a snapshot
   rejoin (§9.1). A resignation is the seat's own request; the relay authors
   the others. Each is bound to a tick, as a command is.
3. **Grant** — `{through, sealedPosition}`: the prefix of commands and
   session events for ticks up to `through` is sealed and those ticks may
   run. No later entry may be assigned to a sealed tick.
4. **Pacing note** — `{kind, argument}`: pause, resume and game speed
   (§4.4). It records a pacing decision for the HUD and for replays and
   changes no simulation state. Single-player recordings carry them; online
   battles carry none until §15 Q5 is revisited.
5. **Pump end** — in single-player replays only, the tick after which the
   host's executor tail ran (§4.5).

**Casual tick assignment.** On receiving a command, or authoring a session
event, the relay assigns it the earliest tick no client can yet have run,
`lastGrant + 1`, and appends it. Entries bound to one tick keep the relay's
arrival order; one seat's commands keep their sending order. The stream is append-only. Acceptance acknowledges the
client sequence and assigned stream position; reconnects retain this identity
so a retransmitted command cannot execute twice (§11.2). A competitive
scheduler may assign a later tick under the room's declared policy (§4.7).

**Application.** A client applies the commands and session events bound to
tick T in phase 1 of tick T, in stream order, through the existing typed
command path that drains the input queue today
([DESIGN_RUNTIME_DETERMINISM §2.5](DESIGN_RUNTIME_DETERMINISM.md), phase 1).
Nothing else in the stream changes simulation state, so there is one
application point and one order. An earlier draft gave pause, resume, speed,
resignation and drop a second application stage between ticks. It is
withdrawn: the first three are not world state (§4.4), and the last two need
nothing a tick-bound entry lacks.

In a lockstep battle **every command and session event received while paused
waits for phase 1 of the next granted tick**. The client may preview accepted
orders, but does not apply any to authoritative state early. A resignation or
drop made during a pause is bound to that same tick at its stream position:
the seat's entries ahead of it apply, and those behind it are refused,
identically by every simulation. Local network buffers never enter the state
digest.

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
longer bound it shows the waiting-for-player state and offers the other seats
the drop decision (§11.1). Pacing is host policy: it decides *when* ticks run,
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
  allowlist, and no fused multiply-add in authoritative packages on arm64 or
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

### 5.3 What does not hold yet

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
`GOAMD64=v3` build could not run under Rosetta and is unmeasured; the
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
armies) and a resolver supplying the per-frame holds the window reads from
the same art: the pool first reaches 300 at tick 3,283, where without the
resolver it peaks at 286. In that tick's unit phase it refuses three shatter
fragments and ten other admissions, which leaves the simulation stream 24
draws and the CRT stream 10 draws behind the other run. The two runs'
partial fingerprints are equal through step 3,000 and different at step
4,500 and at every later checkpoint. A window and a headless host therefore
already compute different battles from the same inputs, and every
fingerprint lock is headless.

A census of the same day shows how narrow the exposure is. Each scene ran
with and without that resolver under each reserved rule set:

| Scene | Strict 3.1 (300 records) | Community 3.9 (3,000) | Modern (3,000) |
|---|---|---|---|
| Benchmark fight, 15,000 steps | full from tick 3,283, in 30 ticks; 30 fragments and 81 other admissions refused | peak 392 | peak 431 |
| Computer-player game, to 54,000 ticks | peak 18 | peak 43 | peak 159 |
| Survival, to its defeat | peak 126 | peak 260 | peak 124 |

Only the Strict fight diverges. Every other pair of runs keeps equal
fingerprints to its end, and no existing lock constant changes with the
resolver. The event window peaks at 48 events in a tick against its bound of
4,096 and drops nothing. ARCHITECTURE §2 already states the intent for
`SimArt` — "so headless and windowed battles run one simulation" — and this
reader of art was left behind when the others moved there.

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
   (§6.3); Q24 leaves the remote-history projection open.
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
sensor source gate — which the rows now carry.

| Work | Contract | Today | Lockstep form |
|---|---|---|---|
| Sensor phase | Five unit walks that write the seen, sonar, jammed and decloak-timer bits from the viewing slot, inside that player's 30-tick settlement deadline, only when more than one player is active; a defeated or watching viewer marks every unit friendly `[03 R-SENSOR-01]` `[03 R-VIS-01 §4]`, and the first walk also marks the units of an owner that has declared alliance toward the viewer and shares radar `[03 R-VIS-01 §7]`. Every unit's fallback targeting reads the seen set `[06 §3.1]`; the sonar bit exempts underwater rejection; the decloak bit feeds cloak upkeep. | Runs in the viewing owner's due block (`step.go`); passes in `internal/visibility/sensors.go`; new units of the viewing owner get the sonar bit at creation (`session/visibility.go`). | One pass per perspective, in that seat's own deadline block. The bits it writes are held per perspective; each reader reads its acting unit's perspective. The alliance rows and radar-share options the first walk reads are per-seat battle state. Pass 4's source gate, which retail writes as "the owner's controller is 1 or 2" and means "simulated on this machine" (`ownerLocallySimulated`), becomes "this perspective's seat or a computer seat it hosts": with every human seat control 1, the unchanged gate would let each perspective's pass write every seat's decloak bit. |
| Direct-visibility predicate | Reads the local player's coverage bit even for a query on behalf of another record `[03 §3.2]`. | `internal/visibility/predicate.go`, reached by every side's target-list rebuild, Modern targeting, danger and repair-pad queue, the Classic computer player's rally sight `[08 R-AI-01 §7]`, and the COB effect gate below. | Reads the querying record's perspective. |
| COB effects | The effect opcode first tests whether the viewing player can see the unit; a failed test does nothing at all. A passing test spawns strip objects that take pool slots and, in the sprinkle and smoke families, CRT draws `[04 R-COB-03 §6]` `[03 R-STRIP-01 §1]`. Draw counts are behaviour [I4], so on one machine the viewer decides the CRT stream's position. | The gate in `composition.go` reads the viewing owner; the passing path also emits a frame event that the fixed effect pool `[03 §1]` admits in phase 4, so the viewer reaches the pool's occupancy and, through it, the simulation stream as well (L9). | Spawned when the unit is visible to **any** perspective. The union is taken ahead of the strips and the pool, so both hold the same records on every client. Each client then draws only what its own seat may see: that filter is presentation and runs after the pool. |
| Effect pool | A fixed pool whose records live for their animation players' authored holds `[03 §1]` `[06 R-WFX-01 §1]`; at capacity it refuses shatter fragments and land-dust puffers before their draws `[04 R-COB-04 §3]` `[03 R-FX-01 §3]`. | Timed by a resolver only the windowed host installs, and fed through an event window that positional audio shares; an audio event is emitted only where the viewing seat can hear it (L9). | Timed from simulation content on every host (M1). Admission of the events that feed the pool does not depend on audio or status events: those are produced for the local seat after the pool, or counted in a window of their own. |
| Temporary sight | Recorded only for victims the viewing slot owns, at most 20 `[08 R-SESS-01 §3]`; expired in the executor tail `[01 R-PLAT-02 §5]`. | `internal/session/eyeball.go`; expiry in `post_loop.go`, once per host pump. | One list of 20 per perspective; expiry after every tick in lockstep (§4.5). |
| End condition | Each machine evaluates its own player in that player's deadline block; online, authored map triggers are never polled `[08 R-TRIG-01 §6]` `[08 R-SKIR-01 §3]`. | `endConditionBlock` runs only on the local owner's due (`step.go`, `result.go`): with two humans the lower seat's defeat is never evaluated and the higher seat's ends the battle for everyone. | Every human seat's block evaluates that seat (§6.5). |
| End countdown | One countdown and ending latch per machine gate its human and hosted computer settlement. Won and lost paths share it; a false due never resets it. Quit and Continue-Watching No set that same ending bit `[05 R-ECO-01 §1]` `[08 R-SKIR-01 §3]`. | `publishEndCountdown` (`result.go`) mirrors one latch onto all ten player records. | Per human perspective; gates that human and its hosted computers for victory as well as defeat. Human work precedes hosted-computer work on the arming due. The kind-3 after-loop no-human site runs every tick, skips Deathmatch and uses the same countdown, ending all perspectives on its sixth consecutive true tick when starting unarmed `[08 R-TRIG-01 §6]`. |
| Commander death | Only the owner's machine clears the storage bonus, files the commander death and sweeps its units. Received deaths do not sweep. A human respawns only in its own block, with the host's starting-resource values; its visibility rebuild resets that machine's complete mapping history and every eligible sight grid `[08 R-SKIR-01 §3]` `[08 R-ENTRY-01 §7]`. | `processPendingCommanderDeaths` runs per owner; respawn is the local owner's only (`commander_death.go`); `RebuildEntry` refills all grids. | Sweep each owner once. Respawn each eligible human through its own block. Rebuild that perspective's own and hosted-computer visibility; whether to reproduce the erased remote explored-history copies remains **OPEN**, Q24. Other perspectives do not rebuild because a remote seat respawns. |
| Watcher entry | Clears mapping and line-of-sight mode bits for the whole watching machine, hosted computer included, and rebuilds its complete visibility/history grids `[08 R-SKIR-01 §3]` `[08 R-ENTRY-01 §7]`. | `clearWatcherVisibilityMasks` in `ai_entry.go`, at entry and restore. | Changes the entering perspective and the hosted computers that borrow it; other perspectives keep their configured modes. Remote explored-history reset scope awaits Q24, as for respawn. |
| Automatic sharing | The local slot only, every 60 ticks, multiplayer only `[05 R-SHARE-01 §3]`. | Inert: the economy's networked flag is never set. | Every human seat, ascending. |
| Explored-map sharing | Every 450 ticks the opted-in sharer asks surviving remote human allies to copy its explored bits; the target applies the request on its own mapping-grid copy after transport delay. The share screen offers a one-time gift to anyone and applies it locally at once `[05 R-SHARE-01 §3]` `[05 R-SHARE-01 §6]`. History affects path probes, placement and Permanent sight. | Not implemented; `internal/economy/tick.go` only counts eligibility. | **Proposal pending Q25:** apply on the request tick, humans in canonical seat order, copying to eligible human recipients; computers neither send nor receive automatic shares. Which explored-history copy is used also depends on Q24. The gift remains a seat command (§7.1). |
| Unit transfer | A local and a remote branch with different copy rules `[05 R-WORK-01 §15]`. | Local branch only. | Chosen by whether old and new owner share a machine. |
| Player-record cheats | `+Give`, `+ATM` and their kin act for the local player `[07 R-CAM-01 §6]`. | Give debits the viewing owner; ATM and the Modern spawn command act for the local owner (`commands.go`, `spawn_command.go`). | The issuing seat (§7.1). |
| Builder options | The Community builder preference belongs to the host player (DESIGN_COMMUNITY_PATCH §4.3). | Installed only for the local owner (`builder_options.go`). | Each seat's own, as a seat command. |
| Survival waves | Waves patrol to the nearest unit of the human team (DESIGN_SURVIVAL §6.7). | `survival.go` targets the local owner and its allies. | All human survivors. |
| Loss statistics | — | Losses of one cause are attributed through the local owner's alliance row (`stats.go`): the scoreboard differs per client. | Each seat's own row. |
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
client once every human seat has latched a result or left, or through the
no-human countdown `[08 R-SESS-01 §1]`.

### 6.6 Computer seats

Retail allows at most one computer per human machine. Its creator hosts it
for the whole battle; only that machine runs its planner and full unit work,
while peers receive results `[08 R-SKIR-01 §13]` `[08 R-AI-01 §21]`.
The computer borrows that machine's sensor picture. A defeated human remains
watching while its computer lives, even when watching is forbidden. Removing
the human removes the machine's computer too and destroys both players' units
`[08 R-LEAVE-01 §9]` `[08 R-LEAVE-01 §10]`.

Initially every client runs every computer seat. Its **host seat** is the
human that added it, fixed in the configuration (§15 Q4). The adopted
Nanolathe policy keeps that computer running if its human leaves, borrowing
the host's last perspective; this deliberately differs from retail removal
of the machine group (§11.1). Q6 governs the seat actually removed, not an
implicit removal of its hosted computers.

Retail difficulty belongs to the hosting machine, is not synchronized and
is consulted only for its computers: the room creator sets hard at START;
other hosts use their existing value `[08 R-AI-01 §21]`. Q23 leaves the
computer cap, explicit per-seat difficulty and Deathmatch exclusion open.
No implementation may resolve these from an unagreed local preference.

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
| `Give` | `+Give p n metal`, open in every session kind: a transfer from the issuing machine's controlling player to seat `p` through the sharing transfer `[07 R-CAM-01 §6]`. | **Seat command** whose source is the issuing seat. |
| `SetResource` | `+NoMetal p n` and `+NoEnergy p n`, also open in every session kind, write any player's stock `[07 R-CAM-01 §6]`. In retail multiplayer a write to another player's stock lasted only until that player's next economy snapshot overwrote it `[08 "Economy and integrity checks"]`. | **Seat command limited to the issuing seat's own stock and gated by the online cheat permission.** This authorization policy applies in every online gameplay mode, including Strict 3.1 (§15 Q8); it is Nanolathe's and not a claim about retail. |
| `SetLogo` | `+Logo n p` sets any seat's insignia `[07 R-CAM-01 §6]` by writing the player record's logo, which saves and result rows carry. | **Local**: a presentation override on the issuing client. The record keeps its configured logo. |
| *(new)* share toggles and thresholds, the share screen's gifts | `+ShareMetal`, `+ShareEnergy`, `+ShareMapping`, `+ShareRadar`, `+ShareAll`, `+SetShareMetal`, `+SetShareEnergy` are network-only; each toggle flips the issuing player's own option, clear at the start `[07 R-CAM-01 §6]` `[05 R-SHARE-01 §3]`. The share screen is `[05 R-SHARE-01 §5]`; declarations have their own row below. Retail's receiver does not check that a share names the player that sent it `[05 R-SHARE-01 §4]`. | **New seat commands** acting on the issuing seat; the relay's seat stamp is the source (§7.2). The share options are per-seat battle state inside the digest (§9.1), not agreed configuration. |
| `View`, `ATM`, `DoubleShot`, `HalfShot`, `Visibility`, `Meteor`, `MakeSelectable`, `Spawn` | Retail cheats, most of them behind the cheat gate `[08 R-OOS-01 §5]`, and the Modern testing spawn. The engine holds no cheat flag: the host refuses the gated ones outside a skirmish session and enqueues the rest unconditionally. Two are ungated in retail too, in every session kind `[07 R-CAM-01 §6]`: `MakeSelectable`, whose bit order resolution, trigger evaluation and the computer player read, and the line-of-sight-type half of `Visibility`, which rewrites the battle's one mode word. `DoubleShot` and `HalfShot` change every shot; the argument-free meteor arms a storm with four CRT draws; the spawn creates a unit and is gated only on the Modern rule set. | **Seat commands only when the lobby enabled cheats** (§15 Q8), the two ungated ones included. The sending UI refuses them when disabled, and every receiving simulation independently enforces that permission before dispatch. `View` moves only the issuing seat's perspective; the others act on the whole battle. |
| *(new)* `DeclareAlliance`, `SharedVictory` | Allies-window declarations and shared-victory control `[05 R-SHARE-01 §1]` `[07 R-FE-01 §7]`. | **Seat commands**: declaration `{target, value}` requires a seated non-eliminated, non-watching issuer and a distinct seated non-eliminated, non-watching human target outside the issuer's team. Shared victory `{on}` refuses watchers and seats in teams of two or more. Matrix storage awaits Q22. |
| *(not yet a command)* `ShootAll` | A retail toggle, ungated in every session kind `[07 R-CAM-01 §6]`, of a mode bit that target admission reads `[06 §3.2]`. The engine has the seam (`combat.Service.ShootAll`) and no typed command. | **Seat command** when it is added, setting that seat's bit: on a retail machine the bit governs the units that machine simulates. It needs the agreed cheat permission under Q8. |
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

## 8. Battle configuration and identity

### 8.1 What the seats agree

The agreed configuration includes every battle-entry input and the match
policies that constrain its participating clients:

1. **The skirmish configuration**, `session.SkirmishConfig`
   (`internal/session/skirmish.go`): map; the seat rows (nickname,
   controller, side, colour, ally group, starting metal and energy, Classic or
   Modern); difficulty; start-location mode; commander-death mode; mapping,
   line of sight and its type; unit limit; Survival options. The relay fills
   the two seed fields (§8.3).
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
   and starting shared-victory bits; the agreed computer difficulty representation
   once Q23 is decided; unit restrictions, cheat permission, watching permission, pause/drop policy,
   scheduling policy,
   spectator/replay release policy and the match view policy (§8.4).
   Its canonical encoding is versioned and covers every effective field,
   including defaults. No battle-affecting option may remain an unagreed
   machine-local default.

Same-team pairs start mutually allied with shared victory enabled for teams
of two or more; a single team holding every player prevents START
`[08 R-SKIR-01 §3]` `[05 R-SHARE-01 §1]`. Q22 and Q23 remain decisions,
not implicit defaults in this configuration list.

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
membership: a departure's session event removes a participant and the rejoin
transition of §11.2 restores one. Catching-up clients and spectators are
excluded.
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
  they continue. Each dissenting seat is moved to the rejoin path (§11.2) —
  or, before that path exists, dropped — with its units left as a
  disconnected seat's are (§11.1). A rejoin replays the stream from the
  start, which cures a transient fault and repeats a deterministic one; a
  seat that disagrees again at the same checkpoint is dropped, and its
  bundle names the platform. The hosting client of an embedded relay is a
  participant like any other: its relay keeps running while its seat
  rejoins. With no strict majority — every two-participant battle — the
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
and session events with their ticks, pump ends, pacing notes, and digests at
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

A resignation, a reconnectable drop and final removal are distinct session
events, assigned a tick by the relay (§4.2). No client may remove another seat
by naming it in a payload. Retail instead accepts a peer's removal notice
without a host check `[08 R-LEAVE-01 §1]` `[08 R-LEAVE-01 §2]`.

- **Resign** ends that seat's battle without a win. Allowing its client to
  stay as a watcher is Nanolathe's interface choice; retail tears down the
  leaving machine and peers handle an ordinary departure
  `[08 R-LEAVE-01 §7]`. Its units follow the final-removal policy below.
- **A reconnectable drop** retains the seat and its units idle while rejoin
  is possible (§11.2), in every mode. It issues no new commands. Rejoin is
  a Nanolathe addition; no retail reconnect protocol has been found.
- **Final removal** closes the rejoin window and applies Q6. In Strict 3.1,
  destroy that seat's units using the established departure contract:
  effect-only self-destruct and no kill credit; the ordinary positive-health
  case leaves no wreck, while an already zero-health unit preserves the
  researched full preamble and script-selected wreck. Stocks freeze when
  the record is removed; resources, scores, identities and the unit-limit
  slice are not gifted or redistributed
  `[08 R-LEAVE-01 §3]` `[08 R-LEAVE-01 §4]` `[08 R-LEAVE-01 §5]`.
- **Computer seats** keep running when their human host departs, with that
  host's last perspective (§6.6). This adopted Nanolathe choice differs from
  retail's removal of the whole machine group `[08 R-LEAVE-01 §9]`.
  Q6 applies only to the seat actually removed: it does not implicitly
  remove hosted computer seats. Defeat alone also does not remove them.
- **The hosting client** of an embedded relay is the relay: losing it ends
  the battle for everyone without a result. This is an infrastructure limit;
  retail migrates its host flag and continues a launched battle
  `[08 R-LEAVE-01 §8]`. A hosted relay has no such player seat.

**Nanolathe Modern policy — final seat removal.** The approved departure is
bounded to a finally removed seat in a multiplayer battle. Modern and
Community retain its units under that owner, idle, with no new commands;
Strict 3.1 uses the destruction path above. Retention transfers neither units
nor stocks to another player and adds no RNG draw of its own. Ordinary unit
simulation and economy remain under their existing rules. Single-player,
commander defeat, reconnectable absence and the separately adopted survival
of hosted computer seats do not select this policy. There is no vote-to-destroy
fallback in Q6.

Select the answer through the existing central `gameplay.Mode` and
`session.RuleSet` composition, following DESIGN_GAMEPLAY_RULES §9. Extend
the existing owning rule interface if it can express the removal decision;
if none can, justify a narrow session-owned seam before implementation and
compose it in that same registry. Strict, Community and Modern defaults must
all supply the answer; no room flag or second registry chooses it. Mutable
seat lifecycle state belongs to the session, not cached rules.

Required M6 tests cover a reconnectable drop and a final removal in all
three reserved modes, repeated removal, and removal while grants are held.
Assert Strict destruction, the positive-health no-wreck path and zero-health
exception, no kill credit or resource transfer, and the researched loss-stat,
effect and RNG consequences; assert Modern/Community retention avoids
those removal effects and draws. Compare later RNG positions and stocks,
cover an associated computer that keeps running, and preserve single-player
fingerprints. Keep unit orders already in flight and the exact meaning of
idle explicit in the lifecycle contract before implementation; Q6 does not
invent queue cancellation or a new per-tick suspension rule.

### 11.2 Rejoining by fast-forward

The relay keeps the battle's configuration, seeds and stream. A rejoining
client receives them, composes the battle, and runs the stream at full speed
without presentation or audio until it reaches the live grant, then resumes
its seat. Admission repeats build/content/configuration checks, including
view restrictions. A per-match seat credential proves ownership without
requiring an account in casual rooms. A new connection epoch fences off the
old connection; only one connection can command a seat at a time.

The relay retains accepted client sequence numbers and their stream
positions across disconnects. Repeating an accepted sequence returns its
original receipt, not another stream entry; a conflicting payload under the
same sequence is refused. Sequence retention and its bounds cover the whole
rejoin window. A client resends unacknowledged commands under their original
identities, handling a connection loss between acceptance and echo without
either duplicating production or silently losing an order.

Catch-up clients do not pace the battle or vote on its digests. They become
active through an ordered membership transition after matching a required
checkpoint and receiving its sealed stream continuation. Commands are admitted
only after that transition. Authentication, takeover, deduplication and
checkpoint handoff are part of reconnect, not later account features.

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
battle configurations, commands, digests and chat. The protocol is open and
documented here so anyone can run a server.

### 12.2 Messages

Client to relay:

| Message | Content |
|---|---|
| Hello | protocol version, release/build manifest identity, content identity, display name, room access credential if required |
| Lobby | join or leave a room; seat, side, colour and team changes; ready; the host's configuration changes |
| Command | the client's sequence number and the command payload (§7.4) |
| Resume seat | battle identity, seat credential, connection epoch and last acknowledged client sequence/stream position |
| Progress | the last tick run (for pacing) |
| Digest | checkpoint tick, digest and the relay-visible summary (below); sub-digests and the tick ring on request |
| Pacing request | pause, resume, speed — not accepted in the first releases (§15 Q5) |
| Seat request | own resignation; request relay removal under the agreed drop policy |
| Chat | recipient seats, resolved by the sender, or the spectator audience; text |
| Ping | for round-trip measurement |

Relay to client:

| Message | Content |
|---|---|
| Welcome | accepted versions, room list (hosted) |
| Lobby state | the room's configuration, seats and readiness |
| Prepare start | frozen configuration, its version/hash, seed pair, seat assignment and required readiness participants |
| Start | confirmation of the acknowledged configuration and agreed initial checkpoint; permits the first grant |
| Accepted | client sequence and immutable stream position; identical receipt on retransmission |
| Frame | ordered stream entries, optionally with a grant sealing a complete prefix; pacing notes and receipts can be delivered without granting ticks |
| Desync | checkpoint tick, sub-digest and tick-ring request, last sealed grant, and the room policy's outcome for each seat (§9.3) |
| Seat status | lag, waiting, disconnected, dropped, rejoining |
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
  become a watcher, for a pause rule that admits only playing seats, and
  that a battle is over, so the room can close and the recording can end.
  The summary lags the world by at most one digest interval. A pause rule
  can afford that; command authority cannot, and is decided inside the
  simulation (§7.2), where the facts are.

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
| Removing another seat | Only the relay authors another seat's removal event; clients can request their own resignation (§11.1). |
| Rewriting another seat's options or host status | The relay holds the agreed configuration; payloads cannot replace another seat's record or authority (§8.1). |
| Ordering another seat's units | The relay stamps the seat; every client's simulation drops commands whose actors the seat does not own (§7.2). |
| Changing one's own state (resources, health, build time) | Honest replicas do not apply uncommanded changes. Reported digests detect honest disagreements at the checkpoint cadence, but a malicious client can lie about its digest or maintain an honest copy. Receiver-side admission prevents unauthorized commands from becoming shared state; a trusted replayer certifies ranked results (§12.6). |
| Seeing through fog or bypassing match view restrictions | Every lockstep client holds the whole state, whether its source is open or closed. Honest clients enforce §8.4; hashes cannot prove camera compliance. Moderation/reports are limited measures; withholding hidden state needs §12.6's different delivery architecture. |
| Ordering attacks on units one cannot see | Casual behavior and the required competitive admission contract are distinct (§7.2, §15 Q9). |
| Automation, macros | Not preventable; community moderation. |
| Stalling, lag switching | Pacing, the waiting-for-player state, drop decisions (§4.4, §11.1). |
| Attacking an opponent's connection | Hosted relays avoid publishing player addresses. An embedded/direct host necessarily exposes its listening address; hosting remains a service attack surface. |
| Flooding the relay | Per-connection rate and size limits; tokens on hosted relays. |
| Forged results or replays | Casual streams are diagnostic records, not independently certified results. Ranked reporting requires the authenticated replayer/result service and a declared archive authentication policy (§12.6). |
| Seat theft or duplicate input on reconnect | Protected per-match seat credentials, fenced connection epochs, sequence receipts and deduplication (§11.2). |
| A client majority expelling an honest peer, or withholding hashes | Casual rooms accept a strict majority's digest and say so in the lobby (§9.3); rated rooms never do, and rely on bounded report deadlines and trusted authority. |
| Ending or stalling a battle by reporting a false digest | Casual: the dissenter is moved to rejoin and the others play on (§9.3). Rated: the match is uncertified, never awarded. |

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
| `internal/relay` | leaf | Rooms and room codes, acknowledged configuration, seat credentials and fencing, command receipts/deduplication, scheduling, grants, pause and speed, bounded digest comparison with the agreed summary and the room's desync policy, and stream retention | `netproto` |
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
- **Effect pool.** The fixed pool and its fragment and debris arithmetic are
  simulation state (L9) and live in `internal/render`, which the guards
  exempt as presentation by package. The code moves into an authoritative
  package or its files join the guarded set, so the map-order, float, fusion
  and goroutine guards read it, and a guard keeps any host from installing
  something the pool reads after composition.
- **Conversions and library calls.** The two I2 source guards of §5.4.

DESIGN_MODS_MUTATORS decision D5 — the client uses the network for the mod
manifest and downloads "and for nothing else" — was widened with adoption to
admit a relay connection (§15 Q1).

## 15. Decisions

The maintainer decided the original questions on 2026-10-01. Later research
adds four **OPEN** decisions, Q22–Q25; their recommendations are proposals
and none blocks M1. A later decision or revision is recorded here first.
Adoption authorizes §16 in order and no gameplay departure beyond those named
here. M1 implementation is expressly approved, with Go's BSD-licensed
pure-Go trigonometric routines adapted unfused and credited in `NOTICE.md`.
The integer-root fidelity audit O22 is authorized separately after M1.

| ID | Question | Decision (2026-10-01) |
|---|---|---|
| Q1 | Adopt relayed deterministic lockstep, move networking, replay and the lobby out of ARCHITECTURE §1 "Deliberately out of scope", and widen DESIGN_MODS_MUTATORS D5 so the client may also connect to a relay? | Yes (§2, §4, §14). |
| Q2 | Is multiplayer a mode-independent session kind, available under every gameplay mode, Strict 3.1 included, like Survival and the Modern AI? | Yes, with owner-machine equivalence and the online command-authorization exception of Q8; the agent instructions and the invariants were amended with this decision. Session and view policy cannot introduce a second gameplay registry: mechanical departures still use `session.RuleSet`. Single-player behavior is preserved except the declared M1/M2 changes. |
| Q3 | Is "Strict 3.1 online" defined by owner-machine equivalence (§6)? | Yes. Retail has no single multiplayer outcome to match (§3.2), and this is the reading that keeps every single-seat battle bit-identical. |
| Q4 | Which seat lends each computer seat its perspective? | The human that added it, recorded and fixed in configuration, matching the established retail host association (§6.6). Test that human becoming a watcher. The adopted computer-survival choice after its human leaves remains explicit in §11.1. A Modern policy using the computer's own sensor perspective requires a separate researched gameplay contract. |
| Q5 | Who may pause and change speed? | Nobody in the first releases: normal speed 10, thirty ticks a second, no pause or speed change. Relay grants still slow or hold for lag or disconnect. Retail authority is established: watchers may pause, only non-watchers may change speed, and receivers trust either from an admitted peer. Any later policy is decided before enabling §4.4. |
| Q6 | What happens to a dropped seat's units? | **Revised 2026-10-01:** retain the seat idle while rejoin is possible. On final removal Strict 3.1 destroys that seat's units; Modern and Community keep them idle. This is the Nanolathe Modern policy of §11.1, selected through `gameplay.Mode` and `session.RuleSet`. It does not remove the seat's hosted computers implicitly. |
| Q7 | Order of delivery: one simulation on every host, commands and identity, the digest, replays, one world per player, LAN co-op with a room-code relay, the public service, snapshots, then ranked? | Yes (§16). The canonical writer precedes digest-verified replay; exact restore has its own milestone and may move earlier if fast-forward rejoin misses its budget. |
| Q8 | Are cheat and debug commands available online? | Only with the agreed cheat permission, enforced on every receiving simulation in every online gameplay mode; competitive rooms always disable it. This includes the commands retail leaves ungated in every session kind that change the world — own-stock `+NoMetal`/`+NoEnergy`, `+Selectable` and `+LOSType` (§7.1) — and is an explicit Nanolathe online authorization exception, not historical Strict behavior. `+ShootAll`, when it is implemented, is classified under the same decision. The developer phrase and developer-only commands cannot bypass this permission or expand the allowed stream classes. Single-player permissions remain unchanged. |
| Q9 | Validate attack-target visibility? | Casual retains the researched admission. Ranked admission requires an approved delayed-observation/radar/direct-target contract with tests (§7.2); neither arbitrary client claims nor a current-sight-only check is accepted as the solution. Any new gameplay decision uses its existing owning rule seam. |
| Q10 | Who runs public relays? | The project runs one, reached by room code; the protocol is open and anyone may run more (§12). |
| Q11 | Identity: display names only, or accounts? | Display names until ranked play needs accounts. |
| Q12 | Do replays require the Nanolathe release that recorded them? | Yes, with matching simulation content and tested platform variants (§10). No cross-release compatibility program is planned. |
| Q13 | What decides a desync? | Casual rooms: a strict majority of participants plays on and each dissenting seat rejoins, or is dropped where rejoin is not yet offered; with no strict majority the battle ends without a result until snapshots exist (M8), and afterwards continues from the room creator's state. Rated rooms: no client majority decides, and the match is uncertified until a trusted replayer or referee rules (§9.3, §12.6). |
| Q14 | May a client that lacks the host's mod fetch it from the nanolathe.gg catalogue in the lobby? | Yes, through the existing verified download path (DESIGN_MODS_MUTATORS §5). |
| Q15 | Require identical catalogs, or negotiate a common roster as retail does? | Identical catalogs first; roster negotiation later, as a filter over the battle catalog like the restriction table. |
| Q16 | Implement retail's multiplayer unit-restriction table? | Yes, for Strict 3.1 fidelity, after the first networked release; it is a configuration field and a lobby screen (§3.3). |
| Q17 | Must the architecture support eventual competition? | **Yes, maintainer direction 2026-10-01.** Preserve scheduling/authorization boundaries and add verified results before ranked release: a trusted replayer, and a live referee only where rooms promise recovery mid-match. Who runs them, with the game content they need, and the integrity level, including whether filtered state delivery is required, remain explicit product decisions (§12.6). |
| Q18 | May a match restrict tactical views for every player? | **Yes, maintainer direction 2026-10-01.** Include shared view policy in the agreed configuration and enforce it in ordinary clients; no-zoom-out is the concrete first example (§8.4). A scale cap does not by itself equalize world viewport area or prevent modified-client bypass. |
| Q19 | Is cross-play with other engines a delivery goal? | **No planned support, maintainer direction 2026-10-01.** Focus on Nanolathe-to-Nanolathe with matching effective content/mods/settings. Other engines may be reconsidered later or dropped entirely (§8.5). |
| Q20 | How are releases, rooms and replays kept compatible? | One simulation build per room, shown in the lobby; a replay or a desync bundle names the release it needs (§8.2, §10). The installer keeps earlier releases on request so that old recordings still play, and no release is published before the cross-architecture and host-kind suites pass on it. |
| Q21 | May a snapshot rejoin restart the Modern AI's controllers on every client? | Yes (§9.1), subject to the play-test of §19 O21 before M8. The computer players lose their plans at that tick, as they already do across a save and load, in exchange for not requiring exact serialization of every brain. A room that forbids snapshot rejoin never restarts them. |
| Q22 | One shared alliance matrix or retail's per-machine stale copies? | **OPEN. Recommendation:** one shared matrix, with row B its transpose, as an explicitly approved online policy. The difference concerns the victory sweep's third-seat alliance knowledge among three or more seats; do not silently change Strict shared victory (§3.3). |
| Q23 | Computer-seat cap, difficulty and Deathmatch admission? | **OPEN. Recommendation:** retain the retail cap of one computer per human and Deathmatch exclusion in Strict rooms, and agree difficulty explicitly per computer seat instead of importing a machine-local value. Cap and exclusion outside Strict also need a stated choice. Nothing here selects the policy (§6.6). |
| Q24 | Respawn/watcher visibility rebuild: preserve retail's erased copies of remote explored history? | **OPEN. Recommendation:** keep one shared history with the own/hosted perspective rebuild, and document its departure before implementation. Retail wipes the entering machine's copies of every player's explored bits; a later allied map share therefore differs from copying the ally's full surviving history (§6.3). Per-perspective remote-history copies remain the alternative. |
| Q25 | Apply explored-map sharing on its request tick or reproduce transport-delay timing? | **OPEN. Recommendation:** apply on the request tick in canonical seat order. Retail applies at the recipient's later network drain. Any deterministic delay alternative needs a separately specified contract; no delay constant is assumed (§6.3). |

## 16. Delivery plan

Each milestone is useful on its own, lands behind the previous one, and leaves
every single-player battle bit-identical except where it says so.

| Milestone | Delivers | Done when |
|---|---|---|
| **M0 Adoption** | The §15 decisions; scope changes in ARCHITECTURE and the agent instructions; the online authorization and view policies; the invariant amendments (§5.4). | Done 2026-10-01: the maintainer decided §15 and the documents agree. |
| **M1 One simulation on every host** | Effect holds in `SimArt`, the pool timed from them on every host and the resolver seam removed (L9). The retail-defined distance routine, the angle tables, the in-repository `Atan2`/`Acos`/`Tan`, the defined conversions, their moved call sites and the two I2 source guards (L1). Two standing ratchets in the retail gate: the fingerprint locks from an amd64 build as well as an arm64 one, and a lock scene long enough to fill the Strict effect pool. | The distance routine implements `[01 R-DET-01 §7]`. Bit-pattern and engine-narrowing vectors agree on native darwin/arm64, linux/amd64 and windows/amd64. The fifteen existing locks are unchanged on both architectures, or each move is explained. A new Strict lock past the tick at which the pool fills equals the windowed probe's fingerprint, and a windowed and a headless run of one scene agree past that tick. Complete cross-platform world equivalence is tested after M3, not claimed from these vectors alone. |
| **M2 Commands, configuration and identity** | Seat-attributed commands, receiver-side permissions, allocation serials, local interface state, explicit wire schemas with the payload audit of §7.4, complete content/build/configuration identities and match view policy fields. | Codec fuzz/round-trip and hostile-input tests; script/model/SimArt mismatches refused; every effective match field changes identity as intended. Existing fingerprints move only by removed interface bits, demonstrated with those bits masked. |
| **M3 Canonical checkpoints and digest** | Reviewed state inventory with the effect pool in it, canonical writer and sub-digests, the tick ring, pump ends and the computer seats' applied-command hashes (§9). | Scripted single-seat scenes agree per checkpoint across supported platforms, host schedules and host kinds; a seeded one-field fault is located to its tick and owner from two rings alone; a checkpoint costs the same with AI workers idle and busy; state-owner checks and measured encoding budgets pass. Exact restore is not yet promised. |
| **M4 Replays** | Recorder, playback, pump ends, pacing notes, digest checks and a headless replay command. | Long Strict and Modern replays agree across platforms and between a windowed recording and headless playback, through pauses, speed changes and late AI workers. Reproducible desync diagnostics retain common-boundary evidence. |
| **M5 One world per player** | Perspective slots, per-seat blocks, latches and countdowns, the union gate ahead of the strips and the effect pool, and every other generalization in §6; the tail after every tick in lockstep battles (§4.5); the multi-seat harness (§17). | The harness passes for two to four human seats with computer seats under every registered rule set; single-seat fingerprint locks unchanged. |
| **M6 LAN and room codes** | Embedded relay; `cmd/nanolathe-server` hosting the same relay, reached by room code over TLS with rate and size limits and no accounts; sealed grants, relay pacing at fixed normal speed, the agreed summary, LAN/direct lobby, chat, departures, shared view restrictions, desync detection, bundles and the casual desync policy. | Mixed-platform battles finish on a LAN and through the hosted relay from behind ordinary home routers; hostile commands are refused; a resignation and a drop made while the relay holds its grants for a lagging seat apply correctly; a seeded desync in a three-seat battle leaves two seats playing; all zoom entry routes honor match restrictions. Impaired-network and CPU-stall runs meet declared budgets, set before acceptance. |
| **M7 Public service** | Room list, hardened and bounded rooms/queues, seat credentials and reconnect deduplication, measured fast-forward rejoin, delayed spectators and controlled archives. | Public play plus duplicate/half-open reconnect, slow-reader, digest-withholding and embargo-bypass tests pass. Advertise rejoin only within measured limits; move M8 earlier if necessary. |
| **M8 Exact snapshots and recovery** | Full world-state readers, the controller-restart event, checkpoint transfer and continuation (§9, §11.3). | Byte-identical round trip and matching long continuations across supported modes, with computer seats restarted at the snapshot tick on every replica; malformed snapshots refused; late-game rejoin meets a declared budget; a casual desync is repaired from an agreeing snapshot. These are Nanolathe snapshots, not retail saves. |
| **M9 Competitive admission** | Trusted replayer and authenticated results/accounts; a live referee where rooms promise recovery mid-match; approved scheduling, target-admission, pause/drop, watcher and view policies; integrity-level decision (§12.6). | Unequal-RTT/host-advantage tests, hostile-client admission, colluding-hash and result-forgery tests, loss of the result authority, spectator/replay isolation and published operating limits pass. Choose filtered state delivery before claiming hidden-state protection; refereed lockstep alone cannot claim it. |
| **Later product work** | Ratings/matchmaking UX, any multiplayer save product, and filtered state delivery if selected. | Scoped after the relevant correctness and integrity gates; no cross-engine milestone. |

### 16.1 M1 work units

M1 is five units. Each is one commit, owns its files exclusively, and lands
on its own through the gates of ARCHITECTURE §6. U1, U3 and U5 are
independent; U2 follows U1; U4 follows U2 and U3.

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
- **M1-C2 Radian functions.** Their results are the amd64 library's, bit for
  bit: asserted against the library on an amd64 host and against committed
  vectors everywhere. Every product is rounded before it is added, and the
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
  battle's own files. A missing bank or entry reports unknown.
- **M1-C8 One timing source.** The pool reads holds from `SimArt`, bound at
  composition before the first unit script runs. No host can supply timing
  afterwards, and the late hydration of records admitted without it is gone.
- **M1-C9 The pool lock.** The Strict benchmark scene is locked at step
  4,500, past tick 3,283 where its pool fills. The constant equals the
  2026-10-01 windowed probe's fingerprint at that step. The fifteen existing
  locks are unchanged.
- **M1-C10 Guards.** After U4 the map-order, float, fusion, goroutine and
  import guards read the pool's code, and `internal/render` holds no state
  that a tick reads.
- **M1-C11 Ratchets.** `tools/check-retail` runs the locks from an amd64
  build where the host can execute one and says SKIPPED where it cannot.

Co-op comes first: humans allied against computer players, and Survival with
several human survivors — DESIGN_SURVIVAL §1 already plans for a buddy row to
become a human seat with nothing in the wave director changing. Competitive
play is required eventually and has explicit additional mechanisms and gates.
Co-op validates the shared simulation and transport; it does not certify the
fairness or integrity of rated matches.

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
  is the failing baseline: equal through step 3,000 and different from step
  4,500, the first refusal falling at tick 3,283.
- **Standing ratchets from M1.** The fingerprint locks from an amd64 build
  and an arm64 build on every retail gate run whose host can execute both
  (they agreed fifteen of fifteen on 2026-10-01), and a lock scene that
  fills the Strict effect pool.
- **Codec and relay.** Round-trip and fuzz tests for every message; tick
  assignment, ordering, grant sealing, resignations and drops while grants
  are held, the agreed summary, readiness invalidation, rate/queue limits
  and desync handling tested in-process. Inject forbidden
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
- **Fingerprint locks.** Unchanged by every milestone except M2, as far as
  is measured: no locked scene fills its effect pool (L9) and a prototype of
  the distance routine left all fifteen unchanged (L1), so a lock that M1
  does move is explained before it lands. M2 moves them by the interface
  bits that leave the hashed status words. Each move carries its reason.
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
| O2 | Departed-player units/resources and hosted-computer termination | **Settled** by `[08 R-LEAVE-01]`: destruction and emptying, no redistribution, machine-group removal, timeout dialog and launched-battle host migration. Q6 records the revised final-removal policy; §11.1 preserves the separate computer-survival choice. |
| O3 | Explored-map sharing: `[05 R-SHARE-01 §6]` described it behind the share-mapping bit; `[03 R-VIS-01 §7]` said nothing is shared | **Settled 2026-10-01.** Both held of different stores: current sight is never merged, explored memory is copied on request, and the radar share marks the sharer's own units (§3.3, §6.3). Whether the share options survive into a second battle of one session stays **Unknown** in the research. |
| O4 | Computer hosting and online difficulty | **Settled** by `[08 R-SKIR-01 §13]` `[08 R-AI-01 §21]`: creator hosts one computer, association fixed, difficulty belongs to that machine. Q23 keeps the resulting product decisions open. |
| O5 | Alliance protocol and declaration timing | **Settled** by `[05 R-SHARE-01 §1]` `[07 R-FE-01 §7]` `[08 R-SKIR-01 §3]`: immediate sender write, target-only receipt, team propagation and victory reads. Q22 leaves the shared-matrix projection open. |
| O6 | Defeated player when watching is forbidden, including peer effects | **Settled** by `[08 R-SKIR-01 §3]` `[08 R-LEAVE-01 §10]`: ordinary lost ending unless a live hosted computer keeps the human watching; peers store the watch bit, and a later exit is ordinary departure. |
| O7 | Four pairs of research statements that conflicted: which host option the overlay's `Watching:` row reads; whether the lobby's command-line words preset the host's options; whether the Tab strip exists outside multiplayer; and which step makes a defeated player a watcher | **Settled 2026-10-01.** Bit 15 is *game closed* and the `Watching:` row reads bit 7 `[08 R-SKIR-01 §12]`. No command-line word presets the lobby; an online service's configuration file does `[01 R-PLAT-01 §2]` `[08 R-SKIR-01 §7]`. The Tab strip and the `h` key are multiplayer-only `[07 R-CAM-01 §2]`. The elimination block makes the watcher, and neither answer to the question changes that `[07 R-FE-01 §9]`. |
| O8 | `[04 R-COB-03 §6]` placed the effect gate outside the deterministic contract because a failed gate does nothing, while I4 counts the passing path's CRT draws as behaviour | **Settled 2026-10-01.** The section now says the failing branch is inert, the passing branch reaches the CRT stream and the strip pool, and the gate breaks no contract between retail's machines. |
| O9 | Floating-point results under `GOAMD64=v3` and on native amd64 hardware; only Rosetta was measured | M1's cross-architecture job. |
| O10 | Checkpoint and tick-ring cost, and a complete state inventory | M3's measurements and owner review; M8's exact-restore and continuation evidence. |
| O11 | Rejoin time on the largest/late battles and whether snapshot recovery must precede hosted release | Live-target catch-up measurements before promising M7 rejoin; advance M8 if the supported window cannot be met. |
| O12 | Kind-3 branches beyond those §6.4 lists | Each implementation unit audits the code it touches against the research. |
| O13 | Competitive scheduling, input/view age limits and acceptable latency advantage | Compare arrival scheduling and a shared-horizon candidate under unequal RTT/jitter and malicious reports; approve the concrete policy before M9. |
| O14 | Fair admission of delayed direct targets, radar contacts and area orders | Research the existing contracts, specify bounded authoritative observation evidence and validate against the scheduler; any gameplay departure follows DESIGN_GAMEPLAY_RULES. |
| O15 | Ranked integrity level: full-state lockstep with verified results, or filtered state delivery | An explicit product decision before advertising ranked guarantees; prototype and budget the different presentation/state protocol if required. |
| O16 | Whether competitive view policy also equalizes world viewport coverage | Decide whether resolution/aspect advantages are acceptable; otherwise define a common world-area bound and test all view/resize routes. No numeric viewport cap is assumed. |
| O17 | Final wire field bounds, start/pacing/reconnect state tables and supported build manifests | M2 schemas and hostile/state-machine tests; implementation must not infer these from Go layouts or sender UI behavior. |
| O18 | Retail's two-argument distance routine: the rounding sequence read on 2026-10-01 (L1), and the retail forms of the other library calls wherever a stored value could change `[01 R-DET-01 §3]` | **Settled 2026-10-01** for the distance routine: `[01 R-DET-01 §7]`, from two independent traces, its exits included. Still open: a manual retail observation that would confirm it (a reach test at an offset such as 20 by 99, where the routine's truncated distance is 100 and the exact one 101), and the retail forms of the other library calls. |
| O19 | How often the effect pool fills in play under each rule set, and whether the 4,096-event window is ever reached | **Settled 2026-10-01** by the census in L9: only the Strict benchmark fight fills its pool, no existing lock moves, and the event window peaks at 48 of 4,096. The path benchmark's players are all allied, so nothing there fires. |
| O20 | Machine scope of end state, respawn visibility and elimination draws | **Settled** by `[08 R-SKIR-01 §3]` `[08 R-ENTRY-01 §7]` `[08 R-CAMP-01 §9]` `[05 R-ECO-01 §1]`: per-machine countdown/latch, complete machine-grid rebuild, and one local CRT draw per elimination on each machine. Q24 leaves the history projection open; transport enumeration and remaining writer/caller scope questions stay in research. |
| O21 | Whether restarting Modern controllers from observation at a snapshot tick is acceptable play (§9.1, §15 Q21) | Play-test computer seats across forced restarts and compare with a save and load, which already restarts them. |
| O22 | Strict fidelity where the engine takes an exact integer root or compares squares and retail calls its distance routine on whole numbers — candidates seen while reading for M1 are the leash test, the guard and nano ranges and the air order distance. The routine truncates to one below the exact root on 722 ordered whole-number pairs up to 3,000, such as 20 by 99 `[01 R-DET-01 §7]`. This is not a lockstep hazard: integer code computes the same everywhere | An audit of each candidate against its research section. Then the authorized separate Strict change after M1 lands the routine, with every lock it moves explained. |
