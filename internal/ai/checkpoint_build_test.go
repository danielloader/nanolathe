package ai

import (
	"errors"
	"fmt"
	"testing"
)

func TestCheckpointBuildVerdictPreservesProducerError(t *testing.T) {
	cause := errors.New("unchanged producer diagnostic")
	for verdict := CheckpointBuildProduct; verdict <= CheckpointBuildOther; verdict++ {
		tagged := WithCheckpointBuildVerdict(cause, verdict)
		if tagged.Error() != cause.Error() || !errors.Is(tagged, cause) {
			t.Fatal("producer error changed")
		}
		got, ok := CheckpointBuildVerdict(fmt.Errorf("caller: %w", tagged))
		if !ok || got != verdict {
			t.Fatal("wrapped verdict lost", got, ok)
		}
	}
	if got, ok := CheckpointBuildVerdict(nil); !ok || got != CheckpointBuildSuccess {
		t.Fatal("nil is not success")
	}
	if WithCheckpointBuildVerdict(nil, CheckpointBuildOther) != nil {
		t.Fatal("nil error wrapped")
	}
	for _, err := range []error{cause, WithCheckpointBuildVerdict(cause, 0), WithCheckpointBuildVerdict(cause, CheckpointBuildSuccess), WithCheckpointBuildVerdict(cause, 255)} {
		if _, ok := CheckpointBuildVerdict(err); ok {
			t.Fatal("unclassified error guessed")
		}
	}
}
