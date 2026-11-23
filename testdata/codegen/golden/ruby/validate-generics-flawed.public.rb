# Generated client for the Generics Flawed API webfunction package.
# Source: https://example.test/package
#
# Wraps the real web_function gem (github.com/robinclart/web_function) -
# every method here calls straight through to the real dynamic client.
# frozen_string_literal: true

require "web_function"

module WebFunctionClient
  class ListOfAnyAttributes
    def initialize(raw)
      @raw = raw
    end

    def total
      @raw["total"]
    end

    def items
      @raw["items"]
    end

  end

  class UserAttributes
    def initialize(raw)
      @raw = raw
    end

    def id
      @raw["id"]
    end

  end

  class ListOfGhostAttributes
    def initialize(raw)
      @raw = raw
    end

    def total
      @raw["total"]
    end

    def items
      @raw["items"]
    end

  end

  class ListOfUser2Attributes
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

    def unapplied
      (v = @real_client.unapplied) && ListOfAnyAttributes.new(v)
    end

    def static_arg
      (v = @real_client.static_arg) && UserAttributes.new(v)
    end

    def empty_app
      @real_client.empty_app
    end

    def bad_key
      @real_client.bad_key
    end

    def bad_arg
      @real_client.bad_arg
    end

    def ghost_arg
      (v = @real_client.ghost_arg) && ListOfGhostAttributes.new(v)
    end

    def wrong_context(m: Undefined)
      args = {}
      args[:m] = m unless m.equal?(Undefined)
      (v = @real_client.wrong_context(**args)) && UserAttributes.new(v)
    end

    def collides
      (v = @real_client.collides) && ListOfUser2Attributes.new(v)
    end

    def t_return
      @real_client.t_return
    end

    def t_argument(v: Undefined)
      args = {}
      args[:v] = v unless v.equal?(Undefined)
      @real_client.t_argument(**args)
    end

    def t_applied
      (v = @real_client.t_applied) && ListOfAnyAttributes.new(v)
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
