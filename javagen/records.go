package javagen

import (
	"fmt"
	"strings"

	"github.com/webfunction-protocol/webfunction-go"
)

// field is a name/type/optional/docs/choices tuple, used to build a
// record from either an endpoint's Attributes (return shape) or its
// Arguments (call shape), or an object.<n>'s own Arguments/Attributes
// when resolving a named ref. Mirrors gogen's field.
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
// including the trivial case of no fields at all. Mirrors gogen's
// allOptional. Used to decide whether an endpoint method gets a
// convenience no-args overload alongside its full-args one - Java's real
// method overloading makes this a cleaner fit than Go's variadic-args
// workaround (see javagen.go's writeMethod).
func allOptional(fields []field) bool {
	for _, f := range fields {
		if !f.optional {
			return false
		}
	}
	return true
}

// javaTypeDecl is one generated top-level-nested type: either a record
// or an enum. Both render into the same ordered stream inside the
// generated Client class.
type javaTypeDecl interface {
	render(b *strings.Builder)
}

type recordField struct {
	javaName string
	javaType string
	wireName string
	nullable bool
	docLines []string
}

// recordDef is a generated Java record type.
type recordDef struct {
	name       string
	doc        string
	fields     []recordField
	argContext bool // true when this record is ever serialized back out as call arguments - see render()
}

func (r *recordDef) render(b *strings.Builder) {
	if r.doc != "" {
		writeJavadoc(b, "    ", docLines(r.doc))
	}
	if r.argContext {
		// Drops null optional fields from the Map<String,Object> Jackson
		// produces when converting this record back into call arguments
		// (see javagen.go's toArgsMap) - the same semantic gogen's
		// `omitempty` json tag gives an unset pointer field. Only needed
		// on records used as arguments; a Result record is only ever
		// deserialized, never re-serialized, so it's omitted there.
		b.WriteString("    @com.fasterxml.jackson.annotation.JsonInclude(com.fasterxml.jackson.annotation.JsonInclude.Include.NON_NULL)\n")
	}
	b.WriteString("    public record " + r.name + "(\n")
	for i, f := range r.fields {
		writeComment := f.docLines
		if len(writeComment) > 0 {
			writeJavadoc(b, "        ", writeComment)
		}
		nullableAnn := ""
		if f.nullable {
			nullableAnn = "@Nullable "
		}
		comma := ","
		if i == len(r.fields)-1 {
			comma = ""
		}
		b.WriteString(fmt.Sprintf("        %s%s %s%s\n", nullableAnn, f.javaType, f.javaName, comma))
	}
	b.WriteString("    ) {\n")

	// Explicit, @JsonCreator-annotated canonical constructor - the same
	// pattern webfunction-java's own model records use (see e.g.
	// Package.java), and for the identical reason: relying on Jackson's
	// automatic record-parameter-name detection combined with a
	// snake_case naming strategy was found to throw an
	// illegal-field-access error on Jackson 2.14 (it fell back to
	// setting the record's final fields directly via reflection instead
	// of using the constructor). Explicit @JsonProperty names sidestep
	// the issue regardless of Jackson version.
	b.WriteString("        @com.fasterxml.jackson.annotation.JsonCreator\n")
	b.WriteString("        public " + r.name + "(\n")
	for i, f := range r.fields {
		comma := ","
		if i == len(r.fields)-1 {
			comma = ""
		}
		b.WriteString(fmt.Sprintf("            @com.fasterxml.jackson.annotation.JsonProperty(%q) %s %s%s\n", f.wireName, f.javaType, f.javaName, comma))
	}
	b.WriteString("        ) {\n")
	for _, f := range r.fields {
		b.WriteString("            this." + f.javaName + " = " + f.javaName + ";\n")
	}
	b.WriteString("        }\n")
	b.WriteString("    }\n\n")
}

type enumConstant struct {
	label   string
	literal string
}

