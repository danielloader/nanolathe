package checkpoint

import (
	"math"
	"testing"
)

func TestReferencesPreserveEncounterOrderAndAliasing(t *testing.T) {
	type object struct{ value int }
	a, b, c := &object{1}, &object{1}, &object{2}
	var r References[*object]
	if got, ok := r.Find(nil); !ok || got != 0 {
		t.Fatalf("absent reference = %d, %t", got, ok)
	}
	if got, err := r.Add(nil); err != nil || got != 0 || len(r.Values()) != 0 {
		t.Fatalf("absent add = %d, %v, values %v", got, err, r.Values())
	}
	if got, ok := r.Find(a); ok || got != 0 || len(r.Values()) != 0 {
		t.Fatal("Find discovered a reference")
	}
	for i, value := range []*object{b, a, b, c, a} {
		want := []ObjectID{1, 2, 1, 3, 2}[i]
		if got, err := r.Add(value); err != nil || got != want {
			t.Fatalf("encounter %d: %d, %v, want %d", i, got, err, want)
		}
	}
	values := r.Values()
	if len(values) != 3 || values[0] != b || values[1] != a || values[2] != c {
		t.Fatalf("encounter order = %v", values)
	}
	values[0] = c
	values = append(values, a)
	if len(values) != 4 || values[0] != c || values[3] != a {
		t.Fatal("detached list did not retain its independent edits")
	}
	if len(r.Values()) != 3 || r.Values()[0] != b {
		t.Fatal("Values aliases the interner's slice")
	}
	a.value = 7
	if r.Values()[1].value != 7 {
		t.Fatal("Values cloned referenced objects")
	}
	if got, ok := r.Find(b); !ok || got != 1 {
		t.Fatalf("Find changed identity: %d, %t", got, ok)
	}
	var independent References[*object]
	if got, err := independent.Add(a); err != nil || got != 1 {
		t.Fatalf("IDs leaked between tables: %d, %v", got, err)
	}
}

func TestReferenceIDExhaustion(t *testing.T) {
	// Exercise the same checked count as Add without allocating a table with
	// billions of entries. The final representable ID is permitted, never zero.
	if id, err := nextObjectID(math.MaxUint32 - 1); err != nil || id != math.MaxUint32 {
		t.Fatalf("last ID = %d, %v", id, err)
	}
	for _, count := range []uint64{math.MaxUint32, math.MaxUint32 + 1, math.MaxUint64} {
		if id, err := nextObjectID(count); err == nil || id != 0 {
			t.Fatalf("exhausted count %d produced %d, %v", count, id, err)
		}
	}
}

func TestSummaryWeightedWordsAndModulo(t *testing.T) {
	var s Summary
	if words, sum := s.Result(); words != 0 || sum != 0 {
		t.Fatalf("zero summary = %d, %d", words, sum)
	}
	// The length/presence/scalar words are caller-selected; successive owner
	// fragments continue one numbering sequence (1*2 + 2*0 + 3*7 = 23).
	s.Word(2)
	s.Word(0)
	s.Word(7)
	if words, sum := s.Result(); words != 3 || sum != 23 {
		t.Fatalf("weighted summary = %d, %d", words, sum)
	}
	var wrap Summary
	wrap.Word(math.MaxUint64) // sign-extended -1
	wrap.Word(math.MaxUint64) // -1 + 2*(-1), modulo 2^64
	wrap.Word(2)              // add 6, wrapping the sum to 3
	if words, sum := wrap.Result(); words != 3 || sum != 3 {
		t.Fatalf("wrapped sum = %d, %d", words, sum)
	}
	wrap = Summary{words: math.MaxUint64 - 1, sum: math.MaxUint64}
	wrap.Word(2) // product wraps to -2, sum wraps to -3
	if words, sum := wrap.Result(); words != math.MaxUint64 || sum != math.MaxUint64-2 {
		t.Fatalf("wrapped product = %d, %d", words, sum)
	}
	wrap.Word(99) // word count wraps to zero; this contribution is zero
	if words, sum := wrap.Result(); words != 0 || sum != math.MaxUint64-2 {
		t.Fatalf("wrapped count = %d, %d", words, sum)
	}
}
