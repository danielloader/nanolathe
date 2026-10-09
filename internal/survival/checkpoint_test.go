package survival

import (
	"bytes"
	"encoding/hex"
	"reflect"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

func regionsCheckpointBytes(t *testing.T, v *Regions) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := v.WriteCheckpoint(checkpoint.NewEncoder(&b)); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestCheckpointRegionsVectorAndStoredResiduals(t *testing.T) {
	v := Regions{H: 2, W: 3, label: []int32{0, 4, -1, 4, 0, 8}, sizes: []int32{9, 99, -3}}
	before := Regions{H: 2, W: 3, label: append([]int32(nil), v.label...), sizes: append([]int32(nil), v.sizes...)}
	want, _ := hex.DecodeString("020000000300000001060000000000000004000000ffffffff040000000000000008000000030000000900000063000000fdffffff")
	if got := regionsCheckpointBytes(t, &v); !bytes.Equal(got, want) {
		t.Fatalf("got %x want %x", got, want)
	}
	if !reflect.DeepEqual(v, before) {
		t.Fatal("capture relabelled stored regions")
	}
	v.sizes[0]++
	if bytes.Equal(want, regionsCheckpointBytes(t, &v)) {
		t.Fatal("unused size residual omitted")
	}
	v.sizes[0]--
	v.label[0], v.label[1] = v.label[1], v.label[0]
	if bytes.Equal(want, regionsCheckpointBytes(t, &v)) {
		t.Fatal("row-major label order omitted")
	}
}

func TestCheckpointRegionsNilLabelGate(t *testing.T) {
	absent := regionsCheckpointBytes(t, &Regions{})
	present := regionsCheckpointBytes(t, &Regions{label: []int32{}})
	if !bytes.Equal(absent, make([]byte, 13)) {
		t.Fatalf("absent %x", absent)
	}
	want := make([]byte, 17)
	want[8] = 1
	if !bytes.Equal(present, want) {
		t.Fatalf("present empty %x", present)
	}
	var v *Regions
	var b bytes.Buffer
	if err := v.WriteCheckpoint(checkpoint.NewEncoder(&b)); err == nil || !strings.Contains(err.Error(), "logical path survival.Regions") || b.Len() != 0 {
		t.Fatalf("nil regions: %v", err)
	}
}
