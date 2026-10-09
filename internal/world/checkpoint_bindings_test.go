package world

import (
	"bytes"
	"encoding/hex"
	"reflect"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

type checkpointRestampOwner struct {
	calls  int
	x, z   int32
	fx, fz int16
}

func (o *checkpointRestampOwner) NoteFeatureFootprint(x, z int32, fx, fz int16) {
	if o == nil {
		panic("typed nil restamp owner called")
	}
	o.calls++
	o.x, o.z, o.fx, o.fz = x, z, fx, fz
}

type checkpointOpaqueMovement []byte

func (checkpointOpaqueMovement) NoteFeatureFootprint(int32, int32, int16, int16) {
	panic("opaque restamp owner called")
}
func (checkpointOpaqueMovement) CellOccupant(int32, int32) uint16 {
	panic("opaque occupancy adapter called")
}

func checkpointBindingTerrain() *Terrain {
	return &Terrain{CellW: 1, CellH: 1, Plot: []PlotCell{{}}, voidSwept: true}
}

func requireTerrainBindingRefusal(t *testing.T, terrain *Terrain, c *CheckpointContext) {
	t.Helper()
	before := *c
	if _, err := terrain.CollectCheckpointReferences(c); err == nil || !strings.HasPrefix(err.Error(), "nanolathe: checkpoint capture failed: logical path ") {
		t.Fatalf("collector error = %v", err)
	}
	if *c != before {
		t.Fatal("failed collection changed context")
	}
	var out bytes.Buffer
	if err := terrain.WriteCheckpoint(checkpoint.NewEncoder(&out), c); err == nil {
		t.Fatal("writer accepted invalid binding")
	}
	if out.Len() != 0 {
		t.Fatal("failed preflight wrote bytes")
	}
}

// The only changed bytes are the two existing lexical presence fields. No
// owner identity or receipt data enters the stream (DESIGN_MULTIPLAYER §16.3.57).
func TestCheckpointMovementBindingVectorsAndPurity(t *testing.T) {
	keys := worldCheckpointKeys(t)
	for _, tc := range []struct {
		name          string
		bound, movers bool
		tags          string
	}{
		{"absent", false, false, "00"},
		{"restamp only", true, false, "01"},
		{"both", true, true, "01"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			terrain := checkpointBindingTerrain()
			c := NewCheckpointContext(keys)
			owner := &checkpointRestampOwner{}
			if tc.movers {
				terrain.SetMovers(checkpointOpaqueMovement{9})
			}
			if tc.bound {
				authority := checkpoint.NewBindingAuthority()
				terrain.SetClassRestampOwnerWithCheckpointBinding(owner, authority)
				if err := c.SetMovementBindings(terrain, authority); err != nil {
					t.Fatal(err)
				}
			}
			before := *terrain
			before.Plot = append([]PlotCell(nil), terrain.Plot...)
			before.classRestamp = nil // method values are deliberately never compared
			if n, err := terrain.CollectCheckpointReferences(c); err != nil || n != 0 {
				t.Fatalf("collect = %d, %v", n, err)
			}
			registered := *c
			var out bytes.Buffer
			if err := terrain.WriteCheckpoint(checkpoint.NewEncoder(&out), c); err != nil {
				t.Fatal(err)
			}
			moverTag := "00"
			if tc.movers {
				moverTag = "01"
			}
			want, err := hex.DecodeString(tc.tags + "00000000" + "00000000" + moverTag + "01000000" + "00000000000000000000" + "00")
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(out.Bytes(), want) {
				t.Fatalf("payload = %x, want %x", out.Bytes(), want)
			}
			after := *terrain
			after.classRestamp = nil
			if !reflect.DeepEqual(before, after) || *c != registered || owner.calls != 0 {
				t.Fatal("capture changed terrain/context or called the owner")
			}
		})
	}
}

