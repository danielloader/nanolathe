package content

import (
	"strings"
	"testing"
)

// soundOrdinalFixture authors three categories in a deliberate file order:
// the ordinal domain is that order, not the sorted or canonical-key order the
// name map keeps [02 §5 "Cross-reference failure policy"][02 R-CAT-01 §5].
const soundOrdinalFixture = `[ZETA_FIRST]
{
	select1=ZetaSel;
}
[ALPHA_SECOND]
{
	select1=AlphaSel;
}
[MID_THIRD]
{
	select1=MidSel;
}
`

func ordinalTestCatalog(t *testing.T) *Catalog {
	t.Helper()
	fs := newFixtureFS(t, fixtureFile{path: "gamedata/sound.tdf", data: soundOrdinalFixture})
	cats, order, err := compileSoundCategoriesOrdered(fs)
	if err != nil {
		t.Fatalf("compileSoundCategoriesOrdered: %v", err)
	}
	if len(order) != 3 {
		t.Fatalf("category order length %d want 3", len(order))
	}
	// File order, not sorted order: the map alone could not tell them apart.
	if order[0].Name != "ZETA_FIRST" || order[1].Name != "ALPHA_SECOND" || order[2].Name != "MID_THIRD" {
		t.Fatalf("category order %s/%s/%s want ZETA_FIRST/ALPHA_SECOND/MID_THIRD [02 R-CAT-01 §5]",
			order[0].Name, order[1].Name, order[2].Name)
	}
	return &Catalog{Sounds: cats, SoundCategoryOrder: order}
}

// TestSoundCategoryOrdinalFallback locks the `soundcategory` failure policy:
// absent → index 0, a name miss → the C-runtime decimal conversion of the
// authored text used as an ordinal, so non-numeric text is 0 and again the
// first authored category [02 §5][02 R-CAT-01 §5]. There is no placeholder
// record, so none of these resolve to nothing.
func TestSoundCategoryOrdinalFallback(t *testing.T) {
	c := ordinalTestCatalog(t)
	cases := []struct {
		authored string
		want     string
	}{
		// Eleven stock definitions author exactly this shape of miss.
		{"NONE", "ZETA_FIRST"},
		{"CORE_KBOT", "ZETA_FIRST"},
		{"", "ZETA_FIRST"},
		{"1", "ALPHA_SECOND"},
		{"2", "MID_THIRD"},
		// The conversion is the ordinary CRT one: leading space, sign and a
		// digit prefix with trailing junk ignored.
		{"  1junk", "ALPHA_SECOND"},
		{"+2", "MID_THIRD"},
		{"65537", "ALPHA_SECOND"},
		{"-65535", "ALPHA_SECOND"},
		{strings.Repeat("0", 99) + "1", "ZETA_FIRST"},
		// A name match wins over any numeric reading of the same text.
		{"alpha_second", "ALPHA_SECOND"},
	}
	for _, tc := range cases {
		got := c.ResolveSoundCategory(tc.authored)
		if got == nil {
			t.Fatalf("soundcategory %q resolved to no category, want %s [02 §5]", tc.authored, tc.want)
		}
		if got.Name != tc.want {
			t.Fatalf("soundcategory %q resolved to %s, want %s [02 R-CAT-01 §5]", tc.authored, got.Name, tc.want)
		}
	}
	// Out of range is the documented open question: retail stores the ordinal
	// after narrowing and indexes past the loaded records. Nanolathe resolves nothing
	// rather than inventing a wrap or a clamp (TODO(question) at the resolver).
	if got := c.ResolveSoundCategory("9"); got != nil {
		t.Fatalf("out-of-range ordinal resolved to %s, want no category until the retail behavior is settled", got.Name)
	}
}

// Stored names and file order are the lookup contract [02 R-CAT-01 §5].
func TestSoundCategoryLookupUsesFirstRetainedName(t *testing.T) {
	prefix := strings.Repeat("q", 63)
	text := `[FIRST]{select=First;}[DUP]{select=Earlier;}[dup]{select=Later;}` +
		"[" + prefix + "extra]{select=Long;}"
	fs := newFixtureFS(t, fixtureFile{path: "gamedata/sound.tdf", data: text})
	cats, order, err := compileSoundCategoriesOrdered(fs)
	if err != nil {
		t.Fatal(err)
	}
	c := &Catalog{Sounds: cats, SoundCategoryOrder: order}
	for _, tc := range []struct{ name, alias string }{
		{"DuP", "Earlier"}, {prefix, "Long"}, {prefix + "extra", "First"},
	} {
		got := c.ResolveSoundCategory(tc.name)
		if got == nil || len(got.Slots[1].Variants) != 1 || got.Slots[1].Variants[0] != tc.alias {
			t.Fatalf("lookup %q = %v, want alias %s", tc.name, got, tc.alias)
		}
	}
	// A late matching record's stored ordinal also narrows. Sparse host entries
	// isolate that consumer arithmetic without authoring thousands of categories.
	c.SoundCategoryOrder = make([]*SoundCategory, 65538)
	c.SoundCategoryOrder[1] = order[0]
	c.SoundCategoryOrder[65537] = &SoundCategory{Name: "Late"}
	if got := c.ResolveSoundCategory("Late"); got != order[0] {
		t.Fatalf("late matching ordinal = %p, want retained ordinal one %p", got, order[0])
	}
}
