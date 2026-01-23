# Go Core Reference

Go development fundamentals: tooling, project structure, naming, formatting, and data types.

## Go Stack

| Component | Tool | Notes |
|-----------|------|-------|
| Runtime | Go 1.21+ | Garbage collected, statically linked binaries |
| Package Manager | Go Modules | `go.mod`, `go.sum` |
| Linter | `golangci-lint` | Aggregates multiple linters |
| Formatter | `gofmt` | Canonical formatting, no configuration |
| Testing | `go test` | Built-in, table-driven patterns |
| Documentation | `godoc` | Comments become documentation |

## Project Structure

```
project/
├── go.mod              # Module definition
├── go.sum              # Dependency checksums
├── main.go             # Entry point (for commands)
├── cmd/                # Multiple entry points
│   └── myapp/
│       └── main.go
├── internal/           # Private packages (enforced by Go)
│   └── config/
├── pkg/                # Public library code (optional convention)
└── *_test.go           # Tests alongside source
```

**Key rules:**
- `internal/` packages cannot be imported outside the module
- Tests live next to the code they test (`foo.go` → `foo_test.go`)
- One package per directory

## Naming Conventions

| Element | Rule | Examples |
|---------|------|----------|
| Packages | Short, lowercase, no underscores | `http`, `io`, `bufio` |
| Files | lowercase, underscores OK | `http_client.go`, `parser.go` |
| Exported names | PascalCase | `NewClient`, `ErrNotFound` |
| Unexported names | camelCase | `parseHeader`, `defaultTimeout` |
| Interfaces | Describe behavior, often "-er" | `Reader`, `Closer`, `Handler` |
| Getters | No "Get" prefix | `func (u *User) Name() string` |
| Setters | "Set" prefix | `func (u *User) SetName(n string)` |
| Acronyms | Consistent case | `HTTPServer` or `httpServer`, not `HttpServer` |

## Formatting

`gofmt` is the law. No configuration, no debate.

**Key rules:**
- Tabs for indentation
- Opening brace on same line
- No trailing whitespace
- Imports grouped: standard library, then external, then internal

```go
import (
    "context"
    "fmt"

    "github.com/pkg/errors"

    "mymodule/internal/config"
)
```

## Data Types

### new vs make

| Function | Creates | Zero value | Returns |
|----------|---------|------------|---------|
| `new(T)` | Any type | Yes | `*T` (pointer) |
| `make(T)` | slice, map, channel | Initialized | `T` (value) |

```go
p := new(int)      // *int, points to 0
s := make([]int, 5) // []int with len 5, cap 5
m := make(map[string]int) // initialized map
```

### Slices vs Arrays

- Arrays have fixed size: `[5]int`
- Slices are dynamic views: `[]int`
- Slices reference underlying arrays; copies share data unless you use `copy()`

### Maps

- Zero value is `nil` (reading returns zero, writing panics)
- Use `make()` before writing
- Comma-ok idiom: `v, ok := m[key]`

## Always/Never

| Always | Never |
|--------|-------|
| Run `gofmt` before committing | Disable or configure gofmt |
| Use `go mod tidy` to clean deps | Vendor without good reason |
| Document exported symbols | Leave public API undocumented |
| Handle errors explicitly | Ignore errors with `_` without reason |
| Use `internal/` for private code | Expose implementation details |
| Prefer standard library | Add deps for trivial functionality |
