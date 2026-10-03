package session

import (
	"reflect"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/ai"
	"github.com/nanolathe-gg/nanolathe/internal/community"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/survival"
)

// matchTestRoom is a room's agreed values for a base-content battle with one
// local human in slot 0.
func matchTestRoom() MatchRoomInputs {
	room := MatchRoomInputs{
		ContentProfile: "retail",
		PlayerView:     MatchView{MinimumScale: 1024, MaximumScale: 2048, FullMap: false},
		SpectatorView:  MatchView{MinimumScale: 256, MaximumScale: 2048, FullMap: true},
		ReplayView:     MatchView{MinimumScale: 256, MaximumScale: 2048, FullMap: true},
		Policies: MatchPolicies{Revision: 1, Scheduling: 1, Pacing: 1, Drop: 1, Audience: 1,
			RejoinGraceMilliseconds: 90000, SpectatorDelayMilliseconds: 0, ReplayReleaseDelayMilliseconds: 0},
	}
	room.Participants[0] = matchTestID(1)
	return room
}

// adapterDigest resolves a setup through the local adapter.
func adapterDigest(t *testing.T, cfg SkirmishConfig, options SkirmishEntryOptions, room MatchRoomInputs) [32]byte {
	t.Helper()
	r, err := NewMatchConfigRequest(cfg, options, room)
	if err != nil {
		t.Fatalf("adapter: %v", err)
	}
	c, err := ResolveMatchConfig(r)
	if err != nil {
		t.Fatalf("resolve the adapted request: %v", err)
	}
	return c.Digest()
}

// The adapter turns today's direct skirmish setup into a request that
// resolves, with the single-player defaults made explicit, and changes
// neither argument.
func TestMatchConfigFromSkirmishSetup(t *testing.T) {
	cfg := DirectSkirmishConfig("Great Divide")
	cfg.Gameplay = gameplay.Strict31
	cfg.RNGSimSeed, cfg.RNGCrtSeed = 5, 6
	options := SkirmishEntryOptions{AIOverrides: AIOverrides{All: "jitter=0"}}
	before, beforeOptions := cfg, options
	r, err := NewMatchConfigRequest(cfg, options, matchTestRoom())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg, before) || !reflect.DeepEqual(options, beforeOptions) {
		t.Fatal("the adapter changed its arguments")
	}
	c := resolveMatch(t, r)
	got := c.Request()
	if got.SessionKind != MatchOnlineSkirmish || got.RuleName != StrictRuleSetName || got.RuleBase != MatchBaseStrict31 {
		t.Fatalf("kind and rules: %d %q %d", got.SessionKind, got.RuleName, got.RuleBase)
	}
	if got.UnitLimit != SkirmishDefaultUnitLimit || got.Community != (community.Features{}) {
		t.Fatalf("Strict 3.1 limit and table: %d %+v", got.UnitLimit, got.Community)
	}
	if got.Location != 1 || got.CommanderDeath != 1 || got.Mapping != 1 || got.LineOfSight != 1 || got.LOSType != 1 {
		t.Fatal("the rule words are not the setup's defaults")
	}
	if got.SimulationSeed != 5 || got.CRTSeed != 6 {
		t.Fatal("the seeds were not carried")
	}
	human, computer := got.Seats[0], got.Seats[1]
	if human.Role != MatchRoleHuman || human.Participant != matchTestID(1) || human.HostSeat != MatchHostNone || human.Metal != 1000 {
		t.Fatalf("the human row: %+v", human)
	}
	if computer.Role != MatchRoleComputer || computer.HostSeat != 0 || computer.Difficulty != SkirmishDefaultDifficulty ||
		computer.ComputerKind != ai.ControllerClassic || !reflect.DeepEqual(computer.AIParams, []AIParam{{Key: "jitter", Value: "0"}}) {
		t.Fatalf("the computer row: %+v", computer)
	}
	want := content.RetailLimits()
	if p := got.ContentProfile; p.Name != "retail" || p.Units != uint32(want.Units) || p.Weapons != uint32(want.Weapons) ||
		p.TNTBytes != uint64(want.TNTBytes) || p.LOSBytes != uint64(want.LOSBytes) || len(p.Directories) != 0 {
		t.Fatalf("the content profile: %+v", p)
	}
	// Under Modern the mainline table's unit limit is the battle's, as
	// battle entry applies it.
	cfg.Gameplay = gameplay.Modern
	r, err = NewMatchConfigRequest(cfg, options, matchTestRoom())
	if err != nil {
		t.Fatal(err)
	}
	if mainline, _ := community.Resolve(false); r.UnitLimit != uint16(mainline.UnitLimit) || r.Community != mainline {
		t.Fatalf("Modern took unit limit %d and another table", r.UnitLimit)
	}
	resolveMatch(t, r)
}

