package world

import (
	"bytes"
	"encoding/hex"
	"math"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

func TestCheckpointWindStoredVectorAndExclusions(t *testing.T) {
	w := Wind{Changed: true, DirX: -1, DirZ: 2, Heading: 0x1234, Max: 3, Min: -4, NextChange: 5, Scalar: math.Float32frombits(1 << 31), Strength: -6}
	want, _ := hex.DecodeString("01ffffffff02000000341203000000fcffffff0500000000000080faffffff")
	var b bytes.Buffer
	if err := w.WriteCheckpoint(checkpoint.NewEncoder(&b)); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b.Bytes(), want) {
		t.Fatalf("payload=%x want %x", b.Bytes(), want)
	}
	w.LastChange, w.BriefingCountdown = 99, -100
	before := w
	b.Reset()
	if err := w.WriteCheckpoint(checkpoint.NewEncoder(&b)); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b.Bytes(), want) || w != before {
		t.Fatal("capture read excluded fields or changed wind")
	}
}

func TestCheckpointWindSummaryAndNaN(t *testing.T) {
	w := Wind{Strength: -1, Heading: 2, Scalar: math.Float32frombits(3), NextChange: 4}
	var s checkpoint.Summary
	if err := w.AppendCheckpointSummary(&s); err != nil {
		t.Fatal(err)
	}
	if count, sum := s.Result(); count != 4 || sum != 28 {
		t.Fatalf("summary=%d,%d want4,28", count, sum)
	}
	before := s
	w.Scalar = math.Float32frombits(0x7fc00001)
	if err := w.AppendCheckpointSummary(&s); err == nil || s != before {
		t.Fatal("NaN summary did not fail atomically")
	}
	var b bytes.Buffer
	if err := w.WriteCheckpoint(checkpoint.NewEncoder(&b)); err == nil {
		t.Fatal("NaN full capture succeeded")
	}
}
