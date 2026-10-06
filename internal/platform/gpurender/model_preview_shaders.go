package gpurender

import "fmt"

// Ebitengine's images store RGBA8, so three conditional MAX passes select the
// high, middle and low depth bytes in order. Maximizing all channels at once
// would combine bytes belonging to different faces. Every pass uses this one
// shader; only the uniform changes (DESIGN_GPU_RENDERER §22.5).
func modelPreviewShaderSource() string { return modelPreviewShaderText(false) }

// modelPreviewComposeShaderSource is the same shader for records with an
// attachment. It adds only the nanoframe key lane and, under the Reveal
// uniform, the battle's reveal verdicts on the attachment's top surface
// [03 R-P0-19-N]: erase leaves the sample uncovered, an index replaces the
// colour unshaded and without finish, keep leaves the composed colour. The
// key is whole height plus bias, wrapped to the byte the battle stores.
func modelPreviewComposeShaderSource() string { return modelPreviewShaderText(true) }

func modelPreviewShaderText(compose bool) string {
	uniforms, keyLane, quadKey, verdict := "", "", "", ""
	if compose {
		uniforms = `var Reveal float
var RevealLine float
var RevealFloor float
var RevealBelow float
var RevealBand float
var RevealAbove float
`
		keyLane = `	key := -color.b-` + fmt.Sprint(modelPreviewKeyOffset) + `.0
`
		quadKey = `		key = floor(lanes.z)
`
		// A lane-less key interpolates one value per corner; the small bias
		// keeps a constant whole key whole through device interpolation.
		verdict = `	if Reveal > 0.5 {
		key = floor(key+1.0/64.0)
		key = key-floor(key/256.0)*256.0
		verdict := RevealBand
		if key < RevealFloor {
			verdict = RevealBelow
		} else if key >= RevealLine {
			verdict = RevealAbove
		}
		if verdict == -2.0 {
			return vec4(0.0)
		}
		if verdict != -1.0 {
			return vec4(imageSrc1AtFromSrc0Pos(imageSrc0Origin()+vec2(verdict+0.5, ` + fmt.Sprint(tableRowPAL) + `.5)).rgb, 1.0)
		}
	}
`
	}
	return `//kage:unit pixels
package main

var Pass float
var DepthTie float
` + uniforms + modelQuadMapperSource + metalGlintShaderSource + modelFinishShaderSource + `
func Fragment(dstPos vec4, srcPos vec2, color vec4, custom vec4) vec4 {
	d := floor(dstPos.xy-imageDstOrigin())
	mode := int(custom.x+0.5)
	encoded := floor(color.g+0.5)
	idx := mod(encoded, 256.0)
	row := color.r
` + keyLane + `	uv := srcPos-imageSrc0Origin()
	if color.b > 0.5 {
		lanes := modelQuadLanes(color.b, d+vec2(` + fmt.Sprint(modelQuadLocalBias) + `.0))
		uv = lanes.xy
		row = lanes.w
` + quadKey + `	} else {
		uv -= vec2(floor(color.a/4096.0), mod(color.a, 4096.0))
	}
	if mode == 1 || mode == 3 {
		slot := vec2(floor(color.a/4096.0), mod(color.a, 4096.0))
		uv = clamp(floor(uv+vec2(1.0/65536.0)), vec2(0.0), custom.zw)
		idx = floor(imageSrc0At(imageSrc0Origin()+slot+uv+vec2(0.5)).r*255.0+0.5)
	}
	// Cutout texels do not write depth either: the surface behind a hole
	// must remain available to the colour pass.
	if idx == 1.0 {
		discard()
		return vec4(0.0)
	}
	depth := clamp(floor(custom.y), 0.0, 16777215.0)
	hi := floor(depth/65536.0)
	mid := mod(floor(depth/256.0), 256.0)
	low := mod(depth, 256.0)
	if Pass < 0.5 {
		return vec4(hi/255.0, 0.0, 0.0, 1.0)
	}
	stored := floor(imageSrc2AtFromSrc0Pos(imageSrc0Origin()+d+vec2(0.5)).rgb*255.0+vec3(0.5))
	if Pass < 2.5 {
		if hi != stored.r || (Pass > 1.5 && mid != stored.g) {
			discard()
			return vec4(0.0)
		}
		if Pass < 1.5 {
			return vec4(hi/255.0, mid/255.0, 0.0, 1.0)
		}
		return vec4(hi, mid, low, 255.0)/255.0
	}
	if depth+DepthTie < dot(stored, vec3(65536.0, 256.0, 1.0)) {
		discard()
		return vec4(0.0)
	}
` + verdict + `	k := 1.0
	if mode >= 2 {
		k = min(0.06875*floor(row), ` + fmt.Sprint(rowScaleMax) + `.0)
	}
	albedo := imageSrc1AtFromSrc0Pos(imageSrc0Origin()+vec2(idx+0.5, ` + fmt.Sprint(tableRowPAL) + `.5)).rgb
	glint := mod(floor(encoded/256.0), 256.0)
	finish := floor(encoded/65536.0)
	return vec4(modelFinish(albedo, metalGlint(albedo, albedo*k, glint), finish), 1.0)
}
`
}

const modelPreviewResolveSource = `//kage:unit pixels
package main

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	d := floor(dstPos.xy-imageDstOrigin())*2.0+imageSrc0Origin()
	a := imageSrc0At(d+vec2(0.5, 0.5))
	b := imageSrc0At(d+vec2(1.5, 0.5))
	c := imageSrc0At(d+vec2(0.5, 1.5))
	e := imageSrc0At(d+vec2(1.5, 1.5))
	return (a+b+c+e)*0.25
}
`

// modelPreviewMergeSource composes the parent's colour and depth (sources 0
// and 1) with the attachment's (2 and 3) per 2x sample, then box-resolves
// the four samples as modelPreviewResolveSource does. A covered attachment
// sample wins unless the parent is deeper by more than the tie band.
const modelPreviewMergeSource = `//kage:unit pixels
package main

var DepthTie float

func previewDepth(v vec4) float {
	return dot(floor(v.rgb*255.0+vec3(0.5)), vec3(65536.0, 256.0, 1.0))
}

func previewSample(p vec2) vec4 {
	attached := imageSrc2AtFromSrc0Pos(p)
	if attached.a > 0.5 && previewDepth(imageSrc3AtFromSrc0Pos(p))+DepthTie >= previewDepth(imageSrc1AtFromSrc0Pos(p)) {
		return attached
	}
	return imageSrc0At(p)
}

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	d := floor(dstPos.xy-imageDstOrigin())*2.0+imageSrc0Origin()
	a := previewSample(d+vec2(0.5, 0.5))
	b := previewSample(d+vec2(1.5, 0.5))
	c := previewSample(d+vec2(0.5, 1.5))
	e := previewSample(d+vec2(1.5, 1.5))
	return (a+b+c+e)*0.25
}
`
