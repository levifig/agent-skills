# Python Database Operations

Async database operations with SQLAlchemy 2.0 and Alembic.

## Model Definition

```python
from sqlalchemy import String, Integer, ForeignKey, DateTime
from sqlalchemy.orm import DeclarativeBase, Mapped, mapped_column, relationship
from datetime import datetime

class Base(DeclarativeBase):
    pass

class User(Base):
    __tablename__ = "users"

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    email: Mapped[str] = mapped_column(String(255), unique=True, nullable=False)
    username: Mapped[str] = mapped_column(String(50), unique=True, nullable=False)
    is_active: Mapped[bool] = mapped_column(default=True)
    created_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow)

    posts: Mapped[list["Post"]] = relationship(back_populates="user", cascade="all, delete-orphan")

class Post(Base):
    __tablename__ = "posts"

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    user_id: Mapped[int] = mapped_column(ForeignKey("users.id"), nullable=False)
    title: Mapped[str] = mapped_column(String(200), nullable=False)
    user: Mapped["User"] = relationship(back_populates="posts")
```

## Database Connection

```python
from sqlalchemy.ext.asyncio import create_async_engine, AsyncSession, async_sessionmaker

engine = create_async_engine(
    "postgresql+asyncpg://user:pass@localhost/db",
    pool_size=5,
    max_overflow=10,
    pool_pre_ping=True
)

async_session_maker = async_sessionmaker(engine, class_=AsyncSession, expire_on_commit=False)

async def get_db() -> AsyncSession:
    async with async_session_maker() as session:
        yield session
```

## CRUD Operations

```python
from sqlalchemy import select, delete

class UserRepository:
    def __init__(self, session: AsyncSession):
        self.session = session

    async def create(self, **kwargs) -> User:
        user = User(**kwargs)
        self.session.add(user)
        await self.session.flush()
        await self.session.refresh(user)
        return user

    async def get_by_id(self, user_id: int) -> User | None:
        result = await self.session.execute(select(User).where(User.id == user_id))
        return result.scalar_one_or_none()

    async def list(self, skip: int = 0, limit: int = 100) -> list[User]:
        result = await self.session.execute(
            select(User).where(User.is_active == True).offset(skip).limit(limit)
        )
        return result.scalars().all()

    async def delete(self, user_id: int) -> bool:
        result = await self.session.execute(delete(User).where(User.id == user_id))
        return result.rowcount > 0
```

## Eager Loading

```python
from sqlalchemy.orm import selectinload

async def get_users_with_posts(session: AsyncSession) -> list[User]:
    result = await session.execute(
        select(User).options(selectinload(User.posts)).where(User.is_active == True)
    )
    return result.scalars().unique().all()
```

## Transactions

```python
async def transfer_ownership(session: AsyncSession, post_id: int, new_owner_id: int):
    async with session.begin():
        post = await session.get(Post, post_id)
        if not post:
            raise ValueError("Post not found")
        post.user_id = new_owner_id
        # Commit happens automatically if no exception
```

## Alembic Migrations

```python
# Create migration
# alembic revision --autogenerate -m "add users table"

# Migration file
from alembic import op
import sqlalchemy as sa

def upgrade() -> None:
    op.create_table(
        'users',
        sa.Column('id', sa.Integer(), nullable=False),
        sa.Column('email', sa.String(255), nullable=False),
        sa.PrimaryKeyConstraint('id'),
        sa.UniqueConstraint('email')
    )

def downgrade() -> None:
    op.drop_table('users')
```

## Critical Rules

### Always
- Use async session and queries
- Close sessions properly
- Use relationship() for foreign keys
- Create indexes for query columns
- Use migrations for schema changes

### Never
- Use sync SQLAlchemy in async apps
- Forget to await database calls
- Skip migrations for schema changes
- Use string queries without parameters
