package gpurender

import "github.com/nanolathe-gg/nanolathe/internal/drawlist"

// Numeric digits extend the existing flat-index/glint lane; this is not a
// float bitcast. The low 16 bits retain index/glint, followed by two material
// bits and three response bits. All corners receive the same integer (21 bits
// maximum). Coefficients are reviewed artistic choices (GPU design §29).
func (r *Renderer) modelFinishColor(d *modelPlaceCtx, f *drawlist.ModelFace, base float32) float32 {
	if !r.materialsEnabled || f.Material == drawlist.ModelMaterialDefault || f.Material > drawlist.ModelMaterialPaint {
		return base
	}
	d.stats.MaterialFaces++
	response := uint32(min(max(f.Normal[0]*-0.35+f.Normal[1]*-0.15+f.Normal[2]*0.9246621, 0), 1)*7 + 0.5)
	return base + float32(uint32(f.Material)+response*4)*65536
}

const modelFinishShaderSource = `
func modelFinish(albedo vec3, lit vec3, finish float) vec3 {
 if finish < 0.5 { return lit }
 material := mod(finish, 4.0)
 response := floor(finish/4.0)/7.0
 if material > 0.5 && material < 1.5 {
  // Brushed steel: a broad reflection anchored at an overhead face (response
  // 6/7), which keeps its palette colour exactly. Faces turned toward the key
  // brighten and faces turned away darken, so the finish moves light rather
  // than adding it. Metal reflects its own colour, so the gain scales the
  // colour and keeps hue and saturation; only a neutral steel highlight takes
  // a cool cast (GPU design §29).
  lobe := response*response
  lobe *= lobe
  peak := max(albedo.r, max(albedo.g, albedo.b))
  low := min(albedo.r, min(albedo.g, albedo.b))
  saturation := (peak-low)/max(peak, 0.001)
  over := lobe - 0.5397751
  // Above the anchor the neutral-steel highlight is the old cool sky colour
  // (0.72, 0.84, 1.0) scaled to unit luminance: it tints the brightening
  // without adding to it.
  sky := mix(vec3(1.0), vec3(0.876, 1.021, 1.216), (1.0-smoothstep(0.2, 0.65, saturation))*step(0.0, over))
  lit = lit*(vec3(1.0)+0.42*over*sky)
 } else if material > 1.5 {
  // Paint: a weak response anchored the same way, and a faint white sheen
  // only on faces turned toward the key; authored hue remains legible.
  over := response*response - 0.7346939
  lit = lit*(1.0+0.12*over) + vec3(0.075)*max(over, 0.0)
 }
 return min(lit, vec3(1.0))
}
`
