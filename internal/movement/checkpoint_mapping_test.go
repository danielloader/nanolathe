package movement

import (
	"bytes"
	"reflect"
	"slices"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/path"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

func movementMappingContext(t *testing.T, source *orders.WorldQueryAdapter, a *checkpoint.BindingAuthority) *CheckpointContext {
	t.Helper()
	c := movementCheckpointContext()
	if err := c.Orders.SetBindings(&orders.QueueBinding{World: source}, nil, nil, a); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestMovementCheckpointMappingVectorsAndPureCapture(t *testing.T) {
	a := checkpoint.NewBindingAuthority()
	source := orders.NewWorldQueryAdapterWithCheckpointBinding(orders.WorldQueryAdapterConfig{MappingWord: func(int32, int32) (uint16, bool) { panic("capture called mapping") }}, a)
	c := movementMappingContext(t, source, a)
	s := &System{layerRegistry: NewClassLayers(nil, nil, nil, nil)}
	r := s.layerRegistry
	l := &ClassLayer{}
	r.byName["a"], r.names = l, []string{"a"}
	r.BindMappingWordWithCheckpointBinding(source.CheckpointMappingWord())
	l.watermark, l.cells, l.commits = 7, []uint32{4, 5}, []commitWord{{set: true, tick: 6}, {}}
	l.stampScratch, l.stampRows, l.restampTiers = []uint8{8}, []uint8{9}, []uint8{10}
	collectMovementCheckpoint(t, s, c)
	registry := movementCheckpointWrite(t, func(e *checkpoint.Encoder) { writeMovementLayers(e, c, s, r, "registry") })
	wantRegistry := movementCheckpointVector(t, uint8(0), uint32(1), uint32(1), uint8('a'), uint16(5), uint32(1),
		uint8(0), uint8(1), uint8(0), uint32(1), uint32(1), uint8('a'), [3]uint8{})
	if !bytes.Equal(registry, wantRegistry) {
		t.Fatalf("mapping registry\ngot %x\nwant %x", registry, wantRegistry)
	}
	layer := movementCheckpointWrite(t, func(e *checkpoint.Encoder) { writeMovementLayer(e, c, s, l, "layer") })
	wantLayer := movementCheckpointVector(t, uint8(0), int32(0), [16]uint8{}, uint8(0), int32(0), uint32(2), uint32(4), uint32(5),
		uint32(2), uint8(1), uint32(6), uint8(0), uint32(0), uint8(1), uint8(0), uint32(7))
	if !bytes.Equal(layer, wantLayer) {
		t.Fatalf("mapping layer\ngot %x\nwant %x", layer, wantLayer)
	}
	before := *l
	before.cells, before.commits, before.stampScratch = slices.Clone(l.cells), slices.Clone(l.commits), slices.Clone(l.stampScratch)
	full := movementCheckpointWrite(t, func(e *checkpoint.Encoder) { _ = s.WriteCheckpoint(e, c) })
	for range 3 {
		collectMovementCheckpoint(t, s, c)
		if !bytes.Equal(full, movementCheckpointWrite(t, func(e *checkpoint.Encoder) { _ = s.WriteCheckpoint(e, c) })) {
			t.Fatal("capture changed retained bytes")
		}
	}
	if l.fullStamps != before.fullStamps || l.watermark != before.watermark || !slices.Equal(l.cells, before.cells) ||
		!slices.Equal(l.commits, before.commits) || !slices.Equal(l.stampScratch, before.stampScratch) || len(r.names) != 1 {
		t.Fatal("capture restamped or grew runtime state")
	}
	if n := testing.AllocsPerRun(50, func() {
		if err := validateCheckpointMapping(c, l.mapping, l.checkpointMapping); err != nil {
			t.Fatal(err)
		}
	}); n != 0 {
		t.Fatalf("mapping validation allocated %g times", n)
	}
}

func TestMovementCheckpointMappingInheritanceRebindAndNil(t *testing.T) {
	a := checkpoint.NewBindingAuthority()
	source := orders.NewWorldQueryAdapterWithCheckpointBinding(orders.WorldQueryAdapterConfig{MappingWord: func(int32, int32) (uint16, bool) { return 0x1234, false }}, a)
	c := movementMappingContext(t, source, a)
	r := NewClassLayers(layerTerrain(1, 1, 0), nil, nil, nil)
	first := r.For("first", Profile{})
	r.BindMappingWordWithCheckpointBinding(source.CheckpointMappingWord())
	second := r.For("second", Profile{})
	oldValue := r.checkpointMapping
	// Source-slot replacement does not rewrite the registry or either copy.
	source.SetMappingWord(func(int32, int32) (uint16, bool) { return 0xFFFF, true })
	third := r.For("third", Profile{})
	for _, l := range []*ClassLayer{first, second, third} {
		if err := validateCheckpointMapping(c, l.mapping, l.checkpointMapping); err != nil {
			t.Fatal("inherited copy followed current source slot", err)
		}
		if word, ok := l.mapping(0, 0); word != 0x1234 || ok || l.fullStamps != 1 {
			t.Fatal("source edit changed retained reader or creation stamp count")
		}
	}
	r.BindMappingWord(nil)
	r.BindMappingWordWithCheckpointBinding(orders.CheckpointMappingWord{})
	if err := validateCheckpointMapping(c, r.mapping, r.checkpointMapping); err != nil {
		t.Fatal("nil bind cleared registry proof", err)
	}
	for _, l := range []*ClassLayer{first, second, third} {
		if err := validateCheckpointMapping(c, l.mapping, l.checkpointMapping); err != nil {
			t.Fatal("nil bind cleared layer proof", err)
		}
	}
	// Only allocation-order entries still found in byName are affected. A
	// detached layer retains its original reader and proof for old searches.
	delete(r.byName, "first")
	source.SetMappingWordWithCheckpointBinding(func(int32, int32) (uint16, bool) { return 0x8001, true }, a)
	r.BindMappingWordWithCheckpointBinding(source.CheckpointMappingWord())
	if word, _ := first.mapping(0, 0); word != 0x1234 {
		t.Fatal("registry rebinding reached a detached layer")
	}
	for _, l := range []*ClassLayer{second, third, r.For("late", Profile{})} {
		if word, ok := l.mapping(0, 0); word != 0x8001 || !ok || validateCheckpointMapping(c, l.mapping, l.checkpointMapping) != nil || l.fullStamps != 1 {
			t.Fatal("registry replacement or late inheritance lost copied reader")
		}
	}
	r.BindMappingWord(oldValue.Reader())
	if validateCheckpointMapping(c, r.mapping, r.checkpointMapping) == nil || validateCheckpointMapping(c, second.mapping, second.checkpointMapping) == nil {
		t.Fatal("ordinary function-only reinstall transferred proof")
	}
	if err := validateCheckpointMapping(c, first.mapping, first.checkpointMapping); err != nil {
		t.Fatal("ordinary replacement cleared an unaffected layer's proof", err)
	}
	var absent *ClassLayers
	absent.BindMappingWord(oldValue.Reader())
	absent.BindMappingWordWithCheckpointBinding(oldValue)
}

func TestMovementCheckpointMappingRefusals(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*ClassLayers, *ClassLayer, *orders.WorldQueryAdapter, *checkpoint.BindingAuthority)
	}{
		{"ordinary registry", func(r *ClassLayers, _ *ClassLayer, src *orders.WorldQueryAdapter, _ *checkpoint.BindingAuthority) {
			r.BindMappingWord(src.MappingWordHook())
		}},
		{"ordinary copied value", func(r *ClassLayers, _ *ClassLayer, src *orders.WorldQueryAdapter, _ *checkpoint.BindingAuthority) {
			src.SetMappingWord(src.MappingWordHook())
			r.BindMappingWordWithCheckpointBinding(src.CheckpointMappingWord())
		}},
		{"copied adapter", func(r *ClassLayers, _ *ClassLayer, src *orders.WorldQueryAdapter, _ *checkpoint.BindingAuthority) {
			copied := *src
			r.BindMappingWordWithCheckpointBinding(copied.CheckpointMappingWord())
		}},
		{"foreign source", func(r *ClassLayers, _ *ClassLayer, src *orders.WorldQueryAdapter, a *checkpoint.BindingAuthority) {
			other := orders.NewWorldQueryAdapterWithCheckpointBinding(orders.WorldQueryAdapterConfig{MappingWord: src.MappingWordHook()}, a)
			r.BindMappingWordWithCheckpointBinding(other.CheckpointMappingWord())
		}},
		{"foreign authority", func(r *ClassLayers, _ *ClassLayer, src *orders.WorldQueryAdapter, _ *checkpoint.BindingAuthority) {
			src.SetMappingWordWithCheckpointBinding(src.MappingWordHook(), checkpoint.NewBindingAuthority())
			r.BindMappingWordWithCheckpointBinding(src.CheckpointMappingWord())
		}},
		{"registry reader absent", func(r *ClassLayers, _ *ClassLayer, _ *orders.WorldQueryAdapter, _ *checkpoint.BindingAuthority) {
			r.mapping = nil
		}},
		{"layer reader absent", func(_ *ClassLayers, l *ClassLayer, _ *orders.WorldQueryAdapter, _ *checkpoint.BindingAuthority) {
			l.mapping = nil
		}},
		{"layer proof absent", func(_ *ClassLayers, l *ClassLayer, _ *orders.WorldQueryAdapter, _ *checkpoint.BindingAuthority) {
			l.checkpointMapping = orders.CheckpointMappingWord{}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := checkpoint.NewBindingAuthority()
			source := orders.NewWorldQueryAdapterWithCheckpointBinding(orders.WorldQueryAdapterConfig{MappingWord: func(int32, int32) (uint16, bool) { panic("capture read mapping") }}, a)
			c := movementMappingContext(t, source, a)
			s := &System{layerRegistry: NewClassLayers(nil, nil, nil, nil)}
			r := s.layerRegistry
			l := &ClassLayer{}
			r.byName["a"], r.names = l, []string{"a"}
			r.BindMappingWordWithCheckpointBinding(source.CheckpointMappingWord())
			collectMovementCheckpoint(t, s, c)
			test.edit(r, l, source, a)
			if _, err := s.CollectCheckpointReferences(c); err == nil {
				t.Fatal("collector accepted invalid mapping")
			}
			var out bytes.Buffer
			if err := s.WriteCheckpoint(checkpoint.NewEncoder(&out), c); err == nil {
				t.Fatal("writer accepted invalid mapping")
			}
			// The record writer must reject before emitting that record.
			out.Reset()
			e := checkpoint.NewEncoder(&out)
			if validateMovementLayers(s, c, r) != nil {
				writeMovementLayers(e, c, s, r, "registry")
			} else {
				writeMovementLayer(e, c, s, l, "layer")
			}
			if e.Err() == nil || out.Len() != 0 {
				t.Fatal("record writer emitted unverified mapping state")
			}
		})
	}
}

