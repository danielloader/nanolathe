package headless

import (
	"encoding/hex"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/mission"
	"github.com/nanolathe-gg/nanolathe/internal/session"
)

// composeCheckpointSimBenchBattle uses the same setup/entry values as the
// ordinary benchmark, through M2 admission. These fixed room values belong to
// this measurement fixture, not to live lobby defaults (DESIGN_MULTIPLAYER
// §16.3.79). Army placement and opening publication remain with the caller.
func composeCheckpointSimBenchBattle(request FreshBattleRequest) (FreshBattle, error) {
	kind, identity, cfg, err := normalizeFreshBattleRequest(request)
	if err != nil {
		return FreshBattle{}, err
	}
	if request.FS == nil {
		return FreshBattle{}, diagnostic("simulation benchmark admission failed", identity, nil, "a mounted retail content view")
	}
	cfg.Gameplay = request.Gameplay
	options := session.SkirmishEntryOptions{CommunitySources: request.CommunitySources}
	catalog := request.Catalog
	if catalog == nil {
		catalog, err = content.Compile(request.FS)
		if err != nil {
			return FreshBattle{}, err
		}
	}
	selected, err := mission.LoadWithType(request.FS, mission.TypeSkirmish, cfg.MapName, cfg.Difficulty, cfg.NumPlayers, nil)
	if err != nil {
		return FreshBattle{}, err
	}
	header := catalog.Maps[content.CanonicalKey(selected.TerrainKey)]
	if header == nil || selected.Schema.Name == "" {
		return FreshBattle{}, diagnostic("simulation benchmark admission failed", identity, providersFromOps(request.FS), "the selected mission schema in the compiled map header")
	}
	schemaIndex := uint32(len(header.Schemas))
	for i, schema := range header.Schemas {
		if schema.Name == selected.Schema.Name {
			schemaIndex = uint32(i)
			break
		}
	}
	room := session.MatchRoomInputs{
		MapSchema: schemaIndex, ContentProfile: "retail",
		PlayerView:    session.MatchView{MinimumScale: 1024, MaximumScale: 2048},
		SpectatorView: session.MatchView{MinimumScale: 256, MaximumScale: 2048, FullMap: true},
		ReplayView:    session.MatchView{MinimumScale: 256, MaximumScale: 2048, FullMap: true},
		Policies: session.MatchPolicies{Revision: 1, Scheduling: 1, Pacing: 1, Drop: 1, Audience: 1,
			RejoinGraceMilliseconds: 90000},
	}
	for i := range room.Participants[simBenchHumanSlot] {
		room.Participants[simBenchHumanSlot][i] = 1
	}
	matchRequest, err := session.NewMatchConfigRequest(cfg, options, room)
	if err != nil {
		return FreshBattle{}, err
	}
	config, err := session.ResolveMatchConfig(matchRequest)
	if err != nil {
		return FreshBattle{}, err
	}
	inputs, err := session.FreezeMatchInputs(request.FS, catalog, config, nil)
	if err != nil {
		return FreshBattle{}, err
	}
	sess, err := session.NewAdmittedSkirmish(inputs, config, nil)
	if err != nil {
		return FreshBattle{}, err
	}
	initial, err := sess.PartialStateFingerprint()
	if err != nil {
		return FreshBattle{}, err
	}
	return FreshBattle{Session: sess, Kind: kind, Identity: identity,
		SimulationSeed: request.SimulationSeed, CRTSeed: request.CRTSeed,
		LocalOwner: sess.LocalOwner, Watching: sess.OwnerIsObserver(int(sess.LocalOwner)),
		TerrainWidth: sess.World.CellW * 16, TerrainHeight: sess.World.CellH * 16,
		InitialFingerprint: initial}, nil
}

// History copying happens once, outside the measured window; per-step failure
// checks read only CheckpointCaptureResult's detached value (§16.3.79).
func simBenchCheckpointReport(sess *session.Session, report *SimBenchReport) error {
	if err := sess.CheckpointCaptureResult().Err; err != nil {
		return err
	}
	history := sess.CheckpointHistory()
	if len(history.Records) == 0 {
		return diagnostic("simulation benchmark checkpoint history missing", report.Map, nil, "at least the admitted entry checkpoint")
	}
	report.CheckpointRecords, report.CheckpointTicks = len(history.Records), len(history.Ticks)
	latest := history.Records[len(history.Records)-1]
	report.CheckpointTick = latest.Position.Tick
	report.CheckpointDigest = hex.EncodeToString(latest.Digests.Full[:])
	return nil
}
