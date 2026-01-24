# Ruby Debugging

Debug patterns and tools for Ruby and Rails.

## binding.irb (Ruby 2.5+)

```ruby
def process_order(order)
  # Opens interactive console at this point
  binding.irb

  order.validate!
end
```

**IRB commands in binding.irb:**

| Command | Action |
|---------|--------|
| `exit` | Continue execution |
| `exit!` | Exit program |
| `whereami` | Show current code context |
| `show_source method_name` | Show method source |

## byebug

```ruby
# Gemfile
gem 'byebug', group: [:development, :test]

# In code
require 'byebug'

def process_order(order)
  byebug  # Execution stops here
  order.validate!
end
```

**byebug commands:**

| Command | Action |
|---------|--------|
| `n` (next) | Execute next line |
| `s` (step) | Step into method |
| `c` (continue) | Continue execution |
| `f` (finish) | Run until current method returns |
| `p expr` | Evaluate expression |
| `info locals` | Show local variables |
| `bt` (backtrace) | Show call stack |
| `up` / `down` | Navigate stack frames |

## Rails Console Debugging

```ruby
# Start console
rails console

# Reload after code changes
reload!

# Find and inspect objects
user = User.find(123)
user.attributes
user.orders.to_sql  # See generated SQL

# Test in sandbox (rollback on exit)
rails console --sandbox
```

## Rails Logger

```ruby
# In application code
Rails.logger.debug "Processing order: #{order.id}"
Rails.logger.info "Order created", order_id: order.id
Rails.logger.error "Payment failed", error: e.message

# Tagged logging
Rails.logger.tagged("OrderService", order.id) do
  Rails.logger.info "Starting processing"
end

# Tail logs
tail -f log/development.log
```

## pry-rails

```ruby
# Gemfile
gem 'pry-rails', group: [:development, :test]

# In code
binding.pry  # Opens Pry console

# Pry commands
ls object          # List methods/variables
cd object          # Change context to object
show-source method # Show method definition
show-doc method    # Show documentation
wtf?               # Show last exception
```

## Exception Debugging

```ruby
begin
  risky_operation
rescue => e
  # Full backtrace
  puts e.backtrace.join("\n")

  # Filtered backtrace (Rails)
  puts Rails.backtrace_cleaner.clean(e.backtrace).join("\n")

  # Re-raise with context
  raise "Order processing failed for order #{order.id}: #{e.message}"
end
```
