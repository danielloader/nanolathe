package checkpoint

import "testing"

func TestBindingAuthorityRequiresExactNonAbsentOwner(t *testing.T) {
	a, b := NewBindingAuthority(), NewBindingAuthority()
	var absent *BindingAuthority
	if !a.Matches(a) || a.Matches(b) || b.Matches(a) || a.Matches(absent) || absent.Matches(a) || absent.Matches(absent) {
		t.Fatal("binding authority admitted a foreign or absent installation")
	}
	if n := testing.AllocsPerRun(100, func() {
		if !a.Matches(a) {
			panic("lost authority")
		}
	}); n != 0 {
		t.Fatal("binding validation allocated", n)
	}
}
