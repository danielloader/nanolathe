package world

// ClassRestamp returns the current callback without installation proof.
func (t *Terrain) ClassRestamp() FootprintRestamp { return t.classRestamp }

// SetClassRestamp installs an ordinary feature-to-movement callback and clears
// the shared movement installation proof, even for a same-value reinstall.
func (t *Terrain) SetClassRestamp(fn FootprintRestamp) {
	if t != nil {
		t.classRestamp = fn
		t.checkpointMovement = nil
	}
}

// Movers returns the current occupancy adapter without installation proof.
func (t *Terrain) Movers() MobileOccupancy { return t.movers }

// SetMovers installs the mover half of the placement occupancy query and clears
// the shared movement installation proof, even for a same-value reinstall.
func (t *Terrain) SetMovers(movers MobileOccupancy) {
	if t != nil {
		t.movers = movers
		t.checkpointMovement = nil
	}
}
