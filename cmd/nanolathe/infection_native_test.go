package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/construction"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/headless"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/session"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/sim/rng"
	"github.com/nanolathe-gg/nanolathe/internal/survival"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// Actual authored Crawler + retail host, ordinary approach/COB/order phases.
// The target holds fire to isolate takeover from combat lethality. No stance,
// callback result, timer or ownership is fabricated by this fixture.
func TestNativeCrawlerInfectionApproachAndReplacement(t *testing.T) {
	mount := vfs.New()
	t.Cleanup(func() { _ = mount.Close() })
	if err := mount.MountGameDirectory(testsupport.RetailRoot(t)); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("..", "..", "art", "infected-ancients", "content")
	var files []vfs.ArchiveFile
	if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files = append(files, vfs.ArchiveFile{Path: filepath.ToSlash(rel), Data: data})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var data bytes.Buffer
	if err := vfs.WriteArchive(&data, files, vfs.ArchiveWriteOptions{Compress: true, Year: 2026}); err != nil {
		t.Fatal(err)
	}
	if _, err := mount.MountArchiveReader("infected-native-test.ufo", bytes.NewReader(data.Bytes()), int64(data.Len()), 100, vfs.ArchiveOptions{}); err != nil {
		t.Fatal(err)
	}
	cat, err := content.Compile(mount)
	if err != nil {
		t.Fatal(err)
	}
	checkNativeInfectedRoster(t, cat)
	for _, tc := range []struct {
		mode   gameplay.Mode
		moving bool
	}{{gameplay.Modern, false}, {gameplay.Modern, true}, {gameplay.Strict31, false}} {
		mode := tc.mode
		label := string(mode)
		if tc.moving {
			label += "-retreat"
		}
		t.Run(label, func(t *testing.T) {
			battle, err := headless.ComposeFreshBattle(headless.FreshBattleRequest{Kind: headless.ScenarioSurvival, Map: "painted desert", Skirmish: session.SurvivalSkirmishConfig("painted desert", 0, session.SurvivalOptions{}), Gameplay: mode, LocalOwner: 0, SimulationSeed: 7, CRTSeed: 7, FS: mount, Catalog: cat})
			if err != nil {
				t.Fatal(err)
			}
			s := battle.Session
			for _, m := range s.AI {
				if m != nil {
					m.Passive = true
				}
			}
			owner, _ := s.SurvivalAttacker()
			create := func(key string, owner uint8, x int64) *units.Unit {
				def, ok := s.Catalog.Unit(key)
				if !ok {
					t.Fatal(key)
				}
				wx, wz := numeric.FixedFromInt(x), numeric.FixedFromInt(1240)
				h, err := s.Units.Create(def, owner, wx, s.World.HeightAt(wx, wz), wz)
				if err != nil {
					t.Fatal(err)
				}
				s.CompleteUnit(h)
				return s.Units.Unit(h)
			}
			actor, victim := create("iacrawl", uint8(owner), 1600), create("armpw", 0, 1440)
			victim.Flags &^= units.StandingFieldMask << units.StandingFireShift
			if tc.moving {
				s.BindStagedOrderQueue(victim)
				move := orders.Lookup("Move_Ground")
				vq := orders.QueueForUnit(victim)
				vq.PurgeUnprotected()
				vq.DropLeadingAutoOps()
				vq.Push(move, orders.NewNodeForOrder(move, 0, numeric.FixedFromInt(1000), victim.Y, victim.Z, s.Clock.GlobalTick, victim.Handle, false))
			}
			for i := 0; i < 3; i++ {
				s.Step(s.Clock.ScaledAnchor + 1)
			}
			id := orders.Resolve(13, actor, victim, nil)
			if mode == gameplay.Strict31 {
				if id != 0 {
					t.Fatal("Strict native Crawler gained capture")
				}
				return
			}
			if id == 0 {
				t.Fatal("Modern native Crawler cannot infect")
			}
			q := orders.QueueForUnit(actor)
			q.PurgeUnprotected()
			q.DropLeadingAutoOps()
			q.Push(id, orders.NewNodeForOrder(id, victim.Handle, victim.X, victim.Y, victim.Z, s.Clock.GlobalTick, actor.Handle, false))
			before := actor.X
			var host *units.Unit
			var sprayed, reverse bool
			var first uint32
			for i := 0; i < 360 && host == nil; i++ {
				s.Step(s.Clock.ScaledAnchor + 1)
				f := s.Snapshot.Current()
				for _, v := range f.Strips {
					if v.Family == frame.StripFamilyNano && v.NanoInfected {
						if !sprayed {
							first = s.Clock.GlobalTick
						}
						sprayed = true
					}
				}
				for _, e := range f.Events {
					if e.Source == actor.Handle && e.Kind == frame.EventKindNanolathe && e.NanolatheBoxAtSource {
						reverse = true
					}
				}
				for _, u := range s.Units.IterSliced() {
					if u != nil && u.Alive && !u.Dying && u.Owner == actor.Owner && u.Def == victim.Def {
						host = u
						break
					}
				}
			}
			if host == nil {
				var head any = q.Head()
				t.Fatalf("native takeover never completed: actor alive=%v stance=%v pos=%d,%d first spray=%d head=%+v", actor.Alive, actor.InBuildStance, actor.X>>16, actor.Z>>16, first, head)
			}
			if actor.X == before || !sprayed || !reverse {
				t.Fatalf("missing approach/spray: moved=%v spray=%v reverse=%v", actor.X != before, sprayed, reverse)
			}
			if tc.moving && (host.X >= numeric.FixedFromInt(1440) || host.X <= numeric.FixedFromInt(1080) || s.Clock.GlobalTick-first != 60) {
				t.Fatalf("retreating target not caught in uninterrupted spray: captured x=%d first=%d end=%d", host.X>>16, first, s.Clock.GlobalTick)
			}
			if host.Handle == victim.Handle || host.Def.Weapon1Def != victim.Def.Weapon1Def || !orders.QueueForUnit(host).HasIssuedWork() {
				t.Fatal("replacement did not retain weapon and join director")
			}
			// Published identity uses attacker ownership immediately, the dynamic skin key.
			published := false
			for _, v := range s.Snapshot.Current().Units {
				if v.Slot == host.Handle && v.Owner == uint8(owner) {
					published = true
				}
			}
			if !published {
				t.Fatal("new infected host absent from committed frame")
			}
			t.Logf("approached and sprayed at tick %d; captured %s as new handle %d at tick %d (capture descriptor %d)", first, host.Def.UnitName, pool.Handle(host.Handle), s.Clock.GlobalTick, id)
		})
	}
}

