package javagen

import (
	"strconv"
	"strings"

	"github.com/webfunction-protocol/webfunction-go"
)

// typeResolver resolves a named object.<n> reference (in a given
// context, already baked in by the caller) to the name of a generated
// Java record type. Mirrors gogen's structResolver.
type typeResolver func(refName string) string

// localTypes holds type names for an endpoint's own inline bare
// object/array attributes (as opposed to a named object.<n> reference,
// which resolves separately via typeResolver). Mirrors gogen's
// localStructs.
type localTypes struct {
	object      string // record type name for a bare "object" return
	arrayOfItem string // item type name for a bare "array" return
}

// baseType describes one concrete Java type: its normal (possibly
// primitive) rendering, whether that rendering is a primitive, and - if
// so - its boxed equivalent. Only primitives need a boxed form: a
// primitive can't be annotated @Nullable or hold null, so an optional or
// nullable field of a primitive-mapped type has to use the boxed
// wrapper class instead. Every other Java type (String, a record,
// List<T>, Map<K,V>, an enum) is already a reference type and can be
// null/annotated as-is.
type baseType struct {
	expr      string
	primitive bool
	boxed     string
}

// javaType returns the concrete Java type expression for t, already
// resolved to its boxed form if the field is optional/nullable and the
// underlying type is a primitive. Unlike gogen's goType (which wraps a
// nullable scalar in a pointer), there's no separate wrapper syntax here -
// nullability is expressed by using the boxed class directly (Integer
// instead of int) plus a separate @Nullable annotation the caller adds
// when rendering the field declaration (see records.go).
//
// forceNullable is set uniformly for both an Attribute's "nullable" flag
// and an Argument's "not required" flag - same collapsing gogen does,
// since Java (like Go) only has one real mechanism (nullable reference /
// boxed wrapper) to cover both "key may be absent" and "value may be
// null" at the type level.
func javaType(t webfunction.Type, local localTypes, forceNullable bool, resolve typeResolver) string {
	base := javaBaseType(t, local, resolve)
	nullable := forceNullable || t.HasBase("null")
	if nullable && base.primitive {
		return base.boxed
	}
	return base.expr
}

// javaBaseType collapses a Type's union to a single concrete Java type.
//
// Java has no type-union mechanism (unlike PHP's real unions or
// TypeScript's) - same expressive-power gap gogen hit for Go. A union
// with more than one non-null alternative falls back to Object, same as
// an unrecognized/"any" base would.
func javaBaseType(t webfunction.Type, local localTypes, resolve typeResolver) baseType {
	var nonNull []webfunction.TypeAlt
	for _, alt := range t.Union {
		if alt.Base != "null" {
			nonNull = append(nonNull, alt)
		}
	}
	if len(nonNull) != 1 {
		return baseType{expr: "Object"}
	}
	return javaAltType(nonNull[0], local, resolve)
}

func javaAltType(alt webfunction.TypeAlt, local localTypes, resolve typeResolver) baseType {
	switch alt.Base {
	case "string":
		return baseType{expr: "String"}
	case "boolean":
		return baseType{expr: "boolean", primitive: true, boxed: "Boolean"}
	case "number":
		// Unlike Go, Java has no unsigned integer types at all - u32/u64
		// both fall back to the closest available signed primitive
		// (long), which covers the full u32 range exactly but can't
		// represent u64 values above Long.MAX_VALUE precisely. A real,
		// flagged imprecision versus gogen's uint32/uint64 - not fixable
		// without a BigInteger-based field, which was judged more
		// friction than the actual gap warrants for a v1.
		switch alt.Refinement {
		case "u32":
			return baseType{expr: "long", primitive: true, boxed: "Long"}
		case "i32":
			return baseType{expr: "int", primitive: true, boxed: "Integer"}
		case "u64":
			return baseType{expr: "long", primitive: true, boxed: "Long"}
		case "i64", "timestamp":
			return baseType{expr: "long", primitive: true, boxed: "Long"}
		case "f32":
			return baseType{expr: "float", primitive: true, boxed: "Float"}
		case "f64":
			return baseType{expr: "double", primitive: true, boxed: "Double"}
		default:
			// No refinement (or one outside the confirmed vocabulary) -
			// JSON's "number" doesn't distinguish int from float on its
			// own; double is the same safe superset Jackson itself
			// defaults to when decoding into Object, matching gogen's
			// float64 fallback reasoning exactly.
			return baseType{expr: "double", primitive: true, boxed: "Double"}
		}
	case "array":
		if alt.Of != nil {
			// Array item types have no choices concept of their own -
			// choices/values apply to the field as a whole, not each
			// element - so no enum resolution happens for the item type,
			// only ordinary javaType recursion (mirrors gogen exactly).
			//
			// forceNullable is passed as true here regardless of the
			// item's own nullability - NOT because array items are
			// nullable, but because Java generics can never hold a
			// primitive at all (List<boolean> is a compile error,
			// independent of whether individual items can be null) -
			// javaType's forceNullable param happens to be the
			// mechanism that already boxes a primitive, so it's reused
			// here for an unrelated reason: forcing the boxed form
			// unconditionally. A real bug (found by compiling
			// generated output against an actual package, not by
			// gofmt/go vet/the fixture test - the fixture never
			// happened to include a boolean/numeric array field) - see
			// package doc comment for the fuller writeup.
			item := javaType(*alt.Of, localTypes{}, true, resolve)
			return baseType{expr: "List<" + item + ">"}
		}
		if local.arrayOfItem != "" {
			return baseType{expr: "List<" + local.arrayOfItem + ">"}
		}
		return baseType{expr: "List<Object>"}
	case "object":
		if alt.IsObjectRef() && resolve != nil {
			return baseType{expr: resolve(alt.Refinement)}
		}
		if local.object != "" {
			return baseType{expr: local.object}
		}
		return baseType{expr: "Map<String, Object>"}
	default: // "any", or anything unrecognized
		return baseType{expr: "Object"}
	}
}

