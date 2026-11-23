// Package javagen generates a typed Java client for a webfunction
// package, targeting webfunction-java (org.webfunction:webfunction).
//
// Design decisions, and how they differ from jsgen/phpgen/gogen:
//
//  1. Real generated record types, not docs-only annotations - same
//     reasoning as gogen (Java has no docs-only static-analysis layer
//     the way tsc/PHPStan give JS/PHP; the compiler is the only
//     checker). Structurally closest to gogen of the three existing
//     targets, for the same underlying reason.
//  2. Numeric refinements map to real Java primitive types where
//     possible: i32->int, i64/timestamp->long, f32->float, f64->double.
//     u32/u64 have no Java equivalent at all (Java has no unsigned
//     integer types, unlike Go) - both fall back to long, which covers
//     the full u32 range exactly but can't represent u64 values above
//     Long.MAX_VALUE precisely. A real, flagged imprecision versus
//     gogen's uint32/uint64 - not fixed with a BigInteger-based field,
//     judged more friction than the gap warrants for a v1.
//  3. Choices/values (an Argument's "choices" or Attribute's "values")
//     become a real generated Java enum type - unlike gogen, which could
//     only manage a doc comment for these (Go has no literal-union
//     mechanism). Java has a real enum construct, so this is strictly
//     stronger than every other target in this suite: invalid values
//     are a compile error at the call site, not just a runtime
//     surprise. Each enum stores its wire value (String/Long/Double/
//     Boolean, chosen from the field's own declared type - see
//     types.go's choiceWireType) and round-trips via @JsonValue/
//     @JsonCreator. An explicit null among the choices doesn't get its
//     own enum constant (Jackson maps JSON null straight to a null
//     field without invoking the creator) - it instead marks the field
//     nullable regardless of what the endpoint's own required/nullable
//     flag says. When the type is an array, the choices are its allowed
//     items (per the spec): the field is a List of the enum, the wire
//     type follows the items (types.go's itemWireType), and items of
//     mixed kinds, or an array mixed with a scalar, get no enum and keep
//     the type they would have without choices.
//  4. Optional Arguments and nullable Attributes both make a field
//     nullable, annotated with JSR-305's javax.annotation.Nullable
//     (chosen as the most common annotation source in the ecosystem,
//     at the cost of a new dependency beyond webfunction-java's
//     existing Jackson-only footprint - confirmed acceptable). Unlike
//     Go, which gets pointer-based nullability "for free" on every
//     type, a nullable primitive-mapped field (e.g. a nullable i32)
//     must use its boxed wrapper class (Integer) instead of the raw
//     primitive (int) - Java primitives can't be null or annotated. See
//     types.go's javaType.
//  5. A type union with more than one non-null alternative collapses to
//     Object - same genuine expressive-power gap gogen hit for Go (no
//     real union types). A sealed-interface-based union is a real,
//     stronger possible enhancement (Java 17, which this target already
//     requires, has sealed interfaces) - not attempted here, flagged as
//     a future option same as gogen's own choices-as-named-type
//     enhancement note.
//  6. Pagination needs a real generated wrapper class per paginated
//     endpoint, same reasoning as gogen/phpgen (not jsgen): the real
//     webfunction-java Page.getItems() returns List<Object>, and there's
//     no docs-only overlay that could claim a stronger type without
//     backing code. UNLIKE gogen's (*T, error) navigation methods,
//     nextPage()/previousPage() return Optional<Wrapper> directly,
//     mirroring the REAL webfunction-java Page's own
//     Optional<Page>-returning convention exactly - Java already has
//     the idiom Go's design had to work around, so there was no reason
//     to import Go's shape instead of the target language's own.
//  7. Escape hatch on the generated Client: call(name, args)/
//     getPackage()/setBearerAuth/setVersion/setPipeline - straight
//     passthroughs to the underlying org.webfunction.Client, matching
//     every other target's escape hatch (jsgen's .call/.package,
//     phpgen's call()/getPackage(), gogen's Call/Package).
//  8. The shared --namespace flag maps onto Java's own naturally
//     dot-separated package-name convention (see names.go's
//     javaPackageName) - the closest fit of any target so far, since
//     Java package names ARE dot-separated namespaces already. The
//     generated class itself is always named "Client" (matching every
//     other target's naming), inside that package.
//  9. A REAL Java-specific constraint with no equivalent in JS/PHP/Go:
//     javac requires a single compilation unit's public top-level type
//     name to match its filename exactly. The generated output is
//     therefore only valid if written to a file literally named
//     "Client.java" - called out explicitly in the generated header
//     comment, since getting this wrong is a real compile error, not a
//     style nit.
//  10. A zero-argument endpoint gets no args parameter at all, matching
//     gogen. An endpoint whose args are all individually optional (or
//     has none) gets TWO overloaded methods instead - a real Java
//     method overload standing in for what Go's variadic-args trick and
//     JS/PHP's default-argument syntax each solve differently. This is
//     a cleaner fit for Java than porting either precedent mechanically:
//     Java has real overloading, so `client.listUsers()` and
//     `client.listUsers(ListUsersArgs)` can coexist as two real,
//     independently-typed methods rather than one loosely-typed variadic
//     signature.
//  11. Result/args conversion uses Jackson's ObjectMapper.convertValue
//     directly against the already-decoded Object graph
//     org.webfunction.Client.call returns (via an inline
//     TypeReference<T>, so even generic destinations like List<Foo>
//     convert correctly) - genuinely simpler than gogen's
//     marshal-then-unmarshal-through-JSON-bytes round trip, since
//     Jackson's conversion machinery works directly on JVM object graphs
//     without needing an intermediate byte serialization step the way
//     Go's encoding/json API requires.
//  12. The generated Client's own ObjectMapper disables
//     FAIL_ON_UNKNOWN_PROPERTIES, mirroring webfunction-java's own
//     Json.MAPPER exactly and for the identical reason: the raw decoded
//     value handed back by org.webfunction.Client.call may still carry
//     fields our own generated record types don't know about (e.g. the
//     documented undocumented "hint" field on some real Arguments) -
//     the underlying library already tolerates this on ITS OWN parse,
//     but our second, separate conversion pass needs the same
//     tolerance or it would fail where the library itself didn't.
//  13. Known, inherited gap (not attempted to fix here, flagged same as
//     PHP's identical gap): a pipelined Client returns a Promise from
//     the raw call() instead of the real decoded value, and a generated
//     typed per-endpoint method has no special handling for that case -
//     calling a typed method on a pipelined Client will attempt to
//     convertValue a Promise into the endpoint's real return type and
//     fail. Pipelining is only usable today via the raw call() escape
//     hatch, same limitation phpgen/gogen already carry.
package javagen

