package gpurender

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"runtime"
	"testing"
	"time"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
)

// placeLockstep places the scenario's frames on a sequential reference and on
// each candidate in lockstep, and fails at the first frame whose output
// differs, naming the sections that do. modes collects each candidate's
// placement mode per frame.
func placeLockstep(t *testing.T, sc *placeScenario, candidates []*Renderer) (modes [][]modelPlaceMode) {
	t.Helper()
	ref := newPlaceRenderer(t)
	t.Cleanup(func() { closeModelPlacePool(ref) })
	ref.modelDirect.place.floor = 1 << 30
	modes = make([][]modelPlaceMode, len(candidates))
	for f := 0; f < modelPlaceFrames; f++ {
		want, wantSections := runPlaceFrameSections(ref, sc, f)
		if ref.modelDirect.placeMode != modelPlacedSequential {
			t.Fatalf("frame %d: the reference was placed in mode %d, want sequential", f, ref.modelDirect.placeMode)
		}
		for i, r := range candidates {
			got, gotSections := runPlaceFrameSections(r, sc, f)
			modes[i] = append(modes[i], r.modelDirect.placeMode)
			if bytes.Equal(got, want) {
				continue
			}
			var differ []string
			for k := range wantSections {
				if !bytes.Equal(wantSections[k].sum, gotSections[k].sum) {
					differ = append(differ, wantSections[k].name)
				}
			}
			t.Fatalf("candidate %d frame %d (mode %d): output differs from the sequential lane in %v%s",
				i, f, r.modelDirect.placeMode, differ, placeBatchDiff(ref, r))
		}
	}
	return modes
}

// placeBatchDiff names the first vertex, index or run at which two lanes'
// batches differ.
func placeBatchDiff(a, b *Renderer) string {
	da, db := &a.modelDirect, &b.modelDirect
	if len(da.verts) != len(db.verts) {
		return fmt.Sprintf(": %d vertices, want %d", len(db.verts), len(da.verts))
	}
	for i := range da.verts {
		if da.verts[i] != db.verts[i] {
			return fmt.Sprintf(": vertex %d is %+v, want %+v", i, db.verts[i], da.verts[i])
		}
	}
	for i := range da.idx {
		if i >= len(db.idx) || da.idx[i] != db.idx[i] {
			return fmt.Sprintf(": index %d differs", i)
		}
	}
	if len(da.runs) != len(db.runs) {
		return fmt.Sprintf(": %d runs, want %d", len(db.runs), len(da.runs))
	}
	for i := range da.runs {
		ra, rb := da.runs[i], db.runs[i]
		if ra.page != rb.page || ra.vOff != rb.vOff || ra.vLen != rb.vLen || ra.iOff != rb.iOff || ra.iLen != rb.iLen {
			return fmt.Sprintf(": run %d is %+v, want %+v", i, rb, ra)
		}
	}
	return ""
}

// placeShuffle is a claim order hook: a seeded permutation of the frame's
// jobs, a different one every frame, so the participants claim them in an
// order unrelated to the jobs' own.
func placeShuffle(seed uint64) func(int) []int32 {
	rng := rand.New(rand.NewPCG(seed, seed+1))
	return func(n int) []int32 {
		order := make([]int32, n)
		for i := range order {
			order[i] = int32(i)
		}
		rng.Shuffle(n, func(i, j int) { order[i], order[j] = order[j], order[i] })
		return order
	}
}

// newParallelPlaceRenderer is a scenario renderer that places every frame the
// pre-check admits on a pool of workers participants, however few its jobs.
func newParallelPlaceRenderer(t *testing.T, workers int) *Renderer {
	r := newPlaceRenderer(t)
	t.Cleanup(func() { closeModelPlacePool(r) })
	r.placeWorkers = workers
	r.modelDirect.place.floor = 1
	return r
}

