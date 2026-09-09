// Package rubygen generates a real, typed Ruby client class wrapping the
// dynamic-dispatch reference gem (github.com/robinclart/web_function,
// "web_function" on RubyGems), plus a companion RBS signature file.
//
// v2 design (superseding the earlier pure-.rbs-only v1, after real
// editor testing showed Steep gives no autocomplete for `hash["key"]`-
// style bracket access, only error-checking on typos - confirmed
// against a real generated interface in Sublime+Steep):
//
//  1. Still backed by the real dynamic client - every generated method
//     is a thin wrapper that calls straight through to the real
//     WebFunction::Client's own method_missing-based dispatch
//     (composition via @real_client, not inheritance - Client is a
//     BasicObject at runtime, awkward to subclass meaningfully).
//  2. Structured RETURN values now get a REAL generated wrapper class
//     with real accessor methods (`def name; @raw["name"]; end`), not
//     an RBS-only `interface`/`[]`-overload. Real methods give real
//     dot-completion (`result.name`) via any Ruby-aware
//     editor/language-server, RBS or not - this is the actual point of
//     v2. A nested object/array-of-object field recursively wraps its
//     own raw value in the appropriate class too, so
//     `result.friend.name` works, not just one level deep.
//  3. Arguments are NOT wrapped or validated - confirmed genuinely
//     symbol-keyed caller-constructed hashes (e.g. filters: {
//     first_name: "Joe" }), passed straight through untouched. rubygen
//     only adds real, named keyword PARAMETERS (for arg-name
//     completion) with real types.
//  4. Real bug avoided proactively (same class of bug phpgen's
//     Client::call() null-vs-omitted session found for real): an
//     optional keyword parameter defaulting to nil and always being
//     forwarded would send an explicit JSON null for an argument the
//     caller genuinely omitted. Fixed via an Undefined sentinel -
//     optional args are only added to the forwarded hash when the
//     caller actually passed something other than the sentinel.
//  5. A digit-leading (or otherwise non-identifier) wire field name
//     can't be a real Ruby keyword PARAMETER at all (a genuine Ruby
//     language restriction, `def foo(2fa_enabled: nil)` is a
//     SyntaxError - stricter than the RBS-only v1's "keyword param
//     names must be identifiers" note, since here it's Ruby itself
//     enforcing it, not just RBS's grammar). Falls back to a trailing
//     `**kwargs` catch-all, merged into the forwarded args, same
//     principle as v1's `**untyped` RBS fallback.
//  6. Pagination: one shared, reusable TypedPage class (not a fresh
//     subclass per paginated endpoint) wraps the real WebFunction::Page,
//     mapping `page`'s items through the right wrapper class and
//     recursing through `next_page`/`previous_page` - confirmed
//     directly against the real, current page.rb source (re-cloned this
//     session: `page`, `next?`, `previous?`, `next_page`,
//     `previous_page`, Enumerable - unchanged from what v1 was built
//     against).
package rubygen

import (
	"fmt"
	"strings"

	"github.com/webfunction-protocol/webfunction-go"
)

