package session

// Frozen simulation content at skirmish battle entry (DESIGN_MULTIPLAYER
// §8.7, contract M2-C8). Battle entry captures its sources before anything
// compiles from them, prepares the catalog and resolves the map through that
// capture, then freezes the battle's inputs: every later simulation read —
// terrain, the AI profile, the animation table and, for the whole battle, each
// unit's model and program at creation — comes from that frozen value, never
// from the live mounts. Single-player behaviour is unchanged; what changes is
// that a file edited during a battle cannot reach it, and the next battle sees
// the edit.

import (
	"encoding/hex"
	"errors"
	"strings"

	"github.com/nanolathe-gg/nanolathe/internal/community"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/mission"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// frozenInputsView is the sealed capture view a battle's frozen inputs hand
// to composition.
type frozenInputsView interface {
	SimulationInputs() *content.SimulationInputs
}

// frozenInputsOf recovers the frozen inputs a view belongs to, or nil for any
// other file system.
func frozenInputsOf(fs vfs.FSOps) *content.SimulationInputs {
	if view, ok := fs.(frozenInputsView); ok {
		return view.SimulationInputs()
	}
	return nil
}

// skirmishSimulationRequest names a skirmish's frozen inputs: the prepared
// catalog, the OTA and TNT map resolution selected with the schema it chose,
// the profile the planner will load, the effective Community table and the
// mutators preparation applied.
func skirmishSimulationRequest(cat *content.Catalog, m *mission.Mission, entry community.Features, options SkirmishEntryOptions) content.SimulationInputRequest {
	request := content.SimulationInputRequest{
		Catalog:         cat,
		SimArt:          options.SimArt,
		AIProfile:       battleAIProfileName(m),
		CommunityDigest: communityDigest(entry),
		Mutators:        options.Mutators,
	}
	if m == nil || strings.TrimSpace(m.TerrainKey) == "" {
		return request
	}
	if cat != nil {
		if header := cat.Maps[content.CanonicalKey(m.TerrainKey)]; header != nil {
			request.MapOTA, request.MapTNT = header.LogicalOTA, header.LogicalTNT
			request.MapSchema = mapSchemaIndex(header, m.Schema.Name)
			return request
		}
	}
	// No compiled header: the paths the mission and terrain loaders fall back
	// to for this terrain key.
	request.MapOTA = "maps/" + m.TerrainKey + ".ota"
	request.MapTNT = "maps/" + strings.ToLower(strings.TrimSpace(m.TerrainKey)) + ".tnt"
	return request
}

// communityDigest is the effective Community table's identity as the frozen
// inputs record it: the SHA-256 that community.Features.Digest spells in hex.
// A spelling that does not decode leaves the zero digest, which no table
// has.
func communityDigest(f community.Features) [32]byte {
	var out [32]byte
	if digest, err := hex.DecodeString(f.Digest()); err == nil && len(digest) == len(out) {
		copy(out[:], digest)
	}
	return out
}

// mapSchemaIndex is the selected schema's index in the map header, the index
// applySchemaStrict selects by name. An unmatched name is the header's schema
// count, which the freeze records as a defined absence; entry then fails
// there.
func mapSchemaIndex(header *content.MapHeader, name string) uint32 {
	for i, schema := range header.Schemas {
		if schema.Name == name {
			return uint32(i)
		}
	}
	return uint32(len(header.Schemas))
}

// freezeSkirmishInputs freezes a skirmish's inputs from its capture.
//
// Single-player adapter policy: a host may hand battle entry an animation
// table it compiled earlier. When that table no longer matches the capture —
// an animation bank was edited since — it is not used; the battle compiles its
// own from the capture instead, as it would had the host passed none.
func freezeSkirmishInputs(sources *content.SimulationSources, request content.SimulationInputRequest) (*content.SimulationInputs, error) {
	inputs, err := content.FreezeSimulationInputs(sources, request)
	if err != nil && request.SimArt != nil && errors.Is(err, content.ErrSimulationInputNotCaptured) {
		request.SimArt = nil
		inputs, err = content.FreezeSimulationInputs(sources, request)
	}
	return inputs, err
}
