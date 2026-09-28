package main

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/session"
)

func TestFPSCommandTogglesHostDisplayAcrossBattles(t *testing.T) {
	shell := &gameShell{}
	first := &battleSession{shell: shell}
	if first.fpsShown() {
		t.Fatal("FPS display started enabled")
	}
	first.dispatchLocalCommand("+FpS")
	if !first.fpsShown() || !(&battleSession{shell: shell}).fpsShown() {
		t.Fatal("FPS display did not stay enabled for the next battle")
	}
	first.dispatchLocalCommand("+fps")
	if first.fpsShown() {
		t.Fatal("second command did not disable the FPS display")
	}
}

func TestFPSOverlayStopsAtCommittedResult(t *testing.T) {
	shell := &gameShell{fpsVisible: true}
	buf := frame.NewBuffer()
	battle := &battleSession{shell: shell, sess: &session.Session{Snapshot: buf}}
	shell.battle = battle
	showFPS := shell.windowOptions().ShowFPS
	if !showFPS() {
		t.Fatal("FPS overlay hidden during battle")
	}
	buf.BeginWrite().Result = frame.ResultView{Ended: true, Kind: "victory"}
	if err := buf.Publish(1); err != nil {
		t.Fatal(err)
	}
	battle.ensurePostBattleController()
	if showFPS() {
		t.Fatal("FPS overlay covered the terminal result")
	}
	shell.battle = &battleSession{shell: shell}
	if !showFPS() || !shell.fpsVisible {
		t.Fatal("terminal result cleared the FPS choice for the next battle")
	}
}
