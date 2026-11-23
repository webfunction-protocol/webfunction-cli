package cmd

import (
	"path/filepath"
	"strings"
	"testing"
)

// Per the package spec, when an argument's or attribute's type is (or
// includes) the array type, "choices" / "values" list the allowed ITEMS, so
// the JSDoc type must stay an array of the literal union. For a union that
// mixes an array with a scalar, the values may apply to either, so both are
// offered. A scalar type with choices stays a plain literal union, and an
// array without choices is unchanged.

func jsArraysFixture() string {
	return filepath.Join("..", "testdata", "codegen", "arrays", "arrays.json")
}

func TestJSArrayChoicesAndValuesKeepTheArrayWrapper(t *testing.T) {
	src := generateCode(t, "js", jsArraysFixture(), false)[".js"]

	have := map[string]bool{}
	for _, l := range strings.Split(src, "\n") {
		l = strings.TrimSpace(strings.TrimRight(l, "\r"))
		have[strings.TrimPrefix(l, "* ")] = true
	}

	cases := []struct{ field, jsType string }{
		// attributes (values)
		{"a_bare_values", `Array<"x"|"y">`},
		{"b_nested_values", `Array<"x"|"y">`},
		{"c_canon_values", `Array<"x"|"y">`},
		{"d_nested_novalues", `Array<string>`},
		{"e_bare_novalues", `Array<any>`},
		{"f_num_values", `Array<1|2>`},
		{"g_union_items_values", `Array<"a"|1>`},
		{"[h_nullable_values]", `(Array<"x"|"y">|null)`}, // nullable attributes are optional
		{"i_mixed_values", `("x"|"y"|Array<"x"|"y">)`},
		{"j_scalar_values", `("x"|"y")`},
		// arguments (choices); every argument here is optional
		{"[a_bare_choices]", `Array<"x"|"y">`},
		{"[b_nested_choices]", `Array<"x"|"y">`},
		{"[c_nested_nochoices]", `Array<string>`},
		{"[d_scalar_choices]", `("x"|"y")`},
		{"[e_mixed_choices]", `("x"|"y"|Array<"x"|"y">)`},
	}
	for _, c := range cases {
		want := "@property {" + c.jsType + "} " + c.field
		t.Run(strings.Trim(c.field, "[]"), func(t *testing.T) {
			if !have[want] {
				t.Errorf("generated JS has no line %q", want)
			}
		})
	}
}