package visibility

// Reader accessors transfer the installed callbacks without checkpoint proof
// (DESIGN_MULTIPLAYER §16.3.49). The CommunityState may still be copied as a
// complete feature projection; capture checks its eventual service owner.
func (c *CommunityState) AlliedReader() func(PlayerID, PlayerID) bool { return c.allied }
func (c *CommunityState) OffMapReader() func(uint16) bool             { return c.offMap }

// SetAllied installs the reader and invalidates only this slot's proof, even
// when fn is the same callback returned by AlliedReader (§16.3.49).
func (c *CommunityState) SetAllied(fn func(PlayerID, PlayerID) bool) {
	c.allied = fn
	c.checkpointAllied = checkpointReaderProof{}
}

// SetOffMap installs the reader and invalidates only this slot's proof.
func (c *CommunityState) SetOffMap(fn func(uint16) bool) {
	c.offMap = fn
	c.checkpointOffMap = checkpointReaderProof{}
}
