# Generated client for the Flawed Example API webfunction package.
# Source: https://example.test/package
#
# Wraps the real web_function gem (github.com/robinclart/web_function) -
# every method here calls straight through to the real dynamic client.
# frozen_string_literal: true

require "web_function"

module WebFunctionClient
  # Wraps a real WebFunction::Page, mapping each item on the page through
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

  # Sentinel distinguishing "caller omitted this optional argument" from
  # "caller explicitly passed nil" - forwarding nil either way would send
  # the real client (and the real server) an explicit JSON null for an
  # argument that was never actually given.
  Undefined = ::Object.new.freeze

  class Client
    # The package endpoint this client was generated from.
    SOURCE_URL = "https://example.test/package"

    # Pass a real ::WebFunction::Client to wrap it (handy in tests), or pass nothing
    # to connect to SOURCE_URL, the endpoint this file was generated from.
    def initialize(real_client = nil, bearer_auth: nil, version: nil, pipelined: false)
      if real_client
        if bearer_auth || version || pipelined
          raise ::ArgumentError, "bearer_auth, version and pipelined apply only when no real_client is given"
        end

        @real_client = real_client
      else
        @real_client = ::WebFunction::Client.from_package_endpoint(SOURCE_URL, bearer_auth: bearer_auth, version: version, pipelined: pipelined)
      end
    end

    # @param url [String]
    def self.from_package_endpoint(url, bearer_auth: nil, version: nil, pipelined: false)
      new(::WebFunction::Client.from_package_endpoint(url, bearer_auth: bearer_auth, version: version, pipelined: pipelined))
    end

    def list_people(status: Undefined)
      args = {}
      args[:status] = status unless status.equal?(Undefined)
      TypedPage.new(@real_client.list_people(**args), nil)
    end

    def list_people2
      @real_client.list_people2
    end

    def get_blob
      @real_client.get_blob
    end

    def get_anything
      @real_client.get_anything
    end

    # Escape hatch for calling an endpoint this file doesn't declare a
    # typed method for.
    def call(endpoint_name, args = {})
      @real_client.call(endpoint_name, args)
    end

    def package
      @real_client.package
    end

    def bearer_auth=(value)
      @real_client.bearer_auth = value
    end

    def version=(value)
      @real_client.version = value
    end

    def pipeline=(value)
      @real_client.pipeline = value
    end

  end
end
