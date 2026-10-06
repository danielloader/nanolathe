package client

// Composed unit-viewer previews: a second model attached to the previewed
// model, such as a factory's product on its build pad, recorded into the same
// projected record (DESIGN_GPU_RENDERER §22.5, DESIGN_DEVELOPER_TOOLS §7).
//
// The attachment reuses the production model path end to end: the loaded
// model, piece transforms, material walk, team colour, shading, the nanoframe
// reveal bands, pulse colours and height keys. What is the viewer's own is the
// frame it is placed in. A battle product is a separate unit whose position
// and orientation the simulation copies from its factory every tick
// [04 R-FAC-02 §2]; the viewer instead composes the attachment under the
// parent's root piece, so the orbit turns factory and product as one rigid
// body and the product stays on its pad at every viewing angle.

import (
	"fmt"
	"slices"
	"strings"

	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	compiledmodel "github.com/nanolathe-gg/nanolathe/internal/model"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	presentationrender "github.com/nanolathe-gg/nanolathe/internal/render"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
)

// ModelPreviewPlacement places an attachment in the parent's root-local model
// space: the frame of the parent's root piece before that piece's own
// transform. Position is the attachment origin in 16.16 model units. Heading,
// Pitch and Bank are added to the attachment's own root angles the way the
// simulation adds a product's orientation, about Y, X and Z respectively
// [03 §2.4] C24. The parent's root transform, including the view orientation
// folded into it, then carries the attachment exactly as it carries the
// parent's own pieces.
type ModelPreviewPlacement struct {
	Position             [3]numeric.Fixed
	Heading, Pitch, Bank uint16
}

// ModelPreviewAttachment is a second model composed into a projected record.
// Model is a logical 3DO name or path, as ModelPreviewOptions.Model is.
// Structure and KeyPlane are the attachment's definition flags, PiecePoses its
// static presentation pose and HiddenPieces its tooling overrides, all with
// the parent's meanings. The attachment takes the parent's Owner.
//
// BuildRemaining is the remaining construction fraction: zero is complete,
// and (0, 1] draws the battle's nanoframe reveal and outline [03 R-P0-19-N]
// [03 R-COMP-01 §3]. NanoframeID and NanoframeTick are the two pulse inputs,
// the unit's own sixteen-bit identifier and the presentation tick.
type ModelPreviewAttachment struct {
	Model          string
	Structure      bool
	KeyPlane       bool
	PiecePoses     []frame.PieceView
	HiddenPieces   []string
	Placement      ModelPreviewPlacement
	BuildRemaining float32
	NanoframeID    uint16
	NanoframeTick  uint32
}

// ModelPreviewPiece names one piece origin for ProjectedPieces: a piece of
// the parent model, or of the attachment when Attachment is set. Names match
// case-insensitively, as PiecePoses do.
type ModelPreviewPiece struct {
	Attachment bool
	Name       string
}

// PiecePlacement returns the root-local placement of piece in model under
// the static pose poses: where a factory's QueryBuildInfo piece carries its
// product. The position is the shared piece locator's chain below the root,
// the same composition the battle hangs a product from [04 R-REV-02]. The
// angles are the piece's own runtime turns, which the battle adds to the
// factory orientation without folding any ancestor [04 R-FAC-02 §2]. The
// root piece itself places the attachment at the root origin with no turn.
//
// The battle orientation copy adds only the unit orientation to the hang
// piece's turns, while composing under the root piece also applies any script
// turn of the root itself. No stock factory script turns, spins or snaps its
// root piece (DESIGN_GPU_RENDERER §22.5 records the census), so stock
// factories differ only by rounding; content that does turn its root carries
// its product along that turn in the viewer.
//
// The caller resolves a COB QueryBuildInfo piece index to its name. A
// product hung from index 128 or above sits at the factory origin
// [04 R-FAC-02 §1]; the caller must not place such a product on a piece. A
// floater product's sea-level clamp [04 R-FAC-02 §2] has no water to clamp
// to in an isolated preview and is not applied.
func (r *ModelPreviewRenderer) PiecePlacement(model string, poses []frame.PieceView, piece string) (ModelPreviewPlacement, error) {
	if r == nil || r.client == nil {
		return ModelPreviewPlacement{}, fmt.Errorf("nanolathe: placing preview attachment: renderer is not initialized")
	}
	c := r.client
	name := previewRenderName(model)
	m := c.modelForUnit(frame.UnitView{Model: name})
	if name == "" || m == nil || m.compiled == nil {
		return ModelPreviewPlacement{}, fmt.Errorf("nanolathe: placing preview attachment: logical path %s, providers searched %s, expected drawable 3DO model", name, previewProviders(c.modelFS))
	}
	index, ok := previewPieceIndex(c, m, piece)
	if !ok {
		return ModelPreviewPlacement{}, fmt.Errorf("nanolathe: placing preview attachment: logical path %s, providers searched [model pieces], expected piece %q", name, piece)
	}
	cm := m.compiled
	root := cm.Root
	if index == root {
		return ModelPreviewPlacement{}, nil
	}
	if !previewDescendsFrom(cm, index, root) {
		return ModelPreviewPlacement{}, fmt.Errorf("nanolathe: placing preview attachment: logical path %s, providers searched [model hierarchy], expected piece %q below the root", name, piece)
	}
	states := slices.Clone(c.modelStates(m, poses))
	// A root with neither turn nor offset contributes only its authored
	// translation, which whole 16.16 words remove exactly; the remaining sum
	// is the chain below the root, rounded as the full composition rounds it.
	states[root] = compiledmodel.PieceState{}
	origin := compiledmodel.Compose(cm, states, index).Origin
	t := cm.Pieces[root].Translate
	own := states[index]
	return ModelPreviewPlacement{
		Position: [3]numeric.Fixed{origin[0].Sub(t[0]), origin[1].Sub(t[1]), origin[2].Sub(t[2])},
		Heading:  own.RotY, Pitch: own.RotX, Bank: own.RotZ,
	}, nil
}

