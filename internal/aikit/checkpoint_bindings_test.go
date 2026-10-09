package aikit

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/ai"
	"github.com/nanolathe-gg/nanolathe/internal/construction"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

type checkpointHostBindingFixture struct {
	h *Host
	c *ai.CheckpointContext
	w ai.CheckpointModernPlanner
	a *checkpoint.BindingAuthority
}

func newCheckpointHostBindingFixture(t *testing.T, initialized bool) *checkpointHostBindingFixture {
	t.Helper()
	keys := &content.CheckpointKeys{}
	m := &ai.Manager{Controller: ai.ControllerModern, Planner: HostPlanner{}, Catalog: &content.Catalog{}, ConstructionRules: construction.StrictRules{}}
	if err := m.EnableCheckpointApplications(checkpointAttachIdentity(), keys); err != nil {
		t.Fatal(err)
	}
	h := NewHost(m, &countBrain{}, Persona{ThinkEvery: 10, Reaction: 3, APM: 60, Burst: 4, Attention: 2, Skill: 7, Ambition: 8})
	m.Ext = h
	if initialized {
		h.begin(0, units.NewSliced(4, nil))
		h.Join()
	}
	f := &checkpointHostBindingFixture{h: h,
		c: ai.NewCheckpointContext(units.NewCheckpointContext(keys), world.NewCheckpointContext(keys)),
		w: ai.NewCheckpointModernPlanner(HostPlanner{}), a: checkpoint.NewBindingAuthority()}
	if err := f.register(); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *checkpointHostBindingFixture) register() error {
	return f.c.SetModernBindings(f.h.m, HostPlanner{}, f.w, CheckpointControllerSource(), f.a)
}

func TestCheckpointHostBindingsOriginalRuleVector(t *testing.T) {
	f := newCheckpointHostBindingFixture(t, true)
	h := f.h
	h.m.ConstructionRules = &construction.ModernRules{}
	first, second := CheckpointControllerSource(), CheckpointControllerSource()
	if first != second || !first.Valid() {
		t.Fatal("controller source did not retain its sealed identity")
	}
	// Zero executor's existing 777 bytes, plus the present table's original
	// rule byte. Manager at 403; table at 768. No pointer identities enter it.
	wantExecutor := make([]byte, 778)
	wantExecutor[403], wantExecutor[768], wantExecutor[769] = 1, 1, 1
	gotExecutor := checkpointLeafBytes(t, func(e *checkpoint.Encoder) error { return h.ex.writeCheckpoint(e, f.c) })
	if !bytes.Equal(gotExecutor, wantExecutor) {
		t.Fatalf("executor vector\ngot  %x\nwant %x", gotExecutor, wantExecutor)
	}
	// Independently framed controller (72), persona (29) and executor (778).
	want := make([]byte, 72)
	hash := checkpointHostInitialHash()
	copy(want[8:40], hash[:])
	want[45], want[50], want[58], want[63] = 1, 1, 1, 1
	want = append(want, checkpointDecode(t, "3c0000000800000002000000040000000003000000070000000a000000")...)
	want = append(want, wantExecutor...)
	before, err := h.checkpointHistory.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		got := checkpointLeafBytes(t, func(e *checkpoint.Encoder) error {
			return CheckpointControllerSource().WriteCheckpoint(h.m, e, f.c)
		})
		if !bytes.Equal(got, want) {
			t.Fatalf("Host vector\ngot  %x\nwant %x", got, want)
		}
	}
	if after, err := h.checkpointHistory.Snapshot(); err != nil || after != before {
		t.Fatal("capture changed history")
	}
	if n := testing.AllocsPerRun(100, func() {
		if err := h.ValidateCheckpointBindings(h.m, f.c); err != nil {
			panic(err)
		}
	}); n != 0 {
		t.Fatalf("binding validation allocations = %v", n)
	}
}

