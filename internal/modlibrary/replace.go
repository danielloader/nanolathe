package modlibrary

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// replacementName holds the previous complete install during a same-version
// catalogue update (§5.3). It is separate from disposable extraction staging:
// an interrupted replacement must restore this copy before staging is cleared.
const replacementName = ".replaced"

func (l *Library) replacementDir(id, version string) string {
	return filepath.Join(l.Root, replacementName, id, version)
}

// commitInstall serializes publication with Open's recovery and Remove in this
// process. Validation and receipt writing have finished before it is called.
func (l *Library) commitInstall(staged, target string, meta Metadata, opts InstallOptions) error {
	if opts.BeforeCommit != nil {
		if err := opts.BeforeCommit(); err != nil {
			return err
		}
	}
	staging.Lock()
	defer staging.Unlock()
	if err := l.recoverReplacement(meta.ID, meta.Version); err != nil {
		return err
	}
	if err := l.checkInstallTarget(target, meta, opts.Replace); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return diagnostic("creating the mod directory failed: "+err.Error(), filepath.Dir(target), []string{l.Root}, "a writable mod library")
	}
	backup := l.replacementDir(meta.ID, meta.Version)
	moved := false
	if _, err := os.Lstat(target); err == nil {
		if err := os.MkdirAll(filepath.Dir(backup), 0o755); err != nil {
			return diagnostic("preparing the previous mod backup failed: "+err.Error(), backup, []string{l.Root}, "a writable backup directory")
		}
		if err := os.Rename(target, backup); err != nil {
			return diagnostic("backing up the previous mod failed: "+err.Error(), target, []string{l.Root}, "a movable installed mod")
		}
		moved = true
	}
	if err := os.Rename(staged, target); err != nil {
		if moved {
			if restoreErr := os.Rename(backup, target); restoreErr != nil {
				return diagnostic("committing the mod failed: "+err.Error()+"; restoring the previous mod failed: "+restoreErr.Error(), backup, []string{l.Root}, "the previous mod backup to remain for recovery on the next library open")
			}
		}
		return diagnostic("committing the mod failed: "+err.Error(), target, []string{l.Root}, "a writable mod library")
	}
	// The new install is complete. A failed cleanup leaves a redundant backup
	// which Open may remove; it cannot turn success into a failed replacement.
	if moved {
		_ = os.RemoveAll(backup)
		_ = os.Remove(filepath.Dir(backup))
	}
	return nil
}

// recoverReplacement chooses only between complete directory publications:
// a missing target restores the previous install; a complete target means the
// new rename finished. An unreadable target keeps the backup and reports it.
func (l *Library) recoverReplacement(id, version string) error {
	backup := l.replacementDir(id, version)
	info, err := os.Lstat(backup)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	fail := func(reason string) error {
		return diagnostic("recovering the previous mod failed: "+reason, backup, []string{l.Root}, "a recoverable mod backup; keep it until recovery succeeds")
	}
	if err != nil {
		return fail(err.Error())
	}
	if !info.IsDir() {
		return fail("the backup is not a directory")
	}
	target := l.modDir(id, version)
	info, err = os.Lstat(target)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fail(err.Error())
		}
		if err := os.Rename(backup, target); err != nil {
			return fail(err.Error())
		}
	case err != nil:
		return fail(err.Error())
	default:
		if _, complete := l.readMod(id, version); !info.IsDir() || !complete {
			return fail("the replacement target is not a complete install")
		}
		if err := os.RemoveAll(backup); err != nil {
			return fail(err.Error())
		}
	}
	_ = os.Remove(filepath.Dir(backup))
	return nil
}

// recoverReplacements runs before Open removes abandoned extraction staging.
// It also runs on later opens with no installs in flight, allowing recovery
// after a publication and rollback both failed without restarting the host.
func (l *Library) recoverReplacements() error {
	root := filepath.Join(l.Root, replacementName)
	ids, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return diagnostic("reading mod backups failed: "+err.Error(), root, []string{l.Root}, "readable mod backups")
	}
	for _, id := range ids {
		if !id.IsDir() || !idPattern.MatchString(id.Name()) {
			continue
		}
		versions, err := os.ReadDir(filepath.Join(root, id.Name()))
		if err != nil {
			return diagnostic("reading mod backups failed: "+err.Error(), id.Name(), []string{root}, "readable mod backups")
		}
		for _, version := range versions {
			if !versionPattern.MatchString(version.Name()) {
				continue
			}
			if err := l.recoverReplacement(id.Name(), version.Name()); err != nil {
				return err
			}
		}
	}
	return nil
}
