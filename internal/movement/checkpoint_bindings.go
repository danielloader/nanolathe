package movement

import (
	"errors"

	"github.com/nanolathe-gg/nanolathe/internal/path"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// NewSystemWithCheckpointBindings preserves NewSystem's construction order and
// records provenance for its scheduler/eligibility, terrain and air grid
// (DESIGN_MULTIPLAYER §16.3.33, §16.3.57, §16.3.61). Session keeps the authority private.
func NewSystemWithCheckpointBindings(terrain *world.Terrain, fallback Profile, grid *OccupancyGrid, authority *checkpoint.BindingAuthority) *System {
	return newSystem(terrain, fallback, grid, authority)
}

// ConfigurePathWithCheckpointBinding runs the ordinary guarded installation,
// then attests only its eligibility and scheduler-provider slots. It cannot
// bless an independently replaced search or publisher (§16.3.28–§16.3.33).
func (s *System) ConfigurePathWithCheckpointBinding(players int, unitLimit int32, eligible func(int) bool, authority *checkpoint.BindingAuthority) {
	s.configurePath(players, unitLimit, eligible, authority)
}

// These capture-local fields attest exact aliases; none is wire data. The
// existing System/world/provider presence fields represent those edges. The
// eligibility field is absent 0 or verified-present 1, while path owns the
// scheduler's three binding tags. Other movement bindings remain unsupported.
type checkpointPathBindings struct {
	system    *System
	world     *units.World
	scheduler *path.Scheduler
	provider  *pathProvider
	authority *checkpoint.BindingAuthority
}

// SetPathBindings records expected owners without changing or polling them.
// Nil world is explicit absence and must match both live world edges. All
// checks precede context registration, which is idempotent for the exact tuple.
func (c *CheckpointContext) SetPathBindings(s *System, w *units.World, authority *checkpoint.BindingAuthority) error {
	if c == nil || c.Paths == nil || s == nil || s.Scheduler == nil || s.pathProvider == nil || authority == nil {
		return movementCheckpointError("movement.pathBindings", errors.New("missing context, system, scheduler, provider or authority"))
	}
	binding := checkpointPathBindings{s, w, s.Scheduler, s.pathProvider, authority}
	if c.system != nil && c.system != s || c.pathBindings.authority != nil && c.pathBindings != binding {
		return movementCheckpointError("movement.pathBindings", errors.New("conflicting path binding registration"))
	}
	if c.auxiliaryBindings.authority != nil && (c.auxiliaryBindings.system != s || c.auxiliaryBindings.world != w || !c.auxiliaryBindings.authority.Matches(authority)) {
		return movementCheckpointError("movement.pathBindings", errors.New("auxiliary world or authority differs"))
	}
	if c.compositionBindings.authority != nil && !c.compositionBindings.authority.Matches(authority) {
		return movementCheckpointError("movement.pathBindings", errors.New("composition authority differs"))
	}
	if err := binding.validate(s); err != nil {
		return movementCheckpointError("movement.pathBindings", err)
	}
	if err := c.Paths.SetSchedulerBindings(s.Scheduler, authority); err != nil {
		return err
	}
	c.system, c.pathBindings = s, binding
	return nil
}

func (b checkpointPathBindings) validate(s *System) error {
	if b.authority == nil || s == nil || s != b.system || s.Scheduler != b.scheduler || s.pathProvider != b.provider || s.world != b.world || b.provider == nil || b.provider.system != s || b.provider.world != b.world {
		return errors.New("path binding owner alias differs")
	}
	if !path.CheckpointProviderMatches(s.Scheduler, b.provider, b.authority) {
		return errors.New("unattested scheduler provider binding")
	}
	if b.provider.eligible != nil && !b.provider.checkpointEligibilityAuthority.Matches(b.authority) {
		return errors.New("unattested path eligibility binding")
	}
	return nil
}
