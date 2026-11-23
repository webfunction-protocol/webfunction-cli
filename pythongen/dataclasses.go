package pythongen

import (
	"fmt"
	"strings"

	"github.com/webfunction-protocol/webfunction-go"
)

// field is a name/type/optional/docs/choices tuple, used to build a
// dataclass from either an endpoint's Attributes (return shape) or its
// Arguments (call shape), or an object.<n>'s own Arguments/Attributes when
// resolving a named ref. Mirrors every other target's identical field.
type field struct {
	name     string
	jsonType webfunction.Type
	optional bool
	docs     string
	choices  []interface{}
}

func attributeFields(attrs []webfunction.Attribute) []field {
	fields := make([]field, len(attrs))
	for i, a := range attrs {
		fields[i] = field{
			name:     a.Name,
			jsonType: a.Type,
			optional: a.Nullable(),
			docs:     withNotes(a.Docs, refinementNote(a.Type)),
			choices:  a.Values,
		}
	}
	return fields
}

func argumentFields(args []webfunction.Argument) []field {
	fields := make([]field, len(args))
	for i, a := range args {
		fields[i] = field{
			name:     a.Name,
			jsonType: a.Type,
			optional: !a.Required(),
			docs:     withNotes(a.Docs, refinementNote(a.Type)),
			choices:  a.Choices,
		}
	}
	return fields
}

// allOptional reports whether every field is individually optional -
// including the trivial case of no fields at all. Mirrors every other
// target's identical helper. Used to decide whether an endpoint's args
// parameter itself can default to None - Python has genuine native
// default-argument values, so (like C#, and unlike Go's variadic trick or
// Java's overload pair) this needs no workaround at all.
func allOptional(fields []field) bool {
	for _, f := range fields {
		if !f.optional {
			return false
		}
	}
	return true
}

type dataclassField struct {
	fieldName string // python attribute name (verbatim snake_case)
	typeExpr  string
	wireName  string
	optional  bool // renders "= field(default=None, ...)"; also drives ordering
	docLines  []string
	// decodeExpr, if non-empty, is a Python expression (referencing the
	// bound name "_raw_<fieldName>") that decodes the raw wire value into
	// this field's real type - needed only when the field's type resolves
	// to a generated dataclass somewhere within it (an object.<n> ref, or
	// a list of one). Left empty for a plain scalar/Any/Union/List-of-
	// scalar field, where the value json.loads already produced is
	// exactly the right Python value with no conversion needed.
	decodeExpr string
}

// dataclassDef is a generated Python dataclass.
type dataclassDef struct {
	name   string
	doc    string
	fields []dataclassField
}

func (d *dataclassDef) render(b *strings.Builder) {
	b.WriteString("@dataclass\n")
	b.WriteString("class " + d.name + ":\n")
	if d.doc != "" {
		writeDocstring(b, "    ", docLines(d.doc))
		b.WriteString("\n")
	}
	if len(d.fields) == 0 {
		b.WriteString("    pass\n\n\n")
		return
	}
	for _, f := range d.fields {
		if f.optional {
			b.WriteString(fmt.Sprintf("    %s: %s = field(default=None, metadata={\"wire_name\": %s})\n",
				f.fieldName, f.typeExpr, pyStr(f.wireName)))
		} else {
			b.WriteString(fmt.Sprintf("    %s: %s = field(metadata={\"wire_name\": %s})\n",
				f.fieldName, f.typeExpr, pyStr(f.wireName)))
		}
		// An attribute docstring is a bare string literal AFTER the
		// assignment, not before it (the opposite convention from e.g.
		// C#'s /// doc comments, which precede the member) - Sphinx and
		// other tools recognize this placement specifically.
		if len(f.docLines) > 0 {
			writeDocstring(b, "    ", f.docLines)
		}
	}
	b.WriteString("\n")

	b.WriteString("    @classmethod\n")
	b.WriteString("    def _from_dict(cls, d: Dict[str, Any]) -> \"" + d.name + "\":\n")
	b.WriteString("        return cls(\n")
	for _, f := range d.fields {
		// A required field indexes the dict directly (d["x"]) rather than
		// .get("x") - not just a style choice: dict.__getitem__ returns
		// plain Any, while .get() returns Optional[Any], and mypy --strict
		// correctly rejects passing that Optional[Any] to a required
		// non-Optional field (Any alone is compatible with anything, but
		// the added None is not). Indexing also raises a clear KeyError if
		// a required field is genuinely missing from the wire response,
		// which is arguably more correct than silently decoding it as
		// None. Optional fields keep .get(), since an absent key
		// legitimately means None for them.
		rawExpr := "d.get(" + pyStr(f.wireName) + ")"
		if !f.optional {
			rawExpr = "d[" + pyStr(f.wireName) + "]"
		}
		if f.decodeExpr == "" {
			b.WriteString(fmt.Sprintf("            %s=%s,\n", f.fieldName, rawExpr))
		} else {
			b.WriteString(fmt.Sprintf(
				"            %s=(%s) if (_raw_%s := %s) is not None else None,\n",
				f.fieldName, f.decodeExpr, f.fieldName, rawExpr))
		}
	}
	b.WriteString("        )\n\n\n")
}

