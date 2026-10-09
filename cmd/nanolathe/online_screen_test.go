package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/gui"
	"github.com/nanolathe-gg/nanolathe/internal/lockstep"
	"github.com/nanolathe-gg/nanolathe/internal/modlibrary"
	"github.com/nanolathe-gg/nanolathe/internal/relay"
	"github.com/nanolathe-gg/nanolathe/internal/session"
	"github.com/nanolathe-gg/nanolathe/internal/ui"
	"github.com/nanolathe-gg/nanolathe/internal/version"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// useStampedTestBuild stands in a stamped release manifest for the test
// binary, which is never stamped. The manifest is authored here.
func useStampedTestBuild(t *testing.T) {
	t.Helper()
	saved := currentBuildManifest
	currentBuildManifest = func() (version.BuildManifest, error) {
		return version.BuildManifest{
			SourceTree: sha256.Sum256([]byte("online test source")),
			GoVersion:  "go1.27.1",
			GoMod:      sha256.Sum256([]byte("go.mod")),
			GoSum:      sha256.Sum256([]byte("go.sum")),
			BuildArgs:  []string{"-mod=readonly", "-trimpath", "./cmd/nanolathe"},
			Variants:   []version.BuildVariant{{GOOS: "darwin", GOARCH: "arm64", ArchitectureLevel: "v8.0", ToolchainArchive: sha256.Sum256([]byte("darwin"))}},
		}, nil
	}
	t.Cleanup(func() { currentBuildManifest = saved })
}

// useOnlineTestTemplate replaces the SELMAP template with an authored window
// of the same shape, so the online windows build without retail assets.
func useOnlineTestTemplate(t *testing.T) {
	t.Helper()
	saved := onlineTemplate
	onlineTemplate = func(*gameShell) (*gui.Window, *formats.PCX, error) {
		return &gui.Window{Name: "Selmap.GUI", Rect: gui.Rect{X: 84, Y: 12, W: 494, H: 420}, Gadgets: []gui.Gadget{
			{Kind: gui.KindPanel, Name: "Selmap.GUI", Active: 1, Rect: gui.Rect{X: 84, Y: 12, W: 494, H: 420}},
			{Kind: gui.KindListBox, Name: "MAPNAMES", Active: 1, Rect: gui.Rect{X: 60, Y: 86, W: 230, H: 193}},
			{Kind: gui.KindButton, Name: "PREVMENU", Text: "Cancel", Active: 1, QuickKey: 'C', Rect: gui.Rect{X: 356, Y: 360, W: 96, H: 20}},
			{Kind: gui.KindButton, Name: "LOAD", Text: "Select Map", Active: 1, QuickKey: 'S', Rect: gui.Rect{X: 357, Y: 325, W: 96, H: 20}},
			{Kind: gui.KindScrollBar, Name: "SLIDER", Active: 1, Rect: gui.Rect{X: 305, Y: 75, W: 16, H: 203}},
			{Kind: gui.KindLabel, Name: "DESCRIPTION", Text: "Description", Active: 1, Attribs: 0x11, ColorF: 15, Rect: gui.Rect{X: 60, Y: 300, W: 235, H: 31}},
			{Kind: gui.KindLabel, Name: "SIZE", Text: "Size", Active: 1, Attribs: 0x11, ColorF: 15, Rect: gui.Rect{X: 60, Y: 330, W: 235, H: 18}},
		}}, nil, nil
	}
	t.Cleanup(func() { onlineTemplate = saved })
}

// useOnlineTestRelay installs a fake relay for the test.
func useOnlineTestRelay(t *testing.T, r *fakeOnlineRelay) {
	t.Helper()
	saved, savedCopy := onlineRelayService, onlineClipboardWrite
	onlineRelayService = r
	onlineClipboardWrite = func(text string) bool { r.copied = append(r.copied, text); return true }
	t.Cleanup(func() { onlineRelayService, onlineClipboardWrite = saved, savedCopy })
}

