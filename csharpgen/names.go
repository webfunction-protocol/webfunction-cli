package csharpgen

import (
	"fmt"
	"strings"
	"unicode"
)

// camelCase/pascalCase/splitWords mirror gogen's/javagen's exactly
// (duplicated rather than imported - matches the existing convention:
// every target already duplicates these rather than importing jsgen's).
func camelCase(name string) string {
	words := splitWords(name)
	if len(words) == 0 {
		return ""
	}
	var b strings.Builder
	for i, w := range words {
		if i == 0 {
			b.WriteString(strings.ToLower(w))
			continue
		}
		b.WriteString(strings.ToUpper(w[:1]))
		if len(w) > 1 {
			b.WriteString(strings.ToLower(w[1:]))
		}
	}
	return b.String()
}

// pascalCase is camelCase with the first letter upper-cased - used for
// every generated C# type name (records, enums, methods, properties),
// matching C#'s own convention that both type names AND method/property
// names are PascalCase (unlike Java, where only type names are).
func pascalCase(name string) string {
	c := camelCase(name)
	if c == "" {
		return c
	}
	return strings.ToUpper(c[:1]) + c[1:]
}

func splitWords(name string) []string {
	var words []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			words = append(words, cur.String())
			cur.Reset()
		}
	}
	runes := []rune(name)
	for i, r := range runes {
		switch {
		case r == '-' || r == '_' || r == ' ':
			flush()
		case i > 0 && isUpper(r) && !isUpper(runes[i-1]):
			flush()
			cur.WriteRune(r)
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return words
}

func isUpper(r rune) bool {
	return r >= 'A' && r <= 'Z'
}

// csharpKeywords is the set of C# reserved words that can't be used as an
// identifier without the "@" verbatim-identifier escape (see identifier
// below). Contextual keywords (var, record, async, etc) are deliberately
// NOT included - they're only reserved in specific syntactic positions and
// remain legal as plain identifiers everywhere else, unlike this list.
var csharpKeywords = map[string]bool{
	"abstract": true, "as": true, "base": true, "bool": true, "break": true,
	"byte": true, "case": true, "catch": true, "char": true, "checked": true,
	"class": true, "const": true, "continue": true, "decimal": true, "default": true,
	"delegate": true, "do": true, "double": true, "else": true, "enum": true,
	"event": true, "explicit": true, "extern": true, "false": true, "finally": true,
	"fixed": true, "float": true, "for": true, "foreach": true, "goto": true,
	"if": true, "implicit": true, "in": true, "int": true, "interface": true,
	"internal": true, "is": true, "lock": true, "long": true, "namespace": true,
	"new": true, "null": true, "object": true, "operator": true, "out": true,
	"override": true, "params": true, "private": true, "protected": true, "public": true,
	"readonly": true, "ref": true, "return": true, "sbyte": true, "sealed": true,
	"short": true, "sizeof": true, "stackalloc": true, "static": true, "string": true,
	"struct": true, "switch": true, "this": true, "throw": true, "true": true,
	"try": true, "typeof": true, "uint": true, "ulong": true, "unchecked": true,
	"unsafe": true, "ushort": true, "using": true, "virtual": true, "void": true,
	"volatile": true, "while": true,
}

// identifier renders name (already camelCase or PascalCase) as a legal C#
// identifier: prefixed with "_" if it would otherwise start with a digit
// (C# identifiers can't start with a digit), or with "@" - C#'s real
// verbatim-identifier escape - if it collides with a reserved keyword.
// The "@" mechanism is a genuine C#-only convenience worth using rather
// than importing another target's workaround: unlike Java (forced trailing
// underscore) or Go (no collision possible, Go has no field named after a
// keyword concern the same way), "@class"/"@event"/"@new" are real,
// idiomatic C# identifiers - "@" strips cleanly and the name underneath
// stays exactly what the wire name would suggest, with no cosmetic suffix.
func identifier(name string) string {
	if name == "" {
		return name
	}
	if unicode.IsDigit(rune(name[0])) {
		name = "_" + name
	}
	if csharpKeywords[name] {
		return "@" + name
	}
	return name
}

// exportedFieldName renders name as a valid C# property identifier:
// PascalCase (C# convention - unlike Java, C# properties are PascalCase,
// not camelCase), with keyword/digit-start handling via identifier().
func exportedFieldName(name string) string {
	p := pascalCase(name)
	if p == "" {
		return "Field"
	}
	return identifier(p)
}

// exportedTypeName is exportedFieldName's counterpart for generated
// record/enum type names - same PascalCase rendering, since C# uses
// PascalCase for both.
func exportedTypeName(name string) string {
	p := pascalCase(name)
	if p == "" {
		return "Type"
	}
	return identifier(p)
}

// parameterName renders name as a valid C# method-parameter identifier:
// camelCase (C# convention for parameters/locals, as opposed to
// PascalCase for properties/methods/types), with keyword/digit-start
// handling via identifier().
func parameterName(name string) string {
	c := camelCase(name)
	if c == "" {
		return "value"
	}
	return identifier(c)
}

// clientReserved is what a generated per-endpoint method name can't
// collide with: the Client class's own escape-hatch member names, plus
// every C# keyword. Mirrors gogen's goReserved/javagen's clientReserved.
var clientReserved = func() map[string]bool {
	m := map[string]bool{
		"call": true, "callasync": true, "package": true, "bearerauth": true,
		"version": true, "pipeline": true, "newclientasync": true, "client": true,
		"tostring": true, "equals": true, "gethashcode": true, "gettype": true,
	}
	for k := range csharpKeywords {
		m[k] = true
	}
	return m
}()

// uniqueMethodName suffixes a name with an incrementing number if it
// collides with a reserved name or one already used on the Client class -
// mirrors gogen's/javagen's identical helper.
func uniqueMethodName(used map[string]bool, name string) string {
	candidate := name
	if clientReserved[strings.ToLower(candidate)] {
		candidate += "2"
	}
	for i := 2; used[strings.ToLower(candidate)]; i++ {
		candidate = fmt.Sprintf("%s%d", name, i)
	}
	used[strings.ToLower(candidate)] = true
	return candidate
}

// csharpNamespace derives a valid, idiomatic C# namespace from the shared
// --namespace flag (see cmd/codegen.go). Like Java, C# namespaces are
// naturally dot-separated, so each segment maps directly - but unlike
// Java's convention (all lower-case, reverse-domain style), idiomatic C#
// namespace segments are PascalCase (e.g. "MyCompany.MyProduct"), matching
// webfunction-csharp's own "WebFunction" namespace and the shared flag's
// own default value ("WebFunctionClient", already PascalCase). Each
// dot-separated segment is sanitized independently: non-letter/digit
// characters dropped, PascalCased, and escaped via identifier() if it
// would start with a digit or collide with a keyword. A segment that is a
// single word written entirely in capitals (an acronym such as "SDK") is
// kept as written rather than re-cased to "Sdk". Falls back to
// "WebFunctionClient" for an empty or fully-invalid namespace.
func csharpNamespace(namespace string) string {
	rawSegments := strings.Split(namespace, ".")
	var segments []string
	for _, seg := range rawSegments {
		// Keep letters/digits AND the separator characters splitWords
		// itself looks for ('-', '_', ' ') - stripping separators here,
		// before pascalCase gets to run its own word-splitting, would
		// destroy the very word-boundary information splitWords needs
		// ("my-company_thing" collapsing to one lower-case blob
		// "Mycompanything" instead of "MyCompanyThing" - a real bug
		// caught by a direct edge-case test, not by the fixture, which
		// never happened to use a hyphenated/underscored namespace
		// segment). Any other character is dropped.
		var b strings.Builder
		for _, r := range seg {
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == ' ' {
				b.WriteRune(r)
			}
		}
		s := pascalCase(b.String())
		if isAllCapsWord(b.String()) {
			s = b.String()
		}
		if s == "" {
			continue
		}
		segments = append(segments, identifier(s))
	}
	if len(segments) == 0 {
		return "WebFunctionClient"
	}
	return strings.Join(segments, ".")
}

// isAllCapsWord reports whether s is a single word written entirely in
// capital letters, such as "SDK" or "API2": no lower-case letters and no
// separators ('-', '_', ' ').
func isAllCapsWord(s string) bool {
	for _, r := range s {
		if !unicode.IsUpper(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}
