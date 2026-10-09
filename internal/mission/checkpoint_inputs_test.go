package mission

import (
	"slices"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/triggers"
)

func checkpointMissionFixture(t *testing.T) *Mission {
	t.Helper()
	o, err := formats.LoadOTA([]byte(`[GlobalHeader]{missionname=Authored;[Schema 0]{type=Network 1;}}`))
	if err != nil {
		t.Fatal(err)
	}
	return &Mission{
		Type: TypeSkirmish, OTA: o, TerrainKey: "map", Schema: Schema{"Schema 0", 2},
		Units: []UnitPlacement{{UnitName: "unit", Ident: "alpha", InitialMission: "move", X: -2147483648, Z: 2147483647, Y: -1, Angle: 65535,
			Player: 2, HealthPercentage: 103, BuildPriority: -7, CreationCountdown: 900,
			InitialGroup: "squad", Kills: 17, RawFlags: 0x8f}, {UnitName: "second"}},
		Specials:   []Special{{Kind: 1, ID: 3, X: -32768, Z: 32767, Name: "StartPos4"}, {Kind: 2}},
		Features:   []FeaturePlacement{{Name: "rock", X: -1, Z: 7, RawX: -12, RawZ: 9}, {Name: "tree"}},
		WindBounds: WindBounds{-5, 13}, UseOnlyPath: "camps/useonly/test.tdf",
		CampaignPath: "campaign", CampaignIndex: -1, CampaignMissionName: "mission", Difficulty: -1,
		Victory: []*triggers.Trigger{{Kind: triggers.KindVictoryTimerRunsOut, Args: [3]int32{100}}},
		Defeat:  []*triggers.Trigger{{Kind: triggers.KindCommanderKilled}}, order: []string{"schema", "placement"},
	}
}

// Every stored authored field is retained even when its current reader is
// inert. Mutation before the first validation is as observable as mutation
// after it (DESIGN_MULTIPLAYER §16.3.74).
func TestCheckpointInputsRetainedMutations(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Mission)
	}{
		{"Type", func(m *Mission) { m.Type++ }},
		{"TerrainKey", func(m *Mission) { m.TerrainKey += "x" }},
		{"Schema.Name", func(m *Mission) { m.Schema.Name += "x" }},
		{"Schema.StartPositions", func(m *Mission) { m.Schema.StartPositions++ }},
		{"WindBounds.Min", func(m *Mission) { m.WindBounds.Min++ }},
		{"WindBounds.Max", func(m *Mission) { m.WindBounds.Max++ }},
		{"UseOnlyPath", func(m *Mission) { m.UseOnlyPath += "x" }},
		{"IsRestore", func(m *Mission) { m.IsRestore = true }},
		{"CampaignPath", func(m *Mission) { m.CampaignPath += "x" }},
		{"CampaignIndex", func(m *Mission) { m.CampaignIndex++ }},
		{"CampaignMissionName", func(m *Mission) { m.CampaignMissionName += "x" }},
		{"Difficulty", func(m *Mission) { m.Difficulty++ }},
		{"UnitName", func(m *Mission) { m.Units[0].UnitName += "x" }},
		{"Ident", func(m *Mission) { m.Units[0].Ident += "x" }},
		{"InitialMission", func(m *Mission) { m.Units[0].InitialMission += "x" }},
		{"X", func(m *Mission) { m.Units[0].X++ }},
		{"Z", func(m *Mission) { m.Units[0].Z-- }},
		{"Y", func(m *Mission) { m.Units[0].Y++ }},
		{"Angle", func(m *Mission) { m.Units[0].Angle-- }},
		{"Player", func(m *Mission) { m.Units[0].Player++ }},
		{"HealthPercentage", func(m *Mission) { m.Units[0].HealthPercentage++ }},
		{"BuildPriority", func(m *Mission) { m.Units[0].BuildPriority++ }},
		{"CreationCountdown", func(m *Mission) { m.Units[0].CreationCountdown++ }},
		{"MissionCriticalUnit", func(m *Mission) { m.Units[0].MissionCriticalUnit = true }},
		{"AiIgnore", func(m *Mission) { m.Units[0].AiIgnore = true }},
		{"AiPriorityTarget", func(m *Mission) { m.Units[0].AiPriorityTarget = true }},
		{"Immune", func(m *Mission) { m.Units[0].Immune = true }},
		{"InitialGroup", func(m *Mission) { m.Units[0].InitialGroup += "x" }},
		{"Kills", func(m *Mission) { m.Units[0].Kills++ }},
		{"RawFlags", func(m *Mission) { m.Units[0].RawFlags ^= 1 }},
		{"Special.Kind", func(m *Mission) { m.Specials[0].Kind++ }},
		{"Special.ID", func(m *Mission) { m.Specials[0].ID++ }},
		{"Special.X", func(m *Mission) { m.Specials[0].X++ }},
		{"Special.Z", func(m *Mission) { m.Specials[0].Z-- }},
		{"Special.Name", func(m *Mission) { m.Specials[0].Name += "x" }},
		{"Feature.Name", func(m *Mission) { m.Features[0].Name += "x" }},
		{"Feature.X", func(m *Mission) { m.Features[0].X++ }},
		{"Feature.Z", func(m *Mission) { m.Features[0].Z++ }},
		{"Feature.RawX", func(m *Mission) { m.Features[0].RawX++ }},
		{"Feature.RawZ", func(m *Mission) { m.Features[0].RawZ++ }},
		{"Units.order", func(m *Mission) { m.Units[0], m.Units[1] = m.Units[1], m.Units[0] }},
		{"Specials.order", func(m *Mission) { m.Specials[0], m.Specials[1] = m.Specials[1], m.Specials[0] }},
		{"Features.order", func(m *Mission) { m.Features[0], m.Features[1] = m.Features[1], m.Features[0] }},
		{"Units.length", func(m *Mission) { m.Units = m.Units[:1] }},
		{"Specials.length", func(m *Mission) { m.Specials = append(m.Specials, Special{}) }},
		{"Features.length", func(m *Mission) { m.Features = nil }},
		{"OTA.values", func(m *Mission) { m.OTA.Global.Items[0].Value += "x" }},
		{"OTA.copy", func(m *Mission) { copy := *m.OTA; m.OTA = &copy }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, validateFirst := range []bool{false, true} {
				m := checkpointMissionFixture(t)
				s, err := SnapshotCheckpointInputs(m)
				if err != nil {
					t.Fatal(err)
				}
				if validateFirst {
					if err := s.Validate(m); err != nil {
						t.Fatal(err)
					}
				}
				tc.mutate(m)
				if err := s.Validate(m); err == nil {
					t.Fatal("accepted altered authored input")
				}
			}
		})
	}
}

