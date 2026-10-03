package version

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
	"testing"
)

func validManifest() BuildManifest {
	return BuildManifest{
		SourceTree: sha256.Sum256([]byte("source")),
		GoVersion:  "go1.27.1",
		GoMod:      sha256.Sum256([]byte("go.mod")),
		GoSum:      sha256.Sum256([]byte("go.sum")),
		Modules: []BuildModule{
			{Path: "github.com/ebitengine/purego", Version: "v0.11.1", Sum: "h1:purego="},
			{Path: "golang.org/x/sys", Version: "v0.47.0", Sum: "h1:sys47="},
			{Path: "golang.org/x/sys", Version: "v0.48.0", Sum: "h1:sys48="},
		},
		BuildTags:    []string{"netgo", "retail"},
		GoExperiment: "",
		CGOEnabled:   false,
		BuildArgs:    []string{"-mod=readonly", "-trimpath", "-buildvcs=false", "-ldflags=-s -w", "./cmd/nanolathe"},
		Variants: []BuildVariant{
			{GOOS: "darwin", GOARCH: "amd64", ArchitectureLevel: "v1", ToolchainArchive: sha256.Sum256([]byte("da"))},
			{GOOS: "darwin", GOARCH: "arm64", ArchitectureLevel: "v8.0", ToolchainArchive: sha256.Sum256([]byte("dx"))},
			{GOOS: "linux", GOARCH: "amd64", ArchitectureLevel: "v1", ToolchainArchive: sha256.Sum256([]byte("la"))},
			{GOOS: "windows", GOARCH: "arm64", ArchitectureLevel: "v8.0", ToolchainArchive: sha256.Sum256([]byte("wa"))},
		},
	}
}

func cloneManifest(m BuildManifest) BuildManifest {
	m.Modules = slices.Clone(m.Modules)
	m.BuildTags = slices.Clone(m.BuildTags)
	m.BuildArgs = slices.Clone(m.BuildArgs)
	m.Variants = slices.Clone(m.Variants)
	return m
}

// M2-C10: the manifest round-trips through its encoding, and its identity is
// SHA-256 of the literal domain, no NUL, then the encoding.
func TestBuildManifestRoundTrip(t *testing.T) {
	m := validManifest()
	payload, err := EncodeBuildManifest(m)
	if err != nil {
		t.Fatal(err)
	}
	back, err := DecodeBuildManifest(payload)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(back.Modules, m.Modules) || !slices.Equal(back.Variants, m.Variants) || !slices.Equal(back.BuildArgs, m.BuildArgs) ||
		!slices.Equal(back.BuildTags, m.BuildTags) || back.SourceTree != m.SourceTree || back.GoVersion != m.GoVersion ||
		back.GoMod != m.GoMod || back.GoSum != m.GoSum || back.GoExperiment != m.GoExperiment || back.CGOEnabled != m.CGOEnabled {
		t.Fatalf("round trip differs:\n got %+v\nwant %+v", back, m)
	}
	d, err := m.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if want := sha256.Sum256(append([]byte("nanolathe/sim-build/1"), payload...)); d != want {
		t.Fatal("the digest is not SHA-256 of the domain and the encoding")
	}
}

