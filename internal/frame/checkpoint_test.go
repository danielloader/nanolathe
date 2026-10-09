package frame

import (
	"bytes"
	"encoding/hex"
	"reflect"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

func eventCheckpointBytes(t *testing.T, b *EventBuffer) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := b.WriteCheckpoint(checkpoint.NewEncoder(&out)); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// Independently authored vector fixes lexical limit order, source-width ints,
// identity exhaustion and the absence of event-window/diagnostic fields.
func TestCheckpointEventVectorAndMutations(t *testing.T) {
	b := &EventBuffer{limits: Limits{MaxEvents: 0x0102030405060708, MaxEffectEvents: -2}, exhausted: true, nextID: 0x89abcdef, nextSequence: 0x1020304050607080}
	want, err := hex.DecodeString("01feffffffffffffff0807060504030201efcdab898070605040302010")
	if err != nil {
		t.Fatal(err)
	}
	if got := eventCheckpointBytes(t, b); !bytes.Equal(got, want) {
		t.Fatalf("got %x; want %x", got, want)
	}
	for _, edit := range []func(*EventBuffer){
		func(b *EventBuffer) { b.exhausted = false },
		func(b *EventBuffer) { b.limits.MaxEvents++ },
		func(b *EventBuffer) { b.limits.MaxEffectEvents++ },
		func(b *EventBuffer) { b.nextID = 0 },
		func(b *EventBuffer) { b.nextSequence = 0 },
	} {
		changed := *b
		edit(&changed)
		if bytes.Equal(eventCheckpointBytes(t, &changed), want) {
			t.Fatal("retained mutation did not change payload")
		}
	}
	b.dropped, b.overflow = ^uint64(0), true
	backing := []Event{{ID: 17, Kind: KindExplosion, DurationsA: []int32{2, 3}}}
	b.events = backing[:0]
	before := *b
	if got := eventCheckpointBytes(t, b); !bytes.Equal(got, want) || !reflect.DeepEqual(before, *b) || backing[0].ID != 17 {
		t.Fatal("diagnostics/backing capacity changed bytes or capture mutated state")
	}
}

func TestCheckpointEventWindowRefusal(t *testing.T) {
	for _, test := range []struct {
		path string
		b    *EventBuffer
	}{
		{"frame.EventBuffer", nil},
		{"frame.EventBuffer.events", &EventBuffer{events: []Event{{Kind: KindStatus}}}},
		{"frame.EventBuffer.effects", &EventBuffer{effects: 1}},
		{"frame.EventBuffer.effects", &EventBuffer{effects: -1}},
	} {
		var out bytes.Buffer
		e := checkpoint.NewEncoder(&out)
		err := test.b.WriteCheckpoint(e)
		if err == nil || !strings.HasPrefix(err.Error(), "nanolathe:") || !strings.Contains(err.Error(), test.path) || out.Len() != 0 {
			t.Fatalf("refusal at %s = %v, %d bytes", test.path, err, out.Len())
		}
		e.U8(1)
		if out.Len() != 0 || e.Err() != err {
			t.Fatal("refusal did not leave a sticky encoder error")
		}
	}
}

func TestCheckpointEventSummarySelectionAndPurity(t *testing.T) {
	b := &EventBuffer{nextID: 3, nextSequence: 5, exhausted: true}
	var summary checkpoint.Summary
	summary.Word(7)
	if err := b.AppendCheckpointSummary(&summary); err != nil {
		t.Fatal(err)
	}
	if n, sum := summary.Result(); n != 4 || sum != 7+2*3+3*5+4 {
		t.Fatalf("summary = (%d,%d)", n, sum)
	}
	baseline, full := summary, eventCheckpointBytes(t, b)
	b.limits.MaxEvents = 99
	var changed checkpoint.Summary
	changed.Word(7)
	if err := b.AppendCheckpointSummary(&changed); err != nil || changed != baseline || bytes.Equal(full, eventCheckpointBytes(t, b)) {
		t.Fatal("limits must be full-only retained state")
	}
	// Summary neither validates nor traverses the unselected event window.
	b.events, b.effects, b.dropped, b.overflow = []Event{{DurationsA: []int32{1}}}, 1, 23, true
	before := *b
	var err error
	if allocations := testing.AllocsPerRun(50, func() { var s checkpoint.Summary; err = b.AppendCheckpointSummary(&s) }); allocations != 0 || err != nil || !reflect.DeepEqual(before, *b) {
		t.Fatalf("summary allocations/purity = %g, %v", allocations, err)
	}
	beforeSummary := summary
	if err := (*EventBuffer)(nil).AppendCheckpointSummary(&summary); err == nil || summary != beforeSummary {
		t.Fatal("nil buffer must fail atomically")
	}
	if err := b.AppendCheckpointSummary(nil); err == nil {
		t.Fatal("nil summary accepted")
	}
}
