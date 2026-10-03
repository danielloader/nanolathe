package session

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/netproto"
	"github.com/nanolathe-gg/nanolathe/internal/version"
)

// The identity comparison a seat runs before it may ready
// (docs/DESIGN_MULTIPLAYER.md §8.2, §8.8, contract M2-C10). A seat reports
// one identity per compared kind — protocol, build, content, map, rules, mod
// and configuration — and every comparison runs on its own: a matching
// configuration digest does not excuse a different content digest, a
// matching content digest does not excuse a different map identity, and no
// later initial-state digest replaces any of them. Every mismatch is
// reported at once, each wrapping its own category. These are Nanolathe
// protocol values, not retail findings.
//
// The comparison does not establish this seat's own admission:
// ValidateMatchInputs does, and a seat runs it on the configuration and the
// inputs FreezeMatchInputs froze for it before it reports or compares.

// The join refusals beside admission's. Content, map and rules mismatches
// wrap admission's ErrMatchContentMismatch, ErrMatchMapMismatch and
// ErrMatchRulesMismatch, and a configuration mismatch wraps
// ErrMatchConfigurationRejected, so one category answers errors.Is whether
// the local admission or the comparison with another seat refused it.
var (
	// ErrMatchProtocolMismatch: the seat speaks another command schema
	// version.
	ErrMatchProtocolMismatch = errors.New("nanolathe: match join rejected the protocol")
	// ErrMatchBuildMismatch: the seat runs another common build, or a build
	// with no admissible identity for this room.
	ErrMatchBuildMismatch = errors.New("nanolathe: match join rejected the build")
	// ErrMatchModMismatch: the seat mounted another mod, or this seat's mount
	// is not the configuration's mod.
	ErrMatchModMismatch = errors.New("nanolathe: match join rejected the mod")
)

// matchMapDomain is the literal domain of the map identity.
const matchMapDomain = "nanolathe/match-map/1"

// MatchBuild is the build a seat joins a room with (§8.7, M2-C10).
type MatchBuild struct {
	// Running is this binary's manifest as version.CurrentBuildManifest
	// reports it: stamped for a release build, unstamped for a development
	// build or a dirty tree.
	Running version.BuildManifest
	// Development is a development room's explicit common build manifest, nil
	// for a normal room. A normal room admits only a stamped release build. A
	// development room admits the build its explicit manifest names: an
	// unstamped build joins under that manifest, and a stamped build only if
	// it is that manifest. The manifest is the room's agreed value, never a
	// flag read from the environment or inferred from a clean tree.
	Development *version.BuildManifest
}

// MatchJoin is what a seat readies with: its build, the content frozen for
// the agreed configuration, that configuration, and the mod its own mount
// applied, as its mod library names it (zero for base content).
type MatchJoin struct {
	Build  MatchBuild
	Inputs *content.SimulationInputs
	Config EffectiveMatchConfig
	Mod    MatchMod
}

// Identity is what this seat reports before it may ready. Every field this
// seat cannot establish is zero and named in the returned error, by category:
// an unstamped build in a normal room, a development manifest that cannot be
// digested, missing inputs or configuration, or a mount whose mod is not the
// configuration's.
func (j MatchJoin) Identity() (netproto.Identity, error) {
	id, errs := j.identity()
	return id, errors.Join(errs...)
}

