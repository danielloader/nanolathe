package hud

import (
	"fmt"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
)

// RetailBuildButtonsPerPage is the stock builder-page product count. The
// CANBUILD sequence assigns buttons 1..6 to page one, 7..12 to page two, and
// so on; the executable's DOWNLOADMENU records carry the authored page and
// gadget slot explicitly [fmt tdf][07 §9].
const RetailBuildButtonsPerPage = 6

// Build law [07 §9][02 "Build-menu catalog keys"][R-P0-03]:
// Build pages driven by CANBUILD + per-builder GUI files; button name and unit
// definition stay data-driven; GUI may not invent products absent from authored
// build list; page encoding (page&7)<<23 bits 23-25 with bit 22 paged indicator;
// MOBILEBUILD (0xE) requires the actor's compiled build-option list to EXIST —
// the catalog compiler allocates one for every `builder`-flagged definition,
// empty or not, so an authored zero-product builder still qualifies — and the
// acting unit to have a live mover, which a structure builder does not, so a
// factory falls through to the plain cursor [07 §8][04 R-ORD-02 §1].

// ProductArmsPlacement reports whether clicking this product's build gadget arms
// the placement latch rather than queueing the product immediately [07 §9].
//
// Retail tests the **product**, not the builder: the build-button handler arms
// MOBILEBUILD and stores the product's definition id only when the product's
// authored `BMcode` is zero, and otherwise falls through to the immediate queue
// path. BMcode zero is the building class — the same test that decides whether a
// definition carries a yard map at all [04 §6.2]. Keying on the builder's
// mobility instead happens to agree across the stock corpus, where factories
// build mobile units and mobile builders build structures, but it is not the
// contract.
func ProductArmsPlacement(def *content.UnitDef) bool {
	return def != nil && def.BMCode == 0
}

// BuildProductsFor copies the retail baseline list [02 "Build-menu catalog keys"].
// Live presentation uses AllowedBuildProducts so the bound construction rule
// remains authoritative; this baseline also supports hand-built frame fixtures.
func BuildProductsFor(cat *content.Catalog, builderKey string) []string {
	if cat == nil || builderKey == "" {
		return nil
	}
	if pm, ok := cat.BuildMenus[builderKey]; ok && pm != nil {
		out := make([]string, len(pm.Buttons))
		copy(out, pm.Buttons)
		return out
	}
	return nil
}

// The page cycle [07 R-HUD-03 §6]. Page 0 is the orders state and pages
// 1..count-1 are the authored build pages, so the producers below are three
// different walks over the same range:
//
//   - the `.` and `,` keys cycle through every page, page 0 included, and wrap
//     at both ends: `.` from the last page returns to page 0, `,` from page 0
//     goes to the last page. That is modular arithmetic over 0..count-1.
//   - the NEXT and PREV gadgets never return to page 0: NEXT from the last page
//     wraps to page 1, and PREV from page 1 goes to the last page.
//   - a digit selects page digit-1 outright and does nothing at all when that
//     page does not exist. It is the only producer that can refuse.
//
// Each takes and returns a page number rather than a flag word, so the caller
// keeps the committed page as the one identity it acts on [I6]. These index
// walks serve presentation-local page ranges, such as the adaptive sidebar's
// (interface design §3.3). Authored builder pages use PageState, whose walks
// are retail's field operations and differ from these once a builder has nine
// or more pages or a remembered field sits under the orders state.

// PageState is a builder's authored page state: its page-shown bit and its
// three-bit page field [07 §9][07 R-HUD-03 §6]. Retail's page producers operate
// on these two values rather than on the displayed page: all field arithmetic
// is modulo eight and `count−1` is compared as an ordinary integer, so with nine
// or more pages no field value equals it.
type PageState struct {
	Paged bool
	Field int // 0..7
}

// PageStateOf reads a builder's page state from its status word.
func PageStateOf(flags uint32) PageState {
	return PageState{Paged: IsPaged(flags), Field: RememberedPage(flags)}
}

// pageRequest is the absolute page SetBuildPage encodes into a given state: 0
// clears the page-shown bit and keeps the field, and a positive page sets the
// bit and stores its low three bits [07 §9]. A shown zero field is requested as
// page 8, the digit routine's own spelling of it; it arises only with nine or
// more pages, so the request stays inside the page count.
func pageRequest(paged bool, field int) int {
	field &= 7
	switch {
	case !paged:
		return 0
	case field == 0:
		return 8
	default:
		return field
	}
}

