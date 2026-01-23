# Go Error Handling Reference

Error patterns, custom types, and panic/recover.

## Philosophy

Errors are values, not exceptions. Returned, checked, and handled explicitly.

## Standard Pattern

```go
f, err := os.Open(filename)
if err != nil {
    return fmt.Errorf("open config: %w", err)
}
defer f.Close()
```

**Key points:** Check before using result, add context when wrapping, use `%w` to preserve chain.

## Custom Error Types

For errors carrying structured information:

```go
type ValidationError struct {
    Field   string
    Message string
}

func (e *ValidationError) Error() string {
    return fmt.Sprintf("%s: %s", e.Field, e.Message)
}
```

## Sentinel Errors

```go
var ErrNotFound = errors.New("not found")

if errors.Is(err, ErrNotFound) { /* handle */ }
```

## Error Wrapping

```go
// errors.Is - checks chain for match
if errors.Is(err, os.ErrNotExist) { }

// errors.As - extracts specific type
var pathErr *os.PathError
if errors.As(err, &pathErr) {
    fmt.Println(pathErr.Path)
}
```

## Panic and Recover

**Panic** - only for truly unrecoverable situations:
- Programming errors that shouldn't happen
- Initialization failures
- Never for expected conditions or in libraries

**Recover** - catches panics in deferred functions:

```go
defer func() {
    if r := recover(); r != nil {
        err = fmt.Errorf("panic: %v", r)
    }
}()
```

Use at boundaries (HTTP handlers, long-running services).

## Always/Never

| Always | Never |
|--------|-------|
| Check errors immediately | Ignore without explicit reason |
| Add context when wrapping | Wrap without adding information |
| Use `%w` to preserve chain | Use `%v` when callers need `errors.Is` |
| Define sentinel errors for known conditions | Compare error strings |
| Panic only for programmer errors | Panic for invalid user input |
| Recover at boundaries | Recover and silently continue |
| Document which errors a function returns | Leave error conditions undocumented |
