package session

import (
	"fmt"
	"slices"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// This is the session fragment appended after visibility.Service (§16.3.39).
// postLoop presence precedes eyeballs.records in actual list order. Record
// fields are lexical: cx, cz, emitter, expiry, heightByte, owner, published,
// sightDistance, x, y, z. Capacity residues are not live records. No expiry or
// raster publication is performed [03 R-COMP-02 §2][08 R-SESS-01 §3].
// visStamps follows, sorted by raw handle, then cx/cz/radius. U0 retains it
// despite its present diagnostic/save readers. Tail hooks, message retirement,
// traces, publicationCount and all visibility publication helpers are excluded.
func (s *Session) writeCheckpointVisibilityTail(e *checkpoint.Encoder) error {
	if e == nil {
		return visibilityTailCheckpointError("visibility.tail", "an encoder")
	}
	e.Field("visibility.tail")
	if s == nil {
		e.Fail(visibilityTailCheckpointError("visibility.tail", "a session"))
		return e.Err()
	}
	e.Field("visibility.postLoop")
	e.Bool(s.postLoop != nil)
	if s.postLoop != nil {
		e.Field("visibility.postLoop.eyeballs.records")
		e.Count(len(s.postLoop.eyeballs.records))
		for _, r := range s.postLoop.eyeballs.records {
			e.I32(r.cx)
			e.I32(r.cz)
			e.U8(r.emitter)
			e.U32(r.expiry)
			e.U8(r.heightByte)
			e.U8(uint8(r.owner))
			e.Bool(r.published)
			e.I16(r.sightDistance)
			e.I64(int64(r.x))
			e.I64(int64(r.y))
			e.I64(int64(r.z))
		}
	}
	// Gather keys only; values are read in the canonical numeric order [I1].
	handles := make([]int, 0, len(s.visStamps))
	for h := range s.visStamps {
		handles = append(handles, h)
	}
	slices.Sort(handles)
	e.Field("visibility.visStamps")
	e.Count(len(handles))
	for _, h := range handles {
		if h < 0 || uint64(h) > uint64(^uint32(0)) {
			e.Fail(visibilityTailCheckpointError("visibility.visStamps.handle", "a raw handle representable by u32"))
			return e.Err()
		}
		r := s.visStamps[h]
		e.U32(uint32(h))
		e.I32(r.cx)
		e.I32(r.cz)
		e.I32(r.radius)
	}
	return e.Err()
}

func visibilityTailCheckpointError(path, expected string) error {
	return fmt.Errorf("nanolathe: checkpoint owner visibility: logical path %s, providers searched [], expected %s", path, expected)
}

// The cheap tail keeps only count and owner/XYZ/expiry/published (§16.3.39).
// An absent postLoop contributes count zero; no allocation-capable tail getter
// is called, and visStamps is deliberately outside the selected words.
func (s *Session) appendCheckpointVisibilityTailSummary(out *checkpoint.Summary) error {
	if s == nil || out == nil {
		return visibilityTailCheckpointError("visibility.tail.summary", "a session and summary")
	}
	next := *out
	if s.postLoop == nil {
		next.Word(0)
	} else {
		next.Word(uint64(len(s.postLoop.eyeballs.records)))
		for _, r := range s.postLoop.eyeballs.records {
			next.Word(uint64(r.owner))
			next.Word(uint64(int64(r.x)))
			next.Word(uint64(int64(r.y)))
			next.Word(uint64(int64(r.z)))
			next.Word(uint64(r.expiry))
			next.Word(checkpointRuntimeBool(r.published))
		}
	}
	*out = next
	return nil
}
