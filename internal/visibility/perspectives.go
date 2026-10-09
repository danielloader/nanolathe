package visibility

// perspectiveSensorMask contains the four sensor-owned bits [03 R-VIS-01 §4].
// Every other bit always comes from the current unit, never a retained bank.
const perspectiveSensorMask uint32 = sensorClearMask | DecloakBit

type perspectiveSensorStatus struct {
	allocationSerial uint64
	status           uint32
}

// EnableOwnerPerspectives selects the online sensor/history interpretation
// before battle entry (DESIGN_MULTIPLAYER §6.3, §16.4.1). It does not change
// grids, mode, local presentation, or an already retained sensor word.
func (s *Service) EnableOwnerPerspectives() {
	if s != nil {
		s.ownerPerspectives = true
	}
}

// StatusForPerspective reads one allocation's latched sensor bits, overlaid
// onto its current non-sensor flags. A new allocation starts with only its own
// perspective's sonar exemption [03 R-VIS-01 §4 Gate]. Serial zero is still
// inside creation, before identity commits, and must never borrow a prior
// occupant's contact (DESIGN_MULTIPLAYER §16.4.1).
func (s *Service) StatusForPerspective(owner PlayerID, id uint16, allocationSerial uint64, unitOwner PlayerID, fallback uint32) uint32 {
	if s == nil || !s.ownerPerspectives {
		return fallback
	}
	var status uint32
	if validPlayer(owner) {
		if owner == unitOwner {
			status = SonarBit
		}
		bank := s.perspectiveStatus[owner]
		if allocationSerial != 0 && int(id) < len(bank) && bank[id].allocationSerial == allocationSerial {
			status = bank[id].status
		}
	}
	return fallback&^perspectiveSensorMask | status&perspectiveSensorMask
}

// SensorTickForPerspective runs the existing sensor algorithm at one human's
// settlement deadline (DESIGN_MULTIPLAYER §6.3, §16.4.1). The input status words
// are read but never written; their sensor output belongs to this owner's
// allocation bank. The real shared reveal deadline is still written in place.
// The prototype has no hosted computers: only this human's units may perform
// the locally-simulated minimum-cloak source work [03 R-VIS-01 §4 pass 4].
func (s *Service) SensorTickForPerspective(owner PlayerID, defeated bool, tick uint32, activePlayers int, units []SensorUnit) {
	if s == nil || !s.ownerPerspectives || !validPlayer(owner) {
		return
	}
	if activePlayers <= 1 {
		// Preserve both the no-write gate and the ordinary diagnostic reset.
		s.sensorTick(tick, activePlayers, units, owner, defeated)
		return
	}

	// Size the whole bank before taking any row pointers. A later sparse slot
	// must not move earlier rows away from the pointers handed to the passes.
	size := len(s.perspectiveStatus[owner])
	for _, u := range units {
		if u.Alive && u.Status != nil && u.AllocationSerial != 0 && int(u.ID)+1 > size {
			size = int(u.ID) + 1
		}
	}
	bank := s.perspectiveStatus[owner]
	if len(bank) < size {
		bank = append(bank, make([]perspectiveSensorStatus, size-len(bank))...)
		s.perspectiveStatus[owner] = bank
	}
	if cap(s.perspectiveUnits) < len(units) {
		s.perspectiveUnits = make([]SensorUnit, len(units))
	}
	s.perspectiveUnits = s.perspectiveUnits[:len(units)]
	copy(s.perspectiveUnits, units)
	if cap(s.perspectiveTransient) < len(units) {
		s.perspectiveTransient = make([]uint32, len(units))
	}
	s.perspectiveTransient = s.perspectiveTransient[:len(units)]
	for i := range s.perspectiveUnits {
		u := &s.perspectiveUnits[i]
		u.OwnerLocallySimulated = u.OwnerLocallySimulated && u.Owner == owner
		if !u.Alive || u.Status == nil {
			continue
		}
		status := s.StatusForPerspective(owner, u.ID, u.AllocationSerial, u.Owner, *u.Status)
		if u.AllocationSerial == 0 {
			// An unfinished creation has fresh temporary status, never a bank
			// identity. Production phase-5 inputs are committed allocations.
			s.perspectiveTransient[i] = status
			u.Status = &s.perspectiveTransient[i]
			continue
		}
		row := &bank[u.ID]
		*row = perspectiveSensorStatus{allocationSerial: u.AllocationSerial, status: status}
		u.Status = &row.status
	}
	s.sensorTick(tick, activePlayers, s.perspectiveUnits, owner, defeated)
}

func (s *Service) historyPlayer(viewer PlayerID) PlayerID {
	if s.ownerPerspectives {
		return viewer
	}
	return s.local
}
