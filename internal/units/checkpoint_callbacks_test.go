package units

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/cob"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/sim/rng"
)

func unitCallbackContext(t *testing.T, w *World, inputs *content.SimulationInputs, authority *checkpoint.BindingAuthority) *CheckpointContext {
	t.Helper()
	keys, err := inputs.CheckpointKeys()
	if err != nil {
		t.Fatal(err)
	}
	c := NewCheckpointContext(keys)
	if authority != nil {
		if err := c.SetLifecycleBindings(w, authority); err != nil {
			t.Fatal(err)
		}
	}
	return c
}

func unitCallbackBytes(t *testing.T, w *World, c *CheckpointContext) []byte {
	t.Helper()
	if _, err := w.CollectCheckpointReferences(c); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := w.WriteCheckpoint(checkpoint.NewEncoder(&out), c); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func unitCallbackRefused(t *testing.T, w *World, c *CheckpointContext, path string) {
	t.Helper()
	want := "nanolathe: unit checkpoint: logical path " + path + ","
	if _, err := w.CollectCheckpointReferences(c); err == nil || !strings.HasPrefix(err.Error(), want) {
		t.Fatalf("collect = %v; want refusal at %s", err, path)
	}
	var out bytes.Buffer
	e := checkpoint.NewEncoder(&out)
	if err := w.WriteCheckpoint(e, c); err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("write = %v; want refusal at %s", err, path)
	}
	before := out.Len()
	e.U64(123)
	if out.Len() != before {
		t.Fatal("callback refusal was not sticky")
	}
}

var unitCallbackSlots = []struct {
	name      string
	bind      func(*Unit, *checkpoint.BindingAuthority)
	reinstall func(*Unit)
	clear     func(*Unit)
	clearWith func(*Unit, *checkpoint.BindingAuthority)
}{
	{
		"statusCue",
		func(u *Unit, a *checkpoint.BindingAuthority) {
			u.SetStatusCueSinkWithCheckpointBinding(func(*Unit, uint8) { panic("capture called status cue") }, a)
		},
		func(u *Unit) { u.SetStatusCueSink(u.statusCue) },
		func(u *Unit) { u.SetStatusCueSink(nil) },
		func(u *Unit, a *checkpoint.BindingAuthority) { u.SetStatusCueSinkWithCheckpointBinding(nil, a) },
	},
	{
		"yardTransaction",
		func(u *Unit, a *checkpoint.BindingAuthority) {
			u.SetYardOpenTransactionWithCheckpointBinding(func(bool) { panic("capture called yard transaction") }, a)
		},
		func(u *Unit) { u.SetYardOpenTransaction(u.yardTransaction) },
		func(u *Unit) { u.SetYardOpenTransaction(nil) },
		func(u *Unit, a *checkpoint.BindingAuthority) { u.SetYardOpenTransactionWithCheckpointBinding(nil, a) },
	},
}

func TestCheckpointUnitCallbackPresenceAndOwnership(t *testing.T) {
	for index, slot := range unitCallbackSlots {
		t.Run(slot.name, func(t *testing.T) {
			w, inputs := checkpointFixture(t)
			u := checkpointCreate(t, w, "armdef", 0)
			a := checkpoint.NewBindingAuthority()
			c := unitCallbackContext(t, w, inputs, a)
			baseline := unitCallbackBytes(t, w, unitCallbackContext(t, w, inputs, nil))
			if !bytes.Equal(baseline[len(baseline)-2:], []byte{0, 0}) || !bytes.Equal(baseline, unitCallbackBytes(t, w, c)) {
				t.Fatal("registered/unregistered nil callback bytes differ")
			}
			slot.bind(u, a)
			// Independent lexical framing: statusCue then yardTransaction are
			// the record's final two bytes. Proof adds no other payload.
			want := bytes.Clone(baseline)
			want[len(want)-2+index] = 1
			if got := unitCallbackBytes(t, w, c); !bytes.Equal(got, want) {
				t.Fatal("binding changed bytes beyond its own presence")
			}
			path := "units.allocations[1]." + slot.name
			unitCallbackRefused(t, w, unitCallbackContext(t, w, inputs, nil), path)
			unitCallbackRefused(t, w, unitCallbackContext(t, w, inputs, checkpoint.NewBindingAuthority()), path)
			foreignWorld := NewSliced(2, w.catalog)
			unitCallbackRefused(t, w, unitCallbackContext(t, foreignWorld, inputs, a), "units.lifecycle")
			slot.reinstall(u)
			unitCallbackRefused(t, w, c, path)
			slot.bind(u, nil)
			unitCallbackRefused(t, w, c, path)
			slot.bind(u, checkpoint.NewBindingAuthority())
			unitCallbackRefused(t, w, c, path)
			slot.bind(u, a)

			copied := *u
			w.units[u.Handle], w.rawUnits[u.Handle] = &copied, &copied
			unitCallbackRefused(t, w, unitCallbackContext(t, w, inputs, a), path)
			// A fresh installation may attest this actual relocated owner.
			slot.bind(&copied, a)
			if got := unitCallbackBytes(t, w, unitCallbackContext(t, w, inputs, a)); !bytes.Equal(got, want) {
				t.Fatal("callback owner address entered payload")
			}
			w.units[u.Handle], w.rawUnits[u.Handle] = u, u
			for _, clear := range []func(){func() { slot.clear(u) }, func() { slot.clearWith(u, a) }} {
				slot.bind(u, a)
				clear()
				if u.checkpointCallbacks[index] != (checkpointCallbackProof{}) {
					t.Fatal("nil callback retained proof")
				}
				if got := unitCallbackBytes(t, w, c); !bytes.Equal(got, baseline) {
					t.Fatal("clearing callback changed nil payload")
				}
			}
		})
	}
}

