package pool

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

func TestCheckpointProjectilePoolResidualVector(t *testing.T) {
	p := NewProjectiles(3)
	p.count = 1
	p.dead = []bool{false, true, true}
	want, _ := hex.DecodeString("0300000000000000010000000000000003000000000101")
	var b bytes.Buffer
	if err := p.WriteCheckpoint(checkpoint.NewEncoder(&b)); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b.Bytes(), want) {
		t.Fatalf("payload=%x, want %x", b.Bytes(), want)
	}
	p.payload[2] = 9
	p.compactScratch[0] = 7
	b.Reset()
	if err := p.WriteCheckpoint(checkpoint.NewEncoder(&b)); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b.Bytes(), want) {
		t.Fatal("diagnostics or scratch entered payload")
	}
	p.dead[2] = false
	b.Reset()
	if err := p.WriteCheckpoint(checkpoint.NewEncoder(&b)); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(b.Bytes(), want) {
		t.Fatal("dead flag beyond live count was lost")
	}
}

func TestCheckpointProjectilePoolLazyAndInvalidStorage(t *testing.T) {
	var p Projectiles
	var b bytes.Buffer
	if err := p.WriteCheckpoint(checkpoint.NewEncoder(&b)); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b.Bytes(), make([]byte, 20)) || p.dead != nil || p.capacity != 0 {
		t.Fatal("capture materialized lazy pool")
	}
	for _, p := range []*Projectiles{nil, {count: -1}, {count: 1}, {capacity: 2, count: 3, dead: make([]bool, 2)}, {capacity: 3, dead: make([]bool, 2)}} {
		b.Reset()
		if err := p.WriteCheckpoint(checkpoint.NewEncoder(&b)); err == nil {
			t.Fatal("accepted malformed pool")
		}
		if b.Len() != 0 {
			t.Fatal("failed validation wrote payload")
		}
	}
}
