# Generated client for the Type Coverage Convert Test webfunction package.
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

  class AddressAttributes
    def initialize(raw)
      @raw = raw
    end

    def street
      @raw["street"]
    end

    def city
      @raw["city"]
    end

    def verified
      @raw["verified"]
    end

  end

  class AccountAttributes
    def initialize(raw)
      @raw = raw
    end

    def id
      @raw["id"]
    end

    def email
      @raw["email"]
    end

    def age
      @raw["age"]
    end

    def active
      @raw["active"]
    end

    def score
      @raw["score"]
    end

    def nickname
      @raw["nickname"]
    end

    def tags
      @raw["tags"]
    end

    def role
      @raw["role"]
    end

    def home
      (v = @raw["home"]) && AddressAttributes.new(v)
    end

    def previous_homes
      (@raw["previous_homes"] || []).map { |item| AddressAttributes.new(item) }
    end

    def extra
      @raw["extra"]
    end

  end

  class NodeAttributes
    def initialize(raw)
      @raw = raw
    end

    def label
      @raw["label"]
    end

    def children
      (@raw["children"] || []).map { |item| NodeAttributes.new(item) }
    end

    def parent
      (v = @raw["parent"]) && NodeAttributes.new(v)
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

    # Exercises every argument type form.
    def create_account(email:, id: Undefined, homepage: Undefined, feed: Undefined, birthday: Undefined, wake_time: Undefined, last_login: Undefined, phone: Undefined, avatar: Undefined, v4: Undefined, v6: Undefined, host: Undefined, age: Undefined, big: Undefined, delta: Undefined, huge_delta: Undefined, ratio: Undefined, precise: Undefined, joined_at: Undefined, score: Undefined, active: Undefined, anything: Undefined, loose_list: Undefined, loose_object: Undefined, tags: Undefined, scores: Undefined, nickname: Undefined, contact: Undefined, role: Undefined, level: Undefined, home: Undefined, previous_homes: Undefined)
      args = {}
      args[:email] = email
      args[:id] = id unless id.equal?(Undefined)
      args[:homepage] = homepage unless homepage.equal?(Undefined)
      args[:feed] = feed unless feed.equal?(Undefined)
      args[:birthday] = birthday unless birthday.equal?(Undefined)
      args[:wake_time] = wake_time unless wake_time.equal?(Undefined)
      args[:last_login] = last_login unless last_login.equal?(Undefined)
      args[:phone] = phone unless phone.equal?(Undefined)
      args[:avatar] = avatar unless avatar.equal?(Undefined)
      args[:v4] = v4 unless v4.equal?(Undefined)
      args[:v6] = v6 unless v6.equal?(Undefined)
      args[:host] = host unless host.equal?(Undefined)
      args[:age] = age unless age.equal?(Undefined)
      args[:big] = big unless big.equal?(Undefined)
      args[:delta] = delta unless delta.equal?(Undefined)
      args[:huge_delta] = huge_delta unless huge_delta.equal?(Undefined)
      args[:ratio] = ratio unless ratio.equal?(Undefined)
      args[:precise] = precise unless precise.equal?(Undefined)
      args[:joined_at] = joined_at unless joined_at.equal?(Undefined)
      args[:score] = score unless score.equal?(Undefined)
      args[:active] = active unless active.equal?(Undefined)
      args[:anything] = anything unless anything.equal?(Undefined)
      args[:loose_list] = loose_list unless loose_list.equal?(Undefined)
      args[:loose_object] = loose_object unless loose_object.equal?(Undefined)
      args[:tags] = tags unless tags.equal?(Undefined)
      args[:scores] = scores unless scores.equal?(Undefined)
      args[:nickname] = nickname unless nickname.equal?(Undefined)
      args[:contact] = contact unless contact.equal?(Undefined)
      args[:role] = role unless role.equal?(Undefined)
      args[:level] = level unless level.equal?(Undefined)
      args[:home] = home unless home.equal?(Undefined)
      args[:previous_homes] = previous_homes unless previous_homes.equal?(Undefined)
      (v = @real_client.create_account(**args)) && AccountAttributes.new(v)
    end

    def list_accounts(limit: Undefined)
      args = {}
      args[:limit] = limit unless limit.equal?(Undefined)
      TypedPage.new(@real_client.list_accounts(**args), nil)
    end

    def get_stats
      @real_client.get_stats
    end

    def ping
      @real_client.ping
    end

    def get_tags
      @real_client.get_tags
    end

    def find_account(id:)
      args = {}
      args[:id] = id
      (v = @real_client.find_account(**args)) && AccountAttributes.new(v)
    end

    def get_tree
      (v = @real_client.get_tree) && NodeAttributes.new(v)
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