func onlineTestShell(t *testing.T) *gameShell {
	t.Helper()
	useOnlineTestTemplate(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	main := &gui.Window{Rect: gui.Rect{W: 640, H: 480}, Gadgets: []gui.Gadget{
		{Kind: gui.KindPanel, Active: 1},
		{Kind: gui.KindButton, Name: "MULTI", Active: 1, Rect: gui.Rect{X: 20, Y: 20, W: 80, H: 30}},
		{Kind: gui.KindButton, Name: "SINGLE", Active: 1, Rect: gui.Rect{X: 20, Y: 60, W: 80, H: 30}},
	}}
	g := &gameShell{frontend: ui.NewFrontend(modeMenuMain), cs: testContentSet(vfs.New()), maps: []string{"Test Map"},
		assets: &menuAssets{panel: map[shellMode]*retailPanelAssets{modeMenuMain: {window: main}}}}
	g.setup.MapName = "Test Map"
	g.openMenu(modeMenuMain)
	return g
}

type fakeOnlineRelay struct {
	mu        sync.Mutex
	describes []string
	opens     []relay.LocalHello
	configs   [][]byte
	describe  func(room string) ([]byte, error)
	open      func(room string, hello relay.LocalHello, config []byte) (onlineLobby, error)
	copied    []string
}

func (r *fakeOnlineRelay) Describe(_ context.Context, address, room string, _ relay.HostedDialOptions) ([]byte, error) {
	r.mu.Lock()
	r.describes = append(r.describes, address+" "+room)
	r.mu.Unlock()
	if r.describe == nil {
		return nil, errors.New("no describe")
	}
	return r.describe(room)
}

func (r *fakeOnlineRelay) Open(_ context.Context, _, room string, hello relay.LocalHello, config []byte, _ relay.HostedDialOptions) (onlineLobby, error) {
	r.mu.Lock()
	r.opens = append(r.opens, hello)
	r.configs = append(r.configs, config)
	r.mu.Unlock()
	if r.open == nil {
		return nil, errors.New("no open")
	}
	return r.open(room, hello, config)
}

type fakeOnlineLobby struct {
	code    string
	seat    uint8
	state   relay.HostedLobbyState
	err     error
	readies []bool
	starts  int
	closes  int
	battle  lockstep.Client
}

func (l *fakeOnlineLobby) Code() string                           { return l.code }
func (l *fakeOnlineLobby) Seat() uint8                            { return l.seat }
func (l *fakeOnlineLobby) State() (relay.HostedLobbyState, error) { return l.state, l.err }
func (l *fakeOnlineLobby) SetReady(ready bool) error {
	l.readies = append(l.readies, ready)
	return nil
}
func (l *fakeOnlineLobby) Start() error { l.starts++; return nil }
func (l *fakeOnlineLobby) Battle() lockstep.Client {
	if !l.state.Started {
		return nil
	}
	return l.battle
}
func (l *fakeOnlineLobby) Close() error { l.closes++; return nil }

type fakeGrantStream struct{ closes int }

func (c *fakeGrantStream) Submit([]byte) (uint64, error) { return 0, nil }
func (c *fakeGrantStream) ReadGrant() (relay.LocalGrant, error) {
	return relay.LocalGrant{}, errors.New("closed")
}
func (c *fakeGrantStream) Acknowledge(uint32, [32]byte, bool) error { return nil }
func (c *fakeGrantStream) Close() error                             { c.closes++; return nil }

func greyed(p *ui.Panel, name string) bool {
	i := p.Index(name)
	return i < 0 || p.Window.Gadgets[i].GrayedOut&1 != 0
}

// awaitOnline polls the shell until the online screen leaves its busy phases.
func awaitOnline(t *testing.T, g *gameShell) {
	t.Helper()
	end := time.Now().Add(10 * time.Second)
	for g.online != nil && g.online.job != nil {
		if time.Now().After(end) {
			t.Fatalf("online job never finished: %+v", g.online)
		}
		g.pollOnline()
		time.Sleep(time.Millisecond)
	}
}

func TestMainMenuMultiOpensTheOnlineScreen(t *testing.T) {
	g := onlineTestShell(t)
	main := g.activePanel()
	multi := main.Index("MULTI")
	if !onlinePlayAvailable() {
		if main.Window.Gadgets[multi].GrayedOut&1 == 0 || main.Fires(multi) {
			t.Fatal("the browser build offered MULTI")
		}
		return
	}
	if main.Window.Gadgets[multi].GrayedOut != 0 || !main.Fires(multi) {
		t.Fatal("MULTI is not enabled")
	}
	g.activateGadget("MULTI")
	if g.online == nil || g.activePanel() != g.online.panel || g.frontend.Panels.Under() != main {
		t.Fatal("MULTI did not push the online screen over the main menu")
	}
	p := g.online.panel
	if p.TextOf("SERVER") != defaultOnlineServer || p.TextOf("NTITLE") != onlineTitle {
		t.Fatalf("server %q title %q", p.TextOf("SERVER"), p.TextOf("NTITLE"))
	}
	for _, name := range []string{"MAPNAMES", "SLIDER"} {
		if p.Index(name) >= 0 {
			t.Fatalf("the map chooser's %s stayed on the online screen", name)
		}
	}
	// The test binary is unstamped: it is told so up front and cannot act.
	if !greyed(p, "CREATE") || !greyed(p, "LOAD") || !strings.Contains(p.TextOf("DESCRIPTION"), "stamped") {
		t.Fatalf("unstamped build: create greyed %v, join greyed %v, status %q", greyed(p, "CREATE"), greyed(p, "LOAD"), p.TextOf("DESCRIPTION"))
	}
	g.activateGadget("PREVMENU")
	if g.online != nil || g.activePanel() != main {
		t.Fatal("Back did not return to the main menu")
	}
}

func TestOnlineServerAddress(t *testing.T) {
	for typed, want := range map[string]string{
		"":                               "wss://relay.nanolathe.gg/relay",
		"  relay.example.test ":          "wss://relay.example.test/relay",
		"relay.example.test:39032":       "relay.example.test:39032",
		"wss://relay.example.test/relay": "wss://relay.example.test/relay",
		"[::1]":                          "wss://[::1]/relay",
	} {
		got, options, err := onlineServerAddress(typed)
		if err != nil || got != want || options.InsecureLoopback || options.TLSConfig != nil {
			t.Fatalf("%q: %q %+v %v, want %q", typed, got, options, err, want)
		}
	}
	if got, options, err := onlineServerAddress("ws://127.0.0.1:8080/relay"); err != nil || got != "ws://127.0.0.1:8080/relay" || !options.InsecureLoopback {
		t.Fatalf("loopback plaintext: %q %+v %v", got, options, err)
	}
	for _, typed := range []string{"ws://relay.example.test/relay", "ws://192.0.2.1:8080/relay", "https://relay.example.test/relay", "wss://relay.example.test/", "relay.example.test:0", "wss://user@relay.example.test/relay", "relay example"} {
		if _, _, err := onlineServerAddress(typed); err == nil {
			t.Fatalf("admitted %q", typed)
		} else if text := onlineRefusalText(err); !strings.Contains(text, "server address is not valid") {
			t.Fatalf("%q refusal: %q", typed, text)
		}
	}
}

func TestOnlineRefusalTextIsPlain(t *testing.T) {
	relayRefusal := func(path, expected string) error {
		return errors.New("nanolathe: hosted relay rejected: logical path " + path + ", providers searched [hosted transport], expected " + expected)
	}
	for _, c := range []struct {
		err  error
		want string
	}{
		{relayRefusal("room", "an existing invitation"), "No game has that code"},
		{relayRefusal("room", "an unoccupied second seat"), "full or has already started"},
		{relayRefusal("room", "an open room"), "That game has closed"},
		{relayRefusal("room host", "the host to stay until the match starts"), "The host left"},
		{relayRefusal("room wait", "a started match within 30 minutes"), "30 minutes"},
		{relayRefusal("room capacity", "space for another room"), "server is full"},
		{relayRefusal("hello build", "identical values from both seats"), "build differs"},
		{relayRefusal("hello content", "identical values from both seats"), "game files differ"},
		{relayRefusal("hello mod", "identical values from both seats"), "mod differs"},
		{relayRefusal("hello initial checksum", "identical values from both seats"), "(initial checksum)"},
		{relayRefusal("handshake", "hosted protocol version 2; this client sent version 1"), "different online protocol"},
		{localMultiplayerError("binary", "a stamped build shared by both clients"), "stamped release build"},
		{localMultiplayerError("mounted content", "the base game or an installed mod"), "base game or an installed mod"},
		{&content.RestrictionsError{Issues: []content.RestrictionIssue{{Unit: "armcom", Reason: content.RestrictionRemovesCommander}}}, "armcom"},
		{errors.New("nanolathe: relay stream failed: logical path hosted dial, providers searched [relay transport], expected a complete transport message: dial tcp: lookup relay.example.test: no such host"), "Could not reach the server: dial tcp: lookup relay.example.test: no such host"},
	} {
		if got := onlineRefusalText(c.err); !strings.Contains(got, c.want) || strings.Contains(got, "logical path") {
			t.Fatalf("%v: %q, want %q in plain words", c.err, got, c.want)
		}
	}
}

func TestOnlineJoinChecksTheRoomBeforeComposing(t *testing.T) {
	useStampedTestBuild(t)
	fake := &fakeOnlineRelay{}
	useOnlineTestRelay(t, fake)
	g := onlineTestShell(t)
	if err := g.openOnlineScreen(); err != nil {
		t.Fatal(err)
	}
	p := g.online.panel
	if greyed(p, "LOAD") || greyed(p, "CREATE") {
		t.Fatal("a stamped build cannot act")
	}
	// A malformed code never reaches the server.
	p.SetText("ROOMCODE", "abc")
	g.activateGadget("LOAD")
	if len(fake.describes) != 0 || !strings.Contains(g.online.status, "six-character") {
		t.Fatalf("short code: %v %q", fake.describes, g.online.status)
	}
	// The joiner's own selection must not leak into the room it adopts.
	g.opts.Mutators = content.Mutators{Health: content.Factor{Num: 2, Den: 1}}
	cs := &contentSet{profile: "retail", limits: content.RetailLimits()}
	roomConfig := func(spec onlineMatchSpec) []byte {
		config, err := onlineMatchConfig(spec, cs, 0)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := session.EncodeMatchConfig(config)
		if err != nil {
			t.Fatal(err)
		}
		return encoded
	}
	next := roomConfig(onlineMatchSpec{mapName: "Absent Map", simSeed: 1, crtSeed: 2})
	fake.describe = func(string) ([]byte, error) { return next, nil }
	p.SetText("SERVER", "relay.example.test")
	p.SetText("ROOMCODE", "abc 234")
	g.activateGadget("ROOMCODE") // Enter in the code field joins
	if g.online.phase != onlineDescribing || !greyed(p, "LOAD") || !greyed(p, "CREATE") {
		t.Fatal("the join did not wait for the room's description")
	}
	awaitOnline(t, g)
	if len(fake.describes) != 1 || fake.describes[0] != "wss://relay.example.test/relay ABC234" || g.onlineServer != "relay.example.test" {
		t.Fatalf("describe calls %v, remembered server %q", fake.describes, g.onlineServer)
	}
	if len(fake.opens) != 0 || !strings.Contains(g.online.status, "Absent Map") || !strings.Contains(g.online.status, "not installed") {
		t.Fatalf("missing map: opens %d, status %q", len(fake.opens), g.online.status)
	}
	// A mod that is not installed is named, and nothing is joined.
	next = roomConfig(onlineMatchSpec{mapName: "Test Map", mod: session.MatchMod{ID: "absentmod", Version: "1.0", Archive: sha256.Sum256([]byte("absentmod"))}})
	g.activateGadget("LOAD")
	awaitOnline(t, g)
	if len(fake.opens) != 0 || pendingContentReload != nil || !strings.Contains(g.online.status, "absentmod 1.0") {
		t.Fatalf("missing mod: opens %d, reload %v, status %q", len(fake.opens), pendingContentReload, g.online.status)
	}
	// A room on this content composes from the room's configuration. The
	// empty test content cannot compose, so the refusal comes from
	// composition, after the room was adopted.
	next = roomConfig(onlineMatchSpec{mapName: "Test Map"})
	g.activateGadget("LOAD")
	if g.online.room != nil {
		t.Fatal("the summary showed a room before it was described")
	}
	awaitOnline(t, g)
	if len(fake.opens) != 0 || g.online.phase != onlineIdle || g.online.status == "" {
		t.Fatalf("composition refusal: opens %d, phase %d, status %q", len(fake.opens), g.online.phase, g.online.status)
	}
}

func TestOnlineCreateRefusalsAndBusyState(t *testing.T) {
	useStampedTestBuild(t)
	fake := &fakeOnlineRelay{}
	useOnlineTestRelay(t, fake)
	g := onlineTestShell(t)
	if err := g.openOnlineScreen(); err != nil {
		t.Fatal(err)
	}
	p := g.online.panel
	p.SetText("SERVER", "ws://192.0.2.1:8080/relay")
	g.activateGadget("CREATE")
	if g.online.job != nil || !strings.Contains(g.online.status, "not valid") {
		t.Fatalf("bad server: %q", g.online.status)
	}
	g.cs.manualRoots = true
	p.SetText("SERVER", "")
	g.activateGadget("CREATE")
	if g.online.job != nil || !strings.Contains(g.online.status, "installed mod") {
		t.Fatalf("manual roots: %q", g.online.status)
	}
	g.cs.manualRoots = false
	g.cs.mod = &modlibrary.Mod{Metadata: modlibrary.Metadata{ID: "folder", Version: "1"}}
	g.activateGadget("CREATE")
	if g.online.job != nil || !strings.Contains(g.online.status, "installed from its archive") {
		t.Fatalf("folder mod: %q", g.online.status)
	}
	g.cs.mod = nil
	g.activateGadget("CREATE")
	if g.online.phase != onlinePreparing || !greyed(p, "CREATE") || !greyed(p, "CHANGEMAP") {
		t.Fatal("create did not run off the game goroutine")
	}
	// Back abandons the job; its result never reaches a closed screen.
	g.activateGadget("PREVMENU")
	if g.online != nil {
		t.Fatal("Back during a job left the screen open")
	}
	onlineWork.Wait()
	if len(fake.opens) != 0 {
		t.Fatal("the empty test content opened a room")
	}
}

func TestOnlineLobbyStateMachine(t *testing.T) {
	fake := &fakeOnlineRelay{}
	useOnlineTestRelay(t, fake)
	g := onlineTestShell(t)
	if err := g.openOnlineScreen(); err != nil {
		t.Fatal(err)
	}
	config, err := onlineMatchConfig(onlineMatchSpec{mapName: "Test Map", mutators: content.Mutators{Sight: content.Factor{Num: 2, Den: 1}}}, &contentSet{profile: "retail", limits: content.RetailLimits()}, 0)
	if err != nil {
		t.Fatal(err)
	}
	stream := &fakeGrantStream{}
	host := &fakeOnlineLobby{code: "ABC234", battle: stream}
	g.openOnlineLobby(&onlinePrepared{config: config, address: "wss://relay.example.test/relay"}, host)
	p := g.online.lobbyPanel
	if g.activePanel() != p || g.frontend.Panels.Under() != g.online.panel {
		t.Fatal("the lobby is not over the online screen")
	}
	if !greyed(p, "LOAD") || p.TextOf("SUM2") != "1 mutator" || p.TextOf("SUMLABEL") != "This game" {
		t.Fatalf("new lobby: start greyed %v, summary %q %q", greyed(p, "LOAD"), p.TextOf("SUMLABEL"), p.TextOf("SUM2"))
	}
	host.state.Present[0] = true
	g.pollOnline()
	if !strings.Contains(p.TextOf("DESCRIPTION"), "Send the room code") || p.TextOf("SEAT1") != "Guest: waiting to join" {
		t.Fatalf("waiting host: %q %q", p.TextOf("DESCRIPTION"), p.TextOf("SEAT1"))
	}
	g.activateGadget("COPY")
	if len(fake.copied) != 1 || fake.copied[0] != "ABC234" {
		t.Fatalf("copy: %v", fake.copied)
	}
	g.activateGadget("READY")
	if len(host.readies) != 1 || !host.readies[0] || p.TextOf("READY") != "Not ready" {
		t.Fatalf("ready: %v %q", host.readies, p.TextOf("READY"))
	}
	host.state = relay.HostedLobbyState{Present: [2]bool{true, true}, Ready: [2]bool{true, false}}
	g.pollOnline()
	if !greyed(p, "LOAD") || p.TextOf("SEAT1") != "Guest: not ready" {
		t.Fatal("start offered before both seats were ready")
	}
	host.state.Ready[1] = true
	g.pollOnline()
	if greyed(p, "LOAD") {
		t.Fatal("start not offered with both seats ready")
	}
	g.activateGadget("LOAD")
	if host.starts != 1 {
		t.Fatal("the host's Start did not reach the relay")
	}
	// Started hands the connection to the battle. This lobby carries no
	// composed battle, so entry fails, the stream closes and the screen
	// says so.
	host.state.Started = true
	g.pollOnline()
	if stream.closes != 1 || host.closes != 1 || g.activePanel() != g.online.panel || !strings.Contains(g.online.status, "could not start") {
		t.Fatalf("failed entry: stream closes %d, lobby closes %d, status %q", stream.closes, host.closes, g.online.status)
	}

	// A relay failure returns to the online screen with its reason.
	gone := &fakeOnlineLobby{code: "ABC234", state: relay.HostedLobbyState{Present: [2]bool{true, true}}}
	g.openOnlineLobby(&onlinePrepared{config: config}, gone)
	gone.err = errors.New("nanolathe: hosted relay rejected: logical path room host, providers searched [hosted transport], expected the host to stay until the match starts")
	g.pollOnline()
	if gone.closes != 1 || g.online.lobbyPanel != nil || g.online.status != "The host left, so the game closed." {
		t.Fatalf("host left: closes %d, status %q", gone.closes, g.online.status)
	}

	// The guest can never start; Leave closes the room's seat.
	guest := &fakeOnlineLobby{code: "ABC234", seat: 1, state: relay.HostedLobbyState{Present: [2]bool{true, true}, Ready: [2]bool{true, true}}}
	g.openOnlineLobby(&onlinePrepared{config: config}, guest)
	g.pollOnline()
	p = g.online.lobbyPanel
	if !greyed(p, "LOAD") || p.TextOf("YOU") != "You are the guest" || !strings.Contains(p.TextOf("DESCRIPTION"), "host to start") {
		t.Fatalf("guest lobby: %q %q", p.TextOf("YOU"), p.TextOf("DESCRIPTION"))
	}
	g.activateGadget("LOAD")
	g.activateGadget("PREVMENU")
	if guest.starts != 0 || guest.closes != 1 || g.activePanel() != g.online.panel || g.online.status != "You left the game." {
		t.Fatalf("guest leave: starts %d closes %d status %q", guest.starts, guest.closes, g.online.status)
	}
}

func TestOnlineMapPickerReturnsToTheOnlineScreen(t *testing.T) {
	g := onlineTestShell(t)
	selmap := &gui.Window{Name: "selmap", Rect: gui.Rect{X: 84, Y: 12, W: 494, H: 420}, Gadgets: []gui.Gadget{{Kind: gui.KindPanel, Active: 1}}}
	g.assets.panel[modeMenuMap] = &retailPanelAssets{window: selmap}
	g.maps = []string{"First", "Second"}
	if err := g.openOnlineScreen(); err != nil {
		t.Fatal(err)
	}
	online := g.online.panel
	g.activateGadget("CHANGEMAP")
	if g.frontend.Mode != modeMenuMap || g.frontend.Panels.Under() != online {
		t.Fatal("Change map did not open the map selector over the online screen")
	}
	g.mapIdx = 1
	g.activateGadget("LOAD")
	if g.frontend.Mode != modeMenuMain || g.activePanel() != online || g.setup.MapName != "Second" || online.TextOf("SUM0") != "Second" {
		t.Fatalf("map choice: mode %d, map %q, summary %q", g.frontend.Mode, g.setup.MapName, online.TextOf("SUM0"))
	}
}

// A room's mod that is installed but not mounted is mounted through the
// ordinary content reload, which carries the join and the player's own
// mutators; a different copy of the mod is refused by name.
func TestOnlineJoinRequestsTheRoomsMod(t *testing.T) {
	useStampedTestBuild(t)
	fake := &fakeOnlineRelay{}
	useOnlineTestRelay(t, fake)
	g := onlineTestShell(t)
	t.Cleanup(func() { pendingContentReload = nil })
	var archive bytes.Buffer
	z := zip.NewWriter(&archive)
	w, err := z.Create(modlibrary.MetadataFile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(`{"schema":1,"id":"testmod","name":"Test Mod","version":"1"}`)); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "testmod.zip")
	if err := os.WriteFile(path, archive.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	lib, err := openModLibrary()
	if err != nil {
		t.Fatal(err)
	}
	installed, err := lib.InstallArchive(path, modlibrary.InstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := matchModOf(&installed)
	if want.Archive != sha256.Sum256(archive.Bytes()) {
		t.Fatal("the mod's identity is not its archive digest")
	}
	if err := g.openOnlineScreen(); err != nil {
		t.Fatal(err)
	}
	g.opts.Mutators = content.Mutators{Damage: content.Factor{Num: 2, Den: 1}}
	encode := func(mod session.MatchMod) []byte {
		config, err := onlineMatchConfig(onlineMatchSpec{mapName: "Test Map", mod: mod, mutators: content.Mutators{Sight: content.Factor{Num: 2, Den: 1}}}, &contentSet{profile: "retail", limits: content.RetailLimits()}, 0)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := session.EncodeMatchConfig(config)
		if err != nil {
			t.Fatal(err)
		}
		return encoded
	}
	other := want
	other.Archive[0] ^= 1
	g.adoptOnlineRoom(onlineRoom{address: "wss://relay.example.test/relay", code: "ABC234", config: encode(other)}, false)
	if pendingContentReload != nil || !strings.Contains(g.online.status, "different copy") {
		t.Fatalf("another copy: reload %v, status %q", pendingContentReload, g.online.status)
	}
	room := onlineRoom{address: "wss://relay.example.test/relay", code: "ABC234", config: encode(want)}
	g.adoptOnlineRoom(room, false)
	r := pendingContentReload
	if r == nil || r.selector != "testmod@1" || r.mod.ID != "testmod" || r.online == nil || r.online.code != "ABC234" || r.mutators != g.opts.Mutators {
		t.Fatalf("reload request %+v", r)
	}
	if !strings.Contains(g.online.status, "testmod 1") || g.online.phase != onlinePreparing || len(fake.opens) != 0 {
		t.Fatalf("status %q phase %d", g.online.status, g.online.phase)
	}
	// After the reload the mod must be the room's; if it is not, the join
	// stops rather than switching again.
	pendingContentReload = nil
	g.adoptOnlineRoom(room, true)
	if pendingContentReload != nil || !strings.Contains(g.online.status, "could not be selected") {
		t.Fatalf("remounted mismatch: %q", g.online.status)
	}
}
