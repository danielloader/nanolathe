package pathlab

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// Snippet is a piece of a recorded game cut out for replay by
// tools/path-lab: the units and structures that stood in a region during a
// window of ticks and the goals the recording shows them being given.
type Snippet struct {
	Schema    int         `json:"schema"`
	ID        string      `json:"id"`
	Rec       string      `json:"rec"`
	Map       string      `json:"map"`
	Content   string      `json:"content"`
	Class     string      `json:"class,omitempty"`
	Note      string      `json:"note,omitempty"`
	T0        int32       `json:"t0"`
	T1        int32       `json:"t1"`
	Start     int32       `json:"start"`
	Region    [4]float64  `json:"region"`
	Players   []int       `json:"players"`
	Units     []SnipUnit  `json:"units"`
	Orders    []SnipOrder `json:"orders"`
	Groups    []SnipGroup `json:"groups,omitempty"`
	Reclaimed [][2]int32  `json:"reclaimed,omitempty"`
}

// SnipGroup is one recorded group move: the units one command moved and
// the displacement it gave every unit that kept its place in the formation
// [04 R-STANCE-01 §5]. The replay issues it as one group command, so each
// rule set assigns the destinations its own way.
type SnipGroup struct {
	Tick   int32   `json:"tick"`
	Player int     `json:"player"`
	DX     float64 `json:"dx"`
	DZ     float64 `json:"dz"`
	Units  []int   `json:"units"`
	// Count is the least size the recorded selection can have had, given
	// how far from its centre a member still kept its formation offset.
	Count int32 `json:"count"`
}

// SnipUnit is one staged unit. ID is the recording's unit ID.
type SnipUnit struct {
	ID        int     `json:"id"`
	Name      string  `json:"name"`
	Player    int     `json:"player"`
	X         float64 `json:"x"`
	Z         float64 `json:"z"`
	HX        float64 `json:"hx,omitempty"`
	HZ        float64 `json:"hz,omitempty"`
	Structure bool    `json:"structure,omitempty"`
	Spawn     int32   `json:"spawn"`
	Die       int32   `json:"die"`
	Subject   bool    `json:"subject,omitempty"`
}

// SnipOrder is one goal given to a unit at a recording tick.
type SnipOrder struct {
	Tick    int32   `json:"tick"`
	Unit    int     `json:"unit"`
	GoalX   float64 `json:"goal_x"`
	GoalZ   float64 `json:"goal_z"`
	Carry   bool    `json:"carry,omitempty"`
	Derived bool    `json:"derived,omitempty"`
	// Group is one more than the index of the group command this order
	// came from, zero for an order of its own.
	Group int `json:"group,omitempty"`
}

// ReadSnippet reads a snippet file, gzip-compressed when its name ends in
// .gz.
func ReadSnippet(path string) (*Snippet, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var r io.Reader = f
	if strings.HasSuffix(path, ".gz") {
		zr, err := gzip.NewReader(f)
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		r = zr
	}
	var s Snippet
	if err := json.NewDecoder(r).Decode(&s); err != nil {
		return nil, fmt.Errorf("nanolathe: snippet is not readable: logical path %s, providers searched [file], expected a path-lab snippet: %w", path, err)
	}
	if s.Start == 0 || s.Start > s.T0 {
		s.Start = s.T0
	}
	if s.Schema != 1 {
		return nil, fmt.Errorf("nanolathe: snippet schema is not supported: logical path %s, providers searched [file], expected schema 1", path)
	}
	return &s, nil
}

// MovesLog is a replay's movement log in the shape of a tools/tad-extract
// movement extract, so the offline miner reads a recording and a replay
// alike [fmt tad §7].
type MovesLog struct {
	Source   string        `json:"source"`
	ID       string        `json:"id"`
	File     string        `json:"file,omitempty"`
	Map      string        `json:"map"`
	MaxUnits int           `json:"max_units"`
	Status   string        `json:"status"`
	LastTick int64         `json:"last_tick"`
	Players  []MovesPlayer `json:"players"`
	Engine   *EngineInfo   `json:"engine,omitempty"`
}

