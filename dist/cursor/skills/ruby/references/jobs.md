# Background Jobs with Solid Queue

Database-backed background processing for Rails 8+.

## Core Philosophy

- **Database-backed** - No Redis dependency, uses FOR UPDATE SKIP LOCKED
- **Active Job interface** - Standard Rails API, queue-agnostic
- **Idempotent jobs** - Safe to retry, safe to run multiple times
- **Simple arguments** - Pass IDs, not objects

## Configuration

```yaml
# config/solid_queue.yml
default: &default
  dispatchers:
    - polling_interval: 1
      batch_size: 500
  workers:
    - queues: [critical]
      threads: 3
      polling_interval: 0.1
    - queues: [default, mailers]
      threads: 5
      polling_interval: 1
    - queues: [low]
      threads: 2
      polling_interval: 5
```

## Job Structure

```ruby
# app/jobs/process_order_job.rb
class ProcessOrderJob < ApplicationJob
  queue_as :default
  limits_concurrency to: 1, key: ->(order_id) { order_id }

  retry_on ActiveRecord::Deadlocked, wait: 5.seconds, attempts: 3
  discard_on ActiveJob::DeserializationError

  def perform(order_id)
    order = Order.find(order_id)
    return if order.processed?  # Idempotency check

    Orders::Process.new(order).call
  end
end
```

## Queue Priorities

| Queue | Use Case | Polling |
|-------|----------|---------|
| `critical` | Payments, security | 0.1s |
| `default` | Standard operations | 1s |
| `mailers` | Email delivery | 1s |
| `low` | Reports, cleanup | 5s |

## Error Handling

```ruby
class WebhookJob < ApplicationJob
  retry_on Net::OpenTimeout, wait: :polynomially_longer, attempts: 5
  retry_on Faraday::ConnectionFailed, wait: 30.seconds, attempts: 3
  discard_on ActiveRecord::RecordNotFound

  def perform(webhook_id)
    Webhooks::Deliver.new(Webhook.find(webhook_id)).call
  rescue Webhooks::PermanentFailure => e
    Rails.error.report(e)  # Log but don't retry
  end
end
```

## Concurrency Controls

```ruby
class ImportJob < ApplicationJob
  # Only one import per user at a time
  limits_concurrency to: 1, key: ->(user_id, _file) { user_id }

  def perform(user_id, file_path)
    Users::Import.new(User.find(user_id), file_path).call
  end
end
```

## Recurring Jobs

```yaml
# config/recurring.yml
production:
  cleanup_sessions:
    class: CleanupSessionsJob
    schedule: every day at 3am
  sync_inventory:
    class: SyncInventoryJob
    schedule: every 15 minutes
```

## Scheduling

```ruby
# Immediate
ProcessOrderJob.perform_later(order.id)

# Delayed
ReminderJob.set(wait: 24.hours).perform_later(user.id)

# Specific time
ReportJob.set(wait_until: Date.tomorrow.noon).perform_later
```

## Testing Jobs

```ruby
class ProcessOrderJobTest < ActiveSupport::TestCase
  test "processes valid order" do
    order = orders(:pending)
    ProcessOrderJob.perform_now(order.id)
    assert order.reload.processed?
  end

  test "is idempotent" do
    order = orders(:pending)
    2.times { ProcessOrderJob.perform_now(order.id) }
    assert_equal 1, order.processing_count
  end
end
```

## Best Practices

- Design jobs to be idempotent (check state before acting)
- Pass IDs, not ActiveRecord objects
- Set retry strategies per error type
- Use concurrency limits for resource-constrained operations
- Never store sensitive data in job arguments
