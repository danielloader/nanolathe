package aikit

import (
	"fmt"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// These records preserve retained cache values, including expired slots and
// old generation tags. No ensure, flood or cache lookup runs during capture
// (DESIGN_MULTIPLAYER §16.3.20).
func (d *gridDedupe) writeCheckpoint(enc *checkpoint.Encoder, path string) error {
	enc.Field(path)
	if d == nil {
		enc.Fail(executorCheckpointError(path, "a present grid dedupe record"))
		return enc.Err()
	}
	writeExecutorCheckpointI32s(enc, path+".cellIdx", d.cellIdx)
	writeExecutorCheckpointU32s(enc, path+".cellStamp", d.cellStamp)
	enc.Field(path + ".gen")
	enc.U32(d.gen)
	writeExecutorCheckpointI32s(enc, path+".unitIdx", d.unitIdx)
	writeExecutorCheckpointU32s(enc, path+".unitStamp", d.unitStamp)
	return enc.Err()
}

func (g *exitGrid) writeGuardCheckpoint(enc *checkpoint.Encoder, path string) error {
	enc.Field(path)
	if g == nil {
		enc.Fail(executorCheckpointError(path, "a present guard grid"))
		return enc.Err()
	}
	if g.checkpointObservation.attempt != nil {
		enc.Field(path + ".checkpointObservation")
		enc.Fail(executorCheckpointError(path+".checkpointObservation", "a completed application boundary"))
		return enc.Err()
	}
	enc.Field(path + ".built")
	enc.U32(g.built)
	// The nil test gates gridFor/gridCurrent/guardPlaced, independently of
	// length. who/blk/dd are rebuilt before their only blocker consumer.
	enc.Field(path + ".cost")
	enc.Bool(g.cost != nil)
	writeExecutorCheckpointI32s(enc, path+".cost", g.cost)
	enc.Field(path + ".fac")
	enc.U32(uint32(g.fac))
	enc.Field(path + ".ox")
	enc.I32(g.ox)
	enc.Field(path + ".oz")
	enc.I32(g.oz)
	writeExecutorCheckpointU32s(enc, path+".reach", g.reach)
	enc.Field(path + ".reachStamp")
	enc.U32(g.reachStamp)
	enc.Field(path + ".sealed")
	enc.Bool(g.sealed)
	writeExecutorCheckpointI32s(enc, path+".seeds", g.seeds)
	writeExecutorCheckpointU32s(enc, path+".seen", g.seen)
	enc.Field(path + ".stamp")
	enc.U32(g.stamp)
	return enc.Err()
}

func (g *exitGrid) writeSelfCheckpoint(enc *checkpoint.Encoder, path string) error {
	enc.Field(path)
	if g == nil {
		enc.Fail(executorCheckpointError(path, "a present self grid"))
		return enc.Err()
	}
	// ensure's cost length gate decides whether seen is cleared. The cost
	// values and all other self-grid fields are rebuilt before consumption.
	enc.Field(path + ".cost")
	enc.I64(int64(len(g.cost)))
	writeExecutorCheckpointU32s(enc, path+".seen", g.seen)
	enc.Field(path + ".stamp")
	enc.U32(g.stamp)
	return enc.Err()
}

func (f *freeCache) writeCheckpoint(enc *checkpoint.Encoder, path string) error {
	enc.Field(path)
	if f == nil {
		enc.Fail(executorCheckpointError(path, "a present free cache"))
		return enc.Err()
	}
	enc.Field(path + ".asked")
	enc.U32(f.asked)
	enc.Field(path + ".class.FootX")
	enc.I32(f.class.FootX)
	enc.Field(path + ".class.FootZ")
	enc.I32(f.class.FootZ)
	enc.Field(path + ".class.MaxDepth")
	enc.I32(f.class.MaxDepth)
	enc.Field(path + ".class.MaxSlope")
	enc.I32(f.class.MaxSlope)
	enc.Field(path + ".class.MaxWaterSlope")
	enc.I32(f.class.MaxWaterSlope)
	enc.Field(path + ".class.MinDepth")
	enc.I32(f.class.MinDepth)
	writeExecutorCheckpointI32s(enc, path+".comp", f.comp)
	writeExecutorCheckpointU32s(enc, path+".compGen", f.compGen)
	enc.Field(path + ".gen")
	enc.U32(f.gen)
	enc.Field(path + ".regions")
	enc.Count(len(f.regions))
	for i := range f.regions {
		r := &f.regions[i]
		p := fmt.Sprintf("%s.regions[%d]", path, i)
		enc.Field(p + ".wide")
		enc.Bool(r.wide)
		enc.Field(p + ".x0")
		enc.I32(r.x0)
		enc.Field(p + ".x1")
		enc.I32(r.x1)
		enc.Field(p + ".z0")
		enc.I32(r.z0)
		enc.Field(p + ".z1")
		enc.I32(r.z1)
	}
	writeExecutorCheckpointU32s(enc, path+".seen", f.seen)
	enc.Field(path + ".seenStamp")
	enc.U32(f.seenStamp)
	writeExecutorCheckpointU32s(enc, path+".stamp", f.stamp)
	enc.Field(path + ".tick")
	enc.U32(f.tick)
	enc.Field(path + ".used")
	enc.Bool(f.used)
	writeExecutorCheckpointBools(enc, path+".val", f.val)
	return enc.Err()
}

func (p *pendingSite) writeCheckpoint(enc *checkpoint.Encoder, path string) error {
	enc.Field(path)
	if p == nil {
		enc.Fail(executorCheckpointError(path, "a present pending reservation"))
		return enc.Err()
	}
	enc.Field(path + ".cx")
	enc.I32(p.cx)
	enc.Field(path + ".cz")
	enc.I32(p.cz)
	enc.Field(path + ".fx")
	enc.I32(p.fx)
	enc.Field(path + ".fz")
	enc.I32(p.fz)
	enc.Field(path + ".g")
	enc.U8(uint8(p.g))
	enc.Field(path + ".tick")
	enc.U32(p.tick)
	return enc.Err()
}

func writeExecutorCheckpointI32s(enc *checkpoint.Encoder, path string, values []int32) {
	enc.Field(path)
	enc.Count(len(values))
	for _, value := range values {
		enc.I32(value)
	}
}

func writeExecutorCheckpointU32s(enc *checkpoint.Encoder, path string, values []uint32) {
	enc.Field(path)
	enc.Count(len(values))
	for _, value := range values {
		enc.U32(value)
	}
}

func writeExecutorCheckpointBools(enc *checkpoint.Encoder, path string, values []bool) {
	enc.Field(path)
	enc.Count(len(values))
	for _, value := range values {
		enc.Bool(value)
	}
}
