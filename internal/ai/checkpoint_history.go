package ai

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// ApplicationHistory is the simulation-thread receipt chain from
// DESIGN_MULTIPLAYER §16.3.7 and §16.3.22. It observes applications; failure
// disables diagnostic reporting, never command execution. A nil history is
// disabled, so ordinary single-player producers need no receipt buffers.
type ApplicationHistory struct {
	player, kind uint8
	nextSerial   uint64
	count        uint64
	digest       checkpoint.Digest
	active       *ApplicationAttempt
	err          error
}

// ApplicationHistoryState is a detached diagnostic value, not a resume image.
// The Modern host supplies its own simulation-thread scheduling fields.
type ApplicationHistoryState struct {
	Enabled           bool
	Player, Kind      uint8
	NextSerial, Count uint64
	Hash              checkpoint.Digest
}

// NewApplicationHistory binds a fresh chain to the admitted battle and player.
// Kind is the history vocabulary: Classic 1, Modern 2, not Controller's enum.
func NewApplicationHistory(identity checkpoint.Identity, player, kind uint8) (*ApplicationHistory, error) {
	if player >= 10 || kind < 1 || kind > 2 {
		return nil, aiCheckpointError("ai.history.identity", "a player in 0..9 and controller kind 1 or 2")
	}
	h := &ApplicationHistory{player: player, kind: kind, nextSerial: 1}
	sum := sha256.New()
	_, _ = sum.Write([]byte("NLCPAIST"))
	e := checkpoint.NewEncoder(sum)
	e.U16(checkpoint.SchemaVersion)
	_, _ = sum.Write(identity.Content[:])
	_, _ = sum.Write(identity.Config[:])
	e.U8(player)
	e.U8(kind)
	copy(h.digest[:], sum.Sum(nil))
	return h, nil
}

// NextSerial assigns an identity when the simulation thread starts a decision
// or schedules a batch, including an empty batch. Zero means disabled/failed.
// Exhaustion refuses reporting instead of wrapping an identity (§16.3.7).
func (h *ApplicationHistory) NextSerial() uint64 {
	if h == nil || h.err != nil {
		return 0
	}
	if h.active != nil || h.nextSerial == math.MaxUint64 {
		h.Fail(errors.New("serial exhausted or application still active"))
		return 0
	}
	n := h.nextSerial
	h.nextSerial++
	return n
}

// BeginAttempt writes only the typed intent/observed operands supplied by the
// controller's reviewed codec. That writer runs synchronously, before execution,
// and must neither invoke producers nor retain the encoder. The common header
// is player, kind, tick, assigned serial and zero-based command ordinal.
func (h *ApplicationHistory) BeginAttempt(tick uint32, serial uint64, ordinal uint32, writeIntent func(*checkpoint.Encoder) error) *ApplicationAttempt {
	if h == nil || h.err != nil {
		return nil
	}
	if h.active != nil || serial == 0 || serial >= h.nextSerial || writeIntent == nil {
		h.Fail(errors.New("overlapping attempt, unassigned serial or missing typed intent"))
		return nil
	}
	a := &ApplicationAttempt{history: h}
	h.active = a
	e := checkpoint.NewEncoder(&a.header)
	e.U8(h.player)
	e.U8(h.kind)
	e.U32(tick)
	e.U64(serial)
	e.U32(ordinal)
	e.Fail(writeIntent(e))
	if err := e.Err(); err != nil {
		h.Fail(err)
	}
	return a
}

// ApplicationAttempt owns only the current receipt buffers. Finish releases
// them; a history retains no command, node, allocation or worker references.
// All methods accept nil so disabled diagnostics do not call payload writers.
type ApplicationAttempt struct {
	history             *ApplicationHistory
	header, operations  bytes.Buffer
	operationCount      uint32
	committedOperations uint32
	issuedOrders        uint32
	coalescedOrders     uint32
	lastInsertion       checkpointInsertion
	finished            bool
}

// Operation appends one actual operation using the owning codec's exact
// operands (§16.3.7, §16.3.21). Tags are the ten published operation kinds.
// The callback consumes borrowed values now; it must not change the world.
func (a *ApplicationAttempt) Operation(kind uint16, write func(*checkpoint.Encoder) error) {
	if a == nil {
		return
	}
	h := a.history
	if a.finished || h.active != a {
		h.Fail(errors.New("operation outside its active attempt"))
		return
	}
	if h.err != nil {
		return
	}
	if kind < 1 || kind > 10 || write == nil || a.operationCount == math.MaxUint32 {
		h.Fail(errors.New("unknown operation, missing operands or operation count exhausted"))
		return
	}
	e := checkpoint.NewEncoder(&a.operations)
	e.U16(kind)
	e.Fail(write(e))
	if err := e.Err(); err != nil {
		h.Fail(err)
		return
	}
	a.operationCount++
	if kind != 4 && kind != 5 {
		a.committedOperations++
	}
	if kind == 3 || kind == 10 {
		a.issuedOrders++
	}
	if kind == 10 {
		a.coalescedOrders++
	}
}

