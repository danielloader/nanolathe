package session

import "github.com/nanolathe-gg/nanolathe/internal/content"

// EffectPoolOccupancy reads the fixed active-effect pool for diagnostics and
// tests: its live records now, the capacity its admission enforces, and how
// many admissions it has refused because it was full since the effect service
// was built (effects.EffectService.RefusedAtCapacity, never reset; shatter
// quads refused inside the pool are not in that count) [03 §1]. It reads only
// and allocates nothing; no tick reads any of it, and none of it is
// fingerprinted or saved. A session with no publication boundary reports
// zeros [I6].
func (s *Session) EffectPoolOccupancy() (live, capacity int, refusedAtCapacity uint64) {
	if s == nil || s.publication == nil || s.publication.effects == nil {
		return 0, 0, 0
	}
	live, capacity = s.publication.effects.Occupancy()
	return live, capacity, s.publication.effects.RefusedAtCapacity()
}

// SimArtDiagnostics reports the effect banks the battle's animation table could
// not compile, in sorted bank order, as rendered host diagnostics. Composition
// binds that table before the first unit script runs, so the list is final at
// battle entry; a session composed without one reports none.
func (s *Session) SimArtDiagnostics() []string {
	if s == nil {
		return nil
	}
	return renderEffectBankDiagnostics(s.simArt.Diagnostics())
}

func renderEffectBankDiagnostics(diagnostics []content.EffectBankDiagnostic) []string {
	if len(diagnostics) == 0 {
		return nil
	}
	out := make([]string, len(diagnostics))
	for i, d := range diagnostics {
		out[i] = d.String()
	}
	return out
}
