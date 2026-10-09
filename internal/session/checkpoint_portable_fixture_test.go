package session

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/community"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// This install is entirely authored here. The real compiler, freezer and M2
// admitted constructor own definition identities, allocation and composition;
// no synthetic world or admission receipt participates (DESIGN_MULTIPLAYER
// §16.3.4 M3-C8). The intentionally weaponless commanders keep the script short.
func checkpointPortableInputs(t *testing.T, mode gameplay.Mode) (*content.SimulationInputs, EffectiveMatchConfig) {
	t.Helper()
	model, err := formats.EncodeThreeDO(&formats.ThreeDO{Root: 0, Objects: []formats.ThreeDOObject{{
		Version: 1, Name: "base", Selection: -1, Parent: -1, FirstChild: -1, NextSibling: -1,
		Vertices: []formats.ThreeDOVertex{{}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	// Authored masks, not an approximation of the retail shapes [fmt gaf].
	frames := make([]formats.GAFWriteFrame, 10)
	for i := range frames {
		frames[i] = formats.GAFWriteFrame{Width: 3, Height: 3, XOffset: 1, YOffset: 1,
			Pixels: []byte{7, 7, 7, 7, 7, 7, 7, 7, 7}}
	}
	masks, err := formats.EncodeGAF([]formats.GAFWriteEntry{{Name: "vismask", Frames: frames}})
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"maps/portable.tnt": string(checkpointPortableTNT()),
		"maps/portable.ota": `[GlobalHeader]{MinWindSpeed=100;MaxWindSpeed=200;Gravity=112;
[Schema 0]{Type=Network 1;SurfaceMetal=1;
[specials]{[special0]{specialwhat=StartPos1;XPos=128;ZPos=128;}
[special1]{specialwhat=StartPos2;XPos=384;ZPos=320;}}}}`,
		"gamedata/moveinfo.tdf":  `[CLASS0]{Name=portable;FootprintX=1;FootprintZ=1;MaxWaterDepth=10;MaxSlope=10;}`,
		"gamedata/los.tdf":       `[TABLEINFO]{numtables=1;}[TABLE1]{numlines=1;line1=1,0,1;}`,
		"gamedata/sidedata.tdf":  checkpointPortableSides(),
		"anims/vismasks.gaf":     string(masks),
		"objects3d/portable.3do": string(model),
		"ai/default.txt":         "plan any\n",
	}
	for _, dir := range []string{"weapons", "features", "download", "guis", "unitpics"} {
		files[dir+"/notes.txt"] = "Authored checkpoint fixture; no records in this family.\n"
	}
	for i, name := range []string{"portarm", "portcore"} {
		files["units/"+name+".fbi"] = fmt.Sprintf(`[UNITINFO]{UnitName=%s;Name=Portable Commander;Side=%s;
ObjectName=portable;Category=COMMANDER MOBILE;Commander=1;BMcode=1;CanMove=1;CanStop=1;
MovementClass=portable;MaxVelocity=2;Acceleration=0.25;BrakeRate=0.5;TurnRate=1024;
MaxDamage=1000;SightDistance=160;FootprintX=1;FootprintZ=1;
BuildCostMetal=1;BuildCostEnergy=1;BuildTime=1;}`, name, []string{"ARM", "CORE"}[i])
		files["scripts/"+name+".cob"] = string(checkpointPortableCOB())
	}
	// FBI discovery admits archive sources [02 R-CAT-01 §4]. Author the
	// package in memory through the normal HPI writer [fmt hpi].
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	archiveFiles := make([]vfs.ArchiveFile, 0, len(paths))
	for _, path := range paths {
		archiveFiles = append(archiveFiles, vfs.ArchiveFile{Path: path, Data: []byte(files[path])})
	}
	var archive bytes.Buffer
	if err := vfs.WriteArchive(&archive, archiveFiles, vfs.ArchiveWriteOptions{}); err != nil {
		t.Fatal(err)
	}
	fs := vfs.New()
	if _, err := fs.MountArchiveReader("portable.hpi", bytes.NewReader(archive.Bytes()), int64(archive.Len()), 10, vfs.ArchiveOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fs.Close() })
	cfg := DirectSkirmishConfig("portable")
	cfg.Gameplay, cfg.UnitLimit = mode, 20
	cfg.RNGSimSeed, cfg.RNGCrtSeed = 7, 11
	cfg.Location = 1 // explicit start-slot selection
	// Let the same explicit small pool size reach every mode; the other
	// Community settings retain their normal source resolution.
	zero := 0
	options := SkirmishEntryOptions{CommunitySources: CommunitySources{Player: community.Overrides{UnitLimit: &zero}}}
	config := admitConfig(t, cfg, options, 0)
	inputs, err := FreezeMatchInputs(fs, nil, config, nil)
	if err != nil {
		t.Fatal(err)
	}
	return inputs, config
}

func checkpointPortableBattle(t *testing.T, inputs *content.SimulationInputs, config EffectiveMatchConfig) *Session {
	t.Helper()
	s, err := NewAdmittedSkirmish(inputs, config, nil)
	if err != nil {
		t.Fatal(err)
	}
	for range 8 {
		if plan := s.PrepareStep(s.Clock.ScaledAnchor); plan.Runs() {
			return s
		}
	}
	t.Fatal("authored entry did not reach its first pump")
	return nil
}

// Canonical TNT: one tile, flat height 20, and no feature anchors [fmt tnt].
func checkpointPortableTNT() []byte {
	const width, height = 32, 32
	const tiles = 0x40
	const attrs = tiles + (width/2)*(height/2)*2
	const graphics = attrs + width*height*4
	const mini = graphics + 1024
	b := make([]byte, mini+8)
	le := binary.LittleEndian
	le.PutUint32(b[0x00:], 0x2000)
	le.PutUint32(b[0x04:], width)
	le.PutUint32(b[0x08:], height)
	le.PutUint32(b[0x0c:], tiles)
	le.PutUint32(b[0x10:], attrs)
	le.PutUint32(b[0x14:], graphics)
	le.PutUint32(b[0x18:], 1)
	le.PutUint32(b[0x20:], mini)
	le.PutUint32(b[0x28:], mini)
	for i := range width * height {
		b[attrs+4*i] = 20
		le.PutUint16(b[attrs+4*i+1:], 0xffff)
	}
	return b
}

// One returning Create script and one matching model piece [fmt cob][fmt 3do].
func checkpointPortableCOB() []byte {
	const code, entries, names, pieces, create, base = 44, 48, 52, 56, 60, 67
	b := make([]byte, 72)
	le := binary.LittleEndian
	le.PutUint32(b[0:], 4)
	le.PutUint32(b[4:], 1)
	le.PutUint32(b[8:], 1)
	le.PutUint32(b[12:], 1)
	le.PutUint32(b[24:], entries)
	le.PutUint32(b[28:], names)
	le.PutUint32(b[32:], pieces)
	le.PutUint32(b[36:], code)
	le.PutUint32(b[40:], create)
	le.PutUint32(b[code:], 0x10065000) // COB return opcode [fmt cob]
	le.PutUint32(b[names:], create)
	le.PutUint32(b[pieces:], base)
	copy(b[create:], "Create\x00")
	copy(b[base:], "base\x00")
	return b
}

func checkpointPortableSides() string {
	var b strings.Builder
	for i, side := range []string{"ARM", "CORE"} {
		fmt.Fprintf(&b, "[SIDE%d]{name=%s;commander=%s;font=portable.fnt;", i, side, []string{"portarm", "portcore"}[i])
		// Side compilation requires these interface anchors [02 §6]. Their
		// zero rectangles are unused by this displayless authored scene.
		for _, name := range strings.Fields("LOGO ENERGYBAR ENERGYNUM ENERGYMAX ENERGY0 METALBAR METALNUM METALMAX METAL0 TOTALUNITS TOTALTIME ENERGYPRODUCED ENERGYCONSUMED METALPRODUCED METALCONSUMED LOGO2 UNITNAME DAMAGEBAR UNITMETALMAKE UNITMETALUSE UNITENERGYMAKE UNITENERGYUSE MISSIONTEXT UNITNAME2 DAMAGEBAR2 NAME DESCRIPTION RELOAD1 RELOAD2 RELOAD3") {
			fmt.Fprintf(&b, "[%s]{}", name)
		}
		b.WriteString("}")
	}
	return b.String()
}
