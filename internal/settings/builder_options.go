package settings

import "github.com/nanolathe-gg/nanolathe/internal/gameplay"

// BuilderOptions persists the local player's preferences. Array indices are
// Hold Position, Maneuver and Roam; values follow the labels in
// DESIGN_COMMUNITY_PATCH §4.3. This leaf package keeps only configuration data.
type BuilderOptions struct {
	Guard  [3]int `json:"guard"`
	Patrol [3]int `json:"patrol"`
}

func DefaultBuilderOptions() BuilderOptions {
	return DefaultBuilderOptionsForMode(gameplay.Modern)
}

// DefaultBuilderOptionsForMode supplies host defaults, not a simulation rule.
// Saved explicit preferences survive mode changes; the bound orders rules own
// battle defaults (DESIGN_UNITS_ORDERS_COB "Modern patrol work").
func DefaultBuilderOptionsForMode(mode gameplay.Mode) BuilderOptions {
	mode = mode.ReservedBase()
	b := BuilderOptions{Guard: [3]int{1, 1, 1}, Patrol: [3]int{1, 1, 1}}
	if mode == gameplay.Strict31 || mode == gameplay.Community39 {
		b.Patrol[0] = 0
	}
	return b
}

func (b *BuilderOptions) Normalize() {
	for i := range b.Guard {
		if b.Guard[i] < 0 || b.Guard[i] > 2 {
			b.Guard[i] = 1 // Cavedog
		}
		if b.Patrol[i] < 0 || b.Patrol[i] > 2 {
			b.Patrol[i] = 1 // Both
		}
	}
}
