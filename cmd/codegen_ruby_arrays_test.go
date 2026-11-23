package cmd

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// Per the package spec, when an argument's or attribute's type is (or
// includes) the array type, "choices" / "values" list the allowed ITEMS, so
// the RBS type must stay an Array of the literal union, on the wrapper
// class's accessors and on the client's keyword parameters alike. RBS has
// no float literals, so a fractional number stays Float inside the union.
// An explicit null among the items makes the items nilable, not the field;
// an array mixed with a scalar is a union of the two with the values
// offered to both (the permissive reading), in union order. A scalar with
// choices, and an array without choices, are unchanged. An accessor's
// return type is wrapped in parentheses whenever it contains a union.

func TestRubyArrayChoicesAndValuesKeepTheArrayWrapper(t *testing.T) {
	type rubyCase struct {
		field string
		arg   bool // a keyword parameter of the client method, not an accessor
		want  string
	}
	fixtures := []struct {
		file  string
		cases []rubyCase
	}{
		{"arrays.json", []rubyCase{
			// attributes (values): wrapper-class accessors
			{"a_bare_values", false, `(Array["x" | "y"])`},
			{"b_nested_values", false, `(Array["x" | "y"])`},
			{"c_canon_values", false, `(Array["x" | "y"])`},
			{"d_nested_novalues", false, `Array[String]`},
			{"e_bare_novalues", false, `Array[untyped]`},
			{"f_num_values", false, `(Array[1 | 2])`},
			{"g_union_items_values", false, `(Array["a" | 1])`},
			{"h_nullable_values", false, `(Array["x" | "y"] | nil)`},
			{"i_mixed_values", false, `("x" | "y" | Array["x" | "y"])`},
			{"j_scalar_values", false, `("x" | "y")`},
			// arguments (choices): keyword parameters, all optional here
			{"a_bare_choices", true, `Array["x" | "y"]`},
			{"b_nested_choices", true, `Array["x" | "y"]`},
			{"c_nested_nochoices", true, `Array[String]`},
			{"d_scalar_choices", true, `"x" | "y"`},
			{"e_mixed_choices", true, `"x" | "y" | Array["x" | "y"]`},
		}},
		{"literals.json", []rubyCase{
			{"a_numbers", false, `(Array[1 | Float])`},
			{"b_bools", false, `(Array[true | false])`},
			{"c_mixed", false, `(Array["a" | 1])`},
			{"d_with_null", false, `(Array["a" | nil])`},
			{"e_any_items", false, `(Array["x" | "y"])`},
			{"f_duplicates", false, `Array["x"]`},
			{"g_union_with_array_first", false, `(Array["x" | "y"] | "x" | "y")`},
			{"a_num_choices", true, `Array[1 | 2]`},
			{"b_required_choices", true, `Array["x" | "y"]`},
		}},
		{"numbers.json", []rubyCase{
			// a scalar number with values is already a literal union: unchanged
			{"n_ints", false, `(1 | 2)`},
			{"n_frac", false, `(1 | Float)`},
			{"n_f64", false, `(1 | 2)`},
			{"n_arr_int", false, `(Array[1 | 2])`},
			{"n_arr_f64", false, `(Array[1 | 2])`},
			{"n_arr_frac", false, `(Array[1 | Float])`},
			// a null alternative in the type does not stop it being a plain array
			{"n_arr_null_type", false, `(Array[1 | 2])`},
		}},
	}
	for _, fx := range fixtures {
		src := generateCode(t, "ruby", goArraysFixture(fx.file), false)[".rbs"]
		for _, c := range fx.cases {
			// An accessor is "def name: () -> TYPE" on its own line; a
			// keyword parameter is "name: TYPE" between "(" or ", " and the
			// next parameter or the closing ") ->" of the method signature.
			pattern := `(?m)^\s+def ` + regexp.QuoteMeta(c.field) + `: \(\) -> (.+?)\r?$`
			if c.arg {
				pattern = `(?:\(|, )\??` + regexp.QuoteMeta(c.field) + `: (.+?)(?:, \??[a-z_0-9]+: |\) ->)`
			}
			re := regexp.MustCompile(pattern)
			t.Run(fx.file+"/"+c.field, func(t *testing.T) {
				m := re.FindStringSubmatch(src)
				if m == nil {
					t.Fatalf("generated RBS has no %s", c.field)
				}
				if m[1] != c.want {
					t.Errorf("%s: got type %s, want %s", c.field, m[1], c.want)
				}
			})
		}
	}
}

// A nullable field whose values include a null is nilable twice over: its
// items (Array["a" | nil]) and the field itself. The nil inside the Array
// must not be taken for the field's own.
func TestRubyNullableArrayWithNullItemsIsNilableTwice(t *testing.T) {
	const pkg = `{"base_url":"https://api.example.com","name":"Nil","objects":[{"name":"u","attributes":[{"name":"p_both","type":"array","values":["a",null],"flags":["nullable"]}]}],"endpoints":[{"name":"get-u","returns":"object.u"}]}`
	file := filepath.Join(t.TempDir(), "nil.json")
	if err := os.WriteFile(file, []byte(pkg), 0o644); err != nil {
		t.Fatal(err)
	}
	src := generateCode(t, "ruby", file, false)[".rbs"]
	m := regexp.MustCompile(`(?m)^\s+def p_both: \(\) -> (.+?)\r?$`).FindStringSubmatch(src)
	if m == nil {
		t.Fatal("generated RBS has no accessor p_both")
	}
	if want := `(Array["a" | nil] | nil)`; m[1] != want {
		t.Errorf("p_both: got type %s, want %s", m[1], want)
	}
}
