package version

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
	"unicode/utf8"
)

// The common build manifest (docs/DESIGN_MULTIPLAYER.md §8.7, M2-C10): what
// a simulation build was made from — its source contents, its dependency
// and toolchain inputs and the build choices that reach the binary — and the
// platform variants that release admits. Two players can share a battle
// only when their builds carry one common manifest; a platform variant
// differs inside that manifest, never between manifests. A revision string
// or Profile is never a substitute for it.
//
// Binary hashes and evidence that the variants are equivalent are detached
// attestations keyed by the manifest's digest and a variant; a manifest
// cannot embed the hash of the binary that carries it, so none is a field.
// A build timestamp is not a field either: it would make two builds of the
// same inputs differ.
//
// The encoding uses the primitives of §7.4.1: a digest is 32 raw bytes;
// text(N) a shortest-varint byte length and at most N bytes of UTF-8 without
// NUL; key a varint length and 1..255 bytes of a canonical key — ASCII lower
// case, no NUL and no surrounding space, tab, carriage return or line feed;
// a collection a varint count. There are no optional fields and no trailing
// bytes. Design reading: the payload carries no version of its own; version
// 1 is named by the identity domain.

// buildManifestDomain is the literal UTF-8 domain the identity hashes before
// the encoding, with no trailing NUL.
const buildManifestDomain = "nanolathe/sim-build/1"

// Protocol limits of manifest version 1 (§8.7). They are admission bounds,
// not facts about Go.
const (
	buildManifestMaxBytes  = 1 << 20
	buildGoVersionMaxBytes = 64
	buildMaxModules        = 4096
	buildModulePathMax     = 1024
	buildModuleTextMax     = 255
	buildMaxTags           = 64
	buildExperimentMax     = 1024
	buildMaxArgs           = 64
	buildArgMaxBytes       = 1024
	buildMaxVariants       = 16
	buildKeyMaxBytes       = 255
)

// BuildManifest is the public positional form of §8.7, in wire order.
type BuildManifest struct {
	// SourceTree is the digest of the canonical build-source inventory: every
	// file of the source the build compiled, the generated stamp excepted.
	// The zero digest marks a build that carries no stamp.
	SourceTree [32]byte
	// GoVersion is the toolchain's runtime.Version, such as "go1.27.1".
	GoVersion string
	// GoMod and GoSum are SHA-256 of the module's go.mod and go.sum bytes.
	GoMod [32]byte
	GoSum [32]byte
	// Modules are the pinned dependency modules, sorted by path then
	// version, each with its go.sum hash.
	Modules []BuildModule
	// BuildTags are the build tags, sorted.
	BuildTags []string
	// GoExperiment is the GOEXPERIMENT the build ran under, empty for none.
	GoExperiment string
	// CGOEnabled is the build's CGO_ENABLED.
	CGOEnabled bool
	// BuildArgs are the go build arguments in order — module mode, trimpath,
	// buildvcs, ldflags and the package — never the output path.
	BuildArgs []string
	// Variants are the admitted platform targets, sorted, each with the
	// SHA-256 of the toolchain archive that builds it.
	Variants []BuildVariant
}

// BuildModule is one pinned dependency.
type BuildModule struct {
	Path    string
	Version string
	Sum     string
}

// BuildVariant is one admitted platform target of a common build.
type BuildVariant struct {
	GOOS              string
	GOARCH            string
	ArchitectureLevel string // GOAMD64, GOARM64 or the architecture's own level setting
	ToolchainArchive  [32]byte
}

// ErrUnstampedBuild reports a manifest with no source identity: a
// development build, which normal-room admission refuses (§8.7). A
// development room needs an explicit common manifest instead.
var ErrUnstampedBuild = errors.New("nanolathe: build manifest has no source identity: logical path <build stamp>, providers searched [build manifest], expected a build stamped from its source inventory")

// Stamped reports whether the manifest identifies its source. An unstamped
// manifest describes the running toolchain for diagnostics and has no
// encoding and no digest.
func (m BuildManifest) Stamped() bool { return m.SourceTree != [32]byte{} }

