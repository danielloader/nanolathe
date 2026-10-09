package ai

import (
	"bytes"
	"encoding/hex"
	"reflect"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

func survivalInfoCheckpointBytes(t *testing.T, s *SurvivalInfo) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := s.WriteCheckpoint(checkpoint.NewEncoder(&b)); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestCheckpointSurvivalInfoVectorAndWarningOrder(t *testing.T) {
	s := &SurvivalInfo{Attacker: 9, CentreX: -2, CentreZ: 3, Computer: []bool{true, false}, Starts: [][2]int32{{4, -5}}, Team: []uint8{2, 0, 1}}
	warnings := []SurvivalWarning{
		{Arrive: 0x11223344, Groups: []SurvivalApproach{{Angle: 0xabcd, Domain: "MiX", X: -6, Z: 7}, {Angle: 8, Domain: "", X: 9, Z: 10}}, Tick: ^uint32(0), Wave: -11},
		{Arrive: 12, Tick: 13, Wave: 14},
	}
	s.warnings.Store(&warnings)
	// Authored bytes: list order is retained even when warning ticks decrease.
	want, _ := hex.DecodeString("09feffffff030000000200000001000100000004000000fbffffff03000000020001" +
		"01020000004433221102000000cdab030000004d6958faffffff07000000080000000000090000000a000000fffffffff5ffffff" +
		"0c000000000000000d0000000e000000")
	if got := survivalInfoCheckpointBytes(t, s); !bytes.Equal(got, want) {
		t.Fatalf("got %x want %x", got, want)
	}
	if s.warnings.Load() != &warnings || !reflect.DeepEqual(s.Team, []uint8{2, 0, 1}) || warnings[0].Tick != ^uint32(0) {
		t.Fatal("capture changed shared input")
	}
	warnings[0], warnings[1] = warnings[1], warnings[0]
	if bytes.Equal(want, survivalInfoCheckpointBytes(t, s)) {
		t.Fatal("warning order vanished")
	}
	warnings[0], warnings[1] = warnings[1], warnings[0]
	warnings[0].Groups[0].Domain = "mix"
	if bytes.Equal(want, survivalInfoCheckpointBytes(t, s)) {
		t.Fatal("stored domain spelling vanished")
	}
}

func TestCheckpointSurvivalInfoPresenceAndNil(t *testing.T) {
	s := &SurvivalInfo{}
	absent := survivalInfoCheckpointBytes(t, s)
	if !bytes.Equal(absent, make([]byte, 22)) {
		t.Fatalf("absent warnings %x", absent)
	}
	var warnings []SurvivalWarning
	s.warnings.Store(&warnings)
	want := make([]byte, 26)
	want[21] = 1
	if got := survivalInfoCheckpointBytes(t, s); !bytes.Equal(got, want) {
		t.Fatalf("present empty warnings %x", got)
	}
	var b bytes.Buffer
	var missing *SurvivalInfo
	if err := missing.WriteCheckpoint(checkpoint.NewEncoder(&b)); err == nil || !strings.Contains(err.Error(), "logical path ai.SurvivalInfo") || b.Len() != 0 {
		t.Fatalf("nil info: %v", err)
	}
}
