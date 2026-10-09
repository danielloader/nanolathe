package session

import (
	"errors"
	"fmt"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/ai"
	"github.com/nanolathe-gg/nanolathe/internal/combat"
	"github.com/nanolathe-gg/nanolathe/internal/construction"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/economy"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

func TestCheckpointBuildErrorClassification(t *testing.T) {
	for _, tc := range []struct {
		cause error
		want  uint8
	}{
		{construction.ErrNilFactory, ai.CheckpointBuildOwner},
		{construction.ErrEmptyDef, ai.CheckpointBuildProduct},
		{construction.ErrUnknownProduct, ai.CheckpointBuildProduct},
		{construction.ErrMissingMovementProfile, ai.CheckpointBuildProduct},
		{construction.ErrNoQueue, ai.CheckpointBuildBinding},
		{construction.ErrNoBuildOrder, ai.CheckpointBuildBinding},
		{construction.ErrLimit, ai.CheckpointBuildLimit},
		{construction.ErrBadCount, ai.CheckpointBuildOther},
	} {
		for _, cause := range []error{tc.cause, fmt.Errorf("authored context: %w", tc.cause)} {
			got := classifyCheckpointBuildError(cause)
			verdict, ok := ai.CheckpointBuildVerdict(got)
			if !ok || verdict != tc.want || got.Error() != cause.Error() || errors.Unwrap(got) != cause || !errors.Is(got, tc.cause) {
				t.Fatalf("classify %v: verdict=%d/%v error=%v", cause, verdict, ok, got)
			}
		}
	}
	// Identical diagnostic text is not a sentinel identity. In particular,
	// ExhaustionError is deliberately not ErrLimit (DESIGN_MULTIPLAYER §16.3.25).
	for _, cause := range []error{errors.New("future refusal"), errors.New(construction.ErrUnknownProduct.Error()), construction.ExhaustionError()} {
		if got := classifyCheckpointBuildError(cause); got != cause {
			t.Fatalf("unknown error replaced: %v", got)
		} else if _, ok := ai.CheckpointBuildVerdict(got); ok {
			t.Fatalf("unknown error classified: %v", got)
		}
	}
	if got := classifyCheckpointBuildError(nil); got != nil {
		t.Fatal(got)
	}
}

// Each existing bind projects income once. The order seam counts only actual
// primary insertion preparations, independently of return classification.
type checkpointBuildTrace struct {
	orders.StrictRules
	binds, commands int
	onBind          func(int)
}

func (v *checkpointBuildTrace) DiscountWord(int, bool) (int, bool) {
	v.binds++
	if v.onBind != nil {
		v.onBind(v.binds)
	}
	return 0, false
}

func (v *checkpointBuildTrace) BeforeCommand(*orders.Queue) { v.commands++ }

func checkpointBuildFixture(t *testing.T) (*Session, *ai.Manager, *units.Unit, *checkpointBuildTrace) {
	t.Helper()
	builder := &content.UnitDef{UnitName: "builder", MaxDamage: 1, Script: fixtureCOBProgram()}
	cat := &content.Catalog{Units: map[string]*content.UnitDef{
		"builder": builder,
		"product": {UnitName: "product"},
		"broken":  {UnitName: "broken", BMCode: 1, FootprintX: 1, FootprintZ: 1},
		"alias":   {UnitName: "ARMMAKEANTI"},
		"lower":   {UnitName: "makenuke"},
	}}
	w := units.NewSliced(2, cat)
	h, err := w.Create(builder, 0, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	trace := &checkpointBuildTrace{}
	rules := StrictRuleSet()
	rules.Orders, rules.ComputerIncome = trace, trace
	s := &Session{Units: w, Catalog: cat, Rules: rules, Econ: &economy.Service{}, Combat: &combat.Service{},
		Build: &construction.Service{OrderBinding: orders.NewQueueBinding(orders.QueueBindingConfig{Lookup: w.Unit})}}
	u := w.Unit(h)
	m := &ai.Manager{}
	bindAIQueue(m, s)
	return s, m, u, trace
}

func TestCheckpointBuildClosureExistingRejections(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*Session, *units.Unit, *ai.BuildRequest)
		want uint8
		text string
		is   error
		bind bool
	}{
		{"missing world", func(s *Session, _ *units.Unit, _ *ai.BuildRequest) { s.Units = nil }, ai.CheckpointBuildBinding, "ai build: session units/catalog unavailable", nil, false},
		{"missing catalog", func(s *Session, _ *units.Unit, _ *ai.BuildRequest) { s.Catalog = nil }, ai.CheckpointBuildBinding, "ai build: session units/catalog unavailable", nil, false},
		{"missing builder", func(_ *Session, _ *units.Unit, r *ai.BuildRequest) { r.Builder = 0 }, ai.CheckpointBuildOwner, "ai build: builder handle 0 not alive", nil, false},
		{"dead builder", func(_ *Session, u *units.Unit, _ *ai.BuildRequest) { u.Alive = false }, ai.CheckpointBuildOwner, "ai build: builder handle 1 not alive", nil, false},
		{"empty product before count", func(_ *Session, _ *units.Unit, r *ai.BuildRequest) { r.UnitKey, r.Count = "", 0 }, ai.CheckpointBuildProduct, construction.ErrEmptyDef.Error(), construction.ErrEmptyDef, true},
		{"count before missing product", func(_ *Session, _ *units.Unit, r *ai.BuildRequest) { r.UnitKey, r.Count = "missing", 0 }, ai.CheckpointBuildOther, construction.ErrBadCount.Error(), construction.ErrBadCount, true},
		{"missing product", func(_ *Session, _ *units.Unit, r *ai.BuildRequest) { r.UnitKey = "missing" }, ai.CheckpointBuildProduct, `construction: product definition unavailable: "missing"`, construction.ErrUnknownProduct, true},
		{"missing profile", func(_ *Session, _ *units.Unit, r *ai.BuildRequest) { r.UnitKey = "broken" }, ai.CheckpointBuildProduct, `construction: product movement profile unavailable: class-less mobile product "broken"`, construction.ErrMissingMovementProfile, true},
		{"stockpile count", func(_ *Session, _ *units.Unit, r *ai.BuildRequest) { r.UnitKey, r.Count = "alias", 0 }, ai.CheckpointBuildOther, "ai build: stockpile round refused for builder 1", nil, true},
		{"stockpile slot", func(_ *Session, _ *units.Unit, r *ai.BuildRequest) { r.UnitKey = "alias" }, ai.CheckpointBuildProduct, "ai build: stockpile round refused for builder 1", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, m, u, trace := checkpointBuildFixture(t)
			r := ai.BuildRequest{Builder: u.Handle, UnitKey: "product", Kind: ai.BuildKindFactoryQueue, Count: 1}
			tc.edit(s, u, &r)
			err := m.QueueBuildTypedHook()(r)
			got, ok := ai.CheckpointBuildVerdict(err)
			if err == nil || err.Error() != tc.text || !ok || got != tc.want || tc.is != nil && !errors.Is(err, tc.is) {
				t.Fatalf("verdict=%d/%v error=%v", got, ok, err)
			}
			wantBinds := 0
			if tc.bind {
				wantBinds = 1
			}
			if trace.binds != wantBinds || trace.commands != 0 || (orders.QueueOfUnit(u) != nil) != tc.bind {
				t.Fatalf("binding/preparation/queue changed: %d/%d/%v", trace.binds, trace.commands, u.Orders)
			}
		})
	}
}