// EncodeBuildManifest validates a manifest and returns its canonical
// encoding.
func EncodeBuildManifest(m BuildManifest) ([]byte, error) {
	if err := m.validate(); err != nil {
		return nil, err
	}
	var w buildWriter
	w.b = append(w.b, m.SourceTree[:]...)
	w.text(m.GoVersion)
	w.b = append(w.b, m.GoMod[:]...)
	w.b = append(w.b, m.GoSum[:]...)
	w.uvarint(uint64(len(m.Modules)))
	for _, mod := range m.Modules {
		w.text(mod.Path)
		w.text(mod.Version)
		w.text(mod.Sum)
	}
	w.uvarint(uint64(len(m.BuildTags)))
	for _, tag := range m.BuildTags {
		w.text(tag)
	}
	w.text(m.GoExperiment)
	if m.CGOEnabled {
		w.b = append(w.b, 1)
	} else {
		w.b = append(w.b, 0)
	}
	w.uvarint(uint64(len(m.BuildArgs)))
	for _, arg := range m.BuildArgs {
		w.text(arg)
	}
	w.uvarint(uint64(len(m.Variants)))
	for _, v := range m.Variants {
		w.text(v.GOOS)
		w.text(v.GOARCH)
		w.text(v.ArchitectureLevel)
		w.b = append(w.b, v.ToolchainArchive[:]...)
	}
	if len(w.b) > buildManifestMaxBytes {
		return nil, buildFieldError("manifest", fmt.Sprintf("an encoding of at most %d bytes, got %d", buildManifestMaxBytes, len(w.b)))
	}
	return w.b, nil
}

// DecodeBuildManifest reads a version 1 payload. It refuses a payload over
// 1 MiB, a count or length over its bound before allocating from it, an
// overlong or overflowing integer, a boolean other than 0 or 1, any field
// EncodeBuildManifest refuses, trailing bytes, and any payload that is not
// the canonical encoding of the value it decodes to. It returns no partially
// valid value.
func DecodeBuildManifest(payload []byte) (BuildManifest, error) {
	if len(payload) > buildManifestMaxBytes {
		return BuildManifest{}, buildFieldError("payload", fmt.Sprintf("at most %d bytes, got %d", buildManifestMaxBytes, len(payload)))
	}
	r := buildReader{b: payload}
	var m BuildManifest
	copy(m.SourceTree[:], r.bytes(32))
	m.GoVersion = r.text(buildGoVersionMaxBytes)
	copy(m.GoMod[:], r.bytes(32))
	copy(m.GoSum[:], r.bytes(32))
	if n := r.count(buildMaxModules, 3); n > 0 {
		m.Modules = make([]BuildModule, n)
		for i := range m.Modules {
			m.Modules[i] = BuildModule{Path: r.text(buildModulePathMax), Version: r.text(buildModuleTextMax), Sum: r.text(buildModuleTextMax)}
		}
	}
	if n := r.count(buildMaxTags, 2); n > 0 {
		m.BuildTags = make([]string, n)
		for i := range m.BuildTags {
			m.BuildTags[i] = r.text(buildKeyMaxBytes)
		}
	}
	m.GoExperiment = r.text(buildExperimentMax)
	switch cgo := r.u8(); {
	case cgo == 1:
		m.CGOEnabled = true
	case cgo > 1:
		r.off--
		r.fail("a boolean of exactly 0 or 1")
	}
	if n := r.count(buildMaxArgs, 1); n > 0 {
		m.BuildArgs = make([]string, n)
		for i := range m.BuildArgs {
			m.BuildArgs[i] = r.text(buildArgMaxBytes)
		}
	}
	if n := r.count(buildMaxVariants, 3+32); n > 0 {
		m.Variants = make([]BuildVariant, n)
		for i := range m.Variants {
			v := &m.Variants[i]
			v.GOOS = r.text(buildKeyMaxBytes)
			v.GOARCH = r.text(buildKeyMaxBytes)
			v.ArchitectureLevel = r.text(buildKeyMaxBytes)
			copy(v.ToolchainArchive[:], r.bytes(32))
		}
	}
	if r.err != nil {
		return BuildManifest{}, r.err
	}
	if r.off != len(payload) {
		return BuildManifest{}, buildFieldError(fmt.Sprintf("payload byte %d", r.off), "no bytes after the last field")
	}
	again, err := EncodeBuildManifest(m)
	if err != nil {
		return BuildManifest{}, err
	}
	if !bytes.Equal(again, payload) {
		return BuildManifest{}, buildFieldError("payload", "the canonical encoding of the manifest it decodes to")
	}
	return m, nil
}