func previewPieceIndex(c *Client, m *unitModel, name string) (int, bool) {
	if m == nil || m.compiled == nil {
		return 0, false
	}
	index, ok := m.pieceByName[c.modelNameKey(name)]
	return index, ok && index >= 0 && index < len(m.compiled.Pieces)
}

// previewDescendsFrom reports whether ancestor is on piece's parent chain,
// bounded so a malformed cycle cannot loop.
func previewDescendsFrom(m *compiledmodel.Model, piece, ancestor int) bool {
	for n := 0; piece >= 0 && piece < len(m.Pieces) && n <= len(m.Pieces); n++ {
		if piece == ancestor {
			return true
		}
		piece = m.Pieces[piece].Parent
	}
	return false
}

// previewAttachmentDraws builds the attachment's two draws. The composed draw
// carries final vertices in the parent's frame: its loaded model is grafted
// below a proxy piece holding the parent root's translation and folded state,
// so the shared transform chain applies the parent root to the attachment
// with the same per-axis rounding as every parent vertex [03 §2.4] C21, and
// the shaded renderer's normals see the final orientation. The local draw,
// built only when keys is set, is the battle's own composition of the
// attachment at its placement angles about its origin, whose heights key the
// nanoframe reveal [03 R-P0-19-N].
func (c *Client) previewAttachmentDraws(parent *presentationrender.UnitDraw, view frame.UnitView, a *ModelPreviewAttachment, keys bool) (draw, local *presentationrender.UnitDraw, m *unitModel, err error) {
	name := previewRenderName(a.Model)
	av := frame.UnitView{
		Slot:            pool.Handle(a.NanoframeID),
		InstanceID:      2,
		Owner:           view.Owner,
		OwnerColor:      view.OwnerColor,
		OwnerColorKnown: view.OwnerColorKnown,
		Model:           name,
		X:               view.X, Y: view.Y, Z: view.Z,
		BMCode:         !a.Structure,
		ZBuffer:        a.KeyPlane,
		NoShadow:       true,
		BuildRemaining: a.BuildRemaining,
	}
	av.Pieces = append(av.Pieces, a.PiecePoses...)
	for _, hidden := range a.HiddenPieces {
		if hidden = strings.TrimSpace(hidden); hidden != "" {
			av.Pieces = append(av.Pieces, frame.PieceView{Name: hidden, Hidden: true})
		}
	}
	m = c.modelForUnit(av)
	if m == nil || m.compiled == nil || parent == nil || parent.Model == nil {
		return nil, nil, nil, fmt.Errorf("nanolathe: rendering projected preview: logical path %s, providers searched %s, expected drawable 3DO attachment", name, previewProviders(c.modelFS))
	}
	cm, pm := m.compiled, parent.Model
	n, proot := len(cm.Pieces), pm.Root
	if cm.Root < 0 || cm.Root >= n || proot < 0 || proot >= len(pm.Pieces) || proot >= len(parent.PieceStates) {
		return nil, nil, nil, fmt.Errorf("nanolathe: rendering projected preview: logical path %s, providers searched [model hierarchy], expected attachment and parent roots", name)
	}
	base := slices.Clone(c.modelStates(m, av.Pieces))

	pieces := make([]compiledmodel.Piece, n+1)
	copy(pieces, cm.Pieces)
	pieces[cm.Root].Parent = n
	pieces[n] = compiledmodel.Piece{Parent: -1, Children: []int{cm.Root}, Translate: pm.Pieces[proot].Translate}
	grafted := &compiledmodel.Model{Pieces: pieces, Root: n, Name: cm.Name, Hash: cm.Hash}
	states := make([]compiledmodel.PieceState, n+1)
	copy(states, base)
	compiledmodel.FoldRootAngles(states, cm.Root, a.Placement.Heading, a.Placement.Pitch, a.Placement.Bank)
	for axis := range states[cm.Root].Trans {
		states[cm.Root].Trans[axis] = states[cm.Root].Trans[axis].Add(a.Placement.Position[axis])
	}
	// The parent's root state already holds the view orientation; only its
	// turns and offset carry over, not its visibility or shading flags.
	rootState := parent.PieceStates[proot]
	states[n] = compiledmodel.PieceState{RotX: rootState.RotX, RotY: rootState.RotY, RotZ: rootState.RotZ, Trans: rootState.Trans}
	draw = presentationrender.BuildUnitDrawInto(grafted, states, 0, 0, 0, av, nil, c.borrowDrawScratch())
	// The proxy has no geometry. Present the loaded model so texture tables,
	// animated cursors and selection plates resolve by their loaded identity.
	draw.Model = cm
	draw.Pieces, draw.Transforms, draw.PieceStates = draw.Pieces[:n], draw.Transforms[:n], draw.PieceStates[:n]
	// The same flags buildUnitDraw derives; a construction fraction keeps
	// every piece in the cached lane [03 R-REN-03A §4].
	draw.Structure = !av.BMCode
	draw.UnderConstruction = av.BuildRemaining > 0
	draw.KeyPlane = av.ZBuffer || av.BuildRemaining > 0
	if keys {
		local = presentationrender.BuildUnitDrawInto(cm, base, a.Placement.Heading, a.Placement.Pitch, a.Placement.Bank, frame.UnitView{BMCode: true}, nil, c.borrowDrawScratch())
	}
	return draw, local, m, nil
}

