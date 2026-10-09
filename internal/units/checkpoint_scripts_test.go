package units

import (
	"bytes"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/cob"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/model"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

func checkpointScriptFixture(t *testing.T) (*World, *content.SimulationInputs, *Unit) {
	t.Helper()
	w, inputs := checkpointFixture(t)
	u := checkpointCreate(t, w, "armdef", 0)
	mdl, ok := inputs.Model("fixture")
	if !ok {
		t.Fatal("missing admitted model")
	}
	vm := cob.NewVM(u.Def.Script)
	bridge := cob.NewCallbackBridge(vm)
	names := make([]string, len(mdl.Pieces))
	for i, p := range mdl.Pieces {
		names[i] = p.Name
	}
	binding := &cob.Binding{
		Program: u.Def.Script, VM: vm, Model: mdl, Callbacks: bridge,
		PieceMap: cob.LinkPieces(u.Def.Script.Pieces, names),
	}
	// U2 tests use absent ports. The production composition attestation is
	// staged at U6; replacing the authored fixture's VM changes no engine path.
	u.Script, u.ScriptState = vm, &ScriptState{VM: vm, Bridge: bridge, Binding: binding}
	return w, inputs, u
}

func checkpointScriptCapture(w *World, inputs *content.SimulationInputs) (checkpoint.Digests, []byte, error) {
	keys, err := inputs.CheckpointKeys()
	if err != nil {
		return checkpoint.Digests{}, nil, err
	}
	c := NewCheckpointContext(keys)
	if _, err := w.CollectCheckpointReferences(c); err != nil {
		return checkpoint.Digests{}, nil, err
	}
	var out bytes.Buffer
	capture, err := checkpoint.NewCapture(checkpoint.Identity{Content: checkpoint.Digest(inputs.Digest())}, &out)
	if err != nil {
		return checkpoint.Digests{}, nil, err
	}
	for owner := checkpoint.OwnerRuntime; owner <= checkpoint.OwnerComputersScenario; owner++ {
		e, err := capture.Section(owner, owner == checkpoint.OwnerUnits || owner == checkpoint.OwnerScripts)
		if err != nil {
			return checkpoint.Digests{}, nil, err
		}
		switch owner {
		case checkpoint.OwnerUnits:
			err = w.WriteCheckpoint(e, c)
		case checkpoint.OwnerScripts:
			err = w.WriteScriptCheckpoint(e, c)
		}
		if err != nil {
			return checkpoint.Digests{}, nil, err
		}
	}
	digests, err := capture.Finish()
	return digests, out.Bytes(), err
}

func TestScriptCheckpointOwnerBoundariesAndAddressIndependence(t *testing.T) {
	w, inputs, u := checkpointScriptFixture(t)
	before := u.Script.Threads
	base, data, err := checkpointScriptCapture(w, inputs)
	if err != nil {
		t.Fatal(err)
	}
	other, admitted, _ := checkpointScriptFixture(t)
	got, independent, err := checkpointScriptCapture(other, admitted)
	if err != nil || got != base || !bytes.Equal(independent, data) {
		t.Fatalf("allocation addresses affected checkpoint: %v", err)
	}
	if u.Script.Threads != before {
		t.Fatal("checkpoint advanced the VM")
	}
	u.Script.Threads[7].Stack[31]++ // inactive, above SP, still reusable state
	changed, _, err := checkpointScriptCapture(w, inputs)
	if err != nil || changed.Full == base.Full || changed.Owners[checkpoint.OwnerScripts-1] == base.Owners[checkpoint.OwnerScripts-1] ||
		changed.Owners[checkpoint.OwnerUnits-1] != base.Owners[checkpoint.OwnerUnits-1] {
		t.Fatalf("VM state did not stay in scripts owner: %v", err)
	}
	u.Script.Threads = before
	u.RenderPieceFlags = append(u.RenderPieceFlags, 3)
	changed, _, err = checkpointScriptCapture(w, inputs)
	if err != nil || changed.Owners[checkpoint.OwnerUnits-1] == base.Owners[checkpoint.OwnerUnits-1] ||
		changed.Owners[checkpoint.OwnerScripts-1] != base.Owners[checkpoint.OwnerScripts-1] {
		t.Fatalf("physical unit flags written outside units owner: %v", err)
	}
}

func TestScriptCheckpointValidatesAliasesAndAdmittedLinks(t *testing.T) {
	for name, edit := range map[string]func(*Unit){
		"unit VM":         func(u *Unit) { u.Script = nil },
		"bridge VM":       func(u *Unit) { u.ScriptState.Bridge.VM = cob.NewVM(u.Def.Script) },
		"binding VM":      func(u *Unit) { u.ScriptState.Binding.VM = cob.NewVM(u.Def.Script) },
		"binding program": func(u *Unit) { copy := *u.Def.Script; u.ScriptState.Binding.Program = &copy },
		"piece links":     func(u *Unit) { u.ScriptState.Binding.PieceMap = []int{99} },
		"foreign model":   func(u *Unit) { copy := *u.ScriptState.Binding.Model; u.ScriptState.Binding.Model = &copy },
		"binding callback": func(u *Unit) {
			u.ScriptState.Binding.SetSFXSink(u.ScriptState.Binding.SFXSink, func(int, int32) bool { return true })
		},
	} {
		t.Run(name, func(t *testing.T) {
			w, inputs, u := checkpointScriptFixture(t)
			edit(u)
			if _, _, err := checkpointScriptCapture(w, inputs); err == nil {
				t.Fatal("invalid alias or unattested binding accepted")
			}
		})
	}
}

func TestScriptCheckpointExcludesLocalCachesAndRefusesUnknownVMRoot(t *testing.T) {
	w, inputs, u := checkpointScriptFixture(t)
	base, data, err := checkpointScriptCapture(w, inputs)
	if err != nil {
		t.Fatal(err)
	}
	u.Script.DrainCalls += 20
	u.ScriptState.Bridge.SetLifecycleSink(func(cob.LifecycleEvent) {})
	u.ScriptState.Binding.ScriptPath = "local diagnostics only"
	u.ScriptState.Binding.LinkNotes = []cob.BindingDiagnostic{{Detail: "local"}}
	u.Script.Pieces[0] = model.PieceState{Hidden: true, DontCache: true, DontShadow: true, DontShade: true}
	got, same, err := checkpointScriptCapture(w, inputs)
	if err != nil || got != base || !bytes.Equal(same, data) {
		t.Fatalf("local cache/trace/provenance changed checkpoint: %v", err)
	}
	keys, err := inputs.CheckpointKeys()
	if err != nil {
		t.Fatal(err)
	}
	c := NewCheckpointContext(keys)
	if _, err := w.CollectCheckpointReferences(c); err != nil {
		t.Fatal(err)
	}
	if _, err := c.VMs.Add(cob.NewVM(u.Def.Script)); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	e := checkpoint.NewEncoder(&out)
	if err := w.WriteScriptCheckpoint(e, c); err == nil || e.Err() == nil {
		t.Fatal("VM with no admitted allocation owner accepted")
	}
}