func pyStr(s string) string {
	return fmt.Sprintf("%q", s)
}

// dataclassSet builds the set of named Python dataclasses needed for a
// package: endpoint argument shapes, endpoint return shapes, and resolved
// named object definitions (object.<n> refs). Mirrors every other
// target's recordSet/structSet exactly.
type dataclassSet struct {
	pkg     *webfunction.Package
	ordered []*dataclassDef
	names   map[string]bool
	// objects memoizes a resolved object.<n> ref by "context:name" so the
	// same reference (in the same context) always maps to the same
	// generated class, and so a self-referential object resolves without
	// infinite recursion.
	objects map[string]string
}

func newDataclassSet(pkg *webfunction.Package) *dataclassSet {
	return &dataclassSet{pkg: pkg, names: map[string]bool{}, objects: map[string]string{}}
}

func (s *dataclassSet) uniqueName(base string) string {
	name := base
	for i := 2; s.names[name]; i++ {
		name = fmt.Sprintf("%s%d", base, i)
	}
	s.names[name] = true
	return name
}

// forFields builds a dataclass describing this set of fields and returns
// its name, or "" if fields is empty (nothing to describe). Always
// creates a fresh dataclass - mirrors every other target's identical
// policy: two endpoints coincidentally sharing an identical shape are not
// the same concept, even though the classes would be structurally
// interchangeable.
func (s *dataclassSet) forFields(baseName, doc string, fields []field, context string) string {
	if len(fields) == 0 {
		return ""
	}
	name := s.uniqueName(baseName)
	resolve := func(refName string) string { return s.resolveObject(refName, context) }
	s.ordered = append(s.ordered, &dataclassDef{
		name:   name,
		doc:    doc,
		fields: s.renderFields(fields, resolve),
	})
	return name
}

// resolveObject returns the dataclass name for a named object.<n>
// reference, building it (recursively - the context propagates into any
// nested object refs within it, per spec) if it hasn't been resolved yet
// in this context. Falls back to "Dict[str, Any]" if the referenced
// object doesn't exist, or has no members defined for this context.
// Mirrors every other target's identical resolveObject.
func (s *dataclassSet) resolveObject(name, context string) string {
	key := context + ":" + name
	if existing, ok := s.objects[key]; ok {
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
		s.objects[key] = "Dict[str, Any]"
		return "Dict[str, Any]"
	}

	className := s.uniqueName(pythonClassName(name) + suffix)
	// Register before rendering fields, so a self-referential (or
	// mutually referential) object resolves to this name instead of
	// recursing forever.
	s.objects[key] = className

	resolve := func(refName string) string { return s.resolveObject(refName, context) }
	doc := className + " is the \"" + name + "\" object definition."
	s.ordered = append(s.ordered, &dataclassDef{
		name:   className,
		doc:    doc,
		fields: s.renderFields(fields, resolve),
	})
	return className
}

