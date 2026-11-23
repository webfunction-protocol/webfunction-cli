package phpgen

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/webfunction-protocol/webfunction-go"
)

// field is a name/type/optional/docs tuple, used to build an inline
// array-shape from either an endpoint's Arguments or its own inline
// Attributes (bare object/array local shape) - or an object.<name>'s own
// Arguments/Attributes when resolving a named ref. Unlike jsgen, there's
// no separate named typedef per endpoint here: per design decision, an
// endpoint's own Args/Return shape stays an inline `array{...}` - only
// object.<name> refs get named, reusable PHPDoc type aliases (see
// aliasSet), since those are the ones that actually need to
// self-reference or be shared.
//
// optional/nullable mirror jsgen's field exactly: optional means the key
// may be absent (rendered as `key?:` in the array-shape); nullable means
// the value itself may additionally be null. An Attribute's `nullable`
// flag means both at once per spec, same as jsgen.
type field struct {
	name     string
	jsonType webfunction.Type
	optional bool
	nullable bool
	docs     string
	choices  []interface{}
}

func attributeFields(attrs []webfunction.Attribute) []field {
	fields := make([]field, len(attrs))
	for i, a := range attrs {
		fields[i] = field{
			name: a.Name, jsonType: a.Type,
			optional: a.Nullable(), nullable: a.Nullable(),
			docs: withNotes(a.Docs, refinementNote(a.Type)), choices: a.Values,
		}
	}
	return fields
}

func argumentFields(args []webfunction.Argument) []field {
	fields := make([]field, len(args))
	for i, a := range args {
		fields[i] = field{
			name: a.Name, jsonType: a.Type,
			optional: !a.Required(),
			docs:     withNotes(a.Docs, refinementNote(a.Type)), choices: a.Choices,
		}
	}
	return fields
}

// allOptional reports whether every field is individually optional -
// including the trivial case of no fields at all. Mirrors jsgen's
// allOptional; used to decide whether the whole $args parameter can be
// given a `= null` default rather than being mandatory.
func allOptional(fields []field) bool {
	for _, f := range fields {
		if !f.optional {
			return false
		}
	}
	return true
}

// localShapes holds inline array-shape strings for an endpoint's own
// bare object/array return, built from its own Attributes - mirrors
// jsgen's localTypedefs, but as ready-to-use inline shape strings rather
// than named typedefs, since endpoint returns stay inline per decision #1.
type localShapes struct {
	object      string // shape when returns is (or includes) bare "object"
	arrayOfItem string // item shape when returns is bare, untyped "array"
}

// aliasResolver resolves a named object.<name> ref (context already
// baked in by the caller - "argument" or "attribute") to a PHPDoc type
// alias name. Mirrors jsgen's objectResolver.
type aliasResolver func(refName string) string

// phpDocType builds the PHPDoc type expression for a Type, given local
// shapes for the endpoint's own bare object/array attributes (zero value
// if none), whether the value may additionally be null on top of
// whatever the type itself says, a resolver for any object.<name>
// references encountered, and any closed set of legal values (an
// Attribute's "values" or Argument's "choices"; nil if none). Mirrors
// jsgen's jsdocType closely, PHPDoc syntax instead of JSDoc's.
//
// When choices is non-empty and the type has no array alternative, it
// takes over the type entirely: the result is a PHPDoc literal union
// (e.g. 'active'|'inactive'|'pending') rather than the bare base type, since
// the literal union is what actually lets PHPStan/Psalm catch a typo or
// an out-of-set value, not just document it. When the type is or includes
// an array, the spec makes the choices the allowed ITEMS of that array,
// so see choiceParts.
func phpDocType(t webfunction.Type, local localShapes, forceNullable bool, resolve aliasResolver, choices []interface{}) string {
	var parts []string
	if lits := literalUnion(choices); len(lits) > 0 {
		parts = choiceParts(t, lits)
	} else {
		parts = make([]string, 0, len(t.Union))
		for _, alt := range t.Union {
			parts = append(parts, phpDocAlt(alt, local, resolve))
		}
	}

	if forceNullable || t.HasBase("null") {
		parts = append(parts, "null")
	}

	parts = dedupe(parts)
	return strings.Join(parts, "|")
}

func phpDocAlt(alt webfunction.TypeAlt, local localShapes, resolve aliasResolver) string {
	switch alt.Base {
	case "string":
		return "string"
	case "number":
		switch alt.Refinement {
		case "u32", "u64", "i32", "i64", "timestamp":
			return "int"
		case "f32", "f64":
			return "float"
		default:
			// No refinement (or one not in the confirmed vocabulary) -
			// JSON's "number" doesn't distinguish int from float on its
			// own, so int|float is the honest type.
			return "int|float"
		}
	case "boolean":
		return "bool"
	case "null":
		return "null"
	case "any":
		return "mixed"
	case "array":
		if alt.Of != nil {
			// A JSON array is always a PHP list (sequential, int-keyed) -
			// list<T> is the more precise PHPStan/Psalm shape for that,
			// not the weaker array<int, T> (which also accepts gaps).
			return "list<" + phpDocType(*alt.Of, localShapes{}, false, resolve, nil) + ">"
		}
		if local.arrayOfItem != "" {
			return "list<" + local.arrayOfItem + ">"
		}
		return "array"
	case "object":
		if alt.Refinement != "" {
			return resolve(alt.Refinement)
		}
		if local.object != "" {
			return local.object
		}
		return "array<string, mixed>"
	default:
		return "mixed"
	}
}

