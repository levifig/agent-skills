# Pydantic Data Validation

Type-safe data validation and settings management with Pydantic v2.

## Basic Models

```python
from pydantic import BaseModel, Field, EmailStr, HttpUrl
from datetime import datetime

class User(BaseModel):
    id: int
    email: EmailStr
    username: str = Field(..., min_length=3, max_length=50)
    created_at: datetime = Field(default_factory=datetime.utcnow)
    website: HttpUrl | None = None

    model_config = {
        "str_strip_whitespace": True,
        "validate_assignment": True,
        "from_attributes": True,
    }

user = User(id=1, email="user@example.com", username="johndoe")
print(user.model_dump())      # Convert to dict
print(user.model_dump_json()) # Convert to JSON
```

## Field Validators

```python
from pydantic import field_validator, model_validator
from typing import Self

class UserRegistration(BaseModel):
    email: EmailStr
    password: str = Field(..., min_length=8)
    password_confirm: str

    @field_validator("password")
    @classmethod
    def password_strength(cls, v: str) -> str:
        if not any(c.isupper() for c in v):
            raise ValueError("Must contain uppercase letter")
        if not any(c.isdigit() for c in v):
            raise ValueError("Must contain digit")
        return v

    @model_validator(mode="after")
    def passwords_match(self) -> Self:
        if self.password != self.password_confirm:
            raise ValueError("Passwords do not match")
        return self
```

## Settings Management

```python
from pydantic_settings import BaseSettings, SettingsConfigDict

class Settings(BaseSettings):
    app_name: str = "My Application"
    debug: bool = False
    database_url: str = Field(..., validation_alias="DATABASE_URL")
    api_key: str = Field(..., validation_alias="API_KEY")
    redis_host: str = "localhost"

    model_config = SettingsConfigDict(
        env_file=".env",
        case_sensitive=False,
        extra="ignore",
    )

settings = Settings()
```

## Nested Models

```python
class Address(BaseModel):
    street: str
    city: str
    postal_code: str = Field(..., pattern=r"^\d{5}(-\d{4})?$")

class Company(BaseModel):
    name: str
    address: Address
    employees: list[User] = []
```

## Computed Fields

```python
from pydantic import computed_field

class Product(BaseModel):
    name: str
    price: float = Field(..., gt=0)
    tax_rate: float = 0.08

    @computed_field
    @property
    def price_with_tax(self) -> float:
        return round(self.price * (1 + self.tax_rate), 2)
```

## Serialization Control

```python
from pydantic import field_serializer

class UserWithPassword(BaseModel):
    username: str
    password: str = Field(..., exclude=True)  # Never serialize

    @field_serializer("last_login")
    def serialize_datetime(self, dt: datetime | None) -> str | None:
        return dt.isoformat() if dt else None

# Serialization modes
user.model_dump(exclude_unset=True)
user.model_dump(exclude={"password"})
user.model_dump(include={"username", "email"})
```

## Critical Rules

### Always
- Use Field() for constraints and metadata
- Use computed_field for calculated properties
- Use proper Pydantic types (EmailStr, HttpUrl)
- Use model_dump() instead of .dict()

### Never
- Bypass validation with model_construct()
- Use mutable defaults (use default_factory)
- Store sensitive data without exclude=True
- Mix Pydantic v1 and v2 patterns