// renderFields turns fields into fully-rendered dataclass components,
// resolving any object.<n> references encountered via resolve, generating
// a typing.Literal[...] type per field that declares choices (see
// buildLiteralField), and stable-partitioning required fields before
// optional ones.
//
// The reordering is necessary, not cosmetic - the exact same real bug
// found against csharpgen's output applies identically to Python
// dataclasses: a generated __init__ is an ordinary parameter list under
// the hood, and Python (like C#) rejects a non-default parameter
// following a default one ("non-default argument follows default
// argument"). A real package's own field order has no such guarantee, so
// declaration order can't just mirror wire/source order. Only the
// declaration order changes here - each field's own wire_name mapping
// (used by both directions of conversion) is unaffected.
func (s *dataclassSet) renderFields(fields []field, resolve typeResolver) []dataclassField {
	out := make([]dataclassField, len(fields))
	for i, f := range fields {
		fieldName := pythonFieldName(f.name)

		nullable := f.optional
		var typ string
		var decodeExpr string
		if len(f.choices) > 0 && f.jsonType.HasBase("array") {
			// Per the spec the choices of an array are its allowed items,
			// so the Literal is the item type and the field a List of it.
			typ = arrayChoicesType(f, nullable, resolve)
		} else if len(f.choices) > 0 {
			literalExpr, usedLiteral, hasNull := buildLiteralField(f)
			if hasNull {
				nullable = true
			}
			if usedLiteral {
				if nullable {
					typ = "Optional[" + literalExpr + "]"
				} else {
					typ = literalExpr
				}
			} else {
				// PEP 586 doesn't allow float values in typing.Literal -
				// fall back to the field's own base type, with the
				// allowed values noted in the docstring instead (see
				// buildLiteralField).
				typ = pythonType(f.jsonType, localTypes{}, nullable, resolve)
			}
		} else {
			typ = pythonType(f.jsonType, localTypes{}, nullable, resolve)
			decodeExpr = decodeValueExpr("_raw_"+fieldName, f.jsonType, localTypes{}, resolve)
			if decodeExpr == "_raw_"+fieldName {
				decodeExpr = ""
			}
		}

		doc := f.docs
		if len(f.choices) > 0 {
			doc = withNotes(doc, choicesNote(f.choices))
		}
		var fieldDocLines []string
		if doc != "" {
			fieldDocLines = docLines(doc)
		}

		out[i] = dataclassField{
			fieldName: fieldName, typeExpr: typ, wireName: f.name,
			optional: nullable, docLines: fieldDocLines, decodeExpr: decodeExpr,
		}
	}
	return orderRequiredFirst(out)
}

func orderRequiredFirst(fields []dataclassField) []dataclassField {
	out := make([]dataclassField, 0, len(fields))
	for _, f := range fields {
		if !f.optional {
			out = append(out, f)
		}
	}
	for _, f := range fields {
		if f.optional {
			out = append(out, f)
		}
	}
	return out
}

// buildLiteralField renders a field's choices/values as a
// typing.Literal[...] expression, based on the field's own declared type
// (see choiceLiteralKind) - EXCEPT when that type is "float": PEP 586
// explicitly disallows float values as typing.Literal members (unlike
// every other target's enum/union mechanism, which has no such
// restriction), so a float-typed choices field can't be represented this
// way at all. In that case usedLiteral is false and the caller falls back
// to the field's ordinary type, with the allowed values only noted in the
// docstring - a real, Python-specific limitation worth flagging alongside
// the numeric-precision one in types.go.
//
// An explicit null among the choices means the wire value itself may
// legally be absent/null on top of whatever the endpoint's own
// required/nullable flag says (hasNull) - it does not get a Literal
// member of its own (None is handled via the Optional[...] wrapper
// instead, more idiomatic than typing.Literal[None, ...]).
func buildLiteralField(f field) (literalExpr string, usedLiteral bool, hasNull bool) {
	kind := choiceLiteralKind(f.jsonType)
	if kind == "float" {
		for _, c := range f.choices {
			if c == nil {
				hasNull = true
			}
		}
		return "", false, hasNull
	}

	seen := map[string]bool{}
	var literals []string
	for _, c := range f.choices {
		if c == nil {
			hasNull = true
			continue
		}
		lit := pythonLiteral(c, kind)
		if seen[lit] {
			continue
		}
		seen[lit] = true
		literals = append(literals, lit)
	}
	if len(literals) == 0 {
		return "", false, hasNull
	}
	return "Literal[" + strings.Join(literals, ", ") + "]", true, hasNull
}

// arrayChoicesType renders an array field that has choices/values. The
// choices are the allowed items, so an array alternative becomes
// List[Literal[...]]. A union may mix an array with a scalar (e.g.
// ["string", "array"]), and the spec does not say which the values belong
// to, so each scalar alternative gets the Literal as well, in union order
// (Python has real unions, so this is a Union[...] rather than a fallback
// to Any). An explicit null among the items makes the items Optional, not
// the field; an object alternative has no literals to offer and is left
// out. When the choices cannot all be Literal members (see
// arrayChoiceLiterals) the field keeps the type it would have without
// choices, with the values noted in the docstring, as for a float scalar.
func arrayChoicesType(f field, nullable bool, resolve typeResolver) string {
	items, scalars, ok := arrayChoiceLiterals(f.choices)
	if !ok {
		return pythonType(f.jsonType, localTypes{}, nullable, resolve)
	}
	var parts []string
	scalarDone := false
	for _, alt := range f.jsonType.Union {
		switch alt.Base {
		case "null", "object":
		case "array":
			parts = append(parts, "List["+items+"]")
		default:
			if !scalarDone {
				parts = append(parts, scalars)
				scalarDone = true
			}
		}
	}
	typ := parts[0]
	if len(parts) > 1 {
		typ = "Union[" + strings.Join(parts, ", ") + "]"
	}
	if nullable {
		typ = "Optional[" + typ + "]"
	}
	return typ
}

