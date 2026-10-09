package main

// The online lobby (DESIGN_MULTIPLAYER §16.6, §16.6.2): the room code, a row
// per present player with their team, side, colour and readiness, the host's
// settings until Start, Ready, Start and Leave. It is the authored SKIRMISH.GUI
// window, whose ten runtime rows, side art, colours and allegiance symbols
// already fit a ten-player room, opened as the front end's skirmish screen.
// Its own controls replace the setup screen's handlers while it is open.

import (
	"fmt"
	"strconv"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/client"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/gui"
	"github.com/nanolathe-gg/nanolathe/internal/relay"
	"github.com/nanolathe-gg/nanolathe/internal/session"
	"github.com/nanolathe-gg/nanolathe/internal/survival"
	"github.com/nanolathe-gg/nanolathe/internal/ui"
)

// The lobby's own controls, added to the SKIRMISH.GUI clone.
const (
	lobbyGameType  = "GAMETYPE"
	lobbyGameLabel = "GAMELABEL"
	lobbyLocLabel  = "LOCLABEL"
	lobbyAirLabel  = "AIRLABEL"
	lobbySeaLabel  = "SEALABEL"
	lobbyReady     = "READY"
	lobbyCopy      = "COPY"
	lobbyRoomLabel = "ROOMLABEL"
	lobbyCode      = "CODE"
	lobbyReadyHead = "READYHEAD"
	lobbyStatus    = "HELPTEXT"
)

// onlineRoomState is the open room: the relay's lobby, the room's latest base
// configuration, and this seat's readiness.
type onlineRoomState struct {
	lobby    onlineLobby
	panel    *ui.Panel
	assets   *retailPanelAssets
	codeFont *formats.FNT
	address  string
	// cat is the unrestricted catalog this install compiled for the room:
	// its map headers, its capacity and the final composition read it.
	cat *content.Catalog

	base        session.EffectiveMatchConfig
	baseReq     session.MatchConfigRequest
	baseVersion uint32
	settings    onlineSettings
	mapMissing  bool
	capacity    int
	state       relay.HostedLobbyState
	// notice is a one-off status line, such as why readiness was cleared;
	// a later change replaces it.
	notice      string
	choosingMap bool

	ready    bool
	readyJob *onlineReadyJob
	prepared *onlinePrepared
	readyErr error
}

// onlineRoomKey is what the final configuration depends on: the base
// configuration and every seat's presence, team, side and colour. A prepared
// battle is valid only while the key is unchanged.
type onlineRoomKey struct {
	base  [32]byte
	seats [relay.HostedMaxSeats]relay.HostedSeatState
}

func (r *onlineRoomState) key() onlineRoomKey {
	k := onlineRoomKey{base: r.base.Digest()}
	for i, seat := range r.state.Seats {
		if seat.Present {
			k.seats[i] = relay.HostedSeatState{Present: true, Team: seat.Team, Side: seat.Side, Color: seat.Color}
		}
	}
	return k
}

func (g *gameShell) onlineLobbyActive() bool {
	return g != nil && g.online != nil && g.online.room.panel != nil && g.activePanel() == g.online.room.panel
}

// onlineSideCount is how many sides a player may choose: the room's catalog's
// (session.OnlineSides), or the setup screen's count before one is compiled.
func (g *gameShell) onlineSideCount() int {
	if g.online != nil && g.online.room.cat != nil {
		if n := len(session.OnlineSides(g.online.room.cat)); n > 0 {
			return n
		}
	}
	return g.skirmishSideCount()
}

// isHost reports whether this seat created the room.
func (r *onlineRoomState) isHost() bool { return r.lobby != nil && r.lobby.Seat() == 0 }

// presentSeats are the seats present now, in ascending seat order.
func (r *onlineRoomState) presentSeats() []int {
	var seats []int
	for i, seat := range r.state.Seats {
		if seat.Present {
			seats = append(seats, i)
		}
	}
	return seats
}

