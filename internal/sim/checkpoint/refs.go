package checkpoint

import (
	"errors"
	"math"
)

// References assigns capture-local IDs in first-encounter order. The zero
// value is ready to use, and the zero value of T denotes an absent reference.
// Callers validate closed pointer variants before interning interface values.
// The lookup is never iterated; Values preserves discovery order [I1].
type References[T comparable] struct {
	ids    map[T]ObjectID
	values []T
}

func (r *References[T]) Add(value T) (ObjectID, error) {
	if id, ok := r.Find(value); ok {
		return id, nil
	}
	id, err := nextObjectID(uint64(len(r.values)))
	if err != nil {
		return 0, err
	}
	if r.ids == nil {
		r.ids = make(map[T]ObjectID)
	}
	r.ids[value] = id
	r.values = append(r.values, value)
	return id, nil
}

func nextObjectID(count uint64) (ObjectID, error) {
	if count >= math.MaxUint32 {
		return 0, contextualError(0, "references", errors.New("object ID exhausted"))
	}
	return ObjectID(count + 1), nil
}

// Find never registers a value. An absent value is always known as ID zero.
func (r *References[T]) Find(value T) (ObjectID, bool) {
	var absent T
	if value == absent {
		return 0, true
	}
	id, ok := r.ids[value]
	return id, ok
}

// Values returns a detached list in assigned-ID order, excluding absent.
// Referenced objects themselves are not copied.
func (r *References[T]) Values() []T {
	return append([]T(nil), r.values...)
}

// Summary is the cheap scalar fold in DESIGN_MULTIPLAYER §16.3.7. It is not
// an equality proof. Owners choose and validate their scalar words; the zero
// value is ready to use and nested owners append to the same accumulator.
type Summary struct {
	words, sum uint64
}

// Word increments the count then adds count * v, all modulo 2^64.
func (s *Summary) Word(v uint64) {
	s.words++
	s.sum += s.words * v
}

func (s *Summary) Result() (words uint64, sum uint64) {
	return s.words, s.sum
}