// NextKey is the `.` key's field operation [07 R-HUD-03 §6]: a hidden page
// shows field 1, a shown field equal to count−1 is hidden in place, and any
// other shown field advances by one modulo eight.
func (s PageState) NextKey(count int) int {
	field := s.Field & 7
	switch {
	case !s.Paged:
		return pageRequest(true, 1)
	case field == count-1:
		return pageRequest(false, field)
	default:
		return pageRequest(true, field+1)
	}
}

// PrevKey is the `,` key's field operation [07 R-HUD-03 §6]: a hidden page
// shows field (count−1) modulo eight, a shown field 1 is hidden in place, and
// any other shown field steps back by one modulo eight.
func (s PageState) PrevKey(count int) int {
	field := s.Field & 7
	switch {
	case !s.Paged:
		return pageRequest(true, count-1)
	case field == 1:
		return pageRequest(false, field)
	default:
		return pageRequest(true, field-1)
	}
}

// NextButton is the NEXT gadget's field operation [07 R-HUD-03 §6]. It does
// not consult the page-shown bit: a field equal to count−1 becomes 1, any other
// advances by one modulo eight, and the page is then shown.
func (s PageState) NextButton(count int) int {
	field := s.Field & 7
	if field == count-1 {
		return pageRequest(true, 1)
	}
	return pageRequest(true, field+1)
}

// PrevButton is the PREV gadget's field operation [07 R-HUD-03 §6]. It does
// not consult the page-shown bit: field 0 or 1 becomes (count−1) modulo eight,
// any other steps back by one, and the page is then shown. With exactly nine
// pages retail therefore stays on the shown zero field, and with ten it stays
// on page 1; both are the executable's arithmetic, not a host clamp.
func (s PageState) PrevButton(count int) int {
	field := s.Field & 7
	if field < 2 {
		return pageRequest(true, count-1)
	}
	return pageRequest(true, field-1)
}

// BuildButton is the page a BUILD click shows. The click sets the page-shown
// bit and writes no field [07 R-HUD-03 §6], so it re-shows the remembered
// field; with nine or more pages a remembered zero field is shown as such.
// Below nine pages a zero or out-of-range field does not follow the creation
// seed of a multi-page builder, and the host keeps its bounds guard of page 1:
// a one-page builder's shown zero field has no page request inside its count.
func (s PageState) BuildButton(count int) int {
	field := s.Field & 7
	if field == 0 && count > 8 {
		return pageRequest(true, field)
	}
	if field <= 0 || field >= count {
		return 1
	}
	return field
}

// NextPageKey is the `.` key's move: the next page, wrapping past the last one
// back to the orders page [07 R-HUD-03 §6].
func NextPageKey(page, count int) int {
	if count <= 0 || page < 0 || page >= count-1 {
		return 0
	}
	return page + 1
}

// PrevPageKey is the `,` key's move: the previous page, wrapping past the
// orders page back to the last one [07 R-HUD-03 §6].
func PrevPageKey(page, count int) int {
	if count <= 0 {
		return 0
	}
	if page <= 0 || page > count-1 {
		return count - 1
	}
	return page - 1
}

// NextPageButton is the NEXT gadget's move. Unlike the `.` key it never returns
// to page 0: from the last page it wraps to page 1 [07 R-HUD-03 §6].
func NextPageButton(page, count int) int {
	if count <= 1 {
		return 0
	}
	if page <= 0 || page >= count-1 {
		return 1
	}
	return page + 1
}

// PrevPageButton is the PREV gadget's move. From page 1 — and from the orders
// page — it goes to the last page rather than to page 0 [07 R-HUD-03 §6].
func PrevPageButton(page, count int) int {
	if count <= 1 {
		return 0
	}
	if page <= 1 || page > count-1 {
		return count - 1
	}
	return page - 1
}

// DigitPage is the digit rule: digit d selects page d-1 when that page exists,
// and otherwise selects nothing [07 R-HUD-03 §6]. The second result reports
// whether the digit named a page at all; false is a no-op, not a clamp, which
// is what separates the digits from the keys and the gadgets.
func DigitPage(digit, count int) (int, bool) {
	if digit < 1 || digit > 9 || count <= 0 {
		return 0, false
	}
	page := DigitToPage(digit)
	if page >= count {
		return 0, false
	}
	return page, true
}