// buildOnlineLobbyWindow shapes a SKIRMISH.GUI clone, with its ten runtime
// rows installed, into the lobby. Difficulty becomes the game type, Survival's
// controls are the Survival setup screen's clones, the metal column shows
// readiness, and the room code takes the free space beside the title.
func buildOnlineLobbyWindow(window *gui.Window) {
	index := func(name string) int { return window.GadgetIndex(name) }
	labelOf := func(text string) int {
		for i, gad := range window.Gadgets {
			if gad.Kind == gui.KindLabel && gad.Text == text {
				return i
			}
		}
		return -1
	}
	start, mapping, difficulty, selectMap := index("StartLocation"), index("Mapping"), index("Difficulty"), index("SelectMap")
	location, diffLabel := labelOf("Location"), labelOf("Difficulty")
	if start < 0 || mapping < 0 || difficulty < 0 || selectMap < 0 || location < 0 || diffLabel < 0 {
		return
	}
	clone := func(template gui.Gadget, name, stages string, n uint8, help string) gui.Gadget {
		b := template
		b.Name, b.SourceName, b.Art = name, survivalSource, template.Name
		b.Text, b.Labels, b.Stages, b.Help = stages, splitStages(stages), n, help
		return b
	}
	label := func(name, text string, x, y, w int32) gui.Gadget {
		l := window.Gadgets[location]
		l.Name, l.SourceName, l.Text = name, survivalSource, text
		l.Rect.X, l.Rect.Y, l.Rect.W = x, y, w
		return l
	}
	button := func(name, text string, x, y int32) gui.Gadget {
		b := window.Gadgets[selectMap]
		b.Name, b.SourceName, b.Art, b.Text, b.QuickKey = name, survivalSource, "SelectMap", text, 0
		b.Rect.X, b.Rect.Y = x, y
		return b
	}
	game := clone(window.Gadgets[mapping], lobbyGameType, "Skirmish|Survival", 2, "Skirmish, or Survival against endless waves.")
	game.Rect = window.Gadgets[difficulty].Rect
	window.Gadgets[difficulty].Active = 0
	window.Gadgets[diffLabel].Name, window.Gadgets[diffLabel].Text = lobbyGameLabel, "Game"
	window.Gadgets[location].Name = lobbyLocLabel
	pace := clone(window.Gadgets[difficulty], survivalPaceButton, "Normal|Relaxed|Relentless", 3, "How quickly the waves grow.")
	pace.Rect = window.Gadgets[start].Rect
	added := []gui.Gadget{game, pace}
	const x0, pitch, labelY, buttonY = 45, 130, 290, 306
	for col, c := range []struct{ caption, name, labelName, help string }{
		{"Air Waves", survivalAirButton, lobbyAirLabel, "Whether waves may be airborne."},
		{"Naval Waves", survivalSeaButton, lobbySeaLabel, "Whether waves may come by sea."},
	} {
		b := clone(window.Gadgets[mapping], c.name, "On|Off", 2, c.help)
		b.Rect.X, b.Rect.Y = int32(x0+col*pitch), buttonY
		added = append(added, label(c.labelName, c.caption, int32(x0+col*pitch), labelY, 117), b)
	}
	added = append(added,
		label(lobbyRoomLabel, "Room code", 300, 12, 160),
		label(lobbyCode, "", 300, 28, 170),
		label(lobbyReadyHead, "Ready", 287, 58, 60),
		button(lobbyCopy, "Copy", 480, 24),
		button(lobbyReady, "Ready", 263, 437),
	)
	window.Gadgets = append(window.Gadgets, added...)
	if i := index("PrevMenu"); i >= 0 {
		window.Gadgets[i].Text, window.Gadgets[i].Labels = "Leave", nil
	}
}

func splitStages(stages string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(stages); i++ {
		if i == len(stages) || stages[i] == '|' {
			out = append(out, stages[start:i])
			start = i + 1
		}
	}
	return out
}

