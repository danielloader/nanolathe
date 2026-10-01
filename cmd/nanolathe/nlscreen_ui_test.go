package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nanolathe-gg/nanolathe/internal/modlibrary"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// These lock the settings screen's frame caches and picture loading, host
// presentation policy rather than retail behaviour
// (DESIGN_INTERFACE_HUD_INPUT §3.17).

// The catalogue is built once; it is rebuilt exactly when the installed mods'
// names or versions — the one input its builders capture — change. Everything
// else the cards show is read when their closures run.
func TestNLScreenCatalogueRebuildsOnlyForModNames(t *testing.T) {
	g := &gameShell{cs: &contentSet{}}
	s := newNLScreen(func() *gameShell { return g })
	s.draft = s.snapshot(g)
	first := s.pages()
	for range 3 {
		// What a frame does: the change count and the cards' changed lamps.
		s.dirty()
		if &s.pages()[0] != &first[0] {
			t.Fatal("an ordinary frame rebuilt the catalogue")
		}
	}
	contentCard := func() *nlCard { return &s.pages()[0].cards[0] }
	alpha := modlibrary.Mod{Metadata: modlibrary.Metadata{ID: "alpha", Name: "Alpha", Version: "1", Summary: "First summary."}}
	s.mods = []modlibrary.Mod{alpha}
	installed := s.pages()
	if &installed[0] == &first[0] || strings.Join(contentCard().steps, "|") != "Total Annihilation|Alpha 1" {
		t.Fatalf("an installed mod did not rebuild the content card: %q", contentCard().steps)
	}
	// A re-read with the same names keeps the catalogue; the description
	// still reads the current list.
	alpha.Summary = "Second summary."
	s.mods = []modlibrary.Mod{alpha}
	if &s.pages()[0] != &installed[0] {
		t.Fatal("re-reading the same mods rebuilt the catalogue")
	}
	if desc := contentCard().desc(&s.draft, 1); !strings.HasPrefix(desc, "Second summary.") {
		t.Fatalf("content description read a stale mod: %q", desc)
	}
	alpha.Version = "2"
	s.mods = []modlibrary.Mod{alpha}
	if strings.Join(contentCard().steps, "|") != "Total Annihilation|Alpha 2" {
		t.Fatalf("a new version did not rebuild the content card: %q", contentCard().steps)
	}
	s.mods = nil
	if len(contentCard().steps) != 1 {
		t.Fatalf("removing the mods left %q", contentCard().steps)
	}
}

// nlPicFS serves authored unit pictures. hold blocks every read while the
// test holds it; after close, a read is a test failure.
type nlPicFS struct {
	t      *testing.T
	files  map[string][]byte
	hold   sync.RWMutex
	mu     sync.Mutex
	reads  int
	closed bool
}

func (f *nlPicFS) ReadFileLimit(name string, _ int64) ([]byte, error) {
	f.mu.Lock()
	f.reads++
	if f.closed {
		f.t.Errorf("read %s after the content closed", name)
	}
	f.mu.Unlock()
	f.hold.RLock()
	defer f.hold.RUnlock()
	if data, ok := f.files[name]; ok {
		return data, nil
	}
	return nil, vfs.ErrNotFound
}

func (f *nlPicFS) readCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reads
}

func (f *nlPicFS) Open(string) (vfs.File, error)           { return nil, vfs.ErrNotFound }
func (f *nlPicFS) ReadDir(string) ([]vfs.EntryInfo, error) { return nil, vfs.ErrNotFound }
func (f *nlPicFS) Stat(string) (vfs.EntryInfo, error)      { return vfs.EntryInfo{}, vfs.ErrNotFound }
func (f *nlPicFS) CacheStamp(string) (string, error)       { return "", vfs.ErrNotFound }

