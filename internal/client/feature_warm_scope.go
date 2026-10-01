package client

import (
	"strings"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/content"
)

// WarmBattleFeatureSequences prepares feature pixels for every definition the
// composed battle can admit. admitted includes the terrain table after mission
// placement and restore. All catalog unit corpses remain candidates regardless
// of current unit presence or build restrictions; subsequent deaths, reclaim,
// burning and reproduction stay inside the resulting closure [05 R-FEAT-01
// §2][05 R-FEAT-01 §5][05 R-FEAT-01 §10][06 §12.1].
//
// This is only a presentation loading policy. It does not alter content.SimArt,
// definition admission order, or any authoritative state [I6].
// An explicit settings-preview registry restricts corpse roots to its fixture's
// unit scope and shares decoded banks through that content set's assets; terrain
// roots and every successor remain included (DESIGN_INTERFACE_HUD_INPUT §3.17).
func (c *Client) WarmBattleFeatureSequences(cat *content.Catalog, admitted []*content.FeatureDef) {
	if c == nil {
		return
	}
	var unitNames []string
	if c.modelTextures != nil {
		unitNames = c.modelTextures.previewUnitNames
	}
	var defs []*content.FeatureDef
	if unitNames == nil {
		defs = battleFeatureDefinitions(cat, admitted)
	} else {
		defs = battleFeatureDefinitionsForUnits(cat, admitted, unitNames)
	}
	c.battleAdmittedFeatures = admitted
	// Whole-catalog event warming could incidentally load a bank used here
	// only for rest art. Keep every reachable rest/shadow bank ready too, so
	// narrowing the event set cannot move those decodes into Draw.
	for _, def := range defs {
		if c.modelTextures != nil && c.modelTextures.previewAssets != nil {
			c.warmPreviewFeatureBank(c.modelTextures.previewAssets, def)
		}
		if def.SeqName != "" || def.SeqNameShad != "" {
			_, _ = c.featureGAFFor(def.Filename)
		}
	}
	c.warmFeatureSequences(defs)
}

func battleFeatureDefinitions(cat *content.Catalog, admitted []*content.FeatureDef) []*content.FeatureDef {
	return battleFeatureDefinitionsForUnits(cat, admitted, nil)
}

func battleFeatureDefinitionsForUnits(cat *content.Catalog, admitted []*content.FeatureDef, unitNames []string) []*content.FeatureDef {
	var defs []*content.FeatureDef
	seen := make(map[*content.FeatureDef]bool)
	add := func(def *content.FeatureDef) {
		if def != nil && !seen[def] {
			seen[def] = true
			defs = append(defs, def)
		}
	}
	byName := func(name string) *content.FeatureDef {
		if cat == nil || name == "" {
			return nil
		}
		return cat.Features[content.CanonicalKey(name)]
	}
	for _, def := range admitted {
		add(def)
	}
	if cat != nil {
		for _, unit := range scopedUnitRecords(cat, unitNames) {
			if unit != nil {
				add(byName(unit.Corpse))
			}
		}
	}
	// Follow the growing list through every successor, including cycles and
	// definitions which have no pixels themselves. Resolved pointers take
	// precedence, matching the feature/model admission paths.
	for i := 0; i < len(defs); i++ {
		def := defs[i]
		for _, link := range [...]struct {
			def  *content.FeatureDef
			name string
		}{
			{def.FeatureDeadDef, def.FeatureDead},
			{def.FeatureReclamateDef, def.FeatureReclamate},
			{def.FeatureBurntDef, def.FeatureBurnt},
		} {
			if link.def != nil {
				add(link.def)
			} else {
				add(byName(link.name))
			}
		}
	}
	return defs
}

func (c *Client) warmPreviewFeatureBank(assets *PreviewModelTextureAssets, def *content.FeatureDef) {
	if def == nil || def.Filename == "" {
		return
	}
	if def.SeqName == "" && def.SeqNameShad == "" && def.SeqNameBurn == "" && def.SeqNameBurnShad == "" &&
		def.SeqNameDie == "" && def.SeqNameDieShad == "" && def.SeqNameReclamate == "" && def.SeqNameReclamateShad == "" {
		return
	}
	key := strings.ToLower(strings.TrimSpace(def.Filename))
	if key == "" {
		return
	}
	if _, ok := c.featureGAFs[key]; ok {
		return
	}
	if _, ok := c.featureGACErr[key]; ok {
		return
	}
	bank := assets.featureBank(key)
	if bank.err != nil {
		c.featureGACErr[key] = bank.err
		return
	}
	c.featureGAFs[key] = bank.gaf
	c.indexDetailBank(key, bank.gaf)
}

// BattleSpriteFrames lists, each once, the rest and shadow frames at the
// current view scale of every feature definition the battle's terrain admits,
// for the executor to place at the loading boundary (DESIGN_GPU_RENDERER
// §14.8). Placed lazily instead, the first sight of a stretch of map — a camera
// jump — placed and uploaded dozens of sprites inside one frame. Event
// sequences (burning, dying, reclaim) and unit corpses appear a few at a time
// during play and are left to first use: across the catalog they are most of
// the art, 61 megapixels on one stock map against 2 for the map's own rest
// frames. Presentation only [I6].
func (c *Client) BattleSpriteFrames() []*formats.GAFFrame {
	if c == nil {
		return nil
	}
	var out []*formats.GAFFrame
	seen := make(map[*formats.GAFFrame]bool)
	for _, def := range c.battleAdmittedFeatures {
		if def == nil || def.Filename == "" {
			continue
		}
		gaf, err := c.featureGAFFor(def.Filename)
		if err != nil || gaf == nil {
			continue
		}
		for _, seq := range [...]string{def.SeqName, def.SeqNameShad} {
			if seq == "" {
				continue
			}
			entry, ok := gaf.Find(seq)
			if !ok || entry == nil {
				continue
			}
			for _, ref := range entry.Frames {
				f := c.viewFrame(ref.Frame)
				if f != nil && !seen[f] {
					seen[f] = true
					out = append(out, f)
				}
			}
		}
	}
	return out
}
