package content

import (
	"reflect"
	"strings"
	"testing"
)

// Generated line names and independent comma/ASCII-space separators are the
// loader contract [03 R-VIS-01 §3], not a discovery-order convention.
func TestLOSLinesUseDeclaredNamedSlots(t *testing.T) {
	fs := newFixtureFS(t, fixtureFile{path: "gamedata/los.tdf", data: `
[TABLEINFO] { numtables=1; }
[TABLE1] {
 numlines=3;
 line4=1,9,8;
 line3=1,3,2;
 line01=1,7,6;
 line1=1,1,2;
}`})
	lt, err := CompileLOSTables(fs, RetailLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := [][]int32{{1, 1, 2}, nil, {1, 3, 2}, {1, 7, 6}, {1, 9, 8}}
	if got := lt.Tables[0].Lines; !reflect.DeepEqual(got, want) {
		t.Fatalf("named slots followed by residue = %v, want %v", got, want)
	}
}

func TestLOSLineSeparators(t *testing.T) {
	for _, tc := range []struct {
		text string
		want []int32
	}{
		{"2 1 2 3 4", []int32{2, 1, 2, 3, 4}},
		{" , 2,, 1 ,2  3,4, ", []int32{2, 1, 2, 3, 4}},
		// A tab inside a token is not a separator; decimal conversion stops
		// there. A leading tab is conversion whitespace [02 §4].
		{"1,2\t9,\t3", []int32{1, 2, 3}},
		{"1,\t,3", []int32{1, 0, 3}},
	} {
		if got := parseLOSLine(tc.text); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("parseLOSLine(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}

// The narrowing is safe here: the resulting allocation count is positive.
func TestLOSTableCountNarrowsBeforeSlotFill(t *testing.T) {
	fs := newFixtureFS(t, fixtureFile{path: "gamedata/los.tdf", data: `
[TABLEINFO] { numtables=65538; }
[TABLE1] { numlines=1; line1=1,1,2; }
[TABLE4] { numlines=1; line1=1,4,5; }
`})
	lt, err := CompileLOSTables(fs, RetailLimits())
	if err != nil {
		t.Fatal(err)
	}
	if lt.NumTables != 65538 {
		t.Fatalf("raw table count = %d", lt.NumTables)
	}
	if len(lt.Tables) != 3 || lt.Tables[0].TableNum != 1 || lt.Tables[1].TableNum != 2 || lt.Tables[2].TableNum != 4 {
		t.Fatalf("slots and residue = %+v, want TABLE1, empty TABLE2, residue TABLE4", lt.Tables)
	}
}

func TestLOSLineConsumerByteBoundary(t *testing.T) {
	for _, zeros := range []int{505, 506} {
		// Both full and shortened values contain all three tokens. Decimal
		// conversion stays in range; only the last digit crosses the boundary.
		value := "1,1," + strings.Repeat("0", zeros) + "27"
		fs := newFixtureFS(t, fixtureFile{path: "gamedata/los.tdf", data: "[TABLEINFO]{numtables=1;}[TABLE1]{numlines=1;line1=" + value + ";}"})
		lt, err := CompileLOSTables(fs, RetailLimits())
		if err != nil {
			t.Fatal(err)
		}
		want := int32(27)
		if len(value) > 511 {
			want = 2
		}
		if got := lt.Tables[0].Lines[0]; !reflect.DeepEqual(got, []int32{1, 1, want}) {
			t.Fatalf("%d-byte line = %v, want final coordinate %d", len(value), got, want)
		}
		if len(value) > 511 {
			if got := lt.Tables[0].Lines; len(got) != 2 || !reflect.DeepEqual(got[1], []int32{1, 1, 27}) {
				t.Fatalf("full authored integers were not retained after consumed slots: %v", got)
			}
		}
	}
}
