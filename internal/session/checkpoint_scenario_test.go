package session

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/ai"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/mission"
	"github.com/nanolathe-gg/nanolathe/internal/movement"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/survival"
	"github.com/nanolathe-gg/nanolathe/internal/triggers"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

func scenarioCheckpointBytes(t *testing.T, s *Session, c *checkpointScenarioContext) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := s.writeScenarioCheckpoint(checkpoint.NewEncoder(&b), c); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// These test-only byte appenders encode literal authored expectations, never
// production records or encoder calls. Each vector below specifies its own
// field order and widths (DESIGN_MULTIPLAYER §16.3.38).
type scenarioVector []byte

func (b *scenarioVector) u8(v ...uint8) { *b = append(*b, v...) }
func (b *scenarioVector) u16(v ...uint16) {
	for _, x := range v {
		*b = binary.LittleEndian.AppendUint16(*b, x)
	}
}
func (b *scenarioVector) i32(v ...int32) {
	for _, x := range v {
		*b = binary.LittleEndian.AppendUint32(*b, uint32(x))
	}
}
func (b *scenarioVector) u32(v ...uint32) {
	for _, x := range v {
		*b = binary.LittleEndian.AppendUint32(*b, x)
	}
}
func (b *scenarioVector) i64(v ...int64) {
	for _, x := range v {
		*b = binary.LittleEndian.AppendUint64(*b, uint64(x))
	}
}
func (b *scenarioVector) text(v string) { b.u32(uint32(len(v))); *b = append(*b, v...) }
func (b *scenarioVector) zero(n int)    { *b = append(*b, make([]byte, n)...) }

func scenarioCheckpointFixture() (*Session, *checkpointScenarioContext) {
	m := &mission.Mission{Type: mission.TypeSkirmish,
		Defeat:  []*triggers.Trigger{nil, {Kind: triggers.KindUnitTypeKilled, Type: "target", Args: [3]int32{-1, 2, -3}, Completed: true, CenterReady: true, CenterX: 4, CenterY: -5, CenterZ: 6}},
		Victory: []*triggers.Trigger{{Kind: triggers.KindDestroyAllUnits, Celebrated: true}},
	}
	c1 := &survivalClass{base: -2, key: "z", profile: movement.Profile{BadSlope: 1, BadWaterSlope: 2, FootPrintX: -3, FootPrintZ: 4, MaxSlope: 5, MaxWaterDepth: -6, MaxWaterSlope: 7, MinWaterDepth: 8}, regions: survival.Regions{H: -3, W: 4}}
	c2 := &survivalClass{base: 12, key: "a"}
	st := &survivalState{attacker: 9, centreX: -7, centreZ: 8, classes: []*survivalClass{c1, c2}, cleanLost: true, info: &ai.SurvivalInfo{}, lastSpawn: 0x01020304,
		nextG: -(1 << 40) + 7, nextP: 1<<41 + 9, nextRetarget: 0xfefdfcfb, opts: survival.Options{NoAir: true}, phase: 3, phaseEnd: 0xa1a2a3a4,
		plan:    survival.Wave{Budget: -13, Groups: []survival.Group{{Angle: 0xabcd, Domain: survival.Air, Picks: []int{2, -3}}, {Angle: 7, Domain: survival.Hover, Picks: []int{1<<40 + 4}}}, Number: -(1 << 39) + 17},
		pool:    survival.Pool{MaxTier: 1<<40 + 26, RatioE: -27, RatioM: 28, Tier1Median: 29, Units: []survival.Unit{{Cost: -30, Domain: survival.Air, Key: "A", Tier: -(1 << 35) + 31}, {Cost: 32, Domain: survival.Amphibious, Key: "a", Tier: 33}}},
		removed: map[pool.Handle]int32{65535: -8, 1: 9, 0: 10}, startClass: c2, startRegion: -16, survived: 1<<40 + 23, team: []uint8{2, 0, 1},
		tuning: survival.Tuning{AirFrom: 1, BaseUnits: 2, BuddyRing: 3, CleanWaveBonus: 4, DirectionEvery: 5, Doubling: 6, DowntimeBase: 7, DowntimeMax: 8, DowntimePerUnit: 9, EdgeInset: 10, FastClearBonus: 11, FirstWaveDelay: 12, MaxDirections: 13, NewTierWeight: 14, RetargetEvery: 15, SpawnPerTick: 16, Straggle: 17, ThemeWeights: [5]int{18, 19, 20, 21, 22}, UnlockUnits: 23, WarningTime: 24, WaveReward: math.Float32frombits(0x80000000)},
		units:  []survivalUnit{{h: 65535, infecting: true, shun: 8, shunCellX: -9, shunCellZ: 10, shunSerial: 1<<50 + 11, shunUntil: 12, target: 13, wave: -(1 << 40) + 14}, {}}, wave: -24, wavePoints: 25, waveUnits: []pool.Handle{9, 0, 65535, 9},
	}
	st.accts[0] = survival.Account{Capacity: math.Float32frombits(0x80000000), Earned: math.Float32frombits(0x7f800000), Stock: -1}
	st.accts[9] = survival.Account{Capacity: 2, Earned: 3, Stock: 4}
	st.settled[0], st.settled[9] = 0xaabbccdd, 19
	st.stats[0] = SurvivalStats{Damage: -17, Destroyed: 18, Lost: -19}
	st.stats[9] = SurvivalStats{Damage: 20, Destroyed: 21, Lost: 22}
	s := &Session{Mission: m, Survival: st, communitySchema: communitySchemaState{active: true, mission: m, deferredPlacements: []int{7, -1, 1<<40 + 9}, neutralOwner: -1, nextDeferred: 1<<39 + 3, playerByStart: [10]int8{-1, 1, 2, 3, 4, 5, 6, 7, 8, 9}}}
	return s, &checkpointScenarioContext{mission: m}
}

