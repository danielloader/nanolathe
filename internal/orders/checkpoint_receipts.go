package orders

import (
	"errors"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// CheckpointOrderReceipt is one synchronous application observation
// (DESIGN_MULTIPLAYER §16.3.21). Kind is purge 1, auto-drop 2, insertion 3,
// coalescence 4, or primary preparation 5. Segment is primary 1 or rear 2;
// Index is zero-based. Preparation is firing-position 1 or rules 2.
// Node is a value copy whose payload slices are borrowed read-only until the
// observer returns. Actor allocation identity belongs to the caller.
type CheckpointOrderReceipt struct {
	Kind                 uint8
	Segment              uint8
	Index                int64
	Node                 Node
	PreviousCount, Added uint32
	Preparation          uint8
}

// CheckpointOrderObserver consumes a receipt immediately. It must not mutate
// gameplay or invoke producers; diagnostic failure remains with the observer.
type CheckpointOrderObserver interface {
	RecordCheckpointOrder(CheckpointOrderReceipt)
}

// CheckpointInsertionResult describes one completed public insertion call
// (DESIGN_MULTIPLAYER §16.3.29). Method is Push 1 or CoalesceTail 2; Row is
// the supplied descriptor. Inserted and Coalesced are mutually exclusive;
// both false means this invocation was refused, regardless of nested work.
// This transient result is not an additional mutation receipt or wire record.
type CheckpointInsertionResult struct {
	Method              uint8
	Row                 ID
	Inserted, Coalesced bool
}

// CheckpointInsertionObserver is an optional addition to CheckpointOrderObserver.
// The observer captured at invocation entry receives completion after all nested
// callbacks return, including on refusal, but never during panic unwinding.
// Like mutation observation, it must not change gameplay or invoke producers.
type CheckpointInsertionObserver interface {
	RecordCheckpointInsertion(CheckpointInsertionResult)
}

// SetCheckpointObserver replaces the current simulation-thread observer and
// returns it for scoped restoration. Nested scopes replace, never fan out.
// An active observer makes full checkpoint capture unsupported.
func (q *Queue) SetCheckpointObserver(observer CheckpointOrderObserver) CheckpointOrderObserver {
	if q == nil {
		return nil
	}
	previous := q.checkpointObserver
	q.checkpointObserver = observer
	return previous
}

// WriteCheckpointValue writes the retained node values without graph IDs.
// It shares the full graph writer's insertion-input validation and excludes
// RetailSubtype restore/save staging (DESIGN_MULTIPLAYER §16.3.6, §16.3.21).
func (n *Node) WriteCheckpointValue(e *checkpoint.Encoder) error {
	e.Field("orders.node")
	if n == nil {
		e.Fail(errors.New("absent order node"))
		return e.Err()
	}
	if field, err := validateCheckpointNode(n); err != nil {
		e.Field("orders.node." + field)
		e.Fail(err)
		return e.Err()
	}
	writeNodeCheckpoint(e, n, "orders.node")
	return e.Err()
}
