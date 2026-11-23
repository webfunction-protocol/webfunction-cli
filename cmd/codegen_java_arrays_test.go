package cmd

import (
	"regexp"
	"strings"
	"testing"
)

// Per the package spec, when an argument's or attribute's type is (or
// includes) the array type, "choices" / "values" list the allowed ITEMS. In
// Java the choices become a generated enum, so an array field is a
// List<Enum>, not a single enum. The enum's wire type follows the items:
// strings String, whole numbers Long, any non-integer (or an f32/f64
// refinement) Double, booleans Boolean. Items of mixed kinds cannot be an
// enum, so the field keeps the type it would have without choices
// (List<Object> for a bare array), as does a union that mixes an array with
// a scalar (Object, as every multi-type union is in Java). An explicit null
// among the items does not make the array field nullable. A plain number
// with a non-integer choice must not be truncated to a Long.

type javaArrayCase struct {
	field    string   // record component name
	javaType string   // component type
	nullable bool     // annotated @Nullable
	enum     string   // generated enum name
	wire     string   // enum wire type; "" means no enum may exist
	consts   []string // constants the enum must contain
}

func TestJavaArrayChoicesAndValues(t *testing.T) {
	const (
		A = "ThingAttributes"
		G = "GetThingArgs"
		L = "LitsAttributes"
		P = "GetLitsArgs"
		N = "NumsAttributes"
	)
	xy := []string{`X("x")`, `Y("y")`}
	fixtures := []struct {
		file  string
		cases []javaArrayCase
	}{
		{"arrays.json", []javaArrayCase{
			// attributes (values)
			{"aBareValues", "List<" + A + "ABareValuesChoice>", false, A + "ABareValuesChoice", "String", xy},
			{"bNestedValues", "List<" + A + "BNestedValuesChoice>", false, A + "BNestedValuesChoice", "String", xy},
			{"cCanonValues", "List<" + A + "CCanonValuesChoice>", false, A + "CCanonValuesChoice", "String", xy},
			{"dNestedNovalues", "List<String>", false, A + "DNestedNovaluesChoice", "", nil},
			{"eBareNovalues", "List<Object>", false, A + "EBareNovaluesChoice", "", nil},
			{"fNumValues", "List<" + A + "FNumValuesChoice>", false, A + "FNumValuesChoice", "Long", []string{"VALUE_1(1L)", "VALUE_2(2L)"}},
			{"gUnionItemsValues", "List<Object>", false, A + "GUnionItemsValuesChoice", "", nil},
			{"hNullableValues", "List<" + A + "HNullableValuesChoice>", true, A + "HNullableValuesChoice", "String", xy},
			{"iMixedValues", "Object", false, A + "IMixedValuesChoice", "", nil},
			{"jScalarValues", A + "JScalarValuesChoice", false, A + "JScalarValuesChoice", "String", xy},
			// arguments (choices); every argument here is optional
			{"aBareChoices", "List<" + G + "ABareChoicesChoice>", true, G + "ABareChoicesChoice", "String", xy},
			{"bNestedChoices", "List<" + G + "BNestedChoicesChoice>", true, G + "BNestedChoicesChoice", "String", xy},
			{"cNestedNochoices", "List<String>", true, G + "CNestedNochoicesChoice", "", nil},
			{"dScalarChoices", G + "DScalarChoicesChoice", true, G + "DScalarChoicesChoice", "String", xy},
			{"eMixedChoices", "Object", true, G + "EMixedChoicesChoice", "", nil},
		}},
		{"literals.json", []javaArrayCase{
			{"aNumbers", "List<" + L + "ANumbersChoice>", false, L + "ANumbersChoice", "Double", []string{"VALUE_1(1d)", "VALUE_2_5(2.5d)"}},
			{"bBools", "List<" + L + "BBoolsChoice>", false, L + "BBoolsChoice", "Boolean", []string{"TRUE(true)", "FALSE(false)"}},
			{"cMixed", "List<Object>", false, L + "CMixedChoice", "", nil},
			{"dWithNull", "List<" + L + "DWithNullChoice>", false, L + "DWithNullChoice", "String", []string{`A("a")`}},
			{"eAnyItems", "List<" + L + "EAnyItemsChoice>", false, L + "EAnyItemsChoice", "String", xy},
			{"fDuplicates", "List<" + L + "FDuplicatesChoice>", false, L + "FDuplicatesChoice", "String", nil},
			{"gUnionWithArrayFirst", "Object", false, L + "GUnionWithArrayFirstChoice", "", nil},
			{"aNumChoices", "List<" + P + "ANumChoicesChoice>", true, P + "ANumChoicesChoice", "Long", []string{"VALUE_1(1L)", "VALUE_2(2L)"}},
			{"bRequiredChoices", "List<" + P + "BRequiredChoicesChoice>", false, P + "BRequiredChoicesChoice", "String", xy},
		}},
		{"numbers.json", []javaArrayCase{
			{"nInts", N + "NIntsChoice", false, N + "NIntsChoice", "Long", []string{"VALUE_1(1L)", "VALUE_2(2L)"}},
			{"nFrac", N + "NFracChoice", false, N + "NFracChoice", "Double", []string{"VALUE_1(1d)", "VALUE_2_5(2.5d)"}},
			{"nF64", N + "NF64Choice", false, N + "NF64Choice", "Double", []string{"VALUE_1(1d)", "VALUE_2(2d)"}},
			{"nArrInt", "List<" + N + "NArrIntChoice>", false, N + "NArrIntChoice", "Long", []string{"VALUE_1(1L)", "VALUE_2(2L)"}},
			{"nArrF64", "List<" + N + "NArrF64Choice>", false, N + "NArrF64Choice", "Double", []string{"VALUE_1(1d)", "VALUE_2(2d)"}},
			{"nArrFrac", "List<" + N + "NArrFracChoice>", false, N + "NArrFracChoice", "Double", []string{"VALUE_1(1d)", "VALUE_2_5(2.5d)"}},
			// a null alternative in the type does not stop it being a plain array
			{"nArrNullType", "List<" + N + "NArrNullTypeChoice>", false, N + "NArrNullTypeChoice", "Long", []string{"VALUE_1(1L)", "VALUE_2(2L)"}},
		}},
	}
	for _, fx := range fixtures {
		src := generateCode(t, "java", goArraysFixture(fx.file), false)[".java"]
		for _, c := range fx.cases {
			// A record component line: an optional @Nullable, the type, the
			// name. The constructor parameters start with a Jackson
			// annotation and the doc comments between components start with
			// "/**" or "*", so neither matches.
			re := regexp.MustCompile(`(?m)^\s+(@Nullable )?(\S+) ` + regexp.QuoteMeta(c.field) + `,?\r?$`)
			t.Run(fx.file+"/"+c.field, func(t *testing.T) {
				m := re.FindStringSubmatch(src)
				if m == nil {
					t.Fatalf("generated Java has no record component %s", c.field)
				}
				if m[2] != c.javaType {
					t.Errorf("%s: got type %s, want %s", c.field, m[2], c.javaType)
				}
				if (m[1] != "") != c.nullable {
					t.Errorf("%s: @Nullable is %v, want %v", c.field, m[1] != "", c.nullable)
				}
				start := strings.Index(src, "public enum "+c.enum+" {")
				if c.wire == "" {
					if start >= 0 {
						t.Errorf("%s: unexpected enum %s", c.field, c.enum)
					}
					return
				}
				if start < 0 {
					t.Fatalf("%s: no enum %s", c.field, c.enum)
				}
				block := src[start:]
				if end := strings.Index(block, "\n    }"); end >= 0 {
					block = block[:end]
				}
				if !strings.Contains(block, "private final "+c.wire+" wireValue;") {
					t.Errorf("%s: enum %s wire type is not %s", c.field, c.enum, c.wire)
				}
				for _, k := range c.consts {
					if !strings.Contains(block, k) {
						t.Errorf("%s: enum %s has no constant %s", c.field, c.enum, k)
					}
				}
			})
		}
	}
}
