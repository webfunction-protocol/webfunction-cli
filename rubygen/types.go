package rubygen

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/webfunction-protocol/webfunction-go"
)

// field is a name/type/optional/docs tuple built from either an
// endpoint's Arguments or Attributes, or an object.<n>'s own
// Arguments/Attributes when resolving a named ref.
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
			docs: a.Docs, choices: a.Values,
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
			docs:     a.Docs, choices: a.Choices,
		}
	}
	return fields
}

// ---- argument side: real Ruby keyword params, untouched pass-through ----
//
// v2 deliberately does NOT wrap or validate outgoing arguments - the
// caller already builds a real, symbol-keyed Ruby hash/keyword-arg call
// exactly the way the real dynamic client expects (confirmed: nested
// object arguments are themselves ordinary caller-constructed hash
// literals, e.g. filters: { first_name: "Joe" }). All rubygen adds on
// the argument side is real, named keyword PARAMETERS (for arg-name
// completion) with real types, forwarded to the real client untouched.

type argResolver func(refName string) string

// rbsArgType renders the RBS/doc type for an argument-side Type. Object
// refs resolve to a loose `Hash[Symbol, untyped]` shape rather than a
// dedicated type (v2 has no argument-side wrapper classes to point at -
// see the package doc comment) - still real per-scalar precision
// everywhere else (choices, numerics, nilability).
func rbsArgType(t webfunction.Type, forceNullable bool, choices []interface{}) string {
	var parts []string
	if lits := literalUnion(choices); len(lits) > 0 {
		parts = lits
	} else {
		parts = make([]string, 0, len(t.Union))
		for _, alt := range t.Union {
			parts = append(parts, rbsArgAlt(alt))
		}
	}
	if forceNullable || t.HasBase("null") {
		parts = append(parts, "nil")
	}
	return strings.Join(dedupe(parts), " | ")
}

func rbsArgAlt(alt webfunction.TypeAlt) string {
	switch alt.Base {
	case "string":
		return "String"
	case "number":
		switch alt.Refinement {
		case "u32", "u64", "i32", "i64", "timestamp":
			return "Integer"
		case "f32", "f64":
			return "Float"
		default:
			return "Integer | Float"
		}
	case "boolean":
		return "bool"
	case "null":
		return "nil"
	case "any":
		return "untyped"
	case "array":
		if alt.Of != nil {
			return "Array[" + rbsArgType(*alt.Of, false, nil) + "]"
		}
		return "Array[untyped]"
	case "object":
		return "Hash[Symbol, untyped]"
	default:
		return "untyped"
	}
}

// ---- return side: real generated wrapper classes ----
//
// Confirmed from Request#execute: JSON.parse(body) with no
// symbolize_names, so every returned object has STRING keys. A wrapper
// class's accessor methods read `@raw["key"]` accordingly. A nested
// object/array-of-object field wraps its own raw value in the
// appropriate wrapper class too (recursively), rather than returning a
// bare Hash - that recursive wrapping is the entire point of this
// version (real `result.friend.name`, not `result["friend"]["name"]`).

// rubyClass is one generated wrapper class: a name, and the fields it
// exposes as real accessor methods.
type rubyClass struct {
	name   string
	fields []classField
}

type classField struct {
	field
	rbsType   string // for the companion .rbs def's return type
	wrapClass string // non-empty if this field's raw value should be wrapped in another rubyClass by name (possibly itself, for self-reference)
	isArray   bool   // true if wrapClass applies per-array-item rather than to the whole value
}

// classSet builds and memoizes generated wrapper classes: one per
// object.<n> ref actually used in a return/attribute position (memoized
// by name, cycle-safe - registered before its own fields are built, so
// a self-referential object resolves cleanly), plus one per endpoint
// whose own bare object/array return is described only by its own
// inline Attributes (always freshly named, never memoized/reused).
type classSet struct {
	pkg     *webfunction.Package
	names   map[string]bool
	byKey   map[string]string
	ordered []*rubyClass
}

func newClassSet(pkg *webfunction.Package) *classSet {
	return &classSet{pkg: pkg, names: map[string]bool{}, byKey: map[string]string{}}
}

// resolve returns the wrapper class name for object.<n> ref `name`,
// generating it on first use. Falls back to no wrapping (empty string)
// if the ref doesn't exist or has no attributes in this context - the
// field then just exposes the raw decoded value untouched.
func (s *classSet) resolve(name string) string {
	if existing, ok := s.byKey[name]; ok {
		return existing
	}
	// Register a placeholder before building fields, so a
	// self-referential (or mutually referential) object resolves to its
	// own name instead of looping.
	obj := s.pkg.Object(name)
	var fields []field
	if obj != nil {
		fields = attributeFields(obj.Attributes)
	}
	if len(fields) == 0 {
		s.byKey[name] = ""
		return ""
	}

	className := uniqueName(s.names, pascalCase(name)+"Attributes")
	s.byKey[name] = className
	s.ordered = append(s.ordered, s.buildClass(className, fields))
	return className
}

