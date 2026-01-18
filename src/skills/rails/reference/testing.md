# Rails Testing with Minitest

Test-driven development the Rails way: Minitest, fixtures, behavior testing.

## Core Philosophy

- **Minitest over RSpec** - Simpler, faster, built-in
- **Fixtures over factories** - Loaded once, predictable, fast
- **Test behavior, not implementation** - What it does, not how
- **Fast tests enable TDD** - Slow tests won't run

## Fixtures

```yaml
# test/fixtures/users.yml
alice:
  email: alice@example.com
  name: Alice Smith
  role: admin

bob:
  email: bob@example.com
  name: Bob Jones
  role: viewer

# test/fixtures/posts.yml
published_post:
  user: alice
  title: Published Article
  published: true
  published_at: <%= 1.day.ago %>

draft_post:
  user: alice
  title: Draft Article
  published: false
```

Access in tests: `users(:alice)`, `posts(:published_post)`

## Model Tests

```ruby
class UserTest < ActiveSupport::TestCase
  test "requires email" do
    user = User.new(name: "Test")
    assert_not user.valid?
    assert_includes user.errors[:email], "can't be blank"
  end

  test "email must be unique" do
    user = User.new(email: users(:alice).email)
    assert_not user.valid?
  end

  test "active scope excludes deactivated" do
    assert_includes User.active, users(:alice)
    assert_not_includes User.active, users(:deactivated)
  end
end
```

## Controller/Integration Tests

```ruby
class PostsControllerTest < ActionDispatch::IntegrationTest
  test "index shows published posts" do
    get posts_url
    assert_response :success
    assert_select "article", minimum: 1
  end

  test "create requires authentication" do
    post posts_url, params: { post: { title: "New" } }
    assert_redirected_to login_url
  end

  test "create with valid params" do
    sign_in_as(users(:alice))
    assert_difference "Post.count", 1 do
      post posts_url, params: { post: { title: "New", body: "Content" } }
    end
    assert_redirected_to post_url(Post.last)
  end
end
```

## System Tests (JavaScript/Turbo)

```ruby
class PostsTest < ApplicationSystemTestCase
  driven_by :selenium, using: :headless_chrome

  test "creating a post with Turbo" do
    sign_in_as(users(:alice))
    visit posts_url

    click_on "New Post"
    fill_in "Title", with: "System Test Post"
    fill_in "Body", with: "Content here"
    click_on "Create Post"

    assert_text "Post was successfully created"
    assert_text "System Test Post"
  end
end
```

## Test Organization

```
test/
├── fixtures/        # YAML test data
├── models/          # Model unit tests
├── controllers/     # HTTP integration tests
├── system/          # Browser tests (JS, Turbo)
├── services/        # Service object tests
└── test_helper.rb   # Shared setup
```

## When to Mock

Only mock external services and time:

```ruby
# External services
Stripe::Charge.stub :create, mock_charge do
  assert PaymentService.charge(user, 1000).success?
end

# Time-dependent behavior
travel_to 31.days.from_now do
  assert subscription.expired?
end
```

Never mock your own models, services, or database operations.

## Test Helpers

```ruby
# test/test_helper.rb
class ActiveSupport::TestCase
  def sign_in_as(user)
    post login_url, params: {
      email: user.email,
      password: "password"  # All fixtures use same password
    }
  end
end
```

## Running Tests

```bash
bin/rails test                    # All tests
bin/rails test:models             # Model tests only
bin/rails test test/models/user_test.rb  # Single file
bin/rails test test/models/user_test.rb:10  # Single test (line number)
bin/rails test:system             # System tests (browser)
```

## Assertions

```ruby
assert user.valid?
assert_not user.admin?
assert_equal "Alice", user.name
assert_includes User.active, user
assert_difference "Post.count", 1 do
  Post.create!(title: "New")
end
assert_no_difference "User.count" do
  User.create(email: nil)  # Invalid
end
```
