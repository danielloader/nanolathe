//go:build darwin

package metalrender

import _ "embed"

// GlowShaderSource precedes the root shader entry points. The bridge includes
// native/glow.h and glow.inc; neither file owns the renderer/window lifecycle.
//
//go:embed shaders/glow.metal
var GlowShaderSource string
