package movement

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// The authored fixture has the stock paths' admission flags, without loading
// assets: armed, can-fly, a non-dropped primary, and hostile targets.
func bindAirInputFixture(t *testing.T, sys *System, u *units.Unit) *orders.Queue {
	t.Helper()
	u.Def.CanAttack = true
	u.Def.Weapon1Def = &content.WeaponDef{ID: 1, Range: 510}
	u.InstallWeapon(0, u.Def.Weapon1Def)
	u.Flags |= units.ArmedStatus
	q := orders.QueueForUnit(u)
	q.Binding().World = &orders.WorldQueryAdapter{
		Hostile:  func(a, b *units.Unit) bool { return a.Owner != b.Owner },
		SeaLevel: func() uint8 { return 0 },
	}
	sys.BindAirOrderLegs()
	sys.SetMoverMode(u, 2)
	return q
}

// A queued hover attack must read the target after the preceding move, without
// overwriting its stored click. The strafing control still refreshes its cache
// in the ordinary order entry [04 R-AIR-01 §8].
func TestQueuedAirApproachReadsMovingTarget(t *testing.T) {
	for _, hover := range []bool{true, false} {
		name := "AirToGround"
		if hover {
			name = "AirToGroundHover"
		}
		t.Run(name, func(t *testing.T) {
			sys, w, u := wideAirFixture(t)
			q := bindAirInputFixture(t, sys, u)
			u.Def.HoverAttack = hover
			target := airTargetFor(t, sys, w, 16, 40)
			old := Vec3{X: target.X, Y: target.Y, Z: target.Z}
			pos := orders.ResolvePos{X: old.X, Y: old.Y, Z: old.Z}
			id := orders.Resolve(3, u, target, &pos)
			if orders.DescriptorFor(id).Name != name {
				t.Fatalf("attack resolved to %s, want %s", orders.DescriptorFor(id).Name, name)
			}
			move := orders.Lookup("VTOL_Move")
			q.Push(move, orders.NewNodeForOrder(move, 0, u.X, u.Y, u.Z, 1, u.Handle, false))
			q.Push(id, orders.NewNodeForOrder(id, target.Handle, old.X, old.Y, old.Z, 1, u.Handle, true))
			attack := q.Primary()[1]
			q.Pump(u, 1)
			if q.Head() == attack {
				t.Fatal("attack did not wait behind the move")
			}
			// The previous target movement commit is an input to this executor;
			// neither target rebinding nor an order-goal rewrite accompanies it.
			target.X += 200 << 16
			sim := q.Binding().SimRNG
			wantRNG := *sim
			var want Vec3
			for tick := uint32(2); tick <= 120; tick++ {
				runMovementTick(sys, tick-1, w)
				wantRNG = *sim
				jitter := uint16(wantRNG.Uint32n(0x4000)) - 0x2000
				h := bearing(u.X, u.Z, target.X, target.Z) + jitter
				d := airPlanarDistance(target.X, target.Z, u.X, u.Z)
				ox, oz := offsetAtBearing(h, numeric.Fixed(d/2))
				want = Vec3{X: u.X - ox, Y: u.Y, Z: u.Z - oz}
				q.Pump(u, tick)
				if q.Head() == attack && attack.Phase == 2 {
					break
				}
			}
			if q.Head() != attack || attack.Phase != 2 {
				t.Fatalf("queued attack did not reach approach: head=%v phase=%d", q.Head(), attack.Phase)
			}
			m := installedMarker(t, sys, u)
			if m.goal != want || m.radius != 128 || attack.DynamicGate != airLegGateStrike {
				t.Fatalf("approach goal=%v radius=%d gate=%x, want %v/128/%x", m.goal, m.radius, attack.DynamicGate, want, airLegGateStrike)
			}
			wantCache := old
			if !hover {
				wantCache = Vec3{X: target.X, Y: target.Y, Z: target.Z}
			}
			if (Vec3{X: attack.GoalX, Y: attack.GoalY, Z: attack.GoalZ}) != wantCache {
				t.Fatalf("stored goal changed incorrectly, want %v", wantCache)
			}
			if *sim != wantRNG {
				t.Fatal("approach must consume exactly its one bounded jitter draw")
			}
		})
	}
}

