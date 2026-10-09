package session

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/community"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// Online unit restrictions, battle-configuration field 12
// (docs/DESIGN_MULTIPLAYER.md §8.6, §16.6; docs/DESIGN_MODS_MUTATORS.md §15.3
// "Field 12", §15.5). The install is authored here, after the portable
// checkpoint fixture, with the records field 12 has to tell apart: two side
// commanders, an ordinary unit to cap and one to remove, a name carried by two
// records, and a norestrict unit. These are Nanolathe protocol values, not
// retail data.

const restrictionMatchMap = "portable"

// restrictionMatchUnits are the authored records: unit name, file, the extra
// FBI keys. Sorted by name the catalog holds portarm 1, portcore 2, portdup 3
// and 4, portlock 5, portscout 6 and porttank 7.
var restrictionMatchUnits = []struct{ name, file, side, extra string }{
	{"portarm", "portarm", "ARM", "Category=COMMANDER MOBILE;Commander=1;"},
	{"portcore", "portcore", "CORE", "Category=COMMANDER MOBILE;Commander=1;"},
	{"porttank", "porttank", "ARM", "Category=MOBILE;"},
	{"portscout", "portscout", "CORE", "Category=MOBILE;"},
	{"portdup", "portdup", "ARM", "Category=MOBILE;"},
	{"portdup", "portdupb", "CORE", "Category=MOBILE;"},
	{"portlock", "portlock", "ARM", "Category=MOBILE;NoRestrict=1;"},
}

