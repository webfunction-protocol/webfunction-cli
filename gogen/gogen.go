// Package gogen generates a typed Go client for a webfunction package,
// targeting webfunction-go (github.com/webfunction-protocol/webfunction-go).
//
// Design decisions, and how they differ from jsgen/phpgen:
//
//  1. Real generated structs, not docs-only annotations. JS/PHP both
//     lean on a separate static-analysis layer (tsc/PHPStan/Psalm) that
//     treats JSDoc/PHPDoc comments as authoritative types while the
//     actual runtime stays dynamically typed underneath. Go has no
//     equivalent docs-only type layer - the compiler is the only
//     checker there is, so every type here has to be real and correct,
//     not just a comment.
//  2. Numeric refinements map to Go's real sized types (uint32/uint64/
//     int32/int64/float32/float64, int64 for timestamp) - more precise
//     than PHP's collapse to int|float, since Go (unlike PHP) actually
//     has sized integer/float types to use.
//  3. Optional Arguments and nullable Attributes both become Go pointer
//     types (*T). jsgen/phpgen track "optional" (key may be absent) and
//     "nullable" (value may be null) as two separate concepts their
//     target's docs-only type system can express independently; a Go
//     pointer already covers both at once, so they collapse into one
//     decision here (see types.go's goType).
//  4. Enum/choices (an Argument's "choices" or Attribute's "values") are
//     documented in a doc comment, not encoded as an enforced type - Go
//     has no literal-union type mechanism the way TypeScript/PHPStan
//     do. A field keeps its plain base type, with one exception: an
//     array whose items the type leaves unspecified (a bare "array")
//     takes its item type from the choices when they are all strings,
//     all numbers or all booleans, since the spec makes them the
//     allowed items (see types.go's narrowArrayItems). (A stronger
//     version - a generated named type plus declared constants - is a
//     real possible enhancement, not attempted here.)
//  5. A type union with more than one non-null alternative (e.g.
//     string|number) collapses to `any` - Go can't express real type
//     unions the way PHP or TypeScript can. This is a genuine
//     expressive-power gap in the target language, not a simplification
//     chosen for convenience.
//  6. Pagination needs a real generated wrapper struct per paginated
//     endpoint, same reasoning as phpgen (not jsgen): the real
//     webfunction-go Page.Items() returns []any, and - unlike JSDoc -
//     Go has no way to *claim* a stronger type via documentation alone;
//     every claimed type must be backed by real code.
//  7. Escape hatch: Call(name, args)/Package()/SetBearerAuth/SetVersion/
//     SetPipeline passthroughs, matching jsgen's .call/.package and
//     phpgen's call()/getPackage()/setBearerAuth/setVersion/setPipeline.
//  8. The shared --namespace flag (see cmd/codegen.go) is reused as the
//     generated Go package name, sanitized into Go's own lowercase-no-
//     underscore convention (see names.go's packageName) rather than
//     adding a separate flag just for this target.
//  9. A zero-argument endpoint gets NO args parameter at all - unlike
//     jsgen, which still types it as an optional `args?: any` escape
//     valve. Go's stricter type system doesn't benefit from an "accept
//     anything just in case" parameter, and the generated Client already
//     exposes a raw Call(name, args) escape hatch for exactly that
//     purpose.
//  10. An endpoint whose args are all individually optional (or has
//     none) still gets a variadic `args ...FooArgs` parameter rather
//     than none - Go has no default-argument syntax (unlike JS/PHP), so
//     variadic-as-optional-single-param is the idiomatic Go way to let
//     a caller omit it (`c.Foo()`) or pass it (`c.Foo(FooArgs{...})`).
package gogen

import (
	"fmt"
	"go/format"
	"strings"

	"github.com/webfunction-protocol/webfunction-go"

	"wfn/generics"
	"wfn/privatefilter"
)

// ImportPath is the Go module path the generated file imports - the
// webfunction-go package itself.
const ImportPath = "github.com/webfunction-protocol/webfunction-go"

