package formats

import (
	"fmt"
	"slices"
)

// CheckpointOTAInputs preserves exact OTA/TDF identities and detached authored
// values for admission validation, without changing the parsed document or
// introducing a wire payload (DESIGN_MULTIPLAYER §16.3.74).
type CheckpointOTAInputs struct {
	captured bool
	owner    *OTA
	values   checkpointOTAValues
	root     *Section
	schemas  []OTASchema
	sections []checkpointOTASection
}

type checkpointOTAValues struct {
	document           *Document
	global             *Section
	missionName        string
	missionDescription string
	memory             string
	numPlayers         string
	size               string
}

func otaCheckpointValues(o *OTA) checkpointOTAValues {
	return checkpointOTAValues{
		document: o.Document, global: o.Global, missionName: o.MissionName,
		missionDescription: o.MissionDescription, memory: o.Memory,
		numPlayers: o.NumPlayers, size: o.Size,
	}
}

type checkpointOTASection struct {
	owner         *Section
	name          string
	originalName  string
	items         []checkpointOTAItem
	resolvedBuilt bool
	resolved      []int32
}

type checkpointOTAItem struct {
	kind        ItemKind
	key         string
	originalKey string
	value       string
	section     *Section
}

func otaCheckpointItem(item Item) checkpointOTAItem {
	return checkpointOTAItem{item.Kind, item.Key, item.OriginalKey, item.Value, item.Section}
}

// SnapshotCheckpointOTAInputs records the actual graph, including duplicate
// assignments and their existing resolved index [fmt tdf "Duplicate keys"].
// It accepts absent optional OTA/document/section edges. A visited-pointer set
// bounds shared or cyclic fixtures; no getters or index resolution run here.
func SnapshotCheckpointOTAInputs(o *OTA) (*CheckpointOTAInputs, error) {
	s := &CheckpointOTAInputs{captured: true, owner: o}
	if o == nil {
		return s, nil
	}
	s.values = otaCheckpointValues(o)
	s.schemas = slices.Clone(o.Schemas)
	if o.Document != nil {
		s.root = o.Document.Root
	}
	seen := make(map[*Section]bool)
	add := func(section *Section) {
		if section == nil || seen[section] {
			return
		}
		seen[section] = true
		row := checkpointOTASection{
			owner: section, name: section.Name, originalName: section.OriginalName,
			resolvedBuilt: section.resolvedBuilt, resolved: slices.Clone(section.resolved),
		}
		if section.Items != nil {
			row.items = make([]checkpointOTAItem, len(section.Items))
			for i, item := range section.Items {
				row.items[i] = otaCheckpointItem(item)
			}
		}
		s.sections = append(s.sections, row)
	}
	add(s.root)
	add(o.Global)
	for _, schema := range s.schemas {
		add(schema.Section)
	}
	// Traverse detached edges in fixed discovery order, including Section
	// edges on non-section Items. Capture later only compares this list; it
	// cannot follow newly inserted edges (DESIGN_MULTIPLAYER §16.3.74).
	for i := 0; i < len(s.sections); i++ {
		for _, item := range s.sections[i].items {
			add(item.section)
		}
	}
	return s, nil
}

// Validate compares stored fields and pointer edges without following the
// current graph or building indexes. Source locations are observations and do
// not participate. Successful validation allocates no storage.
func (s *CheckpointOTAInputs) Validate(o *OTA) error {
	if s == nil || !s.captured || s.owner != o {
		return otaCheckpointInputError("OTA", "the exact snapshotted OTA presence and identity")
	}
	if o == nil {
		return nil
	}
	if s.values != otaCheckpointValues(o) {
		return otaCheckpointInputError("OTA", "unchanged scalar values, Document and Global identities")
	}
	if o.Document != nil && s.root != o.Document.Root {
		return otaCheckpointInputError("OTA.Document.Root", "the snapshotted section identity")
	}
	if (s.schemas == nil) != (o.Schemas == nil) || !slices.Equal(s.schemas, o.Schemas) {
		return otaCheckpointInputError("OTA.Schemas", "unchanged ordered schema values, presence and section identities")
	}
	for i := range s.sections {
		row := &s.sections[i]
		section := row.owner
		if row.name != section.Name || row.originalName != section.OriginalName || row.resolvedBuilt != section.resolvedBuilt {
			return otaCheckpointInputError(fmt.Sprintf("OTA.sections[%d]", i), "unchanged section names and resolved-index state")
		}
		if (row.resolved == nil) != (section.resolved == nil) || !slices.Equal(row.resolved, section.resolved) {
			return otaCheckpointInputError(fmt.Sprintf("OTA.sections[%d].resolved", i), "unchanged resolved index presence and words")
		}
		if (row.items == nil) != (section.Items == nil) || len(row.items) != len(section.Items) {
			return otaCheckpointInputError(fmt.Sprintf("OTA.sections[%d].Items", i), "unchanged item presence and count")
		}
		for j, item := range row.items {
			if item != otaCheckpointItem(section.Items[j]) {
				return otaCheckpointInputError(fmt.Sprintf("OTA.sections[%d].Items[%d]", i, j), "unchanged item values and section identity")
			}
		}
	}
	return nil
}

func otaCheckpointInputError(path, expected string) error {
	return fmt.Errorf("nanolathe: OTA checkpoint input validation failed: logical path %s, providers searched [formats], expected %s", path, expected)
}
