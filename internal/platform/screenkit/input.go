package screenkit

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// Input is one update's pointer and keyboard state in screen pixels.
type Input struct {
	X, Y         float64
	Down         bool // left button held
	Pressed      bool // left button went down this update
	Released     bool // left button came up this update
	RightPressed bool
	WheelY       float64
	Keys         []ebiten.Key // keys that went down this update
	// Held reports any key or mouse button still down, so a screen that
	// closes can wait for the gesture to end before the window underneath
	// sees it.
	Held bool
}

// ReadInput samples Ebitengine's input. The cursor is in layout pixels, which
// the screen keeps equal to device pixels while it is up.
func ReadInput() Input {
	x, y := ebiten.CursorPosition()
	_, wy := ebiten.Wheel()
	return Input{
		X: float64(x), Y: float64(y),
		Down:         ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft),
		Pressed:      inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft),
		Released:     inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft),
		RightPressed: inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight),
		WheelY:       wy,
		Keys:         inpututil.AppendJustPressedKeys(nil),
		Held:         len(inpututil.AppendPressedKeys(nil)) > 0 || ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) || ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight),
	}
}

// KeyPressed reports whether k went down this update.
func (in Input) KeyPressed(k ebiten.Key) bool {
	for _, key := range in.Keys {
		if key == k {
			return true
		}
	}
	return false
}

// Region is one clickable area registered while drawing.
type Region struct {
	ID      string
	Rect    Rect
	Click   func()
	Right   func()
	Drag    func(x, y float64) // called while the left button is held after a press inside
	Hover   func()
	Disable bool
}

// Hits collects the regions a frame drew; the next update tests input
// against them, so a click always lands on what was on screen.
type Hits struct {
	regions []Region
	next    []Region
	hot     string
	active  string // region that received the press
	hover   map[string]float64
}

// Begin starts a frame's registration.
func (h *Hits) Begin() { h.next = h.next[:0] }

// Add registers a region; later regions sit above earlier ones.
func (h *Hits) Add(r Region) { h.next = append(h.next, r) }

// End publishes the frame's regions.
func (h *Hits) End() { h.regions, h.next = h.next, h.regions }

// Hot is the region under the pointer after the last Update.
func (h *Hits) Hot() string { return h.hot }

// Active is the region holding the current press.
func (h *Hits) Active() string { return h.active }

// HoverAmount is an eased 0..1 hover value per region, advanced by dt.
func (h *Hits) HoverAmount(id string) float64 {
	if h.hover == nil {
		return 0
	}
	return h.hover[id]
}

// Update routes input to the published regions and eases hover amounts.
// It reports whether a region consumed the press.
func (h *Hits) Update(in Input, dt float64) bool {
	if h.hover == nil {
		h.hover = map[string]float64{}
	}
	hot := ""
	var hotRegion *Region
	for i := len(h.regions) - 1; i >= 0; i-- {
		r := &h.regions[i]
		if r.Disable {
			continue
		}
		if r.Rect.Contains(in.X, in.Y) {
			hot, hotRegion = r.ID, r
			break
		}
	}
	if hot != h.hot && hotRegion != nil && hotRegion.Hover != nil {
		hotRegion.Hover()
	}
	h.hot = hot
	for id := range h.hover {
		if id != hot {
			h.hover[id] = max(0, h.hover[id]-dt*6)
			if h.hover[id] == 0 {
				delete(h.hover, id)
			}
		}
	}
	if hot != "" {
		h.hover[hot] = min(1, h.hover[hot]+dt*8)
	}
	consumed := false
	if in.Pressed && hotRegion != nil {
		h.active = hot
		consumed = true
		if hotRegion.Drag != nil {
			hotRegion.Drag(in.X, in.Y)
		}
	}
	if in.Down && h.active != "" {
		for i := range h.regions {
			if h.regions[i].ID == h.active && h.regions[i].Drag != nil {
				h.regions[i].Drag(in.X, in.Y)
			}
		}
	}
	if in.Released {
		if h.active != "" && h.active == hot && hotRegion != nil && hotRegion.Click != nil {
			hotRegion.Click()
			consumed = true
		}
		h.active = ""
	}
	if in.RightPressed && hotRegion != nil && hotRegion.Right != nil {
		hotRegion.Right()
		consumed = true
	}
	return consumed
}
