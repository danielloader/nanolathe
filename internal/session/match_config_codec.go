package session

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/nanolathe-gg/nanolathe/internal/ai"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/survival"
)

// The wire form of configuration version 1 (docs/DESIGN_MULTIPLAYER.md §8.6)
// in the primitives of §7.4.1: u8 and bool are one byte, a bool exactly 0 or
// 1; u16, u32 and u64 are the shortest unsigned base-128 varint within their
// width; s32 is the zigzag of the 32-bit value, then a varint; text(N) is a
// u32 byte length and at most N bytes of UTF-8 without NUL; key is a u16 byte
// length and 1..255 bytes of a canonical content key; digest is 32 raw bytes
// and id 16. Collections carry a u32 count. There are no optional fields and
// no trailing bytes, and a field addition is a new version.
//
// Design reading: the payload carries no version of its own. Version 1 is
// named by the identity domain and by the envelope that carries the payload,
// as the command schema version is negotiated outside a command payload
// (§7.4.1).

// matchConfigDomain is the literal UTF-8 domain the identity hashes before
// the encoding, with no trailing NUL.
const matchConfigDomain = "nanolathe/match-config/1"

// EncodeMatchConfig returns the canonical encoding of a resolved
// configuration: a fresh copy of the bytes it was frozen with. The zero value
// is no configuration and has no encoding.
func EncodeMatchConfig(c EffectiveMatchConfig) ([]byte, error) {
	if c.encoding == nil {
		return nil, matchFieldError("configuration", "a configuration ResolveMatchConfig or DecodeMatchConfig produced")
	}
	return append([]byte(nil), c.encoding...), nil
}

// DecodeMatchConfig reads a version 1 payload. It refuses a payload over 2
// MiB, a count or length over its bound before allocating from it, an
// overlong or overflowing integer, a boolean other than 0 or 1, any field
// ResolveMatchConfig refuses, trailing bytes, and any encoding that is not
// the one the decoded value re-encodes to. It never fills a default and
// returns no partially valid value.
func DecodeMatchConfig(payload []byte) (EffectiveMatchConfig, error) {
	if len(payload) > matchConfigMaxBytes {
		return EffectiveMatchConfig{}, matchFieldError("payload", fmt.Sprintf("at most %d bytes, got %d", matchConfigMaxBytes, len(payload)))
	}
	r := matchReader{b: payload}
	request := r.request()
	if r.err != nil {
		return EffectiveMatchConfig{}, r.err
	}
	if r.off != len(payload) {
		return EffectiveMatchConfig{}, matchFieldError(fmt.Sprintf("payload byte %d", r.off), "no bytes after the last field")
	}
	c, err := ResolveMatchConfig(request)
	if err != nil {
		return EffectiveMatchConfig{}, err
	}
	if !bytes.Equal(c.encoding, payload) {
		return EffectiveMatchConfig{}, matchFieldError("payload", "the canonical encoding of the value it decodes to")
	}
	return c, nil
}

