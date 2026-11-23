package gogen

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/webfunction-protocol/webfunction-go"
)

// structResolver resolves a named object.<name> reference (in a given
// context, already baked in by the caller) to the name of a generated Go
// struct type. Mirrors jsgen's objectResolver/phpgen's aliasResolver.
type structResolver func(refName string) string

// localStructs holds struct-type names for an endpoint's own inline bare
// object/array attributes (as opposed to a named object.<name>
// reference, which resolves separately via structResolver). Mirrors
// jsgen's localTypedefs/phpgen's localShapes.
type localStructs struct {
	object      string // struct name for a bare "object" return
	arrayOfItem string // type name for each item in a bare "array" return
}

// goType returns the concrete Go type for t. Unlike jsgen's jsdocType or
// phpgen's phpDocType, this IS the type the Go compiler actually
// enforces - there's no separate docs-only annotation layer the way
// JSDoc/PHPStan give JS/PHP, so it has to be exactly right, not just
// documentation.
//
// forceNullable wraps the result in a pointer - used uniformly for both
// an Attribute's "nullable" flag and an Argument's "not required" flag.
// Unlike jsgen/phpgen, which track "optional" (key may be absent) and
// "nullable" (value may be null) as two separate concepts their target's
// docs-only type system can express independently, a Go pointer already
// covers both at once - so the two concepts collapse into this single
// decision here (see structs.go's field type, which only tracks one
// "optional" flag for this reason).
//
// A pointer is only added around a scalar or named-struct base type -
// wrapping a slice/map/`any` in a pointer would be redundant noise,
// since those are already nil-able in Go.
func goType(t webfunction.Type, local localStructs, forceNullable bool, resolve structResolver) string {
	base := goBaseType(t, local, resolve)
	if !forceNullable && !t.HasBase("null") {
		return base
	}
	if base == "any" || strings.HasPrefix(base, "[]") || strings.HasPrefix(base, "map[") {
		return base
	}
	return "*" + base
}

// goBaseType collapses a Type's union to a single concrete Go type.
//
// Go has no type-union mechanism (unlike PHP's real unions or
// TypeScript's), so a union with more than one non-null alternative
// can't be expressed precisely - it falls back to `any`, same as an
// unrecognized/"any" base would. This is a genuine expressive-power gap
// versus the PHP/JS targets, not a simplification chosen for
// convenience - see the package doc comment in gogen.go.
func goBaseType(t webfunction.Type, local localStructs, resolve structResolver) string {
	var nonNull []webfunction.TypeAlt
	for _, alt := range t.Union {
		if alt.Base != "null" {
			nonNull = append(nonNull, alt)
		}
	}
	if len(nonNull) != 1 {
		return "any"
	}
	return goAltType(nonNull[0], local, resolve)
}

func goAltType(alt webfunction.TypeAlt, local localStructs, resolve structResolver) string {
	switch alt.Base {
	case "string":
		return "string"
	case "boolean":
		return "bool"
	case "number":
		// Go has real sized numeric types, unlike PHP (which collapses
		// every refinement to int|float since PHP's own int/float
		// aren't sized) - u32/u64/i32/i64/f32/f64 map to their exact Go
		// equivalent, giving stronger typing than any other target in
		// this suite can offer for these fields.
		switch alt.Refinement {
		case "u32":
			return "uint32"
		case "u64":
			return "uint64"
		case "i32":
			return "int32"
		case "i64":
			return "int64"
		case "timestamp":
			return "int64"
		case "f32":
			return "float32"
		case "f64":
			return "float64"
		default:
			// No refinement (or one not in the confirmed vocabulary) -
			// JSON's "number" doesn't distinguish int from float on its
			// own; float64 is the same safe superset encoding/json
			// itself defaults to when decoding into `any`.
			return "float64"
		}
	case "array":
		if alt.Of != nil {
			// An item type never carries choices of its own: when a
			// field has choices/values, narrowArrayItems has already
			// filled in the items of an unspecified array from them.
			return "[]" + goType(*alt.Of, localStructs{}, false, resolve)
		}
		if local.arrayOfItem != "" {
			return "[]" + local.arrayOfItem
		}
		return "[]any"
	case "object":
		if alt.IsObjectRef() && resolve != nil {
			return resolve(alt.Refinement)
		}
		if local.object != "" {
			return local.object
		}
		return "map[string]any"
	default: // "any", or anything unrecognized
		return "any"
	}
}