func TestCheckpointBuildPreservesProducerOperands(t *testing.T) {
	for _, tc := range []struct {
		name, product, row string
		kind               ai.BuildKind
	}{
		{"factory", "product", construction.FactoryBuildOrder, ai.BuildKindFactoryQueue},
		{"default", "product", construction.FactoryBuildOrder, ai.BuildKind(255)},
		{"lowercase alias", "lower", construction.FactoryBuildOrder, ai.BuildKindFactoryQueue},
		// The existing mobile producer admits this unresolved product/site.
		// Diagnostic classification must not add a factory or site preflight.
		{"mobile", "missing", construction.MobileBuildOrder, ai.BuildKindMobileSite},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, m, u, trace := checkpointBuildFixture(t)
			r := ai.BuildRequest{Builder: u.Handle, UnitKey: tc.product, Kind: tc.kind, Count: 3, X: numeric.Fixed(-17), Z: numeric.Fixed(29), Tick: 77}
			if err := m.QueueBuildTypedHook()(r); err != nil {
				t.Fatal(err)
			}
			q := orders.QueueOfUnit(u)
			n := q.Head()
			pid, _ := s.Catalog.UnitDefIndex(tc.product)
			if q.LenPrimary() != 1 || q.LenSecondary() != 0 || n.ID != orders.Lookup(tc.row) || n.Owner != u.Handle || n.Param1 != pid || n.Param2 != 3 || n.BuildDefKey != tc.product || n.CreationTick != 0 {
				t.Fatalf("producer changed: %+v", n)
			}
			if tc.kind == ai.BuildKindMobileSite && (n.GoalX != r.X || n.GoalZ != r.Z) {
				t.Fatalf("site changed: %+v", n)
			}
			if trace.binds != 1 || trace.commands != 1 {
				t.Fatalf("bind/preparation calls %d/%d", trace.binds, trace.commands)
			}
		})
	}
}