// Digest is the common build identity: SHA-256 of the domain and the
// canonical encoding. It fails for a manifest EncodeBuildManifest refuses,
// an unstamped one included.
func (m BuildManifest) Digest() ([32]byte, error) {
	payload, err := EncodeBuildManifest(m)
	if err != nil {
		return [32]byte{}, err
	}
	h := sha256.New()
	h.Write([]byte(buildManifestDomain))
	h.Write(payload)
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out, nil
}

// validate checks every field of §8.7.
func (m BuildManifest) validate() error {
	if !m.Stamped() {
		return ErrUnstampedBuild
	}
	if err := buildText("goVersion", m.GoVersion, buildGoVersionMaxBytes); err != nil {
		return err
	}
	if m.GoVersion == "" {
		return buildFieldError("goVersion", "the toolchain's version")
	}
	if len(m.Modules) > buildMaxModules {
		return buildFieldError("modules", fmt.Sprintf("at most %d modules", buildMaxModules))
	}
	for i, mod := range m.Modules {
		at := fmt.Sprintf("modules[%d]", i)
		for _, f := range []struct {
			name, value string
			max         int
		}{{"path", mod.Path, buildModulePathMax}, {"version", mod.Version, buildModuleTextMax}, {"sum", mod.Sum, buildModuleTextMax}} {
			if err := buildText(at+"."+f.name, f.value, f.max); err != nil {
				return err
			}
			if f.value == "" {
				return buildFieldError(at+"."+f.name, "a nonempty value")
			}
		}
		if i > 0 {
			prev := m.Modules[i-1]
			if prev.Path > mod.Path || (prev.Path == mod.Path && prev.Version >= mod.Version) {
				return buildFieldError(at, "modules in strictly ascending path, then version, order")
			}
		}
	}
	if len(m.BuildTags) > buildMaxTags {
		return buildFieldError("buildTags", fmt.Sprintf("at most %d tags", buildMaxTags))
	}
	for i, tag := range m.BuildTags {
		at := fmt.Sprintf("buildTags[%d]", i)
		if err := buildKey(at, tag); err != nil {
			return err
		}
		if i > 0 && m.BuildTags[i-1] >= tag {
			return buildFieldError(at, "tags in strictly ascending order")
		}
	}
	if err := buildText("goExperiment", m.GoExperiment, buildExperimentMax); err != nil {
		return err
	}
	if len(m.BuildArgs) > buildMaxArgs {
		return buildFieldError("buildArgs", fmt.Sprintf("at most %d arguments", buildMaxArgs))
	}
	for i, arg := range m.BuildArgs {
		at := fmt.Sprintf("buildArgs[%d]", i)
		if err := buildText(at, arg, buildArgMaxBytes); err != nil {
			return err
		}
		if arg == "" {
			return buildFieldError(at, "a nonempty argument")
		}
	}
	if len(m.Variants) < 1 || len(m.Variants) > buildMaxVariants {
		return buildFieldError("variants", fmt.Sprintf("1..%d platform variants", buildMaxVariants))
	}
	for i, v := range m.Variants {
		at := fmt.Sprintf("variants[%d]", i)
		for _, f := range []struct{ name, value string }{{"goos", v.GOOS}, {"goarch", v.GOARCH}, {"architectureLevel", v.ArchitectureLevel}} {
			if err := buildKey(at+"."+f.name, f.value); err != nil {
				return err
			}
		}
		if v.ToolchainArchive == ([32]byte{}) {
			return buildFieldError(at+".toolchainArchive", "the toolchain archive's SHA-256")
		}
		if i > 0 && compareVariants(m.Variants[i-1], v) >= 0 {
			return buildFieldError(at, "variants in strictly ascending GOOS, GOARCH, level order")
		}
	}
	return nil
}

func compareVariants(a, b BuildVariant) int {
	if c := strings.Compare(a.GOOS, b.GOOS); c != 0 {
		return c
	}
	if c := strings.Compare(a.GOARCH, b.GOARCH); c != 0 {
		return c
	}
	return strings.Compare(a.ArchitectureLevel, b.ArchitectureLevel)
}

func buildFieldError(path, expected string) error {
	return fmt.Errorf("nanolathe: build manifest rejected: logical path %s, providers searched [build manifest v1], expected %s", path, expected)
}

