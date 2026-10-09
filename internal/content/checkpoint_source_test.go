package content

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/vfs"
)

type checkpointRangeSource struct{ vfs.FSOps }

func (checkpointRangeSource) ReadFileRange(string, int64, int) ([]byte, error) {
	panic("checkpoint called the range provider")
}

type checkpointOrderedSource struct{ vfs.FSOps }

func (checkpointOrderedSource) RetailReadDir(string) ([]vfs.EntryInfo, error) {
	panic("checkpoint called the ordered provider")
}

type checkpointRangeOrderedSource struct{ checkpointRangeSource }

func (checkpointRangeOrderedSource) RetailReadDir(string) ([]vfs.EntryInfo, error) {
	panic("checkpoint called the range-and-ordered provider")
}

// Both value and pointer forms implement the open ownership-reporting method.
// The slice makes interface equality unsafe even before a method is called.
type checkpointHostileSource struct {
	vfs.FSOps
	values []int
}

func (checkpointHostileSource) SimulationInputs() *SimulationInputs {
	panic("checkpoint trusted an open ownership method")
}

func TestCheckpointFilesystemMatchesVariants(t *testing.T) {
	for _, tc := range []struct {
		name string
		live vfs.FSOps
	}{
		{"plain", struct{ vfs.FSOps }{}},
		{"range", checkpointRangeSource{}},
		{"ordered", checkpointOrderedSource{}},
		{"range-and-ordered", checkpointRangeOrderedSource{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snap := newSourceSnapshot(tc.live)
			sources := &SimulationSources{snap: snap, frozen: true}
			sources.view = snap.newView(nil, false)
			in := &SimulationInputs{sources: sources}
			in.view = snap.newView(in, false)
			snap.seal()
			if !in.CheckpointFilesystemMatches(in.Filesystem()) {
				t.Fatal("retained frozen view refused")
			}
			if in.CheckpointFilesystemMatches(sources.Filesystem()) {
				t.Fatal("unbound capture view accepted")
			}
			if in.CheckpointFilesystemMatches(snap.newView(in, false)) {
				t.Fatal("second view over the same snapshot accepted")
			}
			if in.CheckpointFilesystemMatches(snap.newView(in, true)) {
				t.Fatal("transient view accepted")
			}
			if in.CheckpointFilesystemMatches(struct{ vfs.FSOps }{in.view}) {
				t.Fatal("foreign wrapper accepted")
			}
			copyInputs := *in
			if copyInputs.CheckpointFilesystemMatches(in.view) {
				t.Fatal("copied inputs accepted the original view")
			}
			if got := testing.AllocsPerRun(100, func() {
				if !in.CheckpointFilesystemMatches(in.view) {
					panic("retained view changed")
				}
			}); got != 0 {
				t.Fatalf("successful check allocated %v times", got)
			}
			if !sources.frozen || !snap.sealed || len(snap.files) != 0 || len(snap.dirs) != 0 ||
				len(snap.retailDirs) != 0 || len(snap.ranges) != 0 || len(snap.uncaptured) != 0 || snap.manifestDone {
				t.Fatal("identity checks changed capture state")
			}
		})
	}
}

