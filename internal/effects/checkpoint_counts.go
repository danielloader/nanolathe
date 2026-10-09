package effects

// CheckpointCounts reads record and live geometry-slot counts without copying
// presentation metadata or visiting vertices (DESIGN_MULTIPLAYER §16.3.78).
func (p *FixedEffectPool) CheckpointCounts() (records, fragments int) {
	if p == nil {
		return 0, 0
	}
	for i := range p.fragments {
		if p.fragments[i].live {
			fragments++
		}
	}
	return len(p.records), fragments
}
