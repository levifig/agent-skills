# FastAPI Development

Building high-performance REST APIs with FastAPI.

## FastAPI Stack

| Component | Default |
|-----------|---------|
| Server | Uvicorn |
| Validation | Pydantic v2 |
| Auth | OAuth2/JWT |
| Testing | httpx |
| OpenAPI | Built-in |

## Application Structure

```python
from fastapi import FastAPI, Depends, HTTPException, status
from fastapi.middleware.cors import CORSMiddleware

app = FastAPI(
    title="My API",
    version="1.0.0",
    docs_url="/api/docs",
)

app.add_middleware(
    CORSMiddleware,
    allow_origins=["https://example.com"],
    allow_methods=["*"],
    allow_headers=["*"],
)

@app.get("/health")
async def health_check():
    return {"status": "healthy"}
```

## Request/Response Models

```python
from pydantic import BaseModel, EmailStr, Field

class UserCreate(BaseModel):
    email: EmailStr
    username: str = Field(..., min_length=3, max_length=50)
    password: str = Field(..., min_length=8)

class UserResponse(BaseModel):
    id: int
    email: EmailStr
    username: str
    model_config = {"from_attributes": True}
```

## Dependency Injection

```python
from typing import Annotated
from fastapi import Depends, Header

async def get_current_user(
    authorization: Annotated[str, Header()],
    db: Annotated[AsyncSession, Depends(get_db)]
) -> User:
    token = authorization.replace("Bearer ", "")
    user = await authenticate_token(db, token)
    if not user:
        raise HTTPException(status_code=401, detail="Invalid credentials")
    return user

@app.get("/users/me", response_model=UserResponse)
async def read_users_me(
    current_user: Annotated[User, Depends(get_current_user)]
):
    return current_user
```

## Router Organization

```python
from fastapi import APIRouter

router = APIRouter(prefix="/api/v1/users", tags=["users"])

@router.get("/", response_model=list[UserResponse])
async def list_users(skip: int = 0, limit: int = 100, db: AsyncSession = Depends(get_db)):
    result = await db.execute(select(User).offset(skip).limit(limit))
    return result.scalars().all()

@router.get("/{user_id}", response_model=UserResponse)
async def get_user(user_id: int, db: AsyncSession = Depends(get_db)):
    user = await db.get(User, user_id)
    if not user:
        raise HTTPException(status_code=404, detail="User not found")
    return user

app.include_router(router)
```

## Error Handling

```python
from fastapi import Request
from fastapi.responses import JSONResponse

@app.exception_handler(DomainException)
async def domain_exception_handler(request: Request, exc: DomainException):
    return JSONResponse(status_code=400, content={"detail": str(exc)})
```

## Background Tasks

```python
from fastapi import BackgroundTasks

@app.post("/users/", response_model=UserResponse)
async def create_user(
    user: UserCreate,
    background_tasks: BackgroundTasks,
    db: AsyncSession = Depends(get_db)
):
    db_user = User(**user.model_dump())
    db.add(db_user)
    await db.commit()
    background_tasks.add_task(send_welcome_email, user.email)
    return db_user
```

## Critical Rules

### Always
- Use async def for route handlers
- Define response_model for type safety
- Use dependency injection for shared logic
- Include proper status codes

### Never
- Block the event loop with sync I/O
- Return raw database objects
- Skip input validation
- Expose internal errors to clients