func TestCheckpointScenarioAuthoredVector(t *testing.T) {
	s, c := scenarioCheckpointFixture()
	var v scenarioVector
	// Mission presence; two defeat rows (one nil); one victory row.
	v.u8(1)
	v.u32(2)
	v.u8(0, 1)
	v.i32(-1, 2, -3)
	v.u8(0, 1)
	v.i32(4, -5, 6)
	v.u8(1, 14)
	v.text("target")
	v.u32(1)
	v.u8(1)
	v.zero(12)
	v.u8(1, 0)
	v.zero(12)
	v.u8(0, 1)
	v.text("")
	// Community state, including i64 physical deferred indices and cursor.
	v.u8(1)
	v.u32(3)
	v.i64(7, -1, 1<<40+9)
	v.u8(1, 255)
	v.i64(1<<39 + 3)
	v.u8(255, 1, 2, 3, 4, 5, 6, 7, 8, 9)
	v.u8(1) // Survival presence
	v.u32(0x80000000, 0x7f800000, 0xbf800000)
	v.zero(8 * 12)
	v.u32(0x40000000, 0x40400000, 0x40800000)
	v.u8(9)
	v.i32(-7, 8)
	v.u32(2)
	// Classes in allocation order, despite reversed lexical keys. Profiles
	// are 16 bytes; Regions preserve negative dimensions and nil labels.
	v.i32(-2)
	v.text("z")
	v.u8(1, 2)
	v.u16(65533, 4)
	v.u8(5)
	v.i32(-6)
	v.u8(7)
	v.i32(8)
	v.i32(-3, 4)
	v.u8(0)
	v.u32(0)
	v.i32(12)
	v.text("a")
	v.zero(16)
	v.zero(13)
	v.u8(1, 1)
	v.zero(22) // cleanLost, info presence and zero info payload
	v.u32(0x01020304)
	v.i64(-(1<<40)+7, 1<<41+9)
	v.u32(0xfefdfcfb)
	v.u8(1, 0, 3)
	v.u32(0xa1a2a3a4)
	// Plan, exact group/pick order and wide Go ints.
	v.i64(-13)
	v.u32(2)
	v.u16(0xabcd)
	v.u8(4)
	v.u32(2)
	v.i64(2, -3)
	v.u16(7)
	v.u8(2)
	v.u32(1)
	v.i64(1<<40 + 4)
	v.i64(-(1 << 39) + 17)
	v.i64(1<<40+26, -27, 28, 29)
	v.u32(2)
	v.i64(-30)
	v.u8(0, 4)
	v.text("A")
	v.i64(-(1 << 35) + 31)
	v.i64(32)
	v.u8(0, 1)
	v.text("a")
	v.i64(33)
	// Removed values sorted by raw handle, including zero and stale slots.
	v.u32(3, 0)
	v.i32(10)
	v.u32(1)
	v.i32(9)
	v.u32(65535)
	v.i32(-8)
	v.u32(0xaabbccdd)
	v.zero(8 * 4)
	v.u32(19, 2)
	v.i32(-16)
	v.i64(-17, 18, -19)
	v.zero(8 * 24)
	v.i64(20, 21, 22)
	v.i64(1<<40 + 23)
	v.u32(3)
	v.u8(2, 0, 1)
	// All effective tuning words, no DefaultTuning call in the expectation.
	v.u32(1)
	v.i64(2)
	v.i32(3)
	v.i64(4)
	v.u32(5, 6, 7, 8, 9)
	v.i32(10)
	v.i64(11)
	v.u32(12)
	v.i64(13, 14)
	v.u32(15)
	v.i64(16)
	v.u32(17)
	v.i64(18, 19, 20, 21, 22, 23)
	v.u32(24, 0x80000000)
	v.u32(2, 65535)
	v.u8(1)
	v.u32(8)
	v.i32(-9, 10)
	v.i64(1<<50 + 11)
	v.u32(12, 13)
	v.i64(-(1 << 40) + 14)
	v.zero(41)
	v.i64(-24, 25)
	v.u32(4, 9, 0, 65535, 9)
	if got := scenarioCheckpointBytes(t, s, c); !bytes.Equal(got, v) {
		first := 0
		for first < len(got) && first < len(v) && got[first] == v[first] {
			first++
		}
		t.Fatalf("payload differs at %d, lengths %d/%d\ngot %x\nwant %x", first, len(got), len(v), got, []byte(v))
	}
}

