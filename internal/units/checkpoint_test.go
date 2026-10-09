package units

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/cob"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/sim/rng"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// checkpointFixture admits two tiny authored definitions, scripts and a model
// through the real content freezer. No retail bytes or mutable source binding
// is required by the unit world. Other section-owner tests may reuse it.
func checkpointFixture(t *testing.T) (*World, *content.SimulationInputs) {
	t.Helper()
	root := t.TempDir()
	write := func(logical string, data []byte) {
		t.Helper()
		name := filepath.Join(root, filepath.FromSlash(logical))
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	model, err := formats.EncodeThreeDO(&formats.ThreeDO{Root: 0, Objects: []formats.ThreeDOObject{{
		Version: 1, Name: "base", Selection: -1, Parent: -1, FirstChild: -1, NextSibling: -1,
		Vertices: []formats.ThreeDOVertex{{}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	write("objects3d/fixture.3do", model)
	cat := stampedCatalog(t)
	for _, def := range cat.UnitRecords() {
		write("scripts/"+def.UnitName+".cob", unitCreateCOB())
		def.Script, err = cob.Load(unitCreateCOB())
		if err != nil {
			t.Fatal(err)
		}
		def.ObjectName, def.Limit = "fixture", -1
	}
	cat.Weapons = map[string]*content.WeaponDef{
		"pulse": {DefinitionHeader: content.DefinitionHeader{CanonicalKey: "pulse"}, ID: 1},
	}
	fs := vfs.New()
	if err := fs.MountDirectory(root, 10); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fs.Close() })
	sources, err := content.CaptureSimulationSources(fs, "")
	if err != nil {
		t.Fatal(err)
	}
	inputs, err := content.FreezeSimulationInputs(sources, content.SimulationInputRequest{Catalog: cat})
	if err != nil {
		t.Fatal(err)
	}
	return NewSliced(2, cat), inputs
}

func checkpointCreate(t *testing.T, w *World, name string, owner uint8) *Unit {
	t.Helper()
	h, err := w.Create(w.catalog.Units[name], owner, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	return w.Unit(h)
}

func checkpointUnitCapture(w *World, inputs *content.SimulationInputs, extra ...*Unit) (checkpoint.Digests, []byte, *CheckpointContext, error) {
	keys, err := inputs.CheckpointKeys()
	if err != nil {
		return checkpoint.Digests{}, nil, nil, err
	}
	c := NewCheckpointContext(keys)
	if _, err := w.CollectCheckpointReferences(c); err != nil {
		return checkpoint.Digests{}, nil, c, err
	}
	for _, u := range extra {
		if _, err := c.Allocations.Add(u); err != nil {
			return checkpoint.Digests{}, nil, c, err
		}
	}
	if _, err := w.CollectCheckpointReferences(c); err != nil {
		return checkpoint.Digests{}, nil, c, err
	}
	var out bytes.Buffer
	capture, err := checkpoint.NewCapture(checkpoint.Identity{Content: checkpoint.Digest(inputs.Digest())}, &out)
	if err != nil {
		return checkpoint.Digests{}, nil, c, err
	}
	for owner := checkpoint.OwnerRuntime; owner <= checkpoint.OwnerComputersScenario; owner++ {
		e, err := capture.Section(owner, owner == checkpoint.OwnerUnits)
		if err != nil {
			return checkpoint.Digests{}, nil, c, err
		}
		if owner == checkpoint.OwnerUnits {
			if err := w.WriteCheckpoint(e, c); err != nil {
				return checkpoint.Digests{}, nil, c, err
			}
		}
	}
	digests, err := capture.Finish()
	return digests, out.Bytes(), c, err
}

func mustCheckpointUnitCapture(t *testing.T, w *World, inputs *content.SimulationInputs, extra ...*Unit) (checkpoint.Digests, []byte, *CheckpointContext) {
	t.Helper()
	digests, data, c, err := checkpointUnitCapture(w, inputs, extra...)
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint.Digest(sha256.Sum256(data)) != digests.Full {
		t.Fatal("captured bytes and streaming digest disagree")
	}
	return digests, data, c
}

func TestUnitCheckpointRetainedFieldsAndExclusions(t *testing.T) {
	w, inputs := checkpointFixture(t)
	u := checkpointCreate(t, w, "armdef", 0)
	u.Attachment.Cargo = []pool.Handle{17, 19}
	u.Slots[1].Weapon = w.catalog.Weapons["pulse"]
	before, _, _ := mustCheckpointUnitCapture(t, w, inputs)
	original := *u
	for _, test := range []struct {
		name string
		edit func()
	}{
		{"health", func() { u.Health-- }},
		{"remaining signed zero", func() { u.Remaining = math.Float32frombits(0x80000000) }},
		{"metal signed zero", func() { u.SpotMetal = math.Float32frombits(0x80000000) }},
		{"death observer latch", func() { u.deathExtraHookFired = true }},
		{"restored move mode", func() { u.RestoredMoveMode = !u.RestoredMoveMode }},
		{"engagement raw handle", func() { u.EngagementTarget = 60000 }},
		{"move velocity", func() { u.Move.VelZ = -1 }},
		{"cargo order", func() { u.Attachment.Cargo = []pool.Handle{19, 17} }},
		{"reload", func() { u.Slots[1].Reload = -1 }},
		{"aim word", func() { u.Slots[1].Aim.RestoreReadyWord(7); u.Slots[1].Aim.Ready = false }},
		{"desired aim", func() { u.Slots[1].DesiredPitch = 11 }},
		{"target raw handle", func() { u.Slots[1].Target.Unit = 60000 }},
		{"weapon presence", func() { u.Slots[1].Weapon = nil }},
		{"placement", func() { u.PlacementIdent = "first" }},
		{"footprint", func() { u.FootprintSizeZ++ }},
		{"reveal", func() { u.RevealDeadline++ }},
		{"physical piece flags", func() { u.RenderPieceFlags = []uint8{0, 1} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			*u = original
			test.edit()
			after, _, _ := mustCheckpointUnitCapture(t, w, inputs)
			if after.Full == before.Full || after.Owners[1] == before.Owners[1] {
				t.Fatal("retained mutation did not change full and units digests")
			}
		})
	}
	*u = original
	u.LOSByte, u.BlinkSuppress, u.RestoredAIGroup = 17, -1, 42
	u.Move.PendingHeading, u.Move.PendingSpeed = 123, 456
	u.Slots[0].SavedTargetLow, u.Slots[0].SavedTargetHigh = 1, 2
	w.iterHint, w.iterSlicedHint, w.nextDefID = 7, 19, 500
	w.deathDispatches = 42
	w.claimedDefIDs = []uint16{500, 499}
	w.readinessObserver = func(pool.UnitRef, bool) { t.Fatal("checkpoint invoked observation callback") }
	// The scripts/order owners account for these. Section 2 must not duplicate
	// their mutable bodies or reject an opaque order before its owning collector.
	u.Orders = "unrecognized order, inspected by the orders owner"
	u.Script.Threads[0].Stack[31] = 17
	after, _, _ := mustCheckpointUnitCapture(t, w, inputs)
	if after != before {
		t.Fatal("excluded or separately owned state changed the units digest")
	}
}

func TestUnitCheckpointResidualAndRetiredAllocation(t *testing.T) {
	w, inputs := checkpointFixture(t)
	old := checkpointCreate(t, w, "armdef", 0)
	old.Kills, old.Remaining = 13, 0.5
	w.FreeImmediate(old.Handle)
	residual := w.RawUnitRecord(old.Handle)
	if residual == old || residual == nil {
		t.Fatal("expected separate raw residual")
	}
	before, _, _ := mustCheckpointUnitCapture(t, w, inputs)
	residual.Kills++
	after, _, _ := mustCheckpointUnitCapture(t, w, inputs)
	if before.Owners[1] == after.Owners[1] {
		t.Fatal("freed raw-slot kill residual was omitted")
	}
	residual.Health = 99 // no raw reader retains this field
	excluded, _, _ := mustCheckpointUnitCapture(t, w, inputs)
	if excluded != after {
		t.Fatal("residual encoded beyond its four retained fields")
	}
	current := checkpointCreate(t, w, "cordef", 0)
	if current.Handle != old.Handle || current.AllocationSerial == old.AllocationSerial {
		t.Fatal("expected slot reuse with a new allocation identity")
	}
	withOld, _, c := mustCheckpointUnitCapture(t, w, inputs, old)
	allocations := c.Allocations.Values()
	if len(allocations) != 2 || allocations[0] != current || allocations[1] != old || len(c.VMs.Values()) != 2 {
		t.Fatal("retired allocation or its VM redirected to the current slot")
	}
	old.Health--
	changed, _, _ := mustCheckpointUnitCapture(t, w, inputs, old)
	if changed.Owners[1] == withOld.Owners[1] {
		t.Fatal("reachable retired allocation body omitted")
	}
	if added, err := w.CollectCheckpointReferences(c); err != nil || added != 0 {
		t.Fatalf("repeat graph collection = %d, %v", added, err)
	}
}

func TestUnitCheckpointReadOnlyAndAddressIndependent(t *testing.T) {
	w, inputs := checkpointFixture(t)
	first := checkpointCreate(t, w, "cordef", 0)
	second := checkpointCreate(t, w, "armdef", 0)
	unitBefore, worldBefore := *first, *w
	before, beforeBytes, _ := mustCheckpointUnitCapture(t, w, inputs)
	after, afterBytes, _ := mustCheckpointUnitCapture(t, w, inputs)
	if before != after || !bytes.Equal(beforeBytes, afterBytes) || !reflect.DeepEqual(*first, unitBefore) || !reflect.DeepEqual(*w, worldBefore) {
		t.Fatal("capture changed world state or repeated bytes")
	}
	// Relocate the allocation objects and reverse definition-cache insertion.
	firstCopy, secondCopy := *first, *second
	w.units[first.Handle], w.rawUnits[first.Handle] = &firstCopy, &firstCopy
	w.units[second.Handle], w.rawUnits[second.Handle] = &secondCopy, &secondCopy
	w.defMap = make(map[*content.UnitDef]uint16)
	w.defMap[second.Def] = uint16(second.Def.UnitDefID)
	w.defMap[first.Def] = uint16(first.Def.UnitDefID)
	moved, _, _ := mustCheckpointUnitCapture(t, w, inputs)
	if moved != before {
		t.Fatal("allocation addresses or definition-map insertion order entered the digest")
	}
	w.createdCounters[0]++
	counts, _, _ := mustCheckpointUnitCapture(t, w, inputs)
	if counts.Owners[1] == moved.Owners[1] {
		t.Fatal("creation accounting omitted")
	}
}

func TestUnitCheckpointRefusesUnsupportedAndTransientState(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*World, *Unit)
		path string
	}{
		{"unfinalized catalog", func(w *World, _ *Unit) { w.catalog.Hash = "" }, "finalized"},
		{"fixture catalog absent", func(w *World, _ *Unit) { w.catalog = nil }, "catalog"},
		{"pending serial", func(w *World, _ *Unit) { w.pendingAllocationSerials = 1 }, "pendingAllocationSerials"},
		{"incorrect definition identity", func(w *World, u *Unit) { w.defMap[u.Def]++ }, "defMap"},
		{"foreign definition map", func(w *World, u *Unit) { copy := *u.Def; w.defMap[&copy] = 5 }, "defMap"},
		{"unadmitted definition", func(_ *World, u *Unit) { copy := *u.Def; u.Def = &copy }, "slots"},
		{"uncommitted serial", func(_ *World, u *Unit) { u.AllocationSerial = 0 }, "AllocationSerial"},
		{"future serial", func(w *World, u *Unit) { u.AllocationSerial = w.lastAllocationSerial + 1 }, "AllocationSerial"},
		{"raw alias", func(w *World, u *Unit) { copy := *u; w.rawUnits[u.Handle] = &copy }, "slots"},
		{"script alias", func(_ *World, u *Unit) { u.Script = cob.NewVM(u.Def.Script) }, "ScriptState.VM"},
		{"bridge alias", func(_ *World, u *Unit) { u.ScriptState.Bridge = cob.NewCallbackBridge(cob.NewVM(u.Def.Script)) }, "Bridge"},
		{"binding alias", func(_ *World, u *Unit) { u.ScriptState.Binding = &cob.Binding{} }, "Binding"},
		{"creation closure", func(w *World, _ *Unit) { w.SetCreateHook(func(pool.Handle, *Unit) {}) }, "OnCreate"},
		{"COB binder", func(w *World, _ *Unit) { w.cobBinder = func(*Unit) error { return nil } }, "cobBinder"},
		{"source binding", func(w *World, _ *Unit) { w.cobFS = fixtureCOBFS{} }, "cobFS"},
		{"RNG binding", func(w *World, _ *Unit) { sim := rng.NewSimulation(9); w.simulationRNG = &sim }, "simulationRNG"},
		{"yard closure", func(_ *World, u *Unit) { u.yardTransaction = func(bool) {} }, "yardTransaction"},
		{"status closure", func(_ *World, u *Unit) { u.statusCue = func(*Unit, uint8) {} }, "statusCue"},
		{"remaining NaN", func(_ *World, u *Unit) { u.Remaining = math.Float32frombits(0x7fc00001) }, ".Remaining"},
		{"spot metal NaN", func(_ *World, u *Unit) { u.SpotMetal = math.Float32frombits(0x7fc00001) }, ".SpotMetal"},
	} {
		t.Run(test.name, func(t *testing.T) {
			w, inputs := checkpointFixture(t)
			u := checkpointCreate(t, w, "armdef", 0)
			test.edit(w, u)
			digests, data, _, err := checkpointUnitCapture(w, inputs)
			if err == nil || !strings.Contains(err.Error(), test.path) || digests != (checkpoint.Digests{}) || data != nil {
				t.Fatalf("capture = %v, %d bytes, %v", digests, len(data), err)
			}
		})
	}
}

func TestUnitCheckpointRequiresDiscoveredReferencesAndUniqueSerials(t *testing.T) {
	w, inputs := checkpointFixture(t)
	u := checkpointCreate(t, w, "armdef", 0)
	keys, err := inputs.CheckpointKeys()
	if err != nil {
		t.Fatal(err)
	}
	c := NewCheckpointContext(keys)
	var out bytes.Buffer
	if err := w.WriteCheckpoint(checkpoint.NewEncoder(&out), c); err == nil || !strings.Contains(err.Error(), "discovered") {
		t.Fatalf("undiscovered reference was accepted: %v", err)
	}
	if len(c.Allocations.Values()) != 0 || len(c.VMs.Values()) != 0 {
		t.Fatal("writer registered references")
	}
	copy := *u
	copy.Alive = false
	if _, _, _, err := checkpointUnitCapture(w, inputs, &copy); err == nil || !strings.Contains(err.Error(), "distinct successful allocation serial") {
		t.Fatalf("duplicate serial accepted: %v", err)
	}
	// A freed residual preserves the exact float bits and rejects NaN too.
	w.FreeImmediate(u.Handle)
	w.rawUnits[u.Handle].Remaining = math.Float32frombits(0x7fc00001)
	if _, _, _, err := checkpointUnitCapture(w, inputs); err == nil || !strings.Contains(err.Error(), ".Remaining") {
		t.Fatalf("NaN raw residual accepted: %v", err)
	}
}

func TestUnitCheckpointAbsentBindingTagsHaveStablePositions(t *testing.T) {
	w, inputs := checkpointFixture(t)
	checkpointCreate(t, w, "armdef", 0)
	_, _, c := mustCheckpointUnitCapture(t, w, inputs)
	var out bytes.Buffer
	if err := w.WriteCheckpoint(checkpoint.NewEncoder(&out), c); err != nil {
		t.Fatal(err)
	}
	data := out.Bytes()
	// Eight binding tags precede createdCounters; extraction follows it.
	// Two trailing owner bindings follow the 21-slot pool payload, whose size
	// is 25 alive bytes + 46 definition bytes + 8 limit + 160 slice bounds.
	if !bytes.Equal(data[:8], make([]byte, 8)) || data[48] != 0 || data[376] != 0 || data[377] != 0 {
		t.Fatal("absent World binding tags missing from lexical positions")
	}
	if binary.LittleEndian.Uint32(data[8:]) != 1 || binary.LittleEndian.Uint64(data[49:]) != 1 || binary.LittleEndian.Uint64(data[57:]) != 1 || binary.LittleEndian.Uint32(data[378:]) != 21 {
		t.Fatal("binding tags changed retained World field framing")
	}
	// Unit statusCue and yardTransaction are its last two lexical fields.
	if !bytes.Equal(data[len(data)-2:], []byte{0, 0}) {
		t.Fatal("absent Unit bindings omitted from the allocation record")
	}
}

func TestUnitCheckpointFixedHighBitsChangeDigest(t *testing.T) {
	w, inputs := checkpointFixture(t)
	u := checkpointCreate(t, w, "armdef", 0)
	before, _, _ := mustCheckpointUnitCapture(t, w, inputs)
	original := *u
	// Every value differs only above the low 32 bits. A writer that narrows
	// numeric.Fixed to an int32 would silently hash these as the original zero.
	const high numeric.Fixed = 1 << 32
	for _, test := range []struct {
		name string
		edit func()
	}{
		{"X", func() { u.X = high }},
		{"Y", func() { u.Y = high }},
		{"Z", func() { u.Z = high }},
		{"Move.Speed", func() { u.Move.Speed = high }},
		{"Move.VelX", func() { u.Move.VelX = high }},
		{"Move.VelY", func() { u.Move.VelY = high }},
		{"Move.VelZ", func() { u.Move.VelZ = high }},
		{"Target.X", func() { u.Slots[0].Target.X = high }},
		{"Target.Z", func() { u.Slots[0].Target.Z = high }},
	} {
		t.Run(test.name, func(t *testing.T) {
			*u = original
			test.edit()
			after, _, _ := mustCheckpointUnitCapture(t, w, inputs)
			if after.Full == before.Full || after.Owners[1] == before.Owners[1] {
				t.Fatal("high fixed-point bits were omitted")
			}
		})
	}
}

func TestUnitCheckpointFixedWidthVectors(t *testing.T) {
	// These authored vectors spell out signed 64-bit storage, including values
	// outside the int32 domain, without using the checkpoint encoder to form
	// the expected bytes.
	var move bytes.Buffer
	e := checkpoint.NewEncoder(&move)
	writeCheckpointMove(e, &MoveState{
		Bank: 0x1234, Heading: 0x5678, Mode: 1, ModeMirror: 2, Pitch: 0x9abc,
		Speed: 0x100000001, VelX: -0x100000001, VelY: 0x200000002, VelZ: -0x200000002,
	})
	wantMove := []byte{
		0x34, 0x12, 0x78, 0x56, 1, 2, 0xbc, 0x9a,
		1, 0, 0, 0, 1, 0, 0, 0,
		0xff, 0xff, 0xff, 0xff, 0xfe, 0xff, 0xff, 0xff,
		2, 0, 0, 0, 2, 0, 0, 0,
		0xfe, 0xff, 0xff, 0xff, 0xfd, 0xff, 0xff, 0xff,
	}
	if e.Err() != nil || !bytes.Equal(move.Bytes(), wantMove) {
		t.Fatalf("Move vector = %x, %v; want %x", move.Bytes(), e.Err(), wantMove)
	}
	var slot bytes.Buffer
	e = checkpoint.NewEncoder(&slot)
	writeCheckpointSlot(e, nil, &Slot{Target: Target{Kind: TargetGround, Unit: 0xabcd, X: 0x100000001, Z: -0x100000001}}, "slot")
	// The fields preceding Target occupy 31 bytes; its raw handle is u32,
	// X/Z are each int64, and the absent Weapon contributes one trailing byte.
	wantSlot := append(make([]byte, 31), []byte{
		2, 0xcd, 0xab, 0, 0,
		1, 0, 0, 0, 1, 0, 0, 0,
		0xff, 0xff, 0xff, 0xff, 0xfe, 0xff, 0xff, 0xff,
		0,
	}...)
	if e.Err() != nil || !bytes.Equal(slot.Bytes(), wantSlot) {
		t.Fatalf("Slot vector = %x, %v; want %x", slot.Bytes(), e.Err(), wantSlot)
	}
	w, inputs := checkpointFixture(t)
	u := checkpointCreate(t, w, "armdef", 0)
	_, _, c := mustCheckpointUnitCapture(t, w, inputs)
	u.X, u.Y, u.Z = 0x100000001, -0x100000001, 0x200000002
	u.YardOpen, u.deathExtraHookFired = true, true
	var unit bytes.Buffer
	e = checkpoint.NewEncoder(&unit)
	writeCheckpointUnit(e, c, u, "unit")
	// The record ends with X, Y, YardOpen, Z, two death latches and two
	// absent binding tags: 29 bytes in lexical field order.
	wantTail := []byte{
		1, 0, 0, 0, 1, 0, 0, 0,
		0xff, 0xff, 0xff, 0xff, 0xfe, 0xff, 0xff, 0xff,
		1,
		2, 0, 0, 0, 2, 0, 0, 0,
		1, 0, 0, 0,
	}
	if e.Err() != nil || !bytes.Equal(unit.Bytes()[unit.Len()-len(wantTail):], wantTail) {
		t.Fatalf("Unit vector tail = %x, %v; want %x", unit.Bytes(), e.Err(), wantTail)
	}
}