// The adapter turns today's Survival setup into an online Survival request:
// the attacker last with no economy, the buddies computers the human added,
// and the survivors one team with shared victory.
func TestMatchConfigFromSurvivalSetup(t *testing.T) {
	cfg := SurvivalSkirmishConfig("Great Divide", 2, SurvivalOptions{Pace: survival.PaceRelentless, NoAir: true})
	cfg.Players[1].AI = ai.ControllerModern
	before := cfg
	r, err := NewMatchConfigRequest(cfg, SkirmishEntryOptions{}, matchTestRoom())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg, before) {
		t.Fatal("the adapter changed the setup")
	}
	got := resolveMatch(t, r).Request()
	if got.SessionKind != MatchOnlineSurvival || got.SurvivalPace != survival.PaceRelentless || !got.SurvivalNoAir || got.SurvivalNoNaval {
		t.Fatalf("Survival fields: %d %d %v %v", got.SessionKind, got.SurvivalPace, got.SurvivalNoAir, got.SurvivalNoNaval)
	}
	if len(got.Seats) != 4 {
		t.Fatalf("%d seats", len(got.Seats))
	}
	attacker := got.Seats[3]
	if attacker.Role != MatchRoleSurvivalAttacker || attacker.Metal != 0 || attacker.Energy != 0 || attacker.AllyGroup != SkirmishDefaultAllyGroup || attacker.SharedVictory {
		t.Fatalf("the attacker row: %+v", attacker)
	}
	for i := 0; i < 3; i++ {
		if !got.Seats[i].SharedVictory || got.Seats[i].AllyGroup != got.Seats[0].AllyGroup {
			t.Fatalf("survivor %d is not on the shared team: %+v", i, got.Seats[i])
		}
	}
	if got.Seats[1].ComputerKind != ai.ControllerModern || got.Seats[1].HostSeat != 0 || got.Seats[2].HostSeat != 0 {
		t.Fatal("the buddies are not the human's computers")
	}
	// Strict 3.1 admits one computer per human: two buddies resolve only
	// outside it.
	cfg.Gameplay = gameplay.Strict31
	r, err = NewMatchConfigRequest(cfg, SkirmishEntryOptions{}, matchTestRoom())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveMatchConfig(r); err == nil {
		t.Fatal("Strict 3.1 admitted two buddies for one human")
	}
	one := SurvivalSkirmishConfig("Great Divide", 1, SurvivalOptions{})
	one.Gameplay = gameplay.Strict31
	adapterDigest(t, one, SkirmishEntryOptions{}, matchTestRoom())
}

