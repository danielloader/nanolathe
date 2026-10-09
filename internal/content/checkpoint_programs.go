package content

import "fmt"

// The slice position preserves manifest ordinals and holes observed when the
// keys are built, after the complete freeze-time snapshot was validated.
// That snapshot also proves original holes and trailing length (§16.3.63).
// Names and allocator metadata are copied because the definitions
// remain externally reachable after admission.
type checkpointAllocationRecord struct {
	unit         *UnitDef
	unitName     string
	canonicalKey string
	unitDefID    uint32
}

// ValidateCompiledAllocationPrograms proves that every admitted record still
// has its compiled, nonempty program, including records never allocated. This
// is the bounded future-allocation subset in DESIGN_MULTIPLAYER §16.3.56; it
// does not read files or caches or admit the general COB fallback path.
// UnitDefID is retained allocator metadata, separate from the manifest ordinal;
// neither is normalized. Catalog pointer ownership belongs to the caller.
func (k *CheckpointKeys) ValidateCompiledAllocationPrograms(c *Catalog) error {
	if k == nil || k.allocationRecords == nil || c == nil {
		return checkpointReferenceError("<catalog>", "the admitted catalog records and checkpoint keys")
	}
	records := c.unitRecordView()
	if len(records) != len(k.allocationRecords) {
		return checkpointReferenceError("<catalog>", "the admitted unit record count, including absent slots")
	}
	for index, admitted := range k.allocationRecords {
		unit := records[index]
		if unit != admitted.unit {
			return checkpointReferenceError(fmt.Sprintf("unit-record/%d", index+1), "the admitted record at its original ordinal")
		}
		if unit == nil {
			continue
		}
		if unit.UnitName != admitted.unitName || unit.CanonicalKey != admitted.canonicalKey || unit.UnitDefID != admitted.unitDefID {
			return checkpointReferenceError("unit/"+admitted.canonicalKey, "the admitted unit names and allocator identity")
		}
		entry, ok := k.programs[unit]
		if !ok || entry.Presence != SimulationInputPresent {
			return checkpointReferenceError("unit/"+admitted.canonicalKey, "an admitted compiled program without fallback")
		}
		if !usableProgram(unit.Script) || programSemanticDigest(unit.Script) != entry.SemanticDigest {
			return checkpointReferenceError("unit/"+admitted.canonicalKey, "the nonempty compiled program with its admitted semantic digest")
		}
	}
	return nil
}
