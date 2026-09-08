package rubygen

import "strings"

// docLines splits a docs string (markdown, per the wire spec) into lines
// safe to place as RBS `#`-prefixed comment lines directly above a
// declaration. Unlike JSDoc/PHPDoc, RBS comments have no closing token to
// accidentally break out of, so this is simpler than jsgen/phpgen's
// docLines - still trims trailing whitespace per line for tidiness.
func docLines(docs string) []string {
	docs = strings.TrimSpace(docs)
	if docs == "" {
		return nil
	}
	rawLines := strings.Split(docs, "\n")
	lines := make([]string, 0, len(rawLines))
	for _, l := range rawLines {
		lines = append(lines, strings.TrimRight(l, " \t\r"))
	}
	return lines
}

// writeDocComment writes docs (if any) as `#`-prefixed lines at the given
// indent, one per line.
func writeDocComment(b *strings.Builder, indent, docs string) {
	for _, l := range docLines(docs) {
		if l == "" {
			b.WriteString(indent + "#\n")
		} else {
			b.WriteString(indent + "# " + l + "\n")
		}
	}
}