package cob

import "github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"

// AppendCheckpointSummary retains the selected physical thread and static
// words, including inactive stack residuals. It neither examines instructions
// nor invokes bindings (DESIGN_MULTIPLAYER §16.3.77).
func (v *VM) AppendCheckpointSummary(out *checkpoint.Summary) error {
	if v == nil || out == nil {
		return checkpointPortError("VM.summary", "a present VM and summary")
	}
	next := *out
	for i := range v.Threads {
		t := &v.Threads[i]
		next.Word(uint64(int64(t.Status)))
		next.Word(uint64(int64(t.PC)))
		next.Word(uint64(int64(t.SP)))
		next.Word(uint64(int64(t.Sleep)))
		next.Word(uint64(int64(t.WaitPiece)))
		next.Word(uint64(int64(t.WaitAxis)))
		next.Word(uint64(int64(t.WaitThread)))
		next.Word(uint64(int64(t.SignalMask)))
		for _, word := range t.Stack {
			next.Word(uint64(int64(word)))
		}
	}
	next.Word(uint64(len(v.statics)))
	for _, word := range v.statics {
		next.Word(uint64(int64(word)))
	}
	next.Word(uint64(v.activeThreadCount))
	next.Word(v.nextIdentity)
	for _, identity := range v.threadIdentity {
		next.Word(identity)
	}
	*out = next
	return nil
}

// AppendCheckpointSummary preserves both projections and the raw restored aim
// word; ReadyWord would synthesize a different value (§16.3.77).
func (s *AimSlot) AppendCheckpointSummary(out *checkpoint.Summary) error {
	if s == nil || out == nil {
		return checkpointPortError("AimSlot.summary", "a present aim slot and summary")
	}
	next := *out
	if s.IssueBit {
		next.Word(1)
	} else {
		next.Word(0)
	}
	if s.Ready {
		next.Word(1)
	} else {
		next.Word(0)
	}
	next.Word(uint64(s.readyWord))
	*out = next
	return nil
}