// Generate builds a typed Go client for pkg, targeting webfunction-go.
// namespace is sanitized into a Go package name (see names.go's
// packageName) - the shared --namespace flag doesn't map onto Go's own
// package-naming convention directly.
//
// Generic objects are expanded first (generics.Expand): every application
// becomes an ordinary generated object such as ListOfUser, and the generic
// templates are not emitted.
//
// Anything flagged "private" - endpoints, arguments, and attributes - is
// left out of the generated code unless includePrivate is true.
func Generate(pkg *webfunction.Package, sourceURL, namespace string, includePrivate bool) (string, error) {
	pkg = generics.Expand(pkg)
	pkgName := packageName(namespace)

	if !includePrivate {
		pkg = privatefilter.Apply(pkg)
	}
	structs := newStructSet(pkg)
	endpoints := visibleEndpoints(pkg, includePrivate)

	// Pre-compute every endpoint's shapes first, so struct resolution
	// (which mutates structs as it goes, same as jsgen's typedefSet) is
	// complete before anything is rendered.
	infos := make(map[string]endpointInfo, len(endpoints))
	pageStructNames := map[string]bool{}
	for _, ep := range endpoints {
		infos[ep.Name] = buildEndpointInfo(structs, ep, pageStructNames)
	}

	var b strings.Builder
	writeHeader(&b, pkg, sourceURL, pkgName)

	structs.render(&b)

	writeClientStruct(&b, pkg, sourceURL, endpoints, infos)

	for _, ep := range endpoints {
		if info := infos[ep.Name]; info.pageWrapper != nil {
			writePageWrapper(&b, info.pageWrapper)
		}
	}

	// gofmt the output before returning it: canonicalizes struct field
	// column alignment etc, which the string-builder logic above doesn't
	// attempt itself - and doubles as a correctness check, since
	// format.Source fails on invalid Go syntax. A failure here means a
	// real bug in this generator, not in the input package.
	formatted, err := format.Source([]byte(b.String()))
	if err != nil {
		return "", fmt.Errorf("formatting generated go source (this is a gogen bug, not a problem with the input package): %w", err)
	}
	return string(formatted), nil
}

// endpointInfo holds everything about one endpoint needed to write its
// method (and, for a paginated endpoint, its companion wrapper struct).
type endpointInfo struct {
	argsType     string // struct name for args, or "" if the endpoint takes none
	argsOptional bool
	returnType   string       // Go type for a non-paginated return; ignored if pageWrapper != nil
	pageWrapper  *pageWrapper // non-nil for a paginated endpoint
}

type pageWrapper struct {
	structName string
	itemType   string
}

func buildEndpointInfo(structs *structSet, ep webfunction.Endpoint, pageStructNames map[string]bool) endpointInfo {
	argFields := argumentFields(ep.Arguments)
	argsDoc := ""
	if base := pascalCase(ep.Name); base != "" {
		argsDoc = base + "Args holds the arguments for the " + ep.Name + " endpoint."
	}
	argsType := structs.forFields(pascalCase(ep.Name)+"Args", argsDoc, argFields, "argument")

	if ep.HasFlag("paginated") {
		itemType := "any"
		resolve := func(refName string) string { return structs.resolveObject(refName, "attribute") }
		for _, a := range ep.Attributes {
			if a.Name != "page" {
				continue
			}
			for _, alt := range a.Type.Union {
				if alt.Base == "array" && alt.Of != nil {
					itemType = goType(*alt.Of, localStructs{}, false, resolve)
				}
			}
		}
		structName := uniqueStructName(pageStructNames, pascalCase(ep.Name)+"Page")
		return endpointInfo{
			argsType: argsType, argsOptional: allOptional(argFields),
			pageWrapper: &pageWrapper{structName: structName, itemType: itemType},
		}
	}

	local := forEndpointReturn(structs, ep)
	resolve := func(refName string) string { return structs.resolveObject(refName, "attribute") }
	returnType := goType(ep.Returns, local, false, resolve)

	return endpointInfo{
		argsType: argsType, argsOptional: allOptional(argFields),
		returnType: returnType,
	}
}

