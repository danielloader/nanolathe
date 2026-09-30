// Package pathlab is the engine half of the pathfinding laboratory
// (docs/PATHFINDING_LAB.md). It stages a snippet of a recorded game — the
// map, the structures and units that stood in a region at one tick, and the
// orders the recording shows those units received — in an ordinary session,
// runs it under a named gameplay rule set, and writes what each unit's route
// follower did in the same shape tools/tad-extract writes for a recording
// [fmt tad §7]. tools/path-lab then measures both with one set of
// definitions, so "the recorded game took this long, this build takes that
// long" compares like with like.
//
// Nothing here is read by the simulation: the package observes committed
// state after a completed tick [I6] and creates units and orders only
// through the session's ordinary staging calls.
package pathlab
