package session

import (
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
)

func TestDeveloperUnitPatternWholeNameAndByteWildcards(t *testing.T) {
	for _, tc := range []struct {
		pattern, name string
		want          bool
	}{
		{"arm*", "ARMCOM", true}, {"arm*", "arm", true},
		{"arm*", "corarm", false}, {"arm", "armcom", false},
		{"arm?", "ARMX", true}, {"arm?", "arm", false},
		{"*a?b", "xayaxacb", true}, {"*a?b", "xab", false},
		{"a**b*", "Ab", true}, {"*", "", true}, {"?", "", false},
		{"", "", true}, {"[ab]", "a", false}, {"[ab]", "[ab]", true},
		{"?", "\xc0", true},
	} {
		if got := developerUnitPattern(tc.pattern, tc.name); got != tc.want {
			t.Errorf("pattern %q name %q = %v, want %v", tc.pattern, tc.name, got, tc.want)
		}
	}
}

// The first failed match still advances; the second's footprint snapping is
// retained by the running point, and the post-step equality wraps before the
// third's half-width is added [07 R-CAM-01 §6][08 R-ENTRY-01 §6]. Duplicate
// names are separate definitions, rather than one name-index lookup.
func TestDeveloperSpawnDefinitionOrderFailedMatchAndRowBoundary(t *testing.T) {
	for _, mode := range []gameplay.Mode{gameplay.Modern, gameplay.Strict31} {
		for _, edge := range []int32{272, 273} {
			s := wu19205SpawnerSession(t)
			s.Gameplay = mode
			mobile := *s.Catalog.Units["armcom"]
			first, third := mobile, mobile
			second := *s.Catalog.Units["teststruct"]
			first.UnitName, second.UnitName, third.UnitName = "ARMone", "ARMdup", "ARMdup"
			first.FootprintX, second.FootprintX, third.FootprintX = 1, 2, 3
			first.LimitEnabled, first.Limit = true, 0
			first.BuildAngle, second.BuildAngle, third.BuildAngle = 4096, 4096, 4096
			first.UnitDefID, second.UnitDefID, third.UnitDefID = 1, 2, 3
			// Map-only catalogs use sorted fixture keys as their record order.
			s.Catalog.Units = map[string]*content.UnitDef{"01": &first, "02": &second, "03": &third}
			s.World.PlayRight = edge
			s.LocalOwner, s.ViewingOwner = 0, 0
			// Retail does not test the owner's player state before allocation.
			s.Econ.Players[1].Exists = false
			stock := s.Econ.Players[1].Stock
			sim, crt := s.SimRNG().Draws(), s.CrtRNG().Draws()
			c := HumanCommand{Kind: HumanDeveloperSpawn, DeveloperSpawn: HumanDeveloperSpawnCommand{Pattern: "arm*", Owner: 1, X: 165 << 16, Y: 90 << 16, Z: 165 << 16}}
			if err := s.EnqueueHumanCommand(c); err != nil {
				t.Fatal(err)
			}
			_ = s.EnqueueHumanCommand(HumanCommand{Kind: HumanNoShake})
			if got := s.applyPausedHumanCommands(1, nil); got != 0 || len(s.Units.Iter()) != 0 || len(s.PendingHumanCommands()) != 2 {
				t.Fatal("paused spawn did not hold the queue prefix")
			}
			s.applyHumanCommands(1)
			us := s.Units.Iter()
			if len(us) != 2 || us[0].Def != &second || us[1].Def != &third {
				t.Fatalf("definition order: %+v", us)
			}
			wantX, wantZ := numeric.Fixed(184<<16), numeric.Fixed(320<<16)
			if edge == 273 {
				wantX, wantZ = 296<<16, 160<<16
			}
			if us[0].X != 224<<16 || us[0].Y != 10<<16 || us[0].Z != 160<<16 || us[1].X != wantX || us[1].Y != 10<<16 || us[1].Z != wantZ {
				t.Fatalf("edge %d positions = (%d,%d,%d), (%d,%d,%d)", edge, us[0].X>>16, us[0].Y>>16, us[0].Z>>16, us[1].X>>16, us[1].Y>>16, us[1].Z>>16)
			}
			for _, u := range us {
				if u.Owner != 1 || u.Remaining != 0 || u.Health != u.MaxHealth || u.Script == nil {
					t.Fatalf("not fully built for requested owner: %+v", u)
				}
			}
			if s.Econ.Players[1].Stock != stock || s.SimRNG().Draws()-sim != 4 || s.CrtRNG().Draws() != crt {
				t.Fatal("spawn changed resources or ordinary success-only creation RNG")
			}
			s.applyHumanCommand(c, 1)
			us = s.Units.Iter()
			if len(us) != 3 || us[2].X != us[0].X || us[2].Z != us[0].Z {
				t.Fatal("occupied building was refused or the next submission retained the previous running point")
			}
		}
	}
}

