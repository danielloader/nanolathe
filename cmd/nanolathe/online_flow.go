package main

// The online screen's work: Create, Join, the lobby and the battle it starts
// (DESIGN_MULTIPLAYER §16.6.2). Composition and every network call that can
// wait run on a job goroutine; the game goroutine polls the job and the
// lobby's never-blocking state from the shell's step.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/nanolathe-gg/nanolathe/internal/client"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/lockstep"
	"github.com/nanolathe-gg/nanolathe/internal/platform/ebitenapp"
	"github.com/nanolathe-gg/nanolathe/internal/relay"
	"github.com/nanolathe-gg/nanolathe/internal/session"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
)

// onlineRelay is the hosted relay as the online screen uses it. Tests
// substitute a fake; production uses the relay package.
type onlineRelay interface {
	Describe(ctx context.Context, address, room string, options relay.HostedDialOptions) ([]byte, error)
	Open(ctx context.Context, address, room string, hello relay.LocalHello, config []byte, options relay.HostedDialOptions) (onlineLobby, error)
}

// onlineLobby is one seat's room before Start (relay.HostedLobby).
type onlineLobby interface {
	Code() string
	Seat() uint8
	State() (relay.HostedLobbyState, error)
	SetReady(bool) error
	Start() error
	// Battle is the grant stream once Started, nil before.
	Battle() lockstep.Client
	Close() error
}

type hostedOnlineRelay struct{}

func (hostedOnlineRelay) Describe(ctx context.Context, address, room string, options relay.HostedDialOptions) ([]byte, error) {
	return relay.DescribeHostedRoom(ctx, address, room, options)
}

func (hostedOnlineRelay) Open(ctx context.Context, address, room string, hello relay.LocalHello, config []byte, options relay.HostedDialOptions) (onlineLobby, error) {
	l, err := relay.OpenHostedLobby(ctx, address, room, hello, config, options)
	if err != nil {
		return nil, err
	}
	return hostedOnlineLobby{l}, nil
}

type hostedOnlineLobby struct{ *relay.HostedLobby }

// Battle keeps a nil client a nil interface.
func (l hostedOnlineLobby) Battle() lockstep.Client {
	if c := l.HostedLobby.Battle(); c != nil {
		return c
	}
	return nil
}

var onlineRelayService onlineRelay = hostedOnlineRelay{}

// onlineClipboardWrite is the host clipboard's write; tests substitute one so
// they never replace the player's clipboard.
var onlineClipboardWrite = ebitenapp.WriteHostClipboard

// Network waits are host bounds, not simulation inputs.
const (
	onlineDescribeTimeout = 10 * time.Second
	onlineOpenTimeout     = 15 * time.Second
)

// onlineWork counts running jobs. A content reload waits for them before it
// closes the content they read.
var onlineWork sync.WaitGroup

type onlineJob struct {
	cancel context.CancelFunc
	done   chan onlineJobResult
}

// onlineRoom is a room the player is joining: where it is and the
// configuration bytes the relay described.
type onlineRoom struct {
	address string
	options relay.HostedDialOptions
	code    string
	config  []byte
}

type onlineJobResult struct {
	described *onlineRoom // a Describe that succeeded
	prepared  *onlinePrepared
	lobby     onlineLobby
	err       error
}

// onlinePrepared is a battle composed and prepared for its first grant.
type onlinePrepared struct {
	sess    *session.Session
	config  session.EffectiveMatchConfig
	detail  *client.DetailArt
	address string
}

func (s *onlineScreen) run(work func(context.Context) onlineJobResult) {
	ctx, cancel := context.WithCancel(context.Background())
	job := &onlineJob{cancel: cancel, done: make(chan onlineJobResult, 1)}
	s.job = job
	onlineWork.Add(1)
	go func() {
		defer onlineWork.Done()
		job.done <- work(ctx)
	}()
}

// cancelJob abandons the running job. A lobby it opens anyway is closed.
func (s *onlineScreen) cancelJob() {
	job := s.job
	if job == nil {
		return
	}
	s.job = nil
	job.cancel()
	go func() {
		if r := <-job.done; r.lobby != nil {
			_ = r.lobby.Close()
		}
	}()
}

// fail returns the screen to idle with a plain-words reason.
func (g *gameShell) onlineFail(err error) {
	if s := g.online; s != nil {
		fmt.Fprintf(os.Stderr, "nanolathe: online: %v\n", err)
		g.onlineIdleStatus(onlineRefusalText(err))
	}
}

