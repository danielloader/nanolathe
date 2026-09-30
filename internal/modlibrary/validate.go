package modlibrary

import (
	"fmt"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// requiredProducts are the logical files whose absence stops the desktop
// command at start-up. The list mirrors openContent in cmd/nanolathe, which
// owns it: MOVEINFO.TDF and SIDEDATA.TDF are the hard requirements, and
// GAMEDATA.TDF is not one (docs/SPEC_CONFLICTS.md SC2).
var requiredProducts = [...]string{"gamedata/moveinfo.tdf", "gamedata/sidedata.tdf"}

// WithConfig returns the mod with file's config in place of its own: the
// content, rules and recommendations of a nanolathe-mod.json named by
// `--mod-config`, so a config can be tried against an installed mod before it
// is packaged (§4.3). The mod keeps its identity, directory and receipt.
func (m Mod) WithConfig(file Metadata) Mod {
	m.Config = file.Config
	m.ContentProfile = ""
	m.MinimumGameplay = file.MinimumGameplay
	m.Controls = file.Controls
	m.BuildMenuPageSize = file.BuildMenuPageSize
	return m
}

// ResolveProfileDefaults returns the mod unchanged. A mod's controls preset
// and gameplay minimum come from its own config, read with its metadata, so
// there is nothing left to resolve against the content; the function stays
// for the screens that ask before listing a mod (§4.3).
func ResolveProfileDefaults(baseRoots []string, m Mod) Mod { return m }

// ContentValidator is the standard §5.3 step 4 check for InstallOptions:
// mount the base install plus the staged root in a scratch overlay, apply the
// mod's content section (the base game's profile for a mod without a
// config), require every directory the section's `detect` list names, and
// require what the desktop command requires before it will start — the two
// hard-required game-data files through the section's directory view, and a
// readable translation table. A mod that would not start, or whose config
// describes other content, is never installed. baseRoots are the resolved
// base install roots (install.Resolve), without any mod.
func ContentValidator(baseRoots []string) func(stagedRoot string, meta Metadata) error {
	base := append([]string(nil), baseRoots...)
	return func(stagedRoot string, meta Metadata) error {
		roots := append(append([]string(nil), base...), stagedRoot)
		fileSystem := vfs.New()
		defer fileSystem.Close()
		if err := fileSystem.MountGameDirectories(roots); err != nil {
			return diagnostic("mounting the mod for validation failed: "+err.Error(), "<content roots>", roots, "readable content directories and archives")
		}
		profile := meta.Content()
		if missing := profile.MissingMarkers(fileSystem); len(missing) > 0 {
			return diagnostic(fmt.Sprintf("mod %s@%s does not match its config: content directory %s is missing", meta.ID, meta.Version, missing[0]), missing[0], fileSystem.ProviderIDs(), "every directory the config's content.detect names")
		}
		view := profile.Layout().Apply(fileSystem)
		for _, required := range requiredProducts {
			if _, err := view.Stat(required); err != nil {
				return diagnostic(fmt.Sprintf("mod %s@%s would not start: required content is missing", meta.ID, meta.Version), required, fileSystem.ProviderIDs(), "a mounted archive or loose file supplying it")
			}
		}
		if _, err := content.LoadTranslationTable(view, "english"); err != nil {
			return diagnostic(fmt.Sprintf("mod %s@%s would not start: the translation table does not load: %v", meta.ID, meta.Version, err), "gamedata/translate.tdf", fileSystem.ProviderIDs(), "a readable translation table or none")
		}
		return nil
	}
}

// MissingRequirements lists the mod's `requires` paths that the base install
// does not resolve, in the metadata's order (§4.2). A mod with any is listed
// but cannot be selected. base is the base install's mounted overlay, without
// the mod.
func MissingRequirements(base vfs.FSOps, meta Metadata) []string {
	var missing []string
	for _, required := range meta.Requires {
		if base == nil {
			missing = append(missing, required)
			continue
		}
		if _, err := base.Stat(required); err != nil {
			missing = append(missing, required)
		}
	}
	return missing
}
