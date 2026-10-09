package content

import "github.com/nanolathe-gg/nanolathe/vfs"

// CheckpointFilesystemMatches proves that fs is this input set's retained
// frozen view without consulting an open filesystem interface. It proves
// source ownership only, not cached loader provenance or content validity
// (DESIGN_MULTIPLAYER §16.3.52).
func (in *SimulationInputs) CheckpointFilesystemMatches(fs vfs.FSOps) bool {
	if in == nil || in.sources == nil || in.sources.snap == nil {
		return false
	}
	var base *snapshotView
	switch actual := fs.(type) {
	case *snapshotView:
		retained, ok := in.view.(*snapshotView)
		if !ok || actual == nil || actual != retained {
			return false
		}
		base = actual
	case *snapshotRangeView:
		retained, ok := in.view.(*snapshotRangeView)
		if !ok || actual == nil || actual != retained {
			return false
		}
		base = actual.snapshotView
	case *snapshotOrderedView:
		retained, ok := in.view.(*snapshotOrderedView)
		if !ok || actual == nil || actual != retained {
			return false
		}
		base = actual.snapshotView
	case *snapshotRangeOrderedView:
		retained, ok := in.view.(*snapshotRangeOrderedView)
		if !ok || actual == nil || actual != retained {
			return false
		}
		base = actual.snapshotView
	default:
		return false
	}
	return base != nil && !base.transient && base.snap == in.sources.snap && base.inputs == in
}
