# Generated client for the Private Required Test webfunction package.
# Source: https://example.test/package
#
# Wraps the real web_function gem (github.com/robinclart/web_function) -
# every method here calls straight through to the real dynamic client.
# frozen_string_literal: true

require "web_function"

module WebFunctionClient
  class PersonAttributes
    def initialize(raw)
      @raw = raw
    end

    def name
      @raw["name"]
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

    def get_person(id:)
      args = {}
      args[:id] = id
      (v = @real_client.get_person(**args)) && PersonAttributes.new(v)
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
