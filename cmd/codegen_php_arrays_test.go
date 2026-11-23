package cmd

import (
	"regexp"
	"testing"
)

// Per the package spec, when an argument's or attribute's type is (or
// includes) the array type, "choices" / "values" list the allowed ITEMS, so
// the PHPDoc type must stay a list of the literal union. For a union that
// mixes an array with a scalar the values may apply to either, so both are
// offered, in union order. A scalar type with choices stays a plain literal
// union, and an array without choices is unchanged (a bare "array" stays
// "array").

func TestPHPArrayChoicesAndValuesKeepTheListWrapper(t *testing.T) {
	fixtures := []struct {
		file  string
		cases []struct{ field, phpType string }
	}{
		{"arrays.json", []struct{ field, phpType string }{
			// attributes (values); only nullable ones are optional
			{"a_bare_values", `list<'x'|'y'>`},
			{"b_nested_values", `list<'x'|'y'>`},
			{"c_canon_values", `list<'x'|'y'>`},
			{"d_nested_novalues", `list<string>`},
			{"e_bare_novalues", `array`},
			{"f_num_values", `list<1|2>`},
			{"g_union_items_values", `list<'a'|1>`},
			{"h_nullable_values?", `list<'x'|'y'>|null`},
			{"i_mixed_values", `'x'|'y'|list<'x'|'y'>`},
			{"j_scalar_values", `'x'|'y'`},
			// arguments (choices); every argument here is optional
			{"a_bare_choices?", `list<'x'|'y'>`},
			{"b_nested_choices?", `list<'x'|'y'>`},
			{"c_nested_nochoices?", `list<string>`},
			{"d_scalar_choices?", `'x'|'y'`},
			{"e_mixed_choices?", `'x'|'y'|list<'x'|'y'>`},
		}},
		{"literals.json", []struct{ field, phpType string }{
			{"a_numbers", `list<1|2.5>`},
			{"b_bools", `list<true|false>`},
			{"c_mixed", `list<'a'|1>`},
			{"d_with_null", `list<'a'|null>`},
			{"e_any_items", `list<'x'|'y'>`},
			{"f_duplicates", `list<'x'>`},
			{"g_union_with_array_first", `list<'x'|'y'>|'x'|'y'`},
			{"a_num_choices?", `list<1|2>`},
			{"b_required_choices", `list<'x'|'y'>`},
		}},
	}
	for _, fx := range fixtures {
		src := generateCode(t, "php", goArraysFixture(fx.file), false)[".php"]
		for _, c := range fx.cases {
			// "field: type" inside an array{...} shape, bounded by the
			// shape's opening brace or a space before and a comma or closing
			// brace after, so a type is matched whole.
			re := regexp.MustCompile(`[{ ]` + regexp.QuoteMeta(c.field+": "+c.phpType) + `[,}]`)
			t.Run(fx.file+"/"+c.field, func(t *testing.T) {
				if !re.MatchString(src) {
					t.Errorf("generated PHP has no shape entry %q", c.field+": "+c.phpType)
				}
			})
		}
	}
}
