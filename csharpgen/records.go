package csharpgen

import (
	"fmt"
	"strings"

	"github.com/webfunction-protocol/webfunction-go"
)

// field is a name/type/optional/docs/choices tuple, used to build a
// record from either an endpoint's Attributes (return shape) or its
// Arguments (call shape), or an object.<n>'s own Arguments/Attributes
// when resolving a named ref. Mirrors gogen's/javagen's field.
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
// including the trivial case of no fields at all. Mirrors gogen's/
// javagen's allOptional. Used to decide whether an endpoint's args
// parameter itself can default to null (C# has real default parameter
// values, so - unlike Go's variadic workaround or Java's overload pair -
// this is just an ordinary "= null" default, the cleanest of any target
// so far).
func allOptional(fields []field) bool {
	for _, f := range fields {
		if !f.optional {
			return false
		}
	}
	return true
}

// csharpTypeDecl is one generated top-level-nested type: a record, an
// enum (paired with its converter class). All render into the same
// ordered stream inside the generated Client class.
type csharpTypeDecl interface {
	render(b *strings.Builder)
}

type recordField struct {
	propName string
	typeExpr string
	wireName string
	nullable bool
	docLines []string
}

// recordDef is a generated C# positional record type.
//
// Unlike javagen (which needs an explicit @JsonCreator-annotated
// secondary constructor, because Jackson's automatic record-parameter
// detection combined with a global snake_case naming strategy was found
// to throw at runtime - see webfunction-java's own real bug writeup),
// System.Text.Json works directly against a positional record's primary
// constructor with per-parameter [property: JsonPropertyName(...)]
// attributes, no workaround needed. A real simplification, not just a
// stylistic difference.
type recordDef struct {
	name   string
	doc    string
	fields []recordField
}

func (r *recordDef) render(b *strings.Builder) {
	if r.doc != "" {
		writeSummary(b, "    ", docLines(r.doc))
	}
	b.WriteString("    public sealed record " + r.name + "(\n")
	for i, f := range r.fields {
		if len(f.docLines) > 0 {
			writeSummary(b, "        ", f.docLines)
		}
		comma := ","
		if i == len(r.fields)-1 {
			comma = ""
		}
		def := ""
		if f.nullable {
			def = " = null"
		}
		b.WriteString(fmt.Sprintf(
			"        [property: System.Text.Json.Serialization.JsonPropertyName(%q)] %s %s%s%s\n",
			f.wireName, f.typeExpr, f.propName, def, comma))
	}
	b.WriteString("    );\n\n")
}

type enumConstant struct {
	label   string
	literal string
}

// enumDef is a generated C# enum type for an Argument's "choices" or
// Attribute's "values", paired with a small JsonConverter<T> that maps
// between the enum's members and their real wire values (which may be a
// string, bool, or number - C# enums are always integral-backed
// underneath, unlike the wire representation). A real enforced enum type,
// not just a doc comment the way gogen's choicesNote is for Go - the same
// improvement javagen gets for Java, ported to C#'s own converter
// mechanism ([JsonConverter] on the enum type, functionally the same idea
// as Jackson's @JsonValue/@JsonCreator on a Java enum).
type enumDef struct {
	name      string
	converter string // the paired converter class's name
	doc       string
	wireType  string // string, bool, long, or double - see choiceWireType
	constants []enumConstant
}

