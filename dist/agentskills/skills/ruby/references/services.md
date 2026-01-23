# Rails Service Objects

Patterns for encapsulating complex business operations.

## When to Use Services vs Model Methods

| Use Service Objects | Use Model Methods |
|---------------------|-------------------|
| Multi-model operations | Single-model concerns |
| External API calls | Data transformations |
| Complex transactions | Simple validations |
| Workflow orchestration | Derived attributes |

## Service Structure

```ruby
# app/services/users/register.rb
module Users
  class Register
    include ActiveModel::Model
    include ActiveModel::Attributes

    attribute :email, :string
    attribute :name, :string

    validates :email, :name, presence: true

    def call
      return Result.failure(errors.full_messages) unless valid?

      ActiveRecord::Base.transaction do
        user = User.create!(email:, name:)
        Membership.create!(user:, role: :member)
        WelcomeMailer.with(user:).welcome.deliver_later
        Result.success(user)
      end
    rescue ActiveRecord::RecordInvalid => e
      Result.failure([e.message])
    end
  end
end
```

## Result Object Pattern

```ruby
# app/services/result.rb
class Result
  attr_reader :value, :errors

  def initialize(success:, value: nil, errors: [])
    @success = success
    @value = value
    @errors = Array(errors)
  end

  def success? = @success
  def failure? = !@success

  def self.success(value = nil)
    new(success: true, value:)
  end

  def self.failure(errors)
    new(success: false, errors:)
  end

  def on_success
    yield(value) if success?
    self
  end

  def on_failure
    yield(errors) if failure?
    self
  end
end
```

## Controller Usage

```ruby
def create
  Users::Register.new(user_params).call
    .on_success { |user| redirect_to user, notice: "Welcome!" }
    .on_failure { |errors| @errors = errors; render :new, status: :unprocessable_entity }
end
```

## Transaction Handling

```ruby
def call
  ActiveRecord::Base.transaction do
    reserve_inventory!  # Raises on failure, triggers rollback
    charge_payment!
    create_order!
  end
  Result.success(order)
rescue Inventory::InsufficientStock => e
  Result.failure(["Insufficient stock: #{e.message}"])
rescue Payment::Failed => e
  Result.failure(["Payment failed: #{e.message}"])
end
```

## Dependency Injection

```ruby
class Payments::Charge
  def initialize(gateway: Stripe::Gateway.new)
    @gateway = gateway
  end

  def call(amount:, source:)
    result = @gateway.charge(amount:, source:)
    Result.success(result)
  rescue Gateway::Error => e
    Result.failure([e.message])
  end
end

# In tests
mock_gateway = Minitest::Mock.new
Payments::Charge.new(gateway: mock_gateway).call(amount: 1000, source: token)
```

## Testing Services

```ruby
class Users::RegisterTest < ActiveSupport::TestCase
  test "creates user with valid attributes" do
    result = Users::Register.new(
      email: "test@example.com",
      name: "Test User"
    ).call

    assert result.success?
    assert_equal "test@example.com", result.value.email
  end

  test "fails with invalid email" do
    result = Users::Register.new(email: nil, name: "Test").call

    assert result.failure?
    assert_includes result.errors.join, "Email"
  end

  test "rolls back on failure" do
    assert_no_difference "User.count" do
      Users::Register.new(email: nil, name: nil).call
    end
  end
end
```

## File Organization

```
app/services/
├── result.rb                    # Shared result object
├── users/
│   ├── register.rb
│   └── update_profile.rb
├── orders/
│   ├── process.rb
│   └── refund.rb
└── payments/
    └── charge.rb
```

## Naming Conventions

Use verb-based names that describe the operation:
- `Users::Register` not `UserService`
- `Orders::Process` not `OrderProcessor`
- `Payments::Charge` not `PaymentHandler`
