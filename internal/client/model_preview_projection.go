package client

import (
	"fmt"
	"math"

	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
)

// ModelPreviewProjection is the isolated viewer's presentation policy
// (DESIGN_DEVELOPER_TOOLS §7). PixelsPerUnit measures final raster pixels per
// model unit. Pivot is an already-oriented model-relative point; it affects
// screen placement only, never source heights, shading or depth keys.
type ModelPreviewProjection struct {
	PixelsPerUnit float64
	Pivot         [3]numeric.Fixed
}

// RecordProjectedGeometry records a complete isolated model with transformed
// vertex fractions retained until projection at the final raster scale. Its
// Image is nil. Native and doubled corners are rounded independently, so the
// GPU can resolve subpixel coverage without magnifying game-pixel rounding.
//
// Scale must be zero: PixelsPerUnit supplies the entire projection scale.
// Construction, attached children, cloak, digger and waterline options are
// unsupported. RecordModel and RecordGeometry retain those ordinary contracts
// and the retail projection arithmetic [03 R-RAST-01 §2].
func (r *ModelPreviewRenderer) RecordProjectedGeometry(opts ModelPreviewOptions, projection ModelPreviewProjection) (ModelPreviewRecord, error) {
	if !isFinitePositive(projection.PixelsPerUnit) {
		return ModelPreviewRecord{}, fmt.Errorf("nanolathe: rendering projected preview: logical path %s, providers searched [preview projection], expected finite positive pixels per unit", opts.Model)
	}
	if opts.Scale != 0 {
		return ModelPreviewRecord{}, fmt.Errorf("nanolathe: rendering projected preview: logical path %s, providers searched [preview options], expected default Scale with PixelsPerUnit", opts.Model)
	}
	if len(opts.Children) != 0 || opts.BuildRemaining != 0 || opts.Cloaked || opts.Digger || opts.WorldHeight != 0 || opts.UnderwaterExempt {
		return ModelPreviewRecord{}, fmt.Errorf("nanolathe: rendering projected preview: logical path %s, providers searched [preview options], expected complete isolated model without construction, children, cloak, digger or waterline options", opts.Model)
	}
	return r.recordModel(opts, true, &projection)
}

func isFinitePositive(v float64) bool { return v > 0 && !math.IsInf(v, 0) }

type modelPreviewProjector struct {
	projection ModelPreviewProjection
	err        error
}

func (p *modelPreviewProjector) vertex(v, world [3]numeric.Fixed) (x, y, x2, y2 int32) {
	// Preserve the transformed fixed-point fractions and combine the height
	// shear before rounding. The model Z mirror remains the shared projection
	// convention [03 §2.5]; late rounding is tool-only host policy, §7.
	sx := (float64(v[0]-world[0]) - float64(p.projection.Pivot[0])) / 65536 * p.projection.PixelsPerUnit
	sy := (-(float64(v[2]-world[2]) - float64(p.projection.Pivot[2])) - (float64(v[1]-world[1])-float64(p.projection.Pivot[1]))/2) / 65536 * p.projection.PixelsPerUnit
	// Leave room for doubled coordinates, either bound's origin and margins.
	// A malformed scale must fail before an out-of-range float-to-int cast.
	const limit = float64(math.MaxInt32 / 8)
	if math.IsNaN(sx) || math.IsNaN(sy) || math.Abs(sx) > limit || math.Abs(sy) > limit {
		if p.err == nil {
			p.err = fmt.Errorf("nanolathe: rendering projected preview: logical path <model vertices>, providers searched [preview projection], expected representable raster coordinates")
		}
		return 0, 0, 0, 0
	}
	return int32(math.Floor(sx)), int32(math.Floor(sy)), int32(math.Floor(2 * sx)), int32(math.Floor(2 * sy))
}
