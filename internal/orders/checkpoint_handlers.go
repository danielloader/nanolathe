package orders

import (
	"fmt"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

const (
	CheckpointConstructionWake uint8 = 1
	CheckpointGetBuilt         uint8 = 2
	CheckpointAirStandby       uint8 = 3
)

// CheckpointHandlerSource names the exact producer owner and its installation
// authority. These are private diagnostic aliases, never wire data. It exposes
// no authority getter and invokes no owner method (§16.3.62).
type CheckpointHandlerSource struct {
	owner     any
	authority *checkpoint.BindingAuthority
}

// NewCheckpointHandlerSource captures a nonnil concrete owner pointer. Missing
// operands cannot prove any later copied handler (§16.3.62).
func NewCheckpointHandlerSource[T any](owner *T, authority *checkpoint.BindingAuthority) *CheckpointHandlerSource {
	if owner == nil || authority == nil {
		return nil
	}
	return &CheckpointHandlerSource{owner: owner, authority: authority}
}

// Only RegisterCheckpointHandlerSource constructs this matcher: it performs
// the concrete pointer assertion/equality and authority check below. There is
// no application-supplied predicate or interface-method invocation.
type checkpointHandlerRegistration struct {
	source  *CheckpointHandlerSource
	matches func() bool
}

// RegisterCheckpointHandlerSource records one expected source per closed kind.
// Exact repeats revalidate but never replace an earlier capture expectation;
// every refusal is atomic, including a source overwritten after registration.
func RegisterCheckpointHandlerSource[T any](c *CheckpointContext, kind uint8, source *CheckpointHandlerSource, expected *T, authority *checkpoint.BindingAuthority) error {
	if c == nil || !checkpointHandlerKind(kind) || source == nil || expected == nil || authority == nil {
		return bindingCheckpointError("orders.handlerSources", "a context, closed kind, source, concrete owner and authority")
	}
	actual, ok := source.owner.(*T)
	if !ok || actual != expected || !source.authority.Matches(authority) {
		return bindingCheckpointError("orders.handlerSources", "the source's exact concrete owner and authority")
	}
	if (c.bindings != nil && !c.bindings.authority.Matches(authority)) ||
		(c.handlerAuthority != nil && !c.handlerAuthority.Matches(authority)) {
		return bindingCheckpointError("orders.handlerSources", "one authority for binding and handler sources")
	}
	prior := c.handlerSources[kind-1]
	if prior.source != nil {
		if prior.source != source || !prior.matches() {
			return bindingCheckpointError("orders.handlerSources", "the same unchanged registered source")
		}
		return nil
	}
	c.handlerSources[kind-1] = checkpointHandlerRegistration{source: source, matches: func() bool {
		actual, ok := source.owner.(*T)
		return ok && actual == expected && source.authority.Matches(authority)
	}}
	c.handlerAuthority = authority
	return nil
}

func checkpointHandlerKind(kind uint8) bool {
	return kind >= CheckpointConstructionWake && kind <= CheckpointAirStandby
}

// CheckpointOwnedHandler carries one immutable function value with its kind
// and producer. It intentionally does not name a queue: ordinary inheritance
// transfers the same value to a new queue (§16.3.62).
type CheckpointOwnedHandler struct {
	handler OwnedHandler
	kind    uint8
	source  *CheckpointHandlerSource
}

// NewCheckpointOwnedHandler copies all three operands. Invalid diagnostic
// metadata never changes the handler's ordinary execution; capture refuses it.
func NewCheckpointOwnedHandler(handler OwnedHandler, kind uint8, source *CheckpointHandlerSource) CheckpointOwnedHandler {
	return CheckpointOwnedHandler{handler: handler, kind: kind, source: source}
}

// Handler returns the copied function alone, without transferring provenance.
func (h CheckpointOwnedHandler) Handler() OwnedHandler { return h.handler }

// SetOwnedHandlerWithCheckpointBinding performs ordinary installation once,
// then copies proof alongside the installed nonnil row. Nil handlers clear
// proof and preserve ordinary no-allocation behavior (§16.3.62).
func (q *Queue) SetOwnedHandlerWithCheckpointBinding(id ID, handler CheckpointOwnedHandler) {
	q.SetOwnedHandler(id, handler.handler)
	if q == nil || int(id) <= 0 || int(id) >= len(table) || handler.handler == nil {
		return
	}
	if q.checkpointOwnedHandlers == nil {
		q.checkpointOwnedHandlers = make([]CheckpointOwnedHandler, len(table))
	}
	q.checkpointOwnedHandlers[id] = handler
}

// The kind/row relation describes only the reviewed producers, not a second
// dispatcher. Table names are the existing immutable descriptor identities;
// neither handler dispatch nor a callback is used to recognize them (§16.3.62).
func checkpointHandlerRow(kind uint8, id ID) bool {
	if id == 0 || int(id) >= len(table) {
		return false
	}
	switch kind {
	case CheckpointConstructionWake:
		switch table[id].Name {
		case "BuildingBuild", "MobileBuild", "VTOL_MobileBuild", "ReclaimUnit", "VTOL_ReclaimUnit":
			return true
		}
	case CheckpointGetBuilt:
		return table[id].Name == "GetBuilt"
	case CheckpointAirStandby:
		return table[id].Name == "VTOL_Standby"
	}
	return false
}

func (c *CheckpointContext) validateCheckpointHandlerSources() error {
	for i, registration := range c.handlerSources {
		if registration.source != nil && (registration.matches == nil || !registration.matches()) {
			return bindingCheckpointError(fmt.Sprintf("orders.handlerSources[%d]", i+1), "the unchanged registered concrete owner and authority")
		}
	}
	return nil
}

// validateCheckpointHandlers checks physical actual rows against the copied
// values. It never compares functions. Private setter invalidation and the
// reviewed inheritance copy establish that each proof names the installed
// value; malformed shape or proof without its actual row refuses (§16.3.62).
func (q *Queue) validateCheckpointHandlers(c *CheckpointContext) (int, string, error) {
	if len(q.ownedHandlers) != 0 && len(q.ownedHandlers) != len(table) {
		return 0, "ownedHandlers", bindingCheckpointError("ownedHandlers", "the complete handler table")
	}
	if len(q.checkpointOwnedHandlers) != 0 && len(q.checkpointOwnedHandlers) != len(table) {
		return 0, "checkpointOwnedHandlers", bindingCheckpointError("checkpointOwnedHandlers", "the complete proof table")
	}
	count := 0
	for i := 0; i < max(len(q.ownedHandlers), len(q.checkpointOwnedHandlers)); i++ {
		var actual OwnedHandler
		var proof CheckpointOwnedHandler
		if i < len(q.ownedHandlers) {
			actual = q.ownedHandlers[i]
		}
		if i < len(q.checkpointOwnedHandlers) {
			proof = q.checkpointOwnedHandlers[i]
		}
		if actual == nil {
			if proof.handler != nil || proof.kind != 0 || proof.source != nil {
				return checkpointHandlerRowError(i, "no retained proof without an actual handler")
			}
			continue
		}
		if proof.handler == nil || proof.source == nil || !checkpointHandlerRow(proof.kind, ID(i)) {
			return checkpointHandlerRowError(i, "a copied handler value with the reviewed kind and row; TODO(M3-U6) for other handlers")
		}
		registration := c.handlerSources[proof.kind-1]
		if registration.source != proof.source || registration.matches == nil || !registration.matches() {
			return checkpointHandlerRowError(i, "the exact registered handler source")
		}
		count++
	}
	return count, "", nil
}

// The existing ownedHandlers field encodes absence 0, or presence 1 followed
// by u32 active count and numeric row order, each ID u8 then kind u8. Storage
// capacity, source/owner identity and function/proof values add no bytes.
func (q *Queue) writeCheckpointHandlers(e *checkpoint.Encoder, c *CheckpointContext, path string) {
	count, field, err := q.validateCheckpointHandlers(c)
	e.Field(path)
	if err != nil {
		e.Field(path + "." + field)
		e.Fail(err)
		return
	}
	e.Bool(count != 0)
	if count == 0 {
		return
	}
	e.Count(count)
	for i, handler := range q.ownedHandlers {
		if handler != nil {
			e.U8(uint8(i))
			e.U8(q.checkpointOwnedHandlers[i].kind)
		}
	}
}

func checkpointHandlerRowError(row int, expected string) (int, string, error) {
	path := fmt.Sprintf("ownedHandlers[%d]", row)
	return 0, path, bindingCheckpointError(path, expected)
}
