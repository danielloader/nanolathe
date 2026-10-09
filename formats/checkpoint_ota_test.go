package formats

import (
	"slices"
	"strings"
	"testing"
)

func checkpointOTAFixture(t *testing.T) *OTA {
	t.Helper()
	o, err := LoadOTA([]byte(`[GlobalHeader]{
		missionname=Authored; missiondescription=Description; memory=16mb; numplayers=2; size=small;
		[Schema 0]{type=Network 1;aiprofile=profile;humanmetal=11;humanenergy=12;computermetal=13;computerenergy=14;}
		[Schema 1]{type=Network 2;}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func TestCheckpointOTAInputsRetainedMutations(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*OTA)
	}{
		{"MissionName", func(o *OTA) { o.MissionName += "x" }},
		{"MissionDescription", func(o *OTA) { o.MissionDescription += "x" }},
		{"Memory", func(o *OTA) { o.Memory += "x" }},
		{"NumPlayers", func(o *OTA) { o.NumPlayers += "x" }},
		{"Size", func(o *OTA) { o.Size += "x" }},
		{"Schema.Name", func(o *OTA) { o.Schemas[0].Name += "x" }},
		{"Schema.Type", func(o *OTA) { o.Schemas[0].Type += "x" }},
		{"Schema.AIProfile", func(o *OTA) { o.Schemas[0].AIProfile += "x" }},
		{"Schema.HumanMetal", func(o *OTA) { o.Schemas[0].HumanMetal += "x" }},
		{"Schema.HumanEnergy", func(o *OTA) { o.Schemas[0].HumanEnergy += "x" }},
		{"Schema.ComputerMetal", func(o *OTA) { o.Schemas[0].ComputerMetal += "x" }},
		{"Schema.ComputerEnergy", func(o *OTA) { o.Schemas[0].ComputerEnergy += "x" }},
		{"Schema.Section", func(o *OTA) { copy := *o.Schemas[0].Section; o.Schemas[0].Section = &copy }},
		{"Schemas.order", func(o *OTA) { o.Schemas[0], o.Schemas[1] = o.Schemas[1], o.Schemas[0] }},
		{"Schemas.length", func(o *OTA) { o.Schemas = o.Schemas[:1] }},
		{"Document.copy", func(o *OTA) { copy := *o.Document; o.Document = &copy }},
		{"Document.nil", func(o *OTA) { o.Document = nil }},
		{"Root.copy", func(o *OTA) { copy := *o.Document.Root; o.Document.Root = &copy }},
		{"Root.nil", func(o *OTA) { o.Document.Root = nil }},
		{"Global.copy", func(o *OTA) { copy := *o.Global; o.Global = &copy }},
		{"Global.nil", func(o *OTA) { o.Global = nil }},
		{"Section.Name", func(o *OTA) { o.Schemas[1].Section.Name += "x" }},
		{"Section.OriginalName", func(o *OTA) { o.Global.OriginalName += "x" }},
		{"Item.Kind", func(o *OTA) { o.Global.Items[0].Kind = NestedSection }},
		{"Item.Key", func(o *OTA) { o.Global.Items[0].Key += "x" }},
		{"Item.OriginalKey", func(o *OTA) { o.Global.Items[0].OriginalKey += "x" }},
		{"Item.Value", func(o *OTA) { o.Global.Items[0].Value += "x" }},
		{"Item.Section", func(o *OTA) { copy := *o.Global; o.Document.Root.Items[0].Section = &copy }},
		{"Items.order", func(o *OTA) { o.Global.Items[0], o.Global.Items[1] = o.Global.Items[1], o.Global.Items[0] }},
		{"Items.length", func(o *OTA) { o.Global.Items = append(o.Global.Items, Item{}) }},
		{"resolvedBuilt", func(o *OTA) { o.Global.resolvedBuilt = false }},
		{"resolved.word", func(o *OTA) { o.Global.resolved[0]++ }},
		{"resolved.length", func(o *OTA) { o.Global.resolved = o.Global.resolved[:1] }},
		{"insertedCycle", func(o *OTA) { o.Global.Items[0].Section = o.Document.Root }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, validateFirst := range []bool{false, true} {
				o := checkpointOTAFixture(t)
				s, err := SnapshotCheckpointOTAInputs(o)
				if err != nil {
					t.Fatal(err)
				}
				if validateFirst {
					if err := s.Validate(o); err != nil {
						t.Fatal(err)
					}
				}
				tc.mutate(o)
				if err := s.Validate(o); err == nil {
					t.Fatal("accepted changed OTA input")
				}
			}
		})
	}
}

func TestCheckpointOTAInputsNilIdentityAndShape(t *testing.T) {
	s, err := SnapshotCheckpointOTAInputs(nil)
	if err != nil {
		t.Fatal(err)
	}
	if s == nil {
		t.Fatal("nil OTA must have a snapshot")
	}
	if err := s.Validate(nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Validate(&OTA{}); err == nil {
		t.Fatal("accepted added OTA")
	}
	o := &OTA{}
	s, err = SnapshotCheckpointOTAInputs(o)
	if err != nil {
		t.Fatal(err)
	}
	copy := *o
	for _, check := range []func() error{
		func() error { return s.Validate(&copy) },
		func() error { return s.Validate(nil) },
		func() error { return (*CheckpointOTAInputs)(nil).Validate(nil) },
		func() error { return new(CheckpointOTAInputs).Validate(nil) },
	} {
		if err := check(); err == nil || !strings.HasPrefix(err.Error(), "nanolathe: OTA checkpoint input validation failed: logical path ") {
			t.Fatalf("missing owner-shaped refusal: %v", err)
		}
	}
	for _, tc := range []struct {
		name   string
		mutate func(*OTA)
	}{
		{"Document", func(o *OTA) { o.Document = &Document{} }},
		{"Root", func(o *OTA) { o.Document.Root = &Section{} }},
		{"Global", func(o *OTA) { o.Global = &Section{} }},
		{"SchemaSection", func(o *OTA) { o.Schemas[0].Section = &Section{} }},
		{"ItemSection", func(o *OTA) { o.Document.Root.Items[0].Section = &Section{} }},
		{"Schemas", func(o *OTA) { o.Schemas = []OTASchema{} }},
		{"Items", func(o *OTA) { o.Global.Items = []Item{} }},
		{"resolved", func(o *OTA) { o.Global.resolved = []int32{} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := &OTA{Global: &Section{}}
			switch tc.name {
			case "Root":
				o.Document = &Document{}
			case "SchemaSection":
				o.Schemas = []OTASchema{{}}
			case "ItemSection":
				o.Document = &Document{Root: &Section{Items: []Item{{Kind: NestedSection}}}}
			}
			s, err := SnapshotCheckpointOTAInputs(o)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.Validate(o); err != nil {
				t.Fatal(err)
			}
			tc.mutate(o)
			if err := s.Validate(o); err == nil {
				t.Fatal("accepted changed optional presence")
			}
			// The populated form is equally valid to snapshot, including
			// nonnil empty slices and otherwise empty nodes.
			s, err = SnapshotCheckpointOTAInputs(o)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.Validate(o); err != nil {
				t.Fatal(err)
			}
			switch tc.name {
			case "Document":
				o.Document = nil
			case "Root":
				o.Document.Root = nil
			case "Global":
				o.Global = nil
			case "SchemaSection":
				o.Schemas[0].Section = nil
			case "ItemSection":
				o.Document.Root.Items[0].Section = nil
			case "Schemas":
				o.Schemas = nil
			case "Items":
				o.Global.Items = nil
			case "resolved":
				o.Global.resolved = nil
			}
			if err := s.Validate(o); err == nil {
				t.Fatal("accepted cleared optional presence")
			}
		})
	}
}

func TestCheckpointOTAInputsDuplicateAndUnresolvedIndexes(t *testing.T) {
	// Independently authored duplicate sequence: exact spelling replaces;
	// variants enter ahead of the equal run [fmt tdf "Duplicate keys"].
	doc, err := ParseTDF([]byte(`x=1; x=2; X=3; x=4;`))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(doc.Root.resolved, []int32{3, 2, 1}) {
		t.Fatalf("resolved = %v", doc.Root.resolved)
	}
	o := &OTA{Document: doc}
	s, err := SnapshotCheckpointOTAInputs(o)
	if err != nil {
		t.Fatal(err)
	}
	// The replaced raw item remains authored input, despite not appearing in
	// the lookup index. Both representations are retained independently.
	doc.Root.Items[0].Value = "changed"
	if err := s.Validate(o); err == nil {
		t.Fatal("ignored shadowed duplicate")
	}
	doc.Root.Items[0].Value = "1"
	doc.Root.resolved[0], doc.Root.resolved[1] = doc.Root.resolved[1], doc.Root.resolved[0]
	if err := s.Validate(o); err == nil {
		t.Fatal("ignored lookup topology mutation")
	}
	doc.Root.resolved[0], doc.Root.resolved[1] = doc.Root.resolved[1], doc.Root.resolved[0]
	if err := s.Validate(o); err != nil {
		t.Fatal(err)
	}

	unresolved := &Section{Items: []Item{{Key: "x", OriginalKey: "x", Value: "1"}}}
	o = &OTA{Global: unresolved}
	s, err = SnapshotCheckpointOTAInputs(o)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Validate(o); err != nil {
		t.Fatal(err)
	}
	if unresolved.resolvedBuilt || unresolved.resolved != nil {
		t.Fatal("snapshot or validation resolved fixture")
	}
	if got := unresolved.IntValue("x", 0); got != 1 {
		t.Fatalf("fixture getter = %d", got)
	}
	if err := s.Validate(o); err == nil {
		t.Fatal("accepted lazy resolution after snapshot")
	}
}

func TestCheckpointOTAInputsSharedCyclesAndDisconnectedRoots(t *testing.T) {
	root, global, schema, child, opaque := &Section{Name: "root"}, &Section{Name: "global"}, &Section{Name: "schema"}, &Section{Name: "child"}, &Section{Name: "opaque"}
	root.Items = []Item{{Kind: NestedSection, Section: child}, {Kind: NestedSection, Section: child}}
	child.Items = []Item{{Kind: NestedSection, Section: root}}
	// An edge on an assignment is still stored topology. Follow it at
	// snapshot time without depending on a parser invariant for fixtures.
	global.Items = []Item{{Kind: Assignment, Section: opaque}}
	o := &OTA{Document: &Document{Root: root}, Global: global, Schemas: []OTASchema{{Section: schema}, {Section: schema}}}
	s, err := SnapshotCheckpointOTAInputs(o)
	if err != nil {
		t.Fatal(err)
	}
	// Roots are recorded first, then outgoing edges in physical item order.
	if len(s.sections) != 5 || s.sections[0].owner != root || s.sections[1].owner != global || s.sections[2].owner != schema || s.sections[3].owner != child || s.sections[4].owner != opaque {
		t.Fatal("shared or cyclic graph did not retain one ordered record per identity")
	}
	if err := s.Validate(o); err != nil {
		t.Fatal(err)
	}
	for _, node := range []*Section{root, global, schema, child, opaque} {
		node.Name += "x"
		if err := s.Validate(o); err == nil {
			t.Fatal("missed reachable section")
		}
		node.Name = strings.TrimSuffix(node.Name, "x")
	}
	// A same-value node copy and a new back edge both refuse before traversal.
	copy := *child
	root.Items[1].Section = &copy
	if err := s.Validate(o); err == nil {
		t.Fatal("accepted replacement shared node")
	}
	root.Items[1].Section = child
	schema.Items = []Item{{Kind: NestedSection, Section: global}}
	if err := s.Validate(o); err == nil {
		t.Fatal("accepted new cycle")
	}
	schema.Items = nil
	if err := s.Validate(o); err != nil {
		t.Fatal(err)
	}
}

func TestCheckpointOTAInputsExclusionsStorageAndPurity(t *testing.T) {
	o := checkpointOTAFixture(t)
	s, err := SnapshotCheckpointOTAInputs(o)
	if err != nil {
		t.Fatal(err)
	}
	// Equal detached storage is allowed. Source coordinates may change
	// independently; they cannot alter future authored-value reads.
	o.Schemas = slices.Clone(o.Schemas)
	for _, row := range s.sections {
		p := row.owner
		p.Line, p.Column = -7, 19
		p.Items = slices.Clone(p.Items)
		p.resolved = slices.Clone(p.resolved)
		for i := range p.Items {
			p.Items[i].Line, p.Items[i].Column = 29, -31
		}
	}
	if err := s.Validate(o); err != nil {
		t.Fatal(err)
	}
	if allocs := testing.AllocsPerRun(100, func() {
		if err := s.Validate(o); err != nil {
			panic(err)
		}
	}); allocs != 0 {
		t.Fatalf("validation allocations = %v", allocs)
	}
	for _, row := range s.sections {
		p := row.owner
		if p.Line != -7 || p.Column != 19 {
			t.Fatal("changed section source coordinates")
		}
		for _, item := range p.Items {
			if item.Line != 29 || item.Column != -31 {
				t.Fatal("changed item source coordinates")
			}
		}
	}
	// Snapshots must not acquire replacement backing storage during validate.
	o.Schemas[0].HumanMetal = "changed"
	if err := s.Validate(o); err == nil {
		t.Fatal("accepted replacement schema storage mutation")
	}
}