// lobbyBackdrop copies the setup screen's backdrop and paints plain texture
// over its Metal and Energy headings, which the lobby does not use; the Ready
// heading is drawn in their place. The Color heading stays over its column.
func lobbyBackdrop(source *formats.PCX) *formats.PCX {
	if source == nil {
		return nil
	}
	pcx := *source
	pcx.Pixels = append([]byte(nil), source.Pixels...)
	width := int(pcx.Width)
	patch := func(fromX, toX, y, w, h int) {
		for row := 0; row < h; row++ {
			src := (y+row)*width + fromX
			dst := (y+row)*width + toX
			if src >= 0 && dst >= 0 && src+w <= len(pcx.Pixels) && dst+w <= len(pcx.Pixels) {
				copy(pcx.Pixels[dst:dst+w], source.Pixels[src:src+w])
			}
		}
	}
	// The plain strip between the Energy heading and the Commander box
	// supplies the texture.
	const y, h = 50, 18
	patch(395, 283, y, 52, h)
	patch(395, 335, y, 52, h)
	return &pcx
}

// openOnlineLobby shows the room a job opened: the setup screen's window in
// the skirmish mode, replacing the online screen.
func (g *gameShell) openOnlineLobby(lobby onlineLobby, cat *content.Catalog, base session.EffectiveMatchConfig, address string) {
	s := g.online
	asset := g.assets.panel[modeMenuSkirmish]
	if asset == nil || asset.window == nil {
		_ = lobby.Close()
		g.onlineFail(fmt.Errorf("nanolathe: online lobby: logical path guis/skirmish.gui, providers searched [vfs], expected the authored setup window"))
		return
	}
	window := gui.CloneWindow(asset.window)
	slots := make([]ui.SkirmishSlot, relay.HostedMaxSeats)
	for i := range slots {
		slots[i] = ui.SkirmishSlot{SideStages: uint8(g.skirmishSideCount())}
	}
	ui.InstallSkirmishDynamicGadgets(window, slots)
	buildOnlineLobbyWindow(window)
	g.installRetailWindowButtonArt(window, asset.art)
	g.installRetailListScrollbars(window, asset.art)
	g.initializeRetailLabels(window)
	panel := ui.NewPanel(window)
	if panel == nil {
		_ = lobby.Close()
		g.onlineFail(fmt.Errorf("nanolathe: online lobby: panel construction failed"))
		return
	}
	room := onlineRoomState{lobby: lobby, panel: panel, address: address, cat: cat,
		assets: &retailPanelAssets{window: window, background: lobbyBackdrop(asset.background), art: asset.art}}
	if font, err := formats.LoadFNTFile(g.cs.fs, onlineCodeFont); err == nil {
		room.codeFont = font
	}
	s.room, s.phase, s.panel, s.status = room, onlineInLobby, nil, ""
	g.adoptOnlineBase(base)
	g.frontend.Open(modeMenuSkirmish, panel, false)
	flushWindowTokens(clPtr)
	g.refreshOnlineLobby()
}

// adoptOnlineBase makes base the room's configuration: its settings, whether
// this install has its map, and how many players the map seats. The mod is
// the room's for good.
func (g *gameShell) adoptOnlineBase(base session.EffectiveMatchConfig) {
	r := &g.online.room
	r.base, r.baseReq = base, base.Request()
	r.settings = onlineSettingsOf(r.baseReq)
	r.mapMissing = !g.hasSkirmishMap(r.baseReq.MapName)
	r.capacity = 0
	if !r.mapMissing && r.cat != nil {
		if n, err := onlineMapCapacity(r.cat, r.baseReq.MapName); err == nil {
			r.capacity = n
		}
	}
}

// closeOnlineRoom leaves the room: its lobby closes and any check stops.
func (g *gameShell) closeOnlineRoom() {
	if g.online == nil {
		return
	}
	r := &g.online.room
	if r.readyJob != nil {
		r.readyJob.cancel()
	}
	if r.lobby != nil {
		_ = r.lobby.Close()
	}
	g.online.room = onlineRoomState{}
}

// leaveOnlineLobby leaves the room for the online screen, which says why.
func (g *gameShell) leaveOnlineLobby(status string) {
	if g.online == nil {
		return
	}
	detail := "Server " + g.online.room.address
	g.closeOnlineRoom()
	g.online = nil
	g.openMenu(modeMenuMain)
	if err := g.openOnlineScreen(); err != nil {
		reportRetailMessageError(g.showRetailMessage(err.Error()))
		return
	}
	g.online.status, g.online.detail = status, detail
	g.refreshOnlinePanel()
}