// Every input of the §16.2 inventory the adapter reads changes the
// identity, one at a time; the load observer and a precompiled animation
// table do not.
func TestMatchConfigAdapterInventory(t *testing.T) {
	base := func() (SkirmishConfig, SkirmishEntryOptions, MatchRoomInputs) {
		cfg := DirectSkirmishConfig("Great Divide")
		cfg.RNGSimSeed, cfg.RNGCrtSeed = 5, 6
		return cfg, SkirmishEntryOptions{}, matchTestRoom()
	}
	cfg, options, room := base()
	baseDigest := adapterDigest(t, cfg, options, room)
	seen := map[[32]byte]string{baseDigest: "base"}
	vetero := false
	builder := orders.DefaultBuilderOptions()
	builder.Guard[2] = orders.GuardScatter
	for _, c := range []struct {
		name   string
		change func(*SkirmishConfig, *SkirmishEntryOptions, *MatchRoomInputs)
	}{
		{"gameplay", func(c *SkirmishConfig, _ *SkirmishEntryOptions, _ *MatchRoomInputs) { c.Gameplay = gameplay.Strict31 }},
		{"map", func(c *SkirmishConfig, _ *SkirmishEntryOptions, _ *MatchRoomInputs) { c.MapName = "Lava Run" }},
		{"side", func(c *SkirmishConfig, _ *SkirmishEntryOptions, _ *MatchRoomInputs) { c.Players[0].Side = 1 }},
		{"colour", func(c *SkirmishConfig, _ *SkirmishEntryOptions, _ *MatchRoomInputs) { c.Players[1].Color = 7 }},
		{"team", func(c *SkirmishConfig, _ *SkirmishEntryOptions, _ *MatchRoomInputs) { c.Players[1].AllyGroup = 3 }},
		{"nickname", func(c *SkirmishConfig, _ *SkirmishEntryOptions, _ *MatchRoomInputs) { c.Players[0].Nickname = "Ann" }},
		{"metal", func(c *SkirmishConfig, _ *SkirmishEntryOptions, _ *MatchRoomInputs) { c.Players[1].Metal = 500 }},
		{"energy", func(c *SkirmishConfig, _ *SkirmishEntryOptions, _ *MatchRoomInputs) { c.Players[1].Energy = 500 }},
		{"Classic or Modern", func(c *SkirmishConfig, _ *SkirmishEntryOptions, _ *MatchRoomInputs) {
			c.Players[1].AI = ai.ControllerModern
		}},
		{"computer difficulty", func(c *SkirmishConfig, _ *SkirmishEntryOptions, _ *MatchRoomInputs) { c.Difficulty = 2 }},
		{"location", func(c *SkirmishConfig, _ *SkirmishEntryOptions, _ *MatchRoomInputs) { c.Location = 0 }},
		{"commander death", func(c *SkirmishConfig, _ *SkirmishEntryOptions, _ *MatchRoomInputs) { c.CommanderDeath = 0 }},
		{"mapping", func(c *SkirmishConfig, _ *SkirmishEntryOptions, _ *MatchRoomInputs) { c.Mapping = 0 }},
		{"line of sight", func(c *SkirmishConfig, _ *SkirmishEntryOptions, _ *MatchRoomInputs) { c.LineOfSight = 0 }},
		{"LOS type", func(c *SkirmishConfig, _ *SkirmishEntryOptions, _ *MatchRoomInputs) { c.LOSType = 0 }},
		{"unit limit", func(c *SkirmishConfig, o *SkirmishEntryOptions, _ *MatchRoomInputs) {
			zero := 0
			o.CommunitySources.Player = community.Overrides{UnitLimit: &zero}
			c.UnitLimit = 900
		}},
		{"simulation seed", func(c *SkirmishConfig, _ *SkirmishEntryOptions, _ *MatchRoomInputs) { c.RNGSimSeed = 0 }},
		{"CRT seed", func(c *SkirmishConfig, _ *SkirmishEntryOptions, _ *MatchRoomInputs) { c.RNGCrtSeed = 0 }},
		{"builder options", func(_ *SkirmishConfig, o *SkirmishEntryOptions, _ *MatchRoomInputs) { o.BuilderOptions = &builder }},
		{"community source", func(_ *SkirmishConfig, o *SkirmishEntryOptions, _ *MatchRoomInputs) {
			o.CommunitySources.Player = community.Overrides{Veterancy: &vetero}
		}},
		{"content units", func(_ *SkirmishConfig, o *SkirmishEntryOptions, _ *MatchRoomInputs) { o.ContentLimits.Units = 4096 }},
		{"content weapons", func(_ *SkirmishConfig, o *SkirmishEntryOptions, _ *MatchRoomInputs) { o.ContentLimits.Weapons = 4096 }},
		{"content TNT bytes", func(_ *SkirmishConfig, o *SkirmishEntryOptions, _ *MatchRoomInputs) {
			o.ContentLimits.TNTBytes = 32 << 20
		}},
		{"content LOS bytes", func(_ *SkirmishConfig, o *SkirmishEntryOptions, _ *MatchRoomInputs) {
			o.ContentLimits.LOSBytes = 2 << 20
		}},
		{"mutators", func(_ *SkirmishConfig, o *SkirmishEntryOptions, _ *MatchRoomInputs) {
			o.Mutators.Income = content.Factor{Num: 3, Den: 2}
		}},
		{"AI overrides", func(_ *SkirmishConfig, o *SkirmishEntryOptions, _ *MatchRoomInputs) {
			o.AIOverrides.Players[1] = "style=eco"
		}},
		{"map schema", func(_ *SkirmishConfig, _ *SkirmishEntryOptions, r *MatchRoomInputs) { r.MapSchema = 2 }},
		{"mod", func(_ *SkirmishConfig, _ *SkirmishEntryOptions, r *MatchRoomInputs) {
			r.Mod = MatchMod{ID: "prota", Version: "4.8", Archive: [32]byte{1}}
		}},
		{"content profile", func(_ *SkirmishConfig, _ *SkirmishEntryOptions, r *MatchRoomInputs) { r.ContentProfile = "prota" }},
		{"content directories", func(_ *SkirmishConfig, _ *SkirmishEntryOptions, r *MatchRoomInputs) {
			r.ContentDirectories = map[string]string{"weapons": "weaponP"}
		}},
		{"seat identity", func(_ *SkirmishConfig, _ *SkirmishEntryOptions, r *MatchRoomInputs) {
			r.Participants[0] = matchTestID(2)
		}},
		{"restrictions", func(_ *SkirmishConfig, _ *SkirmishEntryOptions, r *MatchRoomInputs) {
			r.UnitRestrictions = []MatchUnitRestriction{{DefinitionID: 1, Unit: "armcom", Limit: 1}}
		}},
		{"cheats", func(_ *SkirmishConfig, _ *SkirmishEntryOptions, r *MatchRoomInputs) { r.CheatsAllowed = true }},
		{"watching", func(_ *SkirmishConfig, _ *SkirmishEntryOptions, r *MatchRoomInputs) { r.WatchingAllowed = true }},
		{"view", func(_ *SkirmishConfig, _ *SkirmishEntryOptions, r *MatchRoomInputs) { r.PlayerView.MinimumScale = 512 }},
		{"policy timing", func(_ *SkirmishConfig, _ *SkirmishEntryOptions, r *MatchRoomInputs) {
			r.Policies.SpectatorDelayMilliseconds = 1000
		}},
	} {
		cfg, options, room := base()
		c.change(&cfg, &options, &room)
		d := adapterDigest(t, cfg, options, room)
		if other, dup := seen[d]; dup {
			t.Errorf("%s: same identity as %s", c.name, other)
		}
		seen[d] = c.name
	}
	// The load observer and a precompiled animation table are not
	// configuration.
	cfg, options, room = base()
	options.Progress = func(string, int) {}
	options.SimArt = &content.SimArt{}
	if adapterDigest(t, cfg, options, room) != baseDigest {
		t.Fatal("a progress observer or a precompiled SimArt changed the identity")
	}
	// The single-player difficulty word is only each computer's own value:
	// a battle with no added computer does not encode it at all.
	alone := SurvivalSkirmishConfig("Great Divide", 0, SurvivalOptions{})
	easy := alone
	easy.Difficulty = 0
	if adapterDigest(t, alone, SkirmishEntryOptions{}, room) != adapterDigest(t, easy, SkirmishEntryOptions{}, room) {
		t.Fatal("the battle-wide difficulty word reached a configuration with no computer")
	}
}

