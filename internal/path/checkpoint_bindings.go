package path

import (
	"errors"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// NewSchedulerWithCheckpointBindings records the authority of the two bindings
// it installs. The authority proves their reviewed installation, not arbitrary
// callback identity (DESIGN_MULTIPLAYER §16.3.28–§16.3.30). Session keeps its
// authority private and supplies it only at known composition sites.
func NewSchedulerWithCheckpointBindings(search SearchFunc, publish PublishFunc, authority *checkpoint.BindingAuthority) *Scheduler {
	s := NewScheduler(search, publish)
	if search != nil {
		s.checkpointSearchAuthority = authority
	}
	if publish != nil {
		s.checkpointPublishAuthority = authority
	}
	return s
}

// SetSearchWithCheckpointBinding installs and attests only the search slot.
func (s *Scheduler) SetSearchWithCheckpointBinding(fn SearchFunc, authority *checkpoint.BindingAuthority) {
	s.SetSearch(fn)
	if fn != nil {
		s.checkpointSearchAuthority = authority
	}
}

// SetPublishWithCheckpointBinding installs and attests only the publisher slot.
func (s *Scheduler) SetPublishWithCheckpointBinding(fn PublishFunc, authority *checkpoint.BindingAuthority) {
	s.SetPublish(fn)
	if fn != nil {
		s.checkpointPublishAuthority = authority
	}
}

// SetCandidateProviderWithCheckpointBinding keeps the existing installation
// work: PlayerCount then UnitLimit, once each. Provenance is set only after that
// work returns; capture never invokes either getter (DESIGN_MULTIPLAYER §16.3.30).
func (s *Scheduler) SetCandidateProviderWithCheckpointBinding(p CandidateProvider, authority *checkpoint.BindingAuthority) {
	s.SetCandidateProvider(p)
	if p != nil {
		s.checkpointProviderAuthority = authority
	}
}

// SetSchedulerBindings registers the capture's exact expected scheduler and
// private session authority. It does not attest or change any live binding.
func (c *CheckpointContext) SetSchedulerBindings(s *Scheduler, authority *checkpoint.BindingAuthority) error {
	if c == nil || s == nil || authority == nil {
		return pathCheckpointError("paths.bindings", errors.New("missing context, scheduler or binding authority"))
	}
	if c.scheduler != nil && (c.scheduler != s || !c.bindingAuthority.Matches(authority)) {
		return pathCheckpointError("paths.bindings", errors.New("conflicting scheduler binding registration"))
	}
	c.scheduler, c.bindingAuthority = s, authority
	return nil
}

// CheckpointProviderMatches validates the provider's authority and concrete
// owner alias without invoking it. Only a nonnil concrete pointer can match;
// an unknown provider may contain noncomparable values and is never compared
// as an open interface (DESIGN_MULTIPLAYER §16.3.30).
func CheckpointProviderMatches[T any, P interface {
	*T
	CandidateProvider
}](s *Scheduler, expected P, authority *checkpoint.BindingAuthority) bool {
	if s == nil || expected == nil || !s.checkpointProviderAuthority.Matches(authority) {
		return false
	}
	actual, ok := s.provider.(P)
	return ok && actual == expected
}

func validateCheckpointScheduler(s *Scheduler, c *CheckpointContext) (string, error) {
	if c.scheduler != nil && c.scheduler != s {
		return "bindings", errors.New("context belongs to another scheduler")
	}
	// The private authority fields are proof only, never payload. Each retained
	// binding emits absent 0 or verified-present 1 at its original lexical field.
	// TODO(M3-U6): movement/session must install their canonical callbacks with
	// the private admitted authority and validate the provider's typed owner.
	if s.search != nil && (c.scheduler != s || !s.checkpointSearchAuthority.Matches(c.bindingAuthority)) {
		return "search", errors.New("unattested search binding")
	}
	if s.publish != nil && (c.scheduler != s || !s.checkpointPublishAuthority.Matches(c.bindingAuthority)) {
		return "publish", errors.New("unattested publication binding")
	}
	if s.provider != nil && (c.scheduler != s || !s.checkpointProviderAuthority.Matches(c.bindingAuthority)) {
		return "provider", errors.New("unattested provider binding")
	}
	return "", nil
}
