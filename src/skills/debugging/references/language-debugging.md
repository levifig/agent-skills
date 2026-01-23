# Language-Specific Debugging

Debug patterns and tools for Python, TypeScript, and Ruby.

## Python Debugging

### Interactive Debugging with pdb

```python
# Insert breakpoint in code
import pdb; pdb.set_trace()

# Python 3.7+ built-in
breakpoint()

# Conditional breakpoint
if suspicious_condition:
    breakpoint()
```

**Common pdb commands:**

| Command | Action |
|---------|--------|
| `n` (next) | Execute next line |
| `s` (step) | Step into function |
| `c` (continue) | Continue to next breakpoint |
| `p expr` | Print expression value |
| `pp expr` | Pretty-print expression |
| `l` (list) | Show current code context |
| `w` (where) | Print stack trace |
| `u` (up) | Move up stack frame |
| `d` (down) | Move down stack frame |
| `q` (quit) | Exit debugger |

### Structured Logging

```python
import structlog

logger = structlog.get_logger()

# Add context that persists across log calls
logger = logger.bind(request_id=request.id, user_id=user.id)

# Log with structured data
logger.info("order_created", order_id=order.id, total=order.total)
logger.error("payment_failed",
    order_id=order.id,
    error_code=e.code,
    error_message=str(e)
)
```

### Traceback Analysis

```python
import traceback

try:
    risky_operation()
except Exception as e:
    # Full traceback as string
    tb_str = traceback.format_exc()
    logger.error("operation_failed", traceback=tb_str)

    # Extract specific frame info
    tb = traceback.extract_tb(e.__traceback__)
    for frame in tb:
        logger.debug("frame",
            filename=frame.filename,
            line=frame.lineno,
            function=frame.name
        )
```

### pytest Debugging

```bash
# Drop into debugger on failure
pytest --pdb

# Drop into debugger on first failure, then exit
pytest -x --pdb

# Show local variables in tracebacks
pytest -l

# Verbose output with print statements
pytest -v -s

# Run specific test
pytest tests/test_orders.py::test_create_order -v
```

### Remote Debugging

```python
# Using debugpy (VS Code compatible)
import debugpy
debugpy.listen(5678)
debugpy.wait_for_client()  # Pause until debugger attaches
breakpoint()
```

## TypeScript Debugging

### Console Methods

```typescript
// Basic logging
console.log('value:', value);
console.error('Error:', error);
console.warn('Warning:', message);

// Structured output
console.table(arrayOfObjects);
console.dir(complexObject, { depth: null });

// Timing
console.time('operation');
// ... operation ...
console.timeEnd('operation');

// Grouping related logs
console.group('Request Processing');
console.log('Step 1');
console.log('Step 2');
console.groupEnd();

// Conditional logging
console.assert(condition, 'Condition failed:', data);

// Stack trace without error
console.trace('How did we get here?');
```

### Debugger Statement

```typescript
function processOrder(order: Order): void {
  // Pauses execution when DevTools is open
  debugger;

  // Conditional debugger
  if (order.total > 10000) {
    debugger;
  }
}
```

### Source Maps

Ensure `tsconfig.json` has source maps enabled:

```json
{
  "compilerOptions": {
    "sourceMap": true,
    "inlineSources": true
  }
}
```

For debugging bundled code, verify source maps are generated and served:

```typescript
// webpack.config.js
module.exports = {
  devtool: 'source-map', // Full source maps for debugging
};
```

### Node.js Debugging

```bash
# Start with inspector
node --inspect dist/server.js

# Break on first line
node --inspect-brk dist/server.js

# With ts-node
node --inspect -r ts-node/register src/server.ts
```

### Chrome DevTools Tips

1. **Breakpoints**: Click line number in Sources panel
2. **Conditional breakpoints**: Right-click line number, add condition
3. **Logpoints**: Right-click, add logpoint (logs without pausing)
4. **Watch expressions**: Add variables to Watch panel
5. **Call stack**: Navigate up/down the call stack
6. **Scope**: Inspect local, closure, and global variables

### Async Debugging

```typescript
// Capture async stack traces
Error.stackTraceLimit = 50;

// Label promises for debugging
const result = await Promise.race([
  fetchData().then(data => ({ source: 'fetch', data })),
  timeout(5000).then(() => ({ source: 'timeout', data: null }))
]);
console.log('Winner:', result.source);
```

## Ruby Debugging

### binding.irb (Ruby 2.5+)

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

### byebug

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

### Rails Console Debugging

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

### Rails Logger

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

### pry-rails

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

### Exception Debugging

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

## Cross-Language Tips

### Binary Search Debugging

When you have a long sequence of operations and don't know where the failure occurs:

1. Add logging/breakpoint at the midpoint
2. Determine if failure is before or after
3. Repeat with the relevant half
4. Continue until you isolate the exact line

### Minimal Reproduction

1. Remove code until bug disappears
2. Add back the last removed piece
3. That piece contains or triggers the bug
4. Create minimal test case that reproduces

### Rubber Duck Debugging

When stuck:

1. Explain the code line-by-line out loud
2. Explain what you expect vs. what happens
3. Explain your hypotheses and why each might be wrong
4. Often, articulating the problem reveals the solution
