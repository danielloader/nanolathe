package meshscene

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/model"
	"github.com/nanolathe-gg/nanolathe/internal/palette"
	"github.com/nanolathe-gg/nanolathe/internal/session"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// retainBattle shares the immutable asset preparation between the independent
// benchmark host and the production battle host. Only the caller owns ticks.
func retainBattle(opts BattleOptions, fs *vfs.FS, cat *content.Catalog, pal *palette.Tables, sess *session.Session, cx, cz float32, wholeMap bool) (*Scene, *battleSource, error) {
	mapName := battleModelMapName(opts.Map)
	originY := float32(0) // Live geometry retains published world height; screen Y uses z-y/2.

	// Preload the complete mounted objects3d namespace, including otherwise
	// unused projectile and corpse art. A future birth never performs GPU upload.
	paths := []string{}
	seen := map[string]bool{}
	for _, entry := range fs.Entries() {
		path := strings.ToLower(strings.ReplaceAll(entry.Path, `\`, "/"))
		if strings.HasPrefix(path, "objects3d/") && strings.HasSuffix(path, ".3do") && !seen[path] {
			seen[path] = true
			paths = append(paths, entry.Path)
		}
	}
	sort.Slice(paths, func(i, j int) bool { return strings.ToLower(paths[i]) < strings.ToLower(paths[j]) })
	models := []*model.Model{}
	failures := []map[string]any{}
	modelIndices := map[string]int{}
	for _, path := range paths {
		loaded, loadErr := model.Load(fs, path)
		if loadErr != nil {
			failures = append(failures, map[string]any{"logical_path": path, "error": loadErr.Error()})
			continue
		}
		modelIndices[battleModelKey(path)] = len(models)
		models = append(models, loaded)
	}
	if len(models) == 0 {
		return nil, nil, fmt.Errorf("nanolathe: Metal models missing: logical path objects3d, providers searched [mounted content], expected retained 3DO meshes")
	}
	atlas, err := buildAtlas(fs, models, pal, opts.TextureScale, opts.MaterialSource)
	if err != nil {
		return nil, nil, err
	}
	atlas.useMaterials = true
	scene := &Scene{Name: "live ordinary three-army battle", Atlas: atlas.texture, Materials: atlas.materials, TextureFrames: atlas.frames, Camera: [3]float32{cx, cz, opts.Zoom}, Metadata: map[string]any{}}
	census := make([]meshCensus, len(models))
	vertices, indices, edges := 0, 0, 0
	for i, m := range models {
		mesh := compileMesh(m, atlas, pal, opts.QuadMultiplier, &census[i])
		scene.Meshes = append(scene.Meshes, mesh)
		vertices += len(mesh.Vertices)
		indices += len(mesh.Indices)
		edges += len(mesh.EdgeIndices)
	}
	scene.Materials, scene.TextureFrames = atlas.materials, atlas.frames
	scene.ModelPalette = atlas.palette
	tntPath := "maps/" + mapName + ".tnt"
	terrain, err := formats.LoadTNTFile(fs, tntPath)
	if err != nil {
		return nil, nil, err
	}
	scene.Terrain, scene.TerrainRect = bakePlayTerrain(terrain, pal)
	scene.TerrainTiles, err = BuildTerrainTiles(terrain, pal.Base)
	if err != nil {
		return nil, nil, err
	}
	scene.HeightField = battleHeightField(sess.World)
	source := newBattleSource(scene, sess, fs, models, modelIndices, originY, [2]int{opts.Width, opts.Height})
	source.spectator, source.palette = opts.ViewPlayer < 0, pal
	source.materialAtlas, source.materialSource = atlas, opts.MaterialSource
	if err := source.prepareFog(); err != nil {
		return nil, nil, err
	}
	if err := source.prepareSprites(pal, cat, opts.DetailSprites); err != nil {
		return nil, nil, err
	}
	current, previous := sess.Snapshot.PinLatest()
	if current == nil {
		return nil, nil, fmt.Errorf("nanolathe: Metal battle publication unavailable: logical path committed frame, providers searched [session snapshot], expected opening battle frame")
	}
	initial := source.buildPublication(previous, current, scene.Camera, nil, nil)
	sess.Snapshot.Unpin(current)
	sess.Snapshot.Unpin(previous)
	scene.Instances = initial.frame.Instances
	scene.Frames = []PoseFrame{{Transforms: initial.frame.Previous}, {Transforms: initial.frame.Current}}
	info, _ := fs.Stat(tntPath)
	scene.Metadata["view_player"] = opts.ViewPlayer
	scene.Metadata["spectator"] = source.spectator
	scene.Metadata["fog"] = map[string]any{"policy": "committed cell operations and retained native-resolution authored Gray/Black masks; integer ordered sampling with production desaturation/checker/keyed-copy rules", "initial_dimensions": [2]int{initial.frame.Fog.Width, initial.frame.Fog.Height}, "initial_cell_rgba_bytes": len(initial.frame.Fog.RGBA), "publication_policy": "reuse detached cell codes while source/version/display-dither mode are unchanged"}
	scene.Metadata["height_field"] = map[string]any{"dimensions": [2]int{scene.HeightField.Width, scene.HeightField.Height}, "bytes": len(scene.HeightField.Values) * 4, "sample_centre_rect": scene.HeightField.Rect, "policy": "immutable map-load height samples for shadow receivers; GPU interpolation is display-only"}
	scene.Metadata["simulation_seed"] = opts.Seed
	scene.Metadata["crt_seed"] = opts.Seed
	scene.Metadata["actual_preticks"] = sess.Clock.GlobalTick
	scene.Metadata["initial_census"] = initial.census
	scene.Metadata["resolved_unit_limit"] = sess.Units.UnitLimit()
	scene.Metadata["unique_models"] = len(models)
	scene.Metadata["model_load_failures"] = failures
	scene.Metadata["models"] = census
	scene.Metadata["retained_vertices"] = vertices
	scene.Metadata["retained_triangles"] = indices / 3
	scene.Metadata["retained_authored_edges"] = edges / 2
	scene.Metadata["retained_mesh_bytes"] = vertices*64 + indices*4 + edges*4
	scene.Metadata["geometry_multiplier"] = opts.QuadMultiplier
	scene.Metadata["texture_multiplier"] = opts.TextureScale
	scene.Metadata["atlas"] = atlas.metadata
	scene.Metadata["terrain"] = map[string]any{"logical_path": tntPath, "provider": info.Source.ProviderID(), "crop_rect": scene.TerrainRect, "width": scene.Terrain.Width, "height": scene.Terrain.Height, "rgba_bytes": len(scene.Terrain.RGBA), "fixed_world_y_offset": originY, "policy": "flat native TNT background without depth writes; immutable height field supplies shadow receivers"}
	scene.Metadata["terrain_tiles"] = map[string]any{"map_dimensions": [2]int{scene.TerrainTiles.Width, scene.TerrainTiles.Height}, "unique_referenced_tiles": scene.TerrainTiles.TileCount, "atlas_dimensions": [2]int{scene.TerrainTiles.Atlas.Width, scene.TerrainTiles.Atlas.Height}, "atlas_rgba_bytes": len(scene.TerrainTiles.Atlas.RGBA), "lookup_bytes": len(scene.TerrainTiles.Lookup) * 4, "tile_side": 32, "policy": "full-resolution original TNT pixels in unique tile atlas; nearest at identity, per-tile edge-clamped bilinear under fractional world transform; optional production detail source may replace at host load"}
	scene.Metadata["pose_source"] = "worker reads pinned completed frame pairs; InstanceID matching snaps births, suppresses deaths and prevents slot-reuse blending"
	scene.Metadata["presentation_buffer_ticks"] = 1
	scene.Metadata["presentation_buffer_seconds"] = battlePeriod.Seconds()
	scene.Metadata["viewport"] = [2]int{opts.Width, opts.Height}
	scene.Metadata["camera_policy"] = "native pixels per world unit; pan clamped to crop, zoom 0.1..4; views wider than crop expose limited terrain; renderer input only"
	scene.Metadata["render_limitations"] = []string{
		"Production-composed battles use isolated subject and group source pages, ordered final commits and source-face quad UV/key/SHD mapping; cached/direct projection and cached/live seed chronology remain distinct from production",
		"GPU quaternion pose interpolation preserves rigid pieces but not the exact fixed-point hierarchy or production cache rebuild history",
		"Construction uses production reveal verdicts and GPU two-chain row endpoints with per-texel maximum keys and block-level colour admission; exact optimized outline key admission remains a documented fidelity gap",
		"Production shadow policy and silhouette composition run; native coverage and offscreen-body shadow admission still differ at boundaries",
		"Production effects, lights, glow, water, reflections, BLUE and fog are integrated; capability coverage does not establish exact pixel parity",
		"Deep attachment chains and projectile secondary-model replacement remain outside the retained instance topology",
	}
	scene.Metadata["sprites"] = source.spriteReport()
	return scene, source, nil
}

func battleModelKey(name string) string {
	name = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(name), `\`, "/"))
	name = strings.TrimPrefix(name, "objects3d/")
	return strings.TrimSuffix(name, ".3do")
}

// battleModelKeys memoizes battleModelKey for the names every frame repeats;
// normalizing a name with upper case would otherwise allocate per subject.
type battleModelKeys map[string]string

func (m *battleModelKeys) key(name string) string {
	if k, ok := (*m)[name]; ok {
		return k
	}
	if *m == nil || len(*m) >= 4096 {
		*m = make(battleModelKeys)
	}
	k := battleModelKey(name)
	(*m)[name] = k
	return k
}

func battleModelMapName(name string) string {
	name = strings.ReplaceAll(name, `\`, "/")
	name = strings.TrimPrefix(name, "maps/")
	name = strings.TrimSuffix(name, ".ota")
	return strings.TrimSuffix(name, ".tnt")
}