// buildText checks a text(max) value.
func buildText(path, s string, max int) error {
	if len(s) > max {
		return buildFieldError(path, fmt.Sprintf("at most %d bytes", max))
	}
	if !utf8.ValidString(s) || strings.IndexByte(s, 0) >= 0 {
		return buildFieldError(path, "valid UTF-8 without NUL")
	}
	return nil
}

// buildKey checks a key value: what content.CanonicalKey leaves unchanged,
// written out here so this package stays a leaf.
func buildKey(path, s string) error {
	ok := len(s) >= 1 && len(s) <= buildKeyMaxBytes && strings.IndexByte(s, 0) < 0
	for i := 0; ok && i < len(s); i++ {
		ok = s[i] < 'A' || s[i] > 'Z'
	}
	if ok {
		space := func(b byte) bool { return b == ' ' || b == '\t' || b == '\r' || b == '\n' }
		ok = !space(s[0]) && !space(s[len(s)-1])
	}
	if !ok {
		return buildFieldError(path, fmt.Sprintf("a canonical key of 1..%d bytes", buildKeyMaxBytes))
	}
	return nil
}

// stamp is the encoded common build manifest of a stamped build. The
// installer's stamp generator (internal/version/stampgen) writes StampFile,
// which sets it, into the verified source before the build; a build without
// that file leaves it empty. The file is the one source file the manifest's
// source inventory leaves out, since it is generated from that inventory.
var stamp string

// StampFile is the generated stamp's path relative to the module root.
const StampFile = "internal/version/stamp_generated.go"

// CurrentBuildManifest is the running build's common manifest. A build with
// no stamp — every development build — returns an unstamped manifest that
// names only the running toolchain and platform, and no error: it runs, and
// Stamped, EncodeBuildManifest and Digest mark it for refusal by
// normal-room admission. A stamp that does not decode, or that describes a
// different build than the running binary records, is an error.
func CurrentBuildManifest() (BuildManifest, error) {
	info, _ := debug.ReadBuildInfo()
	if stamp == "" {
		return unstampedManifest(info), nil
	}
	m, err := DecodeBuildManifest([]byte(stamp))
	if err != nil {
		return unstampedManifest(info), fmt.Errorf("nanolathe: build stamp unreadable: logical path %s, providers searched [build stamp], expected a version 1 build manifest: %w", StampFile, err)
	}
	if err := m.describes(info, runtime.Version(), runtime.GOOS, runtime.GOARCH); err != nil {
		return unstampedManifest(info), err
	}
	return m, nil
}

// unstampedManifest names the running toolchain and platform and nothing
// else: its zero SourceTree marks it unstamped.
func unstampedManifest(info *debug.BuildInfo) BuildManifest {
	return BuildManifest{
		GoVersion: runtime.Version(),
		Variants:  []BuildVariant{{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, ArchitectureLevel: buildSetting(info, architectureLevelSetting(runtime.GOARCH))}},
	}
}

// architectureLevelSetting is the build setting that records an
// architecture's instruction-set level, or "" for an architecture with none.
func architectureLevelSetting(goarch string) string {
	switch goarch {
	case "amd64":
		return "GOAMD64"
	case "arm64":
		return "GOARM64"
	case "386":
		return "GO386"
	case "arm":
		return "GOARM"
	case "mips", "mipsle":
		return "GOMIPS"
	case "mips64", "mips64le":
		return "GOMIPS64"
	case "ppc64", "ppc64le":
		return "GOPPC64"
	case "riscv64":
		return "GORISCV64"
	case "wasm":
		return "GOWASM"
	}
	return ""
}

// buildSetting is one recorded build setting, "" when absent.
func buildSetting(info *debug.BuildInfo, key string) string {
	if info == nil || key == "" {
		return ""
	}
	for _, s := range info.Settings {
		if s.Key == key {
			return s.Value
		}
	}
	return ""
}

