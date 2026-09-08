package rubygen

import (
	"fmt"
	"strings"
)

// rubyMethodName converts a wire endpoint name to the exact method name
// the real WebFunction::Client answers to. Confirmed directly from the
// real gem's Client#initialize (client.rb):
//
//	@endpoints = endpoints.to_h { |e| [e.gsub("-", "_").to_sym, e] }
//
// This is dash-to-underscore ONLY - no camelCase folding, unlike JS's
// client. A generated method name that doesn't match this exactly would
// type-check a call the real dynamic dispatcher actually raises
// NoMethodError on (see method_missing/respond_to_missing?, which look up
// the method name verbatim in this same map).
func rubyMethodName(wireName string) string {
	return strings.ReplaceAll(wireName, "-", "_")
}

// snakeCase folds an arbitrary name (dashes, underscores, existing
// camelCase) into snake_case - used for named RBS type-alias and
// interface identifiers built from object.<n> ref names or endpoint
// names, since those aren't necessarily already snake_case on the wire
// (a package could name an object anything).
func snakeCase(name string) string {
	words := splitWords(name)
	if len(words) == 0 {
		return ""
	}
	lower := make([]string, len(words))
	for i, w := range words {
		lower[i] = strings.ToLower(w)
	}
	return strings.Join(lower, "_")
}

// splitWords splits on dashes, underscores, spaces, and existing
// camelCase boundaries - same logic as every other target's splitWords.
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

// rubyIdentifier reports whether s is a valid bareword hash-key/keyword-
// argument identifier in Ruby (letters/digits/underscore, not starting
// with a digit) - used to decide whether a symbol can be rendered as a
// bare RBS record-type key (`name:`) or needs quoting (`"2fa-enabled":`).
func rubyIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_':
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// recordKey renders a field name as an RBS record-type key: a bare
// `name:` when it's a valid identifier, or a quoted `"name":` otherwise
// (RBS record types accept a quoted-string key form for exactly this
// case, mirroring a Ruby Hash literal's `"2fa-enabled": true` shorthand).
func recordKey(name string) string {
	if rubyIdentifier(name) {
		return name + ":"
	}
	return fmt.Sprintf("%q:", name)
}

// clientReserved is the fixed, non-endpoint surface every generated sig
// declares on WebFunction::Client (the real gem's own fixed API - call,
// package, mutators - confirmed directly from client.rb) plus BasicObject
// survivors (nil?, methods). An endpoint literally named one of these
// collides and gets suffixed, mirroring every other target's call->call2
// handling.
var clientReserved = map[string]bool{
	"call": true, "package": true, "bearer_auth=": true,
	"version=": true, "pipeline=": true, "initialize": true,
	"from_package_endpoint": true, "class": true,
}

// uniqueMethodName suffixes name with an incrementing number if it
// collides with the fixed client surface or one already used by this
// package's own endpoints.
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

func uniqueName(used map[string]bool, base string) string {
	name := base
	for i := 2; used[name]; i++ {
		name = fmt.Sprintf("%s%d", base, i)
	}
	used[name] = true
	return name
}