func TestCheckpointFilesystemMatchesMalformedViews(t *testing.T) {
	for _, tc := range []struct {
		name     string
		wrap     func(*snapshotView) vfs.FSOps
		typedNil vfs.FSOps
	}{
		{"plain", func(b *snapshotView) vfs.FSOps { return b }, (*snapshotView)(nil)},
		{"range", func(b *snapshotView) vfs.FSOps { return &snapshotRangeView{b} }, (*snapshotRangeView)(nil)},
		{"ordered", func(b *snapshotView) vfs.FSOps { return &snapshotOrderedView{b} }, (*snapshotOrderedView)(nil)},
		{"range-and-ordered", func(b *snapshotView) vfs.FSOps { return &snapshotRangeOrderedView{b} }, (*snapshotRangeOrderedView)(nil)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snap := &sourceSnapshot{sealed: true}
			in := &SimulationInputs{sources: &SimulationSources{snap: snap, frozen: true}}
			base := &snapshotView{snap: snap, inputs: in}
			view := tc.wrap(base)
			in.view = view
			if !in.CheckpointFilesystemMatches(view) {
				t.Fatal("valid topology refused")
			}
			// Exact wrapper identity matters even when the copied wrapper keeps
			// the original base. A copied base is not the retained view either.
			baseCopy := *base
			if tc.name != "plain" && in.CheckpointFilesystemMatches(tc.wrap(base)) {
				t.Fatal("copied wrapper accepted")
			}
			if in.CheckpointFilesystemMatches(tc.wrap(&baseCopy)) {
				t.Fatal("copied base accepted")
			}
			for _, mutate := range []struct {
				name string
				set  func()
			}{
				{"transient", func() { base.transient = true }},
				{"unbound", func() { base.inputs = nil }},
				{"foreign inputs", func() { base.inputs = &SimulationInputs{} }},
				{"nil snapshot", func() { base.snap = nil }},
				{"foreign snapshot", func() { base.snap = &sourceSnapshot{sealed: true} }},
				{"nil sources", func() { in.sources = nil }},
				{"nil source snapshot", func() { in.sources = &SimulationSources{} }},
			} {
				t.Run(mutate.name, func(t *testing.T) {
					*base = baseCopy
					in.sources = &SimulationSources{snap: snap, frozen: true}
					mutate.set()
					if in.CheckpointFilesystemMatches(view) {
						t.Fatal("malformed retained view accepted")
					}
				})
			}
			*base = baseCopy
			in.sources = &SimulationSources{snap: snap, frozen: true}
			in.view = tc.typedNil
			if in.CheckpointFilesystemMatches(tc.typedNil) || in.CheckpointFilesystemMatches(view) {
				t.Fatal("typed-nil retained view accepted")
			}
			in.view = checkpointHostileSource{values: []int{1}}
			if in.CheckpointFilesystemMatches(view) {
				t.Fatal("foreign retained view accepted")
			}
			in.view = tc.wrap(nil)
			if in.CheckpointFilesystemMatches(in.view) {
				t.Fatal("nil base accepted")
			}
		})
	}
}

func TestCheckpointFilesystemMatchesHostileAndNil(t *testing.T) {
	in := &SimulationInputs{sources: &SimulationSources{snap: &sourceSnapshot{}}}
	var nilInputs *SimulationInputs
	for _, fs := range []vfs.FSOps{nil, checkpointHostileSource{values: []int{1}}, &checkpointHostileSource{}, (*checkpointHostileSource)(nil)} {
		// Exercise both sides as an arbitrary interface. Even two identical
		// hostile noncomparable values must refuse without equality or calls.
		in.view = fs
		if in.CheckpointFilesystemMatches(fs) || nilInputs.CheckpointFilesystemMatches(fs) {
			t.Fatal("nil or hostile filesystem accepted")
		}
	}
	var empty SimulationInputs
	if empty.CheckpointFilesystemMatches(nil) {
		t.Fatal("empty inputs accepted")
	}
}

func TestCheckpointFilesystemMatchesFrozenInputs(t *testing.T) {
	f := newFrozenFixture(t)
	first := f.freeze(t, SimulationInputRequest{Catalog: f.catalog(t)})
	second := f.freeze(t, SimulationInputRequest{Catalog: f.catalog(t)})
	digest := first.Digest()
	before := len(first.UncapturedLookups())
	if !first.CheckpointFilesystemMatches(first.Filesystem()) || !second.CheckpointFilesystemMatches(second.Filesystem()) {
		t.Fatal("freezer output refused")
	}
	if first.CheckpointFilesystemMatches(second.Filesystem()) || second.CheckpointFilesystemMatches(first.Filesystem()) {
		t.Fatal("another freeze's view accepted")
	}
	if first.Digest() != digest || len(first.UncapturedLookups()) != before {
		t.Fatal("identity check changed frozen inputs")
	}
}
