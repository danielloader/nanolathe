package content

// extendMutatorSight supplies larger compiled raster inputs for the Sight
// mutator, in every mode (docs/DESIGN_MODS_MUTATORS.md §6.5). The ordinary
// publication, horizon and removal algorithms do not know about mutators.
func (c *Catalog) extendMutatorSight(units []*UnitDef) {
	// Sight is saturated at its signed-word store before this call. Only
	// definition radii can be published, including temporary death sight.
	var needed [1024]bool
	last := 0
	for _, u := range units {
		q := int(int16(u.SightDistance)) / 32
		if q > 0 {
			needed[q] = true
			last = max(last, q)
		}
	}
	if c.LOS != nil {
		n := int(int16(c.LOS.NumTables))
		if n > 1 && last >= n {
			old := c.LOS.Tables
			// Keep original reachable slots, fill unused new slots with the
			// former clamped table, and retain unreachable authoring residue
			// after the new declared list. No residue becomes active by accident.
			tables := make([]LOSTable, last+1)
			var fallback LOSTable
			if n-2 < len(old) {
				fallback = old[n-2]
			}
			for i := range tables {
				tables[i] = fallback
				if i < n-1 {
					tables[i] = LOSTable{}
					if i < len(old) {
						tables[i] = old[i]
					}
				} else if q := i + 1; q <= last && needed[q] {
					tables[i] = mutatorSightRays(q)
				}
			}
			c.LOS.Tables = append(tables, old[min(n-1, len(old)):]...)
			c.LOS.NumTables = int32(last + 1)
		}
	}
	if n := c.Sight.Count(); n > 0 && last-5 >= n {
		old := c.Sight.Shapes
		shapes := make([]SightShape, last-4)
		copy(shapes, old)
		for i := n; i < len(shapes); i++ {
			shapes[i] = old[n-1]
			if needed[i+5] {
				shapes[i] = mutatorSightMask(i + 5)
			}
		}
		c.Sight.Shapes = shapes
	}
}

// Larger masks use the inclusive integer disc. This is generated Nanolathe
// content, not a reconstruction of the authored GAF [03 §3.2].
func mutatorSightMask(radius int) SightShape {
	r := int32(radius)
	side := 2*r + 1
	sh := SightShape{W: side, H: side, AnchorX: r, AnchorY: r, Opaque: make([]bool, side*side)}
	for y := -r; y <= r; y++ {
		for x := -r; x <= r; x++ {
			sh.Opaque[(y+r)*side+x+r] = x*x+y*y <= r*r
		}
	}
	return sh
}

// mutatorSightRays builds one quadrant; the existing consumer rotates it four
// times [03 R-VIS-01 §3]. Rays aim at each cell on two sides of the bounding
// square, round each dominant-axis step half up, and stop outside the disc.
// The shared diagonal and quadrant boundary each occur only once. Keeping
// every step lets terrain obstruction retain its ordinary horizon test.
func mutatorSightRays(radius int) LOSTable {
	r := int32(radius)
	tb := LOSTable{TableNum: radius, NumLines: 2 * r, Lines: make([][]int32, 0, 2*r)}
	for edge := int32(0); edge < 2*r; edge++ {
		x, y := r, edge
		if edge > r {
			x, y = 2*r-edge, r
		}
		line := make([]int32, 1, 1+2*r)
		for distance := int32(1); distance <= r; distance++ {
			u, v := (x*distance+r/2)/r, (y*distance+r/2)/r
			if u*u+v*v > r*r {
				break
			}
			line = append(line, u, v)
		}
		line[0] = int32((len(line) - 1) / 2)
		tb.Lines = append(tb.Lines, line)
	}
	return tb
}
