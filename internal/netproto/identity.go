package netproto

// Identity is what a seat reports before it may ready (DESIGN_MULTIPLAYER
// §8.2): one value per compared identity, each compared and diagnosed on its
// own. A seat's client builds it from its own build, frozen content and
// effective configuration; another client and the relay compare it field by
// field. A reported identity is compatibility information, not proof that a
// client is unmodified (§8.2, §13).
//
// Design reading: platform variant and binary hashes are not fields. The
// common build digest already names every admitted variant of a release
// (§8.7), so a seat on another admitted platform reports the same Build; a
// binary hash is a detached attestation keyed by that digest and a variant,
// never a reason to refuse another admitted variant (M2-C10).
type Identity struct {
	// Protocol is the command schema version the seat speaks.
	Protocol uint16
	// Build is the common build manifest's digest (`nanolathe/sim-build/1`):
	// the release's, or a development room's explicit common manifest's.
	Build [DigestBytes]byte
	// Content is the frozen simulation content's digest
	// (`nanolathe/sim-content/1`).
	Content [DigestBytes]byte
	// Map is the identity of the map inputs the battle reads, separately
	// diagnosable from Content although Content covers them.
	Map [DigestBytes]byte
	// Rules names the rule set and its effective extension table.
	Rules RuleIdentity
	// Mod is the mounted mod's public identity; zero for base content.
	Mod ModIdentity
	// Configuration is the effective match configuration's digest
	// (`nanolathe/match-config/1`).
	Configuration [DigestBytes]byte
}

// RuleIdentity is the rule set a seat runs: its registered name, its
// reserved base in the configuration's wire numbering, and the digest of its
// effective Community table (§8.2 "Rules").
type RuleIdentity struct {
	Name      string
	Base      uint8
	Community [DigestBytes]byte
}

// ModIdentity is a mod's public identity: id, version and archive SHA-256
// (§8.2 "Mod", §8.6 field 8). Base content is the zero value.
type ModIdentity struct {
	ID      string
	Version string
	Archive [DigestBytes]byte
}
