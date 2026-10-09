package main

// The MULTI online screen (DESIGN_MULTIPLAYER §16.6.2, DESIGN_INTERFACE_HUD_INPUT
// "Online games"): a server field, Create Game, a room-code field with Join
// Game, and Back. It is a Nanolathe window built on the SELMAP template, like
// the Mods & Mutators screen, pushed over MAINMENU. The lobby it opens is in
// online_lobby.go; the network and composition work in online_flow.go.

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
	"github.com/nanolathe-gg/nanolathe/internal/ui"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// defaultOnlineServer is the server the online screen offers until the
// player names another (DESIGN_MULTIPLAYER §16.6).
const defaultOnlineServer = "relay.nanolathe.gg"

// onlinePlayAvailable reports whether this build has a relay transport. The
// browser build has none, so MULTI stays greyed there (§16.6.2).
func onlinePlayAvailable() bool { return runtime.GOOS != "js" }

// onlineClipboardAvailable reports whether the lobby can offer Copy.
func onlineClipboardAvailable() bool { return ebitenapp.HostClipboardWritable() }

type onlinePhase uint8

const (
	onlineIdle       onlinePhase = iota
	onlineDescribing             // join: reading the room's configuration
	onlineConnecting             // opening the room
	onlineInLobby
)

// onlineScreen is a shell's online screen and, once a room is open, its
// lobby (gameShell.online). The lobby replaces the screen's window; leaving
// it rebuilds the screen.
type onlineScreen struct {
	panel  *ui.Panel
	assets *retailPanelAssets

	phase          onlinePhase
	status, detail string
	job            *onlineJob

	room onlineRoomState
}

func (g *gameShell) onlinePanelActive() bool {
	return g != nil && g.online != nil && g.online.panel != nil && g.activePanel() == g.online.panel
}

