package effects

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/sim/rng"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// A noncomparable interface payload makes accidental interface equality fail;
// its methods make any capture-time invocation fail independently.
type checkpointHostileImpact []int

func (checkpointHostileImpact) GroundFragmentImpact(GroundFragmentImpact) { panic("impact called") }
func (checkpointHostileImpact) WaterFragmentImpact(WaterFragmentImpact)   { panic("impact called") }

type checkpointHostilePool struct {
	EffectPool
	noncomparable []int
}

type checkpointEffectsFixture struct {
	p         *FixedEffectPool
	s         *EffectService
	c         *CheckpointContext
	art       *content.SimArt
	terrain   *world.Terrain
	authority *checkpoint.BindingAuthority
	tick      uint32
	ctx       FragmentStepContext
}

func checkpointEffectsBoundFixture(t *testing.T) checkpointEffectsFixture {
	t.Helper()
	f := checkpointEffectsFixture{p: &FixedEffectPool{}, art: &content.SimArt{}, terrain: &world.Terrain{}, authority: checkpoint.NewBindingAuthority(), tick: 37}
	f.s = NewEffectServiceWithPool(7, f.p, f.art)
	f.c = NewCheckpointContext(nil, f.p)
	f.ctx = FragmentStepContext{TerrainHeight: func(numeric.Fixed, numeric.Fixed) numeric.Fixed { panic("height called") }, Impact: checkpointHostileImpact{1}}
	f.s.SetFragmentStepContextWithCheckpointBinding(f.ctx, f.terrain, f.tick, f.authority)
	if err := f.c.SetBindings(f.s, f.art, f.terrain, f.tick, f.authority); err != nil {
		t.Fatal(err)
	}
	return f
}

func checkpointBoundEffectsBytes(t *testing.T, f checkpointEffectsFixture) ([]byte, []byte) {
	t.Helper()
	var service, pool bytes.Buffer
	if n, err := f.s.CollectCheckpointReferences(f.c); n != 0 || err != nil {
		t.Fatalf("service collect = %d, %v", n, err)
	}
	if n, err := f.p.CollectCheckpointReferences(f.c); n != 0 || err != nil {
		t.Fatalf("pool collect = %d, %v", n, err)
	}
	if err := f.s.WriteCheckpoint(checkpoint.NewEncoder(&service), f.c); err != nil {
		t.Fatal(err)
	}
	if err := f.p.WriteCheckpoint(checkpoint.NewEncoder(&pool), f.c); err != nil {
		t.Fatal(err)
	}
	return service.Bytes(), pool.Bytes()
}

func TestCheckpointEffectsBindingPresenceVector(t *testing.T) {
	f := checkpointEffectsBoundFixture(t)
	f.s.max, f.s.nextID, f.s.lastSequence = -(1 << 40), 0xfedcba98, 0x0102030405060708
	f.p.capacity, f.p.fragmentCursor, f.p.fragmentRoundRobin = 7, -3, true
	f.p.gravity, f.p.seaLevel = -2, 1<<42+3
	f.ctx.Gravity, f.ctx.Lava, f.ctx.SeaLevel = -1, true, 1<<40
	f.p.SetFragmentStepContextWithCheckpointBinding(f.ctx, f.terrain, f.tick, f.authority)
	wantService := checkpointHex(t, "0108070605040302010000000000ffffff98badcfe01")
	// Existing lexical fields, with only the three former absent binding tags
	// now present. The enable predicate and captured tick add no wire words.
	wantPool := checkpointHex(t, "0700000000000000"+
		"ffffffffffffffff010100000000000100000100"+
		"fdffffffffffffff0100000000"+
		"feffffffffffffff00000000000300000000040000")
	service, pool := checkpointBoundEffectsBytes(t, f)
	if !bytes.Equal(service, wantService) || !bytes.Equal(pool, wantPool) {
		t.Fatalf("service %x want %x; pool %x want %x", service, wantService, pool, wantPool)
	}
	for _, height := range []bool{false, true} {
		for _, impact := range []bool{false, true} {
			ctx := f.ctx
			if !height {
				ctx.TerrainHeight = nil
			}
			if !impact {
				ctx.Impact = nil
			}
			f.p.SetFragmentStepContextWithCheckpointBinding(ctx, f.terrain, f.tick, f.authority)
			_, got := checkpointBoundEffectsBytes(t, f)
			want := append([]byte(nil), wantPool...)
			if !impact {
				want[16] = 0
			}
			if !height {
				want[26] = 0
			}
			if !bytes.Equal(got, want) || f.p.fragmentStepping != height {
				t.Fatalf("height=%v impact=%v: %x want %x", height, impact, got, want)
			}
		}
	}
}

