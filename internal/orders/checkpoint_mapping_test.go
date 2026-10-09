package orders

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

func checkpointMappingContext(t *testing.T, source *WorldQueryAdapter, authority *checkpoint.BindingAuthority) *CheckpointContext {
	t.Helper()
	c := NewCheckpointContext(nil)
	if err := c.SetBindings(&QueueBinding{World: source}, nil, nil, authority); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCheckpointMappingWordReaderVectors(t *testing.T) {
	a := checkpoint.NewBindingAuthority()
	var calls [][2]int32
	reader := func(x, z int32) (uint16, bool) {
		calls = append(calls, [2]int32{x, z})
		return uint16(x), z < 0
	}
	source := NewWorldQueryAdapterWithCheckpointBinding(WorldQueryAdapterConfig{MappingWord: reader}, a)
	c := checkpointMappingContext(t, source, a)
	value := source.CheckpointMappingWord()
	if err := c.ValidateMappingWord(value); err != nil || len(calls) != 0 {
		t.Fatal("copy/validation invoked reader or refused its original installation", err)
	}
	for _, tc := range []struct {
		x, z int32
		word uint16
		ok   bool
	}{
		{-1, -1 << 31, 65535, true}, {1<<31 - 1, 0, 65535, false}, {-1 << 31, 1<<31 - 1, 0, false}, {0, -2, 0, true},
	} {
		word, ok := value.Reader()(tc.x, tc.z)
		if word != tc.word || ok != tc.ok || calls[len(calls)-1] != [2]int32{tc.x, tc.z} {
			t.Fatalf("reader changed authored operands/results: (%d,%d) = (%d,%v)", tc.x, tc.z, word, ok)
		}
	}
}

func TestCheckpointMappingWordSourceReplacementKeepsCopy(t *testing.T) {
	a := checkpoint.NewBindingAuthority()
	first := func(int32, int32) (uint16, bool) { return 0x1234, false }
	second := func(int32, int32) (uint16, bool) { return 0x8001, true }
	source := NewWorldQueryAdapterWithCheckpointBinding(WorldQueryAdapterConfig{MappingWord: first}, a)
	c := checkpointMappingContext(t, source, a)
	original := source.CheckpointMappingWord()
	copied := original
	source.SetMappingWordWithCheckpointBinding(second, a)
	replacement := source.CheckpointMappingWord()
	for _, value := range []CheckpointMappingWord{original, copied, replacement} {
		if err := c.ValidateMappingWord(value); err != nil {
			t.Fatal(err)
		}
	}
	if word, ok := copied.Reader()(3, 4); word != 0x1234 || ok {
		t.Fatal("copy followed source slot")
	}
	if word, ok := replacement.Reader()(3, 4); word != 0x8001 || !ok {
		t.Fatal("new getter did not copy current reader")
	}
	// The function-only getter and copied value's Reader both lose proof on an
	// ordinary reinstall, even when the actual function is unchanged.
	for _, getter := range []func() func(int32, int32) (uint16, bool){source.MappingWordHook, replacement.Reader} {
		source.SetMappingWord(getter())
		ordinary := source.CheckpointMappingWord()
		if ordinary.Reader() == nil || c.ValidateMappingWord(ordinary) == nil {
			t.Fatal("ordinary reinstall transferred proof")
		}
		if err := c.ValidateMappingWord(original); err != nil {
			t.Fatal("ordinary source edit invalidated independent copied value", err)
		}
		if c.ValidateBinding(c.bindings.binding) == nil {
			t.Fatal("copied-value validation admitted current ordinary source slot")
		}
	}
	source.SetMappingWord(nil)
	if source.CheckpointMappingWord().Reader() != nil || c.ValidateMappingWord(source.CheckpointMappingWord()) != nil || c.ValidateMappingWord(copied) != nil {
		t.Fatal("clearing source changed original copy or absent semantics")
	}
	// A separately attested source update cannot rewrite the old authority.
	foreign := checkpoint.NewBindingAuthority()
	source.SetMappingWordWithCheckpointBinding(second, foreign)
	foreignCopy := source.CheckpointMappingWord()
	if c.ValidateMappingWord(foreignCopy) == nil || c.ValidateMappingWord(original) != nil {
		t.Fatal("source replacement refreshed an earlier proof")
	}
	other := checkpointMappingContext(t, source, foreign)
	if other.ValidateMappingWord(foreignCopy) != nil || other.ValidateMappingWord(original) == nil {
		t.Fatal("copy authority not checked against each capture")
	}
}

func TestCheckpointMappingWordOrdinaryAndCopiedOwnersRefuse(t *testing.T) {
	a := checkpoint.NewBindingAuthority()
	reader := func(int32, int32) (uint16, bool) { return 7, true }
	source := NewWorldQueryAdapterWithCheckpointBinding(WorldQueryAdapterConfig{MappingWord: reader}, a)
	c := checkpointMappingContext(t, source, a)
	copySource := *source
	for _, ordinary := range []*WorldQueryAdapter{
		NewWorldQueryAdapter(WorldQueryAdapterConfig{MappingWord: reader}),
		NewWorldQueryAdapterWithCheckpointBinding(WorldQueryAdapterConfig{MappingWord: reader}, nil),
		&copySource,
	} {
		value := ordinary.CheckpointMappingWord()
		if value.Reader() == nil || c.ValidateMappingWord(value) == nil || value.proof.owner != nil || value.proof.authority != nil {
			t.Fatal("ordinary/copy owner transferred proof or lost actual reader")
		}
		if word, ok := value.Reader()(0, 0); word != 7 || !ok {
			t.Fatal("diagnostic refusal changed actual callback")
		}
	}
	original := source.CheckpointMappingWord()
	other := NewWorldQueryAdapterWithCheckpointBinding(WorldQueryAdapterConfig{MappingWord: reader}, a)
	if c.ValidateMappingWord(other.CheckpointMappingWord()) == nil || checkpointMappingContext(t, other, a).ValidateMappingWord(original) == nil {
		t.Fatal("same-authority foreign adapter accepted")
	}
	if NewCheckpointContext(nil).ValidateMappingWord(original) == nil || (*CheckpointContext)(nil).ValidateMappingWord(original) == nil || checkpointMappingContext(t, nil, a).ValidateMappingWord(original) == nil {
		t.Fatal("missing registered source accepted")
	}
	before := c.bindings
	if err := c.SetBindings(&QueueBinding{World: other}, nil, nil, a); err == nil || c.bindings != before || c.ValidateMappingWord(original) != nil {
		t.Fatal("conflicting registration changed original capture")
	}
}

func TestCheckpointMappingWordAbsenceAndStaleProof(t *testing.T) {
	c := NewCheckpointContext(nil)
	for _, value := range []CheckpointMappingWord{{}, (*WorldQueryAdapter)(nil).CheckpointMappingWord(), (&WorldQueryAdapter{}).CheckpointMappingWord()} {
		if value.Reader() != nil || c.ValidateMappingWord(value) != nil {
			t.Fatal("absence requires registration or acquires a reader")
		}
	}
	if (*CheckpointContext)(nil).ValidateMappingWord(CheckpointMappingWord{}) == nil {
		t.Fatal("nil context accepted")
	}
	a := checkpoint.NewBindingAuthority()
	source := NewWorldQueryAdapterWithCheckpointBinding(WorldQueryAdapterConfig{MappingWord: func(int32, int32) (uint16, bool) { panic("reader") }}, a)
	c = checkpointMappingContext(t, source, a)
	value := source.CheckpointMappingWord()
	value.reader = nil
	if c.ValidateMappingWord(value) == nil {
		t.Fatal("retained proof without its reader accepted")
	}
	// Private malformed source storage is not normalized into valid absence.
	source.mappingWord = nil
	if c.ValidateMappingWord(source.CheckpointMappingWord()) == nil {
		t.Fatal("getter hid stale proof without a callback")
	}
	for _, proof := range []checkpointCallbackProof[WorldQueryAdapter]{{owner: source}, {authority: a}} {
		if c.ValidateMappingWord(CheckpointMappingWord{proof: proof}) == nil {
			t.Fatal("partial stale proof accepted as absence")
		}
	}
}

func TestCheckpointMappingWordValidationIsPure(t *testing.T) {
	a := checkpoint.NewBindingAuthority()
	source := NewWorldQueryAdapterWithCheckpointBinding(WorldQueryAdapterConfig{MappingWord: func(int32, int32) (uint16, bool) { panic("validation invoked reader") }}, a)
	c := checkpointMappingContext(t, source, a)
	value := source.CheckpointMappingWord()
	before, proof := c.bindings, source.checkpointProofs.mappingWord
	if n := testing.AllocsPerRun(100, func() {
		if source.CheckpointMappingWord().Reader() == nil || c.ValidateMappingWord(value) != nil {
			t.Fatal("pure copy/validation changed admission")
		}
	}); n != 0 {
		t.Fatalf("copy/validation allocated %g times", n)
	}
	if c.bindings != before || source.checkpointProofs.mappingWord != proof || value.proof != proof {
		t.Fatal("validation changed installed or capture-local proof")
	}
}