// Generate builds a real Ruby wrapper-class source file plus a
// companion RBS signature file for pkg. moduleName namespaces the
// generated classes (mirroring every other target's --namespace flag,
// reintroduced now that v2 actually has a generated class to namespace
// - v1 had none). Returns (rubySource, rbsSource, error).
func Generate(pkg *webfunction.Package, sourceURL, moduleName string) (rubySource, rbsSource string, err error) {
	if moduleName == "" {
		moduleName = "WebFunctionClient"
	}
	classes := newClassSet(pkg)
	endpoints := visibleEndpoints(pkg)

	infos := make([]endpointGen, 0, len(endpoints))
	usedMethodNames := map[string]bool{}
	for _, ep := range endpoints {
		methodName := uniqueMethodName(usedMethodNames, rubyMethodName(ep.Name))
		infos = append(infos, buildEndpointGen(classes, ep, methodName))
	}

	needsPage := false
	for _, info := range infos {
		if info.paginated {
			needsPage = true
		}
	}

	var rb, rbs strings.Builder

	header := fmt.Sprintf("# Generated client for the %s webfunction package.\n# Source: %s\n#\n# Wraps the real web_function gem (github.com/robinclart/web_function) -\n# every method here calls straight through to the real dynamic client.\n", packageDisplayName(pkg), sourceURL)

	rb.WriteString(header)
	rb.WriteString("# frozen_string_literal: true\n\n")
	rb.WriteString("require \"web_function\"\n\n")
	rb.WriteString(fmt.Sprintf("module %s\n", moduleName))

	rbs.WriteString(header)
	rbs.WriteString(fmt.Sprintf("module %s\n", moduleName))

	if needsPage {
		writeTypedPageClass(&rb)
		writeTypedPageSig(&rbs)
	}

	for _, cls := range classes.ordered {
		writeRubyClass(&rb, cls)
		writeRbsClass(&rbs, cls)
	}

	writeClientClass(&rb, moduleName, infos)
	writeClientSig(&rbs, infos)

	rb.WriteString("end\n")
	rbs.WriteString("end\n")

	return rb.String(), rbs.String(), nil
}

func packageDisplayName(pkg *webfunction.Package) string {
	if pkg.Name != "" {
		return pkg.Name
	}
	return "(unnamed package)"
}

// ---- shared TypedPage wrapper ----

func writeTypedPageClass(b *strings.Builder) {
	b.WriteString(`  # Wraps a real WebFunction::Page, mapping each item on the page through
  # item_class (or leaving it untouched if item_class is nil, for a
  # bare scalar-item pagination). Recurses through next_page/previous_page
  # so pagination stays typed across every page, not just the first.
  #
  # The real gem only wraps a response in Page when it strictly matches
  # its own pagination-envelope shape (confirmed from the real
  # page.rb/request.rb source: a Hash with "page"/"next"/"previous" keys
  # present, "page" an Array, "next"/"previous" each nil or a Hash) -
  # otherwise Request#execute returns the raw decoded value completely
  # unwrapped. A real API can plausibly fail that check for an edge case
  # the real gem's author didn't anticipate (e.g. a zero-result response
  # that omits the "next"/"previous" keys entirely rather than nulling
  # them) - every method here checks respond_to? first and degrades to a
  # single, final page of whatever raw data is actually present, rather
  # than raising deep inside here when that happens.
  class TypedPage
    include Enumerable

    def initialize(real_page, item_class)
      @real_page = real_page
      @item_class = item_class
    end

    def page
      unless @real_page.respond_to?(:page)
        raw_items = @real_page.is_a?(Hash) ? (@real_page["page"] || []) : Array(@real_page)
        return @item_class ? raw_items.map { |item| @item_class.new(item) } : raw_items
      end

      @item_class ? @real_page.page.map { |item| @item_class.new(item) } : @real_page.page
    end

    def next?
      @real_page.respond_to?(:next?) ? @real_page.next? : false
    end

    def previous?
      @real_page.respond_to?(:previous?) ? @real_page.previous? : false
    end

    def next_page
      return nil unless @real_page.respond_to?(:next_page)

      next_real = @real_page.next_page
      next_real && TypedPage.new(next_real, @item_class)
    end

    def previous_page
      return nil unless @real_page.respond_to?(:previous_page)

      previous_real = @real_page.previous_page
      previous_real && TypedPage.new(previous_real, @item_class)
    end

    def each(&block)
      return enum_for(:each) unless block

      page.each(&block)
      self
    end
  end

`)
}

