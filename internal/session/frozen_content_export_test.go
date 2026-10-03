package session

import (
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// bindUnitCOB is the strict binding with a resolver of its own, the seam the
// composition tests call directly. Production binds through the battle's
// binder, which keeps one resolver for the whole battle.
func (s *Session) bindUnitCOB(fs vfs.FSOps, u *units.Unit) error {
	return s.bindUnitCOBWith(fs, battleModelResolver(fs), u)
}

// frozenSimulationInputs returns the frozen inputs this session's unit world
// creates units from, or nil for a battle composed without them (campaign
// missions and restores, which keep reading their live sources).
func (s *Session) frozenSimulationInputs() *content.SimulationInputs {
	if s == nil || s.Units == nil {
		return nil
	}
	fs, _ := s.Units.COBSource()
	return frozenInputsOf(fs)
}