// Digest is the configuration identity: SHA-256 of the domain and the
// canonical encoding. It cannot fail, because a value exists only after
// resolution validated it; the zero value, no configuration, digests to
// zero.
func (c EffectiveMatchConfig) Digest() [32]byte {
	if c.encoding == nil {
		return [32]byte{}
	}
	h := sha256.New()
	h.Write([]byte(matchConfigDomain))
	h.Write(c.encoding)
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

// encodeMatchRequest writes a request validate has accepted.
func encodeMatchRequest(r *MatchConfigRequest) ([]byte, error) {
	var w matchWriter
	w.u8(uint8(r.SessionKind)) // 1
	w.key(r.RuleName)          // 2
	w.u8(uint8(r.RuleBase))
	w.text(r.MapName) // 3
	w.uvarint(uint64(r.MapSchema))
	w.u8(uint8(len(r.Seats))) // 4
	for i := range r.Seats {
		w.seat(&r.Seats[i])
	}
	w.u8(r.Location) // 5
	w.u8(r.CommanderDeath)
	w.u8(r.Mapping)
	w.u8(r.LineOfSight)
	w.u8(r.LOSType)
	w.uvarint(uint64(r.UnitLimit)) // 6
	w.uvarint(uint64(r.SimulationSeed))
	w.uvarint(uint64(r.CRTSeed))
	w.u8(uint8(r.SurvivalPace)) // 7
	w.boolean(r.SurvivalNoAir)
	w.boolean(r.SurvivalNoNaval)
	w.text(r.Mod.ID) // 8
	w.text(r.Mod.Version)
	w.b = append(w.b, r.Mod.Archive[:]...)
	p := &r.ContentProfile // 9
	w.text(p.Name)
	w.uvarint(uint64(len(p.Directories)))
	for _, d := range p.Directories {
		w.text(d.From)
		w.text(d.To)
	}
	w.uvarint(uint64(p.Units))
	w.uvarint(uint64(p.Weapons))
	w.uvarint(p.TNTBytes)
	w.uvarint(p.LOSBytes)
	community, err := json.Marshal(r.Community) // 10, validated by validate
	if err != nil {
		return nil, matchFieldError("community", "a table with a JSON form: "+err.Error())
	}
	w.uvarint(uint64(len(community)))
	w.b = append(w.b, community...)
	for _, f := range matchMutatorFields(&r.Mutators) { // 11
		index, _ := matchMutatorIndex(*f)
		w.u8(index)
	}
	w.uvarint(uint64(len(r.UnitRestrictions))) // 12
	for _, u := range r.UnitRestrictions {
		w.uvarint(uint64(u.DefinitionID))
		w.key(u.Unit)
		w.u8(u.Limit)
	}
	w.boolean(r.CheatsAllowed) // 13
	w.boolean(r.WatchingAllowed)
	for _, v := range []MatchView{r.PlayerView, r.SpectatorView, r.ReplayView} { // 14
		w.uvarint(uint64(v.MinimumScale))
		w.uvarint(uint64(v.MaximumScale))
		w.boolean(v.FullMap)
	}
	pol := r.Policies // 15
	w.uvarint(uint64(pol.Revision))
	w.u8(pol.Scheduling)
	w.u8(pol.Pacing)
	w.u8(pol.Drop)
	w.u8(pol.Audience)
	w.uvarint(uint64(pol.RejoinGraceMilliseconds))
	w.uvarint(uint64(pol.SpectatorDelayMilliseconds))
	w.uvarint(uint64(pol.ReplayReleaseDelayMilliseconds))
	return w.b, nil
}

// matchWriter appends §7.4.1 primitives. It writes values validate has
// already bounded.
type matchWriter struct{ b []byte }

func (w *matchWriter) u8(v uint8) { w.b = append(w.b, v) }

func (w *matchWriter) boolean(v bool) {
	if v {
		w.b = append(w.b, 1)
	} else {
		w.b = append(w.b, 0)
	}
}

func (w *matchWriter) uvarint(v uint64) {
	for v >= 0x80 {
		w.b = append(w.b, byte(v)|0x80)
		v >>= 7
	}
	w.b = append(w.b, byte(v))
}

// s32 writes the zigzag form of a signed 32-bit value.
func (w *matchWriter) s32(v int32) { w.uvarint(uint64(uint32(v<<1) ^ uint32(v>>31))) }

func (w *matchWriter) text(s string) {
	w.uvarint(uint64(len(s)))
	w.b = append(w.b, s...)
}

func (w *matchWriter) key(s string) {
	w.uvarint(uint64(len(s)))
	w.b = append(w.b, s...)
}

func (w *matchWriter) seat(s *MatchSeat) {
	w.u8(uint8(s.Role))
	w.u8(s.Side)
	w.u8(s.Color)
	w.u8(s.AllyGroup)
	w.text(s.Nickname)
	w.s32(s.Metal)
	w.s32(s.Energy)
	w.b = append(w.b, s.Participant[:]...)
	w.u8(s.HostSeat)
	w.u8(uint8(s.ComputerKind))
	w.u8(s.Difficulty)
	w.boolean(s.SharedVictory)
	for _, g := range s.BuilderOptions.Guard {
		w.u8(uint8(g))
	}
	for _, p := range s.BuilderOptions.Patrol {
		w.u8(uint8(p))
	}
	w.uvarint(uint64(len(s.AIParams)))
	for _, p := range s.AIParams {
		w.text(p.Key)
		w.text(p.Value)
	}
}

// matchUvarintLen is the length of v's shortest varint.
func matchUvarintLen(v uint64) int {
	n := 1
	for v >= 0x80 {
		v >>= 7
		n++
	}
	return n
}

// matchReader reads §7.4.1 primitives. The first failure sticks: every later
// read returns zero, so a decode reports exactly one error and builds nothing
// from a malformed tail.
type matchReader struct {
	b   []byte
	off int
	err error
}

func (r *matchReader) fail(expected string) {
	if r.err == nil {
		r.err = matchFieldError(fmt.Sprintf("payload byte %d", r.off), expected)
	}
}

func (r *matchReader) u8() uint8 {
	if r.err != nil {
		return 0
	}
	if r.off >= len(r.b) {
		r.fail("another byte")
		return 0
	}
	v := r.b[r.off]
	r.off++
	return v
}

func (r *matchReader) boolean() bool {
	v := r.u8()
	if v > 1 {
		r.off--
		r.fail("a boolean of exactly 0 or 1")
		return false
	}
	return v == 1
}

// uvarint reads the shortest varint of a value that fits bits.
func (r *matchReader) uvarint(bits uint) uint64 {
	if r.err != nil {
		return 0
	}
	start := r.off
	var v uint64
	for i := 0; ; i++ {
		if r.off >= len(r.b) {
			r.fail("a complete varint")
			return 0
		}
		c := r.b[r.off]
		r.off++
		if i == 9 && c > 1 {
			r.off = start
			r.fail("a varint within 64 bits")
			return 0
		}
		v |= uint64(c&0x7f) << (7 * uint(i))
		if c < 0x80 {
			if i > 0 && c == 0 {
				r.off = start
				r.fail("the shortest varint")
				return 0
			}
			if bits < 64 && v>>bits != 0 {
				r.off = start
				r.fail(fmt.Sprintf("a value within %d bits", bits))
				return 0
			}
			return v
		}
	}
}

func (r *matchReader) s32() int32 {
	u := uint32(r.uvarint(32))
	return int32(u>>1) ^ -int32(u&1)
}

// bytes reads n raw bytes, after checking that they are present.
func (r *matchReader) bytes(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || n > len(r.b)-r.off {
		r.fail(fmt.Sprintf("%d more bytes", n))
		return nil
	}
	out := r.b[r.off : r.off+n]
	r.off += n
	return out
}

// text reads a text(max) value; its content is checked by validate.
func (r *matchReader) text(max int) string {
	n := r.uvarint(32)
	if r.err != nil {
		return ""
	}
	if n > uint64(max) {
		r.fail(fmt.Sprintf("a text of at most %d bytes", max))
		return ""
	}
	return string(r.bytes(int(n)))
}

// key reads a key value's u16 length and bytes.
func (r *matchReader) key() string {
	n := r.uvarint(16)
	if r.err != nil {
		return ""
	}
	if n < 1 || n > matchKeyMaxBytes {
		r.fail(fmt.Sprintf("a key of 1..%d bytes", matchKeyMaxBytes))
		return ""
	}
	return string(r.bytes(int(n)))
}

// count reads a collection's u32 count and checks it against the bound and
// against the bytes remaining at minRecord bytes a record, before anything
// is allocated from it.
func (r *matchReader) count(max, minRecord int) int {
	n := r.uvarint(32)
	if r.err != nil {
		return 0
	}
	if n > uint64(max) {
		r.fail(fmt.Sprintf("a count of at most %d", max))
		return 0
	}
	if n*uint64(minRecord) > uint64(len(r.b)-r.off) {
		r.fail(fmt.Sprintf("%d records of at least %d bytes", n, minRecord))
		return 0
	}
	return int(n)
}

func (r *matchReader) request() MatchConfigRequest {
	var q MatchConfigRequest
	q.SessionKind = MatchSessionKind(r.u8()) // 1
	q.RuleName = r.key()                     // 2
	q.RuleBase = MatchRuleBase(r.u8())
	q.MapName = r.text(matchMapNameMaxBytes) // 3
	q.MapSchema = uint32(r.uvarint(32))
	seats := r.u8() // 4
	if r.err == nil && (seats < matchMinSeats || seats > SkirmishMaxPlayers) {
		r.off--
		r.fail(fmt.Sprintf("%d..%d seats", matchMinSeats, SkirmishMaxPlayers))
	}
	if r.err != nil {
		return MatchConfigRequest{}
	}
	q.Seats = make([]MatchSeat, seats)
	for i := range q.Seats {
		q.Seats[i] = r.seat()
	}
	q.Location = r.u8() // 5
	q.CommanderDeath = r.u8()
	q.Mapping = r.u8()
	q.LineOfSight = r.u8()
	q.LOSType = r.u8()
	q.UnitLimit = uint16(r.uvarint(16)) // 6
	q.SimulationSeed = uint32(r.uvarint(32))
	q.CRTSeed = uint32(r.uvarint(32))
	q.SurvivalPace = survival.Pace(r.u8()) // 7
	q.SurvivalNoAir = r.boolean()
	q.SurvivalNoNaval = r.boolean()
	q.Mod.ID = r.text(matchModTextMaxBytes) // 8
	q.Mod.Version = r.text(matchModTextMaxBytes)
	copy(q.Mod.Archive[:], r.bytes(32))
	p := &q.ContentProfile // 9
	p.Name = r.text(matchProfileNameMaxBytes)
	if n := r.count(matchMaxDirectories, 2); n > 0 {
		p.Directories = make([]MatchDirectory, n)
		for i := range p.Directories {
			p.Directories[i].From = r.text(matchDirFromMaxBytes)
			p.Directories[i].To = r.text(matchDirToMaxBytes)
		}
	}
	p.Units = uint32(r.uvarint(32))
	p.Weapons = uint32(r.uvarint(32))
	p.TNTBytes = r.uvarint(64)
	p.LOSBytes = r.uvarint(64)
	community := r.uvarint(32) // 10
	if r.err == nil && community > matchCommunityMaxBytes {
		r.fail(fmt.Sprintf("community JSON of at most %d bytes", matchCommunityMaxBytes))
	}
	data := r.bytes(int(community))
	if r.err == nil {
		f, err := decodeMatchCommunity(data)
		if err != nil {
			r.err = err
		}
		q.Community = f
	}
	for _, f := range matchMutatorFields(&q.Mutators) { // 11
		index := r.u8()
		factor, ok := matchMutatorFactor(index)
		if r.err == nil && !ok {
			r.off--
			r.fail("a mutator step index 0..7")
		}
		*f = factor
	}
	if n := r.count(matchMaxRestrictions, 4); n > 0 { // 12
		q.UnitRestrictions = make([]MatchUnitRestriction, n)
		for i := range q.UnitRestrictions {
			u := &q.UnitRestrictions[i]
			u.DefinitionID = uint16(r.uvarint(16))
			u.Unit = r.key()
			u.Limit = r.u8()
		}
	}
	q.CheatsAllowed = r.boolean() // 13
	q.WatchingAllowed = r.boolean()
	for _, v := range []*MatchView{&q.PlayerView, &q.SpectatorView, &q.ReplayView} { // 14
		v.MinimumScale = uint16(r.uvarint(16))
		v.MaximumScale = uint16(r.uvarint(16))
		v.FullMap = r.boolean()
	}
	pol := &q.Policies // 15
	pol.Revision = uint16(r.uvarint(16))
	pol.Scheduling = r.u8()
	pol.Pacing = r.u8()
	pol.Drop = r.u8()
	pol.Audience = r.u8()
	pol.RejoinGraceMilliseconds = uint32(r.uvarint(32))
	pol.SpectatorDelayMilliseconds = uint32(r.uvarint(32))
	pol.ReplayReleaseDelayMilliseconds = uint32(r.uvarint(32))
	if r.err != nil {
		return MatchConfigRequest{}
	}
	return q
}

func (r *matchReader) seat() MatchSeat {
	var s MatchSeat
	s.Role = MatchRole(r.u8())
	s.Side = r.u8()
	s.Color = r.u8()
	s.AllyGroup = r.u8()
	s.Nickname = r.text(matchNicknameMaxBytes)
	s.Metal = r.s32()
	s.Energy = r.s32()
	copy(s.Participant[:], r.bytes(16))
	s.HostSeat = r.u8()
	s.ComputerKind = ai.Controller(r.u8())
	s.Difficulty = r.u8()
	s.SharedVictory = r.boolean()
	for i := range s.BuilderOptions.Guard {
		s.BuilderOptions.Guard[i] = orders.GuardHomeOption(r.u8())
	}
	for i := range s.BuilderOptions.Patrol {
		s.BuilderOptions.Patrol[i] = orders.PatrolWorkOption(r.u8())
	}
	if n := r.count(matchMaxAIParams, 2); n > 0 {
		s.AIParams = make([]AIParam, n)
		for i := range s.AIParams {
			s.AIParams[i].Key = r.text(matchAIParamMaxBytes)
			s.AIParams[i].Value = r.text(matchAIParamMaxBytes)
		}
	}
	return s
}
