package aikit

import (
	"bytes"
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// Independently authored bytes pin source-lexical order, signed widths, raw
// u32 factory handles, and complete stored slices (DESIGN_MULTIPLAYER §16.3.20).
const checkpointDedupeVector = "02000000ffffffff02000000" +
	"02000000ffffffff04000000" + "05000000" +
	"01000000faffffff" + "020000000700000000000080"

const checkpointGuardVector = "04030201" + "0102000000ffffffff02000000" +
	"cdab0000fdffffff04000000" + "020000000500000006000000" +
	"0700000001" + "0200000008000000f7ffffff" + "010000000a000000ffffffff"

const checkpointFreeVector = "01000000" +
	"0200000003000000040000000500000006000000f9ffffff" +
	"02000000ffffffff08000000" + "0100000009000000" + "0a000000" +
	"02000000" + "010b0000000c0000000d0000000e000000" + "00f1ffffff10000000efffffff12000000" +
	"020000001300000014000000" + "15000000" + "0100000016000000" +
	"1700000000" + "03000000010001"

const checkpointPendingVector = "ffffffff020000000300000004000000feffffffff"
const checkpointSelfVector = "0300000000000000" + "020000000300000009000000" + "01000000"

func checkpointDecode(t *testing.T, s string) []byte {
	t.Helper()
	v, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func checkpointLeafBytes(t *testing.T, write func(*checkpoint.Encoder) error) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := write(checkpoint.NewEncoder(&out)); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func checkpointDedupeFixture() gridDedupe {
	return gridDedupe{cellIdx: []int32{-1, 2}, cellStamp: []uint32{^uint32(0), 4}, gen: 5, unitIdx: []int32{-6}, unitStamp: []uint32{7, 1 << 31}}
}

func checkpointGuardFixture() exitGrid {
	return exitGrid{built: 0x01020304, cost: []int32{-1, 2}, fac: 0xabcd, ox: -3, oz: 4, reach: []uint32{5, 6}, reachStamp: 7,
		sealed: true, seeds: []int32{8, -9}, seen: []uint32{10}, stamp: ^uint32(0)}
}

func checkpointFreeFixture() freeCache {
	return freeCache{asked: 1, class: MoveClass{FootX: 2, FootZ: 3, MaxDepth: 4, MaxSlope: 5, MaxWaterSlope: 6, MinDepth: -7},
		comp: []int32{-1, 8}, compGen: []uint32{9}, gen: 10,
		regions: []freeRegion{{wide: true, x0: 11, x1: 12, z0: 13, z1: 14}, {x0: -15, x1: 16, z0: -17, z1: 18}},
		seen:    []uint32{19, 20}, seenStamp: 21, stamp: []uint32{22}, tick: 23, used: false, val: []bool{true, false, true}}
}

func TestCheckpointGridAuthoredVectors(t *testing.T) {
	d, g, f := checkpointDedupeFixture(), checkpointGuardFixture(), checkpointFreeFixture()
	p := pendingSite{cx: -1, cz: 2, fx: 3, fz: 4, g: 0xfe, tick: ^uint32(0)}
	s := exitGrid{cost: []int32{100, 200, 300}, seen: []uint32{3, 9}, stamp: 1}
	cases := []struct {
		name, want string
		write      func(*checkpoint.Encoder) error
	}{
		{"dedupe", checkpointDedupeVector, func(e *checkpoint.Encoder) error { return d.writeCheckpoint(e, "dedupe") }},
		{"guard", checkpointGuardVector, func(e *checkpoint.Encoder) error { return g.writeGuardCheckpoint(e, "guard") }},
		{"free", checkpointFreeVector, func(e *checkpoint.Encoder) error { return f.writeCheckpoint(e, "free") }},
		{"pending", checkpointPendingVector, func(e *checkpoint.Encoder) error { return p.writeCheckpoint(e, "pending") }},
		{"self", checkpointSelfVector, func(e *checkpoint.Encoder) error { return s.writeSelfCheckpoint(e, "self") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := checkpointLeafBytes(t, tc.write)
			if want := checkpointDecode(t, tc.want); !bytes.Equal(got, want) {
				t.Fatalf("got  %x\nwant %x", got, want)
			}
		})
	}
}

func TestCheckpointGuardPresenceAndSelfShape(t *testing.T) {
	g := exitGrid{}
	guard := func(e *checkpoint.Encoder) error { return g.writeGuardCheckpoint(e, "guard") }
	self := func(e *checkpoint.Encoder) error { return g.writeSelfCheckpoint(e, "self") }
	nilGuard, nilSelf := checkpointLeafBytes(t, guard), checkpointLeafBytes(t, self)
	g.cost = []int32{}
	if bytes.Equal(nilGuard, checkpointLeafBytes(t, guard)) {
		t.Fatal("guard nil/present picture gate vanished")
	}
	if !bytes.Equal(nilSelf, checkpointLeafBytes(t, self)) {
		t.Fatal("self cost nilness leaked beyond its length gate")
	}
	// Mismatched cost/seen lengths are preserved: ensure's actual length gate
	// is encoded without imposing an invented reconstruction invariant.
	g.cost = []int32{12, 34}
	g.seen = []uint32{99}
	before := checkpointLeafBytes(t, self)
	g.cost[0], g.cost[1] = -56, -78
	if !bytes.Equal(before, checkpointLeafBytes(t, self)) {
		t.Fatal("self cost values leaked")
	}
	g.cost = g.cost[:1]
	if bytes.Equal(before, checkpointLeafBytes(t, self)) {
		t.Fatal("self ensure length gate vanished")
	}
}

func TestCheckpointGridScratchAndPurity(t *testing.T) {
	g := checkpointGuardFixture()
	write := func(e *checkpoint.Encoder) error { return g.writeGuardCheckpoint(e, "guard") }
	before := checkpointLeafBytes(t, write)
	g.who, g.blk = []int32{91}, []gridBlocker{{feature: true, h: 92, cost: 93}}
	g.dd = &gridDedupe{gen: 94} // arbitrary stale alias is replaced before use
	g.dist, g.prev, g.queue, g.heap = []int32{95}, []int32{96}, []int32{97}, []int64{98}
	snapshot := g
	if !bytes.Equal(before, checkpointLeafBytes(t, write)) || !reflect.DeepEqual(g, snapshot) {
		t.Fatal("guard scratch was encoded or changed")
	}
	if !reflect.DeepEqual(g.cost, []int32{-1, 2}) || !reflect.DeepEqual(g.seen, []uint32{10}) || g.stamp != ^uint32(0) {
		t.Fatal("capture rebuilt or flooded a guard grid")
	}
	f := checkpointFreeFixture()
	free := func(e *checkpoint.Encoder) error { return f.writeCheckpoint(e, "free") }
	before = checkpointLeafBytes(t, free)
	f.queue = []int32{77, 88}
	if !bytes.Equal(before, checkpointLeafBytes(t, free)) || !reflect.DeepEqual(f.queue, []int32{77, 88}) || !reflect.DeepEqual(f, func() freeCache {
		v := checkpointFreeFixture()
		v.queue = []int32{77, 88}
		return v
	}()) {
		t.Fatal("free cache was walked, normalized or encoded with queue scratch")
	}
	self := func(e *checkpoint.Encoder) error { return g.writeSelfCheckpoint(e, "self") }
	before = checkpointLeafBytes(t, self)
	g.built, g.fac, g.ox, g.oz, g.reachStamp, g.sealed = 101, 102, 103, 104, 105, false
	g.reach, g.seeds = []uint32{106}, []int32{107}
	if !bytes.Equal(before, checkpointLeafBytes(t, self)) {
		t.Fatal("rebuilt self grid fields leaked")
	}
}

func TestCheckpointFreeCacheRetainsReuseAndEvictionKeys(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*freeCache)
	}{
		{"used", func(f *freeCache) { f.used = true }},
		{"asked", func(f *freeCache) { f.asked++ }},
		{"tick", func(f *freeCache) { f.tick++ }},
		{"gen", func(f *freeCache) { f.gen++ }},
		{"class", func(f *freeCache) { f.class.MinDepth-- }},
		{"compGen", func(f *freeCache) { f.compGen[0]++ }},
		{"stamp", func(f *freeCache) { f.stamp[0]++ }},
		{"seenStamp", func(f *freeCache) { f.seenStamp++ }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := checkpointFreeFixture()
			write := func(e *checkpoint.Encoder) error { return f.writeCheckpoint(e, "free") }
			before := checkpointLeafBytes(t, write)
			tc.edit(&f)
			if bytes.Equal(before, checkpointLeafBytes(t, write)) {
				t.Fatal("free-cache reuse/eviction key vanished")
			}
		})
	}
}

