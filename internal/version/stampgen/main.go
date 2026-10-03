// Command stampgen writes a build's common manifest into its source before
// the build (docs/DESIGN_MULTIPLAYER.md §8.7). The installer runs it with
// the pinned toolchain, in the environment it builds with, once it has
// verified the source archive:
//
//	go run -mod=readonly -trimpath -buildvcs=false ./internal/version/stampgen \
//	    -source source.tar.gz -prefix nanolathe-<revision>/ -write <extracted root> \
//	    -cgo 0 -variant darwin/arm64/v8.0=<sha256> ... -arg=-mod=readonly -arg=-trimpath ...
//
// Each -arg is one go build argument, in the order the build passes them;
// a repeated flag rather than a trailing list, because PowerShell can drop
// a bare "--" on its way to a native command.
//
// It inventories the source, reads go.mod and go.sum from that inventory,
// takes the toolchain version from the toolchain that runs it, and writes
// version.StampFile under -write. It prints the manifest's digest.
//
// The source inventory is every file under the source root — paths, kinds,
// executable bits, lengths and bytes — except the stamp itself, which is
// generated from it. Nothing is left out for being dirty or untracked: a
// tree holding version-control metadata is refused rather than filtered, so
// a development manifest is always an explicit inventory. A release stamp
// reads the verified archive the installer downloaded, the same tar.gz or
// zip on every platform, so a Windows and a Unix install of one release
// agree even though Windows extraction cannot keep executable bits or
// symbolic links. A directory source is for development trees on systems
// that keep both.
package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/nanolathe-gg/nanolathe/internal/version"
)

func main() {
	digest, err := run(os.Args[1:], runtime.Version())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(digest)
}

// stampError is the generator's one diagnostic shape.
func stampError(what, logicalPath, expected string) error {
	return fmt.Errorf("nanolathe: %s: logical path %s, providers searched [build source inventory], expected %s", what, logicalPath, expected)
}

// listFlag collects a repeated flag's values in order.
type listFlag []string

func (v *listFlag) String() string     { return strings.Join(*v, " ") }
func (v *listFlag) Set(s string) error { *v = append(*v, s); return nil }

