package meshscene

// NativeEvent preserves pointer/key edge order between native event pumps.
// Coordinates are top-left-origin target pixels; modifiers are Shift=1,
// Control=2, Alt=4, Command=8. Buttons are left=0, right=1, middle=2.
// Kinds: move=1, down=2, up=3, key-down=4, key-up=5, modifiers=6,
// translated text=7, focus=8, pinch=9. Key events carry the Cocoa virtual
// keycode in Button; their Key is the native function/character value.
// Pointer down Key is click count. Wheel moves set Key bit4, plus precise1
// and momentum2; Button carries signed zoom notches and WheelX/Y point deltas.
// docs/DESIGN_METAL_RENDERER.md describes the full host mapping.
type NativeEvent struct {
	Kind, Key, Button, Modifiers uint32
	X, Y, WheelX, WheelY         float32
}

// OverlayQuad is a 64-byte display-only quad. Rect is target-pixel x/y/w/h;
// UV is normalized corners; Color is straight RGBA. Params.x selects the
// retained texture: zero OverlayAtlas, one terrain, two fog.
// Params.y selects normal premultiplied blending (0) or destination RGB
// multiplication (1). Overlay runs after fog and never writes world depth.
type OverlayQuad struct{ Rect, UV, Color, Params [4]float32 }
