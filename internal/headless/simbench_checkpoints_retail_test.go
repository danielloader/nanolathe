//go:build retail

package headless

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/session"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport/retailcat"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// This is a correctness comparison, not a timing acceptance run. Both paths
// use the ordinary three-army fixture and exactly the same Step cadence.
func TestSimBenchCheckpointsMatchOrdinaryRetail(t *testing.T) {
	catalog, fs := retailcat.Shared(t)
	opts := SimBenchOptions{Gameplay: gameplay.Modern, Seed: 7, Difficulty: 1,
		WarmupTicks: 30, MeasureTicks: 31, CensusCount: 3}
	ordinary, err := runSimBenchmarkWithContent(opts, fs, catalog, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	opts.Checkpoints = true
	enabled, err := runSimBenchmarkWithContent(opts, fs, catalog, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if ordinary.Checkpoints || ordinary.CheckpointRecords != 0 || ordinary.CheckpointTicks != 0 || ordinary.CheckpointDigest != "" {
		t.Fatal("ordinary benchmark claimed full checkpoint capture")
	}
	if ordinary.InitialFingerprint != enabled.InitialFingerprint || ordinary.WarmFingerprint != enabled.WarmFingerprint || ordinary.FinalFingerprint != enabled.FinalFingerprint {
		t.Fatalf("checkpoint composition changed partial fingerprints: initial %s/%s, warm %s/%s, final %s/%s",
			ordinary.InitialFingerprint, enabled.InitialFingerprint, ordinary.WarmFingerprint, enabled.WarmFingerprint, ordinary.FinalFingerprint, enabled.FinalFingerprint)
	}
	if ordinary.SimulationDraws != enabled.SimulationDraws || ordinary.CRTDraws != enabled.CRTDraws {
		t.Fatalf("checkpoint composition changed RNG draws: %d/%d versus %d/%d", ordinary.SimulationDraws, ordinary.CRTDraws, enabled.SimulationDraws, enabled.CRTDraws)
	}
	// Scene carries private live factory pointers; compare its report values,
	// not the independently composed object graphs behind those pointers.
	ordinaryScene, err := json.Marshal(ordinary.Scene)
	if err != nil {
		t.Fatal(err)
	}
	enabledScene, err := json.Marshal(enabled.Scene)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ordinaryScene, enabledScene) {
		t.Fatalf("checkpoint composition changed scene: ordinary=%s enabled=%s", ordinaryScene, enabledScene)
	}
	if !reflect.DeepEqual(ordinary.Census, enabled.Census) {
		t.Fatalf("checkpoint composition changed census: ordinary=%+v enabled=%+v", ordinary.Census, enabled.Census)
	}
	if !enabled.Checkpoints || enabled.CheckpointRecords < 2 || enabled.CheckpointTicks == 0 || enabled.CheckpointTick == 0 || enabled.CheckpointTick%30 != 0 {
		t.Fatalf("missing runtime checkpoint evidence: records=%d rows=%d tick=%d", enabled.CheckpointRecords, enabled.CheckpointTicks, enabled.CheckpointTick)
	}
	lastTick := enabled.Census[len(enabled.Census)-1].Tick
	if uint32(enabled.CheckpointTicks) != lastTick || enabled.CheckpointTick > lastTick {
		t.Fatalf("history is not the played window: rows=%d full tick=%d final=%d", enabled.CheckpointTicks, enabled.CheckpointTick, lastTick)
	}
	if digest, err := hex.DecodeString(enabled.CheckpointDigest); err != nil || len(digest) != 32 {
		t.Fatalf("full digest = %q, error %v", enabled.CheckpointDigest, err)
	}
	data, err := json.Marshal(enabled)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"checkpoints", "checkpoint_records", "checkpoint_ticks", "checkpoint_tick", "checkpoint_digest"} {
		if !strings.Contains(string(data), "\""+name+"\":") {
			t.Fatalf("report omits %s", name)
		}
	}
}

func TestSimBenchCheckpointCensusAndFailureRetail(t *testing.T) {
	catalog, fs := retailcat.Shared(t)
	battle, scene, err := ComposeSimBenchBattle(SimBenchOptions{Gameplay: gameplay.Modern, Seed: 7, Difficulty: 1, Checkpoints: true}, fs, catalog)
	if err != nil {
		t.Fatal(err)
	}
	s := battle.Session
	entry := s.CheckpointHistory()
	if len(entry.Records) != 1 || len(entry.Ticks) != 0 || entry.Records[0].Position.Boundary != session.CheckpointEntry {
		t.Fatal("composition did not capture the staged, unticked opening")
	}
	deaths := simBenchDeathCounter(s)
	victim := s.Units.FirstLive(func(u *units.Unit) bool { return u.Owner == 1 && u.Def != nil && !u.Def.Commander })
	if victim == nil {
		t.Fatal("fixture has no victim")
	}
	s.Units.Destroy(victim.Handle, units.DeathKilled)
	for s.Clock.GlobalTick < 30 {
		simBenchStep(s)
		if err := s.CheckpointCaptureResult().Err; err != nil {
			t.Fatal(err)
		}
	}
	census := simBenchTakeCensus(s, scene, "test", deaths)
	if census.DeathsSoFar < 1 || uint64(census.DeathsSoFar) != s.Units.DeathDispatches()-*deaths {
		t.Fatalf("death dispatch census = %d", census.DeathsSoFar)
	}
	// Selected geometry admission must still fail after setup; the benchmark
	// reports the failure instead of downgrading to partial fingerprints.
	s.World.SeaLevel++
	if err := s.RequestCheckpointCapture(io.Discard); err != nil {
		t.Fatal(err)
	}
	simBenchStep(s)
	failure := s.CheckpointCaptureResult().Err
	if failure == nil {
		t.Fatal("mutated immutable terrain escaped capture")
	}
	var report SimBenchReport
	if err := simBenchCheckpointReport(s, &report); err != failure {
		t.Fatalf("report error = %v, want %v", err, failure)
	}
}

func TestSimBenchCheckpointUnsupportedScenesRetail(t *testing.T) {
	catalog, fs := retailcat.Shared(t)
	_, _, err := ComposeSimBenchBattle(SimBenchOptions{Gameplay: gameplay.Strict31, Seed: 7, Difficulty: 1, Checkpoints: true}, fs, catalog)
	if err == nil {
		t.Fatal("Strict's one-computer-per-human admission was bypassed")
	}
	_, _, err = ComposeSimBenchBattle(SimBenchOptions{Gameplay: gameplay.Modern, ArmySize: 334, Checkpoints: true}, fs, catalog)
	if err == nil || !strings.Contains(err.Error(), "enlarged relocation") {
		t.Fatalf("enlarged checkpoint fixture error = %v", err)
	}
	var report SimBenchReport
	if err := simBenchCheckpointReport(&session.Session{}, &report); err == nil {
		t.Fatal("unenabled session reported checkpoint evidence")
	}
}
