# Test Debugging

Strategies for diagnosing flaky tests, isolation failures, and environment-related test issues.

## Flaky Test Diagnosis

A flaky test passes sometimes and fails sometimes with the same code. Common causes:

### Timing Dependencies

**Symptoms:**
- Test passes locally, fails in CI
- Test fails more often under load
- Adding `sleep()` makes it pass

**Investigation:**
```python
# Bad: Depends on timing
def test_async_operation():
    start_async_job()
    time.sleep(1)  # Hope it's done
    assert job_completed()

# Good: Wait for condition
def test_async_operation():
    start_async_job()
    wait_for(lambda: job_completed(), timeout=5)
    assert job_completed()
```

**Diagnosis steps:**
1. Run test 100 times in a loop
2. Monitor CPU/memory during failures
3. Add timestamps to understand timing
4. Check for hardcoded timeouts that are too short

### Order Dependencies

**Symptoms:**
- Test passes when run alone, fails in suite
- Test fails only when run after specific other test
- Reordering tests changes results

**Investigation:**
```bash
# pytest: Run in random order
pytest --random-order

# Find the guilty test pair
pytest tests/test_a.py tests/test_b.py -v  # Passes
pytest tests/test_b.py tests/test_a.py -v  # Fails?
```

**Common culprits:**
- Global state modified by previous test
- Database records not cleaned up
- Cached values persisting
- Module-level side effects

### Non-Deterministic Data

**Symptoms:**
- Fails occasionally with no pattern
- Different assertion failures each time
- Works with specific seed/data

**Investigation:**
```python
# Bad: Non-deterministic
def test_random_selection():
    items = get_random_items(10)
    assert len(items) == 10  # What if source has < 10?

# Good: Control randomness
def test_random_selection():
    random.seed(42)
    items = get_random_items(10)
    assert len(items) == 10
```

## Test Isolation Patterns

### Database Isolation

```python
# pytest with transactions
@pytest.fixture
def db_session(engine):
    connection = engine.connect()
    transaction = connection.begin()
    session = Session(bind=connection)

    yield session

    session.close()
    transaction.rollback()
    connection.close()
```

```ruby
# Rails transactional tests
class OrderTest < ActiveSupport::TestCase
  # Each test runs in a transaction that rolls back
  self.use_transactional_tests = true

  test "creates order" do
    order = Order.create!(total: 100)
    assert order.persisted?
  end  # Order is rolled back
end
```

### External Service Isolation

```python
# Mock external services
@pytest.fixture
def mock_payment_api(mocker):
    return mocker.patch(
        'app.services.payment.PaymentAPI.charge',
        return_value={'status': 'success', 'id': 'ch_123'}
    )

def test_order_payment(mock_payment_api):
    order = create_order(total=100)
    order.process_payment()

    mock_payment_api.assert_called_once_with(amount=100)
    assert order.payment_status == 'paid'
```

### Time Isolation

```python
# Freeze time for deterministic tests
from freezegun import freeze_time

@freeze_time("2024-01-15 12:00:00")
def test_order_expiry():
    order = create_order()
    assert not order.expired

    with freeze_time("2024-01-16 12:00:01"):
        assert order.expired  # 24+ hours later
```

```typescript
// Jest fake timers
jest.useFakeTimers();

test('order expires after 24 hours', () => {
  const order = createOrder();
  expect(order.expired).toBe(false);

  jest.advanceTimersByTime(24 * 60 * 60 * 1000 + 1);
  expect(order.expired).toBe(true);
});
```

## State Pollution Detection

### Symptoms

- Test A passes alone, fails after test B
- Test fails only when suite runs in specific order
- "Works on my machine" syndrome

### Detection Strategy

```bash
# 1. Run suspect test in isolation
pytest tests/test_orders.py::test_specific -v

# 2. Run with a known "polluter"
pytest tests/test_other.py tests/test_orders.py::test_specific -v

# 3. Binary search for the polluter
pytest tests/test_orders.py::test_specific --last-failed
```