// choiceWireType picks which Java type a generated enum's wire value
// (its @JsonValue/@JsonCreator payload) should use for a scalar field,
// based on the field's own declared type rather than by sniffing the
// choices/values list (which Java's static enum can't represent as a mix
// anyway), keeping a choices-enum's wire representation consistent with
// what the field's type would have been without choices at all. The one
// thing it does look at is whether a number's choices include a fraction,
// see numberWireType. Array fields use itemWireType instead.
func choiceWireType(t webfunction.Type, choices []interface{}) string {
	for _, alt := range t.Union {
		switch alt.Base {
		case "string":
			return "String"
		case "boolean":
			return "Boolean"
		case "number":
			return numberWireType(alt.Refinement, choices)
		}
	}
	return "String"
}

// numberWireType picks Double for a number with an f32/f64 refinement or
// with any non-integer choice (a Long wire value would truncate 2.5 to
// 2L), and Long otherwise.
func numberWireType(refinement string, choices []interface{}) string {
	if refinement == "f32" || refinement == "f64" {
		return "Double"
	}
	for _, c := range choices {
		if v, ok := c.(float64); ok && v != float64(int64(v)) {
			return "Double"
		}
	}
	return "Long"
}

// isSingleArray reports whether t is an array and nothing else (a null
// alternative aside). Java has no unions, so an array mixed with a scalar
// is not one: it collapses to Object, and a closed set of values cannot be
// an enum that stands for both.
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
// field from its choices/values: String, Boolean, Long or Double (see
// numberWireType; an f32/f64 refinement on the declared item type counts).
// A null among the items is skipped, as in an enum. It returns "" when
// there is nothing to build an enum from: no non-null choices, or items of
// mixed kinds, which Java's static enum cannot represent.
func itemWireType(t webfunction.Type, choices []interface{}) string {
	kind := ""
	for _, c := range choices {
		var k string
		switch c.(type) {
		case nil:
			continue
		case string:
			k = "String"
		case bool:
			k = "Boolean"
		case float64:
			k = "Number"
		default:
			return ""
		}
		if kind != "" && k != kind {
			return ""
		}
		kind = k
	}
	if kind != "Number" {
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

// choiceConstantLabel derives a SCREAMING_SNAKE_CASE Java enum constant
// name from one raw choice/value. Numbers get a "VALUE_" prefix (a bare
// numeral isn't a legal Java identifier); everything else is
// upper-snake-cased via the same splitWords gogen-style word-splitting
// used for type/method names.
func choiceConstantLabel(v interface{}) string {
	switch val := v.(type) {
	case string:
		words := splitWords(val)
		if len(words) == 0 {
			return "EMPTY"
		}
		upper := make([]string, len(words))
		for i, w := range words {
			upper[i] = strings.ToUpper(w)
		}
		label := strings.Join(upper, "_")
		if len(label) > 0 && (label[0] >= '0' && label[0] <= '9') {
			label = "VALUE_" + label
		}
		return label
	case float64: // encoding/json decodes all JSON numbers as float64
		return "VALUE_" + formatNumberLiteral(val)
	case bool:
		if val {
			return "TRUE"
		}
		return "FALSE"
	default:
		return "VALUE"
	}
}

// choiceWireLiteral renders one raw choice/value as a Java literal
// expression matching wireType (see choiceWireType) - used as the
// argument to each enum constant's constructor.
func choiceWireLiteral(v interface{}, wireType string) string {
	switch val := v.(type) {
	case string:
		return strconv.Quote(val)
	case bool:
		return strconv.FormatBool(val)
	case float64:
		if wireType == "Double" {
			return strconv.FormatFloat(val, 'g', -1, 64) + "d"
		}
		return strconv.FormatInt(int64(val), 10) + "L"
	default:
		return "null"
	}
}

func formatNumberLiteral(f float64) string {
	if f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strings.ReplaceAll(strconv.FormatFloat(f, 'g', -1, 64), ".", "_")
}

// javaBoxedForGeneric returns t's boxed form if t is one of the raw
// primitive type names javaType can produce (boolean/int/long/float/
// double), or t unchanged otherwise. Needed anywhere a Java type
// expression is placed inside a generic type argument (e.g.
// TypeReference<T>) - unlike a method's own declared return type
// (where a primitive is fine, and even preferable), a primitive is
// never legal as a generic type parameter itself. See javagen.go's
// writeMethodBody, and the array-item handling above for the same
// underlying constraint hit a different way.
func javaBoxedForGeneric(t string) string {
	switch t {
	case "boolean":
		return "Boolean"
	case "int":
		return "Integer"
	case "long":
		return "Long"
	case "float":
		return "Float"
	case "double":
		return "Double"
	default:
		return t
	}
}

// refinementNote turns any dotted refinements in a type (e.g. "email" in
// "string.email") into a short parenthetical note to append to a Javadoc
// description, e.g. "(email format)". Mirrors gogen's refinementNote
// exactly.
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
