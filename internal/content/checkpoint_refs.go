package content

// Checkpoint references observe admitted content; they never load, prepare or
// normalize battle definitions (DESIGN_MULTIPLAYER §16.3.6, M3-C3).

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/nanolathe-gg/nanolathe/internal/cob"
	"github.com/nanolathe-gg/nanolathe/internal/model"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// CheckpointKeys resolves immutable battle objects to their manifest records.
// Definitions remain immutable by the SimulationInputs contract. The maps are
// lookup-only: their iteration order never enters a checkpoint.
type CheckpointKeys struct {
	inputSnapshot     *checkpointInputSnapshot
	units             map[*UnitDef]checkpoint.Definition
	weapons           map[*WeaponDef]checkpoint.Definition
	features          map[*FeatureDef]checkpoint.Definition
	models            map[*model.Model]checkpoint.Definition
	programs          map[*UnitDef]SimulationInput
	allocationRecords []checkpointAllocationRecord
	sight             *SightShapes
	los               *LOSTables
	sequences         map[string]checkpointSequence
}

type checkpointSequence struct {
	ref    checkpoint.Definition
	delays []int32
}

// CheckpointKeys builds references from this battle's frozen values, including
// retained equal-name unit records. It reads no filesystem and does not repair
// an incomplete admission (DESIGN_MULTIPLAYER §16.3.6).
func (in *SimulationInputs) CheckpointKeys() (*CheckpointKeys, error) {
	if in == nil || in.catalog == nil || in.view == nil || len(in.manifest) == 0 {
		return nil, checkpointReferenceError("<inputs>", "completed frozen simulation inputs")
	}
	if err := in.ValidateCheckpointInputs(); err != nil {
		return nil, err
	}
	if err := validateSimulationInputs(in.manifest); err != nil {
		return nil, err
	}
	if simulationManifestDigest(in.manifest) != in.digest {
		return nil, checkpointReferenceError("<manifest>", "the frozen content digest")
	}
	entries := make(map[checkpoint.Definition]SimulationInput, len(in.manifest))
	for _, entry := range in.manifest {
		ref := checkpointDefinition(entry)
		if _, exists := entries[ref]; exists {
			return nil, checkpointReferenceError(ref.Key, "one manifest record per identity")
		}
		entries[ref] = entry
	}
	present := func(ref checkpoint.Definition, digest [32]byte) error {
		entry, ok := entries[ref]
		if !ok || entry.Presence != SimulationInputPresent || entry.SemanticDigest != digest {
			return checkpointReferenceError(ref.Key, "the admitted record and semantic digest")
		}
		return nil
	}
	records := in.catalog.unitRecordView()
	// A removed record would otherwise disappear from all the loops below.
	// Prove the manifest's complete nonnil unit set before retaining the
	// current layout for future-allocation validation (§16.3.56).
	for _, entry := range in.manifest {
		if entry.Family != SimulationFamilyCOB && (entry.Family != SimulationFamilyCatalog || !strings.HasPrefix(entry.Key, "unit/")) {
			continue
		}
		if entry.Ordinal == 0 || uint64(entry.Ordinal) > uint64(len(records)) {
			return nil, checkpointReferenceError(entry.Key, "every admitted unit record at its manifest ordinal")
		}
		unit := records[int(entry.Ordinal)-1]
		if unit == nil || entry.Key != "unit/"+unit.CanonicalKey {
			return nil, checkpointReferenceError(entry.Key, "every admitted unit record at its manifest ordinal")
		}
	}
	keys := &CheckpointKeys{
		inputSnapshot: in.checkpointInputs,
		units:         make(map[*UnitDef]checkpoint.Definition), weapons: make(map[*WeaponDef]checkpoint.Definition),
		features: make(map[*FeatureDef]checkpoint.Definition), models: make(map[*model.Model]checkpoint.Definition),
		programs:          make(map[*UnitDef]SimulationInput),
		allocationRecords: make([]checkpointAllocationRecord, len(records)),
	}
	for index, unit := range records {
		if unit == nil {
			continue
		}
		if uint64(index) >= uint64(^uint32(0)) {
			return nil, checkpointReferenceError("unit/"+unit.CanonicalKey, "a u32 manifest ordinal")
		}
		ref := checkpoint.Definition{Family: SimulationFamilyCatalog, Ordinal: uint32(index + 1), Key: "unit/" + unit.CanonicalKey}
		if err := present(ref, unitSemanticDigest(unit)); err != nil {
			return nil, err
		}
		if err := addCheckpointKey(keys.units, unit, ref); err != nil {
			return nil, err
		}
		ref.Family = SimulationFamilyCOB
		program, ok := entries[ref]
		if !ok {
			return nil, checkpointReferenceError(ref.Key, "the unit's admitted COB record, including a defined absence")
		}
		keys.programs[unit] = program
		keys.allocationRecords[index] = checkpointAllocationRecord{
			unit: unit, unitName: unit.UnitName, canonicalKey: unit.CanonicalKey, unitDefID: unit.UnitDefID,
		}
	}
	for _, name := range sortedKeys(in.catalog.Weapons) {
		weapon := in.catalog.Weapons[name]
		if weapon == nil {
			continue
		}
		var ordinal uint32
		if weapon.ID >= 0 {
			ordinal = uint32(weapon.ID)
		}
		ref := checkpoint.Definition{Family: SimulationFamilyCatalog, Ordinal: ordinal, Key: "weapon/" + name}
		if err := present(ref, weaponSemanticDigest(weapon)); err != nil {
			return nil, err
		}
		if err := addCheckpointKey(keys.weapons, weapon, ref); err != nil {
			return nil, err
		}
	}
	for _, name := range sortedKeys(in.catalog.Features) {
		feature := in.catalog.Features[name]
		if feature == nil {
			continue
		}
		ref := checkpoint.Definition{Family: SimulationFamilyCatalog, Key: "feature/" + name}
		if err := present(ref, featureSemanticDigest(feature)); err != nil {
			return nil, err
		}
		if err := addCheckpointKey(keys.features, feature, ref); err != nil {
			return nil, err
		}
	}
	for _, logical := range sortedKeys(in.models) {
		mdl := in.models[logical]
		top, ok := in.modelTops[logical]
		if mdl == nil || !ok {
			return nil, checkpointReferenceError(logical, "the frozen parsed model and authored model height")
		}
		ref := checkpoint.Definition{Family: SimulationFamilyModel, Key: snapshotKey(logical)}
		if err := present(ref, modelSemanticDigest(mdl, top)); err != nil {
			return nil, err
		}
		if err := addCheckpointKey(keys.models, mdl, ref); err != nil {
			return nil, err
		}
	}
	// The frozen catalog owns both visibility tables. The manifest already
	// identifies them independently; admit their actual objects, not copies
	// that happen to advertise the same names or hashes.
	for _, entry := range catalogEntries(in.catalog) {
		if entry.Key != "los" && entry.Key != "sightshapes" {
			continue
		}
		ref := checkpointDefinition(entry)
		admitted, ok := entries[ref]
		if !ok || entry != admitted {
			return nil, checkpointReferenceError(entry.Key, "the admitted visibility table record")
		}
	}
	keys.sight, keys.los = in.catalog.Sight, in.catalog.LOS
	keys.sequences = make(map[string]checkpointSequence)
	for _, entry := range simArtEntries(in.simArt) {
		if !strings.HasPrefix(entry.Key, "sequence/") {
			continue
		}
		ref := checkpointDefinition(entry)
		admitted, ok := entries[ref]
		if !ok || entry != admitted {
			return nil, checkpointReferenceError(entry.Key, "the admitted feature sequence record")
		}
		name := strings.TrimPrefix(entry.Key, "sequence/")
		sequence := in.simArt.sequences[name]
		var delays []int32
		if sequence != nil {
			delays = make([]int32, len(sequence.frames))
			for i, frame := range sequence.frames {
				delays[i] = frame.delay
			}
		}
		keys.sequences[name] = checkpointSequence{ref: ref, delays: delays}
	}
	return keys, nil
}

