//go:build !darwin

package metalrender

import (
	"fmt"

	"github.com/nanolathe-gg/nanolathe/internal/meshscene"
)

func Run(_ *meshscene.Scene, _ Options) (map[string]any, error) {
	return nil, fmt.Errorf("metalrender: macOS Metal backend required")
}
