// Command pack writes the Infected Ancients content directory as one UFO
// archive. Unit definitions are admitted only from archives, so the loose
// content files cannot be mounted directly:
//
//	go run ./art/infected-ancients/pack -out local/infected-ancients.ufo
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/nanolathe-gg/nanolathe/vfs"
)

func main() {
	dir := flag.String("content", "art/infected-ancients/content", "content directory to archive")
	out := flag.String("out", "", "archive to write")
	flag.Parse()
	if *out == "" {
		fmt.Fprintln(os.Stderr, "nanolathe: pack: -out is required")
		os.Exit(2)
	}
	if err := pack(*dir, *out); err != nil {
		fmt.Fprintln(os.Stderr, "nanolathe: pack:", err)
		os.Exit(1)
	}
}

func pack(dir, out string) error {
	var files []vfs.ArchiveFile
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%s is not a regular file", path)
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files = append(files, vfs.ArchiveFile{Path: filepath.ToSlash(rel), Data: data})
		return nil
	})
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no content files under %s", dir)
	}
	var buf bytes.Buffer
	if err := vfs.WriteArchive(&buf, files, vfs.ArchiveWriteOptions{Compress: true, Year: 2026}); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	return os.WriteFile(out, buf.Bytes(), 0o644)
}
