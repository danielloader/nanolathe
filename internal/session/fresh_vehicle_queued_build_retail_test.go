//go:build retail

package session

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/construction"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// A construction vehicle given a Shift-queued build on the tick it is finished
// runs its Park walk off the plant first, then walks to the site, and places
// the nanoframe within reach [04 §3.3][04 R-FAC-02 §4]. The play-test report
// of 2026-09-25: the build ran behind the vehicle's Park record, stamped the
// frame from the pad and built it from about 600 world units away. The rule is
// shared, so both modes are checked.
func TestFreshVehicleWalksToItsQueuedBuildRetail(t *testing.T) {
	for _, mode := range []gameplay.Mode{gameplay.Modern, gameplay.Strict31} {
		t.Run(string(mode), func(t *testing.T) {
			sess := aiE2ESkirmishAtMode(t, "ashap plateau", aiE2ESeed, SkirmishDefaultDifficulty, mode)
			driver := int32(1 << 20)
			step := func() { sess.Step(driver); driver++ }
			for i := 0; i < 60 && sess.State != StateBattle; i++ {
				step()
			}
			local := sess.LocalOwner
			var com *units.Unit
			for _, u := range sess.Units.IterSliced() {
				if u != nil && u.Alive && u.Def != nil && u.Def.Commander && u.Owner == local {
					com = u
				}
			}
			if com == nil {
				t.Fatal("no local commander")
			}
			fund := func() {
				p := &sess.Econ.Players[local]
				p.Capacity = [2]float32{100000, 100000}
				p.Stock = [2]float32{100000, 100000}
			}
			plantDef, ok := sess.Catalog.Unit(content.CanonicalKey("armvp"))
			vehicleDef, ok2 := sess.Catalog.Unit(content.CanonicalKey("armcv"))
			solarDef, ok3 := sess.Catalog.Unit(content.CanonicalKey("armsolar"))
			if !ok || !ok2 || !ok3 {
				t.Skip("armvp, armcv or armsolar is absent from the reference install")
			}
			ph, err := sess.Units.Create(plantDef, uint8(local), com.X+world.CellToWorld(12), 0, com.Z)
			if err != nil {
				t.Fatalf("create vehicle plant: %v", err)
			}
			sess.CompleteUnit(ph)
			plant := sess.Units.Unit(ph)
			if err := sess.EnqueueHumanCommand(HumanCommand{Kind: HumanFactoryBuild, FactoryBuild: HumanFactoryBuildCommand{Builder: ph, Product: "armcv", Count: 1}}); err != nil {
				t.Fatalf("queue production: %v", err)
			}
			var vehicle *units.Unit
			for tick := 0; tick < 3000 && vehicle == nil; tick++ {
				fund()
				step()
				for _, u := range sess.Units.IterSliced() {
					if u != nil && u.Alive && u.Def == vehicleDef && u.Owner == local && u.Remaining <= 0 && u.Flags&construction.FlagCompleted != 0 {
						vehicle = u
					}
				}
			}
			if vehicle == nil {
				t.Fatalf("the plant never finished its construction vehicle: %v", sess.Build.Messages())
			}
			// Far outside the vehicle's 40-unit build distance, on open ground.
			x, z := plant.X-world.CellToWorld(10), plant.Z+world.CellToWorld(36)
			if err := sess.EnqueueHumanCommand(HumanCommand{Kind: HumanMobileBuild, ShiftHeld: true, MobileBuild: HumanMobileBuildCommand{
				Builder: vehicle.Handle, Product: "armsolar", WX: x, WZ: z, WY: sess.World.HeightAt(x, z), Queued: true,
			}}); err != nil {
				t.Fatalf("queue build: %v", err)
			}
			for tick := 0; tick < 1500; tick++ {
				fund()
				step()
				for _, u := range sess.Units.IterSliced() {
					if u == nil || !u.Alive || u.Def != solarDef || u.Owner != local {
						continue
					}
					head := orders.QueueForUnit(vehicle).Head()
					if head == nil || orders.DescriptorFor(head.ID).Name != "MobileBuild" || head.Target != u.Handle {
						t.Fatalf("a nanoframe exists that is not the vehicle's front build record's product")
					}
					if sess.Build.OutOfReachPublic(vehicle, head) {
						dx, dz := int64(u.X-vehicle.X)>>16, int64(u.Z-vehicle.Z)>>16
						t.Fatalf("the nanoframe was placed out of reach, %d,%d world units from the vehicle [05 R-WORK-01 §2]", dx, dz)
					}
					return
				}
			}
			t.Fatalf("the vehicle never placed its queued build: %v", sess.Build.Messages())
		})
	}
}