func TestCheckpointUnitCallbackSlotIsolationAndNilReceiver(t *testing.T) {
	a := checkpoint.NewBindingAuthority()
	var absent *Unit
	for _, slot := range unitCallbackSlots {
		slot.bind(absent, a)
		slot.clear(absent)
		slot.clearWith(absent, a)
	}
	w, inputs := checkpointFixture(t)
	u := checkpointCreate(t, w, "armdef", 0)
	c := unitCallbackContext(t, w, inputs, a)
	for _, slot := range unitCallbackSlots {
		slot.bind(u, a)
	}
	for index, slot := range unitCallbackSlots {
		before := u.checkpointCallbacks
		slot.reinstall(u)
		other := 1 - index
		if u.checkpointCallbacks[other] != before[other] || u.checkpointCallbacks[index] != (checkpointCallbackProof{}) {
			t.Fatal("ordinary installation did not clear exactly its own proof")
		}
		unitCallbackSlots[other].bind(u, a)
		unitCallbackRefused(t, w, c, "units.allocations[1]."+slot.name)
		slot.bind(u, a)
		unitCallbackBytes(t, w, c)
	}
}

func TestCheckpointUnitCallbackInstalledBeforeSerialCommit(t *testing.T) {
	w, inputs := checkpointFixture(t)
	a := checkpoint.NewBindingAuthority()
	c := unitCallbackContext(t, w, inputs, a)
	var installed *Unit
	w.SetCOBBinder(func(u *Unit) error {
		if u.AllocationSerial != 0 || w.pendingAllocationSerials == 0 {
			t.Fatal("fixture missed the pre-serial binding boundary")
		}
		installed = u
		for _, slot := range unitCallbackSlots {
			slot.bind(u, a)
		}
		unitCallbackRefused(t, w, c, "units.World.pendingAllocationSerials")
		u.SetScript(cob.NewVM(u.Def.Script))
		return nil
	})
	u := checkpointCreate(t, w, "armdef", 0)
	if u != installed || u.AllocationSerial != 1 {
		t.Fatal("allocation did not commit the installed unit")
	}
	// This unit does not admit the world's independent COB binder.
	unitCallbackRefused(t, w, c, "units.World.cobBinder")
	w.SetCOBBinder(nil)
	if got := unitCallbackBytes(t, w, c); !bytes.Equal(got[len(got)-2:], []byte{1, 1}) {
		t.Fatal("pre-serial callback proof was lost after commit")
	}
	for _, invalid := range []uint64{0, w.lastAllocationSerial + 1} {
		u.AllocationSerial = invalid
		unitCallbackRefused(t, w, c, "units.allocations[1].AllocationSerial")
	}
	u.AllocationSerial = 1
	unitCallbackBytes(t, w, c)
}

