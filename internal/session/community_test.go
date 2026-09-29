package session

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/combat"
	"github.com/nanolathe-gg/nanolathe/internal/community"
	"github.com/nanolathe-gg/nanolathe/internal/construction"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/economy"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/save"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

const communityCompositionTestName = "community-composition-test"

func init() {
	margin := 3
	RegisterRuleSet(communityCompositionTestName, func() RuleSet {
		return RuleSet{Base: gameplay.Community39, Features: community.Overrides{OffMapAircraftMarginTiles: &margin}}
	})
}

func TestCommunityCompositionPrecedenceAndStrictBypass(t *testing.T) {
	flag := false
	margin := 3
	override := 7
	sources := CommunitySources{
		Content:     []community.Overrides{{Table: "escalation"}},
		Player:      community.Overrides{WeaponTargetKeys: &flag, OffMapAircraftMarginTiles: &margin},
		CommandLine: []community.Overrides{{OffMapAircraftMarginTiles: &override}},
	}
	got, err := ResolveCommunity(gameplay.Community39, sources)
	if err != nil {
		t.Fatal(err)
	}
	if got.WeaponTargetKeys || got.OffMapAircraftMarginTiles != 7 || !got.RepairRate.Enabled {
		t.Fatalf("precedence: %+v", got)
	}
	strict, err := ResolveCommunity(gameplay.Strict31, sources)
	if err != nil || strict != (community.Features{}) {
		t.Fatalf("strict: %+v, %v", strict, err)
	}
	name := communityCompositionTestName
	got, err = ResolveCommunity(gameplay.Mode(name), CommunitySources{Content: []community.Overrides{{Table: "escalation"}}})
	if err != nil || got.OffMapAircraftMarginTiles != margin {
		t.Fatalf("registered declaration: %+v, %v", got, err)
	}
	got, err = ResolveCommunity(gameplay.Mode(name), sources)
	if err != nil || got.OffMapAircraftMarginTiles != override {
		t.Fatalf("player/CLI must override registry: %+v, %v", got, err)
	}
}

func TestCommunityRebindingAndInvalidSwitch(t *testing.T) {
	s := &Session{Combat: &combat.Service{}}
	if err := s.SetRules(CommunityRuleSetName); err != nil {
		t.Fatal(err)
	}
	if !s.newOrderBinding().Community.WeaponTargetKeys {
		t.Fatal("new binding projection missing")
	}
	if !s.Combat.Community.WeaponTargetKeys {
		t.Fatal("combat projection missing")
	}
	if allocations := testing.AllocsPerRun(100, s.RebindRules); allocations != 0 {
		t.Fatalf("rebind allocated %g", allocations)
	}
	s.CommunitySources.Player.Table = "unknown-table"
	if err := s.SetRules(StrictRuleSetName); err != nil {
		t.Fatal(err)
	}
	if s.Community != (community.Features{}) || s.Combat.Community != (community.Features{}) {
		t.Fatal("Strict retained features")
	}
	if err := s.SetRules(CommunityRuleSetName); err == nil {
		t.Fatal("invalid declaration accepted")
	}
	if err := s.EnqueueHumanCommand(HumanCommand{Kind: HumanGameplay, Gameplay: gameplay.Community39}); err == nil {
		t.Fatal("invalid switch queued")
	}
	s.SetGameplay(gameplay.Community39)
	if s.Rules.Name != StrictRuleSetName {
		t.Fatal("failed switch changed binding")
	}
}

func TestCommunityFreshAndRestoredEntryShareConfiguration(t *testing.T) {
	f := loadRetailFixture(t)
	limit := 60
	sources := CommunitySources{Content: []community.Overrides{{Table: "escalation"}}, CommandLine: []community.Overrides{{UnitLimit: &limit}}}
	f.cfg.Gameplay = gameplay.Community39
	initialOptions := orders.DefaultBuilderOptions()
	initialOptions.Guard[1] = orders.GuardStay
	src, err := NewSkirmishWithEntryOptions(f.fs, f.cat, f.cfg, SkirmishEntryOptions{CommunitySources: sources, BuilderOptions: &initialOptions})
	if err != nil {
		t.Fatal(err)
	}
	if src.Units.UnitLimit() != limit || src.Combat.Slots.Capacity() != src.EntryCommunity.ProjectileCapacity {
		t.Fatal("table did not size unit pool")
	}
	// CP-LIM-2's special-effects limit sizes the strip table at the same entry
	// (DESIGN_COMMUNITY_PATCH §4.1).
	if sfx := src.EntryCommunity.SfxLimit; sfx != 20480 || src.strips.steadyCap != sfx || src.strips.poolCapacity != 10*sfx {
		t.Fatalf("strip bounds %d/%d for SfxLimit %d", src.strips.steadyCap, src.strips.poolCapacity, sfx)
	}
	inputs, err := src.RetailBattleSaveInputs(RetailBattleSummary(src, "community configuration", "1", limit), save.Camera{})
	if err != nil {
		t.Fatal(err)
	}
	projection, err := src.RetailProjection(inputs)
	if err != nil {
		t.Fatal(err)
	}
	data, err := projection.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	bank, err := save.OpenBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	loadedOptions := orders.DefaultBuilderOptions()
	loadedOptions.Guard[1] = orders.GuardScatter
	staged, err := StageRetailBattle(bank, RetailLoadDeps{FS: f.fs, Catalog: f.cat, SimSeed: 7, CRTSeed: 9, UnitLimit: 250, Gameplay: gameplay.Community39, CommunitySources: sources, BuilderOptions: &loadedOptions})
	if err != nil {
		t.Fatal(err)
	}
	dst := staged.Session
	if src.Build.OrderBinding.BuilderOptions(src.LocalOwner) != initialOptions || dst.Build.OrderBinding.BuilderOptions(dst.LocalOwner) != loadedOptions {
		t.Fatal("battle load did not take the current host builder preferences")
	}
	if src.Community != dst.Community || src.EntryCommunity != dst.EntryCommunity || dst.Units.UnitLimit() != limit || dst.Combat.Slots.Capacity() != src.Combat.Slots.Capacity() {
		t.Fatal("restore discarded entry configuration")
	}
	if dst.strips.steadyCap != src.strips.steadyCap || dst.strips.poolCapacity != src.strips.poolCapacity {
		t.Fatal("restore discarded the strip bounds")
	}
	src.SetGameplay(gameplay.Strict31)
	if src.Community != (community.Features{}) || src.EntryCommunity != dst.EntryCommunity || src.Units.UnitLimit() != limit {
		t.Fatal("live switch resized entry state")
	}
	if src.strips.steadyCap != dst.strips.steadyCap {
		t.Fatal("live switch resized the strip table")
	}
}

