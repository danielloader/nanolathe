package ai

import (
	"bytes"
	"maps"
	"reflect"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

func TestCheckpointProfileVector(t *testing.T) {
	p := &Profile{Limit: map[string]int32{"z": -1, "A": 2}, Plan: "hard", Weight: map[string]int32{},
		allLimits: map[Difficulty]map[string]int32{"z": nil, "a": {}}, directives: []content.AIDirective{{Keyword: "weight", Args: []string{"Same", "0.5"}}, {Keyword: "PLAN"}},
		fixtureWeights: map[string]int32{}, limitsByID: map[uint32]int32{9: -2, 1: 3}, name: "x", recordIDs: map[*content.UnitDef]uint32{}, textLoaded: true, weightsByID: map[uint32]int32{}}
	var out bytes.Buffer
	if err := p.writeCheckpoint(checkpoint.NewEncoder(&out), aiCheckpointContext(t)); err != nil {
		t.Fatal(err)
	}
	want := aiCheckpointHex(t, "0102000000010000004102000000010000007affffffff04000000686172640100000000010200000001000000610100000000010000007a00000002000000020000000400000053616d6503000000302e35060000007765696768740000000004000000504c414e0001000000000102000000010000000300000009000000feffffff01000000780100000000010100000000")
	if !bytes.Equal(out.Bytes(), want) {
		t.Fatalf("got %x\nwant %x", out.Bytes(), want)
	}
}

func TestCheckpointProfilePresenceOrderAndPurity(t *testing.T) {
	c := aiCheckpointContext(t)
	baseline := aiCheckpointBytes(t, &Manager{Profile: &Profile{}}, c)
	for name, edit := range map[string]func(*Profile){
		"Limit": func(p *Profile) { p.Limit = map[string]int32{} }, "Weight": func(p *Profile) { p.Weight = map[string]int32{} },
		"allLimits": func(p *Profile) { p.allLimits = map[Difficulty]map[string]int32{} }, "allWeights": func(p *Profile) { p.allWeights = map[Difficulty]map[string]int32{} },
		"fixtureLimits": func(p *Profile) { p.fixtureLimits = map[string]int32{} }, "fixtureWeights": func(p *Profile) { p.fixtureWeights = map[string]int32{} },
		"limitsByID": func(p *Profile) { p.limitsByID = map[uint32]int32{} }, "weightsByID": func(p *Profile) { p.weightsByID = map[uint32]int32{} },
		"recordIDs": func(p *Profile) { p.recordIDs = map[*content.UnitDef]uint32{} }, "Plan": func(p *Profile) { p.Plan = "RAW" }, "name": func(p *Profile) { p.name = "RAW" }, "textLoaded": func(p *Profile) { p.textLoaded = true },
	} {
		t.Run(name, func(t *testing.T) {
			p := &Profile{}
			edit(p)
			if bytes.Equal(baseline, aiCheckpointBytes(t, &Manager{Profile: p}, c)) {
				t.Fatal("presence/value disappeared")
			}
		})
	}
	p := &Profile{allLimits: map[Difficulty]map[string]int32{"x": nil}, Weight: map[string]int32{"a": 1, "z": 2}, directives: []content.AIDirective{{Keyword: "x", Args: []string{"A", "B"}}, {Keyword: "y"}}}
	first := aiCheckpointBytes(t, &Manager{Profile: p}, c)
	p.allLimits["x"] = map[string]int32{}
	if bytes.Equal(first, aiCheckpointBytes(t, &Manager{Profile: p}, c)) {
		t.Fatal("nested map presence disappeared")
	}
	first = aiCheckpointBytes(t, &Manager{Profile: p}, c)
	p.Weight = map[string]int32{"z": 2, "a": 1}
	if !bytes.Equal(first, aiCheckpointBytes(t, &Manager{Profile: p}, c)) {
		t.Fatal("map construction order entered bytes")
	}
	p.directives[0], p.directives[1] = p.directives[1], p.directives[0]
	if bytes.Equal(first, aiCheckpointBytes(t, &Manager{Profile: p}, c)) {
		t.Fatal("directive order collapsed")
	}
	before := *p
	before.Weight = maps.Clone(p.Weight)
	before.allLimits = maps.Clone(p.allLimits)
	for key, row := range before.allLimits {
		before.allLimits[key] = maps.Clone(row)
	}
	before.directives = append([]content.AIDirective(nil), p.directives...)
	for i := range before.directives {
		before.directives[i].Args = append([]string(nil), p.directives[i].Args...)
	}
	aiCheckpointBytes(t, &Manager{Profile: p}, c)
	if !reflect.DeepEqual(before, *p) {
		t.Fatal("profile was applied or normalized")
	}
}

func TestCheckpointProfileDefinitionIdentity(t *testing.T) {
	a := &content.UnitDef{DefinitionHeader: content.DefinitionHeader{CanonicalKey: "same"}, UnitName: "same"}
	b := *a
	c := aiCheckpointContext(t, a, &b)
	p := &Profile{recordIDs: map[*content.UnitDef]uint32{&b: 3, a: 9}}
	rows, err := p.checkpointRecordIDs(c)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].key.Ordinal != 1 || rows[0].id != 9 || rows[1].key.Ordinal != 2 || rows[1].id != 3 {
		t.Fatalf("definition identity/order %+v", rows)
	}
	var out bytes.Buffer
	if err := p.writeCheckpoint(checkpoint.NewEncoder(&out), c); err != nil {
		t.Fatal(err)
	}
	// All fields around the record-ID map are nil/empty. Equal canonical names
	// still have independent manifest ordinals, and numeric IDs do not sort them.
	want := aiCheckpointHex(t, "0000000000000000000000000000000000000000"+"0102000000010100000009000000756e69742f73616d6509000000010200000009000000756e69742f73616d65030000000000")
	if !bytes.Equal(out.Bytes(), want) {
		t.Fatalf("profile identities got %x want %x", out.Bytes(), want)
	}
	first := aiCheckpointBytes(t, &Manager{Profile: p}, c)
	p.recordIDs = map[*content.UnitDef]uint32{a: 9, &b: 3}
	if !bytes.Equal(first, aiCheckpointBytes(t, &Manager{Profile: p}, c)) {
		t.Fatal("pointer map iteration entered bytes")
	}
	foreign := *a
	p.recordIDs[&foreign] = 4
	if _, err := (&Manager{Profile: p}).CollectCheckpointReferences(c); err == nil {
		t.Fatal("equal-name foreign definition admitted")
	}
	delete(p.recordIDs, &foreign)
	p.recordIDs[nil] = 4
	if _, err := (&Manager{Profile: p}).CollectCheckpointReferences(c); err == nil {
		t.Fatal("nil definition key admitted")
	}
}
