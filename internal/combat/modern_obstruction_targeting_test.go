package combat

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/cob"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/economy"
	"github.com/nanolathe-gg/nanolathe/internal/model"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/sim/rng"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport/retailcat"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// Nanolathe Modern policy: DESIGN_WEAPONS_PROJECTILES "Modern threat targeting
// and incoming fire". Physical refusal must not pin the nearest opportunity
// while another candidate has an unobstructed current direct trajectory.
func TestModernObstructionTargetRanking(t *testing.T) {
	for _, blocker := range []string{"wreck", "ridge", "friend"} {
		t.Run(blocker, func(t *testing.T) {
			s, w, terrain, shooter, solar, weapon := modernCombatFixture(t)
			h, err := w.Create(solar.Def, 1, solar.X, solar.Y, cellCentre(5))
			if err != nil {
				t.Fatal(err)
			}
			vehicle := w.Unit(h)
			stampGroundRect(terrain, 5, 5, 1, 1, vehicle.Handle)
			switch blocker {
			case "wreck":
				terrain.FeatureDefs[0].Height = 64
				terrain.PlotAt(3, 1).SetFeature(0)
			case "ridge":
				terrain.PlotAt(3, 1).SetMinHeight(64)
			case "friend":
				friend, err := w.Create(contactDef(contactModelTop), 0, cellCentre(3), 0, cellCentre(1))
				if err != nil {
					t.Fatal(err)
				}
				stampGroundRect(terrain, 3, 1, 1, 1, friend)
			}
			slot := shooter.SlotAt(0)
			slot.Target = units.Target{Kind: units.TargetUnit, Unit: solar.Handle}
			q := modernTargetQuery(s, w, terrain, shooter, solar, vehicle)
			if !s.modernTargetObstructed(&q, solar) || s.modernTargetObstructed(&q, vehicle) {
				t.Fatal("fixture does not separate blocked and clear trajectories")
			}
			for _, candidates := range [][]Candidate{q.Candidates, {q.Candidates[1], q.Candidates[0]}} {
				q.Candidates = candidates
				if got, _ := s.rules().SelectTarget(s, &q); got != vehicle.Handle {
					t.Fatalf("blocked retained opportunity won over clear vehicle: %d", got)
				}
			}
			// Even a higher-scoring threat cannot pin a shot that Modern holds.
			solar.InstallWeapon(0, weapon)
			if got, _ := s.rules().SelectTarget(s, &q); got != vehicle.Handle {
				t.Fatal("blocked threat score defeated clear-trajectory preference")
			}
			// The launch gate still holds the old target. With no alternative,
			// maintenance preserves it, and retries as geometry changes.
			q = modernTargetQuery(s, w, terrain, shooter, solar)
			if got, _ := s.rules().SelectTarget(s, &q); got != solar.Handle {
				t.Fatal("sole blocked target was released")
			}
		})
	}
}

