package session

import (
	"errors"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/save"
)

// The dialog tests kind as well as presence; the worker tests presence alone
// [08 "Battle versus campaign continuations and timing"]. Authored banks here
// intentionally include forms the ordinary writer never emits.
func TestRetailLoadMarkerPresence(t *testing.T) {
	for _, tc := range []struct {
		name    string
		write   func(*save.Account)
		present bool
	}{
		{"absent", func(*save.Account) {}, false},
		{"integer zero", func(a *save.Account) { a.SetInt("BetweenMissions", 0) }, true},
		{"negative", func(a *save.Account) { a.SetInt("BetweenMissions", -1) }, true},
		{"other integer", func(a *save.Account) { a.SetInt("BetweenMissions", 2) }, true},
		{"string different case", func(a *save.Account) { a.SetString("betweenMISSIONS", "") }, true},
		{"double", func(a *save.Account) { a.SetDouble("BetweenMissions", 0) }, true},
		{"box only", func(a *save.Account) { a.AppendBox("BetweenMissions", 0, []byte{1}) }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, kind := range []int32{GametypeCampaign, GametypeMultiplayer} {
				b := save.NewBuilder()
				a := b.Add(save.SummaryAccount)
				a.SetInt("Gametype", kind)
				tc.write(a)
				bank, err := save.OpenBytes(b.Bytes())
				if err != nil {
					t.Fatal(err)
				}
				got, err := PreflightRetailLoad(bank)
				if err != nil {
					t.Fatal(err)
				}
				want := RetailLoadRouteBattleRestoration
				if tc.present {
					want = RetailLoadRouteFreshEntry
					if kind == GametypeCampaign {
						want = RetailLoadRouteCampaignContinuation
					}
				}
				if got.Route != want || got.Summary.HasBetweenMissions != tc.present {
					t.Fatalf("kind %d: route=%v presence=%v, want %v,%v", kind, got.Route, got.Summary.HasBetweenMissions, want, tc.present)
				}
				if want == RetailLoadRouteFreshEntry {
					_, err = LoadRetailSaveWithDeps(bank, RetailLoadDeps{FS: retailContinuationFS(t)})
					if !errors.Is(err, ErrRetailFreshEntryUnimplemented) {
						t.Fatalf("unresolved fresh-entry setup silently accepted: %v", err)
					}
				}
			}
		})
	}
}
