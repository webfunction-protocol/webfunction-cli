// Package csharpgen generates a typed C# client for a webfunction
// package, targeting webfunction-csharp (the WebFunction namespace).
//
// Confirmed design decisions, and how they differ from
// jsgen/phpgen/gogen/javagen:
//
//  1. Real generated record types, not docs-only annotations - same
//     reasoning as gogen/javagen (C# has no separate docs-only static-
//     analysis layer the way tsc/PHPStan give JS/PHP; the compiler is
//     the only checker). Structurally closest to javagen of the existing
//     targets, for the same underlying reason - and for a second reason
//     specific to this target: webfunction-csharp itself already uses
//     dynamic dispatch (System.Dynamic.DynamicObject) as ITS OWN primary
//     API, so a codegen target that also emitted dynamic-dispatch-based
//     output would add nothing new - the entire point of codegen is to
//     supply the real, compiler-enforced types a dynamic call surface
//     can't give you (the same reason Ruby/JS/PHP, whose client
//     libraries already have dynamic dispatch, have no codegen target in
//     this CLI at all - jsgen/phpgen exist purely to layer real types
//     via JSDoc/PHPDoc on top of an already-dynamic call surface).
//     Generated methods therefore call the underlying Client's typed
//     escape hatch, CallAsync(name, args), never dynamic dispatch.
//  2. Numeric refinements map to real C# sized types, including real
//     unsigned ones: u32->uint, u64->ulong, i32->int, i64/timestamp->long,
//     f32->float, f64->double. Unlike javagen (Java has no unsigned
//     integer types at all, so u32/u64 both fall back to long, losing
//     precision above Long.MAX_VALUE for u64), C# has real uint/ulong -
//     the same precision gogen gets from Go's uint32/uint64.
//  3. Choices/values become a real generated C# enum, paired with a small
//     JsonConverter<T> mapping between enum members and their real wire
//     value (string/bool/long/double, chosen from the field's own
//     declared type - see types.go's choiceWireType) via
//     [JsonConverter(typeof(...))] on the enum type - the same underlying
//     idea as Jackson's @JsonValue/@JsonCreator on a Java enum, just
//     expressed via System.Text.Json's converter mechanism instead of
//     annotations. An explicit null among the choices doesn't get its own
//     enum member (System.Text.Json maps a JSON null straight through for
//     a nullable-annotated property without ever calling the converter) -
//     it instead marks the field nullable regardless of what the
//     endpoint's own required/nullable flag says. Same reasoning as
//     javagen's identical decision. When the type is an array, the
//     choices are its allowed items (per the spec): the field is a List
//     of the enum, the wire type follows the items (types.go's
//     itemWireType), and items of mixed kinds, or an array mixed with a
//     scalar, get no enum and keep the type they would have without
//     choices.
//  4. Optional Arguments and nullable Attributes both make a field
//     nullable via C#'s own "?" annotation (Nullable<T> for value types,
//     nullable-reference-type annotation for reference types) - ONE
//     uniform mechanism for both, genuinely simpler than javagen's need
//     for a separate boxed-class name (a Java primitive can't be null or
//     annotated at all).
//  5. A type union with more than one non-null alternative collapses to
//     object - same genuine expressive-power gap gogen/javagen hit (no
//     real union types in Go or Java either; C# has no stable
//     discriminated-union feature as of C# 12/.NET 8 without an external
//     source generator).
//  6. Pagination needs a real generated wrapper class per paginated
//     endpoint, same reasoning as gogen/javagen (not jsgen): the real
//     webfunction-csharp Page.Items returns IReadOnlyList<object?>, and
//     there's no docs-only overlay that could claim a stronger type
//     without backing code. NextPageAsync/PreviousPageAsync return
//     Task<Wrapper?>, mirroring the real webfunction-csharp Page's own
//     Task<Page?>-returning convention exactly.
//  7. Escape hatch on the generated Client mirrors webfunction-csharp's
//     OWN actual API shape (not another target's escape-hatch shape):
//     CallAsync(name, args), a Package property, and settable BearerAuth/
//     Version/Pipeline PROPERTIES (webfunction-csharp exposes these as
//     properties, not setter methods the way Go/Java/PHP's underlying
//     clients do - the generated passthroughs follow suit).
//  8. The shared --namespace flag maps onto C#'s own naturally
//     dot-separated namespace convention (like Java) - but unlike Java's
//     all-lower-case convention, each segment is PascalCased (idiomatic
//     C#, matching webfunction-csharp's own "WebFunction" namespace and
//     the flag's own PascalCase default, "WebFunctionClient"). The
//     generated class itself is always named "Client" (matching every
//     other target's convention).
//  9. A REAL naming collision, caught during scoping rather than by
//     compiling broken code: the generated class is named "Client", the
//     same name as webfunction-csharp's own WebFunction.Client (the
//     dynamic-dispatch class this generated code wraps). Resolved by
//     fully-qualifying EVERY reference to the library's types throughout
//     this package's output (global::WebFunction.Client,
//     global::WebFunction.Page, global::WebFunction.Pipeline,
//     global::WebFunction.Json) and never emitting a bare "using
//     WebFunction;" - this is literally the same fix already needed for
//     real in phpgen (a "use WebFunction\Client;" collision there),
//     applied proactively here instead of waiting to hit it a third time.
//  10. A zero-argument endpoint gets no args parameter at all, matching
//     gogen/javagen. An endpoint whose args are all individually optional
//     (or has none) gets a single method with a real C# default parameter
//     value ("FooArgs? args = null") - unlike Go (no default arguments,
//     needs the variadic-args trick) or Java (no default arguments
//     either, needs a real overload pair), C# has genuine default
//     parameter values, so this needs no workaround at all: the cleanest
//     of any target so far.
//  11. Args/Result conversion round-trips through webfunction-csharp's
//     OWN Json.Serialize/Json.ToClr helpers plus System.Text.Json's
//     JsonSerializer.Serialize/Deserialize<T> directly against the real
//     generated record types - the C# equivalent of gogen's marshal-then-
//     unmarshal-through-JSON-bytes round trip (Client.CallAsync works
//     with a loose Dictionary<string, object?>/List<object?> graph, not
//     JsonElement, so a typed record has to pass through that shape
//     either way). Two shared private static helpers on the generated
//     Client: ToArgsMap<T> and DecodeResult<T>.
//  12. Unlike javagen (needs an explicit @JsonCreator-annotated secondary
//     constructor on every record, due to a real Jackson bug found
//     against record types), System.Text.Json works directly against a
//     C# positional record's primary constructor with per-parameter
//     [property: JsonPropertyName(...)] attributes - no known equivalent
//     bug, no workaround needed. See records.go's recordDef doc comment.
//  13. System.Text.Json tolerates unknown JSON properties by default
//     (unlike Jackson, which defaults to failing on them) - so, unlike
//     javagen's explicit FAIL_ON_UNKNOWN_PROPERTIES=false configuration
//     (needed there because of the documented undocumented "hint" field
//     on some real Arguments), no equivalent configuration is needed
//     here at all. One less thing to get right.
//  14. Unlike javagen (javac requires a compilation unit's public
//     top-level type name to match its filename exactly, so Java output
//     is only valid saved as "Client.java"), C# has no such constraint -
//     the generated file can be saved under any name.
//  15. Known, inherited gap (not attempted to fix here, flagged same as
//     PHP's/Java's identical gap): a pipelined Client's CallAsync returns
//     a Promise instead of the real decoded value, and a generated typed
//     per-endpoint method has no special handling for that case - calling
//     a typed method on a pipelined Client will attempt to deserialize a
//     Promise into the endpoint's real return type and fail. Pipelining
//     is only usable today via the raw CallAsync escape hatch, same
//     limitation every other target currently carries.
package csharpgen

