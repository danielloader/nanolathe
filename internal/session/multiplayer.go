package session

import (
	"fmt"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/internal/visibility"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// NewPlaytestSkirmish composes the two-human Modern battle of
// DESIGN_MULTIPLAYER §16.4. The local seat selects presentation only. Admission
// runs before world allocation; broader configurations remain explicitly refused.
//
// The host's unit restrictions (field 12) are admitted since the first online
// lobby (§16.6): admission requires them to be the set the frozen catalog was
// restricted with, so every seat composes from the same restricted clone and
// records the same set (DESIGN_MODS_MUTATORS §15.5).
func NewPlaytestSkirmish(inputs *content.SimulationInputs, config EffectiveMatchConfig, localSeat uint8, progress content.Progress) (*Session, error) {
	admitted, err := admitMatch(config, inputs)
	if err != nil {
		return nil, err
	}
	r := admitted.request
	refuse := func() (*Session, error) {
		return nil, matchAdmissionError(ErrMatchConfigurationRejected, inputs, "playtest", "two hostile human seats in Modern skirmish, no cheats, watchers or Deathmatch (DESIGN_MULTIPLAYER §16.4)")
	}
	if localSeat > 1 || r.SessionKind != MatchOnlineSkirmish || r.RuleName != string(gameplay.Modern) || len(r.Seats) != 2 || r.CommanderDeath == 2 || r.CheatsAllowed || r.WatchingAllowed {
		return refuse()
	}
	for _, seat := range r.Seats {
		if seat.Role != MatchRoleHuman {
			return refuse()
		}
	}
	if r.Seats[0].AllyGroup != SkirmishDefaultAllyGroup && r.Seats[0].AllyGroup == r.Seats[1].AllyGroup {
		return refuse()
	}
	cfg, options := matchSkirmishSetup(r)
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	options.Progress = progress
	return composeSkirmish(skirmishEntry{cfg: cfg, features: r.Community, mission: admitted.mission, inputs: inputs, online: &config, localSeat: localSeat}, options, nil)
}

func (s *Session) sensorStatus(viewer uint8, u *units.Unit) uint32 {
	if u == nil {
		return 0
	}
	if s.Vis == nil {
		return u.Flags
	}
	return s.Vis.StatusForPerspective(visibility.PlayerID(viewer), uint16(u.Handle), u.AllocationSerial, visibility.PlayerID(u.Owner), u.Flags)
}

// PrepareGrantedBattle finishes ordinary entry dispatch without a wall-clock
// budget or a zero-tick pump. The local relay calls this before its ready barrier.
func (s *Session) PrepareGrantedBattle() error {
	if s == nil || s.onlineResults == nil {
		return fmt.Errorf("nanolathe: granted entry refused: logical path session, providers searched [session], expected a play-test multiplayer battle")
	}
	for i := 0; i < 10 && (s.State != StateBattle || s.IsPendingBattle()); i++ {
		s.Advance()
	}
	if s.State != StateBattle || s.IsPendingBattle() {
		return fmt.Errorf("nanolathe: granted entry failed: logical path session, providers searched [entry dispatch], expected a ready battle")
	}
	return nil
}

// StepGranted runs exactly the next sealed tick, including its executor tail.
// Socket arrival time and host update cadence never choose simulation work.
func (s *Session) StepGranted(tick uint32) error {
	if s == nil || s.onlineResults == nil || s.Clock == nil || s.State != StateBattle || s.IsPendingBattle() || tick == 0 || tick != s.Clock.GlobalTick+1 {
		return fmt.Errorf("nanolathe: granted tick refused: logical path tick %d, providers searched [session], expected the next tick of a ready online battle", tick)
	}
	s.ExecuteStep(StepPlan{run: true, ticks: 1})
	return nil
}

// OnlineBattleEnded reports the shared end boundary; a local result may become
// terminal earlier, while the other seat must still finish its own countdown.
func (s *Session) OnlineBattleEnded() bool {
	return s != nil && s.onlineResults != nil && s.onlineBattleEnded()
}

// Knowledge is checked before a queued build can cancel or replace anything.
// A stale builder remains the ordinary no-op; it discloses no site result.
func (s *Session) onlineBuildSiteKnown(seat uint8, p MobileBuildPayload) bool {
	if s.commandActor(seat, p.Builder) == nil {
		return true
	}
	if s.Catalog == nil || s.Build == nil || s.World == nil || s.Vis == nil {
		return false
	}
	def, ok := s.Catalog.Unit(p.Product)
	if !ok || def == nil {
		return false
	}
	geometry, err := s.Build.StructureGeometry(def, units.StructureFacing(p.Facing))
	if err != nil {
		return false
	}
	extent, err := world.NewFootprintExtent(geometry.FootprintX, geometry.FootprintZ)
	if err != nil {
		return false
	}
	placement, err := world.SnapMobilePlacement(p.Position.X, p.Position.Y, p.Position.Z, extent)
	if err != nil {
		return false
	}
	rect := placement.Rect()
	if !s.World.KnownPlacementSite(rect, &sessionPlacementViewer{vis: s.Vis, local: seat, player: seat}) {
		return false
	}
	height := s.World.SiteHeight(rect.MinX(), rect.MinZ(), geometry.Yard, int(geometry.FootprintX), int(geometry.FootprintZ), def.Waterline)
	return p.Position.Y == numeric.Fixed(int64(height)<<16)
}

// Union admission keeps effect lifetimes/RNG shared; each client's detached
// publication still uses its own sight (DESIGN_MULTIPLAYER §6.3). This is an
// online presentation policy, not a change to the shared pool's admission.
func (s *Session) filterOnlineVisuals(f *frame.Frame) {
	if s.onlineResults == nil {
		return
	}
	visible := func(x, y, z numeric.Fixed) bool {
		return s.Vis != nil && s.Vis.VisiblePoint(visibility.PlayerID(s.ViewingOwner), x, y, z)
	}
	n := 0
	for i := range f.Effects {
		v := &f.Effects[i]
		if visible(v.X, v.Y, v.Z) {
			// Swap preserves distinct reusable duration buffers in discarded slots.
			f.Effects[n], f.Effects[i] = f.Effects[i], f.Effects[n]
			n++
		}
	}
	f.Effects = f.Effects[:n]
	n = 0
	for i := range f.Debris {
		v := &f.Debris[i]
		if visible(v.X, v.Y, v.Z) {
			f.Debris[n], f.Debris[i] = f.Debris[i], f.Debris[n]
			n++
		}
	}
	f.Debris = f.Debris[:n]
}
