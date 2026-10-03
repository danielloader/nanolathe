package session

import (
	"github.com/nanolathe-gg/nanolathe/internal/mission"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
)

// applyDeveloperSpawnCommand is the default developer handler [07 R-CAM-01
// §6]. UnitRecords omits the sentinel and preserves every retained definition,
// including duplicate names, in catalog order. The producer checked access
// before enqueue; accepted replay requests need no host developer state.
func (s *Session) applyDeveloperSpawnCommand(c HumanDeveloperSpawnCommand) {
	if s.Catalog == nil || s.World == nil || s.Units == nil {
		return
	}
	x, y, z := c.X, c.Y, c.Z
	matched := false
	for _, def := range s.Catalog.UnitRecords() {
		if def == nil || !developerUnitPattern(c.Pattern, def.UnitName) {
			continue
		}
		halfWidth := numeric.Fixed(def.FootprintX) * (8 << 16)
		if matched {
			x += halfWidth
		}
		x, y, z = missionPlacementPosition(s.World, def, mission.UnitPlacement{X: int32(x), Y: int32(y), Z: int32(z)})
		h, err := s.Units.Create(def, c.Owner, x, y, z)
		if err == nil && s.Movement != nil {
			s.Movement.EnsureUnit(s.Units.Unit(h))
		}
		// Failed allocations still count as matches and advance the point.
		// Compare after the step, inclusively at the play-area right edge.
		matched = true
		x += (32 << 16) + halfWidth
		if x >= numeric.Fixed(s.World.PlayRight)<<16 {
			z += 160 << 16
			x = 160 << 16
		}
	}
	// TODO(question): the established debugdat command-script fallback for
	// no matches remains unimplemented. Implement Include's typed command
	// routing before running scripts here [01 R-PLAT-01 §9]. Unmatched names
	// stay silent (DESIGN_DEVELOPER_TOOLS §8).
}

// developerUnitPattern matches a whole byte string: '?' consumes one byte,
// '*' consumes any run (including empty), and literals fold ASCII case
// [07 R-CAM-01 §6]. A remembered star permits backtracking without recursion.
// TODO(question): trace retail case folding outside ASCII [02 R-CAT-01 §3];
// keep those bytes literal, matching the catalog’s existing placeholder.
func developerUnitPattern(pattern, name string) bool {
	p, n, star, retry := 0, 0, -1, 0
	upper := func(b byte) byte {
		if b >= 'a' && b <= 'z' {
			return b - ('a' - 'A')
		}
		return b
	}
	for n < len(name) {
		if p < len(pattern) && pattern[p] == '*' {
			star, retry = p, n
			p++
		} else if p < len(pattern) && (pattern[p] == '?' || upper(pattern[p]) == upper(name[n])) {
			p++
			n++
		} else if star >= 0 {
			retry++
			p, n = star+1, retry
		} else {
			return false
		}
	}
	for p < len(pattern) && pattern[p] == '*' {
		p++
	}
	return p == len(pattern)
}
