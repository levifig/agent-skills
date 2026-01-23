# Go Idioms Reference

Common patterns and conventions that make Go code idiomatic.

## Guard Clauses

Return early, keep the happy path unindented:

```go
func process(data []byte) error {
    if len(data) == 0 {
        return errors.New("empty data")
    }
    // main logic at base indentation
    return nil
}
```

**Omit else after return** - if the `if` returns, there's no need for `else`.

## Comma-Ok Idiom

Safely check optional second return value:

```go
s, ok := val.(string)   // type assertion
v, ok := m[key]         // map access
v, ok := <-ch           // channel receive (ok=false if closed)
```

## Blank Identifier

Discard values intentionally:

```go
_, err := io.Copy(dst, src)         // ignore byte count
for _, v := range slice { }         // ignore index
import _ "image/png"                // import for side effects
var _ Interface = (*Type)(nil)      // compile-time interface check
```

## Defer

Execute cleanup when function returns:

```go
f, err := os.Open(name)
if err != nil { return err }
defer f.Close()
```

**Key behaviors:** LIFO order, arguments evaluated at defer time, runs on panic.

## Interface Satisfaction Check

Compile-time verification:

```go
var _ io.Reader = (*MyReader)(nil)
```

Fails to compile if type doesn't satisfy interface.

## Embedding

### Struct Embedding

```go
type Reader struct {
    *bufio.Reader  // methods promoted
    count int
}
```

### Interface Embedding

```go
type ReadWriter interface {
    io.Reader
    io.Writer
}
```

## Method Receivers

| Use Pointer | Use Value |
|-------------|-----------|
| Modifies receiver | Doesn't modify |
| Large struct | Small type |
| Any method needs pointer | Maps, channels |

**Consistency:** If any method needs pointer, use pointer for all.

## Functional Options

Flexible configuration:

```go
type Option func(*Client)

func WithTimeout(d time.Duration) Option {
    return func(c *Client) { c.timeout = d }
}

client := NewClient("localhost", WithTimeout(5*time.Second))
```

## Always/Never

| Always | Never |
|--------|-------|
| Return early, omit else | Nest deeply with else chains |
| Use comma-ok for safe access | Assume map key exists |
| Defer cleanup immediately after acquiring | Defer in loops (accumulates) |
| Check interface satisfaction at compile time | Discover interface bugs at runtime |
| Be consistent with receiver types | Mix pointer/value arbitrarily |
| Use embedding for composition | Use embedding as inheritance |