func (g *gameShell) onlineIdleStatus(status string) {
	s := g.online
	if s == nil {
		return
	}
	s.phase, s.status, s.room = onlineIdle, status, nil
	g.refreshOnlinePanel()
}

// onlineServerFromPanel reads and remembers the server field.
func (g *gameShell) onlineServerFromPanel() (string, relay.HostedDialOptions, bool) {
	s := g.online
	typed := s.panel.TextOf("SERVER")
	address, options, err := onlineServerAddress(typed)
	if err != nil {
		g.onlineIdleStatus(onlineRefusalText(err))
		return "", options, false
	}
	g.rememberOnlineServer(typed)
	s.detail = "Server " + address
	return address, options, true
}

// prepareOnline composes the battle config describes for seat, prepares it
// and opens the room: a new one carrying encoded for the host, the named one
// for a joiner.
func prepareOnline(ctx context.Context, opts Options, cs *contentSet, cat *content.Catalog, config session.EffectiveMatchConfig, encoded []byte, seat uint8, address, room string, options relay.HostedDialOptions) onlineJobResult {
	sess, identity, err := composeOnlineMatch(cs, cat, config, seat)
	if err != nil {
		return onlineJobResult{err: err}
	}
	if err := ctx.Err(); err != nil {
		return onlineJobResult{err: err}
	}
	// Optional load-time art, prepared here as the loading screen's goroutine
	// prepares it (DESIGN_GPU_RENDERER §14.4).
	detail := detailArtFor(opts, cs, sess.World, nil)
	hello := relay.LocalHello{Seat: seat, Identity: identity, InitialChecksum: sess.UnitStateChecksum()}
	var payload []byte
	if seat == 0 {
		payload = encoded
	}
	dial, cancel := context.WithTimeout(ctx, onlineOpenTimeout)
	defer cancel()
	lobby, err := onlineRelayService.Open(dial, address, room, hello, payload, options)
	if err != nil {
		return onlineJobResult{err: err}
	}
	return onlineJobResult{prepared: &onlinePrepared{sess: sess, config: config, detail: detail, address: address}, lobby: lobby}
}

// startOnlineCreate freezes the host's configuration — its skirmish map, mod,
// mutators and restrictions, Modern rules and a fresh seed pair — and opens a
// room for it (§16.6).
func (g *gameShell) startOnlineCreate() {
	s := g.online
	if s == nil || s.job != nil || s.phase != onlineIdle || !s.stamped {
		return
	}
	address, options, ok := g.onlineServerFromPanel()
	if !ok {
		return
	}
	if err := onlineContentRefusal(g.cs); err != nil {
		g.onlineFail(err)
		return
	}
	if g.cs.mod != nil && matchModOf(g.cs.mod).Archive == ([32]byte{}) {
		// Configuration field 8 names a mod by its archive digest, which a
		// folder install does not have.
		g.onlineFail(localMultiplayerError("mod", "a mod installed from its archive, with an archive digest to send"))
		return
	}
	if strings.TrimSpace(g.setup.MapName) == "" {
		g.onlineIdleStatus("Choose a map first.")
		return
	}
	sim, crt, err := drawOnlineSeeds()
	if err != nil {
		g.onlineFail(err)
		return
	}
	spec := onlineMatchSpec{mapName: g.setup.MapName, simSeed: sim, crtSeed: crt, mutators: g.opts.Mutators, mod: matchModOf(g.cs.mod), community: g.cs.gameplayFeatures}
	restrictions, cs, opts := g.opts.Restrictions, g.cs, g.opts
	s.phase, s.status = onlinePreparing, "Preparing the battle..."
	s.run(func(ctx context.Context) onlineJobResult {
		cat, err := cs.compileCatalog(nil)
		if err != nil {
			return onlineJobResult{err: err}
		}
		if spec.restrictions, err = onlineMatchRestrictions(restrictions, cat); err != nil {
			return onlineJobResult{err: err}
		}
		spec.restrictionSet = restrictions
		schema, err := onlineMapSchema(cs, cat, spec.mapName)
		if err != nil {
			return onlineJobResult{err: err}
		}
		config, err := onlineMatchConfig(spec, cs, schema)
		if err != nil {
			return onlineJobResult{err: err}
		}
		encoded, err := session.EncodeMatchConfig(config)
		if err != nil {
			return onlineJobResult{err: err}
		}
		return prepareOnline(ctx, opts, cs, cat, config, encoded, 0, address, "", options)
	})
	g.refreshOnlinePanel()
}

