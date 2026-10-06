package path

import "testing"

// A latched search spends the combined budget, debiting its owner even below
// zero. Modern's carry cap and idle-sweep stop keep their existing seam;
// neither changes active-search continuation [04 R-PATH-01 §6].
func TestActiveSearchSpendsCombinedBudget(t *testing.T) {
	for _, tc := range []struct {
		name       string
		bounded    bool
		noSeam     bool
		otherCarry int32
		wantPops   int
		wantOwner  int32
		wantOther  int32
		wantCredit int32
	}{
		{name: "Strict", wantPops: 200, wantOwner: -100, wantOther: 100},
		{name: "Modern", bounded: true, wantPops: 200, wantOwner: -100, wantOther: 100},
		{name: "standalone", noSeam: true, wantPops: 200, wantOwner: -100, wantOther: 100},
		{name: "Strict carried credit", otherCarry: 700, wantPops: 900, wantOwner: -800, wantOther: 800},
		{name: "Modern capped credit", bounded: true, otherCarry: 700, wantPops: 500, wantOwner: -400, wantOther: 400, wantCredit: 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &boundProvider{sweep: 1}
			p.eligible[0], p.eligible[1] = true, true
			if tc.bounded {
				p.carry, p.sweepStop = modernCarryTestShares, true
			}
			var provider CandidateProvider = p
			if tc.noSeam {
				provider = retailProvider{p}
			}
			pops, published := 0, false
			s := newBoundScheduler(provider, func(r Request, _ int32, budget int) WorkResult {
				if r.Unit != 1 || budget != 100 {
					t.Fatalf("unexpected active search: unit=%d budget=%d", r.Unit, budget)
				}
				pops += budget
				return WorkResult{Pops: budget}
			})
			s.publish = func(Request, []Point, Status) { published = true }
			s.SetStepAllowance(200)
			// Seed a request already admitted on an earlier call. Each of the
			// two eligible players receives 100 credits on this call.
			s.active = &Request{Unit: 1, Player: 0}
			s.activePlayer = 0
			s.accumulator[1] = tc.otherCarry
			s.Tick(1)
			if pops != tc.wantPops || s.accumulator[0] != tc.wantOwner || s.accumulator[1] != tc.wantOther {
				t.Fatalf("pops=%d balances=[%d,%d], want %d [%d,%d]", pops,
					s.accumulator[0], s.accumulator[1], tc.wantPops, tc.wantOwner, tc.wantOther)
			}
			if s.serviceCount[1] != tc.wantCredit {
				t.Fatalf("credited idle work=%d, want %d", s.serviceCount[1], tc.wantCredit)
			}
			if published || s.active == nil || p.polls != ([10]int{}) {
				t.Fatal("budget boundary published, discarded the active request, or polled a competing unit")
			}
			if tc.otherCarry == 0 {
				// The next top-up takes the negative owner balance to zero;
				// the other player's credit still lets that request continue.
				s.Tick(2)
				if pops != 400 || s.accumulator[0] != -200 || s.accumulator[1] != 200 {
					t.Fatalf("continued request: pops=%d balances=[%d,%d], want 400 [-200,200]",
						pops, s.accumulator[0], s.accumulator[1])
				}
			}
		})
	}
}
