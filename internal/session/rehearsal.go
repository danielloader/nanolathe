package session

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"hash"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// The pre-start rehearsal of DESIGN_MULTIPLAYER §16.7. Before a seat readies,
// it composes a private battle from the match's frozen inputs and
// configuration, runs a fixed script of ordinary seat commands for both seats
// and reports the digest of what happened. The relay starts the match only
// when both seats report the same digest, so two clients whose simulations
// disagree on this content are refused before play rather than at the first
// 30-tick unit checksum. It is a behavioral check of the build and the
// content together: it needs no stamp and vouches only for what it ran.
//
// Everything here is Nanolathe protocol, not retail behavior: the script,
// its timing and geometry constants and the digest encoding. Changing any of
// them changes every digest; such a change is a protocol change and bumps
// rehearsalDomain with it.

// rehearsalDomain separates the rehearsal digest from every other digest.
const rehearsalDomain = "nanolathe/rehearsal/v1"

// The script's timing and geometry. Ticks are granted ticks at 30 Hz.
const (
	// rehearsalTicks is the rehearsal's length: 30 seconds of battle.
	rehearsalTicks = 900
	// rehearsalMoveTick is when each seat's lead unit is ordered to move.
	rehearsalMoveTick = 1
	// rehearsalBuildTick is when each lead unit is ordered to build.
	rehearsalBuildTick = 90
	// rehearsalAttackTick is when each lead unit is ordered to fire at its
	// own build site.
	rehearsalAttackTick = 480
	// rehearsalMoveDistance is the move's offset on each axis toward the
	// map's centre, in world units.
	rehearsalMoveDistance = 128
	// rehearsalMoveMargin keeps a move target this many world units inside
	// the map's edges.
	rehearsalMoveMargin = 32
	// rehearsalSiteRings is how many square rings of footprint cells the
	// build-site search visits beyond the first ring whose sites clear the
	// builder's own footprint.
	rehearsalSiteRings = 10
)

// RehearsalDigest runs the room's pre-start rehearsal: a short deterministic
// battle composed from the same frozen inputs and configuration as the match,
// driven by a fixed script of commands for both seats, and returns the digest
// of its final state (DESIGN_MULTIPLAYER §16.7). Two seats whose simulations
// agree report the same digest; the relay starts the match only then.
//
// The rehearsal composes its own session through the match's constructor,
// NewPlaytestSkirmish, and never touches another: the caller's match
// session, composed from the same inputs, is unaffected. Sessions only read
// their inputs — the catalog, models and animation table are immutable and
// the sealed view locks its own lookups — so one value backs the match and
// its rehearsal, and the rehearsal, which starts no goroutine of its own, may
// run on the caller's goroutine of choice beside an idle or running match.
//
// The digest is independent of the local seat, of local preferences and of
// host timing: the rehearsal always composes as seat 0, a seat's commands are
// computed from the rehearsal's own state alone, and nothing reads a clock.
func RehearsalDigest(inputs *content.SimulationInputs, config EffectiveMatchConfig) ([32]byte, error) {
	return rehearse(inputs, config, 0, nil)
}

// rehearse runs the rehearsal composed for localSeat. Production always
// passes seat 0; the tests pass the other seat to show that the digest does
// not depend on it. observe, when non-nil, sees the session after every
// granted tick and must not change it.
func rehearse(inputs *content.SimulationInputs, config EffectiveMatchConfig, localSeat uint8, observe func(*Session)) ([32]byte, error) {
	s, err := NewPlaytestSkirmish(inputs, config, localSeat, nil)
	if err != nil {
		return [32]byte{}, err
	}
	defer s.closeAIControllers()
	if err := s.PrepareGrantedBattle(); err != nil {
		return [32]byte{}, err
	}
	r := rehearsal{s: s, h: sha256.New()}
	r.h.Write([]byte(rehearsalDomain))
	r.findLeads()
	tick := uint32(0)
	// A battle that ends early, such as by a commander's death, stops the
	// script at that tick on every replica.
	for tick < rehearsalTicks && s.State == StateBattle && !s.OnlineBattleEnded() {
		tick++
		if err := r.issue(tick); err != nil {
			return [32]byte{}, err
		}
		if err := s.StepGranted(tick); err != nil {
			return [32]byte{}, err
		}
		r.recordTick()
		if observe != nil {
			observe(s)
		}
	}
	return r.finish(tick)
}

