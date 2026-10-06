package content

import (
	"reflect"
	"strings"
	"testing"
)

// Registration precedes record replacement, including the unreachable ID-less
// record; surviving definitions alone cannot reproduce admission [03 §8.3].
func TestWeaponSoundHistoryPreservesSectionAdmission(t *testing.T) {
	long := strings.Repeat("p", 260)
	fs := newFixtureFS(t,
		fixtureFile{path: "weapons/a.tdf", data: `[old]{ID=9;soundstart=first;soundhit=hit;soundwater=water;}
[unused]{soundstart=scratch;}
[replace]{ID=9;soundstart=last;soundhit=;}`},
		fixtureFile{path: "weapons/z.tdf", data: "[later]{ID=1;soundstart=" + long + ";soundwater=last;}"},
	)
	got, err := compileWeapons(fs, RetailLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"first", "hit", "water", "scratch", "last", long[:255], "last"}
	if !reflect.DeepEqual(got.soundPaths, want) {
		t.Fatalf("sound admission = %q, want %q", got.soundPaths, want)
	}
	if len(got.weapons) != 2 || got.weapons["replace"].SoundStart != "last" {
		t.Fatalf("surviving records = %+v", got.weapons)
	}
	public, duplicates, err := CompileWeaponsWithDuplicates(fs, RetailLimits())
	if err != nil || !reflect.DeepEqual(public, got.weapons) || !reflect.DeepEqual(duplicates, got.duplicates) {
		t.Fatalf("public weapon result changed: err=%v", err)
	}
	cat := &Catalog{WeaponSoundPaths: got.soundPaths}
	clone := cat.Clone()
	if !reflect.DeepEqual(clone.WeaponSoundPaths, want) {
		t.Fatalf("cloned history = %q", clone.WeaponSoundPaths)
	}
	clone.WeaponSoundPaths[0] = "changed"
	if cat.WeaponSoundPaths[0] != "first" {
		t.Fatal("clone shares mutable history slice")
	}
}