func writeTypedPageSig(b *strings.Builder) {
	b.WriteString("  class TypedPage[out Item]\n")
	b.WriteString("    include Enumerable[Item]\n\n")
	b.WriteString("    def initialize: (untyped real_page, Class? item_class) -> void\n")
	b.WriteString("    def page: () -> Array[Item]\n")
	b.WriteString("    def next?: () -> bool\n")
	b.WriteString("    def previous?: () -> bool\n")
	b.WriteString("    def next_page: () -> TypedPage[Item]?\n")
	b.WriteString("    def previous_page: () -> TypedPage[Item]?\n")
	b.WriteString("  end\n\n")
}

// ---- generated wrapper classes ----

func writeRubyClass(b *strings.Builder, cls *rubyClass) {
	b.WriteString("  class " + cls.name + "\n")
	b.WriteString("    def initialize(raw)\n      @raw = raw\n    end\n\n")
	for _, f := range cls.fields {
		writeDocComment(b, "    ", f.docs)
		b.WriteString("    def " + rubyAccessorName(f.name) + "\n")
		b.WriteString("      " + fieldAccessorBody(f) + "\n")
		b.WriteString("    end\n\n")
	}
	b.WriteString("  end\n\n")
}

// rubyAccessorName is the real Ruby method name for a field - same
// dash-to-underscore rule as endpoint names, since this is real Ruby
// method-definition syntax, not just a string key: a digit-leading wire
// name (e.g. "2fa_enabled") CANNOT be a real method name at all (Ruby
// syntax error), so it falls back to a sanitized, valid name instead.
func rubyAccessorName(wireName string) string {
	name := strings.ReplaceAll(wireName, "-", "_")
	if rubyIdentifier(name) && !dangerousObjectMethods[name] {
		return name
	}
	return "field_" + sanitizeForIdentifier(name)
}

func sanitizeForIdentifier(name string) string {
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}

func fieldAccessorBody(f classField) string {
	key := rubyStringLiteral(f.name)
	if f.wrapClass == "" {
		return "@raw[" + key + "]"
	}
	if f.isArray {
		return fmt.Sprintf("(@raw[%s] || []).map { |item| %s.new(item) }", key, f.wrapClass)
	}
	return fmt.Sprintf("(v = @raw[%s]) && %s.new(v)", key, f.wrapClass)
}

func writeRbsClass(b *strings.Builder, cls *rubyClass) {
	b.WriteString("  class " + cls.name + "\n")
	b.WriteString("    def initialize: (untyped raw) -> void\n\n")
	for _, f := range cls.fields {
		rt := f.rbsType
		if strings.Contains(rt, " | ") {
			rt = "(" + rt + ")"
		}
		b.WriteString("    def " + rubyAccessorName(f.name) + ": () -> " + rt + "\n")
	}
	b.WriteString("  end\n\n")
}

// ---- endpoint generation ----

type endpointGen struct {
	wireName   string
	methodName string
	docs       string
	params     []paramGen
	restParams bool // true if any argument couldn't be a real keyword param (falls back to **kwargs)
	paginated  bool
	itemClass  string // wrapper class name for paginated items, "" if unwrapped
	itemRbs    string
	returnRbs  string
	wrapClass  string // non-paginated: wrapper class name for the return value, "" if unwrapped
	errorCodes []webfunction.ErrorDef
}

type paramGen struct {
	name     string
	rbsType  string
	optional bool
}