func TestCheckpointHostBindingsRefuseChangedAliasesBeforeBytes(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*checkpointHostBindingFixture)
	}{
		{"constructor self", func(f *checkpointHostBindingFixture) { f.h.checkpointOwner.self = &Host{} }},
		{"constructor manager", func(f *checkpointHostBindingFixture) { f.h.checkpointOwner.manager = &ai.Manager{} }},
		{"manager", func(f *checkpointHostBindingFixture) { copied := *f.h.m; copied.Ext = f.h; f.h.m = &copied }},
		{"nil Ext", func(f *checkpointHostBindingFixture) { f.h.m.Ext = nil }},
		{"typed nil Ext", func(f *checkpointHostBindingFixture) { f.h.m.Ext = (*Host)(nil) }},
		{"other Ext", func(f *checkpointHostBindingFixture) { f.h.m.Ext = NewHost(f.h.m, &countBrain{}, PersonaHard) }},
		{"noncomparable Ext", func(f *checkpointHostBindingFixture) { f.h.m.Ext = []int{1} }},
		{"copied Host", func(f *checkpointHostBindingFixture) { copied := *f.h; f.h = &copied; f.h.m.Ext = f.h }},
		{"initialized reset", func(f *checkpointHostBindingFixture) { f.h.inited = false }},
		{"missing executor proof", func(f *checkpointHostBindingFixture) { f.h.checkpointExecutor = checkpointHostExecutor{} }},
		{"executor pointer", func(f *checkpointHostBindingFixture) { copied := f.h.ex; f.h.checkpointExecutor.executor = &copied }},
		{"executor manager", func(f *checkpointHostBindingFixture) { f.h.ex.m = &ai.Manager{} }},
		{"nil executor manager", func(f *checkpointHostBindingFixture) { f.h.ex.m = nil }},
		{"nil table", func(f *checkpointHostBindingFixture) { f.h.ex.table = nil }},
		{"replacement table", func(f *checkpointHostBindingFixture) {
			f.h.ex.table = BuildTable(f.h.m.Catalog, construction.StrictRules{})
		}},
		{"copied table", func(f *checkpointHostBindingFixture) { copied := *f.h.ex.table; f.h.ex.table = &copied }},
		{"catalog", func(f *checkpointHostBindingFixture) { f.h.m.Catalog = &content.Catalog{} }},
		{"terrain", func(f *checkpointHostBindingFixture) { f.h.m.Terrain = &world.Terrain{} }},
		{"world terrain", func(f *checkpointHostBindingFixture) { f.c.World.Terrain = &world.Terrain{} }},
		{"table values", func(f *checkpointHostBindingFixture) { f.h.ex.table.Units = []*UnitInfo{{}} }},
		{"table membership", func(f *checkpointHostBindingFixture) { f.h.ex.table.byKey["foreign"] = &UnitInfo{} }},
		{"active command", func(f *checkpointHostBindingFixture) { f.h.ex.checkpointApplication = &checkpointCommand{} }},
		{"executor serial", func(f *checkpointHostBindingFixture) { f.h.ex.checkpointBatchSerial = 1 }},
		{"history", func(f *checkpointHostBindingFixture) {
			f.h.checkpointHistory, _ = ai.NewApplicationHistory(checkpoint.Identity{}, 0, 2)
		}},
		{"failed history", func(f *checkpointHostBindingFixture) { f.h.checkpointHistory.Fail(errors.New("fixture failure")) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newCheckpointHostBindingFixture(t, true)
			tc.edit(f)
			if err := f.h.ValidateCheckpointBindings(f.h.m, f.c); err == nil {
				t.Fatal("changed binding admitted")
			}
			if err := f.register(); err == nil {
				t.Fatal("repeat repaired binding")
			}
			var out bytes.Buffer
			if err := f.h.WriteControllerCheckpoint(checkpoint.NewEncoder(&out), f.c); err == nil || out.Len() != 0 {
				t.Fatalf("Host refusal emitted bytes: %v, %x", err, out.Bytes())
			}
		})
	}
}

func TestCheckpointHostBindingsBeforeInitialization(t *testing.T) {
	for _, edit := range []func(*Host){
		func(h *Host) { h.inited = true },
		func(h *Host) { h.ex.m = h.m },
		func(h *Host) { h.ex.table = &Table{} },
		func(h *Host) { h.checkpointExecutor.manager = h.m },
	} {
		f := newCheckpointHostBindingFixture(t, false)
		edit(f.h)
		if err := f.h.ValidateCheckpointBindings(f.h.m, f.c); err == nil {
			t.Fatal("premature/missing executor aliases admitted")
		}
	}
	f := newCheckpointHostBindingFixture(t, false)
	if err := f.h.ValidateCheckpointBindings(nil, f.c); err == nil {
		t.Fatal("nil expected manager admitted")
	}
	if err := f.h.ValidateCheckpointBindings(f.h.m, nil); err == nil {
		t.Fatal("nil context admitted")
	}
	if err := (*Host)(nil).ValidateCheckpointBindings(f.h.m, f.c); err == nil {
		t.Fatal("nil Host admitted")
	}
	if err := f.h.ValidateCheckpointBindings(&ai.Manager{}, f.c); err == nil {
		t.Fatal("wrong expected manager admitted")
	}
}