// The shipped pack extends each mode's normal roster and the ordinary planner
// actually selects units outside the original three hand-painted prototypes.
func checkNativeInfectedRoster(t *testing.T, cat *content.Catalog) {
	t.Helper()
	if cat.SurvivalRoster == nil || !cat.SurvivalRoster.IncludeBuildTree {
		t.Fatal("infected pack does not include active build tree")
	}
	for _, rules := range []session.RuleSet{session.StrictRuleSet(), session.CommunityRuleSet(), session.ModernRuleSet()} {
		products := func(m *content.BuildMenuPage) []string { return construction.BuildProducts(rules.Construction, m) }
		baseline := survival.BuildPool(cat, products)
		combined, err := survival.BuildScenarioPool(cat, products)
		if err != nil {
			t.Fatal(err)
		}
		keys := make(map[string]bool)
		for _, u := range combined.Units {
			keys[u.Key] = true
		}
		for _, u := range baseline.Units {
			if !keys[u.Key] {
				t.Fatalf("%s dropped ordinary attacker %s", rules.Name, u.Key)
			}
		}
		for _, key := range []string{"iacrawl", "iaspitter", "iasiege"} {
			if !keys[key] {
				t.Fatalf("missing original unit %s", key)
			}
		}
		r := rng.NewSimulation(7)
		var domains [survival.DomainCount]bool
		newStockPick := false
		for n := 1; n <= 60; n++ {
			wave := survival.Plan(n, uint32(n*1800), &combined, survival.DefaultTuning(survival.PaceNormal), survival.Options{}, nil, &r)
			for _, group := range wave.Groups {
				for _, index := range group.Picks {
					u := combined.Units[index]
					domains[u.Domain] = true
					switch u.Key {
					case "iacrawl", "iaspitter", "iasiege", "armpw", "armstump", "corgol":
					default:
						newStockPick = true
					}
				}
			}
		}
		if !newStockPick || !domains[survival.Air] || !domains[survival.Naval] {
			t.Fatalf("%s wave variety absent: new=%v domains=%v", rules.Name, newStockPick, domains)
		}
		t.Logf("%s active combat roster=%d, plus aliens=%d; diverse land/air/naval plans pass", rules.Name, len(baseline.Units), len(combined.Units))
	}
}
