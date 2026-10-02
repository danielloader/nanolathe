package session

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/combat"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

type parkedFiringPositionRules struct{ orders.ModernRules }

func (*parkedFiringPositionRules) StepFiringPosition(*units.Unit, uint32) bool { return false }

// Nanolathe Modern policy: DESIGN_UNITS_ORDERS_COB "Modern firing positions".
// The positions and pose are from our Great Divide diagnostic at tick 7967.
// The map's metal rock clips the Flash's initial muzzle-to-Weasel trajectory.
func TestModernCapturedFlashFindsFiringPosition(t *testing.T) {
	f := loadRetailFixture(t)
	f.cfg.MapName = "Great Divide"
	for _, tc := range []struct {
		name   string
		mode   gameplay.Mode
		parked bool
	}{
		{"modern", gameplay.Modern, false},
		{"blocked_without_reposition", gameplay.Modern, true},
		{"strict", gameplay.Strict31, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := f.session(t)
			s.SetGameplay(tc.mode)
			if tc.parked {
				rules := ModernRuleSet()
				rules.Orders = &parkedFiringPositionRules{}
				s.BindRules(rules)
			}
			stepRetail(s, 2)
			// Keep the map's starting commanders from joining this isolated pair.
			for player := 0; player < 2; player++ {
				s.Units.ForEachPlayerSliceLive(player, func(u *units.Unit) {
					u.Flags &^= units.StandingFieldMask << units.StandingFireShift
					for i := 0; i < combat.NumSlots; i++ {
						u.SlotAt(i).Flags &^= units.SlotFlagEnabled
					}
				})
			}
			for _, ai := range s.AI {
				if ai != nil {
					for i := range ai.Deadlines {
						ai.Deadlines[i] = ^uint32(0)
					}
				}
			}
			eligible, observers := visibilityModeRefreshInputs(s, 3)
			s.Vis.RefreshMode(3, true, eligible, observers)
			flash := placeCompleteRetailUnit(t, s, "ARMFLASH", 0, 31417389, 26149378)
			weasel := placeCompleteRetailUnit(t, s, "CORFAV", 1, 24095090, 19910973)
			for _, row := range []struct {
				u                    *units.Unit
				y                    numeric.Fixed
				heading, pitch, bank uint16
			}{
				{flash, 5537792, 8144, 326, 521}, {weasel, 5532480, 41181, 0, 386},
			} {
				u := row.u
				if !s.Movement.PlaceUnit(orders.PlaceRequest{Unit: u.Handle, X: u.X, Y: row.y, Z: u.Z}) {
					t.Fatal("place captured unit")
				}
				u.Move.Heading, u.Move.Pitch, u.Move.Bank = row.heading, row.pitch, row.bank
				s.Movement.Collisions[u.Handle].Heading = row.heading
				s.Movement.Steers[u.Handle].Heading = row.heading
				s.Movement.Steers[u.Handle].PendingHeading = row.heading
				s.bindOrderQueue(u)
			}
			flash.Flags = (flash.Flags &^ (units.StandingFieldMask<<units.StandingMoveShift | units.StandingFieldMask<<units.StandingFireShift)) | 2<<units.StandingMoveShift | 2<<units.StandingFireShift
			weasel.Flags &^= units.StandingFieldMask<<units.StandingMoveShift | units.StandingFieldMask<<units.StandingFireShift
			weasel.SlotAt(0).Flags &^= units.SlotFlagEnabled
			q := orders.QueueOfUnit(flash)
			q.Push(orders.Lookup("Attack_Chase"), orders.Node{Owner: flash.Handle, Target: weasel.Handle, Phase: 1, DynamicGate: 0x13808, Deadline: -1, GoalX: flash.X, GoalY: flash.Y, GoalZ: flash.Z})
			combat.ReleaseWeaponSlot(flash, 0)
			combat.SetManualWeaponTarget(flash, 0, weasel.Handle)
			slot := flash.SlotAt(0)
			slot.DesiredYaw, slot.DesiredPitch = 877, 65197
			slot.Aim.IssueBit, slot.Aim.Ready = true, true
			slot.Flags |= units.SlotFlagAimLatch
			x, z, health := flash.X, flash.Z, weasel.Health
			fired, moved := false, false
			for i := 0; i < 360; i++ {
				s.Step(s.Clock.ScaledAnchor + 1)
				dx, dz := (flash.X - x).Int(), (flash.Z - z).Int()
				moved = moved || dx*dx+dz*dz >= 8*8
				s.Combat.ForEachAliveInEntrySpan(func(_ pool.Handle, p *combat.Projectile) {
					if p.Shooter == flash.Handle {
						fired = true
					}
				})
				if tc.mode == gameplay.Modern && !tc.parked && fired && weasel.Health < health {
					break
				}
				if tc.mode == gameplay.Strict31 && fired {
					break
				}
			}
			t.Logf("moved=%v fired=%v damage=%d displacement=(%d,%d)", moved, fired, health-weasel.Health, (flash.X - x).Int(), (flash.Z - z).Int())
			if tc.parked {
				if fired || moved {
					t.Fatal("blocked control did not remain parked")
				}
				return
			}
			if !fired {
				t.Fatal("Flash never fired")
			}
			if tc.mode == gameplay.Modern && (!moved || weasel.Health >= health) {
				t.Fatal("Flash did not reposition and hit the Weasel")
			}
			if tc.mode == gameplay.Strict31 && moved {
				t.Fatal("Strict moved before its original admitted shot")
			}
		})
	}
}