func TestCheckpointRetiredUnitKeepsItsOwnCallbackProof(t *testing.T) {
	w, inputs := checkpointFixture(t)
	old := checkpointCreate(t, w, "armdef", 0)
	a := checkpoint.NewBindingAuthority()
	for _, slot := range unitCallbackSlots {
		slot.bind(old, a)
	}
	proof := old.checkpointCallbacks
	w.FreeImmediate(old.Handle)
	current := checkpointCreate(t, w, "cordef", 0)
	if current.Handle != old.Handle || current.AllocationSerial == old.AllocationSerial || old.checkpointCallbacks != proof {
		t.Fatal("slot reuse changed retired callback ownership")
	}
	c := unitCallbackContext(t, w, inputs, a)
	unitCallbackBytes(t, w, c)
	if _, err := c.Allocations.Add(old); err != nil {
		t.Fatal(err)
	}
	data := unitCallbackBytes(t, w, c)
	if got := c.Allocations.Values(); len(got) != 2 || got[0] != current || got[1] != old || !bytes.Equal(data[len(data)-2:], []byte{1, 1}) {
		t.Fatal("retired unit callbacks redirected to the live slot")
	}
	copied := *old
	copyContext := unitCallbackContext(t, w, inputs, a)
	unitCallbackBytes(t, w, copyContext)
	if _, err := copyContext.Allocations.Add(&copied); err != nil {
		t.Fatal(err)
	}
	unitCallbackRefused(t, w, copyContext, "units.allocations[2].statusCue")
}

func TestCheckpointUnitCallbackCapturePurity(t *testing.T) {
	w, inputs := checkpointFixture(t)
	u := checkpointCreate(t, w, "armdef", 0)
	a := checkpoint.NewBindingAuthority()
	c := unitCallbackContext(t, w, inputs, a)
	sim, crt := rng.NewSimulation(41), rng.NewCRT(43)
	calls := 0
	u.SetYardOpenTransactionWithCheckpointBinding(func(bool) { calls++; sim.Uint32n(100) }, a)
	u.SetStatusCueSinkWithCheckpointBinding(func(*Unit, uint8) { calls++; crt.Rand() }, a)
	state := func() Unit {
		copyUnit := *u
		copyUnit.yardTransaction, copyUnit.statusCue = nil, nil
		return copyUnit
	}
	before, beforeWorld, beforeSim, beforeCRT := state(), *w, sim, crt
	first := unitCallbackBytes(t, w, c)
	if again := unitCallbackBytes(t, w, c); !bytes.Equal(first, again) {
		t.Fatal("repeated capture changed bytes")
	}
	if calls != 0 || sim != beforeSim || crt != beforeCRT || !reflect.DeepEqual(state(), before) || !reflect.DeepEqual(*w, beforeWorld) {
		t.Fatal("capture invoked callbacks or changed unit/world/RNG state")
	}
	// The existing unsupported world RNG remains refused independently.
	w.SetSimulationRNG(&sim)
	unitCallbackRefused(t, w, c, "units.World.simulationRNG")
	w.SetSimulationRNG(nil)
	if calls != 0 || sim != beforeSim || crt != beforeCRT || !reflect.DeepEqual(state(), before) || !reflect.DeepEqual(*w, beforeWorld) {
		t.Fatal("refused capture changed unit/world/RNG state")
	}
}

// Proof is diagnostic only: repeated yard requests still reach the transaction,
// denied commits remain denied, and status cues retain their edge-only codes
// [04 §4.7 port 18][03 R-AUD-01 §7].
func TestCheckpointUnitCallbackInstallationPreservesGameplay(t *testing.T) {
	for _, admitted := range []bool{false, true} {
		u := &Unit{}
		var requests [][2]bool
		var cues []uint8
		yard := func(next bool) {
			requests = append(requests, [2]bool{u.YardOpen, next})
			if next {
				u.YardOpen = true
			}
		}
		status := func(got *Unit, code uint8) {
			if got != u {
				t.Fatal("status cue received another unit")
			}
			cues = append(cues, code)
		}
		if admitted {
			a := checkpoint.NewBindingAuthority()
			u.SetYardOpenTransactionWithCheckpointBinding(yard, a)
			u.SetStatusCueSinkWithCheckpointBinding(status, a)
		} else {
			u.SetYardOpenTransaction(yard)
			u.SetStatusCueSink(status)
		}
		if len(requests) != 0 || len(cues) != 0 {
			t.Fatal("installation invoked a callback")
		}
		port := unitPortHandlers(nil, u)[cob.Port(18)]
		port([]int32{18, 1})
		port([]int32{18, 0})
		port([]int32{18, 1})
		u.SetActivationEdge(true)
		u.SetActivationEdge(true)
		u.SetActivationEdge(false)
		u.SetCloakedInstance(true)
		u.SetCloakedInstance(true)
		u.SetCloakedInstance(false)
		if !u.YardOpen || !reflect.DeepEqual(requests, [][2]bool{{false, true}, {true, false}, {true, true}}) || !bytes.Equal(cues, []byte{3, 4, 14, 15}) {
			t.Fatalf("admitted=%t requests=%v cues=%v yard=%t", admitted, requests, cues, u.YardOpen)
		}
	}
}
