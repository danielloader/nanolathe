package pool

// UnitRef identifies one successful unit creation for delayed human commands
// (DESIGN_MULTIPLAYER §7.4.4). Retail simulation references remain raw Handles;
// the serial adds no generation check to the allocator [I5]. Zero is invalid.
type UnitRef struct {
	Handle Handle
	Serial uint64
}