func buildEndpointGen(classes *classSet, ep webfunction.Endpoint, methodName string) endpointGen {
	argFields := argumentFields(ep.Arguments)
	gen := endpointGen{wireName: ep.Name, methodName: methodName, docs: ep.Docs, errorCodes: ep.Errors}

	for _, f := range argFields {
		name := strings.ReplaceAll(f.name, "-", "_")
		if !rubyIdentifier(name) {
			gen.restParams = true
			continue
		}
		gen.params = append(gen.params, paramGen{
			name:     name,
			rbsType:  rbsArgType(f.jsonType, f.nullable, f.choices),
			optional: f.optional,
		})
	}

	if ep.HasFlag("paginated") {
		gen.paginated = true
		for _, a := range ep.Attributes {
			if a.Name != "page" {
				continue
			}
			for _, alt := range a.Type.Union {
				if alt.Base == "array" && alt.Of != nil {
					itemRbs, itemWrap := classes.arrayItemType(*alt.Of)
					gen.itemClass = itemWrap
					gen.itemRbs = itemRbs
				}
			}
		}
		if gen.itemRbs == "" {
			gen.itemRbs = "untyped"
		}
		return gen
	}

	switch {
	case ep.Returns.HasBase("object") && len(ep.Attributes) > 0:
		gen.wrapClass = classes.resolveLocal(ep.Name, attributeFields(ep.Attributes))
		gen.returnRbs = gen.wrapClass
	case ep.Returns.HasBareArray() && len(ep.Attributes) > 0:
		itemClass := classes.resolveLocal(ep.Name, attributeFields(ep.Attributes))
		gen.wrapClass = itemClass
		gen.returnRbs = "Array[" + itemClass + "]"
	default:
		rbsT, wrapClass := classes.arrayItemTypeOrScalar(ep.Returns)
		gen.wrapClass = wrapClass
		gen.returnRbs = rbsT
	}

	return gen
}

func writeClientClass(b *strings.Builder, moduleName string, infos []endpointGen) {
	b.WriteString("  # Sentinel distinguishing \"caller omitted this optional argument\" from\n")
	b.WriteString("  # \"caller explicitly passed nil\" - forwarding nil either way would send\n")
	b.WriteString("  # the real client (and the real server) an explicit JSON null for an\n")
	b.WriteString("  # argument that was never actually given.\n")
	b.WriteString("  Undefined = ::Object.new.freeze\n\n")

	b.WriteString("  class Client\n")
	b.WriteString("    def initialize(real_client)\n      @real_client = real_client\n    end\n\n")
	b.WriteString("    # @param url [String]\n")
	b.WriteString("    def self.from_package_endpoint(url, bearer_auth: nil, version: nil, pipelined: false)\n")
	b.WriteString("      new(::WebFunction::Client.from_package_endpoint(url, bearer_auth: bearer_auth, version: version, pipelined: pipelined))\n")
	b.WriteString("    end\n\n")

	for _, info := range infos {
		writeClientMethod(b, info)
	}

	b.WriteString("    # Escape hatch for calling an endpoint this file doesn't declare a\n")
	b.WriteString("    # typed method for.\n")
	b.WriteString("    def call(endpoint_name, args = {})\n      @real_client.call(endpoint_name, args)\n    end\n\n")
	b.WriteString("    def package\n      @real_client.package\n    end\n\n")
	b.WriteString("    def bearer_auth=(value)\n      @real_client.bearer_auth = value\n    end\n\n")
	b.WriteString("    def version=(value)\n      @real_client.version = value\n    end\n\n")
	b.WriteString("    def pipeline=(value)\n      @real_client.pipeline = value\n    end\n\n")
	b.WriteString("  end\n")
	_ = moduleName
}

func writeClientMethod(b *strings.Builder, info endpointGen) {
	writeDocComment(b, "    ", info.docs)
	for _, e := range info.errorCodes {
		code := strings.TrimSpace(e.Code)
		if code == "" {
			continue
		}
		line := "    # May raise WebFunction::BadRequestError with code " + code
		if dl := docLines(e.Docs); len(dl) > 0 {
			line += " - " + dl[0]
		}
		b.WriteString(line + "\n")
	}

	sig, buildArgs := renderParamSig(info)
	b.WriteString("    def " + info.methodName + sig + "\n")
	b.WriteString(buildArgs)

	callExpr := "@real_client." + info.methodName + callArgsExpr(info)

	switch {
	case info.paginated:
		itemClassRef := "nil"
		if info.itemClass != "" {
			itemClassRef = info.itemClass
		}
		b.WriteString("      TypedPage.new(" + callExpr + ", " + itemClassRef + ")\n")
	case info.wrapClass != "" && strings.HasPrefix(info.returnRbs, "Array["):
		b.WriteString("      (" + callExpr + " || []).map { |item| " + info.wrapClass + ".new(item) }\n")
	case info.wrapClass != "":
		b.WriteString("      (v = " + callExpr + ") && " + info.wrapClass + ".new(v)\n")
	default:
		b.WriteString("      " + callExpr + "\n")
	}
	b.WriteString("    end\n\n")
}

