package content

// Authored animation metadata the SIMULATION depends on.
//
// These authoritative facts are properties of a GAF entry rather than of a TDF
// record, so they cannot be answered from the compiled catalogs alone:
//
//   - The burning-feature smoke of [05 R-FEAT-01 §10] pass 3a scales its two
//     CRT draws by the CURRENT burn frame's width and height, and offsets the
//     puff by that frame's authored origin.
//   - A die, reclaim or burn animation ends on the visit whose cursor advance
//     clears the sequence pointer — a lifetime in visits of the sum over the
//     entry's frames of max(delay, 1) [05 R-FEAT-01 §10] pass 1.
//   - A smoke puff's own last frame is drawn against the bound effect entry's
//     frame count [03 R-STRIP-01 §2][03 R-FX-01 §3][06 R-WFX-01 §5], and the
//     two flame families wrap on the same quantity for `flamestream`
//     [03 R-FX-02 §2].
//   - A fixed effect's primary player lives for its authored frame holds;
//     pool occupancy gates shatter draws [06 R-WFX-01 §1][04 R-COB-04 §3].
//
// These used to be read through the graphical client, which is the only place
// in the build that held a GAF cache. A headless run installed no resolver, so
// its features took the immediate-replacement path and its puffs were never
// retired by animation: the same battle simulated differently depending on
// whether a window was open. The table below moves that metadata to content,
// where it is compiled once from the VFS before any battle service exists and
// is then immutable, so both shells run one simulation.
//
// Three properties make it safe for an authoritative phase to read:
//
//   - The answer depends on nothing but the file's bytes, so it is identical in
//     every run over the same install (I4).
//   - Compilation walks the catalog in sorted key order, so the load order —
//     and therefore any diagnostic it could produce — is reproducible.
//   - No filesystem handle is retained. A miss at simulation time is a map
//     lookup that reports "unknown"; it can never turn into a load.
//
// Pixels are deliberately not kept here. Only geometry, per-frame delays and
// frame counts cross into content; the decoded art stays behind the
// presentation edge [I6].

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// DefaultEffectBank is the bank an effect request names when it carries no
// bank of its own. The engine's own fixed effect-slot table is bound from `fx`
// at startup [06 R-WFX-01 §1], and every entry the simulation asks about — the
// two smoke entries and `flamestream` — is one of its rows, so a request
// published without a bank is by construction one of those.
const DefaultEffectBank = "fx"

// EffectBankMaxBytes is the most of one effect bank's file either reader
// accepts. Together with EffectBankGAFLimits it is the single Nanolathe host
// loader policy for `anims/<bank>.gaf` effect banks: the simulation's timing
// table below and the graphical client's on-demand effect art both read
// through it, so every bank the client can draw is a bank whose frame holds
// time the fixed effect pool (DESIGN_PRESENTATION_CLIENT "On-demand effect
// art"). These are host safety bounds, not retail limits.
const EffectBankMaxBytes = 256 << 20

// EffectBankGAFLimits is the validation budget of that policy. It keeps the
// eager loader's reference, depth, per-frame and RLE limits and raises only
// the unique and expanded pixel budgets, to 512 Mi each, which admits the
// large banks of the audited mods (TA Zero's ModFX, Escalation's nuke and
// weapon banks) without decoding any pixel. Eager loaders keep
// formats.DefaultGAFLimits.
func EffectBankGAFLimits() formats.GAFLimits {
	limits := formats.DefaultGAFLimits()
	limits.MaxDecodedPixels, limits.MaxExpandedPixels = 512<<20, 512<<20
	return limits
}

// EffectBankPath is the logical path of a canonical effect bank name, the path
// retail constructs from the authored bank name [06 R-WFX-01 §1].
func EffectBankPath(bank string) string { return "anims/" + bank + ".gaf" }

// EffectBankDiagnostic records one effect bank SimArt could not compile. The
// bank's events then have no frame holds, so their primary players stay
// inactive and the records retire sooner than a drawn bank's would; the
// battle still runs, as the client still draws nothing for that bank.
type EffectBankDiagnostic struct {
	// Bank is the canonical bank name a weapon or the default slot table names.
	Bank string
	// Path is the logical path searched.
	Path string
	// Providers lists the content providers that hold the path; it is empty
	// when no mounted provider supplies the file.
	Providers []string
	// Reason is the read or validation error, with a filesystem path error
	// reduced to its operation and cause.
	Reason string
}

