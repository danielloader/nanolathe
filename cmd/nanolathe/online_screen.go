package main

// The MULTI online screen and its lobby (DESIGN_MULTIPLAYER §16.6.2,
// DESIGN_INTERFACE_HUD_INPUT "Online games"). Both are Nanolathe windows built
// on the SELMAP template, like the Mods & Mutators screen, pushed over
// MAINMENU. Network work and battle composition run off the game goroutine;
// the shell polls their results from its step (online_flow.go).

import (
	"errors"
	"fmt"
	"net"
	"runtime"
	"strings"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/client"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/gui"
	"github.com/nanolathe-gg/nanolathe/internal/platform/ebitenapp"
	"github.com/nanolathe-gg/nanolathe/internal/relay"
	"github.com/nanolathe-gg/nanolathe/internal/session"
	"github.com/nanolathe-gg/nanolathe/internal/ui"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// defaultOnlineServer is the server the online screen offers until the
// player names another (DESIGN_MULTIPLAYER §16.6).
const defaultOnlineServer = "relay.nanolathe.gg"

// onlinePlayAvailable reports whether this build has a relay transport. The
// browser build has none, so MULTI stays greyed there (§16.6.2).
func onlinePlayAvailable() bool { return runtime.GOOS != "js" }

type onlinePhase uint8

const (
	onlineIdle       onlinePhase = iota
	onlineDescribing             // join: reading the room's configuration
	onlinePreparing              // composing the battle and opening the lobby
	onlineInLobby
)

// onlineScreen is a shell's open online screen (gameShell.online). The lobby
// panel is pushed over the screen's own panel while a room is open.
type onlineScreen struct {
	panel, lobbyPanel *ui.Panel
	assets            *retailPanelAssets
	codeFont          *formats.FNT

	phase          onlinePhase
	status, detail string
	stamped        bool
	choosingMap    bool
	// room is the configuration of the room being joined or hosted, shown in
	// the summary; nil shows the host's own selection.
	room *session.MatchConfigRequest

	job      *onlineJob
	lobby    onlineLobby
	prepared *onlinePrepared
	ready    bool
	state    relay.HostedLobbyState
}

func (g *gameShell) onlinePanelActive() bool {
	return g != nil && g.online != nil && g.online.panel != nil && g.activePanel() == g.online.panel
}

func (g *gameShell) onlineLobbyActive() bool {
	return g != nil && g.online != nil && g.online.lobbyPanel != nil && g.activePanel() == g.online.lobbyPanel
}

// onlinePanelAssets supplies the backdrop of either online window.
func (g *gameShell) onlinePanelAssets(p *ui.Panel) *retailPanelAssets {
	if g == nil {
		return nil
	}
	if g.online != nil && p != nil && (p == g.online.panel || p == g.online.lobbyPanel) {
		return g.online.assets
	}
	return nil
}

// onlineServerAddress turns the typed server into a relay address: a bare
// host is the WebSocket relay at wss://host/relay; host:port is native TLS;
// a wss:// URL is taken as typed. A ws:// URL is accepted only on a numeric
// loopback address, as an explicit choice of plaintext for a relay on this
// computer. Everything else follows the command line's validation
// (validateHostedAddress).
func onlineServerAddress(typed string) (string, relay.HostedDialOptions, error) {
	address := strings.TrimSpace(typed)
	if address == "" {
		address = defaultOnlineServer
	}
	if !strings.Contains(address, "://") {
		if _, _, err := net.SplitHostPort(address); err != nil {
			address = "wss://" + address + "/relay"
		}
	}
	options := relay.HostedDialOptions{InsecureLoopback: strings.HasPrefix(strings.ToLower(address), "ws://")}
	if err := validateHostedAddress(Options{RelayAddress: address, RelayInsecureLoopback: options.InsecureLoopback}); err != nil {
		return "", options, err
	}
	return address, options, nil
}

// onlineRefusalText says in plain words why the relay or this client refused,
// for the status line. Unrecognised failures keep their own short reason.
func onlineRefusalText(err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	var restricted *content.RestrictionsError
	has := func(parts ...string) bool {
		for _, part := range parts {
			if !strings.Contains(text, part) {
				return false
			}
		}
		return true
	}
	switch {
	case has("logical path room code,"):
		return "Room codes are six letters and digits."
	case has("logical path room,", "existing invitation"):
		return "No game has that code on this server. Check the code and the server."
	case has("unoccupied second seat"):
		return "That game is full or has already started."
	case has("logical path room,", "an open room"):
		return "That game has closed."
	case has("logical path room host,"):
		return "The host left, so the game closed."
	case has("logical path room wait,"):
		return "The game closed after waiting 30 minutes without starting."
	case has("logical path room capacity,"), has("logical path connection capacity,"):
		return "The server is full. Try again later."
	case has("logical path hello build,"):
		return "Your Nanolathe build differs from the host's. Both players need the same release."
	case has("logical path hello content,"):
		return "Your game files differ from the host's, so the game would not play the same."
	case has("logical path hello map,"):
		return "Your copy of the map differs from the host's."
	case has("logical path hello mod,"):
		return "Your copy of the mod differs from the host's."
	case has("logical path hello "):
		rest := text[strings.Index(text, "logical path hello ")+len("logical path hello "):]
		field, _, _ := strings.Cut(rest, ",")
		return "Your game differs from the host's (" + field + ")."
	case has("hosted protocol version"):
		return "The server speaks a different online protocol. Both players and the server need matching releases."
	case has("logical path binary,", "stamped"):
		return "Online play needs a stamped release build."
	case has("logical path mod,", "archive digest"):
		return "Online games need a mod installed from its archive, which identifies it to the other player. Reinstall this mod from its zip."
	case has("logical path mounted content,"):
		return "Online games need the base game or an installed mod. Restart without extra --root or --mod-config content."
	case errors.As(err, &restricted):
		return "The unit restrictions cannot be applied: " + onlineRestrictionIssues(restricted) + "."
	case has("logical path relay address,"), has("logical path relay URL,"), has("logical path WebSocket transport,"), has("logical path plaintext relay,"):
		return "The server address is not valid. Use a host name such as " + defaultOnlineServer + ", host:port, or wss://host/relay."
	case has("logical path hosted dial,"), has("logical path WebSocket"), has("logical path hosted handshake,"), has("logical path envelope length,"):
		return "Could not reach the server: " + onlineCause(text)
	case has("logical path lobby,"):
		return "You left the game."
	}
	return noticeReason(err)
}

// onlineRestrictionIssues names each refused restriction and why.
func onlineRestrictionIssues(e *content.RestrictionsError) string {
	parts := make([]string, len(e.Issues))
	for i, issue := range e.Issues {
		parts[i] = issue.Unit + " (" + restrictionReasonText(issue.Reason) + ")"
	}
	return strings.Join(parts, ", ")
}

// onlineCause is the last clause of a transport failure: the network's own
// words after the diagnostic's expectation.
func onlineCause(text string) string {
	if i := strings.LastIndex(text, "message: "); i >= 0 {
		return strings.TrimSpace(text[i+len("message: "):])
	}
	if i := strings.LastIndex(text, ": "); i >= 0 {
		return strings.TrimSpace(text[i+2:])
	}
	return text
}

// ---------------------------------------------------------------------------
// Windows.

const (
	onlineTitle      = "PLAY ONLINE"
	onlineLobbyTitle = "ONLINE GAME"
	onlineCodeFont   = "fonts/hatt14.fnt"
	onlineServerMax  = 100
	onlineCodeMax    = 16
	onlineSummary    = 4
)

// buildOnlineWindow shapes a SELMAP clone into the online screen or its
// lobby. The map list, its scrollbar and the map picture go; the two action
// buttons and the two detail labels keep their authored rectangles, and the
// left frame holds the screen's fields. The right frame summarises the game.
func buildOnlineWindow(window *gui.Window, lobby bool) {
	var kept []gui.Gadget
	var action, label gui.Gadget
	for _, gad := range window.Gadgets {
		switch gad.Name {
		case "MAPPIC", "MAPNAMES", "SLIDER":
			continue
		case "LOAD", "PREVMENU":
			// Escape finds PREVMENU by name; the authored letters would fire
			// these buttons from keys the fields could be typing.
			gad.QuickKey = 0
			if gad.Name == "LOAD" {
				action = gad
			}
		case "DESCRIPTION":
			label = gad
		}
		kept = append(kept, gad)
	}
	caption := func(name, text string, x, y, w, h int32) {
		c := label
		c.Name, c.SourceName, c.Text = name, name, text
		c.Rect.X, c.Rect.Y, c.Rect.W, c.Rect.H = x, y, w, h
		kept = append(kept, c)
	}
	button := func(name, text string, x, y int32) {
		b := action
		b.Name, b.SourceName, b.Text, b.QuickKey = name, name, text, 0
		b.Rect.X, b.Rect.Y = x, y
		kept = append(kept, b)
	}
	textBox := func(name string, x, y, w int32, maxChars int16, attribs uint32) {
		t := label
		t.Kind, t.Name, t.SourceName, t.Text = gui.KindTextBox, name, name, ""
		t.Rect.X, t.Rect.Y, t.Rect.W, t.Rect.H = x, y, w, 20
		t.Attribs, t.MaxChars = attribs, maxChars
		kept = append(kept, t)
	}
	title := onlineTitle
	if lobby {
		title = onlineLobbyTitle
	}
	caption("NTITLE", title, 60, 44, 240, 18)
	caption("SUMLABEL", "", 352, 150, 116, 16)
	for i := 0; i < onlineSummary; i++ {
		caption(fmt.Sprintf("SUM%d", i), "", 352, 166+int32(i)*14, 118, 14)
	}
	if lobby {
		caption("CODELABEL", "Room code", 70, 94, 210, 16)
		// The code itself is drawn in a larger face over this empty label
		// (drawOnlineRoomCode), which lends it its rectangle and pen.
		caption("CODE", "", 70, 114, 210, 24)
		button("COPY", "Copy", 70, 146)
		caption("SEATLABEL", "Players", 70, 182, 210, 16)
		caption("SEAT0", "", 70, 198, 210, 16)
		caption("SEAT1", "", 70, 214, 210, 16)
		caption("YOU", "", 70, 240, 210, 16)
		button("READY", "Ready", 357, 86)
	} else {
		caption("SERVERLABEL", "Server", 70, 94, 210, 16)
		// Attribute 1 paints the editor's background. The server field
		// leaves attribute 2 clear so ':', '/' and '.' can be typed; the
		// code field admits letters, digits and spaces.
		textBox("SERVER", 68, 110, 214, onlineServerMax, 0x01)
		caption("CODELABEL", "Room code", 70, 142, 210, 16)
		textBox("ROOMCODE", 68, 158, 110, onlineCodeMax, 0x03)
		caption("HELP", "", 70, 192, 210, 80)
		button("CREATE", "Create Game", 357, 86)
		button("CHANGEMAP", "Change map", 357, 240)
	}
	window.Gadgets = kept
}

// onlineTemplate loads the SELMAP template window and the Mods & Mutators
// backdrop from the base install, so the online windows keep one layout
// whichever mod is running: a mod may ship its own map-select window. Tests
// substitute an authored window.
var onlineTemplate = func(g *gameShell) (*gui.Window, *formats.PCX, error) {
	var from vfs.FSOps = g.cs.fs
	if base := vfs.New(); base.MountGameDirectories(g.cs.baseRoots) == nil {
		defer base.Close()
		from = base
	} else {
		base.Close()
	}
	window, err := gui.LoadWithTranslation(from, modsTemplateGUI, g.cs.translations)
	if err != nil {
		return nil, nil, retailFrontendAssetError(g.cs, "online screen GUI unavailable", modsTemplateGUI, "the authored map-select window used as the template", err)
	}
	background, err := modsBackdrop(from)
	if err != nil {
		return nil, nil, retailFrontendAssetError(g.cs, "online screen bitmap", modsTemplateBackdrop, "the authored map-select backdrop", err)
	}
	return window, background, nil
}

// loadOnlinePanel builds one online window on the template.
func (g *gameShell) loadOnlinePanel(lobby bool) (*ui.Panel, error) {
	window, background, err := onlineTemplate(g)
	if err != nil {
		return nil, err
	}
	if g.online != nil && g.online.assets == nil {
		g.online.assets = &retailPanelAssets{window: window, background: background}
	}
	buildOnlineWindow(window, lobby)
	g.installRetailWindowButtonArt(window, nil)
	g.initializeRetailLabels(window)
	panel := ui.NewPanel(window)
	if panel == nil {
		return nil, fmt.Errorf("nanolathe: online screen: panel construction failed")
	}
	return panel, nil
}

func (g *gameShell) openOnlineScreenReporting() {
	if err := g.openOnlineScreen(); err != nil {
		reportRetailMessageError(g.showRetailMessage(err.Error()))
	}
}

// openOnlineScreen pushes the online screen over the main menu. An
// unstamped build is told at once that it cannot play online.
func (g *gameShell) openOnlineScreen() error {
	if g == nil || g.cs == nil || g.cs.fs == nil {
		return fmt.Errorf("nanolathe: online screen: no mounted content")
	}
	if g.online != nil {
		g.closeOnlineScreen()
	}
	state := &onlineScreen{}
	if build, err := currentBuildManifest(); err == nil && build.Stamped() {
		state.stamped = true
	}
	g.online = state
	panel, err := g.loadOnlinePanel(false)
	if err != nil {
		g.online = nil
		return err
	}
	if font, err := formats.LoadFNTFile(g.cs.fs, onlineCodeFont); err == nil {
		state.codeFont = font
	}
	state.panel = panel
	server := g.onlineServer
	if server == "" {
		server = defaultOnlineServer
	}
	panel.SetText("SERVER", server)
	g.frontend.Panels.Push(panel)
	flushWindowTokens(clPtr)
	if state.stamped {
		panel.FocusEditor(panel.Index("ROOMCODE"))
	}
	g.refreshOnlinePanel()
	return nil
}

// closeOnlineScreen leaves the online screen. A running job is abandoned (its
// lobby, if it opens one, is closed) and an open lobby is left.
func (g *gameShell) closeOnlineScreen() {
	s := g.online
	if s == nil {
		return
	}
	s.cancelJob()
	if s.lobby != nil {
		_ = s.lobby.Close()
		s.lobby = nil
	}
	s.prepared = nil
	if g != nil && g.frontend != nil {
		for _, p := range []*ui.Panel{s.lobbyPanel, s.panel} {
			if p != nil && g.frontend.Panels.Top() == p {
				g.frontend.Panels.Pop()
			}
		}
	}
	g.online = nil
	flushWindowTokens(clPtr)
}

// onlineSummaryLines describe the game: the room's when one is known, the
// host's own selection otherwise.
func (g *gameShell) onlineSummaryLines() (string, []string) {
	s := g.online
	if s != nil && s.room != nil {
		r := s.room
		lines := []string{r.MapName, onlineModName(r.Mod), onlineMutatorLine(mutatorCount(r.Mutators))}
		// Field 12 has a record per copy of a duplicated name; the player
		// restricted names.
		units := map[string]bool{}
		for _, record := range r.UnitRestrictions {
			units[record.Unit] = true
		}
		if n := len(units); n > 0 {
			lines = append(lines, restrictionCountText(n))
		}
		return "This game", lines
	}
	mod := session.MatchMod{}
	if g.cs != nil {
		mod = matchModOf(g.cs.mod)
	}
	mapName := g.setup.MapName
	if mapName == "" {
		mapName = "No map chosen"
	}
	lines := []string{mapName, onlineModName(mod), onlineMutatorLine(mutatorCount(g.opts.Mutators))}
	if n := len(g.opts.Restrictions.Entries()); n > 0 {
		lines = append(lines, restrictionCountText(n))
	}
	return "You would host", lines
}

func onlineMutatorLine(n int) string {
	switch n {
	case 0:
		return "No mutators"
	case 1:
		return "1 mutator"
	}
	return fmt.Sprintf("%d mutators", n)
}

func (g *gameShell) fillOnlineSummary(p *ui.Panel) {
	heading, lines := g.onlineSummaryLines()
	p.SetText("SUMLABEL", heading)
	for i := 0; i < onlineSummary; i++ {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		p.SetText(fmt.Sprintf("SUM%d", i), g.fitDetail(line, 118, 1))
	}
}

// refreshOnlinePanel writes the online screen's labels and greys what the
// current phase cannot do.
func (g *gameShell) refreshOnlinePanel() {
	s := g.online
	if s == nil || s.panel == nil {
		return
	}
	p := s.panel
	g.fillOnlineSummary(p)
	busy := s.phase != onlineIdle
	status := s.status
	if status == "" {
		status = "Create a game, or type the room code you were sent and join."
		if !s.stamped {
			status = "Online play needs a stamped release build. This build is unstamped."
		}
	}
	p.SetText("HELP", "Create Game hosts your game on this server and gives you a room code to send. To join, type the code and choose Join Game.")
	p.SetText("DESCRIPTION", g.fitDetail(status, 230, 2))
	p.SetText("SIZE", g.fitDetail(s.detail, 230, 1))
	p.SetText("LOAD", "Join Game")
	p.SetText("PREVMENU", "Back")
	retailGreyGadget(p.Window, "CREATE", busy || !s.stamped)
	retailGreyGadget(p.Window, "LOAD", busy || !s.stamped)
	retailGreyGadget(p.Window, "CHANGEMAP", busy || len(g.maps) == 0)
}

// refreshOnlineLobby writes the lobby: both seats, this seat's readiness and
// what the host may do.
func (g *gameShell) refreshOnlineLobby() {
	s := g.online
	if s == nil || s.lobbyPanel == nil || s.lobby == nil {
		return
	}
	p, st, seat := s.lobbyPanel, s.state, s.lobby.Seat()
	g.fillOnlineSummary(p)
	seatLine := func(name string, i int) string {
		switch {
		case !st.Present[i]:
			return name + ": waiting to join"
		case st.Ready[i]:
			return name + ": ready"
		}
		return name + ": not ready"
	}
	p.SetText("SEAT0", seatLine("Host", 0))
	p.SetText("SEAT1", seatLine("Guest", 1))
	you := "You are the host"
	if seat != 0 {
		you = "You are the guest"
	}
	p.SetText("YOU", you)
	ready := "Ready"
	if s.ready {
		ready = "Not ready"
	}
	p.SetText("READY", ready)
	p.SetText("LOAD", "Start")
	p.SetText("PREVMENU", "Leave")
	both := st.Present[0] && st.Present[1] && st.Ready[0] && st.Ready[1]
	status := s.status
	if status == "" {
		switch {
		case seat == 0 && !st.Present[1]:
			status = "Send the room code to the other player."
		case seat == 0 && both:
			status = "Both players are ready. Choose Start."
		case seat == 0:
			status = "When you are both ready, choose Start."
		case both:
			status = "Waiting for the host to start the game."
		default:
			status = "Choose Ready when you are ready."
		}
	}
	p.SetText("DESCRIPTION", g.fitDetail(status, 230, 2))
	p.SetText("SIZE", g.fitDetail(s.detail, 230, 1))
	p.SetActive("COPY", onlineClipboardAvailable())
	retailGreyGadget(p.Window, "LOAD", seat != 0 || !both)
}

// onlineClipboardAvailable reports whether the lobby can offer Copy.
func onlineClipboardAvailable() bool { return ebitenapp.HostClipboardWritable() }

// drawOnlineExtras paints what the online windows add to the authored
// painters: an empty, uncaptured field's background, which the text-box
// painter skips with its text, and the lobby's room code.
func (g *gameShell) drawOnlineExtras(c *client.Client, p *ui.Panel) {
	s := g.online
	if s == nil || p == nil || p.Window == nil {
		return
	}
	if p == s.panel {
		for _, name := range []string{"SERVER", "ROOMCODE"} {
			i := p.Index(name)
			if i >= 0 && p.TextAt(i) == "" && !(p.EditorCaptured() && p.EditorIndex() == i) {
				r := p.Window.PlacedRect(i)
				c.UIFillRect(int(r.X), int(r.Y), int(r.W), int(r.H), 0)
			}
		}
		return
	}
	if p == s.lobbyPanel && s.lobby != nil {
		g.drawOnlineRoomCode(c, p)
	}
}

// drawOnlineRoomCode draws the lobby's room code in the larger face, in the
// window's heading colour, spaced and split in two groups of three so it can
// be read aloud.
func (g *gameShell) drawOnlineRoomCode(c *client.Client, p *ui.Panel) {
	s := g.online
	i := p.Index("CODE")
	if i < 0 || len(p.Window.Gadgets) == 0 {
		return
	}
	font := s.codeFont
	if font == nil {
		font = g.font
	}
	if font == nil {
		return
	}
	r := p.Window.PlacedRect(i)
	color := g.guiColor(byte(p.Window.Gadgets[0].ColorF & 0xff))
	step := client.MeasureText(font, "W") + 6
	x := int(r.X)
	for n, ch := range s.lobby.Code() {
		if n == 3 {
			x += step / 2
		}
		c.UITextWidth(font, string(ch), x, int(r.Y), -1, color)
		x += step
	}
}

// activateOnlineGadget routes a button on either online window.
func (g *gameShell) activateOnlineGadget(name string) bool {
	switch {
	case g.onlineLobbyActive():
		g.playMenuCue("SmallButton")
		switch name {
		case "READY":
			g.setOnlineReady(!g.online.ready)
		case "LOAD":
			g.startOnlineMatch()
		case "COPY":
			g.copyOnlineCode()
		case "PREVMENU":
			g.leaveOnlineLobby("You left the game.")
		}
		return true
	case g.onlinePanelActive():
		switch name {
		case "CREATE":
			g.playMenuCue("SmallButton")
			g.startOnlineCreate()
		case "LOAD", "ROOMCODE":
			g.playMenuCue("SmallButton")
			g.startOnlineJoin()
		case "CHANGEMAP":
			g.playMenuCue("SmallButton")
			g.openOnlineMapPicker()
		case "PREVMENU":
			g.playMenuCue("SmallButton")
			g.closeOnlineScreen()
		}
		return true
	}
	return false
}

// openOnlineMapPicker opens the ordinary map selector over the online screen;
// its choice is the skirmish map, which Create hosts.
func (g *gameShell) openOnlineMapPicker() {
	s := g.online
	if s == nil || s.phase != onlineIdle {
		return
	}
	if len(g.maps) == 0 {
		s.status = "There are no multiplayer maps to choose from."
		g.refreshOnlinePanel()
		return
	}
	s.choosingMap = true
	g.syncMapIndex()
	g.mapReturn = modeMenuMain
	g.openMenu(modeMenuMap)
}

// leaveMapPicker is SELMAP's PREVMENU and LOAD: back to the screen that
// opened it. The online screen sits under the selector, so it is uncovered
// rather than rebuilt.
func (g *gameShell) leaveMapPicker() {
	if s := g.online; s != nil && s.choosingMap {
		s.choosingMap = false
		if top := g.frontend.Panels.Top(); top != nil && top != s.panel {
			top.ResetPress()
			g.frontend.Panels.Pop()
		}
		g.frontend.SetMode(modeMenuMain)
		flushWindowTokens(clPtr)
		g.refreshOnlinePanel()
		return
	}
	g.openMenu(g.mapReturn)
}

// rememberOnlineServer keeps the typed server for the next visit.
func (g *gameShell) rememberOnlineServer(typed string) {
	typed = strings.TrimSpace(typed)
	if typed == defaultOnlineServer {
		typed = ""
	}
	if typed != g.onlineServer {
		g.onlineServer = typed
		g.saveSettings()
	}
}