// onlinePanelAssets supplies the backdrop of the online screen and its lobby.
func (g *gameShell) onlinePanelAssets(p *ui.Panel) *retailPanelAssets {
	if g == nil || g.online == nil || p == nil {
		return nil
	}
	switch p {
	case g.online.panel:
		return g.online.assets
	case g.online.room.panel:
		return g.online.room.assets
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
	case has("unoccupied"):
		return "That game is full or has already started."
	case has("logical path room,", "an open room"):
		return "That game has closed."
	case has("logical path room host,"):
		return "The host left, so the game closed."
	case has("logical path room wait,"):
		return "The game closed after waiting 30 minutes without starting."
	case has("logical path room capacity,"), has("logical path connection capacity,"):
		return "The server is full. Try again later."
	case has("logical path hello "):
		rest := text[strings.Index(text, "logical path hello ")+len("logical path hello "):]
		field, _, _ := strings.Cut(rest, ",")
		return "Your game differs from the host's (" + field + ")."
	case has("hosted protocol version"):
		return "The server speaks a different online protocol. Both players and the server need matching releases."
	case has("logical path mod,", "archive digest"):
		return "Online games need a mod installed from its archive, which identifies it to the other players. Reinstall this mod from its zip."
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
// The window.

const (
	onlineTitle     = "PLAY ONLINE"
	onlineCodeFont  = "fonts/hatt14.fnt"
	onlineServerMax = 100
	onlineCodeMax   = 16
)

// buildOnlineWindow shapes a SELMAP clone into the online screen. The map
// list, its scrollbar and the map picture go; the two action buttons and the
// two detail labels keep their authored rectangles. The left frame holds the
// two fields and the right frame the help.
func buildOnlineWindow(window *gui.Window) {
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
	textBox := func(name string, x, y, w int32, maxChars int16, attribs uint32) {
		t := label
		t.Kind, t.Name, t.SourceName, t.Text = gui.KindTextBox, name, name, ""
		t.Rect.X, t.Rect.Y, t.Rect.W, t.Rect.H = x, y, w, 20
		t.Attribs, t.MaxChars = attribs, maxChars
		kept = append(kept, t)
	}
	caption("NTITLE", onlineTitle, 60, 44, 240, 18)
	caption("SERVERLABEL", "Server", 70, 94, 210, 16)
	// Attribute 1 paints the editor's background. The server field leaves
	// attribute 2 clear so ':', '/' and '.' can be typed; the code field
	// admits letters, digits and spaces.
	textBox("SERVER", 68, 110, 214, onlineServerMax, 0x01)
	caption("CODELABEL", "Room code", 70, 142, 210, 16)
	textBox("ROOMCODE", 68, 158, 110, onlineCodeMax, 0x03)
	caption("HELP", "", 352, 150, 116, 118)
	b := action
	b.Name, b.SourceName, b.Text, b.QuickKey = "CREATE", "CREATE", "Create Game", 0
	b.Rect.X, b.Rect.Y = 357, 86
	kept = append(kept, b)
	window.Gadgets = kept
}

// onlineTemplate loads the SELMAP template window and the Mods & Mutators
// backdrop from the base install, so the screen keeps one layout whichever
// mod is running: a mod may ship its own map-select window. Tests substitute
// an authored window.
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

func (g *gameShell) openOnlineScreenReporting() {
	if err := g.openOnlineScreen(); err != nil {
		reportRetailMessageError(g.showRetailMessage(err.Error()))
	}
}

// openOnlineScreen pushes the online screen over the main menu.
func (g *gameShell) openOnlineScreen() error {
	if g == nil || g.cs == nil || g.cs.fs == nil {
		return fmt.Errorf("nanolathe: online screen: no mounted content")
	}
	if g.online != nil {
		g.closeOnlineScreen()
	}
	window, background, err := onlineTemplate(g)
	if err != nil {
		return err
	}
	buildOnlineWindow(window)
	g.installRetailWindowButtonArt(window, nil)
	g.initializeRetailLabels(window)
	panel := ui.NewPanel(window)
	if panel == nil {
		return fmt.Errorf("nanolathe: online screen: panel construction failed")
	}
	g.online = &onlineScreen{panel: panel, assets: &retailPanelAssets{window: window, background: background}}
	server := g.onlineServer
	if server == "" {
		server = defaultOnlineServer
	}
	panel.SetText("SERVER", server)
	g.frontend.Panels.Push(panel)
	flushWindowTokens(clPtr)
	panel.FocusEditor(panel.Index("ROOMCODE"))
	g.refreshOnlinePanel()
	return nil
}

// closeOnlineScreen leaves the online screen. A running job is abandoned (its
// room, if it opens one, is closed) and an open lobby is left.
func (g *gameShell) closeOnlineScreen() {
	s := g.online
	if s == nil {
		return
	}
	s.cancelJob()
	g.closeOnlineRoom()
	if g.frontend != nil && s.panel != nil && g.frontend.Panels.Top() == s.panel {
		g.frontend.Panels.Pop()
	}
	g.online = nil
	flushWindowTokens(clPtr)
}

// refreshOnlinePanel writes the online screen's labels and greys what the
// current phase cannot do.
func (g *gameShell) refreshOnlinePanel() {
	s := g.online
	if s == nil || s.panel == nil {
		return
	}
	p := s.panel
	busy := s.phase != onlineIdle
	status := s.status
	if status == "" {
		status = "Create a game, or type the room code you were sent and join."
	}
	p.SetText("HELP", "Create Game opens a room on this server and gives you a code to send. To join, type the code and choose Join Game.")
	p.SetText("DESCRIPTION", g.fitDetail(status, 230, 2))
	p.SetText("SIZE", g.fitDetail(s.detail, 230, 1))
	p.SetText("LOAD", "Join Game")
	p.SetText("PREVMENU", "Back")
	retailGreyGadget(p.Window, "CREATE", busy)
	retailGreyGadget(p.Window, "LOAD", busy)
}

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
	if p == s.room.panel && s.room.lobby != nil {
		g.drawOnlineRoomCode(c, p)
	}
}

// drawOnlineRoomCode draws the lobby's room code in the larger face, in the
// GUI's heading colour, spaced and split in two groups of three so it can be
// read aloud.
func (g *gameShell) drawOnlineRoomCode(c *client.Client, p *ui.Panel) {
	room := &g.online.room
	i := p.Index(lobbyCode)
	if i < 0 {
		return
	}
	font := room.codeFont
	if font == nil {
		font = g.font
	}
	if font == nil {
		return
	}
	r := p.Window.PlacedRect(i)
	color := g.guiColor(onlineHeadingColor)
	step := client.MeasureText(font, "W") + 6
	x := int(r.X)
	for n, ch := range room.lobby.Code() {
		if n == 3 {
			x += step / 2
		}
		c.UITextWidth(font, string(ch), x, int(r.Y), -1, color)
		x += step
	}
}

// onlineHeadingColor is the GUI colour field the room code is drawn in: the
// SELMAP window's own heading colour, the gold of the Mods & Mutators list
// headings.
const onlineHeadingColor = 0xcd

// activateOnlineScreenGadget routes a button on the online screen.
func (g *gameShell) activateOnlineScreenGadget(name string) bool {
	if !g.onlinePanelActive() {
		return false
	}
	switch name {
	case "CREATE":
		g.playMenuCue("SmallButton")
		g.startOnlineCreate()
	case "LOAD", "ROOMCODE":
		g.playMenuCue("SmallButton")
		g.startOnlineJoin()
	case "PREVMENU":
		g.playMenuCue("SmallButton")
		g.closeOnlineScreen()
	}
	return true
}

// activateOnlineGadget routes a button on the online screen or its lobby.
func (g *gameShell) activateOnlineGadget(name string) bool {
	return g.activateOnlineLobbyGadget(name) || g.activateOnlineScreenGadget(name)
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