// Placing a frame on a pool of workers is placing it on one goroutine: the
// same batch, runs, parameter bytes, regions, merges, accounting, retained
// store and body index, reflection batch and fallback, frame after frame,
// for pools of one, two, four and sixteen participants claiming the jobs in
// order and in a shuffled order — reflecting frames included, their face
// reflections recorded on the workers and appended in order
// (docs/DESIGN_GPU_RENDERER.md §22, "Placement workers").
func TestModelPlaceParallelMatchesSequential(t *testing.T) {
	skipAfterDeviceLoop(t)
	sc := newPlaceScenario()
	var candidates []*Renderer
	for _, workers := range []int{1, 2, 4, 16} {
		for _, shuffle := range []bool{false, true} {
			r := newParallelPlaceRenderer(t, workers)
			if shuffle {
				r.modelDirect.place.order = placeShuffle(uint64(workers))
			}
			candidates = append(candidates, r)
		}
	}
	modes := placeLockstep(t, sc, candidates)
	for i := range candidates {
		for f, mode := range modes[i] {
			if mode != modelPlacedParallel {
				t.Errorf("candidate %d frame %d placed in mode %d, want parallel", i, f, mode)
			}
		}
	}
}

// A frame whose reflecting faces fill the reflection batch to its cap part-way
// through is placed in parallel, and the faces past the cap are the ones the
// sequential lane leaves out: the recorded reflections are appended in the
// sequential lane's order, against the same cap.
func TestModelPlaceReflectionsMeetTheirCap(t *testing.T) {
	skipAfterDeviceLoop(t)
	sc := newPlaceScenario()
	sc.flood = true
	r := newParallelPlaceRenderer(t, 4)
	r.modelDirect.place.order = placeShuffle(3)
	capped := false
	ref := newPlaceRenderer(t)
	ref.modelDirect.place.floor = 1 << 30
	for f := 0; f < modelPlaceFrames; f++ {
		want := runPlaceFrame(ref, sc, f)
		got := runPlaceFrame(r, sc, f)
		if !bytes.Equal(got, want) {
			t.Fatalf("frame %d: output differs from the sequential lane%s", f, placeBatchDiff(ref, r))
		}
		if r.modelDirect.placeMode != modelPlacedParallel {
			t.Fatalf("frame %d placed in mode %d, want parallel", f, r.modelDirect.placeMode)
		}
		if f == 2 || f == 6 {
			capped = capped || len(r.reflections.verts) >= reflectionVertexLimit-4096-256
		}
	}
	if !capped {
		t.Fatal("the flooded frames never filled the reflection batch to its cap")
	}
}

// Every guard the pre-check makes sends the whole frame down the sequential
// path, and a batch past the vertex bound is filled again in order from the
// same decisions; either way the output is the sequential lane's.
func TestModelPlaceGuardsKeepTheSequentialOutput(t *testing.T) {
	skipAfterDeviceLoop(t)
	for _, c := range []struct {
		name      string
		duplicate bool
		tune      func(*modelPlaceTuning)
		want      modelPlaceMode
	}{
		{name: "duplicate body", duplicate: true, want: modelPlacedSequential},
		{name: "keyed packets past the cap", tune: func(p *modelPlaceTuning) { p.keyCap = 3 }, want: modelPlacedSequential},
		{name: "parameter bound", tune: func(p *modelPlaceTuning) { p.paramBound = 100 }, want: modelPlacedSequential},
		{name: "too few jobs", tune: func(p *modelPlaceTuning) { p.floor = 1000 }, want: modelPlacedSequential},
		{name: "vertex bound", tune: func(p *modelPlaceTuning) { p.vertexLimit = 1000 }, want: modelPlacedRefilled},
	} {
		t.Run(c.name, func(t *testing.T) {
			sc := newPlaceScenario()
			sc.duplicate = c.duplicate
			r := newParallelPlaceRenderer(t, 4)
			r.modelDirect.place.order = placeShuffle(7)
			if c.tune != nil {
				c.tune(&r.modelDirect.place)
			}
			modes := placeLockstep(t, sc, []*Renderer{r})
			for f, mode := range modes[0] {
				if mode != c.want {
					t.Errorf("frame %d placed in mode %d, want %d", f, mode, c.want)
				}
			}
		})
	}
}