// String renders the diagnostic in the engine's diagnostic shape.
func (d EffectBankDiagnostic) String() string {
	return fmt.Sprintf("nanolathe: effect bank timing unavailable: logical path %s, providers searched [%s], expected an animation bank within the effect-bank loader policy for bank %q: %s",
		d.Path, strings.Join(d.Providers, ", "), d.Bank, d.Reason)
}

// simArtFrame is one frame's contribution: the geometry pass 3a scales by, and
// the authored delay that decides how many visits it holds for.
type simArtFrame struct {
	w, h       int32
	xoff, yoff int32
	delay      int32
}

// simArtSequence is one compiled entry. A nil value in the sequence map is a
// compiled miss — an absent file, an absent sequence, or an empty one.
type simArtSequence struct {
	visits int32
	frames []simArtFrame
}

// SimArt is the immutable simulation-facing animation metadata for one battle.
// The zero value and a nil pointer both answer "unknown" to every question,
// which is the same answer an unresolvable entry gives.
type SimArt struct {
	// effects maps "bank|entry" (both lower-cased) to immutable per-frame holds.
	effects map[string][]int32
	// sequences maps "filename|sequence" (both lower-cased) to the compiled
	// entry, or to nil for a compiled miss.
	sequences map[string]*simArtSequence
	// diagnostics names every effect bank that did not compile, in the sorted
	// bank order compilation visits them. Load-time host diagnostics only: no
	// phase reads them.
	diagnostics []EffectBankDiagnostic
}

// CompileSimArt builds the table from the battle's VFS and compiled catalog.
//
// It compiles exactly what the simulation asks for and nothing else: the
// default and weapon-named effect banks' frame holds, and the three EVENT
// sequences plus their three shadow twins of every feature definition — burn, die and
// reclamate. Shadow entries do not time a cursor, but attachment must record
// whether the named shadow resolved so the runtime draw path can admit it.
// A definition's rest sequences remain presentation-only and stay out of this
// table.
//
// A file or entry that will not resolve is simply absent from the table, which
// makes every consumer report "unknown" rather than a plausible substitute
// [I9]. Compilation never fails: a missing animation bank is a content gap, not
// a reason to refuse a battle. An effect bank that is absent or refused by the
// loader policy is recorded in Diagnostics, as the client records the same
// bank when it cannot draw it.
func CompileSimArt(fs vfs.FSOps, cat *Catalog) *SimArt {
	art := &SimArt{
		effects:   make(map[string][]int32),
		sequences: make(map[string]*simArtSequence),
	}
	if fs == nil {
		return art
	}
	// Gather bank names first and load each once, in sorted order (I4).
	banks := map[string]struct{}{DefaultEffectBank: {}}
	if cat != nil {
		for _, def := range cat.Weapons {
			if def == nil {
				continue
			}
			for _, name := range [...]string{def.ExplosionGaf, def.WaterExplosionGaf, def.LavaExplosionGaf} {
				if bank := CanonicalKey(name); bank != "" {
					banks[bank] = struct{}{}
				}
			}
		}
	}
	bankNames := make([]string, 0, len(banks))
	for bank := range banks {
		bankNames = append(bankNames, bank)
	}
	sort.Strings(bankNames)
	for _, bank := range bankNames {
		art.compileEffectBank(fs, bank)
	}
	if cat == nil || len(cat.Features) == 0 {
		return art
	}
	keys := make([]string, 0, len(cat.Features))
	for key := range cat.Features {
		keys = append(keys, key)
	}
	// Sorted, so the order files are opened in — and therefore anything that
	// could observe that order — is the same in every run (I4).
	sort.Strings(keys)
	// featureBanks memoises one validated metadata index per feature filename,
	// including the failures: a nil value means "tried, absent".
	featureBanks := make(map[string]*formats.GAFMetadata)
	for _, key := range keys {
		def := cat.Features[key]
		if def == nil || trimTDFSemantic(def.Filename) == "" {
			continue
		}
		for _, seq := range [...]string{
			def.SeqNameBurn, def.SeqNameDie, def.SeqNameReclamate,
			def.SeqNameBurnShad, def.SeqNameDieShad, def.SeqNameReclamateShad,
		} {
			if trimTDFSemantic(seq) == "" {
				continue
			}
			art.compileFeatureSequence(fs, featureBanks, def.Filename, seq)
		}
	}
	return art
}