// choiceParts renders the alternatives of a type that carries a closed set
// of legal values, lits being the literals from literalUnion. Per the spec,
// when the type is (or includes) the array type, the values are the allowed
// items of the array, so an array alternative becomes list<literals>. A
// union may mix an array with a scalar (e.g. ["string", "array"]), and the
// spec does not say which the values belong to, so each scalar alternative
// keeps the literals as well. An object alternative has no literals to
// offer and is left out, as it always has been when choices are present;
// null is added by the caller. With no array alternative the literals
// stand for the whole type. Mirrors jsgen's choiceParts.
func choiceParts(t webfunction.Type, lits []string) []string {
	if !t.HasBase("array") {
		return lits
	}
	items := "list<" + strings.Join(dedupe(lits), "|") + ">"
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
// "values" or an Argument's "choices", per spec) as PHPDoc literal type
// parts: a string becomes a single-quoted literal, a bool or number
// becomes its own bare literal, and an explicit null entry becomes the
// "null" keyword. Mirrors jsgen's literalUnion. Returns nil if choices is
// empty.
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
			escaped := strings.ReplaceAll(v, `\`, `\\`)
			escaped = strings.ReplaceAll(escaped, `'`, `\'`)
			parts = append(parts, "'"+escaped+"'")
		case bool:
			parts = append(parts, strconv.FormatBool(v))
		case float64: // encoding/json decodes all JSON numbers as float64
			parts = append(parts, formatNumberLiteral(v))
		default:
			parts = append(parts, fmt.Sprintf("%v", v))
		}
	}
	return parts
}

func formatNumberLiteral(f float64) string {
	if f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// refinementNote turns any dotted refinements in a type (e.g. "email" in
// "string.email") into a short parenthetical note to append to a docs
// description, e.g. "(email format)". Object refinements (object.<name>,
// a type reference rather than a format constraint) are excluded. Mirrors
// jsgen's refinementNote. Returns "" if there's nothing worth noting.
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

// aliasDef is one generated `@phpstan-type`/`@psalm-type` pair.
type aliasDef struct {
	name    string
	phpType string
}

// aliasSet builds the set of named PHPDoc type aliases needed for a
// package's object.<name> refs - the only place named/reusable types are
// used (endpoint Args/Return shapes stay inline per decision #1; only
// object.<name> refs get aliases, per decision #2, uniformly - not just
// the ones that happen to be self-referential, mirroring jsgen's
// approach of always naming object refs). Rendered as combined
// @phpstan-type/@psalm-type tags (both tools' tag names differ for the
// same idea; emitting both costs little and covers either) in the main
// client class's docblock - Page wrapper classes that reference one
// import it via @phpstan-import-type/@psalm-import-type (see phpgen.go).
type aliasSet struct {
	pkg     *webfunction.Package
	names   map[string]bool
	byKey   map[string]string // "context:objectName" -> alias name, or a fallback shape string
	ordered []*aliasDef
}

func newAliasSet(pkg *webfunction.Package) *aliasSet {
	return &aliasSet{pkg: pkg, names: map[string]bool{}, byKey: map[string]string{}}
}

// resolve resolves a named object.<name> ref in the given context
// ("argument" or "attribute" - same name can have different members in
// each context per spec) to a PHPDoc type alias name, registering it the
// first time it's referenced. Falls back to a generic array shape if the
// ref doesn't exist or has no members in that context. Mirrors jsgen's
// resolveObjectTypedef, including registering the name *before* building
// its shape, so a self-referential (or mutually referential) object
// resolves to this alias name instead of recursing forever - PHPStan and
// Psalm both support a recursive `@phpstan-type X = array{...X...}`.
func (s *aliasSet) resolve(name, context string) string {
	key := context + ":" + name
	if existing, ok := s.byKey[key]; ok {
		return existing
	}

	obj := s.pkg.Object(name)

	var fields []field
	suffix := "Attributes"
	if context == "argument" {
		suffix = "Args"
		if obj != nil {
			fields = argumentFields(obj.Arguments)
		}
	} else if obj != nil {
		fields = attributeFields(obj.Attributes)
	}

	if len(fields) == 0 {
		s.byKey[key] = "array<string, mixed>"
		return "array<string, mixed>"
	}

	aliasName := s.uniqueName(pascalCase(name) + suffix)
	s.byKey[key] = aliasName

	shape := s.renderShape(fields, context)
	s.ordered = append(s.ordered, &aliasDef{name: aliasName, phpType: shape})
	return aliasName
}

// renderShape turns fields into an inline `array{...}` PHPDoc shape,
// resolving any object.<name> references encountered via context.
func (s *aliasSet) renderShape(fields []field, context string) string {
	if len(fields) == 0 {
		return "array{}"
	}
	resolve := func(refName string) string { return s.resolve(refName, context) }
	parts := make([]string, len(fields))
	for i, f := range fields {
		typ := phpDocType(f.jsonType, localShapes{}, f.nullable, resolve, f.choices)
		key := f.name
		if f.optional {
			key += "?"
		}
		parts[i] = key + ": " + typ
	}
	return "array{" + strings.Join(parts, ", ") + "}"
}

func (s *aliasSet) uniqueName(base string) string {
	name := base
	for i := 2; s.names[name]; i++ {
		name = fmt.Sprintf("%s%d", base, i)
	}
	s.names[name] = true
	return name
}

// hasAliases reports whether any object.<name> refs were actually
// resolved to a real alias (as opposed to only ever hitting the
// array<string, mixed> fallback) - used to decide whether the main
// class's docblock needs the alias tags at all, and whether any Page
// wrapper class needs an @…-import-type line.
func (s *aliasSet) hasAliases() bool {
	return len(s.ordered) > 0
}
