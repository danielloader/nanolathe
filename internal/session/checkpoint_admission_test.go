package session

import (
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/economy"
	"github.com/nanolathe-gg/nanolathe/internal/features"
	"github.com/nanolathe-gg/nanolathe/internal/mission"
	"github.com/nanolathe-gg/nanolathe/internal/movement"
	"github.com/nanolathe-gg/nanolathe/internal/path"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/internal/visibility"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

func TestCheckpointAdmissionAuthorityBelongsToExactSession(t *testing.T) {
	s := newLoopTestSession(t, 0)
	// This test stops at binding validation, before definition-key discovery.
	// As in the units authored fixtures, Hash supplies the Finalized marker.
	s.Catalog.Hash = "checkpoint-authority-fixture"
	a := checkpointAdmissionForTest(t, nil, EffectiveMatchConfig{}, s.Mission)
	s.checkpointAdmission = a
	if s.checkpointBindingAuthority() != nil {
		t.Fatal("unbound receipt supplied authority")
	}
	a.owner = s // composition assigns this before installing callbacks
	if s.checkpointBindingAuthority() != a.authority {
		t.Fatal("initial owner cannot install canonical callbacks")
	}
	a.ready = true
	s.Units.SetDeathExtraHook(nil) // this fixture tests the primary RegisterAll bindings
	s.RegisterAll()
	c := units.NewCheckpointContext(&content.CheckpointKeys{})
	if err := c.SetLifecycleBindings(s.Units, a.authority); err != nil {
		t.Fatal(err)
	}
	_, before := s.Units.CollectCheckpointReferences(c)
	if before == nil || strings.Contains(before.Error(), "units.World.On") {
		t.Fatalf("fixture did not pass lifecycle proof before its unrelated owner refusal: %v", before)
	}
	// A different Session may alias the same graph and receipt without owning it.
	copied := &Session{Units: s.Units, checkpointAdmission: a, LocalOwner: 9}
	if copied.checkpointBindingAuthority() != nil {
		t.Fatal("copied session acquired original authority")
	}
	copied.RegisterAll() // shared world receives closures capturing the copy
	_, after := s.Units.CollectCheckpointReferences(c)
	if after == nil || !strings.Contains(after.Error(), "units.World.OnDeath") {
		t.Fatalf("copied RegisterAll re-attested hooks against the original world: %v", after)
	}
	if a.owner != s || !a.ready {
		t.Fatal("copy rewrote admission receipt")
	}
	if (*Session)(nil).checkpointBindingAuthority() != nil || (&Session{}).checkpointBindingAuthority() != nil {
		t.Fatal("missing receipt supplied authority")
	}
	// A foreign nonzero token is never a substitute for exact receipt ownership.
	copied.checkpointAdmission = &sessionCheckpointAdmission{owner: s, authority: checkpoint.NewBindingAuthority()}
	if copied.checkpointBindingAuthority() != nil {
		t.Fatal("foreign owner supplied authority")
	}
}

// The production composition sites must carry the exact admission authority;
// ordinary local composition must remain unproved (§16.3.33, §16.3.40).
func TestCheckpointAdmissionPathInstallation(t *testing.T) {
	for _, admitted := range []bool{false, true} {
		s := newLoopTestSession(t, 0)
		a := checkpointAdmissionForTest(t, nil, EffectiveMatchConfig{}, s.Mission)
		if admitted {
			a.owner = s
			s.checkpointAdmission = a
		}
		// Recompose a fresh path owner in this authored fixture, not a copied one.
		s.Movement, s.Path = nil, nil
		if err := createAndBindServicesForTest(t, s); err != nil {
			t.Fatal(err)
		}
		c := movement.NewCheckpointContext(nil, path.NewCheckpointContext())
		err := c.SetPathBindings(s.Movement, s.Units, a.authority)
		if (err == nil) != admitted {
			t.Fatalf("admitted=%v path binding: %v", admitted, err)
		}
	}
}

func TestCheckpointAdmissionEconomyVisibilityInstallation(t *testing.T) {
	s := newLoopTestSession(t, 0)
	a := checkpointAdmissionForTest(t, nil, EffectiveMatchConfig{}, s.Mission)
	a.owner, s.checkpointAdmission = s, a
	// Rebind only the ordinary fixture readers that the real entry installs once.
	s.Vis.Community.SetAllied(nil)
	s.Vis.Community.SetOffMap(nil)
	if err := createAndBindServicesForTest(t, s); err != nil {
		t.Fatal(err)
	}
	s.projectCommunity()
	ec := economy.NewCheckpointContext(&world.CheckpointContext{Terrain: s.World})
	if err := ec.SetBindings(s.Econ, s.Wind, a.authority); err != nil {
		t.Fatal(err)
	}
	vc := visibility.NewCheckpointContext(&content.CheckpointKeys{})
	if err := vc.SetBindings(s.Vis, s.World, a.authority); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Econ.CollectCheckpointReferences(ec); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Vis.CollectCheckpointReferences(vc); err != nil {
		t.Fatal(err)
	}
	if s.Econ.EndConditionHook() != nil {
		t.Fatal("end-condition binding moved before the first player tick")
	}
	s.tickPlayers(1)
	if s.Econ.EndConditionHook() == nil {
		t.Fatal("player tick did not bind end conditions")
	}
	if _, err := s.Econ.CollectCheckpointReferences(ec); err != nil {
		t.Fatal(err)
	}
	// The existing only-if-absent guards preserve custom hooks, whose proof is
	// deliberately lost. Reprojection and later ticks must never bless them.
	s.Vis.Community.SetAllied(s.Vis.Community.AlliedReader())
	s.projectCommunity()
	if _, err := s.Vis.CollectCheckpointReferences(vc); err == nil || !strings.Contains(err.Error(), "Community.Allied") {
		t.Fatalf("reprojection attested ordinary reader: %v", err)
	}
	s.Econ.SetEndCondition(s.Econ.EndConditionHook())
	s.tickPlayers(2)
	if _, err := s.Econ.CollectCheckpointReferences(ec); err == nil || !strings.Contains(err.Error(), "EndCondition") {
		t.Fatalf("player tick attested ordinary end-condition callback: %v", err)
	}
}

func TestCheckpointAdmissionFeatureInstallation(t *testing.T) {
	s := newLoopTestSession(t, 0)
	a := checkpointAdmissionForTest(t, nil, EffectiveMatchConfig{}, s.Mission)
	a.owner, s.checkpointAdmission = s, a
	// Fresh entry installs each of these once. Recreate that state in the fixture.
	s.Features.SetBurnFrameGeometry(nil)
	s.Features.SetBurnSmoke(nil)
	s.Features.SetBurnSound(nil)
	s.Features.SetBurnWeapon(nil)
	s.Features.SetGeothermalSteam(nil)
	s.Features.SetSequenceFrames(nil)
	if err := createAndBindServicesForTest(t, s); err != nil {
		t.Fatal(err)
	}
	c := features.NewCheckpointContext(&world.CheckpointContext{Terrain: s.World, Keys: &content.CheckpointKeys{}})
	if err := c.SetBindings(s.Features, s.SimRNG(), s.CrtRNG(), s.Wind, a.authority); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Features.CollectCheckpointReferences(c); err != nil {
		t.Fatal(err)
	}
	// A later composition keeps an ordinary replacement, so it remains refused.
	s.Features.SetGeothermalSteam(s.Features.GeothermalSteamHook())
	if err := createAndBindServicesForTest(t, s); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Features.CollectCheckpointReferences(c); err == nil || !strings.Contains(err.Error(), "GeothermalSteam") {
		t.Fatalf("recomposition attested an ordinary feature callback: %v", err)
	}
}

func checkpointAdmissionForTest(t *testing.T, inputs *content.SimulationInputs, config EffectiveMatchConfig, m *mission.Mission) *sessionCheckpointAdmission {
	t.Helper()
	a, err := newSessionCheckpointAdmission(inputs, config, m)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestCheckpointAdmissionRetainsMissionInputs(t *testing.T) {
	s := newLoopTestSession(t, 0)
	a := checkpointAdmissionForTest(t, nil, EffectiveMatchConfig{}, s.Mission)
	a.owner, a.terrain, s.checkpointAdmission = s, s.World, a
	if err := a.missionInputs.Validate(s.Mission); err != nil {
		t.Fatal(err)
	}
	s.Mission.TerrainKey += "changed"
	if err := a.missionInputs.Validate(s.Mission); err == nil {
		t.Fatal("accepted changed mission before readiness")
	}
	s.Mission.TerrainKey = a.terrainKey
	if err := a.missionInputs.Validate(s.Mission); err != nil {
		t.Fatal(err)
	}
	a.ready = true
	s.Mission.Schema.StartPositions++
	if err := a.missionInputs.Validate(s.Mission); err == nil {
		t.Fatal("accepted changed mission after readiness")
	}
}
