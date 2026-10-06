package client

import (
	"fmt"
	"path"
	"strings"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// ModelSkin is an immutable, palette-indexed replacement bank. It changes only
// unit-face pixels; models and their phase-7 bindings remain shared.
// Nanolathe presentation policy: DESIGN_GPU_RENDERER "Unit model skins".
type ModelSkin struct {
	name   string
	frames map[string]*formats.GAFFrame
	// generated is keyed by the selected source frame, after phase-7 selection.
	// A bank prepared for another registry cannot replace unrelated storage.
	generated map[string]map[*formats.GAFFrame]*formats.GAFFrame
	flat      *[256]byte
}

func modelSkinError(fs *vfs.FS, logicalPath, reason string) error {
	var providers []string
	if fs != nil {
		providers = fs.ProviderIDs()
	}
	return fmt.Errorf("nanolathe: %s: logical path %s, providers searched [%s], expected static indexed model skin", reason, logicalPath, strings.Join(providers, ", "))
}

func prepareModelSkin(fs *vfs.FS, resolve func(string) (texRef, bool), name, logicalGAFPath string) (*ModelSkin, error) {
	fail := func(reason string) (*ModelSkin, error) { return nil, modelSkinError(fs, logicalGAFPath, reason) }
	if strings.TrimSpace(name) == "" {
		return fail("empty model skin name")
	}
	logicalGAFPath = strings.ToLower(strings.ReplaceAll(logicalGAFPath, `\`, "/"))
	clean := path.Clean(logicalGAFPath)
	if logicalGAFPath != clean || strings.HasPrefix(clean, "/") || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "textures/") || path.Ext(clean) != ".gaf" {
		return fail("model skin requires a logical GAF path outside textures")
	}
	if fs == nil {
		return fail("model skin filesystem is absent")
	}
	bank, err := formats.LoadGAFFile(fs, clean)
	if err != nil {
		return fail("load model skin: " + err.Error())
	}
	if len(bank.Entries) == 0 {
		return fail("model skin bank is empty")
	}
	skin := &ModelSkin{name: name, frames: make(map[string]*formats.GAFFrame, len(bank.Entries))}
	for i := range bank.Entries {
		entry := &bank.Entries[i]
		key := strings.ToLower(entry.Name)
		if key == "" || skin.frames[key] != nil {
			return fail("empty or duplicate model skin entry " + entry.Name)
		}
		if entry.FrameCount != 1 || len(entry.Frames) != 1 || entry.Frames[0].Frame == nil {
			return fail("model skin entry is not static: " + entry.Name)
		}
		f := entry.Frames[0].Frame
		if f.Width == 0 || f.Height == 0 || len(f.Pixels) != int(f.Width)*int(f.Height) || f.SubframeCount != 0 || len(f.Subframes) != 0 || f.AlternateBlitter != 0 {
			return fail("model skin entry is not a nonempty indexed leaf: " + entry.Name)
		}
		if ref, ok := resolve(key); ok && ref.kind != texStatic {
			return fail("model skin entry overrides a team or animated texture: " + entry.Name)
		}
		skin.frames[key] = f
	}
	return skin, nil
}

// PrepareModelSkin validates a detached bank against this battle's texture
// classifications without changing its registry, models, or animation cursors.
// Hosts may prepare before battle adoption, then install after binding this
// registry on the client. The returned object may be shared by clients.
func (r *ModelTextureRegistry) PrepareModelSkin(name, logicalGAFPath string) (*ModelSkin, error) {
	if r == nil {
		return nil, modelSkinError(nil, logicalGAFPath, "model texture registry is absent")
	}
	return prepareModelSkin(r.fs, r.resolve, name, logicalGAFPath)
}

// InstallPreparedModelSkin binds an immutable bank without I/O. A replacement
// under the same name is a new cache identity. Call after content/registry
// installation, on the presentation owner between joined frames, never beside
// Record or its workers. Nil and zero-value banks are ignored.
func (c *Client) InstallPreparedModelSkin(skin *ModelSkin) {
	if c == nil || skin == nil || skin.name == "" || len(skin.frames) == 0 && len(skin.generated) == 0 && skin.flat == nil {
		return
	}
	if c.modelSkins == nil {
		c.modelSkins = make(map[string]*ModelSkin)
	}
	c.modelSkins[skin.name] = skin
	c.pausedWorldRevision++
}

// LoadModelSkin loads and validates a static indexed bank from the current
// model filesystem. Failure leaves any previous bank and mappings unchanged.
// Like all skin setters, call on the presentation owner between joined frames.
func (c *Client) LoadModelSkin(name, logicalGAFPath string) error {
	if c == nil {
		return modelSkinError(nil, logicalGAFPath, "model skin client is absent")
	}
	skin, err := prepareModelSkin(c.modelFS, c.resolveModelTexture, name, logicalGAFPath)
	if err != nil {
		return err
	}
	c.InstallPreparedModelSkin(skin)
	return nil
}

// SetOwnerModelSkin selects a default for units whose current committed owner
// matches owner. Empty clears that default; unknown names leave it unchanged.
func (c *Client) SetOwnerModelSkin(owner uint8, name string) error {
	if c == nil {
		return modelSkinError(nil, name, "model skin client is absent")
	}
	if name != "" && c.modelSkins[name] == nil {
		return modelSkinError(c.modelFS, name, "unknown model skin")
	}
	if c.ownerModelSkins == nil {
		c.ownerModelSkins = make(map[uint8]string)
	}
	if name == "" {
		delete(c.ownerModelSkins, owner)
	} else {
		c.ownerModelSkins[owner] = name
	}
	c.pausedWorldRevision++
	return nil
}

// SetUnitModelSkin selects an override by frame.UnitView.InstanceID. Zero is
// rejected: retail pool slots can be reused and are not a safe substitute.
// Empty removes the override and exposes the current owner's default.
func (c *Client) SetUnitModelSkin(instanceID uint64, name string) error {
	if c == nil {
		return modelSkinError(nil, name, "model skin client is absent")
	}
	if instanceID == 0 {
		return modelSkinError(c.modelFS, name, "model skin instance identity is zero")
	}
	if name != "" && c.modelSkins[name] == nil {
		return modelSkinError(c.modelFS, name, "unknown model skin")
	}
	if c.unitModelSkins == nil {
		c.unitModelSkins = make(map[uint64]string)
	}
	if name == "" {
		delete(c.unitModelSkins, instanceID)
	} else {
		c.unitModelSkins[instanceID] = name
	}
	c.pausedWorldRevision++
	return nil
}

// ClearModelSkins releases all banks and selections at battle/content teardown.
// Existing recorded commands retain their immutable frame pointers. New draws
// rebuild any retained body whose selected bank identity no longer matches.
func (c *Client) ClearModelSkins() {
	if c == nil {
		return
	}
	c.modelSkins, c.ownerModelSkins, c.unitModelSkins = nil, nil, nil
	c.pausedWorldRevision++
}

func (c *Client) selectedModelSkin(instanceID uint64, owner uint8) *ModelSkin {
	if c == nil {
		return nil
	}
	if instanceID != 0 {
		if name := c.unitModelSkins[instanceID]; name != "" {
			return c.modelSkins[name]
		}
	}
	return c.modelSkins[c.ownerModelSkins[owner]]
}