func (e *enumDef) render(b *strings.Builder) {
	if e.doc != "" {
		writeSummary(b, "    ", docLines(e.doc))
	}
	b.WriteString("    [System.Text.Json.Serialization.JsonConverter(typeof(" + e.converter + "))]\n")
	b.WriteString("    public enum " + e.name + "\n    {\n")
	for _, c := range e.constants {
		b.WriteString("        " + c.label + ",\n")
	}
	b.WriteString("    }\n\n")

	readerCall, writerCall := jsonReaderWriter(e.wireType)

	// A bool has exactly two possible values. When both true and false
	// are already covered as switch arms (a "boolean" field declaring an
	// explicit choices: [true, false] - somewhat redundant in the source
	// data, but real - confirmed against reservepay's actual package),
	// the switch is already exhaustive and C# rejects a trailing "_ =>"
	// catch-all as unreachable (CS8510). string/long/double don't have
	// this problem - their domains are unbounded, so the catch-all stays
	// genuinely reachable (an actual wire value outside the declared
	// choices) - only bool needs this special-cased.
	boolExhaustive := e.wireType == "bool" && hasLiteral(e.constants, "true") && hasLiteral(e.constants, "false")

	b.WriteString("    private sealed class " + e.converter + " : System.Text.Json.Serialization.JsonConverter<" + e.name + ">\n    {\n")
	b.WriteString("        public override " + e.name + " Read(ref System.Text.Json.Utf8JsonReader reader, Type typeToConvert, System.Text.Json.JsonSerializerOptions options)\n        {\n")
	b.WriteString("            var wire = reader." + readerCall + ";\n")
	b.WriteString("            return wire switch\n            {\n")
	for _, c := range e.constants {
		b.WriteString("                " + c.literal + " => " + e.name + "." + c.label + ",\n")
	}
	if !boolExhaustive {
		b.WriteString("                _ => throw new System.Text.Json.JsonException($\"unknown value for " + e.name + ": {wire}\"),\n")
	}
	b.WriteString("            };\n")
	b.WriteString("        }\n\n")
	b.WriteString("        public override void Write(System.Text.Json.Utf8JsonWriter writer, " + e.name + " value, System.Text.Json.JsonSerializerOptions options)\n        {\n")
	b.WriteString("            switch (value)\n            {\n")
	for _, c := range e.constants {
		b.WriteString("                case " + e.name + "." + c.label + ": writer." + writerCall + "(" + c.literal + "); break;\n")
	}
	b.WriteString("                default: throw new System.Text.Json.JsonException($\"unknown " + e.name + " member: {value}\");\n")
	b.WriteString("            }\n")
	b.WriteString("        }\n")
	b.WriteString("    }\n\n")
}

// jsonReaderWriter returns the Utf8JsonReader getter call and
// Utf8JsonWriter write-method name matching wireType.
// hasLiteral reports whether any constant's rendered wire literal equals
// literal exactly (e.g. "true"/"false" for a bool-backed enum's
// exhaustiveness check - see enumDef.render).
func hasLiteral(constants []enumConstant, literal string) bool {
	for _, c := range constants {
		if c.literal == literal {
			return true
		}
	}
	return false
}

func jsonReaderWriter(wireType string) (reader, writer string) {
	switch wireType {
	case "bool":
		return "GetBoolean()", "WriteBooleanValue"
	case "long":
		return "GetInt64()", "WriteNumberValue"
	case "double":
		return "GetDouble()", "WriteNumberValue"
	default:
		return "GetString()", "WriteStringValue"
	}
}

// recordSet builds the set of named C# record/enum types needed for a
// package: endpoint argument shapes, endpoint return shapes, resolved
// named object definitions (object.<n> refs), and any per-field choices
// enums encountered along the way. Mirrors gogen's structSet/javagen's
// recordSet.
type recordSet struct {
	pkg     *webfunction.Package
	ordered []csharpTypeDecl
	names   map[string]bool
	// objects memoizes resolved object.<n> records, keyed by
	// "<context>:<name>" - same reasoning as gogen's/javagen's identical
	// field: the same object name can resolve differently per context,
	// and this also guards against infinite recursion on self-referential
	// objects.
	objects map[string]string
}

func newRecordSet(pkg *webfunction.Package) *recordSet {
	return &recordSet{pkg: pkg, names: map[string]bool{}, objects: map[string]string{}}
}

// uniqueName mirrors gogen's/javagen's identical helper - shared across
// records, enums, AND converters, since all three are nested types within
// the same outer Client class and so share one C# identifier namespace.
func (s *recordSet) uniqueName(base string) string {
	name := base
	for i := 2; s.names[name]; i++ {
		name = fmt.Sprintf("%s%d", base, i)
	}
	s.names[name] = true
	return name
}

