package main

import (
	"fmt"
	"math"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/nanolathe-gg/nanolathe/internal/client"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/model"
	"github.com/nanolathe-gg/nanolathe/internal/platform/gpurender"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// The viewer projects detached production geometry at its final raster scale.
// Its frozen Create-pose pivot keeps framing independent of changing bounds.
// No battle cache or source vertex is changed (DESIGN_DEVELOPER_TOOLS §7).
type unitViewerModel struct {
	preview  *client.ModelPreviewRenderer
	gpu      *gpurender.Renderer
	image    *ebiten.Image
	err      error
	radius   float64
	pivot    [3]numeric.Fixed // root-local center of the frozen Create fit
	fitRoot  model.PieceState
	key      unitViewerModelKey
	poses    []frame.PieceView
	poseNote string
	def      *content.UnitDef
	geometry *model.Model
	anim     *unitViewerAnimation
	action   unitViewerAction
	weapon   int
}

type unitViewerModelKey struct {
	def        *content.UnitDef
	yaw, pitch uint16
	zoom       float64
	w, h       int
}

func (m *unitViewerModel) selectUnit() {
	m.err, m.radius, m.image, m.key = nil, 0, nil, unitViewerModelKey{}
	m.pivot, m.fitRoot = [3]numeric.Fixed{}, model.PieceState{}
	m.poses, m.poseNote = nil, ""
	m.def, m.geometry, m.anim = nil, nil, nil
	m.action, m.weapon = unitViewerIdle, 1
}

func (m *unitViewerModel) release() {
	if m.gpu != nil {
		m.gpu.ResetSources()
	}
	*m = unitViewerModel{}
}

func (m *unitViewerModel) draw(cs *contentSet, def *content.UnitDef, yaw, pitch, zoom float64, w, h int) *ebiten.Image {
	if def == nil || cs == nil || m.err != nil {
		return nil
	}
	key := unitViewerModelKey{def, uint16(int64(yaw)), uint16(int64(pitch)), zoom, w, h}
	if m.image != nil && m.key == key {
		return m.image
	}
	if m.preview == nil {
		m.preview, m.err = client.NewModelPreviewRenderer(cs.unmappedMount, cs.presentation.TeamLogos)
		if m.err != nil {
			return nil
		}
	}
	if m.radius == 0 {
		path := vfs.ResourcePath("objects3d", def.ObjectName, "3do")
		mdl, err := model.Load(cs.unmappedMount, path)
		if err != nil {
			m.err = err
			return nil
		}
		m.loadAnimation(def, mdl)
	}
	heading, tilt, bank := unitViewerOrientation(key.yaw, key.pitch)
	record, w, h, err := m.record(def, heading, tilt, bank, zoom, w, h)
	if err != nil {
		m.err = err
		return nil
	}
	commands := record.List.ModelCommands()
	if m.gpu == nil {
		m.gpu, m.err = gpurender.NewChecked(record.Palette, w, h)
		if m.err != nil {
			return nil
		}
	}
	var list drawlist.List
	list.RecordClear()
	unitViewerBackdrop(&list, record.Palette, w, h)
	for _, cmd := range commands {
		if cmd.ShadowOnly || cmd.Geometry == nil {
			continue
		}
		g := cmd.Geometry
		g.Shadow, g.Cache = nil, drawlist.ModelCacheKey{}
		list.RecordModel(drawlist.Model{Geometry: g})
	}
	m.image = m.gpu.Execute(&list, w, h)
	if m.image == nil {
		m.err = fmt.Errorf("nanolathe: unit viewer rendering failed: logical path objects3d/%s.3do, providers searched [mounted content], expected model surface", def.ObjectName)
		return nil
	}
	m.gpu.Expand()
	m.key = key
	return m.image
}

func (m *unitViewerModel) record(def *content.UnitDef, heading, pitch, bank uint16, zoom float64, w, h int) (client.ModelPreviewRecord, int, int, error) {
	opts := client.ModelPreviewOptions{
		Model: vfs.ResourcePath("objects3d", def.ObjectName, "3do"), Width: w, Height: h,
		Heading: heading, Pitch: pitch, Bank: bank,
		Structure: def.BMCode == 0, KeyPlane: def.ZBuffer, PiecePoses: m.poses,
	}
	// 2*sqrt(1.25) bounds the diameter under the half-height projection
	// [03 §2.5]. Both fit and pivot stay fixed while animations move the body.
	projection := client.ModelPreviewProjection{
		PixelsPerUnit: 0.94 * float64(min(w, h)) / (2 * math.Sqrt(1.25) * m.radius) * zoom,
		Pivot:         m.orientedPivot(heading, pitch, bank),
	}
	record, err := m.preview.RecordProjectedGeometry(opts, projection)
	if err != nil {
		return record, w, h, err
	}
	factor, rw, rh := unitViewerRasterSize(record.List.ModelCommands(), 1, w, h)
	if factor < 1 {
		// Re-project from fixed-point vertices at the reduced raster scale.
		// Scaling rounded packets would reintroduce amplified rounding jumps.
		projection.PixelsPerUnit *= factor
		opts.Width, opts.Height = rw, rh
		record, err = m.preview.RecordProjectedGeometry(opts, projection)
	}
	return record, rw, rh, err
}

// Orbit first, then tilt around the display's horizontal axis. Directly
// passing these as body heading/pitch applies tilt before yaw [03 §2.4], C21,
// which makes a turning model lean from side to side. Decompose the viewer's
// X*Y transform into the production Y*X*Z orientation (host policy, §7).
func unitViewerOrientation(yaw, pitch uint16) (heading, tilt, bank uint16) {
	const radians = 2 * math.Pi / 65536
	sy, cy := math.Sincos(float64(yaw) * radians)
	sp, cp := math.Sincos(float64(pitch) * radians)
	x := math.Asin(max(-1, min(1, sp*cy)))
	y, z := math.Atan2(sy, cp*cy), math.Atan2(-sp*sy, cp)
	if math.Abs(math.Cos(x)) < 1e-8 {
		// At a vertical pole, heading and bank share an axis. Fix bank to
		// zero and retain the equivalent heading, avoiding atan2(0, 0).
		y, z = math.Atan2(cp*sy, cy), 0
	}
	angle := func(a float64) uint16 { return uint16(int64(math.Round(a / radians))) }
	return angle(y), angle(x), angle(z)
}

// Large zoomed subjects must still fit the GPU's 4096-pixel composition
// atlas, including its 2x plane and border (DESIGN_GPU_RENDERER §22.2).
// Reduce geometry and canvas together; the UI's final stretch preserves zoom
// and aspect while avoiding the atlas's painter-order overflow fallback.
func unitViewerRasterSize(commands []drawlist.Model, scale float64, w, h int) (float64, int, int) {
	bound := 1.0
	for _, cmd := range commands {
		g := cmd.Geometry
		if cmd.ShadowOnly || g == nil {
			continue
		}
		bound = max(bound, float64(max(g.Width, g.Height)))
		if ss := g.Supersample; ss != nil {
			bound = max(bound, float64(max(ss.Width, ss.Height))/2)
		}
	}
	// 2000 leaves room for independent integer rounding and atlas padding.
	f := min(1, 2000/(scale*bound))
	return scale * f, max(1, int(float64(w)*f)), max(1, int(float64(h)*f))
}

// This is a presentation fit bound over drawable authored vertices, including
// parent translations. Selection plates and attachment-only points must not
// make the visible body tiny [03 §2.4][03 §2.4.1].
func unitViewerBounds(m *model.Model, states []model.PieceState) (lo, hi [3]float64, found bool) {
	lo = [3]float64{math.Inf(1), math.Inf(1), math.Inf(1)}
	hi = [3]float64{math.Inf(-1), math.Inf(-1), math.Inf(-1)}
	for i, p := range m.Pieces {
		hidden := false
		for ancestor, n := i, 0; ancestor >= 0 && ancestor < len(m.Pieces) && n < len(m.Pieces); ancestor, n = m.Pieces[ancestor].Parent, n+1 {
			if ancestor < len(states) && states[ancestor].Hidden {
				hidden = true
				break
			}
		}
		if hidden {
			continue
		}
		transform := model.Compose(m, states, i)
		for fi, face := range p.Primitives {
			if p.Selection && fi == 0 {
				continue
			}
			if len(face.VertexIndices) < 3 || (face.IsColored&1 == 0 && (len(face.VertexIndices) != 4 || face.TextureName == "")) {
				continue
			}
			for _, vi := range face.VertexIndices {
				if int(vi) >= len(p.Vertices) {
					continue
				}
				v := transform.ApplyVertex(p.Vertices[vi])
				for axis := range v {
					f := float64(v[axis]) / 65536
					lo[axis], hi[axis] = min(lo[axis], f), max(hi[axis], f)
				}
				found = true
			}
		}
	}
	return lo, hi, found
}

func unitViewerRadius(m *model.Model, states []model.PieceState) float64 {
	lo, hi, found := unitViewerBounds(m, states)
	if !found {
		return 1
	}
	x, y, z := hi[0]-lo[0], hi[1]-lo[1], hi[2]-lo[2]
	return max(1, math.Sqrt(x*x+y*y+z*z)/2)
}

// Freeze the reference in root-local coordinates so authored root offsets and
// rotations use the same hierarchy as the body. Neutralizing only the root
// leaves every child's creation pose in the fit; the root's frozen state is
// reapplied with each view's angles, independently of later animation.
func (m *unitViewerModel) fitCreatePose(states []model.PieceState) {
	root := m.geometry.Root
	neutral := slices.Clone(states)
	if len(neutral) < len(m.geometry.Pieces) {
		neutral = append(neutral, make([]model.PieceState, len(m.geometry.Pieces)-len(neutral))...)
	}
	m.fitRoot = neutral[root]
	neutral[root].RotX, neutral[root].RotY, neutral[root].RotZ = 0, 0, 0
	neutral[root].Trans = [3]numeric.Fixed{}
	lo, hi, found := unitViewerBounds(m.geometry, neutral)
	m.pivot = [3]numeric.Fixed{}
	if found {
		for axis := range m.pivot {
			m.pivot[axis] = numeric.Fixed(math.Round((lo[axis]+hi[axis])*32768)) - m.geometry.Pieces[root].Translate[axis]
		}
	}
	m.radius = unitViewerRadius(m.geometry, neutral)
}

func (m *unitViewerModel) orientedPivot(heading, pitch, bank uint16) [3]numeric.Fixed {
	root := m.geometry.Root
	states := make([]model.PieceState, root+1)
	states[root] = m.fitRoot
	model.FoldRootAngles(states, root, heading, pitch, bank)
	return model.Compose(m.geometry, states, root).Apply(m.pivot)
}