// describes checks a stamp against what the running binary records about
// its own build: the toolchain, a listed variant for this platform at its
// level, cgo, tags, GOEXPERIMENT, trimpath and ldflags, and every linked
// module. A stamp copied into another build fails here.
func (m BuildManifest) describes(info *debug.BuildInfo, goVersion, goos, goarch string) error {
	fail := func(what string) error {
		return fmt.Errorf("nanolathe: build stamp does not describe this binary: logical path %s, providers searched [build stamp, build info], expected %s", StampFile, what)
	}
	if m.GoVersion != goVersion {
		return fail(fmt.Sprintf("toolchain %s, stamped %s", goVersion, m.GoVersion))
	}
	level := buildSetting(info, architectureLevelSetting(goarch))
	listed := false
	for _, v := range m.Variants {
		if v.GOOS == goos && v.GOARCH == goarch && (level == "" || v.ArchitectureLevel == level) {
			listed = true
		}
	}
	if !listed {
		return fail(fmt.Sprintf("a variant for %s/%s %s", goos, goarch, level))
	}
	if info == nil {
		return nil
	}
	cgo := buildSetting(info, "CGO_ENABLED")
	if cgo != "" && (cgo == "1") != m.CGOEnabled {
		return fail(fmt.Sprintf("CGO_ENABLED=%s", cgo))
	}
	var tags []string
	if t := buildSetting(info, "-tags"); t != "" {
		tags = strings.Split(t, ",")
		slices.Sort(tags)
	}
	if !slices.Equal(tags, m.BuildTags) {
		return fail(fmt.Sprintf("build tags %q", tags))
	}
	if x := buildSetting(info, "GOEXPERIMENT"); x != m.GoExperiment {
		return fail(fmt.Sprintf("GOEXPERIMENT %q", x))
	}
	trimpath := buildSetting(info, "-trimpath") == "true"
	if trimpath != slices.Contains(m.BuildArgs, "-trimpath") {
		return fail(fmt.Sprintf("-trimpath %v", trimpath))
	}
	// The toolchain records -ldflags only without -trimpath, because the
	// flags often hold absolute paths; a trimmed build's ldflags are known
	// from its stamp alone.
	if ldflags := buildSetting(info, "-ldflags"); !trimpath && ldflags != buildArgValue(m.BuildArgs, "-ldflags") {
		return fail(fmt.Sprintf("-ldflags %q", ldflags))
	}
	for _, dep := range info.Deps {
		if dep.Replace != nil {
			return fail(fmt.Sprintf("no replaced module, found %s", dep.Path))
		}
		if !slices.Contains(m.Modules, BuildModule{Path: dep.Path, Version: dep.Version, Sum: dep.Sum}) {
			return fail(fmt.Sprintf("linked module %s %s %s among the stamped modules", dep.Path, dep.Version, dep.Sum))
		}
	}
	return nil
}

// buildArgValue is the value of a "-flag=value" build argument, "" when the
// flag is absent.
func buildArgValue(args []string, flag string) string {
	for _, arg := range args {
		if value, ok := strings.CutPrefix(arg, flag+"="); ok {
			return value
		}
	}
	return ""
}

// buildWriter appends §7.4.1 primitives to a manifest encoding.
type buildWriter struct{ b []byte }

func (w *buildWriter) uvarint(v uint64) {
	for v >= 0x80 {
		w.b = append(w.b, byte(v)|0x80)
		v >>= 7
	}
	w.b = append(w.b, byte(v))
}

func (w *buildWriter) text(s string) {
	w.uvarint(uint64(len(s)))
	w.b = append(w.b, s...)
}

// buildReader reads §7.4.1 primitives; the first failure sticks.
type buildReader struct {
	b   []byte
	off int
	err error
}

func (r *buildReader) fail(expected string) {
	if r.err == nil {
		r.err = buildFieldError(fmt.Sprintf("payload byte %d", r.off), expected)
	}
}

func (r *buildReader) u8() uint8 {
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

// uvarint32 reads the shortest varint of a 32-bit value.
func (r *buildReader) uvarint32() uint64 {
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
		if i == 4 && c > 0x0f {
			r.off = start
			r.fail("a varint within 32 bits")
			return 0
		}
		v |= uint64(c&0x7f) << (7 * uint(i))
		if c < 0x80 {
			if i > 0 && c == 0 {
				r.off = start
				r.fail("the shortest varint")
				return 0
			}
			return v
		}
	}
}

func (r *buildReader) bytes(n int) []byte {
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

func (r *buildReader) text(max int) string {
	n := r.uvarint32()
	if r.err != nil {
		return ""
	}
	if n > uint64(max) {
		r.fail(fmt.Sprintf("a text of at most %d bytes", max))
		return ""
	}
	return string(r.bytes(int(n)))
}

// count reads a collection count and checks it against its bound and the
// bytes remaining before anything is allocated from it.
func (r *buildReader) count(max, minRecord int) int {
	n := r.uvarint32()
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
