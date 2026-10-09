package content

import (
	"maps"
	"reflect"
	"slices"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/cob"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

func checkpointCompiledProgramsFixture(t *testing.T, edit func(*Catalog)) (*SimulationInputs, *CheckpointKeys) {
	t.Helper()
	f := newFrozenFixture(t)
	cat := f.catalog(t)
	if edit != nil {
		edit(cat)
	}
	inputs := f.freeze(t, SimulationInputRequest{Catalog: cat})
	keys, err := inputs.CheckpointKeys()
	if err != nil {
		t.Fatal(err)
	}
	return inputs, keys
}

// Ordinals belong to the stored slice, not the first-name projection or the
// allocator's separately retained UnitDefID (DESIGN_MULTIPLAYER §16.3.56).
// Holes are retained from key construction; the manifest does not encode a
// history of earlier trailing nil slots.
func TestCheckpointCompiledAllocationProgramsRetainRecords(t *testing.T) {
	inputs, keys := checkpointCompiledProgramsFixture(t, func(cat *Catalog) {
		duplicate := *cat.unitRecords[1]
		duplicate.UnitDefID = 77
		cat.unitRecords = []*UnitDef{cat.unitRecords[0], nil, cat.unitRecords[1], &duplicate}
	})
	cat := inputs.catalog
	if err := keys.ValidateCompiledAllocationPrograms(cat); err != nil {
		t.Fatal(err)
	}
	for _, index := range []int{2, 3} {
		unit := cat.unitRecords[index]
		ref, err := keys.ProgramForUnit(unit, unit.Script)
		want := checkpoint.Definition{Family: SimulationFamilyCOB, Ordinal: uint32(index + 1), Key: "unit/testunit"}
		if err != nil || ref != want {
			t.Fatalf("record %d: program = %+v, %v; want %+v", index, ref, err, want)
		}
	}
	// Exact Catalog ownership is a separate context contract. These keys prove
	// the retained records, not the address of the containing Catalog value.
	copyCatalog := *cat
	if err := keys.ValidateCompiledAllocationPrograms(&copyCatalog); err != nil {
		t.Fatalf("same records in copied container refused: %v", err)
	}
	cat.unitRecords[2], cat.unitRecords[3] = cat.unitRecords[3], cat.unitRecords[2]
	if err := keys.ValidateCompiledAllocationPrograms(cat); err == nil {
		t.Fatal("swapped equal-name records accepted")
	}
}

func TestCheckpointCompiledAllocationProgramsRejectRecordChanges(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*Catalog)
	}{
		{"add", func(c *Catalog) { copyUnit := *c.unitRecords[1]; c.unitRecords = append(c.unitRecords, &copyUnit) }},
		{"remove", func(c *Catalog) { c.unitRecords = c.unitRecords[:2] }},
		{"remove trailing hole", func(c *Catalog) { c.unitRecords = c.unitRecords[:3] }},
		{"fill hole", func(c *Catalog) { copyUnit := *c.unitRecords[1]; c.unitRecords[0] = &copyUnit }},
		{"empty slot", func(c *Catalog) { c.unitRecords[1] = nil }},
		{"move hole", func(c *Catalog) { c.unitRecords[0], c.unitRecords[1] = c.unitRecords[1], c.unitRecords[0] }},
		{"reorder", func(c *Catalog) { c.unitRecords[1], c.unitRecords[2] = c.unitRecords[2], c.unitRecords[1] }},
		{"copy", func(c *Catalog) { copyUnit := *c.unitRecords[1]; c.unitRecords[1] = &copyUnit }},
		{"unit name case", func(c *Catalog) { c.unitRecords[1].UnitName = "OtherUnit" }},
		{"canonical key case", func(c *Catalog) { c.unitRecords[1].CanonicalKey = "OtherUnit" }},
		{"allocator identity", func(c *Catalog) { c.unitRecords[1].UnitDefID++ }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inputs, keys := checkpointCompiledProgramsFixture(t, func(c *Catalog) {
				c.unitRecords = []*UnitDef{nil, c.unitRecords[0], c.unitRecords[1], nil}
			})
			if err := keys.ValidateCompiledAllocationPrograms(inputs.catalog); err != nil {
				t.Fatalf("admitted records refused: %v", err)
			}
			tc.edit(inputs.catalog)
			if err := keys.ValidateCompiledAllocationPrograms(inputs.catalog); err == nil {
				t.Fatal("changed admission accepted")
			}
		})
	}
}

