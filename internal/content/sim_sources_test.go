package content

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/nanolathe-gg/nanolathe/vfs"
)

// writeFrozenFixtureFile writes one authored fixture file under root.
func writeFrozenFixtureFile(t *testing.T, root, logical string, data []byte) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(logical))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func mountFrozenFixture(t *testing.T, root string) *vfs.FS {
	t.Helper()
	fs := vfs.New()
	if err := fs.MountDirectory(root, 10); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fs.Close() })
	return fs
}

// mutableFS is a live file system whose contents a test changes underneath a
// capture: a path that appears after the capture missed it must stay missing.
type mutableFS struct {
	files map[string][]byte
	reads int
}

func (m *mutableFS) Open(name string) (vfs.File, error) {
	data, err := m.ReadFileLimit(name, 1<<30)
	if err != nil {
		return nil, err
	}
	return &capturedHandle{Reader: bytes.NewReader(data), data: data}, nil
}

func (m *mutableFS) ReadFileLimit(name string, max int64) ([]byte, error) {
	m.reads++
	data, ok := m.files[snapshotKey(name)]
	if !ok {
		return nil, vfs.ErrNotFound
	}
	if int64(len(data)) > max {
		return nil, vfs.ErrTooLarge
	}
	return bytes.Clone(data), nil
}

func (m *mutableFS) ReadDir(string) ([]vfs.EntryInfo, error) { return nil, nil }

func (m *mutableFS) Stat(name string) (vfs.EntryInfo, error) {
	data, ok := m.files[snapshotKey(name)]
	if !ok {
		return vfs.EntryInfo{}, vfs.ErrNotFound
	}
	return vfs.EntryInfo{Path: snapshotKey(name), Size: int64(len(data))}, nil
}

func (m *mutableFS) CacheStamp(name string) (string, error) { return name, nil }

// A capture answers a loose file the way it first read it, however the file
// changes afterwards; a new capture reads the edit (DESIGN_MULTIPLAYER §8.7).
func TestCaptureAnswersALooseFileOnce(t *testing.T) {
	root := t.TempDir()
	writeFrozenFixtureFile(t, root, "units/a.fbi", []byte("first"))
	fs := mountFrozenFixture(t, root)
	sources, err := CaptureSimulationSources(fs, "")
	if err != nil {
		t.Fatal(err)
	}
	view := sources.Filesystem()
	if got, err := view.ReadFileLimit("UNITS\\A.FBI", 64); err != nil || string(got) != "first" {
		t.Fatalf("first read = %q, %v", got, err)
	}
	writeFrozenFixtureFile(t, root, "units/a.fbi", []byte("edited"))
	got, err := view.ReadFileLimit("units/a.fbi", 64)
	if err != nil || string(got) != "first" {
		t.Fatalf("captured read after the edit = %q, %v; want the first answer", got, err)
	}
	got[0] = 'X' // the caller owns its copy
	if again, _ := view.ReadFileLimit("units/a.fbi", 64); string(again) != "first" {
		t.Fatalf("a caller's write reached the capture: %q", again)
	}
	if _, err := view.ReadFileLimit("units/a.fbi", 2); !errors.Is(err, vfs.ErrTooLarge) {
		t.Fatalf("a smaller loader cap over captured bytes = %v, want ErrTooLarge", err)
	}
	next, err := CaptureSimulationSources(fs, "")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := next.Filesystem().ReadFileLimit("units/a.fbi", 64); err != nil || string(got) != "edited" {
		t.Fatalf("a new capture read %q, %v; want the edit", got, err)
	}
}

// A miss is captured like any other answer: a file that appears afterwards
// stays missing for this capture, and a sealed capture answers a path it never
// read as a defined miss, never a live read.
func TestCaptureRecordsAMissForever(t *testing.T) {
	live := &mutableFS{files: map[string][]byte{}}
	sources, err := CaptureSimulationSources(live, "")
	if err != nil {
		t.Fatal(err)
	}
	view := sources.Filesystem()
	if _, err := view.ReadFileLimit("ai/x.txt", 64); !errors.Is(err, vfs.ErrNotFound) {
		t.Fatalf("missing read = %v", err)
	}
	live.files["ai/x.txt"] = []byte("late")
	if _, err := view.ReadFileLimit("ai/x.txt", 64); !errors.Is(err, vfs.ErrNotFound) {
		t.Fatalf("a captured miss turned into %v", err)
	}
	// A read captures the path's metadata with it, so the two cannot disagree.
	if _, err := view.Stat("ai/x.txt"); !errors.Is(err, vfs.ErrNotFound) {
		t.Fatalf("stat after a captured miss = %v", err)
	}
	sources.snap.seal()
	reads := live.reads
	live.files["units/new.fbi"] = []byte("new")
	if _, err := view.ReadFileLimit("units/new.fbi", 64); !errors.Is(err, vfs.ErrNotFound) {
		t.Fatalf("sealed capture read an uncaptured path: %v", err)
	}
	if live.reads != reads {
		t.Fatal("sealed capture fell through to the live file system")
	}
	if got := sources.snap.uncapturedLookups(); len(got) != 1 || got[0] != "read units/new.fbi" {
		t.Fatalf("uncaptured lookups = %v", got)
	}
}

// The capture offers exactly the live view's optional capabilities, so a
// loader that probes for them takes the same path through either
// (download order, map census ranges).
func TestCaptureOffersTheLiveCapabilities(t *testing.T) {
	fs := mountFrozenFixture(t, t.TempDir())
	sources, _ := CaptureSimulationSources(fs, "")
	if _, ok := sources.Filesystem().(vfs.RangeReader); !ok {
		t.Fatal("capture of an overlay lost byte ranges")
	}
	if _, ok := sources.Filesystem().(retailDirLister); !ok {
		t.Fatal("capture of an overlay lost retail directory order")
	}
	plain, _ := CaptureSimulationSources(&mutableFS{files: map[string][]byte{}}, "")
	if _, ok := plain.Filesystem().(vfs.RangeReader); ok {
		t.Fatal("capture invented byte ranges the live view lacks")
	}
	if _, ok := plain.Filesystem().(retailDirLister); ok {
		t.Fatal("capture invented retail directory order the live view lacks")
	}
}

// The selected map is captured whole at capture time, so a header range read
// and the terrain loader's whole read come from one copy of the bytes.
func TestCaptureSlicesRangesOfTheSelectedMap(t *testing.T) {
	root := t.TempDir()
	writeFrozenFixtureFile(t, root, "maps/Hills.tnt", []byte("0123456789"))
	fs := mountFrozenFixture(t, root)
	sources, err := CaptureSimulationSources(fs, "Hills")
	if err != nil {
		t.Fatal(err)
	}
	writeFrozenFixtureFile(t, root, "maps/Hills.tnt", []byte("abcdefghij"))
	ranged := sources.Filesystem().(vfs.RangeReader)
	if got, err := ranged.ReadFileRange("maps/hills.tnt", 2, 3); err != nil || string(got) != "234" {
		t.Fatalf("range of the captured map = %q, %v", got, err)
	}
	if got, err := sources.Filesystem().ReadFileLimit("maps/hills.tnt", 64); err != nil || string(got) != "0123456789" {
		t.Fatalf("whole read of the captured map = %q, %v", got, err)
	}
}
