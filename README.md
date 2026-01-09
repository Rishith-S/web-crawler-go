# Web Crawler - Lessons Learned

## Project Overview
A concurrent web crawler built in Go that crawls websites starting from a seed URL, respects robots.txt, and collects page titles and URLs.

> **Architecture**: See [ARCHITECTURE.md](ARCHITECTURE.md) for detailed system design and data flow diagrams.



## Key Lessons Learned

### 1. Understanding Unbuffered Channels
**Problem**: Initial implementation had a deadlock where `fetcher` was called synchronously and tried to send to a channel, but the receiver was in the same goroutine.

**Lesson**: 
- Unbuffered channels require both sender and receiver to be ready simultaneously
- Send operation `c <- data` blocks until another goroutine receives
- You can't send and receive on the same goroutine with an unbuffered channel
- **Solution**: Either use `go fetcher()` to run in separate goroutine, or use buffered channel `make(chan T, 1)`

### 2. Always Send to Channels
**Problem**: When HTTP requests failed, `fetcher` never sent anything to the channel, causing the receiver to block forever.

**Lesson**: 
- If a goroutine is expected to send to a channel, it must ALWAYS send something (even nil on error)
- Or use patterns like closing the channel, sending error types, or using select with timeout
- **Solution**: Added `c <- nil` after all retry attempts failed

### 3. Return Statement Placement in Loops
**Problem**: `return` was inside the `for` loop, so retry logic only attempted once.

**Lesson**:
- Be careful with early returns in loops - they skip remaining iterations
- Place `return` after the loop if you want all iterations to complete first
- **Solution**: Moved `return` outside the retry loop, only return early on success

### 4. URL Handling and Validation
**Problem**: Parser was trying to fetch invalid URLs like `#maincontent`, `javascript:`, `mailto:` links.

**Lesson**:
- Always validate and normalize URLs before enqueueing
- Filter out anchor links (`#`), javascript links, mailto links
- Convert relative URLs (`/about`) to absolute URLs (`https://example.com/about`)
- Only crawl URLs from the same domain to avoid infinite crawling
- **Solution**: Added URL validation and conversion logic in parser

### 5. Concurrent vs Synchronous Execution
**Problem**: Making parser concurrent didn't improve performance.

**Lesson**:
- **Identify the bottleneck** before optimizing
- Network I/O (fetching): 100-1000ms - **SLOW**
- CPU operations (parsing): 1-10ms - **FAST**
- Making the fast part concurrent while the slow part is sequential gives minimal gains
- **Real solution**: Concurrent fetching with worker pools, not concurrent parsing

### 6. Channel-URL Mismatch Problem
**Problem**: Using a single channel for multiple concurrent fetchers causes URL-document mismatch.

**Lesson**:
- When launching multiple goroutines that send to a shared channel, you can't guarantee which response corresponds to which request
- **Solution**: Create a new channel for each fetch operation

### 7. Race Conditions with Shared State
**Problem**: Multiple goroutines accessing `queue` and `crawledSet` can cause:
- Same URL crawled multiple times
- Queue size calculations while links are being added

**Lesson**:
- Even with mutexes protecting individual operations, logic-level races can occur
- Check-then-act patterns (`if !contains() { add() }`) have race windows
- **Solution**: Use mutexes around entire check-and-modify sequences, or atomic operations

### 8. Understanding Performance Bottlenecks

**Sequential Fetching (Current)**:
```
URL1 fetch (1000ms) → parse (5ms) → URL2 fetch (1000ms) → parse (5ms)
Total: 2010ms for 2 URLs
```

**Concurrent Fetching (Better)**:
```
URL1 fetch (1000ms) ┐
URL2 fetch (1000ms) ├─ All in parallel
URL3 fetch (1000ms) ┘
Total: ~1000ms for 3 URLs
```

**Lesson**: Concurrency helps when you have I/O-bound operations (network, disk). Use worker pools to limit concurrent requests and avoid overwhelming the server.

## Concurrency Patterns Implemented

### Mutex-Protected Data Structures
```go
type Queue struct {
    elements []string
    mu       sync.Mutex
}

func (q *Queue) enqueue(url string) {
    q.mu.Lock()
    defer q.mu.Unlock()
    q.elements = append(q.elements, url)
}
```

### Channel Communication Between Goroutines
```go
c := make(chan *goquery.Document)
go fetcher(host, c)
content := <-c  // Synchronizes sender and receiver
```

### Dedicated Channels Per Operation
```go
fetchChan := make(chan *goquery.Document)
go fetcher(url, fetchChan)
content := <-fetchChan  // Each fetch has its own channel
```

## Best Practices Applied

1. **Respect robots.txt** - Fetch and parse disallowed paths
2. **Set User-Agent header** - Identify your bot
3. **Retry with exponential backoff** - Handle temporary failures
4. **Request timeouts** - Don't wait forever
5. **Limit redirect depth** - Prevent redirect loops
6. **Mutex protection** - Guard shared data structures
7. **Resource cleanup** - Close response bodies, use defer

## Technologies Used
- **Go** - Concurrency primitives (goroutines, channels, mutexes)
- **goquery** - HTML parsing and DOM traversal
- **net/http** - HTTP client with custom configuration

## Running the Crawler
```bash
go run main.go
```

Output is written to `result.txt` with page titles and URLs.

## Future Improvements
- [ ] Implement concurrent fetching with worker pool
- [ ] Add rate limiting to be polite to servers
- [ ] Use buffered channels to decouple fetching and parsing
- [ ] Better error handling (don't use log.Fatal in library code)
- [ ] Add URL deduplication at enqueue time
- [ ] Implement proper shutdown mechanism
- [ ] Add metrics and monitoring

### TODO: Implement Worker Pool Pattern
```go
maxWorkers := 10
semaphore := make(chan struct{}, maxWorkers)

for queue.size() > 0 {
    url := queue.dequeue()
    semaphore <- struct{}{}  // Acquire worker slot
    go func(u string) {
        defer func() { <-semaphore }()  // Release slot
        // fetch and parse
    }(url)
}
```

### TODO: Use WaitGroup for Goroutine Tracking
```go
var wg sync.WaitGroup
wg.Add(1)
go func() {
    defer wg.Done()
    // work
}()
wg.Wait() // Wait for all goroutines to complete
```

### Other Improvements
- [ ] Implement concurrent fetching with worker pool (see above)
- [ ] Add WaitGroup to track all running goroutines
- [ ] Add rate limiting to be polite to servers
- [ ] Use buffered channels to decouple fetching and parsing
- [ ] Better error handling (don't use log.Fatal in library code)
- [ ] Add URL deduplication at enqueue time
- [ ] Implement proper shutdown mechanism
- [ ] Add metrics and monitoring
- [ ] Handle edge cases (empty queue while parsers still running)