// forEndpointReturn builds localStructs describing the endpoint's own
// inline return shape from its attributes - mirrors jsgen's/phpgen's
// forEndpointReturn exactly (same spec-reading rationale: attributes
// describes the object itself for bare "object"; a bare, untyped "array"
// return with attributes describes each item's shape, honored even
// though the spec's letter only defines attributes for the "object"
// case, because real-world packages use it that way).
func forEndpointReturn(structs *structSet, ep webfunction.Endpoint) localStructs {
	if len(ep.Attributes) == 0 {
		return localStructs{}
	}
	fields := attributeFields(ep.Attributes)
	doc := pascalCase(ep.Name) + "Result is the result of the " + ep.Name + " endpoint."
	name := structs.forFields(pascalCase(ep.Name)+"Result", doc, fields, "attribute")
	if name == "" {
		return localStructs{}
	}

	switch {
	case ep.Returns.HasBase("object"):
		return localStructs{object: name}
	case ep.Returns.HasBareArray():
		return localStructs{arrayOfItem: name}
	default:
		return localStructs{}
	}
}

func visibleEndpoints(pkg *webfunction.Package, includePrivate bool) []webfunction.Endpoint {
	out := make([]webfunction.Endpoint, 0, len(pkg.Endpoints))
	for _, ep := range pkg.Endpoints {
		if !includePrivate && ep.HasFlag("private") {
			continue
		}
		out = append(out, ep)
	}
	return out
}

func uniqueStructName(used map[string]bool, base string) string {
	name := base
	for i := 2; used[name]; i++ {
		name = fmt.Sprintf("%s%d", base, i)
	}
	used[name] = true
	return name
}

func writeHeader(b *strings.Builder, pkg *webfunction.Package, sourceURL, pkgName string) {
	b.WriteString("// Code generated by wfn codegen --target go. DO NOT EDIT -\n")
	b.WriteString("// re-run codegen instead.\n")
	if pkg.Name != "" {
		b.WriteString("// Package: " + pkg.Name + "\n")
	}
	b.WriteString("// Source:  " + sourceURL + "\n")
	b.WriteString("//\n")
	b.WriteString("// IMPORTANT - go get " + ImportPath + "\n\n")
	b.WriteString("package " + pkgName + "\n\n")
	b.WriteString("import (\n")
	b.WriteString("\t\"encoding/json\"\n")
	b.WriteString("\t\"fmt\"\n\n")
	b.WriteString("\twebfunction \"" + ImportPath + "\"\n")
	b.WriteString(")\n\n")
	b.WriteString(helperFuncs)
}

// helperFuncs are two tiny shared helpers every generated method uses:
// toArgsMap builds the map[string]any webfunction.Client.Call expects
// from a typed args struct (round-tripping through JSON so an optional
// pointer field's `omitempty` tag drops it, exactly as an omitted
// argument should be), and decodeResult does the reverse for a call's
// raw result - the same marshal-then-unmarshal trick webfunction-go's
// own Client uses internally to turn a decoded `any` into a concrete
// struct (see webfunction-go's packageFromValue).
const helperFuncs = `func toArgsMap(v any) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("encoding args: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("encoding args: %w", err)
	}
	return m, nil
}

func decodeResult(raw any, target any) error {
	b, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("decoding result: %w", err)
	}
	if err := json.Unmarshal(b, target); err != nil {
		return fmt.Errorf("decoding result: %w", err)
	}
	return nil
}

`

