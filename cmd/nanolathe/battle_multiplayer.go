package main

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"strings"
	"time"

	"github.com/nanolathe-gg/nanolathe/internal/client"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/headless"
	"github.com/nanolathe-gg/nanolathe/internal/lockstep"
	"github.com/nanolathe-gg/nanolathe/internal/netproto"
	"github.com/nanolathe-gg/nanolathe/internal/relay"
	"github.com/nanolathe-gg/nanolathe/internal/session"
)

// The host alone owns transport. Neither a local menu nor a wall-clock budget
// can release a tick of this two-human play test (DESIGN_MULTIPLAYER §16.4.2).
type localBattleDriver interface {
	Pump() (bool, error)
	Submit(session.HumanCommand) (uint64, error)
	Close() error
}

type battleMultiplayer struct {
	driver    localBattleDriver
	relay     *relay.LocalRelay
	failure   error
	completed func() bool // Hosted results wait for explicit relay agreement.
}

func (o Options) localMultiplayer() bool    { return o.LocalMPListen != "" || o.LocalMPJoin != "" }
func (o Options) multiplayerPlaytest() bool { return o.localMultiplayer() || o.RelayAddress != "" }

func localMultiplayerError(path, expected string) error {
	return fmt.Errorf("nanolathe: multiplayer play test refused: logical path %s, providers searched [two-human play test], expected %s", path, expected)
}

func validateLocalMultiplayerOptions(o Options) error {
	if o.LocalMPCommandDelayMS < 0 || o.LocalMPCommandDelayMS > 1000 || o.LocalMPCommandDelayMS != 0 && o.LocalMPListen == "" {
		return localMultiplayerError("command delay", "0..1000 milliseconds on the local listener")
	}
	if o.RelayAddress == "" && (o.RelayRoom != "" || o.RelayCA != "" || o.RelayInsecureLoopback) {
		return localMultiplayerError("relay options", "--relay-address host:port")
	}
	if !o.multiplayerPlaytest() {
		return nil
	}
	if o.LocalMPListen != "" && o.LocalMPJoin != "" {
		return localMultiplayerError("command line", "one of --local-mp-listen or --local-mp-join")
	}
	if o.RelayAddress != "" {
		if o.localMultiplayer() {
			return localMultiplayerError("transport", "one local or hosted relay")
		}
		if err := validateHostedAddress(o); err != nil {
			return err
		}
	} else {
		address := o.LocalMPListen
		if address == "" {
			address = o.LocalMPJoin
		}
		addr, err := netip.ParseAddrPort(address)
		if err != nil || !addr.Addr().IsLoopback() || addr.Addr().Zone() != "" || addr.Port() == 0 {
			return localMultiplayerError(address, "a numeric loopback address and nonzero port, for example 127.0.0.1:39031")
		}
	}
	if strings.TrimSpace(o.Map) == "" {
		return localMultiplayerError("map", "--map naming the same skirmish map on both clients")
	}
	if o.GameplaySet && o.Gameplay != gameplay.Modern || len(o.GameplayOverrides) != 0 || len(o.ComputerAI) != 0 || len(o.AIArgs) != 0 || o.UnitLimit != 0 {
		return localMultiplayerError("configuration", "the fixed Modern two-human configuration without AI, gameplay overrides or unit-limit overrides")
	}
	if o.Mod != "" && o.Mod != "none" || o.ModConfig != "" || o.InstallMod != "" || len(o.Roots) > 1 {
		return localMultiplayerError("content", "base content without mods, a config or a manual root stack")
	}
	if len(o.MutatorArgs) != 0 || !o.Mutators.IsZero() || !o.Restrictions.IsZero() {
		return localMultiplayerError("content transformations", "no mutators or unit restrictions")
	}
	if o.Survival || o.Mission != "" || o.LoadSave != "" || o.Headless || o.ListInstalls || o.CheckInstall || o.Ticks != 0 || o.Report != "" || o.Shot != "" || o.ShotModel != "" || o.ShotDebris != "" || o.ShotUnitViewer != "" || o.Film != "" || o.NLShot != "" || o.WalkPreview != "" || o.LiveTrace != "" || o.LiveScene != "" || o.LiveSpeed != 0 || o.BattleBenchmark != "" || o.BenchmarkCapture != "" {
		return localMultiplayerError("entry", "an ordinary map window without Survival, mission, save, headless, capture or benchmark options")
	}
	return nil
}

// Reject explicit probe options even when their argument happens to equal its
// default. Presentation-only window options remain available.
func localMultiplayerFlagAllowed(name string) bool {
	for _, prefix := range []string{"shot", "film", "nl-shot", "walk-preview", "live-", "benchmark", "survival"} {
		if strings.HasPrefix(name, prefix) {
			return false
		}
	}
	switch name {
	case "battle-benchmark", "headless", "ticks", "mission", "difficulty", "load-save", "report", "list-installs", "check-install", "install-mod", "mutator", "restrict", "ai", "ai-player", "gameplay-feature", "unit-limit", "cpuprofile", "memprofile", "profile-seconds":
		return false
	}
	return true
}

// localMultiplayerConfig is the command-line play test's configuration: the
// --map of both clients, seeds 7/11 or both --seed, and base content with no
// transformations (DESIGN_MULTIPLAYER §16.4.2).
func localMultiplayerConfig(o Options, cs *contentSet, schema uint32) (session.EffectiveMatchConfig, error) {
	spec := onlineMatchSpec{mapName: o.Map, simSeed: 7, crtSeed: 11}
	if o.Seed >= 0 {
		spec.simSeed, spec.crtSeed = uint32(o.Seed), uint32(o.Seed)
	}
	return onlineMatchConfig(spec, cs, schema)
}