// identity builds the seat's identity and the refusals of its own fields.
func (j MatchJoin) identity() (netproto.Identity, []error) {
	var errs []error
	id := netproto.Identity{Protocol: netproto.CommandSchemaVersion}
	if d, err := j.Build.digest(); err != nil {
		errs = append(errs, err)
	} else {
		id.Build = d
	}
	if j.Inputs == nil {
		errs = append(errs,
			matchJoinError(ErrMatchContentMismatch, "content", "the simulation inputs frozen for the agreed configuration"),
			matchJoinError(ErrMatchMapMismatch, "map", "the simulation inputs whose map files the battle reads"))
	} else {
		id.Content = j.Inputs.Digest()
		id.Map = MatchMapIdentity(j.Inputs)
	}
	if j.Config.encoding == nil {
		errs = append(errs,
			matchJoinError(ErrMatchRulesMismatch, "rules", "the agreed configuration's rule set"),
			matchJoinError(ErrMatchConfigurationRejected, "configuration", "a resolved match configuration"))
	} else {
		r := &j.Config.request
		id.Rules = netproto.RuleIdentity{Name: r.RuleName, Base: uint8(r.RuleBase), Community: communityDigest(r.Community)}
		id.Configuration = j.Config.Digest()
		if j.Mod != r.Mod {
			errs = append(errs, matchJoinError(ErrMatchModMismatch, "mod",
				fmt.Sprintf("this seat's mounted mod %s to be the configuration's %s", describeMatchMod(j.Mod), describeMatchMod(r.Mod))))
		}
	}
	id.Mod = netproto.ModIdentity{ID: j.Mod.ID, Version: j.Mod.Version, Archive: j.Mod.Archive}
	return id, errs
}