func TestCheckpointInputsSlicePresence(t *testing.T) {
	for _, mutate := range []struct {
		name string
		set  func(*Mission)
	}{
		{"Units", func(m *Mission) { m.Units = []UnitPlacement{} }},
		{"Specials", func(m *Mission) { m.Specials = []Special{} }},
		{"Features", func(m *Mission) { m.Features = []FeaturePlacement{} }},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			m := &Mission{}
			s, err := SnapshotCheckpointInputs(m)
			if err != nil {
				t.Fatal(err)
			}
			mutate.set(m)
			if err := s.Validate(m); err == nil {
				t.Fatal("nil became empty")
			}
			s, err = SnapshotCheckpointInputs(m)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.Validate(m); err != nil {
				t.Fatal(err)
			}
			m.Units, m.Specials, m.Features = nil, nil, nil
			if err := s.Validate(m); err == nil {
				t.Fatal("empty became nil")
			}
		})
	}
}

func TestCheckpointInputsIdentityAndNil(t *testing.T) {
	if _, err := SnapshotCheckpointInputs(nil); err == nil {
		t.Fatal("snapshotted nil mission")
	}
	m := checkpointMissionFixture(t)
	s, err := SnapshotCheckpointInputs(m)
	if err != nil {
		t.Fatal(err)
	}
	copy := *m
	for _, check := range []func() error{
		func() error { return s.Validate(&copy) },
		func() error { return s.Validate(nil) },
		func() error { return (*CheckpointInputs)(nil).Validate(m) },
		func() error { return new(CheckpointInputs).Validate(m) },
	} {
		if err := check(); err == nil || !strings.HasPrefix(err.Error(), "nanolathe: mission checkpoint input validation failed: logical path ") {
			t.Fatalf("missing owner-shaped refusal: %v", err)
		}
	}
}

func TestCheckpointInputsExclusionsPurityAndStorage(t *testing.T) {
	m := checkpointMissionFixture(t)
	s, err := SnapshotCheckpointInputs(m)
	if err != nil {
		t.Fatal(err)
	}
	// All trigger fields belong to the mutable scenario payload; even queue
	// replacement/default insertion is outside this immutable snapshot.
	*m.Victory[0] = triggers.Trigger{Kind: triggers.KindMoveUnitToRadius, Type: "other", Args: [3]int32{1, 2, 3}, Completed: true, Celebrated: true, CenterReady: true, CenterX: 4, CenterY: 5, CenterZ: 6}
	m.Defeat = append(m.Defeat, &triggers.Trigger{Completed: true})
	m.order[0] = "observation"
	if err := s.Validate(m); err != nil {
		t.Fatal(err)
	}
	m.Victory, m.Defeat, m.order = nil, nil, nil
	// Equal replacement backing arrays/capacities are immaterial.
	m.Units = append(make([]UnitPlacement, 0, 15), m.Units...)
	m.Specials = slices.Clone(m.Specials)
	m.Features = slices.Clone(m.Features)
	if err := s.Validate(m); err != nil {
		t.Fatal(err)
	}
	if allocs := testing.AllocsPerRun(100, func() {
		if err := s.Validate(m); err != nil {
			panic(err)
		}
	}); allocs != 0 {
		t.Fatalf("validation allocations = %v", allocs)
	}
	if m.Victory != nil || m.Defeat != nil || m.order != nil {
		t.Fatal("validation changed excluded state")
	}
	// Restoring a changed value validates again: a failed comparison does not
	// update the admission snapshot or latch a separate failure state.
	m.Units[0].CreationCountdown++
	if err := s.Validate(m); err == nil {
		t.Fatal("accepted detached slice mutation")
	}
	m.Units[0].CreationCountdown--
	if err := s.Validate(m); err != nil {
		t.Fatal(err)
	}
}
