package session

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// The injected health word is retained by both the unit writer and its selected
// summary. Diagnosis must locate the first affected tick between full samples
// from detached rings alone (DESIGN_MULTIPLAYER §16.3.4 M3-C8, §16.3.80).
func TestCheckpointLiveFaultDiagnosis(t *testing.T) {
	for _, mode := range []gameplay.Mode{gameplay.Strict31, gameplay.Modern, gameplay.Community39} {
		t.Run(string(mode), func(t *testing.T) {
			histories := checkpointFaultHistories(t, mode)
			left, right := histories[0], histories[1]
			for peer, history := range histories {
				if len(history.Ticks) != 60 || len(history.Records) != 3 {
					t.Fatalf("peer %d retained %d ticks/%d records, want 60/3", peer, len(history.Ticks), len(history.Records))
				}
				for i, tick := range []uint32{0, 30, 60} {
					if history.Records[i].Position.Tick != tick {
						t.Fatalf("peer %d record %d tick=%d, want %d", peer, i, history.Records[i].Position.Tick, tick)
					}
				}
			}
			// Establish the equal retained prefix, not merely the comparator's
			// claimed onset. No off-cadence full capture exposes the injected word.
			if left.Records[0] != right.Records[0] {
				t.Fatal("entry records differ before injection")
			}
			for i := range 16 {
				if left.Ticks[i] != right.Ticks[i] {
					t.Fatalf("row %d differs before injection", i+1)
				}
			}
			for i, a := range left.Ticks {
				b := right.Ticks[i]
				if a.Position != b.Position || a.SimulationState != b.SimulationState || a.CRTState != b.CRTState ||
					a.SimulationDraws != b.SimulationDraws || a.CRTDraws != b.CRTDraws {
					t.Fatalf("health fault changed pump positions or RNG evidence at row %d", i+1)
				}
			}
			comparison, err := CompareCheckpointHistories(left, right)
			if err != nil {
				t.Fatal(err)
			}
			if comparison.ComparedTicks != 60 || comparison.ComparedRecords != 3 || comparison.MayPredateTicks {
				t.Fatalf("comparison window = %+v", comparison)
			}
			const unitOwner = int(checkpoint.OwnerUnits) - 1
			wantTick := CheckpointPosition{Tick: 17, Boundary: CheckpointFinalPumpTick, Pump: 17}
			first := comparison.TickDifference
			if first == nil || first.Position != wantTick || !first.Owners[unitOwner] || first.RNG || first.Pools || first.Full {
				t.Fatalf("first selected difference = %+v, want tick 17 naming units without RNG/pool changes", first)
			}
			wantRecord := CheckpointPosition{Tick: 30, Boundary: CheckpointFinalPumpTick, Pump: 30}
			full := comparison.RecordDifference
			if full == nil || full.Position != wantRecord || !full.Full || !full.Owners[unitOwner] || comparison.UncoveredOwners[unitOwner] {
				t.Fatalf("first full difference = %+v, uncovered units=%v; want tick 30 naming the selected unit fault", full, comparison.UncoveredOwners[unitOwner])
			}
			if left.Records[2].Digests.Full == right.Records[2].Digests.Full ||
				left.Records[2].Digests.Owners[unitOwner] == right.Records[2].Digests.Owners[unitOwner] {
				t.Fatal("unit divergence disappeared from the second cadence record")
			}
			// Other owners may observe the fault through ordinary gameplay. The
			// units owner must be named; exclusivity is not part of the contract.
			t.Logf("first row tick=%d owners=%v; first full tick=%d owners=%v", first.Position.Tick, first.Owners, full.Position.Tick, full.Owners)
		})
	}
}

// Return only detached diagnostic values, after clearing the source histories.
// The diagnosis above has no Session, actor, content-key or capture access.
func checkpointFaultHistories(t *testing.T, mode gameplay.Mode) [2]CheckpointHistory {
	t.Helper()
	var sessions [2]*Session
	var actors [2]*units.Unit
	var identity checkpoint.Identity
	for peer := range sessions {
		inputs, config := checkpointPortableInputs(t, mode)
		gotIdentity := checkpoint.Identity{Content: inputs.Digest(), Config: config.Digest()}
		if peer == 0 {
			identity = gotIdentity
		} else if gotIdentity != identity {
			t.Fatal("fault peers have different admitted content/configuration identities")
		}
		s := checkpointPortableBattle(t, inputs, config)
		sessions[peer] = s
		for _, u := range s.Units.Iter() {
			if u.Owner == uint8(s.LocalOwner) {
				if actors[peer] != nil {
					t.Fatal("fault fixture has more than one human actor")
				}
				actors[peer] = u
			}
		}
		if actors[peer] == nil || !s.PublishOpeningFrame() {
			t.Fatal("fault fixture needs a human commander and opening publication")
		}
		if err := s.EnableCheckpoints(); err != nil {
			t.Fatal(err)
		}
	}
	for tick := uint32(1); tick <= 60; tick++ {
		if tick == 17 {
			a, b := actors[0], actors[1]
			if !a.Alive || !b.Alive || a.Health <= 1 || a.Health != b.Health || a.Handle != b.Handle || a.AllocationSerial != b.AllocationSerial {
				t.Fatal("tick-17 fault requires corresponding live, equally healthy commanders")
			}
			// One explicit int32 store, before the tick-17 visit. This is a
			// diagnostic fault, not a damage packet or a new gameplay behavior.
			b.Health -= int32(1)
		}
		for peer, s := range sessions {
			s.ExecuteStep(StepPlan{run: true, ticks: 1})
			if result := s.CheckpointCaptureResult(); result.Err != nil || result.Pending {
				t.Fatalf("peer %d tick %d capture: %+v", peer, tick, result)
			}
			if s.State != StateBattle || s.Clock.GlobalTick != tick {
				t.Fatalf("peer %d left the scripted battle at tick %d", peer, tick)
			}
		}
	}
	var histories [2]CheckpointHistory
	for peer, s := range sessions {
		histories[peer] = s.CheckpointHistory()
		s.DisableCheckpoints()
		if cleared := s.CheckpointHistory(); len(cleared.Records) != 0 || len(cleared.Ticks) != 0 {
			t.Fatal("disable did not clear the live diagnostic history")
		}
	}
	return histories
}
