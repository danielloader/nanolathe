// Package metalrender is the native Metal renderer
// (docs/DESIGN_METAL_RENDERER.md). It consumes immutable presentation samples
// through a caller-owned source; it never writes simulation state.
package metalrender

// Frames zero runs until window close and retains the last 8192 timing rows.
// An empty OutputDir writes no report, timing rows or capture.
// Options selects host presentation and measurement, not gameplay behavior.
type Options struct {
	Width, Height, Frames, Warmup, FPS      int
	Offscreen, VSync, Effects, VisualPasses bool
	OutputDir                               string
	Benchmark                               *BenchmarkOptions
	// PrepareAhead reports whether a live frame may be prepared on a worker
	// while the render thread paces and submits the frame before it. Nil
	// prepares every frame on the render thread after pacing.
	PrepareAhead func(frame int) bool `json:"-"`
}