// pollOnlineLobby follows the room each step: a failure returns to the online
// screen with its reason, a new base configuration is adopted, cleared
// readiness is explained, a finished check reports Ready, and Started enters
// the battle.
func (g *gameShell) pollOnlineLobby() {
	r := &g.online.room
	state, err := r.lobby.State()
	if err != nil {
		g.leaveOnlineLobby(onlineRefusalText(err))
		return
	}
	if state.Started {
		g.startOnlineBattle(state)
		return
	}
	changed := state != r.state
	if state.ConfigVersion != r.baseVersion {
		r.baseVersion = state.ConfigVersion
		// The host already holds what it sent; an echo of an earlier change
		// must not undo a later one.
		if bytes := r.lobby.Configuration(); len(bytes) != 0 && !r.isHost() {
			base, err := session.DecodeMatchConfig(bytes)
			if err != nil || base.Request().Mod != r.baseReq.Mod {
				g.leaveOnlineLobby("The host's game settings could not be read. The host may be running a different release.")
				return
			}
			if base.Digest() != r.base.Digest() {
				g.adoptOnlineBase(base)
				changed = true
			}
		}
	}
	if changed {
		local := r.lobby.Seat()
		if r.ready && int(local) < len(state.Seats) && !state.Seats[local].Ready {
			r.ready, r.notice = false, onlineReadyClearedReason(r.state, state)
		}
		r.state = state
	}
	if job := r.readyJob; job != nil {
		select {
		case result := <-job.done:
			r.readyJob = nil
			g.finishOnlineReady(result)
			changed = true
		default:
		}
	}
	if changed {
		g.refreshOnlineLobby()
	}
}

// onlineReadyClearedReason says why the relay cleared every seat's ready.
func onlineReadyClearedReason(before, after relay.HostedLobbyState) string {
	if before.ConfigVersion != after.ConfigVersion {
		return "The host changed the game settings. Choose Ready again."
	}
	for i := range before.Seats {
		if before.Seats[i].Present != after.Seats[i].Present {
			return "A player joined or left. Choose Ready again."
		}
	}
	for i := range before.Seats {
		b, a := before.Seats[i], after.Seats[i]
		if b.Team != a.Team || b.Side != a.Side || b.Color != a.Color {
			return "A player changed team, side or colour. Choose Ready again."
		}
	}
	return "A player joined, left or changed team, side or colour. Choose Ready again."
}

// onlineLobbyBlock is why the room cannot become ready now, for every seat,
// or "" when it can.
func (g *gameShell) onlineLobbyBlock() string {
	r := &g.online.room
	seats, _, _ := onlineSeatsOf(r.state, r.lobby.Seat(), r.settings.survival)
	switch {
	case r.mapMissing:
		return "The host chose " + r.baseReq.MapName + ", which you don't have."
	case len(seats) < 2:
		if r.isHost() {
			return "Send the room code to the other players."
		}
		return "Waiting for another player."
	case r.settings.survival && len(seats) > session.OnlineSurvivalMaxSurvivors:
		return fmt.Sprintf("Survival allows up to %d players.", session.OnlineSurvivalMaxSurvivors)
	case !r.settings.survival && r.capacity > 0 && len(seats) > r.capacity:
		return fmt.Sprintf("This map supports %d players.", r.capacity)
	case !r.settings.survival && onlineOneTeam(seats):
		return "Everyone is on one team. At least two teams, or players without a team, are needed."
	}
	return ""
}

// onlineCanStart reports the host's Start: at least two present, every one
// ready with agreeing digests, and nothing blocking.
func (g *gameShell) onlineCanStart() bool {
	r := &g.online.room
	if !r.isHost() || r.state.Mismatch || g.onlineLobbyBlock() != "" {
		return false
	}
	for _, i := range r.presentSeats() {
		if !r.state.Seats[i].Ready {
			return false
		}
	}
	return true
}

