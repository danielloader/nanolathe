package session

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/clock"
	"github.com/nanolathe-gg/nanolathe/internal/cob"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

type checkpointHostileCOBSink []byte

func (checkpointHostileCOBSink) EmitCOBEvent(cob.PresentationEvent) {
	panic("capture emitted an event")
}

func checkpointUnitSinkFixture() (*Session, *units.Unit, *cobPresentationSink, *cobExplosionSink) {
	s := &Session{Clock: &clock.State{}, publication: &publicationState{}}
	vm := cob.NewVM(nil)
	p := &cobPresentationSink{session: s, publication: s.publication, clock: s.Clock, source: 7, pieceMap: []int{0, -1, 2}}
	x := &cobExplosionSink{presentation: p}
	vm.SetExplosionSink(x)
	b := &cob.Binding{VM: vm, PresentationSink: p, PieceMap: []int{0, -1, 2}}
	u := &units.Unit{Handle: 7, Script: vm, ScriptState: &units.ScriptState{VM: vm, Binding: b}}
	return s, u, p, x
}

func TestCheckpointUnitSinksCheckActualCapturedOwners(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*Session, *units.Unit, *cobPresentationSink, *cobExplosionSink)
	}{
		{"foreign session", func(s *Session, _ *units.Unit, p *cobPresentationSink, _ *cobExplosionSink) {
			copy := Session{Clock: s.Clock, publication: s.publication}
			p.session = &copy
		}},
		{"publication", func(_ *Session, _ *units.Unit, p *cobPresentationSink, _ *cobExplosionSink) {
			p.publication = &publicationState{}
		}},
		{"clock", func(_ *Session, _ *units.Unit, p *cobPresentationSink, _ *cobExplosionSink) { p.clock = &clock.State{} }},
		{"source", func(_ *Session, _ *units.Unit, p *cobPresentationSink, _ *cobExplosionSink) { p.source++ }},
		{"map value", func(_ *Session, _ *units.Unit, p *cobPresentationSink, _ *cobExplosionSink) { p.pieceMap[1] = 0 }},
		{"map length", func(_ *Session, _ *units.Unit, p *cobPresentationSink, _ *cobExplosionSink) {
			p.pieceMap = p.pieceMap[:1]
		}},
		{"missing binding", func(_ *Session, u *units.Unit, _ *cobPresentationSink, _ *cobExplosionSink) {
			u.ScriptState.Binding = nil
		}},
		{"VM", func(_ *Session, u *units.Unit, _ *cobPresentationSink, _ *cobExplosionSink) {
			u.Script = cob.NewVM(nil)
		}},
		{"typed nil presentation", func(_ *Session, u *units.Unit, _ *cobPresentationSink, _ *cobExplosionSink) {
			u.COBBinding().PresentationSink = (*cobPresentationSink)(nil)
		}},
		{"hostile presentation", func(_ *Session, u *units.Unit, _ *cobPresentationSink, _ *cobExplosionSink) {
			u.COBBinding().PresentationSink = checkpointHostileCOBSink{1}
		}},
		{"typed nil explosion", func(_ *Session, u *units.Unit, _ *cobPresentationSink, _ *cobExplosionSink) {
			u.Script.SetExplosionSink((*cobExplosionSink)(nil))
		}},
		{"copied explosion target", func(_ *Session, _ *units.Unit, p *cobPresentationSink, x *cobExplosionSink) {
			copy := *p
			x.presentation = &copy
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, u, p, x := checkpointUnitSinkFixture()
			gotP, gotX, err := s.validateCheckpointCOBSinks(u)
			if err != nil || gotP != p || gotX != x {
				t.Fatalf("canonical sinks refused: %v", err)
			}
			tc.edit(s, u, p, x)
			if _, _, err := s.validateCheckpointCOBSinks(u); err == nil {
				t.Fatal("changed sink graph accepted")
			}
		})
	}
	s, u, p, x := checkpointUnitSinkFixture()
	for range 3 {
		if gotP, gotX, err := s.validateCheckpointCOBSinks(u); err != nil || gotP != p || gotX != x {
			t.Fatal("repeat validation changed owners")
		}
	}
	if s.Clock.GlobalTick != 0 || len(p.pieceMap) != 3 || p.pieceMap[1] != -1 {
		t.Fatal("capture changed live state")
	}
}
