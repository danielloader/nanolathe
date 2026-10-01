package client

import (
	"github.com/nanolathe-gg/nanolathe/formats"
	compiledmodel "github.com/nanolathe-gg/nanolathe/internal/model"
)

// animatedFrame keeps current-frame cursor assertions concise. Production
// composition selects cached or live frames through animatedFrameSelected.
func (r *ModelTextureRegistry) animatedFrame(model *compiledmodel.Model, piece, primitive int, ref texRef) *formats.GAFFrame {
	return r.animatedFrameSelected(model, piece, primitive, ref, false)
}