// Binding transfers no terrain values and performs no stamp or mapping read.
// The subsequent consumer must keep its original tile/owner classifier
// answers and read order [04 R-PATH-01 §2][04 R-PATH-01 §14].
func TestMovementCheckpointMappingClassifierUnchanged(t *testing.T) {
	var baseline []uint8
	var baselineReads [][2]int32
	for _, admitted := range []bool{false, true} {
		var reads [][2]int32
		source := orders.NewWorldQueryAdapterWithCheckpointBinding(orders.WorldQueryAdapterConfig{MappingWord: func(x, z int32) (uint16, bool) {
			reads = append(reads, [2]int32{x, z})
			if x == 4 && z == 4 {
				return 1, true
			}
			return 0, true
		}}, checkpoint.NewBindingAuthority())
		r := NewClassLayers(layerTerrain(32, 32, 20), nil, nil, nil)
		if admitted {
			r.BindMappingWordWithCheckpointBinding(source.CheckpointMappingWord())
		} else {
			r.BindMappingWord(source.MappingWordHook())
		}
		l := r.For("class", kbotsSS2)
		if len(reads) != 0 || l.fullStamps != 1 {
			t.Fatal("mapping installation changed initial stamp behavior")
		}
		l.setValue(8, 8, LayerSteep)
		got := []uint8{l.Passable(-1, 5, 2, 2, 0), l.Passable(5, 5, 2, 2, 0), l.Passable(8, 8, 2, 2, 0), l.Passable(8, 8, 2, 2, 1), l.Passable(7, 8, 4, 2, 0)}
		if !slices.Equal(got, []uint8{LayerBlocked, LayerUnmapped, LayerSteep, LayerUnmapped, LayerClear}) {
			t.Fatalf("classifier answers = %v", got)
		}
		if !admitted {
			baseline, baselineReads = got, reads
		} else if !slices.Equal(got, baseline) || !reflect.DeepEqual(reads, baselineReads) {
			t.Fatal("diagnostic binding changed mapping consumption")
		}
	}
}

