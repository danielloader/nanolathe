//go:build retail

package main

import (
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nanolathe-gg/nanolathe/internal/camera"
	"github.com/nanolathe-gg/nanolathe/internal/client"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/relay"
	"github.com/nanolathe-gg/nanolathe/internal/session"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport/retailcat"
)

func onlineTestSelection(t *testing.T) (content.Mutators, content.Restrictions) {
	t.Helper()
	mutators, err := content.ParseMutators(map[string]string{"buildSpeed": "2", "sight": "1.5"})
	if err != nil {
		t.Fatal(err)
	}
	restrictions, err := content.ParseRestrictions(map[string]int{"armpw": 5, "corak": 0})
	if err != nil {
		t.Fatal(err)
	}
	return mutators, restrictions
}

// The host's configuration, encoded, decoded and composed by the joiner on
// the same content, reports the host's identity and initial checksum; a
// different restriction set is a different configuration (§16.6).
func TestOnlineConfigurationAdoptionRetail(t *testing.T) {
	useStampedTestBuild(t)
	cat, fs := retailcat.Shared(t)
	cs := &contentSet{fs: fs, unmappedMount: fs, profile: "retail", limits: content.RetailLimits()}
	mutators, restrictions := onlineTestSelection(t)
	schema, err := onlineMapSchema(cs, cat, "ashap plateau")
	if err != nil {
		t.Fatal(err)
	}
	records, err := onlineMatchRestrictions(restrictions, cat)
	if err != nil || len(records) == 0 {
		t.Fatalf("restriction records %v: %v", records, err)
	}
	spec := onlineMatchSpec{mapName: "ashap plateau", simSeed: 29, crtSeed: 31, mutators: mutators, restrictions: records, restrictionSet: restrictions}
	hostConfig, err := onlineMatchConfig(spec, cs, schema)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := session.EncodeMatchConfig(hostConfig)
	if err != nil {
		t.Fatal(err)
	}
	joinConfig, err := session.DecodeMatchConfig(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if joinConfig.Digest() != hostConfig.Digest() || joinConfig.Request().Mutators != mutators {
		t.Fatal("the decoded configuration is not the host's")
	}
	if adopted, err := session.RestrictionsFromMatch(cat, joinConfig.Request().UnitRestrictions); err != nil || !adopted.Equal(restrictions) {
		t.Fatalf("adopted restrictions %v: %v", adopted, err)
	}
	host, hostID, err := composeOnlineMatch(cs, cat, hostConfig, 0)
	if err != nil {
		t.Fatal(err)
	}
	joiner, joinID, err := composeOnlineMatch(cs, cat, joinConfig, 1)
	if err != nil {
		t.Fatal(err)
	}
	if hostID != joinID || host.UnitStateChecksum() != joiner.UnitStateChecksum() {
		t.Fatal("the joiner's identity or initial checksum differs from the host's")
	}
	if host.LocalOwner != 0 || joiner.LocalOwner != 1 || host.IsPendingBattle() || joiner.IsPendingBattle() {
		t.Fatal("seats or prepared entry wrong")
	}
	other, err := content.ParseRestrictions(map[string]int{"armpw": 6})
	if err != nil {
		t.Fatal(err)
	}
	spec.restrictionSet = other
	if spec.restrictions, err = onlineMatchRestrictions(other, cat); err != nil {
		t.Fatal(err)
	}
	differ, err := onlineMatchConfig(spec, cs, schema)
	if err != nil {
		t.Fatal(err)
	}
	if differ.Digest() == hostConfig.Digest() {
		t.Fatal("different restrictions share a configuration identity")
	}
}

// onlineRetailShell is a menu shell on the retail install, without saved
// settings, with an online server field pointing at address.
func onlineRetailShell(t *testing.T, cs *contentSet, opts Options) *gameShell {
	t.Helper()
	shell, err := newGameShell(opts, cs)
	if err != nil {
		t.Fatal(err)
	}
	shell.cam = &camera.Camera{ViewW: retailScreenW, ViewH: retailScreenH, MapW: retailScreenW, MapH: retailScreenH}
	return shell
}

func awaitOnlineShells(t *testing.T, what string, done func() bool, shells ...*gameShell) {
	t.Helper()
	end := time.Now().Add(60 * time.Second)
	for !done() {
		if time.Now().After(end) {
			for _, g := range shells {
				if g.online != nil {
					t.Logf("phase %d status %q", g.online.phase, g.online.status)
				}
			}
			t.Fatalf("never reached %s", what)
		}
		for _, g := range shells {
			g.pollOnline()
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// One headless two-client match through a real local relay, started from the
// lobby with a mutator set and unit restrictions: create, describe, join,
// ready, start and granted ticks agreed by the relay's checksums (§16.6.3).
func TestOnlineLobbyTwoClientsRetail(t *testing.T) {
	useStampedTestBuild(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	server, err := relay.ListenHostedWebSocket("127.0.0.1:0", relay.HostedConfig{InsecureLoopback: true}, false)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	address := "ws://" + server.Addr() + "/relay"
	opts := Options{Root: testsupport.RetailRoot(t), Seed: -1}
	cs, err := openContent(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	opts.Root, opts.Roots = cs.root, cs.roots
	host, guest := onlineRetailShell(t, cs, opts), onlineRetailShell(t, cs, opts)
	mapName := ""
	for _, m := range host.maps {
		if strings.EqualFold(m, "ashap plateau") {
			mapName = m
		}
	}
	if mapName == "" {
		t.Fatal("Ashap Plateau is not in the map census")
	}
	mutators, restrictions := onlineTestSelection(t)
	host.setup.MapName = mapName
	host.opts.Mutators, host.opts.Restrictions = mutators, restrictions
	// The joiner's own selection must not reach the room's battle.
	guest.setup.MapName = host.maps[0]
	guest.opts.Mutators = content.Mutators{Health: content.Factor{Num: 3, Den: 1}}

	if err := host.openOnlineScreen(); err != nil {
		t.Fatal(err)
	}
	host.online.panel.SetText("SERVER", address)
	host.activateGadget("CREATE")
	awaitOnlineShells(t, "the host's lobby", func() bool { return host.online.phase == onlineInLobby }, host)
	code := host.online.lobby.Code()
	if len(code) != 6 {
		t.Fatalf("room code %q", code)
	}
	if err := guest.openOnlineScreen(); err != nil {
		t.Fatal(err)
	}
	guest.online.panel.SetText("SERVER", address)
	guest.online.panel.SetText("ROOMCODE", strings.ToLower(code[:3]+" "+code[3:]))
	guest.activateGadget("LOAD")
	awaitOnlineShells(t, "the guest's lobby", func() bool {
		return guest.online.phase == onlineInLobby && host.online.state.Present[1]
	}, host, guest)
	hostRoom, guestRoom := host.online.prepared.config, guest.online.prepared.config
	if guestRoom.Digest() != hostRoom.Digest() || guestRoom.Request().Mutators != mutators || len(guestRoom.Request().UnitRestrictions) == 0 || guestRoom.Request().MapName != mapName {
		t.Fatal("the guest did not adopt the host's configuration")
	}
	host.activateGadget("READY")
	guest.activateGadget("READY")
	awaitOnlineShells(t, "both seats ready", func() bool {
		return host.online.state.Ready == [2]bool{true, true} && !greyed(host.online.lobbyPanel, "LOAD")
	}, host, guest)
	host.activateGadget("LOAD")
	awaitOnlineShells(t, "both battles", func() bool { return host.battle != nil && guest.battle != nil }, host, guest)
	defer host.teardownBattle(nil)
	defer guest.teardownBattle(nil)
	if host.online != nil || guest.online != nil || host.frontend.Mode != modeBattle || guest.battle.sess.LocalOwner != 1 {
		t.Fatal("entering the battle left the online screen open")
	}
	const target = 95 // past three checksum ticks
	end := time.Now().Add(30 * time.Second)
	for host.battle.sess.Clock.GlobalTick < target || guest.battle.sess.Clock.GlobalTick < target {
		if time.Now().After(end) {
			t.Fatalf("ticks %d/%d", host.battle.sess.Clock.GlobalTick, guest.battle.sess.Clock.GlobalTick)
		}
		for _, g := range []*gameShell{host, guest} {
			g.battle.pumpLocalMultiplayer(nil)
			if mp := g.battle.multiplayer; mp == nil || mp.failure != nil {
				t.Fatalf("multiplayer stopped: %v", mp.failure)
			}
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// TestOnlineScreenCapture renders the online screen and both lobbies to PNGs
// for review. A review diagnostic: it runs only when NANOLATHE_ONLINE_CAPTURE
// names an output directory.
func TestOnlineScreenCapture(t *testing.T) {
	out := os.Getenv("NANOLATHE_ONLINE_CAPTURE")
	if out == "" {
		t.Skip("NANOLATHE_ONLINE_CAPTURE is unset")
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	opts := Options{Root: testsupport.RetailRoot(t), Seed: -1}
	cs, err := openContent(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	opts.Root, opts.Roots = cs.root, cs.roots
	shell := onlineRetailShell(t, cs, opts)
	cl, err := client.New(client.Options{Buffer: &frame.Buffer{}, Width: retailScreenW, Height: retailScreenH})
	if err != nil {
		t.Fatal(err)
	}
	saved := clPtr
	clPtr = cl
	defer func() { clPtr = saved }()
	if shell.assets != nil && shell.assets.pal != nil {
		cl.SetPalette(shell.assets.pal)
	}
	if shell.font != nil {
		cl.SetFNT(shell.font)
	}
	cl.SetUIStage(gameShellUIStage{shell: shell})
	shell.openMenu(modeMenuMain)
	capture := func(name string) {
		img := cl.ComposeFrame()
		f, err := os.Create(filepath.Join(out, "u3-"+name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
	}
	capture("main")
	if err := shell.openOnlineScreen(); err != nil {
		t.Fatal(err)
	}
	capture("online-unstamped")
	shell.closeOnlineScreen()
	useStampedTestBuild(t)
	shell.opts.Mutators, _ = onlineTestSelection(t)
	if err := shell.openOnlineScreen(); err != nil {
		t.Fatal(err)
	}
	shell.online.panel.SetText("ROOMCODE", "K7M 2QX")
	capture("online")
	shell.online.status = onlineRefusalText(relayRefusalForCapture())
	shell.refreshOnlinePanel()
	capture("online-refused")
	fake := &fakeOnlineRelay{}
	useOnlineTestRelay(t, fake)
	config, err := onlineMatchConfig(onlineMatchSpec{mapName: shell.setup.MapName, mutators: shell.opts.Mutators}, &contentSet{profile: "retail", limits: content.RetailLimits()}, 0)
	if err != nil {
		t.Fatal(err)
	}
	hostLobby := &fakeOnlineLobby{code: "K7M2QX", state: relay.HostedLobbyState{Present: [2]bool{true, true}, Ready: [2]bool{true, false}}}
	shell.online.status = ""
	shell.openOnlineLobby(&onlinePrepared{config: config, address: "wss://relay.nanolathe.gg/relay"}, hostLobby)
	shell.online.ready = true
	shell.pollOnline()
	capture("lobby-host")
	hostLobby.state.Ready[1] = true
	shell.pollOnline()
	capture("lobby-host-ready")
	shell.leaveOnlineLobby("")
	guestLobby := &fakeOnlineLobby{code: "K7M2QX", seat: 1, state: relay.HostedLobbyState{Present: [2]bool{true, true}, Ready: [2]bool{false, false}}}
	shell.openOnlineLobby(&onlinePrepared{config: config, address: "wss://relay.nanolathe.gg/relay"}, guestLobby)
	shell.pollOnline()
	capture("lobby-guest")
}

func relayRefusalForCapture() error {
	return relayError("room", "an existing invitation")
}

func relayError(path, expected string) error {
	return &missingProductError{what: "hosted relay rejected", logical: path, providers: []string{"hosted transport"}, expected: expected}
}