// resolveLocal synthesizes a fresh, uniquely-named wrapper class for an
// endpoint's own inline (non-object.<n>) return attributes - e.g.
// FindItemResult - never memoized/reused, since each endpoint's own
// bare-return shape is inherently one-off.
func (s *classSet) resolveLocal(endpointName string, fields []field) string {
	className := uniqueName(s.names, pascalCase(endpointName)+"Result")
	s.ordered = append(s.ordered, s.buildClass(className, fields))
	return className
}

func (s *classSet) buildClass(className string, fields []field) *rubyClass {
	cfields := make([]classField, len(fields))
	for i, f := range fields {
		cf := classField{field: f}
		cf.rbsType, cf.wrapClass, cf.isArray = s.fieldType(f)
		cfields[i] = cf
	}
	return &rubyClass{name: className, fields: cfields}
}

// fieldType computes a field's RBS type plus, if applicable, the
// wrapper class its raw value(s) should be instantiated as.
func (s *classSet) fieldType(f field) (rbsType, wrapClass string, isArray bool) {
	if lits := literalUnion(f.choices); len(lits) > 0 {
		t := strings.Join(dedupe(lits), " | ")
		if f.nullable && !strings.Contains(t, "nil") {
			t += " | nil"
		}
		return t, "", false
	}

	rbsType, wrapClass, isArray = s.resolveType(f.jsonType)
	if f.nullable && !strings.Contains(rbsType, "nil") {
		rbsType += " | nil"
	}
	return rbsType, wrapClass, isArray
}

// resolveType is the shared per-alt resolver: used for a field's own
// type (via fieldType above), an endpoint's own bare Returns type
// (arrayItemTypeOrScalar), and an array's item type (arrayItemType) -
// the same three positions v1's rbsReturnAlt/rbsArgAlt handled
// separately, unified here since v2 has no separate argument-vs-return
// distinction to make once past the field level (only whether an
// object ref resolves to a wrapper class).
func (s *classSet) resolveType(t webfunction.Type) (rbsType, wrapClass string, isArray bool) {
	var parts []string
	for _, alt := range t.Union {
		switch alt.Base {
		case "string":
			parts = append(parts, "String")
		case "number":
			switch alt.Refinement {
			case "u32", "u64", "i32", "i64", "timestamp":
				parts = append(parts, "Integer")
			case "f32", "f64":
				parts = append(parts, "Float")
			default:
				parts = append(parts, "Integer | Float")
			}
		case "boolean":
			parts = append(parts, "bool")
		case "null":
			parts = append(parts, "nil")
		case "any":
			parts = append(parts, "untyped")
		case "object":
			if alt.Refinement != "" {
				if cn := s.resolve(alt.Refinement); cn != "" {
					wrapClass = cn
					parts = append(parts, cn)
					continue
				}
			}
			parts = append(parts, "Hash[String, untyped]")
		case "array":
			if alt.Of != nil {
				itemType, itemWrap, _ := s.resolveType(*alt.Of)
				parts = append(parts, "Array["+itemType+"]")
				if itemWrap != "" {
					wrapClass = itemWrap
					isArray = true
				}
			} else {
				parts = append(parts, "Array[untyped]")
			}
		default:
			parts = append(parts, "untyped")
		}
	}
	return strings.Join(dedupe(parts), " | "), wrapClass, isArray
}

// arrayItemType resolves an array's item Type - used when building a
// paginated endpoint's item wrapper.
func (s *classSet) arrayItemType(t webfunction.Type) (rbsType, wrapClass string) {
	rbsType, wrapClass, _ = s.resolveType(t)
	return rbsType, wrapClass
}

// arrayItemTypeOrScalar resolves an endpoint's own bare Returns type
// (which may or may not actually be an array) when it's neither a bare
// "object" nor a bare array both accompanied by the endpoint's own
// inline Attributes (those two cases are handled directly in
// buildEndpointGen, since they read Attributes rather than a nested
// Type - an object.<n> ref or an array-of-object.<n> ref, by contrast,
// carries its shape in the Type itself via Refinement/Of, which
// resolveType already knows how to follow).
func (s *classSet) arrayItemTypeOrScalar(t webfunction.Type) (rbsType, wrapClass string) {
	rbsType, wrapClass, _ = s.resolveType(t)
	return rbsType, wrapClass
}

// ---- shared helpers ----

func literalUnion(choices []interface{}) []string {
	if len(choices) == 0 {
		return nil
	}
	parts := make([]string, 0, len(choices))
	for _, c := range choices {
		switch v := c.(type) {
		case nil:
			parts = append(parts, "nil")
		case string:
			parts = append(parts, rbsStringLiteral(v))
		case bool:
			parts = append(parts, strconv.FormatBool(v))
		case float64:
			if v == float64(int64(v)) {
				parts = append(parts, strconv.FormatInt(int64(v), 10))
			} else {
				parts = append(parts, "Float")
			}
		default:
			parts = append(parts, fmt.Sprintf("%v", v))
		}
	}
	return parts
}

func rbsStringLiteral(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

func rubyStringLiteral(s string) string {
	return rbsStringLiteral(s) // identical escaping rules
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

func pascalCase(name string) string {
	words := splitWords(name)
	var b strings.Builder
	for _, w := range words {
		if w == "" {
			continue
		}
		b.WriteString(strings.ToUpper(w[:1]))
		if len(w) > 1 {
			b.WriteString(strings.ToLower(w[1:]))
		}
	}
	return b.String()
}