func TestCheckpointScenarioRefusals(t *testing.T) {
	for _, tc := range []struct {
		name, path string
		edit       func(*Session, *checkpointScenarioContext)
	}{
		{"mission identity", "scenario.Mission", func(s *Session, c *checkpointScenarioContext) { copy := *s.Mission; c.mission = &copy }},
		{"community identity", "communitySchema.mission", func(s *Session, _ *checkpointScenarioContext) { copy := *s.Mission; s.communitySchema.mission = &copy }},
		{"campaign", "Mission.Type", func(s *Session, _ *checkpointScenarioContext) { s.Mission.Type = mission.TypeCampaign }},
		{"restore", "Mission.IsRestore", func(s *Session, _ *checkpointScenarioContext) { s.Mission.IsRestore = true }},
		{"trigger alias within", "Mission.Defeat[2]", func(s *Session, _ *checkpointScenarioContext) {
			s.Mission.Defeat = append(s.Mission.Defeat, s.Mission.Defeat[1])
		}},
		{"trigger alias across", "Mission.Victory[0]", func(s *Session, _ *checkpointScenarioContext) { s.Mission.Victory[0] = s.Mission.Defeat[1] }},
		{"nil class", "classes[0]", func(s *Session, _ *checkpointScenarioContext) { s.Survival.classes[0] = nil }},
		{"class alias", "classes[1]", func(s *Session, _ *checkpointScenarioContext) { s.Survival.classes[1] = s.Survival.classes[0] }},
		{"foreign start", "startClass", func(s *Session, _ *checkpointScenarioContext) {
			copy := *s.Survival.startClass
			s.Survival.startClass = &copy
		}},
		{"foreign definition", "pool.Units[0].Def", func(s *Session, _ *checkpointScenarioContext) {
			s.Survival.pool.Units[0].Def = &content.UnitDef{UnitName: "A"}
		}},
		{"account NaN", "accts[9].Earned", func(s *Session, _ *checkpointScenarioContext) {
			s.Survival.accts[9].Earned = math.Float32frombits(0x7f800001)
		}},
		{"tuning NaN", "tuning.WaveReward", func(s *Session, _ *checkpointScenarioContext) {
			s.Survival.tuning.WaveReward = math.Float32frombits(0xffc00001)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c := scenarioCheckpointFixture()
			tc.edit(s, c)
			err := s.writeScenarioCheckpoint(checkpoint.NewEncoder(io.Discard), c)
			if err == nil || !strings.HasPrefix(err.Error(), "nanolathe:") || !strings.Contains(err.Error(), tc.path) {
				t.Fatalf("refusal: %v", err)
			}
		})
	}
	for _, tc := range []struct {
		s *Session
		c *checkpointScenarioContext
	}{{nil, &checkpointScenarioContext{}}, {&Session{}, nil}} {
		if err := tc.s.writeScenarioCheckpoint(checkpoint.NewEncoder(io.Discard), tc.c); err == nil {
			t.Fatal("nil input admitted")
		}
	}
}