// composeLocalMultiplayer composes the command-line play test's battle,
// prepared for its first grant, and the identity its hello reports.
func composeLocalMultiplayer(o Options, cs *contentSet) (headless.FreshBattle, netproto.Identity, error) {
	var empty headless.FreshBattle
	var identity netproto.Identity
	if err := validateLocalMultiplayerOptions(o); err != nil {
		return empty, identity, err
	}
	if cs == nil || cs.fs == nil {
		return empty, identity, unavailableBattleContentError()
	}
	if cs.mod != nil || cs.config != nil || cs.manualRoots {
		return empty, identity, localMultiplayerError("mounted content", "base content with no mod or config")
	}
	// Any build may play: command-line rooms start without a rehearsal, and
	// the 30-tick unit checksum stops a match whose seats diverge.
	cat, err := cs.compileCatalog(nil)
	if err != nil {
		return empty, identity, err
	}
	schema, err := onlineMapSchema(cs, cat, o.Map)
	if err != nil {
		return empty, identity, err
	}
	config, err := localMultiplayerConfig(o, cs, schema)
	if err != nil {
		return empty, identity, err
	}
	var seat uint8
	if o.LocalMPJoin != "" || o.RelayRoom != "" {
		seat = 1
	}
	sess, _, identity, err := composeOnlineMatch(cs, cat, config, seat)
	if err != nil {
		return empty, identity, err
	}
	return headless.FreshBattle{Session: sess, Kind: headless.ScenarioSkirmish}, identity, nil
}

// startLocalMultiplayer connects a battle composeLocalMultiplayer already
// prepared and entered.
func (b *battleSession) startLocalMultiplayer(o Options, identity netproto.Identity) error {
	if o.RelayAddress != "" {
		return b.startHostedMultiplayer(o, identity)
	}
	mp := &battleMultiplayer{}
	b.multiplayer = mp
	address := o.LocalMPJoin
	if o.LocalMPListen != "" {
		var r *relay.LocalRelay
		var err error
		if o.LocalMPCommandDelayMS == 0 {
			r, err = relay.ListenLocal(o.LocalMPListen)
		} else {
			r, err = relay.ListenLocalWithCommandDelay(o.LocalMPListen, time.Duration(o.LocalMPCommandDelayMS)*time.Millisecond)
		}
		if err != nil {
			return err
		}
		mp.relay, address = r, r.Addr()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	connection, err := relay.DialLocal(ctx, address, relay.LocalHello{Seat: b.sess.LocalOwner, Identity: identity, InitialChecksum: b.sess.UnitStateChecksum()})
	if err != nil {
		mp.close()
		return err
	}
	mp.driver, err = lockstep.NewLocalDriver(b.sess, connection)
	if err != nil {
		_ = connection.Close()
		mp.close()
		return err
	}
	b.onlineNotice(fmt.Sprintf("Local multiplayer seat %d: waiting for both clients at %s", b.sess.LocalOwner+1, address))
	if o.LocalMPCommandDelayMS != 0 {
		b.onlineNotice(fmt.Sprintf("Responsiveness test: +%d ms order delay for both seats; network timing unchanged", o.LocalMPCommandDelayMS))
	}
	return nil
}

func (b *battleSession) onlineBattle() bool {
	return b != nil && (b.multiplayer != nil || b.sess != nil && b.sess.OnlineCommandContext())
}

func (m *battleMultiplayer) close() {
	if m == nil {
		return
	}
	if m.driver != nil {
		_ = m.driver.Close()
		m.driver = nil
	}
	if m.relay != nil {
		_ = m.relay.Close()
		m.relay = nil
	}
}

func (b *battleSession) onlineNotice(message string) {
	fmt.Fprintln(os.Stderr, message)
	if ring := b.messageRing(); ring != nil {
		ring.Append(message, 4, 0, 10, b.currentTick())
	}
}

func (b *battleSession) pumpLocalMultiplayer(cl *client.Client) {
	if b == nil || b.multiplayer == nil || b.multiplayer.driver == nil || b.multiplayer.failure != nil {
		return
	}
	advanced, err := b.multiplayer.driver.Pump()
	for _, receipt := range b.sess.DrainCommandReceipts() {
		if receipt.Stamp.Seat == b.sess.LocalOwner && receipt.Outcome == session.CommandRejected {
			b.onlineNotice(receipt.Diagnostic)
		}
	}
	if err != nil {
		b.multiplayer.failure = err
		b.multiplayer.close()
		b.onlineNotice("Multiplayer stopped: " + err.Error())
	}
	if advanced {
		b.applyPublishedCamera(b.sess.Snapshot.Current())
		b.syncLocalInterface()
		b.noteTickTiming()
		if cl != nil {
			cl.ObserveCommittedTick()
			cl.NoteTicksReleased(1)
		}
	}
}

func (b *battleSession) submitLocalMultiplayer(c session.HumanCommand) (uint64, error) {
	if b.multiplayer == nil || b.multiplayer.driver == nil {
		return 0, localMultiplayerError("command", "a connected, running local relay")
	}
	return b.multiplayer.driver.Submit(c)
}