// compileEffectBank records every entry's frame holds in one animation bank. The
// bank's logical path is `anims/<name>.gaf`, the path retail constructs from
// the authored bank name [06 R-WFX-01 §1]; the name is ASCII-folded per the VFS
// canonical rules (I1). It reads under the shared effect-bank loader policy,
// never the eager default, so it accepts exactly the banks the client draws.
func (a *SimArt) compileEffectBank(fs vfs.FSOps, name string) {
	bank := CanonicalKey(name)
	if bank == "" {
		bank = DefaultEffectBank
	}
	path := EffectBankPath(bank)
	gaf, err := formats.LoadGAFMetadataFileWithLimits(fs, path, EffectBankMaxBytes, EffectBankGAFLimits())
	if err != nil {
		a.diagnostics = append(a.diagnostics, EffectBankDiagnostic{
			Bank: bank, Path: path, Providers: searchedProviderIDs(fs, path),
			Reason: portableContentCause(err).Error(),
		})
		return
	}
	for i := range gaf.Entries {
		entry := &gaf.Entries[i]
		key := simArtEffectKey(bank, entry.Name)
		// Entry lookup takes the first case-insensitive match [06 R-WFX-01 §1].
		if _, exists := a.effects[key]; exists {
			continue
		}
		holds := make([]int32, len(entry.Frames))
		for j, ref := range entry.Frames {
			holds[j] = max(int32(ref.Value), 1) // [06 R-WFX-01 §1]
		}
		a.effects[key] = holds
	}
}

// compileFeatureSequence compiles one named sequence out of one feature GAF,
// recording both hits and misses so the sim path never has to distinguish
// "not compiled" from "does not exist".
func (a *SimArt) compileFeatureSequence(fs vfs.FSOps, banks map[string]*formats.GAFMetadata, filename, sequence string) {
	key := simArtSequenceKey(filename, sequence)
	if _, seen := a.sequences[key]; seen {
		return
	}
	a.sequences[key] = nil
	file := CanonicalKey(filename)
	gaf, loaded := banks[file]
	if !loaded {
		// The feature sprite source is the TDF `filename` stem without an
		// extension [02 "Feature record"]; the path is lower-cased per the VFS
		// canonical rules (I1).
		g, err := formats.LoadGAFMetadataFile(fs, "anims/"+file+".gaf")
		if err != nil {
			g = nil
		}
		banks[file] = g
		gaf = g
	}
	if gaf == nil {
		return
	}
	entry, found := gaf.Find(sequence)
	if !found || entry == nil || len(entry.Frames) == 0 {
		return
	}
	info := &simArtSequence{frames: make([]simArtFrame, 0, len(entry.Frames))}
	for i := range entry.Frames {
		ref := entry.Frames[i]
		if ref.Frame == nil {
			continue
		}
		info.frames = append(info.frames, simArtFrame{
			w:     int32(ref.Frame.Width),
			h:     int32(ref.Frame.Height),
			xoff:  int32(ref.Frame.XOffset),
			yoff:  int32(ref.Frame.YOffset),
			delay: int32(ref.Value),
		})
		info.visits += simArtHoldVisits(int32(ref.Value))
	}
	if len(info.frames) == 0 {
		return
	}
	a.sequences[key] = info
}

// EffectEntryFrameCount reports how many frames one effect entry holds. An
// empty bank name selects the default bank, as an effect published without one
// does [06 R-WFX-01 §1]. An entry that was not compiled reports ok=false, and
// the caller keeps its documented "unknown" behaviour rather than receiving an
// invented length.
func (a *SimArt) EffectEntryFrameCount(bank, entry string) (int, bool) {
	if a == nil || len(a.effects) == 0 {
		return 0, false
	}
	if entry == "" {
		return 0, false
	}
	holds, ok := a.effects[simArtEffectKey(bank, entry)]
	if !ok || len(holds) == 0 {
		return 0, false
	}
	return len(holds), true
}

// EffectEntryHolds reports each frame's max(authored hold, 1) advances
// [06 R-WFX-01 §1]. An empty bank selects fx; missing or empty entries report
// unknown. The caller owns the returned copy, keeping compiled content immutable.
func (a *SimArt) EffectEntryHolds(bank, entry string) ([]int32, bool) {
	if a == nil || entry == "" {
		return nil, false
	}
	holds, ok := a.effects[simArtEffectKey(bank, entry)]
	if !ok || len(holds) == 0 {
		return nil, false
	}
	return append([]int32(nil), holds...), true
}