// Each field of §8.7 changes the identity on its own; a platform variant
// added inside one common manifest does too, because the variants are what
// the release admits.
func TestBuildManifestEveryFieldChangesIdentity(t *testing.T) {
	base, err := validManifest().Digest()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[[32]byte]string{base: "base"}
	for _, c := range []struct {
		name   string
		change func(*BuildManifest)
	}{
		{"source tree", func(m *BuildManifest) { m.SourceTree[0] ^= 1 }},
		{"go version", func(m *BuildManifest) { m.GoVersion = "go1.27.2" }},
		{"go.mod", func(m *BuildManifest) { m.GoMod[5] ^= 1 }},
		{"go.sum", func(m *BuildManifest) { m.GoSum[5] ^= 1 }},
		{"module sum", func(m *BuildManifest) { m.Modules[0].Sum = "h1:other=" }},
		{"module version", func(m *BuildManifest) { m.Modules[2].Version = "v0.49.0" }},
		{"module removed", func(m *BuildManifest) { m.Modules = m.Modules[1:] }},
		{"tag", func(m *BuildManifest) { m.BuildTags = []string{"netgo"} }},
		{"experiment", func(m *BuildManifest) { m.GoExperiment = "nocoverageredesign" }},
		{"cgo", func(m *BuildManifest) { m.CGOEnabled = true }},
		{"ldflags", func(m *BuildManifest) { m.BuildArgs[3] = "-ldflags=-s" }},
		{"argument order", func(m *BuildManifest) { m.BuildArgs[0], m.BuildArgs[1] = m.BuildArgs[1], m.BuildArgs[0] }},
		{"variant level", func(m *BuildManifest) { m.Variants[2].ArchitectureLevel = "v3" }},
		{"variant toolchain", func(m *BuildManifest) { m.Variants[0].ToolchainArchive[0] ^= 1 }},
		{"variant added", func(m *BuildManifest) {
			m.Variants = append(m.Variants, BuildVariant{GOOS: "windows", GOARCH: "amd64", ArchitectureLevel: "v1", ToolchainArchive: [32]byte{9}})
			slices.SortFunc(m.Variants, compareVariants)
		}},
	} {
		m := cloneManifest(validManifest())
		c.change(&m)
		d, err := m.Digest()
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if other, dup := seen[d]; dup {
			t.Errorf("%s: same identity as %s", c.name, other)
		}
		seen[d] = c.name
	}
}

// Every bound and canonical-form rule refuses what it does not admit.
func TestBuildManifestRefusesInvalidFields(t *testing.T) {
	for _, c := range []struct {
		name   string
		change func(*BuildManifest)
	}{
		{"unstamped", func(m *BuildManifest) { m.SourceTree = [32]byte{} }},
		{"no go version", func(m *BuildManifest) { m.GoVersion = "" }},
		{"long go version", func(m *BuildManifest) { m.GoVersion = strings.Repeat("g", 65) }},
		{"unsorted modules", func(m *BuildManifest) { m.Modules[0], m.Modules[1] = m.Modules[1], m.Modules[0] }},
		{"unsorted versions", func(m *BuildManifest) { m.Modules[1], m.Modules[2] = m.Modules[2], m.Modules[1] }},
		{"repeated module", func(m *BuildManifest) { m.Modules[2] = m.Modules[1] }},
		{"module without sum", func(m *BuildManifest) { m.Modules[0].Sum = "" }},
		{"invalid module path", func(m *BuildManifest) { m.Modules[0].Path = "\xff" }},
		{"too many modules", func(m *BuildManifest) { m.Modules = make([]BuildModule, 4097) }},
		{"unsorted tags", func(m *BuildManifest) { m.BuildTags = []string{"retail", "netgo"} }},
		{"repeated tag", func(m *BuildManifest) { m.BuildTags = []string{"netgo", "netgo"} }},
		{"capital tag", func(m *BuildManifest) { m.BuildTags = []string{"Retail"} }},
		{"spaced tag", func(m *BuildManifest) { m.BuildTags = []string{" retail"} }},
		{"empty tag", func(m *BuildManifest) { m.BuildTags = []string{""} }},
		{"too many tags", func(m *BuildManifest) {
			m.BuildTags = nil
			for i := 0; i < 65; i++ {
				m.BuildTags = append(m.BuildTags, "t"+strings.Repeat("a", i))
			}
		}},
		{"NUL experiment", func(m *BuildManifest) { m.GoExperiment = "a\x00" }},
		{"empty argument", func(m *BuildManifest) { m.BuildArgs = append(m.BuildArgs, "") }},
		{"long argument", func(m *BuildManifest) { m.BuildArgs[3] = strings.Repeat("x", 1025) }},
		{"too many arguments", func(m *BuildManifest) { m.BuildArgs = make([]string, 65) }},
		{"no variant", func(m *BuildManifest) { m.Variants = nil }},
		{"seventeen variants", func(m *BuildManifest) {
			m.Variants = nil
			for i := 0; i < 17; i++ {
				m.Variants = append(m.Variants, BuildVariant{GOOS: "os" + strings.Repeat("x", i), GOARCH: "amd64", ArchitectureLevel: "v1", ToolchainArchive: [32]byte{1}})
			}
		}},
		{"unsorted variants", func(m *BuildManifest) { m.Variants[0], m.Variants[1] = m.Variants[1], m.Variants[0] }},
		{"repeated variant", func(m *BuildManifest) { m.Variants[1] = m.Variants[0] }},
		{"variant without toolchain", func(m *BuildManifest) { m.Variants[0].ToolchainArchive = [32]byte{} }},
		{"capital GOOS", func(m *BuildManifest) { m.Variants[0].GOOS = "Darwin" }},
		{"empty level", func(m *BuildManifest) { m.Variants[0].ArchitectureLevel = "" }},
	} {
		m := cloneManifest(validManifest())
		c.change(&m)
		if _, err := EncodeBuildManifest(m); err == nil {
			t.Errorf("%s: encoded", c.name)
		}
		if _, err := m.Digest(); err == nil {
			t.Errorf("%s: digested", c.name)
		}
	}
	m := validManifest()
	m.SourceTree = [32]byte{}
	if _, err := m.Digest(); !errors.Is(err, ErrUnstampedBuild) {
		t.Fatalf("an unstamped manifest digested with %v", err)
	}
}