func writeClientStruct(b *strings.Builder, pkg *webfunction.Package, sourceURL string, endpoints []webfunction.Endpoint, infos map[string]endpointInfo) {
	name := pkg.Name
	if name == "" {
		name = "(unnamed package)"
	}
	b.WriteString("// Client is a generated, typed client for the " + name + " webfunction package.\n")
	b.WriteString("// Source: " + sourceURL + "\n")
	b.WriteString("type Client struct {\n\traw *webfunction.Client\n}\n\n")

	b.WriteString("// NewClient builds a Client, fetching the package definition from its source URL.\n")
	b.WriteString("func NewClient(opts webfunction.Options) (*Client, error) {\n")
	b.WriteString("\traw, err := webfunction.FromPackageEndpoint(" + goStringLiteral(sourceURL) + ", opts)\n")
	b.WriteString("\tif err != nil {\n\t\treturn nil, err\n\t}\n")
	b.WriteString("\treturn &Client{raw: raw}, nil\n}\n\n")

	b.WriteString("// Call is an escape hatch for calling an endpoint this client doesn't have a typed method for.\n")
	b.WriteString("func (c *Client) Call(name string, args map[string]any) (any, error) {\n\treturn c.raw.Call(name, args)\n}\n\n")

	b.WriteString("// Package returns the underlying package definition.\n")
	b.WriteString("func (c *Client) Package() *webfunction.Package {\n\treturn c.raw.Package()\n}\n\n")

	b.WriteString("// SetBearerAuth updates the bearer token used on subsequent calls.\n")
	b.WriteString("func (c *Client) SetBearerAuth(bearerAuth string) {\n\tc.raw.SetBearerAuth(bearerAuth)\n}\n\n")

	b.WriteString("// SetVersion updates the API version used on subsequent calls.\n")
	b.WriteString("func (c *Client) SetVersion(version string) {\n\tc.raw.SetVersion(version)\n}\n\n")

	b.WriteString("// SetPipeline replaces the client's pipeline. Pass nil to make calls execute immediately again instead of batching.\n")
	b.WriteString("func (c *Client) SetPipeline(p *webfunction.Pipeline) {\n\tc.raw.SetPipeline(p)\n}\n\n")

	used := map[string]bool{}
	for _, ep := range endpoints {
		methodName := uniqueMethodName(used, pascalCase(ep.Name))
		writeMethod(b, ep, methodName, infos[ep.Name])
	}
}

// writeMethod writes one endpoint's method, including its doc comment.
//
// Error documentation is scoped to this endpoint's own declared Errors
// only - NOT merged with the package-level Errors list, matching
// jsgen/phpgen's identical scoping decision (and the same caution: an
// endpoint's Errors, not Package.Errors, is what's confirmed - see
// jsgen's writeMethod comment for the fuller reasoning).
func writeMethod(b *strings.Builder, ep webfunction.Endpoint, methodName string, info endpointInfo) {
	dl := docLines(ep.Docs)
	writeComment(b, "", dl)
	if len(dl) > 0 {
		b.WriteString("//\n")
	}

	for _, e := range ep.Errors {
		code := strings.TrimSpace(e.Code)
		if code == "" {
			continue
		}
		line := "May return *webfunction.BadRequestError with code " + goStringLiteral(code)
		errDocs := docLines(e.Docs)
		if len(errDocs) > 0 {
			line += " - " + errDocs[0]
			b.WriteString("// " + line + "\n")
			writeComment(b, "", errDocs[1:])
		} else {
			b.WriteString("// " + line + "\n")
		}
	}
	// Call-agnostic - webfunction-go can return either of these for any
	// endpoint regardless of what error codes (if any) that endpoint
	// itself declares, so they're always noted rather than keyed off
	// ep.Errors, matching jsgen/phpgen's identical treatment.
	b.WriteString("// May also return *webfunction.UnexpectedStatusCodeError (non-200/400 response) or *webfunction.JsonParseError (invalid JSON body).\n")

	nativeReturn := info.returnType
	if info.pageWrapper != nil {
		nativeReturn = "*" + info.pageWrapper.structName
	}
	zeroExpr := goZeroValue(nativeReturn)

	usesVariadic := info.argsType != "" && info.argsOptional
	var params string
	switch {
	case info.argsType == "":
		params = ""
	case usesVariadic:
		params = "args ..." + info.argsType
		b.WriteString("// args is optional - pass zero or one " + info.argsType + "{...}.\n")
	default:
		params = "args " + info.argsType
	}

	b.WriteString(fmt.Sprintf("func (c *Client) %s(%s) (%s, error) {\n", methodName, params, nativeReturn))

	callArgs := "nil"
	if info.argsType != "" {
		if usesVariadic {
			b.WriteString("\tvar a " + info.argsType + "\n")
			b.WriteString("\tif len(args) > 0 {\n\t\ta = args[0]\n\t}\n")
			b.WriteString("\targsMap, err := toArgsMap(a)\n")
		} else {
			b.WriteString("\targsMap, err := toArgsMap(args)\n")
		}
		b.WriteString("\tif err != nil {\n\t\treturn " + zeroExpr + ", err\n\t}\n")
		callArgs = "argsMap"
	}

	b.WriteString("\traw, err := c.raw.Call(" + goStringLiteral(ep.Name) + ", " + callArgs + ")\n")
	b.WriteString("\tif err != nil {\n\t\treturn " + zeroExpr + ", err\n\t}\n")

	switch {
	case info.pageWrapper != nil:
		b.WriteString("\tpage, ok := raw.(*webfunction.Page)\n")
		b.WriteString("\tif !ok {\n\t\treturn nil, fmt.Errorf(\"unexpected result type %T for paginated endpoint\", raw)\n\t}\n")
		b.WriteString("\treturn &" + info.pageWrapper.structName + "{raw: page}, nil\n")
	case nativeReturn == "any":
		b.WriteString("\treturn raw, nil\n")
	default:
		b.WriteString("\tvar result " + nativeReturn + "\n")
		b.WriteString("\tif err := decodeResult(raw, &result); err != nil {\n\t\treturn " + zeroExpr + ", err\n\t}\n")
		b.WriteString("\treturn result, nil\n")
	}

	b.WriteString("}\n\n")
}