import (
	"fmt"
	"strings"

	"github.com/webfunction-protocol/webfunction-go"

	"wfn/generics"
	"wfn/privatefilter"
)

// LibraryReference documents, in the generated header comment, what the
// generated file depends on. webfunction-csharp is not yet published to
// NuGet as of this writing - callers need a project/assembly reference to
// it directly until it is.
const LibraryReference = "webfunction-csharp (the WebFunction namespace)"

// Generate builds a typed C# client for pkg, targeting webfunction-csharp.
// namespace is sanitized into a C# namespace (see names.go's
// csharpNamespace) - the shared --namespace flag maps onto C#'s own
// dot-separated namespace convention, PascalCased per segment.
//
// Generic objects are expanded first (generics.Expand): every application
// becomes an ordinary generated object such as ListOfUser, and the generic
// templates are not emitted.
//
// Anything flagged "private" - endpoints, arguments, and attributes - is
// left out of the generated code unless includePrivate is true.
func Generate(pkg *webfunction.Package, sourceURL, namespace string, includePrivate bool) (string, error) {
	pkg = generics.Expand(pkg)
	ns := csharpNamespace(namespace)

	if !includePrivate {
		pkg = privatefilter.Apply(pkg)
	}
	records := newRecordSet(pkg)
	endpoints := visibleEndpoints(pkg, includePrivate)

	// Pre-compute every endpoint's shapes first, so record/enum
	// resolution (which mutates records as it goes, same as gogen's
	// structSet/javagen's recordSet) is complete before anything is
	// rendered.
	infos := make(map[string]endpointInfo, len(endpoints))
	pageClassNames := map[string]bool{}
	for _, ep := range endpoints {
		infos[ep.Name] = buildEndpointInfo(records, ep, pageClassNames)
	}

	var b strings.Builder
	writeHeader(&b, pkg, sourceURL, ns)

	b.WriteString("public sealed class Client\n{\n")

	records.render(&b)

	for _, ep := range endpoints {
		if info := infos[ep.Name]; info.pageWrapper != nil {
			writePageWrapper(&b, info.pageWrapper)
		}
	}

	writeClientCore(&b, sourceURL)

	used := map[string]bool{}
	for _, ep := range endpoints {
		methodName := uniqueMethodName(used, pascalCase(ep.Name))
		writeMethod(&b, ep, methodName, infos[ep.Name])
	}

	b.WriteString("}\n")

	// Unlike gogen, there's no lightweight "does this even parse"
	// pre-check available without a full csc/Roslyn invocation - real
	// syntax validation happens in the verification step (compiling
	// generated output with dotnet build against a fixture project)
	// rather than inside Generate itself, same limitation javagen has.
	return b.String(), nil
}

