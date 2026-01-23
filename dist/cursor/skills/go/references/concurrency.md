# Go Concurrency Reference

Goroutines, channels, and synchronization primitives.

## Philosophy

> "Do not communicate by sharing memory; share memory by communicating."

Channels are first-class citizens for coordinating goroutines.

## Goroutines

Lightweight threads managed by Go runtime. Thousands are cheap.

```go
go doWork()
go func() { /* work */ }()
```

**Key points:** Share address space (data races possible), coordinate via channels, main exiting kills all.

## Channels

### Unbuffered vs Buffered

| Type | Declaration | Blocking |
|------|-------------|----------|
| Unbuffered | `make(chan int)` | Send blocks until receive |
| Buffered | `make(chan int, 10)` | Send blocks when full |

### Operations and Direction

```go
ch <- v           // send
v := <-ch         // receive
close(ch)         // close (sender only)

func send(ch chan<- int)  // send-only parameter
func recv(ch <-chan int)  // receive-only parameter
```

## Patterns

### Worker Pool

```go
for w := 1; w <= numWorkers; w++ {
    go worker(jobs, results)
}
```

### Semaphore

```go
sem := make(chan struct{}, maxConcurrent)
sem <- struct{}{}       // acquire
defer func() { <-sem }() // release
```

### Timeout with Select

```go
select {
case result := <-ch:
    return result, nil
case <-time.After(timeout):
    return nil, errors.New("timeout")
case <-ctx.Done():
    return nil, ctx.Err()
}
```

## sync Package

For when channels aren't the right tool:

| Type | Use Case |
|------|----------|
| `sync.Mutex` | Protect shared state |
| `sync.WaitGroup` | Wait for goroutines |
| `sync.Once` | One-time init |

### WaitGroup

```go
var wg sync.WaitGroup
for _, item := range items {
    wg.Add(1)
    go func(i Item) {
        defer wg.Done()
        process(i)
    }(item)
}
wg.Wait()
```

## Always/Never

| Always | Never |
|--------|-------|
| Close channels from sender | Close from receiver |
| Use context for cancellation | Let goroutines leak |
| Range over channels to detect close | Forget to close when done sending |
| Capture loop variables in closures | Share loop variable across goroutines |
| Use sync.Mutex for simple state | Use channels for simple counters |
| Document goroutine ownership | Create goroutines without clear lifecycle |