// Diagnostics reports every effect bank compilation could not read, in sorted
// bank order, as a fresh copy. A battle entry surfaces them as load-time host
// diagnostics; a nil table reports none.
func (a *SimArt) Diagnostics() []EffectBankDiagnostic {
	if a == nil || len(a.diagnostics) == 0 {
		return nil
	}
	out := make([]EffectBankDiagnostic, len(a.diagnostics))
	for i, d := range a.diagnostics {
		d.Providers = append([]string(nil), d.Providers...)
		out[i] = d
	}
	return out
}

// FeatureSequence reports the geometry of the frame a cursor is on after
// `visit` visits, and the whole entry's lifetime in visits; ok is false when
// the sequence does not resolve, and the caller then has no geometry and no
// length.
//
// The visit index walks the same cadence the cursor does: frame i holds for
// max(delay, 1) visits [05 R-FEAT-01 §10] pass 1. A visit at or past the
// entry's end reports the last frame, which is where a cursor sits on the visit
// that finishes it.
func (a *SimArt) FeatureSequence(filename, sequence string, visit int32) (w, h, xoff, yoff, visits int32, ok bool) {
	if a == nil || len(a.sequences) == 0 {
		return 0, 0, 0, 0, 0, false
	}
	if trimTDFSemantic(filename) == "" || trimTDFSemantic(sequence) == "" {
		return 0, 0, 0, 0, 0, false
	}
	info := a.sequences[simArtSequenceKey(filename, sequence)]
	if info == nil || len(info.frames) == 0 {
		return 0, 0, 0, 0, 0, false
	}
	f := info.frames[len(info.frames)-1]
	if visit < 0 {
		visit = 0
	}
	elapsed := int32(0)
	for i := range info.frames {
		elapsed += simArtHoldVisits(info.frames[i].delay)
		if visit < elapsed {
			f = info.frames[i]
			break
		}
	}
	return f.w, f.h, f.xoff, f.yoff, info.visits, true
}

// FeatureSequenceDelays reports the per-frame delay words of one compiled
// feature event sequence, in frame order, exactly as authored [fmt gaf]. This
// is what a live event cursor is built from: the cursor holds a frame index
// and that frame's delay countdown, steps the frame when the countdown is
// below two, reloads the countdown from the new frame's word, and finishes
// when the frame index reaches the count [05 R-FEAT-01 §10] pass 1 — so the
// simulation needs the words themselves, not only the visit total.
//
// ok is false when the sequence does not resolve; the caller then has no
// sequence, which the ignition and transition contracts already define
// [05 R-FEAT-01 §5][05 R-FEAT-01 §9]. The slice is the caller's: a fresh copy
// each call, so the table stays immutable.
func (a *SimArt) FeatureSequenceDelays(filename, sequence string) ([]int32, bool) {
	if a == nil || len(a.sequences) == 0 {
		return nil, false
	}
	if trimTDFSemantic(filename) == "" || trimTDFSemantic(sequence) == "" {
		return nil, false
	}
	info := a.sequences[simArtSequenceKey(filename, sequence)]
	if info == nil || len(info.frames) == 0 {
		return nil, false
	}
	out := make([]int32, len(info.frames))
	for i := range info.frames {
		out[i] = info.frames[i].delay
	}
	return out, true
}

// simArtEffectKey folds a bank and entry name the way the bank cache and the
// GAF's own name index do: the bank name is trimmed and lower-cased because it
// becomes a path component, and the entry name is only ASCII-folded, because
// that is exactly what an entry lookup by name compares [fmt gaf].
func simArtEffectKey(bank, entry string) string {
	b := CanonicalKey(bank)
	if b == "" {
		b = DefaultEffectBank
	}
	return b + "|" + asciiFoldContent(entry)
}

func simArtSequenceKey(filename, sequence string) string {
	return CanonicalKey(filename) + "|" + CanonicalKey(sequence)
}

// simArtHoldVisits is the `max(delay, 1)` of [05 R-FEAT-01 §10]: a frame whose
// authored delay word is zero still occupies one visit, because the cursor
// advance steps the frame when the delay is below two and reloads it from the
// new frame.
func simArtHoldVisits(delay int32) int32 {
	if delay < 1 {
		return 1
	}
	return delay
}
