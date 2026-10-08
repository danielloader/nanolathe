//go:build darwin

package metalrender

import _ "embed"

//go:embed shaders/water.metal
var waterShaderSource string