// refreshOnlineLobby writes the lobby: the rows, the host's settings, the
// status line and what each button may do now.
func (g *gameShell) refreshOnlineLobby() {
	s := g.online
	if s == nil || s.room.panel == nil || s.room.lobby == nil {
		return
	}
	r := &s.room
	p := r.panel
	local, host := r.lobby.Seat(), r.isHost()
	present := r.presentSeats()
	editable := !r.ready && r.readyJob == nil
	sides := g.onlineSideCount()
	for row := 0; row < relay.HostedMaxSeats; row++ {
		suffix := strconv.Itoa(row)
		shown := row < len(present)
		for _, name := range []string{"Player", "Side", "Color", "Allies", "Metal"} {
			p.SetActive(name+suffix, shown)
		}
		p.SetActive("Energy"+suffix, false)
		if !shown {
			continue
		}
		seatIndex := present[row]
		seat := r.state.Seats[seatIndex]
		caption := "Player " + strconv.Itoa(row+1)
		switch {
		case uint8(seatIndex) == local:
			caption += " (You)"
		case seatIndex == 0:
			caption += " (Host)"
		}
		p.SetText("Player"+suffix, caption)
		p.SetStatus("Player"+suffix, 0)
		p.SetStatus("Side"+suffix, 0)
		p.SetStageAt(p.Index("Side"+suffix), clampMenuStage(int(seat.Side), sides))
		p.SetStatus("Color"+suffix, int(seat.Color))
		p.SetStatus("Allies"+suffix, onlineTeamIcon(r.state, present, seatIndex))
		p.SetActive("Allies"+suffix, !r.settings.survival)
		p.SetActive("Metal"+suffix, seat.Ready)
		p.SetText("Metal"+suffix, "Ready")
		own := uint8(seatIndex) == local
		switch {
		case own && editable:
			p.SetHelp("Side"+suffix, "Click to choose your side.")
			p.SetHelp("Color"+suffix, "Click to choose your colour; right-click to step back.")
			p.SetHelp("Allies"+suffix, "Click to choose your team. Teammates are allies and share sight.")
		case own:
			p.SetHelp("Side"+suffix, "Choose Not ready to change your side.")
			p.SetHelp("Color"+suffix, "Choose Not ready to change your colour.")
			p.SetHelp("Allies"+suffix, "Choose Not ready to change your team.")
		default:
			p.SetHelp("Side"+suffix, "This player's side.")
			p.SetHelp("Color"+suffix, "This player's colour.")
			p.SetHelp("Allies"+suffix, "This player's team.")
		}
		help := ""
		switch {
		case seatIndex == 0 && own:
			help = "You are the host: you choose the settings and start the game."
		case seatIndex == 0:
			help = "The host chooses the settings and starts the game."
		case own:
			help = "This is you."
		}
		p.SetHelp("Player"+suffix, help)
	}
	g.refreshOnlineLobbySettings(host)
	p.SetText("MapName", r.baseReq.MapName)
	p.SetActive(lobbyCopy, onlineClipboardAvailable())
	ready := "Ready"
	if r.ready {
		ready = "Not ready"
	}
	p.SetText(lobbyReady, ready)
	block := g.onlineLobbyBlock()
	retailGreyGadget(p.Window, lobbyReady, !r.ready && (r.readyJob != nil || block != ""))
	retailGreyGadget(p.Window, "Start", !g.onlineCanStart())
	g.setOnlineLobbyStatus(g.onlineLobbyStatus(block))
}

