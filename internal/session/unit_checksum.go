package session

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// UnitStateChecksum is the deliberately small play-test check approved in
// DESIGN_MULTIPLAYER §16.4. Call between ticks. It observes live allocations,
// ownership, positions and health; hidden-state differences are detected only
// if they later affect these fields. It neither captures nor restores a world.
func (s *Session) UnitStateChecksum() [32]byte {
	h := sha256.New()
	h.Write([]byte("nanolathe/playtest-units/v1"))
	var row [40]byte
	if s != nil && s.Clock != nil {
		binary.LittleEndian.PutUint32(row[:4], s.Clock.GlobalTick)
	}
	h.Write(row[:4])
	if s != nil && s.Units != nil {
		for player := 0; player < 10; player++ {
			s.Units.ForEachPlayerSliceLive(player, func(u *units.Unit) {
				binary.LittleEndian.PutUint16(row[:2], uint16(u.Handle))
				binary.LittleEndian.PutUint64(row[2:10], u.AllocationSerial)
				row[10], row[11] = u.Owner, 0
				if u.Dying {
					row[11] = 1
				}
				binary.LittleEndian.PutUint64(row[12:20], uint64(u.X))
				binary.LittleEndian.PutUint64(row[20:28], uint64(u.Y))
				binary.LittleEndian.PutUint64(row[28:36], uint64(u.Z))
				binary.LittleEndian.PutUint32(row[36:40], uint32(u.Health))
				h.Write(row[:])
			})
		}
	}
	var out [32]byte
	h.Sum(out[:0])
	return out
}
