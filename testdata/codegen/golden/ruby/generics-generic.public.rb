# Generated client for the Generics Codegen Test webfunction package.
# Source: https://example.test/package
#
# Wraps the real web_function gem (github.com/robinclart/web_function) -
# every method here calls straight through to the real dynamic client.
# frozen_string_literal: true

require "web_function"

module WebFunctionClient
  class UserAttributes
    def initialize(raw)
      @raw = raw
    end

    def id
      @raw["id"]
    end

    def name
      @raw["name"]
    end

  end

  class ListOfUserAttributes
    def initialize(raw)
      @raw = raw
    end

    def items
      (@raw["items"] || []).map { |item| UserAttributes.new(item) }
    end

    def total
      @raw["total"]
    end

  end

  class ListOfUserOrNullAttributes
    def initialize(raw)
      @raw = raw
    end

    def items
      (@raw["items"] || []).map { |item| UserAttributes.new(item) }
    end

    def total
      @raw["total"]
    end

  end

  class PageOfUserAttributes
    def initialize(raw)
      @raw = raw
    end

    def results
      (@raw["results"] || []).map { |item| UserAttributes.new(item) }
    end

    def has_more
      @raw["has_more"]
    end

  end

  class PageOfPageOfUserAttributes
    def initialize(raw)
      @raw = raw
    end

    def results
      (@raw["results"] || []).map { |item| PageOfUserAttributes.new(item) }
    end

    def has_more
      @raw["has_more"]
    end

  end

  class PageOfArrayOfUserAttributes
    def initialize(raw)
      @raw = raw
    end

    def results
      (@raw["results"] || []).map { |item| UserAttributes.new(item) }
    end

    def has_more
      @raw["has_more"]
    end

  end

  class PairOfNumberAttributes
    def initialize(raw)
      @raw = raw
    end

    def first
      @raw["first"]
    end

    def second
      @raw["second"]
    end

  end

  class BothOfNumberAttributes
    def initialize(raw)
      @raw = raw
    end

    def output
      @raw["output"]
    end

  end

  class HolderAttributes
    def initialize(raw)
      @raw = raw
    end

    def users
      (v = @raw["users"]) && ListOfUserAttributes.new(v)
    end

    def tags
      @raw["tags"]
    end

  end

  class PairOfStringAttributes
    def initialize(raw)
      @raw = raw
    end

    def first
      @raw["first"]
    end

    def second
      @raw["second"]
    end

  end

  class WrapperOfStringAttributes
    def initialize(raw)
      @raw = raw
    end

    def items
      @raw["items"]
    end

    def pair
      (v = @raw["pair"]) && PairOfStringAttributes.new(v)
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

    def list_users(filter: Undefined)
      args = {}
      args[:filter] = filter unless filter.equal?(Undefined)
      (v = @real_client.list_users(**args)) && ListOfUserAttributes.new(v)
    end

    def list_users_or_null
      (v = @real_client.list_users_or_null) && ListOfUserOrNullAttributes.new(v)
    end

    def page_of_pages
      (v = @real_client.page_of_pages) && PageOfPageOfUserAttributes.new(v)
    end

    def page_of_arrays
      (v = @real_client.page_of_arrays) && PageOfArrayOfUserAttributes.new(v)
    end

    def pairs
      (@real_client.pairs || []).map { |item| PairOfNumberAttributes.new(item) }
    end

    def convert(payload:)
      args = {}
      args[:payload] = payload
      (v = @real_client.convert(**args)) && BothOfNumberAttributes.new(v)
    end

    def get_holder
      (v = @real_client.get_holder) && HolderAttributes.new(v)
    end

    def get_wrapper
      (v = @real_client.get_wrapper) && WrapperOfStringAttributes.new(v)
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
