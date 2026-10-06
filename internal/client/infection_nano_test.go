package client

import (
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/palette"
	"testing"
)

func TestInfectionNanoUsesActivePaletteWithoutChangingOrdinarySpray(t *testing.T) {
	c, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	p := &palette.Tables{}
	p.Base[41] = [4]byte{153, 43, 66, 255}
	p.Base[63] = [4]byte{173, 177, 74, 255}
	c.SetPalette(p)
	v := frame.StripView{Family: frame.StripFamilyNano, Fill: 0xa3, ColorSample: 1, NanoInfected: true}
	if index, mapped := c.nanoParticleColor(v); !mapped || index != 41 {
		t.Fatalf("infection index %d mapped %v", index, mapped)
	}
	v.NanoInfected = false
	if index, mapped := c.nanoParticleColor(v); mapped || index != v.Fill {
		t.Fatal("ordinary spray changed")
	}
	// Rebinding a mod palette must not keep physical indices from the old one.
	p2 := *p
	p2.Base[92], p2.Base[41] = p2.Base[41], [4]byte{}
	c.SetPalette(&p2)
	v.NanoInfected = true
	if index, _ := c.nanoParticleColor(v); index != 92 {
		t.Fatal("infection ramp retained stale palette")
	}
}