func checkpointDefinition(entry SimulationInput) checkpoint.Definition {
	return checkpoint.Definition{Family: entry.Family, Ordinal: entry.Ordinal, Key: entry.Key}
}

func checkpointReferenceError(logical, expected string) error {
	return fmt.Errorf("nanolathe: checkpoint reference failed: logical path %s, providers searched [frozen simulation inputs], expected %s", logical, expected)
}

func addCheckpointKey[T comparable](table map[T]checkpoint.Definition, value T, ref checkpoint.Definition) error {
	if previous, exists := table[value]; exists && previous != ref {
		return checkpointReferenceError(ref.Key, "an unambiguous admitted object identity")
	}
	table[value] = ref
	return nil
}

// Unit resolves the actual retained record, never the catalog's first name hit.
func (k *CheckpointKeys) Unit(v *UnitDef) (checkpoint.Definition, error) {
	if k != nil && v != nil {
		if ref, ok := k.units[v]; ok {
			return ref, nil
		}
	}
	return checkpoint.Definition{}, checkpointReferenceError("<unit>", "an admitted unit object")
}

// Weapon resolves a weapon record including its manifest slot ordinal.
func (k *CheckpointKeys) Weapon(v *WeaponDef) (checkpoint.Definition, error) {
	if k != nil && v != nil {
		if ref, ok := k.weapons[v]; ok {
			return ref, nil
		}
	}
	return checkpoint.Definition{}, checkpointReferenceError("<weapon>", "an admitted weapon object")
}