// endpointInfo holds everything about one endpoint needed to write its
// method (and, for a paginated endpoint, its companion wrapper class).
type endpointInfo struct {
	argsType     string // record name for args, or "" if the endpoint takes none
	argsOptional bool
	returnType   string       // C# type for a non-paginated return; ignored if pageWrapper != nil
	pageWrapper  *pageWrapper // non-nil for a paginated endpoint
}

type pageWrapper struct {
	className string
	itemType  string
}

func buildEndpointInfo(records *recordSet, ep webfunction.Endpoint, pageClassNames map[string]bool) endpointInfo {
	argFields := argumentFields(ep.Arguments)
	argsDoc := ""
	if base := exportedTypeName(ep.Name); base != "" {
		argsDoc = base + "Args holds the arguments for the " + ep.Name + " endpoint."
	}
	argsType := records.forFields(exportedTypeName(ep.Name)+"Args", argsDoc, argFields, "argument")

	if ep.HasFlag("paginated") {
		itemType := "object"
		resolve := func(refName string) string { return records.resolveObject(refName, "attribute") }
		for _, a := range ep.Attributes {
			if a.Name != "page" {
				continue
			}
			for _, alt := range a.Type.Union {
				if alt.Base == "array" && alt.Of != nil {
					itemType = csharpType(*alt.Of, localTypes{}, false, resolve)
				}
			}
		}
		className := uniqueClassName(pageClassNames, exportedTypeName(ep.Name)+"Page")
		return endpointInfo{
			argsType: argsType, argsOptional: allOptional(argFields),
			pageWrapper: &pageWrapper{className: className, itemType: itemType},
		}
	}

	local := forEndpointReturn(records, ep)
	resolve := func(refName string) string { return records.resolveObject(refName, "attribute") }
	returnType := csharpType(ep.Returns, local, false, resolve)

	return endpointInfo{
		argsType: argsType, argsOptional: allOptional(argFields),
		returnType: returnType,
	}
}