func TestCheckpointEffectsBindingReplacementAndCopies(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*checkpointEffectsFixture)
	}{
		{"ordinary pool reinstall", func(f *checkpointEffectsFixture) { f.p.SetFragmentStepContext(f.ctx) }},
		{"ordinary service reinstall", func(f *checkpointEffectsFixture) { f.s.SetFragmentStepContext(f.ctx) }},
		{"nil authority", func(f *checkpointEffectsFixture) {
			f.p.SetFragmentStepContextWithCheckpointBinding(f.ctx, f.terrain, f.tick, nil)
		}},
		{"foreign authority", func(f *checkpointEffectsFixture) {
			f.p.SetFragmentStepContextWithCheckpointBinding(f.ctx, f.terrain, f.tick, checkpoint.NewBindingAuthority())
		}},
		{"foreign terrain", func(f *checkpointEffectsFixture) {
			f.p.SetFragmentStepContextWithCheckpointBinding(f.ctx, &world.Terrain{}, f.tick, f.authority)
		}},
		{"nil terrain", func(f *checkpointEffectsFixture) {
			f.p.SetFragmentStepContextWithCheckpointBinding(f.ctx, nil, f.tick, f.authority)
		}},
		{"foreign tick", func(f *checkpointEffectsFixture) {
			f.p.SetFragmentStepContextWithCheckpointBinding(f.ctx, f.terrain, f.tick+1, f.authority)
		}},
		{"foreign art", func(f *checkpointEffectsFixture) { f.s.art = &content.SimArt{} }},
		{"cleared art", func(f *checkpointEffectsFixture) { f.s.art = nil }},
		{"foreign owner", func(f *checkpointEffectsFixture) { f.s.owner = &FixedEffectPool{} }},
		{"noncomparable owner", func(f *checkpointEffectsFixture) { f.s.owner = checkpointHostilePool{noncomparable: []int{1}} }},
		{"typed nil owner", func(f *checkpointEffectsFixture) { f.s.owner = (*FixedEffectPool)(nil) }},
		{"cleared owner", func(f *checkpointEffectsFixture) { f.s.owner = nil }},
		{"changed public pool", func(f *checkpointEffectsFixture) { f.c.Pool = &FixedEffectPool{} }},
		{"height enable corrupted", func(f *checkpointEffectsFixture) { f.p.fragmentStepping = false }},
		{"unsupported bounce", func(f *checkpointEffectsFixture) {
			f.p.SetHeightFunc(func(numeric.Fixed, numeric.Fixed) numeric.Fixed { panic("bounce called") })
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := checkpointEffectsBoundFixture(t)
			tc.mutate(&f)
			before := *f.c
			if err := f.c.SetBindings(f.s, f.art, f.terrain, f.tick, f.authority); err == nil || *f.c != before {
				t.Fatalf("repeat accepted stale state or changed context: %v", err)
			}
			if _, err := f.s.CollectCheckpointReferences(f.c); err == nil {
				t.Fatal("service accepted stale binding")
			}
			if _, err := f.p.CollectCheckpointReferences(f.c); err == nil {
				t.Fatal("pool accepted stale binding")
			}
			var out bytes.Buffer
			if err := f.s.WriteCheckpoint(checkpoint.NewEncoder(&out), f.c); err == nil || out.Len() != 0 {
				t.Fatalf("service refusal = %v, bytes=%d", err, out.Len())
			}
			if err := f.p.WriteCheckpoint(checkpoint.NewEncoder(&out), f.c); err == nil || out.Len() != 0 {
				t.Fatalf("pool refusal = %v, bytes=%d", err, out.Len())
			}
		})
	}
	f := checkpointEffectsBoundFixture(t)
	copyPool := *f.p
	copyService := NewEffectServiceWithPool(7, &copyPool, f.art)
	copyContext := NewCheckpointContext(nil, &copyPool)
	if err := copyContext.SetBindings(copyService, f.art, f.terrain, f.tick, f.authority); err == nil {
		t.Fatal("copied pool inherited proof")
	}
	if copyContext.binding.service != nil {
		t.Fatal("failed copy registration changed context")
	}
	copyServiceValue := *f.s
	if _, err := copyServiceValue.CollectCheckpointReferences(f.c); err == nil {
		t.Fatal("foreign service used existing registration")
	}
	checkpointBoundEffectsBytes(t, f)
}