// CompareMatchIdentity compares another seat's reported identity with this
// seat's and reports every mismatch at once, each wrapping its category:
// ErrMatchProtocolMismatch, ErrMatchBuildMismatch, ErrMatchContentMismatch,
// ErrMatchMapMismatch, ErrMatchRulesMismatch, ErrMatchModMismatch and
// ErrMatchConfigurationRejected, in that order. A field this seat cannot
// establish is refused in its own category instead of compared; nothing is
// skipped because another identity matched. A nil result admits the pair on
// identity alone; it is not proof that the other client is unmodified
// (§8.2, §13).
func CompareMatchIdentity(local MatchJoin, remote netproto.Identity) error {
	id, own := local.identity()
	var errs []error
	for _, kind := range matchJoinKinds {
		established := true
		for _, err := range own {
			if errors.Is(err, kind) {
				errs = append(errs, err)
				established = false
			}
		}
		if established {
			if err := compareMatchKind(kind, id, remote); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// matchJoinKinds is the categories in report order.
var matchJoinKinds = []error{
	ErrMatchProtocolMismatch, ErrMatchBuildMismatch, ErrMatchContentMismatch, ErrMatchMapMismatch,
	ErrMatchRulesMismatch, ErrMatchModMismatch, ErrMatchConfigurationRejected,
}

// compareMatchKind compares one category of this seat's established identity
// with the other seat's.
func compareMatchKind(kind error, local, remote netproto.Identity) error {
	switch kind {
	case ErrMatchProtocolMismatch:
		if remote.Protocol != local.Protocol {
			return matchJoinError(kind, "protocol", fmt.Sprintf("command schema version %d, the other seat's is %d", local.Protocol, remote.Protocol))
		}
	case ErrMatchBuildMismatch:
		if remote.Build != local.Build {
			return matchJoinError(kind, "build", fmt.Sprintf("common build %s, the other seat's is %s", shortDigest(local.Build), shortDigest(remote.Build)))
		}
	case ErrMatchContentMismatch:
		if remote.Content != local.Content {
			return matchJoinError(kind, "content", fmt.Sprintf("simulation content %s, the other seat's is %s", shortDigest(local.Content), shortDigest(remote.Content)))
		}
	case ErrMatchMapMismatch:
		if remote.Map != local.Map {
			return matchJoinError(kind, "map", fmt.Sprintf("map inputs %s, the other seat's are %s", shortDigest(local.Map), shortDigest(remote.Map)))
		}
	case ErrMatchRulesMismatch:
		if remote.Rules != local.Rules {
			return matchJoinError(kind, "rules", fmt.Sprintf("rule set %q base %d with Community table %s, the other seat's is %q base %d with %s",
				local.Rules.Name, local.Rules.Base, shortDigest(local.Rules.Community), remote.Rules.Name, remote.Rules.Base, shortDigest(remote.Rules.Community)))
		}
	case ErrMatchModMismatch:
		if remote.Mod != local.Mod {
			return matchJoinError(kind, "mod", fmt.Sprintf("mod %s, the other seat's is %s",
				describeMatchMod(MatchMod{ID: local.Mod.ID, Version: local.Mod.Version, Archive: local.Mod.Archive}),
				describeMatchMod(MatchMod{ID: remote.Mod.ID, Version: remote.Mod.Version, Archive: remote.Mod.Archive})))
		}
	case ErrMatchConfigurationRejected:
		if remote.Configuration != local.Configuration {
			return matchJoinError(kind, "configuration", fmt.Sprintf("configuration %s, the other seat's is %s", shortDigest(local.Configuration), shortDigest(remote.Configuration)))
		}
	}
	return nil
}

// digest is the common build identity this seat joins under.
//
// Design reading (§8.7, M2-C10): the build identity is the common
// manifest's digest alone. A platform variant differs inside one common
// manifest, so another admitted variant of the same release reports the same
// digest; binary hashes are detached attestations, never a reason to refuse
// it. A stamped build in a development room must be that room's manifest, so
// a release cannot pass for another build.
func (b MatchBuild) digest() ([32]byte, error) {
	if b.Development == nil {
		if !b.Running.Stamped() {
			return [32]byte{}, fmt.Errorf("%w: %w", matchJoinError(ErrMatchBuildMismatch, "build",
				"a stamped release build; an unstamped or dirty build joins only a development room that names an explicit common manifest"), version.ErrUnstampedBuild)
		}
		d, err := b.Running.Digest()
		if err != nil {
			return [32]byte{}, fmt.Errorf("%w: %w", matchJoinError(ErrMatchBuildMismatch, "build", "a build manifest that encodes"), err)
		}
		return d, nil
	}
	common, err := b.Development.Digest()
	if err != nil {
		return [32]byte{}, fmt.Errorf("%w: %w", matchJoinError(ErrMatchBuildMismatch, "build.development",
			"a development room's explicit common manifest, stamped from its source inventory"), err)
	}
	if b.Running.Stamped() {
		if d, err := b.Running.Digest(); err != nil || d != common {
			return [32]byte{}, matchJoinError(ErrMatchBuildMismatch, "build",
				fmt.Sprintf("this stamped build to be the development room's common manifest %s", shortDigest(common)))
		}
	}
	return common, nil
}

// MatchMapIdentity is the identity of the map inputs a battle reads (§8.2
// "Map"): SHA-256 of the literal domain `nanolathe/match-map/1` and the
// frozen manifest's map-family entries — entry count u32, then each entry's
// Key text, Ordinal u32, Presence u8, SemanticDigest digest and, for the
// fallback state, FallbackKey text — in manifest order.
//
// Design reading: the map family already digests the selected OTA and TNT
// bytes, the translated-name fallback table, the compiled header and the
// selected schema (U4), so the map identity composes those entries rather
// than hashing files again. The content identity covers the same entries;
// this one keeps a map difference separately diagnosable. The map's name is
// configuration, not a file the battle reads.
func MatchMapIdentity(inputs *content.SimulationInputs) [32]byte {
	var entries []content.SimulationInput
	for _, e := range inputs.Manifest() {
		if e.Family == content.SimulationFamilyMap {
			entries = append(entries, e)
		}
	}
	var w netproto.Writer
	w.U32(uint32(len(entries)))
	for _, e := range entries {
		w.Text(e.Key)
		w.U32(e.Ordinal)
		w.U8(e.Presence)
		w.Digest(e.SemanticDigest)
		if e.Presence == content.SimulationInputFallback {
			w.Text(e.FallbackKey)
		}
	}
	h := sha256.New()
	h.Write([]byte(matchMapDomain))
	h.Write(w.Bytes())
	var out [32]byte
	h.Sum(out[:0])
	return out
}

// matchJoinError is the one diagnostic shape of a join refusal.
func matchJoinError(kind error, path, expected string) error {
	return fmt.Errorf("%w: logical path %s, providers searched [match identity v1, this seat, other seat], expected %s", kind, path, expected)
}

func shortDigest(d [32]byte) string { return hex.EncodeToString(d[:8]) }

func describeMatchMod(m MatchMod) string {
	if m == (MatchMod{}) {
		return "base content"
	}
	return fmt.Sprintf("%q %q %s", m.ID, m.Version, shortDigest(m.Archive))
}