// M2-C9: equivalent default spellings of the same effective battle resolve
// to one identity.
func TestMatchConfigAdapterEquivalentDefaults(t *testing.T) {
	base := func() (SkirmishConfig, SkirmishEntryOptions, MatchRoomInputs) {
		return DirectSkirmishConfig("Great Divide"), SkirmishEntryOptions{}, matchTestRoom()
	}
	defaults := orders.DefaultBuilderOptions()
	mainlineVeterancy := true
	for _, c := range []struct {
		name   string
		change func(*SkirmishConfig, *SkirmishEntryOptions, *MatchRoomInputs)
	}{
		{"the zero gameplay word is Modern", func(c *SkirmishConfig, _ *SkirmishEntryOptions, _ *MatchRoomInputs) { c.Gameplay = gameplay.Modern }},
		{"any nonzero location is identity", func(c *SkirmishConfig, _ *SkirmishEntryOptions, _ *MatchRoomInputs) { c.Location = 5 }},
		{"visibility words by their low bit", func(c *SkirmishConfig, _ *SkirmishEntryOptions, _ *MatchRoomInputs) {
			c.Mapping, c.LineOfSight, c.LOSType = 3, 5, 7
		}},
		{"an unknown commander-death word ends the game", func(c *SkirmishConfig, _ *SkirmishEntryOptions, _ *MatchRoomInputs) { c.CommanderDeath = 9 }},
		{"default builder options", func(_ *SkirmishConfig, o *SkirmishEntryOptions, _ *MatchRoomInputs) { o.BuilderOptions = &defaults }},
		{"retail content limits", func(_ *SkirmishConfig, o *SkirmishEntryOptions, _ *MatchRoomInputs) {
			o.ContentLimits = content.RetailLimits()
		}},
		{"identity mutators spelled 1/1", func(_ *SkirmishConfig, o *SkirmishEntryOptions, _ *MatchRoomInputs) {
			o.Mutators.Damage, o.Mutators.Sight = content.Factor{Num: 1, Den: 1}, content.Factor{Num: 1, Den: 1}
		}},
		{"a source that restates the mainline table", func(_ *SkirmishConfig, o *SkirmishEntryOptions, _ *MatchRoomInputs) {
			o.CommunitySources.Player = community.Overrides{Veterancy: &mainlineVeterancy}
		}},
		{"a directory row to itself", func(_ *SkirmishConfig, _ *SkirmishEntryOptions, r *MatchRoomInputs) {
			r.ContentDirectories = map[string]string{"units": "UNITS"}
		}},
		{"a participant for a computer row", func(_ *SkirmishConfig, _ *SkirmishEntryOptions, r *MatchRoomInputs) {
			r.Participants[1] = matchTestID(9)
		}},
	} {
		cfg, options, room := base()
		want := adapterDigest(t, cfg, options, room)
		c.change(&cfg, &options, &room)
		if got := adapterDigest(t, cfg, options, room); got != want {
			t.Errorf("%s: a different identity", c.name)
		}
	}
	// The AI parameter layers merge to one value whichever layer names a
	// key, and the most specific layer wins.
	layered := func(o AIOverrides) [32]byte {
		cfg, _, room := base()
		return adapterDigest(t, cfg, SkirmishEntryOptions{AIOverrides: o}, room)
	}
	want := layered(AIOverrides{All: "jitter=0"})
	var byDifficulty, bySeat, overridden AIOverrides
	byDifficulty.Difficulty[1] = "jitter=0"
	bySeat.Players[1] = "jitter=0"
	overridden.All = "jitter=1"
	overridden.Players[1] = " jitter = 0 "
	for name, o := range map[string]AIOverrides{"difficulty layer": byDifficulty, "seat layer": bySeat, "overridden": overridden} {
		if layered(o) != want {
			t.Errorf("AI overrides by %s: a different identity", name)
		}
	}
	// A directory table spelled with other capitals is one table.
	cfg, options, room := base()
	room.ContentDirectories = map[string]string{"Weapons": "WeaponP", "gamedata": "GamedatP"}
	a := adapterDigest(t, cfg, options, room)
	room.ContentDirectories = map[string]string{"weapons": "weaponp", "GAMEDATA": "gamedatp"}
	if adapterDigest(t, cfg, options, room) != a {
		t.Error("directory tables differing only in capitals have different identities")
	}
	// Strict 3.1 applies no table, so a missing unit limit is the default.
	cfg, options, room = base()
	cfg.Gameplay = gameplay.Strict31
	cfg.UnitLimit = 0
	b := adapterDigest(t, cfg, options, room)
	cfg.UnitLimit = SkirmishDefaultUnitLimit
	if adapterDigest(t, cfg, options, room) != b {
		t.Error("a missing unit limit and the default have different identities")
	}
}