// The decoder refuses every truncation and trailing byte, and any byte flip
// is either refused or a different manifest that re-encodes exactly.
func TestDecodeBuildManifestRefusesHostilePayloads(t *testing.T) {
	m := validManifest()
	payload, _ := EncodeBuildManifest(m)
	base, _ := m.Digest()
	check := func(p []byte) bool {
		t.Helper()
		got, err := DecodeBuildManifest(p)
		if err != nil {
			if got.Stamped() || got.Modules != nil {
				t.Fatal("a refused payload returned a value")
			}
			return false
		}
		again, err := EncodeBuildManifest(got)
		if err != nil || !bytes.Equal(again, p) {
			t.Fatalf("an accepted payload re-encodes differently: %v", err)
		}
		return true
	}
	for n := 0; n < len(payload); n++ {
		if check(payload[:n]) {
			t.Fatalf("a %d-byte prefix decoded", n)
		}
	}
	if check(append(append([]byte(nil), payload...), 0)) {
		t.Fatal("a trailing byte decoded")
	}
	accepted := 0
	for i := range payload {
		for _, mask := range []byte{0x01, 0x80, 0xff} {
			mutated := append([]byte(nil), payload...)
			mutated[i] ^= mask
			if check(mutated) {
				accepted++
				if d, _ := mustDecode(t, mutated).Digest(); d == base {
					t.Fatalf("byte %d ^ %#x decoded to the original identity", i, mask)
				}
			}
		}
	}
	if accepted == 0 {
		t.Fatal("no mutation was accepted: the sweep cannot tell a strict decoder from a broken one")
	}
	// The module count is at offset 32 + 1 + 8 ("go1.27.1") + 64.
	at := 32 + 1 + len(m.GoVersion) + 64
	if payload[at] != 3 {
		t.Fatalf("unexpected module count byte %d", payload[at])
	}
	for name, p := range map[string][]byte{
		"4097 modules":          append(append(append([]byte(nil), payload[:at]...), 0x81, 0x20), payload[at+1:]...),
		"overlong module count": append(append(append([]byte(nil), payload[:at]...), 0x83, 0x00), payload[at+1:]...),
		"modules over the rest": append(append(append([]byte(nil), payload[:at]...), 0xff, 0x1f), payload[at+1:]...),
		"over 1 MiB":            make([]byte, 1<<20+1),
	} {
		if check(p) {
			t.Errorf("%s: decoded", name)
		}
	}
}

