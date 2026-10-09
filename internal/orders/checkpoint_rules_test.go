package orders

import (
	"bytes"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// The embedded nil interface panics if any gameplay method is called. The
// slice also prevents safely comparing this custom implementation by value.
type checkpointUnknownRules struct {
	Rules
	_ []int
}

func TestCheckpointRulesKind(t *testing.T) {
	for _, tc := range []struct {
		name  string
		rules Rules
		want  uint8
	}{
		{"nil", nil, 0},
		{"strict value", StrictRules{}, 1},
		{"strict pointer", &StrictRules{}, 1},
		{"community value", CommunityRules{}, 2},
		{"community pointer", &CommunityRules{}, 2},
		{"modern pointer", &ModernRules{}, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CheckpointRulesKind(tc.rules)
			if err != nil || got != tc.want {
				t.Fatalf("kind = %d, %v; want %d", got, err, tc.want)
			}
		})
	}
	for _, rules := range []Rules{
		(*StrictRules)(nil), (*CommunityRules)(nil), (*ModernRules)(nil),
		checkpointUnknownRules{}, &checkpointUnknownRules{}, (*checkpointUnknownRules)(nil),
		struct{ StrictRules }{},
	} {
		if got, err := CheckpointRulesKind(rules); err == nil || got != 0 {
			t.Fatalf("unreviewed %T = %d, %v", rules, got, err)
		}
	}
}

func TestCheckpointRulesDoNotAdmitQueueBindings(t *testing.T) {
	for _, rules := range []Rules{nil, StrictRules{}, CommunityRules{}, &ModernRules{}} {
		q := &Queue{binding: &QueueBinding{Rules: rules}}
		c := orderCheckpointContext(t, &units.Unit{Orders: q})
		p := &Pump{}
		if _, err := p.CollectCheckpointReferences(c); err == nil {
			t.Fatalf("collector admitted binding for %T", rules)
		}
		// Discover the queue explicitly to exercise the writer's independent
		// validation after collection has refused its production binding.
		if _, err := c.Queues.Add(q); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := p.WriteCheckpoint(checkpoint.NewEncoder(&out), c); err == nil {
			t.Fatalf("writer admitted binding for %T", rules)
		}
	}
}