// nlTestPCX authors a w×h picture of one palette index.
func nlTestPCX(w, h int) []byte {
	header := make([]byte, 128)
	header[0], header[1] = 0x0a, 5
	binary.LittleEndian.PutUint16(header[8:], uint16(w-1))
	binary.LittleEndian.PutUint16(header[10:], uint16(h-1))
	binary.LittleEndian.PutUint16(header[66:], uint16(w))
	var out bytes.Buffer
	out.Write(header)
	for i := 0; i < w*h; i++ {
		out.WriteByte(7)
	}
	out.Write(make([]byte, 768))
	return out.Bytes()
}

func nlWaitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

// The game goroutine never reads a picture: while every read is held, pic
// returns at once with nothing, and once the worker has decoded the names the
// first that exists wins.
func TestNLPicturesDecodeOffTheGameGoroutine(t *testing.T) {
	fs := &nlPicFS{t: t, files: map[string][]byte{"unitpics/secondpower.pcx": nlTestPCX(5, 3)}}
	cards := []nlPicRequest{{key: "power", pics: []string{"FirstPower", "SecondPower"}}}
	a := &nlArt{}
	cs := &contentSet{}
	a.bindContent(cs)
	a.loader = newNLPictures(fs, nlRenamedCatalog(), cards)
	fs.hold.Lock()
	a.loader.resume()
	nlWaitFor(t, "names", func() bool { return a.pictureNames() != nil })
	names := a.pictureNames().cards["power"]
	if strings.Join(names, ",") != "firstpower,secondpower" {
		t.Fatalf("resolved %v", names)
	}
	nlWaitFor(t, "the worker's first read", func() bool { return fs.readCount() > 0 })
	returned := make(chan bool)
	go func() { returned <- a.pic(names...) == nil }()
	select {
	case none := <-returned:
		if !none {
			t.Fatal("a picture appeared while every read was held")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("pic waited on a file read")
	}
	fs.hold.Unlock()
	nlWaitFor(t, "the decoded picture", func() bool { return a.pic(names...) != nil })
	if img, ok := a.pics["firstpower"]; !ok || img != nil {
		t.Fatal("the missing first name was not recorded as missing")
	}
	if b := a.pic(names...).Bounds(); b.Dx() != 5 || b.Dy() != 3 {
		t.Fatalf("decoded picture is %v", b)
	}
	a.loader.stop()
}

// A content switch stops the old content's loader and waits for the file in
// hand, so nothing reads the old archives once the switch returns.
func TestNLPicturesContentSwitchStopsTheOldLoader(t *testing.T) {
	files := map[string][]byte{}
	var cards []nlPicRequest
	for i := range 20 {
		cards = append(cards, nlPicRequest{key: fmt.Sprint(i), pics: []string{fmt.Sprintf("pic%02d", i)}})
		files[fmt.Sprintf("unitpics/pic%02d.pcx", i)] = nlTestPCX(4, 4)
	}
	cat := nlRenamedCatalog()
	for i := range 20 {
		cat.Units[fmt.Sprintf("pic%02d", i)] = cat.Units["firstpower"]
	}
	old := &nlPicFS{t: t, files: files}
	a := &nlArt{}
	a.bindContent(&contentSet{})
	a.loader = newNLPictures(old, cat, cards)
	old.hold.Lock()
	a.loader.resume()
	nlWaitFor(t, "a read in flight", func() bool { return old.readCount() > 0 })
	next := &contentSet{}
	switched := make(chan struct{})
	go func() {
		a.bindContent(next)
		close(switched)
	}()
	select {
	case <-switched:
		t.Fatal("the switch returned while the old content was being read")
	case <-time.After(50 * time.Millisecond):
	}
	old.hold.Unlock()
	<-switched
	old.mu.Lock()
	old.closed = true
	old.mu.Unlock()
	if a.cs != next || a.loader != nil || len(a.pics) != 0 {
		t.Fatal("the switch kept the old content's pictures")
	}
	if n := old.readCount(); n != 1 {
		t.Fatalf("the old loader read %d pictures; it should stop after the one in hand", n)
	}
	time.Sleep(20 * time.Millisecond) // a straggling read would fail the test
}
