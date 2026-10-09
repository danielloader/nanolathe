package movement

import (
	"errors"
	"fmt"
	"slices"

	"github.com/nanolathe-gg/nanolathe/internal/path"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

func checkpointRequestKeys(row map[pool.Handle]path.Request) []pool.Handle {
	keys := make([]pool.Handle, 0, len(row))
	for key := range row {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func validateMovementProvider(s *System, p *pathProvider, c *CheckpointContext) error {
	if p.eligibleValid {
		return errors.New("provider eligibility cache is active")
	}
	if p.system != nil && p.system != s || p.world != nil && p.world != s.world {
		return errors.New("provider owner alias differs")
	}
	if c.pathBindings.authority != nil {
		if p != c.pathBindings.provider {
			return errors.New("provider differs from path binding registration")
		}
		if err := c.pathBindings.validate(s); err != nil {
			return err
		}
	} else {
		if p.world != nil {
			return errors.New("unattested provider world binding")
		}
		if p.eligible != nil {
			return errors.New("unattested path eligibility binding")
		}
	}
	staged := make([]uint64, len(p.staged))
	for _, row := range p.requests {
		for _, key := range checkpointRequestKeys(row) {
			word := int(key) >> 6
			if word >= len(staged) {
				return errors.New("staged index omits a request")
			}
			staged[word] |= uint64(1) << (uint(key) & 63)
		}
	}
	if !slices.Equal(staged, p.staged) {
		return errors.New("staged index differs from request keys")
	}
	return nil
}

// WritePathProviderCheckpoint is the first fragment of section8. It writes
// provider presence, then cursor, eligible, limit, players, requests, started,
// system, tick, world. Every actual map key is retained, independent of today's
// unit slices; staged/eligibleNow are derived or reset scratch.
func (s *System) WritePathProviderCheckpoint(e *checkpoint.Encoder, c *CheckpointContext) error {
	e.Field("paths.provider")
	if err := s.checkpointContext(c); err != nil {
		e.Fail(err)
		return e.Err()
	}
	p := s.pathProvider
	e.Bool(p != nil)
	if p == nil {
		return e.Err()
	}
	if err := validateMovementProvider(s, p, c); err != nil {
		e.Fail(err)
		return e.Err()
	}
	m := movementCheckpointRecord{e, c, s, "paths.provider"}
	m.field("cursor")
	for _, x := range p.cursor {
		e.I64(int64(x))
	}
	m.boolean("eligible", p.eligible != nil)
	m.i32("limit", p.limit)
	m.i64("players", int64(p.players))
	for player, row := range p.requests {
		keys := checkpointRequestKeys(row)
		q := fmt.Sprintf("requests[%d]", player)
		m.count(q, len(keys))
		for i, key := range keys {
			v := row[key]
			r := fmt.Sprintf("%s[%d]", q, i)
			m.u32(r+".key", uint32(key))
			m.u64(r+".Activation", v.Activation)
			m.goal(r+".Goal", v.Goal)
			m.u8(r+".Player", v.Player)
			m.cell(r+".Start", Cell(v.Start))
			m.u32(r+".Unit", uint32(v.Unit))
		}
	}
	m.field("started")
	for _, x := range p.started {
		e.Bool(x)
	}
	m.boolean("system", p.system != nil)
	m.u32("tick", p.tick)
	m.boolean("world", p.world != nil)
	return e.Err()
}