### Common State Pollution Sources

| Source | Detection | Fix |
|--------|-----------|-----|
| Module globals | Check imports for mutations | Use fixtures, reset in teardown |
| Class variables | Look for `cls.` modifications | Use instance variables |
| Singletons | Check shared instances | Reset or mock singletons |
| Environment variables | Print env in failing test | Restore in teardown |
| File system | Check for temp files | Use tmp directories, clean up |
| Database | Check for leftover records | Transaction rollback |
| Caches | Check memoized values | Clear caches in setup |

### Teardown Pattern

```python
@pytest.fixture(autouse=True)
def reset_global_state():
    # Setup: Save original state
    original_config = app.config.copy()

    yield

    # Teardown: Restore original state
    app.config = original_config
    cache.clear()
```

## Environment Differences

### Local vs CI Failures

| Difference | Detection | Solution |
|------------|-----------|----------|
| Timing | CI slower, races more likely | Use proper waits, not sleeps |
| Resources | CI has less memory/CPU | Check resource usage |
| Parallelism | CI runs tests in parallel | Ensure isolation |
| File paths | Different temp directories | Use portable paths |
| Timezone | CI uses UTC | Freeze time or use UTC |
| Locale | CI uses different locale | Set locale explicitly |

### Debugging CI-Only Failures

```yaml
# GitHub Actions: Enable debug logging
env:
  ACTIONS_STEP_DEBUG: true
  ACTIONS_RUNNER_DEBUG: true

# Add verbose output
steps:
  - name: Run tests
    run: |
      echo "=== Environment ==="
      env | sort
      echo "=== Python version ==="
      python --version
      echo "=== Running tests ==="
      pytest -v --tb=long
```

### Reproducing CI Environment Locally

```bash
# Docker: Use same image as CI
docker run -it --rm -v $(pwd):/app -w /app python:3.11 bash
pip install -r requirements.txt
pytest

# Act: Run GitHub Actions locally
act -j test

# Environment parity
export CI=true
export TZ=UTC
pytest
```

## Test Debugging Workflow

### 1. Isolate the Failure

```bash
# Run failing test alone
pytest tests/test_orders.py::test_failing -v

# If it passes alone, find the interaction
pytest --collect-only | head -20  # See test order
```

### 2. Add Diagnostic Output

```python
def test_order_processing(caplog):
    with caplog.at_level(logging.DEBUG):
        order = process_order(data)

    print(f"Captured logs: {caplog.text}")
    print(f"Order state: {order.__dict__}")
    assert order.status == "completed"
```

### 3. Minimize the Test

```python
# Start with failing test
def test_order_processing():
    user = create_user()
    product = create_product()
    cart = create_cart(user, product)
    order = create_order(cart)
    payment = process_payment(order)
    shipment = create_shipment(order)
    assert shipment.status == "ready"

# Remove pieces until bug disappears
def test_order_processing_minimal():
    order = create_order()  # Minimal setup
    assert order.status == "pending"  # Bug is here!
```

### 4. Check Assumptions

```python
def test_order_total():
    order = create_order()

    # Verify assumptions
    print(f"Order items: {order.items}")
    print(f"Item prices: {[i.price for i in order.items]}")
    print(f"Expected total: {sum(i.price for i in order.items)}")
    print(f"Actual total: {order.total}")

    assert order.total == Decimal("100.00")
```

## Flaky Test Prevention

| Practice | Why It Helps |
|----------|--------------|
| Use factories, not fixtures | Fresh data each test |
| Avoid shared state | Tests can't pollute each other |
| Mock time and randomness | Deterministic behavior |
| Use transactions | Auto-cleanup |
| Run tests in random order | Catch order dependencies |
| Run tests in parallel | Catch isolation issues |
| Set explicit timeouts | Fail fast on hangs |