// TestStripBoundsAreEntryParametersOfTheRuleSet locks where CP-LIM-2's
// special-effects limit reaches the simulation: a Strict battle enters with
// retail's 400-record strips over a 1000-slot pool [03 "Strip storage and
// lifecycle"][03 R-FX-02 §4], and a Modern one with the shipped table's 20480
// and ten times it (DESIGN_COMMUNITY_PATCH §4.1).
func TestStripBoundsAreEntryParametersOfTheRuleSet(t *testing.T) {
	f := loadRetailFixture(t)
	for _, c := range []struct {
		mode       gameplay.Mode
		cap, slots int
	}{
		{gameplay.Strict31, 400, 1000},
		{gameplay.Modern, 20480, 204800},
	} {
		cfg := f.cfg
		cfg.Gameplay = c.mode
		s, err := NewSkirmishWithEntryOptions(f.fs, f.cat, cfg, SkirmishEntryOptions{})
		if err != nil {
			t.Fatalf("%s: %v", c.mode, err)
		}
		if s.strips.steadyCap != c.cap || s.strips.poolCapacity != c.slots {
			t.Errorf("%s strip bounds = %d/%d, want %d/%d", c.mode, s.strips.steadyCap, s.strips.poolCapacity, c.cap, c.slots)
		}
	}
}

// Repair fractions belong to one construction service and one unit world;
// neither a registry value nor repeated rule binding may replace/share them.
func TestCommunityRepairBanksFollowSessionLifecycle(t *testing.T) {
	fixture := func() (*Session, *units.Unit, *units.Unit) {
		def := &content.UnitDef{UnitName: "repair", MaxDamage: 100, BuildTime: 200}
		def.CanonicalKey = "repair"
		cat := &content.Catalog{Units: map[string]*content.UnitDef{"repair": def}}
		w := newSessionFixtureWorld(4, cat)
		bh, err := w.Create(def, 0, 0, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		th, err := w.Create(def, 0, 0, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		target := w.Unit(th)
		target.Health = 10
		enabled, one := true, 1
		s := &Session{Units: w, Econ: &economy.Service{}, Combat: &combat.Service{ControlByte: func(uint8) uint8 { return combat.ControlByteHuman }}, CommunitySources: CommunitySources{Player: community.Overrides{RepairRate: &community.RepairRateOverrides{Enabled: &enabled, RepairMultiplier: &one, SelfHealMultiplier: &one}}}}
		s.Build = construction.NewService(nil, cat, w, s.Econ)
		s.Build.Combat = s.Combat
		if err := s.SetRules(CommunityRuleSetName); err != nil {
			t.Fatal(err)
		}
		return s, w.Unit(bh), target
	}
	a, builderA, targetA := fixture()
	b, builderB, targetB := fixture()
	if !a.Build.Repair(builderA, targetA, 1) || targetA.Health != 10 {
		t.Fatal("first fraction")
	}
	if allocations := testing.AllocsPerRun(25, a.RebindRules); allocations != 0 {
		t.Fatalf("rebind allocated %g", allocations)
	}
	if err := a.SetRules(StrictRuleSetName); err != nil {
		t.Fatal(err)
	}
	if err := a.SetRules(ModernRuleSetName); err != nil {
		t.Fatal(err)
	}
	if !a.Build.Repair(builderA, targetA, 1) || targetA.Health != 11 {
		t.Fatal("switch/rebind lost fraction")
	}
	if !b.Build.Repair(builderB, targetB, 1) || targetB.Health != 10 {
		t.Fatal("independent session inherited fraction")
	}
	// A new world may reuse handles; it must not inherit A's bank. B still
	// owns its separate half-point even while A references the same fixture.
	a.Build.Repair(builderA, targetA, 1)
	a.Units, a.Build.World = b.Units, b.Units
	a.RebindRules()
	if !a.Build.Repair(builderB, targetB, 1) || targetB.Health != 10 {
		t.Fatal("world replacement retained old bank")
	}
	if !b.Build.Repair(builderB, targetB, 1) || targetB.Health != 11 {
		t.Fatal("world replacement changed another service's bank")
	}
}
