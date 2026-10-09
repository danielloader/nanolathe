package pool

import (
	"fmt"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// WriteCheckpoint retains capacity, count, dead in lexical order, including
// flags beyond the active prefix. Diagnostic payload and compaction storage
// are excluded (DESIGN_MULTIPLAYER §16.3.5–§16.3.6). The lazy zero-value pool
// keeps its actual zero capacity word and absent dead row; capture never sizes it.
func (p *Projectiles) WriteCheckpoint(e *checkpoint.Encoder) error {
	e.Field("pool.Projectiles")
	if p == nil {
		e.Fail(fmt.Errorf("missing projectile pool"))
		return e.Err()
	}
	capacity := p.cap()
	if p.capacity < 0 || capacity > MaxProjectileCapacity || p.count < 0 || p.count > capacity {
		e.Fail(fmt.Errorf("invalid projectile count or capacity"))
		return e.Err()
	}
	if len(p.dead) != capacity && (len(p.dead) != 0 || p.count != 0) {
		e.Fail(fmt.Errorf("projectile dead flags do not cover capacity"))
		return e.Err()
	}
	e.Field("pool.Projectiles.capacity")
	e.I64(int64(p.capacity))
	e.Field("pool.Projectiles.count")
	e.I64(int64(p.count))
	e.Field("pool.Projectiles.dead")
	e.Count(len(p.dead))
	for _, dead := range p.dead {
		e.Bool(dead)
	}
	return e.Err()
}
