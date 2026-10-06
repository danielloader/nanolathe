package content

import "strings"

// NanolatheInfectorCategory is the authored category token that marks an
// original takeover attacker. Whether the token grants anything is a rule
// set's answer (DESIGN_UNITS_ORDERS_COB "Modern infection"); the catalog only
// records that it was authored.
const NanolatheInfectorCategory = "NANOLATHE_INFECTOR"

// CategoryHasToken reports an exact, case-insensitive, whitespace-delimited
// token in an authored category list. A substring of a longer token does not
// match.
func CategoryHasToken(category, token string) bool {
	for field := range strings.FieldsSeq(category) {
		if strings.EqualFold(field, token) {
			return true
		}
	}
	return false
}