func TestCheckpointStockpileResultAndBooleanWrapper(t *testing.T) {
	s, _, u, trace := checkpointBuildFixture(t)
	for _, tc := range []struct {
		u     *units.Unit
		count int
		want  uint8
	}{{nil, 0, ai.CheckpointBuildOwner}, {u, 0, ai.CheckpointBuildOther}, {u, 1, ai.CheckpointBuildProduct}} {
		if got := s.queueStockpileRoundsResult(tc.u, tc.count, 77); got != tc.want || s.queueStockpileRounds(tc.u, tc.count, 77) {
			t.Fatalf("refusal=%d want=%d", got, tc.want)
		}
	}
	if trace.binds != 0 || u.Orders != nil {
		t.Fatal("refused stockpile work bound or created a queue")
	}
	u.SlotAt(0).Weapon = &content.WeaponDef{Stockpile: true}
	if !s.queueStockpileRounds(u, 3, 77) || trace.binds != 1 {
		t.Fatal("Boolean wrapper did not execute once")
	}
	if got := s.queueStockpileRoundsResult(u, 4, 99); got != ai.CheckpointBuildSuccess || trace.binds != 2 {
		t.Fatal("result helper did not execute once")
	}
	q := orders.QueueOfUnit(u)
	n := q.Secondary()[0]
	if q.LenPrimary() != 0 || q.LenSecondary() != 1 || n.ID != orders.Lookup("BuildWeapon") || n.Owner != u.Handle || n.Param1 != 0 || n.Param2 != 7 || n.CreationTick != 77 || trace.commands != 0 {
		t.Fatalf("stockpile coalescence changed: %+v", n)
	}
}

func TestCheckpointStockpileAliasKeepsTickAndBindingOrder(t *testing.T) {
	for _, tick := range []uint32{0, 77} {
		s, m, u, trace := checkpointBuildFixture(t)
		weapon := &content.WeaponDef{Stockpile: true}
		u.SlotAt(0).Weapon = weapon
		// Existing AI work binds once before the slot predicate and once after.
		// Changing the slot at the second bind detects a new post-bind predicate.
		trace.onBind = func(n int) {
			if n == 2 {
				weapon.Stockpile = false
			}
		}
		if err := m.QueueBuildTypedHook()(ai.BuildRequest{Builder: u.Handle, UnitKey: "alias", Kind: ai.BuildKindFactoryQueue, Count: 5, Tick: tick}); err != nil {
			t.Fatal(err)
		}
		q := orders.QueueOfUnit(u)
		n := q.Secondary()[0]
		if trace.binds != 2 || trace.commands != 0 || q.LenPrimary() != 0 || q.LenSecondary() != 1 || n.Param1 != 0 || n.Param2 != 5 || n.CreationTick != tick || q.Binding() != s.Build.OrderBinding {
			t.Fatalf("alias work changed: binds=%d node=%+v", trace.binds, n)
		}
	}
}

// Return classification cannot stand in for the actual insertion receipt:
// both counted producers already return success when Push's guard drops work.
func TestCheckpointBuildSuccessPreservesAllocationRefusal(t *testing.T) {
	for _, stockpile := range []bool{false, true} {
		s, m, u, trace := checkpointBuildFixture(t)
		q := orders.QueueForUnit(u)
		row := &orders.Node{ID: orders.Lookup("Stop"), Param2: 9}
		segment := make([]*orders.Node, orders.OOMGuardQueue)
		for i := range segment {
			segment[i] = row
		}
		product := "product"
		wantBinds, wantCommands := 1, 1
		if stockpile {
			row.ID = orders.Lookup("SelfDestruct")
			q.SetSecondary(segment)
			u.SlotAt(0).Weapon = &content.WeaponDef{Stockpile: true}
			product = "alias"
			wantBinds, wantCommands = 2, 0
		} else {
			q.SetPrimary(segment)
		}
		if err := m.QueueBuildTypedHook()(ai.BuildRequest{Builder: u.Handle, UnitKey: product, Kind: ai.BuildKindFactoryQueue, Count: 3, Tick: 77}); err != nil {
			t.Fatal(err)
		}
		if q.LenPrimary()+q.LenSecondary() != orders.OOMGuardQueue || row.Param2 != 9 || trace.binds != wantBinds || trace.commands != wantCommands || q.Binding() != s.Build.OrderBinding {
			t.Fatalf("guard refusal changed: stockpile=%v binds=%d preparations=%d", stockpile, trace.binds, trace.commands)
		}
	}
}
