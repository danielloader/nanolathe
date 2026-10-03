package utiltac

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/aikit/brains/tactics"
	"github.com/nanolathe-gg/nanolathe/internal/aikit/brains/utility"
)

// splitPairs reads "key=value,key=value" test text into a set, failing on a
// shape the canonical text form refuses. The text form itself belongs to the
// session (session.ParseAIParams); this package checks only pairs.
func splitPairs(t *testing.T, text string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, piece := range strings.Split(text, ",") {
		key, value, ok := strings.Cut(piece, "=")
		if !ok || key == "" || value == "" {
			t.Fatalf("test text %q: want key=value pairs", text)
		}
		out[key] = value
	}
	return out
}

// ValidateParams is the strict check a configuration passes before a battle:
// an unknown key and a value a layer would clamp or ignore are refused, and a
// set that mixes the three layers' keys is accepted. These are the cases of
// mods/aikit TestValidateParamsRejectsWhatTheBrainWouldNotRead, less the one
// malformed text, which is the session's text form and not this check.
func TestValidateParamsRejectsWhatTheBrainWouldNotRead(t *testing.T) {
	for _, good := range []string{
		"style=eco,jitter=0,w_army=120,tower_time=1,micro=0,hn=5,pv=main,em=-40,air=0",
		"personality=turtle,trait_towers=-40,trait_aggression=100",
		"personality=off,trait_raids=-100",
		"survival=0,sv_tower=100,sv_walls=1,sv_claim=0",
	} {
		if err := ValidateParams(splitPairs(t, good)); err != nil {
			t.Fatalf("a valid set %q was refused: %v", good, err)
		}
	}
	for _, bad := range []string{
		"w_amry=120",        // unknown key
		"skill=90",          // an arena persona key: the lobby difficulty picks the persona
		"label=x",           // an arena label
		"w_army=401",        // outside the utility range (the reader would clamp)
		"w_army=x",          // not an integer
		"air=+0",            // not plainly spelled: tactics would read it as on
		"w_army=0120",       // not plainly spelled
		"style=rush",        // not a style
		"jitter=2",          // a variety switch is 0 or 1
		"micro=false",       // a tactics switch is 0 or 1
		"hn=0",              // tactics ignores a harass size below one
		"pv=all",            // pv takes main
		"personality=rush",  // not a personality
		"personality=0",     // the draw is turned off with the word off
		"trait_towers=101",  // outside -100..100
		"trait_towers=+5",   // not plainly spelled
		"trait_tower=5",     // unknown key
		"trait_towers=high", // not an integer
		"sv_tower=101",      // outside the survival range
		"em=2147483648",     // a knob is a 32-bit integer
	} {
		if err := ValidateParams(splitPairs(t, bad)); err == nil {
			t.Fatalf("%q was accepted", bad)
		}
	}
}

// A set is valid exactly when each of its pairs is, which is what lets a
// caller holding sorted pairs check them one at a time.
func TestValidateParamAgreesWithTheSet(t *testing.T) {
	set := map[string]string{"style": "eco", "w_army": "401"}
	if err := ValidateParams(set); err == nil {
		t.Fatal("the set with a bad pair was accepted")
	}
	if err := ValidateParam("style", "eco"); err != nil {
		t.Fatalf("the good pair was refused: %v", err)
	}
	if err := ValidateParam("w_army", "401"); err == nil {
		t.Fatal("the bad pair was accepted alone")
	}
}

// The strict vocabulary is exactly the keys the layers read. Each listed
// variety and tactics key changes what its layer's reader returns, and a scan
// of the two readers finds no key the list lacks, so a key added to a layer
// fails here until ValidateParam names it.
func TestTheVocabularyIsWhatTheLayersRead(t *testing.T) {
	varietySample := map[string]string{"style": "eco", "jitter": "0", "att_curve": "0", "att_share": "50", "att_floor": "500", "att_lag": "0", "att_build": "0", "tower_time": "1", "wide_base": "0",
		"open_reclaim": "0", "open_reclaim_hi": "400", "open_reclaim_end": "10", "open_army": "0", "open_fam": "1", "open_follow": "1",
		"personality": "off"}
	for _, k := range utility.TraitKeys {
		varietySample[k] = "40"
	}
	for _, k := range varietyKeys {
		v, err := utility.VarietyFrom(map[string]string{k: varietySample[k]})
		if err != nil || v == utility.DefaultVariety() {
			t.Fatalf("variety key %s=%s is not read (%v)", k, varietySample[k], err)
		}
	}
	for _, k := range tacticsKeys {
		value := "0"
		switch {
		case k.name == "pv":
			value = "main"
		case k.knob:
			value = "7"
		}
		if reflect.DeepEqual(tactics.ParamsFrom(map[string]string{k.name: value}), tactics.DefaultParams()) {
			t.Fatalf("tactics key %s=%s is not read", k.name, value)
		}
	}
	listed := slices.Clone(varietyKeys)
	for _, k := range tacticsKeys {
		listed = append(listed, k.name)
	}
	for _, found := range readerKeys(t, "utility", "VarietyFrom", "random") {
		if !slices.Contains(listed, found) {
			t.Errorf("utility.VarietyFrom reads %q, which ValidateParam does not name", found)
		}
	}
	for _, found := range readerKeys(t, "tactics", "ParamsFrom", "false", "main") {
		if !slices.Contains(listed, found) {
			t.Errorf("tactics.ParamsFrom reads %q, which ValidateParam does not name", found)
		}
	}
}

