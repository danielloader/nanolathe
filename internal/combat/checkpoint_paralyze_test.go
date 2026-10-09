package combat

import (
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"testing"
)

func TestCheckpointParalyzeOriginalInstallation(t *testing.T) {
	prior, receipt := paralyzeTaskPush, checkpointParalyzeTaskInstallation
	t.Cleanup(func() { paralyzeTaskPush, checkpointParalyzeTaskInstallation = prior, receipt })
	calls := 0
	victim := &units.Unit{}
	original := SetParalyzeTaskPush(func(u *units.Unit, credit, tick uint32) {
		if u != victim || credit != 0xffffffff || tick != 17 {
			t.Fatal("callback operands changed")
		}
		calls++
	})
	if err := ValidateCheckpointParalyzeTaskInstallation(original); err != nil {
		t.Fatal(err)
	}
	copy := *original
	if err := ValidateCheckpointParalyzeTaskInstallation(&copy); err == nil {
		t.Fatal("copied receipt admitted")
	}
	if calls != 0 {
		t.Fatal("validation invoked task")
	}
	pushParalyzeTask(victim, 0xffffffff, 17)
	if calls != 1 {
		t.Fatal("ordinary invocation changed")
	}
	replacement := SetParalyzeTaskPush(ParalyzeTaskPushHook())
	if replacement == original {
		t.Fatal("replacement reused receipt")
	}
	if err := ValidateCheckpointParalyzeTaskInstallation(original); err == nil {
		t.Fatal("same function replacement retained proof")
	}
	if err := ValidateCheckpointParalyzeTaskInstallation(replacement); err != nil {
		t.Fatal(err)
	}
	if got := SetParalyzeTaskPush(nil); got != nil || ParalyzeTaskPushHook() != nil {
		t.Fatal("nil installation retained storage")
	}
	if err := ValidateCheckpointParalyzeTaskInstallation(replacement); err == nil {
		t.Fatal("cleared installation accepted")
	}
	pushParalyzeTask(victim, 0xffffffff, 17)
	if calls != 1 {
		t.Fatal("nil installation invoked task")
	}
}
