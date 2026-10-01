package construction

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/cob"
	"github.com/nanolathe-gg/nanolathe/internal/economy"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/save"
	"github.com/nanolathe-gg/nanolathe/internal/sim/rng"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

func issue73BuildFixture(t *testing.T, modern bool) (*Service, *units.Unit, *orders.Node, *cob.Binding, *countingNanoSink) {
	t.Helper()
	s, builder, node := approachFixture(t, 10, 10)
	s.Rules = clearanceRules(modern)
	s.Economy = &economy.Service{}
	node.Phase, node.DynamicGate, node.Deadline = 2, 0, -1
	builder.Script, builder.ScriptState = nil, nil
	// Authored script: readiness follows a delay, then another animation delay
	// keeps StartBuilding alive after readiness. Wake touches BUSY only.
	builder.Def.Script = &cob.Program{
		Pieces: []string{"piece0"}, Scripts: map[string]int{"StartBuilding": 0, "Wake": 14}, ScriptsByID: []int{0, 14},
		Code: []uint32{
			0x10021001, 1000, 0x10013000,
			0x10021001, 5, 0x10021001, 1, 0x10082000,
			0x10021001, 1000, 0x10013000, 0x10021001, 0, 0x10065000,
			0x10021001, 6, 0x10021001, 0, 0x10082000, 0x10021001, 0, 0x10065000,
		},
	}
	sim := rng.NewSimulation(1)
	binding, err := units.BindCOBWithPortsAndVisibilityForUnit(nil, builder, trivialModel(1, nil), &sim, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	q := orders.QueueForUnit(builder)
	q.SetBinding(&orders.QueueBinding{Lookup: s.World.Unit, SimRNG: &sim})
	s.RegisterOrderHandlers(q)
	sink := &countingNanoSink{}
	s.Presentation = sink
	return s, builder, node, binding, sink
}

// Placement may precede readiness, but resource admission, HP, remaining
// fraction and spray must wait together [04 R-ORD-01 §5][05 R-P0-06 §1].
func TestIssue73GroundBuildWaitsForScriptReadiness(t *testing.T) {
	for _, modern := range []bool{false, true} {
		t.Run(map[bool]string{false: "Strict31", true: "Modern"}[modern], func(t *testing.T) {
			s, builder, node, binding, sink := issue73BuildFixture(t, modern)
			q := orders.QueueForUnit(builder)
			s.StepUnit(TickContext{Tick: 100}, builder.Handle)
			product := s.World.Unit(node.Target)
			if product == nil {
				t.Fatalf("placement made no frame: %v", s.Messages())
			}
			builder.RevealDeadline = 17
			initialBuckets := *s.Economy.UnitBuckets(builder.Handle)
			initialDraws := q.Binding().SimRNG.Draws()
			assertWaiting := func(stage string) {
				t.Helper()
				if product.Health != 0 || product.Remaining != 1 || sink.count != 0 || *s.Economy.UnitBuckets(builder.Handle) != initialBuckets || builder.RevealDeadline != 17 || q.Binding().SimRNG.Draws() != initialDraws {
					t.Fatalf("%s before readiness: stance=%t health=%d remaining=%g spray=%d resources=%+v reveal=%d", stage, builder.InBuildStance, product.Health, product.Remaining, sink.count, *s.Economy.UnitBuckets(builder.Handle), builder.RevealDeadline)
				}
				if node.Phase != 2 || node.DynamicGate != 0xE || node.Deadline != -1 {
					t.Fatalf("%s wait phase/gate/deadline=%d/%#x/%d", stage, node.Phase, node.DynamicGate, node.Deadline)
				}
			}
			if product.Health != 0 || product.Remaining != 1 || sink.count != 0 {
				t.Fatal("placement admitted work before readiness")
			}
			binding.VM.Drain(1)
			q.Pump(builder, 101)
			s.StepUnit(TickContext{Tick: 101}, builder.Handle)
			assertWaiting("clear stance")
			// A changed level alone is not the script event that wakes this wait.
			builder.InBuildStance = true
			q.Pump(builder, 102)
			s.StepUnit(TickContext{Tick: 102}, builder.Handle)
			assertWaiting("level without event")
			builder.InBuildStance = false
			binding.Callbacks.Deferred("Wake", nil, nil)
			binding.VM.Drain(1)
			q.Pump(builder, 103)
			s.StepUnit(TickContext{Tick: 103}, builder.Handle)
			assertWaiting("unrelated port write")
			for tick := uint32(104); tick < 160; tick++ {
				binding.VM.Drain(1)
				q.Pump(builder, tick)
				s.StepUnit(TickContext{Tick: tick}, builder.Handle)
				if !builder.InBuildStance {
					assertWaiting("authored delay")
					continue
				}
				if product.Health == 0 || product.Remaining >= 1 || sink.count != 1 || *s.Economy.UnitBuckets(builder.Handle) == initialBuckets || builder.RevealDeadline != tick+300 {
					t.Fatalf("ready visit did not work: health=%d remaining=%g spray=%d", product.Health, product.Remaining, sink.count)
				}
				if binding.VM.Threads[0].Status != cob.ThreadSleeping {
					t.Fatal("fixture did not retain StartBuilding's animation after readiness")
				}
				t.Logf("ready tick=%d health=%d remaining=%g spray=%d; StartBuilding still sleeping", tick, product.Health, product.Remaining, sink.count)
				return
			}
			t.Fatal("authored readiness write never ran")
		})
	}
}

// A high stance is a sufficient level test, even with a deferred callback;
// aircraft retain the discarded verdict with a low stance [04 R-ORD-01 §1,
// §5][04 R-ORD-02 §2]. Neither case waits for callback completion.
func TestIssue73ImmediateBuildReadinessBoundaries(t *testing.T) {
	for _, air := range []bool{false, true} {
		t.Run(map[bool]string{false: "ready-ground", true: "unready-air"}[air], func(t *testing.T) {
			s, builder, node, _, sink := issue73BuildFixture(t, false)
			builder.InBuildStance, builder.Def.CanFly = !air, air
			if air {
				node.ID = orders.Lookup(VTOLMobileBuildOrder)
			}
			s.StepUnit(TickContext{Tick: 100}, builder.Handle)
			product := s.World.Unit(node.Target)
			if product == nil {
				t.Fatal("no product")
			}
			if product.Health == 0 || product.Remaining >= 1 || sink.count != 1 {
				t.Fatalf("first work visit waited: health=%d remaining=%g spray=%d", product.Health, product.Remaining, sink.count)
			}
			if builder.InBuildStance != !air {
				t.Fatal("placement rewrote the builder's script-owned stance")
			}
		})
	}
}

// A saved phase-2 product reference is a readiness visit, including retail's
// phase-2 shape. Restoring it must not validate or allocate the site again
// [04 R-ORD-01 §5][08 R-SAVE-ORDER-01]. This authored fixture tests that
// bounded interpretation without claiming parity for every saved build phase.
func TestIssue73BoundFrameReadinessSurvivesOrderRestore(t *testing.T) {
	s, builder, node, _, sink := issue73BuildFixture(t, false)
	s.StepUnit(TickContext{Tick: 100}, builder.Handle)
	product := s.World.Unit(node.Target)
	if product == nil {
		t.Fatal("no product")
	}
	node.Phase, node.DynamicGate, node.Deadline = 2, 0xE, -1
	stable := map[uint16]pool.Handle{uint16(builder.Handle): builder.Handle, uint16(product.Handle): product.Handle}
	images, err := orders.RetailOrderImagesWithPayload(builder, func(h pool.Handle) (uint16, bool) { return uint16(h), s.World.Unit(h) != nil }, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	records := make([]save.OrderRecord, len(images))
	for i, image := range images {
		records[i] = save.OrderRecord{ParentStableID: image.ParentStableID, Sequence: image.Sequence, Main: image.Main, DescriptorName: image.DescriptorName, BuildTypeName: image.BuildTypeName}
	}
	missing := map[uint16]pool.Handle{uint16(builder.Handle): builder.Handle}
	if err := orders.RetailRestoreOrdersAtTick(builder, records, missing, orders.QueueForUnit(builder).Binding(), 101); err == nil || orders.QueueForUnit(builder).Head() != node {
		t.Fatal("missing saved target did not reject restore without replacing the live queue")
	}
	if err := orders.RetailRestoreOrdersAtTick(builder, records, stable, orders.QueueForUnit(builder).Binding(), 101); err != nil {
		t.Fatal(err)
	}
	q := orders.QueueForUnit(builder)
	s.RegisterOrderHandlers(q)
	node = q.Head()
	s.StepUnit(TickContext{Tick: 102}, builder.Handle)
	if node.Target != product.Handle || node.Phase != 2 || product.Remaining != 1 || sink.count != 0 {
		t.Fatal("restored wait changed its product or admitted work")
	}
	builder.InBuildStance = true
	builder.Pending |= units.PendingScriptTouched
	q.Pump(builder, 103)
	s.StepUnit(TickContext{Tick: 103}, builder.Handle)
	if node.Target != product.Handle || node.Phase != 3 || product.Remaining >= 1 || sink.count != 1 {
		t.Fatal("restored readiness event did not work on its existing frame")
	}
}

// Callback return alone cannot satisfy the level wait [04 R-ORD-01 §1].
func TestIssue73FinishedCallbackWithoutReadinessKeepsWaiting(t *testing.T) {
	s, builder, node, binding, sink := issue73BuildFixture(t, false)
	s.StepUnit(TickContext{Tick: 100}, builder.Handle)
	product := s.World.Unit(node.Target)
	if product == nil {
		t.Fatal("no frame")
	}
	// The authored callback returns without writing a port.
	binding.VM.Threads[0].PC = 11
	binding.VM.Drain(1)
	if binding.VM.Threads[0].Status != cob.ThreadIdle || builder.InBuildStance {
		t.Fatal("fixture did not finish its callback with readiness clear")
	}
	q := orders.QueueForUnit(builder)
	q.Pump(builder, 101)
	s.StepUnit(TickContext{Tick: 101}, builder.Handle)
	if product.Health != 0 || product.Remaining != 1 || node.Phase != 2 || node.DynamicGate != 0xE || sink.count != 0 {
		t.Fatal("callback return bypassed readiness")
	}
}

// Readiness holds remain interruptible. Removal unlinks the frame before slot
// reuse, and cancellation leaves it to ordinary GetBuilt decay rather than the
// factory refund/kill path [04 R-ORD-01 §5][04 R-ORD-01 §6].
func TestIssue73ReadinessWaitCancelAndProductRemoval(t *testing.T) {
	for _, ending := range []string{"cancel", "removed", "reused"} {
		t.Run(ending, func(t *testing.T) {
			s, builder, node, _, sink := issue73BuildFixture(t, false)
			s.StepUnit(TickContext{Tick: 100}, builder.Handle)
			product := s.World.Unit(node.Target)
			if product == nil {
				t.Fatal("no frame")
			}
			q := orders.QueueForUnit(builder)
			q.Binding().Work = &orders.WorkAdapter{CancelNotice: s.DeliverCancelNotice}
			initialBuckets := *s.Economy.UnitBuckets(builder.Handle)
			if ending == "cancel" {
				q.CancelAll()
			} else {
				if !s.NotifyProductRemoved(product.Handle) || node.Target != 0 {
					t.Fatal("removal did not unlink the readiness reference")
				}
				s.World.FreeImmediate(product.Handle)
				if ending == "reused" {
					h, err := s.World.Create(product.Def, builder.Owner, product.X, product.Y, product.Z)
					if err != nil || h != product.Handle {
						t.Fatalf("fixture did not reuse the removed slot: handle=%d err=%v", h, err)
					}
					product = s.World.Unit(h)
				}
				builder.InBuildStance = true
				builder.Pending |= units.PendingScriptTouched
				q.Pump(builder, 101)
			}
			beforeHealth, beforeRemaining := product.Health, product.Remaining
			s.StepUnit(TickContext{Tick: 101}, builder.Handle)
			if q.Head() == node || product.Health != beforeHealth || product.Remaining != beforeRemaining || product.Dying || sink.count != 0 || *s.Economy.UnitBuckets(builder.Handle) != initialBuckets || s.LastKill().Damage != 0 {
				t.Fatal("interrupted readiness admitted work, retained its record or ran factory cleanup")
			}
		})
	}
}