// Feature resolves an admitted base definition. Normalized copies need their
// base relation and are checked by NormalizedFeature instead.
func (k *CheckpointKeys) Feature(v *FeatureDef) (checkpoint.Definition, error) {
	if k != nil && v != nil {
		if ref, ok := k.features[v]; ok {
			return ref, nil
		}
	}
	return checkpoint.Definition{}, checkpointReferenceError("<feature>", "an admitted base feature object")
}

// Model resolves the frozen parsed model used by unit creation. Models whose
// files were only validated for presentation have no parsed binding here.
func (k *CheckpointKeys) Model(v *model.Model) (checkpoint.Definition, error) {
	if k != nil && v != nil {
		if ref, ok := k.models[v]; ok {
			return ref, nil
		}
	}
	return checkpoint.Definition{}, checkpointReferenceError("<model>", "an admitted parsed model object")
}

// ProgramForUnit validates the program against that particular unit record's
// admitted COB semantics. The strict fallback binder recompiles sealed bytes
// into a fresh pointer; pointer equality is not its content identity.
func (k *CheckpointKeys) ProgramForUnit(unit *UnitDef, v *cob.Program) (checkpoint.Definition, error) {
	if k != nil && unit != nil && v != nil {
		if entry, ok := k.programs[unit]; ok && entry.Presence != SimulationInputAbsent &&
			usableProgram(v) && programSemanticDigest(v) == entry.SemanticDigest {
			return checkpointDefinition(entry), nil
		}
	}
	return checkpoint.Definition{}, checkpointReferenceError("<program>", "the admitted program for this unit record")
}

// CheckpointFeature is the closed feature-reference variant from
// DESIGN_MULTIPLAYER §16.3.6: 1 is an admitted base, 2 is a normalized copy.
// Only variant 2 writes the five normalization result words.
type CheckpointFeature struct {
	Variant                                       uint8
	Base                                          checkpoint.Definition
	FootprintX, FootprintZ, Damage, Metal, Energy int32
}

// NormalizedFeature checks the exact existing features.NormalizeDef transform
// without importing that upper package or mutating either definition. U3 must
// retain the base relation at creation; a matching name does not establish it.
func (k *CheckpointKeys) NormalizedFeature(base, value *FeatureDef) (CheckpointFeature, error) {
	ref, err := k.Feature(base)
	if err != nil {
		return CheckpointFeature{}, err
	}
	bad := func() (CheckpointFeature, error) {
		return CheckpointFeature{}, checkpointReferenceError(ref.Key, "the base's unchanged semantic fields and existing five-field normalization")
	}
	if value == nil {
		return bad()
	}
	// A well-formed definition is returned unchanged, including any negative
	// resource/damage fields: NormalizeDef's malformed gate comes first.
	if base.CanonicalKey != "" && base.FootprintX > 0 && base.FootprintZ > 0 {
		if value != base {
			return bad()
		}
		return CheckpointFeature{Variant: 1, Base: ref}, nil
	}
	expected := *base
	expected.FootprintX = max(1, expected.FootprintX)
	expected.FootprintZ = max(1, expected.FootprintZ)
	expected.Damage = max(0, expected.Damage)
	expected.Metal = max(0, expected.Metal)
	expected.Energy = max(0, expected.Energy)
	if !sameCheckpointFeature(&expected, value) {
		return bad()
	}
	return CheckpointFeature{
		Variant: 2, Base: ref, FootprintX: value.FootprintX, FootprintZ: value.FootprintZ,
		Damage: value.Damage, Metal: value.Metal, Energy: value.Energy,
	}, nil
}

