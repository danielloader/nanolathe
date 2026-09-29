package session

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/mission"
)

func TestSpawnCommandModernOwnershipResourcesAndRNG(t *testing.T) {
	s := strictNewSessionWithUnits(t, 0, 7, 11)
	s.LocalOwner, s.ViewingOwner = 1, 0
	s.Mission.Type = mission.TypeCampaign
	s.Catalog.Units["armcom"].BuildAngle = 4096
	stock := s.Econ.Players[1].Stock
	sim, crt := s.SimRNG().Draws(), s.CrtRNG().Draws()
	c := HumanCommand{Kind: HumanSpawn, Spawn: HumanSpawnCommand{Unit: "ARmCOM", X: 160 << 16, Y: 10 << 16, Z: 160 << 16}}
	if err := s.EnqueueHumanCommand(c); err != nil {
		t.Fatal(err)
	}
	if len(s.Units.Iter()) != 0 {
		t.Fatal("enqueue allocated a unit")
	}
	s.applyHumanCommands(1)
	us := s.Units.Iter()
	if len(us) != 1 {
		t.Fatalf("spawned units = %d", len(us))
	}
	u := us[0]
	if u.Owner != 1 || u.X != c.Spawn.X || u.Y != c.Spawn.Y || u.Z != c.Spawn.Z || u.Health != u.MaxHealth || u.Remaining != 0 || u.Script == nil {
		t.Fatalf("spawn state: %+v", u)
	}
	if s.Econ.Players[1].Stock != stock {
		t.Fatal("spawn changed stock")
	}
	// The normal allocator draws build angle then hover phase [04 R-P28-ANG-01R §2].
	if s.SimRNG().Draws()-sim != 2 || s.CrtRNG().Draws() != crt {
		t.Fatal("spawn changed ordinary creation RNG effects")
	}
	// Retail's spawn handler validates no site, so a second request at the
	// occupied point stacks a second unit with the ordinary creation draws
	// [07 R-CAM-01 §6].
	sim = s.SimRNG().Draws()
	s.applyHumanCommand(c, 1)
	if us := s.Units.Iter(); len(us) != 2 || us[1].X != c.Spawn.X || us[1].Z != c.Spawn.Z || s.SimRNG().Draws()-sim != 2 {
		t.Fatalf("occupied site: units=%d draws=%d, want a stacked second unit and 2 draws", len(s.Units.Iter()), s.SimRNG().Draws()-sim)
	}
}

func TestSpawnCommandStrictAndInvalidRequestsDoNotMutate(t *testing.T) {
	for _, kind := range []string{"strict", "unknown", "outside", "player", "limit"} {
		t.Run(kind, func(t *testing.T) {
			s := strictNewSessionWithUnits(t, 0, 7, 11)
			c := HumanCommand{Kind: HumanSpawn, Spawn: HumanSpawnCommand{Unit: "armcom", X: 160 << 16, Y: 10 << 16, Z: 160 << 16}}
			switch kind {
			case "strict":
				s.Gameplay = gameplay.Strict31
			case "unknown":
				c.Spawn.Unit = "missing"
			case "outside":
				c.Spawn.X = -100 << 16
			case "player":
				s.Econ.Players[s.LocalOwner].Exists = false
			case "limit":
				s.Catalog.Units["armcom"].LimitEnabled = true
				s.Catalog.Units["armcom"].Limit = 0
			}
			stock := s.Econ.Players[s.LocalOwner].Stock
			sim, crt := s.SimRNG().Draws(), s.CrtRNG().Draws()
			s.applyHumanCommand(c, 1)
			if len(s.Units.Iter()) != 0 || s.SimRNG().Draws() != sim || s.CrtRNG().Draws() != crt || s.Econ.Players[s.LocalOwner].Stock != stock {
				t.Fatal("rejected spawn changed units, RNG or stock")
			}
		})
	}
	s := strictNewSessionWithUnits(t, 0, 7, 11)
	_ = s.EnqueueHumanCommand(HumanCommand{Kind: HumanGameplay, Gameplay: gameplay.Strict31})
	_ = s.EnqueueHumanCommand(HumanCommand{Kind: HumanSpawn, Spawn: HumanSpawnCommand{Unit: "armcom", X: 160 << 16, Z: 160 << 16}})
	sim, crt := s.SimRNG().Draws(), s.CrtRNG().Draws()
	s.applyHumanCommands(1)
	if len(s.Units.Iter()) != 0 || s.SimRNG().Draws() != sim || s.CrtRNG().Draws() != crt {
		t.Fatal("queued spawn bypassed mode change")
	}
}

// A structure takes the mission spawner's snap and height, and a second one
// on the same site is created on top of the first: the retail spawn handler
// runs no placement validator, not even against depth or an existing yard
// [07 R-CAM-01 §6][08 R-ENTRY-01 §6].
func TestSpawnCommandStructureSnapsAndStacks(t *testing.T) {
	s := wu19205SpawnerSession(t)
	c := HumanCommand{Kind: HumanSpawn, Spawn: HumanSpawnCommand{Unit: "teststruct", X: 165 << 16, Y: 90 << 16, Z: 165 << 16}}
	for range 2 {
		s.applyHumanCommand(c, 1)
	}
	us := s.Units.Iter()
	if len(us) != 2 {
		t.Fatalf("spawned units = %d, want 2 stacked", len(us))
	}
	for _, u := range us {
		if u.X != 160<<16 || u.Z != 160<<16 || u.Y != 10<<16 {
			t.Fatalf("structure point = %d,%d,%d", u.X, u.Y, u.Z)
		}
	}
}
