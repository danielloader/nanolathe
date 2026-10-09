package session

import (
	"fmt"
	"strings"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/mission"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// The online lobby's configuration (DESIGN_MULTIPLAYER §16.6). Every value
// below is Nanolathe lobby policy, not a retail record: the seat-to-row
// mapping, the default names, the colour rule and the team numbering.

// OnlineTeamNone is the lobby team of a seat on no team. Teams 1..5 are the
// configuration's ally groups 0..4; none is the unassigned group 5.
const OnlineTeamNone = 0

// OnlineMaxTeam is the highest lobby team number.
const OnlineMaxTeam = 5

// onlineColors is how many player colours a seat may hold, the configuration
// row's 0..9.
const onlineColors = 10

// OnlineSurvivalMaxSurvivors is how many human survivors an online Survival
// battle seats: the single-player survivor layout's human and buddy rows
// (DESIGN_SURVIVAL §4.1), each a human online.
const OnlineSurvivalMaxSurvivors = 1 + SurvivalMaxBuddies

// OnlineSeat is one present lobby seat, in slot order (DESIGN_MULTIPLAYER
// §16.6). Team is OnlineTeamNone or 1..OnlineMaxTeam; Survival ignores it.
// Side indexes the frozen catalog's sides in their compiled order, the order
// OnlineSides names them. Color is the player's colour, 0..9, held by no
// other seat.
type OnlineSeat struct {
	Team  uint8
	Side  uint8
	Color uint8
}

// OnlineMatchSetup is what a lobby decides beyond the host's frozen content:
// the game type, map, seats with their teams and sides, the host's seed pair
// and, for Survival, its options. Every seat is a human under Modern
// gameplay.
type OnlineMatchSetup struct {
	Survival         bool
	MapName          string
	Seats            []OnlineSeat
	SimSeed, CRTSeed uint32
	SurvivalOptions  SurvivalOptions
	// SideCount is how many sides the frozen catalog defines,
	// len(OnlineSides(cat)). Every seat's Side must be below it.
	SideCount int
}

// Rows is the configuration's row count for the setup: one human row per
// seat and, in Survival, the attacker row after them. It is the player count
// the map-entry code selects the map's schema for (OnlineMapSchema).
func (s OnlineMatchSetup) Rows() int {
	if s.Survival {
		return len(s.Seats) + 1
	}
	return len(s.Seats)
}

// NewOnlineMatchRequest builds the match configuration request for an
// online lobby: one human row per seat in slot order and, for Survival, the
// attacker row as SurvivalConfigFor builds it. A human row carries the
// participant identity whose first byte is its slot plus one (room's own
// Participants are not read), the nickname "Player n" for slot n-1, the
// seat's side and colour, and the skirmish default resources. A skirmish
// row's ally group is its team's (team t is group t-1, no team is the
// unassigned group); every survivor shares the Survival team, and the
// attacker takes the first colour no survivor holds, red when free
// (SurvivalConfigFor, DESIGN_SURVIVAL §4.1).
//
// The rule words and the unit limit are the single-player skirmish defaults
// under Modern, as DirectSkirmishConfig writes them; options and room are as
// NewMatchConfigRequest takes them. room.MapSchema must be the schema the
// map-entry code selects for setup.Rows() players, the human count in a
// skirmish and one more in Survival; OnlineMapSchema resolves it.
//
// A skirmish seats 2..10 humans and refuses one team holding every seat
// [08 R-SKIR-01 §12]; whether the map offers that many start positions is
// the lobby's cap (OnlineMapCapacity). Survival seats 2..3 survivors. A
// colour above 9, or one two seats share, is refused.
func NewOnlineMatchRequest(setup OnlineMatchSetup, options SkirmishEntryOptions, room MatchRoomInputs) (MatchConfigRequest, error) {
	n := len(setup.Seats)
	most := SkirmishMaxPlayers
	if setup.Survival {
		most = OnlineSurvivalMaxSurvivors
	}
	if n < matchMinSeats || n > most {
		return MatchConfigRequest{}, matchFieldError("setup.seats", fmt.Sprintf("%d..%d seats", matchMinSeats, most))
	}
	if setup.SideCount < 1 || setup.SideCount > 256 {
		return MatchConfigRequest{}, matchFieldError("setup.sideCount", "the frozen catalog's side count, len(OnlineSides(cat)), 1..256")
	}
	if strings.TrimSpace(setup.MapName) == "" {
		return MatchConfigRequest{}, matchFieldError("setup.mapName", "a map name")
	}
	cfg := DirectSkirmishConfig(setup.MapName)
	cfg.Gameplay = gameplay.Modern
	cfg.RNGSimSeed, cfg.RNGCrtSeed = setup.SimSeed, setup.CRTSeed
	players := make([]SkirmishPlayer, n)
	for i, seat := range setup.Seats {
		path := fmt.Sprintf("setup.seats[%d]", i)
		if seat.Team > OnlineMaxTeam {
			return MatchConfigRequest{}, matchFieldError(path+".team", fmt.Sprintf("%d for none or 1..%d", OnlineTeamNone, OnlineMaxTeam))
		}
		if int(seat.Side) >= setup.SideCount {
			return MatchConfigRequest{}, matchFieldError(path+".side", fmt.Sprintf("a side below the catalog's %d sides", setup.SideCount))
		}
		if seat.Color >= onlineColors {
			return MatchConfigRequest{}, matchFieldError(path+".color", fmt.Sprintf("0..%d", onlineColors-1))
		}
		for j := range i {
			if setup.Seats[j].Color == seat.Color {
				return MatchConfigRequest{}, matchFieldError(path+".color", fmt.Sprintf("a colour no other seat holds, not seat %d's", j))
			}
		}
		players[i] = SkirmishPlayer{
			Controller: SkirmishControllerHuman,
			Side:       int(seat.Side),
			Color:      int(seat.Color),
			AllyGroup:  onlineAllyGroup(seat.Team),
			Metal:      SkirmishDefaultMetal,
			Energy:     SkirmishDefaultEnergy,
			Nickname:   fmt.Sprintf("Player %d", i+1),
		}
		room.Participants[i] = MatchParticipantID{byte(i + 1)}
	}
	for i := n; i < len(room.Participants); i++ {
		room.Participants[i] = MatchParticipantID{}
	}
	for i := range cfg.Players {
		cfg.Players[i] = SkirmishPlayer{}
	}
	cfg.NumPlayers = n
	if setup.Survival {
		layout := SurvivalConfigFor(setup.MapName, players, setup.SurvivalOptions)
		cfg.Survival = layout.Survival
		cfg.NumPlayers = n + 1
		for i := range players {
			players[i].AllyGroup = layout.Players[i].AllyGroup
		}
		cfg.Players[n] = layout.Players[n]
	}
	copy(cfg.Players[:n], players)
	return newMatchConfigRequest(cfg, options, room, true)
}

// onlineAllyGroup is a lobby team's ally group: team t is group t-1 and no
// team the unassigned group.
func onlineAllyGroup(team uint8) int {
	if team == OnlineTeamNone {
		return SkirmishDefaultAllyGroup
	}
	return int(team) - 1
}

// OnlineMapCapacity is the most players an online skirmish on mapName can
// seat: the largest start-position count among its network schemas in the
// compiled catalog's map header, at most 10. Survival ignores start
// positions.
func OnlineMapCapacity(cat *content.Catalog, mapName string) (int, error) {
	if cat == nil {
		return 0, fmt.Errorf("nanolathe: online map capacity: logical path %s, providers searched [catalog], expected a compiled catalog", mapName)
	}
	header := cat.Maps[content.CanonicalKey(mapName)]
	if header == nil {
		return 0, fmt.Errorf("nanolathe: online map capacity: logical path %s, providers searched [catalog maps], expected a compiled map header", mapName)
	}
	best := 0
	for _, schema := range header.Schemas {
		if formats.NetworkSchemaRank(schema.Type) != 0 && schema.StartPosCount > best {
			best = schema.StartPosCount
		}
	}
	if best == 0 {
		return 0, fmt.Errorf("nanolathe: online map capacity: logical path %s, providers searched [catalog map schemas], expected a network schema with start positions", header.LogicalOTA)
	}
	return min(best, SkirmishMaxPlayers), nil
}

// OnlineMapSchema is the configuration's MapSchema for mapName and rows
// players (OnlineMatchSetup.Rows): the index, in the compiled catalog's map
// header, of the network schema the map-entry code selects for that player
// count, which admission requires (ValidateMatchInputs).
func OnlineMapSchema(fs vfs.FSOps, cat *content.Catalog, mapName string, rows int) (uint32, error) {
	if fs == nil || cat == nil {
		return 0, fmt.Errorf("nanolathe: online map schema: logical path %s, providers searched [content], expected a mounted file system and its compiled catalog", mapName)
	}
	m, err := mission.LoadWithType(fs, mission.TypeSkirmish, mapName, 0, rows, nil)
	if err != nil {
		return 0, err
	}
	header := cat.Maps[content.CanonicalKey(m.TerrainKey)]
	if header == nil {
		return 0, fmt.Errorf("nanolathe: online map schema: logical path %s, providers searched [catalog maps], expected a compiled map header", m.TerrainKey)
	}
	index := mapSchemaIndex(header, m.Schema.Name)
	if int(index) >= len(header.Schemas) {
		return 0, fmt.Errorf("nanolathe: online map schema: logical path %s, providers searched [catalog map schemas], expected the selected schema %q", header.LogicalOTA, m.Schema.Name)
	}
	return index, nil
}

// OnlineSides names the frozen catalog's sides in index order, the order an
// OnlineSeat's Side counts: each side's authored name, such as ARM or CORE,
// or "" for a side without one.
func OnlineSides(cat *content.Catalog) []string {
	if cat == nil {
		return nil
	}
	names := make([]string, len(cat.Sides))
	for i, side := range cat.Sides {
		if side != nil {
			names[i] = strings.TrimSpace(side.Name)
		}
	}
	return names
}
