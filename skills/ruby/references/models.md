# Rails Models and Active Record

Patterns for data integrity and business logic.

## Model Organization

Standard order within model files:

```ruby
class User < ApplicationRecord
  # == Constants
  ROLES = %w[admin editor viewer].freeze

  # == Includes/Extends
  include Searchable

  # == Associations
  belongs_to :organization
  has_many :posts, dependent: :destroy

  # == Validations
  validates :email, presence: true, uniqueness: true
  validates :role, inclusion: { in: ROLES }

  # == Callbacks
  before_validation :normalize_email

  # == Scopes
  scope :active, -> { where(active: true) }
  scope :admins, -> { where(role: 'admin') }

  # == Class Methods
  def self.find_by_credentials(email, password)
    find_by(email: email)&.authenticate(password)
  end

  # == Instance Methods
  def full_name
    "#{first_name} #{last_name}"
  end

  private

  def normalize_email
    self.email = email.downcase.strip if email.present?
  end
end
```

## Associations

```ruby
# Always specify dependent option
has_many :posts, dependent: :destroy
has_many :comments, dependent: :nullify

# Use inverse_of for bidirectional associations
has_many :memberships, inverse_of: :user
belongs_to :user, inverse_of: :memberships

# Counter caches for performance
belongs_to :post, counter_cache: true
```

## Validations

```ruby
# Presence with database NOT NULL
validates :name, presence: true
# Migration: t.string :name, null: false

# Uniqueness with database unique index
validates :email, uniqueness: { case_sensitive: false }
# Migration: add_index :users, :email, unique: true

# Format validation
validates :email, format: { with: URI::MailTo::EMAIL_REGEXP }

# Custom validation
validate :start_date_before_end_date

private

def start_date_before_end_date
  return unless start_date && end_date
  errors.add(:end_date, "must be after start date") if end_date <= start_date
end
```

## Scopes

```ruby
# Simple scopes
scope :published, -> { where(published: true) }
scope :recent, -> { order(created_at: :desc) }

# Scopes with arguments (use class method)
def self.created_after(date)
  where("created_at > ?", date)
end

# Chainable
User.active.admins.recent
```

## Callbacks

Use sparingly - only for data normalization and maintaining consistency:

```ruby
# Good: Data normalization
before_validation :normalize_phone

# Good: Maintaining consistency
after_save :update_search_index

# Bad: External side effects - move to service objects
# after_create :send_welcome_email  # Use service instead
# after_save :sync_to_external_api  # Use job instead
```

## Migrations

```ruby
class CreatePosts < ActiveRecord::Migration[8.0]
  def change
    create_table :posts do |t|
      t.references :user, null: false, foreign_key: true
      t.string :title, null: false
      t.text :body
      t.boolean :published, default: false, null: false
      t.datetime :published_at

      t.timestamps
    end

    add_index :posts, [:user_id, :published]
    add_index :posts, :published_at
  end
end
```

## Query Patterns

```ruby
# Avoid N+1
Post.includes(:author, :comments).find(params[:id])

# Select only needed columns
User.select(:id, :name, :email)
User.where(active: true).pluck(:email)

# Batch processing
User.find_each(batch_size: 1000) { |u| process(u) }
```
