package movement

import (
	"errors"
	"fmt"

	"github.com/nanolathe-gg/nanolathe/internal/path"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

func movementSummaryBool(out *checkpoint.Summary, v bool) {
	if v {
		out.Word(1)
	} else {
		out.Word(0)
	}
}

// AppendCheckpointSummary reads the selected scalars of §16.3.7 directly.
// Each physical slice contributes length, then slot presence and values. It
// inspects no composition bindings and collects no graph or canonical bytes.
func (s *System) AppendCheckpointSummary(out *checkpoint.Summary) error {
	if s == nil || out == nil {
		return movementCheckpointError("movement.summary", errors.New("missing system or summary"))
	}
	out.Word(uint64(len(s.Routes)))
	for _, r := range s.Routes {
		movementSummaryBool(out, r != nil)
		if r == nil {
			continue
		}
		out.Word(uint64(r.Count))
		movementSummaryBool(out, r.Active)
		movementSummaryBool(out, r.Dirty)
		out.Word(uint64(r.Status))
		out.Word(uint64(r.LastRequestTick))
		for _, p := range r.Points {
			out.Word(uint64(int64(p.X)))
			out.Word(uint64(int64(p.Z)))
		}
	}
	out.Word(uint64(len(s.Steers)))
	for _, r := range s.Steers {
		movementSummaryBool(out, r != nil)
		if r == nil {
			continue
		}
		out.Word(uint64(int64(r.X)))
		out.Word(uint64(int64(r.Z)))
		out.Word(uint64(r.Heading))
		out.Word(uint64(int64(r.Speed)))
		movementSummaryBool(out, r.Dirty)
	}
	out.Word(uint64(len(s.Collisions)))
	for _, r := range s.Collisions {
		movementSummaryBool(out, r != nil)
		if r == nil {
			continue
		}
		movementSummaryBool(out, r.HasStamp)
		out.Word(uint64(int64(r.StampedAnchor.X)))
		out.Word(uint64(int64(r.StampedAnchor.Z)))
		out.Word(uint64(r.LastStampTick))
		out.Word(uint64(r.StampedPlane))
		out.Word(uint64(int64(r.BlockerID)))
		movementSummaryBool(out, r.Blocked)
	}
	out.Word(uint64(len(s.Flights)))
	for _, r := range s.Flights {
		movementSummaryBool(out, r != nil)
		if r == nil {
			continue
		}
		out.Word(uint64(r.Mode))
		out.Word(uint64(int64(r.X)))
		out.Word(uint64(int64(r.Y)))
		out.Word(uint64(int64(r.Z)))
		out.Word(uint64(int64(r.VX)))
		out.Word(uint64(int64(r.VY)))
		out.Word(uint64(int64(r.VZ)))
	}
	out.Word(uint64(s.workTick))
	out.Word(uint64(s.workSmooth))
	return nil
}

// AppendPathCheckpointSummary follows the scheduler's fragment, retaining one
// search row for every physical working-set slot. Unknown searches fail the
// complete fragment atomically; even their public inspection methods are not
// called (DESIGN_MULTIPLAYER §16.3.14).
func (s *System) AppendPathCheckpointSummary(out *checkpoint.Summary) error {
	if s == nil || out == nil {
		return movementCheckpointError("paths.working.summary", errors.New("missing system or summary"))
	}
	next := *out
	next.Word(uint64(len(s.sessions)))
	for i, ws := range s.sessions {
		var search path.Search
		if ws != nil {
			search = ws.session
		}
		if err := path.AppendSearchCheckpointSummary(search, &next); err != nil {
			return movementCheckpointError(fmt.Sprintf("paths.working[%d].summary", i), err)
		}
	}
	*out = next
	return nil
}
