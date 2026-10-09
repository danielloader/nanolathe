package effects

import (
	"bytes"
	"encoding/hex"
	"reflect"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
)

func checkpointHex(t *testing.T, text string) []byte {
	t.Helper()
	data, err := hex.DecodeString(text)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func checkpointFixedBytes(t *testing.T, p *FixedEffectPool) []byte {
	t.Helper()
	c := NewCheckpointContext(nil, p)
	if n, err := p.CollectCheckpointReferences(c); n != 0 || err != nil {
		t.Fatalf("collect = %d, %v", n, err)
	}
	var out bytes.Buffer
	if err := p.WriteCheckpoint(checkpoint.NewEncoder(&out), c); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func checkpointServiceBytes(t *testing.T, s *EffectService, p *FixedEffectPool) []byte {
	t.Helper()
	c := NewCheckpointContext(nil, p)
	if n, err := s.CollectCheckpointReferences(c); n != 0 || err != nil {
		t.Fatalf("collect = %d, %v", n, err)
	}
	var out bytes.Buffer
	if err := s.WriteCheckpoint(checkpoint.NewEncoder(&out), c); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// This authored vector includes literal inactive cursor residuals and a frame
// count outside i32. No playback, duration validation or narrowing is allowed.
func TestCheckpointAnimationVector(t *testing.T) {
	a := EffectAnimPlayer{Countdown: -3, Durations: []int32{7, 65536, -1}, Frames: 1<<40 + 3, Idx: -2, Loop: true}
	var out bytes.Buffer
	if err := a.WriteCheckpoint(checkpoint.NewEncoder(&out)); err != nil {
		t.Fatal(err)
	}
	want := checkpointHex(t, "00fdffffff030000000700000000000100ffffffff0300000000010000feffffff01")
	if !bytes.Equal(out.Bytes(), want) {
		t.Fatalf("animation got %x want %x", out.Bytes(), want)
	}
}

func TestCheckpointEffectServiceVectorIdentityAndExclusions(t *testing.T) {
	p := &FixedEffectPool{}
	s := &EffectService{max: -(1 << 40), nextID: 0xfedcba98, lastSequence: 0x0102030405060708, owner: p}
	want := checkpointHex(t, "0008070605040302010000000000ffffff98badcfe01")
	if got := checkpointServiceBytes(t, s, p); !bytes.Equal(got, want) {
		t.Fatalf("service got %x want %x", got, want)
	}
	for _, edit := range []func(*EffectService){
		func(s *EffectService) { s.max++ }, func(s *EffectService) { s.nextID = 0 },
		func(s *EffectService) { s.lastSequence++ }, func(s *EffectService) { s.owner = nil },
	} {
		changed := *s
		edit(&changed)
		if bytes.Equal(checkpointServiceBytes(t, &changed, p), want) {
			t.Fatal("retained service mutation lost")
		}
	}
	s.dropped, s.refusedAtCapacity = 4, 5
	backing := []frame.EffectView{{ID: 42}}
	s.pending = backing[:0]
	before := *s
	if got := checkpointServiceBytes(t, s, p); !bytes.Equal(got, want) || !reflect.DeepEqual(*s, before) || backing[0].ID != 42 {
		t.Fatal("diagnostic/backing state changed bytes or capture mutated service")
	}
}

func TestCheckpointFixedPoolVector(t *testing.T) {
	p := &FixedEffectPool{capacity: 2, fragmentContext: FragmentStepContext{Gravity: -1, Lava: true, SeaLevel: 1 << 40, WaterEffectsWordZero: true},
		fragmentCursor: 1, fragmentRoundRobin: true, gravity: -2, seaLevel: 1<<42 + 3,
		fragments: []fragmentGeometry{{angles: [3]uint16{1, 2, 3}, angularRates: [3]uint16{4, 5, 6}, baseVelocity: [3]int32{-7, 8, -9}}, {live: true}},
		records: []EffectRecord{{AnimA: EffectAnimPlayer{Countdown: -3, Durations: []int32{7, 65536, -1}, Frames: 3, Idx: -2, Loop: true},
			AnimB:      EffectAnimPlayer{Active: true, Countdown: 2, Durations: []int32{11, 12}, Frames: 2, Idx: 1},
			ExpiryTick: 0x89abcdef, FragmentExplodeOnHit: true, FragmentSlot: 2, Gravity: -4, HasModel: true, Kind: "K\xff", Source: 65535, Target: 0x1234,
			VX: 5, VY: -6, VZ: 1 << 40, X: 7, Y: -8, Z: -(1 << 42)}},
	}
	p.fragments[0].vertices[0] = [3]numeric.Fixed{-1, 1 << 40, 3}
	want := checkpointHex(t, "0200000000000000"+"ffffffffffffffff000100000000000100000001"+"010000000000000001"+"02000000")
	want = append(want, checkpointHex(t, "010002000300040005000600f9ffffff08000000f7ffffff00"+"ffffffffffffffff00000000000100000300000000000000")...)
	want = append(want, make([]byte, 7*3*8)...)
	want = append(want, make([]byte, 24)...)
	want = append(want, 1)
	want = append(want, make([]byte, 8*3*8)...)
	want = append(want, checkpointHex(t, "feffffffffffffff0001000000")...)
	want = append(want, checkpointHex(t, "00fdffffff030000000700000000000100ffffffff0300000000000000feffffff01"+
		"0102000000020000000b0000000c00000002000000000000000100000000"+
		"efcdab89010200fcffffffffffffff01"+"020000004bff"+"ffff000034120000"+
		"0500000000000000faffffffffffffff00000000000100000700000000000000f8ffffffffffffff0000000000fcffff"+
		"0300000000040000")...)
	if got := checkpointFixedBytes(t, p); !bytes.Equal(got, want) {
		t.Fatalf("pool payload\ngot  %x\nwant %x", got, want)
	}
}

func TestCheckpointFixedRetainedMutations(t *testing.T) {
	fresh := func() *FixedEffectPool {
		return &FixedEffectPool{capacity: 2, fragments: make([]fragmentGeometry, 2), records: []EffectRecord{{AnimA: EffectAnimPlayer{Durations: []int32{2, 3}}, AnimB: EffectAnimPlayer{Durations: []int32{4}}}}}
	}
	baseline := checkpointFixedBytes(t, fresh())
	for _, test := range []struct {
		name string
		edit func(*FixedEffectPool)
	}{
		{"capacity", func(p *FixedEffectPool) { p.capacity++ }},
		{"context gravity", func(p *FixedEffectPool) { p.fragmentContext.Gravity = 1 << 40 }},
		{"context lava", func(p *FixedEffectPool) { p.fragmentContext.Lava = true }},
		{"context sea", func(p *FixedEffectPool) { p.fragmentContext.SeaLevel = -(1 << 40) }},
		{"context water", func(p *FixedEffectPool) { p.fragmentContext.WaterEffectsWordZero = true }},
		{"cursor", func(p *FixedEffectPool) { p.fragmentCursor = 1 }},
		{"round robin", func(p *FixedEffectPool) { p.fragmentRoundRobin = true }},
		{"fragment count", func(p *FixedEffectPool) { p.fragments = append(p.fragments, fragmentGeometry{}) }},
		{"dead fragment angles", func(p *FixedEffectPool) { p.fragments[1].angles[2] = 65535 }},
		{"dead fragment rates", func(p *FixedEffectPool) { p.fragments[1].angularRates[1] = 7 }},
		{"dead fragment base", func(p *FixedEffectPool) { p.fragments[1].baseVelocity[0] = -9 }},
		{"fragment live", func(p *FixedEffectPool) { p.fragments[1].live = true }},
		{"dead fragment vertices", func(p *FixedEffectPool) { p.fragments[1].vertices[7][2] = 1 << 40 }},
		{"gravity", func(p *FixedEffectPool) { p.gravity = -3 }},
		{"sea", func(p *FixedEffectPool) { p.seaLevel = 99 }},
		{"record count", func(p *FixedEffectPool) { p.records = append(p.records, EffectRecord{}) }},
		{"expiry", func(p *FixedEffectPool) { p.records[0].ExpiryTick = 77 }},
		{"fragment explode", func(p *FixedEffectPool) { p.records[0].FragmentExplodeOnHit = true }},
		{"fragment slot", func(p *FixedEffectPool) { p.records[0].FragmentSlot = 2 }},
		{"record gravity", func(p *FixedEffectPool) { p.records[0].Gravity = 1 << 40 }},
		{"model presence", func(p *FixedEffectPool) { p.records[0].HasModel = true }},
		{"kind", func(p *FixedEffectPool) { p.records[0].Kind = "\xff" }},
		{"source", func(p *FixedEffectPool) { p.records[0].Source = 65535 }},
		{"target", func(p *FixedEffectPool) { p.records[0].Target = 65535 }},
		{"vx", func(p *FixedEffectPool) { p.records[0].VX = 1 << 40 }},
		{"vy", func(p *FixedEffectPool) { p.records[0].VY = -(1 << 40) }},
		{"vz", func(p *FixedEffectPool) { p.records[0].VZ = 9 }},
		{"x", func(p *FixedEffectPool) { p.records[0].X = -9 }},
		{"y", func(p *FixedEffectPool) { p.records[0].Y = 1 << 40 }},
		{"z", func(p *FixedEffectPool) { p.records[0].Z = -(1 << 40) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := fresh()
			test.edit(p)
			if bytes.Equal(checkpointFixedBytes(t, p), baseline) {
				t.Fatal("retained mutation lost")
			}
		})
	}
	for _, field := range []string{"Active", "Countdown", "Durations", "Frames", "Idx", "Loop"} {
		for _, second := range []bool{false, true} {
			p := fresh()
			a := &p.records[0].AnimA
			if second {
				a = &p.records[0].AnimB
			}
			switch field {
			case "Active":
				a.Active = true
			case "Countdown":
				a.Countdown = -1
			case "Durations":
				a.Durations[0] = 65536
			case "Frames":
				a.Frames = 1 << 40
			case "Idx":
				a.Idx = -2
			case "Loop":
				a.Loop = true
			}
			if bytes.Equal(checkpointFixedBytes(t, p), baseline) {
				t.Fatalf("animation %s, second=%v mutation lost", field, second)
			}
		}
	}
}

type checkpointUnknownPool struct{ EffectPool }
type checkpointImpact struct{ FragmentImpactSink }

func TestCheckpointEffectsRefuseUnattestedBindings(t *testing.T) {
	p := &FixedEffectPool{}
	c := NewCheckpointContext(nil, p)
	for _, test := range []struct {
		name string
		s    *EffectService
	}{
		{"effects.EffectService", nil},
		{"effects.EffectService.art", &EffectService{art: &content.SimArt{}}},
		{"effects.EffectService.owner", &EffectService{owner: &FixedEffectPool{}}},
		{"effects.EffectService.owner", &EffectService{owner: (*FixedEffectPool)(nil)}},
		{"effects.EffectService.owner", &EffectService{owner: checkpointUnknownPool{}}},
		{"effects.EffectService.pending", &EffectService{pending: []frame.EffectView{{}}}},
		{"effects.EffectService.pending", &EffectService{owner: p, pending: []frame.EffectView{{}}}},
	} {
		if n, err := test.s.CollectCheckpointReferences(c); n != 0 || err == nil || !strings.HasPrefix(err.Error(), "nanolathe: checkpoint capture failed: logical path "+test.name+",") {
			t.Fatalf("service collect refusal %s = %d, %v", test.name, n, err)
		}
		var out bytes.Buffer
		if err := test.s.WriteCheckpoint(checkpoint.NewEncoder(&out), c); err == nil || out.Len() != 0 {
			t.Fatalf("service write refusal %s = %v, bytes=%d", test.name, err, out.Len())
		}
	}
	if _, err := (&EffectService{}).CollectCheckpointReferences(nil); err == nil {
		t.Fatal("nil service context accepted")
	}
	for _, test := range []struct {
		field string
		edit  func(*FixedEffectPool)
	}{
		{"fragmentStepping", func(p *FixedEffectPool) { p.fragmentStepping = true }},
		{"fragmentContext.Impact", func(p *FixedEffectPool) { p.fragmentContext.Impact = checkpointImpact{} }},
		{"fragmentContext.Impact", func(p *FixedEffectPool) { p.fragmentContext.Impact = (*checkpointImpact)(nil) }},
		{"fragmentContext.TerrainHeight", func(p *FixedEffectPool) {
			p.SetFragmentStepContext(FragmentStepContext{TerrainHeight: func(numeric.Fixed, numeric.Fixed) numeric.Fixed { panic("height called") }})
		}},
		{"heightAt", func(p *FixedEffectPool) {
			p.heightAt = func(numeric.Fixed, numeric.Fixed) numeric.Fixed { panic("height called") }
		}},
	} {
		p := &FixedEffectPool{}
		test.edit(p)
		c := NewCheckpointContext(nil, p)
		if _, err := p.CollectCheckpointReferences(c); err == nil || !strings.Contains(err.Error(), "effects.FixedEffectPool."+test.field) {
			t.Fatalf("pool collect %s = %v", test.field, err)
		}
		var out bytes.Buffer
		if err := p.WriteCheckpoint(checkpoint.NewEncoder(&out), c); err == nil || out.Len() != 0 {
			t.Fatalf("pool write %s = %v, bytes=%d", test.field, err, out.Len())
		}
	}
	for _, c := range []*CheckpointContext{nil, {}, NewCheckpointContext(nil, &FixedEffectPool{})} {
		if _, err := p.CollectCheckpointReferences(c); err == nil {
			t.Fatal("foreign pool context accepted")
		}
	}
	var out bytes.Buffer
	if err := (*EffectAnimPlayer)(nil).WriteCheckpoint(checkpoint.NewEncoder(&out)); err == nil || out.Len() != 0 {
		t.Fatal("nil animation accepted")
	}
}

func TestCheckpointFixedExclusionsOrderingAndPurity(t *testing.T) {
	p := &FixedEffectPool{capacity: 2, fragments: make([]fragmentGeometry, 2), records: []EffectRecord{{Kind: "a", X: 1, AnimA: EffectAnimPlayer{Durations: []int32{7}}}, {Kind: "b", X: 2}}}
	want := checkpointFixedBytes(t, p)
	r := &p.records[0]
	r.PresentationID, r.ID, r.EventSeq, r.EffectID = 1, 2, 3, 4
	r.Piece, r.SFXType, r.SFXClass, r.Mode, r.StartTick = 5, 6, 7, 8, 9
	r.Graphic, r.AssetID, r.SequenceID, r.Strip = "graphic", "asset", "sequence", -1
	r.TargetX, r.TargetY, r.TargetZ = 1, 2, 3
	r.HasBlastProfile, r.BlastAreaOfEffect, r.BlastDamage = true, 17, 19
	r.HasCalculatedFlash, r.CalculatedTable = true, 2
	r.NanolatheGeometryKnown, r.NanolatheTargetBoxKnown, r.NanolatheBoxAtSource = true, true, true
	r.NanolatheTargetMin, r.NanolatheTargetMax = [3]numeric.Fixed{1, 2, 3}, [3]numeric.Fixed{4, 5, 6}
	p.fragments[0].material = FrozenFragmentMaterial{UnitDefID: 1, PieceIndex: 2, PrimitiveIndex: 3, FrameIndex: 4, Valid: true}
	beforeRecords := append([]EffectRecord(nil), p.records...)
	beforeDurations := append([]int32(nil), p.records[0].AnimA.Durations...)
	beforeFragments := append([]fragmentGeometry(nil), p.fragments...)
	if got := checkpointFixedBytes(t, p); !bytes.Equal(got, want) || !reflect.DeepEqual(beforeRecords, p.records) || !reflect.DeepEqual(beforeDurations, p.records[0].AnimA.Durations) || !reflect.DeepEqual(beforeFragments, p.fragments) {
		t.Fatal("excluded metadata changed bytes or capture mutated pool")
	}
	p.records[0], p.records[1] = p.records[1], p.records[0]
	if bytes.Equal(checkpointFixedBytes(t, p), want) {
		t.Fatal("stored record order lost")
	}
	p.records[0], p.records[1] = p.records[1], p.records[0]
	p.fragments[0].baseVelocity[0] = 1
	fragmentOrder := checkpointFixedBytes(t, p)
	p.fragments[0], p.fragments[1] = p.fragments[1], p.fragments[0]
	if bytes.Equal(checkpointFixedBytes(t, p), fragmentOrder) {
		t.Fatal("physical fragment order lost")
	}
	zero := &FixedEffectPool{}
	before := *zero
	checkpointFixedBytes(t, zero)
	if !reflect.DeepEqual(*zero, before) {
		t.Fatal("capture initialized zero-value pool storage")
	}
	// The append path overwrites records outside length; capacity and old
	// storage do not add records to the payload.
	a := &FixedEffectPool{records: []EffectRecord{{X: 3}, {X: 99}}}
	b := &FixedEffectPool{records: []EffectRecord{{X: 3}}}
	a.records = a.records[:1]
	if !bytes.Equal(checkpointFixedBytes(t, a), checkpointFixedBytes(t, b)) {
		t.Fatal("unused record backing leaked into payload")
	}
}

func TestCheckpointAnimationPartialCompletion(t *testing.T) {
	p := &FixedEffectPool{records: []EffectRecord{{
		AnimA: EffectAnimPlayer{Active: true, Frames: 1, Countdown: 1, Durations: []int32{1}},
		AnimB: EffectAnimPlayer{Active: true, Frames: 2, Countdown: 3, Durations: []int32{3, 7}},
	}}}
	p.Update(1)
	if len(p.records) != 1 || p.records[0].AnimA.Active || !p.records[0].AnimB.Active {
		t.Fatal("fixture did not reach partial completion")
	}
	before := p.records[0]
	got := checkpointFixedBytes(t, p)
	if !reflect.DeepEqual(p.records[0], before) || before.AnimA.Frames != 1 || before.AnimB.Countdown != 2 {
		t.Fatal("capture advanced or normalized the partially completed record")
	}
	p.records[0].AnimA.Active = true
	if bytes.Equal(checkpointFixedBytes(t, p), got) {
		t.Fatal("terminated and attached frame-zero cursor collapsed")
	}
}

func TestCheckpointEffectsSummaryVectorAndBlindSpots(t *testing.T) {
	s := &EffectService{nextID: 2, lastSequence: 3}
	p := &FixedEffectPool{records: []EffectRecord{{X: -1, Y: 1 << 40, Z: 3, VX: -4, VY: 5, VZ: 6, ExpiryTick: 7,
		AnimA: EffectAnimPlayer{Idx: -8, Countdown: 9}, AnimB: EffectAnimPlayer{Idx: 10, Countdown: -11}}, {X: 12}}, fragments: []fragmentGeometry{{live: true}, {}, {live: true}}}
	d := &DebrisPool{count: 99, slots: []debrisSlot{{live: true}, {}}}
	words := []uint64{2, 3, 2, 0xffffffffffffffff, 1 << 40, 3, 0xfffffffffffffffc, 5, 6, 7, 0xfffffffffffffff8, 9, 10, 0xfffffffffffffff5,
		12, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 1}
	var summary checkpoint.Summary
	summary.Word(17)
	if err := s.AppendCheckpointSummary(&summary); err != nil {
		t.Fatal(err)
	}
	if err := p.AppendCheckpointSummary(&summary); err != nil {
		t.Fatal(err)
	}
	if err := d.AppendCheckpointSummary(&summary); err != nil {
		t.Fatal(err)
	}
	count, sum := uint64(1), uint64(17)
	for _, word := range words {
		count++
		sum += count * word
	}
	if n, got := summary.Result(); n != count || got != sum {
		t.Fatalf("summary (%d,%x) want (%d,%x)", n, got, count, sum)
	}

	read := func() checkpoint.Summary {
		var result checkpoint.Summary
		if err := p.AppendCheckpointSummary(&result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	beforeSummary, beforeFull := read(), checkpointFixedBytes(t, p)
	p.records[0].AnimA.Active = true
	p.records[0].AnimA.Durations = []int32{9}
	p.fragments[0].vertices[0][0] = 1
	if read() != beforeSummary || bytes.Equal(checkpointFixedBytes(t, p), beforeFull) {
		t.Fatal("animation/geometry must be full-only blind spots")
	}
	p.records[1].Z = 13
	if read() == beforeSummary {
		t.Fatal("selected coordinate mutation lost")
	}
	p.records[1].Z = 0
	// Summary does not validate bindings, fragment stepping or debris partition
	// bounds, because these are outside its selected scalar row.
	p.fragmentStepping = true
	p.heightAt = func(numeric.Fixed, numeric.Fixed) numeric.Fixed { panic("height") }
	p.fragmentContext.Impact = checkpointImpact{}
	s.owner, s.art, s.pending = checkpointUnknownPool{}, &content.SimArt{}, []frame.EffectView{{}}
	var summaryErr error
	for _, allocations := range []float64{
		testing.AllocsPerRun(50, func() { var result checkpoint.Summary; summaryErr = s.AppendCheckpointSummary(&result) }),
		testing.AllocsPerRun(50, func() { var result checkpoint.Summary; summaryErr = p.AppendCheckpointSummary(&result) }),
		testing.AllocsPerRun(50, func() { var result checkpoint.Summary; summaryErr = d.AppendCheckpointSummary(&result) }),
	} {
		if allocations != 0 || summaryErr != nil {
			t.Fatalf("summary allocations = %g, error %v", allocations, summaryErr)
		}
	}
	for _, appendSummary := range []func(*checkpoint.Summary) error{s.AppendCheckpointSummary, p.AppendCheckpointSummary, d.AppendCheckpointSummary} {
		if err := appendSummary(nil); err == nil {
			t.Fatal("nil summary accepted")
		}
	}
	for _, appendSummary := range []func(*checkpoint.Summary) error{(*EffectService)(nil).AppendCheckpointSummary, (*FixedEffectPool)(nil).AppendCheckpointSummary, (*DebrisPool)(nil).AppendCheckpointSummary} {
		before := summary
		if err := appendSummary(&summary); err == nil || summary != before {
			t.Fatal("nil owner must fail atomically")
		}
	}
}