// narrowArrayItems fills in the item type of an array whose items the
// type leaves unspecified (a bare "array", or the equivalent [["any"]])
// from the field's choices/values. Per the spec those are the allowed
// items, so when every one is a string, a number or a boolean the items
// are known to be that type. Mixed items, an explicit null and an item
// type the package spells out are left alone, as is every non-array
// alternative. t itself is never modified.
func narrowArrayItems(t webfunction.Type, choices []interface{}) webfunction.Type {
	base := choiceItemBase(choices)
	if base == "" {
		return t
	}
	narrowed := webfunction.Type{Union: make([]webfunction.TypeAlt, len(t.Union))}
	copy(narrowed.Union, t.Union)
	for i := range narrowed.Union {
		if alt := narrowed.Union[i]; alt.Base == "array" && itemsUnspecified(alt) {
			narrowed.Union[i].Of = &webfunction.Type{Union: []webfunction.TypeAlt{{Base: base}}}
		}
	}
	return narrowed
}

// itemsUnspecified reports whether an array alternative says nothing
// about its items: a bare "array" (no Of), or an explicit array of any.
func itemsUnspecified(alt webfunction.TypeAlt) bool {
	if alt.Of == nil {
		return true
	}
	return len(alt.Of.Union) == 1 && alt.Of.Union[0].Base == "any"
}

// choiceItemBase returns the JSON base type ("string", "number" or
// "boolean") that every entry of choices has, or "" if there are none or
// they differ (an explicit null counts as different).
func choiceItemBase(choices []interface{}) string {
	base := ""
	for _, c := range choices {
		var b string
		switch c.(type) {
		case string:
			b = "string"
		case float64: // encoding/json decodes all JSON numbers as float64
			b = "number"
		case bool:
			b = "boolean"
		default:
			return ""
		}
		if base != "" && b != base {
			return ""
		}
		base = b
	}
	return base
}

// choicesNote renders a closed set of legal values (an Attribute's
// "values" or an Argument's "choices", per spec) as a short doc-comment
// note, e.g. `one of: "active", "inactive", "pending"`.
//
// Unlike jsgen/phpgen, which encode choices as an enforced literal-union
// type (TypeScript/PHPStan can both express and check "active"|
// "inactive" as a real type), Go has no literal-union type mechanism at
// all - a doc note is the most this target can do without generating a
// distinct named type plus constants (a real, more involved enhancement
// - not attempted here; the field keeps its plain base type, apart from
// the array item inference in narrowArrayItems). Returns ""
// if choices is empty.
func choicesNote(choices []interface{}) string {
	if len(choices) == 0 {
		return ""
	}
	parts := make([]string, 0, len(choices))
	for _, c := range choices {
		switch v := c.(type) {
		case nil:
			parts = append(parts, "null")
		case string:
			parts = append(parts, strconv.Quote(v))
		case bool:
			parts = append(parts, strconv.FormatBool(v))
		case float64: // encoding/json decodes all JSON numbers as float64
			parts = append(parts, formatNumberLiteral(v))
		default:
			parts = append(parts, fmt.Sprintf("%v", v))
		}
	}
	return "one of: " + strings.Join(parts, ", ")
}

func formatNumberLiteral(f float64) string {
	if f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// refinementNote turns any dotted refinements in a type (e.g. "email" in
// "string.email") into a short parenthetical note to append to a docs
// description, e.g. "(email format)". Mirrors jsgen/phpgen's
// refinementNote exactly. Returns "" if there's nothing worth noting.
func refinementNote(t webfunction.Type) string {
	var notes []string
	for _, alt := range t.Union {
		if alt.Refinement != "" && alt.Base != "object" {
			notes = append(notes, alt.Refinement)
		}
	}
	if len(notes) == 0 {
		return ""
	}
	return "(" + strings.Join(notes, ", ") + " format)"
}

// withNotes appends any non-empty parenthetical notes to a docs string.
func withNotes(docs string, notes ...string) string {
	docs = strings.TrimSpace(docs)
	for _, n := range notes {
		if n == "" {
			continue
		}
		if docs == "" {
			docs = n
		} else {
			docs = docs + " " + n
		}
	}
	return docs
}