// refreshOnlineLobbySettings writes the host's settings with the setup
// screen's captions and help; only the host may change them.
func (g *gameShell) refreshOnlineLobbySettings(host bool) {
	r := &g.online.room
	p := r.panel
	set := r.settings
	p.SetActive("StartLocation", !set.survival)
	p.SetActive(survivalPaceButton, set.survival)
	for _, name := range []string{survivalAirButton, survivalSeaButton, lobbyAirLabel, lobbySeaLabel} {
		p.SetActive(name, set.survival)
	}
	p.SetText(lobbyLocLabel, "Location")
	if set.survival {
		p.SetText(lobbyLocLabel, "Wave Pace")
	}
	p.SetStageAt(p.Index(lobbyGameType), boolInt(set.survival))
	if set.location == 0 {
		p.SetStageAt(p.Index("StartLocation"), 1)
		p.SetHelp("StartLocation", "Commanders are randomly placed on the battle field.")
	} else {
		p.SetStageAt(p.Index("StartLocation"), 0)
		p.SetHelp("StartLocation", "Commanders are placed at pre-determined locations.")
	}
	if set.commanderDeath == 0 {
		p.SetStageAt(p.Index("CommanderDeath"), 1)
		p.SetHelp("CommanderDeath", "Game continues after Commander is destroyed.")
	} else {
		p.SetStageAt(p.Index("CommanderDeath"), 0)
		p.SetHelp("CommanderDeath", "Game ends when commander is destroyed.")
	}
	if set.mapping == 0 {
		p.SetStageAt(p.Index("Mapping"), 1)
		p.SetHelp("Mapping", "Terrain is visible.")
	} else {
		p.SetStageAt(p.Index("Mapping"), 0)
		p.SetHelp("Mapping", "Terrain is blacked out until explored.")
	}
	switch {
	case set.lineOfSight == 0:
		p.SetStageAt(p.Index("LineOfSight"), 0)
		p.SetHelp("LineOfSight", "All mapped terrain is visible.")
	case set.losType == 1:
		p.SetStageAt(p.Index("LineOfSight"), 1)
		p.SetHelp("LineOfSight", "Terrain elevations affect a unit's view.")
	default:
		p.SetStageAt(p.Index("LineOfSight"), 2)
		p.SetHelp("LineOfSight", "Terrain elevations do not affect a unit's view.")
	}
	p.SetStageAt(p.Index(survivalPaceButton), clampMenuStage(int(set.pace), 3))
	p.SetStageAt(p.Index(survivalAirButton), boolInt(set.noAir))
	p.SetStageAt(p.Index(survivalSeaButton), boolInt(set.noNaval))
	for _, name := range []string{lobbyGameType, "StartLocation", "CommanderDeath", "Mapping", "LineOfSight", survivalPaceButton, survivalAirButton, survivalSeaButton, "SelectMap"} {
		retailGreyGadget(p.Window, name, !host)
	}
}

// onlineTeamIcon is a row's allegiance symbol, as the setup screen draws an
// alliance: no team is blank, and teams 1..5 are the five symbols, joined
// when shared and split when alone (retailAllyIconFrame) [07 §4].
func onlineTeamIcon(state relay.HostedLobbyState, present []int, seat int) int {
	const blank = 10
	team := int(state.Seats[seat].Team)
	if team == 0 || team >= onlineTeams {
		return blank
	}
	shared := 0
	for _, i := range present {
		if int(state.Seats[i].Team) == team {
			shared++
		}
	}
	group := team - 1
	if shared > 1 {
		return group * 2
	}
	return group*2 + 1
}

// onlineLobbyStatus is the status line, most pressing first.
func (g *gameShell) onlineLobbyStatus(block string) string {
	r := &g.online.room
	switch {
	case r.mapMissing:
		return block
	case r.state.Mismatch:
		return "Your games simulate differently. Every player needs the same version."
	case r.readyErr != nil:
		return "The check of your game failed: " + onlineRefusalText(r.readyErr)
	case r.readyJob != nil:
		return "Checking that your game matches..."
	case block != "":
		return block
	case r.notice != "":
		return r.notice
	}
	everyone := true
	for _, i := range r.presentSeats() {
		everyone = everyone && r.state.Seats[i].Ready
	}
	switch {
	case r.isHost() && everyone:
		return "Everyone is ready. Choose Start."
	case r.isHost():
		return "When everyone is ready, choose Start."
	case everyone:
		return "Waiting for the host to start the game."
	}
	return "Choose your team, side and colour, then Ready."
}

// setOnlineLobbyStatus keeps the status for the hover help to fall back to.
func (g *gameShell) setOnlineLobbyStatus(status string) {
	r := &g.online.room
	r.panel.SetHelp(lobbyStatus, status)
	r.panel.SetText(lobbyStatus, status)
}

// updateOnlineLobbyHelp shows the hovered control's help, or the status line.
func (g *gameShell) updateOnlineLobbyHelp(x, y int32) {
	p := g.online.room.panel
	help := ""
	if i := p.HitTest(x, y); i >= 0 {
		help = p.HelpAt(i)
	}
	if help == "" {
		help = p.HelpOf(lobbyStatus)
	}
	p.SetText(lobbyStatus, help)
}