// Fail makes the first diagnostic error sticky. Gameplay callers continue
// their existing path; neither history nor receipt errors are gameplay gates.
func (h *ApplicationHistory) Fail(err error) {
	if h != nil && h.err == nil && err != nil {
		h.err = aiCheckpointError("ai.history", err.Error())
	}
}

// Finish hashes a complete attempt, including rejected/no-op attempts. APM is
// unlimited 1, debited 2, rejected 3; terminal is accepted-no-op 1, success 2,
// rejected 3, partial 4. A rejected APM attempt cannot contain operations.
func (a *ApplicationAttempt) Finish(apm, terminal uint8) {
	if a == nil {
		return
	}
	h := a.history
	if a.finished || h.active != a {
		h.Fail(errors.New("attempt finished outside its active scope"))
		return
	}
	a.finished = true
	h.active = nil
	defer func() {
		a.header = bytes.Buffer{}
		a.operations = bytes.Buffer{}
		a.lastInsertion = checkpointInsertion{}
	}()
	if h.err != nil {
		return
	}
	if apm < 1 || apm > 3 || (h.kind == 1 && apm != 1) || terminal < 1 || terminal > 4 || (apm == 3 && (a.operationCount != 0 || terminal != 3)) {
		h.Fail(errors.New("unclassified APM or terminal outcome"))
		return
	}
	if h.count == math.MaxUint64 {
		h.Fail(errors.New("application count exhausted"))
		return
	}
	sum := sha256.New()
	_, _ = sum.Write([]byte("NLCPAIAP"))
	e := checkpoint.NewEncoder(sum)
	e.U16(checkpoint.SchemaVersion)
	_, _ = sum.Write(h.digest[:])
	_, _ = sum.Write(a.header.Bytes())
	e.U8(apm)
	e.U32(a.operationCount)
	_, _ = sum.Write(a.operations.Bytes())
	e.U8(terminal)
	copy(h.digest[:], sum.Sum(nil))
	h.count++
}

// Snapshot refuses in-flight applications and sticky failures without
// sampling a worker or changing the chain. A nil history is explicitly absent.
func (h *ApplicationHistory) Snapshot() (ApplicationHistoryState, error) {
	if h == nil {
		return ApplicationHistoryState{}, nil
	}
	if h.err != nil {
		return ApplicationHistoryState{}, h.err
	}
	if h.active != nil {
		return ApplicationHistoryState{}, aiCheckpointError("ai.history.active", "a completed application boundary")
	}
	return ApplicationHistoryState{Enabled: true, Player: h.player, Kind: h.kind, NextSerial: h.nextSerial, Count: h.count, Hash: h.digest}, nil
}

// WriteCheckpoint writes presence, count, fixed hash, kind, next serial and
// player. It encodes no current-attempt buffers or diagnostic error strings.
func (h *ApplicationHistory) WriteCheckpoint(e *checkpoint.Encoder) error {
	if e == nil {
		return aiCheckpointError("ai.history.encoder", "a checkpoint encoder")
	}
	s, err := h.Snapshot()
	if err != nil {
		e.Fail(err)
		return e.Err()
	}
	e.Field("ai.history")
	e.Bool(s.Enabled)
	if s.Enabled {
		e.U64(s.Count)
		for _, b := range s.Hash {
			e.U8(b)
		}
		e.U8(s.Kind)
		e.U64(s.NextSerial)
		e.U8(s.Player)
	}
	return e.Err()
}

// AppendCheckpointSummary appends the five published history words only;
// the owning controller fragment separately includes scheduling metadata.
func (h *ApplicationHistory) AppendCheckpointSummary(s *checkpoint.Summary) error {
	if s == nil {
		return aiCheckpointError("ai.history.summary", "a summary accumulator")
	}
	v, err := h.Snapshot()
	if err != nil {
		return err
	}
	if !v.Enabled {
		return aiCheckpointError("ai.history.summary", "an enabled application history")
	}
	next := *s
	next.Word(v.Count)
	for i := 0; i < len(v.Hash); i += 8 {
		next.Word(binary.LittleEndian.Uint64(v.Hash[i : i+8]))
	}
	*s = next
	return nil
}
