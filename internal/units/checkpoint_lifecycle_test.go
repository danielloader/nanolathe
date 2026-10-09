package units

import (
	"bytes"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

func TestCheckpointLifecycleBindings(t *testing.T) {
	w, inputs := checkpointFixture(t)
	keys, err := inputs.CheckpointKeys()
	if err != nil {
		t.Fatal(err)
	}
	authority := checkpoint.NewBindingAuthority()
	c := NewCheckpointContext(keys)
	if err := c.SetLifecycleBindings(w, authority); err != nil {
		t.Fatal(err)
	}
	var baseline bytes.Buffer
	if err := w.WriteCheckpoint(checkpoint.NewEncoder(&baseline), c); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(baseline.Bytes()[:4], []byte{0, 0, 0, 0}) {
		t.Fatal("absent lifecycle framing")
	}
	// The declaration-order slot array differs from the lexical wire order.
	// Each installation changes exactly its own lexical presence byte.
	for _, tc := range []struct {
		name      string
		offset    int
		install   func(*checkpoint.BindingAuthority)
		reinstall func()
		clear     func()
	}{
		{"death", 2, func(a *checkpoint.BindingAuthority) {
			w.SetDeathHookWithCheckpointBinding(func(pool.Handle, DeathCause, *Unit) { panic("capture invoked death") }, a)
		}, func() { w.SetDeathHook(w.DeathHook()) }, func() { w.SetDeathHook(nil) }},
		{"extra", 3, func(a *checkpoint.BindingAuthority) {
			w.SetDeathExtraHookWithCheckpointBinding(func(pool.Handle, DeathCause, *Unit) { panic("capture invoked extra") }, a)
		}, func() { w.SetDeathExtraHook(w.DeathExtraHook()) }, func() { w.SetDeathExtraHook(nil) }},
		{"create", 1, func(a *checkpoint.BindingAuthority) {
			w.SetCreateHookWithCheckpointBinding(func(pool.Handle, *Unit) { panic("capture invoked create") }, a)
		}, func() { w.SetCreateHook(w.CreateHook()) }, func() { w.SetCreateHook(nil) }},
		{"capture", 0, func(a *checkpoint.BindingAuthority) {
			w.SetCaptureHookWithCheckpointBinding(func(pool.Handle, uint8, uint8, *Unit) { panic("capture invoked capture") }, a)
		}, func() { w.SetCaptureHook(w.CaptureHook()) }, func() { w.SetCaptureHook(nil) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer tc.clear()
			tc.install(authority)
			if _, err := w.CollectCheckpointReferences(c); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			if err := w.WriteCheckpoint(checkpoint.NewEncoder(&out), c); err != nil {
				t.Fatal(err)
			}
			want := bytes.Clone(baseline.Bytes())
			want[tc.offset] = 1
			if !bytes.Equal(out.Bytes(), want) {
				t.Fatal("binding changed bytes beyond its presence")
			}
			foreign := NewCheckpointContext(keys)
			if err := foreign.SetLifecycleBindings(w, checkpoint.NewBindingAuthority()); err != nil {
				t.Fatal(err)
			}
			if _, err := w.CollectCheckpointReferences(foreign); err == nil {
				t.Fatal("wrong authority accepted")
			}
			copied := *w
			copiedContext := NewCheckpointContext(keys)
			if err := copiedContext.SetLifecycleBindings(&copied, authority); err != nil {
				t.Fatal(err)
			}
			if _, err := copied.CollectCheckpointReferences(copiedContext); err == nil {
				t.Fatal("copied world retained original ownership proof")
			}
			tc.reinstall()
			if _, err := w.CollectCheckpointReferences(c); err == nil {
				t.Fatal("unchanged function reinstall retained proof")
			}
			out.Reset()
			if err := w.WriteCheckpoint(checkpoint.NewEncoder(&out), c); err == nil || out.Len() != 0 {
				t.Fatal("writer emitted an unattested lifecycle binding")
			}
			tc.install(nil)
			if _, err := w.CollectCheckpointReferences(c); err == nil {
				t.Fatal("nil authority attested callback")
			}
			tc.clear()
			if _, err := w.CollectCheckpointReferences(c); err != nil {
				t.Fatal("absent callback refused", err)
			}
		})
	}
}

func TestCheckpointLifecycleRegistrationAndSlotIsolation(t *testing.T) {
	w := &World{}
	a := checkpoint.NewBindingAuthority()
	c := NewCheckpointContext(nil)
	for _, err := range []error{(*CheckpointContext)(nil).SetLifecycleBindings(w, a), c.SetLifecycleBindings(nil, a), c.SetLifecycleBindings(w, nil)} {
		if err == nil {
			t.Fatal("missing registration operand accepted")
		}
	}
	if c.lifecycleWorld != nil || c.lifecycleAuthority != nil {
		t.Fatal("failed registration mutated context")
	}
	if err := c.SetLifecycleBindings(w, a); err != nil {
		t.Fatal(err)
	}
	if err := c.SetLifecycleBindings(w, a); err != nil {
		t.Fatal("idempotent registration refused", err)
	}
	if err := c.SetLifecycleBindings(&World{}, a); err == nil {
		t.Fatal("conflicting owner accepted")
	}
	if err := c.SetLifecycleBindings(w, checkpoint.NewBindingAuthority()); err == nil {
		t.Fatal("conflicting authority accepted")
	}
	if err := (&World{}).validateCheckpointLifecycle(c); err == nil {
		t.Fatal("context used for foreign world")
	}
	w.SetDeathHookWithCheckpointBinding(func(pool.Handle, DeathCause, *Unit) {}, a)
	w.SetCreateHookWithCheckpointBinding(func(pool.Handle, *Unit) {}, a)
	deathProof := w.checkpointLifecycle[0]
	w.SetCreateHook(w.CreateHook())
	if w.checkpointLifecycle[0] != deathProof || w.checkpointLifecycle[2] != (checkpointLifecycleProof{}) {
		t.Fatal("ordinary setter changed another slot's proof")
	}
	w.SetCreateHook(nil)
	if err := w.validateCheckpointLifecycle(c); err != nil {
		t.Fatal("unrelated attested slot was invalidated", err)
	}
	w.SetCaptureHookWithCheckpointBinding(nil, a)
	if w.checkpointLifecycle[3] != (checkpointLifecycleProof{}) {
		t.Fatal("absent callback retained proof")
	}
}
