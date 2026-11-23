package csharpgen

import (
	"strconv"
	"strings"

	"github.com/webfunction-protocol/webfunction-go"
)

// typeResolver resolves a named object.<n> reference (in a given context,
// already baked in by the caller) to the name of a generated C# record
// type. Mirrors gogen's structResolver/javagen's typeResolver.
type typeResolver func(refName string) string

// localTypes holds type names for an endpoint's own inline bare
// object/array attributes (as opposed to a named object.<n> reference,
// which resolves separately via typeResolver). Mirrors gogen's
// localStructs/javagen's localTypes.
type localTypes struct {
	object      string // record type name for a bare "object" return
	arrayOfItem string // item type name for a bare "array" return
}

// csharpType returns the concrete C# type expression for t, with a
// trailing "?" appended if the field is optional/nullable.
//
// Unlike javagen (which needs a separate "boxed" class name - a Java
// primitive can't be null or annotated, so a nullable int needs the
// wrapper class Integer instead), C#'s nullable-annotation syntax is
// uniform: "int?" and "string?" both just work, for value types AND
// reference types alike, with nullable reference types enabled project-
// wide (matching webfunction-csharp's own <Nullable>enable</Nullable>).
// This removes an entire axis of complexity javagen has to carry.
//
// forceNullable is set uniformly for both an Attribute's "nullable" flag
// and an Argument's "not required" flag - same collapsing gogen/javagen
// do, since C# (like Go and Java) only has one real mechanism to cover
// both "key may be absent" and "value may be null" at the type level.
func csharpType(t webfunction.Type, local localTypes, forceNullable bool, resolve typeResolver) string {
	base := csharpBaseType(t, local, resolve)
	nullable := forceNullable || t.HasBase("null")
	if nullable {
		return base + "?"
	}
	return base
}

// csharpBaseType collapses a Type's union to a single concrete C# type.
//
// C# has no real type-union mechanism (no stable discriminated-union
// feature as of C# 12/.NET 8 without an external source generator) - same
// expressive-power gap gogen/javagen hit for Go/Java. A union with more
// than one non-null alternative falls back to object, same as an
// unrecognized/"any" base would.
func csharpBaseType(t webfunction.Type, local localTypes, resolve typeResolver) string {
	var nonNull []webfunction.TypeAlt
	for _, alt := range t.Union {
		if alt.Base != "null" {
			nonNull = append(nonNull, alt)
		}
	}
	if len(nonNull) != 1 {
		return "object"
	}
	return csharpAltType(nonNull[0], local, resolve)
}

func csharpAltType(alt webfunction.TypeAlt, local localTypes, resolve typeResolver) string {
	switch alt.Base {
	case "string":
		return "string"
	case "boolean":
		return "bool"
	case "number":
		// Unlike Java (no unsigned integer types at all), C# has real
		// sized unsigned types - uint/ulong map exactly to u32/u64, the
		// same precision gogen gets from Go's uint32/uint64, and a real
		// improvement over javagen's u32/u64-both-fall-back-to-long
		// compromise.
		switch alt.Refinement {
		case "u32":
			return "uint"
		case "i32":
			return "int"
		case "u64":
			return "ulong"
		case "i64", "timestamp":
			return "long"
		case "f32":
			return "float"
		case "f64":
			return "double"
		default:
			// No refinement (or one outside the confirmed vocabulary) -
			// JSON's "number" doesn't distinguish int from float on its
			// own; double is the same safe superset every other target's
			// fallback uses (matches System.Text.Json's own default
			// numeric decoding into object, and webfunction-csharp's own
			// Json.ToClr fallback).
			return "double"
		}
	case "array":
		if alt.Of != nil {
			// Array item types have no choices concept of their own -
			// choices/values apply to the field as a whole, not each
			// element - so no enum resolution happens for the item type,
			// only ordinary csharpType recursion (mirrors gogen/javagen).
			//
			// Unlike javagen (which has to force the boxed form here
			// regardless of the item's own nullability, because Java
			// generics can never hold a primitive - List<boolean> is a
			// compile error), C# generics genuinely can hold a value type
			// directly (List<int> is completely legal) - so the item's
			// real nullability is used as-is here, with no structural
			// workaround needed. A concrete simplification C# gets for
			// free that Java's target language couldn't.
			item := csharpType(*alt.Of, localTypes{}, false, resolve)
			return "List<" + item + ">"
		}
		if local.arrayOfItem != "" {
			return "List<" + local.arrayOfItem + ">"
		}
		return "List<object>"
	case "object":
		if alt.IsObjectRef() && resolve != nil {
			return resolve(alt.Refinement)
		}
		if local.object != "" {
			return local.object
		}
		return "Dictionary<string, object?>"
	default: // "any", or anything unrecognized
		return "object"
	}
}

