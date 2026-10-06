package content

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/testsupport"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

func rosterTestUnit(key string) *UnitDef {
	return &UnitDef{DefinitionHeader: DefinitionHeader{CanonicalKey: key}, UnitName: key, CanMove: true, BMCode: 1, BuildCostMetal: 100, Weapon1Def: &WeaponDef{ID: 1}}
}

func TestSurvivalRosterParseAndRejectMalformed(t *testing.T) {
	fs := newFixtureFS(t)
	if r, err := CompileSurvivalRoster(fs); r != nil || err != nil {
		t.Fatalf("absent = %v, %v", r, err)
	}
	parse := func(text string) (*SurvivalRoster, error) {
		return CompileSurvivalRoster(newFixtureFS(t, fixtureFile{SurvivalRosterPath, text}))
	}
	r, err := parse(`[SURVIVAL] { AttackerSkin=skins/INFECTED.gaf; [UNITS] { Zed=3; alien=1; } }`)
	if err != nil {
		t.Fatal(err)
	}
	if r.AttackerSkin != "skins/infected.gaf" || !reflect.DeepEqual(r.Units, []SurvivalRosterEntry{{"alien", 1}, {"zed", 3}}) {
		t.Fatalf("canonical roster: %+v", r)
	}
	for _, body := range []string{
		`[SURVIVAL] { [UNITS] { alien=1; ALIEN=2; } }`,
		`[SURVIVAL] { [UNITS] { alien=0; } }`,
		`[SURVIVAL] { [UNITS] { alien=17; } }`,
		`[SURVIVAL] { [UNITS] { alien=1.5; } }`,
		`[SURVIVAL] { [UNITS] { alien=+1; } }`,
		`[SURVIVAL] { [UNITS] { alien=1junk; } }`,
		`[SURVIVAL] { [UNITS] { ../alien=1; } }`,
		`[SURVIVAL] { [UNITS] { [alien] {} } }`,
		`[SURVIVAL] { [UNITS] {} [UNITS] {} }`,
		`[SURVIVAL] { [UNITS] {} Extra=1; }`,
		`[SURVIVAL] { AttackerSkin=../infected.gaf; [UNITS] {alien=1;} }`,
		`[SURVIVAL] { AttackerSkin=/infected.gaf; [UNITS] {alien=1;} }`,
		`[SURVIVAL] { AttackerSkin=skins/infected.png; [UNITS] {alien=1;} }`,
		`[SURVIVAL] { [UNITS] {alien=1;} } [OTHER] {}`,
	} {
		if _, err := parse(body); err == nil {
			t.Errorf("accepted %s", body)
		}
	}
}

func TestSurvivalRosterEligibilityAndStartingPool(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Catalog)
	}{
		{"missing", func(c *Catalog) { delete(c.Units, "alien") }},
		{"builder", func(c *Catalog) { c.Units["alien"].Builder = true }},
		{"commander", func(c *Catalog) { c.Units["alien"].Commander = true }},
		{"building", func(c *Catalog) { c.Units["alien"].BMCode = 0 }},
		{"stationary", func(c *Catalog) { c.Units["alien"].CanMove = false }},
		{"free", func(c *Catalog) { c.Units["alien"].BuildCostMetal = 0 }},
		{"unarmed", func(c *Catalog) { c.Units["alien"].Weapon1Def = nil }},
		{"anti-air", func(c *Catalog) { c.Units["alien"].Weapon1Def.ToAirWeapon = true }},
		{"interceptor", func(c *Catalog) { c.Units["alien"].Weapon1Def.Interceptor = true }},
		{"late-only", func(c *Catalog) { c.SurvivalRoster.Units[0].Tier = 2 }},
		{"air-only", func(c *Catalog) { c.Units["alien"].CanFly = true }},
		{"naval-only", func(c *Catalog) { c.Units["alien"].Floater = true }},
		{"infector-only", func(c *Catalog) { c.Units["alien"].NanolatheInfector = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &Catalog{Units: map[string]*UnitDef{"alien": rosterTestUnit("alien")}, SurvivalRoster: &SurvivalRoster{Units: []SurvivalRosterEntry{{"alien", 1}}}}
			if err := c.ValidateSurvivalRoster(); err != nil {
				t.Fatal(err)
			}
			tc.mutate(c)
			if err := c.ValidateSurvivalRoster(); err == nil {
				t.Fatal("invalid roster accepted")
			}
		})
	}
}

