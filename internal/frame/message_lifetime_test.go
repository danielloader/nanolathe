package frame

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/pool"
)

// Established: battle entry and F12 reset only cursors; the poster replaces
// message fields but keeps the slot's upper flags [07 R-HUD-03 §14.3].
func TestMessageClearPreservesRecordsAndAppendFlags(t *testing.T) {
	r := NewMessageRing()
	r.Configure(7, 3)
	r.Append("old battle", 1, 7, 10, 48000)
	r.Entries[0].Visited, r.Entries[0].Jumped = true, true
	old := r.Entries
	r.Clear()
	if r.Producer != 0 || r.Display != 0 || len(r.Visible()) != 0 {
		t.Fatal("clear retained a visible span")
	}
	if r.Entries != old || r.TextLines != 7 || r.TextScroll != 3 {
		t.Fatal("clear changed hidden records or settings")
	}
	if _, ok := r.NextUnvisitedSource(func(pool.Handle) bool {
		t.Fatal("empty ring inspected an old source handle")
		return true
	}); ok {
		t.Fatal("empty ring offered a source")
	}
	r.Append("new battle", 0xf2, 9, 2, 10)
	want := MessageLine{Text: "new battle", Class: 2, SourceUnit: 9, SpeakerSlot: 2, StoredTick: 10, Visited: true, Jumped: true}
	if got := r.Entries[0]; got != want {
		t.Fatalf("reused record = %+v, want %+v", got, want)
	}
}

// Retirement owns its age gate independently of whether new lines may be
// posted [01 R-PLAT-02 §8]. This fixture exercises the retained-span contract.
func TestRetainedMessagesAgeWhileLinePostingDisabled(t *testing.T) {
	r := NewMessageRing()
	r.Configure(10, 0)
	r.Append("first", 1, 0, 10, 0)
	r.Append("second", 1, 0, 10, 0)
	r.Configure(0, 0)
	if r.Append("blocked", 1, 0, 10, 31) {
		t.Fatal("zero line limit admitted a new line")
	}
	if r.RetireOne(30) {
		t.Fatal("retired at the strict deadline")
	}
	if !r.RetireOne(31) || r.Display != 1 {
		t.Fatal("disabled posting suspended or batch-retired the retained span")
	}
	r.Configure(10, 0)
	if got := r.Visible(); len(got) != 1 || got[0].Text != "second" {
		t.Fatalf("restored display = %#v", got)
	}
	if !r.RetireOne(31) || r.Display != r.Producer {
		t.Fatal("second pump did not retire the remaining line")
	}
}
