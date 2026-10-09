package units

import "github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"

const (
	checkpointStatusCue = iota
	checkpointYardTransaction
)

// A proof belongs to one exact Unit and private callback installation, including
// installation before allocation commits its serial (DESIGN_MULTIPLAYER §16.3.54).
// Allocation identity remains the collector's separate validation.
type checkpointCallbackProof struct {
	owner     *Unit
	authority *checkpoint.BindingAuthority
}

// SetYardOpenTransactionWithCheckpointBinding installs a reviewed canonical callback.
// The session keeps its authority private; capture checks exact world and unit.
func (u *Unit) SetYardOpenTransactionWithCheckpointBinding(transaction YardOpenTransaction, authority *checkpoint.BindingAuthority) {
	u.SetYardOpenTransaction(transaction)
	if u != nil && transaction != nil && authority != nil {
		u.checkpointCallbacks[checkpointYardTransaction] = checkpointCallbackProof{owner: u, authority: authority}
	}
}

// SetStatusCueSinkWithCheckpointBinding installs a reviewed canonical callback.
func (u *Unit) SetStatusCueSinkWithCheckpointBinding(sink StatusCueSink, authority *checkpoint.BindingAuthority) {
	u.SetStatusCueSink(sink)
	if u != nil && sink != nil && authority != nil {
		u.checkpointCallbacks[checkpointStatusCue] = checkpointCallbackProof{owner: u, authority: authority}
	}
}

func (w *World) validateCheckpointUnitCallbacks(c *CheckpointContext, u *Unit, path string) error {
	present := [2]bool{u.statusCue != nil, u.yardTransaction != nil}
	names := [2]string{"statusCue", "yardTransaction"}
	for slot, set := range present {
		proof := u.checkpointCallbacks[slot]
		if set && (c.lifecycleWorld != w || proof.owner != u || !proof.authority.Matches(c.lifecycleAuthority)) {
			return unitCheckpointError(path+"."+names[slot], "the exact unit's attested callback in the registered world")
		}
	}
	return nil
}