// rehearsal is one running rehearsal: its session, the running digest, the
// last stream position it assigned, and each seat's lead unit and build site.
type rehearsal struct {
	s        *Session
	h        hash.Hash
	position uint64
	leads    [2]pool.UnitRef
	sites    [2]CommandPoint
	sited    [2]bool
}

// findLeads picks each seat's lead unit: its first live unit in slice order
// at battle entry, which is the commander an ordinary start creates first. A
// seat with no unit has a null lead, and its commands are skipped.
func (r *rehearsal) findLeads() {
	for seat := range r.leads {
		found := false
		r.s.Units.ForEachPlayerSliceLive(seat, func(u *units.Unit) {
			if !found {
				r.leads[seat] = pool.UnitRef{Handle: u.Handle, Serial: u.AllocationSerial}
				found = true
			}
		})
	}
}

// lead returns seat's lead unit while that allocation is alive.
func (r *rehearsal) lead(seat int) *units.Unit {
	ref := r.leads[seat]
	if ref.Handle == 0 {
		return nil
	}
	u := r.s.Units.Unit(ref.Handle)
	if u == nil || !u.Alive || u.AllocationSerial != ref.Serial || u.Def == nil {
		return nil
	}
	return u
}

// issue enqueues the script's commands for tick, seat 0 first, as one stream
// would order them. Each command is computed from the rehearsal's state after
// the previous tick, which every replica holds identically. The script is the
// same for both seats: move toward the map's centre, build the first
// structure on the lead's own build list at the nearest known legal site, and
// fire at that site with an ordinary attack order. A command the session
// refuses or cannot carry out is refused identically on every replica.
func (r *rehearsal) issue(tick uint32) error {
	for seat := range r.leads {
		u := r.lead(seat)
		if u == nil {
			continue
		}
		ref := r.leads[seat]
		var c SeatCommand
		switch tick {
		case rehearsalMoveTick:
			x, z := r.moveTarget(u)
			c = SeatCommand{Kind: SeatOrder, Order: OrderPayload{Actors: []pool.UnitRef{ref}, Code: 2,
				Position: CommandPosition{X: x, Y: r.s.World.HeightAt(x, z), Z: z}}}
		case rehearsalBuildTick:
			def := r.product(u)
			if def == nil {
				continue
			}
			site, ok := r.site(uint8(seat), u, def)
			if !ok {
				continue
			}
			r.sites[seat], r.sited[seat] = site, true
			c = SeatCommand{Kind: SeatMobileBuild, MobileBuild: MobileBuildPayload{Builder: ref, Product: def.CanonicalKey, Position: site}}
		case rehearsalAttackTick:
			if !r.sited[seat] {
				continue
			}
			site := r.sites[seat]
			c = SeatCommand{Kind: SeatOrder, Order: OrderPayload{Actors: []pool.UnitRef{ref}, Code: 3,
				Position: CommandPosition{X: site.X, Y: site.Y, Z: site.Z}}}
		default:
			continue
		}
		r.position++
		if err := r.s.EnqueueSeatCommand(CommandStamp{Seat: uint8(seat), Tick: tick, Position: r.position}, c); err != nil {
			return err
		}
	}
	return nil
}

// moveTarget is the lead's position moved rehearsalMoveDistance toward the
// map's centre on each axis, kept inside the map.
func (r *rehearsal) moveTarget(u *units.Unit) (numeric.Fixed, numeric.Fixed) {
	step := func(at numeric.Fixed, cells int32) numeric.Fixed {
		size := numeric.Fixed(int64(cells) * 16 << 16)
		offset := numeric.Fixed(rehearsalMoveDistance << 16)
		if at > size/2 {
			offset = -offset
		}
		lo, hi := numeric.Fixed(rehearsalMoveMargin<<16), size-numeric.Fixed(rehearsalMoveMargin<<16)
		return min(max(at+offset, lo), hi)
	}
	return step(u.X, r.s.World.CellW), step(u.Z, r.s.World.CellH)
}