// run is the command: it returns the manifest digest in hex.
func run(args []string, goVersion string) (string, error) {
	flags := flag.NewFlagSet("stampgen", flag.ContinueOnError)
	source := flags.String("source", "", "the build source: a directory, a .tar.gz or .tgz archive, or a .zip archive")
	prefix := flags.String("prefix", "", "an archive's one top-level directory, ending in a slash")
	write := flags.String("write", "", "the module root the build compiles, where the stamp is written")
	cgo := flags.String("cgo", "", "the build's CGO_ENABLED: 0 or 1")
	tags := flags.String("tags", "", "the build's tags, comma-separated")
	experiment := flags.String("goexperiment", "", "the build's GOEXPERIMENT")
	var variants, buildArgs listFlag
	flags.Var(&variants, "variant", "an admitted platform, GOOS/GOARCH/LEVEL=SHA256 of its toolchain archive (repeatable)")
	flags.Var(&buildArgs, "arg", "one go build argument, in build order (repeatable)")
	if err := flags.Parse(args); err != nil {
		return "", err
	}
	if flags.NArg() != 0 {
		return "", stampError("unexpected argument "+strconv.Quote(flags.Arg(0)), "<command line>", "build arguments as -arg values")
	}
	if *source == "" || *write == "" {
		return "", stampError("missing argument", "<command line>", "-source and -write")
	}
	if *cgo != "0" && *cgo != "1" {
		return "", stampError("missing argument", "<command line>", "-cgo 0 or -cgo 1")
	}
	entries, err := readInventory(*source, *prefix)
	if err != nil {
		return "", err
	}
	m := version.BuildManifest{
		SourceTree:   inventoryDigest(entries),
		GoVersion:    goVersion,
		GoExperiment: *experiment,
		CGOEnabled:   *cgo == "1",
		BuildArgs:    buildArgs,
	}
	goMod, ok := findEntry(entries, "go.mod")
	if !ok {
		return "", stampError("source has no go.mod", *source, "a module root")
	}
	goSum, ok := findEntry(entries, "go.sum")
	if !ok {
		return "", stampError("source has no go.sum", *source, "a module with pinned dependencies")
	}
	m.GoMod, m.GoSum = sha256.Sum256(goMod.data), sha256.Sum256(goSum.data)
	if err := refuseReplacements(goMod.data); err != nil {
		return "", err
	}
	if m.Modules, err = modulesFromSum(goSum.data); err != nil {
		return "", err
	}
	if *tags != "" {
		m.BuildTags = strings.Split(*tags, ",")
		slices.Sort(m.BuildTags)
	}
	for _, arg := range m.BuildArgs {
		if arg == "-o" || strings.HasPrefix(arg, "-o=") || strings.HasPrefix(arg, "-tags") || strings.HasPrefix(arg, "--tags") {
			return "", stampError("build argument "+strconv.Quote(arg)+" refused", "<command line>", "build arguments without the output path; tags go in -tags")
		}
	}
	for _, v := range variants {
		variant, err := parseVariant(v)
		if err != nil {
			return "", err
		}
		m.Variants = append(m.Variants, variant)
	}
	slices.SortFunc(m.Variants, func(a, b version.BuildVariant) int {
		return strings.Compare(a.GOOS+"/"+a.GOARCH+"/"+a.ArchitectureLevel, b.GOOS+"/"+b.GOARCH+"/"+b.ArchitectureLevel)
	})
	payload, err := version.EncodeBuildManifest(m)
	if err != nil {
		return "", err
	}
	// The stamp is what the binary will decode; prove it decodes before it
	// is written.
	decoded, err := version.DecodeBuildManifest(payload)
	if err != nil {
		return "", err
	}
	digest, err := decoded.Digest()
	if err != nil {
		return "", err
	}
	target := filepath.Join(*write, filepath.FromSlash(version.StampFile))
	if err := os.WriteFile(target, stampSource(payload), 0o644); err != nil {
		return "", stampError("cannot write the stamp: "+err.Error(), target, "a writable module root holding internal/version")
	}
	return hex.EncodeToString(digest[:]), nil
}

// stampSource is the generated Go file that carries the encoded manifest.
func stampSource(payload []byte) []byte {
	var b bytes.Buffer
	b.WriteString("// Code generated by internal/version/stampgen. DO NOT EDIT.\n\n")
	b.WriteString("package version\n\n")
	b.WriteString("func init() { stamp = " + strconv.Quote(string(payload)) + " }\n")
	return b.Bytes()
}

// parseVariant reads GOOS/GOARCH/LEVEL=SHA256.
func parseVariant(s string) (version.BuildVariant, error) {
	target, sum, ok := strings.Cut(s, "=")
	parts := strings.Split(target, "/")
	raw, err := hex.DecodeString(sum)
	if !ok || len(parts) != 3 || err != nil || len(raw) != sha256.Size {
		return version.BuildVariant{}, stampError("variant "+strconv.Quote(s)+" unreadable", "<command line>", "GOOS/GOARCH/LEVEL=<64 hex digits>")
	}
	v := version.BuildVariant{GOOS: parts[0], GOARCH: parts[1], ArchitectureLevel: parts[2]}
	copy(v.ToolchainArchive[:], raw)
	return v, nil
}

// entry is one inventoried file.
type entry struct {
	path string
	kind uint8
	data []byte
}

// The inventory's kinds.
const (
	kindRegular    = 1
	kindExecutable = 2
	kindSymlink    = 3 // data is the link target
)

// inventoryDomain is hashed before the inventory, which is: the entry count,
// then each entry in byte order of its path — the path as text, the kind
// byte, the length as a varint, the bytes — in the primitives of §7.4.1.
const inventoryDomain = "nanolathe/source-tree/1"

const inventoryPathMax = 4096