// activateOnlineLobbyGadget routes a control of the lobby.
func (g *gameShell) activateOnlineLobbyGadget(name string) bool {
	if !g.onlineLobbyActive() {
		return false
	}
	r := &g.online.room
	if r.readyJob == nil && r.ready && name != lobbyReady && name != lobbyCopy && name != "PrevMenu" && name != "Start" {
		// A ready seat changes nothing; the relay would clear every seat.
		return true
	}
	g.playMenuCue("SmallButton")
	if slot, kind, ok := dynamicSlot(name); ok {
		g.activateOnlineRow(slot, kind, 1)
		return true
	}
	set := r.settings
	switch name {
	case lobbyReady:
		g.setOnlineReady(!r.ready)
		return true
	case lobbyCopy:
		g.copyOnlineCode()
		return true
	case "Start":
		g.startOnlineMatch()
		return true
	case "PrevMenu":
		g.leaveOnlineLobby("You left the game.")
		return true
	case "SelectMap":
		g.openOnlineMapPicker()
		return true
	case lobbyGameType:
		set.survival = !set.survival
	case "StartLocation":
		set.location ^= 1
	case "CommanderDeath":
		set.commanderDeath ^= 1
	case "Mapping":
		set.mapping ^= 1
	case "LineOfSight":
		set.lineOfSight, set.losType = nextOnlineLineOfSight(set.lineOfSight, set.losType)
	case survivalPaceButton:
		set.pace = survival.Pace((int(set.pace) + 1) % 3)
	case survivalAirButton:
		set.noAir = !set.noAir
	case survivalSeaButton:
		set.noNaval = !set.noNaval
	default:
		return true
	}
	g.pushOnlineSettings(set)
	return true
}

// nextOnlineLineOfSight is the setup screen's line-of-sight cycle
// (cycleLineOfSight): True, Circular, Permanent.
func nextOnlineLineOfSight(lineOfSight, losType uint8) (uint8, uint8) {
	switch {
	case lineOfSight == 0:
		return 1, 1
	case losType == 1:
		return 1, 0
	}
	return 0, 1
}

// activateOnlineColorBack is a right click on a row's colour: the local
// player's own colour steps back while not ready, as the setup screen's
// right click does [08 R-SKIR-01 §1].
func (g *gameShell) activateOnlineColorBack(row int) {
	if !g.onlineLobbyActive() || g.online.room.readyJob == nil && g.online.room.ready {
		return
	}
	g.playMenuCue("SmallButton")
	g.activateOnlineRow(row, "Color", -1)
}

// activateOnlineRow cycles the local player's own side, colour or team while
// not ready; another player's row does nothing. delta steps the colour, +1
// for a left click and -1 for a right click.
func (g *gameShell) activateOnlineRow(row int, kind string, delta int) {
	r := &g.online.room
	present := r.presentSeats()
	if row >= len(present) || uint8(present[row]) != r.lobby.Seat() || r.ready || r.readyJob != nil {
		return
	}
	seat := r.state.Seats[present[row]]
	var err error
	switch kind {
	case "Side":
		err = r.lobby.SetSide(uint8((int(seat.Side) + 1) % g.onlineSideCount()))
	case "Color":
		next, ok := nextOnlineColor(r.state, present[row], delta)
		if !ok {
			return
		}
		err = r.lobby.SetColor(next)
	case "Allies":
		if r.settings.survival {
			return
		}
		err = r.lobby.SetTeam((seat.Team + 1) % onlineTeams)
	default:
		return
	}
	if err != nil {
		r.notice = onlineRefusalText(err)
	}
	g.refreshOnlineLobby()
}