func TestModernObstructionAutomaticMaintenanceAndStrictBypass(t *testing.T) {
	for _, mode := range []struct {
		name string
		rule Rules
	}{
		{"modern", &ModernRules{}}, {"strict", StrictRules{}}, {"community", CommunityRules{}},
	} {
		for _, ordered := range []bool{false, true} {
			intent := "/automatic"
			if ordered {
				intent = "/ordered"
			}
			t.Run(mode.name+intent, func(t *testing.T) {
				s, w, terrain, shooter, solar, _ := modernCombatFixture(t)
				s.Rules = mode.rule
				h, err := w.Create(solar.Def, 1, solar.X, solar.Y, cellCentre(5))
				if err != nil {
					t.Fatal(err)
				}
				terrain.FeatureDefs[0].Height = 64
				terrain.PlotAt(3, 1).SetFeature(0)
				s.targets.primary[shooter.Owner] = []pool.Handle{solar.Handle, h}
				slot := shooter.SlotAt(0)
				slot.Target = units.Target{Kind: units.TargetUnit, Unit: solar.Handle}
				if ordered {
					slot.Flags &^= units.SlotFlagAutonomous
				}
				econ := &economy.Service{}
				econ.Players[0].Stock = [2]float32{50, 500}
				random := rng.NewSimulation(77)
				beforeSlot, beforeStock, beforeRNG := *slot, econ.Players[0], random
				for i := 0; i < 40; i++ {
					s.StepAutonomousForPlayer(shooter.Owner, w, nil, terrain, econ, nil, &random)
				}
				want := solar.Handle
				if mode.name == "modern" && !ordered {
					want = h
				}
				if slot.Target.Kind != units.TargetUnit || slot.Target.Unit != want {
					t.Fatalf("target=%+v, want %d", slot.Target, want)
				}
				beforeSlot.Target = slot.Target
				if *slot != beforeSlot || econ.Players[0] != beforeStock || random != beforeRNG || s.Count() != 0 {
					t.Fatal("maintenance changed weapon work, resources, RNG or projectiles")
				}
			})
		}
	}
}

func TestModernConstantSweetSpotCell(t *testing.T) {
	// Authored COB words, not copied retail bytes. The returned scalar differs
	// from cell zero so this cannot accidentally recognize RETURN as the piece.
	literal := []uint32{0x10021001, 3, 0x10023002, 0, 0x10021001, 17, 0x10065000}
	for _, allocated := range []bool{false, true} {
		code := append([]uint32(nil), literal...)
		if allocated {
			code = append([]uint32{0x10022000}, code...)
		}
		program := &cob.Program{Code: code, Scripts: map[string]int{"SweetSpot": 0}, ScriptsByID: []int{0}}
		piece, known := modernConstantSweetSpotPiece(program)
		result := cob.NewCallbackBridge(cob.NewVM(program)).SweetSpot()
		if !known || piece != 3 || !result.Completed || result.QueryValue() != piece {
			t.Fatalf("literal cell recognition differs from Q query: piece=%d known=%v query=%+v", piece, known, result)
		}
	}
	for _, code := range [][]uint32{
		{0x10021002, 1, 0x10023002, 0, 0x10021001, 17, 0x10065000}, // state-dependent input
		{0x10021001, 3, 0x10023002, 1, 0x10021001, 17, 0x10065000}, // another cell
		{0x10021001, 3, 0x10023004, 0, 0x10021001, 17, 0x10065000}, // static write
		append([]uint32{0x10022000, 0x10022000}, literal...),       // unsupported local layout
		append([]uint32{0x10021001, 1, 0x10023004, 0}, literal...), // earlier side effect
	} {
		if _, known := modernConstantSweetSpotPiece(&cob.Program{Code: code, Scripts: map[string]int{"SweetSpot": 0}}); known {
			t.Fatal("nonliteral or side-effecting query supplied a predicted aim point")
		}
	}
	for n := 0; n < len(literal); n++ {
		if _, known := modernConstantSweetSpotPiece(&cob.Program{Code: literal[:n], Scripts: map[string]int{"SweetSpot": 0}}); known {
			t.Fatalf("truncated query of %d words was admitted", n)
		}
	}
	for _, pc := range []int{-1, len(literal), len(literal) + 1} {
		if _, known := modernConstantSweetSpotPiece(&cob.Program{Code: literal, Scripts: map[string]int{"SweetSpot": pc}}); known {
			t.Fatal("invalid entry supplied a predicted aim point")
		}
	}
	if piece, known := modernConstantSweetSpotPiece(&cob.Program{}); !known || piece != 0 {
		t.Fatal("absent SweetSpot lost the established zero query seed")
	}
}