import (
	"fmt"
	"strings"

	"github.com/webfunction-protocol/webfunction-go"

	"wfn/generics"
	"wfn/privatefilter"
)

// ClientCoordinates documents the Maven coordinates the generated file's
// header comment points to. webfunction-java is not yet published to a
// public Maven repository as of this writing (see the project's own
// hand-off notes) - callers need to obtain the jar/coordinates directly
// from the project until it is.
const ClientCoordinates = "org.webfunction:webfunction"

// Generate builds a typed Java client for pkg, targeting
// webfunction-java. namespace is sanitized into a Java package name
// (see names.go's javaPackageName) - the shared --namespace flag maps
// onto Java's own dot-separated package convention more directly than
// any other target so far.
//
// Generic objects are expanded first (generics.Expand): every application
// becomes an ordinary generated object such as ListOfUser, and the generic
// templates are not emitted.
//
// Anything flagged "private" - endpoints, arguments, and attributes - is
// left out of the generated code unless includePrivate is true.
func Generate(pkg *webfunction.Package, sourceURL, namespace string, includePrivate bool) (string, error) {
	pkg = generics.Expand(pkg)
	packageName := javaPackageName(namespace)

	if !includePrivate {
		pkg = privatefilter.Apply(pkg)
	}
	records := newRecordSet(pkg)
	endpoints := visibleEndpoints(pkg, includePrivate)

	// Pre-compute every endpoint's shapes first, so record/enum
	// resolution (which mutates records as it goes, same as gogen's
	// structSet) is complete before anything is rendered.
	infos := make(map[string]endpointInfo, len(endpoints))
	pageClassNames := map[string]bool{}
	for _, ep := range endpoints {
		infos[ep.Name] = buildEndpointInfo(records, ep, pageClassNames)
	}

	var b strings.Builder
	writeHeader(&b, pkg, sourceURL, packageName)

	b.WriteString("public final class Client {\n\n")

	records.render(&b)

	for _, ep := range endpoints {
		if info := infos[ep.Name]; info.pageWrapper != nil {
			writePageWrapper(&b, info.pageWrapper)
		}
	}

	writeClientCore(&b, pkg, sourceURL)

	used := map[string]bool{}
	for _, ep := range endpoints {
		methodName := uniqueMethodName(used, camelCase(ep.Name))
		writeMethod(&b, ep, methodName, infos[ep.Name])
	}

	b.WriteString("}\n")

	// Unlike gogen, there's no lightweight "does this even parse"
	// pre-check available without a full javac invocation (Go's
	// go/format.Source doubles as both a formatter and a syntax
	// validator; Java has no equivalent in the standard toolchain
	// usable from Go). Real syntax validation happens in the
	// verification step (compiling generated output with javac against
	// a fixture) rather than inside Generate itself.
	return b.String(), nil
}

