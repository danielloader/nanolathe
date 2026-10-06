package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"image/png"

	"github.com/nanolathe-gg/nanolathe/internal/client"
	"github.com/nanolathe-gg/nanolathe/internal/palette"
)

const survivalAttackerSkinName = "survival-attacker"

// Original transparent infection art, already reduced to the compositor's
// 64×64 sampling tile; no retail or mod pixels are embedded (art/README.md).
//
//go:embed art/infection-overlay-tile.png
var infectionOverlayPNG []byte

// prepareAttackerSkin composites the active content's textures during detached
// preparation. Survival presentation uses the actual attacker owner; the
// director and authoritative state never consume these pixels (DESIGN_SURVIVAL §5.2).
func (b *battleSession) prepareAttackerSkin(pal *palette.Tables) error {
	if b == nil || b.sess == nil {
		return nil
	}
	owner, ok := b.sess.SurvivalAttacker()
	if !ok {
		return nil
	}
	var overrides *client.ModelSkin
	var err error
	if b.cat != nil && b.cat.SurvivalRoster != nil && b.cat.SurvivalRoster.AttackerSkin != "" {
		overrides, err = b.modelTextures.PrepareModelSkin(survivalAttackerSkinName, b.cat.SurvivalRoster.AttackerSkin)
		if err != nil {
			return err
		}
	}
	overlay, err := png.Decode(bytes.NewReader(infectionOverlayPNG))
	if err != nil {
		return fmt.Errorf("nanolathe: infection overlay decode failed: logical path art/infection-overlay-tile.png, providers searched [embedded original art], expected 64×64 RGBA tile: %w", err)
	}
	// A tile-sized image is read without resampling (DESIGN_GPU_RENDERER §38).
	skin, err := b.modelTextures.PrepareInfectedModelSkin(survivalAttackerSkinName, pal, overlay, overrides)
	if err != nil {
		return err
	}
	b.attackerSkin, b.attackerSkinOwner = skin, uint8(owner)
	return nil
}

// installAttackerSkin runs on the render thread after the model registry is
// installed. Preparation has already validated the bank, name and owner slot;
// there is no file access or authoritative writer in this commit step.
func (b *battleSession) installAttackerSkin(cl *client.Client) {
	cl.ClearModelSkins()
	if b == nil || b.attackerSkin == nil {
		return
	}
	cl.InstallPreparedModelSkin(b.attackerSkin)
	// The installed constant name and session-owned slot are valid by construction.
	_ = cl.SetOwnerModelSkin(b.attackerSkinOwner, survivalAttackerSkinName)
}
