//go:build retail

package session

import (
	"bytes"
	"crypto/sha256"
	"os"
	"reflect"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/platform/benchlock"
)

// Opt-in exploratory measurements, separate from correctness gates. Entry and
// warmup are outside every measured operation (DESIGN_MULTIPLAYER §16.3.79).
func TestCheckpointCostRetail(t *testing.T) {
	if os.Getenv("NANOLATHE_CHECKPOINT_COST") != "1" {
		t.Skip("set NANOLATHE_CHECKPOINT_COST=1 for checkpoint cost measurements")
	}
	lockPath, err := benchlock.Path()
	if err != nil {
		t.Fatal(err)
	}
	held, err := benchlock.Acquire(lockPath, func() { t.Log("waiting for benchmark lock") })
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	for _, scene := range []struct {
		name  string
		warm  int
		build func(*testing.T) *Session
	}{
		{"ordinary-modern-classic", 900, checkpointLifecycleBattle},
		{"busy-modern-classic-v1", 300, checkpointCostBusyBattle},
	} {
		t.Run(scene.name, func(t *testing.T) {
			s := scene.build(t)
			t.Cleanup(s.closeAIControllers)
			enableLifecycleBattle(t, s)
			for range scene.warm {
				s.ExecuteStep(StepPlan{run: true, ticks: 1})
				if err := s.CheckpointCaptureResult().Err; err != nil {
					t.Fatal(err)
				}
			}
			if s.State != StateBattle || s.Clock.GlobalTick != uint32(scene.warm) {
				t.Fatalf("cost scene ended during warmup: state=%v tick=%d", s.State, s.Clock.GlobalTick)
			}
			census := checkpointCostCount(s)
			if scene.name == "busy-modern-classic-v1" && (census.Live[0]+census.Live[1] < 100 ||
				census.Moving[0]+census.Moving[1] == 0 || census.Routes[0]+census.Routes[1] == 0 ||
				census.Nanoframes[0]+census.Nanoframes[1] == 0 || census.Builds[0]+census.Builds[1] < 2) {
				t.Fatalf("busy cost scene lost its workload: %+v", census)
			}
			measureCheckpointCost(t, s, scene.name, census)
		})
	}
}

func measureCheckpointCost(t *testing.T, s *Session, scene string, census checkpointCostCensus) {
	t.Helper()
	history := s.CheckpointHistory()
	position := history.Records[len(history.Records)-1].Position
	var encoded bytes.Buffer
	beforeSim, beforeCRT := *s.SimRNG(), *s.CrtRNG()
	baseline, err := s.captureCheckpoint(s.checkpoints.keys, position, &encoded)
	if err != nil {
		t.Fatal(err)
	}
	if baseline.Digests.Full != sha256.Sum256(encoded.Bytes()) {
		t.Fatal("cost baseline byte capture differs from full digest")
	}
	selected, err := s.checkpointRingRow(position)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("scene=%s map=%q terrain=%dx%d gameplay=%s seeds=%d/%d tick=%d position=%+v census=%+v canonical-bytes=%d retained-ring-bytes=%d full=%x sim=%d/%d crt=%d/%d",
		scene, s.Skirmish.MapName, s.World.CellW, s.World.CellH, s.Gameplay, s.RNGSimSeed, s.RNGCrtSeed, s.Clock.GlobalTick, position, census,
		encoded.Len(), reflect.TypeOf(checkpointHistoryRing{}).Size(), baseline.Digests.Full, s.SimRNG().State, s.SimRNG().Draws(), s.CrtRNG().State, s.CrtRNG().Draws())
	for _, op := range []struct {
		name string
		run  func() error
	}{
		{"digest", func() error { _, err := s.captureCheckpoint(s.checkpoints.keys, position, nil); return err }},
		{"bytes-reused-buffer", func() error {
			encoded.Reset()
			_, err := s.captureCheckpoint(s.checkpoints.keys, position, &encoded)
			return err
		}},
		{"selected-row", func() error { _, err := s.checkpointRingRow(position); return err }},
	} {
		var captureErr error
		result := testing.Benchmark(func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if err := op.run(); err != nil {
					captureErr = err
					b.Fatal(err)
				}
			}
		})
		if captureErr != nil {
			t.Fatal(captureErr)
		}
		t.Logf("%s: %s; %s", op.name, result.String(), result.MemString())
	}
	after, err := s.captureCheckpoint(s.checkpoints.keys, position, nil)
	if err != nil {
		t.Fatal(err)
	}
	row, err := s.checkpointRingRow(position)
	if err != nil {
		t.Fatal(err)
	}
	if *s.SimRNG() != beforeSim || *s.CrtRNG() != beforeCRT || after != baseline || row != selected ||
		sha256.Sum256(encoded.Bytes()) != baseline.Digests.Full || checkpointCostCount(s) != census || !reflect.DeepEqual(history, s.CheckpointHistory()) {
		t.Fatal("cost measurement changed RNG, world, selected row or retained history")
	}
}
