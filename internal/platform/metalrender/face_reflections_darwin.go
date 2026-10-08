//go:build darwin

package metalrender

import _ "embed"

// Concatenate after facesShaderSource: the ordered budget pass reads the same
// prepared face records; no pose/corner upload or CPU flag readback is needed.
//
//go:embed shaders/face_reflections.metal
var faceReflectionsShaderSource string
