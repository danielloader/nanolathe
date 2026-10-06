package save

import "testing"

// Bulk restoration requires an absent scalar, regardless of its payload
// [08 "Battle versus campaign continuations and timing"].
func TestBattleSummaryRejectsPresentZeroMarker(t *testing.T) {
	b := NewBuilder()
	a := b.Add(SummaryAccount)
	a.SetInt("Gametype", 1)
	a.SetInt("BetweenMissions", 0)
	bank, err := OpenBytes(b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if err := decodeSummary(bank, &BattleImage{}); err == nil {
		t.Fatal("present zero marker admitted to battle restoration")
	}
}
