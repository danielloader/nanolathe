package economy

import (
	"math"
	"testing"
)

// Sharing tests the post-cap amount, then attempts the debit independently of
// the recipient credit [05 R-SHARE-01 §2]. These are helper operand cases; they
// do not assert that ordinary SHARE controls can produce exceptional values.
func TestTransferExceptionalOperands(t *testing.T) {
	nan := math.Float32frombits(0x7fc00123)
	for _, res := range []Res{Metal, Energy} {
		for _, recipient := range []struct {
			name       string
			computer   bool
			fullIncome bool
			factor     float32
		}{
			{"human", false, false, 1},
			{"classic easy", true, false, 0.5},
			{"modern full income", true, true, 1},
		} {
			for _, tc := range []struct {
				name                 string
				stock, amount        float32
				wantStock, wantDebit float32
				wantContribution     float32
			}{
				{"NaN amount", 20, nan, 20, 0, 0},
				{"NaN amount and stock", nan, nan, nan, 0, 0},
				{"NaN stock positive amount", nan, 8, nan, 0, 8},
				{"NaN stock negative amount", nan, -8, nan, 0, -8},
				{"ordered positive", 20, 8, 12, 8, 8},
				{"ordered negative", 20, -8, 28, -8, -8},
				{"clamped to stock", 4, 8, 0, 4, 4},
				{"clamped to zero", 0, 8, 0, 0, 0},
				{"zero after cap", 20, 0, 20, 0, 0},
				{"zero before negative cap", -4, 0, 0, -4, -4},
			} {
				t.Run(recipient.name+"/"+tc.name+"/"+[]string{"metal", "energy"}[res], func(t *testing.T) {
					var svc Service
					svc.SetEconomySelector(0)
					src, dst := &svc.Players[0], &svc.Players[1]
					activePlayer(src)
					activePlayer(dst)
					if recipient.computer {
						dst.ControllerState = 2
					}
					dst.FullIncome = recipient.fullIncome
					src.Stock = [2]float32{19, 19}
					dst.Stock = [2]float32{23, 23}
					src.Stock[res] = tc.stock
					for _, r := range []Res{Metal, Energy} {
						src.Mirror[r] = Bucket{Production: 3, Requested: 7, Accepted: 11, Carry: 13}
						dst.Mirror[r] = Bucket{Production: 5, Requested: 17, Accepted: 29, Carry: 31}
					}
					wantSourceStock, wantDestinationStock := src.Stock, dst.Stock
					wantSourceMirror, wantDestinationMirror := src.Mirror, dst.Mirror
					wantSourceStock[res] = tc.wantStock
					wantSourceMirror[res].Requested += tc.wantDebit
					wantDestinationMirror[res].Production += tc.wantContribution * recipient.factor

					svc.Transfer(0, 1, res, tc.amount)

					// Bit comparisons also prove that a refused debit leaves the
					// source NaN payload alone and neither resource's other fields move.
					for _, pair := range []struct {
						got, want float32
					}{
						{src.Stock[Metal], wantSourceStock[Metal]}, {src.Stock[Energy], wantSourceStock[Energy]},
						{dst.Stock[Metal], wantDestinationStock[Metal]}, {dst.Stock[Energy], wantDestinationStock[Energy]},
					} {
						if math.Float32bits(pair.got) != math.Float32bits(pair.want) {
							t.Fatalf("stock = %v (%08x), want %v (%08x)", pair.got, math.Float32bits(pair.got), pair.want, math.Float32bits(pair.want))
						}
					}
					if src.Mirror != wantSourceMirror || dst.Mirror != wantDestinationMirror {
						t.Fatalf("source mirror = %+v, want %+v; destination mirror = %+v, want %+v", src.Mirror, wantSourceMirror, dst.Mirror, wantDestinationMirror)
					}
				})
			}
		}
	}
}
