package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"github.com/nanolathe-gg/nanolathe/internal/community"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/session"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

func TestStartupBuildIdentityReportsUnavailableMetadata(t *testing.T) {
	for _, info := range []*debug.BuildInfo{nil, {}} {
		revision, modified, engine := startupBuildIdentity(info)
		if revision != "unavailable" || modified != "unknown" || engine != "unavailable" {
			t.Fatalf("missing metadata reported as %q %q %q", revision, modified, engine)
		}
	}
	info := &debug.BuildInfo{
		Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "authored-revision"}, {Key: "vcs.modified", Value: "true"}},
		Deps:     []*debug.Module{{Path: "github.com/hajimehoshi/ebiten/v2", Version: "authored-version", Replace: &debug.Module{Path: "private/path"}}},
	}
	revision, modified, engine := startupBuildIdentity(info)
	if revision != "authored-revision" || modified != "true" || engine != "authored-version (replaced)" {
		t.Fatalf("build metadata = %q %q %q", revision, modified, engine)
	}
}

func TestInstalledSourceRevisionIsBoundedAndDoesNotPrintArbitraryText(t *testing.T) {
	const revision = "0123456789abcdef0123456789abcdef01234567"
	for _, tc := range []struct{ manifest, want string }{
		{"version=authored\nsource_revision=" + revision + "\n", revision},
		{"source_revision=invalid\n", ""},
		{"source_revision=" + strings.Repeat("z", 40) + "\n", ""},
		{strings.Repeat("#\n", 32768) + "source_revision=" + revision + "\n", ""},
	} {
		if got := installedSourceRevision(strings.NewReader(tc.manifest)); got != tc.want {
			t.Fatalf("installed source revision = %q, want %q", got, tc.want)
		}
	}
}

func TestStartupInstalledRevisionPrefersActualCommitOverLegacySnapshot(t *testing.T) {
	const actual = "0123456789abcdef0123456789abcdef01234567"
	const legacy = "abcdef0123456789abcdef0123456789abcdef01"
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "release.txt"), []byte("source_revision="+legacy+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ source, want string }{
		{actual + "\n", actual + " (installed source)"},
		{actual + "\r\n", actual + " (installed source)"},
		{"invalid\n", legacy + " (release manifest)"},
		{strings.Repeat("z", 40), legacy + " (release manifest)"},
		{strings.Repeat(" ", 128) + actual, legacy + " (release manifest)"},
	} {
		if err := os.WriteFile(filepath.Join(directory, "source-revision"), []byte(tc.source), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := startupInstalledRevision(directory); got != tc.want {
			t.Fatalf("installed revision = %q, want %q", got, tc.want)
		}
	}
	if err := os.Remove(filepath.Join(directory, "source-revision")); err != nil {
		t.Fatal(err)
	}
	if got := startupInstalledRevision(directory); got != legacy+" (release manifest)" {
		t.Fatalf("legacy install revision = %q", got)
	}
}

func TestStartupFailureRetainsIssueContextOnStderr(t *testing.T) {
	code, out, errOut := installerRun(t, "--root", t.TempDir())
	if code != 1 || out != "" {
		t.Fatalf("failed start = %d, stdout %q", code, out)
	}
	for _, want := range []string{"nanolathe: startup:", "nanolathe: system:", "nanolathe: build:", "gamedata/moveinfo.tdf"} {
		if !strings.Contains(errOut, want) {
			t.Fatalf("failed start lacks %q: %s", want, errOut)
		}
	}
}

func TestWindowStartupReportUsesEffectiveSettings(t *testing.T) {
	shell := &gameShell{}
	shell.gameplay = gameplay.Strict31
	shell.presentation.Renderer = "classic"
	shell.presentation.FPS = 60
	shell.setup.UnitLimit = 500
	shell.fullscreen = true
	var out bytes.Buffer
	writeWindowStartupReport(&out, shell, 0)
	if !strings.Contains(out.String(), "renderer=classic fps-cap=60 fullscreen=true unit-limit=500") {
		t.Fatalf("effective settings missing: %s", out.String())
	}
	limit := 700
	shell.gameplay = gameplay.Modern
	shell.opts.GameplayOverrides = []community.Overrides{{UnitLimit: &limit}}
	out.Reset()
	writeWindowStartupReport(&out, shell, 0)
	if !strings.Contains(out.String(), "unit-limit=700") {
		t.Fatalf("feature-resolved unit limit missing: %s", out.String())
	}
	shell.gameplay = gameplay.Strict31
	shell.battle = &battleSession{sess: &session.Session{Skirmish: session.SkirmishConfig{UnitLimit: 250}}}
	out.Reset()
	writeWindowStartupReport(&out, shell, 0)
	if !strings.Contains(out.String(), "unit-limit=250") {
		t.Fatalf("active battle limit missing: %s", out.String())
	}
}

func TestWindowStartupReportFollowsChangedContentWithoutRepeatingProviders(t *testing.T) {
	fs := vfs.New()
	defer fs.Close()
	if err := fs.MountDirectory(t.TempDir(), 0); err != nil {
		t.Fatal(err)
	}
	cs := &contentSet{unmappedMount: fs, profile: "first"}
	var out bytes.Buffer
	writeContentStartupReport(&out, cs, time.Millisecond)
	shell := &gameShell{cs: cs}
	writeWindowStartupReport(&out, shell, 0)
	if strings.Count(out.String(), "nanolathe: content: profile=first") != 1 {
		t.Fatalf("reported unchanged content twice: %s", out.String())
	}
	shell.cs = &contentSet{unmappedMount: fs, profile: "replacement", loadDuration: 2 * time.Millisecond}
	writeWindowStartupReport(&out, shell, 0)
	if !strings.Contains(out.String(), "profile=replacement") || !strings.Contains(out.String(), "load=2ms") {
		t.Fatalf("running replacement content absent: %s", out.String())
	}
}

func TestWindowStartupReportDescribesSaveEntryWithoutActiveBattle(t *testing.T) {
	shell := &gameShell{opts: Options{LoadSave: "/private/path/battle.SAV"}}
	var out bytes.Buffer
	writeWindowStartupReport(&out, shell, 0)
	if !strings.Contains(out.String(), "entry=save entry") || strings.Contains(out.String(), shell.opts.LoadSave) {
		t.Fatalf("save entry must not claim an active battle or expose its host path: %s", out.String())
	}
}