func TestCheckpointMovementBindingOwnerExtraction(t *testing.T) {
	terrain := checkpointBindingTerrain()
	authority := checkpoint.NewBindingAuthority()
	owner := &checkpointRestampOwner{}
	ownerCopy := *owner
	terrain.SetClassRestampOwnerWithCheckpointBinding(owner, authority)
	got, ok := terrain.CheckpointMovementOwner(authority)
	if !ok || got != owner || got == &ownerCopy || owner.calls != 0 {
		t.Fatal("installer did not retain its exact, uncalled owner")
	}
	terrain.NoteFootprintRestamp(-7, 11, 0, -3)
	if owner.calls != 1 || owner.x != -7 || owner.z != 11 || owner.fx != 1 || owner.fz != 1 || ownerCopy.calls != 0 {
		t.Fatalf("ordinary restamp behavior changed: %+v", owner)
	}
	if _, ok := terrain.CheckpointMovementOwner(nil); ok {
		t.Fatal("absent authority accepted")
	}
	if _, ok := terrain.CheckpointMovementOwner(checkpoint.NewBindingAuthority()); ok {
		t.Fatal("foreign authority accepted")
	}
	copy := *terrain
	if _, ok := copy.CheckpointMovementOwner(authority); ok {
		t.Fatal("copied terrain borrowed receipt")
	}
	c := NewCheckpointContext(worldCheckpointKeys(t))
	if err := c.SetMovementBindings(&copy, authority); err == nil {
		t.Fatal("copied terrain registered")
	}
	requireTerrainBindingRefusal(t, &copy, c)
}

func TestCheckpointMovementBindingReplacement(t *testing.T) {
	keys := worldCheckpointKeys(t)
	for _, tc := range []struct {
		name string
		edit func(*Terrain, *checkpointRestampOwner, *checkpoint.BindingAuthority)
	}{
		{"callback extracted reinstall", func(v *Terrain, _ *checkpointRestampOwner, _ *checkpoint.BindingAuthority) {
			v.SetClassRestamp(v.ClassRestamp())
		}},
		{"movers extracted reinstall", func(v *Terrain, _ *checkpointRestampOwner, _ *checkpoint.BindingAuthority) { v.SetMovers(v.Movers()) }},
		{"clear callback", func(v *Terrain, _ *checkpointRestampOwner, _ *checkpoint.BindingAuthority) { v.SetClassRestamp(nil) }},
		{"clear both", func(v *Terrain, _ *checkpointRestampOwner, _ *checkpoint.BindingAuthority) {
			v.SetClassRestamp(nil)
			v.SetMovers(nil)
		}},
		{"admitted same owner", func(v *Terrain, o *checkpointRestampOwner, a *checkpoint.BindingAuthority) {
			v.SetClassRestampOwnerWithCheckpointBinding(o, a)
		}},
		{"admitted foreign owner same authority", func(v *Terrain, _ *checkpointRestampOwner, a *checkpoint.BindingAuthority) {
			v.SetClassRestampOwnerWithCheckpointBinding(&checkpointRestampOwner{}, a)
		}},
		{"nil owner", func(v *Terrain, _ *checkpointRestampOwner, a *checkpoint.BindingAuthority) {
			v.SetClassRestampOwnerWithCheckpointBinding(nil, a)
		}},
		{"nil authority", func(v *Terrain, o *checkpointRestampOwner, _ *checkpoint.BindingAuthority) {
			v.SetClassRestampOwnerWithCheckpointBinding(o, nil)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			terrain := checkpointBindingTerrain()
			terrain.SetMovers(checkpointOpaqueMovement{1})
			owner, authority := &checkpointRestampOwner{}, checkpoint.NewBindingAuthority()
			terrain.SetClassRestampOwnerWithCheckpointBinding(owner, authority)
			c := NewCheckpointContext(keys)
			if err := c.SetMovementBindings(terrain, authority); err != nil {
				t.Fatal(err)
			}
			registered := *c
			tc.edit(terrain, owner, authority)
			requireTerrainBindingRefusal(t, terrain, c)
			if err := c.SetMovementBindings(terrain, authority); err == nil || *c != registered {
				t.Fatal("replacement registration accepted or changed context")
			}
			if owner.calls != 0 {
				t.Fatal("capture called owner")
			}
		})
	}
}

