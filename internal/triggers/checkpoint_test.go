package triggers

import (
	"bytes"
	"encoding/hex"
	"reflect"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

func triggerCheckpointBytes(t *testing.T, v *Trigger) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := v.WriteCheckpoint(checkpoint.NewEncoder(&b)); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestCheckpointTriggerVectorAndPurity(t *testing.T) {
	v := Trigger{Args: [3]int32{1, -2, 3}, Celebrated: true, CenterReady: true, CenterX: 4, CenterY: -5, CenterZ: 6, Completed: true, Kind: KindUnitTypeKilled, Type: "MiX\x00"}
	before := v
	want, _ := hex.DecodeString("01000000feffffff03000000010104000000fbffffff06000000010e040000004d695800")
	if got := triggerCheckpointBytes(t, &v); !bytes.Equal(got, want) {
		t.Fatalf("got %x want %x", got, want)
	}
	if !reflect.DeepEqual(v, before) {
		t.Fatal("capture changed progress")
	}
	for _, edit := range []func(*Trigger){
		func(v *Trigger) { v.Args[2]++ }, func(v *Trigger) { v.Celebrated = false }, func(v *Trigger) { v.CenterReady = false },
		func(v *Trigger) { v.CenterX++ }, func(v *Trigger) { v.CenterY++ }, func(v *Trigger) { v.CenterZ++ },
		func(v *Trigger) { v.Completed = false }, func(v *Trigger) { v.Kind++ }, func(v *Trigger) { v.Type = "mix\x00" },
	} {
		changed := v
		edit(&changed)
		if bytes.Equal(want, triggerCheckpointBytes(t, &changed)) {
			t.Fatal("retained field vanished")
		}
	}
	var absent *Trigger
	var b bytes.Buffer
	if err := absent.WriteCheckpoint(checkpoint.NewEncoder(&b)); err == nil || !strings.Contains(err.Error(), "logical path triggers.Trigger") || b.Len() != 0 {
		t.Fatalf("nil trigger: %v", err)
	}
}