func TestCheckpointScenarioOrderResidualsAndExclusions(t *testing.T) {
	s, c := scenarioCheckpointFixture()
	baseline := scenarioCheckpointBytes(t, s, c)
	for name, edit := range map[string]func(*Session){
		"account residual": func(s *Session) { s.Survival.accts[8].Stock = 1 },
		"settled residual": func(s *Session) { s.Survival.settled[8] = 1 },
		"class order": func(s *Session) {
			s.Survival.classes[0], s.Survival.classes[1] = s.Survival.classes[1], s.Survival.classes[0]
		},
		"start class identity": func(s *Session) { s.Survival.startClass = s.Survival.classes[0] },
		"plan order":           func(s *Session) { g := s.Survival.plan.Groups; g[0], g[1] = g[1], g[0] },
		"unit residual":        func(s *Session) { s.Survival.units[1].shunSerial = 91 },
		"membership order":     func(s *Session) { h := s.Survival.waveUnits; h[0], h[1] = h[1], h[0] },
		"stored pool key":      func(s *Session) { s.Survival.pool.Units[0].Key = "a" },
		"shared info":          func(s *Session) { s.Survival.info.Team = []uint8{9} },
		"trigger progress":     func(s *Session) { s.Mission.Defeat[1].Args[0]-- },
		"deferred cursor":      func(s *Session) { s.communitySchema.nextDeferred++ },
		"raw map residual":     func(s *Session) { s.Survival.removed[65535]++ },
	} {
		t.Run(name, func(t *testing.T) {
			s, c := scenarioCheckpointFixture()
			edit(s)
			if bytes.Equal(baseline, scenarioCheckpointBytes(t, s, c)) {
				t.Fatal("retained mutation vanished")
			}
		})
	}
	s.Survival.removed = map[pool.Handle]int32{}
	for _, h := range []pool.Handle{1, 65535, 0} {
		s.Survival.removed[h] = map[pool.Handle]int32{0: 10, 1: 9, 65535: -8}[h]
	}
	s.communitySchema.diagnostics = []string{"ignored"}
	s.Survival.walk = []*units.Unit{{Handle: 9}}
	s.Survival.history = []SurvivalWaveRecord{{Number: 99}}
	s.Survival.deposits, s.Survival.depositWant, s.Survival.depositHave = 1, 2, 3
	s.Survival.depositAt = [][2]int32{{4, 5}}
	before := *s.Survival
	missionBefore := *s.Mission
	schemaBefore := s.communitySchema
	if got := scenarioCheckpointBytes(t, s, c); !bytes.Equal(baseline, got) {
		t.Fatal("map insertion order or excluded state entered payload")
	}
	if !reflect.DeepEqual(*s.Survival, before) || !reflect.DeepEqual(*s.Mission, missionBefore) || !reflect.DeepEqual(s.communitySchema, schemaBefore) {
		t.Fatal("capture changed retained or excluded state")
	}
	if got := scenarioCheckpointBytes(t, s, c); !bytes.Equal(baseline, got) {
		t.Fatal("repeat capture changed output")
	}
}

func TestCheckpointScenarioSummaryVectorAndAtomicity(t *testing.T) {
	s, _ := scenarioCheckpointFixture()
	var got checkpoint.Summary
	got.Word(101)
	if err := s.appendScenarioCheckpointSummary(&got); err != nil {
		t.Fatal(err)
	}
	// Direct authored words follow the summary schema, not the byte encoder.
	words := []uint64{101, 1, 2, 0, 1, 1, 0, 1, 1, 0, 1, 1, 3, 0xa1a2a3a4, ^uint64(23), ^uint64((1 << 40) - 8), 1<<41 + 9, 0xfefdfcfb, 1<<40 + 23, 25}
	words = append(words, ^uint64(16), 18, ^uint64(18))
	words = append(words, make([]uint64, 8*3)...)
	words = append(words, 20, 21, 22)
	words = append(words, 0x80000000, 0x7f800000, 0xbf800000)
	words = append(words, make([]uint64, 8*3)...)
	words = append(words, 0x40000000, 0x40400000, 0x40800000)
	var sum uint64
	for i, v := range words {
		sum += uint64(i+1) * v
	}
	if n, v := got.Result(); n != uint64(len(words)) || v != sum {
		t.Fatalf("summary %d/%x want %d/%x", n, v, len(words), sum)
	}
	baseline := got
	// Failure at the final physical account must leave the caller's prior
	// words intact. Other floats and full-only references are not read.
	s.Survival.accts[9].Stock = math.Float32frombits(0x7fc00001)
	if err := s.appendScenarioCheckpointSummary(&got); err == nil || got != baseline {
		t.Fatal("selected NaN did not fail atomically")
	}
	s.Survival.accts[9].Stock = 4
	s.Survival.tuning.WaveReward = math.Float32frombits(0x7fc00001)
	s.Survival.classes = []*survivalClass{nil}
	s.Survival.startClass = &survivalClass{}
	s.Survival.pool.Units[0].Def = &content.UnitDef{}
	s.communitySchema.mission = &mission.Mission{}
	var selected checkpoint.Summary
	selected.Word(101)
	if err := s.appendScenarioCheckpointSummary(&selected); err != nil || selected != baseline {
		t.Fatalf("unselected state affected summary: %v", err)
	}
	if n := testing.AllocsPerRun(100, func() {
		var v checkpoint.Summary
		if err := s.appendScenarioCheckpointSummary(&v); err != nil {
			panic(err)
		}
	}); n != 0 {
		t.Fatalf("summary allocated %v", n)
	}
	if err := (*Session)(nil).appendScenarioCheckpointSummary(&got); err == nil || got != baseline {
		t.Fatal("nil session summary")
	}
	if err := s.appendScenarioCheckpointSummary(nil); err == nil {
		t.Fatal("nil accumulator admitted")
	}
}