// goZeroValue returns a Go zero-value expression for typ, used as the
// error-path return value alongside a non-nil error.
func goZeroValue(typ string) string {
	switch {
	case typ == "any":
		return "nil"
	case strings.HasPrefix(typ, "*"), strings.HasPrefix(typ, "[]"), strings.HasPrefix(typ, "map["):
		return "nil"
	case typ == "string":
		return `""`
	case typ == "bool":
		return "false"
	case typ == "float32", typ == "float64":
		return "0"
	case strings.HasPrefix(typ, "int"), strings.HasPrefix(typ, "uint"):
		return "0"
	default:
		// A named struct type - its own zero value.
		return typ + "{}"
	}
}

// writePageWrapper generates a small real Go struct per paginated
// endpoint, wrapping the real *webfunction.Page with item-typed access -
// necessary (not just nice-to-have) for the same reason phpgen needs
// one: the real Page.Items() returns []any, and Go has no docs-only
// overlay (unlike JSDoc) that could claim a stronger type without
// backing code.
func writePageWrapper(b *strings.Builder, w *pageWrapper) {
	b.WriteString("// " + w.structName + " is a page of results, wrapping the real *webfunction.Page\n")
	b.WriteString("// with item-level typing on Items() that the real type can't provide by\n")
	b.WriteString("// itself (its own Items() returns []any).\n")
	b.WriteString("type " + w.structName + " struct {\n\traw *webfunction.Page\n}\n\n")

	b.WriteString("// Items decodes this page's items into their typed shape.\n")
	b.WriteString("func (p *" + w.structName + ") Items() ([]" + w.itemType + ", error) {\n")
	b.WriteString("\tvar items []" + w.itemType + "\n")
	b.WriteString("\tif err := decodeResult(p.raw.Items(), &items); err != nil {\n\t\treturn nil, err\n\t}\n")
	b.WriteString("\treturn items, nil\n}\n\n")

	b.WriteString("func (p *" + w.structName + ") HasNext() bool     { return p.raw.HasNext() }\n")
	b.WriteString("func (p *" + w.structName + ") HasPrevious() bool { return p.raw.HasPrevious() }\n\n")

	b.WriteString("func (p *" + w.structName + ") NextPage() (*" + w.structName + ", error) {\n")
	b.WriteString("\tnext, err := p.raw.NextPage()\n")
	b.WriteString("\tif err != nil || next == nil {\n\t\treturn nil, err\n\t}\n")
	b.WriteString("\treturn &" + w.structName + "{raw: next}, nil\n}\n\n")

	b.WriteString("func (p *" + w.structName + ") PreviousPage() (*" + w.structName + ", error) {\n")
	b.WriteString("\tprev, err := p.raw.PreviousPage()\n")
	b.WriteString("\tif err != nil || prev == nil {\n\t\treturn nil, err\n\t}\n")
	b.WriteString("\treturn &" + w.structName + "{raw: prev}, nil\n}\n\n")
}

func goStringLiteral(s string) string {
	return fmt.Sprintf("%q", s)
}