func inventoryDigest(entries []entry) [32]byte {
	h := sha256.New()
	h.Write([]byte(inventoryDomain))
	h.Write(uvarint(uint64(len(entries))))
	for _, e := range entries {
		h.Write(uvarint(uint64(len(e.path))))
		h.Write([]byte(e.path))
		h.Write([]byte{e.kind})
		h.Write(uvarint(uint64(len(e.data))))
		h.Write(e.data)
	}
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

func uvarint(v uint64) []byte {
	var out []byte
	for v >= 0x80 {
		out = append(out, byte(v)|0x80)
		v >>= 7
	}
	return append(out, byte(v))
}

func findEntry(entries []entry, p string) (entry, bool) {
	i, ok := slices.BinarySearchFunc(entries, p, func(e entry, p string) int { return strings.Compare(e.path, p) })
	if !ok || entries[i].kind == kindSymlink {
		return entry{}, false
	}
	return entries[i], true
}

// readInventory reads a source by its form and returns its entries sorted
// by path, the stamp left out.
func readInventory(source, prefix string) ([]entry, error) {
	var entries []entry
	var err error
	switch {
	case strings.HasSuffix(source, ".tar.gz") || strings.HasSuffix(source, ".tgz"):
		entries, err = readTarGz(source, prefix)
	case strings.HasSuffix(source, ".zip"):
		entries, err = readZip(source, prefix)
	default:
		if prefix != "" {
			return nil, stampError("-prefix given for a directory", source, "-prefix only with an archive")
		}
		entries, err = readDir(source)
	}
	if err != nil {
		return nil, err
	}
	kept := entries[:0]
	for _, e := range entries {
		if e.path != version.StampFile {
			kept = append(kept, e)
		}
	}
	slices.SortFunc(kept, func(a, b entry) int { return strings.Compare(a.path, b.path) })
	for i := 1; i < len(kept); i++ {
		if kept[i-1].path == kept[i].path {
			return nil, stampError("source lists a file twice", kept[i].path, "one entry per path")
		}
	}
	return kept, nil
}

// checkPath admits a relative slash-separated path of valid UTF-8 with no
// empty, dot or dot-dot element and no version-control metadata.
func checkPath(p string) error {
	if p == "" || len(p) > inventoryPathMax || !utf8.ValidString(p) || strings.IndexByte(p, 0) >= 0 || strings.HasPrefix(p, "/") {
		return stampError("source path refused", strconv.Quote(p), fmt.Sprintf("a relative UTF-8 path of at most %d bytes", inventoryPathMax))
	}
	for _, element := range strings.Split(p, "/") {
		switch element {
		case "", ".", "..":
			return stampError("source path refused", strconv.Quote(p), "a clean relative path")
		case ".git":
			return stampError("source holds version-control metadata", strconv.Quote(p), "an exported source tree (git archive), not a checkout")
		}
	}
	return nil
}

// stripPrefix removes an archive's one top-level directory.
func stripPrefix(name, prefix string) (string, error) {
	if prefix == "" || !strings.HasSuffix(prefix, "/") {
		return "", stampError("archive prefix missing", "<command line>", "-prefix naming the archive's top-level directory, ending in a slash")
	}
	rest, ok := strings.CutPrefix(name, prefix)
	if !ok {
		return "", stampError("archive entry outside the prefix", strconv.Quote(name), "every entry under "+prefix)
	}
	return rest, nil
}

func readDir(root string) ([]entry, error) {
	var entries []entry
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if err := checkPath(rel); err != nil {
			return err
		}
		info, err := os.Lstat(p)
		if err != nil {
			return err
		}
		switch mode := info.Mode(); {
		case mode.IsDir():
			return nil
		case mode&fs.ModeSymlink != 0:
			target, err := os.Readlink(p)
			if err != nil {
				return err
			}
			entries = append(entries, entry{path: rel, kind: kindSymlink, data: []byte(filepath.ToSlash(target))})
		case mode.IsRegular():
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			entries = append(entries, entry{path: rel, kind: regularKind(mode), data: data})
		default:
			return stampError("source holds a special file", rel, "regular files, executables and symbolic links")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

func regularKind(mode fs.FileMode) uint8 {
	if mode.Perm()&0o111 != 0 {
		return kindExecutable
	}
	return kindRegular
}

func readTarGz(source, prefix string) ([]entry, error) {
	file, err := os.Open(source)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return nil, stampError("archive unreadable: "+err.Error(), source, "a gzip-compressed tar archive")
	}
	tr := tar.NewReader(gz)
	var entries []entry
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, stampError("archive unreadable: "+err.Error(), source, "a tar archive")
		}
		if header.Typeflag == tar.TypeXGlobalHeader {
			continue // the archive's own metadata, such as a commit id
		}
		name, err := stripPrefix(header.Name, prefix)
		if err != nil {
			return nil, err
		}
		if header.Typeflag == tar.TypeDir {
			continue
		}
		name = strings.TrimSuffix(name, "/")
		if err := checkPath(name); err != nil {
			return nil, err
		}
		switch header.Typeflag {
		case tar.TypeReg, '\x00': // '\x00' is the old regular-file flag
			data, err := io.ReadAll(tr)
			if err != nil {
				return nil, stampError("archive unreadable: "+err.Error(), name, "a complete file")
			}
			entries = append(entries, entry{path: name, kind: regularKind(header.FileInfo().Mode()), data: data})
		case tar.TypeSymlink:
			entries = append(entries, entry{path: name, kind: kindSymlink, data: []byte(header.Linkname)})
		default:
			return nil, stampError("archive holds a special entry", name, "regular files, executables, symbolic links and directories")
		}
	}
	return entries, nil
}