// forFields builds a record describing this set of fields and returns its
// name, or "" if fields is empty (nothing to describe). Always creates a
// fresh record - mirrors gogen's/javagen's identical policy: two
// endpoints coincidentally sharing an identical shape are not the same
// concept, even though the records would be structurally interchangeable.
func (s *recordSet) forFields(baseName, doc string, fields []field, context string) string {
	if len(fields) == 0 {
		return ""
	}
	name := s.uniqueName(baseName)
	resolve := func(refName string) string { return s.resolveObject(refName, context) }
	s.ordered = append(s.ordered, &recordDef{
		name:   name,
		doc:    doc,
		fields: s.renderFields(name, fields, resolve),
	})
	return name
}

// resolveObject returns the record name for a named object.<n>
// reference, building it (recursively - the context propagates into any
// nested object refs within it, per spec) if it hasn't been resolved yet
// in this context. Falls back to "Dictionary<string, object?>" if the
// referenced object doesn't exist, or has no members defined for this
// context. Mirrors gogen's/javagen's identical helper.
func (s *recordSet) resolveObject(name, context string) string {
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
		s.objects[key] = "Dictionary<string, object?>"
		return "Dictionary<string, object?>"
	}

	recordName := s.uniqueName(exportedTypeName(name) + suffix)
	// Register before rendering fields, so a self-referential (or
	// mutually referential) object resolves to this name instead of
	// recursing forever.
	s.objects[key] = recordName

	resolve := func(refName string) string { return s.resolveObject(refName, context) }
	doc := recordName + " is the \"" + name + "\" object definition."
	s.ordered = append(s.ordered, &recordDef{
		name:   recordName,
		doc:    doc,
		fields: s.renderFields(recordName, fields, resolve),
	})
	return recordName
}

// renderFields turns fields into fully-rendered record components,
// resolving any object.<n> references encountered via resolve, generating
// a choices enum per field that declares one (see buildEnumField), and
// deduping C# property names within the record.
func (s *recordSet) renderFields(ownerName string, fields []field, resolve typeResolver) []recordField {
	used := map[string]bool{}
	out := make([]recordField, len(fields))
	for i, f := range fields {
		propName := exportedFieldName(f.name)
		for j := 2; used[propName]; j++ {
			propName = fmt.Sprintf("%s%d", exportedFieldName(f.name), j)
		}
		used[propName] = true

		nullable := f.optional
		var typ string
		switch {
		case len(f.choices) == 0:
			typ = csharpType(f.jsonType, localTypes{}, f.optional, resolve)
		case f.jsonType.HasBase("array"):
			// Per the spec the choices of an array are its allowed items,
			// so the enum is the item type and the field a List of it.
			typ = s.arrayChoicesType(ownerName, f, resolve)
		default:
			enumName, hasNull := s.buildEnumField(ownerName, f, choiceWireType(f.jsonType, f.choices))
			typ = enumName
			if hasNull {
				nullable = true
			}
			if nullable {
				typ += "?"
			}
		}

		doc := f.docs
		var fieldDocLines []string
		if doc != "" {
			fieldDocLines = docLines(doc)
		}

		out[i] = recordField{propName: propName, typeExpr: typ, wireName: f.name, nullable: nullable, docLines: fieldDocLines}
	}
	return orderRequiredFirst(out)
}

// orderRequiredFirst stable-partitions record components into required
// fields first, then optional (nullable/defaulted) ones.
//
// This is necessary, not cosmetic: a C# positional record's primary
// constructor is an ordinary C# parameter list under the hood, and C#
// (CS1737) rejects a required parameter appearing after an optional one -
// exactly the same rule as any method signature. The wire spec has no
// such ordering guarantee for an endpoint's arguments/attributes (a real
// package - reservepay's - interleaves them freely), so declaration order
// can't just mirror source order the way every other target's field
// rendering safely does. Only the C# declaration order changes here -
// each field's own [JsonPropertyName] wire mapping is unaffected, so
// nothing about what actually gets sent/received changes, only the order
// callers list constructor arguments in.
func orderRequiredFirst(fields []recordField) []recordField {
	out := make([]recordField, 0, len(fields))
	for _, f := range fields {
		if !f.nullable {
			out = append(out, f)
		}
	}
	for _, f := range fields {
		if f.nullable {
			out = append(out, f)
		}
	}
	return out
}

