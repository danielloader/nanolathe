package units

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/cob"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/model"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
)

// Entry words of orderProbeProgram's three scripts.
const (
	orderProbeSetSpeedPC = 0
	orderProbeActivatePC = 7
	orderProbeCreatePC   = 17
)

// orderProbeProgram is an authored script in which the order of the two
// creation-time deferred starts is visible. Nothing here is copied retail
// data — the opcodes are the documented COB encoding [04 §4.3][04 §4.6].
// `SetSpeed` stores its argument word in static 0; `Activate` spins piece 1 at
// the speed it reads from static 0. When `SetSpeed` runs first the spin
// carries the footprint accumulator; when `Activate` runs first it reads the
// zero the bind left and the piece never turns.
func orderProbeProgram() *cob.Program {
	return &cob.Program{
		Code: []uint32{
			// SetSpeed at word 0.
			0x10021002, 0, // push argument word zero: the footprint accumulator
			0x10023004, 0, // pop it into static 0
			0x10021001, 0, // push the return value
			0x10065000, // return
			// Activate at word 7.
			0x10021001, 0, // push acceleration 0 — immediate, no ramp
			0x10021004, 0, // push static 0 as the speed
			0x10003000, 1, model.AxisY, // spin piece 1 about Y
			0x10021001, 0,
			0x10065000,
			// Create at word 17.
			0x10021001, 0,
			0x10065000,
		},
		Scripts:     map[string]int{"SetSpeed": orderProbeSetSpeedPC, "Activate": orderProbeActivatePC, "Create": orderProbeCreatePC},
		ScriptsByID: []int{orderProbeSetSpeedPC, orderProbeActivatePC, orderProbeCreatePC},
		Pieces:      []string{"base", "arms"},
		Statics:     1,
	}
}

// TestAlreadyBuiltCreationQueuesSetSpeedBeforeActivate locks steps 5 and 6 of
// the creation-time callback sequence [04 R-CB-01 §4] for a definition that
// both extracts metal and carries `activatewhenbuilt`, created already built:
// the extraction pass starts the deferred `SetSpeed` [04 R-CB-01 §5] before
// the activation edge machine starts the deferred `Activate`
// [04 R-SPEC-01 §12], so `SetSpeed` holds the lower thread slot and runs first
// in the unit's first normal drain. Both creators run the sequence.
func TestAlreadyBuiltCreationQueuesSetSpeedBeforeActivate(t *testing.T) {
	const surfaceMetal = 9
	const footprint = 3
	const cell = int32(4)
	// Σ(byte + 1) over the nine covered cells [05 R-PROD-01 §6].
	const footprintSum = (surfaceMetal + 1) * footprint * footprint

	creators := []struct {
		name   string
		create func(w *World, def *content.UnitDef, at numeric.Fixed) (pool.Handle, error)
	}{
		{"general", func(w *World, def *content.UnitDef, at numeric.Fixed) (pool.Handle, error) {
			return w.Create(def, 0, at, 0, at)
		}},
		{"forced-slot", func(w *World, def *content.UnitDef, at numeric.Fixed) (pool.Handle, error) {
			return w.CreateWithForcedSlot(def, 0, at, 0, at, 3)
		}},
	}
	for _, creator := range creators {
		t.Run(creator.name, func(t *testing.T) {
			def := &content.UnitDef{
				DefinitionHeader:  content.DefinitionHeader{CanonicalKey: content.CanonicalKey("orderprobe")},
				UnitName:          "orderprobe",
				MaxDamage:         100,
				Limit:             -1,
				ExtractsMetal:     1,
				FootprintX:        footprint,
				FootprintZ:        footprint,
				ActivateWhenBuilt: true,
				Script:            orderProbeProgram(),
			}
			cat := &content.Catalog{Units: map[string]*content.UnitDef{def.CanonicalKey: def}}
			w := newFixtureWorld(4, cat)
			w.SetExtractionSampler(seededTerrain(t, surfaceMetal))

			h, err := creator.create(w, def, cellCentre(cell))
			if err != nil {
				t.Fatalf("create: %v", err)
			}
			vm := w.Unit(h).GetScript()
			if vm == nil {
				t.Fatal("fixture unit has no script VM")
			}

			// Neither start runs inside Create's immediate drain, so both are
			// still queued here; the lower slot runs first [04 §4.2].
			setSpeedSlot, activateSlot := -1, -1
			for i := range vm.Threads {
				if vm.Threads[i].Status == cob.ThreadIdle {
					continue
				}
				switch vm.Threads[i].PC {
				case orderProbeSetSpeedPC:
					setSpeedSlot = i
				case orderProbeActivatePC:
					activateSlot = i
				}
			}
			if setSpeedSlot < 0 || activateSlot < 0 {
				t.Fatalf("queued starts: SetSpeed slot %d, Activate slot %d; want both queued at creation", setSpeedSlot, activateSlot)
			}
			if setSpeedSlot > activateSlot {
				t.Fatalf("SetSpeed queued in slot %d after Activate in slot %d; want extraction (step 5) before activation (step 6) [04 R-CB-01 §4]", setSpeedSlot, activateSlot)
			}

			// The consequence: Activate reads the accumulator SetSpeed stored, so
			// the piece turns footprintSum/30 per drained tick [04 §4.6].
			before := vm.Pieces[1].RotY
			for i := 0; i < 10; i++ {
				vm.Drain(1)
			}
			if got, want := vm.Pieces[1].RotY-before, uint16(footprintSum/30*10); got != want {
				t.Fatalf("piece turned %d over ten ticks, want %d: Activate did not see SetSpeed's accumulator %d", got, want, footprintSum)
			}
		})
	}
}