// The adapter refuses what it cannot describe: an automated battle, a second
// human, a word that names no rule set, and an out-of-range difficulty.
// A local name cut inside a character by Normalize loses the incomplete
// character rather than failing on another platform.
func TestMatchConfigAdapterRefusals(t *testing.T) {
	room := matchTestRoom()
	cfg := DirectSkirmishConfig("Great Divide")
	if _, err := NewMatchConfigRequest(cfg, SkirmishEntryOptions{AutomatedPlayers: true}, room); err == nil {
		t.Error("an automated battle was adapted")
	}
	two := cfg
	two.NumPlayers = 3
	two.Players[2] = SkirmishPlayer{Controller: SkirmishControllerHuman, AllyGroup: 3, Metal: 1000, Energy: 1000}
	if _, err := NewMatchConfigRequest(two, SkirmishEntryOptions{}, room); err == nil {
		t.Error("a setup with two humans was adapted")
	}
	unknown := cfg
	unknown.Gameplay = "modern-ai"
	if _, err := NewMatchConfigRequest(unknown, SkirmishEntryOptions{}, room); err == nil {
		t.Error("an unknown rule-set word was adapted")
	}
	hard := cfg
	hard.Difficulty = 5
	if _, err := NewMatchConfigRequest(hard, SkirmishEntryOptions{}, room); err == nil {
		t.Error("difficulty 5 was adapted")
	}
	named := cfg
	named.Players[0].Nickname = "a" + "ÄÄÄÄÄÄÄÄ" // 17 bytes: Normalize cuts the last Ä in half
	r, err := NewMatchConfigRequest(named, SkirmishEntryOptions{}, room)
	if err != nil {
		t.Fatal(err)
	}
	if r.Seats[0].Nickname != "a"+"ÄÄÄÄÄÄÄ" {
		t.Fatalf("nickname %q", r.Seats[0].Nickname)
	}
	resolveMatch(t, r)
}