func TestCheckpointCompiledAllocationProgramsRejectNeverBuiltMutation(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*UnitDef)
	}{
		{"missing", func(u *UnitDef) { u.Script = nil }},
		{"empty", func(u *UnitDef) { u.Script.Code = nil }},
		{"code with unchanged checksum", func(u *UnitDef) { u.Script.Code[0]++ }},
		{"entry point", func(u *UnitDef) { u.Script.Scripts["create"]++ }},
		{"entry name", func(u *UnitDef) { u.Script.Scripts["new"] = 0 }},
		{"piece", func(u *UnitDef) { u.Script.Pieces[0] = "changed" }},
		{"statics", func(u *UnitDef) { u.Script.Statics++ }},
		{"indexed entry", func(u *UnitDef) { u.Script.ScriptsByID[0]++ }},
		{"checksum", func(u *UnitDef) { u.Script.SourceChecksum++ }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inputs, keys := checkpointCompiledProgramsFixture(t, nil)
			// There are no live unit instances here. A future allocation must
			// still be covered, rather than validating only existing VMs.
			tc.edit(inputs.catalog.unitRecords[0])
			if err := keys.ValidateCompiledAllocationPrograms(inputs.catalog); err == nil {
				t.Fatal("changed future program accepted")
			}
		})
	}
}

func TestCheckpointCompiledAllocationProgramsRefuseAbsentAdmission(t *testing.T) {
	for _, empty := range []bool{false, true} {
		name := "missing"
		if empty {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			inputs, keys := checkpointCompiledProgramsFixture(t, func(c *Catalog) {
				u := c.unitRecords[0]
				u.UnitName, u.CanonicalKey, u.Script = "absent", "absent", nil
				if empty {
					u.Script = &cob.Program{}
				}
			})
			u := inputs.catalog.unitRecords[0]
			entry, ok := manifestEntry(inputs, SimulationFamilyCOB, "unit/absent")
			if !ok || entry.Presence != SimulationInputAbsent {
				t.Fatalf("expected absent admission: %+v", entry)
			}
			if err := keys.ValidateCompiledAllocationPrograms(inputs.catalog); err == nil {
				t.Fatal("absent compiled admission accepted")
			}
			u.Script = inputs.catalog.unitRecords[1].Script
			if err := keys.ValidateCompiledAllocationPrograms(inputs.catalog); err == nil {
				t.Fatal("later nonempty program repaired absent admission")
			}
		})
	}
}

func TestCheckpointCompiledAllocationProgramsRefuseFallback(t *testing.T) {
	f := newFrozenFixture(t)
	cat := f.catalog(t)
	u := cat.unitRecords[0]
	u.CanonicalKey, u.UnitName, u.Script = "fallback", "fallback.cob", nil
	data := frozenFixtureCOB([]uint32{frozenFixtureCreate}, []string{"Create"}, []uint32{0}, []string{"base"})
	f.write(t, "scripts/fallback.cob.cob", data)
	f.fs = mountFrozenFixture(t, f.root)
	inputs := f.freeze(t, SimulationInputRequest{Catalog: cat})
	keys, err := inputs.CheckpointKeys()
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := manifestEntry(inputs, SimulationFamilyCOB, "unit/fallback")
	if !ok || entry.Presence != SimulationInputFallback {
		t.Fatalf("expected fallback admission: %+v", entry)
	}
	u.Script, err = cob.Load(data)
	if err != nil {
		t.Fatal(err)
	}
	// General references still accept an equal frozen fallback. Future
	// allocations require the stronger compiled-only proof, even after a
	// caller installs that same nonempty program into the definition.
	if _, err := keys.ProgramForUnit(u, u.Script); err != nil {
		t.Fatalf("general fallback reference changed: %v", err)
	}
	if err := keys.ValidateCompiledAllocationPrograms(cat); err == nil {
		t.Fatal("fallback admitted as compiled")
	}
}

