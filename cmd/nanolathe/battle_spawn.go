package main

import (
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/session"
)

// spawnChatCommand captures the submission frame's pointer, before TALK's
// input ownership or a later camera movement can change the requested point.
// Nanolathe Modern policy: DESIGN_INTERFACE_HUD_INPUT "Modern spawn command".
func (b *battleSession) spawnChatCommand(words []string) {
	say := func(text string) {
		if ring := b.messageRing(); ring != nil {
			ring.Append(text, 4, 0, 10, b.currentTick())
		}
	}
	if b.sess == nil {
		return
	}
	if b.sess.Gameplay.Normalize() != gameplay.Modern {
		say("+spawn requires Modern gameplay")
		return
	}
	if len(words) != 2 {
		say("Usage: +spawn <unit>, for example +spawn armck")
		return
	}
	b.spawnAtPointer(words[1], "Point at the battlefield before submitting +spawn")
}

// unitNameChatCommand is the Modern shorthand `+<unit>`: a single word that
// names a catalog unit and matched no registered command queues the same
// request as `+spawn <unit>`. Retail reaches a unit-name default handler only
// with developer access. It creates one fully built unit per definition whose
// name matches the word as a wildcard pattern, owned by the slot the second
// word names (slot 0 when absent), stepping 32 world units between footprints,
// and validates no site [07 R-CAM-01 §6][07 R-CAM-01 §9]. This shorthand is
// Nanolathe Modern policy with the `+spawn` contract instead — exact name, one
// unit, local owner, no developer access — and shares only that placement
// (DESIGN_INTERFACE_HUD_INPUT "Modern spawn command"). A word that names no
// unit, carries arguments, or arrives under Strict 3.1 stays plain chat with
// no feedback, as an unregistered retail command does. The retail handler
// itself belongs with developer mode (DESIGN_DEVELOPER_TOOLS) and is not
// implemented.
func (b *battleSession) unitNameChatCommand(words []string) bool {
	if b == nil || b.sess == nil || b.sess.Catalog == nil || len(words) != 1 {
		return false
	}
	if b.sess.Gameplay.Normalize() != gameplay.Modern {
		return false
	}
	if def, ok := b.sess.Catalog.Unit(words[0]); !ok || def == nil {
		return false
	}
	b.spawnAtPointer(words[0], "Point at the battlefield before submitting +"+words[0])
	return true
}

func (b *battleSession) spawnAtPointer(unit, offWorld string) {
	if b.cl == nil || b.cam == nil || b.cl.Input() == nil || b.cl.Input().Mouse == nil {
		return
	}
	mouse := b.cl.Input().Mouse
	x, y := int32(mouse.X), int32(mouse.Y)
	if !b.overWorld(x, y) {
		if ring := b.messageRing(); ring != nil {
			ring.Append(offWorld, 4, 0, 10, b.currentTick())
		}
		return
	}
	wx, wy, wz := b.cursorWorld(x, y)
	_ = b.sess.EnqueueHumanCommand(session.HumanCommand{
		Kind:  session.HumanSpawn,
		Spawn: session.HumanSpawnCommand{Unit: unit, X: wx, Y: wy, Z: wz},
	})
}