// arrayChoicesType renders an array field that has choices/values. When
// the field is a plain array whose choices are all strings, all numbers or
// all booleans, the choices are the allowed items: the result is a List of
// a generated enum (an explicit null among them makes the items nullable,
// List<Enum?>, but not the field itself). Otherwise there is no
// enum to build - items of mixed kinds, or an array mixed with a scalar -
// and the field keeps the type it would have without choices. Mirrors
// javagen's arrayChoicesType.
func (s *recordSet) arrayChoicesType(ownerName string, f field, resolve typeResolver) string {
	wireType := ""
	if isSingleArray(f.jsonType) {
		wireType = itemWireType(f.jsonType, f.choices)
	}
	if wireType == "" {
		return csharpType(f.jsonType, localTypes{}, f.optional, resolve)
	}
	enumName, hasNull := s.buildEnumField(ownerName, f, wireType)
	item := enumName
	if hasNull {
		// A JSON null cannot be read into a non-nullable enum (the
		// converter would be handed the null token and reject it), so the
		// items are nullable: System.Text.Json maps a null element straight
		// through without calling the converter.
		item += "?"
	}
	typ := "List<" + item + ">"
	if f.optional {
		typ += "?"
	}
	return typ
}

// buildEnumField generates a real C# enum type (plus its paired
// JsonConverter) with the given wire type for a field's choices/values
// and registers both in
// s.ordered. Returns the enum's name and whether an explicit null was
// among the choices - a null entry means the wire value itself may
// legally be absent/null, on top of whatever the endpoint's own
// required/nullable flag says, so the field must be treated as nullable
// regardless (System.Text.Json maps a JSON null straight through for a
// nullable-annotated property without ever calling the converter's Read,
// so no constant is generated for it - only the field's own nullability
// needs adjusting). Mirrors javagen's buildEnumField exactly, adapted to
// C#'s [JsonConverter]-on-the-enum mechanism.
func (s *recordSet) buildEnumField(ownerName string, f field, wireType string) (name string, hasNull bool) {
	enumName := s.uniqueName(ownerName + exportedTypeName(f.name) + "Choice")
	converterName := enumName + "Converter"

	usedLabels := map[string]bool{}
	// usedLiterals dedupes by the RENDERED wire literal, not the raw
	// source value - a real bug hit against reservepay's actual package:
	// two choices producing the same wire literal (a true duplicate
	// value, or two source values that happen to format identically)
	// used to get distinct labels (Active, Active2) via usedLabels alone,
	// but IDENTICAL literals - which compiles to two case arms in the
	// converter's Read() switch expression matching the same pattern,
	// and C# rejects the second as unreachable (CS8510). Skipping a
	// literal that's already been used is also the semantically correct
	// behavior regardless of the compile error: two distinct C# enum
	// members that both represent the identical wire value would be a
	// redundant (and misleading - decoding could only ever produce the
	// first one) model of "the closed set of legal values" in the first
	// place.
	usedLiterals := map[string]bool{}
	var constants []enumConstant
	for _, c := range f.choices {
		if c == nil {
			hasNull = true
			continue
		}
		literal := choiceWireLiteral(c, wireType)
		if usedLiterals[literal] {
			continue
		}
		usedLiterals[literal] = true

		label := choiceConstantLabel(c)
		for i := 2; usedLabels[label]; i++ {
			label = fmt.Sprintf("%s%d", choiceConstantLabel(c), i)
		}
		usedLabels[label] = true
		constants = append(constants, enumConstant{label: label, literal: literal})
	}

	doc := enumName + " is the closed set of legal values for \"" + f.name + "\"."
	s.ordered = append(s.ordered, &enumDef{name: enumName, converter: converterName, doc: doc, wireType: wireType, constants: constants})
	return enumName, hasNull
}

// render writes out every collected record/enum/converter definition, in
// the order they were created.
func (s *recordSet) render(b *strings.Builder) {
	for _, decl := range s.ordered {
		decl.render(b)
	}
}
