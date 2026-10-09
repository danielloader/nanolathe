package ai

import (
	"reflect"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// Cleanup may retire or replace an observed allocation. Diagnostic receipts
// retain that allocation while gameplay still reads the subsequent raw handles
// (DESIGN_MULTIPLAYER §§16.3.27,16.3.29).
func TestClassicCleanupIdentityMutationKeepsObservedActor(t *testing.T) {
	for _, mobile := range []bool{false, true} {
		for _, drop := range []bool{false, true} {
			name := "order"
			if mobile {
				name = "mobile"
			}
			name += "/purge"
			if drop {
				name += "-survivor-drop"
			}
			t.Run(name, func(t *testing.T) {
				type result struct {
					nodes                    []orders.Node
					request                  BuildRequest
					cancels, calls, prepares int
					actor                    checkpoint.Allocation
					state                    uint32
					draws                    uint64
				}
				var baseline result
				for mode := 0; mode < 3; mode++ {
					m, w, u, _, random, _ := constructionOrderGateFixture(t)
					m.Factory = u
					product, _ := m.Catalog.Unit("gate-product")
					h := classicTestHistory(t, m, mode, product)
					actor, _ := CheckpointAllocation(u)
					original := &units.Unit{Handle: u.Handle, AllocationSerial: u.AllocationSerial}
					target := &units.Unit{Handle: 11, AllocationSerial: 13}
					q := orders.QueueForUnit(u)
					previous := &orders.Node{ID: orders.Lookup("MobileBuild"), Owner: u.Handle, DynamicGate: 2}
					if drop {
						previous.Flags = orders.FlagPurgeSurvivor | orders.FlagAutoOp
					}
					q.SetPrimary([]*orders.Node{previous})
					prior := &producerObserverCount{}
					q.SetCheckpointObserver(prior)
					cancels, calls, prepares := 0, 0, 0
					m.OrderBinding.SetLookup(w.Unit)
					m.OrderBinding.SetBuildList(func(*content.UnitDef) bool { return true })
					m.OrderBinding.Rules = classicTestRules{before: func() { prepares++; random.Uint32n(17) }}
					m.OrderBinding.Work = orders.NewWorkAdapter(orders.WorkAdapterConfig{CancelNotice: func(*units.Unit, *orders.Node, uint32) bool {
						cancels++
						u.Handle += 100
						u.AllocationSerial += 1000
						target.Handle += 200
						random.Uint32n(7)
						return true
					}})
					var request BuildRequest
					m.SetQueueBuildTyped(func(req BuildRequest) error {
						calls++
						request = req
						q.CoalesceTail(orders.Lookup("MobileBuild"), orders.NewNodeForOrder(orders.Lookup("MobileBuild"), 0, req.X, 0, req.Z, req.Tick, req.Builder, false))
						return nil
					})
					res := PlacementResult{Valid: true, WorldX: 1 << 40, WorldZ: -3}
					id := orders.Lookup("Follow_Ground")
					if mobile {
						m.issueMobileBuild(u, "gate-product", res, 91)
					} else {
						m.submitResolvedOrder(u, 7, id, target, res.WorldX, -2, res.WorldZ, 91, 0, 160)
					}
					if q.SetCheckpointObserver(nil) != prior {
						t.Fatal("submission did not restore its queue observer")
					}
					if cancels != 1 || prepares != 1 || q.LenPrimary() != 1 || q.Head().Owner != u.Handle {
						t.Fatal("cleanup fixture did not insert through the changed raw handle")
					}
					if mobile {
						if calls != 1 || request.Builder != u.Handle || request.Tick != 0 {
							t.Fatalf("actual typed request changed: %+v", request)
						}
					} else if calls != 0 || q.Head().Target != target.Handle {
						t.Fatal("order stopped reading its gameplay target after cleanup")
					}
					current, _ := CheckpointAllocation(u)
					got := result{classicQueueNodes(u), request, cancels, calls, prepares, current, random.State, random.Draws()}
					if mode == 0 {
						baseline = got
					} else if !reflect.DeepEqual(got, baseline) {
						t.Fatalf("mode %d changed cleanup or gameplay arguments", mode)
					}
					if mode != 1 {
						continue
					}
					intent := ClassicApplicationIntent{Kind: 1, Code: 7, ResolvedRow: uint8(id), Argument: 160,
						X: res.WorldX, Y: -2, Z: res.WorldZ, RawTarget: 11,
						Operands: ApplicationOperands{Target: &checkpoint.Allocation{Handle: 11, Serial: 13}}}
					var productRef *checkpoint.Definition
					if mobile {
						key, err := m.CheckpointApplicationKey(product)
						if err != nil {
							t.Fatal(err)
						}
						productRef = &key
						intent = ClassicApplicationIntent{Kind: 2, UnitKey: "gate-product", X: res.WorldX, Z: res.WorldZ, Count: 1}
					}
					want, a := classicExpectedAttempt(t, m.Player, 91, intent, original, productRef)
					a.RecordOrder(actor, orders.CheckpointOrderReceipt{Kind: 1})
					a.RecordOrder(actor, orders.CheckpointOrderReceipt{Kind: 2})
					expectedClassicInsert(a, actor, *q.Head(), 0)
					if mobile {
						a.RecordBuild(actor, request, nil)
					}
					a.Finish(1, 2)
					assertClassicHistory(t, h, want)
				}
			})
		}
	}
}