// restrictionMatchInstall mounts the authored install and compiles its
// unrestricted catalog, the table field 12's definition IDs index.
func restrictionMatchInstall(t *testing.T) (vfs.FSOps, *content.Catalog) {
	t.Helper()
	model, err := formats.EncodeThreeDO(&formats.ThreeDO{Root: 0, Objects: []formats.ThreeDOObject{{
		Version: 1, Name: "base", Selection: -1, Parent: -1, FirstChild: -1, NextSibling: -1,
		Vertices: []formats.ThreeDOVertex{{}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	frames := make([]formats.GAFWriteFrame, 10)
	for i := range frames {
		frames[i] = formats.GAFWriteFrame{Width: 3, Height: 3, XOffset: 1, YOffset: 1,
			Pixels: []byte{7, 7, 7, 7, 7, 7, 7, 7, 7}}
	}
	masks, err := formats.EncodeGAF([]formats.GAFWriteEntry{{Name: "vismask", Frames: frames}})
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"maps/portable.tnt": string(checkpointPortableTNT()),
		"maps/portable.ota": `[GlobalHeader]{MinWindSpeed=100;MaxWindSpeed=200;Gravity=112;
[Schema 0]{Type=Network 1;SurfaceMetal=1;
[specials]{[special0]{specialwhat=StartPos1;XPos=128;ZPos=128;}
[special1]{specialwhat=StartPos2;XPos=384;ZPos=320;}}}}`,
		"gamedata/moveinfo.tdf":  `[CLASS0]{Name=portable;FootprintX=1;FootprintZ=1;MaxWaterDepth=10;MaxSlope=10;}`,
		"gamedata/los.tdf":       `[TABLEINFO]{numtables=1;}[TABLE1]{numlines=1;line1=1,0,1;}`,
		"gamedata/sidedata.tdf":  checkpointPortableSides(),
		"anims/vismasks.gaf":     string(masks),
		"objects3d/portable.3do": string(model),
		"ai/default.txt":         "plan any\n",
	}
	for _, dir := range []string{"weapons", "features", "download", "guis", "unitpics"} {
		files[dir+"/notes.txt"] = "Authored restriction fixture; no records in this family.\n"
	}
	for _, u := range restrictionMatchUnits {
		files["units/"+u.file+".fbi"] = fmt.Sprintf(`[UNITINFO]{UnitName=%s;Name=Portable Unit;Side=%s;
ObjectName=portable;%sBMcode=1;CanMove=1;CanStop=1;
MovementClass=portable;MaxVelocity=2;Acceleration=0.25;BrakeRate=0.5;TurnRate=1024;
MaxDamage=1000;SightDistance=160;FootprintX=1;FootprintZ=1;
BuildCostMetal=1;BuildCostEnergy=1;BuildTime=1;}`, u.name, u.side, u.extra)
		files["scripts/"+u.name+".cob"] = string(checkpointPortableCOB())
	}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	archiveFiles := make([]vfs.ArchiveFile, 0, len(paths))
	for _, path := range paths {
		archiveFiles = append(archiveFiles, vfs.ArchiveFile{Path: path, Data: []byte(files[path])})
	}
	var archive bytes.Buffer
	if err := vfs.WriteArchive(&archive, archiveFiles, vfs.ArchiveWriteOptions{}); err != nil {
		t.Fatal(err)
	}
	fs := vfs.New()
	if _, err := fs.MountArchiveReader("restrict.hpi", bytes.NewReader(archive.Bytes()), int64(archive.Len()), 10, vfs.ArchiveOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fs.Close() })
	cat, err := content.Compile(fs)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, u := range cat.UnitRecords() {
		names = append(names, content.CanonicalKey(u.UnitName))
	}
	if want := []string{"portarm", "portcore", "portdup", "portdup", "portlock", "portscout", "porttank"}; !slices.Equal(names, want) {
		t.Fatalf("authored catalog records %q, want %q", names, want)
	}
	return fs, cat
}

// restrictionMatchSetup is the authored scene's Modern setup and options.
func restrictionMatchSetup() (SkirmishConfig, SkirmishEntryOptions) {
	cfg := DirectSkirmishConfig(restrictionMatchMap)
	cfg.Gameplay, cfg.UnitLimit = gameplay.Modern, 20
	cfg.RNGSimSeed, cfg.RNGCrtSeed = 7, 11
	cfg.Location = 1
	zero := 0
	return cfg, SkirmishEntryOptions{CommunitySources: CommunitySources{Player: community.Overrides{UnitLimit: &zero}}}
}

// restrictionMatchConfig resolves the scene with the given field-12 records:
// the local adapter's one human and one computer, or, with twoHumans, the
// play test's two hostile humans (§16.4).
func restrictionMatchConfig(t *testing.T, records []MatchUnitRestriction, twoHumans bool) EffectiveMatchConfig {
	t.Helper()
	cfg, options := restrictionMatchSetup()
	room := matchTestRoom()
	room.UnitRestrictions = records
	r, err := NewMatchConfigRequest(cfg, options, room)
	if err != nil {
		t.Fatal(err)
	}
	if twoHumans {
		second := &r.Seats[1]
		second.Role, second.HostSeat = MatchRoleHuman, MatchHostNone
		second.ComputerKind, second.Difficulty, second.AIParams = 0, 0, nil
		second.Participant = matchTestID(2)
	}
	return resolveMatch(t, r)
}

func restrictionMatchSet(t *testing.T, values map[string]int) content.Restrictions {
	t.Helper()
	r, err := content.ParseRestrictions(values)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// The forward mapping writes one record per retained record an entry names,
// a duplicated name included, in ascending definition-ID order, and the
// reverse gives back the same set; the zero set is no records at all.
func TestMatchUnitRestrictionsMapsEveryRecordOfAName(t *testing.T) {
	_, cat := restrictionMatchInstall(t)
	r := restrictionMatchSet(t, map[string]int{"portdup": 4, "portscout": 0, "porttank": 2, "portarm": 1})
	records, err := MatchUnitRestrictions(cat, r)
	if err != nil {
		t.Fatal(err)
	}
	want := []MatchUnitRestriction{
		{DefinitionID: 1, Unit: "portarm", Limit: 1},
		{DefinitionID: 3, Unit: "portdup", Limit: 4},
		{DefinitionID: 4, Unit: "portdup", Limit: 4},
		{DefinitionID: 6, Unit: "portscout", Limit: 0},
		{DefinitionID: 7, Unit: "porttank", Limit: 2},
	}
	if !slices.Equal(records, want) {
		t.Fatalf("field 12 = %+v, want %+v", records, want)
	}
	back, err := RestrictionsFromMatch(cat, records)
	if err != nil || !back.Equal(r) {
		t.Fatalf("reverse mapping = %q (%v), want %q", back.String(), err, r.String())
	}
	if none, err := MatchUnitRestrictions(cat, content.Restrictions{}); none != nil || err != nil {
		t.Fatalf("the zero set mapped to %+v (%v), want no records", none, err)
	}
	if zero, err := RestrictionsFromMatch(cat, nil); !zero.IsZero() || err != nil {
		t.Fatalf("no records mapped to %q (%v), want the zero set", zero.String(), err)
	}
}

// Both directions refuse what field 12 rejects (§8.6, §15.3): a norestrict
// name and a commander's removal in the content's own words, and, in the
// reverse, a duplicate ID, a key whose records disagree on the limit, an ID
// naming another unit or none, a non-canonical key and a partial name group.
// A commander's cap is accepted.
func TestRestrictionsFromMatchRefusesWhatFieldTwelveRejects(t *testing.T) {
	_, cat := restrictionMatchInstall(t)
	issue := func(t *testing.T, err error, reason content.RestrictionReason) {
		t.Helper()
		var refusal *content.RestrictionsError
		if !errors.As(err, &refusal) || len(refusal.Issues) != 1 || refusal.Issues[0].Reason != reason {
			t.Fatalf("refusal %v, want the content issue %v", err, reason)
		}
	}
	_, err := MatchUnitRestrictions(cat, restrictionMatchSet(t, map[string]int{"portlock": 3}))
	issue(t, err, content.RestrictionNoRestrict)
	_, err = MatchUnitRestrictions(cat, restrictionMatchSet(t, map[string]int{"portarm": 0}))
	issue(t, err, content.RestrictionRemovesCommander)
	_, err = MatchUnitRestrictions(cat, restrictionMatchSet(t, map[string]int{"portnone": 0}))
	issue(t, err, content.RestrictionUnknownUnit)
	_, err = RestrictionsFromMatch(cat, []MatchUnitRestriction{{DefinitionID: 5, Unit: "portlock", Limit: 3}})
	issue(t, err, content.RestrictionNoRestrict)
	_, err = RestrictionsFromMatch(cat, []MatchUnitRestriction{{DefinitionID: 1, Unit: "portarm", Limit: 0}})
	issue(t, err, content.RestrictionRemovesCommander)
	if got, err := RestrictionsFromMatch(cat, []MatchUnitRestriction{{DefinitionID: 1, Unit: "portarm", Limit: 1}}); err != nil || got.String() != "portarm=1" {
		t.Fatalf("a commander cap = %q (%v), want accepted", got.String(), err)
	}

	for _, c := range []struct {
		name    string
		records []MatchUnitRestriction
		want    string
	}{
		{"a duplicate definition ID", []MatchUnitRestriction{{3, "portdup", 1}, {3, "portdup", 1}}, "strictly ascending"},
		{"one name, two limits", []MatchUnitRestriction{{3, "portdup", 1}, {4, "portdup", 2}}, `every record of unit "portdup"`},
		{"an ID naming another unit", []MatchUnitRestriction{{6, "porttank", 2}}, `definition 6, "portscout"`},
		{"an ID beyond the catalog", []MatchUnitRestriction{{8, "porttank", 2}}, "unrestricted catalog's 7 records"},
		{"a key that is not canonical", []MatchUnitRestriction{{7, "PORTTANK", 2}}, "canonical content key"},
		{"a partial name group", []MatchUnitRestriction{{3, "portdup", 1}}, `unit key "portdup"; definition 4 has none`},
	} {
		if got, err := RestrictionsFromMatch(cat, c.records); err == nil || !strings.Contains(err.Error(), c.want) || !got.IsZero() {
			t.Errorf("%s: %q, %v; want a refusal naming %q", c.name, got.String(), err, c.want)
		}
	}
}

// The two-human play test admits the host's restrictions (§16.6): each seat
// decodes the host's configuration and freezes its own content from it — one
// from a supplied catalog, the other compiling its own — and both run on the
// same restricted clone, the one single-player battle entry builds for the
// same set: the removed unit absent, the capped records carrying their limit,
// both records of the duplicated name capped, and the set recorded.
func TestOnlineRestrictionsReachBothSeatsCatalogs(t *testing.T) {
	fs, cat := restrictionMatchInstall(t)
	r := restrictionMatchSet(t, map[string]int{"portdup": 3, "portscout": 0, "porttank": 2})
	records, err := MatchUnitRestrictions(cat, r)
	if err != nil {
		t.Fatal(err)
	}
	host := restrictionMatchConfig(t, records, true)
	payload, err := EncodeMatchConfig(host)
	if err != nil {
		t.Fatal(err)
	}
	cfg, options := restrictionMatchSetup()
	options.Restrictions = r
	local, err := prepareSkirmishEntry(fs, cat, cfg, options)
	if err != nil {
		t.Fatalf("single-player entry with the same set: %v", err)
	}
	var copies [2]*Session
	var digests [2][32]byte
	for seat := range copies {
		config, err := DecodeMatchConfig(payload)
		if err != nil {
			t.Fatal(err)
		}
		supplied := cat
		if seat == 1 {
			supplied = nil
		}
		inputs, err := FreezeMatchInputs(fs, supplied, config, nil)
		if err != nil {
			t.Fatalf("seat %d: freeze: %v", seat, err)
		}
		if !inputs.Restrictions().Equal(r) {
			t.Fatalf("seat %d: frozen restrictions %q, want %q", seat, inputs.Restrictions().String(), r.String())
		}
		if err := ValidateMatchInputs(config, inputs); err != nil {
			t.Fatalf("seat %d: %v", seat, err)
		}
		digests[seat] = inputs.Digest()
		if digests[seat] != local.inputs.Digest() || inputs.Catalog().Hash != local.inputs.Catalog().Hash {
			t.Fatalf("seat %d: the configuration froze other content than single-player entry with the same set", seat)
		}
		s, err := NewPlaytestSkirmish(inputs, config, uint8(seat), nil)
		if err != nil {
			t.Fatalf("seat %d: %v", seat, err)
		}
		defer s.closeAIControllers()
		copies[seat] = s
	}
	if cat.UnitRecords()[5].UnitName != "portscout" || cat.Units["porttank"].Limit != -1 {
		t.Fatal("the shared unrestricted catalog was written")
	}
	for seat, s := range copies {
		if !s.Restrictions.Equal(r) {
			t.Fatalf("seat %d recorded %q", seat, s.Restrictions.String())
		}
		if _, ok := s.Catalog.Unit("portscout"); ok {
			t.Fatalf("seat %d: the removed unit is in the battle catalog", seat)
		}
		if tank, ok := s.Catalog.Unit("porttank"); !ok || tank.Limit != 2 || !tank.LimitEnabled {
			t.Fatalf("seat %d: the capped unit carries limit %+v", seat, tank)
		}
		dups := 0
		for _, u := range s.Catalog.UnitRecords() {
			if content.CanonicalKey(u.UnitName) == "portdup" {
				dups++
				if u.Limit != 3 || !u.LimitEnabled {
					t.Fatalf("seat %d: a duplicate-name record carries limit %d", seat, u.Limit)
				}
			}
		}
		if dups != 2 || len(s.Catalog.UnitRecords()) != 6 {
			t.Fatalf("seat %d: %d duplicate records of %d, want 2 of 6", seat, dups, len(s.Catalog.UnitRecords()))
		}
		if err := s.PrepareGrantedBattle(); err != nil {
			t.Fatalf("seat %d: %v", seat, err)
		}
	}
	if copies[0].Catalog.Hash != copies[1].Catalog.Hash || digests[0] != digests[1] {
		t.Fatal("the seats' restricted catalogs differ")
	}
	for tick := uint32(1); tick <= 30; tick++ {
		for _, s := range copies {
			if err := s.StepGranted(tick); err != nil {
				t.Fatal(err)
			}
		}
	}
	if copies[0].UnitStateChecksum() != copies[1].UnitStateChecksum() {
		t.Fatal("the seats diverged on the restricted catalog")
	}
}

// Configurations that differ only in their restrictions have different
// configuration digests and so different join identities; their restricted
// content differs too, and the join comparison reports both (§8.2, §16.6).
// The single-seat admitted constructor composes the same restricted content.
func TestOnlineRestrictionsMoveTheJoinIdentity(t *testing.T) {
	fs, cat := restrictionMatchInstall(t)
	plain := restrictionMatchConfig(t, nil, true)
	records, err := MatchUnitRestrictions(cat, restrictionMatchSet(t, map[string]int{"porttank": 2}))
	if err != nil {
		t.Fatal(err)
	}
	capped := restrictionMatchConfig(t, records, true)
	if plain.Digest() == capped.Digest() {
		t.Fatal("restrictions do not move the configuration digest")
	}
	join := func(c EffectiveMatchConfig) MatchJoin {
		t.Helper()
		inputs, err := FreezeMatchInputs(fs, cat, c, nil)
		if err != nil {
			t.Fatal(err)
		}
		return MatchJoin{Build: MatchBuild{Running: joinRelease()}, Inputs: inputs, Config: c}
	}
	a, b := join(plain), join(capped)
	// An empty field 12 freezes exactly what unrestricted single-player entry
	// freezes, identity included.
	cfg, options := restrictionMatchSetup()
	local, err := prepareSkirmishEntry(fs, cat, cfg, options)
	if err != nil {
		t.Fatal(err)
	}
	if !a.Inputs.Restrictions().IsZero() || a.Inputs.Digest() != local.inputs.Digest() || a.Inputs.Catalog().Hash != local.inputs.Catalog().Hash {
		t.Fatal("an empty field 12 changed the frozen content")
	}
	ida, idb := joinIdentity(t, a), joinIdentity(t, b)
	if ida.Configuration == idb.Configuration || ida.Content == idb.Content {
		t.Fatal("restrictions do not move the join identity")
	}
	err = CompareMatchIdentity(a, idb)
	if !errors.Is(err, ErrMatchConfigurationRejected) || !errors.Is(err, ErrMatchContentMismatch) || errors.Is(err, ErrMatchMapMismatch) {
		t.Fatalf("join comparison = %v, want configuration and content mismatches only", err)
	}

	single := restrictionMatchConfig(t, records, false)
	inputs, err := FreezeMatchInputs(fs, cat, single, nil)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewAdmittedSkirmish(inputs, single, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.closeAIControllers()
	if tank, _ := s.Catalog.Unit("porttank"); tank == nil || tank.Limit != 2 || s.Restrictions.String() != "porttank=2" {
		t.Fatalf("admitted single-seat battle restricted %q", s.Restrictions.String())
	}
}

// Admission compares field 12 with the set the frozen inputs record, as it
// compares the mutators, and checks every surviving record's definition ID
// against the frozen table; freezing refuses records this install's
// unrestricted catalog does not hold. Each refusal is a content mismatch.
func TestOnlineRestrictionsAdmissionComparesTheFrozenSet(t *testing.T) {
	fs, cat := restrictionMatchInstall(t)
	r := restrictionMatchSet(t, map[string]int{"portarm": 1, "portscout": 0, "porttank": 2})
	records, err := MatchUnitRestrictions(cat, r)
	if err != nil {
		t.Fatal(err)
	}
	config := restrictionMatchConfig(t, records, true)
	inputs, err := FreezeMatchInputs(fs, cat, config, nil)
	if err != nil {
		t.Fatal(err)
	}
	unrestricted, err := FreezeMatchInputs(fs, cat, restrictionMatchConfig(t, nil, true), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateMatchInputs(restrictionMatchConfig(t, nil, true), inputs); !errors.Is(err, ErrMatchContentMismatch) {
		t.Fatalf("an empty field 12 admitted restricted content: %v", err)
	}
	if err := ValidateMatchInputs(config, unrestricted); !errors.Is(err, ErrMatchContentMismatch) {
		t.Fatalf("field 12 admitted unrestricted content: %v", err)
	}
	otherCap := slices.Clone(records)
	otherCap[2].Limit = 3
	if err := ValidateMatchInputs(restrictionMatchConfig(t, otherCap, true), inputs); !errors.Is(err, ErrMatchContentMismatch) {
		t.Fatalf("another cap was admitted: %v", err)
	}
	// The same set under another commander's ID: the frozen table's first
	// survivor carries a capped key no record names.
	moved := slices.Clone(records)
	moved[0].DefinitionID = 2
	err = ValidateMatchInputs(restrictionMatchConfig(t, moved, true), inputs)
	if !errors.Is(err, ErrMatchContentMismatch) || !strings.Contains(err.Error(), `unit key "portarm"; definition 1 has none`) {
		t.Fatalf("a moved definition ID was admitted: %v", err)
	}
	if _, err := FreezeMatchInputs(fs, cat, restrictionMatchConfig(t, moved, true), nil); !errors.Is(err, ErrMatchContentMismatch) {
		t.Fatalf("freezing a moved definition ID: %v", err)
	}
	locked := restrictionMatchConfig(t, []MatchUnitRestriction{{DefinitionID: 5, Unit: "portlock", Limit: 1}}, true)
	_, err = FreezeMatchInputs(fs, nil, locked, nil)
	var refusal *content.RestrictionsError
	if !errors.Is(err, ErrMatchContentMismatch) || !errors.As(err, &refusal) || refusal.Issues[0].Reason != content.RestrictionNoRestrict {
		t.Fatalf("freezing a norestrict record: %v", err)
	}
}

// The local adapter takes field 12 from the room; a set in the entry options
// must be the one the room's records describe, so it is never dropped.
func TestMatchConfigRequestAccountsForRestrictionOptions(t *testing.T) {
	_, cat := restrictionMatchInstall(t)
	r := restrictionMatchSet(t, map[string]int{"porttank": 2})
	records, err := MatchUnitRestrictions(cat, r)
	if err != nil {
		t.Fatal(err)
	}
	cfg, options := restrictionMatchSetup()
	options.Restrictions = r
	room := matchTestRoom()
	if _, err := NewMatchConfigRequest(cfg, options, room); err == nil || !strings.Contains(err.Error(), "options.restrictions") {
		t.Fatalf("a set without records was accepted: %v", err)
	}
	room.UnitRestrictions = records
	got, err := NewMatchConfigRequest(cfg, options, room)
	if err != nil || !slices.Equal(got.UnitRestrictions, records) {
		t.Fatalf("records %+v (%v), want %+v", got.UnitRestrictions, err, records)
	}
}