// enumDef is a generated Java enum type for an Argument's "choices" or
// Attribute's "values" - a real enforced enum type, not just a doc
// comment the way gogen's choicesNote is for Go (Java has a real enum
// mechanism Go lacks - see javagen.go's package doc comment).
type enumDef struct {
	name      string
	doc       string
	wireType  string // String, Long, Double, or Boolean - see choiceWireType
	constants []enumConstant
}

func (e *enumDef) render(b *strings.Builder) {
	if e.doc != "" {
		writeJavadoc(b, "    ", docLines(e.doc))
	}
	b.WriteString("    public enum " + e.name + " {\n")
	for i, c := range e.constants {
		term := ","
		if i == len(e.constants)-1 {
			term = ";"
		}
		b.WriteString("        " + c.label + "(" + c.literal + ")" + term + "\n")
	}
	b.WriteString("\n")
	b.WriteString("        private final " + e.wireType + " wireValue;\n\n")
	b.WriteString("        " + e.name + "(" + e.wireType + " wireValue) { this.wireValue = wireValue; }\n\n")
	b.WriteString("        @com.fasterxml.jackson.annotation.JsonValue\n")
	b.WriteString("        public " + e.wireType + " toWireValue() { return wireValue; }\n\n")
	b.WriteString("        @com.fasterxml.jackson.annotation.JsonCreator\n")
	b.WriteString("        public static " + e.name + " fromWireValue(" + e.wireType + " wireValue) {\n")
	b.WriteString("            for (" + e.name + " v : values()) {\n")
	b.WriteString("                if (v.wireValue.equals(wireValue)) return v;\n")
	b.WriteString("            }\n")
	b.WriteString("            throw new IllegalArgumentException(\"unknown value: \" + wireValue);\n")
	b.WriteString("        }\n")
	b.WriteString("    }\n\n")
}

// recordSet builds the set of named Java record/enum types needed for a
// package: endpoint argument shapes, endpoint return shapes, resolved
// named object definitions (object.<n> refs), and any per-field choices
// enums encountered along the way. Mirrors gogen's structSet.
type recordSet struct {
	pkg     *webfunction.Package
	ordered []javaTypeDecl
	names   map[string]bool
	// objects memoizes resolved object.<n> records, keyed by
	// "<context>:<name>" - same reasoning as gogen's structSet.objects:
	// the same object name can resolve differently per context, and this
	// also guards against infinite recursion on self-referential
	// objects.
	objects map[string]string
}

func newRecordSet(pkg *webfunction.Package) *recordSet {
	return &recordSet{pkg: pkg, names: map[string]bool{}, objects: map[string]string{}}
}

// uniqueName mirrors gogen's structSet.uniqueName - shared across
// records AND enums, since both are nested types within the same outer
// Client class and so share one Java identifier namespace.
func (s *recordSet) uniqueName(base string) string {
	name := base
	for i := 2; s.names[name]; i++ {
		name = fmt.Sprintf("%s%d", base, i)
	}
	s.names[name] = true
	return name
}

// forFields builds a record describing this set of fields and returns
// its name, or "" if fields is empty (nothing to describe). Always
// creates a fresh record - mirrors gogen's structSet.forFields: two
// endpoints coincidentally sharing an identical shape are not the same
// concept, even though the records would be structurally interchangeable.
func (s *recordSet) forFields(baseName, doc string, fields []field, context string) string {
	if len(fields) == 0 {
		return ""
	}
	name := s.uniqueName(baseName)
	resolve := func(refName string) string { return s.resolveObject(refName, context) }
	s.ordered = append(s.ordered, &recordDef{
		name:       name,
		doc:        doc,
		fields:     s.renderFields(name, fields, resolve),
		argContext: context == "argument",
	})
	return name
}

// resolveObject returns the record name for a named object.<n>
// reference, building it (recursively - the context propagates into any
// nested object refs within it, per spec) if it hasn't been resolved yet
// in this context. Falls back to "Map<String, Object>" if the referenced
// object doesn't exist, or has no members defined for this context.
// Mirrors gogen's structSet.resolveObject.
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
		s.objects[key] = "Map<String, Object>"
		return "Map<String, Object>"
	}

	recordName := s.uniqueName(exportedTypeName(name) + suffix)
	// Register before rendering fields, so a self-referential (or
	// mutually referential) object resolves to this name instead of
	// recursing forever.
	s.objects[key] = recordName

	resolve := func(refName string) string { return s.resolveObject(refName, context) }
	doc := recordName + " is the \"" + name + "\" object definition."
	s.ordered = append(s.ordered, &recordDef{
		name:       recordName,
		doc:        doc,
		fields:     s.renderFields(recordName, fields, resolve),
		argContext: context == "argument",
	})
	return recordName
}

