package main

import (
	"sync"
	"time"
)

type LogEntry struct {
	Timestamp    time.Time `json:"timestamp"`
	Level        string    `json:"level"`
	Source       string    `json:"source"`
	Method       string    `json:"method"`
	Path         string    `json:"path"`
	Status       int       `json:"status"`
	Duration     string    `json:"duration"`
	Remote       string    `json:"remote"`
	Model        string    `json:"model,omitempty"`
	RequestBody  string    `json:"request_body,omitempty"`
	ResponseBody string    `json:"response_body,omitempty"`
}

type LogRing struct {
	mu       sync.RWMutex
	buf      []LogEntry
	head     int
	size     int
	capacity int
	subs     map[chan LogEntry]struct{}
}

func NewLogRing(capacity int) *LogRing {
	if capacity <= 0 {
		capacity = 2000
	}
	return &LogRing{
		buf:      make([]LogEntry, capacity),
		capacity: capacity,
		subs:     make(map[chan LogEntry]struct{}),
	}
}

func (lr *LogRing) Add(entry LogEntry) {
	lr.mu.Lock()
	lr.buf[lr.head] = entry
	lr.head = (lr.head + 1) % lr.capacity
	if lr.size < lr.capacity {
		lr.size++
	}
	// Copy subscribers to avoid holding lock during send
	subs := make([]chan LogEntry, 0, len(lr.subs))
	for ch := range lr.subs {
		subs = append(subs, ch)
	}
	lr.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- entry:
		default:
		}
	}
}

func (lr *LogRing) Snapshot() []LogEntry {
	lr.mu.RLock()
	defer lr.mu.RUnlock()

	if lr.size == 0 {
		return nil
	}

	result := make([]LogEntry, lr.size)
	start := lr.head - lr.size
	if start < 0 {
		start += lr.capacity
	}
	for i := 0; i < lr.size; i++ {
		idx := (start + i) % lr.capacity
		result[i] = lr.buf[idx]
	}
	return result
}

func (lr *LogRing) Subscribe() chan LogEntry {
	ch := make(chan LogEntry, 64)
	lr.mu.Lock()
	lr.subs[ch] = struct{}{}
	lr.mu.Unlock()
	return ch
}

func (lr *LogRing) Unsubscribe(ch chan LogEntry) {
	lr.mu.Lock()
	delete(lr.subs, ch)
	lr.mu.Unlock()
	close(ch)
}