func TestSurvivalRosterCloneAndContentIdentity(t *testing.T) {
	c := &Catalog{}
	baseHash, baseEntries := catalogHash(c), catalogEntries(c)
	clone := c.Clone()
	if clone.SurvivalRoster != nil || catalogHash(clone) != baseHash || !reflect.DeepEqual(catalogEntries(clone), baseEntries) {
		t.Fatal("absent roster changed clone identity")
	}
	c.SurvivalRoster = &SurvivalRoster{Units: []SurvivalRosterEntry{{"alien", 1}}, AttackerSkin: "skins/a.gaf"}
	withHash, withEntries := catalogHash(c), catalogEntries(c)
	if withHash == baseHash || reflect.DeepEqual(withEntries, baseEntries) {
		t.Fatal("roster omitted from content identity")
	}
	clone = c.Clone()
	clone.SurvivalRoster.Units[0].Tier = 2
	if c.SurvivalRoster.Units[0].Tier != 1 || catalogHash(clone) == withHash || reflect.DeepEqual(catalogEntries(clone), withEntries) {
		t.Fatal("clone aliased roster or tier omitted from identity")
	}
	clone = c.Clone()
	clone.SurvivalRoster.AttackerSkin = "skins/b.gaf"
	if catalogHash(clone) == withHash || !reflect.DeepEqual(catalogEntries(clone), withEntries) {
		t.Fatal("skin must affect catalog identity only, not simulation identity")
	}
}

func TestSurvivalRosterCompilesAgainstLinkedCatalog(t *testing.T) {
	fs := vfs.New()
	defer fs.Close()
	if err := fs.MountGameDirectory(testsupport.RetailRoot(t)); err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	if err := vfs.WriteArchive(&archive, []vfs.ArchiveFile{{Path: SurvivalRosterPath, Data: []byte(`[SURVIVAL]{[UNITS]{armpw=1; corgol=2;}}`)}}, vfs.ArchiveWriteOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.MountArchiveReader("authored-roster.ufo", bytes.NewReader(archive.Bytes()), int64(archive.Len()), 1000, vfs.ArchiveOptions{}); err != nil {
		t.Fatal(err)
	}
	c, err := Compile(fs)
	if err != nil {
		t.Fatal(err)
	}
	if c.SurvivalRoster == nil || len(c.SurvivalRoster.Units) != 2 {
		t.Fatal("Compile lost optional roster")
	}
	c.SurvivalRoster.Units[0].Unit = "missing"
	if err := c.ValidateSurvivalRoster(); err == nil || !strings.Contains(err.Error(), SurvivalRosterPath) {
		t.Fatalf("validation: %v", err)
	}
}

func TestSurvivalRosterBuildTreeIdentityAndParsing(t *testing.T) {
	parse := func(value string) (*SurvivalRoster, error) {
		return CompileSurvivalRoster(newFixtureFS(t, fixtureFile{SurvivalRosterPath, `[SURVIVAL]{` + value + `[UNITS]{alien=1;}}`}))
	}
	for _, raw := range []string{"IncludeBuildTree=2;", "IncludeBuildTree=true;", "IncludeBuildTree=01;", "IncludeBuildTree=1; includebuildtree=0;"} {
		if _, err := parse(raw); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
	r, err := parse("IncludeBuildTree=1;")
	if err != nil || !r.IncludeBuildTree {
		t.Fatalf("additive roster %v: %v", r, err)
	}
	c := &Catalog{SurvivalRoster: r}
	clone := c.Clone()
	if !clone.SurvivalRoster.IncludeBuildTree {
		t.Fatal("clone lost pool selection")
	}
	h, entries := catalogHash(c), catalogEntries(c)
	clone.SurvivalRoster.IncludeBuildTree = false
	if !c.SurvivalRoster.IncludeBuildTree || catalogHash(clone) == h || reflect.DeepEqual(catalogEntries(clone), entries) {
		t.Fatal("pool selection missing from identity or clone aliases source")
	}
	absent, _ := parse("")
	zero, _ := parse("IncludeBuildTree=0;")
	if !reflect.DeepEqual(absent, zero) {
		t.Fatal("default exclusive semantics changed")
	}
}