func TestModernConstantSweetSpotUnavailableQueryKeepsOrdinaryRank(t *testing.T) {
	s, w, terrain, shooter, target, _ := modernCombatFixture(t)
	// Authored literal cell one; its scalar return is deliberately unrelated.
	// The root centre is high enough to clear this wreck, but piece one is low.
	program := &cob.Program{
		Code:        []uint32{0x10021001, 1, 0x10023002, 0, 0x10021001, 17, 0x10065000, 0x10065000},
		Scripts:     map[string]int{"SweetSpot": 0, "Hold": 7},
		ScriptsByID: []int{0, 7},
		Pieces:      []string{"root", "low"},
	}
	vm := cob.NewVM(program)
	bridge := cob.NewCallbackBridge(vm)
	random := rng.NewSimulation(77)
	vm.SetSimulationRNG(&random)
	binding := &cob.Binding{Program: program, VM: vm, Callbacks: bridge,
		PieceMap: []int{0, 1}, Model: &model.Model{Pieces: []model.Piece{
			{Name: "root", Vertices: [][3]numeric.Fixed{{0, numeric.FixedFromInt(64), 0}}},
			{Name: "low"},
		}}}
	target.ScriptState = &units.ScriptState{VM: vm, Binding: binding, Bridge: bridge}
	terrain.PlotAt(3, 1).SetFeature(0)
	q := modernTargetQuery(s, w, terrain, shooter, target)
	if got := UnitTargetPoint(target); got != (Vec3{X: target.X, Y: target.Y, Z: target.Z}) {
		t.Fatalf("callable literal query did not select its low piece: %v", got)
	}
	if !s.modernTargetObstructed(&q, target) {
		t.Fatal("callable literal piece did not establish the fixture's obstruction")
	}
	for idx := range vm.Threads {
		if !vm.Start(7, nil) {
			t.Fatalf("failed to occupy query thread %d", idx)
		}
	}
	beforeThreads, beforeRNG := vm.Threads, random
	wantSeededPoint := Vec3{X: target.X, Y: target.Y.Add(numeric.FixedFromInt(32)), Z: target.Z}
	if got := UnitTargetPoint(target); got != wantSeededPoint {
		t.Fatalf("full query pool did not preserve piece-zero seed: got=%v want=%v", got, wantSeededPoint)
	}
	if s.modernTargetObstructed(&q, target) {
		t.Fatal("full query pool falsely proved the unavailable literal-piece obstruction")
	}
	if vm.Threads != beforeThreads || random != beforeRNG {
		t.Fatal("full-pool resolution or forecast changed threads or randomness")
	}
	availableVM := cob.NewVM(program)
	availableBridge := cob.NewCallbackBridge(availableVM)
	availableThreads := availableVM.Threads
	for _, unavailable := range []func(){
		func() { binding.Callbacks = nil },
		func() { binding.Callbacks = cob.NewCallbackBridge(nil) },
		func() { binding.VM = nil },
	} {
		binding.VM, binding.Callbacks = availableVM, availableBridge
		unavailable()
		if s.modernTargetObstructed(&q, target) {
			t.Fatal("unavailable callback owner supplied a literal-piece flight proof")
		}
	}
	// An absent callback is already known to leave the seed, even without a VM.
	binding.Program = &cob.Program{}
	binding.Callbacks = nil
	if s.modernTargetObstructed(&q, target) {
		t.Fatal("absent callback failed to use the clear piece-zero geometry")
	}
	binding.Model.Pieces[0].Vertices = nil
	if !s.modernTargetObstructed(&q, target) {
		t.Fatal("absent callback was treated as unknown despite a known blocked piece-zero point")
	}
	if vm.Threads != beforeThreads || availableVM.Threads != availableThreads || random != beforeRNG {
		t.Fatal("callback availability inspection changed threads or randomness")
	}
}