func TestCheckpointCompiledAllocationProgramsPureRepeatedValidation(t *testing.T) {
	f := newFrozenFixture(t)
	inputs := f.freeze(t, SimulationInputRequest{})
	// Promoted methods panic on use; both key construction and validation
	// must use only frozen values, even with a broken live filesystem.
	inputs.view = struct{ vfs.FSOps }{}
	keys, err := inputs.CheckpointKeys()
	if err != nil {
		t.Fatal(err)
	}
	cat := inputs.catalog
	u := cat.unitRecords[0]
	program := *u.Script
	program.Code = slices.Clone(program.Code)
	program.Scripts = maps.Clone(program.Scripts)
	program.Pieces = slices.Clone(program.Pieces)
	program.ScriptsByID = slices.Clone(program.ScriptsByID)
	u.Script = &program
	beforeUnit, beforeManifest := *u, inputs.Manifest()
	beforeProgram := programSemanticDigest(u.Script)
	beforeRecords := slices.Clone(cat.unitRecords)
	beforeDigest := inputs.digest
	f.write(t, "scripts/otherunit.cob", []byte("changed after admission"))
	for range 5 {
		if err := keys.ValidateCompiledAllocationPrograms(cat); err != nil {
			t.Fatalf("equal semantic program refused: %v", err)
		}
	}
	if !reflect.DeepEqual(beforeUnit, *u) || beforeProgram != programSemanticDigest(u.Script) ||
		!slices.Equal(beforeRecords, cat.unitRecords) || !slices.Equal(beforeManifest, inputs.Manifest()) || beforeDigest != inputs.digest {
		t.Fatal("validation changed admitted values")
	}
	// The map-only fixture route uses its existing sorted-key record view.
	cat.unitRecords = nil
	if err := keys.ValidateCompiledAllocationPrograms(cat); err != nil {
		t.Fatalf("same map-only fixture records refused: %v", err)
	}
	cat.Units["third"] = cat.Units["testunit"]
	if err := keys.ValidateCompiledAllocationPrograms(cat); err == nil {
		t.Fatal("added map-only fixture record accepted")
	}
}

func TestCheckpointCompiledAllocationProgramsRequireAdmission(t *testing.T) {
	inputs, keys := checkpointCompiledProgramsFixture(t, nil)
	for _, k := range []*CheckpointKeys{nil, {}} {
		if err := k.ValidateCompiledAllocationPrograms(inputs.catalog); err == nil {
			t.Fatal("missing keys accepted")
		}
	}
	if err := keys.ValidateCompiledAllocationPrograms(nil); err == nil {
		t.Fatal("missing catalog accepted")
	}
}

func TestCheckpointCompiledAllocationProgramsRejectPreKeyRecordRemoval(t *testing.T) {
	for _, hole := range []bool{false, true} {
		name := "truncate"
		if hole {
			name = "hole"
		}
		t.Run(name, func(t *testing.T) {
			inputs := newFrozenFixture(t).freeze(t, SimulationInputRequest{})
			if hole {
				inputs.catalog.unitRecords[1] = nil
			} else {
				inputs.catalog.unitRecords = inputs.catalog.unitRecords[:1]
			}
			if _, err := inputs.CheckpointKeys(); err == nil {
				t.Fatal("incomplete admitted record set captured in keys")
			}
		})
	}
}
