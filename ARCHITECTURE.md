# Web Crawler Architecture

## System Architecture

```
┌─────────────────┐
│     Start       │
│                 │
│ • seed URL      │
│ • robots.txt    │
└────────┬────────┘
         │
         ├──────────────────────────────────┐
         │                                  │
         ▼                                  │
┌─────────────────┐                         │
│  Fetch URLs     │                         │
│                 │                         │
│ • sequentially  │                         │
│   (1 at a time) │                         │
│ • retry 5x      │                         │
│ • 10s timeout   │                         │
└────────┬────────┘                         │
         │                                  │
         ▼                                  │
┌─────────────────────────┐                 │
│  Parse Content          │                 │
│                         │                 │
│ • extract links         │                 │
│ • validate URLs         │                 │
│ • convert to absolute   │                 │
│ • filter by domain      │                 │
│ • check robots.txt      │                 │
└────────┬────────────────┘                 │
         │                                  │
         ├──────────┬─────────────┐         │
         │          │             │         │
         ▼          ▼             ▼         │
┌─────────────┐ ┌─────────┐ ┌──────────┐   │
│    Queue    │ │ Crawled │ │result.txt│   │
│             │ │   Set   │ │          │   │
│ • URLs to   │ │         │ │ • title  │   │
│   crawl     │ │ • hashed│ │ • URL    │   │
│ • thread-   │ │   URLs  │ └──────────┘   │
│   safe      │ │ • thread│                │
│ • mutex     │ │   safe  │                │
│   locked    │ │ • mutex │                │
└─────┬───────┘ └─────────┘                │
      │                                     │
      └─────────────────────────────────────┘
           dequeue(url) - loop continues
           until size >= 500 or queue empty
```

## Data Flow

1. **Initialization**: Load robots.txt, create queue & crawled set
2. **Fetch**: HTTP GET with retries and exponential backoff
3. **Parse**: Extract `<a href>` tags, validate, normalize URLs
4. **Enqueue**: Add new URLs to queue (if not crawled and allowed)
5. **Track**: Mark URL as crawled in CrawledSet (hash-based)
6. **Store**: Write title and URL to result.txt
7. **Repeat**: Dequeue next URL until limit reached

## Key Components

### Queue (Thread-Safe)
- Stores URLs to be crawled
- Mutex-protected operations
- Tracks total queued count

### CrawledSet (Thread-Safe)
- Tracks visited URLs using hash (FNV-64a)
- Prevents duplicate crawling
- Mutex-protected operations

### Fetcher
- HTTP client with timeout & redirect limits
- Retry logic with exponential backoff
- Always sends result (doc or nil) to channel

### Parser
- Validates URLs (filters anchors, mailto, javascript)
- Converts relative → absolute URLs
- Enforces domain boundaries
- Respects robots.txt rules

## Current Implementation Characteristics

**Sequential Processing**:
- One URL fetched at a time
- Main loop waits for each fetch to complete
- Simple, predictable flow
- No race conditions

**Thread-Safe Components**:
- Queue operations protected by mutex
- CrawledSet operations protected by mutex
- File writes protected by mutex
- Safe for concurrent access (ready for parallel fetching)

## Potential Improvements

See [README.md](README.md#future-improvements) for planned architectural enhancements including worker pools and concurrent fetching.