// EngineInfo names the replay a log came from and carries the measurements
// only the engine can make.
type EngineInfo struct {
	Rules   string `json:"rules"`
	Snippet string `json:"snippet"`
	Jitter  int    `json:"jitter"`
	// Overlap counts, per unit-tick, a staged mobile unit whose committed
	// footprint shares a cell with another staged mobile unit's.
	OverlapTicks int64 `json:"overlap_unit_ticks"`
	// OverlapPairs is the most overlapping pairs seen on one tick.
	OverlapPairs int `json:"overlap_pairs_max"`
	// OverlapResting is the part of OverlapTicks spent by units holding no
	// route, and OverlapLast the units overlapping on the last tick: units
	// left standing inside one another.
	// MoverTicks counts visits of staged mobile units that held a route;
	// Steers, Probes and Anchors are the movement system's counts over the
	// replay (movement.LabStats).
	MoverTicks     int64  `json:"mover_ticks"`
	Steers         uint64 `json:"steer_visits"`
	Probes         uint64 `json:"probes"`
	Anchors        uint64 `json:"anchors_tested"`
	OverlapResting int64  `json:"overlap_resting_unit_ticks"`
	OverlapLast    int    `json:"overlap_units_last"`
	// Nudged counts staged units moved off their recorded position because
	// it was not free; Dropped counts units no free position was found for.
	Nudged  int `json:"nudged"`
	Dropped int `json:"dropped"`
	// TickNS is the wall time of each authoritative tick, nanoseconds.
	TickNS []int64 `json:"tick_ns,omitempty"`
	// Searches and Pops are the path searches finished and the search
	// steps charged over the window.
	Searches int64 `json:"searches"`
	Pops     int64 `json:"pops"`
	// SteerStarts and SteerTicks are the times movers began to steer round
	// something and the visits they spent steering, by what was ahead
	// (movement.SteerStatic and the rest).
	// Refused is refused ground steps by what refused them
	// (movement.RefusedStatic and the rest).
	Refused     []uint64 `json:"refused,omitempty"`
	RefusedNear []uint64 `json:"refused_near,omitempty"`
	SteerStarts []uint64 `json:"steer_starts,omitempty"`
	SteerTicks  []uint64 `json:"steer_ticks,omitempty"`
	Notes       []string `json:"notes,omitempty"`
}

// MovesPlayer is one player's units.
type MovesPlayer struct {
	Number int          `json:"number"`
	Side   int          `json:"side"`
	Units  []*MovesUnit `json:"units"`
}

// MovesUnit is one unit's log: creation, death and movement entries.
type MovesUnit struct {
	NetID    int        `json:"net_id"`
	Unit     string     `json:"unit"`
	Tick     int64      `json:"tick"`
	X        int        `json:"x"`
	Z        int        `json:"z"`
	FinTick  *int64     `json:"finished_tick,omitempty"`
	DiedTick *int64     `json:"died_tick,omitempty"`
	Path     [][]int32  `json:"path,omitempty"`
	Pose     [][5]int32 `json:"pose,omitempty"`
	// Motion is sampled with the poses, a replay only: tick, then running
	// totals since the unit was staged of distance moved in world units,
	// turning in 256ths of a circle, the times it came to a stop while it
	// held a route, distance moved away from the goal of its order in
	// world units, and reversals: the times it turned one way and then,
	// within a second and a half, the other, each by an eighth of a quarter
	// turn or more.
	Motion [][6]int32 `json:"motion,omitempty"`
	// Goals is the goal of the order at the head of the unit's queue, a
	// replay only: tick, x and z in world units, one row each time it
	// changed. A group command gives each member a goal of its own, which
	// is not the recorded one when the rule set places a group differently.
	Goals [][3]int32 `json:"goals,omitempty"`
}

// WriteJSON writes v to path, gzip-compressed when the name ends in .gz.
func WriteJSON(path string, v any) error {
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
	if err := json.NewEncoder(w).Encode(v); err != nil {
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