func TestCheckpointHostBindingsRejectCopiedExecutorAndUnregisteredOwner(t *testing.T) {
	f := newCheckpointHostBindingFixture(t, true)
	copied := f.h.ex
	var out bytes.Buffer
	if err := copied.writeCheckpoint(checkpoint.NewEncoder(&out), f.c); err == nil || out.Len() != 0 {
		t.Fatal("copied executor wrote payload")
	}
	c := ai.NewCheckpointContext(f.c.Units, f.c.World)
	if err := f.h.WriteControllerCheckpoint(checkpoint.NewEncoder(&out), c); err == nil || out.Len() != 0 {
		t.Fatal("unregistered Host wrote payload")
	}
	// Implementing the provider through an embedding is not the sealed *Host.
	for _, ext := range []any{&RetailTimer{}, struct{ *Host }{f.h}, []int{1}} {
		f.h.m.Ext = ext
		if err := CheckpointControllerSource().WriteCheckpoint(f.h.m, checkpoint.NewEncoder(&out), f.c); err == nil || out.Len() != 0 {
			t.Fatalf("foreign %T wrote payload", ext)
		}
	}
}

func TestCheckpointHostBindingsAttachmentRequiresConstructor(t *testing.T) {
	for _, makeHost := range []func(*ai.Manager) *Host{
		func(m *ai.Manager) *Host { return &Host{m: m} },
		func(m *ai.Manager) *Host {
			original := NewHost(m, &countBrain{}, PersonaHard)
			copied := *original
			return &copied
		},
		func(m *ai.Manager) *Host { h := NewHost(&ai.Manager{}, &countBrain{}, PersonaHard); h.m = m; return h },
	} {
		m := &ai.Manager{Controller: ai.ControllerModern}
		h := makeHost(m)
		m.Ext = h
		if err := h.EnableCheckpointApplications(checkpoint.Identity{}, &content.CheckpointKeys{}); err == nil || h.checkpointHistory != nil || m.CheckpointApplicationHistory() != nil {
			t.Fatal("unconstructed/copied Host attached history")
		}
	}
	// Proof exists before diagnostics are enabled; the ordinary entry route
	// remains attachable after actual initialization.
	m := &ai.Manager{Controller: ai.ControllerModern, Catalog: &content.Catalog{}}
	h := NewHost(m, &countBrain{}, PersonaHard)
	m.Ext = h
	h.begin(1, units.NewSliced(4, nil))
	h.Join()
	if h.checkpointOwner.self != h || h.checkpointExecutor.executor != &h.ex || h.checkpointHistory != nil {
		t.Fatal("disabled construction did not retain provenance")
	}
	if err := h.EnableCheckpointApplications(checkpoint.Identity{}, &content.CheckpointKeys{}); err != nil {
		t.Fatal(err)
	}
}

func TestCheckpointHostBindingsTableSnapshotAndSummaryBoundary(t *testing.T) {
	cat, keys := checkpointTableCatalog(t)
	m := &ai.Manager{Controller: ai.ControllerModern, Planner: HostPlanner{}, Catalog: cat, Terrain: sharedTestTerrain(), ConstructionRules: construction.StrictRules{}}
	if err := m.EnableCheckpointApplications(checkpoint.Identity{}, keys); err != nil {
		t.Fatal(err)
	}
	h := NewHost(m, &countBrain{}, PersonaHard)
	m.Ext = h
	h.begin(1, units.NewSliced(4, nil))
	h.Join()
	c := ai.NewCheckpointContext(units.NewCheckpointContext(keys), world.NewCheckpointContext(keys))
	c.World.Terrain = m.Terrain
	if err := c.SetModernBindings(m, HostPlanner{}, ai.NewCheckpointModernPlanner(HostPlanner{}), CheckpointControllerSource(), checkpoint.NewBindingAuthority()); err != nil {
		t.Fatal(err)
	}
	if err := h.WriteControllerCheckpoint(checkpoint.NewEncoder(io.Discard), c); err != nil {
		t.Fatal(err)
	}
	var before, after checkpoint.Summary
	if err := h.AppendControllerCheckpointSummary(&before); err != nil {
		t.Fatal(err)
	}
	h.ex.table.Units[0].Builds = nil
	var out bytes.Buffer
	if err := h.WriteControllerCheckpoint(checkpoint.NewEncoder(&out), c); err == nil || out.Len() != 0 {
		t.Fatal("mutated derived table emitted controller prefix")
	}
	if err := CheckpointControllerSource().AppendSummary(m, &after); err != nil || after != before {
		t.Fatal("selected summary traversed table", err)
	}
}