// renderParamSig builds the method's parameter list (real keyword
// params, Undefined-sentinel defaults for optional ones, a trailing
// **kwargs catch-all if needed) and the body lines that assemble the
// hash actually forwarded to the real client - only ever including keys
// the caller actually supplied.
func renderParamSig(info endpointGen) (sig string, bodyLines string) {
	if len(info.params) == 0 && !info.restParams {
		return "", ""
	}

	var sigParts []string
	var b strings.Builder
	b.WriteString("      args = {}\n")
	for _, p := range info.params {
		if p.optional {
			sigParts = append(sigParts, p.name+": Undefined")
			b.WriteString(fmt.Sprintf("      args[:%s] = %s unless %s.equal?(Undefined)\n", p.name, p.name, p.name))
		} else {
			sigParts = append(sigParts, p.name+":")
			b.WriteString(fmt.Sprintf("      args[:%s] = %s\n", p.name, p.name))
		}
	}
	if info.restParams {
		sigParts = append(sigParts, "**kwargs")
		b.WriteString("      args.merge!(kwargs)\n")
	}
	return "(" + strings.Join(sigParts, ", ") + ")", b.String()
}

func callArgsExpr(info endpointGen) string {
	if len(info.params) == 0 && !info.restParams {
		return ""
	}
	return "(**args)"
}

func visibleEndpoints(pkg *webfunction.Package) []webfunction.Endpoint {
	out := make([]webfunction.Endpoint, 0, len(pkg.Endpoints))
	for _, ep := range pkg.Endpoints {
		if ep.HasFlag("private") {
			continue
		}
		out = append(out, ep)
	}
	return out
}

// ---- companion .rbs for the Client class ----

func writeClientSig(b *strings.Builder, infos []endpointGen) {
	b.WriteString("  Undefined: untyped\n\n")
	b.WriteString("  class Client\n")
	b.WriteString("    def initialize: (untyped real_client) -> void\n")
	b.WriteString("    def self.from_package_endpoint: (String url, ?bearer_auth: String?, ?version: String?, ?pipelined: bool) -> Client\n\n")

	for _, info := range infos {
		var kwParts []string
		for _, p := range info.params {
			prefix := ""
			if p.optional {
				prefix = "?"
			}
			kwParts = append(kwParts, prefix+p.name+": "+p.rbsType)
		}
		if info.restParams {
			kwParts = append(kwParts, "**untyped")
		}
		sig := "()"
		if len(kwParts) > 0 {
			sig = "(" + strings.Join(kwParts, ", ") + ")"
		}

		returnType := info.returnRbs
		if info.paginated {
			returnType = "TypedPage[" + info.itemRbs + "]"
		}
		if strings.Contains(returnType, " | ") {
			returnType = "(" + returnType + ")"
		}
		b.WriteString(fmt.Sprintf("    def %s: %s -> %s\n", info.methodName, sig, returnType))
	}

	b.WriteString("\n    def call: (String endpoint_name, ?Hash[Symbol, untyped] args) -> untyped\n")
	b.WriteString("    def package: () -> untyped\n")
	b.WriteString("    def bearer_auth=: (String?) -> void\n")
	b.WriteString("    def version=: (String?) -> void\n")
	b.WriteString("    def pipeline=: (untyped) -> void\n")
	b.WriteString("  end\n")
}