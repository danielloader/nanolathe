package client

import "github.com/nanolathe-gg/nanolathe/internal/sim/numeric"

// smoothModelProjection confines late rounding to Enhanced geometry recording.
// This includes the modern-only RecordGeometry preview. Software composition
// and geometry without the Enhanced recording flags retain the retail vertex
// path [03 R-RAST-01 §2]. This is presentation policy, DESIGN_GPU_RENDERER §22.
func (c *Client) smoothModelProjection() bool {
	return c != nil && c.enhanced && c.geometryOnlyModels && c.recordModelGeometry
}

// smoothModelLocalVertex retains transformed fixed fractions through the Z
// mirror, height shear and view scale, then floors once at each raster's own
// resolution. The doubled corners are independent of the native rounding, so
// an interpolated piece can move half a screen pixel (DESIGN_GPU_RENDERER
// §13.5, §22). The height key is deliberately computed separately by callers.
func (c *Client) smoothModelLocalVertex(v, origin [3]numeric.Fixed) (x, y, x2, y2 int32) {
	s := int64(c.modelScale().Px(1))
	rx := int64(v[0] - origin[0])
	ry := int64(v[1] - origin[1])
	rz := int64(v[2] - origin[2])
	x = int32(s * rx >> 16)
	y = int32(s * (-2*rz - ry) >> 17)
	x2 = int32(s * rx >> 15)
	y2 = int32(s * (-2*rz - ry) >> 16)
	return
}