// readerKeys lists the key-shaped string literals in one function of a brain
// package: the keys it reads, less the named values it compares with.
func readerKeys(t *testing.T, pkg, fn string, values ...string) []string {
	t.Helper()
	dir := filepath.Join("..", pkg)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	key := regexp.MustCompile(`^[a-z][a-z_]*$`)
	var out []string
	fset := token.NewFileSet()
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			decl, ok := d.(*ast.FuncDecl)
			if !ok || decl.Recv != nil || decl.Name.Name != fn {
				continue
			}
			ast.Inspect(decl.Body, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				s, err := strconv.Unquote(lit.Value)
				if err == nil && key.MatchString(s) && !slices.Contains(values, s) {
					out = append(out, s)
				}
				return true
			})
		}
	}
	if len(out) == 0 {
		t.Fatalf("found no %s.%s to scan", pkg, fn)
	}
	return out
}

// Until mods/aikit delegates to this package, it carries its own copy of the
// vocabulary tables (mods/aikit/params.go). The two checks must agree, and a
// package under internal/ may not import the mod list, so this reads that
// file's tables as source and compares them with this package's: the
// variety, word and tactics tables and the least knob value. When mods/aikit
// calls ValidateParam instead of keeping tables, there is nothing left to
// compare and the test says so.
func TestTheModCopyOfTheVocabularyAgrees(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "mods", "aikit", "params.go")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("mods/aikit/params.go is not present: %v", err)
	}
	file, err := parser.ParseFile(token.NewFileSet(), path, source, 0)
	if err != nil {
		t.Fatal(err)
	}
	decls := map[string]ast.Expr{}
	for _, d := range file.Decls {
		gen, ok := d.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok || len(value.Names) != 1 || len(value.Values) != 1 {
				continue
			}
			decls[value.Names[0].Name] = value.Values[0]
		}
	}
	names := []string{"varietyKeys", "wordKeys", "tacticsKeys", "minKnob"}
	missing := 0
	for _, name := range names {
		if decls[name] == nil {
			missing++
		}
	}
	if missing == len(names) {
		t.Skip("mods/aikit keeps no vocabulary tables of its own: it delegates to this package")
	}
	if missing != 0 {
		t.Fatalf("mods/aikit/params.go keeps only part of the vocabulary tables %v; it should keep all or none", names)
	}

	// varietyKeys: the literal switches, then utility.TraitKeys appended.
	modVariety := stringLiterals(decls["varietyKeys"])
	ownVariety := varietyKeys[:len(varietyKeys)-len(utility.TraitKeys)]
	if !slices.Equal(modVariety, ownVariety) {
		t.Errorf("variety keys differ:\n mod %q\n own %q", modVariety, ownVariety)
	}
	if !mentionsSelector(decls["varietyKeys"], "utility", "TraitKeys") {
		t.Error("the mod's variety keys do not append utility.TraitKeys")
	}
	if modWords := stringLiterals(decls["wordKeys"]); !slices.Equal(modWords, wordKeys[:]) {
		t.Errorf("word keys differ: mod %q, own %q", modWords, wordKeys)
	}
	if got := intLiteral(t, decls["minKnob"]); got != minKnob {
		t.Errorf("least knob differs: mod %d, own %d", got, int64(minKnob))
	}
	list, ok := decls["tacticsKeys"].(*ast.CompositeLit)
	if !ok {
		t.Fatalf("mods/aikit tacticsKeys is not a composite literal")
	}
	if len(list.Elts) != len(tacticsKeys) {
		t.Fatalf("tactics keys differ in number: mod %d, own %d", len(list.Elts), len(tacticsKeys))
	}
	for i, element := range list.Elts {
		entry, ok := element.(*ast.CompositeLit)
		if !ok {
			t.Fatalf("tactics key %d is not a composite literal", i)
		}
		var got tacticsKey
		for _, field := range entry.Elts {
			pair, ok := field.(*ast.KeyValueExpr)
			if !ok {
				t.Fatalf("tactics key %d is not keyed", i)
			}
			switch pair.Key.(*ast.Ident).Name {
			case "name":
				got.name = stringLiterals(pair.Value)[0]
			case "knob":
				got.knob = pair.Value.(*ast.Ident).Name == "true"
			case "min":
				if ident, ok := pair.Value.(*ast.Ident); ok && ident.Name == "minKnob" {
					got.min = minKnob
				} else {
					got.min = intLiteral(t, pair.Value)
				}
			}
		}
		if got != tacticsKeys[i] {
			t.Errorf("tactics key %d differs: mod %+v, own %+v", i, got, tacticsKeys[i])
		}
	}
}

// stringLiterals lists the string literals under an expression, in source order.
func stringLiterals(expr ast.Expr) []string {
	var out []string
	ast.Inspect(expr, func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			if s, err := strconv.Unquote(lit.Value); err == nil {
				out = append(out, s)
			}
		}
		return true
	})
	return out
}

// mentionsSelector reports whether pkg.name appears under an expression.
func mentionsSelector(expr ast.Expr, pkg, name string) bool {
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == name {
			if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == pkg {
				found = true
			}
		}
		return true
	})
	return found
}

// intLiteral evaluates the integer constant forms the tables use: a decimal
// literal, a negation, and a shift of one.
func intLiteral(t *testing.T, expr ast.Expr) int64 {
	t.Helper()
	switch e := expr.(type) {
	case *ast.BasicLit:
		n, err := strconv.ParseInt(e.Value, 0, 64)
		if err != nil {
			t.Fatalf("integer literal %s: %v", e.Value, err)
		}
		return n
	case *ast.UnaryExpr:
		if e.Op == token.SUB {
			return -intLiteral(t, e.X)
		}
	case *ast.BinaryExpr:
		if e.Op == token.SHL {
			return intLiteral(t, e.X) << intLiteral(t, e.Y)
		}
	case *ast.ParenExpr:
		return intLiteral(t, e.X)
	}
	t.Fatalf("unsupported constant expression %T", expr)
	return 0
}