// arrayChoiceLiterals renders the choices of an array field as Literal
// expressions: scalars is Literal[...] of the distinct non-null values,
// items is the same, wrapped in Optional[...] when the choices include an
// explicit null. A Literal can mix strings, ints and bools, but PEP 586
// forbids floats, so ok is false for a fractional number, for any other
// kind of value, and when there is nothing but null. Whole numbers are
// int members.
func arrayChoiceLiterals(choices []interface{}) (items, scalars string, ok bool) {
	seen := map[string]bool{}
	var literals []string
	hasNull := false
	for _, c := range choices {
		switch v := c.(type) {
		case nil:
			hasNull = true
			continue
		case string, bool:
		case float64:
			if v != float64(int64(v)) {
				return "", "", false
			}
		default:
			return "", "", false
		}
		lit := pythonLiteral(c, "int")
		if seen[lit] {
			continue
		}
		seen[lit] = true
		literals = append(literals, lit)
	}
	if len(literals) == 0 {
		return "", "", false
	}
	scalars = "Literal[" + strings.Join(literals, ", ") + "]"
	items = scalars
	if hasNull {
		items = "Optional[" + scalars + "]"
	}
	return items, scalars, true
}

// choicesNote renders a short "One of: ..." docstring note listing the
// raw choice values - always added alongside the Literal type (redundant
// with the type itself for most editors, but harmless and mirrors gogen's
// comment-only approach for anyone reading generated source directly), and
// the ONLY record of the allowed values at all when a float type forced
// buildLiteralField to skip Literal generation.
func choicesNote(choices []interface{}) string {
	kind := "string" // arbitrary default; only used for literal rendering below
	var parts []string
	for _, c := range choices {
		if c == nil {
			parts = append(parts, "null")
			continue
		}
		if _, ok := c.(float64); ok {
			kind = "float"
		}
		parts = append(parts, pythonLiteral(c, kind))
	}
	return "One of: " + strings.Join(parts, ", ") + "."
}

// decodeValueExpr returns a Python expression that decodes valueExpr
// (already-bound to the raw wire value) into t's real type - recursing
// into array element types - or valueExpr itself unchanged if t needs no
// conversion (a plain scalar/Any/Union/List-of-scalar, where the value
// json.loads already produced is exactly right). Only object.<n> refs
// (and, transitively, lists of them) ever need real conversion, since
// those are the only case where the target type is a generated dataclass
// rather than a native JSON-decoded Python value.
func decodeValueExpr(valueExpr string, t webfunction.Type, local localTypes, resolve typeResolver) string {
	var nonNull []webfunction.TypeAlt
	for _, alt := range t.Union {
		if alt.Base != "null" {
			nonNull = append(nonNull, alt)
		}
	}
	if len(nonNull) != 1 {
		return valueExpr
	}
	return decodeAltExpr(valueExpr, nonNull[0], local, resolve)
}

func decodeAltExpr(valueExpr string, alt webfunction.TypeAlt, local localTypes, resolve typeResolver) string {
	switch alt.Base {
	case "object":
		if alt.IsObjectRef() && resolve != nil {
			className := resolve(alt.Refinement)
			if className != "Dict[str, Any]" {
				return className + "._from_dict(" + valueExpr + ")"
			}
			return valueExpr
		}
		if local.object != "" {
			return local.object + "._from_dict(" + valueExpr + ")"
		}
		return valueExpr
	case "array":
		if alt.Of != nil {
			itemExpr := decodeValueExpr("_item", *alt.Of, localTypes{}, resolve)
			if itemExpr == "_item" {
				return valueExpr
			}
			return "[" + itemExpr + " for _item in (" + valueExpr + " or [])]"
		}
		if local.arrayOfItem != "" {
			return "[" + local.arrayOfItem + "._from_dict(_item) for _item in (" + valueExpr + " or [])]"
		}
		return valueExpr
	default:
		return valueExpr
	}
}

// render writes out every collected dataclass definition, in the order
// they were created.
func (s *dataclassSet) render(b *strings.Builder) {
	for _, decl := range s.ordered {
		decl.render(b)
	}
}
