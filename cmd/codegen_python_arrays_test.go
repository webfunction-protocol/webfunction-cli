package cmd

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// Per the package spec, when an argument's or attribute's type is (or
// includes) the array type, "choices" / "values" list the allowed ITEMS, so
// the annotation must stay a list of the Literal. A Literal can mix
// strings, ints and bools, so items of mixed kinds are fine. An explicit
// null among the items makes the items Optional, not the field. An array
// mixed with a scalar is a real Union of the two, with the values offered
// to both (the permissive reading), in union order. PEP 586 forbids floats
// in a Literal, so whole numbers are int members but a fractional item
// makes the field keep the type it would have without choices (the
// allowed values stay in the docstring). A scalar with choices, and an
// array without choices, are unchanged.

func TestPythonArrayChoicesAndValuesKeepTheListWrapper(t *testing.T) {
	fixtures := []struct {
		file  string
		cases []struct{ field, pyType string }
	}{
		{"arrays.json", []struct{ field, pyType string }{
			// attributes (values); only nullable ones are Optional
			{"a_bare_values", `List[Literal["x", "y"]]`},
			{"b_nested_values", `List[Literal["x", "y"]]`},
			{"c_canon_values", `List[Literal["x", "y"]]`},
			{"d_nested_novalues", `List[str]`},
			{"e_bare_novalues", `List[Any]`},
			{"f_num_values", `List[Literal[1, 2]]`},
			{"g_union_items_values", `List[Literal["a", 1]]`},
			{"h_nullable_values", `Optional[List[Literal["x", "y"]]]`},
			{"i_mixed_values", `Union[Literal["x", "y"], List[Literal["x", "y"]]]`},
			{"j_scalar_values", `Literal["x", "y"]`},
			// arguments (choices); every argument here is optional
			{"a_bare_choices", `Optional[List[Literal["x", "y"]]]`},
			{"b_nested_choices", `Optional[List[Literal["x", "y"]]]`},
			{"c_nested_nochoices", `Optional[List[str]]`},
			{"d_scalar_choices", `Optional[Literal["x", "y"]]`},
			{"e_mixed_choices", `Optional[Union[Literal["x", "y"], List[Literal["x", "y"]]]]`},
		}},
		{"literals.json", []struct{ field, pyType string }{
			{"a_numbers", `List[Any]`},
			{"b_bools", `List[Literal[True, False]]`},
			{"c_mixed", `List[Literal["a", 1]]`},
			{"d_with_null", `List[Optional[Literal["a"]]]`},
			{"e_any_items", `List[Literal["x", "y"]]`},
			{"f_duplicates", `List[Literal["x"]]`},
			{"g_union_with_array_first", `Union[List[Literal["x", "y"]], Literal["x", "y"]]`},
			{"a_num_choices", `Optional[List[Literal[1, 2]]]`},
			{"b_required_choices", `List[Literal["x", "y"]]`},
		}},
		{"numbers.json", []struct{ field, pyType string }{
			// a scalar number cannot be a Literal (float): unchanged
			{"n_ints", `float`},
			{"n_frac", `float`},
			{"n_f64", `float`},
			{"n_arr_int", `List[Literal[1, 2]]`},
			{"n_arr_f64", `List[Literal[1, 2]]`},
			{"n_arr_frac", `List[float]`},
			// a null alternative in the type does not stop it being a plain array
			{"n_arr_null_type", `List[Literal[1, 2]]`},
		}},
	}
	for _, fx := range fixtures {
		src := generateCode(t, "python", goArraysFixture(fx.file), false)[".py"]
		for _, c := range fx.cases {
			// "name: TYPE = field(...)" in a dataclass body.
			re := regexp.MustCompile(`(?m)^\s+` + regexp.QuoteMeta(c.field) + `: (.+?) = field\(`)
			t.Run(fx.file+"/"+c.field, func(t *testing.T) {
				m := re.FindStringSubmatch(src)
				if m == nil {
					t.Fatalf("generated Python has no dataclass field %s", c.field)
				}
				if m[1] != c.pyType {
					t.Errorf("%s: got type %s, want %s", c.field, m[1], c.pyType)
				}
			})
		}
	}
}

// An array mixed with two scalar alternatives is still one Union: the
// scalar Literal is offered once, not once per scalar alternative.
func TestPythonArrayChoicesWithTwoScalarAlternativesOffersTheLiteralOnce(t *testing.T) {
	const pkg = `{"base_url":"https://api.example.com","name":"Unions","objects":[{"name":"u","attributes":[{"name":"p_two_scalars","type":["string","number","array"],"values":["x","y"]}]}],"endpoints":[{"name":"get-u","returns":"object.u"}]}`
	file := filepath.Join(t.TempDir(), "unions.json")
	if err := os.WriteFile(file, []byte(pkg), 0o644); err != nil {
		t.Fatal(err)
	}
	src := generateCode(t, "python", file, false)[".py"]
	m := regexp.MustCompile(`(?m)^\s+p_two_scalars: (.+?) = field\(`).FindStringSubmatch(src)
	if m == nil {
		t.Fatal("generated Python has no dataclass field p_two_scalars")
	}
	if want := `Union[Literal["x", "y"], List[Literal["x", "y"]]]`; m[1] != want {
		t.Errorf("p_two_scalars: got type %s, want %s", m[1], want)
	}
}