// product is the first structure on the lead's own build list, as the bound
// construction rules present it, or nil when it has none.
func (r *rehearsal) product(u *units.Unit) *content.UnitDef {
	for _, key := range r.s.buildProducts(u.Def.UnitName) {
		if def, ok := r.s.Catalog.Unit(key); ok && def != nil && def.BMCode == 0 {
			return def
		}
	}
	return nil
}

// site finds the nearest legal, known build site for def around u, searching
// square rings of cells nearest first and each ring in row order. The first
// ring is the nearest whose sites clear the builder's own footprint, so the
// builder never stands on its site. It tests placement through the issuing
// seat's own knowledge and passes no builder, so the answer never depends on
// the local seat, and it requires the exact known-site admission phase 1
// applies to the command it becomes.
func (r *rehearsal) site(seat uint8, u *units.Unit, def *content.UnitDef) (CommandPoint, bool) {
	s := r.s
	geometry, err := s.Build.StructureGeometry(def, units.FacingSouth)
	if err != nil {
		return CommandPoint{}, false
	}
	fx, fz := geometry.FootprintX, geometry.FootprintZ
	cx, cz := int32(u.X>>20)-fx/2, int32(u.Z>>20)-fz/2
	viewer := &sessionPlacementViewer{vis: s.Vis, local: seat, player: seat}
	first := (max(fx, fz)+max(u.Def.FootprintX, u.Def.FootprintZ))/2 + 1
	for ring := first; ring < first+rehearsalSiteRings; ring++ {
		for dz := -ring; dz <= ring; dz++ {
			for dx := -ring; dx <= ring; dx++ {
				if max(abs32(dx), abs32(dz)) != ring {
					continue
				}
				x, z := cx+dx, cz+dz
				result, err := s.previewPlacement(x, z, def, fx, fz, 0, viewer, units.FacingSouth)
				if err != nil {
					continue
				}
				point := CommandPoint{X: numeric.Fixed(int64(fx+2*x) << 19), Y: numeric.Fixed(int64(result.SiteHeight) << 16), Z: numeric.Fixed(int64(fz+2*z) << 19)}
				if s.onlineBuildSiteKnown(seat, MobileBuildPayload{Builder: r.leads[seat], Product: def.CanonicalKey, Position: point}) {
					return point, true
				}
			}
		}
	}
	return CommandPoint{}, false
}

func abs32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

// recordTick folds one granted tick into the digest: each receipt phase 1
// produced, as its stream position u64 and outcome u8; the play-test unit
// checksum (DESIGN_MULTIPLAYER §16.4); then the simulation stream's state u32
// and draw count u64 and the CRT stream's state u32 and draw count u64, all
// little-endian.
func (r *rehearsal) recordTick() {
	var b [8]byte
	for _, receipt := range r.s.DrainCommandReceipts() {
		binary.LittleEndian.PutUint64(b[:], receipt.Stamp.Position)
		r.h.Write(b[:8])
		r.h.Write([]byte{byte(receipt.Outcome)})
	}
	sum := r.s.UnitStateChecksum()
	r.h.Write(sum[:])
	sim, crt := r.s.SimRNG(), r.s.CrtRNG()
	binary.LittleEndian.PutUint32(b[:4], sim.State)
	r.h.Write(b[:4])
	binary.LittleEndian.PutUint64(b[:], sim.Draws())
	r.h.Write(b[:])
	binary.LittleEndian.PutUint32(b[:4], crt.State)
	r.h.Write(b[:4])
	binary.LittleEndian.PutUint64(b[:], crt.Draws())
	r.h.Write(b[:])
}

// finish closes the digest with the number of ticks run, u32 little-endian,
// and the text of the final state's partial fingerprint, which adds what the
// unit checksum omits: the units' scripts, pieces, orders and weapon slots,
// the economy, pending paths and routes, and the features.
func (r *rehearsal) finish(ticks uint32) ([32]byte, error) {
	fingerprint, err := r.s.PartialStateFingerprint()
	if err != nil {
		return [32]byte{}, fmt.Errorf("nanolathe: rehearsal: %w", err)
	}
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], ticks)
	r.h.Write(b[:])
	r.h.Write([]byte(fingerprint))
	var out [32]byte
	r.h.Sum(out[:0])
	return out, nil
}
