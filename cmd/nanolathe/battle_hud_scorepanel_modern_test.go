package main

import (
	"reflect"
	"testing"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/economy"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/hud"
)

// Modern displays zero live units as defeated, including a restored empty
// player whose created count was not saved. The scoreboard keeps that row's
// rank and counters (DESIGN_INTERFACE_HUD_INPUT "Modern defeated players").
func TestModernScorePanelDefeatUsesOnlyDrawnPlayers(t *testing.T) {
	rows := scorePanelPlayers()
	rows[0].LiveUnits = 0
	rows[2] = frame.PlayerRow{Present: true, Controller: 2, Side: hud.ScoreSideExcluded, Rank: 2}
	rows[3] = frame.PlayerRow{Present: true, Controller: 1, Watcher: true, Rank: 3}
	rows[4] = frame.PlayerRow{Present: true, Controller: 0, Rank: 4}
	_, _, b := scorePanelFixtureWith(t, false, rows, 1, 5)
	f := &frame.Frame{Players: rows}
	slots := make([]hud.ScoreSlot, len(rows))
	for i, row := range rows {
		slots[i] = hud.ScoreSlot{Present: row.Present, Controller: row.Controller, Side: row.Side, LiveUnits: row.LiveUnits, Watcher: row.Watcher, Rank: row.Rank}
	}
	order, _ := hud.ScoreRowOrder(slots, 5)
	if !reflect.DeepEqual(order, []int{0, 1}) {
		t.Fatalf("score order = %v, want both participating rows", order)
	}
	for _, mode := range []gameplay.Mode{gameplay.Modern, "", gameplay.Strict31, gameplay.Community39} {
		b.sess.Gameplay = mode
		status := scorePanelStatusFor(b, f, order)
		if mode.Normalize() != gameplay.Modern {
			if status != (scorePanelStatus{}) {
				t.Fatalf("%s enabled the Modern display: %+v", mode, status)
			}
			continue
		}
		if !status.modern || status.remaining != 1 || status.total != 2 || !status.defeated[0] || status.defeated[1] {
			t.Fatalf("%s: status = %+v, want one of two players remaining", mode, status)
		}
	}
	// A revived side clears the marker on the next committed frame; no
	// presentation latch survives its empty-unit interval.
	b.sess.Gameplay = gameplay.Modern
	f.Players[0].LiveUnits = 1
	if status := scorePanelStatusFor(b, f, order); status.remaining != 2 || status.defeated[0] {
		t.Fatalf("revived player still marked defeated: %+v", status)
	}
	// The wave owner is the last configured seat and is never a remaining
	// survivor, whether the wave is empty or active (DESIGN_SURVIVAL §4.1).
	b.sess.Skirmish.NumPlayers = 2
	f.Survival.Active = true
	f.Players[0].LiveUnits = 0
	for _, attackers := range []int{0, 10} {
		f.Players[1].LiveUnits = attackers
		status := scorePanelStatusFor(b, f, order)
		if status.remaining != 0 || status.total != 1 || !status.defeated[0] || status.defeated[1] {
			t.Fatalf("attackers=%d: Survival status = %+v", attackers, status)
		}
	}
}

func TestModernDefeatedScoreRowDrawsMarkerAndKeepsCounters(t *testing.T) {
	for _, mode := range []gameplay.Mode{gameplay.Modern, gameplay.Strict31, gameplay.Community39} {
		t.Run(string(mode), func(t *testing.T) {
			rows := scorePanelPlayers()
			rows[0].LiveUnits = 0
			c, h, b := scorePanelFixtureWith(t, false, rows, 1, 2)
			b.sess.Gameplay = mode
			b.sess.Econ = &economy.Service{}
			b.sess.Econ.Players[0].Stock[0] = 17
			b.sess.Econ.Players[1].Stock[1] = 29
			resources := b.sess.Econ.Players
			sim, crt := b.sess.SimRNG().Draws(), b.sess.CrtRNG().Draws()
			h.modalFont = &formats.GAFEntry{Frames: make([]formats.GAFFrameRef, 256)}
			for i := 32; i < 127; i++ {
				h.modalFont.Frames[i].Frame = &formats.GAFFrame{Width: 3, Height: 1, YOffset: 1, Pixels: []byte{200, 200, 200}, Transparent: make([]bool, 3)}
			}
			for i := range h.pal.Shade[20] {
				h.pal.Shade[20][i] = 39
			}
			h.score = hud.ScorePanelWidth
			b.panelHoldFlag = true
			img := c.ComposeFrame()
			rowY := int(hud.ScoreRowTop(0))
			if mode == gameplay.Modern {
				rowY += 15
				for _, point := range [][2]int{{517, 32}, {524, rowY + 1}, {524, rowY + 13}, {524, rowY + 26}} {
					if got := img.RGBAAt(point[0], point[1]); got != gray(200) {
						t.Fatalf("Modern count/name/marker/counter at %v = %v, want text", point, got)
					}
				}
				if got := img.RGBAAt(600, rowY+3); got != gray(39) {
					t.Fatalf("defeated artwork = %v, want dimmed row", got)
				}
			} else {
				if got := img.RGBAAt(524, rowY+13); got != gray(40) {
					t.Fatalf("%s drew a defeat marker: %v", mode, got)
				}
				if got := img.RGBAAt(524, rowY+21); got != gray(200) {
					t.Fatalf("%s moved the retail counter: %v", mode, got)
				}
			}
			if b.sess.SimRNG().Draws() != sim || b.sess.CrtRNG().Draws() != crt || !reflect.DeepEqual(b.sess.Econ.Players, resources) {
				t.Fatal("score composition changed RNG draws or resources")
			}
		})
	}
}
