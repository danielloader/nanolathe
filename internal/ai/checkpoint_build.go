package ai

import "errors"

// Build verdicts classify the existing typed producer return paths only
// (DESIGN_MULTIPLAYER §16.3.7, §16.3.25). They never gate gameplay.
const (
	CheckpointBuildSuccess uint8 = iota + 1
	CheckpointBuildProduct
	CheckpointBuildSite
	CheckpointBuildOwner
	CheckpointBuildLimit
	CheckpointBuildBinding
	CheckpointBuildOther
)

type checkpointBuildError struct {
	cause   error
	verdict uint8
}

func (e *checkpointBuildError) Error() string { return e.cause.Error() }
func (e *checkpointBuildError) Unwrap() error { return e.cause }

// WithCheckpointBuildVerdict preserves an existing error's text and unwrap
// chain while tagging the known producer branch. Invalid tags stay unclassified.
// A nil error remains nil; success does not allocate a diagnostic wrapper.
func WithCheckpointBuildVerdict(err error, verdict uint8) error {
	if err == nil {
		return nil
	}
	if verdict < CheckpointBuildProduct || verdict > CheckpointBuildOther {
		verdict = 0
	}
	return &checkpointBuildError{cause: err, verdict: verdict}
}

// CheckpointBuildVerdict does not parse errors or guess a category. A newly
// introduced unclassified failure invalidates its application history.
func CheckpointBuildVerdict(err error) (uint8, bool) {
	if err == nil {
		return CheckpointBuildSuccess, true
	}
	var tagged *checkpointBuildError
	if errors.As(err, &tagged) && tagged != nil && tagged.verdict >= CheckpointBuildProduct && tagged.verdict <= CheckpointBuildOther {
		return tagged.verdict, true
	}
	return 0, false
}