// startOnlineJoin asks the relay for the typed room's configuration.
func (g *gameShell) startOnlineJoin() {
	s := g.online
	if s == nil || s.job != nil || s.phase != onlineIdle || !s.stamped {
		return
	}
	code, ok := relay.NormalizeRoomCode(s.panel.TextOf("ROOMCODE"))
	if !ok {
		g.onlineIdleStatus("Type the six-character room code the host sent you.")
		return
	}
	address, options, ok := g.onlineServerFromPanel()
	if !ok {
		return
	}
	if err := onlineContentRefusal(g.cs); err != nil {
		g.onlineFail(err)
		return
	}
	s.phase, s.status = onlineDescribing, "Looking for game "+code+"..."
	s.run(func(ctx context.Context) onlineJobResult {
		ctx, cancel := context.WithTimeout(ctx, onlineDescribeTimeout)
		defer cancel()
		config, err := onlineRelayService.Describe(ctx, address, code, options)
		if err == nil && len(config) == 0 {
			err = localMultiplayerError("room configuration", "the host's configuration")
		}
		if err != nil {
			return onlineJobResult{err: err}
		}
		return onlineJobResult{described: &onlineRoom{address: address, options: options, code: code, config: config}}
	})
	g.refreshOnlinePanel()
}

// adoptOnlineRoom checks that this install can play the described room and
// composes for it. The room's map, mod, mutators and restrictions come from
// its configuration, never from this player's preferences. A mod that is
// installed but not mounted is mounted through the ordinary content reload,
// which resumes the join on the new shell (resumeOnlineJoin).
func (g *gameShell) adoptOnlineRoom(room onlineRoom, remounted bool) {
	s := g.online
	if s == nil {
		return
	}
	config, err := session.DecodeMatchConfig(room.config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "nanolathe: online: %v\n", err)
		g.onlineIdleStatus("This game's settings could not be read. The host may be running a different release.")
		return
	}
	r := config.Request()
	s.room = &r
	if want := r.Mod; want != matchModOf(g.cs.mod) {
		if remounted {
			g.onlineIdleStatus("This game's mod " + onlineModName(want) + " could not be selected.")
			return
		}
		selector, refusal := g.onlineModPlan(want)
		if refusal != "" {
			g.onlineIdleStatus(refusal)
			return
		}
		s.phase, s.status = onlinePreparing, "Loading "+onlineModName(want)+" for this game..."
		g.refreshOnlinePanel()
		pendingContentReload = &contentReloadRequest{
			selector: selector,
			mod:      settings.ModSelection{ID: want.ID, Version: want.Version},
			mutators: g.opts.Mutators,
			online:   &room,
		}
		return
	}
	if !g.hasSkirmishMap(r.MapName) {
		g.onlineIdleStatus("This game uses the map " + r.MapName + ", which is not installed. Get it with More maps on the skirmish map screen, then join again.")
		return
	}
	cs, opts := g.cs, g.opts
	s.phase = onlinePreparing
	if !remounted {
		s.status = "Preparing the battle..."
	}
	s.run(func(ctx context.Context) onlineJobResult {
		return prepareOnline(ctx, opts, cs, nil, config, nil, 1, room.address, room.code, room.options)
	})
	g.refreshOnlinePanel()
}

// onlineModPlan is the content reload that mounts want, or why it cannot.
func (g *gameShell) onlineModPlan(want session.MatchMod) (selector, refusal string) {
	if onlineContentRefusal(g.cs) != nil {
		return "", onlineRefusalText(onlineContentRefusal(g.cs))
	}
	name := onlineModName(want)
	if want.ID == "" {
		return "none", ""
	}
	lib, err := openModLibrary()
	if err != nil {
		return "", "This game needs the mod " + name + ", but the mod library is unavailable."
	}
	mod, ok, err := lib.Lookup(want.ID, want.Version)
	if err != nil || !ok {
		return "", "This game needs the mod " + name + ", which is not installed. Install it from NANOLATHE, Mods, then join again."
	}
	if sameMod(&mod, g.cs.mod) || want.Archive != ([32]byte{}) && matchModOf(&mod).Archive != want.Archive {
		return "", "This game needs a different copy of " + name + " than the one installed."
	}
	if missing := unmetModRequirements(g.cs.baseRoots, mod); len(missing) > 0 {
		return "", "This game needs " + name + ", which needs " + missing[0] + " from the base install."
	}
	return modSelectorOf(mod.ID, mod.Version), ""
}