// closeModelPlacePool stops a renderer's placement pool at once rather than
// when the renderer is collected, and a second stop is a no-op.
func closeModelPlacePool(r *Renderer) {
	r.placePool.close()
	r.placePool.close()
	r.placePool = nil
}

// The pool stops when it is closed, and when a renderer is dropped the
// collector stops it, so recreating renderers leaks no goroutine.
func TestModelPlacePoolStops(t *testing.T) {
	skipAfterDeviceLoop(t)
	sc := newPlaceScenario()
	waitFor := func(want int) bool {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			runtime.GC()
			if runtime.NumGoroutine() <= want {
				return true
			}
			time.Sleep(10 * time.Millisecond)
		}
		return false
	}
	base := runtime.NumGoroutine()
	place := func(close bool) {
		r := newPlaceRenderer(t)
		r.placeWorkers = 6
		r.modelDirect.place.floor = 1
		runPlaceFrame(r, sc, 0)
		if r.modelDirect.placeMode != modelPlacedParallel || r.placePool == nil {
			t.Fatal("the frame was not placed on the pool")
		}
		if got := runtime.NumGoroutine(); got < base+5 {
			t.Fatalf("%d goroutines with a six-participant pool, want at least %d", got, base+5)
		}
		if close {
			closeModelPlacePool(r)
		}
	}
	place(true)
	if !waitFor(base) {
		t.Fatalf("%d goroutines after closing the pool, want %d", runtime.NumGoroutine(), base)
	}
	for range 3 {
		place(false)
	}
	if !waitFor(base) {
		t.Fatalf("%d goroutines after dropping three renderers, want %d", runtime.NumGoroutine(), base)
	}
}

// A run draws into the page of the region its faces land in, which is the
// page its commit samples. A construction group whose group region fits the
// first page while its child's separate region opens the second has runs on
// both: the carrier's on the first, the child's on the second, sequential and
// parallel alike. Stamping the job's runs with the packer's page once all its
// regions were allocated drew the carrier into the second page at the first
// page's coordinates, and its commit sampled a cleared region.
func TestModelPlaceRunPageIsTheRegionPage(t *testing.T) {
	skipAfterDeviceLoop(t)
	for _, parallel := range []bool{false, true} {
		r := newParallelPlaceRenderer(t, 4)
		if !parallel {
			r.modelDirect.place.floor = 1 << 30
		}
		d := &r.modelDirect
		yard, build := placePageTurnGroup()
		var list drawlist.List
		for i := 0; i < 3; i++ {
			list.RecordModel(drawlist.Model{Geometry: placeBody(placeBodyOpts{faces: 4, w: 1000, h: 1000, ax: 0, ay: 0, seed: 90 + i})})
		}
		list.RecordModel(drawlist.Model{Geometry: yard})
		r.placeModelDirectFrame(&list)
		region, child := d.regions[yard], d.regions[build]
		if !region.ok || !child.ok || region.page != 0 || child.page != 1 {
			t.Fatalf("parallel=%v: the group region is on page %d and its child's on page %d, want 0 and 1", parallel, region.page, child.page)
		}
		carrier, product := d.runs[len(d.runs)-2], d.runs[len(d.runs)-1]
		if carrier.page != region.page || product.page != child.page {
			t.Fatalf("parallel=%v: the carrier's run is on page %d and the product's on %d, want their regions' pages %d and %d",
				parallel, carrier.page, product.page, region.page, child.page)
		}
	}
}

// placePageTurnGroup is a construction group sized, after three subjects a
// page and a half square, to fit the rest of the first atlas page while its
// product's region does not: the product's region opens the second page.
// Its carrier's faces are flat and its product's textured, so their runs do
// not join.
func placePageTurnGroup() (yard, build *drawlist.ModelGeometry) {
	yard = placeBody(placeBodyOpts{faces: 4, w: 1000, h: 900, ax: 0, ay: 0, seed: 95})
	build = placeBody(placeBodyOpts{faces: 4, w: 100, h: 100, ax: 100, ay: 100, texs: []*formats.GAFFrame{benchCargoTexture}, seed: 96, reveal: true})
	yard.Children = []drawlist.ModelChild{{Geometry: build, KeyDelta: 8}}
	return yard, build
}