func readZip(source, prefix string) ([]entry, error) {
	archive, err := zip.OpenReader(source)
	if err != nil {
		return nil, stampError("archive unreadable: "+err.Error(), source, "a zip archive")
	}
	defer archive.Close()
	var entries []entry
	for _, f := range archive.File {
		name, err := stripPrefix(f.Name, prefix)
		if err != nil {
			return nil, err
		}
		mode := f.Mode()
		if mode.IsDir() {
			continue
		}
		if err := checkPath(name); err != nil {
			return nil, err
		}
		if mode&fs.ModeSymlink == 0 && !mode.IsRegular() {
			return nil, stampError("archive holds a special entry", name, "regular files, executables, symbolic links and directories")
		}
		reader, err := f.Open()
		if err != nil {
			return nil, stampError("archive unreadable: "+err.Error(), name, "a readable entry")
		}
		data, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			return nil, stampError("archive unreadable: "+err.Error(), name, "a complete entry")
		}
		kind := regularKind(mode)
		if mode&fs.ModeSymlink != 0 {
			kind = kindSymlink
		}
		entries = append(entries, entry{path: name, kind: kind, data: data})
	}
	return entries, nil
}

// refuseReplacements refuses a go.mod with a replace directive. A replaced
// module's contents are not identified by go.sum, and version 1 of the stamp
// has no field for them; the module has none.
func refuseReplacements(goMod []byte) error {
	for _, line := range strings.Split(string(goMod), "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		fields := strings.Fields(line)
		if len(fields) > 0 && (fields[0] == "replace" || strings.HasPrefix(fields[0], "replace(")) {
			return stampError("go.mod replaces a module", "go.mod", "no replace directive in a stamped build")
		}
	}
	return nil
}

// modulesFromSum lists go.sum's module hashes — every line but the go.mod
// hashes — sorted by path, then version.
func modulesFromSum(goSum []byte) ([]version.BuildModule, error) {
	var modules []version.BuildModule
	for i, line := range strings.Split(string(goSum), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 3 {
			return nil, stampError("go.sum line unreadable", fmt.Sprintf("go.sum:%d", i+1), "module path, version and hash")
		}
		if strings.HasSuffix(fields[1], "/go.mod") {
			continue
		}
		modules = append(modules, version.BuildModule{Path: fields[0], Version: fields[1], Sum: fields[2]})
	}
	slices.SortFunc(modules, func(a, b version.BuildModule) int {
		if c := strings.Compare(a.Path, b.Path); c != 0 {
			return c
		}
		return strings.Compare(a.Version, b.Version)
	})
	return modules, nil
}
