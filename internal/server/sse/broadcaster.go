// Package sse provides a thread-safe fan-out broadcaster for Server-Sent Events.
// It is used by the console observability handler to push real-time telemetry
// events to connected browser clients without polling.
package sse

import (
	"sync"
	"time"
)

// Event is a single SSE event sent to connected clients.
type Event struct {
	// Type is the SSE event name (e.g. "transaction", "pod_heartbeat", "budget_alert").
	Type string
	// Data is the JSON payload serialised as a string.
	Data string
}

// Broadcaster manages a set of SSE client channels and broadcasts events to all of them.
// Thread-safety: safe for concurrent use.
type Broadcaster struct {
	mu      sync.RWMutex
	clients map[chan Event]struct{}
}

// NewBroadcaster constructs an empty Broadcaster ready to accept subscribers.
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		clients: make(map[chan Event]struct{}),
	}
}

// Subscribe registers a new client channel and returns it.
// The caller must call Unsubscribe when the client disconnects to avoid goroutine
// and memory leaks.
// Each client channel is buffered with capacity 16 to absorb brief bursts.
func (b *Broadcaster) Subscribe() chan Event {
	ch := make(chan Event, 16)
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

// Unsubscribe removes a client channel and closes it.
// Calling Unsubscribe with a channel that is not registered is a no-op.
func (b *Broadcaster) Unsubscribe(ch chan Event) {
	b.mu.Lock()
	if _, ok := b.clients[ch]; ok {
		delete(b.clients, ch)
		close(ch)
	}
	b.mu.Unlock()
}

// Broadcast sends an event to all currently-subscribed clients.
// Clients that cannot receive within 1 second are skipped (non-blocking send with
// a timeout) so that a slow or disconnected client cannot stall the broadcaster.
func (b *Broadcaster) Broadcast(event Event) {
	b.mu.RLock()
	// snapshot the client set under a read lock to minimise lock contention.
	targets := make([]chan Event, 0, len(b.clients))
	for ch := range b.clients {
		targets = append(targets, ch)
	}
	b.mu.RUnlock()

	for _, ch := range targets {
		select {
		case ch <- event:
			// delivered successfully.
		case <-time.After(time.Second):
			// client is too slow; skip this event for this client.
		}
	}
}

// ClientCount returns the number of currently connected SSE clients.
func (b *Broadcaster) ClientCount() int {
	b.mu.RLock()
	n := len(b.clients)
	b.mu.RUnlock()
	return n
}