func TestCheckpointMovementBindingRegistrationConflicts(t *testing.T) {
	keys := worldCheckpointKeys(t)
	authority := checkpoint.NewBindingAuthority()
	terrain, other := checkpointBindingTerrain(), checkpointBindingTerrain()
	terrain.SetClassRestampOwnerWithCheckpointBinding(&checkpointRestampOwner{}, authority)
	other.SetClassRestampOwnerWithCheckpointBinding(&checkpointRestampOwner{}, authority)
	c := NewCheckpointContext(keys)
	if err := c.SetMovementBindings(terrain, authority); err != nil {
		t.Fatal(err)
	}
	before := *c
	if err := c.SetMovementBindings(terrain, authority); err != nil || *c != before {
		t.Fatalf("identical registration = %v", err)
	}
	if allocs := testing.AllocsPerRun(10, func() {
		if err := c.SetMovementBindings(terrain, authority); err != nil {
			panic(err)
		}
	}); allocs != 0 {
		t.Fatalf("repeated registration allocated %v", allocs)
	}
	for _, tc := range []struct {
		terrain   *Terrain
		authority *checkpoint.BindingAuthority
	}{
		{other, authority}, {terrain, checkpoint.NewBindingAuthority()},
		{nil, authority}, {terrain, nil}, {checkpointBindingTerrain(), authority},
	} {
		if err := c.SetMovementBindings(tc.terrain, tc.authority); err == nil || *c != before {
			t.Fatal("conflict accepted or changed registration")
		}
	}
	precollected := NewCheckpointContext(keys)
	precollected.Terrain = other
	previous := *precollected
	if err := precollected.SetMovementBindings(terrain, authority); err == nil || *precollected != previous {
		t.Fatal("existing singleton overwritten")
	}
	var absent *CheckpointContext
	if err := absent.SetMovementBindings(terrain, authority); err == nil {
		t.Fatal("nil context accepted")
	}
	c.Terrain = nil
	requireTerrainBindingRefusal(t, terrain, c)
}

// World treats owners and adapters as opaque data. Movement's independent
// preflight must narrow them to its nonnil System and value gridOccupancy.
func TestCheckpointMovementBindingOpaqueInterfacesAndNilInstall(t *testing.T) {
	keys := worldCheckpointKeys(t)
	authority := checkpoint.NewBindingAuthority()
	var typedNil *checkpointRestampOwner
	for _, owner := range []FootprintRestampOwner{typedNil, checkpointOpaqueMovement{1, 2}} {
		terrain := checkpointBindingTerrain()
		terrain.SetMovers(checkpointOpaqueMovement{3, 4})
		terrain.SetClassRestampOwnerWithCheckpointBinding(owner, authority)
		if _, ok := terrain.CheckpointMovementOwner(authority); !ok {
			t.Fatal("world tried to classify opaque owner")
		}
		c := NewCheckpointContext(keys)
		if err := c.SetMovementBindings(terrain, authority); err != nil {
			t.Fatal(err)
		}
		if _, err := terrain.CollectCheckpointReferences(c); err != nil {
			t.Fatal(err)
		}
		if err := terrain.WriteCheckpoint(checkpoint.NewEncoder(&bytes.Buffer{}), c); err != nil {
			t.Fatal(err)
		}
	}
	terrain := checkpointBindingTerrain()
	terrain.SetClassRestampOwnerWithCheckpointBinding(&checkpointRestampOwner{}, authority)
	requireTerrainBindingRefusal(t, terrain, NewCheckpointContext(keys))
	terrain.SetClassRestampOwnerWithCheckpointBinding(nil, authority)
	if terrain.ClassRestamp() != nil {
		t.Fatal("nil owner left callback")
	}
	if _, ok := terrain.CheckpointMovementOwner(authority); ok {
		t.Fatal("nil owner retained proof")
	}
	worldCheckpointBytes(t, terrain, keys)
	terrain.SetClassRestampOwnerWithCheckpointBinding(&checkpointRestampOwner{}, nil)
	if terrain.ClassRestamp() == nil {
		t.Fatal("nil authority suppressed ordinary installation")
	}
	requireTerrainBindingRefusal(t, terrain, NewCheckpointContext(keys))
	var absent *Terrain
	absent.SetMovers(checkpointOpaqueMovement{5})
	absent.SetClassRestamp(func(int32, int32, int16, int16) { panic("nil terrain callback") })
	absent.SetClassRestampOwnerWithCheckpointBinding(checkpointOpaqueMovement{6}, authority)
	if _, ok := absent.CheckpointMovementOwner(authority); ok {
		t.Fatal("nil terrain returned proof")
	}
}