// resumeOnlineJoin continues a join on the shell a content reload built for
// the room's mod.
func (g *gameShell) resumeOnlineJoin(room onlineRoom) {
	if err := g.openOnlineScreen(); err != nil {
		reportRetailMessageError(g.showRetailMessage(err.Error()))
		return
	}
	s := g.online
	s.panel.SetText("ROOMCODE", room.code)
	s.detail = "Server " + room.address
	mod := "the base game"
	if g.cs.mod != nil {
		mod = g.cs.mod.Name + " " + g.cs.mod.Version
	}
	s.status = "Switched to " + mod + " for this game. Preparing the battle..."
	g.adoptOnlineRoom(room, true)
}

// hasSkirmishMap reports whether the census lists name.
func (g *gameShell) hasSkirmishMap(name string) bool {
	for _, m := range g.maps {
		if strings.EqualFold(m, name) {
			return true
		}
	}
	return false
}

// pollOnline takes a finished job and the lobby's latest state. It runs
// every shell step; nothing in it waits.
func (g *gameShell) pollOnline() {
	s := g.online
	if s == nil {
		return
	}
	if job := s.job; job != nil {
		select {
		case r := <-job.done:
			s.job = nil
			job.cancel()
			g.finishOnlineJob(r)
		default:
		}
	}
	if s = g.online; s != nil && s.phase == onlineInLobby && s.lobby != nil {
		g.pollOnlineLobby()
	}
}

func (g *gameShell) finishOnlineJob(r onlineJobResult) {
	switch {
	case r.err != nil:
		g.onlineFail(r.err)
	case r.described != nil:
		g.adoptOnlineRoom(*r.described, false)
	case r.lobby != nil:
		g.openOnlineLobby(r.prepared, r.lobby)
	default:
		g.onlineIdleStatus("")
	}
}

// openOnlineLobby shows the room a job opened.
func (g *gameShell) openOnlineLobby(prepared *onlinePrepared, lobby onlineLobby) {
	s := g.online
	panel, err := g.loadOnlinePanel(true)
	if err != nil || prepared == nil {
		_ = lobby.Close()
		if err == nil {
			err = localMultiplayerError("lobby", "a prepared battle")
		}
		g.onlineFail(err)
		return
	}
	r := prepared.config.Request()
	s.room = &r
	s.lobby, s.prepared, s.ready, s.state = lobby, prepared, false, relay.HostedLobbyState{}
	s.phase, s.status, s.detail = onlineInLobby, "", "Server "+prepared.address
	s.lobbyPanel = panel
	g.frontend.Panels.Push(panel)
	flushWindowTokens(clPtr)
	g.refreshOnlineLobby()
}

// pollOnlineLobby follows the room: a failure returns to the online screen
// with its reason, and Started enters the battle.
func (g *gameShell) pollOnlineLobby() {
	s := g.online
	state, err := s.lobby.State()
	if err != nil {
		g.leaveOnlineLobby(onlineRefusalText(err))
		return
	}
	if state.Started {
		g.startOnlineBattle()
		return
	}
	if state != s.state {
		s.state = state
		g.refreshOnlineLobby()
	}
}

// leaveOnlineLobby closes the room and its prepared battle and uncovers the
// online screen with status.
func (g *gameShell) leaveOnlineLobby(status string) {
	s := g.online
	if s == nil {
		return
	}
	if s.lobby != nil {
		_ = s.lobby.Close()
	}
	s.lobby, s.prepared, s.ready, s.state = nil, nil, false, relay.HostedLobbyState{}
	if s.lobbyPanel != nil && g.frontend.Panels.Top() == s.lobbyPanel {
		s.lobbyPanel.ResetPress()
		g.frontend.Panels.Pop()
	}
	s.lobbyPanel = nil
	flushWindowTokens(clPtr)
	g.onlineIdleStatus(status)
}

func (g *gameShell) setOnlineReady(ready bool) {
	s := g.online
	if s == nil || s.lobby == nil {
		return
	}
	if err := s.lobby.SetReady(ready); err != nil {
		g.leaveOnlineLobby(onlineRefusalText(err))
		return
	}
	s.ready, s.status = ready, ""
	g.refreshOnlineLobby()
}

