//go:build retail

package session

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// Nanolathe Modern policy: DESIGN_UNITS_ORDERS_COB "Modern guard assistance".
// A commander guarding a vehicle plant stays with the plant while it has
// production queued, the gaps between products included, even with a
// construction vehicle building solar collectors inside its sight radius. Once
// the plant's queue empties it helps that nearby work. The play-test report of
// 2026-09-25 was a commander leaving the plant in those gaps.
func TestModernGuardStaysWithAProducingFactoryRetail(t *testing.T) {
	sess := aiE2ESkirmishAtMode(t, "ashap plateau", aiE2ESeed, SkirmishDefaultDifficulty, gameplay.Modern)
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
	solarDef, ok2 := sess.Catalog.Unit(content.CanonicalKey("armsolar"))
	if !ok || !ok2 {
		t.Skip("armvp or armsolar is absent from the reference install")
	}
	ph, err := sess.Units.Create(plantDef, uint8(local), com.X+world.CellToWorld(12), 0, com.Z)
	if err != nil {
		t.Fatalf("create vehicle plant: %v", err)
	}
	sess.CompleteUnit(ph)
	plant := sess.Units.Unit(ph)
	cv := placeCompleteRetailUnit(t, sess, "armcv", uint8(local), plant.X, plant.Z+world.CellToWorld(14))
	sess.Movement.EnsureUnit(cv)
	if err := sess.EnqueueHumanCommand(HumanCommand{Kind: HumanFactoryBuild, FactoryBuild: HumanFactoryBuildCommand{Builder: ph, Product: "armflash", Count: 6}}); err != nil {
		t.Fatalf("queue production: %v", err)
	}
	// Which side of the plant a guard's way there ends on is the movement
	// policy's business, and nearby work is judged from where the guard
	// stands. The commander is therefore walked to the side the collectors
	// are on before it is told to guard.
	post := orders.ResolvePos{X: plant.X, Y: plant.Y, Z: plant.Z + world.CellToWorld(5), InterfaceType: orders.InterfaceTypeRightClick}
	if err := sess.EnqueueHumanCommand(HumanCommand{Kind: HumanOrder, Order: HumanOrderCommand{
		Handles: []pool.Handle{com.Handle}, Code: 2, AssignedPosition: true, Position: post,
	}}); err != nil {
		t.Fatalf("move order: %v", err)
	}
	for i := 0; i < 900; i++ {
		fund()
		step()
		if dx, dz := int64(com.X-post.X)>>16, int64(com.Z-post.Z)>>16; dx*dx+dz*dz <= 32*32 && orders.QueueForUnit(com).Head() == nil {
			break
		}
	}
	if dx, dz := int64(com.X-post.X)>>16, int64(com.Z-post.Z)>>16; dx*dx+dz*dz > 32*32 {
		t.Fatalf("fixture: the commander stands %d, %d from its post", dx, dz)
	}
	if err := sess.EnqueueHumanCommand(HumanCommand{Kind: HumanOrder, Order: HumanOrderCommand{
		Handles: []pool.Handle{com.Handle}, Code: 7, Target: ph,
		Position: orders.ResolvePos{X: plant.X, Y: plant.Y, Z: plant.Z, InterfaceType: orders.InterfaceTypeRightClick},
	}}); err != nil {
		t.Fatalf("guard order: %v", err)
	}
	// A row of collectors south of the plant, most of them within the
	// commander's sight radius of its post beside the plant. The six Flashes
	// keep the plant producing for longer than the first two collectors take.
	for i := int32(0); i < 6; i++ {
		x, z := plant.X+world.CellToWorld(6*i-12), plant.Z+world.CellToWorld(17)
		if err := sess.EnqueueHumanCommand(HumanCommand{Kind: HumanMobileBuild, ShiftHeld: i > 0, MobileBuild: HumanMobileBuildCommand{
			Builder: cv.Handle, Product: "armsolar", WX: x, WZ: z, WY: sess.World.HeightAt(x, z), Queued: i > 0,
		}}); err != nil {
			t.Fatalf("queue collector: %v", err)
		}
	}
	producing := func() bool {
		q := orders.QueueOfUnit(plant)
		if q == nil {
			return false
		}
		for _, rec := range q.Primary() {
			if rec != nil && orders.DescriptorFor(rec.ID).Name == "BuildingBuild" {
				return true
			}
		}
		return false
	}
	drained := -1
	for tick := 0; tick < 4000; tick++ {
		fund()
		step()
		busy := producing()
		if !busy && drained < 0 {
			drained = tick
		}
		head := orders.QueueForUnit(com).Head()
		if head == nil {
			continue
		}
		target := sess.Units.Unit(head.Target)
		if target == nil || target.Def != solarDef {
			continue
		}
		if busy {
			t.Fatalf("tick %d: the commander left a plant with production queued for %s on a solar collector", tick, orders.DescriptorFor(head.ID).Name)
		}
		t.Logf("production drained at tick %d; the commander took nearby work at tick %d", drained, tick)
		return
	}
	t.Fatalf("the commander never helped the nearby collectors after production drained at tick %d", drained)
}
