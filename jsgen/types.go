package jsgen

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/webfunction-protocol/webfunction-go"
)

// objectResolver looks up (creating if needed) the typedef name for a
// referenced object.<name>, given the name that followed "object.".
type objectResolver func(refName string) string

// localTypedefs holds typedefs derived from an endpoint's own inline
// attributes (as opposed to a named object.<name> reference, which
// resolves separately via objectResolver). Per spec, an endpoint's
// `attributes` describes the shape of its `returns` when that's the bare
// `object` type. In practice, packages also use it to describe each
// item's shape when `returns` is a bare, untyped `array` - the spec's
// letter doesn't cover that case, but it's what the attributes are
// clearly there for, so it's honored here too.
//
// paginated/pageItemType are a separate case: webfunction-js (confirmed
// from its real source at github.com/webfunction-protocol/webfunction-js) auto-wraps
// a canonical {previous, page, next} response in its own `Page` class
// rather than handing back the raw envelope - matching what the
// PHP/Ruby clients already do. So a paginated endpoint's return type is
// `Page` itself, not a typedef of the envelope shape; pageItemType is
// only the JSDoc type string describing what `.page` holds (e.g.
// "Array<PersonAttributes>"), for the @returns description - not
// something the type system itself narrows, since nothing confirms
// whether the real Page class is declared generic (e.g. `Page<T>`). Not
// inventing that pending being able to check the actual class source.
// localTypedefs holds typedefs derived from an endpoint's own inline
// attributes (as opposed to a named object.<name> reference, which
// resolves separately via objectResolver). Per spec, an endpoint's
// `attributes` describes the shape of its `returns` when that's the bare
// `object` type. In practice, packages also use it to describe each
// item's shape when `returns` is a bare, untyped `array` - the spec's
// letter doesn't cover that case, but it's what the attributes are
// clearly there for, so it's honored here too.
//
// pageTypedef is a separate case, for paginated endpoints: the name of a
// synthesized <Name>Page typedef (see typedefSet.addPageTypedef) that
// describes the real webfunction-js Page instance the endpoint actually
// returns, but with a real item type on `.page` instead of the `any` the
// real (untyped, plain-JS) Page class itself infers to.
type localTypedefs struct {
	object      string // shape of a bare "object" return
	arrayOfItem string // shape of each item in a bare "array" return
	pageTypedef string // name of the synthesized Page typedef, if paginated
}

// jsdocAlt maps a single type alternative to its JSDoc equivalent.
// local supplies typedefs for the endpoint's own inline shapes (see
// localTypedefs); resolve is used for an actual object.<name> reference.
func jsdocAlt(alt webfunction.TypeAlt, local localTypedefs, resolve objectResolver) string {
	switch alt.Base {
	case "object":
		if alt.IsObjectRef() && resolve != nil {
			return resolve(alt.Refinement)
		}
		if local.object != "" {
			return local.object
		}
		return "Object"
	case "array":
		if alt.Of != nil {
			// An item type never carries choices of its own: when a field
			// has choices/values, jsdocType applies them to the items of
			// its array alternatives (see choiceParts).
			return "Array<" + jsdocType(*alt.Of, localTypedefs{}, false, resolve, nil) + ">"
		}
		if local.arrayOfItem != "" {
			return "Array<" + local.arrayOfItem + ">"
		}
		return "Array<any>"
	case "string", "number", "boolean", "null":
		return alt.Base
	default: // "any", or anything unrecognized
		return "any"
	}
}

// jsdocType builds the JSDoc type string for an Argument or Attribute's
// Type, given typedefs for the endpoint's own inline shapes (see
// localTypedefs; pass the zero value when there are none, e.g. when
// rendering a typedef's own fields), whether the value may additionally
// be null on top of whatever the type itself says, a resolver for any
// object.<name> references encountered, and any closed set of legal
// values (an Attribute's "values" or Argument's "choices"; pass nil when
// there are none, e.g. for an endpoint's own Returns type, which has no
// choices concept).
//
// When choices is non-empty and the type has no array alternative, it
// takes over the type entirely: the result is a JSDoc literal union of the
// exact values (e.g. ("active"|"pending")) rather than the bare base type
// (e.g. "string") - since the base type alone doesn't add anything a
// literal union doesn't already convey, and the literal union is what
// actually lets an editor or tsc catch a typo or an out-of-set value at
// the call site. When the type is or includes an array, the spec makes the
// choices the allowed ITEMS of that array, so see choiceParts.
func jsdocType(t webfunction.Type, local localTypedefs, forceNullable bool, resolve objectResolver, choices []interface{}) string {
	var parts []string
	if lits := literalUnion(choices); len(lits) > 0 {
		parts = choiceParts(t, lits)
	} else {
		parts = make([]string, 0, len(t.Union))
		for _, alt := range t.Union {
			parts = append(parts, jsdocAlt(alt, local, resolve))
		}
	}

	if forceNullable || t.HasBase("null") {
		parts = append(parts, "null")
	}

	parts = dedupe(parts)

	if len(parts) == 1 {
		return parts[0]
	}
	return "(" + strings.Join(parts, "|") + ")"
}

// choiceParts renders the alternatives of a type that carries a closed set
// of legal values, lits being the literals from literalUnion. Per the spec,
// when the type is (or includes) the array type, the values are the allowed
// items of the array, so an array alternative becomes Array<literals>. A
// union may mix an array with a scalar (e.g. ["string", "array"]), and the
// spec does not say which the values belong to, so each scalar alternative
// keeps the literals as well. An object alternative has no literals to
// offer and is left out, as it always has been when choices are present;
// null is added by the caller. With no array alternative the literals
// stand for the whole type.
func choiceParts(t webfunction.Type, lits []string) []string {
	if !t.HasBase("array") {
		return lits
	}
	items := "Array<" + strings.Join(dedupe(lits), "|") + ">"
	parts := make([]string, 0, len(t.Union)+len(lits))
	for _, alt := range t.Union {
		switch alt.Base {
		case "array":
			parts = append(parts, items)
		case "object", "null":
		default:
			parts = append(parts, lits...)
		}
	}
	return parts
}

// literalUnion renders a closed set of legal values (an Attribute's
// "values" or an Argument's "choices", per spec) as JSDoc literal type
// parts: a string becomes a quoted literal ("active"), a bool or number
// becomes its own bare literal (true, 3), and an explicit null entry
// becomes the "null" keyword. Returns nil if choices is empty.
func literalUnion(choices []interface{}) []string {
	if len(choices) == 0 {
		return nil
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
			// Shouldn't happen for values decoded from JSON, but don't
			// drop the entry silently.
			parts = append(parts, fmt.Sprintf("%v", v))
		}
	}
	return parts
}

// formatNumberLiteral renders a float64 as a JS numeric literal without a
// spurious ".0" on whole numbers (choices are very often small integers,
// e.g. status codes or ratings).
func formatNumberLiteral(f float64) string {
	if !math.IsInf(f, 0) && f == math.Trunc(f) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// refinementNote turns any dotted refinements in a type (e.g. "email" in
// "string.email") into a short parenthetical note to append to a docs
// description, e.g. "(email format)". Object refinements (object.<name>,
// a type reference rather than a format constraint) are excluded. Returns
// "" if there's nothing worth noting.
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

func dedupe(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}