// forEndpointReturn builds localTypes describing the endpoint's own
// inline return shape from its attributes - mirrors gogen's/javagen's
// identical helper (same spec-reading rationale: attributes describes the
// object itself for a bare "object" return; a bare, untyped "array"
// return with attributes describes each item's shape, honored even
// though the spec's letter only defines attributes for the "object" case,
// because real-world packages use it that way).
func forEndpointReturn(records *recordSet, ep webfunction.Endpoint) localTypes {
	if len(ep.Attributes) == 0 {
		return localTypes{}
	}
	fields := attributeFields(ep.Attributes)
	doc := exportedTypeName(ep.Name) + "Result is the result of the " + ep.Name + " endpoint."
	name := records.forFields(exportedTypeName(ep.Name)+"Result", doc, fields, "attribute")
	if name == "" {
		return localTypes{}
	}

	switch {
	case ep.Returns.HasBase("object"):
		return localTypes{object: name}
	case ep.Returns.HasBareArray():
		return localTypes{arrayOfItem: name}
	default:
		return localTypes{}
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

func uniqueClassName(used map[string]bool, base string) string {
	name := base
	for i := 2; used[name]; i++ {
		name = fmt.Sprintf("%s%d", base, i)
	}
	used[name] = true
	return name
}

func writeHeader(b *strings.Builder, pkg *webfunction.Package, sourceURL, ns string) {
	b.WriteString("// Code generated by wfn codegen --target csharp. DO NOT EDIT -\n")
	b.WriteString("// re-run codegen instead.\n")
	if pkg.Name != "" {
		b.WriteString("// Package: " + pkg.Name + "\n")
	}
	b.WriteString("// Source:  " + sourceURL + "\n")
	b.WriteString("//\n")
	b.WriteString("// IMPORTANT - depends on " + LibraryReference + ". webfunction-csharp is\n")
	b.WriteString("// not yet published to NuGet as of this writing - obtain it directly from\n")
	b.WriteString("// the project (a project/assembly reference) until it is.\n\n")
	b.WriteString("using System;\n")
	b.WriteString("using System.Collections.Generic;\n")
	b.WriteString("using System.Threading.Tasks;\n\n")
	b.WriteString("namespace " + ns + ";\n\n")

	name := pkg.Name
	if name == "" {
		name = "(unnamed package)"
	}
	writeSummary(b, "", docLines("Generated, typed client for the "+name+" webfunction package.\n\nSource: "+sourceURL))
}

// writeClientCore writes the Client class's fields, private constructor,
// static factory, escape-hatch passthroughs, and shared JSON conversion
// helpers - everything that isn't a per-endpoint method or a generated
// record/enum/page-wrapper type.
func writeClientCore(b *strings.Builder, sourceURL string) {
	b.WriteString("    private const string SourceUrl = " + csharpStringLiteral(sourceURL) + ";\n\n")

	b.WriteString("    private static readonly System.Text.Json.JsonSerializerOptions JsonOptions = new()\n")
	b.WriteString("    {\n")
	b.WriteString("        DefaultIgnoreCondition = System.Text.Json.Serialization.JsonIgnoreCondition.WhenWritingNull,\n")
	b.WriteString("    };\n\n")

	b.WriteString("    private readonly global::WebFunction.Client raw;\n\n")
	b.WriteString("    private Client(global::WebFunction.Client raw)\n    {\n        this.raw = raw;\n    }\n\n")

	writeSummary(b, "    ", []string{"Builds a Client, fetching the package definition from its source URL."})
	b.WriteString("    public static async Task<Client> CreateNew(string? bearerAuth = null, string? version = null)\n    {\n")
	b.WriteString("        var raw = await global::WebFunction.Client.FromPackageEndpointAsync(SourceUrl, bearerAuth: bearerAuth, version: version).ConfigureAwait(false);\n")
	b.WriteString("        return new Client(raw);\n")
	b.WriteString("    }\n\n")

	writeSummary(b, "    ", []string{"Escape hatch for calling an endpoint this client doesn't have a typed method for."})
	b.WriteString("    public Task<object?> CallAsync(string name, Dictionary<string, object?>? args = null) => raw.CallAsync(name, args);\n\n")

	writeSummary(b, "    ", []string{"The underlying package definition."})
	b.WriteString("    public global::WebFunction.Package Package => raw.Package;\n\n")

	// BearerAuth/Version/Pipeline are properties, not setter methods -
	// mirrors webfunction-csharp's own actual API shape (see package doc
	// comment, decision 7), not another target's escape-hatch shape.
	writeSummary(b, "    ", []string{"The bearer authentication token used for subsequent calls."})
	b.WriteString("    public string? BearerAuth\n    {\n        get => raw.BearerAuth;\n        set => raw.BearerAuth = value;\n    }\n\n")

	writeSummary(b, "    ", []string{"The API version sent with subsequent calls."})
	b.WriteString("    public string? Version\n    {\n        get => raw.Version;\n        set => raw.Version = value;\n    }\n\n")

	writeSummary(b, "    ", []string{"The pipeline subsequent calls are queued onto, or null for immediate (non-pipelined) calls."})
	b.WriteString("    public global::WebFunction.Pipeline? Pipeline\n    {\n        get => raw.Pipeline;\n        set => raw.Pipeline = value;\n    }\n\n")

	b.WriteString("    private static Dictionary<string, object?> ToArgsMap<T>(T? args)\n    {\n")
	b.WriteString("        if (args is null)\n        {\n            return new Dictionary<string, object?>();\n        }\n")
	b.WriteString("        var json = System.Text.Json.JsonSerializer.Serialize(args, JsonOptions);\n")
	b.WriteString("        using var doc = System.Text.Json.JsonDocument.Parse(json);\n")
	b.WriteString("        return global::WebFunction.Json.ToClr(doc.RootElement) as Dictionary<string, object?> ?? new Dictionary<string, object?>();\n")
	b.WriteString("    }\n\n")

	b.WriteString("    private static T DecodeResult<T>(object? rawResult)\n    {\n")
	b.WriteString("        var json = global::WebFunction.Json.Serialize(rawResult);\n")
	b.WriteString("        return System.Text.Json.JsonSerializer.Deserialize<T>(json, JsonOptions)!;\n")
	b.WriteString("    }\n\n")
}

// writeMethod writes one endpoint's method, including its XML doc
// comment.
//
// Error documentation is scoped to this endpoint's own declared Errors
// only - NOT merged with the package-level Errors list, matching every
// other target's identical scoping decision.
func writeMethod(b *strings.Builder, ep webfunction.Endpoint, methodName string, info endpointInfo) {
	var doc []string
	doc = append(doc, docLines(ep.Docs)...)

	var extra []string
	for _, e := range ep.Errors {
		code := strings.TrimSpace(e.Code)
		if code == "" {
			continue
		}
		line := "<exception cref=\"global::WebFunction.Exceptions.BadRequestException\">code " + csharpStringLiteral(code)
		errDocs := docLines(e.Docs)
		if len(errDocs) > 0 {
			line += " - " + joinDocLines(errDocs)
		}
		line += "</exception>"
		extra = append(extra, line)
	}
	// Call-agnostic - webfunction-csharp can return either of these for
	// any endpoint regardless of what error codes (if any) that endpoint
	// itself declares, so they're always noted, matching every other
	// target's identical treatment.
	extra = append(extra, "<exception cref=\"global::WebFunction.Exceptions.UnexpectedStatusCodeException\">if the server responds with a non-200/400 status</exception>")
	extra = append(extra, "<exception cref=\"global::WebFunction.Exceptions.JsonParseException\">if the response body isn't valid JSON</exception>")

	writeSummary(b, "    ", doc, extra...)

	nativeReturn := "Task<" + info.returnType + ">"
	if info.pageWrapper != nil {
		nativeReturn = "Task<" + info.pageWrapper.className + ">"
	} else if info.returnType == "object" {
		// The "any"/unrecognized-type fallback returns CallAsync's raw
		// result directly (see writeMethodBody) rather than going through
		// DecodeResult<T>'s null-forgiving "!" - that raw value really can
		// be null, so the method's own declared return type has to say so
		// too. Declaring a non-nullable "object" here while actually
		// returning a nullable value is exactly what triggered a real
		// CS8603 warning against reservepay's package.
		nativeReturn = "Task<object?>"
	}

	switch {
	case info.argsType == "":
		// No args at all.
		b.WriteString("    public async " + nativeReturn + " " + methodName + "()\n    {\n")
		writeMethodBody(b, ep, info, "new Dictionary<string, object?>()")
		b.WriteString("    }\n\n")
	case info.argsOptional:
		// Every field is individually optional (or there are none) - a
		// real C# default parameter value, unlike Go's variadic-args
		// trick or Java's overload pair (see package doc comment,
		// decision 10).
		b.WriteString("    public async " + nativeReturn + " " + methodName + "(" + info.argsType + "? args = null)\n    {\n")
		writeMethodBody(b, ep, info, "ToArgsMap(args)")
		b.WriteString("    }\n\n")
	default:
		b.WriteString("    public async " + nativeReturn + " " + methodName + "(" + info.argsType + " args)\n    {\n")
		writeMethodBody(b, ep, info, "ToArgsMap(args)")
		b.WriteString("    }\n\n")
	}
}

func writeMethodBody(b *strings.Builder, ep webfunction.Endpoint, info endpointInfo, argsMapExpr string) {
	b.WriteString("        var argsMap = " + argsMapExpr + ";\n")
	b.WriteString("        var result = await CallAsync(" + csharpStringLiteral(ep.Name) + ", argsMap).ConfigureAwait(false);\n")

	switch {
	case info.pageWrapper != nil:
		b.WriteString("        if (result is not global::WebFunction.Page page)\n        {\n")
		b.WriteString("            throw new InvalidOperationException($\"unexpected result type {result?.GetType()} for paginated endpoint\");\n")
		b.WriteString("        }\n")
		b.WriteString("        return new " + info.pageWrapper.className + "(page);\n")
	case info.returnType == "object":
		b.WriteString("        return result;\n")
	default:
		b.WriteString("        return DecodeResult<" + info.returnType + ">(result);\n")
	}
}

// writePageWrapper generates a small real C# class per paginated
// endpoint, wrapping the real global::WebFunction.Page with item-typed
// access - necessary (not just nice-to-have) for the same reason
// gogen/javagen need one: the real Page.Items returns
// IReadOnlyList<object?>, and there's no docs-only overlay that could
// claim a stronger type without backing code.
//
// NextPageAsync/PreviousPageAsync return Task<Wrapper?>, mirroring the
// real webfunction-csharp Page's own Task<Page?>-returning convention
// exactly - see package doc comment, decision 6.
func writePageWrapper(b *strings.Builder, w *pageWrapper) {
	writeSummary(b, "    ", []string{
		w.className + " is a page of results, wrapping the real global::WebFunction.Page",
		"with item-level typing on Items that the real type can't provide by itself",
		"(its own Items returns IReadOnlyList<object?>).",
	})
	b.WriteString("    public sealed class " + w.className + "\n    {\n")
	b.WriteString("        private readonly global::WebFunction.Page raw;\n\n")
	b.WriteString("        internal " + w.className + "(global::WebFunction.Page raw)\n        {\n            this.raw = raw;\n        }\n\n")

	writeSummary(b, "        ", []string{"This page's items, decoded into their typed shape."})
	b.WriteString("        public List<" + w.itemType + "> Items => DecodeResult<List<" + w.itemType + ">>(raw.Items);\n\n")

	b.WriteString("        public bool HasNext => raw.HasNext;\n")
	b.WriteString("        public bool HasPrevious => raw.HasPrevious;\n\n")

	b.WriteString("        public async Task<" + w.className + "?> NextPageAsync()\n        {\n")
	b.WriteString("            var next = await raw.NextPageAsync().ConfigureAwait(false);\n")
	b.WriteString("            return next is null ? null : new " + w.className + "(next);\n")
	b.WriteString("        }\n\n")

	b.WriteString("        public async Task<" + w.className + "?> PreviousPageAsync()\n        {\n")
	b.WriteString("            var previous = await raw.PreviousPageAsync().ConfigureAwait(false);\n")
	b.WriteString("            return previous is null ? null : new " + w.className + "(previous);\n")
	b.WriteString("        }\n")
	b.WriteString("    }\n\n")
}

func csharpStringLiteral(s string) string {
	return fmt.Sprintf("%q", s)
}