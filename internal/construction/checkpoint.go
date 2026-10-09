package construction

import (
	"fmt"
	"slices"

	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// CheckpointContext borrows admitted definition keys and the singleton terrain;
// construction adds no object tables (DESIGN_MULTIPLAYER §16.3.15).
type CheckpointContext struct {
	Orders       *orders.CheckpointContext
	World        *world.CheckpointContext
	bindings     *checkpointConstructionBindings
	presentation *checkpointConstructionPresentationMatch
}

func NewCheckpointContext(o *orders.CheckpointContext, w *world.CheckpointContext) *CheckpointContext {
	return &CheckpointContext{Orders: o, World: w}
}

// CollectCheckpointReferences validates the retained edges without registering
// objects, rebuilding placements or consulting any production binding.
func (s *Service) CollectCheckpointReferences(c *CheckpointContext) (int, error) {
	if err := s.checkpointBoundary(c); err != nil {
		return 0, err
	}
	for _, product := range checkpointHandleKeys(s.placements) {
		if _, err := checkpointPlacementDefinition(c, s.placements[product], product); err != nil {
			return 0, err
		}
	}
	return 0, nil
}

// WriteCheckpoint writes only the construction payload. Retained fields in
// lexical order: Allocator, CRTRandom, Catalog, Combat, Community, Economy,
// IsSpecialSecondState, LimitChecker, ModeSelector, ModelForFactory,
// ModelForUnit, Movement, OrderBinding, Presentation, Rules, Terrain, World,
// builderLinks, kickRecords, placements, repairBanks, repairWorld. Binding
// bytes retain absence zero or verified presence one. Terrain is
// presence with checked identity; private callback storage keeps these schema names.
// Maps have numeric product order, and every stored bank/kick row is retained,
// even an unused bank or an invalid kick. Raw handles use the explicit u32
// schema width (DESIGN_MULTIPLAYER §16.3.5–§16.3.6, §16.3.15). Capture does
// not implement a save format or regenerate creation yards.
func (s *Service) WriteCheckpoint(e *checkpoint.Encoder, c *CheckpointContext) error {
	e.Field("construction.Service")
	if err := s.checkpointBoundary(c); err != nil {
		e.Fail(err)
		return e.Err()
	}
	for _, binding := range [...]struct {
		field   string
		present bool
	}{
		{"Allocator", s.Allocator != nil}, {"CRTRandom", s.crtRandom != nil},
		{"Catalog", s.Catalog != nil}, {"Combat", s.Combat != nil},
	} {
		e.Field("construction.Service." + binding.field)
		e.Bool(binding.present)
	}
	if err := s.Community.WriteCheckpoint(e); err != nil {
		return err
	}
	for _, binding := range [...]struct {
		field   string
		present bool
	}{
		{"Economy", s.Economy != nil}, {"IsSpecialSecondState", s.isSpecialSecondState != nil}, {"LimitChecker", s.LimitChecker != nil},
	} {
		e.Field("construction.Service." + binding.field)
		e.Bool(binding.present)
	}
	e.Field("construction.Service.ModeSelector")
	e.I64(int64(s.ModeSelector))
	for _, binding := range [...]struct {
		field   string
		present bool
	}{
		{"ModelForFactory", s.modelForFactory != nil}, {"ModelForUnit", s.modelForUnit != nil},
		{"Movement", s.Movement != nil}, {"OrderBinding", s.OrderBinding != nil}, {"Presentation", s.Presentation != nil},
	} {
		e.Field("construction.Service." + binding.field)
		e.Bool(binding.present)
	}
	e.Field("construction.Service.Rules")
	rulesKind, err := CheckpointRulesKind(s.Rules)
	e.Fail(err)
	e.U8(rulesKind)
	e.Field("construction.Service.Terrain")
	e.Bool(s.Terrain != nil)
	e.Field("construction.Service.World")
	e.Bool(s.World != nil)
	e.Field("construction.Service.builderLinks")
	e.Count(len(s.builderLinks))
	for _, product := range checkpointHandleKeys(s.builderLinks) {
		e.Field(fmt.Sprintf("construction.Service.builderLinks[%d]", product))
		e.U32(uint32(product))
		e.U32(uint32(s.builderLinks[product]))
	}
	e.Field("construction.Service.kickRecords")
	e.Count(len(s.kickRecords))
	for slot, row := range s.kickRecords {
		path := fmt.Sprintf("construction.Service.kickRecords[%d]", slot)
		// Source-field lexical order is valid, x, y, z, including invalid XYZ.
		e.Field(path + ".valid")
		e.Bool(row.valid)
		e.Field(path + ".x")
		e.I64(int64(row.x))
		e.Field(path + ".y")
		e.I64(int64(row.y))
		e.Field(path + ".z")
		e.I64(int64(row.z))
	}
	e.Field("construction.Service.placements")
	e.Count(len(s.placements))
	for _, product := range checkpointHandleKeys(s.placements) {
		row := s.placements[product]
		path := fmt.Sprintf("construction.Service.placements[%d]", product)
		e.Field(path)
		e.U32(uint32(product))
		// placementRecord fields are def, rect, yard. The yard is the retained
		// creation-oriented sequence, not today's rules or rotation cache.
		e.Field(path + ".def")
		e.Bool(row.def != nil)
		if row.def != nil {
			ref, err := checkpointPlacementDefinition(c, row, product)
			if err != nil {
				e.Fail(err)
				return e.Err()
			}
			e.Definition(ref)
		}
		e.Field(path + ".rect")
		if err := row.rect.WriteCheckpoint(e); err != nil {
			return err
		}
		e.Field(path + ".yard")
		e.Count(len(row.yard))
		for _, cell := range row.yard {
			e.U8(uint8(cell))
		}
	}
	e.Field("construction.Service.repairBanks")
	e.Count(len(s.repairBanks))
	for builder, pair := range s.repairBanks {
		for slot, bank := range pair {
			path := fmt.Sprintf("construction.Service.repairBanks[%d][%d]", builder, slot)
			// Both fixed slots, including target-zero residuals; handles retain
			// weak slot identity across reuse (community-patch-engine CP-DMG-4).
			e.Field(path + ".remainder")
			e.I32(bank.remainder)
			e.Field(path + ".target")
			e.U32(uint32(bank.target))
		}
	}
	e.Field("construction.Service.repairWorld")
	e.Bool(s.repairWorld != nil)
	return e.Err()
}

// checkpointHandleKeys only gathers keys before sorting; callers read map
// values in that numeric order. No map iteration chooses a byte or first error.
func checkpointHandleKeys[V any](table map[pool.Handle]V) []pool.Handle {
	keys := make([]pool.Handle, 0, len(table))
	for key := range table {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func checkpointPlacementDefinition(c *CheckpointContext, row placementRecord, product pool.Handle) (checkpoint.Definition, error) {
	if row.def == nil {
		return checkpoint.Definition{}, nil
	}
	ref, err := c.Orders.Units.Keys.Unit(row.def)
	if err != nil {
		return checkpoint.Definition{}, fmt.Errorf("%w: %w", constructionCheckpointError(fmt.Sprintf("construction.Service.placements[%d].def", product), "an admitted unit definition"), err)
	}
	return ref, nil
}

func (s *Service) checkpointBoundary(c *CheckpointContext) error {
	if s == nil {
		return constructionCheckpointError("construction.Service", "a present service")
	}
	if c == nil || c.Orders == nil || c.Orders.Units == nil || c.Orders.Units.Keys == nil || c.World == nil {
		return constructionCheckpointError("construction.context", "orders with admitted unit keys and a world context")
	}
	for _, active := range []struct {
		field string
		set   bool
	}{
		{"completedInPump", s.completedInPump != 0},
		{"reclaimStepNode", s.reclaimStepNode != nil},
		{"vtolBuildStepOwner", s.vtolBuildStepOwner != nil},
	} {
		if active.set {
			return constructionCheckpointError("construction.Service."+active.field, "a completed construction pump")
		}
	}
	if s.Terrain != c.World.Terrain {
		return constructionCheckpointError("construction.Service.Terrain", "the collected terrain identity")
	}
	if _, err := CheckpointRulesKind(s.Rules); err != nil {
		return err
	}
	return s.validateCheckpointBindings(c)
}

// AppendCheckpointSummary reads just bank/kick rows and the two map lengths:
// bank count, each pair's target/remainder; kick count, each x/y/z/valid; then
// placement and builder-link counts (DESIGN_MULTIPLAYER §16.3.7, §16.3.15).
// It never collects keys, resolves bindings or reads map values. All signed
// fields are sign-extended; fixed slot pairs have no count. The successful
// path allocates nothing.
func (s *Service) AppendCheckpointSummary(summary *checkpoint.Summary) error {
	if s == nil {
		return constructionCheckpointError("construction.Service", "a present service")
	}
	if summary == nil {
		return constructionCheckpointError("construction.summary", "a summary accumulator")
	}
	next := *summary
	next.Word(uint64(len(s.repairBanks)))
	for _, pair := range s.repairBanks {
		for _, bank := range pair {
			next.Word(uint64(bank.target))
			next.Word(uint64(int64(bank.remainder)))
		}
	}
	next.Word(uint64(len(s.kickRecords)))
	for _, row := range s.kickRecords {
		next.Word(uint64(row.x))
		next.Word(uint64(row.y))
		next.Word(uint64(row.z))
		var valid uint64
		if row.valid {
			valid = 1
		}
		next.Word(valid)
	}
	next.Word(uint64(len(s.placements)))
	next.Word(uint64(len(s.builderLinks)))
	*summary = next
	return nil
}

func constructionCheckpointError(path, expected string) error {
	return fmt.Errorf("nanolathe: checkpoint capture failed: logical path %s, providers searched [construction], expected %s", path, expected)
}