// endpointInfo holds everything about one endpoint needed to write its
// method (and, for a paginated endpoint, its companion wrapper class).
type endpointInfo struct {
	argsType     string // record name for args, or "" if the endpoint takes none
	argsOptional bool
	returnType   string       // Java type for a non-paginated return; ignored if pageWrapper != nil
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
		itemType := "Object"
		resolve := func(refName string) string { return records.resolveObject(refName, "attribute") }
		for _, a := range ep.Attributes {
			if a.Name != "page" {
				continue
			}
			for _, alt := range a.Type.Union {
				if alt.Base == "array" && alt.Of != nil {
					// forceNullable: true - same reason as types.go's
					// array-item handling: a page's item type sits
					// inside List<T> in the generated wrapper's
					// getItems(), and Java generics can never hold a
					// primitive regardless of nullability.
					itemType = javaType(*alt.Of, localTypes{}, true, resolve)
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
	returnType := javaType(ep.Returns, local, false, resolve)

	return endpointInfo{
		argsType: argsType, argsOptional: allOptional(argFields),
		returnType: returnType,
	}
}

// forEndpointReturn builds localTypes describing the endpoint's own
// inline return shape from its attributes - mirrors gogen's
// forEndpointReturn exactly (same spec-reading rationale: attributes
// describes the object itself for a bare "object" return; a bare,
// untyped "array" return with attributes describes each item's shape,
// honored even though the spec's letter only defines attributes for the
// "object" case, because real-world packages use it that way).
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

func writeHeader(b *strings.Builder, pkg *webfunction.Package, sourceURL, packageName string) {
	b.WriteString("// Code generated by wfn codegen --target java. DO NOT EDIT -\n")
	b.WriteString("// re-run codegen instead.\n")
	if pkg.Name != "" {
		b.WriteString("// Package: " + pkg.Name + "\n")
	}
	b.WriteString("// Source:  " + sourceURL + "\n")
	b.WriteString("//\n")
	b.WriteString("// IMPORTANT - depends on " + ClientCoordinates + " (webfunction-java) and\n")
	b.WriteString("// com.google.code.findbugs:jsr305 (for @Nullable). webfunction-java is not\n")
	b.WriteString("// yet published to a public Maven repository as of this writing - obtain\n")
	b.WriteString("// it directly from the project until it is.\n")
	b.WriteString("//\n")
	b.WriteString("// IMPORTANT - this file MUST be saved as exactly \"Client.java\": javac\n")
	b.WriteString("// requires a compilation unit's public top-level type to match its\n")
	b.WriteString("// filename. It also belongs in a directory matching its package, which\n")
	b.WriteString("// build tools and IDEs expect:\n")
	b.WriteString("//   <source root>/" + strings.ReplaceAll(packageName, ".", "/") + "/Client.java\n")
	b.WriteString("// (<source root> is src/main/java in a Maven or Gradle project.)\n\n")
	b.WriteString("package " + packageName + ";\n\n")
	b.WriteString("import com.fasterxml.jackson.core.type.TypeReference;\n")
	b.WriteString("import com.fasterxml.jackson.databind.DeserializationFeature;\n")
	b.WriteString("import com.fasterxml.jackson.databind.ObjectMapper;\n")
	b.WriteString("import javax.annotation.Nullable;\n")
	b.WriteString("import java.util.List;\n")
	b.WriteString("import java.util.Map;\n")
	b.WriteString("import java.util.Optional;\n\n")

	name := pkg.Name
	if name == "" {
		name = "(unnamed package)"
	}
	writeJavadoc(b, "", []string{
		"Generated, typed client for the " + name + " webfunction package.",
		"",
		"Source: " + sourceURL,
	})
}

// writeClientCore writes the Client class's fields, private constructor,
// static factory, escape-hatch passthroughs, and shared Jackson
// conversion helpers - everything that isn't a per-endpoint method or a
// generated record/enum/page-wrapper type.
func writeClientCore(b *strings.Builder, pkg *webfunction.Package, sourceURL string) {
	b.WriteString("    private static final String SOURCE_URL = " + javaStringLiteral(sourceURL) + ";\n\n")

	// See package doc comment, decision 12: mirrors webfunction-java's
	// own Json.MAPPER configuration exactly, and for the same reason -
	// our conversion pass needs the same unknown-property tolerance the
	// library's own internal parse already has.
	b.WriteString("    private static final ObjectMapper MAPPER = new ObjectMapper()\n")
	b.WriteString("            .configure(DeserializationFeature.FAIL_ON_UNKNOWN_PROPERTIES, false);\n\n")

	b.WriteString("    private final org.webfunction.Client raw;\n\n")
	b.WriteString("    private Client(org.webfunction.Client raw) {\n        this.raw = raw;\n    }\n\n")

	writeJavadoc(b, "    ", []string{"Builds a Client, fetching the package definition from its source URL."})
	b.WriteString("    public static Client createNew(org.webfunction.Options opts) {\n")
	b.WriteString("        return new Client(org.webfunction.Client.fromPackageEndpoint(SOURCE_URL, opts));\n")
	b.WriteString("    }\n\n")

	writeJavadoc(b, "    ", []string{"Escape hatch for calling an endpoint this client doesn't have a typed method for."})
	b.WriteString("    public Object call(String name, Map<String, Object> args) {\n        return raw.call(name, args);\n    }\n\n")

	writeJavadoc(b, "    ", []string{"Returns the underlying package definition."})
	b.WriteString("    public org.webfunction.Package getPackage() {\n        return raw.getPackage();\n    }\n\n")

	writeJavadoc(b, "    ", []string{"Updates the bearer token used on subsequent calls."})
	b.WriteString("    public void setBearerAuth(String bearerAuth) {\n        raw.setBearerAuth(bearerAuth);\n    }\n\n")

	writeJavadoc(b, "    ", []string{"Updates the API version used on subsequent calls."})
	b.WriteString("    public void setVersion(String version) {\n        raw.setVersion(version);\n    }\n\n")

	writeJavadoc(b, "    ", []string{"Replaces the client's pipeline. Pass null to make calls execute immediately again instead of batching."})
	b.WriteString("    public void setPipeline(org.webfunction.Pipeline pipeline) {\n        raw.setPipeline(pipeline);\n    }\n\n")

	b.WriteString("    private static Map<String, Object> toArgsMap(Object args) {\n")
	b.WriteString("        return MAPPER.convertValue(args, new TypeReference<Map<String, Object>>() {});\n")
	b.WriteString("    }\n\n")

	b.WriteString("    private static <T> T decodeResult(Object raw, TypeReference<T> type) {\n")
	b.WriteString("        return MAPPER.convertValue(raw, type);\n")
	b.WriteString("    }\n\n")
}

// writeMethod writes one endpoint's method(s), including Javadoc.
//
// Error documentation is scoped to this endpoint's own declared Errors
// only - NOT merged with the package-level Errors list, matching every
// other target's identical scoping decision (see gogen's writeMethod for
// the fuller reasoning).
func writeMethod(b *strings.Builder, ep webfunction.Endpoint, methodName string, info endpointInfo) {
	var doc []string
	doc = append(doc, docLines(ep.Docs)...)

	for _, e := range ep.Errors {
		code := strings.TrimSpace(e.Code)
		if code == "" {
			continue
		}
		line := "@throws org.webfunction.BadRequestException with code " + javaStringLiteral(code)
		errDocs := docLines(e.Docs)
		if len(errDocs) > 0 {
			line += " - " + errDocs[0]
		}
		if len(doc) > 0 {
			doc = append(doc, "")
		}
		doc = append(doc, line)
		if len(errDocs) > 1 {
			doc = append(doc, errDocs[1:]...)
		}
	}
	// Call-agnostic - webfunction-java can return either of these for
	// any endpoint regardless of what error codes (if any) that endpoint
	// itself declares, so they're always noted, matching every other
	// target's identical treatment.
	doc = append(doc, "@throws org.webfunction.UnexpectedStatusCodeException if the server responds with a non-200/400 status")
	doc = append(doc, "@throws org.webfunction.JsonParseException if the response body isn't valid JSON")

	writeJavadoc(b, "    ", doc)

	nativeReturn := info.returnType
	if info.pageWrapper != nil {
		nativeReturn = info.pageWrapper.className
	}

	switch {
	case info.argsType == "":
		// No args at all - a single method, no overload needed.
		b.WriteString("    public " + nativeReturn + " " + methodName + "() {\n")
		writeMethodBody(b, ep, info, nativeReturn, "Map.of()")
		b.WriteString("    }\n\n")
	case info.argsOptional:
		// Every field is individually optional (or there are none) -
		// a real Java method overload stands in for what Go's
		// variadic-args trick and JS/PHP's default-argument syntax each
		// solve differently (see package doc comment, decision 10).
		b.WriteString("    public " + nativeReturn + " " + methodName + "() {\n")
		b.WriteString("        return " + methodName + "(null);\n")
		b.WriteString("    }\n\n")
		writeJavadoc(b, "    ", []string{"@param args optional - pass null or an instance of " + info.argsType + "."})
		b.WriteString("    public " + nativeReturn + " " + methodName + "(@Nullable " + info.argsType + " args) {\n")
		writeMethodBody(b, ep, info, nativeReturn, "args == null ? Map.of() : toArgsMap(args)")
		b.WriteString("    }\n\n")
	default:
		b.WriteString("    public " + nativeReturn + " " + methodName + "(" + info.argsType + " args) {\n")
		writeMethodBody(b, ep, info, nativeReturn, "toArgsMap(args)")
		b.WriteString("    }\n\n")
	}
}

func writeMethodBody(b *strings.Builder, ep webfunction.Endpoint, info endpointInfo, nativeReturn, argsMapExpr string) {
	b.WriteString("        Map<String, Object> argsMap = " + argsMapExpr + ";\n")
	b.WriteString("        Object result = raw.call(" + javaStringLiteral(ep.Name) + ", argsMap);\n")

	switch {
	case info.pageWrapper != nil:
		b.WriteString("        if (!(result instanceof org.webfunction.Page page)) {\n")
		b.WriteString("            throw new IllegalStateException(\"unexpected result type \" + (result == null ? \"null\" : result.getClass()) + \" for paginated endpoint\");\n")
		b.WriteString("        }\n")
		b.WriteString("        return new " + info.pageWrapper.className + "(page);\n")
	case nativeReturn == "Object":
		b.WriteString("        return result;\n")
	default:
		// nativeReturn is boxed for the TypeReference<T> generic
		// argument specifically (a primitive is never legal there),
		// while the method's own declared return type stays whatever
		// javaType produced (a primitive where possible) - Java
		// autoboxes/unboxes the return statement itself automatically,
		// so this doesn't affect the method's real signature.
		b.WriteString("        return decodeResult(result, new TypeReference<" + javaBoxedForGeneric(nativeReturn) + ">() {});\n")
	}
}

// writePageWrapper generates a small real Java class per paginated
// endpoint, wrapping the real org.webfunction.Page with item-typed
// access - necessary (not just nice-to-have) for the same reason
// gogen/phpgen need one: the real Page.getItems() returns List<Object>,
// and there's no docs-only overlay that could claim a stronger type
// without backing code.
//
// nextPage()/previousPage() return Optional<Wrapper>, mirroring the REAL
// webfunction-java Page's own Optional<Page>-returning convention
// exactly - see package doc comment, decision 6.
func writePageWrapper(b *strings.Builder, w *pageWrapper) {
	writeJavadoc(b, "    ", []string{
		w.className + " is a page of results, wrapping the real org.webfunction.Page",
		"with item-level typing on getItems() that the real type can't provide by",
		"itself (its own getItems() returns List<Object>).",
	})
	b.WriteString("    public static final class " + w.className + " {\n")
	b.WriteString("        private final org.webfunction.Page raw;\n\n")
	b.WriteString("        private " + w.className + "(org.webfunction.Page raw) {\n            this.raw = raw;\n        }\n\n")

	writeJavadoc(b, "        ", []string{"Decodes this page's items into their typed shape."})
	b.WriteString("        public List<" + w.itemType + "> getItems() {\n")
	b.WriteString("            return decodeResult(raw.getItems(), new TypeReference<List<" + w.itemType + ">>() {});\n")
	b.WriteString("        }\n\n")

	b.WriteString("        public boolean hasNext() { return raw.hasNext(); }\n")
	b.WriteString("        public boolean hasPrevious() { return raw.hasPrevious(); }\n\n")

	b.WriteString("        public Optional<" + w.className + "> nextPage() {\n")
	b.WriteString("            return raw.nextPage().map(" + w.className + "::new);\n")
	b.WriteString("        }\n\n")

	b.WriteString("        public Optional<" + w.className + "> previousPage() {\n")
	b.WriteString("            return raw.previousPage().map(" + w.className + "::new);\n")
	b.WriteString("        }\n")
	b.WriteString("    }\n\n")
}

func javaStringLiteral(s string) string {
	return fmt.Sprintf("%q", s)
}
