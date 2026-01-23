# Go Testing Reference

Testing patterns, benchmarks, and examples. Follows `foundations` testing principles.

## Basics

Tests live next to source: `foo.go` → `foo_test.go`

```go
func TestFunctionName(t *testing.T) {
    got := FunctionName()
    want := expected
    if got != want {
        t.Errorf("FunctionName() = %v, want %v", got, want)
    }
}
```

Run tests:
```bash
go test ./...           # all packages
go test -race ./...     # detect data races
go test -cover ./...    # coverage report
```

## Table-Driven Tests

Go's idiomatic pattern:

```go
func TestParse(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    int
        wantErr bool
    }{
        {"valid", "42", 42, false},
        {"invalid", "abc", 0, true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := Parse(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if got != tt.want {
                t.Errorf("Parse(%q) = %v, want %v", tt.input, got, tt.want)
            }
        })
    }
}
```

## Subtests and Helpers

```go
t.Run("subtest name", func(t *testing.T) {
    t.Parallel()  // run concurrently
})

func assertEqual(t *testing.T, got, want int) {
    t.Helper()  // errors report caller's line
    if got != want { t.Errorf("got %d, want %d", got, want) }
}
```

## Benchmarks

```go
func BenchmarkFunction(b *testing.B) {
    for i := 0; i < b.N; i++ {
        Function()
    }
}
```

Run: `go test -bench=. -benchmem`

## Example Functions

Executable documentation, verified by `go test`:

```go
func ExampleParse() {
    result, _ := Parse("42")
    fmt.Println(result)
    // Output: 42
}
```

## Testify (Optional)

```go
assert.Equal(t, expected, actual)
assert.NoError(t, err)
```

Use when readability improves; standard library is fine for simple checks.

## Always/Never

| Always | Never |
|--------|-------|
| Use table-driven tests for multiple cases | Duplicate test logic |
| Name test cases descriptively | Use `test1`, `test2` |
| Run tests with `-race` in CI | Ship without race detection |
| Use `t.Helper()` in test utilities | Obscure failure locations |
| Test edge cases (nil, empty, boundary) | Only test happy path |
| Use `t.Parallel()` where safe | Parallelize tests with shared state |