// The first dogfight deadline reaches phase 1 through the production pump.
// An ordinary point marker supplies arrival where requested; no pending bits
// or phase values are injected [04 R-AIR-01 §1, §8].
func TestDogfightWholeComponentsControlPursuit(t *testing.T) {
	for _, arrival := range []bool{false, true} {
		for _, ahead := range []bool{false, true} {
			name := "side"
			if ahead {
				name = "ahead"
			}
			if arrival {
				name += "_arrival"
			}
			t.Run(name, func(t *testing.T) {
				sys, w, u := wideAirFixture(t)
				q := bindAirInputFixture(t, sys, u)
				target := airTargetFor(t, sys, w, 12, 16)
				target.Def.CanFly = true
				target.X, target.Z = u.X-(100<<16), u.Z+(2<<16)
				if ahead {
					target.X, target.Z = u.X, u.Z+(100<<16)
				}
				fl := handleRow(sys.Flights, u.Handle)
				u.Move.Heading, fl.Heading, fl.TargetHeading = 32768, 32768, 32768
				pos := orders.ResolvePos{X: target.X, Y: target.Y, Z: target.Z}
				id := orders.Resolve(3, u, target, &pos)
				if orders.DescriptorFor(id).Name != "AirToAir" {
					t.Fatal("finite aircraft target did not resolve to dogfight")
				}
				q.Push(id, orders.NewNodeForOrder(id, target.Handle, target.X, target.Y, target.Z, 1, u.Handle, false))
				n := q.Head()
				q.Pump(u, 1)
				if n.Phase != 1 || n.Deadline != 2 {
					t.Fatalf("dogfight entry phase=%d deadline=%d", n.Phase, n.Deadline)
				}
				old := sys.newPointMarker(u, Vec3{X: u.X, Y: u.Y, Z: u.Z})
				sys.installAirGoal(u, n, old)
				// Model the installed movement leg and its ordinary wake gate.
				// The command producer below supplies the actual arrival bits.
				n.DynamicGate |= airLegGate
				if arrival {
					runMovementTick(sys, 1, w)
				}
				sim := q.Binding().SimRNG
				wantRNG := *sim
				q.Pump(u, 2)
				switch {
				case !arrival:
					wantCounter := uint32(45)
					if ahead {
						wantCounter = 0
					}
					if q.Head() != n || n.Param1 != wantCounter || n.Deadline != 47 || fl.Command.Payload != old {
						t.Fatalf("pursuit counter=%d deadline=%d payload=%T", n.Param1, n.Deadline, fl.Command.Payload)
					}
				case ahead:
					m, ok := fl.Command.Payload.(*airVelocityMarker)
					deadline := int32(62 + wantRNG.Uint32n(30))
					if !ok || q.Head() != n || n.Param1 != 0 || n.Deadline != deadline {
						t.Fatalf("straight flight payload=%T counter=%d deadline=%d", fl.Command.Payload, n.Param1, n.Deadline)
					}
					if m.pos.X != u.X || m.pos.Z != u.Z+numeric.Fixed(u.Def.MaxVelocity)*30 || m.vel.Z != numeric.Fixed(u.Def.MaxVelocity) {
						t.Fatalf("straight flight position=%v velocity=%v", m.pos, m.vel)
					}
				default:
					// The ordinary evasion constructor clears its target because
					// that descriptor has no target-observer bit. Its entry then
					// completes without drawing, and dogfight restarts. This locks
					// the composed producer path, not a synthetic retained target.
					if q.Head() != n || n.Phase != 1 || n.Param1 != 0 || n.Deadline != 3 || fl.Command.Payload != nil {
						t.Fatalf("sideways arrival did not release and restart: head=%+v payload=%T", q.Head(), fl.Command.Payload)
					}
				}
				if *sim != wantRNG {
					t.Fatal("dogfight changed bounded RNG consumption")
				}
			})
		}
	}
}
