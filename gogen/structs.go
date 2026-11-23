package gogen

import (
	"fmt"
	"strings"

	"github.com/webfunction-protocol/webfunction-go"
)

// field is a name/type/optional/docs tuple, used to build a struct from
// either an endpoint's Attributes (return shape) or its Arguments (call
// shape), or an object.<name>'s own Arguments/Attributes when resolving
// a named ref. Mirrors jsgen's field/phpgen's field.
//
// Unlike jsgen (which tracks optional and nullable separately) or phpgen
// (same split), Go's type-building only needs one flag: optional drives
// both "may the key be absent" (the JSON tag's omitempty) and "is the Go
// field a pointer" - see types.go's goType for why the two concepts
// collapse into one here.
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
// including the trivial case of no fields at all. Mirrors jsgen/phpgen's
// allOptional. Used to decide whether an endpoint's whole args parameter
// can be made variadic (0-or-1, simulating an optional parameter - Go
// has no default-argument syntax, unlike JS/PHP) rather than mandatory.
func allOptional(fields []field) bool {
	for _, f := range fields {
		if !f.optional {
			return false
		}
	}
	return true
}

type renderedField struct {
	goName   string
	goType   string
	jsonTag  string
	docLines []string
}

type structDef struct {
	name   string
	doc    string
	fields []renderedField
}

// structSet builds the set of named Go struct types needed for a
// package: endpoint argument shapes, endpoint return shapes, and
// resolved named object definitions (object.<name> refs). Mirrors
// jsgen's typedefSet/phpgen's aliasSet.
type structSet struct {
	pkg     *webfunction.Package
	ordered []*structDef
	names   map[string]bool
	// objects memoizes resolved object.<name> structs, keyed by
	// "<context>:<name>" - same reasoning as typedefSet/aliasSet: the
	// same object name can resolve differently per context, and this
	// also guards against infinite recursion on self-referential
	// objects.
	objects map[string]string
}

func newStructSet(pkg *webfunction.Package) *structSet {
	return &structSet{pkg: pkg, names: map[string]bool{}, objects: map[string]string{}}
}

// forFields builds a struct describing this set of fields and returns
// its name, or "" if fields is empty (nothing to describe). Always
// creates a fresh struct - mirrors typedefSet.forFields/aliasSet's
// inline-shape equivalent: two endpoints coincidentally sharing an
// identical shape are not the same concept, even though the structs
// would be structurally interchangeable.
func (s *structSet) forFields(baseName, doc string, fields []field, context string) string {
	if len(fields) == 0 {
		return ""
	}
	name := s.uniqueName(baseName)
	resolve := func(refName string) string { return s.resolveObject(refName, context) }
	s.ordered = append(s.ordered, &structDef{name: name, doc: doc, fields: s.renderFields(fields, resolve)})
	return name
}

// resolveObject returns the struct name for a named object.<name>
// reference, building it (recursively - the context propagates into any
// nested object refs within it, per spec) if it hasn't been resolved yet
// in this context. Falls back to "map[string]any" if the referenced
// object doesn't exist, or has no members defined for this context.
// Mirrors typedefSet.resolveObjectTypedef/aliasSet.resolve.
func (s *structSet) resolveObject(name, context string) string {
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
		s.objects[key] = "map[string]any"
		return "map[string]any"
	}

	structName := s.uniqueName(pascalCase(name) + suffix)
	// Register before rendering fields, so a self-referential (or
	// mutually referential) object resolves to this name instead of
	// recursing forever.
	s.objects[key] = structName

	resolve := func(refName string) string { return s.resolveObject(refName, context) }
	doc := structName + " is the \"" + name + "\" object definition."
	s.ordered = append(s.ordered, &structDef{name: structName, doc: doc, fields: s.renderFields(fields, resolve)})
	return structName
}

// renderFields turns fields into fully-rendered Go struct fields,
// resolving any object.<name> references encountered via resolve, and
// deduping Go field names within the struct (two differently-cased or
// -separated wire names, e.g. "user-id" and "user_id", could otherwise
// collide once both are PascalCased to "UserId").
func (s *structSet) renderFields(fields []field, resolve structResolver) []renderedField {
	used := map[string]bool{}
	out := make([]renderedField, len(fields))
	for i, f := range fields {
		goName := exportedName(f.name)
		for j := 2; used[goName]; j++ {
			goName = fmt.Sprintf("%s%d", exportedName(f.name), j)
		}
		used[goName] = true

		typ := goType(narrowArrayItems(f.jsonType, f.choices), localStructs{}, f.optional, resolve)

		tag := f.name
		if f.optional {
			tag += ",omitempty"
		}

		doc := f.docs
		if note := choicesNote(f.choices); note != "" {
			doc = withNotes(doc, note)
		}
		var fieldDocLines []string
		if doc != "" {
			fieldDocLines = docLines(goName + " " + doc)
		}

		out[i] = renderedField{goName: goName, goType: typ, jsonTag: tag, docLines: fieldDocLines}
	}
	return out
}

func (s *structSet) uniqueName(base string) string {
	name := base
	for i := 2; s.names[name]; i++ {
		name = fmt.Sprintf("%s%d", base, i)
	}
	s.names[name] = true
	return name
}

// render writes out every collected struct definition, in the order
// they were created.
func (s *structSet) render(b *strings.Builder) {
	for _, sd := range s.ordered {
		if sd.doc != "" {
			b.WriteString("// " + sd.doc + "\n")
		}
		b.WriteString("type " + sd.name + " struct {\n")
		for _, f := range sd.fields {
			writeComment(b, "\t", f.docLines)
			b.WriteString(fmt.Sprintf("\t%s %s `json:%q`\n", f.goName, f.goType, f.jsonTag))
		}
		b.WriteString("}\n\n")
	}
}