func TestCheckpointGridRetainsStaleGenerationAndOrder(t *testing.T) {
	d := checkpointDedupeFixture()
	d.gen = 1
	d.cellStamp = []uint32{1, ^uint32(0)}
	dedupe := func(e *checkpoint.Encoder) error { return d.writeCheckpoint(e, "dedupe") }
	before := checkpointLeafBytes(t, dedupe)
	d.cellIdx[1]++
	if bytes.Equal(before, checkpointLeafBytes(t, dedupe)) {
		t.Fatal("old dedupe index lost despite generation wrap")
	}
	f := checkpointFreeFixture() // used=false, with physical residual values
	free := func(e *checkpoint.Encoder) error { return f.writeCheckpoint(e, "free") }
	before = checkpointLeafBytes(t, free)
	f.regions[1].x0--
	if bytes.Equal(before, checkpointLeafBytes(t, free)) {
		t.Fatal("unused free-cache residual region vanished")
	}
	before = checkpointLeafBytes(t, free)
	f.regions[0], f.regions[1] = f.regions[1], f.regions[0]
	if bytes.Equal(before, checkpointLeafBytes(t, free)) {
		t.Fatal("stored region order lost")
	}
	g := checkpointGuardFixture()
	guard := func(e *checkpoint.Encoder) error { return g.writeGuardCheckpoint(e, "guard") }
	g.fac = 0 // an invalidated picture still owns its stored residual values
	before = checkpointLeafBytes(t, guard)
	g.seeds[0], g.seeds[1] = g.seeds[1], g.seeds[0]
	if bytes.Equal(before, checkpointLeafBytes(t, guard)) {
		t.Fatal("invalidated guard's seed order lost")
	}
}
