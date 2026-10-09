package ai

import (
	"errors"
	"reflect"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/economy"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/sim/rng"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

func classicTestHistory(t *testing.T, m *Manager, mode int, product *content.UnitDef) *ApplicationHistory {
	t.Helper()
	if mode == 0 {
		return nil
	}
	h, err := NewApplicationHistory(checkpoint.Identity{}, m.Player, 1)
	if err != nil {
		t.Fatal(err)
	}
	m.checkpointHistory = h
	if product != nil {
		m.checkpointKeys = aiCheckpointContext(t, product).Units.Keys
	}
	if mode == 2 {
		h.Fail(errors.New("fixture diagnostic failure"))
	}
	return h
}

func classicExpectedAttempt(t *testing.T, player uint8, tick uint32, intent ClassicApplicationIntent, actor *units.Unit, product *checkpoint.Definition) (*ApplicationHistory, *ApplicationAttempt) {
	t.Helper()
	h, err := NewApplicationHistory(checkpoint.Identity{}, player, 1)
	if err != nil {
		t.Fatal(err)
	}
	intent.Operands.Actors = []checkpoint.Allocation{{Handle: uint32(actor.Handle), Serial: actor.AllocationSerial}}
	intent.Operands.Product = product
	return h, h.BeginAttempt(tick, h.NextSerial(), 0, intent.WriteCheckpoint)
}

func assertClassicHistory(t *testing.T, actual, expected *ApplicationHistory) {
	t.Helper()
	got, err := actual.Snapshot()
	want, wantErr := expected.Snapshot()
	if err != nil || wantErr != nil || got != want {
		t.Fatalf("application history\n got %+v (%v)\nwant %+v (%v)", got, err, want, wantErr)
	}
}

type classicTestRules struct {
	orders.StrictRules
	before func()
}

func (r classicTestRules) BeforeCommand(*orders.Queue) { r.before() }

func classicQueueNodes(u *units.Unit) []orders.Node {
	q := orders.QueueOfUnit(u)
	if q == nil {
		return nil
	}
	var out []orders.Node
	for _, n := range q.Primary() {
		out = append(out, *n)
	}
	return out
}

func expectedClassicInsert(a *ApplicationAttempt, actor checkpoint.Allocation, node orders.Node, index int64) {
	a.RecordOrder(actor, orders.CheckpointOrderReceipt{Kind: 5, Preparation: 1})
	a.RecordOrder(actor, orders.CheckpointOrderReceipt{Kind: 5, Preparation: 2})
	a.RecordOrder(actor, orders.CheckpointOrderReceipt{Kind: 3, Segment: 1, Index: index, Node: node})
}

// Resolution rejection still purges/inserts row zero; the diagnostic outcome
// is partial. The recorded original command and raw target remain distinct
// from that row and from the observed allocation [04 §3.4][R-ORD-01 §12].
func TestClassicOrderApplicationsPreserveGameplay(t *testing.T) {
	for _, rejected := range []bool{false, true} {
		for _, queued := range []bool{false, true} {
			type result struct {
				nodes []orders.Node
				state uint32
				draws uint64
				calls int
			}
			var baseline result
			for mode := 0; mode < 3; mode++ {
				u := &units.Unit{Handle: 7, AllocationSerial: 9}
				target := &units.Unit{Handle: 11, AllocationSerial: 13}
				random := rng.NewSimulation(17)
				calls := 0
				binding := &orders.QueueBinding{Rules: classicTestRules{before: func() { calls++; random.Uint32n(99) }}}
				m := &Manager{Player: 1, OrderBinding: binding}
				// Replace starts with no queue; queued insertion preserves an
				// existing record and restores its prior diagnostic observer.
				var prior orders.CheckpointOrderObserver
				index := int64(0)
				if queued {
					q := orders.QueueForUnit(u)
					q.Push(orders.Lookup("Move_Ground"), orders.Node{Param1: 23})
					prior = &producerObserverCount{}
					q.SetCheckpointObserver(prior)
					index = 1
				}
				h := classicTestHistory(t, m, mode, nil)
				id := orders.Lookup("Move_Ground")
				if rejected {
					id = 0
				}
				modifier := uint8(0)
				if queued {
					modifier = 1
				}
				m.submitResolvedOrder(u, 2, id, target, 1<<40, -2, 3, 77, modifier, 160)
				q := orders.QueueOfUnit(u)
				if q == nil || q.Binding() != binding || q.LenPrimary() != int(index)+1 || q.SetCheckpointObserver(nil) != prior {
					t.Fatal("submission changed queue/binding or retained an observer")
				}
				inserted := q.PrimaryAt(int(index))
				if inserted.ID != id || inserted.Param1 != 160 {
					t.Fatal("wrong final inserted node")
				}
				got := result{classicQueueNodes(u), random.State, random.Draws(), calls}
				if mode == 0 {
					baseline = got
				} else if !reflect.DeepEqual(got, baseline) {
					t.Fatalf("mode %d changed gameplay: %+v versus %+v", mode, got, baseline)
				}
				if mode != 1 {
					continue
				}
				intent := ClassicApplicationIntent{Kind: 1, Code: 2, ResolvedRow: uint8(id), Modifier: modifier, Argument: 160, X: 1 << 40, Y: -2, Z: 3, RawTarget: 11,
					Operands: ApplicationOperands{Target: &checkpoint.Allocation{Handle: 11, Serial: 13}}}
				want, a := classicExpectedAttempt(t, m.Player, 77, intent, u, nil)
				actor := checkpoint.Allocation{Handle: 7, Serial: 9}
				if !queued {
					a.RecordOrder(actor, orders.CheckpointOrderReceipt{Kind: 1})
					a.RecordOrder(actor, orders.CheckpointOrderReceipt{Kind: 2})
				}
				expectedClassicInsert(a, actor, *inserted, index)
				terminal := uint8(2)
				if rejected {
					terminal = 4
				}
				a.Finish(1, terminal)
				assertClassicHistory(t, h, want)
			}
		}
	}
}

func TestClassicMobileApplicationsPartialAndNestedTypedCall(t *testing.T) {
	for _, outcome := range []string{"insert", "coalesce", "nil without insertion", "typed rejection", "row zero", "missing binding", "unknown failure"} {
		t.Run(outcome, func(t *testing.T) {
			type result struct {
				nodes    []orders.Node
				requests []BuildRequest
				calls    int
				state    uint32
				draws    uint64
			}
			var baseline result
			for mode := 0; mode < 3; mode++ {
				m, _, u, _, random, _ := constructionOrderGateFixture(t)
				m.Factory = u
				product, _ := m.Catalog.Unit("gate-product")
				h := classicTestHistory(t, m, mode, product)
				calls := 0
				m.OrderBinding.SetBuildList(func(*content.UnitDef) bool { calls++; return outcome != "row zero" })
				var requests []BuildRequest
				var typedError error
				if outcome == "typed rejection" {
					typedError = WithCheckpointBuildVerdict(errors.New("fixture product refused"), CheckpointBuildProduct)
				}
				if outcome == "unknown failure" {
					typedError = errors.New("unclassified fixture")
				}
				m.SetQueueBuildTyped(func(req BuildRequest) error {
					requests = append(requests, req)
					if outcome == "insert" || outcome == "coalesce" {
						q := orders.BindQueueBinding(u, m.OrderBinding) // the session's existing bind
						a := m.CheckpointApplicationHistory().ActiveAttempt()
						restore := a.ObserveQueue(q, u)
						q.CoalesceTail(orders.Lookup("MobileBuild"), orders.NewNodeForOrder(orders.Lookup("MobileBuild"), 0, req.X, 0, req.Z, req.Tick, req.Builder, false))
						restore()
						if mode == 1 && a.IssuedOrders() != 1 {
							t.Fatal("nested observer duplicated insertion")
						}
					}
					return typedError
				})
				if outcome == "missing binding" {
					m.SetQueueBuildTyped(nil)
				}
				res := PlacementResult{Valid: true, WorldX: 1 << 40, WorldZ: -3}
				if outcome == "coalesce" {
					n := orders.NewNodeForOrder(orders.Lookup("MobileBuild"), 0, res.WorldX, 0, res.WorldZ, 0, u.Handle, false)
					n.Flags, n.Param2 = orders.FlagPurgeSurvivor, 4
					orders.QueueForUnit(u).Push(n.ID, n)
				} else {
					orders.QueueForUnit(u).Push(orders.Lookup("Move_Ground"), orders.Node{Param1: 99})
				}
				m.issueMobileBuild(u, "gate-product", res, 77)
				q := orders.QueueOfUnit(u)
				if q == nil || q.SetCheckpointObserver(nil) != nil {
					t.Fatal("missing queue or retained observer")
				}
				got := result{classicQueueNodes(u), requests, calls, random.State, random.Draws()}
				if mode == 0 {
					baseline = got
				} else if !reflect.DeepEqual(got, baseline) {
					t.Fatalf("mode %d changed mobile application", mode)
				}
				if len(requests) != 0 && requests[0].Tick != 0 {
					t.Fatal("mobile request acquired an invented tick")
				}
				if mode != 1 {
					continue
				}
				if outcome == "unknown failure" {
					if _, err := h.Snapshot(); err == nil {
						t.Fatal("unclassified producer failure reported a valid history")
					}
					continue
				}
				productRef, err := m.CheckpointApplicationKey(product)
				if err != nil {
					t.Fatal(err)
				}
				intent := ClassicApplicationIntent{Kind: 2, UnitKey: "gate-product", X: res.WorldX, Z: res.WorldZ, Count: 1}
				want, a := classicExpectedAttempt(t, m.Player, 77, intent, u, &productRef)
				actor, _ := CheckpointAllocation(u)
				a.RecordOrder(actor, orders.CheckpointOrderReceipt{Kind: 1})
				a.RecordOrder(actor, orders.CheckpointOrderReceipt{Kind: 2})
				if outcome == "coalesce" {
					a.RecordOrder(actor, orders.CheckpointOrderReceipt{Kind: 4, Segment: 1, PreviousCount: 4, Added: 1, Node: *q.Head()})
				} else if q.LenPrimary() != 0 {
					expectedClassicInsert(a, actor, *q.Head(), 0)
				}
				if len(requests) != 0 {
					a.RecordBuild(actor, requests[0], typedError)
				}
				terminal := uint8(4)
				if outcome == "insert" || outcome == "coalesce" {
					terminal = 2
				}
				a.Finish(1, terminal)
				assertClassicHistory(t, h, want)
			}
		})
	}
}

func TestClassicFactoryApplicationsObserveOnlyExistingSessionBind(t *testing.T) {
	for _, resource := range []bool{false, true} {
		for _, outcome := range []string{"insert", "reject before bind", "nil without insertion", "missing binding"} {
			type result struct {
				nodes    []orders.Node
				requests []BuildRequest
				state    uint32
				draws    uint64
			}
			var baseline result
			for mode := 0; mode < 3; mode++ {
				m, w, u, econ, random, _ := constructionOrderGateFixture(t)
				u.Flags |= classifierBuilding
				product, _ := m.Catalog.Unit("gate-product")
				product.BMCode = 1
				h := classicTestHistory(t, m, mode, product)
				var requests []BuildRequest
				var typedError error
				if outcome == "reject before bind" {
					typedError = WithCheckpointBuildVerdict(errors.New("fixture owner refused"), CheckpointBuildOwner)
				}
				m.SetQueueBuildTyped(func(req BuildRequest) error {
					requests = append(requests, req)
					if orders.QueueOfUnit(u) != nil {
						t.Fatal("instrumentation bound a factory queue before the producer")
					}
					if outcome == "insert" {
						q := orders.BindQueueBinding(u, m.OrderBinding)
						restore := m.CheckpointApplicationHistory().ActiveAttempt().ObserveQueue(q, u)
						q.CoalesceTail(orders.Lookup("BuildingBuild"), orders.Node{Owner: req.Builder, BuildDefKey: req.UnitKey, Param2: uint32(req.Count), CreationTick: req.Tick})
						restore()
					}
					return typedError
				})
				if outcome == "missing binding" {
					m.SetQueueBuildTyped(nil)
				}
				if resource {
					m.doResourceGroup(90, w, econ, []pool.Handle{u.Handle})
				} else {
					m.constructionPlacePass(90, w, econ, m.Strategic.CenterX, m.Strategic.CenterZ, 0)
				}
				got := result{classicQueueNodes(u), requests, random.State, random.Draws()}
				if mode == 0 {
					baseline = got
				} else if !reflect.DeepEqual(got, baseline) {
					t.Fatalf("resource %v outcome %s mode %d changed factory behavior", resource, outcome, mode)
				}
				if outcome == "missing binding" {
					if len(requests) != 0 || mode == 1 && h.nextSerial != 1 {
						t.Fatal("unsubmitted request acquired an attempt")
					}
					continue
				}
				if len(requests) != 1 || requests[0].Tick != 90 {
					t.Fatalf("factory requests = %+v", requests)
				}
				if mode != 1 {
					continue
				}
				ref, err := m.CheckpointApplicationKey(product)
				if err != nil {
					t.Fatal(err)
				}
				want, a := classicExpectedAttempt(t, m.Player, 90, ClassicApplicationIntent{Kind: 2, BuildKind: 1, UnitKey: "gate-product", Count: 1, RequestTick: 90}, u, &ref)
				actor, _ := CheckpointAllocation(u)
				terminal := uint8(3)
				if q := orders.QueueOfUnit(u); q != nil {
					expectedClassicInsert(a, actor, *q.Head(), 0)
					terminal = 2
					if q.SetCheckpointObserver(nil) != nil {
						t.Fatal("session observer was not restored")
					}
				}
				a.RecordBuild(actor, requests[0], typedError)
				a.Finish(1, terminal)
				assertClassicHistory(t, h, want)
			}
		}
	}
}

func TestClassicActivationApplicationsRetainActualCallsAndRNG(t *testing.T) {
	var seeds [2]uint32
	for seed := uint32(1); seeds[0] == 0 || seeds[1] == 0; seed++ {
		r := rng.NewSimulation(seed)
		if r.Uint32n(5) == 0 {
			seeds[0] = seed
		} else {
			seeds[1] = seed
		}
	}
	for _, tc := range []struct {
		initial            bool
		energy, production float32
		seed               uint32
		calls              bool
		active             bool
	}{
		{false, 200, 20, seeds[0], true, false}, // an already-equal setter still records
		{true, 200, 20, seeds[0], true, false},
		{false, 201, 20, seeds[1], true, true},
		{false, 201, 20, seeds[0], false, false},
		{false, 201, 10, seeds[1], false, false},
	} {
		type result struct {
			active bool
			cues   int
			state  uint32
			draws  uint64
		}
		var baseline result
		for mode := 0; mode < 3; mode++ {
			def := &content.UnitDef{UnitName: "maker", MaxDamage: 100, MakesMetal: 1, OnOffable: true}
			w := newAIFixtureWorld(1, nil)
			handle, err := w.Create(def, 0, 0, 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			u := w.Unit(handle)
			u.SetActivated(tc.initial)
			random := rng.NewSimulation(tc.seed)
			m := &Manager{Player: 0, RNG: &random, GroupResource: []pool.Handle{handle}}
			h := classicTestHistory(t, m, mode, nil)
			cues := 0
			u.SetStatusCueSink(func(*units.Unit, uint8) {
				cues++
				if mode == 1 {
					a := h.ActiveAttempt()
					if a == nil || a.operationCount != 1 || a.CommittedOperations() != 1 {
						t.Fatal("activation receipt did not precede setter effects")
					}
				}
			})
			econ := runtimeEconomy(0, 2)
			econ.Players[0].Stock[economy.Metal], econ.Players[0].Stock[economy.Energy] = 100, tc.energy
			econ.Players[0].AIProduction[economy.Energy], econ.Players[0].AIConsumption[economy.Energy] = tc.production, 10
			m.doResource(30, w, econ)
			got := result{u.Activated, cues, random.State, random.Draws()}
			if mode == 0 {
				baseline = got
			} else if got != baseline {
				t.Fatalf("mode %d changed activation: %+v vs %+v", mode, got, baseline)
			}
			if u.Activated != tc.active {
				t.Fatal("unexpected activation result")
			}
			if mode != 1 {
				continue
			}
			if !tc.calls {
				if h.count != 0 || h.nextSerial != 1 {
					t.Fatal("non-submission invented an activation attempt")
				}
				continue
			}
			want, a := classicExpectedAttempt(t, 0, 30, ClassicApplicationIntent{Kind: 3, Active: tc.active}, u, nil)
			actor, _ := CheckpointAllocation(u)
			a.RecordActivation(actor, tc.active)
			a.Finish(1, 2)
			assertClassicHistory(t, h, want)
		}
	}
}

func TestClassicExactBuildErrorClassificationAndBorrowedScope(t *testing.T) {
	u := &units.Unit{Handle: 7, AllocationSerial: 9}
	for _, tc := range []struct {
		m       *Manager
		valid   bool
		text    string
		verdict uint8
	}{
		{&Manager{}, false, "ai: placement result is invalid", CheckpointBuildSite},
		{&Manager{}, true, "ai: builder unavailable", CheckpointBuildOwner},
		{&Manager{Factory: u}, true, "ai: typed build queue unavailable", CheckpointBuildBinding},
	} {
		actor := classicApplicationActor(tc.m.CheckpointApplicationHistory().ActiveAttempt(), tc.m.Factory)
		err := queueExactResult(tc.m, "raw", PlacementResult{Valid: tc.valid}, actor)
		verdict, known := CheckpointBuildVerdict(err)
		if err == nil || err.Error() != tc.text || !known || verdict != tc.verdict {
			t.Fatalf("classification %v / %d", err, verdict)
		}
	}
	cause := errors.New("fixture limit")
	m := NewManager(ManagerConfig{Factory: u, QueueBuildTyped: func(BuildRequest) error { return WithCheckpointBuildVerdict(cause, CheckpointBuildLimit) }})
	actor := classicApplicationActor(m.CheckpointApplicationHistory().ActiveAttempt(), m.Factory)
	err := queueExactResult(m, "raw", PlacementResult{Valid: true}, actor)
	verdict, known := CheckpointBuildVerdict(err)
	if err.Error() != "ai: typed build queue: fixture limit" || !errors.Is(err, cause) || !known || verdict != CheckpointBuildLimit {
		t.Fatal("wrapped error lost text or identity", err)
	}
	// Direct placement fixtures do not invent a submission when no caller
	// opened a scope. Production's enclosing mobile call owns the attempt.
	h := classicTestHistory(t, m, 1, nil)
	actor = classicApplicationActor(h.ActiveAttempt(), m.Factory)
	_ = queueExactResult(m, "raw", PlacementResult{Valid: true}, actor)
	if h.count != 0 || h.nextSerial != 1 || h.err != nil {
		t.Fatal("queueExactResult created an independent attempt")
	}
}

// Broadcast serials follow the actual pool-ordered member submissions, even
// when a task vector has been reordered [08 R-AI-01 §9]. Each carries the
// original command, one actor and ordinal zero.
func TestClassicBroadcastApplicationSerialOrder(t *testing.T) {
	def := &content.UnitDef{UnitName: "walker", MaxDamage: 100, BMCode: 1, CanMove: true}
	w := newAIFixtureWorld(3, nil)
	var actors []*units.Unit
	for i := 0; i < 2; i++ {
		h, err := w.Create(def, 1, 0, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		u := w.Unit(h)
		u.Group = 3
		actors = append(actors, u)
	}
	m := &Manager{Player: 1, GroupRegroupA: []pool.Handle{actors[1].Handle, actors[0].Handle}}
	h := classicTestHistory(t, m, 1, nil)
	m.broadcastGroupOrder(w, 3, 2, 0, nil, 5, 7, 11, 91, 160)
	want, err := NewApplicationHistory(checkpoint.Identity{}, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range actors {
		q := orders.QueueOfUnit(u)
		if q == nil || q.LenPrimary() != 1 {
			t.Fatal("broadcast did not submit each member")
		}
		ref, _ := CheckpointAllocation(u)
		intent := ClassicApplicationIntent{Kind: 1, Code: 2, ResolvedRow: uint8(q.Head().ID), Argument: 160,
			X: 5, Y: 7, Z: 11, Operands: ApplicationOperands{Actors: []checkpoint.Allocation{ref}}}
		a := want.BeginAttempt(91, want.NextSerial(), 0, intent.WriteCheckpoint)
		a.RecordOrder(ref, orders.CheckpointOrderReceipt{Kind: 1})
		a.RecordOrder(ref, orders.CheckpointOrderReceipt{Kind: 2})
		expectedClassicInsert(a, ref, *q.Head(), 0)
		a.Finish(1, 2)
	}
	assertClassicHistory(t, h, want)
}

func TestClassicApplicationIdentityFailuresNeverGateSubmission(t *testing.T) {
	for _, failure := range []string{"unknown product", "unadmitted product", "missing actor serial"} {
		m, _, u, _, _, _ := constructionOrderGateFixture(t)
		m.Factory = u
		product, _ := m.Catalog.Unit("gate-product")
		h := classicTestHistory(t, m, 1, product)
		key := "gate-product"
		switch failure {
		case "unknown product":
			key = " raw-missing-key "
		case "unadmitted product":
			copy := *product
			m.Catalog.Units[key] = &copy
		case "missing actor serial":
			u.AllocationSerial = 0
		}
		calls := 0
		m.SetQueueBuildTyped(func(req BuildRequest) error {
			calls++
			if req.UnitKey != key {
				t.Fatal("raw request key was normalized")
			}
			return WithCheckpointBuildVerdict(errors.New("fixture rejected product"), CheckpointBuildProduct)
		})
		m.issueMobileBuild(u, key, PlacementResult{Valid: true}, 37)
		if calls != 1 || orders.QueueOfUnit(u) == nil {
			t.Fatal("diagnostic identity failure gated gameplay")
		}
		_, err := h.Snapshot()
		if failure == "unknown product" {
			if err != nil || h.count != 1 {
				t.Fatalf("unknown raw product was not recorded: %v", err)
			}
		} else if err == nil {
			t.Fatal("unadmitted identity reported a valid history")
		}
	}
}

func TestClassicOuterInsertionRefusalWithNestedPreparationInsert(t *testing.T) {
	u := &units.Unit{Handle: 7, AllocationSerial: 9}
	q := orders.QueueForUnit(u)
	nodes := make([]*orders.Node, orders.OOMGuardQueue-1)
	for i := range nodes {
		nodes[i] = &orders.Node{}
	}
	q.SetPrimary(nodes)
	m := &Manager{Player: 1}
	m.OrderBinding = &orders.QueueBinding{Rules: classicTestRules{before: func() {
		q.PushHead(orders.Lookup("Stop"), orders.Node{Param1: 23})
	}}}
	h := classicTestHistory(t, m, 1, nil)
	id := orders.Lookup("Move_Ground")
	m.submitResolvedOrder(u, 2, id, nil, 5, 7, 11, 91, 1, 160)
	if q.LenPrimary() != orders.OOMGuardQueue || q.Head().Param1 != 23 || len(q.Diagnostics()) != 1 {
		t.Fatal("fixture did not insert during preparation and refuse the outer request")
	}
	want, a := classicExpectedAttempt(t, 1, 91, ClassicApplicationIntent{Kind: 1, Code: 2, ResolvedRow: uint8(id), Modifier: 1, Argument: 160, X: 5, Y: 7, Z: 11}, u, nil)
	actor, _ := CheckpointAllocation(u)
	a.RecordOrder(actor, orders.CheckpointOrderReceipt{Kind: 5, Preparation: 1})
	a.RecordOrder(actor, orders.CheckpointOrderReceipt{Kind: 5, Preparation: 2})
	a.RecordOrder(actor, orders.CheckpointOrderReceipt{Kind: 3, Segment: 1, Index: 0, Node: *q.Head()})
	a.Finish(1, 4)
	assertClassicHistory(t, h, want)
}

func TestClassicTypedInsertionRefusalWithNestedPreparationInsert(t *testing.T) {
	m, _, u, _, _, _ := constructionOrderGateFixture(t)
	m.Factory = u
	product, _ := m.Catalog.Unit("gate-product")
	h := classicTestHistory(t, m, 1, product)
	q := orders.QueueForUnit(u)
	nodes := make([]*orders.Node, orders.OOMGuardQueue-1)
	for i := range nodes {
		nodes[i] = &orders.Node{Flags: orders.FlagPurgeSurvivor}
	}
	q.SetPrimary(nodes)
	m.OrderBinding.Rules = classicTestRules{before: func() {
		q.PushHead(orders.Lookup("Stop"), orders.Node{Param1: 23})
	}}
	var request BuildRequest
	m.SetQueueBuildTyped(func(req BuildRequest) error {
		request = req
		bound := orders.BindQueueBinding(u, m.OrderBinding)
		restore := h.ActiveAttempt().ObserveQueue(bound, u)
		bound.CoalesceTail(orders.Lookup("MobileBuild"), orders.Node{Owner: req.Builder, BuildDefKey: req.UnitKey, Param2: 1})
		restore()
		return nil
	})
	m.issueMobileBuild(u, "gate-product", PlacementResult{Valid: true}, 91)
	if q.LenPrimary() != orders.OOMGuardQueue || q.Head().Param1 != 23 || len(q.Diagnostics()) != 1 {
		t.Fatal("fixture did not preserve nested insertion while refusing the requested typed build")
	}
	productRef, err := m.CheckpointApplicationKey(product)
	if err != nil {
		t.Fatal(err)
	}
	want, a := classicExpectedAttempt(t, m.Player, 91, ClassicApplicationIntent{Kind: 2, UnitKey: "gate-product", Count: 1}, u, &productRef)
	actor, _ := CheckpointAllocation(u)
	a.RecordOrder(actor, orders.CheckpointOrderReceipt{Kind: 1})
	a.RecordOrder(actor, orders.CheckpointOrderReceipt{Kind: 2})
	a.RecordOrder(actor, orders.CheckpointOrderReceipt{Kind: 5, Preparation: 1})
	a.RecordOrder(actor, orders.CheckpointOrderReceipt{Kind: 5, Preparation: 2})
	a.RecordOrder(actor, orders.CheckpointOrderReceipt{Kind: 3, Segment: 1, Index: 0, Node: *q.Head()})
	a.RecordBuild(actor, request, nil)
	a.Finish(1, 4)
	assertClassicHistory(t, h, want)
}

func TestClassicApplicationPanicsRemainIncomplete(t *testing.T) {
	for _, mobile := range []bool{false, true} {
		name := "direct preparation"
		if mobile {
			name = "mobile typed callback"
		}
		t.Run(name, func(t *testing.T) {
			m, _, u, _, _, _ := constructionOrderGateFixture(t)
			m.Factory = u
			product, _ := m.Catalog.Unit("gate-product")
			h := classicTestHistory(t, m, 1, product)
			beforeCount, beforeHash := h.count, h.digest
			q := orders.QueueForUnit(u)
			prior := &producerObserverCount{}
			q.SetCheckpointObserver(prior)
			payload := errors.New("fixture producer panic")
			calls := 0
			var submit func()
			if mobile {
				m.SetQueueBuildTyped(func(BuildRequest) error {
					calls++
					panic(payload)
				})
				submit = func() { m.issueMobileBuild(u, "gate-product", PlacementResult{Valid: true}, 91) }
			} else {
				m.OrderBinding.Rules = classicTestRules{before: func() {
					calls++
					panic(payload)
				}}
				submit = func() { m.submitResolvedOrder(u, 2, orders.Lookup("Move_Ground"), nil, 5, 7, 11, 91, 1, 0) }
			}
			var caught any
			func() {
				defer func() { caught = recover() }()
				submit()
			}()
			if caught != payload || calls != 1 {
				t.Fatalf("producer panic changed: %v, calls %d", caught, calls)
			}
			if q.SetCheckpointObserver(nil) != prior {
				t.Fatal("panic did not restore the previous observer")
			}
			if h.count != beforeCount || h.digest != beforeHash {
				t.Fatal("panic advanced the completed application chain")
			}
			if _, err := h.Snapshot(); err == nil {
				t.Fatal("incomplete application produced a usable history")
			}
		})
	}
}