// BuilderPageCount is the builder definition's build-menu page-count byte, the
// guard every page producer below is bounded by: valid pages are
// `0 .. count-1`, page 0 is the orders state, and the authored build pages are
// `1 .. count-1` with page N holding entries `(N-1)*perPage .. N*perPage-1`
// [07 R-HUD-03 §6].
//
// The byte is compiled from the authored `guis/<unitname>N.GUI` windows
// [02 R-CAT-01 §5 step 5], never from the length of the `CANBUILD` list.
// "Generated `<unit>N.GUI` pages are authoritative for page existence and
// placement, so a replacement engine must not infer an eight-slot grid or
// synthesize missing pages" [07 §9].
//
// This replaces PageCountFromButtons, which derived the count as
// `ceil(len(CANBUILD)/6) + 1`. That agrees with the authored pages for 39 of
// the reference install's 45 builders and overshoots by one for the other six:
// `ARMCA`/`ARMCK`/`ARMCV` author nineteen products and `CORCA`/`CORCK`/`CORCV`
// twenty, all across three authored pages, so the arithmetic claimed a fourth.
// Paging onto it asked the command switch for a window that does not exist,
// and the panel went blank — no build page and no orders page — until the
// selection changed.
//
// A definition with no authored page window has a count of 0, which is the
// established value and not an error: its products are unreachable in retail
// too, because there is no window to reach them through.
func BuilderPageCount(def *content.UnitDef) int {
	if def == nil {
		return 0
	}
	return int(def.BuildPageCount)
}

// ProductsForPage slices the authored button list for the given page. Page 0 is
// the orders page and carries no products; page N carries the authored entries
// (N-1)*perPage..N*perPage-1 [07 R-HUD-03 §6]. A page past the last authored
// one carries nothing rather than repeating the last page's products.
func ProductsForPage(all []string, page, perPage int) []string {
	if len(all) == 0 || page <= 0 {
		return nil
	}
	if perPage <= 0 {
		perPage = RetailBuildButtonsPerPage
	}
	start := (page - 1) * perPage
	if start >= len(all) {
		return nil
	}
	end := start + perPage
	if end > len(all) {
		end = len(all)
	}
	return all[start:end]
}

// QueueCountLabel returns the retail product-button count text: **one**
// running total summed over both the primary and secondary order lists for
// the matching product, formatted "+%d" [07 R-P0-11 §2]. A zero total clears
// the label; there is no clamp and no display cap.
//
// Correction (WU-19-135): this previously formatted the two lists as
// separate numbers — "%d" alone, "+%d" alone, or "%d +%d" together — on the
// theory that the secondary list was a distinct "+N" queued half. The
// refinement folded into [07 R-P0-11 §2] settles this from the executable:
// the bit-0x04 branch every build-product button authors resolves the toy's
// name to a product id and calls a **single** count query that walks the
// builder's primary list and then its secondary list into one running total,
// which is what gets formatted "+%d". A queue of five in the primary list and
// two in the secondary shows "+7", never "5 +2" — the two-number form is not
// a retail shape.
//
// The battle HUD's productQueueCountLabel selects the committed command page's
// builder and calls this; there is one implementation of the sum.
func QueueCountLabel(queues []frame.OrderQueueView, product string) string {
	key := content.CanonicalKey(product)
	if key == "" {
		return ""
	}
	var total uint32
	for _, q := range queues {
		for _, o := range q.Primary {
			if content.CanonicalKey(o.BuildProduct) == key {
				total += o.BuildCount
			}
		}
		for _, o := range q.Secondary {
			if content.CanonicalKey(o.BuildProduct) == key {
				total += o.BuildCount
			}
		}
	}
	if total == 0 {
		return ""
	}
	return fmt.Sprintf("+%d", total)
}

// AllowedBuildProducts reads the complete membership published for the selected
// builder. Legacy hand-authored frames fall back to the retail catalog list.
// The returned frame slice is immutable; callers must not modify it [I6].
func AllowedBuildProducts(cat *content.Catalog, f *frame.Frame) []string {
	if f == nil || f.CommandPage.Builder == 0 {
		return nil
	}
	if f.CommandPage.AllowedProducts != nil {
		return f.CommandPage.AllowedProducts
	}
	for _, unit := range f.Units {
		if unit.Slot == f.CommandPage.Builder {
			return BuildProductsFor(cat, content.CanonicalKey(unit.DefName))
		}
	}
	return nil
}
