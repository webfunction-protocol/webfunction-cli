package cmd

import (
	"path/filepath"
	"strings"
	"testing"
)

// Per the package spec, a bare "array" (and "[[\"any\"]]", which is the same
// thing) is an array of any, but when an argument has "choices" or an
// attribute has "values" on an array type, those are the allowed items. Go
// cannot express a literal union, so it keeps the doc note, but the item
// type is no longer unknown when every allowed item is a string, a number
// or a boolean: the field is then []string, []float64 or []bool. Mixed
// items (or an explicit null) stay []any, an item type the package spells
// out is never overridden, and a union that mixes an array with a scalar
// collapses to any, as every multi-type union does in Go.

func goArraysFixture(name string) string {
	return filepath.Join("..", "testdata", "codegen", "arrays", name)
}

// goFieldTypes maps each struct field name in the generated Go to its type.
func goFieldTypes(src string) map[string]string {
	types := map[string]string{}
	for _, l := range strings.Split(src, "\n") {
		f := strings.Fields(strings.TrimRight(l, "\r"))
		if len(f) >= 3 && strings.HasPrefix(f[len(f)-1], "`json:") {
			types[f[0]] = f[1]
		}
	}
	return types
}

func TestGoArrayItemTypeFromChoicesAndValues(t *testing.T) {
	fixtures := []struct {
		file  string
		cases []struct{ field, goType string }
	}{
		{"arrays.json", []struct{ field, goType string }{
			// attributes (values)
			{"ABareValues", "[]string"},
			{"BNestedValues", "[]string"},
			{"CCanonValues", "[]string"},
			{"DNestedNovalues", "[]string"},
			{"EBareNovalues", "[]any"},
			{"FNumValues", "[]float64"},
			{"GUnionItemsValues", "[]any"},
			{"HNullableValues", "[]string"},
			{"IMixedValues", "any"},
			{"JScalarValues", "string"},
			// arguments (choices)
			{"ABareChoices", "[]string"},
			{"BNestedChoices", "[]string"},
			{"CNestedNochoices", "[]string"},
			{"DScalarChoices", "*string"},
			{"EMixedChoices", "any"},
		}},
		{"literals.json", []struct{ field, goType string }{
			{"ANumbers", "[]float64"},
			{"BBools", "[]bool"},
			{"CMixed", "[]any"},
			{"DWithNull", "[]any"},
			{"EAnyItems", "[]string"},
			{"FDuplicates", "[]string"},
			{"GUnionWithArrayFirst", "any"},
			{"ANumChoices", "[]float64"},
			{"BRequiredChoices", "[]string"},
		}},
	}
	for _, fx := range fixtures {
		types := goFieldTypes(generateCode(t, "go", goArraysFixture(fx.file), false)[".go"])
		for _, c := range fx.cases {
			t.Run(fx.file+"/"+c.field, func(t *testing.T) {
				got, ok := types[c.field]
				if !ok {
					t.Fatalf("generated Go has no field %s", c.field)
				}
				if got != c.goType {
					t.Errorf("%s: got type %s, want %s", c.field, got, c.goType)
				}
			})
		}
	}
}