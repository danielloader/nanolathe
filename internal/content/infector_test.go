package content

import "testing"

// The authored infector token is compiled once from the bounded category
// string: exact, case-insensitive and whitespace-delimited. The derived flag
// adds no identity of its own, so stock definition hashes and simulation
// digests cannot move, and clones carry it (DESIGN_UNITS_ORDERS_COB "Modern
// infection").
func TestNanolatheInfectorCompiledFromCategoryToken(t *testing.T) {
	for _, tc := range []struct {
		category string
		want     bool
	}{
		{"KBOT\tNaNoLaThE_InFeCtOr\nWEAPON", true},
		{"NANOLATHE_INFECTOR", true},
		{"NOT_NANOLATHE_INFECTOR", false},
		{"NANOLATHE_INFECTORS KBOT", false},
		{"ARM KBOT WEAPON", false},
		{"", false},
	} {
		u := compileOneUnit(t, "Category="+tc.category+";")
		if u.NanolatheInfector != tc.want || CategoryHasToken(tc.category, NanolatheInfectorCategory) != tc.want {
			t.Errorf("category %q compiled infector=%t, want %t", tc.category, u.NanolatheInfector, tc.want)
		}
	}

	u := compileOneUnit(t, "Category=KBOT NANOLATHE_INFECTOR;")
	flipped := *u
	flipped.NanolatheInfector = false
	if string(writeUnitCanonical(u)) != string(writeUnitCanonical(&flipped)) || unitSemanticDigest(u) != unitSemanticDigest(&flipped) {
		t.Fatal("derived infector flag entered a definition identity")
	}
	if cloned := cloneUnit(u); !cloned.NanolatheInfector {
		t.Fatal("clone dropped the compiled infector flag")
	}
}