// Provenance is host-local metadata. Everything else in the shallow-copy
// contract is compared explicitly, including successor object identity and
// inert authored keys. No reflection or hash that trusts an old Hash string
// can establish that the normalization changed only its five owned fields.
func sameCheckpointFeature(a, b *FeatureDef) bool {
	return a.CanonicalKey == b.CanonicalKey && a.Hash == b.Hash &&
		a.Description == b.Description && a.FootprintX == b.FootprintX && a.FootprintZ == b.FootprintZ && a.Height == b.Height &&
		a.Object == b.Object && a.Filename == b.Filename && a.SeqName == b.SeqName && a.SeqNameShad == b.SeqNameShad &&
		a.SeqNameBurn == b.SeqNameBurn && a.SeqNameBurnShad == b.SeqNameBurnShad &&
		a.SeqNameDie == b.SeqNameDie && a.SeqNameDieShad == b.SeqNameDieShad &&
		a.SeqNameReclamate == b.SeqNameReclamate && a.SeqNameReclamateShad == b.SeqNameReclamateShad &&
		a.Metal == b.Metal && a.Energy == b.Energy && a.Damage == b.Damage &&
		a.SpreadChance == b.SpreadChance && a.Reproduce == b.Reproduce && a.ReproduceArea == b.ReproduceArea &&
		a.SparkTime == b.SparkTime && a.BurnWeapon == b.BurnWeapon && a.Animating == b.Animating &&
		a.AnimTrans == b.AnimTrans && a.ShadTrans == b.ShadTrans && a.Flamable == b.Flamable &&
		a.Geothermal == b.Geothermal && a.Blocking == b.Blocking && a.Reclaimable == b.Reclaimable &&
		a.Autoreclaimable == b.Autoreclaimable && a.Indestructible == b.Indestructible &&
		a.NoDisplayInfo == b.NoDisplayInfo && a.NoDrawUnderGray == b.NoDrawUnderGray &&
		a.FeatureDead == b.FeatureDead && a.FeatureReclamate == b.FeatureReclamate && a.FeatureBurnt == b.FeatureBurnt &&
		a.FeatureDeadDef == b.FeatureDeadDef && a.FeatureReclamateDef == b.FeatureReclamateDef && a.FeatureBurntDef == b.FeatureBurntDef &&
		maps.Equal(a.Unknown, b.Unknown)
}

// SightShapes resolves the admitted sprite-mask table by object identity.
// Nil remains the caller's explicit absence (DESIGN_MULTIPLAYER §16.3.6).
func (k *CheckpointKeys) SightShapes(value *SightShapes) (checkpoint.Definition, error) {
	if k != nil && value != nil && value == k.sight {
		return checkpoint.Definition{Family: SimulationFamilyCatalog, Key: "sightshapes"}, nil
	}
	return checkpoint.Definition{}, checkpointReferenceError("sightshapes", "the admitted sprite-mask table")
}

// LOSTables resolves the admitted ray table, including its authored count.
func (k *CheckpointKeys) LOSTables(value *LOSTables) (checkpoint.Definition, error) {
	if k != nil && value != nil && value == k.los {
		return checkpoint.Definition{Family: SimulationFamilyCatalog, Key: "los"}, nil
	}
	return checkpoint.Definition{}, checkpointReferenceError("los", "the admitted terrain-ray table")
}

// FeatureSequence resolves a live event cursor's sequence and verifies its
// delay words against the frozen metadata. SequenceFrames returns detached
// delay slices, so equality is by value, not slice address. Neither lookup nor
// validation reads a file or calls a producer (DESIGN_MULTIPLAYER §16.3.6).
func (k *CheckpointKeys) FeatureSequence(filename, sequence string, delays []int32) (checkpoint.Definition, error) {
	name := simArtSequenceKey(filename, sequence)
	if k != nil && delays != nil {
		if value, ok := k.sequences[name]; ok && len(value.delays) != 0 && slices.Equal(value.delays, delays) {
			return value.ref, nil
		}
	}
	return checkpoint.Definition{}, checkpointReferenceError("sequence/"+name, "an admitted feature sequence with its exact delay words")
}

// FeatureSequenceAbsent verifies the no-sequence result cached by features.
// The metadata consumer also reports absence for a blank argument or a
// resolved zero-frame sequence. An arbitrary unrequested name is not an
// admitted absence (DESIGN_MULTIPLAYER §16.3.11).
func (k *CheckpointKeys) FeatureSequenceAbsent(filename, sequence string) error {
	if k != nil {
		if trimTDFSemantic(filename) == "" || trimTDFSemantic(sequence) == "" {
			return nil
		}
		if value, ok := k.sequences[simArtSequenceKey(filename, sequence)]; ok && len(value.delays) == 0 {
			return nil
		}
	}
	return checkpointReferenceError("sequence/"+simArtSequenceKey(filename, sequence), "the admitted no-sequence result")
}
