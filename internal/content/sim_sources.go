package content

// The closed source snapshot a battle's simulation content is read from
// (DESIGN_MULTIPLAYER §8.7, contract M2-C8). This is Nanolathe protocol
// plumbing, not retail behaviour: retail rereads its archives whenever a unit
// is created, and nothing here changes what any loader decides.
//
// A capture answers every lookup the way the live file system answered the
// first time it was asked, and from then on answers it the same way: the bytes
// a loader read, the error a missing file produced, the listing a directory
// scan returned. Catalog compilation, map and schema resolution and rule and
// mutator preparation read through it, so every value they derive was derived
// from one consistent set of files. Freezing the battle's inputs then seals
// it: a later lookup the capture never answered is a defined miss forever,
// never a fresh read of the live providers. A loose file edited after the
// capture therefore cannot reach the battle, while a second battle captures
// again and sees the edit.
//
// Design reading: capture is first-read rather than eager. Reading every file
// of every simulation family up front would read archive members no loader
// consults; first-read capture records exactly the lookups the loaders make,
// in their own discovery order, including their misses. Only the selected
// map's two files are read at capture time, so the map census's header range
// reads and the terrain loader's whole read come from one copy of the bytes.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// captureEagerLimit bounds the selected map's capture-time reads. It is a host
// allocation bound, not a format limit: a loader's own smaller cap still
// applies when it reads the captured bytes, and a larger file is left to be
// captured by whichever loader first asks for it.
const captureEagerLimit = 1 << 30

// SimulationSources is the closed source snapshot of one battle's simulation
// content. Its Filesystem is the read-only view that catalog compilation, map
// and schema resolution and rule and mutator preparation consume; the battle's
// SimulationInputs are frozen from it once, after which the view is sealed.
type SimulationSources struct {
	snap    *sourceSnapshot
	view    vfs.FSOps
	mapName string

	mu     sync.Mutex
	frozen bool
}

// CaptureSimulationSources begins the source snapshot of one battle over fs.
// mapName is the selected skirmish map's name as the lobby or command line
// names it; its OTA and TNT are captured at once. An empty name captures no map
// up front. The live file system must not be closed while the capture or the
// inputs frozen from it are in use.
func CaptureSimulationSources(fs vfs.FSOps, mapName string) (*SimulationSources, error) {
	if fs == nil {
		return nil, fmt.Errorf("nanolathe: simulation source capture failed: logical path <capture>, providers searched [none], expected a mounted content filesystem")
	}
	snap := newSourceSnapshot(fs)
	sources := &SimulationSources{snap: snap, mapName: strings.TrimSpace(mapName)}
	sources.view = snap.newView(nil, false)
	if base := captureMapBase(sources.mapName); base != "" {
		snap.captureWhole("maps/" + base + ".ota")
		snap.captureWhole("maps/" + base + ".tnt")
	}
	return sources, nil
}

// Filesystem returns the capture's read view. It never writes and never
// mounts; before the battle's inputs are frozen a first lookup of a path
// reads the live providers once, and afterwards it answers only from the
// capture.
func (s *SimulationSources) Filesystem() vfs.FSOps {
	if s == nil {
		return nil
	}
	return s.view
}

// captureMapBase forms the selected map's file stem the way the skirmish map
// loader forms its OTA path: the last path component with any extension
// removed (mission.LoadWithType, [08 R-CAMP-01 §11]).
func captureMapBase(name string) string {
	name = strings.ReplaceAll(strings.TrimSpace(name), "\\", "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	if dot := strings.LastIndex(name, "."); dot >= 0 {
		name = name[:dot]
	}
	return name
}

// snapshotKey folds a logical name the way the overlay does: either slash
// style, empty and "." components dropped, the established ASCII fold. A name
// the overlay refuses (absolute, or with a ".." component) keeps its literal
// spelling behind a NUL so its refusal is captured on its own.
func snapshotKey(name string) string {
	clean := strings.ReplaceAll(name, "\\", "/")
	if strings.HasPrefix(clean, "/") {
		return "\x00" + name
	}
	parts := strings.Split(clean, "/")
	kept := parts[:0]
	for _, part := range parts {
		switch part {
		case "", ".":
			continue
		case "..":
			return "\x00" + name
		}
		kept = append(kept, part)
	}
	return formats.FoldASCII(strings.Join(kept, "/"))
}

// capturedFile is everything the capture has answered about one logical path.
// Each answer is recorded on its first request and never replaced, except
// that a read refused as too large may be retried under a larger cap before
// the capture is sealed.
type capturedFile struct {
	statDone bool
	info     vfs.EntryInfo
	statErr  error

	dataDone  bool
	data      []byte
	dataErr   error
	dataLimit int64
	// released marks bytes consumed into a frozen compiled value (an
	// animation bank folded into SimArt) and not retained; a later read is a
	// defined miss.
	released bool

	stampDone bool
	stamp     string
	stampErr  error

	sourcesDone bool
	sources     []vfs.EntryInfo
}

