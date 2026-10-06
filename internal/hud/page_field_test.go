package hud

import "testing"

// stateAfter applies a producer's page request through the selected-builder
// writer, the path every authored page request takes.
func stateAfter(t *testing.T, s PageState, request, count int) PageState {
	t.Helper()
	builder := &SelectUnit{DefID: 1}
	if s.Paged {
		builder.Flags = PagePagedBit
	}
	builder.Flags |= uint32(s.Field&7) << 23
	if request < 0 || request >= count {
		t.Fatalf("request %d outside count %d from %+v", request, count, s)
	}
	SetBuildPage(builder, request, count, nil)
	return PageStateOf(builder.Flags)
}

// Retail's page producers are three-bit field operations [07 R-HUD-03 §6]. With
// nine or more pages no field equals count−1, so NEXT and `.` wrap from field 7
// to a shown zero field and then to page 1 instead of sticking at page 7.
func TestNinePageFieldOperations(t *testing.T) {
	type step struct {
		name  string
		from  PageState
		count int
		op    func(PageState, int) int
		want  PageState
	}
	next, prev := PageState.NextKey, PageState.PrevKey
	nextB, prevB := PageState.NextButton, PageState.PrevButton
	shown := func(f int) PageState { return PageState{Paged: true, Field: f} }
	hidden := func(f int) PageState { return PageState{Field: f} }
	for _, tc := range []step{
		{"dot from seven wraps to shown zero", shown(7), 9, next, shown(0)},
		{"dot from shown zero reaches page one", shown(0), 9, next, shown(1)},
		{"next from seven wraps to shown zero", shown(7), 9, nextB, shown(0)},
		{"next from shown zero reaches page one", shown(0), 9, nextB, shown(1)},
		{"comma from shown zero reaches seven", shown(0), 9, prev, shown(7)},
		{"comma from page one hides in place", shown(1), 9, prev, hidden(1)},
		{"comma from hidden shows count minus one modulo eight", hidden(1), 9, prev, shown(0)},
		{"comma from hidden with ten pages", hidden(4), 10, prev, shown(1)},
		{"prev from page one with nine pages", shown(1), 9, prevB, shown(0)},
		{"prev from shown zero with nine pages stays", shown(0), 9, prevB, shown(0)},
		{"prev from page one with ten pages stays", shown(1), 10, prevB, shown(1)},
		{"prev from page two", shown(2), 10, prevB, shown(1)},
		{"dot from hidden shows page one", hidden(5), 12, next, shown(1)},
		{"next from hidden advances the remembered field", hidden(5), 12, nextB, shown(6)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := stateAfter(t, tc.from, tc.op(tc.from, tc.count), tc.count); got != tc.want {
				t.Fatalf("%+v with %d pages -> %+v, want %+v", tc.from, tc.count, got, tc.want)
			}
		})
	}
}

// A nine-page builder pages through every selectable state with `.` and NEXT
// and returns to page 1; page 8 itself is unreachable [07 R-HUD-03 §6].
func TestNinePageCycleDoesNotStick(t *testing.T) {
	for _, op := range []func(PageState, int) int{PageState.NextKey, PageState.NextButton} {
		s := PageState{Paged: true, Field: 1}
		var seen []int
		for i := 0; i < 8; i++ {
			s = stateAfter(t, s, op(s, 9), 9)
			if !s.Paged {
				t.Fatalf("nine-page cycle hid the page at %+v", s)
			}
			seen = append(seen, s.Field)
		}
		want := []int{2, 3, 4, 5, 6, 7, 0, 1}
		for i := range want {
			if seen[i] != want[i] {
				t.Fatalf("field cycle %v, want %v", seen, want)
			}
		}
	}
}

// Within two to eight pages every key move, and every button move from a shown
// page, is the ordinary page walk the index producers describe.
func TestOrdinaryCountsMatchIndexWalk(t *testing.T) {
	for count := 2; count <= 8; count++ {
		for field := 1; field < count; field++ {
			for _, paged := range []bool{false, true} {
				s := PageState{Paged: paged, Field: field}
				page := 0 // a hidden page displays the orders state
				if paged {
					page = field
				}
				if got, want := s.NextKey(count), NextPageKey(page, count); got != want {
					t.Fatalf("count %d %+v: NextKey %d, index walk %d", count, s, got, want)
				}
				if got, want := s.PrevKey(count), PrevPageKey(page, count); got != want {
					t.Fatalf("count %d %+v: PrevKey %d, index walk %d", count, s, got, want)
				}
				if !paged {
					continue
				}
				if got, want := s.NextButton(count), NextPageButton(page, count); got != want {
					t.Fatalf("count %d %+v: NextButton %d, index walk %d", count, s, got, want)
				}
				if got, want := s.PrevButton(count), PrevPageButton(page, count); got != want {
					t.Fatalf("count %d %+v: PrevButton %d, index walk %d", count, s, got, want)
				}
			}
		}
	}
}

// BUILD sets the page-shown bit without writing the field [07 R-HUD-03 §6].
func TestBuildButtonShowsRememberedField(t *testing.T) {
	for _, tc := range []struct {
		s     PageState
		count int
		want  int
	}{
		{PageState{Field: 3}, 5, 3},
		{PageState{Field: 0}, 9, 8},
		{PageState{Field: 0}, 5, 1}, // host bounds guard below nine pages
		{PageState{Field: 6}, 4, 1},
	} {
		if got := tc.s.BuildButton(tc.count); got != tc.want {
			t.Fatalf("%+v with %d pages: BUILD requests %d, want %d", tc.s, tc.count, got, tc.want)
		}
	}
}
