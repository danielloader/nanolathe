package orders

import (
	"errors"
	"fmt"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// AppendCheckpointSummary retains selected nodes in their physical queue order.
// Repeated references remain repeated; a nil row is malformed. Detached pump
// state and bindings are full-only blind spots (DESIGN_MULTIPLAYER §16.3.77).
func (q *Queue) AppendCheckpointSummary(out *checkpoint.Summary) error {
	if q == nil || out == nil {
		return orderCheckpointError("orders.Queue.summary", errors.New("a present queue and summary required"))
	}
	next := *out
	for segment, nodes := range [2][]*Node{q.primary, q.secondary} {
		next.Word(uint64(len(nodes)))
		for i, n := range nodes {
			if n == nil {
				name := "primary"
				if segment == 1 {
					name = "secondary"
				}
				return orderCheckpointError(fmt.Sprintf("orders.Queue.summary.%s[%d]", name, i), errors.New("a nonnil retained node required"))
			}
			next.Word(uint64(n.ID))
			next.Word(uint64(n.Phase))
			next.Word(uint64(n.Target))
			next.Word(uint64(int64(n.GoalX)))
			next.Word(uint64(int64(n.GoalY)))
			next.Word(uint64(int64(n.GoalZ)))
			next.Word(uint64(int64(n.Deadline)))
			next.Word(uint64(n.Param1))
			next.Word(uint64(n.Param2))
			next.Word(uint64(n.Param3))
			next.Word(uint64(n.Flags))
		}
	}
	next.Word(uint64(q.lastPumpTick))
	*out = next
	return nil
}