type scenarioFailWriter struct{ err error }

func (w scenarioFailWriter) Write([]byte) (int, error) { return 0, w.err }

func TestCheckpointScenarioEmptyPresenceAndSinkFailure(t *testing.T) {
	s := &Session{}
	c := &checkpointScenarioContext{}
	if got := scenarioCheckpointBytes(t, s, c); !bytes.Equal(got, make([]byte, 27)) {
		t.Fatalf("empty framing %x", got)
	}
	var summary checkpoint.Summary
	if err := s.appendScenarioCheckpointSummary(&summary); err != nil {
		t.Fatal(err)
	}
	if n, v := summary.Result(); n != 2 || v != 0 {
		t.Fatalf("absent summary %d/%d", n, v)
	}
	failure := errors.New("authored sink failure")
	e := checkpoint.NewEncoder(scenarioFailWriter{failure})
	if err := s.writeScenarioCheckpoint(e, c); !errors.Is(err, failure) {
		t.Fatalf("sink failure %v", err)
	}
}

func TestCheckpointScenarioPoolDefinitionIdentity(t *testing.T) {
	defs := [2]*content.UnitDef{}
	for i := range defs {
		defs[i] = &content.UnitDef{DefinitionHeader: content.DefinitionHeader{CanonicalKey: "same"}, UnitName: "same", ObjectName: "fixture"}
	}
	fs := fsFromMapSkirmish(t, map[string]string{"objects3d/fixture.3do": string(fixture3DO(0))})
	sources, err := content.CaptureSimulationSources(fs, "")
	if err != nil {
		t.Fatal(err)
	}
	cat := &content.Catalog{Units: map[string]*content.UnitDef{"a": defs[0], "b": defs[1]}}
	inputs, err := content.FreezeSimulationInputs(sources, content.SimulationInputRequest{Catalog: cat})
	if err != nil {
		t.Fatal(err)
	}
	keys, err := inputs.CheckpointKeys()
	if err != nil {
		t.Fatal(err)
	}
	p := survival.Pool{Units: []survival.Unit{{Def: defs[1], Key: "stored spell"}, {Def: defs[0]}}}
	var out bytes.Buffer
	writeScenarioPool(checkpoint.NewEncoder(&out), &p, keys)
	var want scenarioVector
	want.zero(32)
	want.u32(2)
	want.i64(0)
	want.u8(1, 1)
	want.u32(2)
	want.text("unit/same")
	want.u8(0)
	want.text("stored spell")
	want.i64(0)
	want.i64(0)
	want.u8(1, 1)
	want.u32(1)
	want.text("unit/same")
	want.u8(0)
	want.text("")
	want.i64(0)
	if !bytes.Equal(out.Bytes(), want) {
		t.Fatalf("definition ordinals got %x want %x", out.Bytes(), []byte(want))
	}
	foreign := *defs[1]
	p.Units[0].Def = &foreign
	e := checkpoint.NewEncoder(io.Discard)
	writeScenarioPool(e, &p, keys)
	if err := e.Err(); err == nil || !strings.Contains(err.Error(), "pool.Units[0].Def") {
		t.Fatalf("foreign same-name object: %v", err)
	}
}