// nextOnlineColor is the setup screen's colour step for the room: one logo
// frame per click, +1 or -1 modulo the room's colours, stepping again past a
// colour another present seat holds [08 R-SKIR-01 §1]. A room never has more
// seats than colours, so a free one is always found; false means none is.
func nextOnlineColor(state relay.HostedLobbyState, seat, delta int) (uint8, bool) {
	const colors = relay.HostedColors
	candidate := int(state.Seats[seat].Color)
	for range colors {
		candidate = ((candidate+delta)%colors + colors) % colors
		if !onlineColorHeld(state, seat, uint8(candidate)) {
			return uint8(candidate), uint8(candidate) != state.Seats[seat].Color
		}
	}
	return 0, false
}

// onlineColorHeld reports whether a present seat other than seat holds color.
func onlineColorHeld(state relay.HostedLobbyState, seat int, color uint8) bool {
	for i, other := range state.Seats {
		if i != seat && other.Present && other.Color == color {
			return true
		}
	}
	return false
}

// pushOnlineSettings is the host replacing the room's base configuration with
// new settings, which every seat adopts. Its frozen part — seeds, mod,
// mutators and restrictions — is unchanged.
func (g *gameShell) pushOnlineSettings(next onlineSettings) {
	r := &g.online.room
	if !r.isHost() || r.cat == nil {
		return
	}
	base, err := onlineConfig(g.cs, r.cat, next, onlinePlaceholderSeats(), onlineFrozenOf(r.baseReq))
	if err == nil {
		var encoded []byte
		if encoded, err = session.EncodeMatchConfig(base); err == nil {
			if err = r.lobby.SetConfiguration(encoded); err == nil {
				g.adoptOnlineBase(base)
				r.notice = ""
			}
		}
	}
	if err != nil {
		r.notice = "That setting could not be used: " + onlineRefusalText(err)
	}
	g.refreshOnlineLobby()
}

// openOnlineMapPicker opens the ordinary map selector over the lobby; its
// choice is the skirmish map, which the room then plays.
func (g *gameShell) openOnlineMapPicker() {
	r := &g.online.room
	if !r.isHost() {
		return
	}
	if len(g.maps) == 0 {
		r.notice = "There are no multiplayer maps to choose from."
		g.refreshOnlineLobby()
		return
	}
	r.choosingMap = true
	g.setup.MapName = r.settings.mapName
	g.syncMapIndex()
	g.mapReturn = modeMenuSkirmish
	g.openMenu(modeMenuMap)
}

// leaveMapPicker is SELMAP's PREVMENU and LOAD: back to the screen that
// opened it. The lobby sits under the selector, so it is uncovered rather
// than rebuilt, and a new map becomes the room's.
func (g *gameShell) leaveMapPicker() {
	if s := g.online; s != nil && s.room.choosingMap {
		r := &s.room
		r.choosingMap = false
		if top := g.frontend.Panels.Top(); top != nil && top != r.panel {
			top.ResetPress()
			g.frontend.Panels.Pop()
		}
		g.frontend.SetMode(modeMenuSkirmish)
		flushWindowTokens(clPtr)
		if g.setup.MapName != r.settings.mapName {
			next := r.settings
			next.mapName = g.setup.MapName
			g.pushOnlineSettings(next)
			return
		}
		g.refreshOnlineLobby()
		return
	}
	g.openMenu(g.mapReturn)
}

// copyOnlineCode copies the room code where the host has a clipboard.
func (g *gameShell) copyOnlineCode() {
	r := &g.online.room
	r.notice = "Copied the room code " + r.lobby.Code() + "."
	if !onlineClipboardWrite(r.lobby.Code()) {
		r.notice = "The room code could not be copied. It is " + r.lobby.Code() + "."
	}
	g.refreshOnlineLobby()
}

// startOnlineMatch is the host's Start. A refused start leaves the room open.
func (g *gameShell) startOnlineMatch() {
	r := &g.online.room
	if !g.onlineCanStart() {
		return
	}
	if err := r.lobby.Start(); err != nil {
		g.leaveOnlineLobby(onlineRefusalText(err))
		return
	}
	r.notice = "Starting..."
	g.refreshOnlineLobby()
}

// drawOnlineLobbyHelp feeds the status line from the hovered control's help.
func (g *gameShell) drawOnlineLobbyHelp(c *client.Client) {
	mouse, _ := c.Input().PointerSample()
	g.updateOnlineLobbyHelp(int32(mouse.X), int32(mouse.Y))
}
