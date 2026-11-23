# Generated client for the Generics Clean API webfunction package.
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

  end

  class ListOfUserAttributes
    def initialize(raw)
      @raw = raw
    end

    def total
      @raw["total"]
    end

    def items
      (@raw["items"] || []).map { |item| UserAttributes.new(item) }
    end

  end

  class ListOfUserOrNullAttributes
    def initialize(raw)
      @raw = raw
    end

    def total
      @raw["total"]
    end

    def items
      (@raw["items"] || []).map { |item| UserAttributes.new(item) }
    end

  end

  class PageOfUserAttributes
    def initialize(raw)
      @raw = raw
    end

    def results
      (@raw["results"] || []).map { |item| UserAttributes.new(item) }
    end

  end

  class PageOfPageOfUserAttributes
    def initialize(raw)
      @raw = raw
    end

    def results
      (@raw["results"] || []).map { |item| PageOfUserAttributes.new(item) }
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

  class HolderAttributes
    def initialize(raw)
      @raw = raw
    end

    def inner
      (v = @raw["inner"]) && ListOfUserAttributes.new(v)
    end

  end

  class ListOfListOfUserAttributes
    def initialize(raw)
      @raw = raw
    end

    def total
      @raw["total"]
    end

    def items
      (@raw["items"] || []).map { |item| ListOfUserAttributes.new(item) }
    end

  end

  class NestedHolderAttributes
    def initialize(raw)
      @raw = raw
    end

    def deep
      (v = @raw["deep"]) && ListOfListOfUserAttributes.new(v)
    end

  end

  class CommentAttributes
    def initialize(raw)
      @raw = raw
    end

    def message
      @raw["message"]
    end

    def replies
      (@raw["replies"] || []).map { |item| CommentAttributes.new(item) }
    end

  end

  class ThreadAttributes
    def initialize(raw)
      @raw = raw
    end

    def comments
      (v = @raw["comments"]) && ListOfThreadAttributes.new(v)
    end

  end

  class ListOfThreadAttributes
    def initialize(raw)
      @raw = raw
    end

    def total
      @raw["total"]
    end

    def items
      (@raw["items"] || []).map { |item| ThreadAttributes.new(item) }
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

    def pairs
      (@raw["pairs"] || []).map { |item| PairOfStringAttributes.new(item) }
    end

    def items
      @raw["items"]
    end

  end

  class PageOfArrayOfUserAttributes
    def initialize(raw)
      @raw = raw
    end

    def results
      (@raw["results"] || []).map { |item| UserAttributes.new(item) }
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

    def list_optional_users
      (v = @real_client.list_optional_users) && ListOfUserOrNullAttributes.new(v)
    end

    def nested_pages
      (v = @real_client.nested_pages) && PageOfPageOfUserAttributes.new(v)
    end

    def pairs
      (@real_client.pairs || []).map { |item| PairOfNumberAttributes.new(item) }
    end

    def get_holders
      (@real_client.get_holders || []).map { |item| NestedHolderAttributes.new(item) }
    end

    def get_comments
      (@real_client.get_comments || []).map { |item| CommentAttributes.new(item) }
    end

    def get_threads
      (v = @real_client.get_threads) && ListOfThreadAttributes.new(v)
    end

    def wrapped
      (v = @real_client.wrapped) && WrapperOfStringAttributes.new(v)
    end

    def page_of_users
      (v = @real_client.page_of_users) && PageOfArrayOfUserAttributes.new(v)
    end

    def users
      (@real_client.users || []).map { |item| UserAttributes.new(item) }
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
