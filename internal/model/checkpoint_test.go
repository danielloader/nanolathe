package model

import (
	"bytes"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
)

func TestCheckpointPieceUsesPoseAndExcludesPresentationFlags(t *testing.T) {
	s := PieceState{RotX: 0x1234, RotY: 0x5678, RotZ: 0x9abc, Trans: [3]numeric.Fixed{-1, 2, 3}}
	encode := func() []byte {
		t.Helper()
		var out bytes.Buffer
		if err := s.WriteCheckpoint(checkpoint.NewEncoder(&out)); err != nil {
			t.Fatal(err)
		}
		return out.Bytes()
	}
	got := encode()
	want := []byte{0x34, 0x12, 0x78, 0x56, 0xbc, 0x9a,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		2, 0, 0, 0, 0, 0, 0, 0, 3, 0, 0, 0, 0, 0, 0, 0}
	if !bytes.Equal(got, want) {
		t.Fatalf("piece bytes %x, want %x", got, want)
	}
	s.DontCache, s.DontShade, s.DontShadow, s.Hidden = true, true, true, true
	if !bytes.Equal(encode(), got) {
		t.Fatal("presentation flags changed checkpoint")
	}
	s.Trans[2]++
	if bytes.Equal(encode(), got) {
		t.Fatal("translation missing from checkpoint")
	}
}
