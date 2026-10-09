package survival

import (
	"fmt"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// WriteCheckpoint retains the cached regions of their original terrain, with
// lexical H, W, label, sizes fields (DESIGN_MULTIPLAYER §16.3.38). In particular,
// nil labels have their own reader gate; neither labels nor sizes are rebuilt.
// The caller supplies record presence.
func (r *Regions) WriteCheckpoint(e *checkpoint.Encoder) error {
	e.Field("survival.Regions")
	if r == nil {
		e.Fail(fmt.Errorf("nanolathe: checkpoint capture failed: logical path survival.Regions, providers searched [survival], expected present regions"))
		return e.Err()
	}
	e.I32(r.H)
	e.I32(r.W)
	e.Field("survival.Regions.label")
	e.Bool(r.label != nil)
	if r.label != nil {
		e.Count(len(r.label))
		for _, v := range r.label {
			e.I32(v)
		}
	}
	e.Field("survival.Regions.sizes")
	e.Count(len(r.sizes))
	for _, v := range r.sizes {
		e.I32(v)
	}
	return e.Err()
}