func TestCheckpointEffectsBindingRegistrationAtomicity(t *testing.T) {
	f := checkpointEffectsBoundFixture(t)
	for _, change := range []func(*checkpointEffectsFixture){
		func(f *checkpointEffectsFixture) { f.s = NewEffectServiceWithPool(7, f.p, f.art) },
		func(f *checkpointEffectsFixture) { f.art = &content.SimArt{} },
		func(f *checkpointEffectsFixture) { f.terrain = &world.Terrain{} },
		func(f *checkpointEffectsFixture) { f.tick++ },
		func(f *checkpointEffectsFixture) { f.authority = checkpoint.NewBindingAuthority() },
		func(f *checkpointEffectsFixture) { f.s = nil },
		func(f *checkpointEffectsFixture) { f.art = nil },
		func(f *checkpointEffectsFixture) { f.terrain = nil },
		func(f *checkpointEffectsFixture) { f.authority = nil },
	} {
		changed := f
		change(&changed)
		before := *f.c
		if err := f.c.SetBindings(changed.s, changed.art, changed.terrain, changed.tick, changed.authority); err == nil || *f.c != before {
			t.Fatalf("conflict accepted or changed context: %v", err)
		}
		// Initial registration validates identities, not just repeated tuples.
		fresh := NewCheckpointContext(nil, f.p)
		if changed.s != nil && changed.s != f.s {
			continue
		} // an independently supplied service may own this same pool
		if err := fresh.SetBindings(changed.s, changed.art, changed.terrain, changed.tick, changed.authority); err == nil || fresh.binding.service != nil {
			t.Fatalf("invalid first registration accepted: %v", err)
		}
	}
	for _, c := range []*CheckpointContext{nil, NewCheckpointContext(nil, nil), NewCheckpointContext(nil, &FixedEffectPool{})} {
		if err := c.SetBindings(f.s, f.art, f.terrain, f.tick, f.authority); err == nil {
			t.Fatal("missing/foreign context pool accepted")
		}
	}
	if err := f.c.SetBindings(f.s, f.art, f.terrain, f.tick, f.authority); err != nil {
		t.Fatal(err)
	}
	checkpointBoundEffectsBytes(t, f)
}

// Unknown pools keep exactly their ordinary setter dispatch. Neither the
// diagnostic narrowing nor any capture may invoke one of their methods.
type checkpointForwardPool struct {
	EffectPool
	calls int
	ctx   FragmentStepContext
}

func (p *checkpointForwardPool) SetFragmentStepContext(ctx FragmentStepContext) {
	p.calls++
	p.ctx = ctx
}

func TestCheckpointEffectPoolNarrowingAndOrdinaryForwarding(t *testing.T) {
	for _, s := range []*EffectService{nil, {}, {owner: (*FixedEffectPool)(nil)}, {owner: checkpointHostilePool{noncomparable: []int{1}}}} {
		if s.CheckpointFixedPool() != nil {
			t.Fatal("foreign owner narrowed")
		}
	}
	p := &FixedEffectPool{}
	s := NewEffectServiceWithPool(7, p, nil)
	if s.CheckpointFixedPool() != p {
		t.Fatal("exact pool not returned")
	}
	(*EffectService)(nil).SetFragmentStepContextWithCheckpointBinding(FragmentStepContext{}, nil, 0, nil)
	(*FixedEffectPool)(nil).SetFragmentStepContextWithCheckpointBinding(FragmentStepContext{}, nil, 0, checkpoint.NewBindingAuthority())
	s.owner = (*FixedEffectPool)(nil)
	s.SetFragmentStepContextWithCheckpointBinding(FragmentStepContext{}, nil, 0, nil)
	foreign := &checkpointForwardPool{}
	s.owner = foreign
	ctx := FragmentStepContext{Gravity: 19, Impact: checkpointHostileImpact{1}}
	s.SetFragmentStepContextWithCheckpointBinding(ctx, &world.Terrain{}, 17, checkpoint.NewBindingAuthority())
	if foreign.calls != 1 || foreign.ctx.Gravity != 19 || foreign.ctx.Impact == nil {
		t.Fatal("ordinary forwarding changed")
	}
	if _, err := s.CollectCheckpointReferences(NewCheckpointContext(nil, p)); err == nil || foreign.calls != 1 {
		t.Fatal("foreign capture admitted or invoked owner")
	}
}

