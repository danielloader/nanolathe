package client

import (
	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
)

// Artistic cooling for the requested modern prototype (GPU design §28).
// Both phases sample committed age; replay and pausing cannot reheat a wreck.
func (c *Client) applyWreckHeat(g *drawlist.ModelGeometry, f frame.FeatureView) {
	// Packets may reuse storage, so clear even when the source stops qualifying.
	g.WreckEmission = [3]float32{}
	g.WreckHeatStrength, g.WreckHeatTime, g.WreckHeatScale = 0, 0, 0
	// The cooling emission and the shimmer are two switches (§30): the wreck
	// glow writes the emission colour, which the wreck light borrows, and the
	// wreck shimmer writes the plume's strength and clock. With both off a
	// fresh wreck keeps its ordinary palette colour and refracts nothing.
	glow, shimmer := c.effects.WreckGlow, c.effects.WreckShimmer
	if !c.enhanced || (!glow && !shimmer) || !f.WreckHeatKnown || c.buffer == nil {
		return
	}
	cur := c.committedFrame()
	if cur == nil || f.Y < cur.Visibility.SeaLevel || !SnapshotPointVisible(cur.Visibility, f.X, f.Y, f.Z, cur.ViewingPlayer) {
		return
	}
	elapsed := cur.Tick - f.WreckBornTick
	if elapsed >= 300 {
		return
	}
	age := float32(elapsed)
	if c.interpolation {
		age += c.TickFraction()
	}
	// The view scale is shared: the plume's size and the wreck light's reach
	// both read it.
	g.WreckHeatScale = float32(c.viewScale().Float())
	if glow {
		flash := max(1-age/6, 0)
		cool := max(1-age/180, 0)
		red, amber := cool*cool, cool*cool*cool*cool
		g.WreckEmission = [3]float32{0.75*red + 0.15*flash, 0.20*amber + 0.55*flash, 0.015*amber + 0.42*flash}
	}
	if shimmer {
		heat := max(1-age/300, 0)
		g.WreckHeatStrength = 0.55 * heat * heat
		g.WreckHeatTime = float32(cur.Tick%3600) + float32((f.CX*13+f.CZ*7)&255)
		if c.interpolation {
			g.WreckHeatTime += c.TickFraction()
		}
	}
}
