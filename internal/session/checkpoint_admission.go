package session

import (
	"github.com/nanolathe-gg/nanolathe/internal/community"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/mission"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// sessionCheckpointAdmission exists only for a successfully admitted entry.
// It keeps constructor provenance separate from the mutable owner graph that
// full capture must still verify (DESIGN_MULTIPLAYER §16.3.40). The authority
// is available to canonical composition before any callback installation, but
// no owner is admitted merely because this receipt exists.
type sessionCheckpointAdmission struct {
	identity      checkpoint.Identity
	rules         checkpointRuleContext
	owner         *Session
	inputs        *content.SimulationInputs
	config        EffectiveMatchConfig
	mission       *mission.Mission
	missionInputs *mission.CheckpointInputs
	terrainKey    string
	terrain       *world.Terrain
	authority     *checkpoint.BindingAuthority
	ready         bool
	ticked        bool // sticky: a wrapped runtime tick is not unticked entry
}

func newSessionCheckpointAdmission(inputs *content.SimulationInputs, config EffectiveMatchConfig, m *mission.Mission) (*sessionCheckpointAdmission, error) {
	snapshot, err := mission.SnapshotCheckpointInputs(m)
	if err != nil {
		return nil, err
	}
	defaults, err := community.Table(community.Mainline)
	if err != nil {
		return nil, err
	}
	return &sessionCheckpointAdmission{identity: checkpoint.Identity{Content: inputs.Digest(), Config: config.Digest()}, rules: checkpointRuleContext{entryMode: gameplay.Mode(config.request.RuleName), entryCommunity: config.request.Community, defaultCommunity: defaults}, inputs: inputs, config: config, mission: m, missionInputs: snapshot, terrainKey: m.TerrainKey, authority: checkpoint.NewBindingAuthority()}, nil
}

// validateCheckpointInputs compares entry-owned materialized inputs, without
// adopting a replacement or consulting a loader (DESIGN_MULTIPLAYER §16.3.74).
func (s *Session) validateCheckpointInputs() error {
	if s == nil || s.checkpointAdmission == nil || s.checkpointAdmission.owner != s {
		return runtimeCheckpointError("admission", "the original admitted session")
	}
	a := s.checkpointAdmission
	if s.Mission != a.mission || s.World == nil || s.World != a.terrain {
		return runtimeCheckpointError("admission.inputs", "the original mission and loaded terrain")
	}
	if err := a.missionInputs.Validate(s.Mission); err != nil {
		return err
	}
	return s.World.ValidateCheckpointInputs(a.inputs, a.terrainKey)
}

// checkpointBindingAuthority is used only by reviewed composition sites.
// Receipt completion and every actual owner edge are checked at capture.
func (s *Session) checkpointBindingAuthority() *checkpoint.BindingAuthority {
	if s == nil || s.checkpointAdmission == nil || s.checkpointAdmission.owner != s {
		return nil
	}
	return s.checkpointAdmission.authority
}