type capturedDir struct {
	entries []vfs.EntryInfo
	err     error
}

type capturedRangeKey struct {
	key    string
	offset int64
	length int
}

type capturedRange struct {
	data []byte
	err  error
}

type retailDirLister interface {
	RetailReadDir(string) ([]vfs.EntryInfo, error)
}

type sourceLister interface {
	Sources(string) []vfs.EntryInfo
}

type manifestHasher interface {
	ManifestHash() (string, error)
}

// sourceSnapshot is the capture's storage. The mutex serializes lookups; a
// capture is read by one battle's loaders, but nothing prevents a host from
// reading a view from two goroutines, and the answer must still be one answer.
type sourceSnapshot struct {
	mu sync.Mutex

	live    vfs.FSOps
	ranged  vfs.RangeReader
	ordered retailDirLister
	lister  sourceLister
	hasher  manifestHasher

	manifestDone bool
	manifest     string

	files      map[string]*capturedFile
	dirs       map[string]*capturedDir
	retailDirs map[string]*capturedDir
	ranges     map[capturedRangeKey]*capturedRange

	sealed bool
	// uncaptured records lookups made after sealing that the capture could
	// not answer. They are answered as defined misses; the list is a
	// diagnostic for tests and hosts auditing that no later read escaped.
	uncaptured map[string]struct{}
}

func newSourceSnapshot(live vfs.FSOps) *sourceSnapshot {
	s := &sourceSnapshot{
		live:       live,
		files:      make(map[string]*capturedFile),
		dirs:       make(map[string]*capturedDir),
		retailDirs: make(map[string]*capturedDir),
		ranges:     make(map[capturedRangeKey]*capturedRange),
		uncaptured: make(map[string]struct{}),
	}
	s.ranged, _ = live.(vfs.RangeReader)
	s.ordered, _ = live.(retailDirLister)
	s.lister, _ = live.(sourceLister)
	// The live overlay's own identity is answered once. *vfs.FS satisfies
	// this interface, so a catalog compiled through a capture records the
	// same Catalog.Manifest it would have recorded from the live overlay.
	s.hasher, _ = live.(manifestHasher)
	return s
}

// file returns the record for name, creating it unless the capture is sealed.
func (s *sourceSnapshot) file(name string) *capturedFile {
	key := snapshotKey(name)
	f := s.files[key]
	if f == nil && !s.sealed {
		f = &capturedFile{}
		s.files[key] = f
	}
	return f
}

func (s *sourceSnapshot) noteUncaptured(op, name string) error {
	s.uncaptured[op+" "+snapshotKey(name)] = struct{}{}
	return fmt.Errorf("%w: %s (not captured for this battle)", vfs.ErrNotFound, name)
}

func (s *sourceSnapshot) seal() {
	s.mu.Lock()
	s.sealed = true
	s.mu.Unlock()
}

