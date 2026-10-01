package gpurender

import (
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
)

// Compiled programs are shared by every Renderer in the process
// (DESIGN_GPU_RENDERER §2.3 "Shared programs"). A Kage program is immutable
// once compiled and no Renderer deallocates one, so a second renderer — each
// settings-screen preview scene, its compare twin, a film or capture — takes
// the first one's programs instead of handing the backend the same sources
// again. The backend compiles each new program on the render thread inside a
// frame, and a renderer carries about thirty; Ebitengine's Direct3D backend
// compiles every new pixel shader with D3DCompile.
var shaderPrograms struct {
	mu   sync.Mutex
	byID map[string]shaderProgram
}

type shaderProgram struct {
	shader *ebiten.Shader
	err    error
}

// compileShader returns the process's program for a Kage source, compiling it
// on first use. A source that fails to compile fails the same way every time,
// so its error is kept with it.
func compileShader(src string) (*ebiten.Shader, error) {
	shaderPrograms.mu.Lock()
	defer shaderPrograms.mu.Unlock()
	if p, ok := shaderPrograms.byID[src]; ok {
		return p.shader, p.err
	}
	shader, err := ebiten.NewShader([]byte(src))
	if shaderPrograms.byID == nil {
		shaderPrograms.byID = make(map[string]shaderProgram)
	}
	shaderPrograms.byID[src] = shaderProgram{shader: shader, err: err}
	return shader, err
}
