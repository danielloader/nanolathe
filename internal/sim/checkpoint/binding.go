package checkpoint

// BindingAuthority identifies one session's reviewed callback installations.
// The session keeps it private; owners retain it beside private callback slots,
// and capture contexts borrow it. Ordinary setters clear only the changed slot's
// authority. This is diagnostic provenance, never a gameplay rule or wire value
// (DESIGN_MULTIPLAYER §16.3.28).
//
// The nonzero size matters: Go may give distinct zero-size allocations the same
// address. No caller can recover a live owner's authority through this type.
type BindingAuthority struct{ _ byte }

func NewBindingAuthority() *BindingAuthority { return new(BindingAuthority) }

// Matches refuses unadmitted (nil) slots even when the expected authority is nil.
// Pointer equality is local validation only; it never enters canonical bytes.
func (a *BindingAuthority) Matches(expected *BindingAuthority) bool {
	return a != nil && a == expected
}
