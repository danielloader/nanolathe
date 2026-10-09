package movement

import (
	"errors"
	"fmt"

	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/path"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// CollectCheckpointReferences visits System edges in lexical field order,
// then scans every object already registered in its tables. It never allocates
// runtime movement state or asks a rule/pilot/goal/search for behavior.
func (s *System) CollectCheckpointReferences(c *CheckpointContext) (added int, err error) {
	defer func() {
		if err != nil {
			err = movementCheckpointError("movement", err)
		}
	}()
	if err := s.checkpointContext(c); err != nil {
		return 0, err
	}
	if err := s.validateMovementCheckpoint(c); err != nil {
		return 0, movementCheckpointError("movement", err)
	}
	c.system = s
	addUnit := func(v *units.Unit) error {
		n, e := movementCheckpointAdd(&c.Orders.Units.Allocations, v)
		added += n
		return e
	}
	addOrder := func(v *orders.Node) error { n, e := movementCheckpointAdd(&c.Orders.Nodes, v); added += n; return e }
	addGoal := func(v path.Goal) error {
		if e := checkpointMovementGoal(v); e != nil {
			return e
		}
		n, e := movementCheckpointAdd(&c.Paths.Goals, v)
		added += n
		return e
	}
	addPayload := func(v GoalPayload) error {
		if e := checkpointMovementPayload(s, v); e != nil {
			return e
		}
		n, e := movementCheckpointAdd(&c.Payloads, v)
		added += n
		return e
	}
	addPilot := func(v any) error {
		if e := checkpointMovementPilot(v); e != nil {
			return e
		}
		n, e := movementCheckpointAdd(&c.pilots, v)
		added += n
		return e
	}
	addGround := func(v *moveGoal) error { n, e := movementCheckpointAdd(&c.moveGoals, v); added += n; return e }
	for _, v := range s.Flights {
		if v != nil {
			if x := v.Command; x != nil {
				if e := addPayload(x.Payload); e != nil {
					return added, e
				}
				if e := addUnit(x.Unit); e != nil {
					return added, e
				}
				if e := addOrder(x.payloadOwner); e != nil {
					return added, e
				}
			}
			if e := addUnit(v.Unit); e != nil {
				return added, e
			}
		}
	}
	if e := addPilot(s.PilotState); e != nil {
		return added, e
	}
	for _, v := range s.activeOrders {
		if v != nil {
			if e := addOrder(v.order); e != nil {
				return added, e
			}
		}
	}
	for _, v := range s.arrivalHandles {
		if v != nil {
			if e := addOrder(v.order); e != nil {
				return added, e
			}
			if e := addGoal(v.payload); e != nil {
				return added, e
			}
		}
	}
	for _, v := range s.clearanceRoutes {
		if v != nil {
			if e := addOrder(v.order); e != nil {
				return added, e
			}
		}
	}
	if r := s.layerRegistry; r != nil {
		for _, key := range checkpointLayerKeys(r) {
			n, e := movementCheckpointAdd(&c.Layers, r.byName[key])
			added += n
			if e != nil {
				return added, e
			}
		}
	}
	for _, v := range s.moveGoals {
		if e := addGround(v); e != nil {
			return added, e
		}
	}
	if p := s.pathProvider; p != nil {
		for _, row := range p.requests {
			for _, key := range checkpointRequestKeys(row) {
				if e := addGoal(row[key].Goal); e != nil {
					return added, e
				}
			}
		}
	}
	for _, v := range s.pockets {
		if e := addOrder(v.order); e != nil {
			return added, e
		}
	}
	for _, row := range s.recordGoals {
		for _, v := range row {
			if e := addPayload(v.air); e != nil {
				return added, e
			}
			if e := addGround(v.ground); e != nil {
				return added, e
			}
			if e := addOrder(v.node); e != nil {
				return added, e
			}
		}
	}
	for _, v := range s.repairLandings {
		if v != nil {
			if e := addOrder(v.node); e != nil {
				return added, e
			}
			if e := addUnit(v.pad); e != nil {
				return added, e
			}
			if e := addUnit(v.unit); e != nil {
				return added, e
			}
		}
	}
	for i, v := range s.sessions {
		if v != nil {
			if e := addGoal(v.goal); e != nil {
				return added, e
			}
			n, e := collectCheckpointAccessors(c, v.checkpoint)
			added += n
			if e != nil {
				return added, movementCheckpointError(fmt.Sprintf("movement.sessions[%d].accessors", i), e)
			}
			a, e := lowerCheckpointAccessors(c, v.checkpoint)
			if e != nil {
				return added, e
			}
			if v.session != nil {
				if e := c.Paths.SetAccessors(v.session, a); e != nil {
					return added, e
				}
			} else if len(a.Nodes) != 0 {
				return added, errors.New("accessors retained without a search")
			}
			n, e = movementCheckpointAdd(&c.Paths.Searches, v.session)
			added += n
			if e != nil {
				return added, e
			}
		}
	}
	for _, v := range s.unreachable {
		if e := addGoal(v.goal); e != nil {
			return added, e
		}
		if e := addOrder(v.order); e != nil {
			return added, e
		}
	}
	for i, v := range c.Layers.Values() {
		if e := validateMovementLayer(s, c, v); e != nil {
			return added, movementCheckpointError(fmt.Sprintf("movement.layers[%d]", i), e)
		}
	}
	for _, v := range c.pilots.Values() {
		n, e := collectMovementPilot(c, v)
		added += n
		if e != nil {
			return added, e
		}
	}
	for _, v := range c.claimRows.Values() {
		if e := validateCheckpointClaimRow(v); e != nil {
			return added, e
		}
	}
	for _, v := range c.Payloads.Values() {
		n, e := collectMovementPayload(c, s, v)
		added += n
		if e != nil {
			return added, e
		}
	}
	for _, v := range c.moveGoals.Values() {
		if e := addGoal(v.goal); e != nil {
			return added, e
		}
		if e := addOrder(v.order); e != nil {
			return added, e
		}
	}
	return added, nil
}