// renderFields turns fields into fully-rendered record components,
// resolving any object.<n> references encountered via resolve,
// generating a choices enum per field that declares one (see
// buildEnumField), and deduping Java field names within the record.
func (s *recordSet) renderFields(ownerName string, fields []field, resolve typeResolver) []recordField {
	used := map[string]bool{}
	out := make([]recordField, len(fields))
	for i, f := range fields {
		javaName := exportedFieldName(f.name)
		for j := 2; used[javaName]; j++ {
			javaName = fmt.Sprintf("%s%d", exportedFieldName(f.name), j)
		}
		used[javaName] = true

		nullable := f.optional
		var typ string
		switch {
		case len(f.choices) == 0:
			typ = javaType(f.jsonType, localTypes{}, f.optional, resolve)
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
		}

		doc := f.docs
		var fieldDocLines []string
		if doc != "" {
			fieldDocLines = docLines(doc)
		}

		out[i] = recordField{javaName: javaName, javaType: typ, wireName: f.name, nullable: nullable, docLines: fieldDocLines}
	}
	return out
}

// arrayChoicesType renders an array field that has choices/values. When
// the field is a plain array whose choices are all strings, all numbers or
// all booleans, the choices are the allowed items: the result is a List of
// a generated enum (an explicit null among them is simply a null list
// element, so it does not make the field nullable). Otherwise there is no
// enum to build - items of mixed kinds, or an array mixed with a scalar -
// and the field keeps the type it would have without choices.
func (s *recordSet) arrayChoicesType(ownerName string, f field, resolve typeResolver) string {
	wireType := ""
	if isSingleArray(f.jsonType) {
		wireType = itemWireType(f.jsonType, f.choices)
	}
	if wireType == "" {
		return javaType(f.jsonType, localTypes{}, f.optional, resolve)
	}
	enumName, _ := s.buildEnumField(ownerName, f, wireType)
	return "List<" + enumName + ">"
}

// buildEnumField generates a real Java enum type for a field's
// choices/values (see javagen.go's package doc comment, decision 3) with
// the given wire type, and registers it in s.ordered. Returns the enum's name and whether an
// explicit null was among the choices - a null entry means the wire
// value itself may legally be absent/null, on top of whatever the
// endpoint's own required/nullable flag says, so the field must be
// treated as nullable regardless (Jackson maps a JSON null straight to a
// null field without ever calling the enum's @JsonCreator, so no
// constant is generated for it - only the field's own nullability needs
// adjusting).
func (s *recordSet) buildEnumField(ownerName string, f field, wireType string) (name string, hasNull bool) {
	enumName := s.uniqueName(ownerName + exportedTypeName(f.name) + "Choice")

	usedLabels := map[string]bool{}
	var constants []enumConstant
	for _, c := range f.choices {
		if c == nil {
			hasNull = true
			continue
		}
		label := choiceConstantLabel(c)
		for i := 2; usedLabels[label]; i++ {
			label = fmt.Sprintf("%s_%d", choiceConstantLabel(c), i)
		}
		usedLabels[label] = true
		constants = append(constants, enumConstant{label: label, literal: choiceWireLiteral(c, wireType)})
	}

	doc := enumName + " is the closed set of legal values for \"" + f.name + "\"."
	s.ordered = append(s.ordered, &enumDef{name: enumName, doc: doc, wireType: wireType, constants: constants})
	return enumName, hasNull
}

// render writes out every collected record/enum definition, in the
// order they were created.
func (s *recordSet) render(b *strings.Builder) {
	for _, decl := range s.ordered {
		decl.render(b)
	}
}
