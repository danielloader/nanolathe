package content

import (
	"errors"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// SurvivalRosterPath is the optional scenario content file (DESIGN_SURVIVAL §5.1).
const SurvivalRosterPath = "gamedata/survival_roster.tdf"

// SurvivalMaxTier is the existing Survival build-tree depth bound.
const SurvivalMaxTier = 16

// SurvivalRosterEntry admits one canonical unit key at an authored wave tier.
type SurvivalRosterEntry struct {
	Unit string
	Tier int
}

// SurvivalRoster is immutable scenario content, independent of gameplay mode.
// Units are sorted by canonical key. AttackerSkin is a presentation-only GAF path.
type SurvivalRoster struct {
	Units            []SurvivalRosterEntry
	AttackerSkin     string
	IncludeBuildTree bool // Extend the active catalog's bound build-tree pool.
}

func survivalRosterError(detail string) error {
	return fmt.Errorf("nanolathe: survival roster %s: logical path %s, providers searched [catalog overlay], expected unique eligible unit keys at tiers 1..%d with a tier-1 ground, hover or amphibious unit that is not an authored infector", detail, SurvivalRosterPath, SurvivalMaxTier)
}

// SurvivalUnitEligible is the shared wave admission predicate (DESIGN_SURVIVAL §5).
func SurvivalUnitEligible(def *UnitDef) bool {
	if def == nil || !def.CanMove || def.BMCode == 0 || def.Builder || def.Commander || def.BuildCostMetal <= 0 {
		return false
	}
	for _, w := range [3]*WeaponDef{def.Weapon1Def, def.Weapon2Def, def.Weapon3Def} {
		if w != nil && w.ID != 0 && !w.Interceptor && !w.ToAirWeapon {
			return true
		}
	}
	return false
}

// ValidateSurvivalRoster checks both compiled and hand-authored catalogs before
// a director consumes their roster. It changes no definitions or build menus.
func (c *Catalog) ValidateSurvivalRoster() error {
	if c == nil || c.SurvivalRoster == nil {
		return nil
	}
	seen := make(map[string]bool)
	tier1 := false
	for _, e := range c.SurvivalRoster.Units {
		if !survivalUnitKey(e.Unit) || CanonicalKey(e.Unit) != e.Unit || seen[e.Unit] || e.Tier < 1 || e.Tier > SurvivalMaxTier {
			return survivalRosterError(fmt.Sprintf("invalid entry %q tier %d", e.Unit, e.Tier))
		}
		seen[e.Unit] = true
		def, _ := c.Unit(e.Unit)
		if !SurvivalUnitEligible(def) {
			return survivalRosterError(fmt.Sprintf("missing or ineligible unit %q", e.Unit))
		}
		// Air and naval can both be disabled by existing scenario options.
		// A starting pool must contain a domain available with both disabled.
		naval := def.Floater || (def.MovementClass == "" && def.MinWaterDepth > 0)
		if mc := c.Movement[CanonicalKey(def.MovementClass)]; mc != nil && mc.MinWaterDepth > 0 {
			naval = true
		}
		// An exclusive roster's opening must also contain an ordinary attacker:
		// Modern plans authored infectors only as support picks, so an
		// infector-only opening would plan empty waves (DESIGN_SURVIVAL §5.1).
		// A build-tree roster's combined pool is checked at Survival entry.
		if e.Tier == 1 && !def.CanFly && (def.CanHover || def.Amphibious || !naval) && (c.SurvivalRoster.IncludeBuildTree || !def.NanolatheInfector) {
			tier1 = true
		}
	}
	if !tier1 {
		return survivalRosterError("has no usable tier-1 pool")
	}
	return nil
}

func survivalUnitKey(key string) bool {
	if len(key) == 0 || len(key) > 31 {
		return false
	}
	for _, c := range key {
		if c < 'a' || c > 'z' {
			if c < '0' || c > '9' {
				if c != '_' && c != '-' {
					return false
				}
			}
		}
	}
	return true
}

// CompileSurvivalRoster loads only the optional scenario file. Absent content
// leaves the historical catalog and wave-pool identity unchanged.
func CompileSurvivalRoster(fs vfs.FSOps) (*SurvivalRoster, error) {
	if fs == nil {
		return nil, survivalRosterError("nil filesystem")
	}
	entries, err := fs.ReadDir("gamedata")
	if errors.Is(err, vfs.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, survivalRosterError("discovery failed: " + err.Error())
	}
	found := false
	for _, e := range entries {
		if !e.IsDir && CanonicalKey(e.Path) == SurvivalRosterPath {
			found = true
			break
		}
	}
	if !found {
		return nil, nil
	}
	data, err := fs.ReadFileLimit(SurvivalRosterPath, 1<<20)
	if err != nil {
		return nil, survivalRosterError("read failed: " + err.Error())
	}
	doc, err := formats.ParseTDF(data)
	if err != nil {
		return nil, formats.WithTDFContext(fs, err, SurvivalRosterPath)
	}
	if len(doc.Root.Items) != 1 || doc.Root.Items[0].Kind != formats.NestedSection || CanonicalKey(doc.Root.Items[0].Section.Name) != "survival" {
		return nil, survivalRosterError("requires one SURVIVAL section")
	}
	r := &SurvivalRoster{}
	var unitSection *formats.Section
	skinSeen, treeSeen := false, false
	for _, item := range doc.Root.Items[0].Section.Items {
		if item.Kind == formats.NestedSection {
			if CanonicalKey(item.Section.Name) != "units" || unitSection != nil {
				return nil, survivalRosterError("unknown or repeated section")
			}
			unitSection = item.Section
		} else {
			if CanonicalKey(item.Key) == "includebuildtree" {
				value := strings.TrimSpace(item.Value)
				if treeSeen || (value != "0" && value != "1") {
					return nil, survivalRosterError("invalid or repeated IncludeBuildTree; expected 0 or 1")
				}
				treeSeen, r.IncludeBuildTree = true, value == "1"
				continue
			}
			if CanonicalKey(item.Key) != "attackerskin" || skinSeen {
				return nil, survivalRosterError("unknown or repeated property")
			}
			skinSeen = true
			skin := CanonicalKey(strings.ReplaceAll(strings.TrimSpace(item.Value), "\\", "/"))
			if skin != "" {
				if strings.HasPrefix(skin, "/") || strings.Contains(skin, ":") || path.Clean(skin) != skin || path.Ext(skin) != ".gaf" || strings.HasPrefix(skin, "../") {
					return nil, survivalRosterError("invalid AttackerSkin logical GAF path")
				}
			}
			r.AttackerSkin = skin
		}
	}
	if unitSection == nil {
		return nil, survivalRosterError("missing UNITS section")
	}
	seen := make(map[string]bool)
	for _, item := range unitSection.Items {
		key := CanonicalKey(item.Key)
		if item.Kind != formats.Assignment || !survivalUnitKey(key) || seen[key] {
			return nil, survivalRosterError("invalid or repeated unit key " + key)
		}
		seen[key] = true
		raw := strings.TrimSpace(item.Value)
		tier, err := strconv.Atoi(raw)
		if err != nil || tier < 1 || tier > SurvivalMaxTier || strconv.Itoa(tier) != raw {
			return nil, survivalRosterError("invalid tier for " + key)
		}
		r.Units = append(r.Units, SurvivalRosterEntry{Unit: key, Tier: tier})
	}
	sort.Slice(r.Units, func(i, j int) bool { return r.Units[i].Unit < r.Units[j].Unit })
	return r, nil
}