// choiceWireType picks which C# type a generated enum's wire value should
// use for a scalar field, based on the field's own declared type rather
// than by sniffing the choices/values list, keeping a choices-enum's wire
// representation consistent with what the field's type would have been
// without choices at all. The one thing it does look at is whether a
// number's choices include a fraction, see numberWireType. Array fields
// use itemWireType instead. Mirrors javagen's choiceWireType.
func choiceWireType(t webfunction.Type, choices []interface{}) string {
	for _, alt := range t.Union {
		switch alt.Base {
		case "string":
			return "string"
		case "boolean":
			return "bool"
		case "number":
			return numberWireType(alt.Refinement, choices)
		}
	}
	return "string"
}

// numberWireType picks double for a number with an f32/f64 refinement or
// with any non-integer choice (a long wire value would truncate 2.5 to
// 2L), and long otherwise.
func numberWireType(refinement string, choices []interface{}) string {
	if refinement == "f32" || refinement == "f64" {
		return "double"
	}
	for _, c := range choices {
		if v, ok := c.(float64); ok && v != float64(int64(v)) {
			return "double"
		}
	}
	return "long"
}

// isSingleArray reports whether t is an array and nothing else (a null
// alternative aside). An array mixed with a scalar is a multi-type union,
// which is object in C#, and a closed set of values cannot be an enum
// that stands for both.
func isSingleArray(t webfunction.Type) bool {
	n, array := 0, false
	for _, alt := range t.Union {
		if alt.Base == "null" {
			continue
		}
		n++
		if alt.Base == "array" {
			array = true
		}
	}
	return n == 1 && array
}

// itemWireType picks the wire type of the enum for the items of an array
// field from its choices/values: string, bool, long or double (see
// numberWireType; an f32/f64 refinement on the declared item type
// counts). A null among the items is skipped, as in an enum. It returns
// "" when there is nothing to build an enum from: no non-null choices, or
// items of mixed kinds, which a C# enum cannot represent. Mirrors
// javagen's itemWireType.
func itemWireType(t webfunction.Type, choices []interface{}) string {
	kind := ""
	for _, c := range choices {
		var k string
		switch c.(type) {
		case nil:
			continue
		case string:
			k = "string"
		case bool:
			k = "bool"
		case float64:
			k = "number"
		default:
			return ""
		}
		if kind != "" && k != kind {
			return ""
		}
		kind = k
	}
	if kind != "number" {
		return kind
	}
	refinement := ""
	for _, alt := range t.Union {
		if alt.Base == "array" && alt.Of != nil {
			for _, item := range alt.Of.Union {
				if item.Base == "number" {
					refinement = item.Refinement
				}
			}
		}
	}
	return numberWireType(refinement, choices)
}

// choiceConstantLabel derives a PascalCase C# enum member name from one
// raw choice/value. Numbers get a "Value" prefix (a bare numeral isn't a
// legal C# identifier); everything else is PascalCased via splitWords -
// mirrors javagen's choiceConstantLabel, adapted to C#'s PascalCase enum-
// member convention (Java's is SCREAMING_SNAKE_CASE).
func choiceConstantLabel(v interface{}) string {
	switch val := v.(type) {
	case string:
		label := pascalCase(val)
		if label == "" {
			return "Empty"
		}
		if label[0] >= '0' && label[0] <= '9' {
			label = "Value" + label
		}
		return label
	case float64: // encoding/json decodes all JSON numbers as float64
		return "Value" + formatNumberLabel(val)
	case bool:
		if val {
			return "True"
		}
		return "False"
	default:
		return "Value"
	}
}

// choiceWireLiteral renders one raw choice/value as a C# literal
// expression matching wireType (see choiceWireType) - used as the wire-
// value argument passed to a generated converter's mapping table.
func choiceWireLiteral(v interface{}, wireType string) string {
	switch val := v.(type) {
	case string:
		return strconv.Quote(val)
	case bool:
		return strconv.FormatBool(val)
	case float64:
		if wireType == "double" {
			return strconv.FormatFloat(val, 'g', -1, 64) + "d"
		}
		return strconv.FormatInt(int64(val), 10) + "L"
	default:
		return "null"
	}
}

func formatNumberLabel(f float64) string {
	if f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strings.ReplaceAll(strconv.FormatFloat(f, 'g', -1, 64), ".", "_")
}

// refinementNote turns any dotted refinements in a type (e.g. "email" in
// "string.email") into a short parenthetical note to append to an XML doc
// description, e.g. "(email format)". Mirrors gogen's/javagen's
// refinementNote exactly.
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
