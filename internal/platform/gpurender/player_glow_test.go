package gpurender

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
)

// Player source amounts multiply content amounts without either overwriting
// the other. The classification follows the recorder's source tag (§19.4).
func TestPlayerGlowAmountsStayIndependentOfContent(t *testing.T) {
	r := &Renderer{}
	e := drawlist.AllEffects()
	e.WeaponGlowStrength, e.ExplosionGlowStrength, e.NanoGlowStrength = 50, 150, 25
	r.SetEffects(e)
	for i := 0; i < 2; i++ {
		r.SetGlowFamilies(50, 200, 75)
		if r.weaponGlowScale() != 0.25 || r.effectGlowScale() != 0.75 || r.nanoFamilyScale() != 0.5 || r.families.scale(glowFamilyGround) != 0.75 {
			t.Fatal("content refresh replaced the player's chosen source amounts")
		}
		if r.spriteGlowScale(drawlist.SpriteLightingProjectile) != 0.25 {
			t.Fatal("projectile body did not follow weapon amount")
		}
		for _, kind := range []drawlist.SpriteLightingKind{drawlist.SpriteLightingNone, drawlist.SpriteLightingExplosion, drawlist.SpriteLightingFire, drawlist.SpriteLightingSpark, drawlist.SpriteLightingSmoke} {
			if r.spriteGlowScale(kind) != 0.75 {
				t.Fatalf("effect/strip source %v did not follow effect amount", kind)
			}
		}
	}
	r.resetSources(func(*ebiten.Image) {})
	if r.Effects() != e || r.weaponGlowScale() != 0.25 || r.effectGlowScale() != 0.75 || r.nanoFamilyScale() != 0.5 {
		t.Fatal("source reset lost the player or content amounts")
	}
}

// Zero admission is independent of neighbouring source families; doubling a
// family changes its gain alone and leaves the ordinary source drawing intact.
func TestPlayerGlowZeroAndGain(t *testing.T) {
	for _, family := range []string{"weapon", "effect", "nano"} {
		for _, percent := range []int{0, 50, 100, 200} {
			r := &Renderer{w: 320, h: 240}
			r.tables.atlas, r.surfaces[0], r.sched.flash.img = &ebiten.Image{}, &ebiten.Image{}, &ebiten.Image{}
			r.SetGlow(true)
			r.SetGlowStrength(100)
			e := drawlist.AllEffects()
			switch family {
			case "weapon":
				e.WeaponGlowStrength = percent
			case "effect":
				e.ExplosionGlowStrength = percent
			case "nano":
				e.NanoGlowStrength = percent
			}
			r.SetEffects(e)
			draw := []func(){
				func() { r.glowLine(drawlist.Line{X0: 10, Y0: 10, X1: 60, Y1: 10, Index: 255}) },
				func() { r.glowFlash(20, 20, 40, 40, 0, 0, 20, 20) },
				func() {
					r.glowNano(drawlist.Fill{Nano: true, Rect: drawlist.Rect{X: 100, Y: 80, W: 2, H: 2}, Index: 163})
				},
			}
			for i, name := range []string{"weapon", "effect", "nano"} {
				before := r.glow.quads
				draw[i]()
				if name == family && percent == 0 {
					if r.glow.quads != before {
						t.Fatalf("zero %s kept its glow source", family)
					}
					continue
				}
				v, ok := r.glow.lastVertex()
				gain := []float32{glowGain, glowLightGain * glowGain, nanoGlowGain * glowGain}[i]
				if name == family {
					gain *= float32(percent) / 100
				}
				if !ok || r.glow.quads != before+1 || v.ColorG != gain {
					t.Fatalf("%s at %d reached %s gain: %+v, want %v", family, percent, name, v, gain)
				}
			}
		}
	}
}

func TestPlayerAmountsClampAtExecutor(t *testing.T) {
	r := &Renderer{}
	e := drawlist.AllEffects()
	e.WeaponGlowStrength, e.ExplosionGlowStrength, e.NanoGlowStrength, e.ShadowSoftness = -1, 300, 900, -2
	r.SetEffects(e)
	got := r.Effects()
	if got.WeaponGlowStrength != 0 || got.ExplosionGlowStrength != 200 || got.NanoGlowStrength != 200 || got.ShadowSoftness != 0 {
		t.Fatalf("unbounded executor amounts: %+v", got)
	}
}

// Nano's player amount scales its local illumination as well as its halo,
// leaving the original spray cores available in the unchanged recorded list.
func TestPlayerNanoAmountScalesLocalLight(t *testing.T) {
	var list drawlist.List
	for i := int32(0); i < 10; i++ {
		list.RecordFill(drawlist.Fill{Nano: true, Rect: drawlist.Rect{X: 100 + i, Y: 80, W: 2, H: 2}, Index: 163, WorldHeight: 16})
	}
	r := &Renderer{w: 320, h: 240}
	for i := 161; i <= 167; i++ {
		r.displayPalette[i] = [4]byte{40, 200, 40, 255}
	}
	e := drawlist.AllEffects()
	read := func(pct int) (int, [3]float32) {
		e.NanoGlowStrength = pct
		r.SetEffects(e)
		r.prepareBattleLighting(&list)
		if len(r.lighting.lights) == 0 {
			return 0, [3]float32{}
		}
		return len(r.lighting.lights), r.lighting.lights[0].color
	}
	n, base := read(100)
	if n == 0 {
		t.Fatal("default nano light missing")
	}
	for _, pct := range []int{25, 50, 200} {
		count, c := read(pct)
		if count != n || c[1] != base[1]*float32(pct)/100 {
			t.Fatalf("nano amount %d changed cluster count or gain: %d/%v against %d/%v", pct, count, c, n, base)
		}
	}
	if count, _ := read(0); count != 0 {
		t.Fatal("zero nano amount still lent local light")
	}
	if count, _ := read(100); count != n {
		t.Fatal("restoring nano amount lost recorded particles")
	}
}