// startOnlineMatch is the host's Start. A refused start leaves the room open.
func (g *gameShell) startOnlineMatch() {
	s := g.online
	if s == nil || s.lobby == nil || s.lobby.Seat() != 0 {
		return
	}
	if err := s.lobby.Start(); err != nil {
		g.leaveOnlineLobby(onlineRefusalText(err))
		return
	}
	s.status = "Starting..."
	g.refreshOnlineLobby()
}

func (g *gameShell) copyOnlineCode() {
	s := g.online
	if s == nil || s.lobby == nil {
		return
	}
	s.status = "Copied the room code " + s.lobby.Code() + "."
	if !onlineClipboardWrite(s.lobby.Code()) {
		s.status = "The room code could not be copied. It is " + s.lobby.Code() + "."
	}
	g.refreshOnlineLobby()
}

// startOnlineBattle enters the prepared battle with the stream the lobby
// handed over, and closes the online screen behind it.
func (g *gameShell) startOnlineBattle() {
	s := g.online
	conn := s.lobby.Battle()
	if conn == nil {
		g.leaveOnlineLobby("The game could not start.")
		return
	}
	prepared, code := s.prepared, s.lobby.Code()
	if err := g.enterOnlineBattle(prepared, conn, code); err != nil {
		fmt.Fprintf(os.Stderr, "nanolathe: online: %v\n", err)
		g.leaveOnlineLobby("The game could not start: " + noticeReason(err))
		return
	}
	// The battle owns the connection now, and the lobby's Close does nothing.
	// The online windows stay under the battle until it returns to the menu.
	s.lobby, s.prepared = nil, nil
	g.online = nil
}

// enterOnlineBattle adopts a prepared online battle and drives it from conn,
// as the command-line path does after its dial (startHostedMultiplayer). An
// online battle has no opening arrival: the opening holds the pump, and
// grants keep arriving (§16.4.2). The driver is made before the presentation
// so a failure closes the connection with nothing adopted.
func (g *gameShell) enterOnlineBattle(p *onlinePrepared, conn lockstep.Client, code string) error {
	if p == nil || p.sess == nil {
		_ = conn.Close()
		return localMultiplayerError("battle", "a prepared online battle")
	}
	sess := p.sess
	driver, err := lockstep.NewPacedDriver(sess, conn)
	if err != nil {
		_ = conn.Close()
		return err
	}
	if g.audioOwner != nil {
		sess.Audio = g.audioOwner
	}
	// The loading transition's second half: the battle takes the chosen
	// display size before its HUD opens [07 "The loading screen"].
	g.applyDisplayMode(clPtr)
	battle, err := composeBattleEntryDetached(sess, sess.Catalog, g.cs, g, nil)
	if err != nil {
		_ = driver.Close()
		g.applyDisplaySize(clPtr, retailScreenW, retailScreenH)
		return err
	}
	g.importedRetailBattle, g.lastBattleSurvival = false, false
	g.pendingDetail = p.detail
	g.commitBattleCandidate(battle)
	battle.multiplayer = &battleMultiplayer{driver: driver, completed: driver.Completed}
	battle.returnToMenu = g.returnFromOnlineBattle
	battle.returnToSkirmish = g.returnFromOnlineBattle
	if clPtr != nil {
		clPtr.PrepareBattlePresentation()
	}
	battle.onlineNotice(fmt.Sprintf("Online game %s: you are seat %d", code, sess.LocalOwner+1))
	return nil
}

// returnFromOnlineBattle leaves an online battle, or its result, for the
// online screen, saying how the game ended.
func (g *gameShell) returnFromOnlineBattle(cl *client.Client) {
	message := "You left the game."
	if b := g.battle; b != nil {
		switch {
		case b.multiplayer != nil && b.multiplayer.failure != nil:
			message = "The game stopped: " + onlineRefusalText(b.multiplayer.failure)
		case b.sess != nil && b.sess.OnlineBattleEnded():
			message = "The game has ended."
		}
	}
	g.returnFromBattle(cl)
	if err := g.openOnlineScreen(); err != nil {
		reportRetailMessageError(g.showRetailMessage(err.Error()))
		return
	}
	g.online.status = message
	g.refreshOnlinePanel()
}
