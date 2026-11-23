package pythongen

import (
	"fmt"
	"strings"
	"unicode"
)

// pythonKeywords is every Python 3 reserved word - unlike every other
// target in this suite, Claude doesn't have to hand-derive or guess at
// this list from scratch, since it's small, fixed, and exactly what
// Python's own stdlib keyword.iskeyword() checks against (the generator
// itself is written in Go, so it can't call that function directly - this
// is just its contents transcribed once). Soft keywords (match, case, _,
// type) are deliberately NOT included: they're only reserved in specific
// syntactic positions (PEP 634) and remain completely legal as ordinary
// identifiers everywhere else, unlike this list.
var pythonKeywords = map[string]bool{
	"False": true, "None": true, "True": true, "and": true, "as": true,
	"assert": true, "async": true, "await": true, "break": true, "class": true,
	"continue": true, "def": true, "del": true, "elif": true, "else": true,
	"except": true, "finally": true, "for": true, "from": true, "global": true,
	"if": true, "import": true, "in": true, "is": true, "lambda": true,
	"nonlocal": true, "not": true, "or": true, "pass": true, "raise": true,
	"return": true, "try": true, "while": true, "with": true, "yield": true,
}

// camelCase/pascalCase/splitWords mirror gogen's/javagen's/csharpgen's
// exactly (duplicated rather than imported, matching the established
// convention) - used ONLY for generated CLASS names (Python classes are
// PascalCase by convention, same as every other target's type names).
// Field and method names deliberately do NOT go through this - see
// pythonFieldName below.
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

func isUpper(r rune) bool { return r >= 'A' && r <= 'Z' }

func pascalCase(name string) string {
	words := splitWords(name)
	if len(words) == 0 {
		return ""
	}
	var b strings.Builder
	for _, w := range words {
		b.WriteString(strings.ToUpper(w[:1]))
		if len(w) > 1 {
			b.WriteString(strings.ToLower(w[1:]))
		}
	}
	return b.String()
}

// pythonClassName renders name as a PascalCase Python class name, escaping
// a leading digit (Python identifiers can't start with one) and a keyword
// collision (vanishingly unlikely for a PascalCase name in practice, since
// no Python keyword is capitalized this way except True/False/None -
// handled defensively anyway).
func pythonClassName(name string) string {
	p := pascalCase(name)
	if p == "" {
		return "Type"
	}
	if unicode.IsDigit(rune(p[0])) {
		p = "Type" + p
	}
	if pythonKeywords[p] {
		p += "_"
	}
	return p
}

// pythonFieldName renders a wire name (e.g. "list-items", "2fa_enabled")
// as a Python field/method/parameter identifier.
//
// Deliberately NOT case-folded or word-split the way pythonClassName is -
// dash-to-underscore is the ONLY transformation, exactly matching
// webfunction-python's own naming convention (its Client.__getattr__ maps
// "list_items" -> "list-items" via a plain replace, no case-folding at
// all) - genuinely the simplest field-naming story of any target in this
// suite, same property already true of the library itself.
//
// A leading digit gets a "field_" prefix (mirrors rubygen's identical
// fallback for the same underlying problem - a wire name that can't be a
// legal identifier verbatim). A keyword collision gets a single trailing
// underscore - the real, PEP 8-blessed Python idiom (class_, import_) -
// rather than another target's prefix-based workaround. Non-identifier
// characters (anything not a letter, digit, or underscore) are replaced
// with "_" defensively, though wire names are expected to already be
// identifier-safe aside from dashes.
func pythonFieldName(wireName string) string {
	var b strings.Builder
	for _, r := range wireName {
		switch {
		case r == '-':
			b.WriteRune('_')
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	name := b.String()
	if name == "" {
		return "field"
	}
	if unicode.IsDigit(rune(name[0])) {
		name = "field_" + name
	}
	if pythonKeywords[name] {
		name += "_"
	}
	return name
}

// clientReserved is what a generated per-endpoint method name can't
// collide with: the Client/AsyncClient classes' own escape-hatch member
// names, plus every Python keyword. Mirrors gogen's goReserved/javagen's
// clientReserved/csharpgen's clientReserved.
var clientReserved = func() map[string]bool {
	m := map[string]bool{
		"call": true, "package": true, "bearer_auth": true, "version": true,
		"pipeline": true, "create_new": true, "aclose": true, "close": true,
	}
	for k := range pythonKeywords {
		m[strings.ToLower(k)] = true
	}
	return m
}()

// uniqueMethodName suffixes a name with an incrementing number if it
// collides with a reserved name or one already used on the Client class -
// mirrors every other target's identical helper.
func uniqueMethodName(used map[string]bool, name string) string {
	candidate := name
	if clientReserved[candidate] {
		candidate += "2"
	}
	for i := 2; used[candidate]; i++ {
		candidate = fmt.Sprintf("%s%d", name, i)
	}
	used[candidate] = true
	return candidate
}