func TestDeveloperSpawnFirstMatchAndSilentRefusals(t *testing.T) {
	s := strictNewSessionWithUnits(t, 0, 7, 11)
	s.Gameplay = gameplay.Strict31
	s.Catalog.Units["armcom"].BuildAngle = 4096
	s.World.PlayRight = 1024
	s.LocalOwner, s.ViewingOwner = 1, 1
	c := HumanCommand{Kind: HumanDeveloperSpawn, DeveloperSpawn: HumanDeveloperSpawnCommand{Pattern: "ARmCOM", X: 165 << 16, Y: 10 << 16, Z: 165 << 16}}
	sim, crt, stock := *s.SimRNG(), *s.CrtRNG(), s.Econ.Players[0].Stock
	for _, bad := range []HumanDeveloperSpawnCommand{{Pattern: "missing"}, {Pattern: "arm*", Owner: 255}} {
		s.applyHumanCommand(HumanCommand{Kind: HumanDeveloperSpawn, DeveloperSpawn: bad}, 1)
		if len(s.Units.Iter()) != 0 || *s.SimRNG() != sim || *s.CrtRNG() != crt || s.Econ.Players[0].Stock != stock {
			t.Fatal("silent refusal mutated the battle")
		}
	}
	s.applyHumanCommand(c, 1)
	us := s.Units.Iter()
	if len(us) != 1 || us[0].Owner != 0 || us[0].X != c.DeveloperSpawn.X || us[0].Z != c.DeveloperSpawn.Z {
		t.Fatal("first match advanced or used the viewing/controlling owner")
	}
}

func TestDeveloperSpawnReplayAndOnlineIsolation(t *testing.T) {
	for _, online := range []bool{false, true} {
		f := newSeatFixture(t, online, true)
		f.s.Gameplay = gameplay.Strict31
		f.s.LocalOwner = 1
		f.s.World.PlayRight = 1024
		c := SeatCommand{Kind: SeatDeveloperSpawn, DeveloperSpawn: DeveloperSpawnPayload{Pattern: "sco?*", Owner: 0, Position: CommandPoint{X: 300 << 16, Z: 300 << 16}}}
		sim, crt, stock, count := *f.s.SimRNG(), *f.s.CrtRNG(), f.s.Econ.Players[0].Stock, len(f.s.Units.Iter())
		receipt := f.issue(t, 1, c)
		if online {
			expectOutcome(t, receipt, CommandRejected)
			if len(f.s.Units.Iter()) != count || *f.s.SimRNG() != sim || *f.s.CrtRNG() != crt || f.s.Econ.Players[0].Stock != stock {
				t.Fatal("online developer request mutated state")
			}
		} else {
			expectOutcome(t, receipt, CommandApplied)
			us := f.s.Units.Iter()
			if len(us) != count+1 {
				t.Fatal("replay did not create one matched definition")
			}
			var found bool
			for _, u := range us {
				if u.X == c.DeveloperSpawn.Position.X && u.Z == c.DeveloperSpawn.Position.Z && u.Owner == 0 {
					found = true
				}
			}
			if !found || f.s.Econ.Players[0].Stock != stock {
				t.Fatal("replay changed ownership or resources")
			}
		}
	}
}

// The reported +arm* case reaches every retained stock definition, subject to
// the ordinary owner/definition limits, without changing stocks. Count the
// accepted definitions from content rather than locking an install census.
func TestDeveloperSpawnRetailArmDefinitions(t *testing.T) {
	s := loadRetailFixture(t).session(t)
	s.SetGameplay(gameplay.Strict31)
	var prior uint64
	before := s.Units.Iter()
	for _, u := range before {
		if u.AllocationSerial > prior {
			prior = u.AllocationSerial
		}
	}
	var want []*content.UnitDef
	for _, def := range s.Catalog.UnitRecords() {
		if !strings.HasPrefix(strings.ToUpper(def.UnitName), "ARM") {
			continue
		}
		count := int32(0)
		for _, u := range before {
			if u.Owner == 0 && u.Def == def {
				count++
			}
		}
		if def.LimitEnabled && def.Limit >= 0 && count >= def.Limit {
			continue
		}
		want = append(want, def)
	}
	if len(want) == 0 {
		t.Fatal("fixture has no eligible Arm definitions")
	}
	stock := s.Econ.Players[0].Stock
	s.applyHumanCommand(HumanCommand{Kind: HumanDeveloperSpawn, DeveloperSpawn: HumanDeveloperSpawnCommand{Pattern: "aRm*", X: 160 << 16, Z: 160 << 16}}, 1)
	var created []*content.UnitDef
	for _, u := range s.Units.Iter() {
		if u.AllocationSerial > prior {
			if u.Owner != 0 || u.Remaining != 0 || u.Health != u.MaxHealth {
				t.Fatalf("spawned definition %s is not fully built for slot 0", u.Def.UnitName)
			}
			created = append(created, u.Def)
		}
	}
	if len(created) != len(want) {
		t.Fatalf("spawned %d definitions, want %d eligible definitions", len(created), len(want))
	}
	for i, def := range want {
		if created[i] != def {
			t.Fatalf("definition order at %d: %s, want %s", i, created[i].UnitName, def.UnitName)
		}
	}
	if s.Econ.Players[0].Stock != stock {
		t.Fatal("wildcard spawn changed resource stocks")
	}
}
