package main

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// movesFile is the subset of a tad-extract -moves extract the laboratory
// reads [fmt tad §7]. The engine's replay runner writes the same shape, so
// one miner measures a recording and a simulation with the same definitions.
type movesFile struct {
	Source   string        `json:"source"`
	ID       string        `json:"id"`
	File     string        `json:"file,omitempty"`
	Map      string        `json:"map"`
	MaxUnits int           `json:"max_units"`
	Recorder string        `json:"recorder,omitempty"`
	Status   string        `json:"status"`
	LastTick int64         `json:"last_tick"`
	Players  []movesPlayer `json:"players"`
	// Engine is set by the replay runner: the rule set and snippet the log
	// came from. A recording leaves it empty.
	Engine *engineInfo `json:"engine,omitempty"`
}

type engineInfo struct {
	Rules          string  `json:"rules"`
	Snippet        string  `json:"snippet"`
	Jitter         int     `json:"jitter"`
	OverlapTicks   int64   `json:"overlap_unit_ticks"`
	OverlapPairs   int     `json:"overlap_pairs_max"`
	OverlapResting int64   `json:"overlap_resting_unit_ticks"`
	OverlapLast    int     `json:"overlap_units_last"`
	MoverTicks     int64   `json:"mover_ticks"`
	Steers         uint64  `json:"steer_visits"`
	Probes         uint64  `json:"probes"`
	Anchors        uint64  `json:"anchors_tested"`
	Searches       int64   `json:"searches"`
	Pops           int64   `json:"pops"`
	Nudged         int     `json:"nudged"`
	Dropped        int     `json:"dropped"`
	TickNS         []int64 `json:"tick_ns,omitempty"`
}

type movesPlayer struct {
	Number   int          `json:"number"`
	Side     int          `json:"side"`
	Units    []*movesUnit `json:"units"`
	Reclaims [][3]int32   `json:"reclaims,omitempty"`
}

type movesUnit struct {
	NetID    int        `json:"net_id"`
	Type     int        `json:"type"`
	Unit     string     `json:"unit,omitempty"`
	Air      *bool      `json:"air,omitempty"`
	Tick     int64      `json:"tick"`
	X        int        `json:"x"`
	Z        int        `json:"z"`
	Builder  int        `json:"builder,omitempty"`
	FinTick  *int64     `json:"finished_tick,omitempty"`
	DiedTick *int64     `json:"died_tick,omitempty"`
	Path     [][]int32  `json:"path,omitempty"`
	Pos      [][6]int32 `json:"pos,omitempty"`
	Pose     [][5]int32 `json:"pose,omitempty"`
	Motion   [][]int32  `json:"motion,omitempty"`
	Goals    [][3]int32 `json:"goals,omitempty"`
	Hurt     [][2]int32 `json:"hurt,omitempty"`
	Fire     [][2]int32 `json:"fire,omitempty"`
}

func readJSONMaybeGz(path string, v any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	var r io.Reader = f
	if strings.HasSuffix(path, ".gz") {
		zr, err := gzip.NewReader(f)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		defer zr.Close()
		r = zr
	}
	if err := json.NewDecoder(r).Decode(v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func writeJSONMaybeGz(path string, v any) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	var w io.Writer = f
	var zw *gzip.Writer
	if strings.HasSuffix(path, ".gz") {
		zw, _ = gzip.NewWriterLevel(f, gzip.BestSpeed)
		w = zw
	}
	enc := json.NewEncoder(w)
	if err := enc.Encode(v); err != nil {
		f.Close()
		return err
	}
	if zw != nil {
		if err := zw.Close(); err != nil {
			f.Close()
			return err
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// unitDef is what the laboratory needs to know about a unit type. The table
// is written from the engine's compiled catalog (path-lab units), so every
// number is the authored definition's, never an estimate.
type unitDef struct {
	Name string `json:"unitname"`
	// Air is canfly; Mobile is a definition that can move on its own.
	Air    bool `json:"air"`
	Mobile bool `json:"mobile"`
	// FX, FZ is the footprint in 16-world-unit cells.
	FX int `json:"foot_x"`
	FZ int `json:"foot_z"`
	// VMax is MaxVelocity in world units per tick.
	VMax  float64 `json:"vmax"`
	Accel float64 `json:"accel,omitempty"`
	Brake float64 `json:"brake,omitempty"`
	Turn  int     `json:"turn,omitempty"`
	// Class is the authored movement class name, Kind a coarse family:
	// kbot, vehicle, hover, ship, amphibious, sub, structure, air.
	Class   string `json:"class,omitempty"`
	Kind    string `json:"kind,omitempty"`
	Builder bool   `json:"builder,omitempty"`
	// ClassFX, ClassFZ is the movement class record's footprint, the one
	// route points are biased by [04 R-PATH-01 §7].
	ClassFX int `json:"class_foot_x,omitempty"`
	ClassFZ int `json:"class_foot_z,omitempty"`
}

type unitTable map[string]*unitDef

// loadUnitTable reads either the laboratory's own table or the older stock
// summary (derived/unit_table.json), whose speed is world units per second
// and whose canfly is the "air" role.
func loadUnitTable(path string) (unitTable, error) {
	var rows []map[string]any
	if err := readJSONMaybeGz(path, &rows); err != nil {
		return nil, err
	}
	t := unitTable{}
	for _, r := range rows {
		name, _ := r["unitname"].(string)
		if name == "" {
			continue
		}
		d := &unitDef{Name: strings.ToUpper(name)}
		num := func(k string) float64 { f, _ := r[k].(float64); return f }
		d.FX, d.FZ = int(num("foot_x")), int(num("foot_z"))
		if _, own := r["vmax"]; own {
			d.VMax = num("vmax")
			d.Air, _ = r["air"].(bool)
			d.Mobile, _ = r["mobile"].(bool)
			d.Accel, d.Brake, d.Turn = num("accel"), num("brake"), int(num("turn"))
			d.Class, _ = r["class"].(string)
			d.Kind, _ = r["kind"].(string)
			d.Builder, _ = r["builder"].(bool)
			d.ClassFX, d.ClassFZ = int(num("class_foot_x")), int(num("class_foot_z"))
		} else {
			d.VMax = num("speed") / 30
			d.Mobile = d.VMax > 0
			if roles, ok := r["roles"].([]any); ok {
				for _, x := range roles {
					switch x {
					case "air":
						d.Air = true
					case "constructor":
						d.Builder = true
					}
				}
			}
			d.Builder = d.Builder || num("build_power") > 0 && d.Mobile
			switch {
			case d.Air:
				d.Kind = "air"
			case !d.Mobile:
				d.Kind = "structure"
			default:
				d.Kind = "ground"
			}
		}
		t[d.Name] = d
	}
	return t, nil
}

func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