func TestMovementCheckpointMappingRequesterAndSearch(t *testing.T) {
	s, u, q, _, req := newGroundPathStatusFixture(t, path.Cell{X: 2, Z: 2}, path.Cell{X: 18, Z: 18})
	a := checkpoint.NewBindingAuthority()
	source := orders.NewWorldQueryAdapterWithCheckpointBinding(orders.WorldQueryAdapterConfig{MappingWord: func(int32, int32) (uint16, bool) { return 0x3FF, true }}, a)
	b := q.Binding()
	if b == nil {
		b = &orders.QueueBinding{}
		q.SetBinding(b)
	}
	b.World = source
	c := movementMappingContext(t, source, a)
	if err := c.Orders.ValidateMappingWord(s.checkpointMappingWordSource(u.Handle)); err != nil {
		t.Fatal(err)
	}
	s.searchFunc(req, 65536, 0)
	r := s.layerRegistry
	l := r.Existing(s.classKeyFor(u.Handle))
	if r == nil || l == nil || validateCheckpointMapping(c, r.mapping, r.checkpointMapping) != nil || validateCheckpointMapping(c, l.mapping, l.checkpointMapping) != nil {
		t.Fatal("path admission did not retain mapping source")
	}
	// A diagnostic reader performs its existing retained installation as well.
	r.BindMappingWord(source.MappingWordHook())
	if _, _, ok := s.LabGround(u, 2, 2, 3, 3); !ok || validateCheckpointMapping(c, r.mapping, r.checkpointMapping) != nil {
		t.Fatal("laboratory path lost copied source")
	}
	u.Orders = nil
	if s.checkpointMappingWordSource(u.Handle).Reader() != nil || u.Orders != nil {
		t.Fatal("source lookup created a queue")
	}
	for _, absent := range []*System{nil, {}} {
		if absent.checkpointMappingWordSource(u.Handle).Reader() != nil {
			t.Fatal("absent world produced a mapping source")
		}
	}
	if s.checkpointMappingWordSource(0).Reader() != nil {
		t.Fatal("null requester produced a source")
	}
	q = orders.QueueForUnit(u)
	q.SetBinding(&orders.QueueBinding{})
	if s.checkpointMappingWordSource(u.Handle).Reader() != nil {
		t.Fatal("absent adapter produced a source")
	}
	q.SetBinding(&orders.QueueBinding{World: &orders.WorldQueryAdapter{}})
	if s.checkpointMappingWordSource(u.Handle).Reader() != nil {
		t.Fatal("absent callback produced a source")
	}
}
