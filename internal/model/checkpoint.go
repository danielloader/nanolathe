package model

import "github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"

// WriteCheckpoint retains RotX, RotY, RotZ, Trans, in lexical field order.
// The four presentation booleans derive from the active piece-flag store,
// which units owns for a production unit and the VM owns for an unbound
// fixture (DESIGN_MULTIPLAYER §16.3.5).
func (s *PieceState) WriteCheckpoint(e *checkpoint.Encoder) error {
	e.Field("PieceState.RotX")
	e.U16(s.RotX)
	e.Field("PieceState.RotY")
	e.U16(s.RotY)
	e.Field("PieceState.RotZ")
	e.U16(s.RotZ)
	e.Field("PieceState.Trans")
	for _, value := range s.Trans {
		e.I64(int64(value))
	}
	return e.Err()
}
