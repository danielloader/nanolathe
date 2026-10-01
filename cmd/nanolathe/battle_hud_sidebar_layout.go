package main

// The shared battle/preview layout follows Nanolathe's host policy (HUD §3.3):
// build capacity comes first, then optional orders, then any spare build rows.
type sidebarPageLayout struct {
	capacity     int
	inlineOrders bool
	commands     []sidebarProduct
	spacing      []sidebarCommandGap
	lowerHeight  int32
	commandTop   int32
}

func (c *sidebarProductCatalog) sidebarLayout(height, limit int, orders bool) sidebarPageLayout {
	// Hiding inline orders must not activate a pager whose dedicated Orders
	// page cannot fit. Such custom scaffolds keep the authored/fitted path.
	if _, fits := sidebarFitSpacing(int32(height)-128-c.upperHeight-c.lowerHeight, c.spacing); !fits {
		return sidebarPageLayout{}
	}
	required := limit
	if required <= 0 {
		required = 6
	}
	if orders {
		spacing, fits := sidebarFitSpacing(c.sidebarBuildBudget(height, c.lowerHeight, required), c.spacing)
		if fits {
			p := sidebarPageLayout{inlineOrders: true, commands: c.commands, spacing: spacing, lowerHeight: c.lowerHeight}
			p.capacity = c.sidebarLayoutCapacity(height, p)
			if limit > 0 {
				p.capacity = min(p.capacity, limit)
			}
			return c.sidebarPlaceCommands(height, limit, p)
		}
	}
	spacing, fits := sidebarFitSpacing(c.sidebarBuildBudget(height, c.buildCommandHeight, required), c.buildCommandSpacing)
	if !fits && limit > 0 {
		// A constrained fixed count takes every achievable complete row before
		// retaining the remaining gap pixels. Free flow keeps its six-slot
		// floor rather than entering this fallback.
		available := height - 128 - int(c.upperHeight) - int(c.buildCommandHeight)
		for _, gap := range c.buildCommandSpacing {
			if gap.pixels > 0 {
				available--
			}
		}
		fittingSlots := max(0, available/64) * 2
		if fittingSlots == 0 {
			return sidebarPageLayout{}
		}
		spacing, fits = sidebarFitSpacing(c.sidebarBuildBudget(height, c.buildCommandHeight, fittingSlots), c.buildCommandSpacing)
	}
	if !fits {
		return sidebarPageLayout{}
	}
	p := sidebarPageLayout{commands: c.buildCommands, spacing: spacing, lowerHeight: c.buildCommandHeight}
	p.capacity = c.sidebarLayoutCapacity(height, p)
	if limit > 0 {
		p.capacity = min(p.capacity, limit)
	}
	return c.sidebarPlaceCommands(height, limit, p)
}

// Fixed counts keep the control panel directly below their reserved build
// rows, including on partial pages. Free flow uses the full rail (HUD §3.3).
func (c *sidebarProductCatalog) sidebarPlaceCommands(height, limit int, p sidebarPageLayout) sidebarPageLayout {
	p.commandTop = int32(height) - p.lowerHeight
	for _, gap := range p.spacing {
		if gap.at >= 0 {
			p.commandTop -= gap.pixels
		}
	}
	if limit > 0 {
		p.commandTop = 128 + c.upperHeight + int32((p.capacity+1)/2)*64
		for _, gap := range p.spacing {
			if gap.at < 0 {
				p.commandTop += gap.pixels
			}
		}
	}
	return p
}

func (c *sidebarProductCatalog) sidebarBuildBudget(height int, lowerHeight int32, slots int) int32 {
	available := height - 128 - int(c.upperHeight) - int(lowerHeight)
	rows := slots/2 + slots%2
	// Compare before multiplying: legacy or hand-edited counts can be larger
	// than the rail, and must not overflow into a falsely fitting layout.
	if available < 0 || rows > available/64 {
		return -1
	}
	return int32(available - rows*64)
}

func (c *sidebarProductCatalog) sidebarLayoutCapacity(height int, p sidebarPageLayout) int {
	reserved := int32(128) + c.upperHeight + p.lowerHeight
	for _, gap := range p.spacing {
		reserved += gap.pixels
	}
	return max(0, int((int32(height)-reserved)/64)*2)
}