func TestModernObstructionRankingUncertaintyAndBlockedRetention(t *testing.T) {
	s, w, terrain, shooter, solar, weapon := modernCombatFixture(t)
	h, err := w.Create(solar.Def, 1, solar.X, solar.Y, cellCentre(5))
	if err != nil {
		t.Fatal(err)
	}
	vehicle := w.Unit(h)
	terrain.FeatureDefs[0].Height = 64
	terrain.PlotAt(3, 1).SetFeature(0)
	terrain.PlotAt(3, 3).SetFeature(0)
	shooter.SlotAt(0).Target = units.Target{Kind: units.TargetUnit, Unit: vehicle.Handle}
	q := modernTargetQuery(s, w, terrain, shooter, solar, vehicle)
	if !s.modernTargetObstructed(&q, solar) || !s.modernTargetObstructed(&q, vehicle) {
		t.Fatal("both-targets-blocked fixture has a clear alternative")
	}
	if got, _ := s.rules().SelectTarget(s, &q); got != vehicle.Handle {
		t.Fatal("all-blocked ranking discarded ordinary retention hysteresis")
	}
	weapon.Ballistic, weapon.LineOfSight = true, false
	if s.modernTargetObstructed(&q, solar) {
		t.Fatal("unsupported family was penalized without a flight proof")
	}
	weapon.Ballistic, weapon.LineOfSight = false, true
	// A bound target with no resolvable piece does not inherit a guessed origin.
	solar.ScriptState = &units.ScriptState{Binding: &cob.Binding{Program: &cob.Program{}}}
	if s.modernTargetObstructed(&q, solar) {
		t.Fatal("unresolved bound target piece supplied a flight proof")
	}
}

func TestModernInstalledSentinelObstructionTargeting(t *testing.T) {
	catalog, fs := retailcat.Shared(t)
	shooterDef, solarDef, vehicleDef := catalog.Units["armhlt"], catalog.Units["armsolar"], catalog.Units["armcv"]
	if shooterDef == nil || solarDef == nil || vehicleDef == nil || shooterDef.Weapon1Def == nil {
		t.Fatal("installed Sentinel, solar or construction vehicle missing")
	}
	s, _, terrain, _, _, _ := modernCombatFixture(t)
	w := units.NewSliced(10, catalog)
	w.SetCOBSource(fs, cob.NewCachedLoader())
	random := rng.NewSimulation(77)
	w.SetCOBBinder(func(u *units.Unit) error {
		mdl, err := model.Load(fs, "objects3d/"+u.Def.ObjectName+".3do")
		if err != nil {
			return err
		}
		_, err = units.BindCOBWithPortsAndVisibilityForUnit(fs, u, mdl, &random, nil, nil)
		return err
	})
	makeUnit := func(def *content.UnitDef, owner uint8, x, z int32) *units.Unit {
		h, err := w.Create(def, owner, cellCentre(x), 0, cellCentre(z))
		if err != nil {
			t.Fatal(err)
		}
		return w.Unit(h)
	}
	shooter, solar, vehicle := makeUnit(shooterDef, 0, 4, 8), makeUnit(solarDef, 1, 16, 8), makeUnit(vehicleDef, 1, 16, 16)
	// Models, scripts and definitions are installed content; geometry remains
	// an authored scenario, not a claimed reproduction of the screenshot.
	terrain.FeatureDefs[0].Height = 64
	terrain.PlotAt(11, 8).SetFeature(0)
	shooter.SlotAt(0).Target = units.Target{Kind: units.TargetUnit, Unit: solar.Handle}
	q := modernTargetQuery(s, w, terrain, shooter, solar, vehicle)
	before := random
	if got, _ := s.rules().SelectTarget(s, &q); got != vehicle.Handle {
		t.Fatalf("installed Sentinel retained wreck-blocked solar: %d", got)
	}
	if random != before {
		t.Fatal("candidate ranking consumed script randomness")
	}
	terrain.PlotAt(11, 8).SetFeature(world.PlotFeatureNone)
	if got, _ := s.rules().SelectTarget(s, &q); got != solar.Handle {
		t.Fatal("removing the wreck did not restore solar eligibility")
	}
}