func (s *sourceSnapshot) uncapturedLookups() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.uncaptured))
	for key := range s.uncaptured {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// captureWhole records name's metadata and whole bytes under the eager bound.
// A refusal as too large is not recorded, so a loader asking with its own cap
// still receives the live answer for that cap.
func (s *sourceSnapshot) captureWhole(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.file(name)
	if f == nil || f.dataDone {
		return
	}
	data, err := s.live.ReadFileLimit(name, captureEagerLimit)
	if err != nil && errors.Is(err, vfs.ErrTooLarge) {
		return
	}
	s.captureStatLocked(f, name)
	f.dataDone, f.data, f.dataErr, f.dataLimit = true, data, err, captureEagerLimit
}

func (s *sourceSnapshot) captureStatLocked(f *capturedFile, name string) {
	if f.statDone {
		return
	}
	f.info, f.statErr = s.live.Stat(name)
	f.statDone = true
}

func (s *sourceSnapshot) stat(name string) (vfs.EntryInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.file(name)
	if f == nil || !f.statDone {
		if s.sealed {
			return vfs.EntryInfo{}, s.noteUncaptured("stat", name)
		}
		s.captureStatLocked(f, name)
	}
	return f.info, f.statErr
}

// readFileLimit answers a bounded whole-file read. Over captured bytes it
// applies the overlay's own two checks — the indexed size against the cap
// before reading, then the read length — so a loader sees the refusal it
// would have seen from the live overlay [vfs.FS.ReadFileLimit]. The caller
// owns the returned slice: loaders may rewrite their input in place.
//
// transient reads that are first served by this request are not retained:
// the bytes go to a compiler that folds them into a frozen value, and the
// capture records only that the file was consumed.
func (s *sourceSnapshot) readFileLimit(name string, max int64, transient bool) ([]byte, error) {
	if max <= 0 {
		return nil, fmt.Errorf("%w: invalid limit %d", vfs.ErrTooLarge, max)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.file(name)
	retry := f != nil && f.dataDone && f.dataErr != nil && errors.Is(f.dataErr, vfs.ErrTooLarge) && max > f.dataLimit
	if f == nil || !f.dataDone || retry || f.released {
		if s.sealed {
			return nil, s.noteUncaptured("read", name)
		}
		s.captureStatLocked(f, name)
		data, err := s.live.ReadFileLimit(name, max)
		f.dataDone, f.dataErr, f.dataLimit, f.released = true, err, max, false
		if err != nil {
			f.data = nil
			return nil, err
		}
		if transient {
			f.data, f.released = nil, true
			return data, nil
		}
		f.data = data
		return bytes.Clone(data), nil
	}
	if f.dataErr != nil {
		if errors.Is(f.dataErr, vfs.ErrTooLarge) && f.statErr == nil && f.info.Size > max {
			return nil, fmt.Errorf("%w: %s is %d bytes (limit %d)", vfs.ErrTooLarge, name, f.info.Size, max)
		}
		return nil, f.dataErr
	}
	if f.statErr == nil && !f.info.IsDir && f.info.Size >= 0 && f.info.Size > max {
		return nil, fmt.Errorf("%w: %s is %d bytes (limit %d)", vfs.ErrTooLarge, name, f.info.Size, max)
	}
	if int64(len(f.data)) > max {
		return nil, fmt.Errorf("%w: %s exceeds %d bytes", vfs.ErrTooLarge, name, max)
	}
	return bytes.Clone(f.data), nil
}

// readFileRange answers a byte-range read. Ranges of a file whose whole bytes
// are captured are sliced from them, so a header read and the later whole
// read agree; other ranges are captured per request.
func (s *sourceSnapshot) readFileRange(name string, offset int64, length int) ([]byte, error) {
	if offset < 0 {
		return nil, fmt.Errorf("%w: negative offset %d", vfs.ErrNotFound, offset)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if f := s.files[snapshotKey(name)]; f != nil && f.dataDone && f.dataErr == nil && !f.released {
		data := f.data
		if offset > int64(len(data)) {
			return nil, fmt.Errorf("%w: range offset %d outside %d-byte file", vfs.ErrNotFound, offset, len(data))
		}
		data = data[offset:]
		if length >= 0 && length < len(data) {
			data = data[:length]
		}
		return bytes.Clone(data), nil
	}
	key := capturedRangeKey{key: snapshotKey(name), offset: offset, length: length}
	r := s.ranges[key]
	if r == nil {
		if s.sealed || s.ranged == nil {
			return nil, s.noteUncaptured("range", name)
		}
		data, err := s.ranged.ReadFileRange(name, offset, length)
		r = &capturedRange{data: data, err: err}
		s.ranges[key] = r
	}
	if r.err != nil {
		return nil, r.err
	}
	return bytes.Clone(r.data), nil
}

func (s *sourceSnapshot) readDir(name string, retail bool) ([]vfs.EntryInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	table := s.dirs
	op := "readdir"
	if retail {
		table, op = s.retailDirs, "retailreaddir"
	}
	key := snapshotKey(name)
	d := table[key]
	if d == nil {
		if s.sealed {
			return nil, s.noteUncaptured(op, name)
		}
		var entries []vfs.EntryInfo
		var err error
		if retail {
			entries, err = s.ordered.RetailReadDir(name)
		} else {
			entries, err = s.live.ReadDir(name)
		}
		d = &capturedDir{entries: entries, err: err}
		table[key] = d
	}
	if d.err != nil {
		return nil, d.err
	}
	return slices.Clone(d.entries), nil
}

// cacheStamp is host cache metadata, never content identity. It is captured
// like every other answer so a frozen view reports one stamp per path.
func (s *sourceSnapshot) cacheStamp(name string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.file(name)
	if f == nil || !f.stampDone {
		if s.sealed {
			return "", fmt.Errorf("%w: %s (not captured for this battle)", vfs.ErrNotFound, name)
		}
		f.stamp, f.stampErr = s.live.CacheStamp(name)
		f.stampDone = true
	}
	return f.stamp, f.stampErr
}

// sources is a diagnostic: the providers holding a path. An uncaptured path
// after sealing answers nil, which is what a view without the report answers.
func (s *sourceSnapshot) sources(name string) []vfs.EntryInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.file(name)
	if f == nil || !f.sourcesDone {
		if s.sealed || s.lister == nil {
			return nil
		}
		f.sources = s.lister.Sources(name)
		f.sourcesDone = true
	}
	return slices.Clone(f.sources)
}

func (s *sourceSnapshot) manifestHash() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.manifestDone && !s.sealed && s.hasher != nil {
		// A failed overlay manifest reports an empty identity, which is what
		// the catalog records from it in that case.
		s.manifest, _ = s.hasher.ManifestHash()
		s.manifestDone = true
	}
	return s.manifest, nil
}

// newView returns a read view over the capture that offers exactly the
// optional capabilities the live file system offers. A loader that probes for
// byte ranges or retail directory order then takes the same path through the
// capture as through the live view: download membership order depends on the
// retail enumerator [02 R-CAT-01 §8], and the map census costs seconds rather
// than milliseconds without ranges. inputs is set on the sealed view of a
// frozen battle; transient views do not retain the bytes they first read.
func (s *sourceSnapshot) newView(inputs *SimulationInputs, transient bool) vfs.FSOps {
	base := &snapshotView{snap: s, inputs: inputs, transient: transient}
	switch {
	case s.ranged != nil && s.ordered != nil:
		return &snapshotRangeOrderedView{snapshotView: base}
	case s.ordered != nil:
		return &snapshotOrderedView{snapshotView: base}
	case s.ranged != nil:
		return &snapshotRangeView{snapshotView: base}
	}
	return base
}

type snapshotView struct {
	snap      *sourceSnapshot
	inputs    *SimulationInputs
	transient bool
}

var _ vfs.FSOps = (*snapshotView)(nil)

func (v *snapshotView) Open(name string) (vfs.File, error) {
	data, err := v.snap.readFileLimit(name, int64(^uint64(0)>>1), v.transient)
	if err != nil {
		return nil, err
	}
	info, statErr := v.snap.stat(name)
	if statErr != nil {
		info = vfs.EntryInfo{Path: snapshotKey(name), Name: name, Size: int64(len(data))}
	}
	return &capturedHandle{Reader: bytes.NewReader(data), data: data, info: info}, nil
}

func (v *snapshotView) ReadFileLimit(name string, max int64) ([]byte, error) {
	return v.snap.readFileLimit(name, max, v.transient)
}

func (v *snapshotView) ReadDir(name string) ([]vfs.EntryInfo, error) {
	return v.snap.readDir(name, false)
}

func (v *snapshotView) Stat(name string) (vfs.EntryInfo, error) { return v.snap.stat(name) }

func (v *snapshotView) CacheStamp(name string) (string, error) { return v.snap.cacheStamp(name) }

// Sources reports the providers that held a path when the capture first
// asked, best precedence first.
func (v *snapshotView) Sources(name string) []vfs.EntryInfo { return v.snap.sources(name) }

// ManifestHash reports the live overlay's mount identity as captured. It is
// diagnostic provenance (Catalog.Manifest), never content identity.
func (v *snapshotView) ManifestHash() (string, error) { return v.snap.manifestHash() }

// SimulationInputs returns the frozen inputs this sealed view belongs to, or
// nil for a capture view. Composition recovers a battle's frozen inputs from
// the view its unit world reads, so later unit creation can be checked to
// consume the admitted value.
func (v *snapshotView) SimulationInputs() *SimulationInputs { return v.inputs }

type snapshotRangeView struct{ *snapshotView }

func (v *snapshotRangeView) ReadFileRange(name string, offset int64, length int) ([]byte, error) {
	return v.snap.readFileRange(name, offset, length)
}

type snapshotOrderedView struct{ *snapshotView }

func (v *snapshotOrderedView) RetailReadDir(name string) ([]vfs.EntryInfo, error) {
	return v.snap.readDir(name, true)
}

type snapshotRangeOrderedView struct{ *snapshotView }

func (v *snapshotRangeOrderedView) ReadFileRange(name string, offset int64, length int) ([]byte, error) {
	return v.snap.readFileRange(name, offset, length)
}

func (v *snapshotRangeOrderedView) RetailReadDir(name string) ([]vfs.EntryInfo, error) {
	return v.snap.readDir(name, true)
}

var (
	_ vfs.RangeReader = (*snapshotRangeView)(nil)
	_ retailDirLister = (*snapshotOrderedView)(nil)
	_ vfs.RangeReader = (*snapshotRangeOrderedView)(nil)
	_ retailDirLister = (*snapshotRangeOrderedView)(nil)
)

// capturedHandle is an open captured file. The bytes are the caller's own
// copy, so handing them over whole is safe [vfs.WholeFile].
type capturedHandle struct {
	*bytes.Reader
	data []byte
	info vfs.EntryInfo
}

func (h *capturedHandle) Close() error          { return nil }
func (h *capturedHandle) Info() vfs.EntryInfo   { return h.info }
func (h *capturedHandle) Whole() ([]byte, bool) { return h.data, true }

var (
	_ vfs.File      = (*capturedHandle)(nil)
	_ vfs.WholeFile = (*capturedHandle)(nil)
	_ io.ReaderAt   = (*capturedHandle)(nil)
)
