package cmd

import (
	"regexp"
	"strings"
	"testing"
)

// Per the package spec, when an argument's or attribute's type is (or
// includes) the array type, "choices" / "values" list the allowed ITEMS. In
// C# the choices become a generated enum with a JsonConverter, so an array
// field is a List<Enum>, not a single enum. The converter's wire type
// follows the items: strings string, whole numbers long, any non-integer
// (or an f32/f64 refinement) double, booleans bool. Items of mixed kinds
// cannot be an enum, so the field keeps the type it would have without
// choices (List<object> for a bare array), as does a union that mixes an
// array with a scalar (object, as every multi-type union is in C#). An
// explicit null among the items does not make the array field nullable, but
// its items are nullable (List<Enum?>), because a JSON null cannot be read
// into a non-nullable enum. A plain number with a non-integer choice must not be truncated to a long.

type csharpArrayCase struct {
	prop   string      // record property name
	csType string      // property type, with a trailing ? when nullable
	enum   string      // generated enum name
	wire   string      // converter wire type; "" means no enum may exist
	consts [][2]string // {wire literal, enum member label}
}

func TestCSharpArrayChoicesAndValues(t *testing.T) {
	const (
		A = "ThingAttributes"
		G = "GetThingArgs"
		L = "LitsAttributes"
		P = "GetLitsArgs"
		N = "NumsAttributes"
	)
	xy := [][2]string{{`"x"`, "X"}, {`"y"`, "Y"}}
	one2 := [][2]string{{"1L", "Value1"}, {"2L", "Value2"}}
	readers := map[string]string{"string": "GetString()", "long": "GetInt64()", "double": "GetDouble()", "bool": "GetBoolean()"}
	fixtures := []struct {
		file  string
		cases []csharpArrayCase
	}{
		{"arrays.json", []csharpArrayCase{
			// attributes (values)
			{"ABareValues", "List<" + A + "ABareValuesChoice>", A + "ABareValuesChoice", "string", xy},
			{"BNestedValues", "List<" + A + "BNestedValuesChoice>", A + "BNestedValuesChoice", "string", xy},
			{"CCanonValues", "List<" + A + "CCanonValuesChoice>", A + "CCanonValuesChoice", "string", xy},
			{"DNestedNovalues", "List<string>", A + "DNestedNovaluesChoice", "", nil},
			{"EBareNovalues", "List<object>", A + "EBareNovaluesChoice", "", nil},
			{"FNumValues", "List<" + A + "FNumValuesChoice>", A + "FNumValuesChoice", "long", one2},
			{"GUnionItemsValues", "List<object>", A + "GUnionItemsValuesChoice", "", nil},
			{"HNullableValues", "List<" + A + "HNullableValuesChoice>?", A + "HNullableValuesChoice", "string", xy},
			{"IMixedValues", "object", A + "IMixedValuesChoice", "", nil},
			{"JScalarValues", A + "JScalarValuesChoice", A + "JScalarValuesChoice", "string", xy},
			// arguments (choices); every argument here is optional
			{"ABareChoices", "List<" + G + "ABareChoicesChoice>?", G + "ABareChoicesChoice", "string", xy},
			{"BNestedChoices", "List<" + G + "BNestedChoicesChoice>?", G + "BNestedChoicesChoice", "string", xy},
			{"CNestedNochoices", "List<string>?", G + "CNestedNochoicesChoice", "", nil},
			{"DScalarChoices", G + "DScalarChoicesChoice?", G + "DScalarChoicesChoice", "string", xy},
			{"EMixedChoices", "object?", G + "EMixedChoicesChoice", "", nil},
		}},
		{"literals.json", []csharpArrayCase{
			{"ANumbers", "List<" + L + "ANumbersChoice>", L + "ANumbersChoice", "double", [][2]string{{"1d", "Value1"}, {"2.5d", "Value2_5"}}},
			{"BBools", "List<" + L + "BBoolsChoice>", L + "BBoolsChoice", "bool", [][2]string{{"true", "True"}, {"false", "False"}}},
			{"CMixed", "List<object>", L + "CMixedChoice", "", nil},
			{"DWithNull", "List<" + L + "DWithNullChoice?>", L + "DWithNullChoice", "string", [][2]string{{`"a"`, "A"}}},
			{"EAnyItems", "List<" + L + "EAnyItemsChoice>", L + "EAnyItemsChoice", "string", xy},
			{"FDuplicates", "List<" + L + "FDuplicatesChoice>", L + "FDuplicatesChoice", "string", [][2]string{{`"x"`, "X"}}},
			{"GUnionWithArrayFirst", "object", L + "GUnionWithArrayFirstChoice", "", nil},
			{"ANumChoices", "List<" + P + "ANumChoicesChoice>?", P + "ANumChoicesChoice", "long", one2},
			{"BRequiredChoices", "List<" + P + "BRequiredChoicesChoice>", P + "BRequiredChoicesChoice", "string", xy},
		}},
		{"numbers.json", []csharpArrayCase{
			{"NInts", N + "NIntsChoice", N + "NIntsChoice", "long", one2},
			{"NFrac", N + "NFracChoice", N + "NFracChoice", "double", [][2]string{{"1d", "Value1"}, {"2.5d", "Value2_5"}}},
			{"NF64", N + "NF64Choice", N + "NF64Choice", "double", [][2]string{{"1d", "Value1"}, {"2d", "Value2"}}},
			{"NArrInt", "List<" + N + "NArrIntChoice>", N + "NArrIntChoice", "long", one2},
			{"NArrF64", "List<" + N + "NArrF64Choice>", N + "NArrF64Choice", "double", [][2]string{{"1d", "Value1"}, {"2d", "Value2"}}},
			{"NArrFrac", "List<" + N + "NArrFracChoice>", N + "NArrFracChoice", "double", [][2]string{{"1d", "Value1"}, {"2.5d", "Value2_5"}}},
			// a null alternative in the type does not stop it being a plain array
			{"NArrNullType", "List<" + N + "NArrNullTypeChoice>", N + "NArrNullTypeChoice", "long", one2},
		}},
	}
	for _, fx := range fixtures {
		src := generateCode(t, "csharp", goArraysFixture(fx.file), false)[".cs"]
		for _, c := range fx.cases {
			// A record property: its [property: ...] attribute, then the
			// type, then the name, with "= null" on an optional one.
			re := regexp.MustCompile(`(?m)\] (\S+) ` + regexp.QuoteMeta(c.prop) + `( = null)?,?\r?$`)
			t.Run(fx.file+"/"+c.prop, func(t *testing.T) {
				m := re.FindStringSubmatch(src)
				if m == nil {
					t.Fatalf("generated C# has no record property %s", c.prop)
				}
				if m[1] != c.csType {
					t.Errorf("%s: got type %s, want %s", c.prop, m[1], c.csType)
				}
				if strings.HasSuffix(c.csType, "?") != (m[2] != "") {
					t.Errorf("%s: optional default (= null) is %v, want %v", c.prop, m[2] != "", strings.HasSuffix(c.csType, "?"))
				}
				if c.wire == "" {
					if strings.Contains(src, "public enum "+c.enum+"\n") {
						t.Errorf("%s: unexpected enum %s", c.prop, c.enum)
					}
					return
				}
				if !strings.Contains(src, "public enum "+c.enum+"\n") {
					t.Fatalf("%s: no enum %s", c.prop, c.enum)
				}
				start := strings.Index(src, "private sealed class "+c.enum+"Converter")
				if start < 0 {
					t.Fatalf("%s: no converter for %s", c.prop, c.enum)
				}
				block := src[start:]
				if end := strings.Index(block, "\n    }"); end >= 0 {
					block = block[:end]
				}
				if !strings.Contains(block, "var wire = reader."+readers[c.wire]+";") {
					t.Errorf("%s: converter for %s does not read a %s", c.prop, c.enum, c.wire)
				}
				for _, k := range c.consts {
					if want := k[0] + " => " + c.enum + "." + k[1] + ","; !strings.Contains(block, want) {
						t.Errorf("%s: converter for %s has no arm %q", c.prop, c.enum, want)
					}
				}
			})
		}
	}
}