func TestCheckpointEffectsBindingAbsentContextAndCapturePurity(t *testing.T) {
	f := checkpointEffectsBoundFixture(t)
	f.p.SetFragmentStepContext(FragmentStepContext{})
	if f.p.checkpointFragment != (checkpointFragmentBinding{}) {
		t.Fatal("ordinary clearing retained proof")
	}
	fresh := NewCheckpointContext(nil, f.p)
	if err := fresh.SetBindings(f.s, f.art, f.terrain, f.tick, f.authority); err != nil {
		t.Fatal("absent context rejected:", err)
	}
	f.p.fragmentStepping = true
	if _, err := f.p.CollectCheckpointReferences(fresh); err == nil || !strings.Contains(err.Error(), ".fragmentStepping") {
		t.Fatal("incoherent enabled state accepted")
	}
	f.p.SetFragmentStepContextWithCheckpointBinding(f.ctx, f.terrain, f.tick, f.authority)
	beforeContext, beforeProof := *f.c, f.p.checkpointFragment
	beforeService, beforePool := checkpointBoundEffectsBytes(t, f)
	for n := 0; n < 3; n++ {
		if err := f.c.SetBindings(f.s, f.art, f.terrain, f.tick, f.authority); err != nil {
			t.Fatal(err)
		}
		service, pool := checkpointBoundEffectsBytes(t, f)
		if !bytes.Equal(beforeService, service) || !bytes.Equal(beforePool, pool) || *f.c != beforeContext || f.p.checkpointFragment != beforeProof {
			t.Fatal("capture changed state")
		}
	}
	if allocations := testing.AllocsPerRun(20, func() {
		if err := f.c.SetBindings(f.s, f.art, f.terrain, f.tick, f.authority); err != nil {
			panic(err)
		}
		if _, err := f.s.CollectCheckpointReferences(f.c); err != nil {
			panic(err)
		}
		if _, err := f.p.CollectCheckpointReferences(f.c); err != nil {
			panic(err)
		}
	}); allocations != 0 {
		t.Fatalf("binding/collection allocations = %v", allocations)
	}
	// Proof words remain local metadata and never alter the direct summary.
	var before, after checkpoint.Summary
	if err := f.p.AppendCheckpointSummary(&before); err != nil {
		t.Fatal(err)
	}
	f.p.SetFragmentStepContext(f.ctx)
	if err := f.p.AppendCheckpointSummary(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("summary inspected proof")
	}
}

type checkpointGameplayImpact struct{ ground, water int }

func (s *checkpointGameplayImpact) GroundFragmentImpact(GroundFragmentImpact) { s.ground++ }
func (s *checkpointGameplayImpact) WaterFragmentImpact(WaterFragmentImpact)   { s.water++ }

func TestCheckpointEffectsBindingDoesNotChangeShatterOrCallbacks(t *testing.T) {
	type outcome struct {
		records                         []EffectRecord
		fragments                       []fragmentGeometry
		state                           uint32
		draws                           uint64
		heights, freezes, ground, water int
	}
	run := func(mode int) outcome {
		p := NewFixedEffectPool(1)
		art, terrain, authority := &content.SimArt{}, &world.Terrain{}, checkpoint.NewBindingAuthority()
		s := NewEffectServiceWithPool(1, p, art)
		stream := rng.SimulationFromState(7)
		heights, freezes := 0, 0
		sink := &checkpointGameplayImpact{}
		ctx := FragmentStepContext{TerrainHeight: func(numeric.Fixed, numeric.Fixed) numeric.Fixed { heights++; return 100 << 16 }, Impact: sink}
		if mode == 0 {
			s.SetFragmentStepContext(ctx)
		} else {
			s.SetFragmentStepContextWithCheckpointBinding(ctx, terrain, 9, authority)
		}
		if heights != 0 || sink.ground != 0 || sink.water != 0 {
			t.Fatal("installation invoked a port")
		}
		if mode != 0 {
			c := NewCheckpointContext(nil, p)
			tick := uint32(9)
			if mode == 2 {
				tick++
			}
			err := c.SetBindings(s, art, terrain, tick, authority)
			if (err != nil) != (mode == 2) {
				t.Fatalf("mode %d registration: %v", mode, err)
			}
		}
		if !s.AdmitShatter(FragmentRequest{ExplodeOnHit: true, Quads: []FragmentQuad{{}, {}}, Freeze: func(uint16, int, FragmentQuad) FrozenFragmentMaterial { freezes++; return FrozenFragmentMaterial{} }}, stream.Uint32n) {
			t.Fatal("shatter refused")
		}
		// Contact at zero vertical speed selects the actual impact-before-release
		// branch; the second quad was refused before its eight draws [04 R-COB-04 §3].
		p.records[0].VY = 0
		s.Advance(9, nil)
		if heights != 1 || freezes != 1 || sink.ground != 1 || stream.Draws() != 8 {
			t.Fatalf("height=%d freeze=%d impact=%d draws=%d", heights, freezes, sink.ground, stream.Draws())
		}
		return outcome{p.records, p.fragments, stream.State, stream.Draws(), heights, freezes, sink.ground, sink.water}
	}
	ordinary := run(0)
	for _, mode := range []int{1, 2} {
		if got := run(mode); !reflect.DeepEqual(got, ordinary) {
			t.Fatalf("binding mode %d changed gameplay", mode)
		}
	}
}