// projectedPreviewAttachment records the attachment's ordinary packet and its
// precise payload: faces in the parent's frame keyed by the attachment's own
// heights, plus the reveal and outline while it is under construction.
func (c *Client) projectedPreviewAttachment(parent *presentationrender.UnitDraw, view frame.UnitView, a *ModelPreviewAttachment, projection ModelPreviewProjection, anchorX, anchorY int32) (*drawlist.ModelGeometry, *drawlist.ModelPreviewAttachment, error) {
	draw, local, _, err := c.previewAttachmentDraws(parent, view, a, true)
	if err != nil {
		return nil, nil, err
	}
	av := frame.UnitView{Slot: pool.Handle(a.NanoframeID), OwnerColor: view.OwnerColor, OwnerColorKnown: view.OwnerColorKnown, BuildRemaining: a.BuildRemaining}
	projector := modelPreviewProjector{projection: projection}
	polys := c.collectDrawPolysProjection(draw, unitTeamColor(av), 0, modelCursorUnit, presentationrender.PieceLaneAll, false, &projector)
	if projector.err != nil {
		return nil, nil, projector.err
	}
	payload := &drawlist.ModelPreviewAttachment{}
	var g *drawlist.ModelGeometry
	if len(polys) != 0 {
		// A carried child keys its own image from its own origin
		// [03 R-COMP-01 §3]: the key is the corner's height in the local
		// draw, which shares every piece and vertex index with the composed one.
		keys := make([]int32, 0, len(projector.positions))
		for i := range polys {
			p := &polys[i]
			corners := draw.Pieces[p.piece].Primitives[p.primitive].VertexIndices
			heights := local.Pieces[p.piece].WorldVertices
			for _, vi := range corners {
				if int(vi) >= len(heights) {
					return nil, nil, fmt.Errorf("nanolathe: rendering projected preview: logical path %s, providers searched [attachment pieces], expected matching local corners", draw.Model.Name)
				}
				keys = append(keys, modelHeightKey(heights[vi][1], false))
			}
		}
		width, height, originX, originY := directModelExtent(polys)
		supersample := c.modelSupersampleGeometry(polys, draw.KeyPlane, width, height, doubledPlacement{originX: originX, originY: originY, exact: true})
		placeFaces(polys, originX, originY, 1)
		g = c.borrowModelPacket(polys, int32(width), int32(height), originX, originY, anchorX, anchorY, 1, draw.KeyPlane, drawlist.ModelFallbackNone)
		g.Supersample = supersample
		c.setModelLightingHeight(g, draw)
		payload.Faces = projector.geometry(g.Faces, anchorX, anchorY).Faces
		k := 0
		for i := range payload.Faces {
			vertices := payload.Faces[i].Face.Vertices
			for j := range vertices {
				vertices[j].Key = keys[k]
				k++
			}
		}
	}
	if a.BuildRemaining > 0 {
		// The battle's own reveal and pulse colours, at the supplied tick and
		// identifier, including any host team-colour mapping [03 R-P0-19-N].
		previousTick := c.frameTick
		c.frameTick = a.NanoframeTick
		reveal, outline := c.unitNanoframeReveal(av)
		c.frameTick = previousTick
		payload.Reveal = &drawlist.ModelReveal{Line: reveal.Line, Floor: reveal.Floor, Below: reveal.Below, Band: reveal.Band, Above: reveal.Above}
		if payload.Outline, err = previewOutline(draw, projector, outline, anchorX, anchorY); err != nil {
			return nil, nil, err
		}
	}
	return g, payload, nil
}