func mustDecode(t *testing.T, p []byte) BuildManifest {
	t.Helper()
	m, err := DecodeBuildManifest(p)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// withStamp runs f with the package's stamp set to payload.
func withStamp(t *testing.T, payload string, f func()) {
	t.Helper()
	saved := stamp
	stamp = payload
	defer func() { stamp = saved }()
	f()
}

// runningManifest is a stamp that describes this test binary as its own
// build info records it.
func runningManifest(t *testing.T) BuildManifest {
	t.Helper()
	info, ok := debug.ReadBuildInfo()
	if !ok {
		t.Skip("this test binary records no build info")
	}
	m := validManifest()
	m.GoVersion = runtime.Version()
	m.CGOEnabled = buildSetting(info, "CGO_ENABLED") == "1"
	m.GoExperiment = buildSetting(info, "GOEXPERIMENT")
	m.BuildTags = nil
	if tags := buildSetting(info, "-tags"); tags != "" {
		m.BuildTags = strings.Split(tags, ",")
		slices.Sort(m.BuildTags)
	}
	m.BuildArgs = []string{"-mod=readonly"}
	if buildSetting(info, "-trimpath") == "true" {
		m.BuildArgs = append(m.BuildArgs, "-trimpath")
	}
	if ldflags := buildSetting(info, "-ldflags"); ldflags != "" {
		m.BuildArgs = append(m.BuildArgs, "-ldflags="+ldflags)
	}
	m.Modules = nil
	for _, dep := range info.Deps {
		m.Modules = append(m.Modules, BuildModule{Path: dep.Path, Version: dep.Version, Sum: dep.Sum})
	}
	slices.SortFunc(m.Modules, func(a, b BuildModule) int { return strings.Compare(a.Path+" "+a.Version, b.Path+" "+b.Version) })
	level := buildSetting(info, architectureLevelSetting(runtime.GOARCH))
	if level == "" {
		level = "none"
	}
	m.Variants = []BuildVariant{{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, ArchitectureLevel: level, ToolchainArchive: [32]byte{1}}}
	return m
}

// A development build carries no stamp: CurrentBuildManifest still answers,
// with an unstamped manifest naming the running toolchain, which cannot be
// encoded or digested and so fails normal-room admission.
func TestCurrentBuildManifestOfAnUnstampedBuild(t *testing.T) {
	withStamp(t, "", func() {
		m, err := CurrentBuildManifest()
		if err != nil {
			t.Fatal(err)
		}
		if m.Stamped() || m.GoVersion != runtime.Version() || len(m.Variants) != 1 || m.Variants[0].GOOS != runtime.GOOS || m.Variants[0].GOARCH != runtime.GOARCH {
			t.Fatalf("unstamped manifest %+v", m)
		}
		if _, err := m.Digest(); !errors.Is(err, ErrUnstampedBuild) {
			t.Fatalf("an unstamped manifest digested with %v", err)
		}
		if _, err := EncodeBuildManifest(m); !errors.Is(err, ErrUnstampedBuild) {
			t.Fatalf("an unstamped manifest encoded with %v", err)
		}
	})
}

// A stamped build returns its stamp when the stamp describes the running
// binary, and refuses a stamp copied from another build.
func TestCurrentBuildManifestChecksTheStampAgainstTheBinary(t *testing.T) {
	m := runningManifest(t)
	payload, err := EncodeBuildManifest(m)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := m.Digest()
	withStamp(t, string(payload), func() {
		got, err := CurrentBuildManifest()
		if err != nil {
			t.Fatal(err)
		}
		if d, err := got.Digest(); err != nil || d != want {
			t.Fatalf("the current manifest is not the stamp: %v", err)
		}
	})
	for _, c := range []struct {
		name   string
		change func(*BuildManifest)
	}{
		{"another toolchain", func(m *BuildManifest) { m.GoVersion = "go0.0.1" }},
		{"another platform", func(m *BuildManifest) { m.Variants[0].GOOS = "plan9x" }},
		{"another cgo", func(m *BuildManifest) { m.CGOEnabled = !m.CGOEnabled }},
	} {
		changed := cloneManifest(m)
		c.change(&changed)
		p, err := EncodeBuildManifest(changed)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		withStamp(t, string(p), func() {
			got, err := CurrentBuildManifest()
			if err == nil {
				t.Errorf("%s: accepted", c.name)
			}
			if got.Stamped() {
				t.Errorf("%s: a refused stamp was returned", c.name)
			}
		})
	}
	withStamp(t, "not a manifest", func() {
		if _, err := CurrentBuildManifest(); err == nil {
			t.Error("an unreadable stamp was accepted")
		}
	})
}

// The stamp check compares every recorded build fact with the stamp: the
// toolchain, the platform and its level, cgo, tags, GOEXPERIMENT, trimpath,
// ldflags where the toolchain records them, and each linked module, which
// may not be a replacement.
func TestStampDescribesItsBuild(t *testing.T) {
	m := validManifest()
	m.BuildTags = []string{"retail"}
	info := &debug.BuildInfo{
		Settings: []debug.BuildSetting{
			{Key: "-tags", Value: "retail"}, {Key: "-trimpath", Value: "true"},
			{Key: "CGO_ENABLED", Value: "0"}, {Key: "GOARCH", Value: "arm64"}, {Key: "GOOS", Value: "darwin"}, {Key: "GOARM64", Value: "v8.0"},
		},
		Deps: []*debug.Module{{Path: "golang.org/x/sys", Version: "v0.48.0", Sum: "h1:sys48="}},
	}
	if err := m.describes(info, "go1.27.1", "darwin", "arm64"); err != nil {
		t.Fatalf("a matching build was refused: %v", err)
	}
	for _, c := range []struct {
		name string
		edit func(*debug.BuildInfo) (goVersion, goos, goarch string)
	}{
		{"toolchain", func(*debug.BuildInfo) (string, string, string) { return "go1.27.2", "darwin", "arm64" }},
		{"platform", func(*debug.BuildInfo) (string, string, string) { return "go1.27.1", "linux", "arm64" }},
		{"level", func(i *debug.BuildInfo) (string, string, string) {
			i.Settings[5].Value = "v9.0"
			return "go1.27.1", "darwin", "arm64"
		}},
		{"cgo", func(i *debug.BuildInfo) (string, string, string) {
			i.Settings[2].Value = "1"
			return "go1.27.1", "darwin", "arm64"
		}},
		{"tags", func(i *debug.BuildInfo) (string, string, string) {
			i.Settings[0].Value = "retail,pathbench"
			return "go1.27.1", "darwin", "arm64"
		}},
		{"trimpath", func(i *debug.BuildInfo) (string, string, string) {
			i.Settings[1].Value = "false"
			return "go1.27.1", "darwin", "arm64"
		}},
		{"experiment", func(i *debug.BuildInfo) (string, string, string) {
			i.Settings = append(i.Settings, debug.BuildSetting{Key: "GOEXPERIMENT", Value: "arenas"})
			return "go1.27.1", "darwin", "arm64"
		}},
		{"unlisted module", func(i *debug.BuildInfo) (string, string, string) {
			i.Deps = append(i.Deps, &debug.Module{Path: "example.com/other", Version: "v1.0.0", Sum: "h1:x="})
			return "go1.27.1", "darwin", "arm64"
		}},
		{"module of another sum", func(i *debug.BuildInfo) (string, string, string) {
			i.Deps[0].Sum = "h1:changed="
			return "go1.27.1", "darwin", "arm64"
		}},
		{"replaced module", func(i *debug.BuildInfo) (string, string, string) {
			i.Deps[0].Replace = &debug.Module{Path: "../sys"}
			return "go1.27.1", "darwin", "arm64"
		}},
	} {
		edited := *info
		edited.Settings = slices.Clone(info.Settings)
		edited.Deps = []*debug.Module{{Path: "golang.org/x/sys", Version: "v0.48.0", Sum: "h1:sys48="}}
		goVersion, goos, goarch := c.edit(&edited)
		if err := m.describes(&edited, goVersion, goos, goarch); err == nil {
			t.Errorf("%s: a different build was accepted", c.name)
		}
	}
	// Without -trimpath the toolchain records the ldflags, and they must be
	// the stamped ones; with it, it records none, as above.
	untrimmed := cloneManifest(m)
	untrimmed.BuildArgs = []string{"-mod=readonly", "-ldflags=-s -w", "./cmd/nanolathe"}
	recorded := &debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "-ldflags", Value: "-s -w"}, {Key: "-tags", Value: "retail"},
		{Key: "CGO_ENABLED", Value: "0"}, {Key: "GOARM64", Value: "v8.0"}}}
	if err := untrimmed.describes(recorded, "go1.27.1", "darwin", "arm64"); err != nil {
		t.Fatalf("an untrimmed build with its stamped ldflags was refused: %v", err)
	}
	recorded.Settings[0].Value = "-s"
	if err := untrimmed.describes(recorded, "go1.27.1", "darwin", "arm64"); err == nil {
		t.Error("an untrimmed build with other ldflags was accepted")
	}
}
